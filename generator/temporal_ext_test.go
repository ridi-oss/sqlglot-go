package generator_test

// Round-trip checks for the STR_TO_*/TIME_STR_TO_* temporal family (generator/temporal_ext.go),
// which ports timestrtotime_sql (dialects/dialect.py:1729-1744) plus its mysql/postgres gating
// (generators/mysql.py:212-217, generators/postgres.py:371) and subsecond_precision
// (time.py:668-688). Cases are drawn from testdata/dialect_identity.jsonl:253-260,406 and
// verified against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.transpile(\"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.1+00:00')\", read='mysql', write='mysql')[0])"
//	SELECT CAST('2023-01-01 13:14:15.1+00:00' AS DATETIME(3))

import "testing"

func TestTimeStrToTimeSQLMySQL(t *testing.T) {
	cases := []struct{ sql, want string }{
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15+00:00')",
			"SELECT CAST('2023-01-01 13:14:15+00:00' AS DATETIME)",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15-08:00', 'America/Los_Angeles')",
			"SELECT TIMESTAMP('2023-01-01 13:14:15-08:00')",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.1+00:00')",
			"SELECT CAST('2023-01-01 13:14:15.1+00:00' AS DATETIME(3))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.12+00:00')",
			"SELECT CAST('2023-01-01 13:14:15.12+00:00' AS DATETIME(3))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.123+00:00')",
			"SELECT CAST('2023-01-01 13:14:15.123+00:00' AS DATETIME(3))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.1234+00:00')",
			"SELECT CAST('2023-01-01 13:14:15.1234+00:00' AS DATETIME(6))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.12345+00:00')",
			"SELECT CAST('2023-01-01 13:14:15.12345+00:00' AS DATETIME(6))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.123456+00:00')",
			"SELECT CAST('2023-01-01 13:14:15.123456+00:00' AS DATETIME(6))",
		},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "mysql", tc.sql); got != tc.want {
			t.Errorf("mysql %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestTimeStrToTimeSubsecondISOForms locks in the subsecondPrecision port (time.py:668-688)
// against ISO-8601 offset/separator forms datetime.fromisoformat accepts beyond the corpus's
// `+00:00` fixtures: colon-less offsets (`+0000`), comma fractional separators, and a bare `Z`.
// Verified against the pinned oracle (transpile read/write='mysql'). A regex that only matched
// the colon'd `+00:00` form silently dropped precision here.
func TestTimeStrToTimeSubsecondISOForms(t *testing.T) {
	cases := []struct{ sql, want string }{
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.123+0000')",
			"SELECT CAST('2023-01-01 13:14:15.123+0000' AS DATETIME(3))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15,123+00:00')",
			"SELECT CAST('2023-01-01 13:14:15,123+00:00' AS DATETIME(3))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15.1234+0000')",
			"SELECT CAST('2023-01-01 13:14:15.1234+0000' AS DATETIME(6))",
		},
		{
			"SELECT TIME_STR_TO_TIME('2023-01-01 13:14:15Z')",
			"SELECT CAST('2023-01-01 13:14:15Z' AS DATETIME)",
		},
		// Compact/basic ISO forms datetime.fromisoformat accepts (a shape regex missed these):
		// a fully compact date+time and a compact date with an extended time.
		{
			"SELECT TIME_STR_TO_TIME('20230101T121314.123')",
			"SELECT CAST('20230101T121314.123' AS DATETIME(3))",
		},
		{
			"SELECT TIME_STR_TO_TIME('20230101T12:13:14.1234')",
			"SELECT CAST('20230101T12:13:14.1234' AS DATETIME(6))",
		},
		// Non-calendar dates are rejected by fromisoformat -> precision 0 (no DATETIME(n)); a
		// shape regex wrongly kept the fractional precision here.
		{
			"SELECT TIME_STR_TO_TIME('2023-99-99 13:14:15.123')",
			"SELECT CAST('2023-99-99 13:14:15.123' AS DATETIME)",
		},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "mysql", tc.sql); got != tc.want {
			t.Errorf("mysql %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

func TestTimeStrToUnixSQLMySQL(t *testing.T) {
	if got := roundTrip(t, "mysql", "TIME_STR_TO_UNIX(x)"); got != "UNIX_TIMESTAMP(x)" {
		t.Errorf("mysql TIME_STR_TO_UNIX(x) -> got %q, want UNIX_TIMESTAMP(x)", got)
	}
}

// TestTimeStrToTimeFallback guards the base dialect: no dispatch override exists, so
// TimeStrToTime/TimeStrToUnix keep their default names via functionFallbackSQL - not
// corpus-tested (dialect_identity.jsonl has no base-dialect TIME_STR_TO_TIME/UNIX fixtures),
// so this only pins the fallback rendering this port derives generically from ClassName.
func TestTimeStrToTimeFallback(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"", "TIME_STR_TO_TIME(x)", "TIME_STR_TO_TIME(x)"},
		{"", "TIME_STR_TO_UNIX(x)", "TIME_STR_TO_UNIX(x)"},
		{"postgres", "TIME_STR_TO_TIME(x)", "CAST(x AS TIMESTAMP)"},
		{"postgres", "TIME_STR_TO_UNIX(x)", "TIME_STR_TO_UNIX(x)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

// TestStrToFamilyFallback pins the STR_TO_DATE/STR_TO_TIME/STR_TO_UNIX/TIME_STR_TO_DATE
// registry entries (expressions/functions.go): all four parse via the generic FunctionByName
// path (FromArgList) and, absent a dispatch override in this port's scope, render back through
// functionFallbackSQL under their default (camelToSnake'd) names.
func TestStrToFamilyFallback(t *testing.T) {
	cases := []string{
		"STR_TO_DATE(x, y)",
		"STR_TO_TIME(x, y)",
		"STR_TO_UNIX(x, y)",
		"TIME_STR_TO_DATE(x)",
	}
	for _, sql := range cases {
		if got := roundTrip(t, "", sql); got != sql {
			t.Errorf("%q -> got %q, want %q", sql, got, sql)
		}
	}
}
