package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindDirectory] = (*Generator).directorySQL
	dispatch[expressions.KindRowFormatDelimitedProperty] = (*Generator).rowFormatDelimitedPropertySQL
	dispatch[expressions.KindRowFormatSerdeProperty] = (*Generator).rowFormatSerdePropertySQL
	dispatch[expressions.KindSerdeProperties] = (*Generator).serdePropertiesSQL
	dispatch[expressions.KindWithTableHint] = (*Generator).withTableHintSQL
	dispatch[expressions.KindIndexTableHint] = (*Generator).indexTableHintSQL
}

// directorySQL ports directory_sql (generator.py:1770-1774): Hive/Spark
// `INSERT OVERWRITE [LOCAL] DIRECTORY '...' [ROW FORMAT ...]`.
func (g *Generator) directorySQL(e expressions.Expression) string {
	local := ""
	if boolValue(e.Arg("local")) {
		local = "LOCAL "
	}
	rowFormat := g.sqlKey(e, "row_format")
	if rowFormat != "" {
		rowFormat = " " + rowFormat
	}
	return local + "DIRECTORY " + g.sqlKey(e, "this") + rowFormat
}

// rowFormatDelimitedPropertySQL ports rowformatdelimitedproperty_sql (generator.py:2348-
// 2361). Note "serde" (arg_types) is unused here too, matching upstream verbatim.
func (g *Generator) rowFormatDelimitedPropertySQL(e expressions.Expression) string {
	fields := g.sqlKey(e, "fields")
	if fields != "" {
		fields = " FIELDS TERMINATED BY " + fields
	}
	escaped := g.sqlKey(e, "escaped")
	if escaped != "" {
		escaped = " ESCAPED BY " + escaped
	}
	items := g.sqlKey(e, "collection_items")
	if items != "" {
		items = " COLLECTION ITEMS TERMINATED BY " + items
	}
	keys := g.sqlKey(e, "map_keys")
	if keys != "" {
		keys = " MAP KEYS TERMINATED BY " + keys
	}
	lines := g.sqlKey(e, "lines")
	if lines != "" {
		lines = " LINES TERMINATED BY " + lines
	}
	null := g.sqlKey(e, "null")
	if null != "" {
		null = " NULL DEFINED AS " + null
	}
	return "ROW FORMAT DELIMITED" + fields + escaped + items + keys + lines + null
}

// rowFormatSerdePropertySQL has no upstream generator method (ROW FORMAT SERDE has zero
// round-trip corpus coverage in this port, base+MySQL+Postgres): this renders the natural
// inverse of parseRowFormat's SERDE branch (parser_dml.go).
func (g *Generator) rowFormatSerdePropertySQL(e expressions.Expression) string {
	serdeProperties := g.sqlKey(e, "serde_properties")
	if serdeProperties != "" {
		serdeProperties = " " + serdeProperties
	}
	return "ROW FORMAT SERDE " + g.sqlKey(e, "this") + serdeProperties
}

// serdePropertiesSQL has no upstream generator method either (see rowFormatSerdePropertySQL);
// renders the natural inverse of parseSerdeProperties.
func (g *Generator) serdePropertiesSQL(e expressions.Expression) string {
	with_ := ""
	if boolValue(e.Arg("with_")) {
		with_ = "WITH "
	}
	return with_ + "SERDEPROPERTIES (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}

// withTableHintSQL ports withtablehint_sql (generator.py:2363-2364): T-SQL `WITH (...)`
// table hints.
func (g *Generator) withTableHintSQL(e expressions.Expression) string {
	return "WITH (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}

// indexTableHintSQL ports indextablehint_sql (generator.py:2366-2370): MySQL
// `USE|FORCE|IGNORE INDEX [FOR <target>] (...)`.
func (g *Generator) indexTableHintSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this") + " INDEX"
	target := g.sqlKey(e, "target")
	if target != "" {
		target = " FOR " + target
	}
	return this + target + " (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}
