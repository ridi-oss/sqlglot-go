package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Set-returning function signatures parse structurally: `RETURNS TABLE (…)` (upstream) and
// PostgreSQL `RETURNS SETOF <type>` (DEVIATIONS "Grammar extensions beyond upstream").
func TestCreateFunctionReturnsTableAndSetof(t *testing.T) {
	cases := []struct {
		dialect string
		sql     string
		setof   bool
		table   bool
	}{
		{"postgres", `CREATE FUNCTION hr.f(d DATE) RETURNS TABLE (a INT, "personId" INT) LANGUAGE sql STABLE AS $$ SELECT 1 $$`, false, true},
		{"postgres", "CREATE FUNCTION hr.f(d DATE) RETURNS SETOF INT LANGUAGE sql STABLE AS $$ SELECT 1 $$", true, false},
		{"postgres", "CREATE FUNCTION hr.f() RETURNS SETOF hr.t LANGUAGE sql AS $$ SELECT 1 $$", true, false},
		{"postgres", "CREATE FUNCTION hr.f() RETURNS INT LANGUAGE sql AS $$ SELECT 1 $$", false, false},
	}
	for _, c := range cases {
		t.Run(c.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(c.sql, c.dialect)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindCreate || e.Text("kind") != "FUNCTION" {
				t.Fatalf("kind = %v, want Create:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
			returns := e.FindAll(exp.KindReturnsProperty)
			if len(returns) != 1 || (returns[0].Arg("setof") == true) != c.setof || (returns[0].Arg("is_table") == true) != c.table {
				t.Fatalf("ReturnsProperty want setof=%v is_table=%v:\n%s", c.setof, c.table, e.ToS())
			}
			if c.table {
				schema, _ := returns[0].Arg("this").(exp.Expression)
				if schema == nil || schema.Kind() != exp.KindSchema || len(schema.Expressions()) != 2 || schema.Expressions()[1].Kind() != exp.KindColumnDef {
					t.Fatalf("want a TABLE schema of two ColumnDefs:\n%s", e.ToS())
				}
			}
			out, err := sqlglot.Generate(e, c.dialect, generator.Options{})
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			if out != c.sql {
				t.Fatalf("round-trip = %q, want %q", out, c.sql)
			}
		})
	}
	if e, err := sqlglot.ParseOne("CREATE FUNCTION f() RETURNS SETOF INT AS 'x'", "mysql"); err == nil && e.Kind() != exp.KindCommand {
		t.Fatalf("mysql SETOF should not structure:\n%s", e.ToS())
	}
}
