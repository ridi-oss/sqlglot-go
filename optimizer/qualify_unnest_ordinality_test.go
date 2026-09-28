package optimizer_test

import (
	"reflect"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

// Unnest.selects (array.py:277-282) includes the ordinal column, so the resolver sees it as a
// source column even though the parser pops it out of the alias column list.
func TestQualifyUnnestOrdinalityColumn(t *testing.T) {
	mapping, err := schema.NewMappingSchema(schema.M("t", schema.M("arr", "INT[]")), dialects.Postgres(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sql, want string }{
		{"SELECT v, n FROM unnest(ARRAY[1, 2]) WITH ORDINALITY AS u(v, n)", `SELECT "u"."v" AS "v", "u"."n" AS "n" FROM UNNEST(ARRAY[1, 2]) WITH ORDINALITY AS "u"("v", n)`},
		{"SELECT u.n FROM t, unnest(t.arr) WITH ORDINALITY AS u(v, n) WHERE u.n > 1", `SELECT "u"."n" AS "n" FROM "t" AS "t", UNNEST("t"."arr") WITH ORDINALITY AS "u"("v", n) WHERE "u"."n" > 1`},
		{"SELECT * FROM unnest(ARRAY[1, 2]) WITH ORDINALITY AS u(v, n)", `SELECT "u"."v" AS "v", "u"."n" AS "n" FROM UNNEST(ARRAY[1, 2]) WITH ORDINALITY AS "u"("v", n)`},
		{"SELECT * FROM unnest(ARRAY[1, 2]) WITH ORDINALITY AS u", `SELECT "u"."offset" AS "offset" FROM UNNEST(ARRAY[1, 2]) WITH ORDINALITY AS "u"`},
		{"SELECT v FROM unnest(ARRAY[1, 2]) WITH ORDINALITY AS u(v)", `SELECT "u"."v" AS "v" FROM UNNEST(ARRAY[1, 2]) WITH ORDINALITY AS "u"("v")`},
	} {
		e, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Fatal(err)
		}
		opts := optimizer.DefaultQualifyOpts()
		opts.Dialect = "postgres"
		opts.Schema = mapping
		got, err := sqlglot.Generate(optimizer.Qualify(e, opts), "postgres", generator.Options{})
		if err != nil || got != tc.want {
			t.Errorf("%s:\n  got  %q, %v\n  want %q", tc.sql, got, err, tc.want)
		}
	}
}

// AliasColumnNames stays the raw alias list (alias_column_names); NamedSelects adds the ordinal.
func TestUnnestOrdinalitySelectsShape(t *testing.T) {
	for _, tc := range []struct {
		dialect, sql   string
		aliases, names []string
	}{
		{"postgres", "SELECT * FROM unnest(a) WITH ORDINALITY AS u(v, n)", []string{"v"}, []string{"v", "n"}},
		{"postgres", "SELECT * FROM unnest(a) WITH ORDINALITY AS u", []string{}, []string{"offset"}},
		{"mysql", "SELECT * FROM UNNEST(a) AS t WITH OFFSET AS y", []string{}, []string{"y"}},
		{"mysql", "SELECT * FROM UNNEST(a) AS t(v) WITH OFFSET", []string{"v"}, []string{"v", "offset"}},
	} {
		e, err := sqlglot.ParseOne(tc.sql, tc.dialect)
		if err != nil {
			t.Fatal(err)
		}
		unnest := e.FindAll(exp.KindUnnest)[0]
		if got := unnest.AliasColumnNames(); !reflect.DeepEqual(got, tc.aliases) {
			t.Errorf("%s AliasColumnNames = %v, want %v", tc.sql, got, tc.aliases)
		}
		if got := unnest.NamedSelects(); !reflect.DeepEqual(got, tc.names) {
			t.Errorf("%s NamedSelects = %v, want %v", tc.sql, got, tc.names)
		}
	}
	mapping, err := schema.NewMappingSchema(schema.M("x", schema.M("a", "INT[]")), dialects.MySQL(), true)
	if err != nil {
		t.Fatal(err)
	}
	e, err := sqlglot.ParseOne("SELECT y FROM x, UNNEST(x.a) AS t(v) WITH OFFSET AS y", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect, opts.Schema, opts.QuoteIdentifiers, opts.Identify = "mysql", mapping, false, false
	want := "SELECT t.y AS y FROM x AS x, UNNEST(x.a) WITH ORDINALITY AS t(v, y)"
	if got, err := sqlglot.Generate(optimizer.Qualify(e, opts), "mysql", generator.Options{}); err != nil || got != want {
		t.Errorf("mysql WITH OFFSET: got %q, %v; want %q", got, err, want)
	}
}
