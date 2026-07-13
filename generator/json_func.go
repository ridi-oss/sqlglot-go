package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindJSONObject] = (*Generator).jsonObjectSQL
	dispatch[expressions.KindJSONObjectAgg] = (*Generator).jsonObjectSQL
	dispatch[expressions.KindJSONKeyValue] = (*Generator).jsonKeyValueSQL
	dispatch[expressions.KindOnCondition] = (*Generator).onConditionSQL
	dispatch[expressions.KindJSONValue] = (*Generator).jsonValueSQL
}

// jsonObjectSQL ports _jsonobject_sql (generator.py:3788-3812), shared by JSON_OBJECT and
// JSON_OBJECTAGG (registered separately in dispatch above, matching upstream's TRANSFORMS
// entries at generator.py:210-211 - both map to the same private method).
func (g *Generator) jsonObjectSQL(e expressions.Expression) string {
	nullHandling := g.sqlKey(e, "null_handling")
	if nullHandling != "" {
		nullHandling = " " + nullHandling
	}

	var uniqueKeys string
	if v, ok := e.Arg("unique_keys").(bool); ok {
		if v {
			uniqueKeys = " WITH UNIQUE KEYS"
		} else {
			uniqueKeys = " WITHOUT UNIQUE KEYS"
		}
	}

	returnType := g.sqlKey(e, "return_type")
	if returnType != "" {
		returnType = " RETURNING " + returnType
	}
	encoding := g.sqlKey(e, "encoding")
	if encoding != "" {
		encoding = " ENCODING " + encoding
	}

	name := "JSON_OBJECT"
	if e.Kind() == expressions.KindJSONObjectAgg {
		name = "JSON_OBJECTAGG"
		// Postgres renders JSONObjectAgg as JSON_OBJECT_AGG (generators/postgres.py:389,
		// rename_func("JSON_OBJECT_AGG")); base/mysql keep the JSON_OBJECTAGG spelling.
		if g.dialect.Name == "postgres" {
			name = "JSON_OBJECT_AGG"
		}
	}

	suffix := nullHandling + uniqueKeys + returnType + encoding + ")"
	return g.funcCall(name, listFromValue(e.Arg("expressions")), "(", suffix, true)
}

// jsonKeyValueSQL ports jsonkeyvalue_sql (generator.py:3746-3747). JSON_KEY_VALUE_PAIR_SEP
// is ":" by upstream base default (generator.py:460); MySQL overrides it to "," (generators/
// mysql.py:144) - Postgres has no override, so it keeps the base colon.
func (g *Generator) jsonKeyValueSQL(e expressions.Expression) string {
	sep := ":"
	if g.dialect.Name == "mysql" {
		sep = ","
	}
	return g.sqlKey(e, "this") + sep + " " + g.sqlKey(e, "expression")
}

// onConditionSQL ports oncondition_sql (generator.py:5577-5603): renders the trailing
// "<empty> <error> <null>" clause shared by JSON_VALUE/JSON_TABLE/JSON_EXISTS. Static
// options ("NULL ON ERROR") are stored as plain strings; "DEFAULT <expr> ON <on>" is stored
// as the raw <expr> Expression, so onConditionPart re-adds the DEFAULT/ON wrapper around it.
func (g *Generator) onConditionSQL(e expressions.Expression) string {
	empty := g.onConditionPart(e, "empty", "EMPTY")
	error_ := g.onConditionPart(e, "error", "ERROR")

	// ON_CONDITION_EMPTY_BEFORE_ERROR is true for base/mysql/postgres alike (dialect.py:654),
	// so when both are set, EMPTY always precedes ERROR here (unlike upstream's
	// dialect-conditional order).
	if error_ != "" && empty != "" {
		error_ = empty + " " + error_
		empty = ""
	}

	null := g.sqlKey(e, "null")

	return empty + error_ + null
}

func (g *Generator) onConditionPart(e expressions.Expression, key, label string) string {
	if expr, ok := e.Arg(key).(expressions.Expression); ok && !isNilExpression(expr) {
		return "DEFAULT " + g.gen(expr) + " ON " + label
	}
	return g.sqlKey(e, key)
}

// jsonValueSQL ports jsonvalue_sql (generator.py:5550-5558): JSON_VALUE(<this>, <path>
// [RETURNING <type>] [<on_condition>]).
func (g *Generator) jsonValueSQL(e expressions.Expression) string {
	path := g.sqlKey(e, "path")
	returning := g.sqlKey(e, "returning")
	if returning != "" {
		returning = " RETURNING " + returning
	}
	onCondition := g.sqlKey(e, "on_condition")
	if onCondition != "" {
		onCondition = " " + onCondition
	}
	return g.funcCall("JSON_VALUE", []any{e.Arg("this"), path + returning + onCondition}, "(", ")", true)
}
