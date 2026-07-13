package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestQualifiedFunctionCall covers _parse_column_ops' to_dot rewrite
// (parser.py:6793-6799): a schema-qualified function call must become a Dot
// chain leading to the call, NOT a Column whose `this` is a function.
func TestQualifiedFunctionCall(t *testing.T) {
	// my_schema.my_func(x) -> Dot(Identifier(my_schema), Anonymous(my_func, [x]))
	proj := parseOne(t, "SELECT my_schema.my_func(x) FROM t").Expressions()[0]
	if proj.Kind() != exp.KindDot {
		t.Fatalf("my_schema.my_func(x): got kind %d, want Dot:\n%s", proj.Kind(), proj.ToS())
	}
	if this := proj.This(); this == nil || this.Kind() != exp.KindIdentifier || this.Name() != "my_schema" {
		t.Fatalf("Dot.this should be Identifier(my_schema):\n%s", proj.ToS())
	}
	if e := proj.Expr(); e == nil || e.Kind() != exp.KindAnonymous || e.Name() != "my_func" {
		t.Fatalf("Dot.expression should be Anonymous(my_func):\n%s", proj.ToS())
	}

	// a.b.FOO(x) -> Dot(Dot(a, b), Anonymous(FOO, [x]))
	proj = parseOne(t, "SELECT a.b.FOO(x) FROM t").Expressions()[0]
	if proj.Kind() != exp.KindDot {
		t.Fatalf("a.b.FOO(x): got kind %d, want Dot:\n%s", proj.Kind(), proj.ToS())
	}
	if this := proj.This(); this == nil || this.Kind() != exp.KindDot {
		t.Fatalf("a.b.FOO(x): Dot.this should itself be a Dot:\n%s", proj.ToS())
	}
	if e := proj.Expr(); e == nil || e.Kind() != exp.KindAnonymous || e.Name() != "FOO" {
		t.Fatalf("a.b.FOO(x): Dot.expression should be Anonymous(FOO):\n%s", proj.ToS())
	}

	// A qualified window function keeps the Window at the top with a Dot as its func.
	proj = parseOne(t, "SELECT t.foo(x) OVER (PARTITION BY a) FROM t").Expressions()[0]
	if proj.Kind() != exp.KindWindow {
		t.Fatalf("t.foo(x) OVER (...): got kind %d, want Window:\n%s", proj.Kind(), proj.ToS())
	}
	if inner := proj.This(); inner == nil || inner.Kind() != exp.KindDot {
		t.Fatalf("qualified window func's `this` should be a Dot:\n%s", proj.ToS())
	}
}
