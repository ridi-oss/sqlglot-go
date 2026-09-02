package schema_test

import (
	"strings"
	"testing"

	"github.com/ridi-oss/sqlglot-go/schema"
)

// Tests for the NON-UPSTREAM implicit-column marking (NewMappingSchemaWithImplicit): in-mapping
// columns marked implicit are excluded from ColumnNames(onlyVisible=true) (the star/USING paths)
// but resolve like any column when named explicitly.

func TestImplicitColumnsVisible(t *testing.T) {
	mapping := schema.M("users", schema.M("id", "INT", "name", "TEXT", "ctid", "TID"))
	s, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("users", []string{"ctid"}), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := s.ColumnNames("users", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visible, ","); got != "id,name" {
		t.Errorf("visible = %q, want id,name", got)
	}
	all, err := s.ColumnNames("users", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(all, ","); got != "id,name,ctid" {
		t.Errorf("all = %q, want id,name,ctid", got)
	}
	// A table without a marking stays all-visible.
	s2, err := schema.NewMappingSchemaWithImplicit(
		schema.M("a", schema.M("x", "INT"), "b", schema.M("y", "INT", "ctid", "TID")),
		schema.M("b", []string{"ctid"}), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	av, _ := s2.ColumnNames("a", true, nil, nil)
	if got := strings.Join(av, ","); got != "x" {
		t.Errorf("unmarked table visible = %q, want x", got)
	}
}

func TestImplicitColumnsNormalization(t *testing.T) {
	// Unquoted implicit names fold with the mapping's normalization (PG lowercases).
	mapping := schema.M("Users", schema.M("ID", "INT", "CTID", "TID"))
	s, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("Users", []string{"CTID"}), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := s.ColumnNames("users", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visible, ","); got != "id" {
		t.Errorf("visible = %q, want id", got)
	}
}

func TestImplicitColumnsNested(t *testing.T) {
	mapping := schema.M("sch", schema.M("users", schema.M("id", "INT", "ctid", "TID")))
	s, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("sch", schema.M("users", []string{"ctid"})), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := s.ColumnNames("sch.users", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visible, ","); got != "id" {
		t.Errorf("visible = %q, want id", got)
	}
}

func TestImplicitColumnsCatalogNested(t *testing.T) {
	// The primary consumer form: catalog -> schema -> table nesting, addressed by the dotted path.
	mapping := schema.M("cat", schema.M("sch", schema.M("users", schema.M("id", "INT", "ctid", "TID"))))
	s, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("cat", schema.M("sch", schema.M("users", []string{"ctid"}))), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := s.ColumnNames("cat.sch.users", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visible, ","); got != "id" {
		t.Errorf("visible = %q, want id", got)
	}
}

func TestImplicitColumnsPartialKeyLookup(t *testing.T) {
	// Regression (review finding): the visible entry is stored at the FULL normalized path, but
	// callers may probe with a partial name — ColumnNames must trie-resolve like Find, or the
	// partial probe misses the entry and the implicit column leaks into star expansion.
	mapping := schema.M("cat", schema.M("sch", schema.M("users", schema.M("id", "INT", "ctid", "TID"))))
	s, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("cat", schema.M("sch", schema.M("users", []string{"ctid"}))), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cat.sch.users", "sch.users", "users"} {
		visible, err := s.ColumnNames(key, true, nil, nil)
		if err != nil {
			t.Fatalf("ColumnNames(%q): %v", key, err)
		}
		if got := strings.Join(visible, ","); got != "id" {
			t.Errorf("ColumnNames(%q, onlyVisible) = %q, want id", key, got)
		}
	}
}

func TestImplicitColumnsFailClosed(t *testing.T) {
	mapping := schema.M("users", schema.M("id", "INT"))
	if _, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("users", []string{"ctid"}), "postgres", true); err == nil {
		t.Error("implicit column absent from mapping must error")
	}
	if _, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("nope", []string{"id"}), "postgres", true); err == nil {
		t.Error("implicit marking for unknown table must error")
	}
	if _, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("a", schema.M("users", []string{"id"})), "postgres", true); err == nil {
		t.Error("implicit key nesting mismatch must error")
	}
}

func TestImplicitColumnsAddTableRecomputesVisible(t *testing.T) {
	// AddTable on a marked table must recompute the visible complement — a stale list would hide
	// a newly added real column from star expansion.
	s, err := schema.NewMappingSchemaWithImplicit(
		schema.M("users", schema.M("id", "INT", "ctid", "TID")),
		schema.M("users", []string{"ctid"}), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTable("users", schema.M("id", "INT", "email", "TEXT", "ctid", "TID"), nil, nil, false); err != nil {
		t.Fatal(err)
	}
	visible, err := s.ColumnNames("users", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visible, ","); got != "id,email" {
		t.Errorf("visible after AddTable = %q, want id,email", got)
	}
}

func TestImplicitColumnsDottedTableName(t *testing.T) {
	// The implicit *Mapping uses node keys, never re-split on dots: a table literally named
	// "x.foo" is addressable and distinct from a schema path x -> foo.
	mapping := schema.M("sch", schema.M("x.foo", schema.M("id", "INT", "ctid", "TID"), "x", schema.M("id", "INT", "ctid", "TID")))
	s, err := schema.NewMappingSchemaWithImplicit(mapping,
		schema.M("sch", schema.M("x.foo", []string{"ctid"})), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := s.ColumnNames(`sch."x.foo"`, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(visible, ","); got != "id" {
		t.Errorf("dotted-name visible = %q, want id", got)
	}
	// The unmarked sibling x stays all-visible.
	xv, err := s.ColumnNames("sch.x", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(xv, ","); got != "id,ctid" {
		t.Errorf("sibling x visible = %q, want id,ctid", got)
	}
}

func TestImplicitColumnsMalformedShapes(t *testing.T) {
	// Validation must walk every branch: empty nested branches and wrong-depth leaves error
	// rather than silently marking nothing (an unmarked ctid would leak into star expansion).
	mapping := schema.M("public", schema.M("users", schema.M("id", "INT", "ctid", "TID")))
	if _, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("nope", schema.NewMapping()), "postgres", true); err == nil {
		t.Error("empty implicit branch must error")
	}
	if _, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("public", []string{"x"}), "postgres", true); err == nil {
		t.Error("shallow implicit leaf must error")
	}
	if _, err := schema.NewMappingSchemaWithImplicit(mapping, schema.M("public", schema.M("users", "ctid")), "postgres", true); err == nil {
		t.Error("non-[]string leaf must error")
	}
}
