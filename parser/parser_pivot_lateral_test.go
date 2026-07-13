package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestParsePivotLateralUnnestValues(t *testing.T) {
	root := parseOne(t, "SELECT * FROM t PIVOT (SUM(x) FOR y IN (1, 2))")
	if pivots := root.FindAll(exp.KindPivot); len(pivots) != 1 {
		t.Fatalf("pivot count = %d, want 1:\n%s", len(pivots), root.ToS())
	}

	root = parseOne(t, "SELECT * FROM t, LATERAL (SELECT 1)")
	lateral := root.Find(exp.KindLateral)
	if lateral == nil {
		t.Fatalf("missing lateral:\n%s", root.ToS())
	}
	inner := lateral.Find(exp.KindSelect)
	if inner == nil || inner.FindAncestor(exp.KindLateral) == nil {
		t.Fatalf("nested lateral ancestor mismatch:\n%s", root.ToS())
	}

	root = parseOne(t, "SELECT * FROM UNNEST(x)")
	if from := exprArg(t, root, "from_"); from.This() == nil || from.This().Kind() != exp.KindUnnest {
		t.Fatalf("unnest FROM mismatch:\n%s", root.ToS())
	}

	root = parseOne(t, "SELECT * FROM (VALUES (1), (2)) AS t(x)")
	if from := exprArg(t, root, "from_"); from.This() == nil || from.This().Kind() != exp.KindValues {
		t.Fatalf("VALUES FROM mismatch:\n%s", root.ToS())
	}
}

// CROSS/OUTER APPLY (SQL Server) route through parseJoin into the ported parseLateral
// branch, producing a Join whose `this` is a Lateral (LOCAL fix).
func TestParseApplyJoin(t *testing.T) {
	cases := []struct {
		sql        string
		crossApply bool
		want       string // canonical round-trip (sqlglot 30.12.0 default dialect)
	}{
		{"SELECT * FROM t CROSS APPLY foo(x)", true, "SELECT * FROM t INNER JOIN LATERAL FOO(x)"},
		{"SELECT * FROM t OUTER APPLY (SELECT 1)", false, "SELECT * FROM t LEFT JOIN LATERAL (SELECT 1)"},
	}
	for _, tc := range cases {
		root := parseOne(t, tc.sql)
		joins := expressionsForArg(root, "joins")
		if len(joins) != 1 {
			t.Fatalf("%s: join count = %d, want 1:\n%s", tc.sql, len(joins), root.ToS())
		}
		lateral := joins[0].This()
		if lateral == nil || lateral.Kind() != exp.KindLateral {
			t.Fatalf("%s: join.this should be a Lateral:\n%s", tc.sql, root.ToS())
		}
		if lateral.Arg("cross_apply") != tc.crossApply {
			t.Fatalf("%s: cross_apply = %v, want %v:\n%s", tc.sql, lateral.Arg("cross_apply"), tc.crossApply, root.ToS())
		}
		// Regression for the generator reading `cross_apply` off the Lateral (join.this),
		// not the Join: the buggy path emitted a spurious ", " before the INNER/LEFT JOIN.
		got, err := generateSQL(t, root, "")
		if err != nil {
			t.Fatalf("%s: Generate: %v", tc.sql, err)
		}
		if got != tc.want {
			t.Fatalf("%s: round-trip = %q, want %q", tc.sql, got, tc.want)
		}
	}
}

// A CROSS/OUTER APPLY over a table-function with a trailing bare alias must keep the
// alias on the Lateral. Eager firstExpression evaluation in parseLateral used to let
// parseIdVar swallow the alias after parseFunction consumed the function source (LOCAL fix).
func TestParseApplyFunctionAlias(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM t CROSS APPLY foo(x) a",
		"SELECT * FROM t CROSS APPLY foo(x) AS a",
	} {
		root := parseOne(t, sql)
		joins := expressionsForArg(root, "joins")
		if len(joins) != 1 {
			t.Fatalf("%s: join count = %d, want 1:\n%s", sql, len(joins), root.ToS())
		}
		lateral := joins[0].This()
		if lateral == nil || lateral.Kind() != exp.KindLateral {
			t.Fatalf("%s: join.this should be a Lateral:\n%s", sql, root.ToS())
		}
		alias, ok := lateral.Arg("alias").(exp.Expression)
		if !ok || alias == nil || alias.Kind() != exp.KindTableAlias {
			t.Fatalf("%s: lateral alias should be a TableAlias, got %v:\n%s", sql, lateral.Arg("alias"), root.ToS())
		}
		if alias.Name() != "a" {
			t.Fatalf("%s: alias name = %q, want %q:\n%s", sql, alias.Name(), "a", root.ToS())
		}
	}
}

func TestPivotAnyAlias(t *testing.T) {
	root := parseOne(t, "SELECT * FROM t PIVOT (SUM(x) FOR y IN (ANY ORDER BY y))")
	pivot := root.Find(exp.KindPivot)
	if pivot == nil {
		t.Fatalf("missing pivot:\n%s", root.ToS())
	}
	fields := expressionsForArg(pivot, "fields")
	if len(fields) != 1 || len(fields[0].Expressions()) != 1 || fields[0].Expressions()[0].Kind() != exp.KindPivotAny {
		t.Fatalf("PivotAny mismatch:\n%s", pivot.ToS())
	}

	root = parseOne(t, "SELECT * FROM t PIVOT (SUM(x) FOR y IN (1 AS one, 2 AS two))")
	pivot = root.Find(exp.KindPivot)
	fields = expressionsForArg(pivot, "fields")
	if len(fields) != 1 || len(fields[0].Expressions()) != 2 || fields[0].Expressions()[0].Kind() != exp.KindPivotAlias {
		t.Fatalf("PivotAlias mismatch:\n%s", pivot.ToS())
	}
}

func TestUnpivotTargets(t *testing.T) {
	root := parseOne(t, "SELECT * FROM t UNPIVOT (v FOR k IN (a, b))")
	pivot := root.Find(exp.KindPivot)
	if pivot == nil || pivot.Arg("unpivot") != true {
		t.Fatalf("missing unpivot:\n%s", root.ToS())
	}
	if len(pivot.Expressions()) != 1 || pivot.Expressions()[0].Kind() != exp.KindIdentifier {
		t.Fatalf("unpivot value target mismatch:\n%s", pivot.ToS())
	}
	fields := expressionsForArg(pivot, "fields")
	if len(fields) != 1 || fields[0].This() == nil || fields[0].This().Kind() != exp.KindIdentifier {
		t.Fatalf("unpivot FOR target mismatch:\n%s", pivot.ToS())
	}
}
