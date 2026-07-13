package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/parser"
)

func TestParseEmpty(t *testing.T) {
	if _, err := sqlglot.ParseOne("", ""); err == nil {
		t.Fatal("expected ParseError")
	}
}

func TestIdentify(t *testing.T) {
	expression, err := sqlglot.ParseOne(`
            SELECT a, "b", c AS c, d AS "D", e AS "y|z'"
            FROM y."z"
        `, "")
	if err != nil {
		t.Fatalf("ParseOne identify error: %v", err)
	}

	expressions := expression.Expressions()
	if expressions[0].Name() != "a" {
		t.Fatalf("expression 0 name = %q", expressions[0].Name())
	}
	if expressions[1].Name() != "b" {
		t.Fatalf("expression 1 name = %q", expressions[1].Name())
	}
	if expressions[2].Alias() != "c" {
		t.Fatalf("expression 2 alias = %q", expressions[2].Alias())
	}
	if expressions[3].Alias() != "D" {
		t.Fatalf("expression 3 alias = %q", expressions[3].Alias())
	}
	if expressions[4].Alias() != "y|z'" {
		t.Fatalf("expression 4 alias = %q", expressions[4].Alias())
	}
	table := expression.Arg("from_").(exp.Expression).This()
	if table.Name() != "z" {
		t.Fatalf("table name = %q", table.Name())
	}
	if db := table.Arg("db").(exp.Expression); db.Name() != "y" {
		t.Fatalf("table db = %q", db.Name())
	}
}

func TestMulti(t *testing.T) {
	expressions, err := sqlglot.Parse(`
            SELECT * FROM a; SELECT * FROM b;
        `, "")
	if err != nil {
		t.Fatalf("Parse multi error: %v", err)
	}
	if len(expressions) != 2 {
		t.Fatalf("expression count = %d, want 2", len(expressions))
	}
	if expressions[0].Arg("from_").(exp.Expression).Name() != "a" {
		t.Fatalf("first FROM name = %q", expressions[0].Arg("from_").(exp.Expression).Name())
	}
	if expressions[1].Arg("from_").(exp.Expression).Name() != "b" {
		t.Fatalf("second FROM name = %q", expressions[1].Arg("from_").(exp.Expression).Name())
	}

	expressions, err = sqlglot.Parse("SELECT 1; ; SELECT 2", "")
	if err != nil {
		t.Fatalf("Parse empty middle statement error: %v", err)
	}
	if len(expressions) != 3 {
		t.Fatalf("expression count = %d, want 3", len(expressions))
	}
	if expressions[1] != nil {
		t.Fatalf("middle expression = %#v, want nil", expressions[1])
	}
}

func TestExpression(t *testing.T) {
	ignore := parser.NewWithErrorLevel(dialects.Base(), sqlerrors.IGNORE)
	if got := ignore.Expression(exp.Hint(exp.Args{"expressions": []exp.Expression{}})); got.Kind() != exp.KindHint {
		t.Fatalf("ignore hint kind = %v", got.Kind())
	}
	if got := ignore.Expression(exp.New(exp.KindHint, exp.Args{"y": ""})); got.Kind() != exp.KindHint {
		t.Fatalf("ignore invalid hint kind = %v", got.Kind())
	}
	if got := ignore.Expression(exp.Hint(nil)); got.Kind() != exp.KindHint {
		t.Fatalf("ignore missing hint kind = %v", got.Kind())
	}

	defaultParser := parser.NewWithErrorLevel(dialects.Base(), sqlerrors.RAISE)
	assertPanicsAsArgError(t, func() { defaultParser.Expression(exp.New(exp.KindHint, exp.Args{"y": ""})) })
	if got := defaultParser.Expression(exp.Hint(exp.Args{"expressions": []exp.Expression{}})); got.Kind() != exp.KindHint {
		t.Fatalf("default hint kind = %v", got.Kind())
	}
	defaultParser.Expression(exp.Hint(nil))
	if len(defaultParser.Errors()) != 2 {
		t.Fatalf("default parser errors = %d, want 2", len(defaultParser.Errors()))
	}

	warn := parser.NewWithErrorLevel(dialects.Base(), sqlerrors.WARN)
	warn.Expression(exp.Hint(nil))
	if len(warn.Errors()) != 1 {
		t.Fatalf("warn parser errors = %d, want 1", len(warn.Errors()))
	}
}

func TestJoinOnSmoke(t *testing.T) {
	expression, err := sqlglot.ParseOne("SELECT * FROM a JOIN b ON a.x = b.y", "")
	if err != nil {
		t.Fatalf("ParseOne join error: %v", err)
	}
	joins, ok := expression.Arg("joins").([]exp.Expression)
	if !ok || len(joins) != 1 {
		t.Fatalf("joins = %#v, want len 1", expression.Arg("joins"))
	}
	on := joins[0].Arg("on").(exp.Expression)
	if on.Kind() != exp.KindEQ {
		t.Fatalf("join on kind = %v, want EQ", on.Kind())
	}
}

// TestParseAliases locks in the _parse_alias L_PAREN fix: `<expr> AS (a, b)` must build
// exp.Aliases (this + expressions), matching upstream. The earlier code built an
// exp.Tuple{this: ...}, but Tuple declares no `this` arg, so expression() raised an
// ArgError instead of producing a valid node.
func TestParseAliases(t *testing.T) {
	expression, err := sqlglot.ParseOne("SELECT x AS (a, b) FROM t", "")
	if err != nil {
		t.Fatalf("ParseOne aliases error: %v", err)
	}
	aliases := expression.Find(exp.KindAliases)
	if aliases == nil {
		t.Fatalf("expected an Aliases node, got:\n%s", expression.ToS())
	}
	if got := aliases.This().Name(); got != "x" {
		t.Fatalf("Aliases.this name = %q, want x", got)
	}
	if got := len(aliases.Expressions()); got != 2 {
		t.Fatalf("Aliases expressions count = %d, want 2", got)
	}
}

// TestQualifiedColumnDotChain locks in the _parse_column_ops plain-DOT fix: a
// dot-continuation field that is a non-VAR keyword or a star forces the slow
// path, which must produce a bare Identifier/Star field (not a Column-wrapped
// one) so Name()/Text("table")/Text("db") resolve. Parity verified against
// Python sqlglot 30.12.0.
func TestQualifiedColumnDotChain(t *testing.T) {
	cases := []struct {
		sql, name, table, db string
	}{
		{"SELECT t.end", "end", "t", ""},
		{"SELECT a.b.*", "*", "b", "a"},
		{"SELECT a.b.c.*", "*", "c", "b"},
		{"SELECT t.end.foo", "foo", "end", "t"},
	}
	for _, tc := range cases {
		expression, err := sqlglot.ParseOne(tc.sql, "")
		if err != nil {
			t.Fatalf("ParseOne(%q) error: %v", tc.sql, err)
		}
		col := expression.Find(exp.KindColumn)
		if col == nil {
			t.Fatalf("%q: no column found", tc.sql)
		}
		if col.Name() != tc.name || col.Text("table") != tc.table || col.Text("db") != tc.db {
			t.Fatalf("%q: got Name=%q table=%q db=%q; want %q/%q/%q",
				tc.sql, col.Name(), col.Text("table"), col.Text("db"), tc.name, tc.table, tc.db)
		}
	}
}

func assertPanicsAsArgError(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if _, ok := r.(*exp.ArgError); !ok {
			t.Fatalf("panic = %T %#v, want *expressions.ArgError", r, r)
		}
	}()
	f()
}
