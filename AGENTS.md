# sqlglot-go — agent guide

A Go port of **[tobymao/sqlglot](https://github.com/tobymao/sqlglot) v30.12.0** (a pure-Python SQL
parser, generator, and optimizer). It ports the **parse → AST → generate** core ~1:1 (tokenizer, AST,
parser, generator, schema) plus the `qualify` and `scope` optimizer passes that column qualification
and **lineage** build on. It is **not** a full port of sqlglot: the rest of the optimizer (simplify,
normalize, pushdown/eliminate/merge/unnest passes, the `optimize()` orchestrator), cross-dialect
transpilation, and dialects beyond **base + MySQL + Postgres** are out of scope for now. This repo is
the SQL engine only; it has no application-specific code.

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

## How deviations are tracked (READ before intentionally diverging)

Every place the port *intentionally* behaves differently from upstream is recorded in
**[DEVIATIONS.md](./DEVIATIONS.md)** — the single ledger, grouped by how observable the divergence is
(§1 = *changes same-dialect parse→generate output*; §2+ = cross-dialect-only / output-preserving / scope
boundary). It complements the per-site code comments (grep `divergence` / `Unlike upstream`) and
ROADMAP.md's known-divergences + resolved-findings ledgers. Two kinds of divergence, two disciplines:

- **Correctness fixes — upstream is wrong vs the real engine.** When upstream's parsing/tokenizing/folding
  disagrees with the actual DB, sqlglot-go matches the DB, not upstream. Precedents: §1.1 (ASCII identifier
  fold — upstream over-folds with full-Unicode `str.lower()`) and §1.4 (MySQL `--` requires a trailing
  space; upstream mis-tokenizes `1--2`). Discipline: add a **DEVIATIONS §1 entry** + a `divergence` code
  comment citing the real-engine behavior + a test asserting the port matches the **DB** (not upstream). No
  tripwire needed — we *want* to stay diverged; if upstream later fixes it, the test still passes.
- **Grammar beyond upstream — constructs upstream does not parse.** Permitted, but each must be (a) correct
  (round-trip + AST-shape asserted) and (b) tracked so an upstream bump can't silently collide. The plan
  (not yet built — no such construct is ported today) is an **extension ledger** (`testdata/upstream_
  extensions.jsonl`) listing each construct pinned upstream does *not* parse, plus a `.reference`-gated
  **pin-tripwire test** that re-asserts upstream's behavior so the next reference bump fails loudly with a
  reconciliation note. Model each extension on upstream's likely eventual node (reuse a Kind/family).

Do **not** invent a same-dialect deviation for convenience: the default is faithfulness. A deviation needs a
correctness rationale (matches the DB) or an explicit, tracked feature decision — and a DEVIATIONS entry.

## Current status

`go test ./...` is green. The parse → generate pipeline is at **100% round-trip parity** for base +
MySQL + Postgres — **1847/1847** identity-corpus cases (base 955/955, MySQL 424/424, Postgres 468/468),
enforced by a monotonic corpus floor (`corpus_test.go` + `testdata/parity_gaps.txt`, now empty). AST
fidelity is enforced too: no statement fakes a round-trip via a raw-text `exp.Command` where upstream
builds a structured node (`fidelity_test.go` + `testdata/fidelity_cases.txt`). Working for base + MySQL
+ Postgres: the tokenizer, the AST + node model, the parser, the generator (`Expression → SQL`),
`schema.MappingSchema` + `DataType.build`, the `qualify` pass (qualify_tables → normalize_identifiers →
qualify_columns → quote_identifiers → validate), and `traverse_scope` + the full `Scope` API.

**Remaining work** (see `ROADMAP.md`): the rest of sqlglot's optimizer — `simplify` (full),
`normalize`, `pushdown_predicates`/`pushdown_projections`, `eliminate_*`, `merge_subqueries`,
`unnest_subqueries`, `optimize_joins`, `canonicalize`, and the top-level `optimize()` orchestrator;
full `annotate_types`; verified cross-dialect transpilation; and dialects beyond base/MySQL/Postgres.
The parse/generate round-trip itself is done — a construct upstream parses that this port doesn't is a
regression, not expected.

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
