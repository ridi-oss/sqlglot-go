package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func trinoProjection(t *testing.T, sql string) exp.Expression {
	t.Helper()
	root := parseOneDialect(t, sql, "trino")
	projections := root.Expressions()
	if len(projections) != 1 {
		t.Fatalf("want one Trino projection, got %d:\n%s", len(projections), root.ToS())
	}
	return projections[0]
}

func TestTrinoRefreshAndNoParenFunctions(t *testing.T) {
	refresh := parseOneDialect(t, "REFRESH MATERIALIZED VIEW mynamespace.test_view", "trino")
	if refresh.Kind() != exp.KindRefresh || refresh.Arg("kind") != "MATERIALIZED VIEW" {
		t.Fatalf("REFRESH MATERIALIZED VIEW shape mismatch:\n%s", refresh.ToS())
	}
	if table := refresh.This(); table == nil || table.Kind() != exp.KindTable || table.Name() != "test_view" || table.Text("db") != "mynamespace" {
		t.Fatalf("REFRESH target mismatch:\n%s", refresh.ToS())
	}
	if got, err := generateSQL(t, refresh, "trino"); err != nil || got != "REFRESH MATERIALIZED VIEW mynamespace.test_view" {
		t.Fatalf("Trino REFRESH generation = %q, %v", got, err)
	}

	tableRefresh := parseOneDialect(t, "REFRESH TABLE test_table", "trino")
	if tableRefresh.Kind() != exp.KindRefresh || tableRefresh.Arg("kind") != "TABLE" {
		t.Fatalf("REFRESH TABLE shape mismatch:\n%s", tableRefresh.ToS())
	}
	literalRefresh := parseOneDialect(t, "REFRESH 'cache-key'", "trino")
	if literalRefresh.Kind() != exp.KindRefresh || literalRefresh.Arg("kind") != "" || literalRefresh.This() == nil || literalRefresh.This().Name() != "cache-key" {
		t.Fatalf("literal REFRESH shape mismatch:\n%s", literalRefresh.ToS())
	}
	fallback := parseOneDialect(t, "REFRESH unsupported_target", "trino")
	if fallback.Kind() != exp.KindCommand || fallback.Arg("this") != "unsupported_target" {
		t.Fatalf("unsupported REFRESH should preserve upstream Command fallback:\n%s", fallback.ToS())
	}

	catalog := trinoProjection(t, "SELECT CURRENT_CATALOG")
	if catalog.Kind() != exp.KindCurrentCatalog {
		t.Fatalf("CURRENT_CATALOG kind = %v, want CurrentCatalog:\n%s", catalog.Kind(), catalog.ToS())
	}
	for _, dialect := range []string{"", "mysql", "postgres", "presto", "hive"} {
		root := parseOneDialect(t, "SELECT CURRENT_CATALOG", dialect)
		if projection := root.Expressions()[0]; projection.Kind() == exp.KindCurrentCatalog {
			t.Fatalf("Trino CURRENT_CATALOG parser leaked to %q:\n%s", dialect, root.ToS())
		}
	}
	version := trinoProjection(t, "SELECT VERSION()")
	if version.Kind() != exp.KindCurrentVersion {
		t.Fatalf("VERSION() kind = %v, want CurrentVersion:\n%s", version.Kind(), version.ToS())
	}
}

func TestTrinoJSONQueryAndJSONValue(t *testing.T) {
	const sql = "SELECT JSON_QUERY(content, 'strict $.HY.*' WITHOUT CONDITIONAL WRAPPER KEEP QUOTES NULL ON ERROR)"
	jsonQuery := trinoProjection(t, sql)
	if jsonQuery.Kind() != exp.KindJSONExtract || jsonQuery.Arg("json_query") != true {
		t.Fatalf("JSON_QUERY should be JSONExtract(json_query=true):\n%s", jsonQuery.ToS())
	}
	if jsonQuery.This() == nil || jsonQuery.This().Name() != "content" {
		t.Fatalf("JSON_QUERY document mismatch:\n%s", jsonQuery.ToS())
	}
	path := exprArg(t, jsonQuery, "expression")
	if path.Kind() != exp.KindLiteral || path.Name() != "strict $.HY.*" {
		t.Fatalf("JSON_QUERY raw path mismatch:\n%s", jsonQuery.ToS())
	}
	option := exprArg(t, jsonQuery, "option")
	if option.Kind() != exp.KindVar || option.Name() != "WITHOUT CONDITIONAL WRAPPER" {
		t.Fatalf("JSON_QUERY option mismatch:\n%s", jsonQuery.ToS())
	}
	quote := exprArg(t, jsonQuery, "quote")
	if quote.Kind() != exp.KindJSONExtractQuote || quote.Arg("option") != "KEEP" || quote.Arg("scalar") != false {
		t.Fatalf("JSON_QUERY quote mismatch:\n%s", jsonQuery.ToS())
	}
	onCondition := exprArg(t, jsonQuery, "on_condition")
	if onCondition.Kind() != exp.KindOnCondition || onCondition.Arg("error") != "NULL ON ERROR" {
		t.Fatalf("JSON_QUERY ON ERROR mismatch:\n%s", jsonQuery.ToS())
	}
	if got, err := generateSQL(t, parseOneDialect(t, sql, "trino"), "trino"); err != nil || got != sql {
		t.Fatalf("Trino JSON_QUERY generation = %q, %v", got, err)
	}

	conditionalArray := trinoProjection(t, "SELECT JSON_QUERY(content, 'p' WITH CONDITIONAL ARRAY WRAPPED)")
	if option := exprArg(t, conditionalArray, "option"); option.Name() != "WITH CONDITIONAL ARRAY WRAPPED" {
		t.Fatalf("CONDITIONAL ARRAY WRAPPED option mismatch:\n%s", conditionalArray.ToS())
	}

	scalarQuote := trinoProjection(t, "SELECT JSON_QUERY(description, 'p' OMIT QUOTES ON SCALAR STRING ERROR ON EMPTY)")
	quote = exprArg(t, scalarQuote, "quote")
	if quote.Arg("option") != "OMIT" || quote.Arg("scalar") != true {
		t.Fatalf("OMIT QUOTES ON SCALAR STRING mismatch:\n%s", scalarQuote.ToS())
	}
	if empty := exprArg(t, scalarQuote, "on_condition").Arg("empty"); empty != "ERROR ON EMPTY" {
		t.Fatalf("JSON_QUERY ON EMPTY = %#v, want ERROR ON EMPTY", empty)
	}

	jsonValue := parseOneDialect(t, "JSON_VALUE(jl.extra_attributes, 'lax $.amount_source' RETURNING VARCHAR)", "trino")
	if jsonValue.Kind() != exp.KindJSONValue {
		t.Fatalf("JSON_VALUE kind = %v, want JSONValue:\n%s", jsonValue.Kind(), jsonValue.ToS())
	}
	if returning := exprArg(t, jsonValue, "returning"); returning.Name() != "VARCHAR" {
		t.Fatalf("JSON_VALUE RETURNING VARCHAR mismatch:\n%s", jsonValue.ToS())
	}
}

func TestTrinoRestoresTrimParser(t *testing.T) {
	trim := trinoProjection(t, "SELECT TRIM(BOTH '$' FROM '$var$')")
	if trim.Kind() != exp.KindTrim || trim.Arg("position") != "BOTH" {
		t.Fatalf("Trino TRIM should use the dedicated parser:\n%s", trim.ToS())
	}
	if trim.This() == nil || trim.This().Name() != "$var$" {
		t.Fatalf("Trino TRIM source mismatch:\n%s", trim.ToS())
	}
	if characters := exprArg(t, trim, "expression"); characters.Name() != "$" {
		t.Fatalf("Trino TRIM characters mismatch:\n%s", trim.ToS())
	}
}

func TestTrinoListaggOverflow(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		kind      exp.Kind
		filler    string
		withCount any
	}{
		{
			name: "error",
			sql:  "SELECT LISTAGG(col, '; ' ON OVERFLOW ERROR) WITHIN GROUP (ORDER BY col ASC) FROM tbl",
			kind: exp.KindVar,
		},
		{
			name:      "truncate default count",
			sql:       "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE) WITHIN GROUP (ORDER BY col ASC) FROM tbl",
			kind:      exp.KindOverflowTruncateBehavior,
			withCount: true,
		},
		{
			name:      "truncate filler without count",
			sql:       "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE '...' WITHOUT COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl",
			kind:      exp.KindOverflowTruncateBehavior,
			filler:    "...",
			withCount: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listagg := trinoProjection(t, tc.sql)
			if listagg.Kind() != exp.KindGroupConcat {
				t.Fatalf("LISTAGG kind = %v, want GroupConcat:\n%s", listagg.Kind(), listagg.ToS())
			}
			if listagg.This() == nil || listagg.This().Kind() != exp.KindOrder {
				t.Fatalf("LISTAGG WITHIN GROUP should store an Order in this:\n%s", listagg.ToS())
			}
			overflow := exprArg(t, listagg, "on_overflow")
			if overflow.Kind() != tc.kind {
				t.Fatalf("overflow kind = %v, want %v:\n%s", overflow.Kind(), tc.kind, listagg.ToS())
			}
			if tc.kind == exp.KindVar {
				if overflow.Name() != "ERROR" {
					t.Fatalf("ON OVERFLOW ERROR value mismatch:\n%s", listagg.ToS())
				}
				return
			}
			if overflow.Arg("with_count") != tc.withCount {
				t.Fatalf("with_count = %#v, want %#v:\n%s", overflow.Arg("with_count"), tc.withCount, listagg.ToS())
			}
			if tc.filler == "" {
				if overflow.This() != nil {
					t.Fatalf("default TRUNCATE filler should be nil:\n%s", listagg.ToS())
				}
			} else if overflow.This() == nil || overflow.This().Name() != tc.filler {
				t.Fatalf("TRUNCATE filler mismatch:\n%s", listagg.ToS())
			}
		})
	}
}

func TestTrinoInheritedParserFeatures(t *testing.T) {
	zoned := trinoProjection(t, "SELECT TIMESTAMP '2012-10-31 01:00:00 +02:00'")
	if zoned.Kind() != exp.KindCast || exprArg(t, zoned, "to").Arg("this") != exp.DTypeTimestampTz {
		t.Fatalf("Trino should inherit Presto's zoned timestamp constructor:\n%s", zoned.ToS())
	}

	analyze := parseOneDialect(t, "ANALYZE tbl", "trino")
	if analyze.Kind() != exp.KindAnalyze || analyze.This() == nil || analyze.This().Name() != "tbl" {
		t.Fatalf("Trino ANALYZE shape mismatch:\n%s", analyze.ToS())
	}
	create := parseOneDialect(t, "CREATE TABLE foo.bar WITH (LOCATION='s3://bucket/foo/bar') AS SELECT 1", "trino")
	if create.Kind() != exp.KindCreate || create.Arg("expression") == nil {
		t.Fatalf("Trino CTAS should stay structured:\n%s", create.ToS())
	}

	arrayFirst := trinoProjection(t, "SELECT ARRAY_FIRST(ARRAY['a', 'b'], x -> x = 'b') FROM tbl")
	if arrayFirst.Kind() != exp.KindArrayFirst {
		t.Fatalf("ARRAY_FIRST kind = %v, want ArrayFirst:\n%s", arrayFirst.Kind(), arrayFirst.ToS())
	}
	if predicate := exprArg(t, arrayFirst, "expression"); predicate.Kind() != exp.KindLambda {
		t.Fatalf("ARRAY_FIRST predicate should be Lambda:\n%s", arrayFirst.ToS())
	}
}

func TestTrinoMatchRecognizeDeferred(t *testing.T) {
	t.Skip("MATCH_RECOGNIZE is a documented pre-existing Presto parser gap")
}
