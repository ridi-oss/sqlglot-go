package schema_test

import (
	"testing"

	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/schema"
)

// TestSchemaDialectTypeArgs guards the polymorphic dialect contract of the Schema interface:
// every method's dialect argument is a dialects.DialectType (nil | string | *dialects.Dialect),
// not just a string. It fixes the cycle-driven regression where these were string-only, and
// asserts a *Dialect behaves identically to its name. If a refactor narrows the interface back
// to string, this test stops compiling.
func TestSchemaDialectTypeArgs(t *testing.T) {
	mysql, err := dialects.GetOrRaise("mysql")
	if err != nil {
		t.Fatalf("GetOrRaise(mysql): %v", err)
	}

	// Build the same schema three ways — via a string dialect, a *Dialect, and nil — then
	// read it back through the matching dialect form. All three must agree.
	cases := []struct {
		name    string
		dialect dialects.DialectType
	}{
		{"string", "mysql"},
		{"pointer", mysql},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := schema.NewMappingSchema(nil, tc.dialect, true)
			if err != nil {
				t.Fatalf("NewMappingSchema(%v): %v", tc.name, err)
			}
			// AddTable, ColumnNames, GetColumnType, HasColumn all take DialectType.
			if err := s.AddTable("t", schema.M("a", "INT", "b", "VARCHAR"), tc.dialect, nil, true); err != nil {
				t.Fatalf("AddTable(%v): %v", tc.name, err)
			}
			cols, err := s.ColumnNames("t", false, tc.dialect, nil)
			if err != nil {
				t.Fatalf("ColumnNames(%v): %v", tc.name, err)
			}
			if len(cols) != 2 {
				t.Fatalf("ColumnNames(%v) = %v, want 2 columns", tc.name, cols)
			}
			ct, err := s.GetColumnType("t", "a", tc.dialect, nil)
			if err != nil {
				t.Fatalf("GetColumnType(%v): %v", tc.name, err)
			}
			if ct.Arg("this") != exp.DTypeInt {
				t.Fatalf("GetColumnType(%v).this = %v, want INT", tc.name, ct.Arg("this"))
			}
			has, err := s.HasColumn("t", "b", tc.dialect, nil)
			if err != nil {
				t.Fatalf("HasColumn(%v): %v", tc.name, err)
			}
			if !has {
				t.Fatalf("HasColumn(%v, b) = false, want true", tc.name)
			}
		})
	}

	// EnsureSchema accepts a *Dialect for its dialect argument, and a SchemaType (nil | *Mapping |
	// Schema) for its schema argument.
	if _, err := schema.EnsureSchema(schema.M("t", schema.M("a", "INT")), mysql, true); err != nil {
		t.Fatalf("EnsureSchema(*Mapping, *Dialect): %v", err)
	}
	if _, err := schema.EnsureSchema(nil, mysql, true); err != nil {
		t.Fatalf("EnsureSchema(nil schema): %v", err)
	}
	built, err := schema.NewMappingSchema(schema.M("t", schema.M("a", "INT")), "mysql", true)
	if err != nil {
		t.Fatalf("NewMappingSchema: %v", err)
	}
	// A value already implementing Schema is returned as-is (same instance).
	got, err := schema.EnsureSchema(built, "mysql", true)
	if err != nil {
		t.Fatalf("EnsureSchema(Schema): %v", err)
	}
	if got != schema.Schema(built) {
		t.Fatal("EnsureSchema(Schema) should return the same instance")
	}

	// GetUDFType's dialect argument is a DialectType too (currently a stub, but the signature
	// must accept a *Dialect).
	if _, err := built.GetUDFType("f", mysql, nil); err != nil {
		t.Fatalf("GetUDFType(*Dialect): %v", err)
	}
}
