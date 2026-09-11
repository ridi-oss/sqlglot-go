package sqlglot_test

import (
	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"testing"
)

func TestAthenaStatementRoundTrip(t *testing.T) {
	for _, sql := range []string{
		"SHOW COLUMNS FROM db.t", "SHOW COLUMNS IN db.t", "SHOW CREATE TABLE db.t", "SHOW CREATE VIEW db.v",
		"SHOW DATABASES LIKE 'x%'", "SHOW SCHEMAS", "SHOW SCHEMAS LIKE 'x%'", "SHOW PARTITIONS db.t",
		"SHOW TABLES", "SHOW TABLES IN db 'x*'", "SHOW TBLPROPERTIES db.t", "SHOW TBLPROPERTIES db.t ('key')",
		"SHOW VIEWS", "SHOW VIEWS IN db LIKE 'x*'",
		"SHOW COLUMNS FROM orders FROM customers", "SHOW COLUMNS IN orders IN customers", "SHOW COLUMNS IN `hms-catalog-1`.db.orders", "SHOW TABLES IN `hms-catalog-1`.db",
		"EXPLAIN SELECT 1", "EXPLAIN ANALYZE SELECT 1", "EXPLAIN ANALYZE (FORMAT JSON) SELECT * FROM t", "EXPLAIN CREATE TABLE x AS SELECT 1", "EXPLAIN INSERT INTO t SELECT 1",
		"EXPLAIN (FORMAT JSON) SELECT 1", "EXPLAIN (FORMAT GRAPHVIZ, TYPE LOGICAL) SELECT * FROM t", "EXPLAIN (TYPE VALIDATE) SELECT * FROM t", "EXPLAIN (TYPE DISTRIBUTED, FORMAT TEXT) SELECT * FROM t",
		"UNLOAD (SELECT * FROM t) TO 's3://bucket/prefix/' WITH (format = 'PARQUET')",
		"UNLOAD (SELECT * FROM t) TO 's3://bucket/prefix/' WITH (format = 'PARQUET', compression = 'SNAPPY')",
		"UNLOAD (SELECT * FROM t) TO 's3://bucket/prefix/' WITH (format = 'TEXTFILE', partitioned_by = ARRAY['key1'], field_delimiter = ',')",
		"UNLOAD (SELECT * FROM t) TO 's3://bucket/prefix/' WITH (format = 'PARQUET', compression = 'ZSTD', compression_level = 4)",
		"ALTER TABLE t PARTITION(ds = 'old') RENAME TO PARTITION(ds = 'new')",
	} {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "athena")
			if err != nil {
				t.Fatal(err)
			}
			if e.Kind() == exp.KindCommand {
				t.Fatalf("statement fell back to Command: %s", e.ToS())
			}
			got, err := sqlglot.Generate(e, "athena", generator.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got != sql {
				t.Fatalf("got %q, want %q\n%s", got, sql, e.ToS())
			}
		})
	}
}

func TestAthenaCopyKeepsUpstreamRendering(t *testing.T) {
	for _, tc := range []struct{ sql, want string }{
		{"COPY foo FROM 'x'", "COPY INTO foo FROM 'x'"},
		{"COPY (SELECT 1) TO 'x' WITH (format = 'PARQUET')", "COPY INTO (SELECT 1) TO 'x' WITH (format 'PARQUET')"},
	} {
		for _, dialect := range []string{"athena", "trino"} {
			e, err := sqlglot.ParseOne(tc.sql, dialect)
			if err != nil {
				t.Fatal(err)
			}
			if e.Kind() != exp.KindCopy {
				t.Fatalf("%s: %s", tc.sql, e.ToS())
			}
			got, err := sqlglot.Generate(e, dialect, generator.Options{})
			if err != nil || got != tc.want {
				t.Fatalf("%s under %s: got %q (%v), want %q", tc.sql, dialect, got, err, tc.want)
			}
		}
	}
}
