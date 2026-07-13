package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestSelectSubqueryClauses(t *testing.T) {
	expression := parseOne(t, "select * from (select 1) x order by x.y")
	if expression.Arg("order") == nil {
		t.Fatalf("order clause is nil:\n%s", expression.ToS())
	}

	expression = parseOne(t, "select * from (select 1) x cross join y")
	joins := expressionsForArg(expression, "joins")
	if len(joins) != 1 {
		t.Fatalf("join count = %d, want 1", len(joins))
	}

	expression = parseOne(t, "select * from x where a = (select 1) order by x.y")
	if expression.Arg("order") == nil {
		t.Fatalf("order clause after WHERE subquery is nil:\n%s", expression.ToS())
	}
	where := expression.Arg("where").(exp.Expression)
	if where.Find(exp.KindSubquery) == nil {
		t.Fatalf("WHERE subquery not found:\n%s", where.ToS())
	}
}
