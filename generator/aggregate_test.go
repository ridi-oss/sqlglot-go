package generator_test

// Round-trip checks for groupConcatSQL/logicalOrSQL (generator/aggregate.go).
// groupConcatSQL ports groupconcat_sql (dialects/dialect.py:2423-2467) dialect-gated per
// generators/postgres.py:313-315. logicalOrSQL ports the LogicalOr renames from
// generators/postgres.py:332 and generators/mysql.py:180. Cases are drawn from
// testdata/parity_gaps.txt (postgres STRING_AGG/LOGICAL_OR entries) and testdata/
// identity.sql / dialect_identity.jsonl (base/mysql LISTAGG/GROUP_CONCAT entries).

import "testing"

func TestGroupConcatSQLPostgres(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"STRING_AGG(x, y)", "STRING_AGG(x, y)"},
		{"STRING_AGG(x, ',' ORDER BY y)", "STRING_AGG(x, ',' ORDER BY y)"},
		{"STRING_AGG(x, ',' ORDER BY y DESC)", "STRING_AGG(x, ',' ORDER BY y DESC)"},
		{"STRING_AGG(DISTINCT x, ',' ORDER BY y DESC)", "STRING_AGG(DISTINCT x, ',' ORDER BY y DESC)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestGroupConcatSQLFallback guards that registering the KindGroupConcat dispatch entry
// leaves base/mysql GROUP_CONCAT and base LISTAGG on their pre-dispatch-entry rendering
// (functionFallbackSQL and Anonymous respectively - LISTAGG isn't even a registered
// function in base FunctionByName, see expressions/functions.go).
func TestGroupConcatSQLFallback(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"", "SELECT GROUP_CONCAT(x, ',')", "SELECT GROUP_CONCAT(x, ',')"},
		{"mysql", "SELECT GROUP_CONCAT(x, ',')", "SELECT GROUP_CONCAT(x, ',')"},
		{"", "SELECT LISTAGG(x) WITHIN GROUP (ORDER BY x DESC)", "SELECT LISTAGG(x) WITHIN GROUP (ORDER BY x DESC)"},
		{"", "SELECT LISTAGG(x) WITHIN GROUP (ORDER BY x) AS y", "SELECT LISTAGG(x) WITHIN GROUP (ORDER BY x) AS y"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

func TestLogicalOrSQL(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"postgres", "SELECT a, LOGICAL_OR(b) FROM table GROUP BY a", "SELECT a, BOOL_OR(b) FROM table GROUP BY a"},
		{"postgres", "SELECT BOOL_OR(b)", "SELECT BOOL_OR(b)"},
		{"postgres", "SELECT BOOLOR_AGG(b)", "SELECT BOOL_OR(b)"},
		{"mysql", "SELECT LOGICAL_OR(b)", "SELECT MAX(b)"},
		{"", "SELECT LOGICAL_OR(b)", "SELECT LOGICAL_OR(b)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}
