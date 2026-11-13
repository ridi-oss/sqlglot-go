package generator

import (
	"strings"

	"github.com/ridi-oss/sqlglot-go/expressions"
)

// unicodeStringSQL ports unicodestring_sql (generator.py:1653-1680) for dialects with
// UNICODE_START set ("U&'" / "'"): only the delimiter is escaped, by doubling it.
func (g *Generator) unicodeStringSQL(e expressions.Expression) string {
	escape := g.sqlKey(e, "escape")
	if escape != "" {
		escape = " UESCAPE " + escape
	}
	this := strings.ReplaceAll(g.replaceLineBreaks(e.Name()), "'", "''")
	return "U&'" + this + "'" + escape
}

// structSQL ports struct_sql (generator.py:5267-5278): PropertyEQ fields render as aliases.
func (g *Generator) structSQL(e expressions.Expression) string {
	args := []any{}
	for _, field := range e.Expressions() {
		if field.Kind() == expressions.KindPropertyEQ {
			var alias any = field.This()
			if field.This().IsString() {
				alias = field.Name()
			}
			field = expressions.AliasExpr(field.Arg("expression"), alias, false)
		}
		args = append(args, field)
	}
	return g.funcCall(g.funcName(e), args, "(", ")", true)
}

// versionSQL ports version_sql (generator.py:2649-2653).
func (g *Generator) versionSQL(e expressions.Expression) string {
	name := e.Name()
	if g.isDialect("bigquery") && name == "TIMESTAMP" {
		// generators/bigquery.py:701-704.
		name = "SYSTEM_TIME"
	}
	sql := "FOR " + name + " " + e.Text("kind")
	if expression := g.sqlKey(e, "expression"); expression != "" {
		sql += " " + expression
	}
	return sql
}

func init() {
	dispatch[expressions.KindUnicodeString] = (*Generator).unicodeStringSQL
	dispatch[expressions.KindVersion] = (*Generator).versionSQL
	dispatch[expressions.KindStruct] = (*Generator).structSQL
}
