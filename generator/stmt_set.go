package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindSet] = (*Generator).setSQL
	dispatch[expressions.KindSetItem] = (*Generator).setItemSQL
}

// setItemSQL ports setitem_sql (generator.py:2925-2936). The upstream
// SET_ASSIGNMENT_REQUIRES_VARIABLE_KEYWORD branch (kind == "VARIABLE" special-case) only
// fires when that flag is false; it defaults true and no dialect in this port's base/
// mysql/postgres scope overrides it, so it's omitted here.
func (g *Generator) setItemSQL(e expressions.Expression) string {
	// Postgres SET special-forms whose shape doesn't fit the generic `kind this expressions`
	// order (see parser/dialect_postgres_set.go).
	switch e.Text("kind") {
	case "CONSTRAINTS":
		// `CONSTRAINTS { ALL | name, ... } { DEFERRED | IMMEDIATE }`: targets in expressions, mode in this.
		mode := g.sqlKey(e, "this")
		if mode != "" {
			mode = " " + mode
		}
		return "CONSTRAINTS " + g.expressions(exprsOptions{expression: e}) + mode
	case "SESSION CHARACTERISTICS":
		return "SESSION CHARACTERISTICS AS TRANSACTION " + g.expressions(exprsOptions{expression: e})
	}

	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind += " "
	}
	this := g.sqlKey(e, "this")
	expressionsSQL := g.expressions(exprsOptions{expression: e})
	collate := g.sqlKey(e, "collate")
	if collate != "" {
		collate = " COLLATE " + collate
	}
	global := ""
	if boolValue(e.Arg("global_")) {
		global = "GLOBAL "
	}
	return global + kind + this + expressionsSQL + collate
}

// setSQL ports set_sql (generator.py:2938-2941).
func (g *Generator) setSQL(e expressions.Expression) string {
	expressionsSQL := " " + g.expressions(exprsOptions{expression: e, flat: true})
	tag := ""
	if boolValue(e.Arg("tag")) {
		tag = " TAG"
	}
	verb := "SET"
	if boolValue(e.Arg("unset")) {
		verb = "UNSET"
	}
	return verb + tag + expressionsSQL
}
