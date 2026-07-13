package generator

import "github.com/ridi/sqlglot-go/expressions"

// sliceSQL ports slice_sql (generator.py:5667-5673): `start:end` with an optional
// `:step` suffix, and a bare `start:` when both end and step are empty.
func (g *Generator) sliceSQL(e expressions.Expression) string {
	step := g.sqlKey(e, "step")
	end := g.sqlKey(e, "expression")
	begin := g.sqlKey(e, "this")

	sql := end
	if step != "" {
		sql = end + ":" + step
	}
	if sql != "" {
		return begin + ":" + sql
	}
	return begin + ":"
}

// nationalSQL ports national_sql (generator.py:2010-2012): re-quotes the raw text as an
// ordinary string literal (so dialect-specific quoting/escaping applies) and prefixes it
// with "N".
func (g *Generator) nationalSQL(e expressions.Expression) string {
	return "N" + g.quoteStart + g.escapeStr(e.Name()) + g.quoteEnd
}

func init() {
	dispatch[expressions.KindSlice] = (*Generator).sliceSQL
	dispatch[expressions.KindNational] = (*Generator).nationalSQL
}
