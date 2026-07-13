package generator

import "github.com/ridi/sqlglot-go/expressions"

// Register the MySQL CREATE TABLE RANGE/LIST partition property family from
// generators/mysql.py:818-839.
func init() {
	dispatch[expressions.KindPartitionByRangeProperty] = (*Generator).partitionByRangePropertySQL
	dispatch[expressions.KindPartitionByListProperty] = (*Generator).partitionByListPropertySQL
	dispatch[expressions.KindPartitionList] = (*Generator).partitionListSQL
	dispatch[expressions.KindPartitionRange] = (*Generator).partitionRangeSQL
}

// mysqlPartitionBySQL ports MySQLGenerator._partition_by_sql
// (generators/mysql.py:818-823).
func (g *Generator) mysqlPartitionBySQL(e expressions.Expression, kind string) string {
	partitions := g.expressions(exprsOptions{expression: e, key: "partition_expressions", flat: true})
	create := g.expressions(exprsOptions{expression: e, key: "create_expressions", flat: true})
	return "PARTITION BY " + kind + " (" + partitions + ") (" + create + ")"
}

// partitionByRangePropertySQL ports partitionbyrangeproperty_sql
// (generators/mysql.py:825-826).
func (g *Generator) partitionByRangePropertySQL(e expressions.Expression) string {
	return g.mysqlPartitionBySQL(e, "RANGE")
}

// partitionByListPropertySQL ports partitionbylistproperty_sql
// (generators/mysql.py:828-829).
func (g *Generator) partitionByListPropertySQL(e expressions.Expression) string {
	return g.mysqlPartitionBySQL(e, "LIST")
}

// partitionListSQL ports partitionlist_sql (generators/mysql.py:831-834).
func (g *Generator) partitionListSQL(e expressions.Expression) string {
	name := g.sqlKey(e, "this")
	values := g.expressions(exprsOptions{expression: e, flat: true})
	return "PARTITION " + name + " VALUES IN (" + values + ")"
}

// partitionRangeSQL ports partitionrange_sql (generators/mysql.py:836-839).
func (g *Generator) partitionRangeSQL(e expressions.Expression) string {
	name := g.sqlKey(e, "this")
	values := g.expressions(exprsOptions{expression: e, flat: true})
	return "PARTITION " + name + " VALUES LESS THAN (" + values + ")"
}
