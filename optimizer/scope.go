package optimizer

import (
	"fmt"
	"log"
	"strings"

	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
)

type ScopeType int

const (
	ScopeTypeRoot ScopeType = iota
	ScopeTypeSubquery
	ScopeTypeDerivedTable
	ScopeTypeCTE
	ScopeTypeUnion
	ScopeTypeUDTF
)

type reference struct {
	Name string
	Node exp.Expression
}

type selectedSource struct {
	Node   exp.Expression
	Source any
}

type scopeOptions struct {
	sources         map[string]any
	lateralSources  map[string]any
	cteSources      map[string]any
	outerColumns    []string
	parent          *Scope
	scopeType       ScopeType
	canBeCorrelated bool
}

type Scope struct {
	Expression exp.Expression
	Sources    map[string]any
	Parent     *Scope
	ScopeType  ScopeType

	SubqueryScopes     []*Scope
	DerivedTableScopes []*Scope
	TableScopes        []*Scope
	CTEScopes          []*Scope
	UnionScopes        []*Scope
	UDTFScopes         []*Scope

	LateralSources  map[string]any
	CTESources      map[string]any
	OuterColumns    []string
	canBeCorrelated bool

	sourceOrder []string
	// DML traversal interleaves source and scalar subqueries in AST order. Keep
	// that exact child sequence for Scope.Traverse without duplicating TableScopes.
	dmlChildScopes []*Scope

	collected               bool
	scansAllSubscopeColumns bool
	rawColumns              []exp.Expression
	tableColumns            []exp.Expression
	stars                   []exp.Expression
	derivedTables           []exp.Expression
	udtfs                   []exp.Expression
	tables                  []exp.Expression
	ctes                    []exp.Expression
	subqueries              []exp.Expression
	joinHints               []exp.Expression
	semiAntiJoinTables      map[string]bool
	columnIndex             map[exp.Expression]bool
	columns                 []exp.Expression
	columnsDone             bool
	externalColumns         []exp.Expression
	externalColumnsDone     bool
	localColumns            []exp.Expression
	localColumnsDone        bool
	selectedSources         map[string]selectedSource
	selectedSourcesOrder    []string
	selectedSourcesDone     bool
	references              []reference
	referencesDone          bool
	pivots                  []exp.Expression
	pivotsDone              bool
}

var logWarning = func(format string, args ...any) { log.Printf(format, args...) }

func newScope(expression exp.Expression, opts scopeOptions) *Scope {
	sources := map[string]any{}
	order := []string{}
	for k, v := range opts.sources {
		sources[k] = v
		order = appendSourceOrder(order, k)
	}
	lateralSources := copySources(opts.lateralSources)
	cteSources := copySources(opts.cteSources)
	for k, v := range lateralSources {
		sources[k] = v
		order = appendSourceOrder(order, k)
	}
	for k, v := range cteSources {
		sources[k] = v
		order = appendSourceOrder(order, k)
	}
	scopeType := opts.scopeType
	if scopeType == 0 && opts.parent == nil {
		scopeType = ScopeTypeRoot
	}
	scope := &Scope{
		Expression:         expression,
		Sources:            sources,
		Parent:             opts.parent,
		ScopeType:          scopeType,
		LateralSources:     lateralSources,
		CTESources:         cteSources,
		OuterColumns:       append([]string(nil), opts.outerColumns...),
		canBeCorrelated:    opts.canBeCorrelated,
		sourceOrder:        order,
		semiAntiJoinTables: map[string]bool{},
		columnIndex:        map[exp.Expression]bool{},
	}
	scope.clearCache()
	return scope
}

func appendSourceOrder(order []string, name string) []string {
	for _, existing := range order {
		if existing == name {
			return order
		}
	}
	return append(order, name)
}

func (s *Scope) clearCache() {
	s.collected = false
	s.scansAllSubscopeColumns = false
	s.rawColumns = nil
	s.tableColumns = nil
	s.stars = nil
	s.derivedTables = nil
	s.udtfs = nil
	s.tables = nil
	s.ctes = nil
	s.subqueries = nil
	s.joinHints = nil
	s.semiAntiJoinTables = map[string]bool{}
	s.columnIndex = map[exp.Expression]bool{}
	s.selectedSources = nil
	s.selectedSourcesOrder = nil
	s.selectedSourcesDone = false
	s.columns = nil
	s.columnsDone = false
	s.externalColumns = nil
	s.externalColumnsDone = false
	s.localColumns = nil
	s.localColumnsDone = false
	s.pivots = nil
	s.pivotsDone = false
	s.references = nil
	s.referencesDone = false
}

func (s *Scope) branch(expression exp.Expression, opts scopeOptions) *Scope {
	if expression != nil {
		expression = expression.Unnest()
	}
	return newScope(expression, scopeOptions{
		sources:         copySources(opts.sources),
		parent:          s,
		scopeType:       opts.scopeType,
		cteSources:      mergeSources(s.CTESources, opts.cteSources),
		lateralSources:  copySources(opts.lateralSources),
		canBeCorrelated: s.canBeCorrelated || opts.scopeType == ScopeTypeSubquery || opts.scopeType == ScopeTypeUDTF,
		outerColumns:    opts.outerColumns,
	})
}

type scopeTraversalMode int

const (
	scopeTraversalAnalysis scopeTraversalMode = iota
	scopeTraversalOptimizer
)

func TraverseScope(expression exp.Expression) []*Scope { return traverseScope(expression) }

func traverseScope(expression exp.Expression) []*Scope {
	return traverseScopeWithMode(expression, scopeTraversalAnalysis)
}

// Transformation and validation passes must use this compatibility traversal because the
// Go-only R3 analysis traversal intentionally emits more scopes than pinned upstream, including
// source-subquery scopes created solely to bind DML sources.
func traverseScopeForOptimizer(expression exp.Expression) []*Scope {
	return traverseScopeWithMode(expression, scopeTraversalOptimizer)
}

func traverseScopeWithMode(expression exp.Expression, mode scopeTraversalMode) []*Scope {
	if !isTraversable(expression) {
		return nil
	}
	out := []*Scope{}
	_traverseScope(newScope(expression, scopeOptions{}), &out, mode)
	return out
}

func BuildScope(expression exp.Expression) *Scope { return buildScope(expression) }

func buildScope(expression exp.Expression) *Scope {
	scopes := traverseScope(expression)
	scope, ok := seqGet(scopes, -1)
	if !ok {
		return nil
	}
	// Complete-or-none for DML roots: _traverseDML OMITS the DML-root scope when the write
	// source set is incomplete/malformed, leaving only retained CHILD scopes (e.g. a
	// scalar-subquery scope). The last such child is NOT a scope over the whole statement —
	// its Sources miss the target + FROM/USING sources — so returning it here would fail open
	// (a consumer treating a non-nil BuildScope as the statement scope sees a partial source
	// set). Only return the last scope as the build root when it actually IS the root scope for
	// this DML statement; otherwise signal "no root scope" with nil. Non-DML roots are
	// unaffected (a SELECT's last scope is its root scope).
	if isDMLRootKind(expression.Kind()) && !(isDMLRootScope(scope) && scope.Expression == expression) {
		return nil
	}
	return scope
}

func _traverseScope(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	expression := scope.Expression
	if expression == nil {
		return
	}

	if expression.Kind() == exp.KindSelect {
		_traverseSelect(scope, out, mode)
	} else if expression.Is(exp.TraitSetOperation) {
		_traverseCtes(scope, out, mode)
		_traverseUnion(scope, out, mode)
		return
	} else if expression.Kind() == exp.KindSubquery {
		if scope.IsRoot() {
			_traverseSelect(scope, out, mode)
		} else {
			_traverseSubqueries(scope, out, mode)
		}
	} else if expression.Kind() == exp.KindTable {
		_traverseTables(scope, out, mode)
	} else if expression.Is(exp.TraitUDTF) {
		_traverseUdtfs(scope, out, mode)
	} else if expression.Is(exp.TraitDDL) {
		ddlExpression := asExpression(expression.Arg("expression"))
		if ddlExpression != nil && ddlExpression.Is(exp.TraitQuery) {
			_traverseCtes(scope, out, mode)
			_traverseScope(newScope(ddlExpression, scopeOptions{cteSources: scope.CTESources}), out, mode)
		}
		return
	} else if expression.Is(exp.TraitDML) {
		_traverseCtes(scope, out, mode)
		if mode == scopeTraversalAnalysis && isDMLRootKind(expression.Kind()) {
			_traverseDML(scope, out, mode)
			return
		}
		_traverseDMLQueries(scope, out, mode)
		return
	} else {
		logWarning("Cannot traverse scope %s with type '%s'", expression.ToS(), exp.ClassName(expression.Kind()))
		return
	}

	*out = append(*out, scope)
}

func _traverseSelect(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	_traverseCtes(scope, out, mode)
	_traverseTables(scope, out, mode)
	_traverseSubqueries(scope, out, mode)
}

func _traverseUnion(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	var prevScope *Scope
	unionScopeStack := []*Scope{scope}
	setOp := scope.Expression
	expressionStack := []exp.Expression{setOp.Expr(), setOp.This()}

	for len(expressionStack) > 0 {
		expression := expressionStack[len(expressionStack)-1]
		expressionStack = expressionStack[:len(expressionStack)-1]
		unionScope := unionScopeStack[len(unionScopeStack)-1]

		newScope := unionScope.branch(expression, scopeOptions{
			outerColumns: unionScope.OuterColumns,
			scopeType:    ScopeTypeUnion,
		})

		if isSetOperation(expression) {
			_traverseCtes(newScope, out, mode)
			unionScopeStack = append(unionScopeStack, newScope)
			expressionStack = append(expressionStack, expression.Expr(), expression.This())
			continue
		}

		before := len(*out)
		_traverseScope(newScope, out, mode)
		var childScope *Scope
		if len(*out) > before {
			childScope = (*out)[len(*out)-1]
		}

		if prevScope != nil {
			unionScopeStack = unionScopeStack[:len(unionScopeStack)-1]
			unionScope.UnionScopes = []*Scope{prevScope, childScope}
			prevScope = unionScope
			*out = append(*out, unionScope)
		} else {
			prevScope = childScope
		}
	}
}

func _traverseCtes(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	sources := map[string]any{}
	sourceOrder := []string{}

	for _, cte := range scope.CTEs() {
		cteName := cte.Alias()

		with := asExpression(scope.Expression.Arg("with_"))
		if with != nil && truthy(with.Arg("recursive")) {
			union := cte.This()
			if isSetOperation(union) {
				sources[cteName] = scope.branch(union.This(), scopeOptions{scopeType: ScopeTypeCTE})
				sourceOrder = appendSourceOrder(sourceOrder, cteName)
			}
		}

		before := len(*out)
		_traverseScope(scope.branch(cte.This(), scopeOptions{
			cteSources:   sources,
			outerColumns: cte.AliasColumnNames(),
			scopeType:    ScopeTypeCTE,
		}), out, mode)

		var childScope *Scope
		if len(*out) > before {
			childScope = (*out)[len(*out)-1]
		}
		if childScope != nil {
			sources[cteName] = childScope
			sourceOrder = appendSourceOrder(sourceOrder, cteName)
			scope.CTEScopes = append(scope.CTEScopes, childScope)
		}
	}

	for _, name := range sourceOrder {
		scope.Sources[name] = sources[name]
		scope.CTESources[name] = sources[name]
		scope.sourceOrder = appendSourceOrder(scope.sourceOrder, name)
	}
}

type dmlSourceCandidate struct {
	expression exp.Expression
	location   string
	isTarget   bool
}

type dmlSourceValidationError struct {
	location string
	reason   string
}

func isDMLRootKind(kind exp.Kind) bool {
	return kind == exp.KindUpdate || kind == exp.KindDelete || kind == exp.KindMerge
}

func _traverseDML(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	scope.dmlChildScopes = append(scope.dmlChildScopes, scope.CTEScopes...)
	candidates, validationErr := collectDMLSourceCandidates(scope.Expression)
	if validationErr != nil {
		logWarning(
			"Cannot build %s root scope: invalid %s (%s)",
			exp.ClassName(scope.Expression.Kind()),
			validationErr.location,
			validationErr.reason,
		)
		_traverseDMLQueries(scope, out, mode)
		return
	}

	sourceSubqueries := map[exp.Expression]bool{}
	for _, candidate := range candidates {
		if candidate.expression.Kind() == exp.KindSubquery {
			sourceSubqueries[candidate.expression] = true
		}
	}

	queryScopes := map[exp.Expression]*Scope{}
	processedSourceSubqueries := map[exp.Expression]bool{}
	for _, query := range findAllInScope(scope.Expression, exp.TraitQuery) {
		parent := query.Parent()
		if parent != nil && (parent.Kind() == exp.KindCTE || parent.Kind() == exp.KindSubquery) {
			continue
		}

		isSourceSubquery := sourceSubqueries[query]
		if isSourceSubquery && processedSourceSubqueries[query] {
			continue
		}
		childScope, wrapperScope := traverseDMLQuery(scope, query, isSourceSubquery, out, mode)
		if isSourceSubquery {
			processedSourceSubqueries[query] = true
		}
		if childScope == nil {
			continue
		}
		queryScopes[query] = childScope
		if isSourceSubquery {
			attachDMLSourceScope(scope, childScope, wrapperScope)
		} else {
			scope.SubqueryScopes = append(scope.SubqueryScopes, childScope)
			scope.dmlChildScopes = append(scope.dmlChildScopes, childScope)
		}
	}

	// A source can be attached through a defensive, undeclared argument key. ArgKeys does
	// not expose such keys to walkInScope, so make sure every validated source Subquery is
	// still traversed exactly once.
	for _, candidate := range candidates {
		subquery := candidate.expression
		if subquery.Kind() != exp.KindSubquery || processedSourceSubqueries[subquery] {
			continue
		}
		childScope, wrapperScope := traverseDMLQuery(scope, subquery, true, out, mode)
		processedSourceSubqueries[subquery] = true
		if childScope == nil {
			continue
		}
		queryScopes[subquery] = childScope
		attachDMLSourceScope(scope, childScope, wrapperScope)
	}

	if bindingErr := bindDMLSources(scope, candidates, queryScopes); bindingErr != nil {
		logWarning(
			"Cannot build %s root scope: invalid %s (%s)",
			exp.ClassName(scope.Expression.Kind()),
			bindingErr.location,
			bindingErr.reason,
		)
		return
	}

	*out = append(*out, scope)
}

func attachDMLSourceScope(scope *Scope, childScope *Scope, wrapperScope *Scope) {
	scope.DerivedTableScopes = append(scope.DerivedTableScopes, childScope)
	scope.TableScopes = append(scope.TableScopes, childScope)
	scope.dmlChildScopes = append(scope.dmlChildScopes, wrapperScope)
}

// _traverseDMLQueries mirrors the pinned v30.12.0 generic DML walk. Its query
// discovery is intentionally shape-dependent; do not replace it with a source walk.
func _traverseDMLQueries(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	for _, query := range findAllInScope(scope.Expression, exp.TraitQuery) {
		parent := query.Parent()
		if parent == nil || (parent.Kind() != exp.KindCTE && parent.Kind() != exp.KindSubquery) {
			_traverseScope(newScope(query, scopeOptions{cteSources: scope.CTESources}), out, mode)
		}
	}
}

func traverseDMLQuery(scope *Scope, query exp.Expression, isSourceSubquery bool, out *[]*Scope, mode scopeTraversalMode) (*Scope, *Scope) {
	before := len(*out)
	if isSourceSubquery {
		wrapperScope := newScope(query, scopeOptions{
			parent:       scope,
			scopeType:    ScopeTypeDerivedTable,
			cteSources:   scope.CTESources,
			outerColumns: query.AliasColumnNames(),
		})
		_traverseScope(wrapperScope, out, mode)
		if len(wrapperScope.SubqueryScopes) > 0 {
			childScope := wrapperScope.SubqueryScopes[len(wrapperScope.SubqueryScopes)-1]
			configureDMLSourceScope(childScope, scope, query)
			return childScope, wrapperScope
		}

		// A Subquery used as a DML source has a From/Join parent, so the regular
		// Subquery boundary logic intentionally leaves its inner query to the outer
		// table traversal. DML uses a dedicated source collector instead, therefore
		// traverse that inner query here while retaining the already-created wrapper.
		if len(*out) > before && (*out)[len(*out)-1] == wrapperScope {
			*out = (*out)[:len(*out)-1]
		}
		inner := query.This()
		if isNilDMLExpression(inner) || !inner.Is(exp.TraitQuery) {
			*out = append(*out, wrapperScope)
			return nil, wrapperScope
		}
		innerBefore := len(*out)
		_traverseScope(wrapperScope.branch(inner, scopeOptions{
			scopeType:    ScopeTypeDerivedTable,
			outerColumns: query.AliasColumnNames(),
		}), out, mode)
		if len(*out) == innerBefore {
			*out = append(*out, wrapperScope)
			return nil, wrapperScope
		}
		childScope := (*out)[len(*out)-1]
		configureDMLSourceScope(childScope, scope, query)
		wrapperScope.SubqueryScopes = append(wrapperScope.SubqueryScopes, childScope)
		*out = append(*out, wrapperScope)
		return childScope, wrapperScope
	}

	_traverseScope(newScope(query, scopeOptions{cteSources: scope.CTESources}), out, mode)
	if len(*out) == before {
		return nil, nil
	}
	return (*out)[len(*out)-1], nil
}

func configureDMLSourceScope(childScope *Scope, parent *Scope, subquery exp.Expression) {
	childScope.Parent = parent
	childScope.ScopeType = ScopeTypeDerivedTable
	childScope.OuterColumns = append([]string(nil), subquery.AliasColumnNames()...)
	childScope.canBeCorrelated = false
}

// dmlRejectNestedJoins fail-closes on a Join node that carries its own `joins` arg (e.g.
// u.joins=[joinV], joinV.joins=[joinW]) — a shape current parsers do not produce (joins are
// flattened onto the source's `joins` list). The source walk only recurses join.This(), so such
// nested joins would be silently missed, emitting a DML-root scope with an incomplete source set
// (a fail-open). Refuse it instead, so the root is omitted (complete-or-none) rather than partial.
func dmlRejectNestedJoins(join exp.Expression, location string) *dmlSourceValidationError {
	nested, err := strictDMLExpressionList(join.Arg("joins"), false, location+".joins")
	if err != nil {
		return err
	}
	if len(nested) > 0 {
		return &dmlSourceValidationError{
			location: location + ".joins",
			reason:   "joins nested on a Join node are not collected — omitting DML root fail-closed",
		}
	}
	return nil
}

func collectDMLSourceCandidates(root exp.Expression) ([]dmlSourceCandidate, *dmlSourceValidationError) {
	candidates := []dmlSourceCandidate{}
	visited := map[exp.Expression]bool{}

	var addCandidate func(exp.Expression, string, bool) *dmlSourceValidationError
	addCandidate = func(expression exp.Expression, location string, isTarget bool) *dmlSourceValidationError {
		if isNilDMLExpression(expression) {
			return &dmlSourceValidationError{location: location, reason: "missing Table or Subquery"}
		}
		if expression.Kind() != exp.KindTable && expression.Kind() != exp.KindSubquery {
			return &dmlSourceValidationError{
				location: location,
				reason:   fmt.Sprintf("expected Table or Subquery, got %s", exp.ClassName(expression.Kind())),
			}
		}
		if visited[expression] {
			return nil
		}
		visited[expression] = true
		candidates = append(candidates, dmlSourceCandidate{
			expression: expression,
			location:   location,
			isTarget:   isTarget,
		})

		joins, err := strictDMLExpressionList(expression.Arg("joins"), false, location+".joins")
		if err != nil {
			return err
		}
		for i, join := range joins {
			joinLocation := fmt.Sprintf("%s.joins[%d]", location, i)
			if join.Kind() != exp.KindJoin {
				return &dmlSourceValidationError{
					location: joinLocation,
					reason:   fmt.Sprintf("expected Join, got %s", exp.ClassName(join.Kind())),
				}
			}
			if err := dmlRejectNestedJoins(join, joinLocation); err != nil {
				return err
			}
			if err := addCandidate(join.This(), joinLocation+".this", false); err != nil {
				return err
			}
		}
		return nil
	}

	target := root.This()
	if !isNilDMLExpression(target) && target.Kind() == exp.KindSchema {
		target = target.This()
	}
	if err := addCandidate(target, "this", true); err != nil {
		return nil, err
	}

	fromUnderscore := root.Arg("from_")
	fromFallback := root.Arg("from")
	if fromUnderscore != nil && fromFallback != nil {
		return nil, &dmlSourceValidationError{
			location: "from_",
			reason:   "both from_ and defensive from fallback are present",
		}
	}
	fromValue := fromUnderscore
	fromLocation := "from_"
	if fromValue == nil {
		fromValue = fromFallback
		fromLocation = "from"
	}
	if fromValue != nil {
		fromExpression, ok := fromValue.(exp.Expression)
		if !ok || isNilDMLExpression(fromExpression) {
			return nil, &dmlSourceValidationError{location: fromLocation, reason: "expected From"}
		}
		if fromExpression.Kind() != exp.KindFrom {
			return nil, &dmlSourceValidationError{
				location: fromLocation,
				reason:   fmt.Sprintf("expected From, got %s", exp.ClassName(fromExpression.Kind())),
			}
		}
		if err := addCandidate(fromExpression.This(), fromLocation+".this", false); err != nil {
			return nil, err
		}
		fromExpressions, err := strictDMLExpressionList(fromExpression.Arg("expressions"), false, fromLocation+".expressions")
		if err != nil {
			return nil, err
		}
		for i, expression := range fromExpressions {
			if err := addCandidate(expression, fmt.Sprintf("%s.expressions[%d]", fromLocation, i), false); err != nil {
				return nil, err
			}
		}
	}

	rootJoins, err := strictDMLExpressionList(root.Arg("joins"), false, "joins")
	if err != nil {
		return nil, err
	}
	for i, join := range rootJoins {
		location := fmt.Sprintf("joins[%d]", i)
		if join.Kind() != exp.KindJoin {
			return nil, &dmlSourceValidationError{
				location: location,
				reason:   fmt.Sprintf("expected Join, got %s", exp.ClassName(join.Kind())),
			}
		}
		if err := dmlRejectNestedJoins(join, location); err != nil {
			return nil, err
		}
		if err := addCandidate(join.This(), location+".this", false); err != nil {
			return nil, err
		}
	}

	usingRequired := root.Kind() == exp.KindMerge
	usingExpressions, err := strictDMLExpressionList(root.Arg("using"), true, "using")
	if err != nil {
		return nil, err
	}
	if usingRequired && len(usingExpressions) == 0 {
		return nil, &dmlSourceValidationError{location: "using", reason: "missing required Table or Subquery"}
	}
	for i, expression := range usingExpressions {
		location := "using"
		if len(usingExpressions) > 1 {
			location = fmt.Sprintf("using[%d]", i)
		}
		if err := addCandidate(expression, location, false); err != nil {
			return nil, err
		}
	}

	return candidates, nil
}

func strictDMLExpressionList(value any, allowScalar bool, location string) ([]exp.Expression, *dmlSourceValidationError) {
	if value == nil {
		return nil, nil
	}

	var expressions []exp.Expression
	switch values := value.(type) {
	case exp.Expression:
		if !allowScalar {
			return nil, &dmlSourceValidationError{location: location, reason: "expected expression list"}
		}
		expressions = []exp.Expression{values}
	case []exp.Expression:
		expressions = values
	case []any:
		expressions = make([]exp.Expression, 0, len(values))
		for i, value := range values {
			expression, ok := value.(exp.Expression)
			if !ok || isNilDMLExpression(expression) {
				return nil, &dmlSourceValidationError{
					location: fmt.Sprintf("%s[%d]", location, i),
					reason:   "expected non-nil expression",
				}
			}
			expressions = append(expressions, expression)
		}
	default:
		return nil, &dmlSourceValidationError{location: location, reason: "expected expression list"}
	}

	for i, expression := range expressions {
		if isNilDMLExpression(expression) {
			return nil, &dmlSourceValidationError{
				location: fmt.Sprintf("%s[%d]", location, i),
				reason:   "expected non-nil expression",
			}
		}
	}
	return expressions, nil
}

func isNilDMLExpression(expression exp.Expression) bool {
	if expression == nil {
		return true
	}
	if node, ok := expression.(*exp.Node); ok {
		return node == nil
	}
	return false
}

func bindDMLSources(scope *Scope, candidates []dmlSourceCandidate, queryScopes map[exp.Expression]*Scope) *dmlSourceValidationError {
	taken := copySources(scope.Sources)
	bound := map[string]any{}
	boundOrder := []string{}

	for _, candidate := range candidates {
		expression := candidate.expression
		var source any
		var sourceName string

		if expression.Kind() == exp.KindSubquery {
			childScope := queryScopes[expression]
			if childScope == nil {
				return &dmlSourceValidationError{
					location: candidate.location,
					reason:   "Subquery did not produce a child scope",
				}
			}
			sourceName = getSourceAlias(expression)
			if candidate.isTarget {
				// The write target must remain the physical AST relation even when it is
				// subquery-shaped; only read-side Subqueries bind to child scopes.
				source = expression
			} else {
				source = childScope
			}
		} else {
			tableName := expression.Name()
			sourceName = expression.AliasOrName()
			source = expression

			if !candidate.isTarget && expression.DbName() == "" {
				if cteSource, ok := scope.Sources[tableName]; ok {
					pivots := expressionsFor(expression, "pivots")
					if len(pivots) > 0 {
						sourceName = pivots[0].Alias()
					} else {
						source = cteSource
					}
				}
			}
		}

		key := sourceName
		if _, alreadyBound := bound[key]; alreadyBound || dmlSourceCollision(scope.Sources[key], source) {
			key = findNewName(taken, sourceName)
		}
		bound[key] = source
		boundOrder = append(boundOrder, key)
		taken[key] = source
	}

	for _, name := range boundOrder {
		scope.Sources[name] = bound[name]
		scope.sourceOrder = appendSourceOrder(scope.sourceOrder, name)
	}
	return nil
}

func dmlSourceCollision(existing any, source any) bool {
	if existing == nil {
		return false
	}
	existingScope, existingIsScope := existing.(*Scope)
	sourceScope, sourceIsScope := source.(*Scope)
	return !existingIsScope || !sourceIsScope || existingScope != sourceScope
}

func _traverseTables(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	sources := map[string]any{}
	sourceOrder := []string{}
	expressions := []exp.Expression{}

	if from := asExpression(scope.Expression.Arg("from_")); from != nil {
		expressions = append(expressions, from.This())
	}
	for _, join := range expressionsFor(scope.Expression, "joins") {
		expressions = append(expressions, join.This())
	}
	if scope.Expression.Kind() == exp.KindTable {
		expressions = append(expressions, scope.Expression)
	}
	expressions = append(expressions, expressionsFor(scope.Expression, "laterals")...)

	for i := 0; i < len(expressions); i++ {
		expression := expressions[i]
		if expression == nil {
			continue
		}
		if expression.Kind() == exp.KindFinal {
			expression = expression.This()
			if expression == nil {
				continue
			}
		}

		if expression.Kind() == exp.KindTable {
			tableName := expression.Name()
			sourceName := expression.AliasOrName()
			var key string

			if source, ok := scope.Sources[tableName]; ok && expression.DbName() == "" {
				pivots := expressionsFor(expression, "pivots")
				if len(pivots) > 0 {
					key = pivots[0].Alias()
					sources[key] = expression
				} else {
					key = sourceName
					sources[key] = source
				}
			} else if _, ok := sources[sourceName]; ok {
				key = findNewName(sources, tableName)
				sources[key] = expression
			} else {
				key = sourceName
				sources[key] = expression
			}
			sourceOrder = appendSourceOrder(sourceOrder, key)

			if expression != scope.Expression {
				for _, join := range expressionsFor(expression, "joins") {
					expressions = append(expressions, join.This())
				}
			}
			continue
		}

		if !expression.Is(exp.TraitDerivedTable) {
			continue
		}

		var lateralSources map[string]any
		scopeType := ScopeTypeDerivedTable
		var scopes *[]*Scope

		if expression.Is(exp.TraitUDTF) {
			lateralSources = sources
			scopeType = ScopeTypeUDTF
			scopes = &scope.UDTFScopes
		} else if isDerivedTable(expression) {
			scopeType = ScopeTypeDerivedTable
			scopes = &scope.DerivedTableScopes
			for _, join := range expressionsFor(expression, "joins") {
				expressions = append(expressions, join.This())
			}
		} else {
			expressions = append(expressions, expression.This())
			for _, join := range expressionsFor(expression, "joins") {
				expressions = append(expressions, join.This())
			}
			continue
		}

		before := len(*out)
		_traverseScope(scope.branch(expression, scopeOptions{
			lateralSources: lateralSources,
			outerColumns:   expression.AliasColumnNames(),
			scopeType:      scopeType,
		}), out, mode)

		var childScope *Scope
		if len(*out) > before {
			childScope = (*out)[len(*out)-1]
		}
		if childScope != nil {
			key := getSourceAlias(expression)
			sources[key] = childScope
			sourceOrder = appendSourceOrder(sourceOrder, key)
			*scopes = append(*scopes, childScope)
			scope.TableScopes = append(scope.TableScopes, childScope)
		}
	}

	for _, name := range sourceOrder {
		scope.Sources[name] = sources[name]
		scope.sourceOrder = appendSourceOrder(scope.sourceOrder, name)
	}
}

func _traverseSubqueries(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	for _, subquery := range scope.Subqueries() {
		before := len(*out)
		_traverseScope(scope.branch(subquery, scopeOptions{scopeType: ScopeTypeSubquery}), out, mode)
		if len(*out) > before {
			scope.SubqueryScopes = append(scope.SubqueryScopes, (*out)[len(*out)-1])
		}
	}
}

func _traverseUdtfs(scope *Scope, out *[]*Scope, mode scopeTraversalMode) {
	udtfExpressions := []exp.Expression{}
	if scope.Expression.Kind() == exp.KindUnnest {
		udtfExpressions = scope.Expression.Expressions()
	} else if scope.Expression.Kind() == exp.KindLateral {
		udtfExpressions = []exp.Expression{scope.Expression.This()}
	}

	sources := map[string]any{}
	sourceOrder := []string{}
	for _, expression := range udtfExpressions {
		if expression == nil || expression.Kind() != exp.KindSubquery {
			continue
		}
		before := len(*out)
		_traverseScope(scope.branch(expression, scopeOptions{
			scopeType:    ScopeTypeSubquery,
			outerColumns: expression.AliasColumnNames(),
		}), out, mode)
		if len(*out) > before {
			childScope := (*out)[len(*out)-1]
			key := getSourceAlias(expression)
			sources[key] = childScope
			sourceOrder = appendSourceOrder(sourceOrder, key)
			scope.SubqueryScopes = append(scope.SubqueryScopes, childScope)
		}
	}

	for _, name := range sourceOrder {
		scope.Sources[name] = sources[name]
		scope.sourceOrder = appendSourceOrder(scope.sourceOrder, name)
	}
}

func WalkInScope(expression exp.Expression, prune func(exp.Expression) bool) []exp.Expression {
	return walkInScope(expression, prune)
}

func walkInScope(expression exp.Expression, prune func(exp.Expression) bool) []exp.Expression {
	if expression == nil {
		return nil
	}
	stack := []exp.Expression{expression}
	out := []exp.Expression{}

	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		out = append(out, node)

		parent := node.Parent()
		isBoundary := node != expression && (node.Kind() == exp.KindCTE || node.Is(exp.TraitQuery)) && (node.Kind() == exp.KindCTE || (parent != nil && (parent.Kind() == exp.KindFrom || parent.Kind() == exp.KindJoin) && isDerivedTable(node)) || (parent != nil && parent.Is(exp.TraitUDTF)) || isUnwrappedQuery(node))
		if isBoundary {
			if node.Kind() == exp.KindSubquery || node.Is(exp.TraitUDTF) {
				for _, key := range []string{"joins", "laterals", "pivots"} {
					for _, arg := range expressionsFor(node, key) {
						out = append(out, walkInScope(arg, nil)...)
					}
				}
			}
			continue
		}

		if prune != nil && prune(node) {
			continue
		}

		keys := exp.ArgKeys(node.Kind())
		for i := len(keys) - 1; i >= 0; i-- {
			value := node.Arg(keys[i])
			switch v := value.(type) {
			case []exp.Expression:
				for j := len(v) - 1; j >= 0; j-- {
					if v[j] != nil {
						stack = append(stack, v[j])
					}
				}
			case exp.Expression:
				if v != nil {
					stack = append(stack, v)
				}
			}
		}
	}
	return out
}

func FindAllInScope(expression exp.Expression, target any) []exp.Expression {
	return findAllInScope(expression, target)
}

func findAllInScope(expression exp.Expression, target any) []exp.Expression {
	out := []exp.Expression{}
	for _, node := range walkInScope(expression, nil) {
		if matchesTarget(node, target) {
			out = append(out, node)
		}
	}
	return out
}

func FindInScope(expression exp.Expression, target any) exp.Expression {
	return findInScope(expression, target)
}

func findInScope(expression exp.Expression, target any) exp.Expression {
	all := findAllInScope(expression, target)
	if len(all) == 0 {
		return nil
	}
	return all[0]
}

func matchesTarget(node exp.Expression, target any) bool {
	if node == nil {
		return false
	}
	switch t := target.(type) {
	case exp.Kind:
		return node.Kind() == t
	case exp.Trait:
		return node.Is(t)
	case []exp.Kind:
		for _, k := range t {
			if node.Kind() == k {
				return true
			}
		}
	case []exp.Trait:
		for _, tr := range t {
			if node.Is(tr) {
				return true
			}
		}
	}
	return false
}

func (s *Scope) _collect() {
	s.tables = nil
	s.ctes = nil
	s.subqueries = nil
	s.derivedTables = nil
	s.udtfs = nil
	s.rawColumns = nil
	s.tableColumns = nil
	s.stars = nil
	s.joinHints = nil
	s.semiAntiJoinTables = map[string]bool{}
	s.columnIndex = map[exp.Expression]bool{}
	s.scansAllSubscopeColumns = false

	for _, node := range s.Walk(nil) {
		if node == s.Expression {
			continue
		}

		if node.Kind() == exp.KindDot && node.IsStar() {
			s.stars = append(s.stars, node)
		} else if node.Kind() == exp.KindColumn {
			s.columnIndex[node] = true
			if node.This() != nil && node.This().Kind() == exp.KindStar {
				s.stars = append(s.stars, node)
			} else {
				s.rawColumns = append(s.rawColumns, node)
			}
		} else if node.Kind() == exp.KindTable && (node.Parent() == nil || node.Parent().Kind() != exp.KindJoinHint) {
			parent := node.Parent()
			if parent != nil && parent.Kind() == exp.KindJoin && isSemiOrAntiJoin(parent) {
				s.semiAntiJoinTables[node.AliasOrName()] = true
			}
			s.tables = append(s.tables, node)
		} else if node.Kind() == exp.KindJoinHint {
			s.joinHints = append(s.joinHints, node)
		} else if node.Kind() == exp.KindLateral || (node.Is(exp.TraitUDTF) && node.Parent() != nil && (node.Parent().Kind() == exp.KindFrom || node.Parent().Kind() == exp.KindJoin)) {
			s.udtfs = append(s.udtfs, node)
		} else if node.Kind() == exp.KindCTE {
			s.ctes = append(s.ctes, node)
		} else if isDerivedTable(node) && isFromOrJoin(node) {
			s.derivedTables = append(s.derivedTables, node)
		} else if isUnwrappedQuery(node) && !isFromOrJoin(node) {
			s.subqueries = append(s.subqueries, node)
		} else if node.Kind() == exp.KindTableColumn {
			s.tableColumns = append(s.tableColumns, node)
		} else if node.Kind() == exp.KindStar {
			parent := node.Parent()
			if truthy(node.Arg("except_")) || parent == nil || parent.Kind() != exp.KindCount {
				s.scansAllSubscopeColumns = true
			}
		}
	}

	s.collected = true
}

func (s *Scope) ensureCollected() {
	if !s.collected {
		s._collect()
	}
}

func (s *Scope) Walk(prune func(exp.Expression) bool) []exp.Expression {
	return walkInScope(s.Expression, prune)
}

func (s *Scope) Find(target any) exp.Expression {
	return findInScope(s.Expression, target)
}

func (s *Scope) FindAll(target any) []exp.Expression {
	return findAllInScope(s.Expression, target)
}

func (s *Scope) Replace(old exp.Expression, new exp.Expression) {
	old.Replace(new)
	s.clearCache()
}

func (s *Scope) Tables() []exp.Expression {
	s.ensureCollected()
	return s.tables
}

func (s *Scope) CTEs() []exp.Expression {
	s.ensureCollected()
	return s.ctes
}

func (s *Scope) DerivedTables() []exp.Expression {
	s.ensureCollected()
	return s.derivedTables
}

func (s *Scope) UDTFs() []exp.Expression {
	s.ensureCollected()
	return s.udtfs
}

func (s *Scope) Subqueries() []exp.Expression {
	s.ensureCollected()
	return s.subqueries
}

func (s *Scope) ScansAllSubscopeColumns() bool {
	s.ensureCollected()
	return s.scansAllSubscopeColumns
}

func (s *Scope) Stars() []exp.Expression {
	s.ensureCollected()
	return s.stars
}

func (s *Scope) ColumnIndex() map[exp.Expression]bool {
	s.ensureCollected()
	return s.columnIndex
}

func (s *Scope) Columns() []exp.Expression {
	if !s.columnsDone {
		s.ensureCollected()
		columns := append([]exp.Expression(nil), s.rawColumns...)
		for _, child := range s.SubqueryScopes {
			columns = append(columns, child.ExternalColumns()...)
		}
		for _, child := range s.UDTFScopes {
			columns = append(columns, child.ExternalColumns()...)
		}
		for _, child := range s.DerivedTableScopes {
			if child.canBeCorrelated {
				columns = append(columns, child.ExternalColumns()...)
			}
		}

		namedSelects := map[string]bool{}
		if s.Expression != nil && s.Expression.Is(exp.TraitQuery) {
			for _, name := range s.Expression.NamedSelects() {
				namedSelects[name] = true
			}
		}

		s.columns = nil
		for _, column := range columns {
			ancestor := column.FindAncestor(exp.KindSelect, exp.KindQualify, exp.KindOrder, exp.KindHaving, exp.KindHint, exp.KindTable, exp.KindStar, exp.KindDistinct)
			var ancestorParent exp.Expression
			if ancestor != nil {
				ancestorParent = ancestor.Parent()
			}
			parentIsWindowOrWithinGroup := ancestorParent != nil && (ancestorParent.Kind() == exp.KindWindow || ancestorParent.Kind() == exp.KindWithinGroup)
			parentIsSelect := ancestorParent != nil && ancestorParent.Kind() == exp.KindSelect
			include := ancestor == nil || column.Text("table") != "" || ancestor.Kind() == exp.KindSelect || (ancestor.Kind() == exp.KindTable && (ancestor.This() == nil || !ancestor.This().Is(exp.TraitFunc))) || ((ancestor.Kind() == exp.KindOrder || ancestor.Kind() == exp.KindDistinct) && (parentIsWindowOrWithinGroup || !parentIsSelect || !namedSelects[column.Name()])) || (ancestor.Kind() == exp.KindStar && column.ArgKey() != "except_")
			if include {
				s.columns = append(s.columns, column)
			}
		}
		s.columnsDone = true
	}
	return s.columns
}

func (s *Scope) TableColumns() []exp.Expression {
	s.ensureCollected()
	return s.tableColumns
}

func (s *Scope) SelectedSources() map[string]selectedSource { return s.selectedSourcesMap() }

func (s *Scope) SelectedSourceNames() []string {
	s.selectedSourcesMap()
	return append([]string(nil), s.selectedSourcesOrder...)
}

func (s *Scope) selectedSourcesMap() map[string]selectedSource {
	if !s.selectedSourcesDone {
		result := map[string]selectedSource{}
		order := []string{}
		for _, ref := range s.References() {
			if s.SemiOrAntiJoinTables()[ref.Name] {
				continue
			}
			if _, ok := result[ref.Name]; ok {
				panic(&sqlerrors.OptimizeError{Msg: "Alias already used: " + ref.Name})
			}
			if source, ok := s.Sources[ref.Name]; ok {
				result[ref.Name] = selectedSource{Node: ref.Node, Source: source}
				order = append(order, ref.Name)
			}
		}
		s.selectedSources = result
		s.selectedSourcesOrder = order
		s.selectedSourcesDone = true
	}
	return s.selectedSources
}

func (s *Scope) References() []reference {
	if !s.referencesDone {
		s.references = nil
		for _, table := range s.Tables() {
			s.references = append(s.references, reference{Name: table.AliasOrName(), Node: table})
		}
		for _, expression := range append(append([]exp.Expression(nil), s.DerivedTables()...), s.UDTFs()...) {
			node := expression
			if len(expressionsFor(expression, "pivots")) == 0 {
				node = expression.Unnest()
			}
			s.references = append(s.references, reference{Name: getSourceAlias(expression), Node: node})
		}
		s.referencesDone = true
	}
	return s.references
}

func (s *Scope) ExternalColumns() []exp.Expression {
	if !s.externalColumnsDone {
		if isSetOperation(s.Expression) && len(s.UnionScopes) == 2 {
			s.externalColumns = append(append([]exp.Expression(nil), s.UnionScopes[0].ExternalColumns()...), s.UnionScopes[1].ExternalColumns()...)
		} else {
			localSourceNames := map[string]bool{}
			for _, ref := range s.References() {
				localSourceNames[ref.Name] = true
			}
			s.externalColumns = nil
			semi := s.SemiOrAntiJoinTables()
			for _, column := range s.Columns() {
				table := column.Text("table")
				if !localSourceNames[table] && !semi[table] {
					s.externalColumns = append(s.externalColumns, column)
				}
			}
		}
		s.externalColumnsDone = true
	}
	return s.externalColumns
}

func (s *Scope) LocalColumns() []exp.Expression {
	if !s.localColumnsDone {
		external := map[exp.Expression]bool{}
		for _, column := range s.ExternalColumns() {
			external[column] = true
		}
		s.localColumns = nil
		for _, column := range s.Columns() {
			if !external[column] {
				s.localColumns = append(s.localColumns, column)
			}
		}
		s.localColumnsDone = true
	}
	return s.localColumns
}

func (s *Scope) UnqualifiedColumns() []exp.Expression {
	var out []exp.Expression
	for _, column := range s.Columns() {
		if column.Text("table") == "" {
			out = append(out, column)
		}
	}
	return out
}

func (s *Scope) JoinHints() []exp.Expression {
	s.ensureCollected()
	return s.joinHints
}

func (s *Scope) Pivots() []exp.Expression {
	if !s.pivotsDone {
		s.pivots = nil
		for _, ref := range s.References() {
			s.pivots = append(s.pivots, expressionsFor(ref.Node, "pivots")...)
		}
		s.pivotsDone = true
	}
	return s.pivots
}

func (s *Scope) SemiOrAntiJoinTables() map[string]bool {
	s.ensureCollected()
	return s.semiAntiJoinTables
}

func (s *Scope) SourceColumns(sourceName string) []exp.Expression {
	var out []exp.Expression
	for _, column := range s.Columns() {
		if column.Text("table") == sourceName {
			out = append(out, column)
		}
	}
	return out
}

func (s *Scope) IsSubquery() bool     { return s.ScopeType == ScopeTypeSubquery }
func (s *Scope) IsDerivedTable() bool { return s.ScopeType == ScopeTypeDerivedTable }
func (s *Scope) IsUnion() bool        { return s.ScopeType == ScopeTypeUnion }
func (s *Scope) IsCTE() bool          { return s.ScopeType == ScopeTypeCTE }
func (s *Scope) IsRoot() bool         { return s.ScopeType == ScopeTypeRoot }
func (s *Scope) IsUDTF() bool         { return s.ScopeType == ScopeTypeUDTF }

func isDMLRootScope(scope *Scope) bool {
	return scope != nil && scope.IsRoot() && scope.Expression != nil && isDMLRootKind(scope.Expression.Kind())
}

func (s *Scope) IsCorrelatedSubquery() bool {
	return s.canBeCorrelated && len(s.ExternalColumns()) > 0
}

func (s *Scope) RenameSource(oldName *string, newName string) {
	old := ""
	if oldName != nil {
		old = *oldName
	}
	if source, ok := s.Sources[old]; ok {
		delete(s.Sources, old)
		s.Sources[newName] = source
		for i, name := range s.sourceOrder {
			if name == old {
				s.sourceOrder[i] = newName
				return
			}
		}
		s.sourceOrder = appendSourceOrder(s.sourceOrder, newName)
	}
}

func (s *Scope) AddSource(name string, source any) {
	s.Sources[name] = source
	s.sourceOrder = appendSourceOrder(s.sourceOrder, name)
	s.clearCache()
}

func (s *Scope) RemoveSource(name string) {
	delete(s.Sources, name)
	for i, existing := range s.sourceOrder {
		if existing == name {
			s.sourceOrder = append(s.sourceOrder[:i], s.sourceOrder[i+1:]...)
			break
		}
	}
	s.clearCache()
}

func (s *Scope) Traverse() []*Scope {
	stack := []*Scope{s}
	result := []*Scope{}
	for len(stack) > 0 {
		scope := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		result = append(result, scope)
		if isDMLRootScope(scope) {
			stack = append(stack, scope.dmlChildScopes...)
		} else {
			stack = append(stack, scope.CTEScopes...)
			stack = append(stack, scope.UnionScopes...)
			stack = append(stack, scope.TableScopes...)
			stack = append(stack, scope.SubqueryScopes...)
		}
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

func (s *Scope) RefCount() map[any]int {
	counts := map[any]int{}
	for _, scope := range s.Traverse() {
		for _, selected := range scope.selectedSourcesMap() {
			counts[selected.Source]++
		}
		for name := range scope.SemiOrAntiJoinTables() {
			if source, ok := scope.Sources[name]; ok {
				counts[source]++
			}
		}
	}
	return counts
}

func (s *Scope) orderedSourceNames() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, name := range s.sourceOrder {
		if _, ok := s.Sources[name]; ok && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	for _, ref := range s.References() {
		if _, ok := s.Sources[ref.Name]; ok && !seen[ref.Name] {
			out = append(out, ref.Name)
			seen[ref.Name] = true
		}
	}
	for name := range s.Sources {
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out
}

func (s *Scope) String() string {
	if s == nil || s.Expression == nil {
		return "Scope<>"
	}
	sql, err := s.Expression.SQL(exp.GenerateOptions{})
	if err != nil {
		return "Scope<" + strings.TrimSpace(s.Expression.ToS()) + ">"
	}
	return "Scope<" + sql + ">"
}
