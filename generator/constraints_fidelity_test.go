package generator_test

import (
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestConstraintFidelityTransforms(t *testing.T) {
	cases := []struct {
		name string
		node exp.Expression
		want string
	}{
		{
			"compress scalar",
			exp.CompressColumnConstraint(exp.Args{"this": exp.LiteralString("a")}),
			"COMPRESS 'a'",
		},
		{
			"compress list",
			exp.CompressColumnConstraint(exp.Args{"this": []exp.Expression{
				exp.LiteralString("a"), exp.LiteralString("b"),
			}}),
			"COMPRESS ('a', 'b')",
		},
		{
			"date format",
			exp.DateFormatColumnConstraint(exp.Args{"this": exp.LiteralString("YYYY-MM-DD")}),
			"FORMAT 'YYYY-MM-DD'",
		},
		{
			"inline length",
			exp.InlineLengthColumnConstraint(exp.Args{"this": numLit("1")}),
			"INLINE LENGTH 1",
		},
		{
			"title",
			exp.TitleColumnConstraint(exp.Args{"this": exp.LiteralString("title")}),
			"TITLE 'title'",
		},
		{
			"uppercase",
			exp.UppercaseColumnConstraint(nil),
			"UPPERCASE",
		},
		{
			"with operator",
			exp.WithOperator(exp.Args{
				"this": exp.Column(exp.Args{"this": exp.ToIdentifier("c")}),
				"op":   exp.Var(exp.Args{"this": "&&"}),
			}),
			"c WITH &&",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := genSQL(t, nil, tc.node); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExcludeColumnConstraintSQL(t *testing.T) {
	withOp := exp.WithOperator(exp.Args{
		"this": exp.Ordered(exp.Args{
			"this": exp.Opclass(exp.Args{
				"this":       exp.Column(exp.Args{"this": exp.ToIdentifier("col")}),
				"expression": exp.Table(exp.Args{"this": exp.ToIdentifier("varchar_pattern_ops")}),
			}),
			"desc":        true,
			"nulls_first": false,
		}),
		"op": exp.Var(exp.Args{"this": "&&"}),
	})
	params := exp.IndexParameters(exp.Args{
		"using":   exp.Var(exp.Args{"this": "gist"}),
		"columns": []exp.Expression{withOp},
		"with_storage": []exp.Expression{
			exp.Property(exp.Args{"this": exp.Var(exp.Args{"this": "sp1"}), "value": numLit("1")}),
			exp.Property(exp.Args{"this": exp.Var(exp.Args{"this": "sp2"}), "value": numLit("2")}),
		},
	})
	node := exp.ExcludeColumnConstraint(exp.Args{"this": params})
	want := "EXCLUDE USING gist(col varchar_pattern_ops DESC NULLS LAST WITH &&) WITH (sp1=1, sp2=2)"
	if got := genSQL(t, dialects.Postgres(), node); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestInOutColumnConstraintSQL(t *testing.T) {
	cases := []struct {
		name string
		d    *dialects.Dialect
		args exp.Args
		want string
	}{
		{"in", nil, exp.Args{"input_": true}, "IN"},
		{"out", nil, exp.Args{"output": true}, "OUT"},
		{"base in out", nil, exp.Args{"input_": true, "output": true}, "IN OUT"},
		{"postgres inout", dialects.Postgres(), exp.Args{"input_": true, "output": true}, "INOUT"},
		{"variadic wins", dialects.Postgres(), exp.Args{"input_": true, "output": true, "variadic": true}, "VARIADIC"},
		{"empty", nil, exp.Args{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := genSQL(t, tc.d, exp.InOutColumnConstraint(tc.args)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
