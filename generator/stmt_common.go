package generator

import "github.com/ridi/sqlglot-go/expressions"

// partitionSQL ports partition_sql (generator.py:2015-2017): `PARTITION(...)` /
// `SUBPARTITION(...)`, plus MySQL's CREATE TABLE partition-property override
// (generators/mysql.py:812-816).
func (g *Generator) partitionSQL(e expressions.Expression) string {
	if g.dialect.Name == "mysql" {
		if parent := e.Parent(); parent != nil && (parent.Kind() == expressions.KindPartitionByRangeProperty || parent.Kind() == expressions.KindPartitionByListProperty) {
			return g.expressions(exprsOptions{expression: e, flat: true})
		}
	}

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
