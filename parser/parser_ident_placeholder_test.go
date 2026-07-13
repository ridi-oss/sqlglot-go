package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestColonPlaceholder covers the PLACEHOLDER_PARSERS COLON entry (parser.py:1175-1183):
// a leading ":" followed by an ID_VAR_TOKENS token builds a named exp.Placeholder, distinct
// from the bare "?" exp.Placeholder(this=nil).
func TestColonPlaceholder(t *testing.T) {
	root := parseOne(t, "SELECT :hello, ? FROM x LIMIT :my_limit")
	projections := root.Expressions()
	if len(projections) != 2 {
		t.Fatalf("projection count = %d, want 2:\n%s", len(projections), root.ToS())
	}
	if projections[0].Kind() != exp.KindPlaceholder || projections[0].Arg("this") != "hello" {
		t.Fatalf(":hello should be Placeholder(this=\"hello\"), got %#v:\n%s", projections[0], root.ToS())
	}
	if projections[1].Kind() != exp.KindPlaceholder || projections[1].Arg("this") != nil {
		t.Fatalf("? should be Placeholder(this=nil), got %#v:\n%s", projections[1], root.ToS())
	}
	limit := exprArg(t, root, "limit")
	limitExpr := exprArg(t, limit, "expression")
	if limitExpr.Kind() != exp.KindPlaceholder || limitExpr.Arg("this") != "my_limit" {
		t.Fatalf("LIMIT :my_limit should be Placeholder(this=\"my_limit\"), got %#v:\n%s", limitExpr, root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT :hello, ? FROM x LIMIT :my_limit"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestColonPlaceholderRetreatsWithoutIdVar covers the generic _parse_placeholder's
// "self._advance(-1)" retreat on a falsy PLACEHOLDER_PARSERS[COLON] result: a bare ":" not
// followed by an ID_VAR_TOKENS token must not be consumed as a placeholder attempt (here, it
// surfaces as an ordinary parse error rather than silently swallowing the colon).
func TestColonPlaceholderRetreatsWithoutIdVar(t *testing.T) {
	if _, err := sqlglot.ParseOne("SELECT : FROM x", ""); err == nil {
		t.Fatalf("bare colon with no following id-var should not parse as a placeholder")
	}
}

// TestValuesAsIdentifier covers _parse_column_reference's VALUES branch (parser.py:6674-6688):
// a bare VALUES not immediately followed by "(" is reparsed as a plain identifier when the
// dialect allows it (Dialect.ValuesFollowedByParen, true for base/postgres).
func TestValuesAsIdentifier(t *testing.T) {
	root := parseOne(t, "SELECT values")
	if projections := root.Expressions(); len(projections) != 1 || projections[0].Kind() != exp.KindColumn {
		t.Fatalf("SELECT values: want a single Column projection:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "SELECT values"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}

	root = parseOne(t, "SELECT values AS values FROM t WHERE values + 1 > 3")
	got, err = generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "SELECT values AS values FROM t WHERE values + 1 > 3"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}

	root = parseOne(t, "FOO(values.c)")
	got, err = generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "FOO(values.c)"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestValuesFollowedByParenStillTableConstructor confirms the VALUES-as-identifier fallback
// only fires when VALUES is NOT immediately followed by "(": VALUES(...) must still build the
// table-constructor Values node, both bare and as an IN(...) subquery source.
func TestValuesFollowedByParenStillTableConstructor(t *testing.T) {
	root := parseOne(t, "SELECT * FROM t WHERE x IN (VALUES (1))")
	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "SELECT * FROM t WHERE x IN (VALUES (1))"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestMySQLValuesFollowedByParenDisabled covers dialects.Dialect.ValuesFollowedByParen's
// MySQL override to false (parsers/mysql.py:303): MySQL never lets a bare VALUES fall back to
// a plain identifier, so "SELECT values" stays a parse error under mysql.
func TestMySQLValuesFollowedByParenDisabled(t *testing.T) {
	if _, err := sqlglot.ParseOne("SELECT values", "mysql"); err == nil {
		t.Fatalf("mysql should not reparse a bare VALUES as an identifier")
	}
}

// TestCaseElseIntervalEnd covers the CASE...END reference repair (parser.py:7763-7770):
// "ELSE interval END" misparses its ELSE branch as an exp.Interval that swallowed the closing
// END as a bare-identifier unit; recover it into the column reference `interval` instead of
// raising "Expected END after CASE".
func TestCaseElseIntervalEnd(t *testing.T) {
	root := parseOne(t, "CASE WHEN TRUE THEN 1 ELSE interval END")
	if root.Kind() != exp.KindCase {
		t.Fatalf("kind = %v, want Case:\n%s", root.Kind(), root.ToS())
	}
	def := exprArg(t, root, "default")
	if def.Kind() != exp.KindColumn || def.Name() != "interval" {
		t.Fatalf("default should be Column(interval), got %#v:\n%s", def, root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "CASE WHEN TRUE THEN 1 ELSE interval END"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestCaseMissingEndStillRaises confirms the reference repair is narrowly scoped to the
// "ELSE interval END" shape: an ordinary missing END still raises "Expected END after CASE".
func TestCaseMissingEndStillRaises(t *testing.T) {
	if _, err := sqlglot.ParseOne("CASE WHEN TRUE THEN 1 ELSE 2", ""); err == nil {
		t.Fatalf("CASE without END should still raise")
	}
}
