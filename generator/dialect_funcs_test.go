package generator_test

// Round-trip checks for generator/dialect_funcs.go (KindDayOfMonth/DayOfWeek/DayOfYear/
// WeekOfYear/Trunc/TimeStrToUnix/CurrentSchema/StrPosition mysql renames, plus the
// dialect-agnostic KindOverlay/KindVariadic renderers). Cases are drawn from
// testdata/dialect_identity.jsonl and parity_gaps.txt.

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
)

// TestMySQLRenameFuncFallback guards that base and postgres keep each Kind's default
// (upstream _sql_names[0] / auto-derived class name) spelling via functionFallbackSQL, and
// that mysql renames to the upstream generators/mysql.py spelling.
func TestMySQLRenameFuncFallback(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"mysql", "SELECT DAY_OF_MONTH('2023-01-01')", "SELECT DAYOFMONTH('2023-01-01')"},
		{"", "SELECT DAY_OF_MONTH('2023-01-01')", "SELECT DAY_OF_MONTH('2023-01-01')"},
		{"mysql", "SELECT DAY_OF_WEEK('2023-01-01')", "SELECT DAYOFWEEK('2023-01-01')"},
		{"mysql", "SELECT DAY_OF_YEAR('2023-01-01')", "SELECT DAYOFYEAR('2023-01-01')"},
		{"mysql", "SELECT WEEK_OF_YEAR('2023-01-01')", "SELECT WEEKOFYEAR('2023-01-01')"},
		{"mysql", "TRUNC(3.14159, 2)", "TRUNCATE(3.14159, 2)"},
		{"", "TRUNC(3.14159, 2)", "TRUNC(3.14159, 2)"},
		{"mysql", "TIME_STR_TO_UNIX(x)", "UNIX_TIMESTAMP(x)"},
		{"", "TIME_STR_TO_UNIX(x)", "TIME_STR_TO_UNIX(x)"},
		{"mysql", "DATABASE()", "SCHEMA()"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

// TestStrPositionSQL guards strPositionSQL's mysql LOCATE(substr, this[, position]) rename
// (dialect.py:1281-1321's strposition_sql, func_name="LOCATE", supports_position=True) versus
// the STR_POSITION(...) fallback everywhere else, including the pre-existing
// testdata/identity.sql STR_POSITION cases this dispatch entry must not regress.
func TestStrPositionSQL(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"mysql", "SELECT INSTR('str', 'substr')", "SELECT LOCATE('substr', 'str')"},
		{"", "STR_POSITION(haystack, needle)", "STR_POSITION(haystack, needle)"},
		{"", "STR_POSITION(haystack, needle, pos)", "STR_POSITION(haystack, needle, pos)"},
		// POSITION(needle, haystack, pos) (the non-"IN" comma form of parsePosition) sets the
		// "position" arg, exercising strPositionSQL's supports_position=True third argument.
		{"mysql", "SELECT POSITION(needle, haystack, pos)", "SELECT LOCATE(needle, haystack, pos)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

// TestOverlaySQL guards overlaySQL's OVERLAY(this PLACING expr FROM from[ FOR for]) rendering
// (generator.py:5746-5753), which is not dialect-gated upstream.
func TestOverlaySQL(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT OVERLAY(a PLACING b FROM 1 FOR 1)", "SELECT OVERLAY(a PLACING b FROM 1 FOR 1)"},
		{"SELECT OVERLAY(a PLACING b FROM 1)", "SELECT OVERLAY(a PLACING b FROM 1)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestVariadicSQL guards variadicSQL's "VARIADIC <this>" rendering (generator.py:286), only
// reachable in this port via postgres's noParenFunctionParsers["VARIADIC"] entry.
func TestVariadicSQL(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT MLEAST(VARIADIC ARRAY[10, -1, 5, 4.4])", "SELECT MLEAST(VARIADIC ARRAY[10, -1, 5, 4.4])"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestSubstrFromForMySQL guards the SUBSTR(x FROM y FOR z) MySQL-only alias (parsers/
// mysql.py:162) reusing parseSubstring, plain-comma SUBSTR still working the same as before
// (via the FunctionByName["SUBSTR"] path when the FUNCTION_PARSERS entry isn't registered for
// a dialect), and base/postgres continuing to reject the FROM/FOR form (parity_gaps.txt gap
// 127 is mysql-only - verified against the pinned reference, see parser/parser.go's SUBSTR
// gate comment).
func TestSubstrFromForMySQL(t *testing.T) {
	if got := roundTrip(t, "mysql", "SELECT SUBSTR(1 FROM 2 FOR 3)"); got != "SELECT SUBSTRING(1, 2, 3)" {
		t.Errorf("mysql SUBSTR FROM/FOR -> %q, want %q", got, "SELECT SUBSTRING(1, 2, 3)")
	}
	if got := roundTrip(t, "mysql", "SELECT SUBSTR(x, 2, 3)"); got != "SELECT SUBSTRING(x, 2, 3)" {
		t.Errorf("mysql SUBSTR comma-form -> %q, want %q", got, "SELECT SUBSTRING(x, 2, 3)")
	}
	// SUBSTR renders as SUBSTRING regardless of dialect/input spelling (Substring._sql_names[0]
	// = "SUBSTRING", string.py:222) - unaffected by the mysql-only FUNCTION_PARSERS gate.
	if got := roundTrip(t, "", "SELECT SUBSTR(x, 2, 3)"); got != "SELECT SUBSTRING(x, 2, 3)" {
		t.Errorf("base SUBSTR comma-form -> %q, want %q", got, "SELECT SUBSTRING(x, 2, 3)")
	}

	if expression, err := sqlglot.ParseOne("SELECT SUBSTR(1 FROM 2 FOR 3)", ""); err == nil {
		t.Errorf("base SUBSTR FROM/FOR unexpectedly parsed: %v", expression)
	}
}
