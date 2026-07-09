package generator_test

// Round-trip checks for the dialect-aware branches of dataTypeSQL and castSQLWithPrefix
// (generator/sql.go): mysql's full-replacement TYPE_MAPPING (generators/mysql.py:255-273)
// and narrow CAST_MAPPING/TIMESTAMP_FUNC_TYPES (generators/mysql.py:315-336), and postgres's
// TYPE_MAPPING delta (generators/postgres.py:271-284). Cases are drawn from
// testdata/dialect_identity.jsonl and testdata/parity_gaps.txt (mysql/postgres cast/type
// entries), confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.transpile('CAST(x AS BIGINT)', read='mysql', write='mysql')[0])"
//	CAST(x AS SIGNED)
//	>>> sqlglot.transpile("CAST(x AS TIMESTAMP)", read="mysql", write="mysql")[0]
//	'TIMESTAMP(x)'
//	>>> sqlglot.transpile("cast(a as FLOAT)", read="postgres", write="postgres")[0]
//	'CAST(a AS DOUBLE PRECISION)'

import "testing"

func TestDataTypeSQLMySQLTypeMapping(t *testing.T) {
	// mysql's TYPE_MAPPING is a full replacement of the base table: unlike base, it does NOT
	// fold LONGTEXT/MEDIUMTEXT/TINYTEXT/*BLOB down to TEXT/BLOB/VARBINARY - those keep their
	// own MySQL-native names when they appear as a plain (non-CAST) column type.
	cases := []struct{ sql, want string }{
		{"CREATE TABLE t (a LONGTEXT)", "CREATE TABLE t (a LONGTEXT)"},
		{"CREATE TABLE t (a MEDIUMBLOB)", "CREATE TABLE t (a MEDIUMBLOB)"},
		{"CREATE TABLE test (ts TIMESTAMP, ts_tz TIMESTAMPTZ, ts_ltz TIMESTAMPLTZ)",
			"CREATE TABLE test (ts TIMESTAMP, ts_tz TIMESTAMP, ts_ltz TIMESTAMP)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "mysql", tc.sql); got != tc.want {
			t.Errorf("mysql %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

func TestCastSQLMySQLCastMapping(t *testing.T) {
	cases := []struct{ sql, want string }{
		// CHAR_CAST_MAPPING: text/blob-ish targets collapse to CHAR.
		{"CAST(x AS LONGBLOB)", "CAST(x AS CHAR)"},
		{"CAST(x AS LONGTEXT)", "CAST(x AS CHAR)"},
		{"CAST(x AS MEDIUMBLOB)", "CAST(x AS CHAR)"},
		{"CAST(x AS MEDIUMTEXT)", "CAST(x AS CHAR)"},
		{"CAST(x AS TEXT)", "CAST(x AS CHAR)"},
		{"CAST(x AS TINYBLOB)", "CAST(x AS CHAR)"},
		{"CAST(x AS TINYTEXT)", "CAST(x AS CHAR)"},
		{"CAST(x AS VARCHAR)", "CAST(x AS CHAR)"},
		// SIGNED_CAST_MAPPING: signed-integer-ish targets (plus BOOLEAN) collapse to SIGNED.
		{"CAST(x AS BIGINT)", "CAST(x AS SIGNED)"},
		{"CAST(x AS BOOLEAN)", "CAST(x AS SIGNED)"},
		{"CAST(x AS INT)", "CAST(x AS SIGNED)"},
		{"CAST(x AS MEDIUMINT)", "CAST(x AS SIGNED)"},
		{"CAST(x AS SMALLINT)", "CAST(x AS SIGNED)"},
		{"CAST(x AS TINYINT)", "CAST(x AS SIGNED)"},
		// A type outside CAST_MAPPING (e.g. one with its own params) is untouched.
		{"CAST(x AS MEDIUMINT) + CAST(y AS YEAR(4))", "CAST(x AS SIGNED) + CAST(y AS YEAR(4))"},
		// TIMESTAMP_FUNC_TYPES: renders as a TIMESTAMP(...) function call, not a CAST at all.
		{"CAST(x AS TIMESTAMP)", "TIMESTAMP(x)"},
		{"CAST(x AS TIMESTAMPTZ)", "TIMESTAMP(x)"},
		{"CAST(x AS TIMESTAMPLTZ)", "TIMESTAMP(x)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "mysql", tc.sql); got != tc.want {
			t.Errorf("mysql %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

func TestDataTypeSQLPostgresTypeMapping(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"cast(a as FLOAT)", "CAST(a AS DOUBLE PRECISION)"},
		{"cast(a as FLOAT4)", "CAST(a AS REAL)"},
		{"cast(a as FLOAT8)", "CAST(a AS DOUBLE PRECISION)"},
		{"ROUND(CAST(x AS DOUBLE PRECISION))", "ROUND(CAST(x AS DOUBLE PRECISION))"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestParameterSQLPostgresToken guards postgres's PARAMETER_TOKEN="$" override
// (generators/postgres.py:240), found while investigating why CAST($1 AS TEXT) failed to
// round-trip under postgres: the parameterSQL bug (base's "@" sigil leaking into postgres)
// was unrelated to TYPE_MAPPING, but sits in the same castSQLWithPrefix-adjacent code path.
func TestParameterSQLPostgresToken(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT $1", "SELECT $1"},
		{"SELECT x FROM t WHERE CAST($1 AS TEXT) = 'ok'", "SELECT x FROM t WHERE CAST($1 AS TEXT) = 'ok'"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestTryCastSQL guards tryCastSQL's dialect branch (generator/sql.go): mysql and postgres
// have no TRY_CAST, so upstream routes exp.TryCast through no_trycast_sql -> a plain CAST
// (generators/mysql.py:223, generators/postgres.py:377), which means the mysql cast-target
// renames (TEXT -> CHAR, BIGINT -> SIGNED) still apply with no stray TRY_ prefix; every other
// dialect keeps TRY_CAST. Wants confirmed against the pinned oracle. Without this the branch
// was untestable from the corpus (no TRY_CAST records) and could silently revert.
func TestTryCastSQL(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"mysql", "TRY_CAST(x AS TEXT)", "CAST(x AS CHAR)"},
		{"mysql", "TRY_CAST(x AS BIGINT)", "CAST(x AS SIGNED)"},
		{"postgres", "TRY_CAST(x AS INT)", "CAST(x AS INT)"},
		{"postgres", "TRY_CAST(x AS TEXT)", "CAST(x AS TEXT)"},
		// Base and other dialects keep TRY_CAST verbatim.
		{"", "TRY_CAST(x AS INT)", "TRY_CAST(x AS INT)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}
