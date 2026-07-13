package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
	"github.com/ridi/sqlglot-go/schema"
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
