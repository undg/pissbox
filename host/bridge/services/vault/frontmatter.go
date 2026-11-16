package vault

import (
	"errors"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var frontmatter = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---(?:\r?\n|$)`)
var inlineTag = regexp.MustCompile(`(?:^|[\s(])#([\p{L}\p{N}_/-]+)`)
var validTag = regexp.MustCompile(`^[\p{L}\p{N}_-]+(/[\p{L}\p{N}_-]+)*$`)

func newFrontmatter(name, body string, tags, aliases []string) (string, error) {
	for _, tag := range tags {
		if !validTag.MatchString(tag) {
			return "", errors.New("invalid tag")
		}
	}
	data, err := yaml.Marshal(struct {
		ID      string   `yaml:"id"`
		Aliases []string `yaml:"aliases"`
		Tags    []string `yaml:"tags"`
	}{strings.TrimSuffix(path.Base(name), ".md"), append([]string{}, aliases...), append([]string{}, tags...)})
	if err != nil {
		return "", err
	}
	return "---\n" + string(data) + "---\n\n" + body, nil
}

func parseFrontmatter(content string) (*yaml.Node, []int, error) {
	span := frontmatter.FindStringSubmatchIndex(content)
	if span == nil {
		if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
			return nil, nil, errors.New("unclosed frontmatter")
		}
		return nil, nil, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content[span[2]:span[3]]), &doc); err != nil {
		return nil, nil, errors.New("invalid YAML frontmatter")
	}
	if len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode}, span, nil
	}
	node := doc.Content[0]
	if node.Kind != yaml.MappingNode {
		return nil, nil, errors.New("frontmatter must be a mapping")
	}
	// Decode once to reject duplicate keys; aliases are not accepted as tags.
	var check map[string]any
	if err := node.Decode(&check); err != nil {
		return nil, nil, errors.New("invalid or duplicate frontmatter keys")
	}
	return node, span, nil
}
func tagValues(node *yaml.Node) ([]string, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil, nil
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		return strings.Fields(strings.ReplaceAll(node.Value, ",", " ")), nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, errors.New("tags must be strings or a sequence of strings")
	}
	var tags []string
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return nil, errors.New("tags must contain strings")
		}
		tags = append(tags, item.Value)
	}
	return tags, nil
}
func tagsIn(content string) ([]string, error) {
	node, _, err := parseFrontmatter(content)
	if err != nil {
		return nil, err
	}
	unique := map[string]bool{}
	var result []string
	add := func(tag string) {
		if !unique[tag] {
			unique[tag] = true
			result = append(result, tag)
		}
	}
	if node != nil {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == "tags" {
				tags, err := tagValues(node.Content[i+1])
				if err != nil {
					return nil, err
				}
				for _, tag := range tags {
					add(tag)
				}
			}
		}
	}
	for _, match := range inlineTag.FindAllStringSubmatch(content, -1) {
		add(match[1])
	}
	return result, nil
}
func changeTag(content, tag, action string) (string, error) {
	if !validTag.MatchString(tag) || (action != "add" && action != "remove") {
		return "", errors.New("expected a valid tag and add/remove action")
	}
	node, span, err := parseFrontmatter(content)
	if err != nil {
		return "", err
	}
	if node == nil {
		if action == "remove" {
			return content, nil
		}
		return "---\ntags: [" + tag + "]\n---\n" + content, nil
	}
	if node.Style&yaml.FlowStyle != 0 {
		return "", errors.New("edit flow-style frontmatter explicitly with vault_edit")
	}
	index := -1
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == "tags" {
			index = i
			break
		}
	}
	var tags []string
	if index >= 0 {
		tags, err = tagValues(node.Content[index+1])
		if err != nil {
			return "", err
		}
	}
	updated := make([]string, 0, len(tags)+1)
	found := false
	for _, t := range tags {
		if t == tag {
			found = true
			if action == "remove" {
				continue
			}
		}
		updated = append(updated, t)
	}
	if action == "add" && !found {
		updated = append(updated, tag)
	}
	if (action == "add" && found) || (action == "remove" && !found) {
		return content, nil
	}
	return replaceTagField(content, node, span, index, updated)
}

func replaceTagField(content string, node *yaml.Node, span []int, index int, tags []string) (string, error) {
	// Encode a single field, not the document, so other fields and the body
	// retain their original bytes (including comments and CRLF).
	list := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, tag := range tags {
		list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: tag})
	}
	encoded, err := yaml.Marshal(list)
	if err != nil {
		return "", err
	}
	newline := "\n"
	if strings.Contains(content[:span[1]], "\r\n") {
		newline = "\r\n"
	}
	replacement := "tags: " + strings.TrimSpace(string(encoded))
	header := content[span[2]:span[3]]
	if index < 0 {
		return content[:span[3]] + newline + replacement + content[span[3]:], nil
	}
	lines := strings.Split(header, newline)
	start := node.Content[index].Line - 1
	end := len(lines)
	if index+2 < len(node.Content) {
		end = node.Content[index+2].Line - 1
	}
	// Keep blank lines and comments preceding the next field (or delimiter).
	for end > start+1 {
		line := strings.TrimSpace(lines[end-1])
		if line != "" && !strings.HasPrefix(line, "#") {
			break
		}
		end--
	}
	result := append([]string{}, lines[:start]...)
	result = append(result, replacement)
	result = append(result, lines[end:]...)
	return content[:span[2]] + strings.Join(result, newline) + content[span[3]:], nil
}
