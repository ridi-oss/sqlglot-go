package optimizer

import (
	"fmt"

	"github.com/ridi-oss/sqlglot-go/dialects"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/schema"
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
	// unambiguousNoShadow is unambiguousColumns without UNNEST-alias shadowing.
	unambiguousNoShadow map[string]string
	allColumns          map[string]bool
	inferSchema         bool

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
	columnExpr, _ := column.(exp.Expression)
	return r.getTableAt(column, columnExpr)
}

// getTableAt resolves column as GetTable does; at is the node whose position decides shadowing.
func (r *Resolver) getTableAt(column any, at exp.Expression) exp.Expression {
	columnName := ""
	if s, ok := column.(string); ok {
		columnName = s
	} else if expression, ok := column.(exp.Expression); ok && expression != nil {
		columnName = expression.Name()
	}

	columnExpr, _ := column.(exp.Expression)
	isColumn := columnExpr != nil && columnExpr.Kind() == exp.KindColumn
	// divergence (DEVIATIONS §1.25): upstream's UNNEST-alias shadowing (resolver.py:326-353) can bind
	// a column in an ON or a FROM item to an UNNEST joined after it, losing the earlier column's read.
	// There no shadowing applies: an ambiguous name fails.
	shadow := at == nil || r.dialect == nil || !r.dialect.UnnestColumnOnly || !r.inJoinOrFromItem(at)
	tableName := r.getTableNameFromSources(columnName, nil, nil, shadow)

	if tableName == "" && isColumn {
		if joinContext := r.getColumnJoinContext(columnExpr); joinContext != nil {
			tableName = r.getTableNameFromJoinContextRecover(columnName, joinContext, shadow)
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
		// v30.17.0 (resolver.py:119-120): access args directly — set-op operands aren't
		// guaranteed Query nodes (VALUES (1) UNION ALL SELECT 1), where .Left/.Right differ.
		left := r.GetSourceColumnsFromSetOp(asExpression(expression.Arg("this")))
		right := r.GetSourceColumnsFromSetOp(asExpression(expression.Arg("expression")))
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

	if table, ok := source.(exp.Expression); ok && table != nil && table.Kind() == exp.KindTable && table.SchemaName() == "" && len(expressionsFor(table, "pivots")) > 0 {
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
			// resolver.py:159-172: BigQuery scopes the fields of an unnested struct as columns.
			if r.dialect != nil && r.dialect.UnnestColumnOnly && sourceExpr.Kind() == exp.KindUnnest {
				columns = append(columns, r.structFieldNames(r.unnestElementType(sourceExpr, source))...)
			}
			// TODO(slice 4c): port the Explode / Lateral Query type-aware branches.
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
			// Positional column aliases apply to the ROW columns only: an implicit (system)
			// column never consumes an alias position and keeps its own name (PG: `users AS
			// u(a, b)` aliases id and name; u.ctid still resolves). Implicit = in the full
			// list but not the visible list; for sources without a visibility notion the two
			// lists are identical and this is the plain positional zip.
			implicit := map[string]bool{}
			if table, ok := source.(exp.Expression); ok && !visible && table != nil && table.Kind() == exp.KindTable {
				visibleColumns, err := r.schema.ColumnNames(table, true, "", nil)
				if err != nil {
					panic(err)
				}
				visibleSet := map[string]bool{}
				for _, col := range visibleColumns {
					visibleSet[col] = true
				}
				for _, col := range columns {
					if !visibleSet[col] {
						implicit[col] = true
					}
				}
			}
			aliased := make([]string, len(columns))
			copy(aliased, columns)
			aliasIndex := 0
			for i := range aliased {
				if implicit[aliased[i]] {
					continue
				}
				if aliasIndex < len(columnAliases) && columnAliases[aliasIndex] != "" {
					aliased[i] = columnAliases[aliasIndex]
				}
				aliasIndex++
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

func (r *Resolver) getTableNameFromSources(columnName string, sourceColumns map[string][]string, order []string, shadow bool) string {
	if sourceColumns == nil {
		if !shadow {
			if r.unambiguousNoShadow == nil {
				r.unambiguousNoShadow = r.getUnambiguousColumns(r.getAllSourceColumns(), r.sourceColumnsOrder, false)
			}
			return r.unambiguousNoShadow[columnName]
		}
		if r.unambiguousColumns == nil {
			r.unambiguousColumns = r.getUnambiguousColumns(r.getAllSourceColumns(), r.sourceColumnsOrder, true)
		}
		return r.unambiguousColumns[columnName]
	}
	return r.getUnambiguousColumns(sourceColumns, order, shadow)[columnName]
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
	return r.getTableNameFromSources(columnName, sourceColumns, order, true)
}

func (r *Resolver) getTableNameFromJoinContextRecover(columnName string, joinContext exp.Expression, shadow bool) (tableName string) {
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
	return r.getTableNameFromSources(columnName, available, order, shadow)
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

func (r *Resolver) getUnambiguousColumns(sourceColumns map[string][]string, order []string, shadow bool) map[string]string {
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

	// resolver.py:326-339: with UNNEST_COLUMN_ONLY an UNNEST alias shadows a same-named column
	// of another source.
	unnestOriginalAliases := map[string]string{}
	if shadow && r.dialect != nil && r.dialect.UnnestColumnOnly {
		for sourceName, source := range r.scope.Sources {
			scope, ok := source.(*Scope)
			if !ok || scope.Expression == nil || scope.Expression.Kind() != exp.KindUnnest {
				continue
			}
			if columns := scope.Expression.AliasColumnNames(); len(columns) > 0 {
				// Two UNNESTs with one alias stay ambiguous (Go map order is random).
				if _, seen := unnestOriginalAliases[columns[0]]; seen {
					unnestOriginalAliases[columns[0]] = ""
				} else {
					unnestOriginalAliases[columns[0]] = sourceName
				}
			}
		}
	}
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
			if source := unnestOriginalAliases[column]; source != "" {
				unambiguousColumns[column] = source
				continue
			}
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

// structFieldNames ports _struct_field_names (resolver.py:360-368).
func (r *Resolver) structFieldNames(colType exp.Expression) []string {
	if exp.IsType(colType, exp.DTypeArray) {
		colType = firstExpr(colType)
	}
	if !exp.IsType(colType, exp.DTypeStruct) {
		return nil
	}
	var names []string
	for _, field := range expressionsFor(colType, "expressions") {
		names = append(names, field.Name())
	}
	return names
}

func firstExpr(e exp.Expression) exp.Expression {
	if list := expressionsFor(e, "expressions"); len(list) > 0 {
		return list[0]
	}
	return nil
}

// getUnnestColumnType ports _get_unnest_column_type (resolver.py:370-393).
func (r *Resolver) getUnnestColumnType(column exp.Expression, scope *Scope) exp.Expression {
	tableName := column.Text("table")
	if tableName == "" {
		tableIdentifier := NewResolver(scope, r.schema, r.inferSchema).GetTable(column)
		if tableIdentifier == nil {
			return nil
		}
		tableName = tableIdentifier.Name()
	}
	source, ok := scope.Sources[tableName]
	if !ok {
		return nil
	}
	return r.getColumnTypeFromScope(source, column)
}

// getColumnTypeFromScope ports _get_column_type_from_scope (resolver.py:395-432), uncached.
func (r *Resolver) getColumnTypeFromScope(source any, column exp.Expression) exp.Expression {
	switch src := source.(type) {
	case exp.Expression:
		if src == nil || src.Kind() != exp.KindTable {
			return nil
		}
		colType, err := r.schema.GetColumnType(src, column, nil, nil)
		if err != nil || colType == nil || exp.IsType(colType, exp.DTypeUnknown) {
			return nil
		}
		return colType
	case *Scope:
		// divergence: upstream (resolver.py:423-428) searches nested sources by the output name,
		// which picks the wrong column under a rename (`SELECT o AS s FROM t`). Follow the
		// projection instead; anything but a pass-through column gives no type.
		expression := src.Expression
		if expression == nil || expression.Kind() != exp.KindSelect {
			return nil
		}
		for _, selection := range expression.Selects() {
			if selection.AliasOrName() != column.Name() {
				continue
			}
			inner := selection
			if inner.Kind() == exp.KindAlias {
				inner = inner.This()
			}
			if inner != nil && inner.Kind() != exp.KindColumn {
				return literalType(inner)
			}
			if inner == nil || inner.Text("table") == "" {
				return nil
			}
			if nested, ok := src.Sources[inner.Text("table")]; ok {
				return r.getColumnTypeFromScope(nested, inner)
			}
			return nil
		}
	}
	return nil
}

// unnestElementType resolves (and caches in meta, standing in for Unnest.type) the element
// type of an UNNEST over a single column (resolver.py:162-171). Upstream's ANNOTATE_ALL_SCOPES
// pass types an UNNEST from its own scope's lateral sources first; the parent scope covers a
// correlated `(SELECT ... FROM UNNEST(col))`.
func (r *Resolver) unnestElementType(unnest exp.Expression, unnestScope *Scope) exp.Expression {
	if t, ok := unnest.MetaGet("type").(exp.Expression); ok && t != nil && !exp.IsType(t, exp.DTypeUnknown) {
		return t
	}
	exprs := expressionsFor(unnest, "expressions")
	if len(exprs) == 0 {
		return nil
	}
	if exprs[0].Kind() != exp.KindColumn {
		colType := literalType(exprs[0])
		if exp.IsType(colType, exp.DTypeArray) {
			colType = firstExpr(colType)
		}
		if colType != nil {
			unnest.Meta()["type"] = colType
		}
		return colType
	}
	var colType exp.Expression
	if exprs[0].Text("table") != "" {
		if _, ok := unnestScope.Sources[exprs[0].Text("table")]; ok {
			colType = r.getUnnestColumnType(exprs[0], unnestScope)
		}
	}
	if colType == nil && r.scope.Parent != nil {
		colType = r.getUnnestColumnType(exprs[0], r.scope.Parent)
	}
	if exp.IsType(colType, exp.DTypeArray) {
		colType = firstExpr(colType)
		if colType == nil {
			return nil
		}
	}
	if colType != nil {
		colType = colType.Copy()
		unnest.Meta()["type"] = colType
	}
	return colType
}

// literalType infers the STRUCT/ARRAY type of a literal from its syntax, standing in for the
// annotate_types results upstream reads: `STRUCT(1 AS a)`, `[STRUCT(...)]`, `ARRAY<T>[...]`,
// and a parenthesized single-projection query over those. Anything else gives nil.
func literalType(e exp.Expression) exp.Expression {
	switch e.Kind() {
	case exp.KindCast:
		return asExpression(e.Arg("to"))
	case exp.KindParen, exp.KindSubquery:
		if inner := e.This(); inner != nil {
			return literalType(inner)
		}
	case exp.KindSelect:
		if selects := e.Selects(); len(selects) == 1 {
			return literalType(selects[0])
		}
	case exp.KindAlias:
		if inner := e.This(); inner != nil {
			return literalType(inner)
		}
	case exp.KindArray:
		elems := expressionsFor(e, "expressions")
		if len(elems) == 0 {
			return nil
		}
		elem := literalType(elems[0])
		if elem == nil {
			return nil
		}
		return exp.DataType(exp.Args{"this": exp.DTypeArray, "expressions": []exp.Expression{elem}, "nested": true})
	case exp.KindStruct:
		var fields []exp.Expression
		for _, field := range expressionsFor(e, "expressions") {
			var name, value exp.Expression
			switch field.Kind() {
			case exp.KindAlias:
				name, value = asExpression(field.Arg("alias")), field.This()
			case exp.KindPropertyEQ:
				name, value = field.This(), field.Expr()
			}
			if name == nil {
				// An anonymous field: the struct cannot be expanded by name.
				return nil
			}
			kind := exp.DTypeUnknown.IntoExpr(nil)
			if value != nil {
				if t := literalType(value); t != nil {
					kind = t
				}
			}
			fields = append(fields, exp.ColumnDef(exp.Args{"this": exp.ToIdentifier(name.Name()), "kind": kind}))
		}
		return exp.DataType(exp.Args{"this": exp.DTypeStruct, "expressions": fields, "nested": true})
	}
	return nil
}

// inJoinOrFromItem reports whether node reaches this scope's query through its JOIN or FROM clause
// (a correlated subquery's column included).
func (r *Resolver) inJoinOrFromItem(node exp.Expression) bool {
	for n := node; n.Parent() != nil; n = n.Parent() {
		if n.Parent() == r.scope.Expression {
			return n.Kind() == exp.KindJoin || n.Kind() == exp.KindFrom
		}
	}
	return false
}
