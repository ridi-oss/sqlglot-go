package generator_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
)

func TestTrinoJSONQueryRoundTrip(t *testing.T) {
	queries := []string{
		"JSON_QUERY(m.properties, 'lax $.area' OMIT QUOTES NULL ON ERROR)",
		"JSON_QUERY(content, 'lax $.HY.*')",
		"JSON_QUERY(content, 'strict $.HY.*' WITH WRAPPER)",
		"JSON_QUERY(content, 'strict $.HY.*' WITH ARRAY WRAPPER)",
		"JSON_QUERY(content, 'strict $.HY.*' WITH UNCONDITIONAL WRAPPER)",
		"JSON_QUERY(content, 'strict $.HY.*' WITHOUT CONDITIONAL WRAPPER)",
		"JSON_QUERY(description, 'strict $.comment' KEEP QUOTES)",
		"JSON_QUERY(description, 'strict $.comment' OMIT QUOTES ON SCALAR STRING)",
		"JSON_QUERY(content, 'strict $.HY.*' WITH UNCONDITIONAL WRAPPER KEEP QUOTES)",
	}

	for _, dialect := range []string{"trino", "athena"} {
		for _, query := range queries {
			t.Run(dialect+"/"+query, func(t *testing.T) {
				if got := roundTrip(t, dialect, query); got != query {
					t.Fatalf("%s %q ->\n  got  %q\n  want %q", dialect, query, got, query)
				}
			})
		}
	}
}

func TestJSONExtractExistingDialectRendering(t *testing.T) {
	cases := []struct {
		dialect string
		sql     string
		want    string
	}{
		{"", "JSON_EXTRACT(content, json_path)", "JSON_EXTRACT(content, json_path)"},
		{"", "JSON_EXTRACT(a, '$.b', '$.c')", "JSON_EXTRACT(a, '$.b', '$.c')"},
		{"mysql", "JSON_EXTRACT(content, json_path)", "JSON_EXTRACT(content, json_path)"},
		{"postgres", "JSON_EXTRACT(content, json_path)", "JSON_EXTRACT_PATH(content, json_path)"},
		{"postgres", "content -> 'x'", "content -> 'x'"},
		{"trino", "JSON_EXTRACT(content, json_path)", "JSON_EXTRACT(content, json_path)"},
	}

	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

func TestJSONQueryRenderingIsTrinoFamilyOnly(t *testing.T) {
	expression, err := sqlglot.ParseOne("JSON_QUERY(content, 'lax $.HY.*')", "trino")
	if err != nil {
		t.Fatalf("ParseOne error: %v", err)
	}

	cases := []struct {
		dialect string
		want    string
	}{
		{"", "JSON_EXTRACT(content, 'lax $.HY.*')"},
		{"mysql", "JSON_EXTRACT(content, 'lax $.HY.*')"},
		{"postgres", "JSON_EXTRACT_PATH(content, 'lax $.HY.*')"},
		{"trino", "JSON_QUERY(content, 'lax $.HY.*')"},
		{"athena", "JSON_QUERY(content, 'lax $.HY.*')"},
	}

	for _, tc := range cases {
		got, err := sqlglot.Generate(expression, tc.dialect, generator.Options{})
		if err != nil {
			t.Fatalf("Generate(%q) error: %v", tc.dialect, err)
		}
		if got != tc.want {
			t.Errorf("Generate(%q) = %q, want %q", tc.dialect, got, tc.want)
		}
	}
}

func TestRefreshMaterializedViewRoundTrip(t *testing.T) {
	const query = "REFRESH MATERIALIZED VIEW mynamespace.test_view"
	for _, dialect := range []string{"trino", "athena"} {
		if got := roundTrip(t, dialect, query); got != query {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", dialect, query, got, query)
		}
	}
}

func TestOverflowTruncateBehaviorRendering(t *testing.T) {
	cases := []struct {
		args expressions.Args
		want string
	}{
		{expressions.Args{"with_count": true}, "TRUNCATE WITH COUNT"},
		{expressions.Args{"with_count": false}, "TRUNCATE WITHOUT COUNT"},
		{expressions.Args{"this": expressions.LiteralString("..."), "with_count": true}, "TRUNCATE '...' WITH COUNT"},
		{expressions.Args{"this": expressions.LiteralString("..."), "with_count": false}, "TRUNCATE '...' WITHOUT COUNT"},
	}

	for _, tc := range cases {
		expression := expressions.New(expressions.KindOverflowTruncateBehavior, tc.args)
		got, err := sqlglot.Generate(expression, "trino", generator.Options{})
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}
		if got != tc.want {
			t.Errorf("Generate(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestTrinoVersionKeepsCanonicalRendering(t *testing.T) {
	// VERSION -> CURRENT_VERSION is the canonical Func fallback. The Trino VERSION transform is
	// intentionally out of scope for this part.
	for _, dialect := range []string{"trino", "athena"} {
		if got := roundTrip(t, dialect, "SELECT VERSION()"); got != "SELECT CURRENT_VERSION()" {
			t.Errorf("%s VERSION rendering = %q, want %q", dialect, got, "SELECT CURRENT_VERSION()")
		}
	}
}

func TestTrinoListaggAndTrimParseShapeOnly(t *testing.T) {
	// Trino's GroupConcat -> LISTAGG and Trim standard-form spellings require generator
	// TRANSFORMS, which are explicitly out of scope. These assertions therefore stop at the
	// parser shape and do not round-trip either expression through the generator.
	for _, dialect := range []string{"trino", "athena"} {
		t.Run(dialect+"/listagg", func(t *testing.T) {
			expression, err := sqlglot.ParseOne("SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE '...' WITH COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl", dialect)
			if err != nil {
				t.Fatalf("ParseOne error: %v", err)
			}
			listagg := expression.Find(expressions.KindGroupConcat)
			if listagg == nil {
				t.Fatalf("LISTAGG did not parse to GroupConcat: %s", expression.ToS())
			}
			overflow, ok := listagg.Arg("on_overflow").(expressions.Expression)
			if !ok || overflow == nil || overflow.Kind() != expressions.KindOverflowTruncateBehavior {
				t.Fatalf("LISTAGG on_overflow = %#v, want OverflowTruncateBehavior", listagg.Arg("on_overflow"))
			}
			if !boolValueForTest(overflow.Arg("with_count")) {
				t.Fatalf("LISTAGG overflow with_count = %#v, want true", overflow.Arg("with_count"))
			}
		})

		t.Run(dialect+"/trim", func(t *testing.T) {
			expression, err := sqlglot.ParseOne("SELECT TRIM(BOTH '$' FROM '$var$')", dialect)
			if err != nil {
				t.Fatalf("ParseOne error: %v", err)
			}
			if trim := expression.Find(expressions.KindTrim); trim == nil {
				t.Fatalf("TRIM did not parse to Trim: %s", expression.ToS())
			}
		})
	}
}

func boolValueForTest(value any) bool {
	result, _ := value.(bool)
	return result
}
