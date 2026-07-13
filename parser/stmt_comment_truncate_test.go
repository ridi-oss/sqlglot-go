package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseCommentStructured ports the structured branches of _parse_comment
// (parser.py:2192-2222): base TABLE/COLUMN/DATABASE targets and the postgres
// MATERIALIZED VIEW/SEQUENCE/TYPE/VIEW/INDEX targets (all fall to the else branch,
// parse_table_parts). Cases mirror testdata/parity_gaps.txt.
func TestParseCommentStructured(t *testing.T) {
	comment := parseOne(t, "COMMENT ON TABLE my_schema.my_table IS 'Employee Information'")
	if comment.Kind() != exp.KindComment || comment.Arg("kind") != "TABLE" {
		t.Fatalf("kind mismatch:\n%s", comment.ToS())
	}
	this := exprArg(t, comment, "this")
	if this.Kind() != exp.KindTable || this.Name() != "my_table" || this.Text("db") != "my_schema" {
		t.Fatalf("comment target mismatch:\n%s", comment.ToS())
	}
	if got := exprArg(t, comment, "expression").Text("this"); got != "Employee Information" {
		t.Fatalf("comment text = %q:\n%s", got, comment.ToS())
	}
	got, err := generateSQL(t, comment, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "COMMENT ON TABLE my_schema.my_table IS 'Employee Information'"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}

	comment = parseOne(t, "COMMENT ON COLUMN my_schema.my_table.my_column IS 'Employee ID number'")
	if comment.Arg("kind") != "COLUMN" {
		t.Fatalf("kind mismatch:\n%s", comment.ToS())
	}
	col := exprArg(t, comment, "this")
	if col.Kind() != exp.KindColumn || col.Name() != "my_column" || col.Text("table") != "my_table" || col.Text("db") != "my_schema" {
		t.Fatalf("column target mismatch:\n%s", comment.ToS())
	}

	comment = parseOne(t, "COMMENT ON DATABASE my_database IS 'Development Database'")
	if comment.Arg("kind") != "DATABASE" {
		t.Fatalf("kind mismatch:\n%s", comment.ToS())
	}
	if this := exprArg(t, comment, "this"); this.Kind() != exp.KindTable || this.Name() != "my_database" {
		t.Fatalf("database target mismatch:\n%s", comment.ToS())
	}

	for _, sql := range []string{
		"COMMENT ON MATERIALIZED VIEW foo.my_view IS 'x'",
		"COMMENT ON MATERIALIZED VIEW my_view IS 'this'",
		"COMMENT ON SEQUENCE public.seq IS 'x'",
		"COMMENT ON TYPE foo.mood IS 'x'",
		"COMMENT ON TYPE mood IS 'x'",
		"COMMENT ON VIEW foo.bat IS 'x'",
		"COMMENT ON INDEX public.idx IS 'x'",
		"COMMENT ON TABLE mytable IS 'this'",
	} {
		comment = parseOneDialect(t, sql, "postgres")
		if comment.Kind() != exp.KindComment {
			t.Fatalf("%q: kind = %v, want Comment:\n%s", sql, comment.Kind(), comment.ToS())
		}
		got, err = generateSQL(t, comment, "postgres")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

// TestParseCommentProcedure ports _parse_comment's FUNCTION/PROCEDURE target branch
// through _parse_user_defined_function (parser.py:2203-2205).
func TestParseCommentProcedure(t *testing.T) {
	sql := "COMMENT ON PROCEDURE my_proc(integer, integer) IS 'Runs a report'"
	comment := parseOne(t, sql)
	if comment.Kind() != exp.KindComment || comment.Text("kind") != "PROCEDURE" || comment.Arg("exists") != false {
		t.Fatalf("%q: comment kind/existence mismatch:\n%s", sql, comment.ToS())
	}

	udf := exprArg(t, comment, "this")
	if udf.Kind() != exp.KindUserDefinedFunction || udf.Arg("wrapped") != true {
		t.Fatalf("%q: target should be a wrapped UserDefinedFunction:\n%s", sql, comment.ToS())
	}
	name := exprArg(t, udf, "this")
	if name.Kind() != exp.KindTable || name.Name() != "my_proc" {
		t.Fatalf("%q: procedure name mismatch:\n%s", sql, comment.ToS())
	}
	parameters := expressionsForArg(udf, "expressions")
	if len(parameters) != 2 {
		t.Fatalf("%q: parameters = %#v, want two typed parameters:\n%s", sql, udf.Arg("expressions"), comment.ToS())
	}
	for i, parameter := range parameters {
		if parameter.Kind() != exp.KindIdentifier || parameter.Name() != "integer" {
			t.Fatalf("%q: parameter %d mismatch:\n%s", sql, i, comment.ToS())
		}
	}
	if body := exprArg(t, comment, "expression"); body.Kind() != exp.KindLiteral || body.Text("this") != "Runs a report" {
		t.Fatalf("%q: comment body mismatch:\n%s", sql, comment.ToS())
	}
	if commands := comment.FindAll(exp.KindCommand); len(commands) != 0 {
		t.Fatalf("%q: found %d Command descendants:\n%s", sql, len(commands), comment.ToS())
	}

	got, err := generateSQL(t, comment, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}

// TestParseCommentNonPlainStringBodies covers COMMENT bodies that aren't plain STRING
// literals. Base N'...' (NATIONAL_STRING) now parses as National (parseString/
// STRING_PARSERS, slice-strings cluster), so parseComment builds a structured Comment
// that round-trips byte-for-byte (matches the pinned oracle: PYTHONPATH=.reference/
// sqlglot-v30.12.0 python3 -c "import sqlglot; print(sqlglot.parse_one(\"COMMENT ON
// TABLE my_schema.my_table IS N'National String'\").sql())"). Postgres $$...$$
// (HEREDOC_STRING) parses as a RawString (parseString/STRING_PARSERS), so parseComment
// builds a structured Comment whose generator normalizes the dollar-quote to a plain
// single-quoted string.
func TestParseCommentNonPlainStringBodies(t *testing.T) {
	sql := "COMMENT ON TABLE my_schema.my_table IS N'National String'"
	comment := parseOne(t, sql)
	if comment.Kind() != exp.KindComment || comment.Arg("kind") != "TABLE" {
		t.Fatalf("%q: kind mismatch:\n%s", sql, comment.ToS())
	}
	expression := exprArg(t, comment, "expression")
	if expression.Kind() != exp.KindNational || expression.Text("this") != "National String" {
		t.Fatalf("%q: expression mismatch:\n%s", sql, comment.ToS())
	}
	got, err := generateSQL(t, comment, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}

	pg := parseOneDialect(t, "COMMENT ON TABLE mytable IS $$doc this$$", "postgres")
	if pg.Kind() != exp.KindComment {
		t.Fatalf("postgres $$ comment: kind = %v, want Comment:\n%s", pg.Kind(), pg.ToS())
	}
	pgGot, err := generateSQL(t, pg, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "COMMENT ON TABLE mytable IS 'doc this'"; pgGot != want {
		t.Fatalf("postgres $$ round-trip = %q, want %q", pgGot, want)
	}
}

// TestParseTruncateTableStructured ports _parse_truncate_table's structured path
// (parser.py:9466-9515): base TABLE target plus the postgres CASCADE/RESTRICT and
// RESTART/CONTINUE IDENTITY trailers. Cases mirror testdata/parity_gaps.txt.
func TestParseTruncateTableStructured(t *testing.T) {
	truncate := parseOne(t, "TRUNCATE TABLE t")
	if truncate.Kind() != exp.KindTruncateTable {
		t.Fatalf("kind = %v, want TruncateTable:\n%s", truncate.Kind(), truncate.ToS())
	}
	tables := expressionsForArg(truncate, "expressions")
	if len(tables) != 1 || tables[0].Kind() != exp.KindTable || tables[0].Name() != "t" {
		t.Fatalf("truncate target mismatch:\n%s", truncate.ToS())
	}

	for _, sql := range []string{
		"TRUNCATE TABLE t1 CASCADE",
		"TRUNCATE TABLE t1 CONTINUE IDENTITY",
		"TRUNCATE TABLE t1 CONTINUE IDENTITY CASCADE",
		"TRUNCATE TABLE t1 RESTART IDENTITY",
		"TRUNCATE TABLE t1 RESTART IDENTITY RESTRICT",
		"TRUNCATE TABLE t1 RESTRICT",
	} {
		truncate = parseOneDialect(t, sql, "postgres")
		if truncate.Kind() != exp.KindTruncateTable {
			t.Fatalf("%q: kind = %v, want TruncateTable:\n%s", sql, truncate.Kind(), truncate.ToS())
		}
		got, err := generateSQL(t, truncate, "postgres")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

// TestParseTruncateNumericFunction locks in the "not to be confused with" guard
// (parser.py:9469-9471): a bare `TRUNCATE(number, decimals)` statement is the numeric
// function call, not TRUNCATE TABLE, and must retreat into an ordinary function-call
// parse rather than degrade to Command (which would wrongly insert a space and produce
// "TRUNCATE (...)").
func TestParseTruncateNumericFunction(t *testing.T) {
	for _, sql := range []string{"TRUNCATE(3.14159, 2)", "TRUNCATE(price, 0)"} {
		fn := parseOneDialect(t, sql, "mysql")
		if fn.Kind() == exp.KindTruncateTable || fn.Kind() == exp.KindCommand {
			t.Fatalf("%q: kind = %v, want a function call:\n%s", sql, fn.Kind(), fn.ToS())
		}
		got, err := generateSQL(t, fn, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

// TestParseTruncateTableOnlyWildcard covers postgres `ONLY t*` inheritance table refs:
// the ONLY prefix and the wildcard `*` suffix both parse structurally. The `*` is a
// no-op consumed and dropped by parseTable (parser.py:4890-4891), so generation matches
// upstream by emitting the tables without their trailing stars.
func TestParseTruncateTableOnlyWildcard(t *testing.T) {
	sql := "TRUNCATE TABLE ONLY t1, t2*, ONLY t3, t4, t5* RESTART IDENTITY CASCADE"
	want := "TRUNCATE TABLE ONLY t1, t2, ONLY t3, t4, t5 RESTART IDENTITY CASCADE"
	truncate := parseOneDialect(t, sql, "postgres")
	if truncate.Kind() != exp.KindTruncateTable {
		t.Fatalf("%q: kind = %v, want TruncateTable:\n%s", sql, truncate.Kind(), truncate.ToS())
	}
	got, err := generateSQL(t, truncate, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != want {
		t.Fatalf("%q: round-trip = %q, want %q", sql, got, want)
	}
}
