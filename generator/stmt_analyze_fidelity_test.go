package generator_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestAnalyzeAuxiliarySQL locks the three renderers used by MySQL ANALYZE
// HISTOGRAM against generator.py:142,279,5922-5929.
func TestAnalyzeAuxiliarySQL(t *testing.T) {
	column := exp.Column_("col1", nil, nil, nil, nil)
	cases := []struct {
		name string
		node exp.Expression
		want string
	}{
		{
			name: "analyze with",
			node: exp.AnalyzeWith(exp.Args{"expressions": []string{"5 BUCKETS", "ASYNC MODE"}}),
			want: "WITH 5 BUCKETS WITH ASYNC MODE",
		},
		{
			name: "using data",
			node: exp.UsingData(exp.Args{"this": exp.LiteralString("json_data")}),
			want: "USING DATA 'json_data'",
		},
		{
			name: "histogram with automatic update",
			node: exp.AnalyzeHistogram(exp.Args{
				"this":           "UPDATE",
				"expressions":    []exp.Expression{column},
				"expression":     exp.AnalyzeWith(exp.Args{"expressions": []string{"5 BUCKETS"}}),
				"update_options": "AUTO",
			}),
			want: "UPDATE HISTOGRAM ON col1 WITH 5 BUCKETS AUTO UPDATE",
		},
		{
			name: "histogram using data",
			node: exp.AnalyzeHistogram(exp.Args{
				"this":        "UPDATE",
				"expressions": []exp.Expression{column.Copy()},
				"expression":  exp.UsingData(exp.Args{"this": exp.LiteralString("json_data")}),
			}),
			want: "UPDATE HISTOGRAM ON col1 USING DATA 'json_data'",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := genSQL(t, nil, tc.node); got != tc.want {
				t.Fatalf("Generate() = %q, want %q", got, tc.want)
			}
		})
	}
}
