package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestCTE(t *testing.T) {
	expression := parseOne(t, "WITH x AS (SELECT a FROM y) SELECT a FROM x")
	with := expression.Find(exp.KindWith)
	if with == nil {
		t.Fatalf("With node not found:\n%s", expression.ToS())
	}
	cte := expression.Find(exp.KindCTE)
	if cte == nil {
		t.Fatalf("CTE node not found:\n%s", expression.ToS())
	}
	alias := cte.Arg("alias").(exp.Expression)
	if got := alias.This().Name(); got != "x" {
		t.Fatalf("CTE alias = %q, want x", got)
	}
}

func TestCTEColumnAliasList(t *testing.T) {
	expression := parseOne(t, "WITH y(a,b) AS (SELECT a,b FROM z) SELECT a FROM y")
	cte := expression.Find(exp.KindCTE)
	if cte == nil {
		t.Fatalf("CTE node not found:\n%s", expression.ToS())
	}
	alias := cte.Arg("alias").(exp.Expression)
	columns := expressionsForArg(alias, "columns")
	if len(columns) != 2 || columns[0].Name() != "a" || columns[1].Name() != "b" {
		t.Fatalf("CTE alias columns = %#v, want [a b]", columns)
	}
}

func TestRecursiveCTE(t *testing.T) {
	expression := parseOne(t, "WITH RECURSIVE x AS (SELECT 1) SELECT * FROM x")
	with := expression.Find(exp.KindWith)
	if with == nil {
		t.Fatalf("With node not found:\n%s", expression.ToS())
	}
	if with.Arg("recursive") != true {
		t.Fatalf("recursive = %#v, want true", with.Arg("recursive"))
	}
}
