package generator_test

// Round-trip checks for lambdaSQL (generator/lambda.go), which ports lambda_sql
// (generator.py:2861-2864). Cases are drawn from testdata/parity_gaps.txt (base "gen:
// mismatch" entries #2,3,5,8,12,13,14 - see ROADMAP/plan notes for the exact line numbers).

import "testing"

func TestLambdaSQL(t *testing.T) {
	cases := []struct{ sql, want string }{
		// Single param: no parens around the arrow's left side.
		{"SELECT TRANSFORM(a, b -> b) AS x", "SELECT TRANSFORM(a, b -> b) AS x"},
		{`SELECT X(a -> a + ("z" - 1))`, `SELECT X(a -> a + ("z" - 1))`},
		// Multiple params: parenthesized, flat-joined with ", ".
		{"SELECT AGGREGATE(a, (a, b) -> a + b) AS x", "SELECT AGGREGATE(a, (a, b) -> a + b) AS x"},
		{"SELECT X((a, b) -> a + b, z -> z) AS x", "SELECT X((a, b) -> a + b, z -> z) AS x"},
		// Deep dot chains in the lambda body round-trip unchanged.
		{"FILTER(a, x -> x.a.b.c.d.e.f.g)", "FILTER(a, x -> x.a.b.c.d.e.f.g)"},
		{
			"FILTER(a, x -> FOO(x.a.b.c.d.e.f.g) + x.a.b.c.d.e.f.g)",
			"FILTER(a, x -> FOO(x.a.b.c.d.e.f.g) + x.a.b.c.d.e.f.g)",
		},
		{
			`REGEXP_REPLACE('new york', '(\w)(\w*)', x -> UPPER(x[1]) || LOWER(x[2]))`,
			`REGEXP_REPLACE('new york', '(\w)(\w*)', x -> UPPER(x[1]) || LOWER(x[2]))`,
		},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "", tc.sql); got != tc.want {
			t.Errorf("%q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestLambdaSQLNotAJSONArrow guards the parseFunctionArgs-only scope of lambda parsing: a
// top-level `a -> b` (outside any function call's argument list) must keep parsing as
// exp.JSONExtract, not exp.Lambda (parser.go:1341/parser_lambda.go). JSON `->`/`->>`
// round-tripping to the operator form (rather than JSON_EXTRACT(...)) is itself a separate,
// deliberately-deferred gap (parity_gaps.txt:161,221; not this part's scope) - these
// assertions only pin the *kind* of divergence (still JSON_EXTRACT(...), not a lambda arrow)
// so a future fix to that gap has a clean signal if this regresses instead.
func TestLambdaSQLNotAJSONArrow(t *testing.T) {
	if got := roundTrip(t, "", "SELECT a -> b"); got != "SELECT JSON_EXTRACT(a, b)" {
		t.Errorf("SELECT a -> b -> %q, want JSON_EXTRACT(a, b) (JSONExtract, not Lambda)", got)
	}
	if got := roundTrip(t, "postgres", "SELECT x::JSON -> 'd' ->> -1"); got != "SELECT JSON_EXTRACT_SCALAR(JSON_EXTRACT(CAST(x AS JSON), 'd'), -1)" {
		t.Errorf("postgres JSON arrow chain -> %q", got)
	}
}
