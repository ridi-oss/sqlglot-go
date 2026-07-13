package generator_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
)

func parseOne(t *testing.T, sql string) string {
	t.Helper()
	return roundTrip(t, "", sql)
}

// roundTrip parses sql under the given dialect ("" = base) and regenerates it in that same
// dialect, failing the test on any parse/generate error. Shared by every dialect-aware
// generator test (cast/interval/trim/substring/rename/lambda/aggregate); parseOne is the
// base-dialect shorthand.
func roundTrip(t *testing.T, dialect, sql string) string {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q) error: %v", sql, dialect, err)
	}
	generated, err := sqlglot.Generate(expression, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate(%q, %q) error: %v", sql, dialect, err)
	}
	return generated
}

func TestIdentify(t *testing.T) {
	cases := []struct {
		sql      string
		identify any
		want     string
	}{
		{"x", nil, "x"},
		{"x", true, "\"x\""},
		{"\"x\"", false, "\"x\""},
		{"X", "safe", "X"},
		{"x AS 1", "safe", "\"x\" AS \"1\""},
	}
	for _, tc := range cases {
		expression, err := sqlglot.ParseOne(tc.sql, "")
		if err != nil {
			t.Fatalf("ParseOne(%q) error: %v", tc.sql, err)
		}
		got, err := sqlglot.Generate(expression, "", generator.Options{Identify: tc.identify})
		if err != nil {
			t.Fatalf("Generate(%q) error: %v", tc.sql, err)
		}
		if got != tc.want {
			t.Fatalf("Generate(%q, identify=%v) = %q, want %q", tc.sql, tc.identify, got, tc.want)
		}
	}
}

// TestUnnestWithOrdinality guards that folding a `WITH OFFSET AS <col>` into the alias
// column list still emits WITH ORDINALITY: upstream unnest_sql clears only the offset ARG,
// keeping the local offset truthy (generator.py:3444-3447 vs 3456-3457). A prior bug also
// nil'd the local, dropping the keyword and corrupting the column semantics.
func TestUnnestWithOrdinality(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT a FROM UNNEST(x) AS t WITH OFFSET AS y", "SELECT a FROM UNNEST(x) WITH ORDINALITY AS t(y)"},
		{"SELECT a FROM UNNEST(x) AS t(v) WITH OFFSET AS y", "SELECT a FROM UNNEST(x) WITH ORDINALITY AS t(v, y)"},
		{"SELECT a FROM UNNEST(x) WITH ORDINALITY", "SELECT a FROM UNNEST(x) WITH ORDINALITY"},
	}
	for _, tc := range cases {
		if got := parseOne(t, tc.sql); got != tc.want {
			t.Fatalf("%q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

func TestGenerateNestedBinary(t *testing.T) {
	sql := "'foo'" + strings.Repeat(" || 'foo'", 1000)
	if got := parseOne(t, sql); got != sql {
		t.Fatalf("nested binary round-trip mismatch: got len %d, want len %d", len(got), len(sql))
	}
}

// TestIdentifyNormalizeSafe guards that the safe-identifier check runs against the
// ORIGINAL (mixed-case) identifier text, not the normalized/lowercased output. With
// normalize=true a case-sensitive name lowercases but must stay unquoted under "safe".
func TestIdentifyNormalizeSafe(t *testing.T) {
	expression, err := sqlglot.ParseOne("SELECT Foo FROM Bar", "")
	if err != nil {
		t.Fatalf("ParseOne error: %v", err)
	}
	got, err := sqlglot.Generate(expression, "", generator.Options{Normalize: true, Identify: "safe"})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	want := "SELECT foo FROM bar"
	if got != want {
		t.Fatalf("Generate(normalize,safe) = %q, want %q", got, want)
	}
}

// TestWithinGroupPretty guards that the WITHIN GROUP order clause has its leading
// separator stripped in pretty mode (a newline, not a space).
func TestWithinGroupPretty(t *testing.T) {
	expression, err := sqlglot.ParseOne("SELECT PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY x)", "")
	if err != nil {
		t.Fatalf("ParseOne error: %v", err)
	}
	got, err := sqlglot.Generate(expression, "", generator.Options{Pretty: true})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if !strings.Contains(got, "WITHIN GROUP (ORDER BY") {
		t.Fatalf("pretty WITHIN GROUP order clause not stripped: %q", got)
	}
	if strings.Contains(got, "WITHIN GROUP (\n") {
		t.Fatalf("pretty WITHIN GROUP has an unstripped leading newline: %q", got)
	}
}

func TestEscapingRoundTrip(t *testing.T) {
	cases := []string{
		"''''",
		`'\x'`,
		`'\\z'`,
		`"""x"""`,
		"1E+2",
	}
	for _, sql := range cases {
		if got := parseOne(t, sql); got != sql {
			t.Fatalf("Generate(%q) = %q, want %q", sql, got, sql)
		}
	}
}
