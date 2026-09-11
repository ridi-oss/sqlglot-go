package parser_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
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

// PIVOT/UNPIVOT are TABLE_ALIAS_TOKENS (parser.py:836); only a following `(` makes them a
// clause, because _parse_table_parts consumes pivots before the alias (parser.py:4973).
func TestPivotAsIdentifier(t *testing.T) {
	for _, dialect := range []string{"", "mysql", "postgres", "presto", "athena"} {
		for _, tc := range []struct{ sql, want string }{
			{"WITH pivot AS (SELECT 1) SELECT * FROM pivot", "WITH pivot AS (SELECT 1) SELECT * FROM pivot"},
			{"SELECT * FROM t pivot WHERE pivot.a = 1", "SELECT * FROM t AS pivot WHERE pivot.a = 1"},
			{"SELECT * FROM (SELECT 1) pivot", "SELECT * FROM (SELECT 1) AS pivot"},
			{"SELECT * FROM t unpivot JOIN u ON TRUE", "SELECT * FROM t AS unpivot JOIN u ON TRUE"},
		} {
			e := parseOneDialect(t, tc.sql, dialect)
			if got, err := generateSQL(t, e, dialect); err != nil || got != tc.want {
				t.Errorf("%s %q: got %q, %v; want %q", dialect, tc.sql, got, err, tc.want)
			}
		}
		// A following `(` still makes it a clause. SQL is asserted for base only: upstream
		// mysql/postgres/presto drop PIVOT via no_pivot_sql, which the generator does not port yet.
		e := parseOneDialect(t, "SELECT * FROM t pivot (SUM(a) FOR b IN (1, 2))", dialect)
		if len(e.FindAll(exp.KindPivot)) != 1 || e.Arg("from_").(exp.Expression).This().Arg("alias") != nil {
			t.Errorf("%s: want one Pivot and no alias: %s", dialect, e.ToS())
		}
		if dialect == "" {
			if got, err := generateSQL(t, e, dialect); err != nil || got != "SELECT * FROM t PIVOT(SUM(a) FOR b IN (1, 2))" {
				t.Errorf("base pivot clause: got %q, %v", got, err)
			}
		}
		if _, err := sqlglot.ParseOne("SELECT * FROM t pivot (a, b)", dialect); err == nil {
			t.Errorf("%s: `pivot (a, b)` must fail like upstream", dialect)
		}
	}
}

// Go-specific table-parts callers stay fail-closed when a PIVOT clause trails the target
// (upstream rejects these; parseTableParts parses pivots for every caller as upstream does).
func TestPivotClauseRejectedOnRestrictedTargets(t *testing.T) {
	for _, tc := range []struct{ dialect, sql string }{
		{"mysql", "TABLE t PIVOT(SUM(a) FOR b IN (1))"},
		{"mysql", "DROP INDEX i ON t PIVOT(SUM(a) FOR b IN (1))"},
		{"postgres", "DROP INDEX i ON t PIVOT(SUM(a) FOR b IN (1))"},
	} {
		if _, err := sqlglot.ParseOne(tc.sql, tc.dialect); err == nil {
			t.Errorf("%s %q: want parse error", tc.dialect, tc.sql)
		}
	}
}
