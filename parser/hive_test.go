package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func parseHiveCreate(t *testing.T, sql string) exp.Expression {
	t.Helper()
	create := parseOneDialect(t, sql, "hive")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("Hive DDL kind = %v, want Create (never Command):\n%s", create.Kind(), create.ToS())
	}
	return create
}

func hiveProjection(t *testing.T, root exp.Expression) exp.Expression {
	t.Helper()
	projections := root.Expressions()
	if len(projections) != 1 {
		t.Fatalf("want one projection, got %d:\n%s", len(projections), root.ToS())
	}
	return projections[0]
}

func assertHivePropertyKinds(t *testing.T, create exp.Expression, want ...exp.Kind) []exp.Expression {
	t.Helper()
	properties := createProperties(t, create)
	if len(properties) != len(want) {
		t.Fatalf("property count = %d, want %d (%v):\n%s", len(properties), len(want), want, create.ToS())
	}
	for i, kind := range want {
		if properties[i].Kind() != kind {
			t.Fatalf("property %d kind = %v, want %v:\n%s", i, properties[i].Kind(), kind, create.ToS())
		}
	}
	return properties
}

func hiveLiteralArg(t *testing.T, expression exp.Expression, key, want string) exp.Expression {
	t.Helper()
	literal := exprArg(t, expression, key)
	if literal.Kind() != exp.KindLiteral || literal.Name() != want {
		t.Fatalf("%v.%s = %v(%q), want Literal(%q):\n%s", expression.Kind(), key, literal.Kind(), literal.Name(), want, expression.ToS())
	}
	return literal
}

func hiveColumnDType(t *testing.T, column exp.Expression) exp.Expression {
	t.Helper()
	if column.Kind() != exp.KindColumnDef {
		t.Fatalf("kind = %v, want ColumnDef:\n%s", column.Kind(), column.ToS())
	}
	dtype := exprArg(t, column, "kind")
	if dtype.Kind() != exp.KindDataType {
		t.Fatalf("ColumnDef.kind = %v, want DataType:\n%s", dtype.Kind(), column.ToS())
	}
	return dtype
}

func TestHiveCreatePartitionedBy(t *testing.T) {
	create := parseHiveCreate(t, "CREATE TABLE x (w STRING) PARTITIONED BY (y INT, z INT)")
	properties := assertHivePropertyKinds(t, create, exp.KindPartitionedByProperty)
	partitionSchema := exprArg(t, properties[0], "this")
	if partitionSchema.Kind() != exp.KindSchema || len(partitionSchema.Expressions()) != 2 {
		t.Fatalf("PARTITIONED BY should contain a two-column Schema:\n%s", create.ToS())
	}
	for i, wantName := range []string{"y", "z"} {
		column := partitionSchema.Expressions()[i]
		if column.This() == nil || column.This().Name() != wantName || hiveColumnDType(t, column).Arg("this") != exp.DTypeInt {
			t.Fatalf("partition column %d mismatch:\n%s", i, create.ToS())
		}
	}

	tableSchema := create.This()
	if tableSchema == nil || tableSchema.Kind() != exp.KindSchema || len(tableSchema.Expressions()) != 1 {
		t.Fatalf("CREATE target should contain the table schema:\n%s", create.ToS())
	}
	column := tableSchema.Expressions()[0]
	if column.This() == nil || column.This().Name() != "w" || hiveColumnDType(t, column).Arg("this") != exp.DTypeText {
		t.Fatalf("Hive STRING schema column should parse as TEXT:\n%s", create.ToS())
	}
}

func TestHiveCreateStoredAsAndTableProperties(t *testing.T) {
	create := parseHiveCreate(t, "CREATE TABLE test STORED AS parquet TBLPROPERTIES ('x'='1', 'Z'='2') AS SELECT 1")
	properties := assertHivePropertyKinds(t, create, exp.KindFileFormatProperty, exp.KindProperty, exp.KindProperty)
	format := properties[0]
	if format.Arg("hive_format") != true || format.This() == nil || format.This().Kind() != exp.KindVar || format.This().Name() != "parquet" {
		t.Fatalf("STORED AS parquet FileFormatProperty mismatch:\n%s", create.ToS())
	}
	for i, want := range []struct{ key, value string }{{"x", "1"}, {"Z", "2"}} {
		property := properties[i+1]
		if property.This() == nil || property.This().Kind() != exp.KindLiteral || property.This().Name() != want.key {
			t.Fatalf("TBLPROPERTIES property %d key mismatch:\n%s", i, create.ToS())
		}
		if value := exprArg(t, property, "value"); value.Kind() != exp.KindLiteral || value.Name() != want.value {
			t.Fatalf("TBLPROPERTIES property %d value mismatch:\n%s", i, create.ToS())
		}
	}
	if expression := exprArg(t, create, "expression"); expression.Kind() != exp.KindSelect {
		t.Fatalf("CTAS expression should be Select:\n%s", create.ToS())
	}
}

func TestHiveCreateStoredAsInputOutputFormat(t *testing.T) {
	create := parseHiveCreate(t, "CREATE TABLE test STORED AS INPUTFORMAT 'foo1' OUTPUTFORMAT 'foo2'")
	properties := assertHivePropertyKinds(t, create, exp.KindFileFormatProperty)
	format := properties[0]
	if format.Arg("hive_format") != true || format.This() == nil || format.This().Kind() != exp.KindInputOutputFormat {
		t.Fatalf("STORED AS INPUTFORMAT should use a Hive InputOutputFormat:\n%s", create.ToS())
	}
	io := format.This()
	hiveLiteralArg(t, io, "input_format", "foo1")
	hiveLiteralArg(t, io, "output_format", "foo2")
}

func TestHiveCreateExternalWithSerdeProperties(t *testing.T) {
	create := parseHiveCreate(t, "CREATE EXTERNAL TABLE x (y INT) ROW FORMAT SERDE 'serde' ROW FORMAT DELIMITED FIELDS TERMINATED BY '1' WITH SERDEPROPERTIES ('input.regex'='')")
	properties := assertHivePropertyKinds(t, create,
		exp.KindExternalProperty,
		exp.KindRowFormatSerdeProperty,
		exp.KindRowFormatDelimitedProperty,
		exp.KindSerdeProperties,
	)
	hiveLiteralArg(t, properties[1], "this", "serde")
	hiveLiteralArg(t, properties[2], "fields", "1")

	serde := properties[3]
	if serde.Arg("with_") != true || len(serde.Expressions()) != 1 || serde.Expressions()[0].Kind() != exp.KindProperty {
		t.Fatalf("WITH SERDEPROPERTIES should contain one real Property and with_=true:\n%s", create.ToS())
	}
	property := serde.Expressions()[0]
	if property.This() == nil || property.This().Kind() != exp.KindLiteral || property.This().Name() != "input.regex" {
		t.Fatalf("SERDEPROPERTIES key mismatch:\n%s", create.ToS())
	}
	if value := exprArg(t, property, "value"); value.Kind() != exp.KindLiteral || value.Name() != "" {
		t.Fatalf("SERDEPROPERTIES value mismatch:\n%s", create.ToS())
	}
}

func TestHiveDDLPropertyRegistrationIsIsolated(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{name: "external", sql: "CREATE EXTERNAL TABLE t (a INT)"},
		{name: "location", sql: "CREATE TABLE t (a INT) LOCATION 'x'"},
		{name: "clustered by", sql: "CREATE TABLE t (a INT) CLUSTERED BY (a) INTO 8 BUCKETS"},
		{name: "row format serde", sql: "CREATE TABLE t (a INT) ROW FORMAT SERDE 'x'"},
		{name: "stored as", sql: "CREATE TABLE t (a INT) STORED AS PARQUET"},
		{name: "stored as input output format", sql: "CREATE TABLE t (a INT) STORED AS INPUTFORMAT 'i' OUTPUTFORMAT 'o'"},
		{name: "stored by", sql: "CREATE TABLE t (a INT) STORED BY 'handler'"},
		{name: "table properties", sql: "CREATE TABLE t (a INT) TBLPROPERTIES ('x'='1')"},
		{name: "using file format", sql: "CREATE TABLE t (a INT) USING PARQUET"},
		{name: "with serde properties", sql: "CREATE TABLE t (a INT) WITH SERDEPROPERTIES ('x'='1')"},
		{name: "serde properties", sql: "CREATE TABLE t (a INT) ROW FORMAT SERDE 'x' SERDEPROPERTIES ('k'='v')"},
	}
	dialects := []struct {
		name    string
		dialect string
	}{
		{name: "base"},
		{name: "mysql", dialect: "mysql"},
		{name: "postgres", dialect: "postgres"},
		{name: "presto", dialect: "presto"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hive := parseOneDialect(t, tc.sql, "hive")
			if hive.Kind() != exp.KindCreate {
				t.Fatalf("Hive DDL kind = %v, want Create (never Command):\n%s", hive.Kind(), hive.ToS())
			}

			for _, dialect := range dialects {
				t.Run(dialect.name, func(t *testing.T) {
					command := parseOneDialect(t, tc.sql, dialect.dialect)
					if command.Kind() != exp.KindCommand {
						t.Fatalf("%s DDL kind = %v, want fail-closed Command:\n%s", dialect.name, command.Kind(), command.ToS())
					}
					got, err := generateSQL(t, command, dialect.dialect)
					if err != nil {
						t.Fatalf("generate %s Command: %v", dialect.name, err)
					}
					if got != tc.sql {
						t.Fatalf("generate %s Command = %q, want byte-identical %q", dialect.name, got, tc.sql)
					}
				})
			}
		})
	}
}

func TestHiveCreateExternalFullFormat(t *testing.T) {
	create := parseHiveCreate(t, "CREATE EXTERNAL TABLE `my_table` (`a7` ARRAY<DATE>) ROW FORMAT SERDE 'a' STORED AS INPUTFORMAT 'b' OUTPUTFORMAT 'c' LOCATION 'd' TBLPROPERTIES ('e'='f')")
	properties := assertHivePropertyKinds(t, create,
		exp.KindExternalProperty,
		exp.KindRowFormatSerdeProperty,
		exp.KindFileFormatProperty,
		exp.KindLocationProperty,
		exp.KindProperty,
	)
	hiveLiteralArg(t, properties[1], "this", "a")

	format := properties[2]
	if format.Arg("hive_format") != true || format.This() == nil || format.This().Kind() != exp.KindInputOutputFormat {
		t.Fatalf("external table file format mismatch:\n%s", create.ToS())
	}
	hiveLiteralArg(t, format.This(), "input_format", "b")
	hiveLiteralArg(t, format.This(), "output_format", "c")
	hiveLiteralArg(t, properties[3], "this", "d")

	property := properties[4]
	if property.This() == nil || property.This().Kind() != exp.KindLiteral || property.This().Name() != "e" {
		t.Fatalf("TBLPROPERTIES key mismatch:\n%s", create.ToS())
	}
	if value := exprArg(t, property, "value"); value.Kind() != exp.KindLiteral || value.Name() != "f" {
		t.Fatalf("TBLPROPERTIES value mismatch:\n%s", create.ToS())
	}

	schema := create.This()
	if schema == nil || schema.Kind() != exp.KindSchema || schema.This() == nil || schema.This().Kind() != exp.KindTable {
		t.Fatalf("external table target schema mismatch:\n%s", create.ToS())
	}
	tableIdentifier := schema.This().This()
	if tableIdentifier == nil || tableIdentifier.Name() != "my_table" || tableIdentifier.Arg("quoted") != true {
		t.Fatalf("backtick table identifier mismatch:\n%s", create.ToS())
	}
	column := schema.Expressions()[0]
	if column.This() == nil || column.This().Name() != "a7" || column.This().Arg("quoted") != true {
		t.Fatalf("backtick column identifier mismatch:\n%s", create.ToS())
	}
	array := hiveColumnDType(t, column)
	if array.Arg("this") != exp.DTypeArray || array.Arg("nested") != true || len(array.Expressions()) != 1 || array.Expressions()[0].Arg("this") != exp.DTypeDate {
		t.Fatalf("ARRAY<DATE> type mismatch:\n%s", create.ToS())
	}
}

func TestHiveCreateStoredBy(t *testing.T) {
	create := parseHiveCreate(t, "CREATE EXTERNAL TABLE X (y INT) STORED BY 'x'")
	properties := assertHivePropertyKinds(t, create, exp.KindExternalProperty, exp.KindStorageHandlerProperty)
	hiveLiteralArg(t, properties[1], "this", "x")
}

func TestHiveCreateClusteredBy(t *testing.T) {
	create := parseHiveCreate(t, "CREATE TABLE x (a INT) CLUSTERED BY (a) SORTED BY (a DESC) INTO 8 BUCKETS")
	properties := assertHivePropertyKinds(t, create, exp.KindClusteredByProperty)
	clustered := properties[0]
	if columns := clustered.Expressions(); len(columns) != 1 || columns[0].Name() != "a" {
		t.Fatalf("CLUSTERED BY columns mismatch:\n%s", create.ToS())
	}
	sorted := expressionsForArg(clustered, "sorted_by")
	if len(sorted) != 1 {
		t.Fatalf("SORTED BY expression count = %d, want 1:\n%s", len(sorted), create.ToS())
	}
	ordered := sorted[0]
	if ordered.Kind() != exp.KindOrdered || ordered.This() == nil || ordered.This().Name() != "a" || ordered.Arg("desc") != true {
		t.Fatalf("SORTED BY expression mismatch:\n%s", create.ToS())
	}
	buckets := exprArg(t, clustered, "buckets")
	if buckets.Kind() != exp.KindLiteral || buckets.Name() != "8" {
		t.Fatalf("bucket count mismatch:\n%s", create.ToS())
	}
}

func TestHiveNestedSchemaTypes(t *testing.T) {
	varcharCreate := parseHiveCreate(t, "CREATE TABLE foo (col STRUCT<struct_col_a: VARCHAR((50))>)")
	outerColumn := varcharCreate.This().Expressions()[0]
	structType := hiveColumnDType(t, outerColumn)
	if structType.Arg("this") != exp.DTypeStruct || structType.Arg("nested") != true || len(structType.Expressions()) != 1 {
		t.Fatalf("STRUCT type mismatch:\n%s", varcharCreate.ToS())
	}
	field := structType.Expressions()[0]
	varchar := hiveColumnDType(t, field)
	if field.This() == nil || field.This().Name() != "struct_col_a" || varchar.Arg("this") != exp.DTypeVarchar || len(varchar.Expressions()) != 1 {
		t.Fatalf("VARCHAR((50)) must remain VARCHAR inside a schema:\n%s", varcharCreate.ToS())
	}
	param := varchar.Expressions()[0]
	if param.Kind() != exp.KindDataTypeParam || param.This() == nil || param.This().Kind() != exp.KindParen || param.This().This() == nil || param.This().This().Kind() != exp.KindLiteral || param.This().This().Name() != "50" {
		t.Fatalf("VARCHAR((50)) nested size mismatch:\n%s", varcharCreate.ToS())
	}

	nestedCreate := parseHiveCreate(t, "CREATE TABLE db.example_table (col_a STRUCT<struct_col_a: INT, struct_col_b: STRUCT<nested_col_a: STRING, nested_col_b: STRING>>)")
	outerStruct := hiveColumnDType(t, nestedCreate.This().Expressions()[0])
	if outerStruct.Arg("this") != exp.DTypeStruct || len(outerStruct.Expressions()) != 2 {
		t.Fatalf("outer nested STRUCT mismatch:\n%s", nestedCreate.ToS())
	}
	innerStruct := hiveColumnDType(t, outerStruct.Expressions()[1])
	if innerStruct.Arg("this") != exp.DTypeStruct || len(innerStruct.Expressions()) != 2 {
		t.Fatalf("inner nested STRUCT mismatch:\n%s", nestedCreate.ToS())
	}
	for i, field := range innerStruct.Expressions() {
		if hiveColumnDType(t, field).Arg("this") != exp.DTypeText {
			t.Fatalf("nested STRING field %d should be TEXT:\n%s", i, nestedCreate.ToS())
		}
	}

	collectionCreate := parseHiveCreate(t, "CREATE TABLE nested_types (c ARRAY<MAP<STRING, STRUCT<x: INT, y: ARRAY<BIGINT>>>>)")
	array := hiveColumnDType(t, collectionCreate.This().Expressions()[0])
	if array.Arg("this") != exp.DTypeArray || array.Arg("nested") != true || len(array.Expressions()) != 1 {
		t.Fatalf("outer ARRAY mismatch:\n%s", collectionCreate.ToS())
	}
	mapType := array.Expressions()[0]
	if mapType.Kind() != exp.KindDataType || mapType.Arg("this") != exp.DTypeMap || mapType.Arg("nested") != true || len(mapType.Expressions()) != 2 {
		t.Fatalf("nested MAP mismatch:\n%s", collectionCreate.ToS())
	}
	if mapType.Expressions()[0].Arg("this") != exp.DTypeText {
		t.Fatalf("MAP key should be STRING/TEXT:\n%s", collectionCreate.ToS())
	}
	mapValue := mapType.Expressions()[1]
	if mapValue.Arg("this") != exp.DTypeStruct || len(mapValue.Expressions()) != 2 {
		t.Fatalf("MAP value should be STRUCT:\n%s", collectionCreate.ToS())
	}
	nestedArray := hiveColumnDType(t, mapValue.Expressions()[1])
	if nestedArray.Arg("this") != exp.DTypeArray || len(nestedArray.Expressions()) != 1 || nestedArray.Expressions()[0].Arg("this") != exp.DTypeBigInt {
		t.Fatalf("STRUCT.y should be ARRAY<BIGINT>:\n%s", collectionCreate.ToS())
	}
}

func TestHiveCreateUsingFileFormat(t *testing.T) {
	create := parseHiveCreate(t, "CREATE TABLE t (a INT) USING PARQUET")
	properties := assertHivePropertyKinds(t, create, exp.KindFileFormatProperty)
	format := properties[0].This()
	if format == nil || format.Kind() != exp.KindVar || format.Name() != "PARQUET" {
		t.Fatalf("USING PARQUET should contain a PARQUET Var:\n%s", create.ToS())
	}
}

func TestHiveCreateFunctionUsingResources(t *testing.T) {
	cases := []struct {
		name, sql, functionClass, resourceKind, resourcePath string
		replace, temporary                                   bool
	}{
		{
			name:          "jar",
			sql:           "CREATE FUNCTION my_func AS 'com.example.MyFunc' USING JAR 'hdfs://path/to/my.jar'",
			functionClass: "com.example.MyFunc",
			resourceKind:  "JAR",
			resourcePath:  "hdfs://path/to/my.jar",
		},
		{
			name:          "replace temporary jar",
			sql:           "CREATE OR REPLACE TEMPORARY FUNCTION some_func AS 'my_jar.SomeFunctionUDF' USING JAR 's3://bucket/my.jar'",
			functionClass: "my_jar.SomeFunctionUDF",
			resourceKind:  "JAR",
			resourcePath:  "s3://bucket/my.jar",
			replace:       true,
			temporary:     true,
		},
		{
			name:          "file",
			sql:           "CREATE FUNCTION my_func AS 'com.example.MyFunc' USING FILE 'hdfs://path/to/file.py'",
			functionClass: "com.example.MyFunc",
			resourceKind:  "FILE",
			resourcePath:  "hdfs://path/to/file.py",
		},
		{
			name:          "archive",
			sql:           "CREATE FUNCTION my_func AS 'com.example.MyFunc' USING ARCHIVE 'hdfs://path/to/archive.zip'",
			functionClass: "com.example.MyFunc",
			resourceKind:  "ARCHIVE",
			resourcePath:  "hdfs://path/to/archive.zip",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			create := parseHiveCreate(t, tc.sql)
			if create.Arg("kind") != "FUNCTION" || create.Arg("replace") != tc.replace {
				t.Fatalf("CREATE FUNCTION flags mismatch:\n%s", create.ToS())
			}
			hiveLiteralArg(t, create, "expression", tc.functionClass)
			wantKinds := []exp.Kind{exp.KindUsingProperty}
			if tc.temporary {
				wantKinds = []exp.Kind{exp.KindTemporaryProperty, exp.KindUsingProperty}
			}
			properties := assertHivePropertyKinds(t, create, wantKinds...)
			using := properties[len(properties)-1]
			if using.Arg("kind") != tc.resourceKind {
				t.Fatalf("UsingProperty.kind = %v, want %q:\n%s", using.Arg("kind"), tc.resourceKind, create.ToS())
			}
			hiveLiteralArg(t, using, "this", tc.resourcePath)
		})
	}
}

func TestHivePercentileDistinctAndAll(t *testing.T) {
	distinct := parseOneDialect(t, "PERCENTILE(DISTINCT x, 0.5)", "hive")
	if distinct.Kind() != exp.KindQuantile || distinct.This() == nil || distinct.This().Kind() != exp.KindDistinct {
		t.Fatalf("PERCENTILE(DISTINCT ...) should be Quantile(Distinct):\n%s", distinct.ToS())
	}
	if values := distinct.This().Expressions(); len(values) != 1 || values[0].Name() != "x" {
		t.Fatalf("PERCENTILE DISTINCT argument mismatch:\n%s", distinct.ToS())
	}
	if quantile := exprArg(t, distinct, "quantile"); quantile.Kind() != exp.KindLiteral || quantile.Name() != "0.5" {
		t.Fatalf("PERCENTILE DISTINCT quantile mismatch:\n%s", distinct.ToS())
	}

	all := parseOneDialect(t, "PERCENTILE(ALL x, 0.5)", "hive")
	if all.Kind() != exp.KindQuantile || all.This() == nil || all.This().Kind() != exp.KindColumn || all.This().Name() != "x" {
		t.Fatalf("PERCENTILE(ALL ...) should discard ALL and keep the column:\n%s", all.ToS())
	}
	if quantile := exprArg(t, all, "quantile"); quantile.Kind() != exp.KindLiteral || quantile.Name() != "0.5" {
		t.Fatalf("PERCENTILE ALL quantile mismatch:\n%s", all.ToS())
	}

	approx := parseOneDialect(t, "PERCENTILE_APPROX(DISTINCT x, 0.5, 100)", "hive")
	if approx.Kind() != exp.KindApproxQuantile || approx.This() == nil || approx.This().Kind() != exp.KindDistinct {
		t.Fatalf("PERCENTILE_APPROX should be ApproxQuantile(Distinct):\n%s", approx.ToS())
	}
	if accuracy := exprArg(t, approx, "accuracy"); accuracy.Kind() != exp.KindLiteral || accuracy.Name() != "100" {
		t.Fatalf("PERCENTILE_APPROX accuracy mismatch:\n%s", approx.ToS())
	}
}

func TestHiveTransformShapes(t *testing.T) {
	simple := hiveProjection(t, parseOneDialect(t, "SELECT TRANSFORM(a, b)", "hive"))
	if simple.Kind() != exp.KindTransform || simple.This() == nil || simple.This().Name() != "a" || exprArg(t, simple, "expression").Name() != "b" {
		t.Fatalf("simple TRANSFORM should map two arguments to Transform.this/expression:\n%s", simple.ToS())
	}

	query := hiveProjection(t, parseOneDialect(t, "SELECT TRANSFORM(a, b) USING 'cat' AS (x INT, y STRING)", "hive"))
	if query.Kind() != exp.KindQueryTransform || len(query.Expressions()) != 2 || query.Expressions()[0].Name() != "a" || query.Expressions()[1].Name() != "b" {
		t.Fatalf("QueryTransform input expressions mismatch:\n%s", query.ToS())
	}
	hiveLiteralArg(t, query, "command_script", "cat")
	schema := exprArg(t, query, "schema")
	if schema.Kind() != exp.KindSchema || len(schema.Expressions()) != 2 || hiveColumnDType(t, schema.Expressions()[0]).Arg("this") != exp.DTypeInt || hiveColumnDType(t, schema.Expressions()[1]).Arg("this") != exp.DTypeText {
		t.Fatalf("QueryTransform output schema mismatch:\n%s", query.ToS())
	}

	full := hiveProjection(t, parseOneDialect(t, "SELECT TRANSFORM(a) ROW FORMAT DELIMITED FIELDS TERMINATED BY ',' RECORDWRITER 'writer' USING 'cat' AS (x STRING) ROW FORMAT SERDE 'serde' RECORDREADER 'reader'", "hive"))
	if full.Kind() != exp.KindQueryTransform {
		t.Fatalf("full TRANSFORM kind = %v, want QueryTransform:\n%s", full.Kind(), full.ToS())
	}
	before := exprArg(t, full, "row_format_before")
	if before.Kind() != exp.KindRowFormatDelimitedProperty {
		t.Fatalf("row_format_before kind = %v, want RowFormatDelimitedProperty:\n%s", before.Kind(), full.ToS())
	}
	hiveLiteralArg(t, before, "fields", ",")
	hiveLiteralArg(t, full, "record_writer", "writer")
	after := exprArg(t, full, "row_format_after")
	if after.Kind() != exp.KindRowFormatSerdeProperty {
		t.Fatalf("row_format_after kind = %v, want RowFormatSerdeProperty:\n%s", after.Kind(), full.ToS())
	}
	hiveLiteralArg(t, after, "this", "serde")
	hiveLiteralArg(t, full, "record_reader", "reader")
}

func TestHiveStrictCastAndNumericSuffixes(t *testing.T) {
	for _, tc := range []struct {
		sql   string
		dtype exp.DType
	}{
		{"1L", exp.DTypeBigInt},
		{"2S", exp.DTypeSmallInt},
		{"3Y", exp.DTypeTinyInt},
		{"4D", exp.DTypeDouble},
		{"5F", exp.DTypeFloat},
		{"6BD", exp.DTypeDecimal},
	} {
		cast := parseOneDialect(t, tc.sql, "hive")
		if cast.Kind() != exp.KindTryCast || exprArg(t, cast, "to").Arg("this") != tc.dtype {
			t.Fatalf("Hive numeric suffix %q should be TryCast to %v:\n%s", tc.sql, tc.dtype, cast.ToS())
		}
	}

	plain := parseOneDialect(t, "CAST(1 AS INT)", "hive")
	if plain.Kind() != exp.KindTryCast || exprArg(t, plain, "to").Arg("this") != exp.DTypeInt {
		t.Fatalf("Hive CAST should use TryCast:\n%s", plain.ToS())
	}
	dcolon := parseOneDialect(t, "1::SMALLINT", "hive")
	if dcolon.Kind() != exp.KindTryCast || exprArg(t, dcolon, "to").Arg("this") != exp.DTypeSmallInt {
		t.Fatalf("Hive dcolon cast should use TryCast:\n%s", dcolon.ToS())
	}

	for _, dialect := range []string{"", "mysql", "postgres", "presto"} {
		cast := parseOneDialect(t, "CAST(1 AS INT)", dialect)
		if cast.Kind() != exp.KindCast || exprArg(t, cast, "to").Arg("this") != exp.DTypeInt {
			t.Fatalf("%q CAST AST changed by Hive strict-cast flag:\n%s", dialect, cast.ToS())
		}
		cast = parseOneDialect(t, "1::SMALLINT", dialect)
		if cast.Kind() != exp.KindCast || exprArg(t, cast, "to").Arg("this") != exp.DTypeSmallInt {
			t.Fatalf("%q dcolon CAST AST changed by Hive strict-cast flag:\n%s", dialect, cast.ToS())
		}
	}

	for _, tc := range []struct {
		dialect string
		kind    exp.Kind
	}{
		{"", exp.KindAlias},
		{"mysql", exp.KindColumn},
		{"postgres", exp.KindAlias},
		{"presto", exp.KindAlias},
	} {
		expression := parseOneDialect(t, "1S", tc.dialect)
		if expression.Kind() != tc.kind {
			t.Fatalf("%q 1S kind = %v, want unchanged %v:\n%s", tc.dialect, expression.Kind(), tc.kind, expression.ToS())
		}
	}
}

func TestHiveCharVarcharTypesAreTextOnlyOutsideSchema(t *testing.T) {
	for _, typeSQL := range []string{"CHAR(10)", "VARCHAR(50)"} {
		cast := parseOneDialect(t, "CAST(x AS "+typeSQL+")", "hive")
		to := exprArg(t, cast, "to")
		if cast.Kind() != exp.KindTryCast || to.Arg("this") != exp.DTypeText || len(to.Expressions()) != 0 {
			t.Fatalf("Hive CAST AS %s should become size-less TEXT:\n%s", typeSQL, cast.ToS())
		}
	}

	create := parseHiveCreate(t, "CREATE TABLE t (c VARCHAR(50), d CHAR(10))")
	schema := create.This()
	if schema == nil || schema.Kind() != exp.KindSchema || len(schema.Expressions()) != 2 {
		t.Fatalf("CREATE TABLE schema mismatch:\n%s", create.ToS())
	}
	for i, want := range []exp.DType{exp.DTypeVarchar, exp.DTypeChar} {
		dtype := hiveColumnDType(t, schema.Expressions()[i])
		if dtype.Arg("this") != want || len(dtype.Expressions()) != 1 {
			t.Fatalf("schema type %d = %v with %d params, want %v with one param:\n%s", i, dtype.Arg("this"), len(dtype.Expressions()), want, create.ToS())
		}
	}

	for _, dialect := range []string{"", "mysql", "postgres", "presto"} {
		for _, tc := range []struct {
			typeSQL string
			dtype   exp.DType
		}{
			{"CHAR(10)", exp.DTypeChar},
			{"VARCHAR(50)", exp.DTypeVarchar},
		} {
			cast := parseOneDialect(t, "CAST(x AS "+tc.typeSQL+")", dialect)
			to := exprArg(t, cast, "to")
			if cast.Kind() != exp.KindCast || to.Arg("this") != tc.dtype || len(to.Expressions()) != 1 {
				t.Fatalf("%q CAST AS %s AST changed by Hive type hook:\n%s", dialect, tc.typeSQL, cast.ToS())
			}
		}
	}
}

func TestHiveJoinFlagsAreAdditive(t *testing.T) {
	comma := parseOneDialect(t, "SELECT * FROM a, b", "hive")
	joins := expressionsForArg(comma, "joins")
	if len(joins) != 1 || joins[0].Kind() != exp.KindJoin || joins[0].Arg("kind") != "CROSS" || joins[0].Arg("on") != nil {
		t.Fatalf("Hive comma join should become CROSS without an ON clause:\n%s", comma.ToS())
	}
	for _, dialect := range []string{"", "mysql", "postgres", "presto"} {
		root := parseOneDialect(t, "SELECT * FROM a, b", dialect)
		joins := expressionsForArg(root, "joins")
		if len(joins) != 1 || joins[0].Arg("kind") != nil || joins[0].Arg("on") != nil {
			t.Fatalf("%q comma join AST changed by Hive flags:\n%s", dialect, root.ToS())
		}
	}

	for _, joinType := range []string{"FULL OUTER", "LEFT", "RIGHT", "LEFT OUTER", "RIGHT OUTER", "INNER"} {
		t.Run(joinType, func(t *testing.T) {
			root := parseOneDialect(t, "SELECT * FROM t1 "+joinType+" JOIN t2", "hive")
			joins := expressionsForArg(root, "joins")
			if len(joins) != 1 {
				t.Fatalf("Hive %s JOIN count = %d, want 1:\n%s", joinType, len(joins), root.ToS())
			}
			on := exprArg(t, joins[0], "on")
			if on.Kind() != exp.KindBoolean || on.Arg("this") != true {
				t.Fatalf("Hive %s JOIN should inject ON TRUE:\n%s", joinType, root.ToS())
			}

			for _, dialect := range []string{"", "mysql", "postgres", "presto"} {
				other := parseOneDialect(t, "SELECT * FROM t1 "+joinType+" JOIN t2", dialect)
				otherJoins := expressionsForArg(other, "joins")
				if len(otherJoins) != 1 || otherJoins[0].Arg("on") != nil {
					t.Fatalf("%q %s JOIN AST changed by Hive ADD_JOIN_ON_TRUE:\n%s", dialect, joinType, other.ToS())
				}
			}
		})
	}
}
