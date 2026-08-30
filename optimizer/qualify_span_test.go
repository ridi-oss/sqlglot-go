package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

// Span text survives Qualify: it aliases computed projections via a wrapper without
// replacing the inner node, so "DATABASE( )" is still readable after the rewrite.
func TestQualifyPreservesProjectionSpanText(t *testing.T) {
	d := dialects.MySQL()
	mapping, err := schema.NewMappingSchema(schema.M(
		"t", schema.M("a", "INT"),
	), d, true)
	if err != nil {
		t.Fatalf("NewMappingSchema: %v", err)
	}
	expression, err := sqlglot.ParseOne("SELECT DATABASE( ), 1+1 FROM t", "mysql")
	if err != nil {
		t.Fatalf("ParseOne: %v", err)
	}
	q := optimizer.Qualify(expression, optimizer.QualifyOpts{Dialect: d, Schema: mapping})
	want := []string{"DATABASE( )", "1+1"}
	selects := q.Selects()
	if len(selects) != len(want) {
		t.Fatalf("%d projections, want %d", len(selects), len(want))
	}
	for i, projection := range selects {
		node := projection
		if node.Kind() == exp.KindAlias {
			node = node.This()
		}
		if text, ok := node.SpanText(); !ok || text != want[i] {
			t.Errorf("projection %d span text after Qualify = %q,%v; want %q,true", i, text, ok, want[i])
		}
	}
}
