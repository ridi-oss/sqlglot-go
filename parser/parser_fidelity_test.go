package parser_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func parseOneErr(sql string) (exp.Expression, error) {
	return sqlglot.ParseOne(sql, "")
}

// FUNC_TOKENS is {explicit} | TYPE_TOKENS | SUBQUERY_PREDICATES (parser.py:825),
// not the much larger ID_VAR_TOKENS. Non-type keywords followed by `(` must therefore
// not be treated as function-callable in the non-any_token gate.
func TestFuncTokensExcludeNonTypeKeywords(t *testing.T) {
	for _, sql := range []string{
		"SELECT default(x)", "SELECT partition(x)", "SELECT view(x)", "SELECT update(x)",
	} {
		proj := parseOne(t, sql).Expressions()[0]
		if proj.Kind() == exp.KindAnonymous {
			t.Fatalf("%s: keyword wrongly parsed as a function call:\n%s", sql, proj.ToS())
		}
	}
	// VAR-tokenized and type-named functions still dispatch.
	if p := parseOne(t, "SELECT count(x) FROM t").Expressions()[0]; p.Kind() != exp.KindCount {
		t.Fatalf("count(x): got kind %d:\n%s", p.Kind(), p.ToS())
	}
}

// _parse_column_ops rewrites every Column in the left operand `this` to its dot form
// (parser.py:6796-6799 transform), not just a top-level Column.
func TestColumnOpsToDotIsRecursive(t *testing.T) {
	// a.b.c.d.e chain (in `this`) flattens to Identifiers; Column(x) lives in FOO's args
	// (field), which upstream leaves intact -> exactly one Column overall.
	proj := parseOne(t, "SELECT a.b.c.d.e.FOO(x) FROM t").Expressions()[0]
	if cols := proj.FindAll(exp.KindColumn); len(cols) != 1 {
		t.Fatalf("a.b.c.d.e.FOO(x): want 1 Column, got %d:\n%s", len(cols), proj.ToS())
	}
	// FOO(x).BAR(y): the transform recurses into `this` (=FOO(x)), flattening its Column(x)
	// to Identifier(x). BAR(y) is `field` and is left untouched.
	proj = parseOne(t, "SELECT FOO(x).BAR(y) FROM t").Expressions()[0]
	if cols := proj.This().FindAll(exp.KindColumn); len(cols) != 0 {
		t.Fatalf("FOO(x).BAR(y): FOO's arg should be flattened, got %d Column(s):\n%s", len(cols), proj.ToS())
	}
}

// Base dialect maps NVL/IFNULL/COALESCE to the same builder without is_nvl (parser.py:329).
func TestNvlEqualsCoalesce(t *testing.T) {
	nvl := parseOne(t, "SELECT NVL(a, b)").Expressions()[0]
	coalesce := parseOne(t, "SELECT COALESCE(a, b)").Expressions()[0]
	if nvl.Arg("is_nvl") != nil {
		t.Fatalf("NVL should not set is_nvl:\n%s", nvl.ToS())
	}
	if !nvl.Equal(coalesce) {
		t.Fatalf("NVL(a,b) should equal COALESCE(a,b):\n%s\nvs\n%s", nvl.ToS(), coalesce.ToS())
	}
}

// validate_expression rejects fixed-arity functions given more args than arg_types
// (error_messages, core.py:1320), but never var-len functions.
func TestFunctionOverArity(t *testing.T) {
	if _, err := parseOneErr("SELECT ABS(a, b)"); err == nil {
		t.Fatalf("ABS(a, b) should raise a parse error")
	} else if !strings.Contains(err.Error(), "number of provided arguments") {
		t.Fatalf("unexpected error for ABS(a, b): %v", err)
	}
	for _, sql := range []string{"SELECT GREATEST(a, b, c, d)", "SELECT COALESCE(a, b, c, d, e)", "SELECT COUNT(a, b)"} {
		if _, err := parseOneErr(sql); err != nil {
			t.Fatalf("var-len %q should not raise: %v", sql, err)
		}
	}
}

// parse_set_operation stores by_name only when matched (parser.py:5738 `... or None`).
func TestSetOperationByNameOmittedWhenAbsent(t *testing.T) {
	u := parseOne(t, "SELECT a FROM t UNION SELECT b FROM t")
	if u.Arg("by_name") != nil {
		t.Fatalf("by_name should be nil when absent, got %#v:\n%s", u.Arg("by_name"), u.ToS())
	}
	if strings.Contains(u.ToS(), "by_name") {
		t.Fatalf("ToS should omit by_name when absent:\n%s", u.ToS())
	}
}

func TestPivotMissingAggFunc(t *testing.T) {
	if _, err := parseOneErr("SELECT * FROM t PIVOT(FOR x IN (1, 2))"); err == nil {
		t.Fatalf("pivot missing aggregation should raise a parse error")
	} else if !strings.Contains(err.Error(), "Expecting an aggregation function in PIVOT") {
		t.Fatalf("unexpected pivot error: %v", err)
	}
}
