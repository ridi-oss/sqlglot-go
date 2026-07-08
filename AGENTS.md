# sqlglot-go — agent guide

A faithful, near-1:1 Go port of **[tobymao/sqlglot](https://github.com/tobymao/sqlglot) v30.12.0**
(a pure-Python SQL parser, generator, and optimizer). The goal is behavioral parity with upstream
for **base + MySQL + Postgres** — tokenizer, AST, parser, generator, schema, and the optimizer
passes. This repo is the SQL engine only; it has no application-specific code.

## Source of truth (READ THIS FIRST, always)

- The pinned Python source is fetched to **`.reference/sqlglot-v30.12.0/`** (gitignored — run
  `scripts/fetch-reference.sh` once). It is the **exact** upstream version being ported
  (`sqlglot==30.12.0`, git SHA in `.reference/sqlglot-v30.12.0/GIT_SHA.txt`).
- Port from this reference, file by file, **as 1:1 as possible** — same file layout, same
  function/method names (Go-cased), same structure, same comments where they carry intent. When Go
  forces a divergence (static typing, no metaclasses, error/panic instead of exceptions), keep it
  minimal and note *why* in a comment that cites the reference line.
- **Port the corresponding unit tests too**, 1:1, from `.reference/sqlglot-v30.12.0/tests/`. The
  upstream tests and `tests/fixtures/*.sql` are the correctness oracle — reuse the `.sql` fixtures
  verbatim (they live under each package's `testdata/`), reimplement the loader/assertions in Go.

## Current status

`go test ./...` is green. Working for base + MySQL + Postgres: the tokenizer, the AST + node model,
the generator (`Expression → SQL`), `schema.MappingSchema` + `DataType.build`, and the optimizer
passes `qualify` (qualify_tables → normalize_identifiers → qualify_columns → quote_identifiers →
validate) and `traverse_scope` + the full `Scope` API.

**Remaining work = parser/feature parity with upstream** (see `ROADMAP.md`): the parser tail (table-
valued function sources like `generate_series(...)` in FROM, `ARRAY[...]` literals, `JSON_TABLE`,
`SIMILAR TO`, `FROM ONLY`, `CONNECT BY`, the long function registry tail, DDL constraint detail),
full `annotate_types`, and per-dialect parser/generator overrides. Anything upstream sqlglot parses
should parse here too; a construct that doesn't parse yet is a gap to close, not a feature.

## Central design decision — the AST node model

Upstream `Expression` is dynamically typed: an `args: dict[str, Any]` of children
(node | list | str | bool | None), a per-class `arg_types` map, a metaclass dialect registry, and
heavy reflection (`node.key`, `find_all(*types)` via isinstance). The parser (~10k LOC) and generator
(~6k LOC) manipulate every node generically through `args`. The Go port mirrors this with a **single
`*Node` struct** behind an `Expression` interface, discriminated by a `Kind` enum, with per-Kind
metadata *tables* in `expressions/kinds.go` (ordered arg keys / traits / class name). Adding a node
type = one `Kind` const + one row in each table + a one-line builder — nodes are **data**, not ~300
structs. This keeps the generic parser/generator/optimizer code a close 1:1 of the Python.

## How to continue the port

1. `scripts/fetch-reference.sh` to get the pinned Python source (needed for parity + as the oracle).
2. Read `ROADMAP.md` — it lists the remaining slices (**1d** parser tail, **4c** full
   `annotate_types`, **5b** per-dialect parser/generator override tables) and, crucially, the
   **known divergences** + **resolved-findings** ledger so you don't re-litigate settled decisions.
3. Pick a slice, port from `.reference/` 1:1, port its tests, keep `go test ./...` green.
4. Verify against upstream: port the matching upstream tests, and for parser/generator work do a
   differential check against the pinned Python, e.g.
   `PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c "import sqlglot; print(repr(sqlglot.parse_one('…','postgres')))"`
   and compare the AST / `.sql()` round-trip to the Go output.

This port was built with a multi-model review pipeline (plan → implement → integrate → adversarial
review), verifying every review finding against the pinned source before acting. Keep that rigor:
confirm a claimed bug against `.reference/` before "fixing" it — some findings are phantom.

## Conventions

- Go 1.23. Module `github.com/sjincho/sqlglot-go`. Zero third-party deps (stdlib + `testing` only).
- Comments in **English**, US spelling (`canceled`, `color`, `catalog`).
- `gofmt` + `go vet` clean; `go test ./...` green before any commit/push.
- Package layout mirrors the Python module layout (`expressions/`, `optimizer/`, `dialects/`,
  `generator/`, `parser/`, `tokens/`, `schema/`, …).
