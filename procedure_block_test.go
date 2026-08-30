package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// CREATE PROCEDURE/FUNCTION bodies: BEGIN...END blocks (exp.Block + exp.EndStatement,
// parser.py:2114-2135,2463) and Postgres dollar-quoted heredoc bodies (exp.Heredoc,
// parser.py:9368-9370). Expected outputs verified against pinned upstream v30.12.0.
func TestCreateProcedureBody(t *testing.T) {
	cases := []struct {
		sql      string
		dialects []string
		rootKind exp.Kind
		want     string
	}{
		{"CREATE PROCEDURE p() BEGIN SELECT 1; END", []string{"base", "mysql", "postgres"},
			exp.KindCreate, "CREATE PROCEDURE p() AS BEGIN SELECT 1; END"},
		{"CREATE PROCEDURE p() BEGIN UPDATE t SET a = 1; SELECT 1; END", []string{"mysql"},
			exp.KindCreate, "CREATE PROCEDURE p() AS BEGIN UPDATE t SET a = 1; SELECT 1; END"},
		{"CREATE PROCEDURE p(IN x INT) BEGIN UPDATE t SET a = x; SELECT 1; END", []string{"postgres"},
			exp.KindCreate, "CREATE PROCEDURE p(IN x INT) AS BEGIN UPDATE t SET a = x; SELECT 1; END"},
		// The already-consumed AS spelling round-trips identically.
		{"CREATE PROCEDURE p() AS BEGIN SELECT 1; END", []string{"mysql"},
			exp.KindCreate, "CREATE PROCEDURE p() AS BEGIN SELECT 1; END"},
		// Unterminated block: the trailing END is simply absent from the Block.
		{"CREATE PROCEDURE p() BEGIN SELECT 1", []string{"mysql"},
			exp.KindCreate, "CREATE PROCEDURE p() AS BEGIN SELECT 1"},
		// FUNCTION keeps the single-statement body path; the top-level trailing END chunk
		// becomes a sibling EndStatement, so ParseOne wraps the pair in a Block root.
		{"CREATE FUNCTION f() RETURNS INT BEGIN RETURN 1; END", []string{"mysql"},
			exp.KindBlock, "CREATE FUNCTION f() RETURNS INT AS BEGIN RETURN 1; END"},
		// Postgres dollar-quoted bodies -> exp.Heredoc. divergence: a named tag is preserved
		// (pinned upstream drops it and always emits `$$`, which real PostgreSQL rejects when
		// the body itself contains `$$`); see DEVIATIONS §1 and TestHeredocTagPreserved.
		{"CREATE PROCEDURE p() LANGUAGE plpgsql AS $$ BEGIN UPDATE t SET a = 1; END $$", []string{"postgres"},
			exp.KindCreate, "CREATE PROCEDURE p() LANGUAGE plpgsql AS $$ BEGIN UPDATE t SET a = 1; END $$"},
		{"CREATE FUNCTION f() RETURNS INT AS $fn$ BEGIN RETURN 1; END $fn$ LANGUAGE plpgsql", []string{"postgres"},
			exp.KindCreate, "CREATE FUNCTION f() RETURNS INT LANGUAGE plpgsql AS $fn$ BEGIN RETURN 1; END $fn$"},
	}
	for _, tc := range cases {
		for _, dialect := range tc.dialects {
			e, err := sqlglot.ParseOne(tc.sql, dialect)
			if err != nil {
				t.Errorf("%q [%s]: parse: %v", tc.sql, dialect, err)
				continue
			}
			if e.Kind() != tc.rootKind {
				t.Errorf("%q [%s]: root Kind=%s, want %s", tc.sql, dialect, exp.ClassName(e.Kind()), exp.ClassName(tc.rootKind))
			}
			got, err := sqlglot.Generate(e, dialect, generator.Options{})
			if err != nil {
				t.Errorf("%q [%s]: generate: %v", tc.sql, dialect, err)
				continue
			}
			if got != tc.want {
				t.Errorf("%q [%s]:\ngot  %q\nwant %q", tc.sql, dialect, got, tc.want)
			}
		}
	}
}

// A standalone `BEGIN SELECT 1; END` is not a valid statement in any target dialect (BEGIN
// starts a transaction); upstream parse-errors it too. Must stay fail-closed.
func TestBareBeginBlockFailsClosed(t *testing.T) {
	for _, dialect := range []string{"base", "mysql", "postgres"} {
		if _, err := sqlglot.ParseOne("BEGIN SELECT 1; END", dialect); err == nil {
			t.Errorf("[%s] bare BEGIN block parsed; want error", dialect)
		}
	}
}

// divergence test for DEVIATIONS §1: a named dollar-quote tag survives the round-trip even
// when the body contains `$$` — pinned upstream regenerates `$$ SELECT '$$'; $$`, which real
// PostgreSQL 17 rejects (the embedded `$$` closes the body early).
func TestHeredocTagPreserved(t *testing.T) {
	sql := "CREATE FUNCTION f() RETURNS INT AS $fn$ SELECT '$$'; $fn$ LANGUAGE plpgsql"
	e, err := sqlglot.ParseOne(sql, "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := sqlglot.Generate(e, "postgres", generator.Options{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want := "CREATE FUNCTION f() RETURNS INT LANGUAGE plpgsql AS $fn$ SELECT '$$'; $fn$"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := sqlglot.ParseOne(got, "postgres"); err != nil {
		t.Fatalf("regenerated SQL does not re-tokenize: %v", err)
	}
}

// A structured CREATE whose block parse stopped mid-statement degrades to Command IN PLACE
// (parser.py:2624): the Command spans through the chunk the parse stopped in — a block body
// may have consumed several — and parsing resumes at the next chunk, so trailing statements
// survive. Upstream yields [Command("...SELECT 1; ELSE"), Select] for this input.
func TestCreateProcedureDegradeSpansConsumedChunks(t *testing.T) {
	statements, err := sqlglot.Parse("CREATE PROCEDURE p() BEGIN SELECT 1; ELSE; SELECT 2", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(statements) != 2 {
		t.Fatalf("got %d statements, want 2 (Command + Select)", len(statements))
	}
	if statements[0].Kind() != exp.KindCommand {
		t.Errorf("statements[0] Kind=%s, want Command", exp.ClassName(statements[0].Kind()))
	}
	if got, want := statements[0].Text("this")+statements[0].Text("expression"), "CREATE PROCEDURE p() BEGIN SELECT 1; ELSE"; got != want {
		t.Errorf("Command text %q, want %q", got, want)
	}
	if statements[1].Kind() != exp.KindSelect {
		t.Errorf("statements[1] Kind=%s, want Select", exp.ClassName(statements[1].Kind()))
	}
}

// WHILE statement AST parity (_parse_whileblock, parser.py:2278-2281); base generation is
// unsupported upstream too (generator.py:6208-6210), so only the AST shape is asserted.
func TestWhileBlockAST(t *testing.T) {
	e, err := sqlglot.ParseOne("WHILE (x < 3) SELECT 1", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := "WhileBlock(\n" +
		"  this=LT(\n" +
		"    this=Column(\n" +
		"      this=Identifier(this=x, quoted=False)),\n" +
		"    expression=Literal(this=3, is_string=False)),\n" +
		"  body=Block(\n" +
		"    expressions=[\n" +
		"      Select(\n" +
		"        expressions=[\n" +
		"          Literal(this=1, is_string=False)])]))"
	if got := e.ToS(); got != want {
		t.Errorf("ToS() mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
