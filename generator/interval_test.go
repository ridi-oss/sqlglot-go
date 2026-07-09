package generator_test

// Round-trip checks for the dialect-aware branches of intervalSQL (generator/sql.go),
// which port the two missing branches of interval_sql (generator.py:3910-3930):
// SINGLE_STRING_INTERVAL (postgres, generators/postgres.py:233) and
// INTERVAL_ALLOWS_PLURAL_FORM (mysql, generators/mysql.py:132). Cases are drawn from
// testdata/dialect_identity.jsonl and testdata/parity_gaps.txt (postgres INTERVAL
// entries), confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.transpile(\"SELECT INTERVAL '-1 MONTH'\", read='postgres', write='postgres')[0])"
//	SELECT INTERVAL '-1 MONTH'
//	>>> sqlglot.transpile("SELECT date_col - INTERVAL '1' HOUR AS one_hour_later", read="postgres", write="postgres")[0]
//	"SELECT date_col - INTERVAL '1 HOUR' AS one_hour_later"
//	>>> sqlglot.transpile("SELECT INTERVAL '5' DAYS", read="mysql", write="mysql")[0]
//	"SELECT INTERVAL '5' DAY"

import "testing"

func TestIntervalSQLPostgresSingleString(t *testing.T) {
	cases := []struct{ sql, want string }{
		// A bare INTERVAL literal already carries its unit inside the single quoted string, so
		// this=the whole "-1 MONTH" text and unit_expression is unset - unchanged round trip.
		{"SELECT INTERVAL '-1 MONTH'", "SELECT INTERVAL '-1 MONTH'"},
		{"SELECT INTERVAL '-10.75 MINUTE'", "SELECT INTERVAL '-10.75 MINUTE'"},
		{"SELECT INTERVAL '0.123456789 SECOND'", "SELECT INTERVAL '0.123456789 SECOND'"},
		{"SELECT INTERVAL '2.5 MONTH'", "SELECT INTERVAL '2.5 MONTH'"},
		{"SELECT INTERVAL '3.14159 HOUR'", "SELECT INTERVAL '3.14159 HOUR'"},
		{"SELECT INTERVAL '4.1 DAY'", "SELECT INTERVAL '4.1 DAY'"},
		// This=magnitude and unit are separate INTERVAL '<n>' <UNIT> tokens on input; postgres
		// SINGLE_STRING_INTERVAL folds them into one quoted string on output.
		{"SELECT date_col - INTERVAL '1' HOUR AS one_hour_later", "SELECT date_col - INTERVAL '1 HOUR' AS one_hour_later"},
		{"SELECT date_col - INTERVAL '30' DAY FROM t", "SELECT date_col - INTERVAL '30 DAY' FROM t"},
		// No unit at all: this alone, still single-quoted.
		{"SELECT date_col - INTERVAL '30' FROM t", "SELECT date_col - INTERVAL '30' FROM t"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

func TestIntervalSQLMySQLSingularUnit(t *testing.T) {
	// mysql INTERVAL_ALLOWS_PLURAL_FORM=false: a plural unit (DAYS/HOURS/...) singularizes via
	// timePartSingulars, independent of SINGLE_STRING_INTERVAL (mysql doesn't set that flag, so
	// this and unit stay as separate tokens, matching base's INTERVAL <this> <unit> shape).
	cases := []struct{ sql, want string }{
		{"SELECT INTERVAL '5' DAYS", "SELECT INTERVAL '5' DAY"},
		{"SELECT x + INTERVAL '2' HOURS", "SELECT x + INTERVAL '2' HOUR"},
		// A unit that's already singular (or has no plural mapping) passes through unchanged.
		{"SELECT INTERVAL '1' YEAR", "SELECT INTERVAL '1' YEAR"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "mysql", tc.sql); got != tc.want {
			t.Errorf("mysql %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}
