package generator_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
)

func createTable(name string) exp.Expression {
	return exp.Table(exp.Args{"this": exp.ToIdentifier(name)})
}

func createQualifiedTable(db, name string) exp.Expression {
	return exp.Table(exp.Args{"this": exp.ToIdentifier(name), "db": exp.ToIdentifier(db)})
}

func createColumn(name string) exp.Expression {
	return exp.Column(exp.Args{"this": exp.ToIdentifier(name)})
}

func TestCreatePropertyRenderers(t *testing.T) {
	stringLiteral := func(value string) exp.Expression { return exp.LiteralString(value) }
	numberLiteral := func(value string) exp.Expression {
		return exp.Literal(exp.Args{"this": value, "is_string": false})
	}
	propertyOption := func(kind, value string) exp.Expression {
		return exp.Property(exp.Args{"this": kind, "value": exp.Var(exp.Args{"this": value})})
	}

	cases := []struct {
		name string
		d    *dialects.Dialect
		node exp.Expression
		want string
	}{
		{"generic property", nil, exp.Property(exp.Args{"this": "x", "value": stringLiteral("2")}), "x='2'"},
		{"dotted generic property", nil, exp.Property(exp.Args{
			"this":  exp.Dot(exp.Args{"this": exp.ToIdentifier("catalog"), "expression": exp.ToIdentifier("option")}),
			"value": stringLiteral("x"),
		}), "catalog.option='x'"},
		{"algorithm", nil, exp.AlgorithmProperty(exp.Args{"this": exp.Var(exp.Args{"this": "UNDEFINED"})}), "ALGORITHM=UNDEFINED"},
		{"auto increment", nil, exp.AutoIncrementProperty(exp.Args{"this": numberLiteral("1")}), "AUTO_INCREMENT=1"},
		{"collate", nil, exp.CollateProperty(exp.Args{"this": exp.Var(exp.Args{"this": "utf8_bin"}), "default": true}), "COLLATE=utf8_bin"},
		{"definer", nil, exp.DefinerProperty(exp.Args{"this": "foo@%"}), "DEFINER=foo@%"},
		{"engine", nil, exp.EngineProperty(exp.Args{"this": exp.Var(exp.Args{"this": "InnoDB"})}), "ENGINE=InnoDB"},
		{"lock", nil, exp.LockProperty(exp.Args{"this": exp.Var(exp.Args{"this": "EXCLUSIVE"})}), "LOCK=EXCLUSIVE"},
		{"schema comment", nil, exp.SchemaCommentProperty(exp.Args{"this": stringLiteral("comment")}), "COMMENT='comment'"},
		{"temporary", nil, exp.TemporaryProperty(exp.Args{}), "TEMPORARY"},
		{"materialized", nil, exp.MaterializedProperty(exp.Args{}), "MATERIALIZED"},
		{"unlogged", nil, exp.UnloggedProperty(exp.Args{}), "UNLOGGED"},
		{"inherits", dialects.Postgres(), exp.InheritsProperty(exp.Args{"expressions": []exp.Expression{createQualifiedTable("s", "t1"), createQualifiedTable("s", "t2")}}), "INHERITS (s.t1, s.t2)"},
		{"like", nil, exp.LikeProperty(exp.Args{"this": createTable("b"), "expressions": []exp.Expression{propertyOption("INCLUDING", "CONSTRAINT")}}), "LIKE b INCLUDING CONSTRAINT"},
		{"like postgres outside schema", dialects.Postgres(), exp.LikeProperty(exp.Args{"this": createTable("b")}), "(LIKE b)"},
		{"no primary index", nil, exp.NoPrimaryIndexProperty(exp.Args{}), "NO PRIMARY INDEX"},
		{"on commit preserve", nil, exp.OnCommitProperty(exp.Args{"delete": false}), "ON COMMIT PRESERVE ROWS"},
		{"on commit delete", nil, exp.OnCommitProperty(exp.Args{"delete": true}), "ON COMMIT DELETE ROWS"},
		{"SQL read write", nil, exp.SqlReadWriteProperty(exp.Args{"this": "MODIFIES SQL DATA"}), "MODIFIES SQL DATA"},
		{"locking", nil, exp.LockingProperty(exp.Args{"kind": "ROW", "for_or_in": "FOR", "lock_type": "ACCESS", "override": false}), "LOCKING ROW FOR ACCESS"},
		{"partitioned by base", nil, exp.PartitionedByProperty(exp.Args{"this": exp.Anonymous(exp.Args{"this": "HASH", "expressions": []exp.Expression{createColumn("foo")}})}), "PARTITIONED_BY=HASH(foo)"},
		{"partitioned by postgres", dialects.Postgres(), exp.PartitionedByProperty(exp.Args{"this": exp.Anonymous(exp.Args{"this": "HASH", "expressions": []exp.Expression{createColumn("foo")}})}), "PARTITION BY HASH(foo)"},
		{"partition bound list", dialects.Postgres(), exp.PartitionBoundSpec(exp.Args{"this": []exp.Expression{stringLiteral("a"), stringLiteral("b")}}), "IN ('a', 'b')"},
		{"partition bound modulus", dialects.Postgres(), exp.PartitionBoundSpec(exp.Args{"this": numberLiteral("3"), "expression": numberLiteral("2")}), "WITH (MODULUS 3, REMAINDER 2)"},
		{"partition bound range", dialects.Postgres(), exp.PartitionBoundSpec(exp.Args{"from_expressions": []exp.Expression{numberLiteral("1")}, "to_expressions": []exp.Expression{numberLiteral("2")}}), "FROM (1) TO (2)"},
		{"partition of default", dialects.Postgres(), exp.PartitionedOfProperty(exp.Args{"this": createTable("cities"), "expression": exp.Var(exp.Args{"this": "DEFAULT"})}), "PARTITION OF cities DEFAULT"},
		{"partition of values", dialects.Postgres(), exp.PartitionedOfProperty(exp.Args{
			"this":       createTable("cities"),
			"expression": exp.PartitionBoundSpec(exp.Args{"this": []exp.Expression{stringLiteral("a"), stringLiteral("b")}}),
		}), "PARTITION OF cities FOR VALUES IN ('a', 'b')"},
		{"with data", nil, exp.WithDataProperty(exp.Args{"no": false}), "WITH DATA"},
		{"with no data and no statistics", nil, exp.WithDataProperty(exp.Args{"no": true, "statistics": false}), "WITH NO DATA AND NO STATISTICS"},
		{"with data and statistics", nil, exp.WithDataProperty(exp.Args{"no": false, "statistics": true}), "WITH DATA AND STATISTICS"},
		{"character set existing renderer", nil, exp.CharacterSetProperty(exp.Args{"this": exp.Var(exp.Args{"this": "utf8"}), "default": true}), "DEFAULT CHARACTER SET=utf8"},
		{"file format existing renderer", nil, exp.FileFormatProperty(exp.Args{"this": stringLiteral("ORC")}), "FORMAT='ORC'"},
		{"row format existing renderer", nil, exp.RowFormatProperty(exp.Args{"this": exp.Var(exp.Args{"this": "DYNAMIC"})}), "ROW_FORMAT=DYNAMIC"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := genSQL(t, tc.d, tc.node); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLikePropertyInsidePostgresSchema(t *testing.T) {
	node := exp.Schema(exp.Args{
		"this":        createTable("a"),
		"expressions": []exp.Expression{exp.LikeProperty(exp.Args{"this": createTable("b")})},
	})
	if got, want := genSQL(t, dialects.Postgres(), node), "a (LIKE b)"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPropertiesSQLBucketsRootAndWith(t *testing.T) {
	node := exp.Properties(exp.Args{"expressions": []exp.Expression{
		exp.EngineProperty(exp.Args{"this": exp.Var(exp.Args{"this": "InnoDB"})}),
		exp.AutoIncrementProperty(exp.Args{"this": exp.LiteralNumber(1)}),
		exp.Property(exp.Args{"this": "x", "value": exp.LiteralString("2")}),
		exp.FileFormatProperty(exp.Args{"this": exp.LiteralString("ORC")}),
	}})
	if got, want := genSQL(t, nil, node), "ENGINE=InnoDB AUTO_INCREMENT=1 WITH (x='2', FORMAT='ORC')"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCreatePropertyLocations(t *testing.T) {
	properties := exp.Properties(exp.Args{"expressions": []exp.Expression{
		exp.TemporaryProperty(exp.Args{}),
		exp.EngineProperty(exp.Args{"this": exp.Var(exp.Args{"this": "InnoDB"})}),
		exp.Property(exp.Args{"this": "x", "value": exp.LiteralString("2")}),
		exp.LockingProperty(exp.Args{"kind": "ROW", "for_or_in": "FOR", "lock_type": "ACCESS", "override": false}),
		exp.WithDataProperty(exp.Args{"no": false}),
	}})
	node := exp.Create(exp.Args{
		"this":       createTable("t"),
		"kind":       "TABLE",
		"properties": properties,
		"expression": exp.Select(exp.Args{"expressions": []exp.Expression{exp.LiteralNumber(1)}}),
	})
	want := "CREATE TEMPORARY TABLE t ENGINE=InnoDB WITH (x='2') AS LOCKING ROW FOR ACCESS SELECT 1 WITH DATA"
	if got := genSQL(t, nil, node); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPartitionedByPropertyLocationByDialect(t *testing.T) {
	columnDef := exp.ColumnDef(exp.Args{
		"this": exp.ToIdentifier("foo"),
		"kind": exp.DataType(exp.Args{"this": exp.DTypeInt}),
	})
	this := exp.Schema(exp.Args{"this": createTable("test"), "expressions": []exp.Expression{columnDef}})

	base := exp.Create(exp.Args{
		"this": this,
		"kind": "TABLE",
		"properties": exp.Properties(exp.Args{"expressions": []exp.Expression{
			exp.PartitionedByProperty(exp.Args{"this": exp.Schema(exp.Args{"expressions": []exp.Expression{columnDef.Copy()}})}),
		}}),
	})
	if got, want := genSQL(t, nil, base), "CREATE TABLE test (foo INT) WITH (PARTITIONED_BY=(foo INT))"; got != want {
		t.Fatalf("base got %q, want %q", got, want)
	}

	postgres := exp.Create(exp.Args{
		"this": this.Copy(),
		"kind": "TABLE",
		"properties": exp.Properties(exp.Args{"expressions": []exp.Expression{
			exp.PartitionedByProperty(exp.Args{"this": exp.Anonymous(exp.Args{"this": "HASH", "expressions": []exp.Expression{createColumn("foo")}})}),
		}}),
	})
	if got, want := genSQL(t, dialects.Postgres(), postgres), "CREATE TABLE test (foo INT) PARTITION BY HASH(foo)"; got != want {
		t.Fatalf("postgres got %q, want %q", got, want)
	}
}

func TestCreateRestoredAssemblyArgs(t *testing.T) {
	node := exp.Create(exp.Args{
		"this":              createTable("t"),
		"kind":              "TABLE",
		"refresh":           true,
		"clustered":         false,
		"indexes":           []exp.Expression{exp.Var(exp.Args{"this": "INDEX i"})},
		"no_schema_binding": true,
		"clone":             exp.Var(exp.Args{"this": "CLONE source"}),
	})
	want := "CREATE NONCLUSTERED COLUMNSTORE OR REFRESH TABLE t INDEX i WITH NO SCHEMA BINDING CLONE source"
	if got := genSQL(t, nil, node); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestUnknownCreatePropertyIsUnsupported(t *testing.T) {
	level := sqlerrors.RAISE
	node := exp.Properties(exp.Args{"expressions": []exp.Expression{exp.New(exp.KindExpression, exp.Args{"this": "unknown"})}})
	_, err := generator.New(dialects.Base(), generator.Options{UnsupportedLevel: &level}).Generate(node)
	if err == nil || !strings.Contains(err.Error(), "Unsupported property") {
		t.Fatalf("Generate() error = %v, want unsupported property error", err)
	}
}

func TestDialectCreatePropertyOverrides(t *testing.T) {
	view := exp.Create(exp.Args{
		"this": createTable("v"),
		"kind": "VIEW",
		"properties": exp.Properties(exp.Args{"expressions": []exp.Expression{
			exp.SqlSecurityProperty(exp.Args{"this": "DEFINER"}),
		}}),
		"expression": exp.Select(exp.Args{"expressions": []exp.Expression{exp.LiteralNumber(1)}}),
	})
	if got, want := genSQL(t, dialects.MySQL(), view), "CREATE SQL SECURITY DEFINER VIEW v AS SELECT 1"; got != want {
		t.Fatalf("mysql view got %q, want %q", got, want)
	}

	level := sqlerrors.RAISE
	unsupported := []struct {
		name string
		d    *dialects.Dialect
		node exp.Expression
	}{
		{"mysql partitioned by", dialects.MySQL(), exp.Properties(exp.Args{"expressions": []exp.Expression{
			exp.PartitionedByProperty(exp.Args{"this": exp.Schema(exp.Args{"expressions": []exp.Expression{createColumn("x")}})}),
		}})},
		{"postgres schema comment", dialects.Postgres(), exp.SchemaCommentProperty(exp.Args{"this": exp.LiteralString("comment")})},
	}
	for _, tc := range unsupported {
		t.Run(tc.name, func(t *testing.T) {
			_, err := generator.New(tc.d, generator.Options{UnsupportedLevel: &level}).Generate(tc.node)
			if err == nil {
				t.Fatal("Generate() error = nil, want unsupported error")
			}
		})
	}
}

// These are the CREATE rows assigned to this generator part from the pinned 95-row fidelity
// oracle. Every expected string below is the exact v30.12.0 same-dialect .sql() output.
func TestCreatePropertyFidelityRows(t *testing.T) {
	cases := []struct {
		row     int
		dialect string
		input   string
		want    string
	}{
		{3, "", "CREATE ALGORITHM=UNDEFINED DEFINER=foo@% VIEW a SQL SECURITY DEFINER AS (SELECT a FROM b)", "CREATE ALGORITHM=UNDEFINED DEFINER=foo@% VIEW a SQL SECURITY DEFINER AS (SELECT a FROM b)"},
		{4, "", "CREATE FUNCTION a.b(x TEXT) LANGUAGE SQL READS SQL DATA RETURNS TEXT AS RETURN x", "CREATE FUNCTION a.b(x TEXT) LANGUAGE SQL READS SQL DATA RETURNS TEXT AS RETURN x"},
		{5, "", "CREATE FUNCTION a.b(x TEXT) RETURNS TEXT CONTAINS SQL AS RETURN x", "CREATE FUNCTION a.b(x TEXT) RETURNS TEXT CONTAINS SQL AS RETURN x"},
		{6, "", "CREATE FUNCTION a.b(x TEXT) RETURNS TEXT LANGUAGE SQL MODIFIES SQL DATA AS RETURN x", "CREATE FUNCTION a.b(x TEXT) RETURNS TEXT LANGUAGE SQL MODIFIES SQL DATA AS RETURN x"},
		{7, "", "CREATE MATERIALIZED VIEW x.y.z AS SELECT a FROM b", "CREATE MATERIALIZED VIEW x.y.z AS SELECT a FROM b"},
		{8, "", "CREATE OR REPLACE TEMPORARY VIEW x AS SELECT *", "CREATE OR REPLACE TEMPORARY VIEW x AS SELECT *"},
		{9, "", "CREATE TABLE a (b INT) ON COMMIT PRESERVE ROWS", "CREATE TABLE a (b INT) ON COMMIT PRESERVE ROWS"},
		{10, "", "CREATE TABLE a.b AS (SELECT 1) NO PRIMARY INDEX", "CREATE TABLE a.b AS (SELECT 1) NO PRIMARY INDEX"},
		{11, "", "CREATE TABLE a.b AS (SELECT 1) PRIMARY AMP INDEX index1 (a) UNIQUE INDEX index2 (b)", "CREATE TABLE a.b AS (SELECT 1) PRIMARY AMP INDEX index1 (a) UNIQUE INDEX index2 (b)"},
		{12, "", "CREATE TABLE a.b AS (SELECT 1) UNIQUE PRIMARY INDEX index1 (a) UNIQUE INDEX index2 (b)", "CREATE TABLE a.b AS (SELECT 1) UNIQUE PRIMARY INDEX index1 (a) UNIQUE INDEX index2 (b)"},
		{13, "", "CREATE TABLE a.b AS SELECT 1 WITH DATA AND STATISTICS", "CREATE TABLE a.b AS SELECT 1 WITH DATA AND STATISTICS"},
		{14, "", "CREATE TABLE a.b AS SELECT 1 WITH NO DATA AND NO STATISTICS", "CREATE TABLE a.b AS SELECT 1 WITH NO DATA AND NO STATISTICS"},
		{15, "", "CREATE TABLE asd AS SELECT asd FROM asd WITH DATA", "CREATE TABLE asd AS SELECT asd FROM asd WITH DATA"},
		{16, "", "CREATE TABLE asd AS SELECT asd FROM asd WITH NO DATA", "CREATE TABLE asd AS SELECT asd FROM asd WITH NO DATA"},
		{17, "", "CREATE TABLE db.foo (id INT NOT NULL, valid_date DATE FORMAT 'YYYY-MM-DD', measurement INT COMPRESS)", "CREATE TABLE db.foo (id INT NOT NULL, valid_date DATE FORMAT 'YYYY-MM-DD', measurement INT COMPRESS)"},
		{18, "", "CREATE TABLE foo (baz CHAR(4) CHARACTER SET LATIN UPPERCASE NOT CASESPECIFIC COMPRESS 'a')", "CREATE TABLE foo (baz CHAR(4) CHARACTER SET LATIN UPPERCASE NOT CASESPECIFIC COMPRESS 'a')"},
		{19, "", "CREATE TABLE foo (baz DATE FORMAT 'YYYY/MM/DD' TITLE 'title' INLINE LENGTH 1 COMPRESS ('a', 'b'))", "CREATE TABLE foo (baz DATE FORMAT 'YYYY/MM/DD' TITLE 'title' INLINE LENGTH 1 COMPRESS ('a', 'b'))"},
		{20, "", "CREATE TABLE z (z INT) WITH (PARTITIONED_BY=(x INT)) AS SELECT 1", "CREATE TABLE z (z INT) WITH (PARTITIONED_BY=(x INT)) AS SELECT 1"},
		{21, "", "CREATE TABLE z (z INT) WITH (PARTITIONED_BY=(x INT, y INT))", "CREATE TABLE z (z INT) WITH (PARTITIONED_BY=(x INT, y INT))"},
		{22, "", "CREATE TABLE z WITH (FORMAT='ORC', x='2') AS SELECT 1", "CREATE TABLE z WITH (FORMAT='ORC', x='2') AS SELECT 1"},
		{23, "", "CREATE TABLE z WITH (FORMAT='parquet') AS SELECT 1", "CREATE TABLE z WITH (FORMAT='parquet') AS SELECT 1"},
		{24, "", "CREATE TABLE z WITH (TABLE_FORMAT='iceberg', FORMAT='ORC', x='2') AS SELECT 1", "CREATE TABLE z WITH (TABLE_FORMAT='iceberg', FORMAT='ORC', x='2') AS SELECT 1"},
		{25, "", "CREATE TABLE z WITH (TABLE_FORMAT='iceberg', FORMAT='parquet') AS SELECT 1", "CREATE TABLE z WITH (TABLE_FORMAT='iceberg', FORMAT='parquet') AS SELECT 1"},
		{26, "", "CREATE TEMPORARY FUNCTION f", "CREATE TEMPORARY FUNCTION f"},
		{27, "", "CREATE TEMPORARY FUNCTION f AS 'g'", "CREATE TEMPORARY FUNCTION f AS 'g'"},
		{28, "", "CREATE TEMPORARY TABLE IF NOT EXISTS x AS SELECT a FROM d", "CREATE TEMPORARY TABLE IF NOT EXISTS x AS SELECT a FROM d"},
		{29, "", "CREATE TEMPORARY TABLE x AS SELECT a FROM d", "CREATE TEMPORARY TABLE x AS SELECT a FROM d"},
		{30, "", "CREATE TEMPORARY VIEW IF NOT EXISTS x AS SELECT a FROM d", "CREATE TEMPORARY VIEW IF NOT EXISTS x AS SELECT a FROM d"},
		{31, "", "CREATE TEMPORARY VIEW x AS SELECT a FROM d", "CREATE TEMPORARY VIEW x AS SELECT a FROM d"},
		{32, "", "CREATE TEMPORARY VIEW x AS WITH y AS (SELECT 1) SELECT * FROM y", "CREATE TEMPORARY VIEW x AS WITH y AS (SELECT 1) SELECT * FROM y"},
		{33, "", "CREATE VIEW z AS LOCKING ROW FOR ACCESS SELECT a FROM b", "CREATE VIEW z AS LOCKING ROW FOR ACCESS SELECT a FROM b"},
		{42, "mysql", "CREATE SQL SECURITY DEFINER VIEW id_test (id, foo) AS SELECT 0, foo FROM test", "CREATE SQL SECURITY DEFINER VIEW id_test (id, foo) AS SELECT 0, foo FROM test"},
		{43, "mysql", "CREATE SQL SECURITY INVOKER VIEW id_test (id, foo) AS SELECT 0, foo FROM test", "CREATE SQL SECURITY INVOKER VIEW id_test (id, foo) AS SELECT 0, foo FROM test"},
		{44, "mysql", "CREATE TABLE A LIKE B", "CREATE TABLE A LIKE B"},
		{52, "mysql", "CREATE TABLE z (a INT DEFAULT NULL, PRIMARY KEY (a)) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'", "CREATE TABLE z (a INT DEFAULT NULL, PRIMARY KEY (a)) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'"},
		{53, "mysql", "CREATE TABLE z (a INT) ENGINE=InnoDB AUTO_INCREMENT=1 CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'", "CREATE TABLE z (a INT) ENGINE=InnoDB AUTO_INCREMENT=1 CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'"},
		{54, "mysql", "CREATE TABLE z (a INT) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'", "CREATE TABLE z (a INT) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'"},
		{55, "postgres", "CREATE FUNCTION add(INT, INT) RETURNS INT SET search_path TO 'public' AS 'select $1 + $2;' LANGUAGE SQL IMMUTABLE", "CREATE FUNCTION add(INT, INT) RETURNS INT SET search_path TO 'public' AS 'select $1 + $2;' LANGUAGE SQL IMMUTABLE"},
		{78, "postgres", "CREATE TABLE cities_ab PARTITION OF cities (CONSTRAINT city_id_nonzero CHECK (city_id <> 0)) FOR VALUES IN ('a', 'b')", "CREATE TABLE cities_ab PARTITION OF cities (CONSTRAINT city_id_nonzero CHECK (city_id <> 0)) FOR VALUES IN ('a', 'b')"},
		{79, "postgres", "CREATE TABLE cities_ab PARTITION OF cities (CONSTRAINT city_id_nonzero CHECK (city_id <> 0)) FOR VALUES IN ('a', 'b') PARTITION BY RANGE(population)", "CREATE TABLE cities_ab PARTITION OF cities (CONSTRAINT city_id_nonzero CHECK (city_id <> 0)) FOR VALUES IN ('a', 'b') PARTITION BY RANGE(population)"},
		{80, "postgres", "CREATE TABLE cities_partdef PARTITION OF cities DEFAULT", "CREATE TABLE cities_partdef PARTITION OF cities DEFAULT"},
		{81, "postgres", "CREATE TABLE cust_part3 PARTITION OF customers FOR VALUES WITH (MODULUS 3, REMAINDER 2)", "CREATE TABLE cust_part3 PARTITION OF customers FOR VALUES WITH (MODULUS 3, REMAINDER 2)"},
		{82, "postgres", "CREATE TABLE measurement_y2016m07 PARTITION OF measurement (unitsales DEFAULT 0) FOR VALUES FROM ('2016-07-01') TO ('2016-08-01')", "CREATE TABLE measurement_y2016m07 PARTITION OF measurement (unitsales DEFAULT 0) FOR VALUES FROM ('2016-07-01') TO ('2016-08-01')"},
		{83, "postgres", "CREATE TABLE measurement_ym_older PARTITION OF measurement_year_month FOR VALUES FROM (MINVALUE, MINVALUE) TO (2016, 11)", "CREATE TABLE measurement_ym_older PARTITION OF measurement_year_month FOR VALUES FROM (MINVALUE, MINVALUE) TO (2016, 11)"},
		{84, "postgres", "CREATE TABLE measurement_ym_y2016m11 PARTITION OF measurement_year_month FOR VALUES FROM (2016, 11) TO (2016, 12)", "CREATE TABLE measurement_ym_y2016m11 PARTITION OF measurement_year_month FOR VALUES FROM (2016, 11) TO (2016, 12)"},
		{85, "postgres", "CREATE TABLE s.t (c CHAR(2) UNIQUE NOT NULL) INHERITS (s.t1, s.t2)", "CREATE TABLE s.t (c CHAR(2) UNIQUE NOT NULL) INHERITS (s.t1, s.t2)"},
		{86, "postgres", "CREATE TABLE t (c CHAR(2) UNIQUE NOT NULL) INHERITS (t1)", "CREATE TABLE t (c CHAR(2) UNIQUE NOT NULL) INHERITS (t1)"},
		{87, "postgres", "CREATE TABLE t (i INT, EXCLUDE USING btree(INT4RANGE(vid, nid, '[]') ASC NULLS FIRST WITH &&) INCLUDE (col1, col2))", "CREATE TABLE t (i INT, EXCLUDE USING btree(INT4RANGE(vid, nid, '[]') ASC NULLS FIRST WITH &&) INCLUDE (col1, col2))"},
		{88, "postgres", "CREATE TABLE t (i INT, EXCLUDE USING gin(col1 WITH &&, col2 WITH ||) USING INDEX TABLESPACE tablespace WHERE (id > 5))", "CREATE TABLE t (i INT, EXCLUDE USING gin(col1 WITH &&, col2 WITH ||) USING INDEX TABLESPACE tablespace WHERE (id > 5))"},
		{89, "postgres", "CREATE TABLE t (i INT, PRIMARY KEY (i), EXCLUDE USING gist(col varchar_pattern_ops DESC NULLS LAST WITH &&) WITH (sp1=1, sp2=2))", "CREATE TABLE t (i INT, PRIMARY KEY (i), EXCLUDE USING gist(col varchar_pattern_ops DESC NULLS LAST WITH &&) WITH (sp1=1, sp2=2))"},
		{90, "postgres", "CREATE TABLE t (vid INT NOT NULL, CONSTRAINT ht_vid_nid_fid_idx EXCLUDE (INT4RANGE(vid, nid) WITH &&, INT4RANGE(fid, fid, '[]') WITH &&))", "CREATE TABLE t (vid INT NOT NULL, CONSTRAINT ht_vid_nid_fid_idx EXCLUDE (INT4RANGE(vid, nid) WITH &&, INT4RANGE(fid, fid, '[]') WITH &&))"},
		{91, "postgres", "CREATE TABLE test (foo INT) PARTITION BY HASH(foo)", "CREATE TABLE test (foo INT) PARTITION BY HASH(foo)"},
		{92, "postgres", "CREATE TYPE inventory_item AS (name TEXT, supplier_id INT, price DECIMAL)", "CREATE TYPE inventory_item AS (name TEXT, supplier_id INT, price DECIMAL)"},
		{93, "postgres", "CREATE TYPE mood AS ENUM ('sad', 'ok', 'happy')", "CREATE TYPE mood AS ENUM ('sad', 'ok', 'happy')"},
		{94, "postgres", "CREATE TYPE mood AS ENUM ()", "CREATE TYPE mood AS ENUM ()"},
		{95, "postgres", "CREATE TYPE public.mood AS ENUM ('sad', 'ok')", "CREATE TYPE public.mood AS ENUM ('sad', 'ok')"},
		{96, "postgres", "CREATE UNLOGGED TABLE foo AS WITH t(c) AS (SELECT 1) SELECT * FROM (SELECT c AS c FROM t) AS temp", "CREATE UNLOGGED TABLE foo AS WITH t(c) AS (SELECT 1) SELECT * FROM (SELECT c AS c FROM t) AS temp"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("row_%d_%s", tc.row, tc.dialect), func(t *testing.T) {
			if got := roundTrip(t, tc.dialect, tc.input); got != tc.want {
				t.Fatalf("row %d generated\n  got:  %q\n  want: %q", tc.row, got, tc.want)
			}
		})
	}
}
