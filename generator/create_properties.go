package generator

import (
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

// propertyLocation mirrors exp.Properties.Location (expressions/properties.py:572-586).
// Only locations reached by the base, MySQL, and Postgres CREATE paths are needed here, but
// POST_INDEX remains explicit because createSQL preserves its upstream assembly position.
type propertyLocation uint8

const (
	propertyLocationUnsupported propertyLocation = iota
	propertyLocationPostCreate
	propertyLocationPostSchema
	propertyLocationPostWith
	propertyLocationPostAlias
	propertyLocationPostExpression
	propertyLocationPostIndex
)

// propertyLocations is the worklist-scoped port of Generator.PROPERTIES_LOCATION
// (generator.py:672-787). PartitionedByProperty is dialect-dependent: Postgres uses
// PARTITION BY after the schema, while base uses PARTITIONED_BY= inside WITH (...).
var propertyLocations = map[expressions.Kind]propertyLocation{
	expressions.KindAlgorithmProperty:          propertyLocationPostCreate,
	expressions.KindDefinerProperty:            propertyLocationPostCreate,
	expressions.KindMaterializedProperty:       propertyLocationPostCreate,
	expressions.KindTemporaryProperty:          propertyLocationPostCreate,
	expressions.KindUnloggedProperty:           propertyLocationPostCreate,
	expressions.KindAutoIncrementProperty:      propertyLocationPostSchema,
	expressions.KindCalledOnNullInputProperty:  propertyLocationPostSchema,
	expressions.KindCharacterSetProperty:       propertyLocationPostSchema,
	expressions.KindCollateProperty:            propertyLocationPostSchema,
	expressions.KindEngineProperty:             propertyLocationPostSchema,
	expressions.KindInheritsProperty:           propertyLocationPostSchema,
	expressions.KindLanguageProperty:           propertyLocationPostSchema,
	expressions.KindLikeProperty:               propertyLocationPostSchema,
	expressions.KindLockProperty:               propertyLocationPostSchema,
	expressions.KindPartitionByListProperty:    propertyLocationPostSchema,
	expressions.KindPartitionByRangeProperty:   propertyLocationPostSchema,
	expressions.KindPartitionedOfProperty:      propertyLocationPostSchema,
	expressions.KindReturnsProperty:            propertyLocationPostSchema,
	expressions.KindRowFormatDelimitedProperty: propertyLocationPostSchema,
	expressions.KindRowFormatProperty:          propertyLocationPostSchema,
	expressions.KindRowFormatSerdeProperty:     propertyLocationPostSchema,
	expressions.KindSchemaCommentProperty:      propertyLocationPostSchema,
	expressions.KindSerdeProperties:            propertyLocationPostSchema,
	expressions.KindSetConfigProperty:          propertyLocationPostSchema,
	expressions.KindSqlReadWriteProperty:       propertyLocationPostSchema,
	expressions.KindSqlSecurityProperty:        propertyLocationPostSchema,
	expressions.KindStabilityProperty:          propertyLocationPostSchema,
	expressions.KindStrictProperty:             propertyLocationPostSchema,
	expressions.KindFileFormatProperty:         propertyLocationPostWith,
	expressions.KindProperty:                   propertyLocationPostWith,
	expressions.KindLockingProperty:            propertyLocationPostAlias,
	expressions.KindNoPrimaryIndexProperty:     propertyLocationPostExpression,
	expressions.KindOnCommitProperty:           propertyLocationPostExpression,
	expressions.KindTriggerProperties:          propertyLocationPostExpression,
	expressions.KindWithDataProperty:           propertyLocationPostExpression,
}

// propertyNames is the worklist-scoped port of Properties.PROPERTY_TO_NAME. Kinds with
// dedicated renderers (CharacterSet/FileFormat/RowFormat and PartitionedBy on Postgres) do
// not need duplicate entries here.
var propertyNames = map[expressions.Kind]string{
	expressions.KindAlgorithmProperty:     "ALGORITHM",
	expressions.KindAutoIncrementProperty: "AUTO_INCREMENT",
	expressions.KindCollateProperty:       "COLLATE",
	expressions.KindDefinerProperty:       "DEFINER",
	expressions.KindEngineProperty:        "ENGINE",
	expressions.KindLockProperty:          "LOCK",
	expressions.KindSchemaCommentProperty: "COMMENT",
	expressions.KindPartitionedByProperty: "PARTITIONED_BY",
}

func init() {
	dispatch[expressions.KindProperty] = (*Generator).propertySQL
	dispatch[expressions.KindAlgorithmProperty] = (*Generator).propertySQL
	dispatch[expressions.KindAutoIncrementProperty] = (*Generator).propertySQL
	dispatch[expressions.KindCollateProperty] = (*Generator).propertySQL
	dispatch[expressions.KindDefinerProperty] = (*Generator).propertySQL
	dispatch[expressions.KindEngineProperty] = (*Generator).propertySQL
	dispatch[expressions.KindLockProperty] = (*Generator).propertySQL
	dispatch[expressions.KindSchemaCommentProperty] = (*Generator).propertySQL
	dispatch[expressions.KindTemporaryProperty] = (*Generator).temporaryPropertySQL
	dispatch[expressions.KindMaterializedProperty] = (*Generator).materializedPropertySQL
	dispatch[expressions.KindUnloggedProperty] = (*Generator).unloggedPropertySQL
	dispatch[expressions.KindInheritsProperty] = (*Generator).inheritsPropertySQL
	dispatch[expressions.KindLikeProperty] = (*Generator).likePropertySQL
	dispatch[expressions.KindNoPrimaryIndexProperty] = (*Generator).noPrimaryIndexPropertySQL
	dispatch[expressions.KindOnCommitProperty] = (*Generator).onCommitPropertySQL
	dispatch[expressions.KindSqlReadWriteProperty] = (*Generator).sqlReadWritePropertySQL
	dispatch[expressions.KindLockingProperty] = (*Generator).lockingPropertySQL
	dispatch[expressions.KindPartitionedByProperty] = (*Generator).partitionedByPropertySQL
	dispatch[expressions.KindPartitionBoundSpec] = (*Generator).partitionBoundSpecSQL
	dispatch[expressions.KindPartitionedOfProperty] = (*Generator).partitionedOfPropertySQL
	dispatch[expressions.KindWithDataProperty] = (*Generator).withDataPropertySQL
}

func (g *Generator) propertyLocation(property expressions.Expression) (propertyLocation, bool) {
	if property.Kind() == expressions.KindPartitionedByProperty {
		switch g.dialect.Name {
		case "postgres":
			return propertyLocationPostSchema, true
		case "mysql":
			return propertyLocationUnsupported, true
		default:
			return propertyLocationPostWith, true
		}
	}

	// generators/mysql.py:630-640 moves SQL SECURITY from POST_SCHEMA to POST_CREATE for
	// views only; functions and procedures retain the base POST_SCHEMA placement.
	if property.Kind() == expressions.KindSqlSecurityProperty && g.dialect.Name == "mysql" {
		properties := property.Parent()
		if properties != nil {
			create := properties.Parent()
			if create != nil && create.Kind() == expressions.KindCreate && create.Text("kind") == "VIEW" {
				return propertyLocationPostCreate, true
			}
		}
	}

	location, ok := propertyLocations[property.Kind()]
	return location, ok
}

// locateProperties ports locate_properties (generator.py:2067-2076). Unknown properties are
// fail-closed: report them as unsupported and omit them instead of guessing a placement.
func (g *Generator) locateProperties(properties expressions.Expression) map[propertyLocation][]expressions.Expression {
	locations := map[propertyLocation][]expressions.Expression{}
	if properties == nil {
		return locations
	}
	for _, property := range properties.Expressions() {
		location, ok := g.propertyLocation(property)
		if !ok || location == propertyLocationUnsupported {
			g.unsupported("Unsupported property " + strings.ToLower(expressions.ClassName(property.Kind())))
			continue
		}
		locations[location] = append(locations[location], property)
	}
	return locations
}

func (g *Generator) propertiesExpression(items []expressions.Expression, parent expressions.Expression) expressions.Expression {
	properties := expressions.Properties(expressions.Args{"expressions": items})
	if parent != nil {
		properties.SetParent(parent, "properties", -1)
	}
	return properties
}

// rootPropertiesSQL ports root_properties (generator.py:2044-2047).
func (g *Generator) rootPropertiesSQL(properties expressions.Expression) string {
	if properties == nil || len(properties.Expressions()) == 0 {
		return ""
	}
	return g.expressions(exprsOptions{expression: properties, noIndent: true, sep: " "})
}

// renderProperties ports properties (generator.py:2049-2062).
func (g *Generator) renderProperties(properties expressions.Expression, prefix, sep, suffix string, wrapped bool) string {
	if properties == nil || len(properties.Expressions()) == 0 {
		return ""
	}
	sql := g.expressions(exprsOptions{expression: properties, noIndent: true, sep: sep})
	if sql == "" {
		return ""
	}
	if wrapped {
		sql = g.wrap(sql)
	}
	space := ""
	if strings.TrimSpace(prefix) != "" {
		space = " "
	}
	return prefix + space + sql + suffix
}

// withPropertiesSQL ports with_properties (generator.py:2064-2065). Base, MySQL, and
// Postgres all use the base WITH_PROPERTIES_PREFIX of "WITH".
func (g *Generator) withPropertiesSQL(properties expressions.Expression) string {
	return g.renderProperties(properties, g.seg("WITH", ""), ", ", "", true)
}

// propertyName ports property_name (generator.py:2078-2081).
func (g *Generator) propertyName(e expressions.Expression, stringKey bool) string {
	if this := asExpression(e.Arg("this")); this != nil && this.Kind() == expressions.KindDot {
		return g.sqlKey(e, "this")
	}
	name := e.Name()
	if stringKey {
		return "'" + strings.ReplaceAll(name, "'", "''") + "'"
	}
	return name
}

// propertySQL ports property_sql (generator.py:2083-2092).
func (g *Generator) propertySQL(e expressions.Expression) string {
	if e.Kind() == expressions.KindProperty {
		return g.propertyName(e, false) + "=" + g.sqlKey(e, "value")
	}
	if e.Kind() == expressions.KindSchemaCommentProperty && g.dialect.Name == "postgres" {
		g.unsupported("Table comments are not supported in the CREATE statement")
		return ""
	}
	name, ok := propertyNames[e.Kind()]
	if !ok {
		g.unsupported("Unsupported property " + strings.ToLower(expressions.ClassName(e.Kind())))
		return ""
	}
	return name + "=" + g.sqlKey(e, "this")
}

func (g *Generator) temporaryPropertySQL(expressions.Expression) string    { return "TEMPORARY" }
func (g *Generator) materializedPropertySQL(expressions.Expression) string { return "MATERIALIZED" }
func (g *Generator) unloggedPropertySQL(expressions.Expression) string     { return "UNLOGGED" }

func (g *Generator) inheritsPropertySQL(e expressions.Expression) string {
	return "INHERITS (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}

// likePropertySQL ports likeproperty_sql (generator.py:2097-2112). All in-scope dialects
// support CREATE TABLE LIKE; Postgres requires a LIKE property outside Schema to be wrapped.
func (g *Generator) likePropertySQL(e expressions.Expression) string {
	options := make([]string, 0, len(e.Expressions()))
	for _, option := range e.Expressions() {
		optionSQL := strings.TrimSpace(option.Name() + " " + g.sqlKey(option, "value"))
		if optionSQL != "" {
			options = append(options, optionSQL)
		}
	}
	optionsSQL := ""
	if len(options) > 0 {
		optionsSQL = " " + strings.Join(options, " ")
	}
	like := "LIKE " + g.sqlKey(e, "this") + optionsSQL
	if g.dialect.Name == "postgres" && (e.Parent() == nil || e.Parent().Kind() != expressions.KindSchema) {
		like = "(" + like + ")"
	}
	return like
}

func (g *Generator) noPrimaryIndexPropertySQL(expressions.Expression) string {
	return "NO PRIMARY INDEX"
}

func (g *Generator) onCommitPropertySQL(e expressions.Expression) string {
	rows := "PRESERVE"
	if boolValue(e.Arg("delete")) {
		rows = "DELETE"
	}
	return "ON COMMIT " + rows + " ROWS"
}

func (g *Generator) sqlReadWritePropertySQL(e expressions.Expression) string { return e.Name() }

func (g *Generator) lockingPropertySQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	forOrIn := stringValue(e.Arg("for_or_in"))
	if forOrIn != "" {
		forOrIn = " " + forOrIn
	}
	override := ""
	if boolValue(e.Arg("override")) {
		override = " OVERRIDE"
	}
	return "LOCKING " + stringValue(e.Arg("kind")) + this + forOrIn + " " + stringValue(e.Arg("lock_type")) + override
}

func (g *Generator) partitionedByPropertySQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return "PARTITION BY " + g.sqlKey(e, "this")
	}
	return propertyNames[expressions.KindPartitionedByProperty] + "=" + g.sqlKey(e, "this")
}

func (g *Generator) partitionBoundSpecSQL(e expressions.Expression) string {
	switch e.Arg("this").(type) {
	case []expressions.Expression, []any:
		return "IN (" + g.expressions(exprsOptions{sqls: listFromValue(e.Arg("this")), flat: true}) + ")"
	}
	if this := g.sqlKey(e, "this"); this != "" {
		return "WITH (MODULUS " + this + ", REMAINDER " + g.sqlKey(e, "expression") + ")"
	}
	fromExpressions := g.expressions(exprsOptions{expression: e, key: "from_expressions", flat: true})
	toExpressions := g.expressions(exprsOptions{expression: e, key: "to_expressions", flat: true})
	return "FROM (" + fromExpressions + ") TO (" + toExpressions + ")"
}

func (g *Generator) partitionedOfPropertySQL(e expressions.Expression) string {
	bound := " DEFAULT"
	if expression := asExpression(e.Arg("expression")); expression != nil && expression.Kind() == expressions.KindPartitionBoundSpec {
		bound = " FOR VALUES " + g.gen(expression)
	}
	return "PARTITION OF " + g.sqlKey(e, "this") + bound
}

func (g *Generator) withDataPropertySQL(e expressions.Expression) string {
	no := ""
	if boolValue(e.Arg("no")) {
		no = "NO "
	}
	sql := "WITH " + no + "DATA"
	if statistics := e.Arg("statistics"); statistics != nil {
		statisticsNo := ""
		if !boolValue(statistics) {
			statisticsNo = "NO "
		}
		sql += " AND " + statisticsNo + "STATISTICS"
	}
	return sql
}
