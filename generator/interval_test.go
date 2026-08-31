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

// TestIntervalUnitCurrentDate: since v30.17.0's is_unit gate, CURRENT_DATE after an
// interval literal is an ALIAS, not a unit (parser.py:4054-4060) — matching pinned
// upstream's identity fixture (`INTERVAL '-1' AS CURRENT_DATE`).
func TestIntervalUnitCurrentDate(t *testing.T) {
	sql := "INTERVAL '-1' CURRENT_DATE"
	want := "INTERVAL '-1' AS CURRENT_DATE"
	if got := roundTrip(t, "", sql); got != want {
		t.Errorf("%q ->\n  got  %q\n  want %q", sql, got, want)
	}
}

// TestDateAddMySQLCompoundUnit covers mysqlDateAddSQL (generator/sql.go), which reconstructs
// `DATE_ADD(this, INTERVAL <value> <unit>)` from the DateAdd's expression/unit args
// (including MySQL's DAY_HOUR-style compound units) instead of the base dateadd_sql's
// generic `DATE_ADD(this, expression, 'unit')` shape. Every case here is an identity round
// trip drawn from testdata/dialect_identity.jsonl, confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one('DATE_ADD(base_date, INTERVAL day_interval DAY_HOUR)', read='mysql').sql(dialect='mysql'))"
//	DATE_ADD(base_date, INTERVAL day_interval DAY_HOUR)
func TestDateAddMySQLCompoundUnit(t *testing.T) {
	cases := []string{
		"DATE_ADD(base_date, INTERVAL day_interval DAY_HOUR)",
		"DATE_ADD(base_date, INTERVAL day_interval DAY_MICROSECOND)",
		"DATE_ADD(base_date, INTERVAL day_interval YEAR_MONTH)",
		"DATE_ADD(x, INTERVAL '1' YEAR)",
	}
	for _, sql := range cases {
		if got := roundTrip(t, "mysql", sql); got != sql {
			t.Errorf("mysql %q ->\n  got  %q\n  want %q", sql, got, sql)
		}
	}
}

// TestDateAddPostgresInterval covers postgresDateAddSQL (generator/sql.go): postgres
// renders exp.DateAdd as `this + INTERVAL '<value> <unit>'` rather than a DATE_ADD(...)
// call. This only closes the DateAdd half of testdata/parity_gaps.txt's postgres
// "date_add(current_date, interval '7' day)" case - the CURRENT_DATE argument itself still
// round-trips as a lowercase `current_date` column (this port has no CurrentDate Kind /
// NO_PAREN_FUNCTIONS support yet, parser.go:1953 TODO(1d)), so that entry is exercised here
// against a non-CURRENT_DATE `this` instead, confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one('SELECT date_add(x, interval \'7\' day)', read='postgres').sql(dialect='postgres'))"
//	SELECT x + INTERVAL '7 DAY'
func TestDateAddPostgresInterval(t *testing.T) {
	got := roundTrip(t, "postgres", "SELECT date_add(x, interval '7' day)")
	want := "SELECT x + INTERVAL '7 DAY'"
	if got != want {
		t.Errorf("postgres date_add ->\n  got  %q\n  want %q", got, want)
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
