package optimizer

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// sourceVisibility is what a column at one position of a SELECT can see.
type sourceVisibility struct {
	// all: every selected source (SELECT list, WHERE, GROUP BY, ...).
	all bool
	// names: otherwise the visible sources, in FROM order.
	names []string
}

// visibility ports the engines' FROM-clause scoping (DEVIATIONS §1.27): an ON sees the items
// joined up to it within its own join group, a LATERAL/UNNEST/function item sees the items
// before it, and a derived table sees none.
func (r *Resolver) visibility(node exp.Expression) sourceVisibility {
	all := sourceVisibility{all: true}
	query := r.scope.Expression
	if node == nil || query == nil || query.Kind() != exp.KindSelect {
		return all
	}
	// path[0] is the child of query that leads to node.
	var path []exp.Expression
	n := node
	for ; n != nil && n != query; n = n.Parent() {
		path = append(path, n)
	}
	if n == nil || len(path) == 0 {
		return all
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	var head exp.Expression
	if from := asExpression(query.Arg("from_")); from != nil {
		head = from.This()
	}
	if head == nil {
		return all
	}
	top := path[0]
	switch {
	case top.Kind() == exp.KindFrom && len(path) > 1 && path[1] == head:
		return r.itemVisibility(head, path[1:], nil)
	case top.Kind() == exp.KindJoin && top.ArgKey() == "joins":
		vis := r.chainVisibility(r.itemNames(head), expressionsFor(query, "joins"), path, nil)
		// Hive's LATERAL VIEW parses to the same tree before or after a JOIN, so an ON sees it.
		for _, lateral := range expressionsFor(query, "laterals") {
			vis.names = append(vis.names, r.sourceNames(lateral.AliasOrName())...)
		}
		return vis
	}
	return all
}

// chainVisibility handles a position under path[0], a join of the chain `head joins...`.
// preceding holds the enclosing chains' earlier items, visible to lateral items only.
func (r *Resolver) chainVisibility(headNames []string, joins []exp.Expression, path []exp.Expression, preceding []string) sourceVisibility {
	index := -1
	for i, join := range joins {
		if join == path[0] {
			index = i
			break
		}
	}
	if index < 0 {
		return sourceVisibility{all: true}
	}
	join := joins[index]

	// items[0] is the head; items[k+1] is joins[k]. A semi/anti-joined table is visible only in
	// its own ON.
	items := [][]string{headNames}
	for k := 0; k < index; k++ {
		if isSemiOrAntiJoin(joins[k]) {
			items = append(items, nil)
		} else {
			items = append(items, r.itemNames(joins[k].This()))
		}
	}

	if len(path) > 1 && path[1] == join.This() {
		return r.itemVisibility(join.This(), path[1:], append(append([]string(nil), preceding...), flatten(items)...))
	}

	items = append(items, r.itemNames(join.This()))
	// Standard SQL binds JOIN tighter than a comma, so an ON does not see the items before the
	// last comma. A dialect that joins left to right parses its comma as a CROSS JOIN.
	start := 0
	for k := index; k >= 0; k-- {
		if isCommaJoin(joins[k]) {
			start = k + 1
			break
		}
	}
	return sourceVisibility{names: flatten(items[start:])}
}

// itemVisibility handles a position inside the FROM item path[0].
func (r *Resolver) itemVisibility(item exp.Expression, path []exp.Expression, preceding []string) sourceVisibility {
	// `b JOIN c ON ...` nested under a join: the item heads its own chain.
	if len(path) > 1 && path[1].Kind() == exp.KindJoin && path[1].ArgKey() == "joins" {
		return r.chainVisibility(r.headNames(item), expressionsFor(item, "joins"), path[1:], preceding)
	}
	// A PIVOT/UNPIVOT reads the item it is attached to.
	if len(path) > 1 && path[1].ArgKey() == "pivots" {
		return sourceVisibility{names: r.headNames(item)}
	}
	if item.Kind() == exp.KindValues {
		return sourceVisibility{}
	}
	if item.Kind() == exp.KindSubquery {
		if inner := parenthesizedItem(item); inner == nil {
			// A derived table sees no sibling.
			return sourceVisibility{}
		} else if len(path) > 1 && path[1] == inner {
			return r.itemVisibility(inner, path[1:], preceding)
		}
	}
	return sourceVisibility{names: preceding}
}

// parenthesizedItem returns the FROM item a Subquery merely parenthesizes, else nil.
func parenthesizedItem(item exp.Expression) exp.Expression {
	inner := item.This()
	if inner != nil && (inner.Kind() == exp.KindTable || inner.Kind() == exp.KindSubquery) {
		return inner
	}
	return nil
}

// itemNames lists the sources a FROM item contributes, nested joins included.
func (r *Resolver) itemNames(item exp.Expression) []string {
	if item == nil {
		return nil
	}
	names := r.headNames(item)
	for _, join := range expressionsFor(item, "joins") {
		if !isSemiOrAntiJoin(join) {
			names = append(names, r.itemNames(join.This())...)
		}
	}
	return names
}

// headNames lists the sources of a FROM item without the joins chained to it.
func (r *Resolver) headNames(item exp.Expression) []string {
	// `((SELECT ...)) AS d` is a derived table named d, not a join group.
	if names := r.sourceNames(item.AliasOrName()); names != nil {
		return names
	}
	if inner := parenthesizedItem(item); item.Kind() == exp.KindSubquery && inner != nil {
		return r.itemNames(inner)
	}
	return nil
}

func (r *Resolver) sourceNames(name string) []string {
	if _, ok := r.scope.Sources[name]; ok && name != "" {
		return []string{name}
	}
	return nil
}

// isCommaJoin reports a join the parser read from a comma; MySQL's bare `a JOIN b` is the same
// node without the mark.
func isCommaJoin(join exp.Expression) bool {
	return join.MetaGet("comma") == true
}

func flatten(lists [][]string) []string {
	var out []string
	for _, list := range lists {
		out = append(out, list...)
	}
	return out
}

type havingSight int

const (
	havingSees havingSight = iota
	// havingHides: the name is not this query's; an outer query has it or it is unknown.
	havingHides
	// havingUnsure: inside a call that may or may not be an aggregate.
	havingUnsure
)

// mysqlAggregates: MySQL 8.4 Reference Manual, "Aggregate Function Descriptions" and ST_Collect.
var mysqlAggregates = map[string]bool{
	"AVG": true, "BIT_AND": true, "BIT_OR": true, "BIT_XOR": true, "COUNT": true, "GROUP_CONCAT": true,
	"JSON_ARRAYAGG": true, "JSON_OBJECTAGG": true, "MAX": true, "MIN": true, "STD": true, "STDDEV": true,
	"STDDEV_POP": true, "STDDEV_SAMP": true, "SUM": true, "VAR_POP": true, "VAR_SAMP": true, "VARIANCE": true,
	"ST_COLLECT": true,
}

// mysqlAggregateArg reports whether the column is an argument of an aggregate in this HAVING,
// where MySQL reads a FROM column before a select alias.
func (r *Resolver) mysqlAggregateArg(column, having exp.Expression) bool {
	if !r.havingSelectedOnly {
		return false
	}
	for n := column.Parent(); n != nil && n != having; n = n.Parent() {
		if n.Is(exp.TraitAggFunc) || (n.Kind() == exp.KindAnonymous && mysqlAggregates[strings.ToUpper(n.Name())]) {
			return true
		}
	}
	return false
}

// mysqlAggregates reports whether the query has a GROUP BY or an aggregate of its own.
func (r *Resolver) mysqlAggregates(query, having exp.Expression) bool {
	if query.Arg("group") != nil {
		return true
	}
	for _, item := range append(query.Selects(), having) {
		for _, node := range walkInScope(item, nil) {
			aggregate := node.Is(exp.TraitAggFunc) || (node.Kind() == exp.KindAnonymous && mysqlAggregates[strings.ToUpper(node.Name())])
			// A window function does not aggregate the query.
			if aggregate && (node.Parent() == nil || node.Parent().Kind() != exp.KindWindow) {
				return true
			}
		}
	}
	return false
}

// mysqlHaving ports MySQL's HAVING lookup (DEVIATIONS §1.27) for a column at or below this
// query's HAVING: outside an aggregate, FROM supplies a name only if the select list or GROUP BY
// has it as a column. The table is set when that column is written qualified.
func (r *Resolver) mysqlHaving(column exp.Expression) (sight havingSight, table string, nested bool) {
	query := r.scope.Expression
	if !r.havingSelectedOnly || query == nil || query.Kind() != exp.KindSelect || column.Text("table") != "" {
		return havingSees, "", false
	}
	having := asExpression(query.Arg("having"))
	if having == nil {
		return havingSees, "", false
	}
	// aggregate: the column is an aggregate's argument; outer: that aggregate is this query's.
	aggregate, outer, unsure := false, false, false
	// direct: the column sits in the HAVING of its own subquery, outside an aggregate.
	direct := false
	n := column.Parent()
	for ; n != nil && n != having; n = n.Parent() {
		switch {
		case n.Kind() == exp.KindHaving:
			direct = direct || (!nested && !aggregate)
		case n.Kind() == exp.KindSelect:
			nested, outer = true, false
		case n.Is(exp.TraitAggFunc), n.Kind() == exp.KindAnonymous && mysqlAggregates[strings.ToUpper(n.Name())]:
			aggregate, outer = true, true
		case n.Kind() == exp.KindAnonymous:
			// Under opaque_functions a call that is no built-in may be a loadable aggregate.
			unsure = true
		}
	}
	if n == nil {
		return havingSees, "", false
	}
	r.mysqlHavingNames()
	// A subquery of the HAVING reads a select alias in its own HAVING, elsewhere only when FROM
	// lacks the name. The column stays an alias reference no outer query sees.
	if nested && !outer && r.havingAliases[strings.ToLower(column.Name())] && (direct || (r.GetTable(column) == nil && !r.ambiguous(column))) {
		r.scope.markHavingAliasRef(column)
		return havingHides, "", nested
	}
	if aggregate {
		return havingSees, "", false
	}
	if table, ok := r.havingNames[strings.ToLower(column.Name())]; ok || r.havingAll {
		return havingSees, table, nested
	}
	// A subquery of the HAVING sees every FROM column unless the query groups or aggregates.
	if nested && !r.mysqlAggregates(query, having) {
		return havingSees, "", nested
	}
	if unsure {
		return havingUnsure, "", nested
	}
	return havingHides, "", nested
}

// mysqlHavingNames collects the names the query's HAVING finds outside FROM, each with the table
// of its column when written. MySQL's order: a select alias, a GROUP BY column, an unaliased
// select column, the column behind an alias. USING expansion rewrites these, so it calls this first.
func (r *Resolver) mysqlHavingNames() {
	query := r.scope.Expression
	if r.havingNames != nil || !r.havingSelectedOnly || query == nil || query.Kind() != exp.KindSelect {
		return
	}
	r.havingNames = map[string]string{}
	r.havingAliases = map[string]bool{}
	add := func(item exp.Expression) {
		for item.Kind() == exp.KindParen {
			item = item.This()
		}
		names, table := []string{item.Name()}, item.Text("table")
		switch {
		case item.Kind() == exp.KindStar:
			r.havingAll = true
			return
		case item.Kind() == exp.KindColumn && item.IsStar():
			names = r.GetSourceColumns(table)
		case item.Kind() != exp.KindColumn:
			return
		}
		for _, name := range names {
			if _, ok := r.havingNames[strings.ToLower(name)]; !ok {
				r.havingNames[strings.ToLower(name)] = table
			}
		}
	}
	var aliased []exp.Expression
	for _, item := range query.Selects() {
		if item.Kind() == exp.KindAlias {
			r.havingNames[strings.ToLower(item.Alias())] = ""
			r.havingAliases[strings.ToLower(item.Alias())] = true
			aliased = append(aliased, item.This())
		}
	}
	if group := asExpression(query.Arg("group")); group != nil {
		for _, item := range group.Expressions() {
			add(item)
		}
	}
	for _, item := range query.Selects() {
		if item.Kind() != exp.KindAlias {
			add(item)
		}
	}
	for _, item := range aliased {
		add(item)
	}
}
