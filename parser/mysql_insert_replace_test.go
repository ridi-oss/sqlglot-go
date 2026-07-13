package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func assertMySQLSetInsertShape(t *testing.T, insert exp.Expression, columns []string, values []string) {
	t.Helper()

	target := exprArg(t, insert, "this")
	if target.Kind() != exp.KindSchema {
		t.Fatalf("target kind = %v, want Schema:\n%s", target.Kind(), insert.ToS())
	}
	table := exprArg(t, target, "this")
	if table.Kind() != exp.KindTable || table.Name() != "t" {
		t.Fatalf("schema target = %v %q, want Table t:\n%s", table.Kind(), table.Name(), insert.ToS())
	}

	gotColumns := target.Expressions()
	if len(gotColumns) != len(columns) {
		t.Fatalf("column count = %d, want %d:\n%s", len(gotColumns), len(columns), insert.ToS())
	}
	for i, column := range gotColumns {
		if column.Kind() != exp.KindIdentifier || column.Name() != columns[i] {
			t.Fatalf("column %d = %v %q, want Identifier %q:\n%s", i, column.Kind(), column.Name(), columns[i], insert.ToS())
		}
	}

	valuesExpression := exprArg(t, insert, "expression")
	if valuesExpression.Kind() != exp.KindValues {
		t.Fatalf("insert expression kind = %v, want Values:\n%s", valuesExpression.Kind(), insert.ToS())
	}
	rows := valuesExpression.Expressions()
	if len(rows) != 1 || rows[0].Kind() != exp.KindTuple {
		t.Fatalf("values rows = %#v, want one Tuple:\n%s", rows, insert.ToS())
	}
	gotValues := rows[0].Expressions()
	if len(gotValues) != len(values) {
		t.Fatalf("value count = %d, want %d:\n%s", len(gotValues), len(values), insert.ToS())
	}
	for i, value := range gotValues {
		if value.Name() != values[i] {
			t.Fatalf("value %d = %q, want %q:\n%s", i, value.Name(), values[i], insert.ToS())
		}
	}
}

func TestParseMySQLInsertSet(t *testing.T) {
	insert := parseOneDialect(t, "INSERT INTO t SET a = 1, b = 2", "mysql")
	if insert.Kind() != exp.KindInsert {
		t.Fatalf("kind = %v, want Insert:\n%s", insert.Kind(), insert.ToS())
	}
	if insert.Arg("replace") == true {
		t.Fatalf("replace = true, want false or nil:\n%s", insert.ToS())
	}
	assertMySQLSetInsertShape(t, insert, []string{"a", "b"}, []string{"1", "2"})

	const want = "INSERT INTO t (a, b) VALUES (1, 2)"
	got, err := generateSQL(t, insert, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != want {
		t.Fatalf("generated SQL = %q, want %q", got, want)
	}
	reparsed := parseOneDialect(t, got, "mysql")
	if !insert.Equal(reparsed) {
		t.Fatalf("normalized AST changed after reparse:\noriginal: %s\nreparsed: %s", insert.ToS(), reparsed.ToS())
	}

	insert = parseOneDialect(t, "INSERT INTO t SET a = 1 ON DUPLICATE KEY UPDATE b = 2", "mysql")
	if insert.Kind() != exp.KindInsert || insert.Arg("replace") == true {
		t.Fatalf("kind/replace = %v/%#v, want Insert without replace:\n%s", insert.Kind(), insert.Arg("replace"), insert.ToS())
	}
	assertMySQLSetInsertShape(t, insert, []string{"a"}, []string{"1"})
	conflict := exprArg(t, insert, "conflict")
	if conflict.Kind() != exp.KindOnConflict || conflict.Arg("duplicate") != true {
		t.Fatalf("conflict = %#v, want duplicate OnConflict:\n%s", conflict, insert.ToS())
	}
	if expressions := conflict.Expressions(); len(expressions) != 1 || expressions[0].Kind() != exp.KindEQ {
		t.Fatalf("conflict expressions should contain one EQ:\n%s", insert.ToS())
	}

	const wantDuplicate = "INSERT INTO t (a) VALUES (1) ON DUPLICATE KEY UPDATE b = 2"
	got, err = generateSQL(t, insert, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != wantDuplicate {
		t.Fatalf("generated SQL = %q, want %q", got, wantDuplicate)
	}
	reparsed = parseOneDialect(t, got, "mysql")
	if !insert.Equal(reparsed) {
		t.Fatalf("normalized duplicate-key AST changed after reparse:\noriginal: %s\nreparsed: %s", insert.ToS(), reparsed.ToS())
	}
}

func TestParseMySQLReplace(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		set  bool
	}{
		{name: "values", in: "REPLACE INTO t VALUES (1)", want: "REPLACE INTO t VALUES (1)"},
		{name: "columns", in: "REPLACE INTO t (a, b) VALUES (1, 2)", want: "REPLACE INTO t (a, b) VALUES (1, 2)"},
		{name: "select", in: "REPLACE t SELECT * FROM s", want: "REPLACE INTO t SELECT * FROM s"},
		{name: "set", in: "REPLACE INTO t SET a = 1, b = 2", want: "REPLACE INTO t (a, b) VALUES (1, 2)", set: true},
		{name: "table named table with columns", in: "REPLACE INTO table (a) VALUES (1)", want: "REPLACE INTO table (a) VALUES (1)"},
		{name: "table named table", in: "REPLACE INTO table SELECT id FROM table2 WHERE cnt > 100", want: "REPLACE INTO table SELECT id FROM table2 WHERE cnt > 100"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			insert := parseOneDialect(t, tc.in, "mysql")
			if insert.Kind() != exp.KindInsert {
				t.Fatalf("kind = %v, want Insert:\n%s", insert.Kind(), insert.ToS())
			}
			if insert.Arg("replace") != true {
				t.Fatalf("replace = %#v, want true:\n%s", insert.Arg("replace"), insert.ToS())
			}
			if tc.set {
				assertMySQLSetInsertShape(t, insert, []string{"a", "b"}, []string{"1", "2"})
			}

			got, err := generateSQL(t, insert, "mysql")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != tc.want {
				t.Fatalf("generated SQL = %q, want %q", got, tc.want)
			}

			reparsed := parseOneDialect(t, got, "mysql")
			if !insert.Equal(reparsed) {
				t.Fatalf("normalized AST changed after reparse:\noriginal: %s\nreparsed: %s", insert.ToS(), reparsed.ToS())
			}
			if reparsed.Arg("replace") != true {
				t.Fatalf("reparsed replace = %#v, want true:\n%s", reparsed.Arg("replace"), reparsed.ToS())
			}
		})
	}
}

func TestMySQLInsertReplaceGuardrails(t *testing.T) {
	const selectSQL = "SELECT REPLACE(a, 'x', 'y') FROM t"
	selectExpression := parseOneDialect(t, selectSQL, "mysql")
	if replace := selectExpression.Find(exp.KindReplace); replace == nil {
		t.Fatalf("SELECT does not contain a Replace function:\n%s", selectExpression.ToS())
	}
	got, err := generateSQL(t, selectExpression, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != selectSQL {
		t.Fatalf("generated SQL = %q, want %q", got, selectSQL)
	}

	replace := parseOneDialect(t, "REPLACE(a, 'x', 'y')", "mysql")
	if replace.Kind() != exp.KindReplace {
		t.Fatalf("kind = %v, want Replace:\n%s", replace.Kind(), replace.ToS())
	}

	markedInsert := parseOneDialect(t, "REPLACE INTO t VALUES (1)", "mysql")
	for _, dialect := range []string{"", "postgres"} {
		got, err = generateSQL(t, markedInsert, dialect)
		if err != nil {
			t.Fatalf("Generate marked Insert for dialect %q: %v", dialect, err)
		}
		if got != "INSERT INTO t VALUES (1)" {
			t.Fatalf("marked Insert generated for dialect %q = %q, want ordinary INSERT", dialect, got)
		}
	}

	for _, sql := range []string{
		"REPLACE LOCAL t VALUES (1)",
		"REPLACE INTO t VALUES (1) ON DUPLICATE KEY UPDATE a = 2",
		"REPLACE INTO t",
	} {
		t.Run("fallback "+sql, func(t *testing.T) {
			command := parseOneDialect(t, sql, "mysql")
			if command.Kind() != exp.KindCommand {
				t.Fatalf("kind = %v, want Command:\n%s", command.Kind(), command.ToS())
			}
			got, err := generateSQL(t, command, "mysql")
			if err != nil {
				t.Fatalf("Generate fallback Command: %v", err)
			}
			if got != sql {
				t.Fatalf("fallback SQL = %q, want source-preserving %q", got, sql)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		sql     string
		dialect string
	}{
		{name: "insert set base", sql: "INSERT INTO t SET a = 1, b = 2", dialect: ""},
		{name: "insert set postgres", sql: "INSERT INTO t SET a = 1, b = 2", dialect: "postgres"},
		{name: "replace base", sql: "REPLACE INTO t VALUES (1)", dialect: ""},
		{name: "replace postgres", sql: "REPLACE INTO t VALUES (1)", dialect: "postgres"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if expression, err := sqlglot.ParseOne(tc.sql, tc.dialect); err == nil {
				t.Fatalf("ParseOne(%q, %q) = %s, want error", tc.sql, tc.dialect, expression.ToS())
			}
		})
	}
}
