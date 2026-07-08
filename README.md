# sqlglot-go

A faithful, near-1:1 **Go port of [tobymao/sqlglot](https://github.com/tobymao/sqlglot) v30.12.0** —
the pure-Python SQL parser, transpiler, and optimizer.

This is not a wrapper or a reimagining: it mirrors sqlglot's architecture (tokenizer → parser → AST →
generator → optimizer passes) file-by-file, so behavior tracks the Python original and upstream tests
port across directly. It has **zero third-party dependencies** (Go stdlib only).

> **Status: in progress.** The tokenizer, AST, generator, schema, and the `qualify`/`scope` optimizer
> passes work for **base + MySQL + Postgres**, validated against the ported upstream test suite. The
> parser is being brought to full upstream parity — some tail constructs are not parsed yet (tracked
> in [ROADMAP.md](./ROADMAP.md)). Dialect coverage beyond base/MySQL/Postgres is future work.

## What works today

| Capability | Package | Notes |
|---|---|---|
| Tokenize | `tokens`, `trie` | full base tokenizer |
| Parse → AST | `parser`, `expressions` | SELECT/set-ops/CTE/subqueries, all query clauses, predicates, functions, DML + DDL roots (INSERT/UPDATE/DELETE/MERGE/CREATE), CAST/DataType, PIVOT/LATERAL/VALUES, INTERVAL, JSON ops. Parser tail in progress — see ROADMAP. |
| Generate (AST → SQL) | `generator` | base dialect; 732/955 `identity.sql` lines round-trip |
| Schema | `schema` | `MappingSchema`, `DataType.build`, type category sets |
| Optimize | `optimizer` | `qualify` (qualify_tables, normalize_identifiers, qualify_columns, quote_identifiers, validate), `traverse_scope` + full `Scope` API |
| Dialects | `dialects` | MySQL + Postgres (tokenizer, normalization, quoting) |

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
go test ./...          # green
gofmt -l . && go vet ./...
```

The upstream test suite is ported alongside the code (`*_test.go` + `testdata/*.sql` fixtures reused
verbatim) and is the correctness oracle. For a live differential check against the pinned Python
source, `scripts/fetch-reference.sh` fetches sqlglot 30.12.0 into `.reference/`, then e.g.
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
