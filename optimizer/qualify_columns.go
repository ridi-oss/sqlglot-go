package optimizer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/schema"
)

type aliasRef struct {
	expression exp.Expression
	index      int
}

type orderedNameSet struct {
	names []string
	set   map[string]bool
}

func newOrderedNameSet() *orderedNameSet { return &orderedNameSet{set: map[string]bool{}} }

func (s *orderedNameSet) add(name string) {
	if name == "" {
		return
	}
	if !s.set[name] {
		s.set[name] = true
		s.names = append(s.names, name)
	}
}

func QualifyColumns(expression exp.Expression, schemaArg any, expandAliasRefs bool, expandStars bool, inferSchema *bool, allowPartialQualification bool, dialect any) exp.Expression {
	return qualifyColumns(expression, schemaArg, expandAliasRefs, expandStars, inferSchema, allowPartialQualification, dialect, nil)
}

func qualifyColumns(expression exp.Expression, schemaArg any, expandAliasRefs bool, expandStars bool, inferSchema *bool, allowPartialQualification bool, dialect any, report map[exp.Expression]ResolvedSource) exp.Expression {
	s, err := schema.EnsureSchema(schemaArg, dialect, true)
	if err != nil {
		panic(err)
	}
	annotator := NewTypeAnnotator(s)
	infer := s.Empty()
	if inferSchema != nil {
		infer = *inferSchema
	}
	d := s.Dialect()
	if d == nil {
		d = dialects.Base()
	}
	pseudocolumns := d.Pseudocolumns
	if pseudocolumns == nil {
		pseudocolumns = map[string]bool{}
	}

	optimizerScopes := traverseScopeForOptimizer(expression)
	if report != nil {
		// R3's DML-root scopes — the DML target plus its FROM/USING/JOIN sources — live only in
		// the analysis traversal; the optimizer traversal deliberately omits them for
		// upstream-faithful qualification. So a DML root must populate the report from the analysis
		// traversal, or its sources would classify as Unresolved even with R3 present. For a
		// non-DML root the two traversals are identical, so reuse the optimizer scopes (no second
		// pass) — preserving R2's single-traversal benefit for the common SELECT case.
		if isDMLRootKind(expression.Kind()) {
			populateResolutionReport(expression, report)
		} else {
			populateResolutionReportForScopes(expression, optimizerScopes, report)
		}
	}
	for _, scope := range optimizerScopes {
		if d.PreferCTEAliasColumn {
			PushdownCTEAliasColumns(scope)
		}

		scopeExpression := scope.Expression
		isSelect := scopeExpression != nil && scopeExpression.Kind() == exp.KindSelect

		separatePseudocolumns(scope, pseudocolumns)

		resolver := NewResolver(scope, s, infer)
		popTableColumnAliases(scope.CTEs())
		popTableColumnAliases(scope.DerivedTables())
		usingColumnTables := expandUsing(scope, resolver)

		if (s.Empty() || d.ForceEarlyAliasRefExpansion) && expandAliasRefs {
			expandAliasRefsInScope(scope, resolver, d, d.ExpandOnlyGroupAliasRef)
		}

		convertColumnsToDots(scope, resolver)
		qualifyColumnsInScope(scope, resolver, allowPartialQualification)

		if !s.Empty() && expandAliasRefs {
			expandAliasRefsInScope(scope, resolver, d, false)
		}

		if isSelect {
			if expandStars {
				expandStarsInScope(scope, resolver, usingColumnTables, pseudocolumns, annotator)
			}
			QualifyOutputs(scope)
		}

		expandGroupBy(scope, d)
		expandOrderByAndDistinctOn(scope, resolver)

		if d.AnnotateAllScopes {
			annotator.AnnotateScope(scope)
		}
	}
	return expression
}

func ValidateQualifyColumns(expression exp.Expression, sql string) exp.Expression {
	var allUnqualifiedColumns []exp.Expression
	for _, scope := range traverseScopeForOptimizer(expression) {
		if scope.Expression != nil && scope.Expression.Kind() == exp.KindSelect {
			unqualifiedColumns := scope.UnqualifiedColumns()
			if externalColumns := scope.ExternalColumns(); len(externalColumns) > 0 && !scope.IsCorrelatedSubquery() && len(scope.Pivots()) == 0 {
				column := externalColumns[0]
				forTable := ""
				if column.Text("table") != "" {
					forTable = fmt.Sprintf(" for table: '%s'", column.Text("table"))
				}
				// TODO(slice 4c): error-position highlighting once Node has meta.
				panic(&sqlerrors.OptimizeError{Msg: fmt.Sprintf("Column '%s' could not be resolved%s.", column.Name(), forTable)})
			}
			allUnqualifiedColumns = append(allUnqualifiedColumns, unqualifiedColumns...)
		}
	}

	if len(allUnqualifiedColumns) > 0 {
		firstColumn := allUnqualifiedColumns[0]
		// TODO(slice 4c): error-position highlighting once Node has meta.
		panic(&sqlerrors.OptimizeError{Msg: fmt.Sprintf("Ambiguous column '%s'", firstColumn.Name())})
	}
	_ = sql
	return expression
}

func separatePseudocolumns(scope *Scope, pseudocolumns map[string]bool) {
	if len(pseudocolumns) == 0 {
		return
	}
	// TODO(slice 4c): port Pseudocolumn node replacement once pseudocolumn support exists.
}

func popTableColumnAliases(derivedTables []exp.Expression) {
	for _, derivedTable := range derivedTables {
		parent := derivedTable.Parent()
		if parent != nil && parent.Kind() == exp.KindWith && truthy(parent.Arg("recursive")) {
			continue
		}
		if tableAlias := asExpression(derivedTable.Arg("alias")); tableAlias != nil {
			tableAlias.Set("columns", nil)
		}
	}
}

func expandUsing(scope *Scope, resolver *Resolver) map[string][]string {
	columns := map[string]string{}
	updateSourceColumns := func(sourceName string) {
		for _, columnName := range resolver.GetSourceColumns(sourceName) {
			if _, ok := columns[columnName]; !ok {
				columns[columnName] = sourceName
			}
		}
	}

	joins := scope.FindAll(exp.KindJoin)
	if len(joins) == 0 {
		return map[string][]string{}
	}

	names := map[string]bool{}
	for _, join := range joins {
		names[join.AliasOrName()] = true
	}
	var ordered []string
	for _, key := range scope.SelectedSourceNames() {
		if !names[key] {
			ordered = append(ordered, key)
		}
	}

	if len(names) > 0 && len(ordered) == 0 {
		panic(&sqlerrors.OptimizeError{Msg: fmt.Sprintf("Joins %v missing source table %s", names, scope.Expression.ToS())})
	}

	columnTables := map[string]*orderedNameSet{}
	hasUsing := false
	for _, join := range joins {
		if len(expressionsFor(join, "using")) > 0 {
			hasUsing = true
			break
		}
	}
	if !hasUsing {
		return map[string][]string{}
	}

	for _, sourceName := range ordered {
		updateSourceColumns(sourceName)
	}

	for i, join := range joins {
		sourceTable := ""
		if len(ordered) > 0 {
			sourceTable = ordered[len(ordered)-1]
		}
		if sourceTable != "" {
			updateSourceColumns(sourceTable)
		}

		joinTable := join.AliasOrName()
		ordered = append(ordered, joinTable)

		using := expressionsFor(join, "using")
		if len(using) == 0 {
			continue
		}

		joinColumns := resolver.GetSourceColumns(joinTable)
		var conditions []exp.Expression
		usingIdentifierCount := len(using)
		isSemiOrAntiJoin := isSemiOrAntiJoin(join)

		for _, identifierExpression := range using {
			identifier := identifierExpression.Name()
			table := columns[identifier]

			if table == "" || !containsString(joinColumns, identifier) {
				if len(columns) > 0 && columns["*"] == "" && len(joinColumns) > 0 {
					panic(&sqlerrors.OptimizeError{Msg: "Cannot automatically join: " + identifier})
				}
			}

			if table == "" {
				table = sourceTable
			}

			var lhs exp.Expression
			if i == 0 || usingIdentifierCount == 1 {
				lhs = exp.Column_(identifier, table, nil, nil, nil)
			} else {
				var coalesceColumns []exp.Expression
				for _, tableName := range ordered[:len(ordered)-1] {
					if containsString(resolver.GetSourceColumns(tableName), identifier) {
						coalesceColumns = append(coalesceColumns, exp.Column_(identifier, tableName, nil, nil, nil))
					}
				}
				if len(coalesceColumns) > 1 {
					lhs = coalesce(coalesceColumns)
				} else {
					lhs = exp.Column_(identifier, table, nil, nil, nil)
				}
			}

			conditions = append(conditions, exp.EQ(exp.Args{"this": lhs, "expression": exp.Column_(identifier, joinTable, nil, nil, nil)}))

			tables := columnTables[identifier]
			if tables == nil {
				tables = newOrderedNameSet()
				columnTables[identifier] = tables
			}
			if !isSemiOrAntiJoin {
				tables.add(table)
				tables.add(joinTable)
			}
		}

		join.Set("using", nil)
		join.Set("on", exp.And_(conditions...))
	}

	if len(columnTables) > 0 {
		for _, column := range scope.Columns() {
			if column.Text("table") == "" {
				if tables := columnTables[column.Name()]; tables != nil {
					var coalesceArgs []exp.Expression
					for _, table := range tables.names {
						coalesceArgs = append(coalesceArgs, exp.Column_(column.Name(), table, nil, nil, nil))
					}
					replacement := coalesce(coalesceArgs)
					if parent := column.Parent(); parent != nil && parent.Kind() == exp.KindSelect {
						replacement = exp.AliasExpr(replacement, column.Name(), false)
					} else {
						// TODO(slice 4c): STRUCT/PropertyEQ USING replacement.
					}
					scope.Replace(column, replacement)
				}
			}
		}
	}

	out := map[string][]string{}
	for column, tables := range columnTables {
		out[column] = copyStringSlice(tables.names)
	}
	return out
}

func expandAliasRefsInScope(scope *Scope, resolver *Resolver, dialect *dialects.Dialect, expandOnlyGroupBy bool) {
	expression := scope.Expression
	if expression == nil || expression.Kind() != exp.KindSelect || dialect.DisablesAliasRefExpansion {
		return
	}

	aliasToExpression := map[string]aliasRef{}
	projections := map[string]bool{}
	for _, selection := range expression.Selects() {
		projections[selection.AliasOrName()] = true
	}
	replaced := false

	var replaceColumns func(node exp.Expression, resolveTable bool, literalIndex bool)
	replaceColumns = func(node exp.Expression, resolveTable bool, literalIndex bool) {
		if node == nil {
			return
		}
		isGroupBy := node.Kind() == exp.KindGroup
		isHaving := node.Kind() == exp.KindHaving
		isQualify := node.Kind() == exp.KindQualify
		if expandOnlyGroupBy && !isGroupBy {
			return
		}

		for _, column := range walkInScope(node, func(n exp.Expression) bool { return n.IsStar() }) {
			if column.Kind() != exp.KindColumn {
				continue
			}
			if expandOnlyGroupBy && isGroupBy && column.Parent() != node {
				continue
			}

			skipReplace := false
			var table exp.Expression
			if resolveTable && column.Text("table") == "" {
				table = resolver.GetTable(column.Name())
			}
			alias, hasAlias := aliasToExpression[column.Name()]
			aliasExpr := alias.expression

			if hasAlias && aliasExpr != nil {
				skipReplace = aliasExpr.Find(exp.TraitAggFunc) != nil && findAncestorTrait(column, exp.TraitAggFunc) != nil && !ancestorBeforeSelectIsWindow(column)

				if (isHaving || isQualify) && dialect.ProjectionAliasesShadowSourceNames {
					for _, node := range aliasExpr.FindAll(exp.KindColumn) {
						parts := node.Parts()
						if len(parts) > 0 && projections[parts[0].Name()] {
							skipReplace = true
							break
						}
					}
				}
			} else if dialect.ProjectionAliasesShadowSourceNames && (isGroupBy || isHaving || isQualify) {
				columnTable := column.Text("table")
				if table != nil {
					columnTable = table.Name()
				}
				if projections[columnTable] {
					column.Replace(exp.ToIdentifier(column.Name()))
					replaced = true
					return
				}
			}

			if table != nil && (!hasAlias || skipReplace) {
				column.Set("table", table)
			} else if column.Text("table") == "" && hasAlias && aliasExpr != nil && !skipReplace {
				if (aliasExpr.Kind() == exp.KindLiteral || aliasExpr.IsNumber()) && (literalIndex || resolveTable) {
					if literalIndex {
						column.Replace(exp.LiteralNumber(alias.index))
						replaced = true
					}
				} else {
					replaced = true
					replacement := exp.Paren(exp.Args{"this": aliasExpr.Copy()})
					column.Replace(replacement)
					simplified := SimplifyParens(replacement, dialect)
					if simplified != replacement {
						replacement.Replace(simplified)
					}
				}
			}
		}
	}

	for i, projection := range expression.Selects() {
		replaceColumns(projection, false, false)
		if projection.Kind() == exp.KindAlias {
			aliasToExpression[projection.Alias()] = aliasRef{expression: projection.This(), index: i + 1}
		}
	}

	parentScope := scope
	onRightSubTree := false
	for parentScope != nil && !parentScope.IsCTE() {
		parentScope = parentScope.Parent
		if parentScope != nil && parentScope.Expression != nil && parentScope.Expression.Kind() == exp.KindUnion {
			// Faithful to sqlglot v30.12.0 (qualify_columns.py:402): the union's right operand is
			// compared against the union itself, which can never hold, so onRightSubTree is always
			// false and the recursive-CTE column-pop below is dead in the pinned reference. Kept
			// 1:1 for fidelity rather than "fixing" the upstream bug (would be an untracked divergence).
			right := parentScope.Expression.Right()
			onRightSubTree = right != nil && right == parentScope.Expression
		}
	}

	if parentScope != nil && onRightSubTree && parentScope.Expression != nil {
		cte := parentScope.Expression.Parent()
		if cte != nil {
			with := cte.FindAncestor(exp.KindWith)
			if with != nil && truthy(with.Arg("recursive")) {
				aliasColumns := cte.AliasColumnNames()
				if len(aliasColumns) == 0 && cte.This() != nil {
					for _, selection := range cte.This().Selects() {
						aliasColumns = append(aliasColumns, selection.OutputName())
					}
				}
				for _, name := range aliasColumns {
					delete(aliasToExpression, name)
				}
			}
		}
	}

	replaceColumns(asExpression(expression.Arg("where")), false, false)
	replaceColumns(asExpression(expression.Arg("group")), false, true)
	replaceColumns(asExpression(expression.Arg("having")), true, false)
	replaceColumns(asExpression(expression.Arg("qualify")), true, false)

	if dialect.SupportsAliasRefsInJoinConditions {
		for _, join := range expressionsFor(expression, "joins") {
			replaceColumns(join, false, false)
		}
	}

	if replaced {
		scope.clearCache()
	}
}

func findAncestorTrait(node exp.Expression, trait exp.Trait) exp.Expression {
	for ancestor := node.Parent(); ancestor != nil; ancestor = ancestor.Parent() {
		if ancestor.Is(trait) {
			return ancestor
		}
	}
	return nil
}

func ancestorBeforeSelectIsWindow(node exp.Expression) bool {
	for ancestor := node.Parent(); ancestor != nil; ancestor = ancestor.Parent() {
		if ancestor.Kind() == exp.KindWindow {
			return true
		}
		if ancestor.Kind() == exp.KindSelect {
			return false
		}
	}
	return false
}

func expandGroupBy(scope *Scope, dialect *dialects.Dialect) {
	expression := scope.Expression
	if expression == nil {
		return
	}
	group := asExpression(expression.Arg("group"))
	if group == nil {
		return
	}
	group.Set("expressions", expandPositionalReferences(scope, group.Expressions(), dialect, false))
	expression.Set("group", group)
}

func expandOrderByAndDistinctOn(scope *Scope, resolver *Resolver) {
	expression := scope.Expression
	if expression == nil || !expression.Is(exp.TraitQuery) {
		return
	}

	for _, modifierKey := range []string{"order", "distinct"} {
		modifier := asExpression(expression.Arg(modifierKey))
		if modifier != nil && modifier.Kind() == exp.KindDistinct {
			modifier = asExpression(modifier.Arg("on"))
		}
		if modifier == nil {
			continue
		}

		modifierExpressions := modifier.Expressions()
		if modifierKey == "order" {
			ordered := make([]exp.Expression, 0, len(modifierExpressions))
			for _, expression := range modifierExpressions {
				if expression.This() != nil {
					ordered = append(ordered, expression.This())
				}
			}
			modifierExpressions = ordered
		}

		expandedExpressions := expandPositionalReferences(scope, modifierExpressions, resolver.dialect, true)
		for i, original := range modifierExpressions {
			if i >= len(expandedExpressions) {
				break
			}
			expanded := expandedExpressions[i]
			for _, agg := range original.FindAll(exp.TraitAggFunc) {
				for _, col := range agg.FindAll(exp.KindColumn) {
					if col.Text("table") == "" {
						col.Set("table", resolver.GetTable(col.Name()))
					}
				}
			}
			original.Replace(expanded)
		}

		if asExpression(expression.Arg("group")) != nil {
			// Python keys `selects` by projection body (exp.Expression __hash__ == structural
			// equality); qualify_columns.py:471. Go interface maps use pointer identity, so we
			// key by the structural form (ToS) to fold ORDER BY/DISTINCT ON expressions that
			// duplicate a projection body onto the projection alias.
			selects := map[string]exp.Expression{}
			for _, selection := range expression.Selects() {
				if this := selection.This(); this != nil {
					selects[this.ToS()] = exp.Column_(selection.AliasOrName(), nil, nil, nil, nil)
				}
			}
			for _, node := range modifierExpressions {
				if node.IsInt() {
					node.Replace(exp.ToIdentifier(selectByPos(expression, node).Alias()))
				} else if replacement, ok := selects[node.ToS()]; ok {
					node.Replace(replacement)
				}
			}
		}
	}
}

func expandPositionalReferences(scope *Scope, expressions []exp.Expression, dialect *dialects.Dialect, alias bool) []exp.Expression {
	var newNodes []exp.Expression
	var ambiguousProjections map[string]bool
	expression := scope.Expression
	if expression == nil || !expression.Is(exp.TraitQuery) {
		return newNodes
	}

	for _, node := range expressions {
		if node != nil && node.IsInt() && node.Kind() == exp.KindLiteral {
			selection := selectByPos(expression, node)
			if alias {
				aliasExpression := asExpression(selection.Arg("alias"))
				if aliasExpression != nil {
					newNodes = append(newNodes, exp.Column_(aliasExpression.Copy(), nil, nil, nil, nil))
				} else {
					newNodes = append(newNodes, node)
				}
			} else {
				selectExpr := selection.This()
				ambiguous := false
				if dialect.ProjectionAliasesShadowSourceNames {
					if ambiguousProjections == nil {
						ambiguousProjections = map[string]bool{}
						selectedSources := stringSet(scope.SelectedSourceNames())
						for _, selection := range expression.Selects() {
							if selectedSources[selection.AliasOrName()] {
								ambiguousProjections[selection.AliasOrName()] = true
							}
						}
					}
					for _, column := range selectExpr.FindAll(exp.KindColumn) {
						parts := column.Parts()
						if len(parts) > 0 && ambiguousProjections[parts[0].Name()] {
							ambiguous = true
							break
						}
					}
				}
				if isConstant(selectExpr) || selectExpr.IsNumber() || selectExpr.Find(exp.KindUnnest) != nil || ambiguous {
					newNodes = append(newNodes, node)
				} else {
					newNodes = append(newNodes, selectExpr.Copy())
				}
			}
		} else {
			newNodes = append(newNodes, node)
		}
	}
	return newNodes
}

func selectByPos(expression exp.Expression, node exp.Expression) exp.Expression {
	pos, err := strconv.Atoi(node.Text("this"))
	if err != nil || pos <= 0 || pos > len(expression.Selects()) {
		panic(&sqlerrors.OptimizeError{Msg: "Unknown output column: " + node.Name()})
	}
	selection := expression.Selects()[pos-1]
	if selection.Kind() != exp.KindAlias {
		panic(&sqlerrors.OptimizeError{Msg: "Unknown output column: " + node.Name()})
	}
	return selection
}

func convertColumnsToDots(scope *Scope, resolver *Resolver) {
	converted := false
	columns := append(append([]exp.Expression(nil), scope.Columns()...), scope.Stars()...)
	selectedSources := stringSet(scope.SelectedSourceNames())
	for _, column := range columns {
		if column.Kind() == exp.KindDot {
			continue
		}

		columnTable := column.Text("table")
		if columnTable != "" && !selectedSources[columnTable] && (scope.Parent == nil || scope.Parent.Sources[columnTable] == nil || !scope.IsCorrelatedSubquery()) {
			parts := column.Parts()
			if len(parts) == 0 {
				continue
			}
			root := parts[0]
			parts = parts[1:]

			var columnTableExpression any = columnTable
			if root.Kind() == exp.KindIdentifier && selectedSources[root.Name()] {
				columnTableExpression = root
				if len(parts) == 0 {
					continue
				}
				root = parts[0]
				parts = parts[1:]
			} else if table := resolver.GetTable(root.Name()); table != nil {
				columnTableExpression = table
			} else {
				continue
			}

			newColumn := exp.Column_(root, columnTableExpression, nil, nil, nil)
			if len(parts) == 0 {
				column.Replace(newColumn)
			} else {
				column.Replace(exp.DotBuild(append([]exp.Expression{newColumn}, parts...)))
			}
			converted = true
		}
	}
	if converted {
		scope.clearCache()
	}
}

func qualifyColumnsInScope(scope *Scope, resolver *Resolver, allowPartialQualification bool) {
	for _, column := range scope.Columns() {
		columnTable := column.Text("table")
		columnName := column.Name()

		if columnTable != "" {
			if _, ok := scope.Sources[columnTable]; ok {
				sourceColumns := resolver.GetSourceColumns(columnTable)
				// TODO(slice 4c): pivot output column validation.
				if !allowPartialQualification && len(sourceColumns) > 0 && !containsString(sourceColumns, columnName) && !containsString(sourceColumns, "*") {
					panic(&sqlerrors.OptimizeError{Msg: "Unknown column: " + columnName})
				}
			}
		}

		if columnTable == "" {
			if len(scope.Pivots()) > 0 && column.FindAncestor(exp.KindPivot) == nil {
				column.Set("table", exp.ToIdentifier(scope.Pivots()[0].Alias()))
				continue
			}

			table := resolver.GetTable(column)
			if table != nil {
				if source, ok := scope.Sources[table.Name()].(*Scope); ok && source.ColumnIndex()[column] {
					continue
				}
				column.Set("table", table)
			} else if resolver.dialect.TablesReferenceableAsColumns && len(column.Parts()) == 1 {
				if _, ok := scope.SelectedSources()[columnName]; ok {
					scope.Replace(column, exp.TableColumn(exp.Args{"this": column.This()}))
				}
			}
		}
	}

	for _, pivot := range scope.Pivots() {
		for _, column := range pivot.FindAll(exp.KindColumn) {
			if column.Text("table") == "" && resolver.AllColumns()[column.Name()] {
				if table := resolver.GetTable(column.Name()); table != nil {
					column.Set("table", table)
				}
			}
		}
	}
}

func expandStarsInScope(scope *Scope, resolver *Resolver, usingColumnTables map[string][]string, pseudocolumns map[string]bool, annotator *TypeAnnotator) {
	var newSelections []exp.Expression
	coalescedColumns := map[string]bool{}
	dialect := resolver.dialect

	pivots := scope.Pivots()
	var pivot exp.Expression
	if len(pivots) > 0 {
		pivot = pivots[0]
	}
	_ = pivot

	if dialect.SupportsStructStarExpansion {
		for _, col := range scope.Stars() {
			if col.Kind() == exp.KindDot {
				annotator.AnnotateScope(scope)
				break
			}
		}
	}

	scopeExpression := scope.Expression
	if scopeExpression == nil || !scopeExpression.Is(exp.TraitQuery) {
		return
	}

	// The except/replace/rename maps are declared ONCE outside the loop to mimic upstream's
	// id(table) keying (qualify_columns.py:778-780, 846): a full `*` reuses the stable
	// selected_sources keys, so a modifier set by an earlier full star LEAKS into a later bare
	// `*`; a qualified star (x.*) uses a fresh per-occurrence key, so it never leaks. In Go,
	// where maps key by value, we encode this with the table name for full stars (stable) and a
	// per-selection-unique suffix for qualified stars (fresh).
	exceptColumns := map[string]map[string]bool{}
	replaceColumns := map[string]map[string]exp.Expression{}
	renameColumns := map[string]map[string]string{}

	for selIdx, expression := range scopeExpression.Selects() {
		var tables []string
		var tableKeys []string
		ilikePattern := ""
		if expression.Kind() == exp.KindStar {
			for _, name := range scope.SelectedSourceNames() {
				tables = append(tables, name)
				tableKeys = append(tableKeys, name) // stable across selections -> leaks
			}
			addExceptColumns(expression, tableKeys, exceptColumns)
			addReplaceColumns(expression, tableKeys, replaceColumns)
			addRenameColumns(expression, tableKeys, renameColumns)
			ilikePattern = addILikeColumns(expression)
		} else if expression.IsStar() {
			if expression.Kind() == exp.KindColumn {
				name := expression.Text("table")
				tables = append(tables, name)
				// fresh per occurrence (mimics id(column.table)) -> never leaks
				tableKeys = append(tableKeys, name+"\x00"+strconv.Itoa(selIdx))
				if expression.This() != nil {
					addExceptColumns(expression.This(), tableKeys, exceptColumns)
					addReplaceColumns(expression.This(), tableKeys, replaceColumns)
					addRenameColumns(expression.This(), tableKeys, renameColumns)
					ilikePattern = addILikeColumns(expression.This())
				}
			} else if expression.Kind() == exp.KindDot {
				// TODO(slice 4c): struct star expansion for BigQuery/RisingWave.
			}
		}

		if len(tables) == 0 {
			newSelections = append(newSelections, expression)
			continue
		}

		for i, table := range tables {
			if _, ok := scope.Sources[table]; !ok {
				panic(&sqlerrors.OptimizeError{Msg: "Unknown table: " + table})
			}
			tableKey := tableKeys[i]

			columns := resolver.GetSourceColumns(table, true)
			if len(columns) == 0 {
				columns = scope.OuterColumns
			}

			if len(pseudocolumns) > 0 && dialect.ExcludesPseudocolumnsFromStar {
				filtered := make([]string, 0, len(columns))
				for _, name := range columns {
					if !pseudocolumns[strings.ToUpper(name)] {
						filtered = append(filtered, name)
					}
				}
				columns = filtered
			}

			if len(columns) == 0 || containsString(columns, "*") {
				return
			}

			columnsToExclude := exceptColumns[tableKey]
			renamed := renameColumns[tableKey]
			replaced := replaceColumns[tableKey]

			// TODO(slice 4c): pivot output column expansion.
			for _, name := range columns {
				if columnsToExclude[name] || coalescedColumns[name] {
					continue
				}
				if ilikePattern != "" {
					matched, err := regexp.MatchString("(?i)^"+ilikePattern+"$", name)
					if err != nil || !matched {
						continue
					}
				}
				if usingTables, ok := usingColumnTables[name]; ok && containsString(usingTables, table) {
					coalescedColumns[name] = true
					var coalesceArgs []exp.Expression
					for _, tableName := range usingTables {
						coalesceArgs = append(coalesceArgs, exp.Column_(name, tableName, nil, nil, nil))
					}
					newSelections = append(newSelections, exp.AliasExpr(coalesce(coalesceArgs), name, false))
				} else {
					aliasName := name
					if renamed != nil && renamed[name] != "" {
						aliasName = renamed[name]
					}
					selectionExpr := exp.Expression(nil)
					if replaced != nil {
						selectionExpr = replaced[name]
					}
					if selectionExpr == nil {
						selectionExpr = exp.Column_(name, table, nil, nil, nil)
					}
					if aliasName != name {
						newSelections = append(newSelections, exp.AliasExpr(selectionExpr, aliasName, false))
					} else {
						newSelections = append(newSelections, selectionExpr)
					}
				}
			}
		}
	}

	if len(newSelections) > 0 && scopeExpression.Kind() == exp.KindSelect {
		scopeExpression.Set("expressions", newSelections)
	}
}

func addILikeColumns(expression exp.Expression) string {
	ilike := asExpression(expression.Arg("ilike"))
	if ilike == nil {
		return ""
	}
	var builder strings.Builder
	for _, r := range ilike.Name() {
		switch r {
		case '%':
			builder.WriteString(".*")
		case '_':
			builder.WriteString(".")
		default:
			builder.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return builder.String()
}

func addExceptColumns(expression exp.Expression, keys []string, exceptColumns map[string]map[string]bool) {
	except := expressionsFor(expression, "except_")
	if len(except) == 0 {
		return
	}
	columns := map[string]bool{}
	for _, column := range except {
		columns[column.Name()] = true
	}
	for _, key := range keys {
		exceptColumns[key] = columns
	}
}

func addRenameColumns(expression exp.Expression, keys []string, renameColumns map[string]map[string]string) {
	rename := expressionsFor(expression, "rename")
	if len(rename) == 0 {
		return
	}
	columns := map[string]string{}
	for _, item := range rename {
		if item.This() != nil {
			columns[item.This().Name()] = item.Alias()
		}
	}
	for _, key := range keys {
		renameColumns[key] = columns
	}
}

func addReplaceColumns(expression exp.Expression, keys []string, replaceColumns map[string]map[string]exp.Expression) {
	replace := expressionsFor(expression, "replace")
	if len(replace) == 0 {
		return
	}
	columns := map[string]exp.Expression{}
	for _, item := range replace {
		columns[item.Alias()] = item
	}
	for _, key := range keys {
		replaceColumns[key] = columns
	}
}

func QualifyOutputs(scopeOrExpression any) {
	var scope *Scope
	if expression, ok := scopeOrExpression.(exp.Expression); ok {
		scope = buildScope(expression)
		if scope == nil {
			return
		}
	} else if s, ok := scopeOrExpression.(*Scope); ok {
		scope = s
	} else {
		return
	}

	expression := scope.Expression
	if expression == nil || !expression.Is(exp.TraitQuery) {
		return
	}

	var newSelections []exp.Expression
	selects := expression.Selects()
	maxLen := len(selects)
	if len(scope.OuterColumns) > maxLen {
		maxLen = len(scope.OuterColumns)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(selects) {
			break
		}
		selection := selects[i]
		// Python also breaks on exp.QueryTransform (qualify_columns.py:958). The QueryTransform
		// node kind does not exist in the Go port yet (Spark SELECT TRANSFORM — slice 5), so only
		// the nil case is reachable. TODO(slice 5): break on KindQueryTransform once it exists.
		if selection == nil {
			break
		}

		if selection.Kind() == exp.KindSubquery {
			if selection.OutputName() == "" {
				selection.Set("alias", exp.TableAlias(exp.Args{"this": exp.ToIdentifier(fmt.Sprintf("_col_%d", i))}))
			}
		} else if selection.Kind() != exp.KindAlias && selection.Kind() != exp.KindAliases && !selection.IsStar() {
			aliasName := selection.OutputName()
			if aliasName == "" {
				aliasName = fmt.Sprintf("_col_%d", i)
			}
			selection = exp.AliasExpr(selection, aliasName, false)
		}
		if i < len(scope.OuterColumns) && scope.OuterColumns[i] != "" {
			selection.Set("alias", exp.ToIdentifier(scope.OuterColumns[i]))
		}
		newSelections = append(newSelections, selection)
	}

	if len(newSelections) > 0 && expression.Kind() == exp.KindSelect {
		expression.Set("expressions", newSelections)
	}
}

func QuoteIdentifiers(expression exp.Expression, dialect any, identify bool) exp.Expression {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		panic(err)
	}
	for _, node := range expression.Walk() {
		if node.Kind() == exp.KindIdentifier {
			d.QuoteIdentifier(node, identify)
		}
	}
	return expression
}

func PushdownCTEAliasColumns(scope *Scope) {
	for _, cte := range scope.CTEs() {
		aliasColumns := cte.AliasColumnNames()
		if len(aliasColumns) == 0 || cte.This() == nil || cte.This().Kind() != exp.KindSelect {
			continue
		}
		var newExpressions []exp.Expression
		for i, projection := range cte.This().Expressions() {
			if i >= len(aliasColumns) {
				break
			}
			aliasName := aliasColumns[i]
			if projection.Kind() == exp.KindAlias {
				projection.Set("alias", exp.ToIdentifier(aliasName))
			} else {
				projection = exp.AliasExpr(projection, aliasName, false)
			}
			newExpressions = append(newExpressions, projection)
		}
		cte.This().Set("expressions", newExpressions)
	}
}

func coalesce(args []exp.Expression) exp.Expression {
	if len(args) == 0 {
		return nil
	}
	return exp.Coalesce(exp.Args{"this": args[0], "expressions": args[1:]})
}

func isConstant(expression exp.Expression) bool {
	if expression == nil {
		return false
	}
	switch expression.Kind() {
	case exp.KindLiteral, exp.KindBoolean, exp.KindNull:
		return true
	default:
		return false
	}
}
