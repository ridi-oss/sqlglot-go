package optimizer

import (
	"fmt"
	"strings"

	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/schema"
)

// normalizeRelationIdentifier folds a bare relation name (a schema/catalog arg or a search-path entry)
// with the dialect's strategy in the correct RELATION-role context. It parses the name, gives it a
// parent under argKey ("schema" or "catalog") so the role-aware MySQL lctn=0 strategy preserves it — a
// detached identifier has no parent and would be misread as a foldable column, folding a schema name
// like `App` to `app` — and so the INFORMATION_SCHEMA exception can fire (a search-path/schema entry that
// is information_schema folds). Quoting is respected exactly as NormalizeIdentifier does; non-role-aware
// strategies ignore the parent, so their result is unchanged from a detached normalization.
func normalizeRelationIdentifier(name exp.IdentifierName, argKey string, d *dialects.Dialect) exp.Expression {
	// Copy so that when `name` is an existing Identifier node (shared with another AST) we neither
	// reparent it onto the throwaway table below nor fold its text in place — upstream leaves the
	// caller's node untouched. A string input already yields a fresh node; the copy is then cheap.
	id := exp.ParseIdentifier(name, d.Name).Copy()
	// qualify_tables.py:55/59 parity: mark table-level for a dialect whose normalize reads the meta
	// (only BigQuery does; inert for base/mysql/pg, whose role now comes from the parent below).
	id.Meta()["is_table"] = true
	exp.Table(exp.Args{argKey: id}) // throwaway parent for role (and schema-sibling) resolution
	// Go through NormalizeIdentifiers (not d.NormalizeIdentifier) so an identifier carrying the
	// case_sensitive meta is skipped, matching upstream and the fixed-DB path. Pass the resolved *d
	// (not d.Name, which drops the normalization strategy) so the same strategy is applied.
	return NormalizeIdentifiers(id, d)
}

func QualifyTables(expression exp.Expression, schemaName exp.IdentifierName, catalog exp.IdentifierName, dialect dialects.DialectType, canonicalizeTableAliases bool, onQualify func(exp.Expression), searchPath []string, s schema.Schema) exp.Expression {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		panic(err)
	}
	nextAliasName := nameSequence("_")

	var schemaIdentifier exp.Expression
	if present(schemaName) {
		schemaIdentifier = normalizeRelationIdentifier(schemaName, "schema", d)
	}
	var catalogIdentifier exp.Expression
	if present(catalog) {
		catalogIdentifier = normalizeRelationIdentifier(catalog, "catalog", d)
	}

	searchPathMode := len(searchPath) > 0
	var searchPathIdentifiers []exp.Expression
	if searchPathMode {
		searchPathIdentifiers = make([]exp.Expression, 0, len(searchPath))
		for _, candidate := range searchPath {
			if candidate == "" {
				continue
			}
			searchPathIdentifiers = append(searchPathIdentifiers, normalizeRelationIdentifier(candidate, "schema", d))
		}
	}

	supportsSchema := false
	if searchPathMode && s != nil {
		for _, tableArg := range s.SupportedTableArgs() {
			if tableArg == "schema" {
				supportsSchema = true
				break
			}
		}
	}

	qualify := func(table exp.Expression) {
		if table == nil || table.Kind() != exp.KindTable {
			return
		}
		if table.This() != nil && table.This().Kind() == exp.KindIdentifier {
			if table.Arg("schema") == nil {
				if searchPathMode {
					// Unlike .reference/sqlglot-v30.12.0/sqlglot/optimizer/qualify_tables.py:62-67,
					// this opt-in extension requires schema-backed proof instead of applying one fixed schema.
					if supportsSchema {
						for _, candidate := range searchPathIdentifiers {
							probe := exp.Table(exp.Args{"this": table.This().Copy(), "schema": candidate.Copy()})
							// Fold the probe's table name in the candidate schema's context: an
							// INFORMATION_SCHEMA table is case-insensitive, so `FROM Tables` must probe the
							// normalized `tables` key. A no-op for ordinary (case-sensitive) schemas.
							NormalizeIdentifiers(probe, d)
							mapping, err := s.Find(probe, false, false)
							if err != nil {
								panic(err)
							}
							if mapping != nil {
								table.Set("schema", candidate.Copy())
								// The schema is now known: re-fold the table name in that context so the
								// stamped identity matches the schema key (again, only INFORMATION_SCHEMA
								// table names change; every other relation name is preserved).
								NormalizeIdentifiers(table.This(), d)
								break
							}
						}
					}
				} else if schemaIdentifier != nil {
					table.Set("schema", schemaIdentifier.Copy())
				}
			}
			if !searchPathMode && catalogIdentifier != nil && table.Arg("catalog") == nil && table.Arg("schema") != nil {
				table.Set("catalog", catalogIdentifier.Copy())
			}
		}
	}

	if (schemaIdentifier != nil || catalogIdentifier != nil || searchPathMode) && !expression.Is(exp.TraitQuery) {
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
		} else {
			return
		}

		alias.Set("this", exp.ToIdentifier(newAliasName))

		// divergence (DEVIATIONS.md §1.2): fold the injected default alias WITH its relation-level
		// role. Upstream folds a bare, parentless identifier — normalize_identifiers(new_alias_name)
		// BEFORE it is attached under the TableAlias (qualify_tables.py:93). That works for its own
		// strategies but starves the port's role-aware MySQLCaseSensitiveTableNames (lctn=0) of the
		// parent it reads: a parentless alias is misread as column-level and folded to lowercase, so
		// it no longer matches the case-preserved column qualifier and scope resolution can't bind
		// (empty lineage). Normalizing AFTER the attach above gives the identifier its
		// TableAlias("this") parent (isRelationLevelIdentifier), keeping a table alias case-sensitive
		// while still honoring quoting for the ASCII strategies — output-identical to upstream for
		// every strategy except the non-upstream role-aware one this fixes.
		if normalize && !canonicalizeTableAliases && targetAlias != "" && alias.This() != nil {
			NormalizeIdentifiers(alias.This(), dialect)
			newAliasName = alias.This().Name()
		}
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

	for _, scope := range traverseScopeForOptimizer(expression) {
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
			if column.Text("schema") != "" {
				parts := column.Parts()
				if len(parts) > 1 {
					tableAlias := tableAliases[partsName(parts[:len(parts)-1])]
					if tableAlias != nil {
						for _, key := range []string{"table", "schema", "catalog"} {
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
