package optimizer

import (
	"fmt"

	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/schema"
)

type sourceColumnsKey struct {
	name        string
	onlyVisible bool
}

type Resolver struct {
	scope   *Scope
	schema  schema.Schema
	dialect *dialects.Dialect

	sourceColumns      map[string][]string
	sourceColumnsOrder []string
	unambiguousColumns map[string]string
	allColumns         map[string]bool
	inferSchema        bool

	getSourceColumnsCache map[sourceColumnsKey][]string
}

func NewResolver(scope *Scope, s schema.Schema, inferSchema bool) *Resolver {
	var d *dialects.Dialect
	if s != nil {
		d = s.Dialect()
	}
	if d == nil {
		d = dialects.Base()
	}
	return &Resolver{
		scope:                 scope,
		schema:                s,
		dialect:               d,
		inferSchema:           inferSchema,
		getSourceColumnsCache: map[sourceColumnsKey][]string{},
	}
}

func (r *Resolver) GetTable(column any) exp.Expression {
	columnName := ""
	if s, ok := column.(string); ok {
		columnName = s
	} else if expression, ok := column.(exp.Expression); ok && expression != nil {
		columnName = expression.Name()
	}

	tableName := r.getTableNameFromSources(columnName, nil, nil)

	if tableName == "" {
		if columnExpr, ok := column.(exp.Expression); ok && columnExpr != nil && columnExpr.Kind() == exp.KindColumn {
			if joinContext := r.getColumnJoinContext(columnExpr); joinContext != nil {
				tableName = r.getTableNameFromJoinContextRecover(columnName, joinContext)
			}
		}
	}

	if tableName == "" && r.inferSchema {
		allColumns := r.getAllSourceColumns()
		var candidates []string
		for _, source := range r.sourceColumnsOrder {
			columns := allColumns[source]
			if len(columns) == 0 || containsString(columns, "*") {
				candidates = append(candidates, source)
			}
		}
		if len(candidates) == 1 {
			tableName = candidates[0]
		}
	}

	if tableName == "" {
		return nil
	}

	selected := r.scope.SelectedSources()
	item, ok := selected[tableName]
	if !ok {
		return exp.ToIdentifier(tableName)
	}

	node := item.Node
	if node != nil && node.Is(exp.TraitQuery) {
		for node != nil && node.Alias() != tableName && node.Parent() != nil {
			node = node.Parent()
		}
	}

	if node != nil {
		nodeAlias := asExpression(node.Arg("alias"))
		if nodeAlias != nil && nodeAlias.This() != nil {
			return exp.ToIdentifier(nodeAlias.This().Copy())
		}
	}

	return exp.ToIdentifier(tableName)
}

func (r *Resolver) AllColumns() map[string]bool {
	if r.allColumns == nil {
		r.allColumns = map[string]bool{}
		for _, columns := range r.getAllSourceColumns() {
			for _, column := range columns {
				r.allColumns[column] = true
			}
		}
	}
	return r.allColumns
}

func (r *Resolver) GetSourceColumnsFromSetOp(expression exp.Expression) []string {
	if expression != nil && expression.Kind() == exp.KindSelect {
		return expression.NamedSelects()
	}
	if expression != nil && expression.Kind() == exp.KindSubquery && expression.This() != nil && expression.This().Is(exp.TraitSetOperation) {
		return r.GetSourceColumnsFromSetOp(expression.This())
	}
	if expression == nil || !expression.Is(exp.TraitSetOperation) {
		panic(&sqlerrors.OptimizeError{Msg: fmt.Sprintf("Unknown set operation: %v", expression)})
	}

	onColumnList := expressionsFor(expression, "on")
	var columns []string
	if len(onColumnList) > 0 {
		for _, col := range onColumnList {
			columns = append(columns, col.Name())
		}
	} else if expression.Text("side") != "" || expression.Text("kind") != "" {
		side := expression.Text("side")
		kind := expression.Text("kind")
		left := r.GetSourceColumnsFromSetOp(expression.Left())
		right := r.GetSourceColumnsFromSetOp(expression.Right())
		switch side {
		case "LEFT":
			columns = left
		case "FULL":
			seen := map[string]bool{}
			for _, column := range append(copyStringSlice(left), right...) {
				if !seen[column] {
					seen[column] = true
					columns = append(columns, column)
				}
			}
		case "":
			if kind == "INNER" {
				rightSet := stringSet(right)
				for _, column := range left {
					if rightSet[column] && !containsString(columns, column) {
						columns = append(columns, column)
					}
				}
			}
		}
	} else {
		columns = expression.NamedSelects()
	}
	return columns
}

func (r *Resolver) GetSourceColumns(name string, onlyVisible ...bool) []string {
	visible := false
	if len(onlyVisible) > 0 {
		visible = onlyVisible[0]
	}
	cacheKey := sourceColumnsKey{name: name, onlyVisible: visible}
	if columns, ok := r.getSourceColumnsCache[cacheKey]; ok {
		return columns
	}

	source, ok := r.scope.Sources[name]
	if !ok {
		panic(&sqlerrors.OptimizeError{Msg: "Unknown table: " + name})
	}

	if table, ok := source.(exp.Expression); ok && table != nil && table.Kind() == exp.KindTable && table.DbName() == "" && len(expressionsFor(table, "pivots")) > 0 {
		if cteSource, ok := r.scope.CTESources[table.Name()]; ok {
			source = cteSource
		}
	}

	var columns []string
	switch source := source.(type) {
	case exp.Expression:
		if source != nil && source.Kind() == exp.KindTable {
			var err error
			columns, err = r.schema.ColumnNames(source, visible, "", nil)
			if err != nil {
				panic(err)
			}
		} else if source != nil {
			columns = source.NamedSelects()
		}
	case *Scope:
		sourceExpr := source.Expression
		if sourceExpr != nil && sourceExpr.Is(exp.TraitUDTF) {
			columns = copyStringSlice(sourceExpr.NamedSelects())
			// TODO(slice 4c): port UNNEST_COLUMN_ONLY / Explode / Lateral Query type-aware branches.
		} else if sourceExpr != nil && sourceExpr.Is(exp.TraitSetOperation) {
			columns = r.GetSourceColumnsFromSetOp(sourceExpr)
		} else if sourceExpr != nil {
			// TODO(slice 5): QueryTransform Spark schema branch.
			columns = sourceExpr.NamedSelects()
		}
	}

	if selected, ok := r.scope.SelectedSources()[name]; ok {
		columnAliases := []string{}
		if selected.Node != nil {
			columnAliases = selected.Node.AliasColumnNames()
		} else if sourceScope, ok := selected.Source.(*Scope); ok && sourceScope.Expression != nil {
			columnAliases = sourceScope.Expression.AliasColumnNames()
		}
		if len(columnAliases) > 0 {
			aliased := make([]string, len(columns))
			copy(aliased, columns)
			for i := range aliased {
				if i < len(columnAliases) && columnAliases[i] != "" {
					aliased[i] = columnAliases[i]
				}
			}
			columns = aliased
		}
	}

	columns = copyStringSlice(columns)
	r.getSourceColumnsCache[cacheKey] = columns
	return columns
}

func (r *Resolver) getAllSourceColumns() map[string][]string {
	if r.sourceColumns == nil {
		r.sourceColumns = map[string][]string{}
		r.sourceColumnsOrder = nil
		for _, sourceName := range r.scope.SelectedSourceNames() {
			r.sourceColumns[sourceName] = r.GetSourceColumns(sourceName)
			r.sourceColumnsOrder = append(r.sourceColumnsOrder, sourceName)
		}
		for sourceName := range r.scope.LateralSources {
			if _, ok := r.sourceColumns[sourceName]; !ok {
				r.sourceColumns[sourceName] = r.GetSourceColumns(sourceName)
				r.sourceColumnsOrder = append(r.sourceColumnsOrder, sourceName)
			}
		}
	}
	return r.sourceColumns
}

func (r *Resolver) getTableNameFromSources(columnName string, sourceColumns map[string][]string, order []string) string {
	if sourceColumns == nil {
		if r.unambiguousColumns == nil {
			r.unambiguousColumns = r.getUnambiguousColumns(r.getAllSourceColumns(), r.sourceColumnsOrder)
		}
		return r.unambiguousColumns[columnName]
	}
	return r.getUnambiguousColumns(sourceColumns, order)[columnName]
}

func (r *Resolver) getTableNameFromSourcesRecover(columnName string, sourceColumns map[string][]string, order []string) (tableName string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(*sqlerrors.OptimizeError); ok {
				tableName = ""
				return
			}
			panic(recovered)
		}
	}()
	return r.getTableNameFromSources(columnName, sourceColumns, order)
}

func (r *Resolver) getTableNameFromJoinContextRecover(columnName string, joinContext exp.Expression) (tableName string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(*sqlerrors.OptimizeError); ok {
				tableName = ""
				return
			}
			panic(recovered)
		}
	}()
	available, order := r.getAvailableSourceColumns(joinContext)
	return r.getTableNameFromSources(columnName, available, order)
}

func (r *Resolver) getColumnJoinContext(column exp.Expression) exp.Expression {
	if r.scope == nil || r.scope.Expression == nil {
		return nil
	}
	if len(expressionsFor(r.scope.Expression, "joins")) == 0 || truthy(r.scope.Expression.Arg("laterals")) || truthy(r.scope.Expression.Arg("pivots")) {
		return nil
	}

	joinAncestor := column.FindAncestor(exp.KindJoin, exp.KindSelect)
	if joinAncestor != nil && joinAncestor.Kind() == exp.KindJoin {
		if _, ok := r.scope.SelectedSources()[joinAncestor.AliasOrName()]; ok {
			return joinAncestor
		}
	}
	return nil
}

func (r *Resolver) getAvailableSourceColumns(joinAncestor exp.Expression) (map[string][]string, []string) {
	availableSources := map[string][]string{}
	order := []string{}
	from := asExpression(r.scope.Expression.Arg("from_"))
	if from != nil && from.This() != nil {
		fromName := from.This().AliasOrName()
		availableSources[fromName] = r.GetSourceColumns(fromName)
		order = append(order, fromName)
	}

	joins := expressionsFor(r.scope.Expression, "joins")
	limit := joinAncestor.Index()
	if limit < 0 || limit >= len(joins) {
		limit = len(joins) - 1
	}
	for i := 0; i <= limit && i < len(joins); i++ {
		join := joins[i]
		joinName := join.AliasOrName()
		availableSources[joinName] = r.GetSourceColumns(joinName)
		order = append(order, joinName)
	}
	return availableSources, order
}

func (r *Resolver) getUnambiguousColumns(sourceColumns map[string][]string, order []string) map[string]string {
	if len(sourceColumns) == 0 {
		return map[string]string{}
	}
	if len(order) == 0 {
		for _, name := range r.scope.SelectedSourceNames() {
			if _, ok := sourceColumns[name]; ok {
				order = append(order, name)
			}
		}
		for name := range sourceColumns {
			if !containsString(order, name) {
				order = append(order, name)
			}
		}
	}

	firstTable := order[0]
	firstColumns := sourceColumns[firstTable]
	if len(order) == 1 {
		out := map[string]string{}
		for _, column := range firstColumns {
			out[column] = firstTable
		}
		return out
	}

	// TODO(slice 4c): UNNEST_COLUMN_ONLY original-alias shadowing.
	unambiguousColumns := map[string]string{}
	allColumns := map[string]bool{}
	for _, column := range firstColumns {
		unambiguousColumns[column] = firstTable
		allColumns[column] = true
	}

	for _, table := range order[1:] {
		columns := sourceColumns[table]
		unique := stringSet(columns)
		ambiguous := map[string]bool{}
		for column := range unique {
			if allColumns[column] {
				ambiguous[column] = true
			}
		}
		for _, column := range columns {
			allColumns[column] = true
		}
		for column := range ambiguous {
			delete(unambiguousColumns, column)
		}
		for column := range unique {
			if !ambiguous[column] {
				unambiguousColumns[column] = table
			}
		}
	}
	return unambiguousColumns
}

func (r *Resolver) structFieldNames(colType exp.Expression) []string {
	// TODO(slice 4c): port resolver.py:359-367 once TypeAnnotator/DataType inference is live.
	return nil
}

func (r *Resolver) getUnnestColumnType(column exp.Expression, scope *Scope) exp.Expression {
	// TODO(slice 4c): port resolver.py:369-392.
	return nil
}

func (r *Resolver) getColumnTypeFromScope(source any, column exp.Expression) exp.Expression {
	// TODO(slice 4c): port resolver.py:394-419.
	return nil
}
