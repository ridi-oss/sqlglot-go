package generator_test

import (
	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	"github.com/ridi-oss/sqlglot-go/generator"
	"testing"
)

// Expected SQL comes from pinned v30.17.0 generators/hive.py.
func TestHiveGeneratorParity(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"DATE_SUB(CURRENT_DATE, 1 + 1)", "DATE_ADD(CURRENT_DATE, (1 + 1) * -1)"},
		{"DAY(TO_DATE(x))", "DAY(TO_DATE(x))"},
		{"PERCENTILE(ALL x, 0.5)", "PERCENTILE(x, 0.5)"},
		{"PERCENTILE(DISTINCT x, 0.5)", "PERCENTILE(DISTINCT x, 0.5)"},
		{"SELECT TO_JSON(PARSE_JSON('{\"key\": 123}'))", "SELECT TO_JSON(PARSE_JSON('{\"key\": 123}'))"},
		{"SELECT WEEKOFYEAR('2024-05-22'), DAYOFMONTH('2024-05-22'), DAYOFWEEK('2024-05-22')", "SELECT WEEKOFYEAR('2024-05-22'), DAYOFMONTH('2024-05-22'), DAYOFWEEK('2024-05-22')"},
		{"TO_DATE(TO_DATE(x))", "TO_DATE(TO_DATE(x))"},
		{"TRUNC(date_col, 'MM')", "TRUNC(date_col, 'MM')"},
		{"ALTER TABLE X ADD COLUMNS (y INT, z STRING)", "ALTER TABLE X ADD COLUMNS (y INT, z STRING)"},
		{"ALTER TABLE X ADD COLUMNS (y INT, z STRING) CASCADE", "ALTER TABLE X ADD COLUMNS (y INT, z STRING) CASCADE"},
		{"ALTER TABLE x CHANGE a a VARCHAR(10)", "ALTER TABLE x CHANGE COLUMN a a VARCHAR(10)"},
		{"ALTER VIEW v1 SET TBLPROPERTIES ('tblp1'='1', 'tblp2'='2')", "ALTER VIEW v1 SET TBLPROPERTIES ('tblp1'='1', 'tblp2'='2')"},
		{"CREATE EXTERNAL TABLE X (y INT) STORED BY 'x'", "CREATE EXTERNAL TABLE X (y INT) STORED BY 'x'"},
		{"CREATE EXTERNAL TABLE x (y INT) ROW FORMAT SERDE 'serde' ROW FORMAT DELIMITED FIELDS TERMINATED BY '1' WITH SERDEPROPERTIES ('input.regex'='')", "CREATE EXTERNAL TABLE x (y INT) ROW FORMAT SERDE 'serde' ROW FORMAT DELIMITED FIELDS TERMINATED BY '1' WITH SERDEPROPERTIES ('input.regex'='')"},
		{"CREATE FUNCTION my_func AS 'com.example.MyFunc' USING ARCHIVE 'hdfs://path/to/archive.zip'", "CREATE FUNCTION my_func AS 'com.example.MyFunc' USING ARCHIVE 'hdfs://path/to/archive.zip'"},
		{"CREATE FUNCTION my_func AS 'com.example.MyFunc' USING FILE 'hdfs://path/to/file.py'", "CREATE FUNCTION my_func AS 'com.example.MyFunc' USING FILE 'hdfs://path/to/file.py'"},
		{"CREATE FUNCTION my_func AS 'com.example.MyFunc' USING JAR 'hdfs://path/to/my.jar'", "CREATE FUNCTION my_func AS 'com.example.MyFunc' USING JAR 'hdfs://path/to/my.jar'"},
		{"CREATE OR REPLACE TEMPORARY FUNCTION some_func AS 'my_jar.SomeFunctionUDF' USING JAR 's3://bucket/my.jar'", "CREATE OR REPLACE TEMPORARY FUNCTION some_func AS 'my_jar.SomeFunctionUDF' USING JAR 's3://bucket/my.jar'"},
		{"CREATE TABLE foo (col STRUCT<struct_col_a: VARCHAR((50))>)", "CREATE TABLE foo (col STRUCT<struct_col_a: VARCHAR((50))>)"},
		{"CREATE EXTERNAL TABLE `my_table` (`a7` ARRAY<DATE>) ROW FORMAT SERDE 'a' STORED AS INPUTFORMAT 'b' OUTPUTFORMAT 'c' LOCATION 'd' TBLPROPERTIES ('e'='f')", "CREATE EXTERNAL TABLE `my_table` (`a7` ARRAY<DATE>) ROW FORMAT SERDE 'a' STORED AS INPUTFORMAT 'b' OUTPUTFORMAT 'c' LOCATION 'd' TBLPROPERTIES ('e'='f')"},
		{"ALTER TABLE db.t ADD PARTITION (ds = '2026-01-01') LOCATION 's3://bucket/'", "ALTER TABLE db.t ADD PARTITION(ds = '2026-01-01') LOCATION 's3://bucket/'"},
		{"ALTER TABLE db.t ADD IF NOT EXISTS PARTITION (ds = '2026-01-01') LOCATION 's3://bucket/'", "ALTER TABLE db.t ADD IF NOT EXISTS PARTITION(ds = '2026-01-01') LOCATION 's3://bucket/'"},
		{"ALTER TABLE db.t PARTITION (ds = 'x') SET LOCATION 's3://bucket/'", "ALTER TABLE db.t PARTITION(ds = 'x') SET LOCATION 's3://bucket/'"},
		{"CREATE DATABASE IF NOT EXISTS db LOCATION 's3://bucket/'", "CREATE DATABASE IF NOT EXISTS db LOCATION 's3://bucket/'"},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "hive")
			if err != nil {
				t.Fatal(err)
			}
			before := e.ToS()
			got, err := sqlglot.Generate(e, "hive", generator.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
			if e.ToS() != before {
				t.Fatal("generation mutated the input AST")
			}
		})
	}
}

func TestHiveOverridesLeaveMySQLPostgresUnchanged(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		for _, sql := range []string{"CREATE TABLE x (a VARCHAR(10), b TEXT)", "SELECT JSON_FORMAT(x)", "ALTER TABLE x ADD COLUMN y INT"} {
			t.Run(dialect+"/"+sql, func(t *testing.T) {
				e, err := sqlglot.ParseOne(sql, dialects.DialectType(dialect))
				if err != nil {
					t.Fatal(err)
				}
				got, err := sqlglot.Generate(e, dialects.DialectType(dialect), generator.Options{})
				if err != nil {
					t.Fatal(err)
				}
				if got != sql {
					t.Fatalf("got %s, want %s", got, sql)
				}
			})
		}
	}
}

func TestHivePropertyOverridesLeaveOtherDialectsUnchanged(t *testing.T) {
	for _, tc := range []struct{ dialect, sql string }{
		{"postgres", "CREATE TABLE x (a INT) WITH (fillfactor=70)"},
		{"postgres", "ALTER TABLE x SET (fillfactor = 70)"},
		{"mysql", "CREATE TABLE x (a INT) ENGINE=InnoDB"},
	} {
		t.Run(tc.dialect+"/"+tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, dialects.DialectType(tc.dialect))
			if err != nil {
				t.Fatal(err)
			}
			got, err := sqlglot.Generate(e, dialects.DialectType(tc.dialect), generator.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.sql {
				t.Fatalf("got %s, want %s", got, tc.sql)
			}
		})
	}
}

// Pinned v30.17.0 generators/hive.py:274-395, 482-527, 593-610 — the Hive parser's own
// structured nodes render back in Hive spelling.
func TestHiveFunctionSpellings(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT SIZE(xs)", "SELECT SIZE(xs)"},
		{"SELECT COLLECT_LIST(x)", "SELECT COLLECT_LIST(x)"},
		{"SELECT COLLECT_LIST(DISTINCT x)", "SELECT COLLECT_LIST(DISTINCT x)"},
		{"SELECT GET_JSON_OBJECT(x, '$.a')", "SELECT GET_JSON_OBJECT(x, '$.a')"},
		{"SELECT SEQUENCE(1, 3)", "SELECT SEQUENCE(1, 3)"},
		{"SELECT SEQUENCE(1, 3, 1)", "SELECT SEQUENCE(1, 3, 1)"},
		{"SELECT NAMED_STRUCT('a', 1, 'b', 2)", "SELECT STRUCT(1, 2)"},
		{"SELECT MAP('a', 1, 'b', 2)", "SELECT MAP('a', 1, 'b', 2)"},
		{"SELECT MAP(*)", "SELECT MAP(*)"},
		{"SELECT DATE_FORMAT(x, 'yyyy')", "SELECT DATE_FORMAT(x, 'yyyy')"},
		{"SELECT DATE_FORMAT(x, 'yyyy-MM-dd HH:mm:ss')", "SELECT DATE_FORMAT(x, 'yyyy-MM-dd HH:mm:ss')"},
		{"SELECT DATE_FORMAT(x, 'M/d')", "SELECT DATE_FORMAT(x, 'M/d')"},
		{"SELECT FROM_UNIXTIME(x)", "SELECT FROM_UNIXTIME(x)"},
		{"SELECT FROM_UNIXTIME(x, 'yyyy')", "SELECT FROM_UNIXTIME(x, 'yyyy')"},
		{"SELECT UNIX_TIMESTAMP()", "SELECT UNIX_TIMESTAMP(CURRENT_TIMESTAMP())"},
		{"SELECT UNIX_TIMESTAMP(x)", "SELECT UNIX_TIMESTAMP(x)"},
		{"SELECT UNIX_TIMESTAMP(x, 'yyyy')", "SELECT UNIX_TIMESTAMP(x, 'yyyy')"},
		{"SELECT TO_DATE(x, 'yyyy')", "SELECT TO_DATE(x, 'yyyy')"},
		{"SELECT TO_DATE(x, 'yyyy-MM-dd')", "SELECT TO_DATE(x)"},
		{"SELECT DAY(TO_DATE(x, 'yyyy'))", "SELECT DAY(TO_DATE(x, 'yyyy'))"},
		{"SELECT REGEXP_EXTRACT(a, 'b')", "SELECT REGEXP_EXTRACT(a, 'b')"},
		{"SELECT REGEXP_EXTRACT(a, 'b', 1)", "SELECT REGEXP_EXTRACT(a, 'b')"},
		{"SELECT REGEXP_EXTRACT(a, 'b', 2)", "SELECT REGEXP_EXTRACT(a, 'b', 2)"},
		{"SELECT REGEXP_EXTRACT_ALL(a, 'b', 1)", "SELECT REGEXP_EXTRACT_ALL(a, 'b')"},
		{"SELECT STR_TO_MAP(a)", "SELECT STR_TO_MAP(a, ',', ':')"},
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne(tc.sql, "hive")
		if err != nil {
			t.Fatalf("%s: %v", tc.sql, err)
		}
		got, err := sqlglot.Generate(e, "hive", generator.Options{})
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q (%v), want %q", tc.sql, got, err, tc.want)
		}
	}
}

// hive.py:199-209, 274, 315, 366, 432-437.
func TestHiveCastRegexpIgnoreNullsStrToDate(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT CAST(x AS INT), TRY_CAST(x AS INT)", "SELECT CAST(x AS INT), CAST(x AS INT)"},
		{"SELECT APPROX_COUNT_DISTINCT(a)", "SELECT APPROX_COUNT_DISTINCT(a)"},
		{"SELECT FIRST(c, TRUE), LAST(c, TRUE), FIRST_VALUE(c, TRUE), LAST_VALUE(c, TRUE)", "SELECT FIRST(c, TRUE), LAST(c, TRUE), FIRST_VALUE(c, TRUE), LAST_VALUE(c, TRUE)"},
		{"SELECT a REGEXP 'x', a RLIKE 'x'", "SELECT a RLIKE 'x', a RLIKE 'x'"},
		{"SELECT STR_TO_DATE(x)", "SELECT CAST(x AS DATE)"},
		{"SELECT STR_TO_DATE(x, 'yyyy')", "SELECT CAST(FROM_UNIXTIME(UNIX_TIMESTAMP(x, 'yyyy')) AS DATE)"},
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne(tc.sql, "hive")
		if err != nil {
			t.Fatalf("%s: %v", tc.sql, err)
		}
		got, err := sqlglot.Generate(e, "hive", generator.Options{})
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q (%v), want %q", tc.sql, got, err, tc.want)
		}
	}
}
