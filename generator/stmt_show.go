package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

// showSQL ports generator.py:6027-6029 (base Generator.show_sql stub) and, for MySQL,
// generators/mysql.py:699-748 MySQLGenerator.show_sql verbatim. Base/Postgres never actually
// produce an exp.Show node in this port: SHOW degrades to a raw Command outside MySQL (see
// parser/stmt_show.go's parseShow), so the base branch below only matters for an exp.Show built
// programmatically rather than parsed.
func (g *Generator) showSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		// Postgres `SHOW <name>` (a grammar extension): the parameter name is in "this".
		return "SHOW " + e.Text("this")
	}
	if g.dialect.Name != "mysql" {
		g.unsupported("Unsupported SHOW statement")
		return ""
	}

	this := " " + e.Name()
	full := ""
	if truthy(e.Arg("full")) {
		full = " FULL"
	}
	global_ := ""
	if truthy(e.Arg("global_")) {
		global_ = " GLOBAL"
	}

	target := g.sqlKey(e, "target")
	if target != "" {
		target = " " + target
	}
	switch e.Name() {
	case "COLUMNS", "INDEX":
		target = " FROM" + target
	case "GRANTS":
		target = " FOR" + target
	case "LINKS", "PARTITIONS":
		if target != "" {
			target = " ON" + target
		}
	case "PROJECTIONS":
		if target != "" {
			target = " ON TABLE" + target
		}
	}

	db := g.prefixedSQL("FROM", e, "db")

	like := g.prefixedSQL("LIKE", e, "like")
	where := g.sqlKey(e, "where")

	types := g.expressions(exprsOptions{expression: e, key: "types"})
	if types != "" {
		types = " " + types
	}
	query := g.prefixedSQL("FOR QUERY", e, "query")

	var offset, limit string
	if e.Name() == "PROFILE" {
		offset = g.prefixedSQL("OFFSET", e, "offset")
		limit = g.prefixedSQL("LIMIT", e, "limit")
	} else {
		limit = g.oldstyleLimitSQL(e)
	}

	log := g.prefixedSQL("IN", e, "log")
	position := g.prefixedSQL("FROM", e, "position")

	channel := g.prefixedSQL("FOR CHANNEL", e, "channel")

	mutexOrStatus := ""
	if e.Name() == "ENGINE" {
		if truthy(e.Arg("mutex")) {
			mutexOrStatus = " MUTEX"
		} else {
			mutexOrStatus = " STATUS"
		}
	}

	forTable := g.prefixedSQL("FOR TABLE", e, "for_table")
	forGroup := g.prefixedSQL("FOR GROUP", e, "for_group")
	forUser := g.prefixedSQL("FOR USER", e, "for_user")
	forRole := g.prefixedSQL("FOR ROLE", e, "for_role")
	intoOutfile := g.prefixedSQL("INTO OUTFILE", e, "into_outfile")
	json := ""
	if truthy(e.Arg("json")) {
		json = " JSON"
	}

	return "SHOW" + full + global_ + this + json + target + forTable + types + db + query + log +
		position + channel + mutexOrStatus + like + where + offset + limit + forGroup + forUser +
		forRole + intoOutfile
}

// prefixedSQL mirrors generators/mysql.py:764-766 MySQLGenerator._prefixed_sql.
func (g *Generator) prefixedSQL(prefix string, e expressions.Expression, arg string) string {
	sql := g.sqlKey(e, arg)
	if sql == "" {
		return ""
	}
	return " " + prefix + " " + sql
}

// oldstyleLimitSQL mirrors generators/mysql.py:768-774 MySQLGenerator._oldstyle_limit_sql.
func (g *Generator) oldstyleLimitSQL(e expressions.Expression) string {
	limit := g.sqlKey(e, "limit")
	offset := g.sqlKey(e, "offset")
	if limit == "" {
		return ""
	}
	limitOffset := limit
	if offset != "" {
		limitOffset = offset + ", " + limit
	}
	return " LIMIT " + limitOffset
}

func init() {
	dispatch[expressions.KindShow] = (*Generator).showSQL
}
