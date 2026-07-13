package optimizer_test

import (
	"fmt"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
	"github.com/ridi/sqlglot-go/schema"
)

func TestQualifyResolutionReport(t *testing.T) {
	t.Run("source kind contract", func(t *testing.T) {
		var source optimizer.ResolvedSource
		if source.Kind != optimizer.Unresolved {
			t.Fatalf("zero-value ResolvedSource.Kind = %v, want Unresolved", source.Kind)
		}

		cases := []struct {
			kind optimizer.SourceKind
			want string
		}{
			{optimizer.Unresolved, "Unresolved"},
			{optimizer.Physical, "Physical"},
			{optimizer.CTE, "CTE"},
			{optimizer.Derived, "Derived"},
			{optimizer.Subquery, "Subquery"},
			{optimizer.SourceKind(99), "SourceKind(99)"},
		}
		for _, tc := range cases {
			if got := tc.kind.String(); got != tc.want {
				t.Errorf("SourceKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
			}
		}
	})

	t.Run("CTE reference is not physical", func(t *testing.T) {
		result, report := qualifyWithResolutionReport(t,
			"WITH orders AS (SELECT id, total FROM sales) UPDATE sink SET x = (SELECT total FROM orders o WHERE o.id = sink.id)",
			schema.M(
				"sink", schema.M("id", "INT", "x", "INT"),
				"sales", schema.M("id", "INT", "total", "INT"),
			),
			func(opts *optimizer.QualifyOpts) {
				opts.Dialect = "postgres"
				opts.ValidateQualifyColumns = false
			},
		)

		sink := findTableByName(t, result, "sink")
		sales := findTableByName(t, result, "sales")
		orders := findTableByName(t, result, "orders")
		assertResolvedSource(t, report, sink, optimizer.Physical, "", "", "sink")
		assertResolvedSource(t, report, sales, optimizer.Physical, "", "", "sales")
		assertResolvedSource(t, report, orders, optimizer.CTE, "", "", "")

		if orders.Kind() != exp.KindTable {
			t.Fatalf("orders reference kind = %v, want KindTable", orders.Kind())
		}
		cte := report[orders]
		if cte.Catalog != "" || cte.Schema != "" || cte.Table != "" {
			t.Fatalf("CTE identity = (%q, %q, %q), want empty", cte.Catalog, cte.Schema, cte.Table)
		}
		for expression, resolved := range report {
			if resolved.Kind != optimizer.Physical && (resolved.Catalog != "" || resolved.Schema != "" || resolved.Table != "") {
				t.Errorf("non-physical %v has identity (%q, %q, %q)", expression.Kind(), resolved.Catalog, resolved.Schema, resolved.Table)
			}
		}
		assertASTInvariants(t, result)
	})

	t.Run("physical identity preserves all table parts", func(t *testing.T) {
		result, report := qualifyWithResolutionReport(t,
			"SELECT id FROM catalog_name.schema_name.table_name",
			schema.M(
				"catalog_name", schema.M(
					"schema_name", schema.M(
						"table_name", schema.M("id", "INT"),
					),
				),
			), nil,
		)

		table := findTableByName(t, result, "table_name")
		assertResolvedSource(t, report, table, optimizer.Physical, "catalog_name", "schema_name", "table_name")
		assertASTInvariants(t, result)
	})

	t.Run("derived and predicate subqueries use unwrapped query keys", func(t *testing.T) {
		result, report := qualifyWithResolutionReport(t,
			"SELECT d.id FROM (SELECT id FROM sales) AS d WHERE EXISTS (SELECT 1 FROM audit WHERE audit.id = d.id)",
			schema.M(
				"sales", schema.M("id", "INT"),
				"audit", schema.M("id", "INT"),
			), nil,
		)

		var derived exp.Expression
		for _, subquery := range result.FindAll(exp.KindSubquery) {
			if subquery.AliasOrName() == "d" {
				derived = subquery
				break
			}
		}
		exists := result.Find(exp.KindExists)
		if derived == nil || exists == nil || exists.This() == nil {
			t.Fatalf("subqueries: derived=%v predicate=%v", derived != nil, exists != nil && exists.This() != nil)
		}

		derivedQuery := derived.This().Unwrap()
		predicateQuery := exists.This().Unwrap()
		assertResolvedSource(t, report, derivedQuery, optimizer.Derived, "", "", "")
		assertResolvedSource(t, report, predicateQuery, optimizer.Subquery, "", "", "")
		if _, ok := report[derived]; ok {
			t.Error("derived report key is the Subquery wrapper, want its unwrapped query")
		}
		if _, ok := report[exists]; ok {
			t.Error("predicate report key is the Exists expression, want its unwrapped query")
		}
		assertResolvedSource(t, report, findTableByName(t, result, "sales"), optimizer.Physical, "", "", "sales")
		assertResolvedSource(t, report, findTableByName(t, result, "audit"), optimizer.Physical, "", "", "audit")
		assertASTInvariants(t, result)
	})

	// R2 composes with R3: a DML's FROM/USING/JOIN read-sources are classified (not left
	// Unresolved) because R2 populates the report from the analysis traversal, which carries R3's
	// DML-root scopes. R2 alone (or without R3) would report the source as Unresolved — only the
	// exact DML target is Physical. This is the composition the PR-#7 review flagged.
	t.Run("DML FROM source classifies under R3", func(t *testing.T) {
		result, report := qualifyWithResolutionReport(t,
			"UPDATE sink SET x = dangling.x FROM dangling",
			schema.M(
				"sink", schema.M("x", "INT"),
				"dangling", schema.M("x", "INT"),
			),
			func(opts *optimizer.QualifyOpts) { opts.ValidateQualifyColumns = false },
		)

		// Target and FROM source both resolve to their physical tables.
		assertResolvedSource(t, report, findTableByName(t, result, "sink"), optimizer.Physical, "", "", "sink")
		assertResolvedSource(t, report, findTableByName(t, result, "dangling"), optimizer.Physical, "", "", "dangling")
		assertASTInvariants(t, result)
	})

	// The leak-critical DML case: a DML FROM source that is a CTE must classify CTE, never
	// Physical — the DML analog of the CTE-vs-physical leak, now reachable because R3 scopes the
	// DML source and R2 reads it from the analysis traversal.
	t.Run("DML FROM source that is a CTE is not physical", func(t *testing.T) {
		result, report := qualifyWithResolutionReport(t,
			"WITH c AS (SELECT id FROM sales) UPDATE sink SET x = 1 FROM c WHERE sink.id = c.id",
			schema.M(
				"sink", schema.M("id", "INT", "x", "INT"),
				"sales", schema.M("id", "INT"),
			),
			func(opts *optimizer.QualifyOpts) { opts.ValidateQualifyColumns = false },
		)

		assertResolvedSource(t, report, findTableByName(t, result, "sink"), optimizer.Physical, "", "", "sink")
		assertResolvedSource(t, report, findTableByName(t, result, "sales"), optimizer.Physical, "", "", "sales")
		// `c` is the CTE reference in the UPDATE FROM — must be CTE, not the physical guess.
		assertResolvedSource(t, report, findTableByName(t, result, "c"), optimizer.CTE, "", "", "")
		assertASTInvariants(t, result)
	})

	t.Run("report works when column qualification is disabled", func(t *testing.T) {
		result, report := qualifyWithResolutionReport(t,
			"SELECT id FROM sales",
			schema.M("sales", schema.M("id", "INT")),
			func(opts *optimizer.QualifyOpts) {
				opts.QualifyColumns = false
				opts.ValidateQualifyColumns = false
			},
		)

		assertResolvedSource(t, report, findTableByName(t, result, "sales"), optimizer.Physical, "", "", "sales")
		column := result.Find(exp.KindColumn)
		if column == nil {
			t.Fatal("qualified result has no column")
		}
		if got := column.TableName(); got != "" {
			t.Fatalf("column table = %q, want empty with QualifyColumns disabled", got)
		}
		assertASTInvariants(t, result)
	})

	t.Run("caller-owned entries are preserved and current entries overwritten", func(t *testing.T) {
		expression := parseOneForResolution(t, "SELECT id FROM sales")
		table := findTableByName(t, expression, "sales")
		unrelated := parseOneForResolution(t, "SELECT 1")
		report := map[exp.Expression]optimizer.ResolvedSource{
			table:     {Kind: optimizer.CTE},
			unrelated: {Kind: optimizer.Derived},
		}
		opts := optimizer.DefaultQualifyOpts()
		opts.Schema = schema.M("sales", schema.M("id", "INT"))
		opts.ResolutionReport = report

		result := optimizer.Qualify(expression, opts)
		assertResolvedSource(t, report, table, optimizer.Physical, "", "", "sales")
		assertResolvedSource(t, report, unrelated, optimizer.Derived, "", "", "")
		assertASTInvariants(t, result)
	})

	t.Run("nil report is a no-op", func(t *testing.T) {
		if opts := optimizer.DefaultQualifyOpts(); opts.ResolutionReport != nil {
			t.Fatalf("DefaultQualifyOpts().ResolutionReport = %#v, want nil", opts.ResolutionReport)
		}

		const query = "WITH active AS (SELECT id FROM sales) SELECT a.id FROM active AS a WHERE EXISTS (SELECT 1 FROM audit WHERE audit.id = a.id)"
		testSchema := schema.M(
			"sales", schema.M("id", "INT"),
			"audit", schema.M("id", "INT"),
		)
		withoutReport := parseAndQualify(t, query, testSchema, nil)
		report := make(map[exp.Expression]optimizer.ResolvedSource)
		withReport := parseAndQualify(t, query, testSchema, report)

		withoutSQL := generateSQL(t, withoutReport)
		withSQL := generateSQL(t, withReport)
		if withSQL != withoutSQL {
			t.Fatalf("Qualify with report = %q, without report = %q", withSQL, withoutSQL)
		}
		if len(report) == 0 {
			t.Fatal("requested resolution report is empty")
		}
		assertASTInvariants(t, withoutReport)
		assertASTInvariants(t, withReport)
	})
}

func qualifyWithResolutionReport(t *testing.T, query string, testSchema *schema.Mapping, configure func(*optimizer.QualifyOpts)) (exp.Expression, map[exp.Expression]optimizer.ResolvedSource) {
	t.Helper()
	report := make(map[exp.Expression]optimizer.ResolvedSource)
	expression := parseOneForResolution(t, query)
	opts := optimizer.DefaultQualifyOpts()
	opts.Schema = testSchema
	opts.ResolutionReport = report
	if configure != nil {
		configure(&opts)
	}
	return optimizer.Qualify(expression, opts), report
}

func parseAndQualify(t *testing.T, query string, testSchema *schema.Mapping, report map[exp.Expression]optimizer.ResolvedSource) exp.Expression {
	t.Helper()
	expression := parseOneForResolution(t, query)
	opts := optimizer.DefaultQualifyOpts()
	opts.Schema = testSchema
	opts.ResolutionReport = report
	return optimizer.Qualify(expression, opts)
}

func parseOneForResolution(t *testing.T, query string) exp.Expression {
	t.Helper()
	expression, err := sqlglot.ParseOne(query, "postgres")
	if err != nil {
		t.Fatalf("ParseOne(%q): %v", query, err)
	}
	return expression
}

func generateSQL(t *testing.T, expression exp.Expression) string {
	t.Helper()
	sql, err := sqlglot.Generate(expression, "postgres", generator.Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return sql
}

func findTableByName(t *testing.T, root exp.Expression, name string) exp.Expression {
	t.Helper()
	var matches []exp.Expression
	for _, table := range root.FindAll(exp.KindTable) {
		if table.Name() == name {
			matches = append(matches, table)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("found %d KindTable nodes named %q, want 1", len(matches), name)
	}
	return matches[0]
}

func assertResolvedSource(t *testing.T, report map[exp.Expression]optimizer.ResolvedSource, expression exp.Expression, kind optimizer.SourceKind, catalog, schemaName, table string) {
	t.Helper()
	resolved, ok := report[expression]
	if !ok {
		t.Fatalf("no resolution record for %s", expressionSummary(expression))
	}
	if resolved.Kind != kind || resolved.Catalog != catalog || resolved.Schema != schemaName || resolved.Table != table {
		t.Fatalf("resolution for %s = {%s %q %q %q}, want {%s %q %q %q}",
			expressionSummary(expression), resolved.Kind, resolved.Catalog, resolved.Schema, resolved.Table,
			kind, catalog, schemaName, table,
		)
	}
}

func expressionSummary(expression exp.Expression) string {
	if expression == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v(%q)", expression.Kind(), expression.Name())
}
