package parser_test

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"testing"
)

func TestAthenaStatementShape(t *testing.T) {
	for _, tc := range []struct{ sql, name string }{
		{"SHOW COLUMNS FROM db.t", "COLUMNS"}, {"SHOW CREATE TABLE db.t", "CREATE TABLE"},
		{"SHOW CREATE VIEW db.t", "CREATE VIEW"}, {"SHOW PARTITIONS db.t", "PARTITIONS"},
		{"SHOW TBLPROPERTIES db.t ('key')", "TBLPROPERTIES"},
	} {
		e := parseOneDialect(t, tc.sql, "athena")
		if e.Kind() != exp.KindShow || e.Text("this") != tc.name {
			t.Fatalf("%s: %s", tc.sql, e.ToS())
		}
		target, ok := e.Arg("target").(exp.Expression)
		if !ok || target.Kind() != exp.KindTable || target.SchemaName() != "db" || target.Name() != "t" {
			t.Fatalf("%s: %s", tc.sql, e.ToS())
		}
	}
	for _, tc := range []struct {
		sql  string
		kind exp.Kind
	}{
		{"EXPLAIN (TYPE IO, FORMAT JSON) SELECT * FROM t", exp.KindDescribe},
		{"UNLOAD (SELECT * FROM t) TO 's3://bucket/prefix/' WITH (format = 'PARQUET')", exp.KindUnload},
	} {
		e := parseOneDialect(t, tc.sql, "athena")
		if e.Kind() != tc.kind {
			t.Fatalf("%s: %s", tc.sql, e.ToS())
		}
		query := e.Find(exp.KindSelect)
		if query == nil || query.Find(exp.KindFrom) == nil {
			t.Fatalf("missing inner SELECT FROM: %s", e.ToS())
		}
		if tc.kind == exp.KindDescribe && e.Text("kind") != "EXPLAIN" {
			t.Fatal(e.ToS())
		}
		if tc.kind == exp.KindUnload {
			if e.This().Kind() != exp.KindSubquery {
				t.Fatal(e.ToS())
			}
			files := expressionsForArg(e, "files")
			params := expressionsForArg(e, "params")
			if len(files) != 1 || !files[0].IsString() || files[0].Name() != "s3://bucket/prefix/" {
				t.Fatal(e.ToS())
			}
			if len(params) != 1 || params[0].Kind() != exp.KindCopyParameter || params[0].This().Kind() != exp.KindVar || params[0].This().Name() != "format" {
				t.Fatal(e.ToS())
			}
		} else {
			options := e.Expressions()
			if e.Arg("wrapped") != true || len(options) != 2 || options[0].This().Name() != "TYPE" || options[0].Text("expression") != "IO" || options[1].This().Name() != "FORMAT" || options[1].Text("expression") != "JSON" {
				t.Fatal(e.ToS())
			}
		}
	}
	e := parseOneDialect(t, "ALTER TABLE t PARTITION (ds = 'old') RENAME TO PARTITION (ds = 'new')", "athena")
	if e.Kind() != exp.KindAlter || e.This().Kind() != exp.KindTable || e.This().Arg("partition") == nil {
		t.Fatal(e.ToS())
	}
	rename := e.Find(exp.KindAlterRename)
	if rename == nil || rename.This().Kind() != exp.KindPartition {
		t.Fatal(e.ToS())
	}
}

func TestAthenaStatementsFailClosed(t *testing.T) {
	for _, sql := range []string{
		"SHOW COLUMNS", "SHOW COLUMNS FROM db.", "SHOW TBLPROPERTIES t ()", "SHOW PARTITIONS t unexpected", "SHOW CREATE MATERIALIZED VIEW v", "SHOW VIEWS LIKE",
		"EXPLAIN (FORMAT YAML) SELECT * FROM t", "EXPLAIN (FORMAT JSON,) SELECT * FROM t", "EXPLAIN SELECT * FROM", "EXPLAIN (TYPE) SELECT * FROM t", "EXPLAIN VERBOSE SELECT 1", "EXPLAIN ANALYZE VERBOSE SELECT 1", "EXPLAIN ANALYZE (TYPE IO) SELECT 1", "EXPLAIN ANALYZE (FORMAT GRAPHVIZ) SELECT 1", "EXPLAIN SHOW TABLES", "EXPLAIN CREATE TABLE x (a INT)", "EXPLAIN UPDATE t SET a = 1",
		"SHOW COLUMNS FROM db.orders FROM customers", "SHOW COLUMNS FROM orders IN a.b.c",
		"SHOW COLUMNS FROM t PIVOT(SUM(a) FOR b IN (1))", "SHOW COLUMNS FROM t IN db PIVOT(SUM(a) FOR b IN (1))", "SHOW PARTITIONS t PIVOT(SUM(a) FOR b IN (1))",
		"UNLOAD (SELECT * FROM t) WITH (format = 'PARQUET')", "UNLOAD (SELECT * FROM t) TO 's3://bucket/'", "UNLOAD (SELECT * FROM t) TO 's3://bucket/' WITH (format =)", "UNLOAD (SELECT * FROM t) TO 's3://bucket/' WITH (compression = 'SNAPPY')", "UNLOAD (SELECT * FROM t) TO 's3://bucket/' WITH (format = 'CSV')", "UNLOAD (SELECT * FROM t) TO 's3://bucket/' WITH (format = 'JSON', bogus = 1)", "UNLOAD (SELECT * FROM t) TO 's3://bucket/' WITH (format = 'JSON', format = 'ORC')",
		"ALTER TABLE t PARTITION (ds = 'old') RENAME TO PARTITION (ds =)", "ALTER TABLE t PARTITION (ds = 'old') RENAME TO PARTITION ()", "ALTER TABLE t RENAME TO PARTITION (ds = 'new')", "ALTER TABLE t PARTITION () RENAME TO PARTITION (ds = 'new')",
	} {
		t.Run(sql, func(t *testing.T) {
			e := parseOneDialect(t, sql, "athena")
			if e.Kind() != exp.KindCommand {
				t.Fatalf("expected Command: %s", e.ToS())
			}
		})
	}
}
