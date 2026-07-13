package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestTableValuedFunctionSource covers parseTablePart's function-parsing branch
// (parser.py:4664-4670 _parse_table_part): outside schema position, a function call
// (e.g. GENERATE_SERIES) is a valid FROM/JOIN table source, becoming exp.Table's "this".
// Verified against the pinned reference: parse_one("SELECT * FROM generate_series(1, 10)
// AS g(x)") -> Table(this=GenerateSeries(...), alias=TableAlias(...)).
func TestTableValuedFunctionSource(t *testing.T) {
	root := parseOne(t, "SELECT * FROM generate_series(1, 10) AS g(x)")
	from := exprArg(t, root, "from_")
	table := from.This()
	if table == nil || table.Kind() != exp.KindTable {
		t.Fatalf("FROM should be a Table:\n%s", root.ToS())
	}
	fn := table.This()
	if fn == nil || fn.Kind() != exp.KindGenerateSeries {
		t.Fatalf("table.this should be GenerateSeries, got %v:\n%s", table.Kind(), root.ToS())
	}
	alias := exprArg(t, table, "alias")
	if alias.Kind() != exp.KindTableAlias || alias.This() == nil || alias.This().Name() != "g" {
		t.Fatalf("table alias mismatch:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT * FROM GENERATE_SERIES(1, 10) AS g(x)"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestJSONTableSource covers _parse_json_table (parser.py:8166-8179) plus its helpers
// _parse_json_schema/_parse_json_column_def/_parse_format_json/_parse_on_handling.
// Verified against the pinned reference (base dialect; the grammar itself is not
// dialect-specific, only the surrounding COLUMNS(...) test SQL is borrowed from
// test_oracle.py:572 to exercise FORMAT JSON + ON ERROR/ON EMPTY).
func TestJSONTableSource(t *testing.T) {
	root := parseOne(t, "SELECT * FROM JSON_TABLE(foo FORMAT JSON, 'bla' ERROR ON ERROR NULL ON EMPTY COLUMNS(foo PATH 'bar'))")
	table := exprArg(t, root, "from_").This()
	if table == nil || table.Kind() != exp.KindTable {
		t.Fatalf("FROM should be a Table:\n%s", root.ToS())
	}
	jsonTable := table.This()
	if jsonTable == nil || jsonTable.Kind() != exp.KindJSONTable {
		t.Fatalf("table.this should be JSONTable, got %v:\n%s", table.Kind(), root.ToS())
	}
	if this := jsonTable.This(); this == nil || this.Kind() != exp.KindFormatJson {
		t.Fatalf("JSONTable.this should be FormatJson (FORMAT JSON):\n%s", root.ToS())
	}
	if got, want := jsonTable.Arg("error_handling"), "ERROR ON ERROR"; got != want {
		t.Fatalf("error_handling = %#v, want %q:\n%s", got, want, root.ToS())
	}
	if got, want := jsonTable.Arg("empty_handling"), "NULL ON EMPTY"; got != want {
		t.Fatalf("empty_handling = %#v, want %q:\n%s", got, want, root.ToS())
	}
	schema := exprArg(t, jsonTable, "schema")
	if schema.Kind() != exp.KindJSONSchema {
		t.Fatalf("JSONTable.schema should be JSONSchema:\n%s", root.ToS())
	}
	cols := expressionsForArg(schema, "expressions")
	if len(cols) != 1 || cols[0].Kind() != exp.KindJSONColumnDef {
		t.Fatalf("schema.expressions should be one JSONColumnDef:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT * FROM JSON_TABLE(foo FORMAT JSON, 'bla' ERROR ON ERROR NULL ON EMPTY COLUMNS(foo PATH 'bar'))"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestJSONTableOrdinalityAndCommaJoin ports test_mysql.py:316-318: a NESTED-free
// JSON_TABLE with a FOR ORDINALITY column, joined to a plain table via a comma.
func TestJSONTableOrdinalityAndCommaJoin(t *testing.T) {
	sql := "SELECT * FROM source, JSON_TABLE(source.links, '$.org[*]' COLUMNS(row_id FOR ORDINALITY, link VARCHAR(255) PATH '$.link')) AS links"
	root := parseOneDialect(t, sql, "mysql")
	joins := expressionsForArg(root, "joins")
	if len(joins) != 1 {
		t.Fatalf("join count = %d, want 1:\n%s", len(joins), root.ToS())
	}
	jsonTable := joins[0].This().This()
	if jsonTable == nil || jsonTable.Kind() != exp.KindJSONTable {
		t.Fatalf("join.this.this should be JSONTable:\n%s", root.ToS())
	}
	cols := expressionsForArg(exprArg(t, jsonTable, "schema"), "expressions")
	if len(cols) != 2 {
		t.Fatalf("column count = %d, want 2:\n%s", len(cols), root.ToS())
	}
	if cols[0].Arg("ordinality") != true {
		t.Fatalf("row_id should have ordinality=true:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}

// TestSimilarToEscape covers binary_range_parser(exp.SimilarTo) + _parse_escape
// (parser.py:62-71,5966-5971): `x SIMILAR TO y [ESCAPE z]`, and confirms `NOT ... SIMILAR
// TO` wraps in exp.Not (SimilarTo has no "negate" arg, unlike Like/ILike).
func TestSimilarToEscape(t *testing.T) {
	root := parseOne(t, "SELECT '%' SIMILAR TO '^%' ESCAPE '^'")
	projection := root.Expressions()[0]
	if projection.Kind() != exp.KindEscape {
		t.Fatalf("kind = %v, want Escape:\n%s", projection.Kind(), projection.ToS())
	}
	similarTo := exprArg(t, projection, "this")
	if similarTo.Kind() != exp.KindSimilarTo {
		t.Fatalf("Escape.this should be SimilarTo:\n%s", projection.ToS())
	}

	notExpr := parseOne(t, "SELECT * FROM t WHERE a NOT SIMILAR TO 'x'")
	where := exprArg(t, notExpr, "where")
	if where.This() == nil || where.This().Kind() != exp.KindNot {
		t.Fatalf("NOT SIMILAR TO should wrap in exp.Not:\n%s", notExpr.ToS())
	}
	if inner := where.This().This(); inner == nil || inner.Kind() != exp.KindSimilarTo {
		t.Fatalf("Not.this should be SimilarTo:\n%s", notExpr.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT '%' SIMILAR TO '^%' ESCAPE '^'"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestFromOnly ports test_postgres.py:96 (`SELECT * FROM ONLY t1`): Postgres-only
// ONLY token (parser.py:4876-4888), excluding descendant partitions from a table scan.
func TestFromOnly(t *testing.T) {
	root := parseOneDialect(t, "SELECT * FROM ONLY t1", "postgres")
	table := exprArg(t, root, "from_").This()
	if table.Kind() != exp.KindTable || table.Arg("only") != true {
		t.Fatalf("expected Table with only=true:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT * FROM ONLY t1"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestValuesDefaultBuildsVar ports test_mysql.py:205-211: SUPPORTS_VALUES_DEFAULT
// (dialect.py:670, true by default for base/mysql/postgres) makes `VALUES (DEFAULT)`
// parse as exp.var("DEFAULT") rather than misparsing DEFAULT as a plain column.
func TestValuesDefaultBuildsVar(t *testing.T) {
	root := parseOneDialect(t, "INSERT INTO t (i) VALUES (DEFAULT)", "mysql")
	values := exprArg(t, root, "expression")
	if values.Kind() != exp.KindValues {
		t.Fatalf("insert expression should be Values:\n%s", root.ToS())
	}
	tuples := expressionsForArg(values, "expressions")
	if len(tuples) != 1 {
		t.Fatalf("tuple count = %d, want 1:\n%s", len(tuples), root.ToS())
	}
	items := expressionsForArg(tuples[0], "expressions")
	if len(items) != 1 || items[0].Kind() != exp.KindVar || items[0].Name() != "DEFAULT" {
		t.Fatalf("VALUES (DEFAULT) should build Var(this=DEFAULT), got %#v:\n%s", items, root.ToS())
	}
}

// TestMySQLValuesFunction ports dialects.Dialect.ValuesIsFunction (parsers/mysql.py:63-70,
// 158-160): `VALUES(col)` outside a table-constructor position is MySQL's function
// referencing the row that would have been inserted (used in ON DUPLICATE KEY UPDATE).
// Uses a bare SELECT rather than the full INSERT ... ON DUPLICATE KEY UPDATE form to avoid
// conflating this parser gap with the separate DuplicateKeyUpdateWithSet generator flag.
func TestMySQLValuesFunction(t *testing.T) {
	root := parseOneDialect(t, "SELECT VALUES(a) + VALUES(b)", "mysql")
	add := root.Expressions()[0]
	if add.Kind() != exp.KindAdd {
		t.Fatalf("kind = %v, want Add:\n%s", add.Kind(), add.ToS())
	}
	for _, key := range []string{"this", "expression"} {
		operand := exprArg(t, add, key)
		if operand.Kind() != exp.KindAnonymous || operand.Arg("this") != "VALUES" {
			t.Fatalf("%s should be Anonymous(this=VALUES):\n%s", key, add.ToS())
		}
	}

	got, err := generateSQL(t, root, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT VALUES(a) + VALUES(b)"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}

	// Base dialect has no VALUES(...) function (parsers/mysql.py-only FUNC_TOKENS addition).
	if _, err := sqlglot.ParseOne("SELECT VALUES(a)", ""); err == nil {
		t.Fatalf("base dialect should not parse VALUES(a) as a function call")
	}

	// A quoted identifier named "VALUES" must NOT be hijacked by the MySQL VALUES function
	// parser in dialects that don't opt in: upstream FUNCTION_PARSERS["VALUES"] is MySQL-only,
	// so base/Postgres parse `"VALUES"(a)` as an Anonymous call with a quoted name.
	for _, dialect := range []string{"", "postgres"} {
		quoted, err := sqlglot.ParseOne(`SELECT "VALUES"(a)`, dialect)
		if err != nil {
			t.Fatalf(`[%q] "VALUES"(a) should parse: %v`, dialect, err)
		}
		call := quoted.Expressions()[0]
		if call.Kind() != exp.KindAnonymous {
			t.Fatalf(`[%q] "VALUES"(a) should be Anonymous, got %v:%s`, dialect, call.Kind(), call.ToS())
		}
		got, err := generateSQL(t, quoted, dialect)
		if err != nil {
			t.Fatalf("[%q] Generate: %v", dialect, err)
		}
		if want := `SELECT "VALUES"(a)`; got != want {
			t.Fatalf("[%q] round-trip = %q, want %q", dialect, got, want)
		}
	}
}
