package generator_test

// Round-trip checks for trimSQLStandard (generator/sql.go), which ports the mysql/postgres-
// shared standard-SQL TRIM form (dialects/dialect.py:1782-1797, wired at generators/mysql.py:
// 221 and generators/postgres.py:376). Cases are drawn from testdata/dialect_identity.jsonl
// and testdata/parity_gaps.txt (mysql/postgres TRIM entries), confirmed against the pinned
// oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.transpile(\"SELECT TRIM(BOTH 'bla' FROM ' XXX ')\", read='mysql', write='mysql')[0])"
//	SELECT TRIM(BOTH 'bla' FROM ' XXX ')
//	>>> sqlglot.transpile('SELECT TRIM(LEADING \' XXX \' COLLATE "de_DE")', read="postgres", write="postgres")[0]
//	'SELECT LTRIM(\' XXX \' COLLATE "de_DE")'

import "testing"

func TestTrimSQLStandardForm(t *testing.T) {
	// remove_chars present: TRIM(<position> <remove> FROM <target> COLLATE <collation>).
	cases := []struct{ dialect, sql, want string }{
		{"mysql", "SELECT TRIM('bla' FROM ' XXX ')", "SELECT TRIM('bla' FROM ' XXX ')"},
		{"mysql", "SELECT TRIM(BOTH 'bla' FROM ' XXX ')", "SELECT TRIM(BOTH 'bla' FROM ' XXX ')"},
		{"mysql", "SELECT TRIM(LEADING 'bla' FROM ' XXX ')", "SELECT TRIM(LEADING 'bla' FROM ' XXX ')"},
		{"mysql", "SELECT TRIM(TRAILING 'bla' FROM ' XXX ')", "SELECT TRIM(TRAILING 'bla' FROM ' XXX ')"},
		{"postgres", "SELECT TRIM(' X' FROM ' XXX ')", "SELECT TRIM(' X' FROM ' XXX ')"},
		{"postgres", "SELECT TRIM(LEADING 'bla' FROM ' XXX ' COLLATE utf8_bin)", "SELECT TRIM(LEADING 'bla' FROM ' XXX ' COLLATE utf8_bin)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

// TestTrimSQLStandardFallback guards the no-remove_chars fallback to the LTRIM/RTRIM form
// (trimSQLBase), including the COLLATE splice this port needs since the parser keeps
// "collation" as Trim's own arg rather than nesting a Collate binary node in "this" the way
// upstream's shared bitwise-expression parsing does (see trimSQLBase's doc comment).
func TestTrimSQLStandardFallback(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"postgres", `SELECT TRIM(LEADING ' XXX ' COLLATE "de_DE")`, `SELECT LTRIM(' XXX ' COLLATE "de_DE")`},
		{"postgres", `SELECT TRIM(TRAILING ' XXX ' COLLATE "de_DE")`, `SELECT RTRIM(' XXX ' COLLATE "de_DE")`},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

// TestTrimSQLBaseCollation guards the base (LTRIM/RTRIM/TRIM) form's COLLATE splice when a
// remove-chars operand IS present. Upstream folds the trailing COLLATE into a Collate node
// inside `this` (so its trim_sql renders it for free), but this port keeps collation as a
// separate Trim arg, so trimSQLBase must attach it to the target itself - not only in the
// no-remove-chars fallback path. Wants confirmed against the pinned oracle (base dialect):
//
//	SELECT TRIM(BOTH 'bla' FROM ' XXX ' COLLATE utf8_bin) -> SELECT TRIM(' XXX ' COLLATE utf8_bin, 'bla')
func TestTrimSQLBaseCollation(t *testing.T) {
	cases := []struct{ sql, want string }{
		{`SELECT TRIM(BOTH 'bla' FROM ' XXX ' COLLATE utf8_bin)`, `SELECT TRIM(' XXX ' COLLATE utf8_bin, 'bla')`},
		{`SELECT TRIM('bla' FROM ' XXX ' COLLATE utf8_bin)`, `SELECT TRIM(' XXX ' COLLATE utf8_bin, 'bla')`},
		{`SELECT TRIM(LEADING 'bla' FROM ' XXX ' COLLATE utf8_bin)`, `SELECT LTRIM(' XXX ' COLLATE utf8_bin, 'bla')`},
		{`SELECT TRIM(TRAILING 'bla' FROM ' XXX ' COLLATE "de_DE")`, `SELECT RTRIM(' XXX ' COLLATE "de_DE", 'bla')`},
		// No collation: unchanged two-arg form (regression guard for the splice condition).
		{`SELECT TRIM(BOTH 'bla' FROM ' XXX ')`, `SELECT TRIM(' XXX ', 'bla')`},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "", tc.sql); got != tc.want {
			t.Errorf("base %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}
