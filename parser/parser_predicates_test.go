package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestInPredicate(t *testing.T) {
	expression := parseOne(t, "x IN (SELECT y FROM t)")
	if expression.Kind() != exp.KindIn {
		t.Fatalf("kind = %v, want In", expression.Kind())
	}
	query, ok := expression.Arg("query").(exp.Expression)
	if !ok || query.Kind() != exp.KindSubquery {
		t.Fatalf("query = %#v, want Subquery", expression.Arg("query"))
	}

	expression = parseOne(t, "x IN (1,2)")
	if expression.Kind() != exp.KindIn {
		t.Fatalf("kind = %v, want In", expression.Kind())
	}
	if got := len(expression.Expressions()); got != 2 {
		t.Fatalf("IN expressions count = %d, want 2", got)
	}
}

func TestSubqueryAndQuantifierPredicates(t *testing.T) {
	if expression := parseOne(t, "EXISTS(SELECT 1)"); expression.Kind() != exp.KindExists {
		t.Fatalf("EXISTS kind = %v, want Exists", expression.Kind())
	}
	if expression := parseOne(t, "x LIKE ANY (y)"); expression.Kind() != exp.KindLike {
		t.Fatalf("LIKE ANY kind = %v, want Like", expression.Kind())
	}
	if expression := parseOne(t, "x ILIKE ANY (y)"); expression.Kind() != exp.KindILike {
		t.Fatalf("ILIKE ANY kind = %v, want ILike", expression.Kind())
	}
}

func TestBetweenIsDistinctCaseIf(t *testing.T) {
	expression := parseOne(t, "a BETWEEN SYMMETRIC 1 AND 2")
	if expression.Kind() != exp.KindBetween || expression.Arg("symmetric") != true {
		t.Fatalf("BETWEEN = %s, symmetric=%#v; want Between symmetric", expression.ToS(), expression.Arg("symmetric"))
	}

	expression = parseOne(t, "a IS DISTINCT FROM b")
	if expression.Kind() != exp.KindNullSafeNEQ {
		t.Fatalf("IS DISTINCT kind = %v, want NullSafeNEQ", expression.Kind())
	}
	expression = parseOne(t, "a IS NOT DISTINCT FROM b")
	if expression.Kind() != exp.KindNullSafeEQ {
		t.Fatalf("IS NOT DISTINCT kind = %v, want NullSafeEQ", expression.Kind())
	}

	expression = parseOne(t, "CASE WHEN a THEN 1 ELSE 2 END")
	if expression.Kind() != exp.KindCase {
		t.Fatalf("CASE kind = %v, want Case", expression.Kind())
	}
	ifs := expressionsForArg(expression, "ifs")
	if len(ifs) != 1 || ifs[0].Kind() != exp.KindIf {
		t.Fatalf("CASE ifs = %#v, want one If", ifs)
	}

	expression = parseOne(t, "IF(a,b,c)")
	if expression.Kind() != exp.KindIf {
		t.Fatalf("IF kind = %v, want If", expression.Kind())
	}
}
