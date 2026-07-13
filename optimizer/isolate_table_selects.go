package optimizer

import (
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/schema"
)

func IsolateTableSelects(expression exp.Expression, schemaArg any, dialect any) exp.Expression {
	s, err := schema.EnsureSchema(schemaArg, dialect, true)
	if err != nil {
		panic(err)
	}

	for _, scope := range traverseScopeForOptimizer(expression) {
		selected := scope.selectedSourcesMap()
		if len(selected) == 1 {
			continue
		}

		for _, item := range selected {
			source, ok := item.Source.(exp.Expression)
			if !ok || source == nil {
				continue
			}
			parent := source.Parent()
			var grandparent exp.Expression
			if parent != nil {
				grandparent = parent.Parent()
			}
			columnNames, err := s.ColumnNames(source, false, "", nil)
			if err != nil {
				panic(err)
			}

			if source.Kind() != exp.KindTable || len(columnNames) == 0 || (parent != nil && parent.Kind() == exp.KindSubquery) || (grandparent != nil && grandparent.Kind() == exp.KindTable) {
				continue
			}

			if source.Alias() == "" {
				panic(&sqlerrors.OptimizeError{Msg: "Tables require an alias. Run qualify_tables optimization."})
			}

			aliasOrName := source.AliasOrName()
			aliasName := source.Alias()
			aliased := exp.AliasExpr(source.Copy(), aliasOrName, true)
			selectExpr := exp.Select(exp.Args{
				"expressions": []exp.Expression{exp.Star(exp.Args{})},
				"from_":       exp.From(exp.Args{"this": aliased}),
			})
			subquery := exp.Subquery(exp.Args{
				"this":  selectExpr,
				"alias": exp.TableAlias(exp.Args{"this": exp.ToIdentifier(aliasName)}),
			})
			source.Replace(subquery)
		}
	}
	return expression
}
