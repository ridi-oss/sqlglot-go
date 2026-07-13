package optimizer

import (
	"fmt"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// SourceKind classifies an AST report entry by its resolved scope source.
type SourceKind int

const (
	// Unresolved is intentionally the zero value so missing report entries fail closed.
	Unresolved SourceKind = iota
	Physical
	CTE
	Derived
	Subquery
)

func (k SourceKind) String() string {
	switch k {
	case Unresolved:
		return "Unresolved"
	case Physical:
		return "Physical"
	case CTE:
		return "CTE"
	case Derived:
		return "Derived"
	case Subquery:
		return "Subquery"
	default:
		return fmt.Sprintf("SourceKind(%d)", int(k))
	}
}

// ResolvedSource describes a resolved source. Identity fields are populated only for Physical sources.
type ResolvedSource struct {
	Catalog, Schema, Table string
	Kind                   SourceKind
}

func populateResolutionReport(expression exp.Expression, report map[exp.Expression]ResolvedSource) {
	scopes := traverseScope(expression)
	populateResolutionReportForScopes(expression, scopes, report)
}

func populateResolutionReportForScopes(expression exp.Expression, scopes []*Scope, report map[exp.Expression]ResolvedSource) {
	if report == nil {
		return
	}

	classifiedNodes := map[exp.Expression]struct{}{}
	for _, scope := range scopes {
		if scope == nil {
			continue
		}

		selectedNodes := map[exp.Expression]struct{}{}
		for _, selected := range scope.SelectedSources() {
			if selected.Node == nil {
				continue
			}
			selectedNodes[selected.Node] = struct{}{}
			classifiedNodes[selected.Node] = struct{}{}
			report[selected.Node] = classifySelectedSource(selected.Source)
		}

		semiAntiTables := scope.SemiOrAntiJoinTables()
		for _, ref := range scope.References() {
			if ref.Node == nil || semiAntiTables[ref.Name] {
				continue
			}
			if _, selected := selectedNodes[ref.Node]; selected {
				continue
			}
			classifiedNodes[ref.Node] = struct{}{}
			report[ref.Node] = ResolvedSource{Kind: Unresolved}
		}

		if scope.IsSubquery() && scope.Expression != nil {
			classifiedNodes[scope.Expression] = struct{}{}
			report[scope.Expression] = ResolvedSource{Kind: Subquery}
		}
	}

	populateDMLResolutionReport(expression, report, classifiedNodes)
}

func classifySelectedSource(source any) ResolvedSource {
	switch source := source.(type) {
	case exp.Expression:
		if source != nil && source.Kind() == exp.KindTable {
			return physicalSource(source)
		}
	case *Scope:
		if source == nil {
			return ResolvedSource{Kind: Unresolved}
		}
		switch {
		case source.IsCTE():
			return ResolvedSource{Kind: CTE}
		case source.IsDerivedTable(), source.IsUDTF():
			return ResolvedSource{Kind: Derived}
		case source.IsSubquery():
			return ResolvedSource{Kind: Subquery}
		}
	}
	return ResolvedSource{Kind: Unresolved}
}

func physicalSource(table exp.Expression) ResolvedSource {
	return ResolvedSource{
		Catalog: table.CatalogName(),
		Schema:  table.DbName(),
		Table:   table.Name(),
		Kind:    Physical,
	}
}

func populateDMLResolutionReport(expression exp.Expression, report map[exp.Expression]ResolvedSource, classifiedNodes map[exp.Expression]struct{}) {
	if expression == nil {
		return
	}

	var target exp.Expression
	switch expression.Kind() {
	case exp.KindUpdate, exp.KindDelete, exp.KindMerge:
		target = expression.This()
	case exp.KindInsert:
		target = expression.This()
		if target != nil && target.Kind() == exp.KindSchema {
			target = target.This()
		}
	default:
		return
	}
	if target == nil || target.Kind() != exp.KindTable {
		target = nil
	}

	for _, node := range expression.WalkWithPrune(true, func(node exp.Expression) bool {
		return node != expression && node.Is(exp.TraitQuery)
	}) {
		if node.Kind() != exp.KindTable {
			continue
		}
		if node == target {
			classifiedNodes[node] = struct{}{}
			report[node] = physicalSource(node)
			continue
		}
		if _, classified := classifiedNodes[node]; !classified {
			classifiedNodes[node] = struct{}{}
			report[node] = ResolvedSource{Kind: Unresolved}
		}
	}
}
