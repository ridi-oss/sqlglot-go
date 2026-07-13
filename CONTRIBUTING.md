# Contributing to sqlglot-go

Thanks for your interest! sqlglot-go is a faithful Go port of
[tobymao/sqlglot](https://github.com/tobymao/sqlglot) **v30.12.0**. Please read
**[AGENTS.md](./AGENTS.md)** first — it is the canonical guide to how the port is built and what is in
scope. This file is the quick contributor checklist.

## Prerequisites

- **Go 1.23+** (toolchain pinned in `mise.toml`; `mise install` if you use [mise](https://mise.jdx.dev)).
- No third-party dependencies — the standard library only.

## Build, test, lint

Tasks are defined in `mise.toml` (run `mise tasks` to list):

```bash
mise run check      # build + vet + gofmt-check + race tests (the pre-push gate)
# or individually:
mise run build      # go build ./...
mise run test       # go test ./...
mise run vet        # go vet ./...
mise run fmt        # gofmt -w .   (fmt-check fails if anything needs formatting)
mise run lint       # golangci-lint (optional locally; CI runs it on new code)
```

`go test ./...` must be green before every push. A round-trip parity corpus (`corpus_test.go`) and an
AST-fidelity gate (`fidelity_test.go`) are **monotonic** — they can only tighten. CI runs the same
checks across Go 1.23 and latest stable.

## The source-of-truth rule

Port **from the pinned Python** at `.reference/sqlglot-v30.12.0/` (run `make reference` once to fetch
it — it is gitignored), **file-by-file, as close to 1:1 as Go allows**. Port the matching upstream tests
and reuse the `.sql` fixtures verbatim. When Go forces a divergence, keep it minimal and cite the
reference line in a comment.

Every *intentional* difference from upstream is recorded in **[DEVIATIONS.md](./DEVIATIONS.md)** — a
same-dialect behavior change needs a correctness rationale (it matches the real database) **or** a
tracked feature decision, never convenience. New grammar beyond upstream must be round-trip + AST-shape
asserted and added to the extension ledger (`testdata/upstream_extensions.jsonl`).

## Commits & pull requests

This project uses **[Conventional Commits](https://www.conventionalcommits.org/)** —
[release-please](https://github.com/googleapis/release-please) derives the next version and the
changelog from them, so the format is required.

- PRs are **squash-merged**, and the **PR title becomes the commit** — so the **PR title must be a
  Conventional Commit**. CI lints it.
- Types: `feat` (→ minor, pre-1.0), `fix` (→ patch), `perf`, `docs`, `refactor`, `test`, `build`,
  `ci`, `chore`, `revert`. A breaking change uses `type!:` or a `BREAKING CHANGE:` footer.
- Examples: `feat: add opt-in search-path qualification`, `fix(tokenizer): require space after --`,
  `docs: clarify DEVIATIONS §1.5`.

Checklist before opening a PR:

1. `mise run check` is green; `gofmt -l .` is empty.
2. New behavior has tests; ported behavior reuses the upstream fixtures.
3. Any intentional divergence is in `DEVIATIONS.md` with a rationale.
4. The PR title is a Conventional Commit.

Do **not** hand-edit `CHANGELOG.md` or version tags — release-please owns those (see
[AGENTS.md § Releasing](./AGENTS.md#releasing)).

## License

By contributing, you agree that your contributions are licensed under the [MIT License](./LICENSE).
