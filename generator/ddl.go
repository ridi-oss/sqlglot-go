package generator

import (
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

// init registers the ALTER TABLE/VIEW/INDEX and DROP node family (ddl.py:241-401), plus
// ColumnPosition/AddPartition/DropPartition (query.py:498,1941,1949); see the Kind block
// comment in expressions/kinds.go for the full upstream reference.
func init() {
	dispatch[expressions.KindAlter] = (*Generator).alterSQL
	dispatch[expressions.KindDrop] = (*Generator).dropSQL
	dispatch[expressions.KindAlterColumn] = (*Generator).alterColumnSQL
	dispatch[expressions.KindModifyColumn] = (*Generator).modifyColumnSQL
	dispatch[expressions.KindAlterIndex] = (*Generator).alterIndexSQL
	dispatch[expressions.KindRenameColumn] = (*Generator).renameColumnSQL
	dispatch[expressions.KindRenameIndex] = (*Generator).renameIndexSQL
	dispatch[expressions.KindAlterRename] = (*Generator).alterRenameSQL
	dispatch[expressions.KindAlterSet] = (*Generator).alterSetSQL
	dispatch[expressions.KindAddConstraint] = (*Generator).addConstraintSQL
	dispatch[expressions.KindDropPartition] = (*Generator).dropPartitionSQL
	dispatch[expressions.KindAddPartition] = (*Generator).addPartitionSQL
	dispatch[expressions.KindDropPrimaryKey] = func(g *Generator, e expressions.Expression) string {
		return "DROP PRIMARY KEY"
	}
}

// dropSQL ports drop_sql (generator.py:1796-1818).
//
// Divergence: upstream rewrites `kind` through `self.dialect.INVERSE_CREATABLE_KIND_MAPPING`
// (a per-dialect CREATABLE_KIND_MAPPING inverse, e.g. Postgres's `WAREHOUSE` -> `DATABASE`
// remaps); that table isn't ported (out of scope for this slice, base/mysql/postgres don't
// populate it for anything DROP-relevant), so `kind` is used as-is - a no-op equivalent to
// `.get(kind) or kind` when the mapping is empty.
func (g *Generator) dropSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = " (" + exprs + ")"
	}
	kind := e.Text("kind")

	iceberg := ""
	if boolValue(e.Arg("iceberg")) && g.dialect.SupportsDropAlterIcebergProperty {
		iceberg = " ICEBERG"
	}
	existsSQL := " "
	if boolValue(e.Arg("exists")) {
		existsSQL = " IF EXISTS "
	}
	concurrentlySQL := ""
	if boolValue(e.Arg("concurrently")) {
		concurrentlySQL = " CONCURRENTLY"
	}
	onCluster := g.sqlKey(e, "cluster")
	if onCluster != "" {
		onCluster = " " + onCluster
	}
	temporary := ""
	if boolValue(e.Arg("temporary")) {
		temporary = " TEMPORARY"
	}
	materialized := ""
	if boolValue(e.Arg("materialized")) {
		materialized = " MATERIALIZED"
	}
	cascade := ""
	if boolValue(e.Arg("cascade")) {
		cascade = " CASCADE"
	}
	restrict := ""
	if boolValue(e.Arg("restrict")) {
		restrict = " RESTRICT"
	}
	constraints := ""
	if boolValue(e.Arg("constraints")) {
		constraints = " CONSTRAINTS"
	}
	purge := ""
	if boolValue(e.Arg("purge")) {
		purge = " PURGE"
	}
	sync := ""
	if boolValue(e.Arg("sync")) {
		sync = " SYNC"
	}
	return "DROP" + temporary + materialized + iceberg + " " + kind + concurrentlySQL + existsSQL + this + onCluster + exprs + cascade + restrict + constraints + purge + sync
}

// alterColumnSQL ports altercolumn_sql (generator.py:4157-4191) and the MySQL override
// (generators/mysql.py:756-762): when a `dtype` is set, MySQL rewrites the whole statement
// to `MODIFY COLUMN <this> <dtype>` instead of `ALTER COLUMN <this> SET DATA TYPE <dtype>`.
func (g *Generator) alterColumnSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	dtype := g.sqlKey(e, "dtype")

	if dtype != "" {
		if g.dialect.Name == "mysql" {
			return "MODIFY COLUMN " + this + " " + dtype
		}

		collate := g.sqlKey(e, "collate")
		if collate != "" {
			collate = " COLLATE " + collate
		}
		using := g.sqlKey(e, "using")
		if using != "" {
			using = " USING " + using
		}
		alterSetType := ""
		if g.dialect.AlterSetType != "" {
			alterSetType = g.dialect.AlterSetType + " "
		}
		return "ALTER COLUMN " + this + " " + alterSetType + dtype + collate + using
	}

	if def := g.sqlKey(e, "default"); def != "" {
		return "ALTER COLUMN " + this + " SET DEFAULT " + def
	}

	if comment := g.sqlKey(e, "comment"); comment != "" {
		return "ALTER COLUMN " + this + " COMMENT " + comment
	}

	if visible := e.Arg("visible"); truthy(visible) {
		return "ALTER COLUMN " + this + " SET " + stringValue(visible)
	}

	allowNull := e.Arg("allow_null")
	drop := e.Arg("drop")
	if !boolValue(drop) && !boolValue(allowNull) {
		g.unsupported("Unsupported ALTER COLUMN syntax")
	}
	if allowNull != nil {
		keyword := "SET"
		if boolValue(drop) {
			keyword = "DROP"
		}
		return "ALTER COLUMN " + this + " " + keyword + " NOT NULL"
	}
	return "ALTER COLUMN " + this + " DROP DEFAULT"
}

// modifyColumnSQL ports modifycolumn_sql (generator.py:4193-4202).
func (g *Generator) modifyColumnSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	renameFrom := g.sqlKey(e, "rename_from")
	if renameFrom != "" {
		if !g.dialect.SupportsChangeColumn {
			g.unsupported("CHANGE COLUMN is not supported in this dialect")
		}
		return "CHANGE COLUMN " + renameFrom + " " + this
	}
	if !g.dialect.SupportsModifyColumn {
		g.unsupported("MODIFY COLUMN is not supported in this dialect")
	}
	return "MODIFY COLUMN " + this
}

// alterIndexSQL ports alterindex_sql (generator.py:4204-4210).
func (g *Generator) alterIndexSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	visibleSQL := "INVISIBLE"
	if boolValue(e.Arg("visible")) {
		visibleSQL = "VISIBLE"
	}
	return "ALTER INDEX " + this + " " + visibleSQL
}

// alterRenameSQL ports alterrename_sql (generator.py:4225-4233) and the MySQL/Doris/
// StarRocks override (generators/mysql.py:750-754), which drops the `TO` keyword.
func (g *Generator) alterRenameSQL(e expressions.Expression) string {
	if !g.dialect.RenameTableWithDB {
		// Remove db from tables.
		for _, tbl := range e.FindAll(expressions.KindTable) {
			tbl.Replace(expressions.Table(expressions.Args{"this": tbl.This()}))
		}
	}
	this := g.sqlKey(e, "this")
	toKW := " TO"
	if g.dialect.Name == "mysql" {
		toKW = ""
	}
	return "RENAME" + toKW + " " + this
}

// renameColumnSQL ports renamecolumn_sql (generator.py:4235-4239).
func (g *Generator) renameColumnSQL(e expressions.Expression) string {
	exists := ""
	if boolValue(e.Arg("exists")) {
		exists = " IF EXISTS"
	}
	oldColumn := g.sqlKey(e, "this")
	newColumn := g.sqlKey(e, "to")
	return "RENAME COLUMN" + exists + " " + oldColumn + " TO " + newColumn
}

// renameIndexSQL ports renameindex_sql (generator.py:6228-6231).
func (g *Generator) renameIndexSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	to := g.sqlKey(e, "to")
	return "RENAME INDEX " + this + " TO " + to
}

// alterSetSQL ports alterset_sql (generator.py:4241-4246) and the Postgres override
// (generators/postgres.py:465-475).
func (g *Generator) alterSetSQL(e expressions.Expression) string {
	exprs := g.expressions(exprsOptions{expression: e, flat: true})

	if g.dialect.Name == "postgres" {
		if exprs != "" {
			exprs = "(" + exprs + ")"
		}
		accessMethod := g.sqlKey(e, "access_method")
		if accessMethod != "" {
			accessMethod = "ACCESS METHOD " + accessMethod
		}
		tablespace := g.sqlKey(e, "tablespace")
		if tablespace != "" {
			tablespace = "TABLESPACE " + tablespace
		}
		option := g.sqlKey(e, "option")
		return "SET " + exprs + accessMethod + tablespace + option
	}

	if g.dialect.AlterSetWrapped {
		exprs = "(" + exprs + ")"
	}
	return "SET " + exprs
}

// alterSQL ports alter_sql (generator.py:4248-4292), dispatching ColumnDef/Schema actions
// through addColumnSQL, gated by dialect.AlterTableAddRequiredForEachColumn (the ADD-
// shortcut branch) and dialect.AlterTableIncludeColumnKeyword (inside addColumnSQL).
func (g *Generator) alterSQL(e expressions.Expression) string {
	actions := listFromValue(e.Arg("actions"))

	useAddShortcut := false
	if !g.dialect.AlterTableAddRequiredForEachColumn && len(actions) > 0 {
		if first := asExpression(actions[0]); first != nil && first.Kind() == expressions.KindColumnDef {
			useAddShortcut = true
		}
	}

	var actionsSQL string
	if useAddShortcut {
		actionsSQL = "ADD " + g.expressions(exprsOptions{expression: e, key: "actions", flat: true})
	} else {
		parts := make([]any, 0, len(actions))
		for _, item := range actions {
			action := asExpression(item)
			if action == nil {
				continue
			}
			var actionSQL string
			if action.Kind() == expressions.KindColumnDef || action.Kind() == expressions.KindSchema {
				actionSQL = g.addColumnSQL(action)
			} else {
				actionSQL = g.gen(action)
				if action.Is(expressions.TraitQuery) {
					actionSQL = "AS " + actionSQL
				}
			}
			parts = append(parts, actionSQL)
		}
		actionsSQL = strings.TrimLeft(g.formatArgs(parts, ", "), "\n")
	}

	iceberg := ""
	if boolValue(e.Arg("iceberg")) && g.dialect.SupportsDropAlterIcebergProperty {
		iceberg = "ICEBERG "
	}
	exists := ""
	if boolValue(e.Arg("exists")) {
		exists = " IF EXISTS"
	}
	onCluster := g.sqlKey(e, "cluster")
	if onCluster != "" {
		onCluster = " " + onCluster
	}
	only := ""
	if boolValue(e.Arg("only")) {
		only = " ONLY"
	}
	options := g.expressions(exprsOptions{expression: e, key: "options"})
	if options != "" {
		options = ", " + options
	}
	kind := g.sqlKey(e, "kind")
	notValid := ""
	if boolValue(e.Arg("not_valid")) {
		notValid = " NOT VALID"
	}
	check := ""
	if boolValue(e.Arg("check")) {
		check = " WITH CHECK"
	}
	cascade := ""
	if boolValue(e.Arg("cascade")) && g.dialect.AlterTableSupportsCascade {
		cascade = " CASCADE"
	}
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}

	return "ALTER " + iceberg + kind + exists + only + this + onCluster + check + g.sep() + actionsSQL + notValid + options + cascade
}

// addColumnSQL ports add_column_sql (generator.py:4299-4308).
func (g *Generator) addColumnSQL(e expressions.Expression) string {
	sql := g.gen(e)
	columnText := ""
	switch e.Kind() {
	case expressions.KindSchema:
		columnText = " COLUMNS"
	case expressions.KindColumnDef:
		if g.dialect.AlterTableIncludeColumnKeyword {
			columnText = " COLUMN"
		}
	}
	return "ADD" + columnText + " " + sql
}

// dropPartitionSQL ports droppartition_sql (generator.py:4310-4313).
func (g *Generator) dropPartitionSQL(e expressions.Expression) string {
	exprs := g.expressions(exprsOptions{expression: e})
	exists := " "
	if boolValue(e.Arg("exists")) {
		exists = " IF EXISTS "
	}
	return "DROP" + exists + exprs
}

// addConstraintSQL ports addconstraint_sql (generator.py:4318-4319).
func (g *Generator) addConstraintSQL(e expressions.Expression) string {
	return "ADD " + g.expressions(exprsOptions{expression: e, noIndent: true})
}

// addPartitionSQL ports addpartition_sql (generator.py:4321-4325).
func (g *Generator) addPartitionSQL(e expressions.Expression) string {
	exists := ""
	if boolValue(e.Arg("exists")) {
		exists = "IF NOT EXISTS "
	}
	location := g.sqlKey(e, "location")
	if location != "" {
		location = " " + location
	}
	return "ADD " + exists + g.sqlKey(e, "this") + location
}
