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
// exp.JSONExtract, not exp.Lambda (parser.go:1341/parser_lambda.go). The base case stays on the
// JSON_EXTRACT(...) function form (base never sets only_json_types); the postgres case now
// round-trips through the operator/function split driven by only_json_types (generator/
// json_arrow.go) - verified against the pinned oracle: `x::JSON -> 'd' ->> -1` parses as
// JSONExtractScalar(JSONExtract(CAST(x AS JSON), 'd', only_json_types=True), Neg(1)); the inner
// literal RHS ('d') sets only_json_types so it renders as the `->` operator, but the outer RHS
// (-1, a Neg wrapping a Literal, not itself a Literal) does not, so the outer renders as the
// JSON_EXTRACT_PATH_TEXT function form wrapping the inner operator expression.
func TestLambdaSQLNotAJSONArrow(t *testing.T) {
	if got := roundTrip(t, "", "SELECT a -> b"); got != "SELECT JSON_EXTRACT(a, b)" {
		t.Errorf("SELECT a -> b -> %q, want JSON_EXTRACT(a, b) (JSONExtract, not Lambda)", got)
	}
	if got := roundTrip(t, "postgres", "SELECT x::JSON -> 'd' ->> -1"); got != "SELECT JSON_EXTRACT_PATH_TEXT(CAST(x AS JSON) -> 'd', -1)" {
		t.Errorf("postgres JSON arrow chain -> %q", got)
	}
}

// TestJSONExtractFunctionFormVarargs guards the function form of JSON_EXTRACT/JSON_EXTRACT_SCALAR
// (parsed via jsonExtractFunction, expressions/functions.go). Upstream build_extract_json_with_path
// (parser.py:104-118) preserves the 3rd+ path arguments only for JSONExtract, so JSON_EXTRACT(a,
// b, c) keeps `c` (base/mysql under the JSON_EXTRACT name, postgres renamed to JSON_EXTRACT_PATH),
// while JSON_EXTRACT_SCALAR(a, b, c) drops it: base -> JSON_EXTRACT_SCALAR(a, b), postgres ->
// JSON_EXTRACT_PATH_TEXT(a, b), mysql -> the `a ->> b` arrow form. Verified against the pinned
// oracle (bare-identifier args, so no JSONPath rewrite is involved).
func TestJSONExtractFunctionFormVarargs(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"", "SELECT JSON_EXTRACT(a, b, c)", "SELECT JSON_EXTRACT(a, b, c)"},
		{"mysql", "SELECT JSON_EXTRACT(a, b, c)", "SELECT JSON_EXTRACT(a, b, c)"},
		{"postgres", "SELECT JSON_EXTRACT(a, b, c)", "SELECT JSON_EXTRACT_PATH(a, b, c)"},
		{"", "SELECT JSON_EXTRACT_SCALAR(a, b, c)", "SELECT JSON_EXTRACT_SCALAR(a, b)"},
		{"mysql", "SELECT JSON_EXTRACT_SCALAR(a, b, c)", "SELECT a ->> b"},
		{"postgres", "SELECT JSON_EXTRACT_SCALAR(a, b, c)", "SELECT JSON_EXTRACT_PATH_TEXT(a, b)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}
