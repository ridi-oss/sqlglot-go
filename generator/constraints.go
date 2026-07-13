package generator

import (
	"strings"
	"unicode"

	"github.com/ridi/sqlglot-go/expressions"
)

// init registers the CREATE-table constraint node family (see expressions/kinds.go's block
// comment for the upstream constraints.py line references) plus the table/column-level
// Constraint/ForeignKey/PrimaryKey/Reference nodes they compose with.
func init() {
	dispatch[expressions.KindColumnConstraint] = (*Generator).columnConstraintSQL
	dispatch[expressions.KindConstraint] = (*Generator).constraintSQL
	dispatch[expressions.KindPrimaryKey] = (*Generator).primaryKeySQL
	dispatch[expressions.KindPrimaryKeyColumnConstraint] = (*Generator).primaryKeyColumnConstraintSQL
	dispatch[expressions.KindForeignKey] = (*Generator).foreignKeySQL
	dispatch[expressions.KindReference] = (*Generator).referenceSQL
	dispatch[expressions.KindNotNullColumnConstraint] = (*Generator).notNullColumnConstraintSQL
	dispatch[expressions.KindUniqueColumnConstraint] = (*Generator).uniqueColumnConstraintSQL
	dispatch[expressions.KindCheckColumnConstraint] = (*Generator).checkColumnConstraintSQL
	dispatch[expressions.KindAutoIncrementColumnConstraint] = (*Generator).autoIncrementColumnConstraintSQL
	dispatch[expressions.KindGeneratedAsIdentityColumnConstraint] = (*Generator).generatedAsIdentityColumnConstraintSQL
	dispatch[expressions.KindGeneratedAsRowColumnConstraint] = (*Generator).generatedAsRowColumnConstraintSQL
	dispatch[expressions.KindComputedColumnConstraint] = (*Generator).computedColumnConstraintSQL
	dispatch[expressions.KindIndexColumnConstraint] = (*Generator).indexColumnConstraintSQL
	dispatch[expressions.KindIndexConstraintOption] = (*Generator).indexConstraintOptionSQL
	dispatch[expressions.KindIndexParameters] = (*Generator).indexParametersSQL
	dispatch[expressions.KindColumnPrefix] = (*Generator).columnPrefixSQL
	dispatch[expressions.KindColumnPosition] = (*Generator).columnPositionSQL
	dispatch[expressions.KindCompressColumnConstraint] = (*Generator).compressColumnConstraintSQL
	dispatch[expressions.KindExcludeColumnConstraint] = (*Generator).excludeColumnConstraintSQL
	dispatch[expressions.KindWithOperator] = (*Generator).withOperatorSQL
	dispatch[expressions.KindInOutColumnConstraint] = (*Generator).inOutColumnConstraintSQL

	// TRANSFORMS one-liners (generator.py:149-293, exact lambdas transcribed verbatim).
	dispatch[expressions.KindCollateColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "COLLATE " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindCommentColumnConstraint] = (*Generator).commentColumnConstraintSQL
	dispatch[expressions.KindDefaultColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "DEFAULT " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindCharacterSetColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "CHARACTER SET " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindDateFormatColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "FORMAT " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindInlineLengthColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "INLINE LENGTH " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindTitleColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "TITLE " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindUppercaseColumnConstraint] = func(_ *Generator, _ expressions.Expression) string {
		return "UPPERCASE"
	}
	dispatch[expressions.KindOnUpdateColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "ON UPDATE " + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindZeroFillColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "ZEROFILL"
	}
	dispatch[expressions.KindInvisibleColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "INVISIBLE"
	}
	dispatch[expressions.KindNotForReplicationColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		return "NOT FOR REPLICATION"
	}
	dispatch[expressions.KindCaseSpecificColumnConstraint] = func(g *Generator, e expressions.Expression) string {
		not := ""
		if boolValue(e.Arg("not_")) {
			not = "NOT "
		}
		return not + "CASESPECIFIC"
	}
}

// commentColumnConstraintSQL ports the base CommentColumnConstraint transform
// (generator.py:162, `COMMENT {this}`) plus the Postgres override (generators/postgres.py:405-407),
// which treats column comments inside CREATE as unsupported and emits nothing (the empty result is
// filtered out by g.expressions' flat join, so the column renders with no trailing space).
func (g *Generator) commentColumnConstraintSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		g.unsupported("Column comments are not supported in the CREATE statement")
		return ""
	}
	return "COMMENT " + g.sqlKey(e, "this")
}

// compressColumnConstraintSQL ports compresscolumnconstraint_sql (generator.py:1203-1209).
func (g *Generator) compressColumnConstraintSQL(e expressions.Expression) string {
	var this string
	switch e.Arg("this").(type) {
	case []expressions.Expression, []any:
		this = g.wrap(g.expressions(exprsOptions{expression: e, key: "this", flat: true}))
	default:
		this = g.sqlKey(e, "this")
	}
	return "COMPRESS " + this
}

// excludeColumnConstraintSQL ports the ExcludeColumnConstraint TRANSFORM
// (generator.py:190), including the left trim needed because IndexParameters begins with a
// leading space when it has a USING method but no columns prefix.
func (g *Generator) excludeColumnConstraintSQL(e expressions.Expression) string {
	return "EXCLUDE " + strings.TrimLeftFunc(g.sqlKey(e, "this"), unicode.IsSpace)
}

// withOperatorSQL ports the WithOperator TRANSFORM (generator.py:293).
func (g *Generator) withOperatorSQL(e expressions.Expression) string {
	return g.sqlKey(e, "this") + " WITH " + g.sqlKey(e, "op")
}

// inOutColumnConstraintSQL ports inoutcolumnconstraint_sql (generator.py:1279-1295).
// Postgres spells the combined mode INOUT; the base generator uses IN OUT.
func (g *Generator) inOutColumnConstraintSQL(e expressions.Expression) string {
	if boolValue(e.Arg("variadic")) {
		return "VARIADIC"
	}
	input := boolValue(e.Arg("input_"))
	output := boolValue(e.Arg("output"))
	if input && output {
		if g.dialect.Name == "postgres" {
			return "INOUT"
		}
		return "IN OUT"
	}
	if input {
		return "IN"
	}
	if output {
		return "OUT"
	}
	return ""
}

// columnConstraintSQL ports columnconstraint_sql (generator.py:1184-1187).
func (g *Generator) columnConstraintSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	kindSQL := strings.TrimSpace(g.sqlKey(e, "kind"))
	if this != "" {
		return "CONSTRAINT " + this + " " + kindSQL
	}
	return kindSQL
}

// computedColumnConstraintSQL ports computedcolumnconstraint_sql (generator.py:1189-1198),
// the MySQL override (generators/mysql.py:644-646), and the Postgres override
// (generators/postgres.py:511-512).
func (g *Generator) computedColumnConstraintSQL(e expressions.Expression) string {
	switch g.dialect.Name {
	case "mysql":
		persisted := "VIRTUAL"
		if boolValue(e.Arg("persisted")) {
			persisted = "STORED"
		}
		unnestedSQL := ""
		if this := e.This(); this != nil {
			unnestedSQL = g.gen(this.Unnest())
		}
		return "GENERATED ALWAYS AS (" + unnestedSQL + ") " + persisted
	case "postgres":
		return "GENERATED ALWAYS AS (" + g.sqlKey(e, "this") + ") STORED"
	}

	this := g.sqlKey(e, "this")
	persisted := ""
	if boolValue(e.Arg("not_null")) {
		persisted = " PERSISTED NOT NULL"
	} else if boolValue(e.Arg("persisted")) {
		persisted = " PERSISTED"
	}
	return "AS " + this + persisted
}

// autoIncrementColumnConstraintSQL ports autoincrementcolumnconstraint_sql
// (generator.py:1200-1201): `self.token_sql(TokenType.AUTO_INCREMENT)`. The base
// TOKEN_MAPPING is empty and neither MySQL nor Postgres override it, so token_sql just
// falls back to the token's name, "AUTO_INCREMENT" (generator.py:4712-4713).
func (g *Generator) autoIncrementColumnConstraintSQL(e expressions.Expression) string {
	return "AUTO_INCREMENT"
}

// generatedAsIdentityColumnConstraintSQL ports generatedasidentitycolumnconstraint_sql
// (generator.py:1211-1242).
//
// Divergence: upstream interpolates start/increment/minvalue/maxvalue via bare f-string
// (`f"START WITH {start}"`), which calls the Expression's own zero-arg `.sql()` (always the
// base dialect, ignoring the generator's current dialect - see Expr.__str__,
// expressions/core.py:1236-1237). These args are always numeric literals in practice, whose
// rendering doesn't vary by dialect, so we render them with the generator's normal
// dialect-aware g.sqlKey instead of replicating the base-dialect quirk.
func (g *Generator) generatedAsIdentityColumnConstraintSQL(e expressions.Expression) string {
	thisSQL := ""
	if thisArg := e.Arg("this"); thisArg != nil {
		onNull := ""
		if boolValue(e.Arg("on_null")) {
			onNull = " ON NULL"
		}
		if boolValue(thisArg) {
			thisSQL = " ALWAYS"
		} else {
			thisSQL = " BY DEFAULT" + onNull
		}
	}

	start := g.sqlKey(e, "start")
	if start != "" {
		start = "START WITH " + start
	}
	increment := g.sqlKey(e, "increment")
	if increment != "" {
		increment = " INCREMENT BY " + increment
	}
	minvalue := g.sqlKey(e, "minvalue")
	if minvalue != "" {
		minvalue = " MINVALUE " + minvalue
	}
	maxvalue := g.sqlKey(e, "maxvalue")
	if maxvalue != "" {
		maxvalue = " MAXVALUE " + maxvalue
	}

	cycleSQL := ""
	if cycle := e.Arg("cycle"); cycle != nil {
		if boolValue(cycle) {
			cycleSQL = " CYCLE"
		} else {
			cycleSQL = " NO CYCLE"
		}
		if start == "" && increment == "" {
			cycleSQL = strings.TrimSpace(cycleSQL)
		}
	}

	sequenceOpts := ""
	if start != "" || increment != "" || cycleSQL != "" {
		sequenceOpts = " (" + strings.TrimSpace(start+increment+minvalue+maxvalue+cycleSQL) + ")"
	}

	expr := g.sqlKey(e, "expression")
	if expr != "" {
		expr = "(" + expr + ")"
	} else {
		expr = "IDENTITY"
	}

	return "GENERATED" + thisSQL + " AS " + expr + sequenceOpts
}

// generatedAsRowColumnConstraintSQL ports generatedasrowcolumnconstraint_sql
// (generator.py:1244-1249).
func (g *Generator) generatedAsRowColumnConstraintSQL(e expressions.Expression) string {
	start := "END"
	if boolValue(e.Arg("start")) {
		start = "START"
	}
	hidden := ""
	if boolValue(e.Arg("hidden")) {
		hidden = " HIDDEN"
	}
	return "GENERATED ALWAYS AS ROW " + start + hidden
}

// notNullColumnConstraintSQL ports notnullcolumnconstraint_sql (generator.py:1256-1257).
func (g *Generator) notNullColumnConstraintSQL(e expressions.Expression) string {
	if boolValue(e.Arg("allow_null")) {
		return "NULL"
	}
	return "NOT NULL"
}

// primaryKeyColumnConstraintSQL ports primarykeycolumnconstraint_sql
// (generator.py:1259-1265).
func (g *Generator) primaryKeyColumnConstraintSQL(e expressions.Expression) string {
	if desc := e.Arg("desc"); desc != nil {
		if boolValue(desc) {
			return "PRIMARY KEY DESC"
		}
		return "PRIMARY KEY ASC"
	}
	options := g.expressions(exprsOptions{expression: e, key: "options", flat: true, sep: " "})
	if options != "" {
		options = " " + options
	}
	return "PRIMARY KEY" + options
}

// uniqueColumnConstraintSQL ports uniquecolumnconstraint_sql (generator.py:1267-1277).
func (g *Generator) uniqueColumnConstraintSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	indexType := stringValue(e.Arg("index_type"))
	if indexType != "" {
		indexType = " USING " + indexType
	}
	onConflict := g.sqlKey(e, "on_conflict")
	if onConflict != "" {
		onConflict = " " + onConflict
	}
	nullsSQL := ""
	if boolValue(e.Arg("nulls")) {
		nullsSQL = " NULLS NOT DISTINCT"
	}
	options := g.expressions(exprsOptions{expression: e, key: "options", flat: true, sep: " "})
	if options != "" {
		options = " " + options
	}
	return "UNIQUE" + nullsSQL + this + indexType + onConflict + options
}

// checkColumnConstraintSQL ports checkcolumnconstraint_sql (generator.py:4922-4924). Not to
// be confused with check_sql (exp.Check, generator.py:3700-3702) - this is the column-level
// CheckColumnConstraint, which additionally supports the ENFORCED suffix.
func (g *Generator) checkColumnConstraintSQL(e expressions.Expression) string {
	enforced := ""
	if boolValue(e.Arg("enforced")) {
		enforced = " ENFORCED"
	}
	return "CHECK (" + g.sqlKey(e, "this") + ")" + enforced
}

// indexColumnConstraintSQL ports indexcolumnconstraint_sql (generator.py:4926-4937).
func (g *Generator) indexColumnConstraintSQL(e expressions.Expression) string {
	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind = kind + " INDEX"
	} else {
		kind = "INDEX"
	}
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	indexType := g.sqlKey(e, "index_type")
	if indexType != "" {
		indexType = " USING " + indexType
	}
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = " (" + exprs + ")"
	}
	options := g.expressions(exprsOptions{expression: e, key: "options", sep: " "})
	if options != "" {
		options = " " + options
	}
	return kind + this + indexType + exprs + options
}

// indexConstraintOptionSQL ports indexconstraintoption_sql (generator.py:4890-4920).
func (g *Generator) indexConstraintOptionSQL(e expressions.Expression) string {
	if keyBlockSize := g.sqlKey(e, "key_block_size"); keyBlockSize != "" {
		return "KEY_BLOCK_SIZE = " + keyBlockSize
	}
	if using := g.sqlKey(e, "using"); using != "" {
		return "USING " + using
	}
	if parser := g.sqlKey(e, "parser"); parser != "" {
		return "WITH PARSER " + parser
	}
	if comment := g.sqlKey(e, "comment"); comment != "" {
		return "COMMENT " + comment
	}
	if visible := e.Arg("visible"); visible != nil {
		if boolValue(visible) {
			return "VISIBLE"
		}
		return "INVISIBLE"
	}
	if engineAttr := g.sqlKey(e, "engine_attr"); engineAttr != "" {
		return "ENGINE_ATTRIBUTE = " + engineAttr
	}
	if secondaryEngineAttr := g.sqlKey(e, "secondary_engine_attr"); secondaryEngineAttr != "" {
		return "SECONDARY_ENGINE_ATTRIBUTE = " + secondaryEngineAttr
	}
	g.unsupported("Unsupported index constraint option.")
	return ""
}

// columnPrefixSQL ports columnprefix_sql (generator.py:4964-4965).
func (g *Generator) columnPrefixSQL(e expressions.Expression) string {
	return g.sqlKey(e, "this") + "(" + g.sqlKey(e, "expression") + ")"
}

// columnPositionSQL ports columnposition_sql (generator.py:1163-1167).
func (g *Generator) columnPositionSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	position := g.sqlKey(e, "position")
	return position + this
}

// constraintSQL ports constraint_sql (generator.py:3601-3604).
func (g *Generator) constraintSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	return "CONSTRAINT " + this + " " + exprs
}

// foreignKeySQL ports foreignkey_sql (generator.py:3704-3715).
func (g *Generator) foreignKeySQL(e expressions.Expression) string {
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = " (" + exprs + ")"
	}
	reference := g.sqlKey(e, "reference")
	if reference != "" {
		reference = " " + reference
	}
	del := g.sqlKey(e, "delete")
	if del != "" {
		del = " ON DELETE " + del
	}
	update := g.sqlKey(e, "update")
	if update != "" {
		update = " ON UPDATE " + update
	}
	options := g.expressions(exprsOptions{expression: e, key: "options", flat: true, sep: " "})
	if options != "" {
		options = " " + options
	}
	return "FOREIGN KEY" + exprs + reference + del + update + options
}

// primaryKeySQL ports primarykey_sql (generator.py:3717-3724).
func (g *Generator) primaryKeySQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	include := g.sqlKey(e, "include")
	options := g.expressions(exprsOptions{expression: e, key: "options", flat: true, sep: " "})
	if options != "" {
		options = " " + options
	}
	return "PRIMARY KEY" + this + " (" + exprs + ")" + include + options
}

// indexParametersSQL ports indexparameters_sql (generator.py:1926-1944): the trailing index-
// parameter block shared by PRIMARY KEY, CREATE INDEX, and EXCLUDE, including EXCLUDE columns
// and generic WITH (...) storage properties.
func (g *Generator) indexParametersSQL(e expressions.Expression) string {
	using := g.sqlKey(e, "using")
	if using != "" {
		using = " USING " + using
	}
	columns := g.expressions(exprsOptions{expression: e, key: "columns", flat: true})
	if columns != "" {
		columns = "(" + columns + ")"
	}
	partitionBy := g.expressions(exprsOptions{expression: e, key: "partition_by", flat: true})
	if partitionBy != "" {
		partitionBy = " PARTITION BY " + partitionBy
	}
	where := g.sqlKey(e, "where")
	include := g.expressions(exprsOptions{expression: e, key: "include", flat: true})
	if include != "" {
		include = " INCLUDE (" + include + ")"
	}
	withStorage := ""
	if truthy(e.Arg("with_storage")) {
		withStorage = g.expressions(exprsOptions{expression: e, key: "with_storage", flat: true})
		if withStorage != "" {
			withStorage = " WITH (" + withStorage + ")"
		}
	}
	tablespace := g.sqlKey(e, "tablespace")
	if tablespace != "" {
		tablespace = " USING INDEX TABLESPACE " + tablespace
	}
	on := g.sqlKey(e, "on")
	if on != "" {
		on = " ON " + on
	}
	return using + columns + include + withStorage + tablespace + partitionBy + where + on
}

// referenceSQL ports reference_sql (generator.py:3935-3941).
func (g *Generator) referenceSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = "(" + exprs + ")"
	}
	options := g.expressions(exprsOptions{expression: e, key: "options", flat: true, sep: " "})
	if options != "" {
		options = " " + options
	}
	return "REFERENCES " + this + exprs + options
}
