# sqlglot-go

[![CI](https://github.com/ridi-oss/sqlglot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/ridi-oss/sqlglot-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ridi-oss/sqlglot-go.svg)](https://pkg.go.dev/github.com/ridi-oss/sqlglot-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/ridi-oss/sqlglot-go)](https://goreportcard.com/report/github.com/ridi-oss/sqlglot-go)
[![Release](https://img.shields.io/github/v/release/ridi-oss/sqlglot-go)](https://github.com/ridi-oss/sqlglot-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

A Go port of the **parse → AST → generate** core of
[tobymao/sqlglot](https://github.com/tobymao/sqlglot) v30.12.0, plus the `qualify` and `scope`
optimizer passes that column **qualification and lineage** are built on. Ported file-by-file from the
pinned Python source, as close to 1:1 as Go allows, so upstream tests port across directly. **Zero
third-party dependencies** (Go stdlib only).

## Scope — what this is (and isn't)

This is **not** a port of all of sqlglot. It ports the front half of the pipeline in full and a
targeted slice of the optimizer:

**Ported (faithful, ~1:1):**
- **Tokenizer, AST/node model, parser, generator** — the full round-trip. Anything upstream sqlglot
  parses and regenerates for **base + MySQL + Postgres**, this port does identically:
  **100% round-trip parity — 1847/1847 corpus cases** (base 955/955, MySQL 424/424, Postgres 468/468),
  checked against the ported upstream test suite. No statement fakes a round-trip via a raw-text
  fallback where upstream builds a structured node (guarded by a committed fidelity test).
- **Schema** — `MappingSchema`, `DataType.build`, type-category sets.
- **`qualify` optimizer pass** — qualify_tables → normalize_identifiers → qualify_columns →
  quote_identifiers → validate.
- **`scope` / `traverse_scope` + the full `Scope` API** — sources, columns, unions, CTEs, subqueries:
  the foundation for **column lineage**.

**Not ported (yet):**
- The rest of sqlglot's **optimizer**: `simplify` (full), `normalize`, `pushdown_predicates`,
  `pushdown_projections`, `eliminate_ctes` / `eliminate_joins` / `eliminate_subqueries`,
  `merge_subqueries`, `unnest_subqueries`, `optimize_joins`, `canonicalize`, and the top-level
  `optimize()` rule orchestrator. (`annotate_types` and `simplify` are only partially present.)
- **Cross-dialect transpilation** — the generator can target a dialect, but only **same-dialect**
  round-trip is a goal and is tested; reading one dialect and writing another is not verified.
- **Dialects beyond base / MySQL / Postgres** (upstream ships 30+).

**Beyond upstream — opt-in, additive (default output unchanged):**
- **Search-path table qualification** — `QualifyOpts.SearchPath` resolves an unqualified table
  against an ordered schema list by proven existence (fail-closed).
- **Top-level `UPDATE`/`DELETE`/`MERGE` scopes** — `TraverseScope`/`BuildScope` bind DML targets and
  their `FROM`/`USING`/`JOIN` sources for analysis (fail-closed; upstream yields none).
- **Qualify resolution report** — `QualifyOpts.ResolutionReport` surfaces each source's `SourceKind`
  (Physical / CTE / Derived / Subquery / Unresolved) + identity.
- **MySQL version/executable comments** — `mysql_version=<MYSQL_VERSION_ID>` activates `/*!… */`
  bodies into the token stream; default-off strips them as upstream does.

See [CHANGELOG.md](./CHANGELOG.md) for the per-version history, [ROADMAP.md](./ROADMAP.md) for the
remaining work and resolved-findings ledger, and [DEVIATIONS.md](./DEVIATIONS.md) for every place the
port *intentionally* behaves differently from upstream sqlglot (headline: ASCII-only identifier
case-folding, to match real engines).

## What works today

| Capability | Package | Notes |
|---|---|---|
| Tokenize | `tokens` | base + MySQL + Postgres tokenizers |
| Parse → AST | `parser`, `expressions` | full: SELECT/set-ops/CTE/subqueries, all query clauses, predicates, functions, DML + DDL (INSERT/UPDATE/DELETE/MERGE/CREATE + properties), CAST/DataType, PIVOT/LATERAL/VALUES/TABLESAMPLE, INTERVAL, JSON/JSONB ops, window functions |
| Generate (AST → SQL) | `generator` | **100% round-trip** on the upstream identity corpus for base/MySQL/Postgres (1847/1847) |
| Schema | `schema` | `MappingSchema`, `DataType.build`, type-category sets |
| Qualify | `optimizer` | `qualify` (qualify_tables, normalize_identifiers, qualify_columns, quote_identifiers, validate) |
| Scope / lineage | `optimizer` | `traverse_scope` + full `Scope` API (sources, columns, unions, CTEs) |
| Dialects | `dialects` | MySQL + Postgres (tokenizer, normalization, quoting, per-dialect functions/flags) |

## Quick start

```bash
go get github.com/ridi-oss/sqlglot-go
```

```go
package main

import (
	"fmt"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

func main() {
	// Parse
	expr, _ := sqlglot.ParseOne("SELECT id, name FROM users WHERE id = 1", "postgres")

	// Qualify against a schema (bind columns to sources, expand *, validate)
	sch := schema.M("users", schema.M("id", "INT", "name", "TEXT"))
	opts := optimizer.DefaultQualifyOpts()
	opts.Schema = sch
	opts.Dialect = "postgres"
	qualified := optimizer.Qualify(expr, opts)

	// Generate SQL back out
	sql, _ := sqlglot.Generate(qualified, "postgres", generator.Options{})
	fmt.Println(sql)
	// SELECT "users"."id" AS "id", "users"."name" AS "name" FROM "users" AS "users" WHERE "users"."id" = 1

	// Walk scopes (sources, columns, unions, …) for lineage
	for _, s := range optimizer.TraverseScope(qualified) {
		fmt.Printf("scope: %d columns, union=%v\n", len(s.Columns()), s.IsUnion())
	}
}
```

## Development

```bash
go test ./...          # green
gofmt -l . && go vet ./...
```

The upstream test suite is ported alongside the code (`*_test.go` + `testdata/*.sql` fixtures reused
verbatim) and is the correctness oracle. A round-trip parity corpus and an AST-fidelity gate keep the
port honest — both are monotonic (they can only tighten). For a live differential check against the
pinned Python source, `scripts/fetch-reference.sh` fetches sqlglot 30.12.0 into `.reference/`, then e.g.
`PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c "import sqlglot; print(sqlglot.parse_one('…','postgres').sql())"`.

## Continuing the port

Read [AGENTS.md](./AGENTS.md) and [ROADMAP.md](./ROADMAP.md). The reference Python source
(`.reference/`, fetched via `scripts/fetch-reference.sh`) is the source of truth — port from it 1:1,
port the matching upstream tests, keep `go test ./...` green. `ROADMAP.md` records the remaining
slices, every known divergence, and resolved-findings so settled decisions aren't re-litigated.

## License & attribution

MIT — see [LICENSE](./LICENSE). This is a derivative work: a Go translation of
[tobymao/sqlglot](https://github.com/tobymao/sqlglot) (© Toby Mao, MIT). The upstream MIT license is
preserved. sqlglot-go is not affiliated with or endorsed by the upstream project.
