package schema_test

import (
	stderrors "errors"
	"reflect"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/schema"
)

func mustSchema(t *testing.T, mapping *schema.Mapping) *schema.MappingSchema {
	t.Helper()
	s, err := schema.NewMappingSchema(mapping, "", true)
	if err != nil {
		t.Fatalf("NewMappingSchema error: %v", err)
	}
	return s
}

func mustEnsureSchema(t *testing.T, mapping *schema.Mapping) schema.Schema {
	t.Helper()
	s, err := schema.EnsureSchema(mapping, "", true)
	if err != nil {
		t.Fatalf("EnsureSchema error: %v", err)
	}
	return s
}

func mustToTable(t *testing.T, table string) exp.Expression {
	t.Helper()
	tableExpr, err := exp.ToTable(table, "", true, nil)
	if err != nil {
		t.Fatalf("ToTable(%q) error: %v", table, err)
	}
	return tableExpr
}

func assertColumnNames(t *testing.T, s schema.Schema, table string, want []string) {
	t.Helper()
	got, err := s.ColumnNames(mustToTable(t, table), false, "", nil)
	if err != nil {
		t.Fatalf("ColumnNames(%q) error: %v", table, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ColumnNames(%q) = %#v, want %#v", table, got, want)
	}
}

func assertColumnNamesRaises(t *testing.T, s schema.Schema, table string) {
	t.Helper()
	_, err := s.ColumnNames(mustToTable(t, table), false, "", nil)
	if err == nil {
		t.Fatalf("ColumnNames(%q) expected SchemaError", table)
	}
	var schemaErr *sqlerrors.SchemaError
	if !stderrors.As(err, &schemaErr) {
		t.Fatalf("ColumnNames(%q) error = %T %v, want SchemaError", table, err, err)
	}
}

func assertColumnNamesEmpty(t *testing.T, s schema.Schema, table string) {
	t.Helper()
	got, err := s.ColumnNames(mustToTable(t, table), false, "", nil)
	if err != nil {
		t.Fatalf("ColumnNames(%q) error: %v", table, err)
	}
	if len(got) != 0 {
		t.Fatalf("ColumnNames(%q) = %#v, want empty", table, got)
	}
}

func TestSchema(t *testing.T) {
	s := mustEnsureSchema(t, schema.M(
		"x", schema.M("a", "uint64"),
		"y", schema.M("b", "uint64", "c", "uint64"),
	))

	assertColumnNames(t, s, "x", []string{"a"})
	assertColumnNames(t, s, "y", []string{"b", "c"})
	assertColumnNames(t, s, "z.x", []string{"a"})
	assertColumnNames(t, s, "z.x.y", []string{"b", "c"})

	assertColumnNamesEmpty(t, s, "z")
	assertColumnNamesEmpty(t, s, "z.z")
	assertColumnNamesEmpty(t, s, "z.z.z")
}

func TestSchemaDb(t *testing.T) {
	s := mustEnsureSchema(t, schema.M(
		"d1", schema.M(
			"x", schema.M("a", "uint64"),
			"y", schema.M("b", "uint64"),
		),
		"d2", schema.M(
			"x", schema.M("c", "uint64"),
		),
	))

	assertColumnNames(t, s, "d1.x", []string{"a"})
	assertColumnNames(t, s, "d2.x", []string{"c"})
	assertColumnNames(t, s, "y", []string{"b"})
	assertColumnNames(t, s, "d1.y", []string{"b"})
	assertColumnNames(t, s, "z.d1.y", []string{"b"})

	assertColumnNamesRaises(t, s, "x")

	assertColumnNamesEmpty(t, s, "z.x")
	assertColumnNamesEmpty(t, s, "z.y")
}

func TestSchemaCatalog(t *testing.T) {
	s := mustEnsureSchema(t, schema.M(
		"c1", schema.M(
			"d1", schema.M(
				"x", schema.M("a", "uint64"),
				"y", schema.M("b", "uint64"),
				"z", schema.M("c", "uint64"),
			),
		),
		"c2", schema.M(
			"d1", schema.M(
				"y", schema.M("d", "uint64"),
				"z", schema.M("e", "uint64"),
			),
			"d2", schema.M(
				"z", schema.M("f", "uint64"),
			),
		),
	))

	assertColumnNames(t, s, "x", []string{"a"})
	assertColumnNames(t, s, "d1.x", []string{"a"})
	assertColumnNames(t, s, "c1.d1.x", []string{"a"})
	assertColumnNames(t, s, "c1.d1.y", []string{"b"})
	assertColumnNames(t, s, "c1.d1.z", []string{"c"})
	assertColumnNames(t, s, "c2.d1.y", []string{"d"})
	assertColumnNames(t, s, "c2.d1.z", []string{"e"})
	assertColumnNames(t, s, "d2.z", []string{"f"})
	assertColumnNames(t, s, "c2.d2.z", []string{"f"})

	assertColumnNamesRaises(t, s, "y")
	assertColumnNamesRaises(t, s, "z")
	assertColumnNamesRaises(t, s, "d1.y")
	assertColumnNamesRaises(t, s, "d1.z")

	assertColumnNamesEmpty(t, s, "q")
	assertColumnNamesEmpty(t, s, "d2.x")
	assertColumnNamesEmpty(t, s, "a.b.c")
}

func TestSchemaAddTableWithAndWithoutMapping(t *testing.T) {
	s := mustSchema(t, nil)
	if err := s.AddTable("test", nil, "", nil, true); err != nil {
		t.Fatalf("AddTable(test) error: %v", err)
	}
	assertColumnNames(t, s, "test", []string{})
	if err := s.AddTable("test", schema.M("x", "string"), "", nil, true); err != nil {
		t.Fatalf("AddTable(test x) error: %v", err)
	}
	assertColumnNames(t, s, "test", []string{"x"})
	if err := s.AddTable("test", schema.M("x", "string", "y", "int"), "", nil, true); err != nil {
		t.Fatalf("AddTable(test x y) error: %v", err)
	}
	assertColumnNames(t, s, "test", []string{"x", "y"})
	if err := s.AddTable("test", nil, "", nil, true); err != nil {
		t.Fatalf("AddTable(test no mapping) error: %v", err)
	}
	assertColumnNames(t, s, "test", []string{"x", "y"})
}

func TestSchemaGetColumnType(t *testing.T) {
	s := mustSchema(t, schema.M("A", schema.M("b", "varchar")))
	assertColumnType(t, s, "a", "B", exp.DTypeVarchar)
	assertColumnType(t, s, exp.Table_("a", nil, nil, nil, nil), exp.Column_("b", nil, nil, nil, nil), exp.DTypeVarchar)
	assertColumnType(t, s, "a", exp.Column_("b", nil, nil, nil, nil), exp.DTypeVarchar)
	assertColumnType(t, s, exp.Table_("a", nil, nil, nil, nil), "b", exp.DTypeVarchar)

	s = mustSchema(t, schema.M("a", schema.M("b", schema.M("c", "varchar"))))
	assertColumnType(t, s, exp.Table_("b", "a", nil, nil, nil), exp.Column_("c", nil, nil, nil, nil), exp.DTypeVarchar)
	assertColumnType(t, s, exp.Table_("b", "a", nil, nil, nil), "c", exp.DTypeVarchar)

	s = mustSchema(t, schema.M("a", schema.M("b", schema.M("c", schema.M("d", "varchar")))))
	assertColumnType(t, s, exp.Table_("c", "b", "a", nil, nil), exp.Column_("d", nil, nil, nil, nil), exp.DTypeVarchar)
	assertColumnType(t, s, exp.Table_("c", "b", "a", nil, nil), "d", exp.DTypeVarchar)

	intType, err := sqlglot.ParseInto("INT", "", exp.KindDataType)
	if err != nil {
		t.Fatalf("ParseInto(INT, DataType) error: %v", err)
	}
	s = mustSchema(t, schema.M("foo", schema.M("bar", intType)))
	assertColumnType(t, s, "foo", "bar", exp.DTypeInt)
}

func assertColumnType(t *testing.T, s schema.Schema, table any, column any, want exp.DType) {
	t.Helper()
	got, err := s.GetColumnType(table, column, "", nil)
	if err != nil {
		t.Fatalf("GetColumnType(%v, %v) error: %v", table, column, err)
	}
	if got.Arg("this") != want {
		t.Fatalf("GetColumnType(%v, %v).this = %v, want %v", table, column, got.Arg("this"), want)
	}
}

func TestSchemaNormalization(t *testing.T) {
	// Check that add_table normalizes both the table and the column names to be added / updated.
	s := mustSchema(t, nil)
	if err := s.AddTable("Foo", schema.M("SomeColumn", "INT", "\"SomeColumn\"", "DOUBLE"), "", nil, true); err != nil {
		t.Fatalf("AddTable(Foo) error: %v", err)
	}
	got, err := s.ColumnNames(exp.Table_("fOO", nil, nil, nil, nil), false, "", nil)
	if err != nil {
		t.Fatalf("ColumnNames(fOO) error: %v", err)
	}
	want := []string{"somecolumn", "SomeColumn"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ColumnNames(fOO) = %#v, want %#v", got, want)
	}

	// TODO(slice 5): port clickhouse/snowflake/bigquery/tsql normalization cases and nested STRUCT normalization.
}

func TestSameNumberOfQualifiers(t *testing.T) {
	s := mustSchema(t, schema.M("x", schema.M("y", schema.M("c1", "int"))))
	assertSchemaErrorMessage(t, s.AddTable("z", schema.M("c2", "int"), "", nil, true), "Table z must match the schema's nesting level: 2.")

	s = mustSchema(t, nil)
	if err := s.AddTable("x.y", schema.M("c1", "int"), "", nil, true); err != nil {
		t.Fatalf("AddTable(x.y) error: %v", err)
	}
	assertSchemaErrorMessage(t, s.AddTable("z", schema.M("c2", "int"), "", nil, true), "Table z must match the schema's nesting level: 2.")

	_, err := schema.NewMappingSchema(schema.M(
		"x", schema.M("y", schema.M("c1", "int")),
		"z", schema.M("c2", "int"),
	), "", true)
	assertSchemaErrorMessage(t, err, "Table z must match the schema's nesting level: 2.")

	_, err = schema.NewMappingSchema(schema.M(
		"catalog", schema.M(
			"db", schema.M("tbl", schema.M("col", "a")),
		),
		"tbl2", schema.M("col", "b"),
	), "", true)
	assertSchemaErrorMessage(t, err, "Table tbl2 must match the schema's nesting level: 3.")

	_, err = schema.NewMappingSchema(schema.M(
		"tbl2", schema.M("col", "b"),
		"catalog", schema.M(
			"db", schema.M("tbl", schema.M("col", "a")),
		),
	), "", true)
	assertSchemaErrorMessage(t, err, "Table catalog.db.tbl must match the schema's nesting level: 1.")
}

func assertSchemaErrorMessage(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected SchemaError %q, got nil", want)
	}
	var schemaErr *sqlerrors.SchemaError
	if !stderrors.As(err, &schemaErr) {
		t.Fatalf("error = %T %v, want SchemaError", err, err)
	}
	if got := schemaErr.Error(); got != want {
		t.Fatalf("SchemaError = %q, want %q", got, want)
	}
}

func TestHasColumn(t *testing.T) {
	s := mustSchema(t, schema.M("x", schema.M("c", "int")))
	got, err := s.HasColumn("x", exp.Column_("c", nil, nil, nil, nil), "", nil)
	if err != nil {
		t.Fatalf("HasColumn(x, c) error: %v", err)
	}
	if !got {
		t.Fatal("HasColumn(x, c) = false, want true")
	}
	got, err = s.HasColumn("x", exp.Column_("k", nil, nil, nil, nil), "", nil)
	if err != nil {
		t.Fatalf("HasColumn(x, k) error: %v", err)
	}
	if got {
		t.Fatal("HasColumn(x, k) = true, want false")
	}
}

func TestFind(t *testing.T) {
	s := mustSchema(t, schema.M("x", schema.M("c", "int")))
	found, err := s.Find(mustToTable(t, "x"), true, false)
	if err != nil {
		t.Fatalf("Find(x) error: %v", err)
	}
	if !mappingEqual(found, schema.M("c", "int")) {
		t.Fatalf("Find(x) = %#v, want c:int", found)
	}
	expectedType, err := exp.DataTypeBuild("int", "", false, true, nil)
	if err != nil {
		t.Fatalf("DataTypeBuild(int) error: %v", err)
	}
	found, err = s.Find(mustToTable(t, "x"), true, true)
	if err != nil {
		t.Fatalf("Find(x, ensureDataTypes) error: %v", err)
	}
	if !mappingEqual(found, schema.M("c", expectedType)) {
		t.Fatalf("Find(x, ensureDataTypes) = %#v, want c:INT", found)
	}
}

func mappingEqual(a, b *schema.Mapping) bool {
	if a == nil || b == nil {
		return a == b
	}
	if !reflect.DeepEqual(a.Keys(), b.Keys()) {
		return false
	}
	for _, key := range a.Keys() {
		av, _ := a.Get(key)
		bv, _ := b.Get(key)
		ae, aok := av.(exp.Expression)
		be, bok := bv.(exp.Expression)
		if aok || bok {
			if !aok || !bok || !ae.Equal(be) {
				return false
			}
			continue
		}
		if av != bv {
			return false
		}
	}
	return true
}
