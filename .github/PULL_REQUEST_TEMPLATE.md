<!--
The PR title MUST be a Conventional Commit — it becomes the squash commit and drives
release-please (version + changelog). e.g. "feat: ...", "fix(tokenizer): ...", "docs: ...".
-->

## What & why

<!-- What does this change and why? Link issues (e.g. "Closes #123"). -->

## Checklist

- [ ] `mise run check` is green (`go build`, `go vet`, `gofmt -l .` empty, `go test -race ./...`)
- [ ] New behavior has tests; ported behavior reuses the upstream `.sql` fixtures
- [ ] Ported 1:1 from `.reference/sqlglot-v30.12.0/` where applicable (cite reference lines for any divergence)
- [ ] Any intentional difference from upstream is recorded in `DEVIATIONS.md` with a rationale
- [ ] PR title is a Conventional Commit
- [ ] Did **not** hand-edit `CHANGELOG.md` or version tags (release-please owns those)
