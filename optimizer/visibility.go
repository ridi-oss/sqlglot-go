package optimizer

import (
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
