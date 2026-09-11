package generator_test

import (
	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"testing"
)

// Expected SQL comes from pinned v30.17.0 generators/athena.py.
func TestAthenaGeneratorParity(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"/* leading comment */CREATE SCHEMA foo", "/* leading comment */ CREATE SCHEMA foo"},
		{"/* leading comment */SELECT * FROM foo", "/* leading comment */ SELECT * FROM foo"},
		{"DESCRIBE foo.bar", "DESCRIBE foo.bar"},
		{"DROP TABLE \"foo\"", "DROP TABLE `foo`"},
		{"DROP TABLE `foo`", "DROP TABLE `foo`"},
		{"DROP TABLE foo", "DROP TABLE foo"},
		{"WITH foo AS (SELECT a, b FROM bar) SELECT * FROM foo", "WITH foo AS (SELECT a, b FROM bar) SELECT * FROM foo"},
		{"ALTER TABLE `foo` DROP COLUMN `id`", "ALTER TABLE `foo` DROP COLUMN `id`"},
		{"ALTER TABLE `foo`.`bar` ADD COLUMN `end_ts` BIGINT", "ALTER TABLE `foo`.`bar` ADD COLUMNS (`end_ts` BIGINT)"},
		{"CREATE EXTERNAL TABLE IF NOT EXISTS foo (a INT, b STRING) ROW FORMAT SERDE 'org.openx.data.jsonserde.JsonSerDe' WITH SERDEPROPERTIES ('case.insensitive'='FALSE') LOCATION 's3://table/path'", "CREATE EXTERNAL TABLE IF NOT EXISTS foo (a INT, b STRING) ROW FORMAT SERDE 'org.openx.data.jsonserde.JsonSerDe' WITH SERDEPROPERTIES ('case.insensitive'='FALSE') LOCATION 's3://table/path'"},
		{"CREATE EXTERNAL TABLE `foo` (`id` INT) LOCATION 's3://foo/'", "CREATE EXTERNAL TABLE `foo` (`id` INT) LOCATION 's3://foo/'"},
		{"CREATE EXTERNAL TABLE foo (id INT) COMMENT 'test comment'", "CREATE EXTERNAL TABLE foo (id INT) COMMENT 'test comment'"},
		{"CREATE EXTERNAL TABLE foo (id INT) LOCATION 's3://foo/'", "CREATE EXTERNAL TABLE foo (id INT) LOCATION 's3://foo/'"},
		{"CREATE EXTERNAL TABLE foo (id INT, val STRING) CLUSTERED BY (id, val) INTO 10 BUCKETS", "CREATE EXTERNAL TABLE foo (id INT, val STRING) CLUSTERED BY (id, val) INTO 10 BUCKETS"},
		{"CREATE EXTERNAL TABLE foo (id INT, val STRING) STORED AS PARQUET LOCATION 's3://foo' TBLPROPERTIES ('has_encryped_data'='true', 'classification'='test')", "CREATE EXTERNAL TABLE foo (id INT, val STRING) STORED AS PARQUET LOCATION 's3://foo' TBLPROPERTIES ('has_encryped_data'='true', 'classification'='test')"},
		{"CREATE EXTERNAL TABLE george.t (id INT COMMENT 'foo \\\\ bar') LOCATION 's3://my-bucket/'", "CREATE EXTERNAL TABLE george.t (id INT COMMENT 'foo \\\\ bar') LOCATION 's3://my-bucket/'"},
		{"CREATE EXTERNAL TABLE my_table (id BIGINT COMMENT 'this is the row\\'s id') LOCATION 's3://my-s3-bucket'", "CREATE EXTERNAL TABLE my_table (id BIGINT COMMENT 'this is the row\\'s id') LOCATION 's3://my-s3-bucket'"},
		{"CREATE EXTERNAL TABLE x (y INT) ROW FORMAT SERDE 'serde' ROW FORMAT DELIMITED FIELDS TERMINATED BY '1' WITH SERDEPROPERTIES ('input.regex'='')", "CREATE EXTERNAL TABLE x (y INT) ROW FORMAT SERDE 'serde' ROW FORMAT DELIMITED FIELDS TERMINATED BY '1' WITH SERDEPROPERTIES ('input.regex'='')"},
		{"CREATE OR REPLACE TABLE iceberg_table (`id` BIGINT, `data` STRING, category STRING) PARTITIONED BY (category, BUCKET(16, id)) LOCATION 's3://amzn-s3-demo-bucket/your-folder/' TBLPROPERTIES ('table_type'='ICEBERG', 'write_compression'='snappy')", "CREATE OR REPLACE TABLE iceberg_table (`id` BIGINT, `data` STRING, category STRING) PARTITIONED BY (category, BUCKET(16, id)) LOCATION 's3://amzn-s3-demo-bucket/your-folder/' TBLPROPERTIES ('table_type'='ICEBERG', 'write_compression'='snappy')"},
		{"CREATE SCHEMA \"foo\"", "CREATE SCHEMA `foo`"},
		{"CREATE SCHEMA `foo`", "CREATE SCHEMA `foo`"},
		{"CREATE TABLE IF NOT EXISTS t (name STRING) LOCATION 's3://bucket/tmp/mytable/' TBLPROPERTIES ('table_type'='iceberg', 'FORMAT'='parquet')", "CREATE TABLE IF NOT EXISTS t (name STRING) LOCATION 's3://bucket/tmp/mytable/' TBLPROPERTIES ('table_type'='iceberg', 'FORMAT'='parquet')"},
		{"CREATE TABLE foo WITH (table_type='HIVE', external_location='s3://foo/', format='parquet', partitioned_by=ARRAY['ds']) AS SELECT * FROM a", "CREATE TABLE foo WITH (table_type='HIVE', external_location='s3://foo/', format='parquet', partitioned_by=ARRAY['ds']) AS SELECT * FROM a"},
		{"CREATE TABLE foo WITH (table_type='ICEBERG', location='s3://foo/', format='orc', partitioning=ARRAY['bucket(id, 5)']) AS SELECT * FROM a", "CREATE TABLE foo WITH (table_type='ICEBERG', location='s3://foo/', format='orc', partitioning=ARRAY['bucket(id, 5)']) AS SELECT * FROM a"},
		{"CREATE TABLE iceberg_table (`id` BIGINT, `data` STRING, category STRING) PARTITIONED BY (category, BUCKET(16, id)) LOCATION 's3://amzn-s3-demo-bucket/your-folder/' TBLPROPERTIES ('table_type'='ICEBERG', 'write_compression'='snappy')", "CREATE TABLE iceberg_table (`id` BIGINT, `data` STRING, category STRING) PARTITIONED BY (category, BUCKET(16, id)) LOCATION 's3://amzn-s3-demo-bucket/your-folder/' TBLPROPERTIES ('table_type'='ICEBERG', 'write_compression'='snappy')"},
		{"CREATE VIEW foo AS SELECT id FROM tbl", "CREATE VIEW foo AS SELECT id FROM tbl"},
		{"CREATE EXTERNAL TABLE `my_table` (`a7` ARRAY<DATE>) ROW FORMAT SERDE 'a' STORED AS INPUTFORMAT 'b' OUTPUTFORMAT 'c' LOCATION 'd' TBLPROPERTIES ('e'='f')", "CREATE EXTERNAL TABLE `my_table` (`a7` ARRAY<DATE>) ROW FORMAT SERDE 'a' STORED AS INPUTFORMAT 'b' OUTPUTFORMAT 'c' LOCATION 'd' TBLPROPERTIES ('e'='f')"},
		{"ALTER TABLE db.t ADD PARTITION (ds = '2026-01-01') LOCATION 's3://bucket/'", "ALTER TABLE db.t ADD PARTITION(ds = '2026-01-01') LOCATION 's3://bucket/'"},
		{"ALTER TABLE db.t ADD IF NOT EXISTS PARTITION (ds = '2026-01-01') LOCATION 's3://bucket/'", "ALTER TABLE db.t ADD IF NOT EXISTS PARTITION(ds = '2026-01-01') LOCATION 's3://bucket/'"},
		{"ALTER TABLE db.t PARTITION (ds = 'x') SET LOCATION 's3://bucket/'", "ALTER TABLE db.t PARTITION(ds = 'x') SET LOCATION 's3://bucket/'"},
		{"CREATE DATABASE IF NOT EXISTS db LOCATION 's3://bucket/'", "CREATE DATABASE IF NOT EXISTS db LOCATION 's3://bucket/'"},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "athena")
			if err != nil {
				t.Fatal(err)
			}
			before := e.ToS()
			got, err := sqlglot.Generate(e, "athena", generator.Options{})
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

func TestAthenaStructuredCTASProperties(t *testing.T) {
	for _, tc := range []struct{ tableType, location, partition string }{
		{"HIVE", "external_location", "partitioned_by"},
		{"ICEBERG", "location", "partitioning"},
		{"iceberg", "location", "partitioning"},
	} {
		t.Run(tc.tableType, func(t *testing.T) {
			e, err := sqlglot.ParseOne("CREATE TABLE foo AS SELECT * FROM a", "athena")
			if err != nil {
				t.Fatal(err)
			}
			e.Set("properties", expressions.Properties(expressions.Args{"expressions": []expressions.Expression{
				expressions.New(expressions.KindProperty, expressions.Args{"this": expressions.LiteralString("table_type"), "value": expressions.LiteralString(tc.tableType)}),
				expressions.New(expressions.KindLocationProperty, expressions.Args{"this": expressions.LiteralString("s3://foo/")}),
				expressions.New(expressions.KindPartitionedByProperty, expressions.Args{"this": expressions.New(expressions.KindArray, expressions.Args{"expressions": []expressions.Expression{expressions.LiteralString("ds")}})}),
			}}))
			got, err := sqlglot.Generate(e, "athena", generator.Options{})
			if err != nil {
				t.Fatal(err)
			}
			want := "CREATE TABLE foo WITH (table_type='" + tc.tableType + "', " + tc.location + "='s3://foo/', " + tc.partition + "=ARRAY['ds']) AS SELECT * FROM a"
			if got != want {
				t.Fatalf("got %s\nwant %s", got, want)
			}
		})
	}
}
