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
	// MySQL `SET ROLE …` / `SET DEFAULT ROLE … TO …` (parser/dialect_mysql_set_role.go). Gated on the
	// MySQL dialect so it doesn't collide with the Postgres `SET ROLE` form, which shares kind="ROLE"
	// but renders through the generic path below.
	if g.dialect.Name == "mysql" {
		if kind := e.Text("kind"); kind == "ROLE" || kind == "DEFAULT ROLE" {
			return g.mysqlSetRoleSQL(e)
		}
	}

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
	case "PASSWORD":
		// MySQL `PASSWORD [FOR <user>] { = '<string>' | TO RANDOM }`: the optional FOR user is in
		// "this"; the value is in expressions — a string Literal renders as `= '…'`, the RANDOM
		// keyword (a Var) as `TO RANDOM`.
		forUser := g.sqlKey(e, "this")
		if forUser != "" {
			forUser = " FOR " + forUser
		}
		value := g.expressions(exprsOptions{expression: e})
		delim := " = "
		if items := e.Expressions(); len(items) == 1 && items[0].Kind() == expressions.KindVar {
			delim = " TO "
		}
		return "PASSWORD" + forUser + delim + value
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
	// scope is the optional SESSION/LOCAL prefix on the Postgres special forms whose own label is
	// carried in "kind" (e.g. `SET LOCAL ROLE r` -> scope=LOCAL, kind=ROLE). A plain assignment's
	// scope lives in "kind" instead, so this stays empty there.
	scope := g.sqlKey(e, "scope")
	if scope != "" {
		scope += " "
	}
	return global + scope + kind + this + expressionsSQL + collate
}

// mysqlSetRoleSQL renders MySQL `SET ROLE …` / `SET DEFAULT ROLE … TO …` (parser/dialect_mysql_set_role.go):
//
//	ROLE { DEFAULT | NONE | ALL [EXCEPT r,…] | r,… }
//	DEFAULT ROLE { NONE | ALL | r,… } TO u,…
//
// kind = "ROLE"|"DEFAULT ROLE"; the DEFAULT/NONE/ALL keyword (when present) is in "this"; the role list
// (or the ALL EXCEPT list) is in "expressions"; `except` marks the EXCEPT form; the TO users are in "to".
func (g *Generator) mysqlSetRoleSQL(e expressions.Expression) string {
	out := g.sqlKey(e, "kind") // "ROLE" or "DEFAULT ROLE"
	if this := g.sqlKey(e, "this"); this != "" {
		out += " " + this
	}
	if boolValue(e.Arg("except")) {
		out += " EXCEPT"
	}
	if roles := g.expressions(exprsOptions{expression: e}); roles != "" {
		out += " " + roles
	}
	if to := g.expressions(exprsOptions{expression: e, key: "to"}); to != "" {
		out += " TO " + to
	}
	return out
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
