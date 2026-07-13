package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindComment] = (*Generator).commentSQL
	dispatch[expressions.KindTruncateTable] = (*Generator).truncateTableSQL
}

// commentSQL ports comment_sql (generator.py:4110-4116).
func (g *Generator) commentSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	kind := e.Text("kind")
	materialized := ""
	if boolValue(e.Arg("materialized")) {
		materialized = " MATERIALIZED"
	}
	existsSQL := " "
	if boolValue(e.Arg("exists")) {
		existsSQL = " IF EXISTS "
	}
	expressionSQL := g.sqlKey(e, "expression")
	return "COMMENT" + existsSQL + "ON" + materialized + " " + kind + " " + this + " IS " + expressionSQL
}

// truncateTableSQL ports truncatetable_sql (generator.py:5202-5220).
func (g *Generator) truncateTableSQL(e expressions.Expression) string {
	target := "TABLE"
	if boolValue(e.Arg("is_database")) {
		target = "DATABASE"
	}
	tables := " " + g.expressions(exprsOptions{expression: e})

	exists := ""
	if boolValue(e.Arg("exists")) {
		exists = " IF EXISTS"
	}

	onCluster := g.sqlKey(e, "cluster")
	if onCluster != "" {
		onCluster = " " + onCluster
	}

	identity := g.sqlKey(e, "identity")
	if identity != "" {
		identity = " " + identity + " IDENTITY"
	}

	option := g.sqlKey(e, "option")
	if option != "" {
		option = " " + option
	}

	partition := g.sqlKey(e, "partition")
	if partition != "" {
		partition = " " + partition
	}

	return "TRUNCATE " + target + exists + tables + onCluster + identity + option + partition
}
