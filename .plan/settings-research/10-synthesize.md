# 10 — Synthesize results into one recommendation

Read `.plan/README.md` (shared context) and `AGENTS.md` first. Then read every
`.plan/results/*.md` that exists. List missing result files explicitly; do not
invent their content.

## Mission

Produce one recommended design with a minimal implementable path, plus a
runner-up and rejected options with reasons. Design only.

## Questions

1. Which layout best fits: shared sessions/extensions/auth, two workstation
   provider profiles, minimal machinery, low git churn?
2. Resolve contradictions between result files; flag any unsourced claims.
3. Numbered implementation plan: files to change, exact validation commands,
   tests to add, new-sandbox requirement.
4. Open questions only the user can answer.

## Constraints

- Repo validation expectations: `sbx kit validate ./sbx-kit`,
  `bash -n bin/pissbox`, `git diff --check`,
  `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v`.
- No code changes.

## Deliverable

`.plan/results/10-synthesize.md`: recommendation, runner-up, rejected options,
implementation plan, open questions.
