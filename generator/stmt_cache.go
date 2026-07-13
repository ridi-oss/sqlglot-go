package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindCache] = (*Generator).cacheSQL
	dispatch[expressions.KindUncache] = (*Generator).uncacheSQL
}

// uncacheSQL ports uncache_sql (generator.py:1120-1123).
func (g *Generator) uncacheSQL(e expressions.Expression) string {
	table := g.sqlKey(e, "this")
	existsSQL := ""
	if boolValue(e.Arg("exists")) {
		existsSQL = " IF EXISTS"
	}
	return "UNCACHE TABLE" + existsSQL + " " + table
}

// cacheSQL ports cache_sql (generator.py:1125-1133).
func (g *Generator) cacheSQL(e expressions.Expression) string {
	lazy := ""
	if boolValue(e.Arg("lazy")) {
		lazy = " LAZY"
	}
	table := g.sqlKey(e, "this")

	options := ""
	if opts, ok := e.Arg("options").([]expressions.Expression); ok && len(opts) > 0 {
		options = " OPTIONS(" + g.gen(opts[0]) + " = " + g.gen(opts[1]) + ")"
	}

	sql := g.sqlKey(e, "expression")
	if sql != "" {
		sql = " AS" + g.sep() + sql
	}

	sql = "CACHE" + lazy + " TABLE " + table + options + sql
	return g.prependCtes(e, sql)
}
