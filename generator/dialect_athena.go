package generator

import (
	"github.com/ridi-oss/sqlglot-go/expressions"
	"strings"
)

// generators/athena.py:18-45, 134-155.
func init() {
	registerDialectDispatch("athena-hive", map[expressions.Kind]func(*Generator, expressions.Expression) string{
		expressions.KindAlter: func(g *Generator, e expressions.Expression) string {
			e = e.Copy()
			actions := listFromValue(e.Arg("actions"))
			if e.Text("kind") == "TABLE" && len(actions) > 0 {
				if first := asExpression(actions[0]); first != nil && first.Kind() == expressions.KindColumnDef {
					items := make([]expressions.Expression, 0, len(actions))
					for _, a := range actions {
						items = append(items, asExpression(a))
					}
					e.Set("actions", []expressions.Expression{expressions.New(expressions.KindSchema, expressions.Args{"expressions": items})})
				}
			}
			return g.alterSQL(e)
		},
	})
	registerDialectPropertyLocations("athena-trino", map[expressions.Kind]propertyLocation{expressions.KindLocationProperty: propertyLocationPostWith})
	registerDialectDispatch("athena-trino", map[expressions.Kind]func(*Generator, expressions.Expression) string{
		expressions.KindLocationProperty: func(g *Generator, e expressions.Expression) string {
			name := "external_location"
			if athenaIcebergProperties(e.Parent()) {
				name = "location"
			}
			return name + "=" + g.sqlKey(e, "this")
		},
		expressions.KindPartitionedByProperty: func(g *Generator, e expressions.Expression) string {
			name := "partitioned_by"
			if athenaIcebergProperties(e.Parent()) {
				name = "partitioning"
			}
			return name + "=" + g.sqlKey(e, "this")
		},
	})
}

func athenaIcebergProperties(e expressions.Expression) bool {
	if e != nil && e.Kind() == expressions.KindProperties {
		for _, p := range e.Expressions() {
			if p.Kind() == expressions.KindProperty && p.Name() == "table_type" {
				return strings.EqualFold(p.Text("value"), "iceberg")
			}
		}
	}
	return false
}
