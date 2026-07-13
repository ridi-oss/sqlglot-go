package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseDropTable ports the DROP TABLE identity cases (testdata/identity.sql:678-684):
// plain, dotted, IF EXISTS, CASCADE, CASCADE CONSTRAINTS, PURGE.
func TestParseDropTable(t *testing.T) {
	drop := parseOne(t, "DROP TABLE a")
	if drop.Kind() != exp.KindDrop || drop.Arg("kind") != "TABLE" {
		t.Fatalf("DROP TABLE kind mismatch:\n%s", drop.ToS())
	}
	if exprArg(t, drop, "this").Name() != "a" {
		t.Fatalf("DROP TABLE target mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP TABLE a.b")
	tbl := exprArg(t, drop, "this")
	if tbl.Kind() != exp.KindTable || tbl.Text("db") != "a" || tbl.Text("this") != "b" {
		t.Fatalf("DROP TABLE dotted target mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP TABLE IF EXISTS a")
	if drop.Arg("exists") != true {
		t.Fatalf("DROP TABLE IF EXISTS mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP TABLE a CASCADE")
	if drop.Arg("cascade") != true || drop.Arg("restrict") == true {
		t.Fatalf("DROP TABLE CASCADE mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP TABLE s_hajo CASCADE CONSTRAINTS")
	if drop.Arg("cascade") != true || drop.Arg("constraints") != true {
		t.Fatalf("DROP TABLE CASCADE CONSTRAINTS mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP TABLE a PURGE")
	if drop.Arg("purge") != true {
		t.Fatalf("DROP TABLE PURGE mismatch:\n%s", drop.ToS())
	}
}

// TestParseDropView ports testdata/identity.sql:685-688.
func TestParseDropView(t *testing.T) {
	drop := parseOne(t, "DROP VIEW a")
	if drop.Kind() != exp.KindDrop || drop.Arg("kind") != "VIEW" {
		t.Fatalf("DROP VIEW kind mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP VIEW IF EXISTS a.b")
	if drop.Arg("exists") != true {
		t.Fatalf("DROP VIEW IF EXISTS mismatch:\n%s", drop.ToS())
	}
}

// TestParseDropMaterializedView ports testdata/identity.sql:656: MATERIALIZED is a separate
// boolean flag (not folded into "kind"), matching parser.py:2309-2313's
// `materialized = self._match_text_seq("MATERIALIZED")` preceding the CREATABLES kind match.
func TestParseDropMaterializedView(t *testing.T) {
	drop := parseOne(t, "DROP MATERIALIZED VIEW x.y.z")
	if drop.Kind() != exp.KindDrop || drop.Arg("kind") != "VIEW" || drop.Arg("materialized") != true {
		t.Fatalf("DROP MATERIALIZED VIEW mismatch:\n%s", drop.ToS())
	}
	tbl := exprArg(t, drop, "this")
	if tbl.Text("catalog") != "x" || tbl.Text("db") != "y" || tbl.Text("this") != "z" {
		t.Fatalf("DROP MATERIALIZED VIEW target mismatch:\n%s", drop.ToS())
	}
}

// TestParseDropFunctionProcedureIndex ports testdata/identity.sql:654-666: a trailing
// parenthesized type signature parses into "expressions".
func TestParseDropFunctionProcedureIndex(t *testing.T) {
	drop := parseOne(t, "DROP FUNCTION a.b.c (INT)")
	if drop.Kind() != exp.KindDrop || drop.Arg("kind") != "FUNCTION" {
		t.Fatalf("DROP FUNCTION kind mismatch:\n%s", drop.ToS())
	}
	sig := expressionsForArg(drop, "expressions")
	if len(sig) != 1 || !exp.IsType(sig[0], exp.DTypeInt) {
		t.Fatalf("DROP FUNCTION signature mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP PROCEDURE a.b.c (INT)")
	if drop.Arg("kind") != "PROCEDURE" {
		t.Fatalf("DROP PROCEDURE kind mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP INDEX a.b.c")
	if drop.Arg("kind") != "INDEX" {
		t.Fatalf("DROP INDEX kind mismatch:\n%s", drop.ToS())
	}
}

// TestParseDropSchemaCatalogMapping ports test_parser.py:849 test_parse_drop_schema (the
// DROP SCHEMA db/catalog mapping via isDBReference) and :901 test_parse_create_schema (the
// CREATE-side counterpart, sharing the same parseTableParts(isDBReference=true) mechanism):
// a 2-part name maps to db+catalog (not this+db), and a 1-part name maps to db alone.
func TestParseDropSchemaCatalogMapping(t *testing.T) {
	drop := parseOne(t, "DROP SCHEMA catalog.schema")
	if drop.Kind() != exp.KindDrop || drop.Arg("kind") != "SCHEMA" {
		t.Fatalf("DROP SCHEMA kind mismatch:\n%s", drop.ToS())
	}
	tbl := exprArg(t, drop, "this")
	if tbl.Arg("this") != nil {
		t.Fatalf("DROP SCHEMA catalog.schema: table 'this' should be unset:\n%s", drop.ToS())
	}
	if tbl.Text("db") != "schema" || tbl.Text("catalog") != "catalog" {
		t.Fatalf("DROP SCHEMA catalog.schema: db/catalog mismatch:\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP SCHEMA IF EXISTS catalog.schema")
	if drop.Arg("exists") != true {
		t.Fatalf("DROP SCHEMA IF EXISTS mismatch:\n%s", drop.ToS())
	}
	tbl = exprArg(t, drop, "this")
	if tbl.Text("db") != "schema" || tbl.Text("catalog") != "catalog" {
		t.Fatalf("DROP SCHEMA IF EXISTS catalog.schema: db/catalog mismatch (must not be displaced):\n%s", drop.ToS())
	}

	drop = parseOne(t, "DROP SCHEMA IF EXISTS myschema")
	tbl = exprArg(t, drop, "this")
	if tbl.Text("db") != "myschema" || tbl.Arg("catalog") != nil {
		t.Fatalf("DROP SCHEMA single-part name mismatch:\n%s", drop.ToS())
	}

	create := parseOne(t, "CREATE SCHEMA catalog.schema")
	if create.Kind() != exp.KindCreate || create.Arg("kind") != "SCHEMA" {
		t.Fatalf("CREATE SCHEMA kind mismatch:\n%s", create.ToS())
	}
	createTbl := exprArg(t, create, "this")
	if createTbl.Text("db") != "schema" || createTbl.Text("catalog") != "catalog" {
		t.Fatalf("CREATE SCHEMA db/catalog mismatch:\n%s", create.ToS())
	}
}

// TestParseDropPostgresConcurrently ports the postgres DROP INDEX CONCURRENTLY gaps
// (parser.py:2317).
func TestParseDropPostgresConcurrently(t *testing.T) {
	drop := parseOneDialect(t, "DROP INDEX CONCURRENTLY IF EXISTS ix_table_id", "postgres")
	if drop.Arg("concurrently") != true || drop.Arg("exists") != true {
		t.Fatalf("DROP INDEX CONCURRENTLY IF EXISTS mismatch:\n%s", drop.ToS())
	}

	drop = parseOneDialect(t, "DROP INDEX CONCURRENTLY ix_table_id", "postgres")
	if drop.Arg("concurrently") != true {
		t.Fatalf("DROP INDEX CONCURRENTLY mismatch:\n%s", drop.ToS())
	}
}

// TestParseDropDegradesToCommand covers DROP statements this port doesn't structurally
// model: an unrecognized creatable (out of the creatables set) and ICEBERG-qualified DROP
// on a non-TABLE kind (parser.py:2311-2315).
func TestParseDropDegradesToCommand(t *testing.T) {
	cases := []string{
		"DROP ROLE foo",
		"DROP ICEBERG VIEW x",
	}
	for _, sql := range cases {
		t.Run(sql, func(t *testing.T) {
			root := parseOne(t, sql)
			if root.Kind() != exp.KindCommand {
				t.Fatalf("kind = %v, want Command (degrade):\n%s", root.Kind(), root.ToS())
			}
		})
	}
}
