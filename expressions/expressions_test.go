package expressions_test

import (
	"reflect"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func parseOne(t *testing.T, sql string) exp.Expression {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, "")
	if err != nil {
		t.Fatalf("ParseOne(%q) error: %v", sql, err)
	}
	return expression
}

func TestToS(t *testing.T) {
	cases := []struct {
		sql  string
		want string
	}{
		{"5", "Literal(this=5, is_string=False)"},
		{"5.3", "Literal(this=5.3, is_string=False)"},
		{"True", "Boolean(this=True)"},
		{"'  x'", "Literal(this='  x', is_string=True)"},
		{"' \n  x'", "Literal(this=' \\n  x', is_string=True)"},
		{"   x ", "Column(\n  this=Identifier(this=x, quoted=False))"},
		{"\"   x \"", "Column(\n  this=Identifier(this='   x ', quoted=True))"},
	}
	for _, tc := range cases {
		if got := parseOne(t, tc.sql).ToS(); got != tc.want {
			t.Fatalf("ParseOne(%q).ToS() = %q, want %q", tc.sql, got, tc.want)
		}
	}
}

func TestEqOnSameInstanceShortCircuits(t *testing.T) {
	expression := exp.LiteralNumber(1)
	if !expression.Equal(expression) {
		t.Fatal("same expression should be equal")
	}
	hash := reflect.ValueOf(expression).Elem().FieldByName("hash")
	if !hash.IsNil() {
		t.Fatal("same-instance equality should not compute hash")
	}
}

func TestEqTrimmed(t *testing.T) {
	query := parseOne(t, "SELECT x FROM t")
	if !query.Equal(query.Copy()) {
		t.Fatal("query should equal its copy")
	}
	if exp.ToIdentifier("a").Equal(exp.ToIdentifier("A")) {
		t.Fatal("identifiers are hash-raw and should be case-sensitive")
	}
	left := exp.Column(exp.Args{"table": exp.ToIdentifier("b"), "this": exp.ToIdentifier("b")})
	right := exp.Column(exp.Args{"this": exp.ToIdentifier("b"), "table": exp.ToIdentifier("b")})
	if !left.Equal(right) {
		t.Fatal("column equality should be independent of arg insertion order")
	}
	if exp.ToIdentifier("A", true).Equal(exp.ToIdentifier("A")) {
		t.Fatal("quoted flag should affect identifier equality")
	}
	if parseOne(t, "'x'").Equal(parseOne(t, "'X'")) {
		t.Fatal("string literals should be case-sensitive")
	}
	if parseOne(t, "'1'").Equal(parseOne(t, "1")) {
		t.Fatal("string and number literals should not be equal")
	}
	if !parseOne(t, "select a, b+1").Equal(parseOne(t, "SELECT a, b + 1")) {
		t.Fatal("case-insensitive non-raw args should compare equal")
	}
}

func TestFindAllSubqueries(t *testing.T) {
	expression := parseOne(t, `
		SELECT *
		FROM (
			SELECT b.*
			FROM a.b b
		) x
		JOIN (
		  SELECT c.foo
		  FROM a.c c
		  WHERE foo = 1
		) y
		  ON x.c = y.foo
		CROSS JOIN (
		  SELECT *
		  FROM (
			SELECT d.bar
			FROM d
		  ) nested
		) z
		  ON x.c = y.foo
	`)
	assertNames(t, expression.FindAll(exp.KindTable), []string{"b", "c", "d"})
}

func TestFindAll(t *testing.T) {
	expression := parseOne(t, "select a + b + c + d")
	assertNames(t, expression.FindAll(exp.KindColumn), []string{"d", "c", "a", "b"})
	assertNames(t, expression.FindAll(exp.KindColumn, false), []string{"a", "b", "c", "d"})
}

// TestReplaceSingleValueArg guards the core tree-rewrite primitive against a regression
// where Replace silently no-op'd on single-value (non-list) args. Those carry index<0
// ("no index", i.e. upstream's index=None); the earlier code routed them through SetAt,
// whose list-index guard dropped the mutation. Mirrors the intent of upstream test_replace,
// asserted on the AST here since .sql() is a later slice.
func TestReplaceSingleValueArg(t *testing.T) {
	// Add.this is a single-value arg (the broken path).
	add := parseOne(t, "a + b")
	old := add.This()
	returned := old.Replace(parseOne(t, "z"))
	if got := add.This().Name(); got != "z" {
		t.Fatalf("Replace single-value arg: Add.this name = %q, want z", got)
	}
	if returned.Name() != "z" {
		t.Fatalf("Replace should return the new node; got name %q", returned.Name())
	}
	if old.Parent() != nil {
		t.Fatal("Replace should detach the replaced node from its parent")
	}
	if got := add.Expr().Name(); got != "b" {
		t.Fatalf("Replace clobbered a sibling arg: Add.expression name = %q, want b", got)
	}
	if got, err := add.SQL(exp.GenerateOptions{}); err != nil || got != "z + b" {
		t.Fatalf("Replace single-value arg SQL = %q, %v; want z + b", got, err)
	}

	// List-element args (Select.expressions[0]) must keep working too.
	sel := parseOne(t, "SELECT a, b FROM x")
	sel.Find(exp.KindColumn).Replace(parseOne(t, "c"))
	if got := sel.Expressions()[0].Name(); got != "c" {
		t.Fatalf("Replace list-element arg: first projection = %q, want c", got)
	}
	if got := sel.Expressions()[1].Name(); got != "b" {
		t.Fatalf("Replace list-element arg clobbered sibling: second projection = %q, want b", got)
	}
	if got, err := sel.SQL(exp.GenerateOptions{}); err != nil || got != "SELECT c, b FROM x" {
		t.Fatalf("Replace list-element arg SQL = %q, %v; want SELECT c, b FROM x", got, err)
	}
}

// TestPopSingleValueArg is the Pop counterpart to TestReplaceSingleValueArg (Pop is
// Replace(nil)); mirrors upstream test_arg_deletion.
func TestPopSingleValueArg(t *testing.T) {
	// Not.this is a single-value arg — Pop must remove it (previously a silent no-op).
	not := parseOne(t, "NOT a")
	child := not.This()
	popped := child.Pop()
	if not.This() != nil {
		t.Fatalf("Pop single-value arg: Not.this still present:\n%s", not.ToS())
	}
	if popped != child {
		t.Fatal("Pop should return the popped node")
	}
	if child.Parent() != nil {
		t.Fatal("Pop should detach the node from its parent")
	}

	// List-element pop must keep working.
	sel := parseOne(t, "SELECT a, b FROM x")
	sel.Find(exp.KindColumn).Pop()
	if got := len(sel.Expressions()); got != 1 {
		t.Fatalf("Pop list element: projection count = %d, want 1", got)
	}
	if got := sel.Expressions()[0].Name(); got != "b" {
		t.Fatalf("Pop list element removed the wrong column: remaining = %q, want b", got)
	}
}

func TestFindAncestor(t *testing.T) {
	column := parseOne(t, "select * from foo where (a + 1 > 2)").Find(exp.KindColumn)
	if column == nil || column.Kind() != exp.KindColumn {
		t.Fatalf("column = %#v", column)
	}
	if parentSelect := column.ParentSelect(); parentSelect == nil || parentSelect.Kind() != exp.KindSelect {
		t.Fatalf("parent select = %#v", parentSelect)
	}
	if ancestor := column.FindAncestor(exp.KindJoin); ancestor != nil {
		t.Fatalf("join ancestor = %#v, want nil", ancestor)
	}
}

func TestIdentifier(t *testing.T) {
	if !boolArg(exp.ToIdentifier("\"x\"").Arg("quoted")) {
		t.Fatal("identifier containing quotes should be quoted")
	}
	if boolArg(exp.ToIdentifier("x").Arg("quoted")) {
		t.Fatal("safe identifier should not be quoted")
	}
	if !boolArg(exp.ToIdentifier("foo ").Arg("quoted")) {
		t.Fatal("identifier with spaces should be quoted")
	}
	if boolArg(exp.ToIdentifier("_x").Arg("quoted")) {
		t.Fatal("underscore-prefixed identifier should not be quoted")
	}
}

func TestColumnPartial(t *testing.T) {
	column := parseOne(t, "a.b.c.d")
	if column.CatalogName() != "a" || column.DbName() != "b" || column.TableName() != "c" || column.Name() != "d" {
		t.Fatalf("column parts = catalog:%q db:%q table:%q name:%q", column.CatalogName(), column.DbName(), column.TableName(), column.Name())
	}

	column = parseOne(t, "a")
	if column.Name() != "a" || column.TableName() != "" {
		t.Fatalf("single column name/table = %q/%q", column.Name(), column.TableName())
	}
}

func TestTextPartial(t *testing.T) {
	column := parseOne(t, "a.b.c.d.e")
	if column.Text("expression") != "e" {
		t.Fatalf("dot expression text = %q, want e", column.Text("expression"))
	}
	if column.Text("y") != "" {
		t.Fatalf("missing text = %q, want empty", column.Text("y"))
	}
	table := parseOne(t, "select * from x.y").Find(exp.KindTable)
	if table == nil || table.Text("db") != "x" {
		t.Fatalf("table db text = %q, want x", table.Text("db"))
	}
	if parseOne(t, "select *").Name() != "" {
		t.Fatalf("select name = %q, want empty", parseOne(t, "select *").Name())
	}
	if parseOne(t, "1 + 1").Name() != "1" {
		t.Fatalf("binary name = %q, want 1", parseOne(t, "1 + 1").Name())
	}
	if parseOne(t, "'a'").Name() != "a" {
		t.Fatalf("literal name = %q, want a", parseOne(t, "'a'").Name())
	}
}

func assertNames(t *testing.T, expressions []exp.Expression, want []string) {
	t.Helper()
	if len(expressions) != len(want) {
		t.Fatalf("names length = %d, want %d", len(expressions), len(want))
	}
	for i, expression := range expressions {
		if expression.Name() != want[i] {
			t.Fatalf("name %d = %q, want %q", i, expression.Name(), want[i])
		}
	}
}

func boolArg(value any) bool {
	b, _ := value.(bool)
	return b
}
