package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

type athenaLineageCase struct {
	name        string
	sql         string
	schema      *schema.Mapping
	wantScopes  int
	wantSources []string
	wantSQL     string
}

func TestAthenaPublicAPIQualifyAndLineage(t *testing.T) {
	cases := []athenaLineageCase{
		{
			name:        "simple projection and filter",
			sql:         "SELECT * FROM sample_users WHERE enabled = TRUE",
			schema:      schema.M("sample_users", schema.M("user_id", "BIGINT", "enabled", "BOOLEAN")),
			wantScopes:  1,
			wantSources: []string{"sample_users"},
			wantSQL:     "SELECT \"sample_users\".\"user_id\" AS \"user_id\", \"sample_users\".\"enabled\" AS \"enabled\" FROM \"sample_users\" AS \"sample_users\" WHERE \"sample_users\".\"enabled\" = TRUE",
		},
		{
			name: "multi-table join",
			sql:  "SELECT c.customer_id, o.order_total FROM sample_customers AS c JOIN sample_orders AS o ON c.customer_id = o.customer_id WHERE o.order_total > 0",
			schema: schema.M(
				"sample_customers", schema.M("customer_id", "BIGINT", "region_id", "BIGINT"),
				"sample_orders", schema.M("order_id", "BIGINT", "customer_id", "BIGINT", "order_total", "DOUBLE"),
			),
			wantScopes:  1,
			wantSources: []string{"c", "o"},
		},
		{
			name: "CTE aggregate",
			sql:  "WITH category_totals AS (SELECT category_id, SUM(amount) AS total_amount FROM sample_sales GROUP BY category_id) SELECT category_id, total_amount FROM category_totals WHERE total_amount > 100",
			schema: schema.M(
				"sample_sales", schema.M("category_id", "BIGINT", "amount", "DOUBLE"),
			),
			wantScopes:  2,
			wantSources: []string{"sample_sales", "category_totals"},
		},
		{
			name:        "window query",
			sql:         "SELECT user_id, ROW_NUMBER() OVER (PARTITION BY group_id ORDER BY event_time DESC) AS row_position FROM sample_activity",
			schema:      schema.M("sample_activity", schema.M("user_id", "BIGINT", "group_id", "BIGINT", "event_time", "TIMESTAMP")),
			wantScopes:  1,
			wantSources: []string{"sample_activity"},
		},
		{
			name: "nested union",
			sql:  "SELECT user_id FROM (SELECT user_id FROM sample_current_users UNION ALL SELECT user_id FROM sample_archived_users) AS combined_users",
			schema: schema.M(
				"sample_current_users", schema.M("user_id", "BIGINT"),
				"sample_archived_users", schema.M("user_id", "BIGINT"),
			),
			wantScopes:  4,
			wantSources: []string{"sample_current_users", "sample_archived_users", "combined_users"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expression, err := sqlglot.ParseOne(tc.sql, "athena")
			if err != nil {
				t.Fatalf("ParseOne(athena): %v", err)
			}

			opts := optimizer.DefaultQualifyOpts()
			opts.Dialect = "athena"
			opts.Schema = tc.schema
			qualified := optimizer.Qualify(expression, opts)
			scopes := optimizer.TraverseScope(qualified)
			if len(scopes) != tc.wantScopes {
				t.Fatalf("TraverseScope(athena) len = %d, want %d", len(scopes), tc.wantScopes)
			}

			selectedSources := map[string]bool{}
			for i, scope := range scopes {
				for _, name := range scope.SelectedSourceNames() {
					selectedSources[name] = true
				}
				if external := scope.ExternalColumns(); len(external) != 0 {
					t.Fatalf("scope %d has %d unresolved external columns", i, len(external))
				}
			}
			if len(selectedSources) == 0 {
				t.Fatal("qualified query has no selected sources")
			}
			for _, source := range tc.wantSources {
				if !selectedSources[source] {
					t.Errorf("selected sources missing %q: %v", source, selectedSources)
				}
			}

			if tc.wantSQL != "" {
				got, err := sqlglot.Generate(qualified, "athena", generator.Options{})
				if err != nil {
					t.Fatalf("Generate(athena): %v", err)
				}
				if got != tc.wantSQL {
					t.Fatalf("qualified Athena SQL = %q, want %q", got, tc.wantSQL)
				}
			}
		})
	}
}

func TestAthenaPublicAPIDDLRouting(t *testing.T) {
	externalTable, err := sqlglot.ParseOne(
		"CREATE EXTERNAL TABLE sample_archive (record_id BIGINT, payload STRING) STORED AS PARQUET LOCATION 's3://sanitized-bucket/sample/'",
		"athena",
	)
	if err != nil {
		t.Fatalf("ParseOne(athena external table): %v", err)
	}
	if externalTable.Kind() != exp.KindCreate {
		t.Fatalf("external table kind = %v, want Create", externalTable.Kind())
	}
	for _, kind := range []exp.Kind{exp.KindExternalProperty, exp.KindFileFormatProperty, exp.KindLocationProperty} {
		if athenaKindCount(externalTable, kind) == 0 {
			t.Errorf("external table is missing %v", kind)
		}
	}
	if commands := athenaKindCount(externalTable, exp.KindCommand); commands != 0 {
		t.Fatalf("external table contains %d Command nodes", commands)
	}

	ctas, err := sqlglot.ParseOne(
		"CREATE TABLE sample_summary AS SELECT category_id, SUM(amount) AS total_amount FROM sample_sales GROUP BY category_id",
		"athena",
	)
	if err != nil {
		t.Fatalf("ParseOne(athena CTAS): %v", err)
	}
	if ctas.Kind() != exp.KindCreate {
		t.Fatalf("CTAS kind = %v, want Create", ctas.Kind())
	}
	if commands := athenaKindCount(ctas, exp.KindCommand); commands != 0 {
		t.Fatalf("CTAS contains %d Command nodes", commands)
	}
	query, _ := ctas.Arg("expression").(exp.Expression)
	if query == nil {
		t.Fatal("CTAS is missing its structured query expression")
	}
	scopes := optimizer.TraverseScope(query)
	if len(scopes) == 0 {
		t.Fatal("CTAS query has no traversable scopes")
	}
	foundSource := false
	for _, scope := range scopes {
		for _, source := range scope.SelectedSourceNames() {
			if source == "sample_sales" {
				foundSource = true
			}
		}
	}
	if !foundSource {
		t.Fatal("CTAS query scopes are missing sample_sales")
	}
}

func athenaKindCount(expression exp.Expression, kind exp.Kind) int {
	count := 0
	for _, node := range expression.Walk() {
		if node.Kind() == kind {
			count++
		}
	}
	return count
}

func TestAthenaOpaqueFunctionSettings(t *testing.T) {
	const dialect = "athena, opaque_functions=true"
	for _, call := range []string{"SUBSTR(a, 1, 2)", "CONCAT(a, b)", "date_format(x, 'y')"} {
		sql := "SELECT " + call + " FROM t"
		expression, err := sqlglot.ParseOne(sql, dialect)
		if err != nil {
			t.Fatal(err)
		}
		if got := expression.Expressions()[0]; got.Kind() != exp.KindAnonymous {
			t.Fatalf("%s: expected opaque function, got %s", call, got.ToS())
		}
		generated, err := sqlglot.Generate(expression, dialect, generator.Options{})
		if err != nil || generated != sql {
			t.Fatalf("%s: generated %q, %v", sql, generated, err)
		}
		reparsed, err := sqlglot.ParseOne(generated, dialect)
		if err != nil {
			t.Fatal(err)
		}
		again, err := sqlglot.Generate(reparsed, dialect, generator.Options{})
		if err != nil || again != generated {
			t.Fatalf("not idempotent: %q, %v", again, err)
		}
	}
	for _, tc := range []struct {
		sql  string
		kind exp.Kind
	}{
		{"SELECT SUBSTRING(a FROM 1 FOR 2)", exp.KindSubstring},
		{"SELECT CAST(a AS VARCHAR)", exp.KindCast},
	} {
		e, err := sqlglot.ParseOne(tc.sql, dialect)
		if err != nil || e.Expressions()[0].Kind() != tc.kind {
			t.Fatalf("%s: got %v, %v", tc.sql, e, err)
		}
	}
}

func TestAthenaMixedBatchSpans(t *testing.T) {
	for _, tc := range []struct {
		sql   string
		first exp.Kind
		spans []string
	}{
		{`SHOW TABLES; SELECT "$path" FROM t`, exp.KindShow, []string{"SHOW TABLES", `SELECT "$path" FROM t`}},
		{"-- lead\nCREATE TABLE `t` (x INT); SELECT \"x\" FROM \"t\";", exp.KindCreate, []string{"CREATE TABLE `t` (x INT)", `SELECT "x" FROM "t"`}},
	} {
		expressions, err := sqlglot.Parse(tc.sql, "athena")
		if err != nil || len(expressions) != len(tc.spans) {
			t.Fatalf("%q: %v, %v", tc.sql, expressions, err)
		}
		if expressions[0].Kind() != tc.first || expressions[1].Kind() != exp.KindSelect {
			t.Fatalf("unexpected statement kinds: %v, %v", expressions[0].Kind(), expressions[1].Kind())
		}
		for i, e := range expressions {
			if text, ok := e.SpanText(); !ok || text != tc.spans[i] {
				t.Fatalf("statement %d span = %q, want %q", i, text, tc.spans[i])
			}
		}
		column := expressions[1].Expressions()[0]
		if column.Kind() != exp.KindColumn || column.This().Arg("quoted") != true {
			t.Fatalf("query projection must be a quoted column: %s", column.ToS())
		}
	}
	for _, sql := range []string{";; SELECT 1; SELECT 2;", "-- lead\nSELECT '한;글'; -- middle\nSELECT \"x;y\"; /* end */", "SELECT 1;  ", "  "} {
		got, err := sqlglot.Parse(sql, "athena")
		if err != nil {
			t.Fatal(err)
		}
		want, err := sqlglot.Parse(sql, "postgres")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("%q: got %d statements, want %d", sql, len(got), len(want))
		}
		for i := range got {
			if got[i] == nil || want[i] == nil {
				if got[i] != nil || want[i] != nil {
					t.Fatalf("%q: statement %d nil mismatch", sql, i)
				}
				continue
			}
			gotText, gotOK := got[i].SpanText()
			wantText, wantOK := want[i].SpanText()
			if gotText != wantText || gotOK != wantOK {
				t.Fatalf("%q: span %d = %q, want %q", sql, i, gotText, wantText)
			}
		}
	}
}

func TestAthenaNormalizationSettings(t *testing.T) {
	for _, tc := range []struct {
		dialect string
		want    string
	}{
		{"athena", `SELECT "mixed", unquoted FROM "table"`},
		{"athena, normalization_strategy=case_sensitive", `SELECT "MiXeD", Unquoted FROM "TaBlE"`},
		{"athena, normalization_strategy=uppercase", `SELECT "MiXeD", UNQUOTED FROM "TaBlE"`},
	} {
		e, err := sqlglot.ParseOne(`SELECT "MiXeD", Unquoted FROM "TaBlE"`, tc.dialect)
		if err != nil {
			t.Fatal(err)
		}
		e = optimizer.NormalizeIdentifiers(e, tc.dialect)
		got, err := sqlglot.Generate(e, tc.dialect, generator.Options{})
		if err != nil || got != tc.want {
			t.Fatalf("%s: got %q, %v, want %q", tc.dialect, got, err, tc.want)
		}
	}
}
