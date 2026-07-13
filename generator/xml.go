package generator

import "github.com/ridi/sqlglot-go/expressions"

// xmlElementSQL ports xmlelement_sql (generator.py:5873-5876):
// XMLELEMENT(NAME name[, expr, ...]) / XMLELEMENT(EVALNAME expr[, expr, ...]).
func (g *Generator) xmlElementSQL(e expressions.Expression) string {
	prefix := "NAME"
	if boolValue(e.Arg("evalname")) {
		prefix = "EVALNAME"
	}
	name := prefix + " " + g.sqlKey(e, "this")
	args := append([]any{name}, listFromValue(e.Arg("expressions"))...)
	return g.funcCall("XMLELEMENT", args, "(", ")", true)
}

// xmlTableSQL ports xmltable_sql (generator.py:5964-5973).
func (g *Generator) xmlTableSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")

	namespaces := g.expressions(exprsOptions{expression: e, key: "namespaces"})
	if namespaces != "" {
		namespaces = "XMLNAMESPACES(" + namespaces + "), "
	}

	passing := g.expressions(exprsOptions{expression: e, key: "passing"})
	if passing != "" {
		passing = g.sep() + "PASSING" + g.seg(passing)
	}

	columns := g.expressions(exprsOptions{expression: e, key: "columns"})
	if columns != "" {
		columns = g.sep() + "COLUMNS" + g.seg(columns)
	}

	byRef := ""
	if boolValue(e.Arg("by_ref")) {
		byRef = g.sep() + "RETURNING SEQUENCE BY REF"
	}

	inner := g.indent(namespaces+this+passing+byRef+columns, 0, nil, false, false)
	return "XMLTABLE(" + g.sep("") + inner + g.seg(")", "")
}

// xmlNamespaceSQL ports xmlnamespace_sql (generator.py:5975-5977).
func (g *Generator) xmlNamespaceSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if thisExpr := e.This(); !isNilExpression(thisExpr) && thisExpr.Kind() == expressions.KindAlias {
		return this
	}
	return "DEFAULT " + this
}

// pathColumnConstraintSQL ports the exp.PathColumnConstraint TRANSFORMS lambda
// (generator.py:233, `f"PATH {self.sql(e, 'this')}"`). See the KindPathColumnConstraint
// comment in expressions/kinds.go for why this Kind is defined/dispatched from this slice.
func (g *Generator) pathColumnConstraintSQL(e expressions.Expression) string {
	return "PATH " + g.sqlKey(e, "this")
}

func init() {
	dispatch[expressions.KindXMLElement] = (*Generator).xmlElementSQL
	dispatch[expressions.KindXMLTable] = (*Generator).xmlTableSQL
	dispatch[expressions.KindXMLNamespace] = (*Generator).xmlNamespaceSQL
	dispatch[expressions.KindPathColumnConstraint] = (*Generator).pathColumnConstraintSQL
}
