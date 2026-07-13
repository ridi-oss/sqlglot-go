package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestSetOperations(t *testing.T) {
	cases := []string{
		"SELECT * FROM (SELECT 1) UNION SELECT 2",
		"SELECT x FROM y HAVING x > (SELECT 1) UNION SELECT 2",
	}
	for _, sql := range cases {
		expression := parseOne(t, sql)
		if expression.Kind() != exp.KindUnion {
			t.Fatalf("%q kind = %v, want Union", sql, expression.Kind())
		}
	}
}

func TestSetOperationModifiersMoveToTopmostUnion(t *testing.T) {
	expression := parseOne(t, "SELECT x FROM t1 UNION ALL SELECT x FROM t2 LIMIT 1")
	if expression.Kind() != exp.KindUnion {
		t.Fatalf("kind = %v, want Union", expression.Kind())
	}
	if limit, ok := expression.Arg("limit").(exp.Expression); !ok || limit.Kind() != exp.KindLimit {
		t.Fatalf("top-level limit = %#v, want Limit", expression.Arg("limit"))
	}
	if expression.Arg("distinct") != false {
		t.Fatalf("distinct = %#v, want false", expression.Arg("distinct"))
	}

	expression = parseOne(t, "SELECT x FROM t1 UNION SELECT x FROM t2 UNION SELECT x FROM t3 LIMIT 1")
	if expression.Kind() != exp.KindUnion {
		t.Fatalf("kind = %v, want Union", expression.Kind())
	}
	if expression.This().Kind() != exp.KindUnion {
		t.Fatalf("left side kind = %v, want Union", expression.This().Kind())
	}
	if limit, ok := expression.Arg("limit").(exp.Expression); !ok || limit.Kind() != exp.KindLimit {
		t.Fatalf("top-level limit = %#v, want Limit", expression.Arg("limit"))
	}
}
