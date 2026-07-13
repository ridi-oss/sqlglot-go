package parser_test

import (
	"strings"
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func createProperties(t *testing.T, create exp.Expression) []exp.Expression {
	t.Helper()
	if create.Kind() != exp.KindCreate {
		t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
	}
	properties, _ := create.Arg("properties").(exp.Expression)
	if properties == nil {
		return nil
	}
	return properties.Expressions()
}

func assertToSContains(t *testing.T, expression exp.Expression, fragments ...string) {
	t.Helper()
	toS := expression.ToS()
	for _, fragment := range fragments {
		if !strings.Contains(toS, fragment) {
			t.Fatalf("ToS missing %q:\n%s", fragment, toS)
		}
	}
}

func TestCreatePropertyOrderingAndLocations(t *testing.T) {
	create := parseOne(t, "CREATE ALGORITHM=UNDEFINED DEFINER=foo@% VIEW a SQL SECURITY DEFINER AS (SELECT a FROM b)")
	properties := createProperties(t, create)
	wantKinds := []exp.Kind{exp.KindAlgorithmProperty, exp.KindDefinerProperty, exp.KindSqlSecurityProperty}
	if len(properties) != len(wantKinds) {
		t.Fatalf("property count = %d, want %d:\n%s", len(properties), len(wantKinds), create.ToS())
	}
	for i, want := range wantKinds {
		if properties[i].Kind() != want {
			t.Fatalf("property %d kind = %v, want %v:\n%s", i, properties[i].Kind(), want, create.ToS())
		}
	}
	toS := create.ToS()
	algorithm := strings.Index(toS, "AlgorithmProperty(")
	definer := strings.Index(toS, "DefinerProperty(")
	security := strings.Index(toS, "SqlSecurityProperty(")
	if algorithm < 0 || definer <= algorithm || security <= definer {
		t.Fatalf("property ToS order mismatch:\n%s", toS)
	}

	quotedDefiner := parseOneDialect(t, "CREATE DEFINER='admin'@'localhost' VIEW v AS SELECT 1", "mysql")
	quotedProperties := createProperties(t, quotedDefiner)
	if len(quotedProperties) != 1 || quotedProperties[0].Kind() != exp.KindDefinerProperty || quotedProperties[0].Arg("this") != `"admin"@"localhost"` {
		t.Fatalf("quoted DEFINER mismatch:\n%s", quotedDefiner.ToS())
	}

	cases := []struct {
		sql      string
		kind     exp.Kind
		fragment string
	}{
		{"CREATE TEMPORARY TABLE x AS SELECT 1", exp.KindTemporaryProperty, "TemporaryProperty("},
		{"CREATE MATERIALIZED VIEW x AS SELECT 1", exp.KindMaterializedProperty, "MaterializedProperty("},
		{"CREATE UNLOGGED TABLE x AS SELECT 1", exp.KindUnloggedProperty, "UnloggedProperty("},
		{"CREATE TABLE x (a INT) ENGINE=InnoDB", exp.KindEngineProperty, "EngineProperty("},
		{"CREATE VIEW z AS LOCKING ROW FOR ACCESS SELECT a FROM b", exp.KindLockingProperty, "for_or_in=FOR"},
		{"CREATE TABLE a (b INT) ON COMMIT PRESERVE ROWS", exp.KindOnCommitProperty, "OnCommitProperty("},
		{"CREATE TABLE a (b INT) ON COMMIT DELETE ROWS", exp.KindOnCommitProperty, "delete=True"},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			root := parseOne(t, tc.sql)
			properties := createProperties(t, root)
			if len(properties) != 1 || properties[0].Kind() != tc.kind {
				t.Fatalf("property mismatch:\n%s", root.ToS())
			}
			assertToSContains(t, root, tc.fragment)
		})
	}
}

func TestCreateWithGenericProperties(t *testing.T) {
	create := parseOne(t, "CREATE TABLE z WITH (FORMAT='ORC', x='2') AS SELECT 1")
	properties := createProperties(t, create)
	if len(properties) != 2 || properties[0].Kind() != exp.KindFileFormatProperty || properties[1].Kind() != exp.KindProperty {
		t.Fatalf("WITH properties mismatch:\n%s", create.ToS())
	}
	if properties[0].Name() != "ORC" || properties[1].Name() != "x" {
		t.Fatalf("WITH property values mismatch:\n%s", create.ToS())
	}
	if properties[0].Parent() == nil || properties[0].Parent().Kind() != exp.KindProperties || properties[0].Parent().Parent() != create {
		t.Fatalf("WITH properties must be flattened under Create.Properties:\n%s", create.ToS())
	}
	assertToSContains(t, create, "FileFormatProperty(", "Property(", "value=Literal(this='2'")

	partitioned := parseOne(t, "CREATE TABLE z (z INT) WITH (PARTITIONED_BY=(x INT, y INT))")
	properties = createProperties(t, partitioned)
	if len(properties) != 1 || properties[0].Kind() != exp.KindPartitionedByProperty {
		t.Fatalf("PARTITIONED_BY mismatch:\n%s", partitioned.ToS())
	}
	partitionSchema := exprArg(t, properties[0], "this")
	if partitionSchema.Kind() != exp.KindSchema || len(partitionSchema.Expressions()) != 2 {
		t.Fatalf("PARTITIONED_BY schema mismatch:\n%s", partitioned.ToS())
	}
}

func TestCreateFunctionReadWriteProperties(t *testing.T) {
	cases := []struct {
		sql  string
		want string
	}{
		{"CREATE FUNCTION a.b(x TEXT) RETURNS TEXT CONTAINS SQL AS RETURN x", "CONTAINS SQL"},
		{"CREATE FUNCTION a.b(x TEXT) RETURNS TEXT LANGUAGE SQL MODIFIES SQL DATA AS RETURN x", "MODIFIES SQL DATA"},
		{"CREATE FUNCTION a.b(x TEXT) LANGUAGE SQL READS SQL DATA RETURNS TEXT AS RETURN x", "READS SQL DATA"},
		{"CREATE FUNCTION a.b(x TEXT) RETURNS TEXT NO SQL AS RETURN x", "NO SQL"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			create := parseOne(t, tc.sql)
			properties := createProperties(t, create)
			found := false
			for _, property := range properties {
				if property.Kind() == exp.KindSqlReadWriteProperty && property.Name() == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("read/write property %q missing:\n%s", tc.want, create.ToS())
			}
			assertToSContains(t, create, "SqlReadWriteProperty(this="+tc.want)
		})
	}
}

func TestCreatePostExpressionPropertiesAndIndexes(t *testing.T) {
	cases := []struct {
		sql        string
		kind       exp.Kind
		no         any
		statistics any
	}{
		{"CREATE TABLE a.b AS SELECT 1 WITH DATA AND STATISTICS", exp.KindWithDataProperty, false, true},
		{"CREATE TABLE a.b AS SELECT 1 WITH NO DATA AND NO STATISTICS", exp.KindWithDataProperty, true, false},
		{"CREATE TABLE a.b AS (SELECT 1) NO PRIMARY INDEX", exp.KindNoPrimaryIndexProperty, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			create := parseOne(t, tc.sql)
			properties := createProperties(t, create)
			if len(properties) != 1 || properties[0].Kind() != tc.kind {
				t.Fatalf("post-expression property mismatch:\n%s", create.ToS())
			}
			if tc.kind == exp.KindWithDataProperty && (properties[0].Arg("no") != tc.no || properties[0].Arg("statistics") != tc.statistics) {
				t.Fatalf("WITH DATA flags mismatch:\n%s", create.ToS())
			}
		})
	}

	create := parseOne(t, "CREATE TABLE a.b AS (SELECT 1) UNIQUE PRIMARY INDEX index1 (a) UNIQUE INDEX index2 (b)")
	indexes := expressionsForArg(create, "indexes")
	if len(indexes) != 2 || indexes[0].Kind() != exp.KindIndex || indexes[1].Kind() != exp.KindIndex {
		t.Fatalf("bare indexes mismatch:\n%s", create.ToS())
	}
	if indexes[0].Arg("unique") != true || indexes[0].Arg("primary") != true || indexes[0].Name() != "index1" {
		t.Fatalf("first bare index flags mismatch:\n%s", create.ToS())
	}
	if indexes[1].Arg("unique") != true || indexes[1].Arg("primary") != false || indexes[1].Name() != "index2" {
		t.Fatalf("second bare index flags mismatch:\n%s", create.ToS())
	}
	assertToSContains(t, create, "indexes=[", "unique=True", "primary=True")

	ampCreate := parseOne(t, "CREATE TABLE a.b AS (SELECT 1) PRIMARY AMP INDEX index1 (a)")
	ampIndexes := expressionsForArg(ampCreate, "indexes")
	if len(ampIndexes) != 1 || ampIndexes[0].Arg("primary") != true || ampIndexes[0].Arg("amp") != true {
		t.Fatalf("AMP index flags mismatch:\n%s", ampCreate.ToS())
	}
	assertToSContains(t, ampCreate, "primary=True", "amp=True")
}

func TestCreateLikeInheritsAndPartitionOf(t *testing.T) {
	like := parseOneDialect(t, "CREATE TABLE A LIKE B INCLUDING DEFAULTS EXCLUDING CONSTRAINTS", "postgres")
	properties := createProperties(t, like)
	if len(properties) != 1 || properties[0].Kind() != exp.KindLikeProperty || len(properties[0].Expressions()) != 2 {
		t.Fatalf("CREATE LIKE mismatch:\n%s", like.ToS())
	}
	assertToSContains(t, like, "LikeProperty(", "this=INCLUDING", "this=EXCLUDING")

	inherits := parseOneDialect(t, "CREATE TABLE t (c INT) INHERITS (t1, s.t2)", "postgres")
	properties = createProperties(t, inherits)
	if len(properties) != 1 || properties[0].Kind() != exp.KindInheritsProperty || len(properties[0].Expressions()) != 2 {
		t.Fatalf("INHERITS mismatch:\n%s", inherits.ToS())
	}

	partition := parseOneDialect(t, "CREATE TABLE t PARTITION OF measurement FOR VALUES FROM (MINVALUE) TO (MAXVALUE)", "postgres")
	properties = createProperties(t, partition)
	if len(properties) != 1 || properties[0].Kind() != exp.KindPartitionedOfProperty {
		t.Fatalf("PARTITION OF mismatch:\n%s", partition.ToS())
	}
	bound := exprArg(t, properties[0], "expression")
	if bound.Kind() != exp.KindPartitionBoundSpec || len(expressionsForArg(bound, "from_expressions")) != 1 || len(expressionsForArg(bound, "to_expressions")) != 1 {
		t.Fatalf("partition bound mismatch:\n%s", partition.ToS())
	}

	partitionedBy := parseOneDialect(t, "CREATE TABLE test (foo INT) PARTITION BY HASH(foo)", "postgres")
	properties = createProperties(t, partitionedBy)
	if len(properties) != 1 || properties[0].Kind() != exp.KindPartitionedByProperty || exprArg(t, properties[0], "this").Kind() != exp.KindAnonymous {
		t.Fatalf("PARTITION BY mismatch:\n%s", partitionedBy.ToS())
	}
	assertToSContains(t, partitionedBy, "PartitionedByProperty(", "this=HASH")
}

func TestCreateType(t *testing.T) {
	for _, tc := range []struct {
		sql        string
		valueCount int
	}{
		{"CREATE TYPE mood AS ENUM ('sad', 'ok', 'happy')", 3},
		{"CREATE TYPE mood AS ENUM ()", 0},
	} {
		create := parseOneDialect(t, tc.sql, "postgres")
		if create.Kind() != exp.KindCreate || create.Arg("kind") != "TYPE" {
			t.Fatalf("CREATE TYPE ENUM mismatch:\n%s", create.ToS())
		}
		dtype := exprArg(t, create, "expression")
		if dtype.Kind() != exp.KindDataType || dtype.Arg("this") != exp.DTypeEnum {
			t.Fatalf("ENUM type mismatch:\n%s", create.ToS())
		}
		values, ok := dtype.Arg("expressions").([]exp.Expression)
		if !ok || values == nil || len(values) != tc.valueCount {
			t.Fatalf("ENUM values mismatch:\n%s", create.ToS())
		}
		assertToSContains(t, create, "DataType(", "this=DType.ENUM")
	}

	composite := parseOneDialect(t, "CREATE TYPE inventory_item AS (name TEXT, supplier_id INT, price DECIMAL)", "postgres")
	schema := exprArg(t, composite, "expression")
	if schema.Kind() != exp.KindSchema || len(schema.Expressions()) != 3 {
		t.Fatalf("composite CREATE TYPE mismatch:\n%s", composite.ToS())
	}

	unsupported := parseOneDialect(t, "CREATE TYPE widget", "postgres")
	if unsupported.Kind() != exp.KindCommand {
		t.Fatalf("CREATE TYPE without AS must stay Command:\n%s", unsupported.ToS())
	}
}
