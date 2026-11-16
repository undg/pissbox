package github

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

const summaryFields = `number title url state isDraft author { login } createdAt updatedAt
baseRefName headRefName headRefOid reviewDecision
commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }`

const listQuery = `query($owner: String!, $name: String!, $states: [PullRequestState!], $limit: Int!, $cursor: String) {
repository(owner: $owner, name: $name) {
pullRequests(states: $states, first: $limit, after: $cursor, orderBy: {field: UPDATED_AT, direction: DESC}) {
nodes { ` + summaryFields + ` }
pageInfo { hasNextPage endCursor }
} } }`

const viewQuery = `query($owner: String!, $name: String!, $number: Int!) {
repository(owner: $owner, name: $name) { pullRequest(number: $number) {
` + summaryFields + `
body closedAt mergedAt mergeable mergeStateStatus additions deletions changedFiles
reviewRequests(first: 50) { totalCount nodes { requestedReviewer {
... on User { login } ... on Team { name slug }
} } }
} } }`

const checksQuery = `query($owner: String!, $name: String!, $number: Int!, $limit: Int!, $cursor: String) {
repository(owner: $owner, name: $name) { pullRequest(number: $number) {
headRefOid commits(last: 1) { nodes { commit { statusCheckRollup {
state contexts(first: $limit, after: $cursor) {
nodes { __typename
... on CheckRun { name status conclusion detailsUrl startedAt completedAt checkSuite { workflowRun { url workflow { name } } } }
... on StatusContext { context state description targetUrl createdAt }
}
pageInfo { hasNextPage endCursor }
} } } } }
} } }`

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type connection[T any] struct {
	Nodes    []T      `json:"nodes"`
	PageInfo pageInfo `json:"pageInfo"`
}

type pullRequest struct {
	Number           int     `json:"number,omitempty"`
	Title            string  `json:"title,omitempty"`
	URL              string  `json:"url,omitempty"`
	State            string  `json:"state,omitempty"`
	IsDraft          bool    `json:"isDraft"`
	Author           *actor  `json:"author,omitempty"`
	CreatedAt        string  `json:"createdAt,omitempty"`
	UpdatedAt        string  `json:"updatedAt,omitempty"`
	BaseRefName      string  `json:"baseRefName,omitempty"`
	HeadRefName      string  `json:"headRefName,omitempty"`
	HeadRefOid       string  `json:"headRefOid"`
	ReviewDecision   string  `json:"reviewDecision,omitempty"`
	Body             *string `json:"body,omitempty"`
	NextOffset       int     `json:"nextOffset,omitempty"`
	ClosedAt         string  `json:"closedAt,omitempty"`
	MergedAt         string  `json:"mergedAt,omitempty"`
	Mergeable        string  `json:"mergeable,omitempty"`
	MergeStateStatus string  `json:"mergeStateStatus,omitempty"`
	Additions        int     `json:"additions,omitempty"`
	Deletions        int     `json:"deletions,omitempty"`
	ChangedFiles     int     `json:"changedFiles,omitempty"`
	ReviewRequests   *struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			RequestedReviewer struct {
				Login string `json:"login,omitempty"`
				Name  string `json:"name,omitempty"`
				Slug  string `json:"slug,omitempty"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests,omitempty"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State    string                       `json:"state"`
					Contexts *connection[json.RawMessage] `json:"contexts,omitempty"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

func graphql(ctx context.Context, op string, r Request) (any, error) {
	parts := strings.SplitN(r.Repo, "/", 2)
	query := viewQuery
	if op == "pr_list" {
		query = listQuery
	} else if op == "pr_checks" {
		query = checksQuery
	}
	args := []string{"api", "--hostname", "github.com", "graphql", "-f", "query=" + query, "-f", "owner=" + parts[0], "-f", "name=" + parts[1]}
	if op == "pr_list" {
		states := []string{"OPEN"}
		switch r.State {
		case "closed":
			states = []string{"CLOSED"}
		case "merged":
			states = []string{"MERGED"}
		case "all":
			states = []string{"OPEN", "CLOSED", "MERGED"}
		}
		for _, state := range states {
			args = append(args, "-f", "states[]="+state)
		}
	} else {
		args = append(args, "-F", "number="+strconv.Itoa(r.Number))
	}
	if op != "pr_view" {
		args = append(args, "-F", "limit="+strconv.Itoa(pageSize(r.Limit)))
		if r.Cursor != "" {
			args = append(args, "-f", "cursor="+r.Cursor)
		}
	}
	data, err := run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Repository *struct {
				PullRequests *connection[pullRequest] `json:"pullRequests"`
				PullRequest  *pullRequest             `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(data, &response); err != nil || len(response.Errors) != 0 || response.Data.Repository == nil {
		return nil, errors.New("GitHub query failed or returned incomplete data; check host gh auth and repository access")
	}
	repo := response.Data.Repository
	if op == "pr_list" {
		if repo.PullRequests == nil || repo.PullRequests.Nodes == nil {
			return nil, errors.New("invalid GitHub PR list response")
		}
		return repo.PullRequests, nil
	}
	pr := repo.PullRequest
	if pr == nil {
		return nil, errors.New("GitHub PR not found or inaccessible")
	}
	if pr.HeadRefOid == "" {
		return nil, errors.New("invalid GitHub PR response: missing head commit")
	}
	if op == "pr_view" {
		if pr.Body == nil {
			return nil, errors.New("invalid GitHub PR view response")
		}
		body, next, err := chunk(*pr.Body, r.Offset, 12000)
		if err != nil {
			return nil, err
		}
		pr.Body, pr.NextOffset = &body, next
		return pr, nil
	}
	result := struct {
		HeadRefOid string `json:"headRefOid"`
		State      string `json:"state"`
		connection[json.RawMessage]
	}{HeadRefOid: pr.HeadRefOid, State: "NONE", connection: connection[json.RawMessage]{Nodes: []json.RawMessage{}}}
	if len(pr.Commits.Nodes) > 0 {
		rollup := pr.Commits.Nodes[0].Commit.StatusCheckRollup
		if rollup != nil {
			if rollup.Contexts == nil || rollup.Contexts.Nodes == nil {
				return nil, errors.New("invalid GitHub checks response")
			}
			result.State, result.connection = rollup.State, *rollup.Contexts
		}
	}
	return result, nil
}
