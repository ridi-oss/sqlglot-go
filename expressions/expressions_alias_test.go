package expressions_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestAliasColumnNames(t *testing.T) {
	expression := parseOne(t, "SELECT * FROM (SELECT * FROM x) AS y")
	subquery := expression.Find(exp.KindSubquery)
	assertNames(t, aliasColumnNames(subquery), []string{})

	expression = parseOne(t, "SELECT * FROM (SELECT * FROM x) AS y(a)")
	subquery = expression.Find(exp.KindSubquery)
	assertNames(t, aliasColumnNames(subquery), []string{"a"})

	expression = parseOne(t, "SELECT * FROM (SELECT * FROM x) AS y(a, b)")
	subquery = expression.Find(exp.KindSubquery)
	assertNames(t, aliasColumnNames(subquery), []string{"a", "b"})

	expression = parseOne(t, "WITH y AS (SELECT * FROM x) SELECT * FROM y")
	cte := expression.Find(exp.KindCTE)
	assertNames(t, aliasColumnNames(cte), []string{})

	expression = parseOne(t, "WITH y(a, b) AS (SELECT * FROM x) SELECT * FROM y")
	cte = expression.Find(exp.KindCTE)
	assertNames(t, aliasColumnNames(cte), []string{"a", "b"})

	expression = parseOne(t, "SELECT * FROM tbl AS tbl(a, b)")
	table := expression.Find(exp.KindTable)
	assertNames(t, aliasColumnNames(table), []string{"a", "b"})
}

func TestCTEs(t *testing.T) {
	expression := parseOne(t, "SELECT a FROM x")
	if got := len(expression.FindAll(exp.KindCTE)); got != 0 {
		t.Fatalf("CTE count = %d, want 0", got)
	}

	expression = parseOne(t, "WITH x AS (SELECT a FROM y) SELECT a FROM x")
	ctes := expression.FindAll(exp.KindCTE)
	if len(ctes) != 1 {
		t.Fatalf("CTE count = %d, want 1", len(ctes))
	}
	if got := ctes[0].Arg("alias").(exp.Expression).This().Name(); got != "x" {
		t.Fatalf("CTE alias = %q, want x", got)
	}
}

func aliasColumnNames(expression exp.Expression) []exp.Expression {
	if expression == nil {
		return nil
	}
	alias, _ := expression.Arg("alias").(exp.Expression)
	if alias == nil {
		return nil
	}
	columns, _ := alias.Arg("columns").([]exp.Expression)
	return columns
}
