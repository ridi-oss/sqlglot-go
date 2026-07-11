package optimizer

import (
	"fmt"
	"strings"

	"github.com/sjincho/sqlglot-go/dialects"
	exp "github.com/sjincho/sqlglot-go/expressions"
)

func QualifyTables(expression exp.Expression, db any, catalog any, dialect string, canonicalizeTableAliases bool, onQualify func(exp.Expression)) exp.Expression {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		panic(err)
	}
	nextAliasName := nameSequence("_")

	var dbIdentifier exp.Expression
	if present(db) {
		dbIdentifier = exp.ParseIdentifier(db, dialect)
		// qualify_tables.py:55: mark as table-level so a dialect's normalize_identifier can
		// treat it accordingly (only BigQuery reads meta["is_table"]; inert for base/mysql/pg).
		dbIdentifier.Meta()["is_table"] = true
		dbIdentifier = NormalizeIdentifiers(dbIdentifier, dialect)
	}
	var catalogIdentifier exp.Expression
	if present(catalog) {
		catalogIdentifier = exp.ParseIdentifier(catalog, dialect)
		// qualify_tables.py:59: see the db.meta["is_table"] note above.
		catalogIdentifier.Meta()["is_table"] = true
		catalogIdentifier = NormalizeIdentifiers(catalogIdentifier, dialect)
	}

	qualify := func(table exp.Expression) {
		if table == nil || table.Kind() != exp.KindTable {
			return
		}
		if table.This() != nil && table.This().Kind() == exp.KindIdentifier {
			if dbIdentifier != nil && table.Arg("db") == nil {
				table.Set("db", dbIdentifier.Copy())
			}
			if catalogIdentifier != nil && table.Arg("catalog") == nil && table.Arg("db") != nil {
				table.Set("catalog", catalogIdentifier.Copy())
			}
		}
	}

	if (dbIdentifier != nil || catalogIdentifier != nil) && !expression.Is(exp.TraitQuery) {
		cteNames := map[string]bool{}
		with := asExpression(expression.Arg("with_"))
		if with != nil {
			for _, cte := range with.Expressions() {
				cteNames[cte.AliasOrName()] = true
			}
		}
		for _, node := range expression.WalkWithPrune(true, func(n exp.Expression) bool { return n.Is(exp.TraitQuery) }) {
			if node.Kind() == exp.KindTable && !cteNames[node.Name()] {
				qualify(node)
			}
		}
	}

	setAlias := func(expression exp.Expression, canonicalAliases map[string]string, targetAlias string, scope *Scope, normalize bool, columns []exp.Expression) {
		alias := asExpression(expression.Arg("alias"))
		if alias == nil {
			alias = exp.TableAlias(exp.Args{})
		}

		newAliasName := ""
		if canonicalizeTableAliases {
			newAliasName = nextAliasName()
			key := alias.Name()
			if key == "" {
				key = targetAlias
			}
			canonicalAliases[key] = newAliasName
		} else if alias.Name() == "" {
			if targetAlias != "" {
				newAliasName = targetAlias
			} else {
				newAliasName = nextAliasName()
			}
			if normalize && targetAlias != "" {
				newAliasName = NormalizeIdentifiersString(newAliasName, dialect).Name()
			}
		} else {
			return
		}

		alias.Set("this", exp.ToIdentifier(newAliasName))
		if len(columns) > 0 {
			copied := make([]exp.Expression, 0, len(columns))
			for _, column := range columns {
				if column != nil {
					copied = append(copied, column.Copy())
				}
			}
			alias.Set("columns", copied)
		}
		expression.Set("alias", alias)
		if scope != nil {
			scope.RenameSource(nil, newAliasName)
		}
	}

	for _, scope := range traverseScope(expression) {
		localColumns := scope.LocalColumns()
		canonicalAliases := map[string]string{}

		for _, query := range scope.Subqueries() {
			subquery := query.Parent()
			if subquery != nil && subquery.Kind() == exp.KindSubquery {
				unwrapped := subquery.Unwrap()
				if unwrapped.Parent() != nil && unwrapped.Parent().Kind() == exp.KindCreate && unwrapped != subquery {
					unwrapped.Set("this", subquery)
				} else {
					unwrapped.Replace(subquery)
				}
			}
		}

		for _, derivedTable := range scope.DerivedTables() {
			unnested := derivedTable.Unnest()
			if unnested != nil && unnested.Kind() == exp.KindTable {
				joins := unnested.Arg("joins")
				unnested.Set("joins", nil)
				selectExpr := exp.Select(exp.Args{
					"expressions": []exp.Expression{exp.Star(exp.Args{})},
					"from_":       exp.From(exp.Args{"this": unnested.Copy()}),
				})
				if derivedTable.This() != nil {
					derivedTable.This().Replace(selectExpr)
				}
				if derivedTable.This() != nil {
					derivedTable.This().Set("joins", joins)
				}
			}

			setAlias(derivedTable, canonicalAliases, "", scope, false, nil)
			if pivot, ok := seqGet(expressionsFor(derivedTable, "pivots"), 0); ok {
				setAlias(pivot, canonicalAliases, "", nil, false, nil)
			}
		}

		tableAliases := map[string]exp.Expression{}

		for _, name := range scope.orderedSourceNames() {
			source := scope.Sources[name]
			if table, ok := source.(exp.Expression); ok && table != nil && table.Kind() == exp.KindTable {
				isRealTableSource := name != ""
				if pivot, ok := seqGet(expressionsFor(table, "pivots"), 0); ok && pivot != nil {
					name = table.Name()
				}

				tableThis := table.This()
				tableAlias := asExpression(table.Arg("alias"))
				var functionColumns []exp.Expression
				if tableThis != nil && tableThis.Is(exp.TraitFunc) {
					if tableAlias == nil {
						functionColumns = identifiersFromStrings(d.DefaultFunctionsColumnNames[tableThis.Kind()])
					} else if columns := expressionsFor(tableAlias, "columns"); len(columns) > 0 {
						functionColumns = columns
					} else if _, ok := d.DefaultFunctionsColumnNames[tableThis.Kind()]; ok {
						functionColumns = []exp.Expression{exp.ToIdentifier(table.AliasOrName())}
						table.Set("alias", nil)
						name = ""
					}
				}

				targetAlias := name
				if targetAlias == "" {
					targetAlias = table.Name()
				}
				setAlias(table, canonicalAliases, targetAlias, nil, true, functionColumns)

				sourceFQN := partsName(table.Parts())
				hadExplicitAlias := tableAlias != nil && tableAlias.Name() != ""
				if _, ok := tableAliases[sourceFQN]; !hadExplicitAlias || !ok {
					if alias := asExpression(table.Arg("alias")); alias != nil && alias.This() != nil {
						tableAliases[sourceFQN] = alias.This().Copy()
					}
				}

				if pivot, ok := seqGet(expressionsFor(table, "pivots"), 0); ok && pivot != nil {
					targetAlias := ""
					if unpivot, _ := pivot.Arg("unpivot").(bool); unpivot {
						targetAlias = table.Alias()
					}
					setAlias(pivot, canonicalAliases, targetAlias, nil, true, nil)

					if _, ok := scope.Sources[table.AliasOrName()].(*Scope); ok {
						continue
					}
				}

				if isRealTableSource {
					qualify(table)
					if onQualify != nil {
						onQualify(table)
					}
				}
			} else if sourceScope, ok := source.(*Scope); ok && sourceScope.IsUDTF() {
				udtf := sourceScope.Expression
				setAlias(udtf, canonicalAliases, "", nil, false, nil)

				tableAlias := asExpression(udtf.Arg("alias"))
				if udtf.Kind() == exp.KindValues && tableAlias != nil && len(expressionsFor(tableAlias, "columns")) == 0 {
					columnAliases := d.GenerateValuesAliases(udtf)
					for _, alias := range columnAliases {
						NormalizeIdentifiers(alias, dialect)
					}
					tableAlias.Set("columns", columnAliases)
				}
			}
		}

		for _, table := range scope.Tables() {
			parent := table.Parent()
			if table.Alias() == "" && parent != nil && (parent.Kind() == exp.KindFrom || parent.Kind() == exp.KindJoin) {
				setAlias(table, canonicalAliases, table.Name(), nil, false, nil)
			}
		}

		for _, column := range localColumns {
			columnTable := column.Text("table")
			if column.Text("db") != "" {
				parts := column.Parts()
				if len(parts) > 1 {
					tableAlias := tableAliases[partsName(parts[:len(parts)-1])]
					if tableAlias != nil {
						for _, key := range []string{"table", "db", "catalog"} {
							column.Set(key, nil)
						}
						column.Set("table", tableAlias.Copy())
					}
				}
			} else if len(canonicalAliases) > 0 && columnTable != "" {
				if canonicalTable := canonicalAliases[columnTable]; canonicalTable != "" && canonicalTable != columnTable {
					column.Set("table", exp.ToIdentifier(canonicalTable))
				}
			}
		}
	}

	return expression
}

func present(value any) bool {
	if value == nil {
		return false
	}
	if s, ok := value.(string); ok {
		return s != ""
	}
	return fmt.Sprint(value) != ""
}

func identifiersFromStrings(names []string) []exp.Expression {
	if len(names) == 0 {
		return nil
	}
	out := make([]exp.Expression, 0, len(names))
	for _, name := range names {
		out = append(out, exp.ToIdentifier(name))
	}
	return out
}

func partsName(parts []exp.Expression) string {
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != nil {
			names = append(names, part.Name())
		}
	}
	return strings.Join(names, ".")
}
