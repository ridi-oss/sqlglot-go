package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Statement-level source spans (Go-only, DEVIATIONS §6.2): every expression Parse returns is
// stamped with the rune span + verbatim text of its statement, so a batch consumer can split a
// multi-statement input into the exact submitted bytes — no Generate() rewrite. A statement
// whose body consumed several semicolon chunks (a procedure BEGIN...END, spanning its inner
// semicolons) is one statement with one span.
func TestStatementSpans(t *testing.T) {
	cases := []struct {
		sql     string
		dialect string
		want    []string // per-statement verbatim span text
	}{
		{"SELECT 'a;b' FROM t; SELECT 2", "postgres",
			[]string{"SELECT 'a;b' FROM t", "SELECT 2"}},
		{"select id,   ssn from t -- keep me", "postgres",
			[]string{"select id,   ssn from t"}},
		{"CREATE PROCEDURE p() BEGIN UPDATE t SET a = 1; SELECT 1; END", "mysql",
			[]string{"CREATE PROCEDURE p() BEGIN UPDATE t SET a = 1; SELECT 1; END"}},
		// divergence (DEVIATIONS §1.18): END terminates the block, so a statement after it is
		// its own statement — upstream folds SELECT 9 into the procedure's Block, but real
		// PostgreSQL 16 (BEGIN ATOMIC) runs the trailing statement separately.
		{"CREATE PROCEDURE p() BEGIN SELECT 1; END; SELECT 9", "mysql",
			[]string{"CREATE PROCEDURE p() BEGIN SELECT 1; END", "SELECT 9"}},
		{"CREATE FUNCTION f() RETURNS INT AS $fn$ SELECT '$$'; $fn$ LANGUAGE plpgsql; SELECT 9", "postgres",
			[]string{"CREATE FUNCTION f() RETURNS INT AS $fn$ SELECT '$$'; $fn$ LANGUAGE plpgsql", "SELECT 9"}},
		// The trailing-comment `;` chunk parses to exp.Semicolon (its span is the `;`).
		{"SELECT 1; -- note", "postgres",
			[]string{"SELECT 1", ";"}},
		// A comment-bearing `;` mid-batch is its own chunk; the following bare `;` leaves an
		// empty chunk = a nil slot ("" below), matching upstream's [Select, Semicolon, None,
		// Select].
		{"SELECT 1; /* c */ ; SELECT 2", "postgres",
			[]string{"SELECT 1", ";", "", "SELECT 2"}},
		// Rune (not byte) offsets: multibyte text before and inside a statement.
		{"SELECT '한글' FROM t; SELECT 'β'", "postgres",
			[]string{"SELECT '한글' FROM t", "SELECT 'β'"}},
		// The span is the statement's token range: leading trivia (whitespace, a detached
		// comment) and inter-statement whitespace sit in gaps outside every span.
		{"  /* c */  SELECT 1;   \n  SELECT 2", "postgres",
			[]string{"SELECT 1", "SELECT 2"}},
		// An activated MySQL executable comment splices body tokens into the statement; the
		// span is widened back to the `/*!NNNNN ... */` delimiters (Token.WrapStart/WrapEnd) so
		// the slice stays executable MySQL — both trailing and leading wrapper positions.
		{"SELECT 1 /*!40101 + 2 */; SELECT 3", "mysql, mysql_version=80035",
			[]string{"SELECT 1 /*!40101 + 2 */", "SELECT 3"}},
		{"/*!40101 SELECT */ 1", "mysql, mysql_version=80035",
			[]string{"/*!40101 SELECT */ 1"}},
	}
	for _, tc := range cases {
		statements, err := sqlglot.Parse(tc.sql, tc.dialect)
		if err != nil {
			t.Errorf("%q [%s]: parse: %v", tc.sql, tc.dialect, err)
			continue
		}
		if len(statements) != len(tc.want) {
			t.Errorf("%q [%s]: got %d statements, want %d", tc.sql, tc.dialect, len(statements), len(tc.want))
			continue
		}
		runes := []rune(tc.sql)
		for i, stmt := range statements {
			if tc.want[i] == "" {
				if stmt != nil {
					t.Errorf("%q [%s] stmt %d: got %s, want nil (empty chunk)", tc.sql, tc.dialect, i, exp.ClassName(stmt.Kind()))
				}
				continue
			}
			start, end, ok := stmt.Span()
			if !ok {
				t.Errorf("%q [%s] stmt %d: Span() not set", tc.sql, tc.dialect, i)
				continue
			}
			if got := string(runes[start:end]); got != tc.want[i] {
				t.Errorf("%q [%s] stmt %d: span slice %q, want %q", tc.sql, tc.dialect, i, got, tc.want[i])
			}
			if text, ok := stmt.SpanText(); !ok || text != tc.want[i] {
				t.Errorf("%q [%s] stmt %d: SpanText()=%q,%v, want %q", tc.sql, tc.dialect, i, text, ok, tc.want[i])
			}
		}
	}
}

// A `;` inside an activated executable comment splits the body across two statements; no
// source slice can represent either half with its wrapper, so both spans stay ABSENT (never
// a wrong slice like `/*!40101 SELECT 1` or `SELECT 2 */`).
func TestExecutableCommentSplitStatementSpansAbsent(t *testing.T) {
	statements, err := sqlglot.Parse("/*!40101 SELECT 1; SELECT 2 */", "mysql, mysql_version=80035")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(statements) != 2 {
		t.Fatalf("got %d statements, want 2", len(statements))
	}
	for i, stmt := range statements {
		if _, _, ok := stmt.Span(); ok {
			text, _ := stmt.SpanText()
			t.Errorf("stmt %d: span set (%q), want absent", i, text)
		}
	}
}

// Wrap widening is statement-level ONLY: a projection whose boundary token closes the
// executable comment must keep its own narrow span, not inherit the comment's full extent.
func TestExecutableCommentProjectionSpansNotWidened(t *testing.T) {
	sql := "/*!40101 SELECT a, b */"
	e, err := sqlglot.ParseOne(sql, "mysql, mysql_version=80035")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if text, _ := e.SpanText(); text != sql {
		t.Errorf("statement SpanText()=%q, want %q", text, sql)
	}
	projections := e.Expressions()
	if len(projections) != 2 {
		t.Fatalf("got %d projections, want 2", len(projections))
	}
	for i, want := range []string{"a", "b"} {
		if text, ok := projections[i].SpanText(); !ok || text != want {
			t.Errorf("projection %d SpanText()=%q,%v, want %q", i, text, ok, want)
		}
	}
}

// A trailing `;` whose chunk carries only a comment parses to exp.Semicolon holding the
// comment (parser.py:1110, 2159-2161) — upstream parity; previously this parse-errored.
func TestTrailingCommentSemicolon(t *testing.T) {
	for _, dialect := range []string{"base", "mysql", "postgres"} {
		statements, err := sqlglot.Parse("SELECT 1; -- note", dialect)
		if err != nil {
			t.Fatalf("[%s] parse: %v", dialect, err)
		}
		if len(statements) != 2 {
			t.Fatalf("[%s] got %d statements, want 2", dialect, len(statements))
		}
		if statements[1].Kind() != exp.KindSemicolon {
			t.Errorf("[%s] statements[1] Kind=%s, want Semicolon", dialect, exp.ClassName(statements[1].Kind()))
		}
		if comments := statements[1].Comments(); len(comments) != 1 || comments[0] != " note" {
			t.Errorf("[%s] comments=%q, want [\" note\"]", dialect, comments)
		}
		// generator.py:5421-5422: the node body renders empty; only the comment survives.
		if got, err := sqlglot.Generate(statements[1], dialect, generator.Options{}); err != nil || got != "/* note */" {
			t.Errorf("[%s] Generate(Semicolon)=%q,%v, want \"/* note */\"", dialect, got, err)
		}
	}
}
