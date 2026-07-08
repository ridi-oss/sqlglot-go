# sqlglot-go

A faithful, near-1:1 **Go port of [tobymao/sqlglot](https://github.com/tobymao/sqlglot) v30.12.0** —
the pure-Python SQL parser, transpiler, and optimizer.

This is not a wrapper or a reimagining: it mirrors sqlglot's architecture (tokenizer → parser → AST →
generator → optimizer passes) file-by-file, so behavior tracks the Python original and upstream tests
port across directly. It has **zero third-party dependencies** (Go stdlib only).

> **Status: Milestone 1 complete.** The port targets exactly the sqlglot API surface used by a SQL
> **column-lineage probe** (`parse` → `qualify` → `traverse_scope`), on **MySQL** and **Postgres**,
> and is verified at **94/94 parity** against the real Python `probe.py` running on sqlglot 30.12.0.
> Broader dialect/feature coverage is in progress — see [ROADMAP.md](./ROADMAP.md).

## What works today

| Capability | Package | Notes |
|---|---|---|
| Tokenize | `tokens`, `trie` | full base tokenizer |
| Parse → AST | `parser`, `expressions` | SELECT/set-ops/CTE/subqueries, all query clauses, predicates, functions, DML + DDL roots (INSERT/UPDATE/DELETE/MERGE/CREATE), CAST/DataType, PIVOT/LATERAL/VALUES, INTERVAL, JSON ops |
| Generate (AST → SQL) | `generator` | base dialect; 732/955 `identity.sql` lines round-trip |
| Schema | `schema` | `MappingSchema`, `DataType.build`, type category sets |
| Optimize | `optimizer` | `qualify` (qualify_tables, normalize_identifiers, qualify_columns, quote_identifiers, validate), `traverse_scope` + full `Scope` API |
| Dialects | `dialects` | MySQL + Postgres (tokenizer, normalization, quoting) |
| Lineage probe | `probe` | ported end-to-end, with a Python-parity harness |

## Quick start

```bash
go get github.com/sjincho/sqlglot-go
```

```go
package main

import (
	"fmt"

	sqlglot "github.com/sjincho/sqlglot-go"
	"github.com/sjincho/sqlglot-go/generator"
	"github.com/sjincho/sqlglot-go/optimizer"
	"github.com/sjincho/sqlglot-go/schema"
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
go test ./...          # ~122 tests, green
gofmt -l . && go vet ./...
```

**Parity harness.** The `probe` package proves the port faithfully reproduces the Python lineage
analyzer. `probe/golden_test.go` runs hermetically against committed golden results
(`probe/testdata/golden.json`) — no Python needed. To re-verify against *live* Python and refresh the
goldens:

```bash
scripts/fetch-reference.sh                                   # fetch pinned sqlglot 30.12.0 into .reference/
PROBE_REGEN=1 go test ./probe/ -run TestProbeParity          # run the real probe.py + regenerate goldens
```

## Continuing the port

Read [AGENTS.md](./AGENTS.md) and [ROADMAP.md](./ROADMAP.md). The reference Python source
(`.reference/`, fetched via `scripts/fetch-reference.sh`) is the source of truth — port from it 1:1,
port the matching upstream tests, keep `go test ./...` green. `ROADMAP.md` records the remaining
slices, every known divergence, and resolved-findings so settled decisions aren't re-litigated.

## License & attribution

MIT — see [LICENSE](./LICENSE). This is a derivative work: a Go translation of
[tobymao/sqlglot](https://github.com/tobymao/sqlglot) (© Toby Mao, MIT). The upstream MIT license is
preserved. sqlglot-go is not affiliated with or endorsed by the upstream project.
