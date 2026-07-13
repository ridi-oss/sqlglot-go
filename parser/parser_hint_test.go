package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestHintRebalance ports the parity_gaps.txt case `SELECT /*+ REBALANCE */ * FROM foo`:
// a single bare-var hint (_parse_var(upper=True), parser.py:4239) round-trips through
// hint=Hint(expressions=[Var(this=REBALANCE)]).
func TestHintRebalance(t *testing.T) {
	root := parseOne(t, "SELECT /*+ REBALANCE */ * FROM foo")
	hint := exprArg(t, root, "hint")
	if hint.Kind() != exp.KindHint {
		t.Fatalf("select.hint should be Hint, got %v:\n%s", hint.Kind(), root.ToS())
	}
	items := hint.Expressions()
	if len(items) != 1 || items[0].Kind() != exp.KindVar || items[0].Name() != "REBALANCE" {
		t.Fatalf("hint.expressions should be [Var(REBALANCE)], got %#v:\n%s", items, root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT /*+ REBALANCE */ * FROM foo"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestHintFunctionCall ports the parity_gaps.txt case `SELECT /*+ SOME_HINT(foo) */ 1`:
// a hint item that looks like a function call parses via _parse_hint_function_call
// (parser.py:4228-4229) into an Anonymous call, not a bare Var.
func TestHintFunctionCall(t *testing.T) {
	root := parseOne(t, "SELECT /*+ SOME_HINT(foo) */ 1")
	hint := exprArg(t, root, "hint")
	items := hint.Expressions()
	if len(items) != 1 || items[0].Kind() != exp.KindAnonymous || items[0].Name() != "SOME_HINT" {
		t.Fatalf("hint.expressions should be [Anonymous(SOME_HINT, ...)], got %#v:\n%s", items, root.ToS())
	}
	args := items[0].Expressions()
	if len(args) != 1 || args[0].Kind() != exp.KindColumn || args[0].Name() != "foo" {
		t.Fatalf("SOME_HINT args should be [Column(foo)], got %#v:\n%s", args, root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT /*+ SOME_HINT(foo) */ 1"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestHintMultipleMySQLHints ports the parity_gaps.txt case
// `SELECT /*+ BKA(t1) NO_BKA(t2) */ * FROM t1 INNER JOIN t2`: a comma-separated (here,
// space-separated function-call-per-item) hint list, each becoming its own Anonymous call.
// Note: this drives parseHintBody's outer retry loop (parser_hint.go), which is required
// precisely because there's no comma between "BKA(t1)" and "NO_BKA(t2)" - parseCsv alone
// only splits on COMMA, so a single parseCsv call would stop after the first item and the
// leftover "NO_BKA(t2)" would incorrectly trigger the raw-string fallback.
//
// The round-trip is byte-for-byte: upstream's MySQL generator overrides QUERY_HINT_SEP to
// " " (generators/mysql.py:138) instead of the base default ", " (generator.py:368), which
// hintSQL (generator/sql.go) mirrors by dialect-gating the item separator on mysql.
func TestHintMultipleMySQLHints(t *testing.T) {
	sql := "SELECT /*+ BKA(t1) NO_BKA(t2) */ * FROM t1 INNER JOIN t2"
	root := parseOneDialect(t, sql, "mysql")
	hint := exprArg(t, root, "hint")
	items := hint.Expressions()
	if len(items) != 2 {
		t.Fatalf("hint.expressions count = %d, want 2:\n%s", len(items), root.ToS())
	}
	if items[0].Kind() != exp.KindAnonymous || items[0].Name() != "BKA" {
		t.Fatalf("hint[0] should be Anonymous(BKA), got %#v:\n%s", items[0], root.ToS())
	}
	if items[1].Kind() != exp.KindAnonymous || items[1].Name() != "NO_BKA" {
		t.Fatalf("hint[1] should be Anonymous(NO_BKA), got %#v:\n%s", items[1], root.ToS())
	}

	got, err := generateSQL(t, root, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT /*+ BKA(t1) NO_BKA(t2) */ * FROM t1 INNER JOIN t2"
	if got != want {
		t.Fatalf("generated = %q, want %q (see QUERY_HINT_SEP note above)", got, want)
	}
}

// TestHintIndexAndMerge covers the remaining MySQL hint parity_gaps.txt cases:
// `SELECT /*+ INDEX(t, i) */ c1 FROM t WHERE c2 = 'value'` (multi-arg function hint) and
// `SELECT /*+ MERGE(dt) */ * FROM (SELECT * FROM t1) AS dt` (hint on a query with a
// subquery FROM source, confirming the hint doesn't interfere with FROM parsing).
func TestHintIndexAndMerge(t *testing.T) {
	for _, sql := range []string{
		"SELECT /*+ INDEX(t, i) */ c1 FROM t WHERE c2 = 'value'",
		"SELECT /*+ MERGE(dt) */ * FROM (SELECT * FROM t1) AS dt",
	} {
		root := parseOneDialect(t, sql, "mysql")
		hint := exprArg(t, root, "hint")
		if hint == nil || hint.Kind() != exp.KindHint {
			t.Fatalf("[%s] select.hint should be Hint:\n%s", sql, root.ToS())
		}
		got, err := generateSQL(t, root, "mysql")
		if err != nil {
			t.Fatalf("[%s] Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("[%s] round-trip = %q, want %q", sql, got, sql)
		}
	}
}

// TestHintFallbackToString ports _parse_hint_fallback_to_string (parser.py:4220-4226): a
// hint body that isn't a clean csv of function-calls/vars (here, two adjacent number
// literals with no comma) degrades to a single raw-text Hint expression rather than
// failing the whole statement.
func TestHintFallbackToString(t *testing.T) {
	root := parseOne(t, "SELECT /*+ 1 2 */ * FROM t")
	hint := exprArg(t, root, "hint")
	// The fallback item is a raw Go string, not an exp.Expression node, so the generic
	// Expressions() accessor (which only surfaces Expression-typed children) reports it
	// empty; assert against the raw []any arg instead.
	if items := hint.Expressions(); len(items) != 0 {
		t.Fatalf("fallback hint.Expressions() should be empty (raw string, not a node), got %#v", items)
	}
	if raw, _ := hint.Arg("expressions").([]any); len(raw) != 1 || raw[0] != "1 2" {
		t.Fatalf("fallback hint.expressions raw = %#v, want [%q]", raw, "1 2")
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT /*+ 1 2 */ * FROM t"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestOperationModifiersMySQL ports the parity_gaps.txt case
// `SELECT HIGH_PRIORITY STRAIGHT_JOIN SQL_CALC_FOUND_ROWS * FROM t`: MySQL's
// OPERATION_MODIFIERS (parsers/mysql.py:290-299), consumed immediately after SELECT/DISTINCT
// and before projections, each becoming a bare exp.Var.
func TestOperationModifiersMySQL(t *testing.T) {
	sql := "SELECT HIGH_PRIORITY STRAIGHT_JOIN SQL_CALC_FOUND_ROWS * FROM t"
	root := parseOneDialect(t, sql, "mysql")
	modifiers := expressionsForArg(root, "operation_modifiers")
	want := []string{"HIGH_PRIORITY", "STRAIGHT_JOIN", "SQL_CALC_FOUND_ROWS"}
	if len(modifiers) != len(want) {
		t.Fatalf("operation_modifiers count = %d, want %d:\n%s", len(modifiers), len(want), root.ToS())
	}
	for i, w := range want {
		if modifiers[i].Kind() != exp.KindVar || modifiers[i].Name() != w {
			t.Fatalf("operation_modifiers[%d] = %#v, want Var(%s):\n%s", i, modifiers[i], w, root.ToS())
		}
	}

	got, err := generateSQL(t, root, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}

	// Base dialect has no OPERATION_MODIFIERS (parser.py:1747's empty base set), so
	// HIGH_PRIORITY there is just an ordinary (undefined) column reference, not consumed as
	// a modifier - i.e. it must NOT show up as this Select's operation_modifiers.
	base, err := sqlglot.ParseOne("SELECT HIGH_PRIORITY, x FROM t", "")
	if err != nil {
		t.Fatalf("base ParseOne: %v", err)
	}
	if mods := expressionsForArg(base, "operation_modifiers"); len(mods) != 0 {
		t.Fatalf("base dialect should not populate operation_modifiers, got %#v", mods)
	}
}

// TestParseIntoHint exercises the exp.KindHint entry added to Parser.ParseInto directly
// (parser.go's ParseInto switch), independent of the SELECT call-site.
func TestParseIntoHint(t *testing.T) {
	hint, err := sqlglot.ParseInto("REBALANCE", "", exp.KindHint)
	if err != nil {
		t.Fatalf("ParseInto(KindHint): %v", err)
	}
	if hint.Kind() != exp.KindHint {
		t.Fatalf("kind = %v, want Hint", hint.Kind())
	}
	items := hint.Expressions()
	if len(items) != 1 || items[0].Kind() != exp.KindVar || items[0].Name() != "REBALANCE" {
		t.Fatalf("expressions = %#v, want [Var(REBALANCE)]", items)
	}
}
