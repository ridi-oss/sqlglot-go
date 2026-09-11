package parser_test

import (
	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"testing"
)

func TestHiveParserDrift(t *testing.T) {
	cases := []struct {
		sql, want, seam string
		kind            exp.Kind
	}{
		{"SELECT CURRENT_TIME", "SELECT CURRENT_TIME", "", exp.KindColumn},
		{"SELECT ROW() OVER (DISTRIBUTE BY x SORT BY y)", "SELECT ROW() OVER (PARTITION BY x ORDER BY y)", "", exp.KindWindow},
		{"SELECT ${hiveconf:some_var}", "SELECT ${hiveconf:some_var}", "hiveconf", exp.KindParameter},
		{"SELECT ${some_var}", "SELECT ${some_var}", "", exp.KindParameter},
		{"SELECT {'a', b}", "SELECT STRUCT('a', b)", "", exp.KindStruct},
		{"SELECT * FROM x.z y TABLESAMPLE (10 PERCENT) WHERE 1 = 1", "SELECT * FROM x.z AS y WHERE 1 = 1 TABLESAMPLE (10 PERCENT)", "", exp.KindStar},
		{"SELECT LOCALTIME()", "SELECT LOCALTIME", "", exp.KindLocaltime},
		{"SELECT LOCALTIME(3)", "SELECT LOCALTIME(3)", "", exp.KindLocaltime},
		{"SELECT * FROM my_table TIMESTAMP AS OF DATE_ADD(CURRENT_DATE, -1)", "SELECT * FROM my_table TIMESTAMP AS OF DATE_ADD(CURRENT_DATE, -1)", "", exp.KindStar},
		{"SELECT * FROM my_table VERSION AS OF DATE_ADD(CURRENT_DATE, -1)", "SELECT * FROM my_table VERSION AS OF DATE_ADD(CURRENT_DATE, -1)", "", exp.KindStar},
		{"SELECT * FROM x TABLESAMPLE (1 PERCENT) AS foo", "SELECT * FROM x TABLESAMPLE (1 PERCENT) AS foo", "", exp.KindStar},
		{"SELECT * FROM x.z TABLESAMPLE(10 PERCENT) y", "SELECT * FROM x.z TABLESAMPLE (10 PERCENT) AS y", "", exp.KindStar},
		{"SELECT UNIX_TIMESTAMP(x, 'yyyy-MM-dd')", "SELECT UNIX_TIMESTAMP(x, 'yyyy-MM-dd')", "", exp.KindStrToUnix},
		{"SELECT TO_DATE(x, 'MM/dd/yyyy')", "SELECT TO_DATE(x, 'MM/dd/yyyy')", "", exp.KindTsOrDsToDate},
		{"SELECT DATE_FORMAT(x, 'MM')", "SELECT DATE_FORMAT(x, 'MM')", "", exp.KindTimeToStr},
		{"SELECT FROM_UNIXTIME(x, 'dd')", "SELECT FROM_UNIXTIME(x, 'dd')", "", exp.KindUnixToStr},
		{"SELECT GET_JSON_OBJECT(x, '$.a[0]')", "SELECT GET_JSON_OBJECT(x, '$.a[0]')", "", exp.KindJSONExtractScalar},
		{"SELECT DATE_ADD(d, 1)", "SELECT DATE_ADD(d, 1)", "", exp.KindTsOrDsAdd},
		{"SELECT DATE_SUB(d, 1)", "SELECT DATE_ADD(d, 1 * -1)", "", exp.KindTsOrDsAdd},
		{"SELECT TRUNC(d, 'MM')", "SELECT TRUNC(d, 'MM')", "", exp.KindTimestampTrunc},
		{"SELECT TRUNC(d, 'month')", "SELECT TRUNC(d, 'MONTH')", "", exp.KindTimestampTrunc},
		{"SELECT TRUNC(d, 'Q')", "SELECT TRUNC(d, 'QUARTER')", "", exp.KindTimestampTrunc},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e := parseOneDialect(t, tc.sql, "hive")
			projection := hiveProjection(t, e)
			if projection.Kind() != tc.kind {
				t.Fatalf("kind = %v, want %v", projection.Kind(), tc.kind)
			}
			if tc.kind == exp.KindParameter && tc.seam == "hiveconf" && exprArg(t, projection, "expression").Name() != "some_var" {
				t.Fatalf("parameter shape: %s", projection.ToS())
			}
			if tc.kind == exp.KindStruct {
				fields := projection.Expressions()
				if len(fields) != 2 || fields[0].Kind() != exp.KindPropertyEQ || fields[0].This().Name() != "col1" || fields[1].This().Name() != "b" {
					t.Fatalf("struct shape: %s", projection.ToS())
				}
			}
			if tc.kind == exp.KindWindow {
				if len(projection.Arg("partition_by").([]exp.Expression)) != 1 || exprArg(t, projection, "order").Kind() != exp.KindOrder {
					t.Fatalf("window shape: %s", projection.ToS())
				}
			}
			if tc.kind == exp.KindTsOrDsAdd {
				unit := exprArg(t, projection, "unit")
				if unit.Kind() != exp.KindVar || unit.Name() != "DAY" {
					t.Fatalf("unit = %s", unit.ToS())
				}
			}
			got, err := sqlglot.Generate(e, "hive", generator.Options{})
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// parser.py:3650-3652: STORED precedes BY NAME and IF EXISTS.
func TestHiveInsertStoredOrder(t *testing.T) {
	for _, sql := range []string{"INSERT INTO t STORED AS PARQUET BY NAME SELECT 1", "INSERT INTO t STORED AS PARQUET IF EXISTS SELECT 1"} {
		e := parseOneDialect(t, sql, "hive")
		if got, err := sqlglot.Generate(e, "hive", generator.Options{}); err != nil || got != sql {
			t.Fatalf("got %q, %v; want %q", got, err, sql)
		}
	}
	if _, err := sqlglot.ParseOne("INSERT INTO t BY NAME STORED AS PARQUET SELECT 1", "hive"); err == nil {
		t.Fatal("BY NAME before STORED must not parse")
	}
}

func TestHiveDirectoryStoredDrift(t *testing.T) {
	for _, local := range []string{"", "LOCAL "} {
		t.Run(local, func(t *testing.T) {
			sql := "INSERT OVERWRITE " + local + "DIRECTORY 'x' ROW FORMAT DELIMITED FIELDS TERMINATED BY '\x01' COLLECTION ITEMS TERMINATED BY ',' MAP KEYS TERMINATED BY ':' LINES TERMINATED BY '' STORED AS TEXTFILE SELECT * FROM `a`.`b`"
			e := parseOneDialect(t, sql, "hive")
			if e.This().Kind() != exp.KindDirectory || exprArg(t, e, "stored").Kind() != exp.KindFileFormatProperty {
				t.Fatalf("unexpected AST: %s", e.ToS())
			}
			got, err := sqlglot.Generate(e, "hive", generator.Options{})
			if err != nil || got != sql {
				t.Fatalf("got %q, %v; want %q", got, err, sql)
			}
		})
	}
}
