package generator_test

// Round-trip checks for substringSQL (generator/substring.go), which ports
// _substring_sql (generators/postgres.py:106-114). Cases are drawn from
// testdata/dialect_identity.jsonl and testdata/parity_gaps.txt (postgres SUBSTRING
// entries), confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one(\"SELECT SUBSTRING('Thomas' FOR 3 FROM 2)\", read='postgres').sql(dialect='postgres'))"
//	SELECT SUBSTRING('Thomas' FROM 2 FOR 3)

import "testing"

func TestSubstringSQLPostgres(t *testing.T) {
	cases := []struct{ sql, want string }{
		// FROM-then-FOR key order is emitted regardless of source order.
		{"SELECT SUBSTRING('Thomas' FOR 3 FROM 2)", "SELECT SUBSTRING('Thomas' FROM 2 FOR 3)"},
		{"SELECT SUBSTRING('afafa' for 1)", "SELECT SUBSTRING('afafa' FROM 1 FOR 1)"},
		// Already-canonical FROM..FOR forms are unaffected.
		{"SELECT SUBSTRING('abcdefg' FROM 1 FOR 2)", "SELECT SUBSTRING('abcdefg' FROM 1 FOR 2)"},
		{"SELECT SUBSTRING('abcdefg' FROM 1)", "SELECT SUBSTRING('abcdefg' FROM 1)"},
		{"SELECT SUBSTRING('abcdefg')", "SELECT SUBSTRING('abcdefg')"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestSubstringSQLFallback guards that registering the KindSubstring dispatch entry
// leaves base and mysql on the pre-existing comma-form (functionFallbackSQL) - the same
// output produced before this dispatch entry existed, via the TraitFunc fallback path in
// genWithComment.
func TestSubstringSQLFallback(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"", "SELECT SUBSTR('abcdefg', 1, 2)", "SELECT SUBSTRING('abcdefg', 1, 2)"},
		{"mysql", "SELECT SUBSTR(1, 2, 3)", "SELECT SUBSTRING(1, 2, 3)"},
		{"", "SELECT SUBSTRING(1, 2, 3)", "SELECT SUBSTRING(1, 2, 3)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}
