package generator

import "github.com/ridi/sqlglot-go/expressions"

// chrSQL ports chr_sql (generator.py:6190-6194): NAME(<comma-joined expressions>[ USING
// <charset>]). Base/postgres keep the default "CHR" name; mysql renames it to "CHAR"
// (generators/mysql.py:160, mirrored inline via the dialect check below rather than a
// separate dialect-flag/override table).
func (g *Generator) chrSQL(e expressions.Expression) string {
	name := "CHR"
	if g.dialect.Name == "mysql" {
		name = "CHAR"
	}
	this := g.expressions(exprsOptions{expression: e})
	charset := g.sqlKey(e, "charset")
	using := ""
	if charset != "" {
		using = " USING " + charset
	}
	return g.funcCall(name, []any{this + using}, "", "", true)
}

func init() {
	dispatch[expressions.KindChr] = (*Generator).chrSQL
}
