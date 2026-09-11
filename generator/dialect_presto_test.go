package generator_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Pinned v30.17.0 tests/dialects/test_presto.py and generators/presto.py.
func TestPrestoGeneratorParity(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"presto", "DATE_ADD('day', 1, y)", "DATE_ADD('DAY', 1, y)"},
		{"presto", "DATE_DIFF('day', a, b)", "DATE_DIFF('DAY', a, b)"},
		{"presto", "APPROX_PERCENTILE(a, b, c, d)", "APPROX_PERCENTILE(a, b, c, d)"},
		{"presto", "DATE_ADD('DAY', 1, y)", "DATE_ADD('DAY', 1, y)"},
		{"presto", "DATE_ADD('DAY', FLOOR(5), y)", "DATE_ADD('DAY', FLOOR(5), y)"},
		{"presto", "FROM_UNIXTIME(a, b)", "FROM_UNIXTIME(a, b)"},
		{"presto", "FROM_UNIXTIME(a, b, c)", "FROM_UNIXTIME(a, b, c)"},
		{"presto", "FROM_UTF8(x, y)", "FROM_UTF8(x, y)"},
		{"presto", "SELECT * FROM x OFFSET 1 LIMIT 1", "SELECT * FROM x OFFSET 1 LIMIT 1"},
		{"presto", "SELECT BOOL_OR(a > 10) FROM asd AS T(a)", "SELECT BOOL_OR(a > 10) FROM asd AS T(a)"},
		{"presto", "SELECT SPLIT_TO_MAP('a:1;b:2;a:3', ';', ':', (k, v1, v2) -> CONCAT(v1, v2))", "SELECT SPLIT_TO_MAP('a:1;b:2;a:3', ';', ':', (k, v1, v2) -> CONCAT(v1, v2))"},
		{"presto", "START TRANSACTION ISOLATION LEVEL REPEATABLE READ", "START TRANSACTION ISOLATION LEVEL REPEATABLE READ"},
		{"presto", "START TRANSACTION READ WRITE, ISOLATION LEVEL SERIALIZABLE", "START TRANSACTION READ WRITE, ISOLATION LEVEL SERIALIZABLE"},
		{"presto", "TRUNCATE(3.14159)", "TRUNCATE(3.14159)"},
		{"presto", "TRUNCATE(3.14159, 2)", "TRUNCATE(3.14159, 2)"},
		{"presto", "VAR_POP(a)", "VAR_POP(a)"},
		{"presto", "string_agg(x, ',')", "ARRAY_JOIN(ARRAY_AGG(x), ',')"},
		{"presto", "CAST(x AS HYPERLOGLOG)", "CAST(x AS HYPERLOGLOG)"},
		{"presto", "SELECT DATE_ADD('DAY', MOD(5, 2.5), y), DATE_ADD('DAY', CEIL(5.5), y)", "SELECT DATE_ADD('DAY', CAST(5 % 2.5 AS BIGINT), y), DATE_ADD('DAY', CAST(CEIL(5.5) AS BIGINT), y)"},
		{"presto", "TIMESTAMP '2025-06-20 11:22:29 Europe/Prague'", "CAST('2025-06-20 11:22:29 Europe/Prague' AS TIMESTAMP WITH TIME ZONE)"},
		{"presto", "CREATE OR REPLACE VIEW v SECURITY DEFINER AS SELECT id FROM t", "CREATE OR REPLACE VIEW v SECURITY DEFINER AS SELECT id FROM t"},
		{"presto", "CREATE OR REPLACE VIEW v SECURITY INVOKER AS SELECT id FROM t", "CREATE OR REPLACE VIEW v SECURITY INVOKER AS SELECT id FROM t"},
		{"presto", "SELECT ROW(1, 'x')", "SELECT ROW(1, 'x')"},
		{"presto", "SELECT ELEMENT_AT(xs, 1)", "SELECT ELEMENT_AT(xs, 1)"},
		{"presto", "SELECT CARDINALITY(xs)", "SELECT CARDINALITY(xs)"},
		{"presto", "SELECT SEQUENCE(1, 10)", "SELECT SEQUENCE(1, 10)"},
		{"presto", "SELECT APPROX_PERCENTILE(x, 0.5)", "SELECT APPROX_PERCENTILE(x, 0.5)"},
		{"presto", "SELECT AT_TIMEZONE(ts, 'UTC')", "SELECT AT_TIMEZONE(ts, 'UTC')"},
		{"presto", "SELECT ARBITRARY(x), DAY_OF_WEEK(x), STRPOS(x, 'a', 2)", "SELECT ARBITRARY(x), DAY_OF_WEEK(x), STRPOS(x, 'a', 2)"},
		{"presto", "SELECT CONTAINS(xs, 1), SLICE(xs, 1, 2), SET_AGG(x), LEVENSHTEIN_DISTANCE(x, y)", "SELECT CONTAINS(xs, 1), SLICE(xs, 1, 2), SET_AGG(x), LEVENSHTEIN_DISTANCE(x, y)"},
		{"presto", "SELECT FROM_HEX(x), TO_UNIXTIME(x), TO_UTF8(x), SHA256(x), SHA512(x), MD5(x)", "SELECT FROM_HEX(x), TO_UNIXTIME(x), TO_UTF8(x), SHA256(x), SHA512(x), MD5(x)"},
		{"presto", "SELECT BITWISE_AND(x, y), BITWISE_OR(x, y), BITWISE_XOR(x, y), BITWISE_NOT(x)", "SELECT BITWISE_AND(x, y), BITWISE_OR(x, y), BITWISE_XOR(x, y), BITWISE_NOT(x)"},
		{"presto", "SELECT CURRENT_TIMESTAMP, CURRENT_TIME, CURRENT_USER", "SELECT CURRENT_TIMESTAMP, CURRENT_TIME, CURRENT_USER"},
		{"presto", "SELECT IF(x, y, z)", "SELECT IF(x, y, z)"},
		{"presto", "SELECT x ILIKE y", "SELECT LOWER(x) LIKE LOWER(y)"},
		{"presto", "SELECT INITCAP(x)", "SELECT REGEXP_REPLACE(x, '(\\w)(\\w*)', x -> UPPER(x[1]) || LOWER(x[2]))"},
		{"presto", "SELECT SUBSTRING(x, 1, 2)", "SELECT SUBSTR(x, 1, 2)"},
		{"presto", "SELECT JSON_FORMAT(JSON_PARSE(x)), JSON_EXTRACT(x, '$.a')", "SELECT JSON_FORMAT(JSON_PARSE(x)), JSON_EXTRACT(x, '$.a')"},
		{"presto", "SELECT DATE_DIFF('DAY', a, b)", "SELECT DATE_DIFF('DAY', a, b)"},
		{"presto", "SELECT * FROM t TABLESAMPLE BERNOULLI (10)", "SELECT * FROM t TABLESAMPLE BERNOULLI (10)"},
		{"presto", "SELECT CAST(x AS ROW(a INTEGER, b VARCHAR)), CAST(x AS ARRAY(INTEGER)), CAST(x AS MAP(VARCHAR, INTEGER))", "SELECT CAST(x AS ROW(a INTEGER, b VARCHAR)), CAST(x AS ARRAY(INTEGER)), CAST(x AS MAP(VARCHAR, INTEGER))"},
		{"presto", "SELECT INTERVAL '2' WEEKS", "SELECT (2 * INTERVAL '7' DAY)"},
		{"presto", "SELECT DATE_ADD('DAY', MOD(5, 2), y)", "SELECT DATE_ADD('DAY', 5 % 2, y)"},
		{"trino", "SELECT ELEMENT_AT(xs, 1)", "SELECT ELEMENT_AT(xs, 1)"},
		{"trino", "SELECT CARDINALITY(xs)", "SELECT CARDINALITY(xs)"},
		{"trino", "SELECT SEQUENCE(1, 10)", "SELECT SEQUENCE(1, 10)"},
		{"trino", "SELECT APPROX_PERCENTILE(x, 0.5)", "SELECT APPROX_PERCENTILE(x, 0.5)"},
		{"trino", "SELECT AT_TIMEZONE(ts, 'UTC')", "SELECT AT_TIMEZONE(ts, 'UTC')"},
		{"trino", "SELECT ARBITRARY(x), DAY_OF_WEEK(x), STRPOS(x, 'a', 2)", "SELECT ARBITRARY(x), DAY_OF_WEEK(x), STRPOS(x, 'a', 2)"},
		{"trino", "SELECT CONTAINS(xs, 1), SLICE(xs, 1, 2), SET_AGG(x), LEVENSHTEIN_DISTANCE(x, y)", "SELECT CONTAINS(xs, 1), SLICE(xs, 1, 2), ARRAY_AGG(DISTINCT x), LEVENSHTEIN_DISTANCE(x, y)"},
		{"trino", "SELECT FROM_HEX(x), TO_UNIXTIME(x), TO_UTF8(x), SHA256(x), SHA512(x), MD5(x)", "SELECT FROM_HEX(x), TO_UNIXTIME(x), TO_UTF8(x), SHA256(x), SHA512(x), MD5(x)"},
		{"trino", "SELECT BITWISE_AND(x, y), BITWISE_OR(x, y), BITWISE_XOR(x, y), BITWISE_NOT(x)", "SELECT BITWISE_AND(x, y), BITWISE_OR(x, y), BITWISE_XOR(x, y), BITWISE_NOT(x)"},
		{"trino", "SELECT CURRENT_TIMESTAMP, CURRENT_TIME, CURRENT_USER", "SELECT CURRENT_TIMESTAMP, CURRENT_TIME, CURRENT_USER"},
		{"trino", "SELECT IF(x, y, z)", "SELECT IF(x, y, z)"},
		{"trino", "SELECT x ILIKE y", "SELECT LOWER(x) LIKE LOWER(y)"},
		{"trino", "SELECT INITCAP(x)", "SELECT REGEXP_REPLACE(x, '(\\w)(\\w*)', x -> UPPER(x[1]) || LOWER(x[2]))"},
		{"trino", "SELECT SUBSTRING(x, 1, 2)", "SELECT SUBSTR(x, 1, 2)"},
		{"trino", "SELECT JSON_FORMAT(JSON_PARSE(x)), JSON_EXTRACT(x, '$.a')", "SELECT JSON_FORMAT(JSON_PARSE(x)), JSON_EXTRACT(x, '$.a')"},
		{"trino", "SELECT DATE_DIFF('DAY', a, b)", "SELECT DATE_DIFF('DAY', a, b)"},
		{"trino", "SELECT * FROM t TABLESAMPLE BERNOULLI (10)", "SELECT * FROM t TABLESAMPLE BERNOULLI (10)"},
		{"trino", "SELECT CAST(x AS ROW(a INTEGER, b VARCHAR)), CAST(x AS ARRAY(INTEGER)), CAST(x AS MAP(VARCHAR, INTEGER))", "SELECT CAST(x AS ROW(a INTEGER, b VARCHAR)), CAST(x AS ARRAY(INTEGER)), CAST(x AS MAP(VARCHAR, INTEGER))"},
		{"trino", "SELECT INTERVAL '2' WEEKS", "SELECT (2 * INTERVAL '7' DAY)"},
		{"trino", "SELECT DATE_ADD('DAY', MOD(5, 2), y)", "SELECT DATE_ADD('DAY', 5 % 2, y)"},
		{"athena", "SELECT ELEMENT_AT(xs, 1)", "SELECT ELEMENT_AT(xs, 1)"},
		{"athena", "SELECT CARDINALITY(xs)", "SELECT CARDINALITY(xs)"},
		{"athena", "SELECT SEQUENCE(1, 10)", "SELECT SEQUENCE(1, 10)"},
		{"athena", "SELECT APPROX_PERCENTILE(x, 0.5)", "SELECT APPROX_PERCENTILE(x, 0.5)"},
		{"athena", "SELECT AT_TIMEZONE(ts, 'UTC')", "SELECT AT_TIMEZONE(ts, 'UTC')"},
		{"athena", "SELECT ARBITRARY(x), DAY_OF_WEEK(x), STRPOS(x, 'a', 2)", "SELECT ARBITRARY(x), DAY_OF_WEEK(x), STRPOS(x, 'a', 2)"},
		{"athena", "SELECT CONTAINS(xs, 1), SLICE(xs, 1, 2), SET_AGG(x), LEVENSHTEIN_DISTANCE(x, y)", "SELECT CONTAINS(xs, 1), SLICE(xs, 1, 2), ARRAY_AGG(DISTINCT x), LEVENSHTEIN_DISTANCE(x, y)"},
		{"athena", "SELECT FROM_HEX(x), TO_UNIXTIME(x), TO_UTF8(x), SHA256(x), SHA512(x), MD5(x)", "SELECT FROM_HEX(x), TO_UNIXTIME(x), TO_UTF8(x), SHA256(x), SHA512(x), MD5(x)"},
		{"athena", "SELECT BITWISE_AND(x, y), BITWISE_OR(x, y), BITWISE_XOR(x, y), BITWISE_NOT(x)", "SELECT BITWISE_AND(x, y), BITWISE_OR(x, y), BITWISE_XOR(x, y), BITWISE_NOT(x)"},
		{"athena", "SELECT CURRENT_TIMESTAMP, CURRENT_TIME, CURRENT_USER", "SELECT CURRENT_TIMESTAMP, CURRENT_TIME, CURRENT_USER"},
		{"athena", "SELECT IF(x, y, z)", "SELECT IF(x, y, z)"},
		{"athena", "SELECT x ILIKE y", "SELECT LOWER(x) LIKE LOWER(y)"},
		{"athena", "SELECT INITCAP(x)", "SELECT REGEXP_REPLACE(x, '(\\w)(\\w*)', x -> UPPER(x[1]) || LOWER(x[2]))"},
		{"athena", "SELECT SUBSTRING(x, 1, 2)", "SELECT SUBSTR(x, 1, 2)"},
		{"athena", "SELECT JSON_FORMAT(JSON_PARSE(x)), JSON_EXTRACT(x, '$.a')", "SELECT JSON_FORMAT(JSON_PARSE(x)), JSON_EXTRACT(x, '$.a')"},
		{"athena", "SELECT DATE_DIFF('DAY', a, b)", "SELECT DATE_DIFF('DAY', a, b)"},
		{"athena", "SELECT * FROM t TABLESAMPLE BERNOULLI (10)", "SELECT * FROM t TABLESAMPLE BERNOULLI (10)"},
		{"athena", "SELECT CAST(x AS ROW(a INTEGER, b VARCHAR)), CAST(x AS ARRAY(INTEGER)), CAST(x AS MAP(VARCHAR, INTEGER))", "SELECT CAST(x AS ROW(a INTEGER, b VARCHAR)), CAST(x AS ARRAY(INTEGER)), CAST(x AS MAP(VARCHAR, INTEGER))"},
		{"athena", "SELECT INTERVAL '2' WEEKS", "SELECT (2 * INTERVAL '7' DAY)"},
		{"athena", "SELECT DATE_ADD('DAY', MOD(5, 2), y)", "SELECT DATE_ADD('DAY', 5 % 2, y)"},
		{"athena", "/* leading comment */SELECT * FROM foo", "/* leading comment */ SELECT * FROM foo"},
		{"athena", "WITH foo AS (SELECT a, b FROM bar) SELECT * FROM foo", "WITH foo AS (SELECT a, b FROM bar) SELECT * FROM foo"},
	}
	for _, tc := range cases {
		t.Run(tc.dialect+"/"+tc.sql, func(t *testing.T) {
			if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestPrestoOverridesLeaveExistingDialectsUnchanged(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"postgres", "SELECT * FROM t TABLESAMPLE BERNOULLI (10 PERCENT)", "SELECT * FROM t TABLESAMPLE BERNOULLI (10)"},
		{"postgres", "CAST(x AS TIMESTAMPTZ)", "CAST(x AS TIMESTAMPTZ)"},
		{"postgres", "SELECT BOOL_OR(x), SUBSTRING(x, 1, 2)", "SELECT BOOL_OR(x), SUBSTRING(x FROM 1 FOR 2)"},
		{"mysql", "SELECT x & y, IF(a, b, c)", "SELECT x & y, CASE WHEN a THEN b ELSE c END"},
		{"mysql", "SELECT GROUP_CONCAT(x SEPARATOR ',')", "SELECT GROUP_CONCAT(x SEPARATOR ',')"},
	}
	for _, tc := range cases {
		t.Run(tc.dialect+"/"+tc.sql, func(t *testing.T) {
			if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
				t.Fatalf("got %s; want %s", got, tc.want)
			}
		})
	}
}

// generators/presto.py:147-169 _to_int: non-integer-typed intervals are cast to BIGINT.
func TestPrestoDateAddIntervalCast(t *testing.T) {
	cases := []struct{ interval, want string }{
		{"1", "1"}, {"-1", "-1"}, {"(1)", "(1)"}, {"1 + 1", "1 + 1"}, {"FLOOR(5)", "FLOOR(5)"},
		{"MOD(5, 2)", "5 % 2"}, {"COALESCE(1, 2)", "COALESCE(1, 2)"}, {"CAST(x AS INT)", "CAST(x AS INTEGER)"},
		{"CEIL(5.5)", "CAST(CEIL(5.5) AS BIGINT)"}, {"MOD(5, 2.5)", "CAST(5 % 2.5 AS BIGINT)"},
		{"x", "CAST(x AS BIGINT)"}, {"1 + 0.5", "CAST(1 + 0.5 AS BIGINT)"}, {"x * 0.5", "CAST(x * 0.5 AS BIGINT)"},
		{"1 + x", "CAST(1 + x AS BIGINT)"}, {"COALESCE(a, 1)", "CAST(COALESCE(a, 1) AS BIGINT)"},
		{"CAST(x AS DOUBLE)", "CAST(CAST(x AS DOUBLE) AS BIGINT)"}, {"1.0", "CAST(1.0 AS BIGINT)"},
		{"'1'", "CAST('1' AS BIGINT)"}, {"ABS(x)", "CAST(ABS(x) AS BIGINT)"},
	}
	for _, tc := range cases {
		got := roundTrip(t, "presto", "SELECT DATE_ADD('DAY', "+tc.interval+", ts)")
		want := "SELECT DATE_ADD('DAY', " + tc.want + ", ts)"
		if got != want {
			t.Errorf("interval %s: got %q, want %q", tc.interval, got, want)
		}
	}
}

func TestPrestoInitcapCustomDelimiterUnsupported(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT INITCAP(x, '-_')", "presto")
	if err != nil {
		t.Fatal(err)
	}
	level := sqlerrors.RAISE
	if _, err := sqlglot.Generate(e, "presto", generator.Options{UnsupportedLevel: &level}); err == nil {
		t.Fatal("custom INITCAP delimiters must be reported unsupported (presto.py:50-58)")
	}
}

// generators/presto.py:69-89 _schema_sql and trino.py:31 LocationProperty.
func TestPrestoPartitionedBySchemaAndTrinoLocation(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"presto", `CREATE TABLE z (z INT) WITH (PARTITIONED_BY=(order INT, "group" INT, ok INT))`, `CREATE TABLE z (z INTEGER, "order" INTEGER, "group" INTEGER, ok INTEGER) WITH (PARTITIONED_BY=ARRAY['order', 'group', 'ok'])`},
		{"presto", "CREATE TABLE x (w VARCHAR, y INTEGER, z INTEGER) WITH (PARTITIONED_BY=ARRAY['y', 'z'])", "CREATE TABLE x (w VARCHAR, y INTEGER, z INTEGER) WITH (PARTITIONED_BY=ARRAY['y', 'z'])"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.sql, got, tc.want)
		}
	}
	// A LocationProperty node (not the generic Property the parser builds for location=...) renders LOCATION=.
	e, err := sqlglot.ParseOne("CREATE TABLE t WITH (format='ORC') AS SELECT 1", "trino")
	if err != nil {
		t.Fatal(err)
	}
	e.Find(exp.KindProperties).Append("expressions", exp.LocationProperty(exp.Args{"this": exp.LiteralString("s3://b")}))
	got, err := sqlglot.Generate(e, "trino", generator.Options{})
	if err != nil || got != "CREATE TABLE t WITH (format='ORC', LOCATION='s3://b') AS SELECT 1" {
		t.Fatalf("got %q (%v)", got, err)
	}
}

// presto.py:272 HEX_FUNC, :381 SchemaCommentProperty, :591-619 struct_sql, :34-46 SHA2Digest,
// :639-648 create_sql, generator.py:3678-3680 SUPPORTS_SINGLE_ARG_CONCAT.
func TestPrestoClassBehaviors(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT HEX(x)", "SELECT TO_HEX(x)"},
		{"SELECT (ROW(a := 1)).a", "SELECT (CAST(ROW(1) AS ROW(a INTEGER))).a"},
		{"SELECT ROW(a := 1, b := 'x', c := 1.5, d := TRUE)", "SELECT CAST(ROW(1, 'x', 1.5, TRUE) AS ROW(a INTEGER, b VARCHAR, c DOUBLE, d BOOLEAN))"},
		{"SELECT ROW(1, 'x')", "SELECT ROW(1, 'x')"},
		{"SELECT SHA256(CAST(x AS VARCHAR)), SHA256(x)", "SELECT SHA256(TO_UTF8(CAST(x AS VARCHAR))), SHA256(x)"},
		{"SELECT CONCAT(x), CONCAT(x, y)", "SELECT x, CONCAT(x, y)"},
		{"CREATE VIEW v (c) AS SELECT 1", "CREATE VIEW v AS SELECT 1"},
		{`CREATE TABLE IF NOT EXISTS x ("cola" INTEGER, "ds" TEXT) COMMENT 'comment' WITH (PARTITIONED BY=("ds"))`, `CREATE TABLE IF NOT EXISTS x ("cola" INTEGER, "ds" VARCHAR) COMMENT 'comment' WITH (PARTITIONED_BY=ARRAY['ds'])`},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "presto", tc.sql); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.sql, got, tc.want)
		}
	}
}
