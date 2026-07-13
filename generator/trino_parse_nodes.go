package generator

import "github.com/ridi/sqlglot-go/expressions"

// jsonExtractQuoteSQL ports jsonextractquote_sql (generator.py:5605-5607). It is a plain
// expression nested in Trino/Athena JSON_QUERY's quote arg, not a dialect TRANSFORMS entry.
func (g *Generator) jsonExtractQuoteSQL(e expressions.Expression) string {
	scalar := ""
	if boolValue(e.Arg("scalar")) {
		scalar = " ON SCALAR STRING"
	}
	return g.sqlKey(e, "option") + " QUOTES" + scalar
}

// overflowTruncateBehaviorSQL ports overflowtruncatebehavior_sql (generator.py:5782-5786).
// GroupConcat's Trino LISTAGG transform is intentionally out of scope; this renderer only makes
// the structured on_overflow child independently generatable.
func (g *Generator) overflowTruncateBehaviorSQL(e expressions.Expression) string {
	filler := g.sqlKey(e, "this")
	if filler != "" {
		filler = " " + filler
	}
	withCount := "WITHOUT COUNT"
	if boolValue(e.Arg("with_count")) {
		withCount = "WITH COUNT"
	}
	return "TRUNCATE" + filler + " " + withCount
}

// refreshSQL ports refresh_sql (generator.py:5069-5072). Literal targets omit the statement
// kind upstream; parsed REFRESH MATERIALIZED VIEW targets are tables and retain their kind.
func (g *Generator) refreshSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	kind := ""
	if target, ok := e.Arg("this").(expressions.Expression); !ok || target == nil || target.Kind() != expressions.KindLiteral {
		kind = e.Text("kind") + " "
	}
	return "REFRESH " + kind + this
}

func init() {
	dispatch[expressions.KindJSONExtractQuote] = (*Generator).jsonExtractQuoteSQL
	dispatch[expressions.KindOverflowTruncateBehavior] = (*Generator).overflowTruncateBehaviorSQL
	dispatch[expressions.KindRefresh] = (*Generator).refreshSQL
}
