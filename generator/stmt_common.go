package generator

import "github.com/sjincho/sqlglot-go/expressions"

// partitionSQL ports partition_sql (generator.py:2015-2017): `PARTITION(...)` /
// `SUBPARTITION(...)`.
func (g *Generator) partitionSQL(e expressions.Expression) string {
	keyword := "PARTITION"
	if truthy(e.Arg("subpartition")) {
		keyword = "SUBPARTITION"
	}
	return keyword + "(" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}

// pragmaSQL ports pragma_sql (generator.py:2951-2952): `PRAGMA <this>`.
func (g *Generator) pragmaSQL(e expressions.Expression) string {
	return "PRAGMA " + g.sqlKey(e, "this")
}
