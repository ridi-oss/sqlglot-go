package generator

import (
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

// The "ddl" cluster: CREATE FUNCTION/PROCEDURE/INDEX/TRIGGER structured rendering, plus the
// small PROPERTY_PARSERS subset this port supports. See the Kind block comment in
// expressions/kinds.go for the upstream class/line references. Per the documented
// parallelism seam (generator/dispatch.go:5-15), this file owns its own init() and
// generator/dispatch.go is never edited.
func init() {
	dispatch[expressions.KindProperties] = (*Generator).propertiesSQL
	dispatch[expressions.KindUserDefinedFunction] = (*Generator).userDefinedFunctionSQL
	dispatch[expressions.KindReturnsProperty] = (*Generator).returnsPropertySQL
	dispatch[expressions.KindLanguageProperty] = (*Generator).languagePropertySQL
	dispatch[expressions.KindSqlSecurityProperty] = (*Generator).sqlSecurityPropertySQL
	dispatch[expressions.KindCalledOnNullInputProperty] = (*Generator).calledOnNullInputPropertySQL
	dispatch[expressions.KindStrictProperty] = (*Generator).strictPropertySQL
	dispatch[expressions.KindStabilityProperty] = (*Generator).stabilityPropertySQL
	dispatch[expressions.KindSetConfigProperty] = (*Generator).setConfigPropertySQL
	dispatch[expressions.KindCharacterSetProperty] = (*Generator).characterSetPropertySQL
	dispatch[expressions.KindRowFormatProperty] = (*Generator).rowFormatPropertySQL
	dispatch[expressions.KindReturn] = (*Generator).returnSQL
	dispatch[expressions.KindIndex] = (*Generator).indexSQL
	dispatch[expressions.KindOpclass] = (*Generator).opclassSQL
	dispatch[expressions.KindTriggerProperties] = (*Generator).triggerPropertiesSQL
	dispatch[expressions.KindTriggerExecute] = (*Generator).triggerExecuteSQL
	dispatch[expressions.KindTriggerEvent] = (*Generator).triggerEventSQL
	dispatch[expressions.KindTriggerReferencing] = (*Generator).triggerReferencingSQL
}

// propertiesSQL ports properties_sql (generator.py:2019-2042). CREATE first buckets the
// complete property list by location; this method renders the POST_SCHEMA subset as a flat,
// space-separated root list and the POST_WITH subset as one comma-separated WITH (...) block.
func (g *Generator) propertiesSQL(e expressions.Expression) string {
	locations := g.locateProperties(e)
	rootProperties := g.propertiesExpression(locations[propertyLocationPostSchema], e.Parent())
	withProperties := g.propertiesExpression(locations[propertyLocationPostWith], e.Parent())

	rootSQL := g.rootPropertiesSQL(rootProperties)
	withSQL := g.withPropertiesSQL(withProperties)
	if rootSQL != "" && withSQL != "" && !g.pretty {
		withSQL = " " + withSQL
	}
	return rootSQL + withSQL
}

// userdefinedfunction_sql (generator.py:4715-4721). no_identify is not ported (this port has
// no identify-suppression context; Identify defaults to false in every corpus case that
// reaches this method, so the two are behaviorally identical here).
func (g *Generator) userDefinedFunctionSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if boolValue(e.Arg("wrapped")) {
		exprs = g.wrap(exprs)
	} else if exprs != "" {
		exprs = " " + exprs
	}
	if strings.TrimSpace(exprs) != "" {
		return this + exprs
	}
	return this
}

// naked_property (generator.py:4703-4707), inlined per property: "<NAME> <this>". Only the
// small fixed set of properties this port supports needs it, so the generic
// Properties.PROPERTY_TO_NAME table is skipped in favor of hardcoding each NAME (matching
// properties.py:593-616 NAME_TO_PROPERTY for LANGUAGE/RETURNS/CHARACTER SET/ROW_FORMAT).

// returnsproperty_sql (generator.py:247-249): "RETURNS NULL ON NULL INPUT" when null is set,
// else the naked_property form "RETURNS <this>".
func (g *Generator) returnsPropertySQL(e expressions.Expression) string {
	if boolValue(e.Arg("null")) {
		return "RETURNS NULL ON NULL INPUT"
	}
	return "RETURNS " + g.sqlKey(e, "this")
}

// languageproperty_sql: naked_property -> "LANGUAGE <this>" (generator.py:212).
func (g *Generator) languagePropertySQL(e expressions.Expression) string {
	return "LANGUAGE " + g.sqlKey(e, "this")
}

// sqlsecurityproperty_sql (generator.py:260): "SQL SECURITY <this>". `this` is a raw
// string (DEFINER/INVOKER/NONE), not an Expression - sqlKey's underlying gen() already
// handles a bare string arg (mirrors self.sql() short-circuiting on isinstance(x, str)).
func (g *Generator) sqlSecurityPropertySQL(e expressions.Expression) string {
	return "SQL SECURITY " + g.sqlKey(e, "this")
}

// exp.CalledOnNullInputProperty: lambda *_: "CALLED ON NULL INPUT" (generator.py:152).
func (g *Generator) calledOnNullInputPropertySQL(expressions.Expression) string {
	return "CALLED ON NULL INPUT"
}

// exp.StrictProperty: lambda *_: "STRICT" (generator.py:264).
func (g *Generator) strictPropertySQL(expressions.Expression) string {
	return "STRICT"
}

// exp.StabilityProperty: lambda _, e: e.name (generator.py:261) - `this` is always a
// Literal.string("IMMUTABLE"|"STABLE"|"VOLATILE"), and Name() on the property node itself
// unwraps a Literal/Identifier/Var "this" the same way Python's .name property does.
func (g *Generator) stabilityPropertySQL(e expressions.Expression) string {
	return e.Name()
}

// exp.SetConfigProperty: lambda self, e: self.sql(e, "this") (generator.py:255) - `this` is
// the already-ported exp.Set node (setSQL/setItemSQL, generator/stmt_set.go), which renders
// its own leading "SET " keyword.
func (g *Generator) setConfigPropertySQL(e expressions.Expression) string {
	return g.sqlKey(e, "this")
}

// characterset_sql-adjacent: CharacterSetProperty (generator.py:155-157): `[DEFAULT ]
// CHARACTER SET=<this>`. Note the canonical NAME is always "CHARACTER SET" regardless of
// whether the source spelled it CHARSET or CHARACTER SET (both PROPERTY_PARSERS keys share
// this one parser/renderer - parser.py:1239-1240).
func (g *Generator) characterSetPropertySQL(e expressions.Expression) string {
	def := ""
	if boolValue(e.Arg("default")) {
		def = "DEFAULT "
	}
	return def + "CHARACTER SET=" + g.sqlKey(e, "this")
}

// RowFormatProperty has no TRANSFORMS/dedicated method upstream, so it falls through to the
// generic property_sql fallback (generator.py:2083-2092): "<NAME>=<this>" via
// PROPERTY_TO_NAME (properties.py:606: "ROW_FORMAT" -> RowFormatProperty). Hardcoded here
// since this port has only the one PROPERTY_TO_NAME-fallback-rendered property in scope.
func (g *Generator) rowFormatPropertySQL(e expressions.Expression) string {
	return "ROW_FORMAT=" + g.sqlKey(e, "this")
}

// return_sql (generator.py:3932-3933).
func (g *Generator) returnSQL(e expressions.Expression) string {
	return "RETURN " + g.sqlKey(e, "this")
}

// index_sql (generator.py:1946-1958). INDEX_ON defaults to "ON" (only hive overrides it to
// "ON TABLE", out of scope).
func (g *Generator) indexSQL(e expressions.Expression) string {
	unique := ""
	if boolValue(e.Arg("unique")) {
		unique = "UNIQUE "
	}
	primary := ""
	if boolValue(e.Arg("primary")) {
		primary = "PRIMARY "
	}
	amp := ""
	if boolValue(e.Arg("amp")) {
		amp = "AMP "
	}
	name := g.sqlKey(e, "this")
	if name != "" {
		name += " "
	}
	table := g.sqlKey(e, "table")
	if table != "" {
		table = "ON " + table
	}
	index := ""
	if table == "" {
		index = "INDEX "
	}
	params := g.sqlKey(e, "params")
	return unique + primary + amp + index + name + table + params
}

// opclass_sql (generator.py:4967-4968).
func (g *Generator) opclassSQL(e expressions.Expression) string {
	return g.sqlKey(e, "this") + " " + g.sqlKey(e, "expression")
}

// triggerproperties_sql (generator.py:1446-1473).
func (g *Generator) triggerPropertiesSQL(e expressions.Expression) string {
	timing := stringValue(e.Arg("timing"))
	var eventParts []string
	for _, ev := range listFromValue(e.Arg("events")) {
		if sql := g.gen(ev); sql != "" {
			eventParts = append(eventParts, sql)
		}
	}
	events := strings.Join(eventParts, " OR ")
	timingEvents := ""
	if timing != "" || events != "" {
		timingEvents = strings.TrimSpace(timing + " " + events)
	}
	parts := []string{timingEvents, "ON", g.sqlKey(e, "table")}
	if refTable := g.sqlKey(e, "referenced_table"); refTable != "" {
		parts = append(parts, "FROM", refTable)
	}
	if deferrable := stringValue(e.Arg("deferrable")); deferrable != "" {
		parts = append(parts, deferrable)
	}
	if initially := stringValue(e.Arg("initially")); initially != "" {
		parts = append(parts, "INITIALLY "+initially)
	}
	if referencing := g.sqlKey(e, "referencing"); referencing != "" {
		parts = append(parts, referencing)
	}
	if forEach := stringValue(e.Arg("for_each")); forEach != "" {
		parts = append(parts, "FOR EACH "+forEach)
	}
	if when := g.sqlKey(e, "when"); when != "" {
		parts = append(parts, "WHEN ("+when+")")
	}
	parts = append(parts, g.sqlKey(e, "execute"))
	return strings.Join(parts, g.sep())
}

// exp.TriggerExecute: lambda self, e: f"EXECUTE FUNCTION {self.sql(e, 'this')}"
// (generator.py:275) - always renders FUNCTION, canonicalizing a source EXECUTE PROCEDURE.
func (g *Generator) triggerExecuteSQL(e expressions.Expression) string {
	return "EXECUTE FUNCTION " + g.sqlKey(e, "this")
}

// triggerevent_sql (generator.py:1486-1491).
func (g *Generator) triggerEventSQL(e expressions.Expression) string {
	if columns := g.expressions(exprsOptions{expression: e, key: "columns", flat: true}); columns != "" {
		return stringValue(e.Arg("this")) + " OF " + columns
	}
	return g.sqlKey(e, "this")
}

// triggerreferencing_sql (generator.py:1475-1484).
func (g *Generator) triggerReferencingSQL(e expressions.Expression) string {
	var parts []string
	if old := g.sqlKey(e, "old"); old != "" {
		parts = append(parts, "OLD TABLE AS "+old)
	}
	if new := g.sqlKey(e, "new"); new != "" {
		parts = append(parts, "NEW TABLE AS "+new)
	}
	return "REFERENCING " + strings.Join(parts, " ")
}
