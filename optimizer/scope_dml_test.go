package optimizer

import (
	"fmt"
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/schema"
)

func finalDMLRoot(t *testing.T, scopes []*Scope, kind exp.Kind) *Scope {
	t.Helper()
	if len(scopes) == 0 {
		t.Fatalf("traverseScope returned no scopes, want final %s root", exp.ClassName(kind))
	}
	root := scopes[len(scopes)-1]
	if root.Expression == nil || root.Expression.Kind() != kind {
		got := "<nil>"
		if root.Expression != nil {
			got = exp.ClassName(root.Expression.Kind())
		}
		t.Fatalf("final scope kind = %s, want %s", got, exp.ClassName(kind))
	}
	if !root.IsRoot() {
		t.Fatalf("final %s scope type = %v, want root", exp.ClassName(kind), root.ScopeType)
	}
	return root
}

func assertDMLSourceKeys(t *testing.T, scope *Scope, want ...string) {
	t.Helper()
	assertStringSet(t, sourceNameSet(scope), want...)
	gotOrder := scope.orderedSourceNames()
	if strings.Join(gotOrder, ",") != strings.Join(want, ",") {
		t.Fatalf("source order = %v, want %v", gotOrder, want)
	}
}

func assertQualifiedColumnsBind(t *testing.T, scope *Scope) {
	t.Helper()
	for _, column := range scope.Columns() {
		qualifier := column.Text("table")
		if qualifier == "" {
			continue
		}
		if _, ok := scope.Sources[qualifier]; !ok {
			t.Fatalf("qualified column %q has no source in %v", sqlOf(t, column), scope.orderedSourceNames())
		}
	}
}

func assertDMLTableSource(t *testing.T, scope *Scope, name string) exp.Expression {
	t.Helper()
	source, ok := scope.Sources[name].(exp.Expression)
	if !ok || source == nil || source.Kind() != exp.KindTable {
		t.Fatalf("source %q = %T, want exp.Table", name, scope.Sources[name])
	}
	return source
}

func assertDMLScopeSource(t *testing.T, scope *Scope, name string) *Scope {
	t.Helper()
	source, ok := scope.Sources[name].(*Scope)
	if !ok || source == nil {
		t.Fatalf("source %q = %T, want *Scope", name, scope.Sources[name])
	}
	return source
}

func dmlTable(name string) exp.Expression {
	return exp.Table_(name, nil, nil, nil, nil)
}

func dmlSubquery(alias, table string) exp.Expression {
	return exp.Subquery(exp.Args{
		"this": exp.Select(exp.Args{
			"expressions": []exp.Expression{exp.Column_("x", nil, nil, nil, nil)},
			"from_":       exp.From(exp.Args{"this": dmlTable(table)}),
		}),
		"alias": exp.TableAlias(exp.Args{"this": exp.ToIdentifier(alias)}),
	})
}

func parseDMLDialect(t *testing.T, sql, dialect string) exp.Expression {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q): %v", sql, dialect, err)
	}
	return expression
}

func dmlSourceSubquery(t *testing.T, expression exp.Expression, alias string) exp.Expression {
	t.Helper()
	for _, subquery := range expression.FindAll(exp.KindSubquery) {
		if subquery.Alias() == alias {
			return subquery
		}
	}
	t.Fatalf("%s source Subquery %q not found", exp.ClassName(expression.Kind()), alias)
	return nil
}

func qualifyDMLSQL(t *testing.T, sql, dialect string, mapping *schema.Mapping, isolateTables bool) string {
	t.Helper()
	expression := parseDMLDialect(t, sql, dialect)
	opts := DefaultQualifyOpts()
	opts.Dialect = dialect
	opts.Schema = mapping
	opts.IsolateTables = isolateTables
	// Keep validation and every other default enabled. Only quoting is disabled so exact
	// compatibility strings remain readable and match pinned upstream's unquoted output.
	opts.QuoteIdentifiers = false

	var result exp.Expression
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Qualify(%q, dialect=%q, isolateTables=%v) panicked: %v", sql, dialect, isolateTables, recovered)
			}
		}()
		result = Qualify(expression, opts)
	}()

	got, err := sqlglot.Generate(result, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate(Qualify(%q), %q): %v", sql, dialect, err)
	}
	return got
}

func assertDMLTraversalSurfaces(
	t *testing.T,
	sql string,
	dialect string,
	kind exp.Kind,
	targetName string,
	sourceAlias string,
	optimizerTraversesInner bool,
) {
	t.Helper()
	expression := parseDMLDialect(t, sql, dialect)
	sourceSubquery := dmlSourceSubquery(t, expression, sourceAlias)
	inner := sourceSubquery.This()

	analysisScopes := traverseScope(expression)
	root := finalDMLRoot(t, analysisScopes, kind)
	assertDMLSourceKeys(t, root, targetName, sourceAlias)
	assertDMLTableSource(t, root, targetName)
	boundSource := assertDMLScopeSource(t, root, sourceAlias)
	if boundSource.Expression != inner {
		got := "<nil>"
		if boundSource.Expression != nil {
			got = exp.ClassName(boundSource.Expression.Kind())
		}
		t.Fatalf("analysis source %q expression = %s, want source inner query", sourceAlias, got)
	}
	if boundSource.Parent != root || !boundSource.IsDerivedTable() {
		t.Fatalf("analysis source %q is not a derived-table child of the complete %s root", sourceAlias, exp.ClassName(kind))
	}

	optimizerScopes := traverseScopeForOptimizer(expression)
	for _, scope := range optimizerScopes {
		if scope.Expression == expression || (scope.Expression != nil && scope.Expression.Kind() == kind) {
			t.Fatalf("optimizer traversal emitted the %s root", exp.ClassName(kind))
		}
	}

	if optimizerTraversesInner {
		if len(optimizerScopes) != 2 {
			t.Fatalf("optimizer scope count = %d, want inner Select plus source Subquery", len(optimizerScopes))
		}
		if optimizerScopes[0].Expression != inner || optimizerScopes[0].ScopeType != ScopeTypeSubquery {
			t.Fatalf("optimizer scope[0] is not the upstream-discovered source inner Select")
		}
		if !optimizerScopes[0].canBeCorrelated {
			t.Fatalf("optimizer source inner Select must remain correlatable")
		}
		if optimizerScopes[1].Expression != sourceSubquery || !optimizerScopes[1].IsRoot() {
			t.Fatalf("optimizer scope[1] is not the root source Subquery")
		}
		return
	}

	if len(optimizerScopes) != 1 {
		t.Fatalf("optimizer scope count = %d, want only the source Subquery wrapper", len(optimizerScopes))
	}
	if optimizerScopes[0].Expression != sourceSubquery || !optimizerScopes[0].IsRoot() {
		t.Fatalf("optimizer traversal newly exposed the UPDATE source inner Select")
	}
}

func assertFreshPublicDMLScopes(t *testing.T, sql, dialect string, kind exp.Kind, targetName, sourceAlias string) {
	t.Helper()
	expression := parseDMLDialect(t, sql, dialect)

	root := finalDMLRoot(t, TraverseScope(expression), kind)
	assertDMLSourceKeys(t, root, targetName, sourceAlias)
	assertDMLTableSource(t, root, targetName)
	assertDMLScopeSource(t, root, sourceAlias)

	built := BuildScope(expression)
	if built == nil || built.Expression != expression || built.Expression.Kind() != kind || !built.IsRoot() {
		t.Fatalf("BuildScope did not return the complete %s root", exp.ClassName(kind))
	}
	assertDMLSourceKeys(t, built, targetName, sourceAlias)
	assertDMLTableSource(t, built, targetName)
	assertDMLScopeSource(t, built, sourceAlias)
}

func TestDMLScopeUpdateSourcesColumnsAndResolution(t *testing.T) {
	expression := parseOne(t, "UPDATE t SET x=1 FROM u WHERE t.id=u.id")
	scope := finalDMLRoot(t, traverseScope(expression), exp.KindUpdate)
	assertDMLSourceKeys(t, scope, "t", "u")
	assertDMLTableSource(t, scope, "t")
	assertDMLTableSource(t, scope, "u")
	assertStringSet(t, columnSQLSet(t, scope.Columns()), "x", "t.id", "u.id")
	assertStringSet(t, columnSQLSet(t, scope.SourceColumns("t")), "t.id")
	assertStringSet(t, columnSQLSet(t, scope.SourceColumns("u")), "u.id")
	assertQualifiedColumnsBind(t, scope)

	mapping, err := schema.NewMappingSchema(schema.M(
		"t", schema.M("id", "INT", "x", "INT"),
		"u", schema.M("id", "INT"),
	), "", true)
	if err != nil {
		t.Fatalf("NewMappingSchema: %v", err)
	}
	resolved := NewResolver(scope, mapping, false).GetTable("x")
	if resolved == nil || resolved.Name() != "t" {
		got := "<nil>"
		if resolved != nil {
			got = resolved.Name()
		}
		t.Fatalf("resolver table for x = %q, want t", got)
	}

	built := BuildScope(expression)
	if built == nil || built.Expression != expression {
		t.Fatalf("BuildScope did not return the UPDATE root")
	}
	assertDMLSourceKeys(t, built, "t", "u")
}

func TestDMLScopeParserNestedJoinSources(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		sql     string
		kind    exp.Kind
		want    []string
	}{
		{
			name: "update from join nested on source table",
			sql:  "UPDATE t SET x=1 FROM u JOIN v ON u.id=v.id WHERE t.id=u.id",
			kind: exp.KindUpdate,
			want: []string{"t", "u", "v"},
		},
		{
			name:    "mysql target joins nested on target table",
			dialect: "mysql",
			sql:     "UPDATE t JOIN u ON t.id=u.id JOIN v ON u.id=v.id SET t.x=v.x",
			kind:    exp.KindUpdate,
			want:    []string{"t", "u", "v"},
		},
		{
			name: "delete using join nested on using table",
			sql:  "DELETE FROM t USING u JOIN v ON u.id=v.id WHERE t.id=u.id",
			kind: exp.KindDelete,
			want: []string{"t", "u", "v"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expression, err := sqlglot.ParseOne(tc.sql, tc.dialect)
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			scope := finalDMLRoot(t, traverseScope(expression), tc.kind)
			assertDMLSourceKeys(t, scope, tc.want...)
			for _, name := range tc.want {
				assertDMLTableSource(t, scope, name)
			}
			assertQualifiedColumnsBind(t, scope)
		})
	}
}

func TestDMLScopeDeleteAndMerge(t *testing.T) {
	deleteExpr := parseOne(t, "DELETE FROM t USING u WHERE t.id=u.id")
	deleteScope := finalDMLRoot(t, traverseScope(deleteExpr), exp.KindDelete)
	assertDMLSourceKeys(t, deleteScope, "t", "u")
	assertDMLTableSource(t, deleteScope, "t")
	assertDMLTableSource(t, deleteScope, "u")
	assertQualifiedColumnsBind(t, deleteScope)

	mergeExpr := parseOne(t, "MERGE INTO t USING u ON t.id=u.id WHEN MATCHED THEN UPDATE SET a=u.a")
	mergeScope := finalDMLRoot(t, traverseScope(mergeExpr), exp.KindMerge)
	assertDMLSourceKeys(t, mergeScope, "t", "u")
	assertDMLTableSource(t, mergeScope, "t")
	assertDMLTableSource(t, mergeScope, "u")
	assertStringSet(t, columnSQLSet(t, mergeScope.Columns()), "t.id", "u.id", "a", "u.a")
	assertQualifiedColumnsBind(t, mergeScope)
}

func TestDMLScopeCTEShadowing(t *testing.T) {
	expression := parseOne(t, "WITH c AS (SELECT 1 AS id) UPDATE t SET x=1 FROM c WHERE t.id=c.id")
	scopes := traverseScope(expression)
	root := finalDMLRoot(t, scopes, exp.KindUpdate)
	assertDMLSourceKeys(t, root, "c", "t")

	cteScope := assertDMLScopeSource(t, root, "c")
	cteSource, ok := root.CTESources["c"].(*Scope)
	if !ok || cteSource == nil {
		t.Fatalf("CTESources[c] = %T, want *Scope", root.CTESources["c"])
	}
	if cteScope != cteSource {
		t.Fatalf("Sources[c] and CTESources[c] do not reference the same scope")
	}
	if _, ok := root.Sources["c"].(exp.Expression); ok {
		t.Fatalf("Sources[c] is a physical expression, want the CTE scope")
	}
	target := assertDMLTableSource(t, root, "t")
	if target != expression.This() {
		t.Fatalf("target source is not the original UPDATE target")
	}
	assertQualifiedColumnsBind(t, root)

	sameName := parseOne(t, "WITH t AS (SELECT 1 AS id) UPDATE t SET id=1")
	sameNameRoot := finalDMLRoot(t, traverseScope(sameName), exp.KindUpdate)
	assertDMLSourceKeys(t, sameNameRoot, "t", "t_2")
	if sameNameRoot.Sources["t"] != sameNameRoot.CTESources["t"] {
		t.Fatalf("same-named CTE binding was not preserved")
	}
	if physical := assertDMLTableSource(t, sameNameRoot, "t_2"); physical != sameName.This() {
		t.Fatalf("same-named CTE hid the physical UPDATE target")
	}
}

func TestDMLScopeDerivedSourceOrdering(t *testing.T) {
	expression := parseOne(t, "UPDATE t SET x=s.x FROM (SELECT x FROM u) AS s WHERE t.id=s.x")
	scopes := traverseScope(expression)
	root := finalDMLRoot(t, scopes, exp.KindUpdate)
	assertDMLSourceKeys(t, root, "t", "s")
	assertDMLTableSource(t, root, "t")
	derived := assertDMLScopeSource(t, root, "s")
	assertDMLSourceKeys(t, derived, "u")
	if derived.Parent != root || !derived.IsDerivedTable() {
		t.Fatalf("derived source scope is not attached to the DML root")
	}
	if len(root.DerivedTableScopes) != 1 || root.DerivedTableScopes[0] != derived {
		t.Fatalf("DML root DerivedTableScopes does not contain the source scope exactly once")
	}
	if len(root.TableScopes) != 1 || root.TableScopes[0] != derived {
		t.Fatalf("DML root TableScopes does not contain the source scope exactly once")
	}

	innerCount := 0
	for _, scope := range scopes[:len(scopes)-1] {
		if source, ok := scope.Sources["u"]; ok {
			if table, ok := source.(exp.Expression); ok && table != nil && table.Kind() == exp.KindTable {
				innerCount++
			}
		}
	}
	if innerCount != 1 {
		t.Fatalf("inner u scope count = %d, want 1", innerCount)
	}
	if scopes[len(scopes)-1] != root {
		t.Fatalf("DML root is not last")
	}
	builtRoot := BuildScope(expression)
	if builtRoot == nil {
		t.Fatalf("BuildScope returned nil for valid UPDATE")
	}
	builtScopes := builtRoot.Traverse()
	if len(builtScopes) != len(scopes) {
		t.Fatalf("BuildScope traversal count = %d, want %d", len(builtScopes), len(scopes))
	}
	for i := range scopes {
		if builtScopes[i].Expression != scopes[i].Expression || builtScopes[i].ScopeType != scopes[i].ScopeType {
			t.Fatalf("BuildScope traversal scope[%d] does not match traverseScope", i)
		}
	}
	assertQualifiedColumnsBind(t, root)
}

func TestDMLScopeSubqueryTargetRemainsPhysical(t *testing.T) {
	target := dmlSubquery("t", "physical_target")
	expression := exp.Update(exp.Args{"this": target})
	scopes := traverseScope(expression)
	root := finalDMLRoot(t, scopes, exp.KindUpdate)
	assertDMLSourceKeys(t, root, "t")

	physical, ok := root.Sources["t"].(exp.Expression)
	if !ok || physical != target {
		t.Fatalf("target source = %T, want the original physical Subquery", root.Sources["t"])
	}
	if len(scopes) < 2 {
		t.Fatalf("target Subquery child scopes were not retained")
	}
}

func TestDMLScopeDefensiveASTSourceShapes(t *testing.T) {
	join := func(source exp.Expression) exp.Expression {
		return exp.Join(exp.Args{"this": source})
	}
	withNestedJoins := func() exp.Expression {
		u := dmlTable("u")
		v := dmlTable("v")
		v.Set("joins", []exp.Expression{join(dmlTable("w"))})
		u.Set("joins", []exp.Expression{join(v)})
		return u
	}
	usingSources := func(representation string) any {
		u := dmlTable("u")
		u.Set("joins", []exp.Expression{join(dmlTable("v"))})
		switch representation {
		case "scalar":
			return u
		case "expressions":
			return []exp.Expression{u}
		case "any":
			return []any{u}
		default:
			panic("unknown representation")
		}
	}

	type sourceShapeCase struct {
		name  string
		build func() exp.Expression
		kind  exp.Kind
		want  []string
		scope []string
		count int
	}
	tests := []sourceShapeCase{
		{
			name: "schema wrapped target",
			build: func() exp.Expression {
				return exp.Update(exp.Args{
					"this": exp.Schema(exp.Args{
						"this":        dmlTable("t"),
						"expressions": []exp.Expression{exp.ToIdentifier("x")},
					}),
					"from_": exp.From(exp.Args{"this": dmlTable("u")}),
				})
			},
			kind: exp.KindUpdate,
			want: []string{"t", "u"},
		},
		{
			name: "from this and raw expressions",
			build: func() exp.Expression {
				return exp.Update(exp.Args{
					"this": dmlTable("t"),
					"from_": exp.From(exp.Args{
						"this":        dmlTable("u"),
						"expressions": []exp.Expression{dmlTable("v"), dmlSubquery("s", "inner")},
					}),
				})
			},
			kind:  exp.KindUpdate,
			want:  []string{"t", "u", "v", "s"},
			scope: []string{"s"},
		},
		{
			name: "root level joins",
			build: func() exp.Expression {
				return exp.Update(exp.Args{
					"this":  dmlTable("t"),
					"joins": []exp.Expression{join(dmlTable("u")), join(dmlSubquery("s", "inner"))},
				})
			},
			kind:  exp.KindUpdate,
			want:  []string{"t", "u", "s"},
			scope: []string{"s"},
		},
		{
			name: "recursively nested table joins",
			build: func() exp.Expression {
				return exp.Update(exp.Args{
					"this":  dmlTable("t"),
					"from_": exp.From(exp.Args{"this": withNestedJoins()}),
				})
			},
			kind: exp.KindUpdate,
			want: []string{"t", "u", "v", "w"},
		},
		{
			name: "same source subquery identity is processed once",
			build: func() exp.Expression {
				source := dmlSubquery("s", "inner")
				return exp.Delete(exp.Args{
					"this":  dmlTable("t"),
					"using": []exp.Expression{source, source},
				})
			},
			kind:  exp.KindDelete,
			want:  []string{"t", "s"},
			scope: []string{"s"},
			count: 3,
		},
	}

	for _, dmlKind := range []struct {
		name string
		kind exp.Kind
	}{
		{name: "delete", kind: exp.KindDelete},
		{name: "merge", kind: exp.KindMerge},
	} {
		for _, representation := range []string{"scalar", "expressions", "any"} {
			dmlKind := dmlKind
			representation := representation
			tests = append(tests, sourceShapeCase{
				name: fmt.Sprintf("%s using %s", dmlKind.name, representation),
				build: func() exp.Expression {
					args := exp.Args{"this": dmlTable("t"), "using": usingSources(representation)}
					if dmlKind.kind == exp.KindMerge {
						args["whens"] = exp.Whens(exp.Args{"expressions": []exp.Expression{}})
						return exp.Merge(args)
					}
					return exp.Delete(args)
				},
				kind: dmlKind.kind,
				want: []string{"t", "u", "v"},
			})
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expression := tc.build()
			scopes := traverseScope(expression)
			scope := finalDMLRoot(t, scopes, tc.kind)
			if tc.count > 0 && len(scopes) != tc.count {
				t.Fatalf("scope count = %d, want %d", len(scopes), tc.count)
			}
			assertDMLSourceKeys(t, scope, tc.want...)
			scopeSources := map[string]bool{}
			for _, name := range tc.scope {
				scopeSources[name] = true
			}
			for _, name := range tc.want {
				if scopeSources[name] {
					assertDMLScopeSource(t, scope, name)
				} else {
					assertDMLTableSource(t, scope, name)
				}
			}
		})
	}
}

func TestDMLScopeMalformedSourcesFailClosed(t *testing.T) {
	values := func() exp.Expression {
		return exp.Values(exp.Args{"expressions": []exp.Expression{exp.Tuple(exp.Args{"expressions": []exp.Expression{exp.LiteralNumber(1)}})}})
	}
	join := func(source exp.Expression) exp.Expression {
		return exp.Join(exp.Args{"this": source})
	}

	malformedWithChild := parseOne(t, "UPDATE t SET x=(SELECT y FROM child)")
	malformedWithChild.Set("from_", exp.From(exp.Args{"this": values()}))

	tests := []struct {
		name      string
		root      exp.Expression
		wantChild bool
	}{
		{
			name: "unsupported schema wrapped target",
			root: exp.Update(exp.Args{
				"this": exp.Schema(exp.Args{"this": values()}),
			}),
		},
		{
			name: "unsupported from source",
			root: exp.Update(exp.Args{
				"this":  dmlTable("t"),
				"from_": exp.From(exp.Args{"this": values()}),
			}),
		},
		{
			name: "unsupported delete using source",
			root: exp.Delete(exp.Args{
				"this":  dmlTable("t"),
				"using": []exp.Expression{values()},
			}),
		},
		{
			name: "unsupported merge using source",
			root: exp.Merge(exp.Args{
				"this":  dmlTable("t"),
				"using": values(),
				"whens": exp.Whens(exp.Args{"expressions": []exp.Expression{}}),
			}),
		},
		{
			name: "invalid any member",
			root: exp.Delete(exp.Args{
				"this":  dmlTable("t"),
				"using": []any{dmlTable("u"), "not an expression"},
			}),
		},
		{
			name: "join missing this",
			root: exp.Update(exp.Args{
				"this":  dmlTable("t"),
				"joins": []exp.Expression{exp.Join(exp.Args{})},
			}),
		},
		{
			name: "join with unsupported this",
			root: exp.Update(exp.Args{
				"this": dmlTable("t"),
				"from_": exp.From(exp.Args{"this": func() exp.Expression {
					u := dmlTable("u")
					u.Set("joins", []exp.Expression{join(values())})
					return u
				}()}),
			}),
		},
		{
			name:      "child scope retained after malformed root",
			root:      malformedWithChild,
			wantChild: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldWarning := logWarning
			warnings := []string{}
			logWarning = func(format string, args ...any) {
				warnings = append(warnings, fmt.Sprintf(format, args...))
			}
			defer func() { logWarning = oldWarning }()

			var scopes []*Scope
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("traverseScope panicked: %v", recovered)
					}
				}()
				scopes = traverseScope(tc.root)
			}()

			if len(warnings) == 0 {
				t.Fatalf("expected malformed DML warning")
			}
			for _, scope := range scopes {
				if scope.Expression == tc.root {
					t.Fatalf("malformed %s root scope was emitted", exp.ClassName(tc.root.Kind()))
				}
			}
			if tc.wantChild {
				found := false
				for _, scope := range scopes {
					if scope.Expression != nil && scope.Expression.Kind() == exp.KindSelect {
						if source, ok := scope.Sources["child"].(exp.Expression); ok && source != nil && source.Kind() == exp.KindTable {
							found = true
						}
					}
				}
				if !found {
					t.Fatalf("nested child scope was not retained; scopes = %d", len(scopes))
				}
			}
		})
	}
}

func TestDMLScopeFailedSubqueryBindingIsTraversedOnce(t *testing.T) {
	source := exp.Subquery(exp.Args{
		"this":  exp.Values(exp.Args{"expressions": []exp.Expression{exp.Tuple(exp.Args{"expressions": []exp.Expression{exp.LiteralNumber(1)}})}}),
		"alias": exp.TableAlias(exp.Args{"this": exp.ToIdentifier("s")}),
	})
	root := exp.Update(exp.Args{
		"this":  dmlTable("t"),
		"from_": exp.From(exp.Args{"this": source}),
	})

	oldWarning := logWarning
	warnings := []string{}
	logWarning = func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}
	defer func() { logWarning = oldWarning }()

	scopes := traverseScope(root)
	if len(warnings) == 0 {
		t.Fatalf("expected failed Subquery binding warning")
	}
	wrapperCount := 0
	for _, scope := range scopes {
		if scope.Expression == root {
			t.Fatalf("root with failed Subquery binding was emitted")
		}
		if scope.Expression == source {
			wrapperCount++
		}
	}
	if wrapperCount != 1 {
		t.Fatalf("failed source Subquery wrapper count = %d, want 1", wrapperCount)
	}
}

func TestDMLScopeOptimizerContainment(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		kind exp.Kind
	}{
		{
			name: "with update",
			sql:  "WITH c AS (SELECT id FROM u) UPDATE t SET x=c.id FROM c WHERE t.id=c.id",
			kind: exp.KindUpdate,
		},
		{
			name: "with delete",
			sql:  "WITH c AS (SELECT id FROM u) DELETE FROM t USING c WHERE t.id=c.id",
			kind: exp.KindDelete,
		},
	} {
		t.Run(tc.name+" qualify tables", func(t *testing.T) {
			expression := parseOne(t, tc.sql)
			target := expression.This()
			QualifyTables(expression, nil, nil, "", false, nil, nil, nil)
			if expression.Kind() != tc.kind || expression.This() != target || target.Kind() != exp.KindTable {
				t.Fatalf("QualifyTables rewrote the DML target shape")
			}
			if target.Alias() != "" {
				t.Fatalf("DML target alias = %q, want empty", target.Alias())
			}
			var nested exp.Expression
			for _, table := range expression.FindAll(exp.KindTable) {
				if table.Name() == "u" {
					nested = table
					break
				}
			}
			if nested == nil || nested.Alias() != "u" {
				t.Fatalf("nested SELECT table was not qualified with its existing alias behavior")
			}
		})
	}

	columnsExpression := parseOne(t, "WITH c AS (SELECT id FROM u) UPDATE t SET x=c.id FROM c WHERE t.id=c.id")
	columnsSchema := schema.M(
		"t", schema.M("id", "INT", "x", "INT"),
		"u", schema.M("id", "INT"),
	)
	QualifyColumns(columnsExpression, columnsSchema, true, false, nil, false, nil)
	assignment := columnsExpression.Expressions()[0]
	if assignment.This().Text("table") != "" {
		t.Fatalf("QualifyColumns processed the DML root assignment")
	}
	var nestedID exp.Expression
	for _, selectExpr := range columnsExpression.FindAll(exp.KindSelect) {
		for _, column := range selectExpr.FindAll(exp.KindColumn) {
			if column.Name() == "id" && column.Text("table") == "u" {
				nestedID = column
				break
			}
		}
	}
	if nestedID == nil {
		t.Fatalf("QualifyColumns did not process the nested SELECT scope")
	}

	expression := parseOne(t, "WITH c AS (SELECT u.id FROM u JOIN v ON u.id=v.id) UPDATE t SET x=c.id FROM c WHERE t.id=c.id")
	target := expression.This()
	mapping := schema.M(
		"t", schema.M("id", "INT", "x", "INT"),
		"u", schema.M("id", "INT"),
		"v", schema.M("id", "INT"),
	)
	opts := DefaultQualifyOpts()
	opts.Schema = mapping
	opts.IsolateTables = true
	opts.QualifyColumns = false
	opts.QuoteIdentifiers = false
	opts.ValidateQualifyColumns = false
	result := Qualify(expression, opts)
	if result != expression || result.Kind() != exp.KindUpdate || result.This() != target || target.Kind() != exp.KindTable {
		t.Fatalf("Qualify rewrote the UPDATE root or target")
	}
	if target.Alias() != "" {
		t.Fatalf("Qualify introduced target alias %q", target.Alias())
	}
	for _, name := range []string{"u", "v"} {
		found := false
		for _, table := range result.FindAll(exp.KindTable) {
			if table.Name() == name && table.Alias() == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("nested SELECT source %q was not qualified", name)
		}
	}
}

func TestDMLScopeOptimizerUpdateFromParity(t *testing.T) {
	const sql = "UPDATE t SET x = s.x FROM (SELECT x, id FROM u) AS s WHERE t.id = s.id"
	mapping := schema.M(
		"t", schema.M("id", "INT", "x", "INT"),
		"u", schema.M("id", "INT", "x", "INT"),
	)

	assertDMLTraversalSurfaces(t, sql, "", exp.KindUpdate, "t", "s", false)
	if got := qualifyDMLSQL(t, sql, "", mapping, false); got != sql {
		t.Fatalf("Qualify() = %q, want pinned upstream %q", got, sql)
	}
	assertFreshPublicDMLScopes(t, sql, "", exp.KindUpdate, "t", "s")
}

func TestDMLScopeOptimizerMySQLTargetJoinParity(t *testing.T) {
	const sql = "UPDATE t JOIN (SELECT id, x FROM u) AS s ON t.id=s.id SET t.x=s.x"
	const want = "UPDATE t JOIN (SELECT id, x FROM u) AS s ON t.id = s.id SET t.x = s.x"
	mapping := schema.M(
		"t", schema.M("id", "INT", "x", "INT"),
		"u", schema.M("id", "INT", "x", "INT"),
	)

	if got := qualifyDMLSQL(t, sql, "mysql", mapping, false); got != want {
		t.Fatalf("Qualify() = %q, want pinned upstream %q", got, want)
	}
	assertFreshPublicDMLScopes(t, sql, "mysql", exp.KindUpdate, "t", "s")
}

func TestDMLScopeOptimizerCorrelatedDeleteParity(t *testing.T) {
	const sql = "DELETE FROM t USING (SELECT id FROM u WHERE u.a = t.id) AS s WHERE t.id = s.id"
	const want = "DELETE FROM t USING (SELECT u.id AS id FROM u AS u WHERE u.a = t.id) AS s WHERE t.id = s.id"
	mapping := schema.M(
		"t", schema.M("id", "INT"),
		"u", schema.M("id", "INT", "a", "INT"),
	)

	assertDMLTraversalSurfaces(t, sql, "", exp.KindDelete, "t", "s", true)
	if got := qualifyDMLSQL(t, sql, "", mapping, false); got != want {
		t.Fatalf("Qualify() = %q, want pinned upstream %q", got, want)
	}
	assertFreshPublicDMLScopes(t, sql, "", exp.KindDelete, "t", "s")
}

func TestDMLScopeOptimizerMergeSourceParity(t *testing.T) {
	const sql = "MERGE INTO t USING (SELECT id, x FROM u) AS s ON t.id=s.id WHEN MATCHED THEN UPDATE SET x=s.x"
	const want = "MERGE INTO t USING (SELECT u.id AS id, u.x AS x FROM u AS u) AS s ON t.id = s.id WHEN MATCHED THEN UPDATE SET x = s.x"
	mapping := schema.M(
		"t", schema.M("id", "INT", "x", "INT"),
		"u", schema.M("id", "INT", "x", "INT"),
	)

	assertDMLTraversalSurfaces(t, sql, "", exp.KindMerge, "t", "s", true)
	if got := qualifyDMLSQL(t, sql, "", mapping, false); got != want {
		t.Fatalf("Qualify() = %q, want pinned upstream %q", got, want)
	}
	assertFreshPublicDMLScopes(t, sql, "", exp.KindMerge, "t", "s")
}

func TestDMLScopeOptimizerIsolateTablesParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		want string
		kind exp.Kind
	}{
		{
			name: "update source is not newly exposed",
			sql:  "UPDATE t SET x = s.x FROM (SELECT x, id FROM u) AS s WHERE t.id = s.id",
			want: "UPDATE t SET x = s.x FROM (SELECT x, id FROM u) AS s WHERE t.id = s.id",
			kind: exp.KindUpdate,
		},
		{
			name: "merge source retains upstream isolation traversal",
			sql:  "MERGE INTO t USING (SELECT id, x FROM u) AS s ON t.id=s.id WHEN MATCHED THEN UPDATE SET x=s.x",
			want: "MERGE INTO t USING (SELECT u.id AS id, u.x AS x FROM u AS u) AS s ON t.id = s.id WHEN MATCHED THEN UPDATE SET x = s.x",
			kind: exp.KindMerge,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapping := schema.M(
				"t", schema.M("id", "INT", "x", "INT"),
				"u", schema.M("id", "INT", "x", "INT"),
			)
			if got := qualifyDMLSQL(t, tc.sql, "", mapping, true); got != tc.want {
				t.Fatalf("Qualify(IsolateTables=true) = %q, want pinned upstream %q", got, tc.want)
			}
			assertFreshPublicDMLScopes(t, tc.sql, "", tc.kind, "t", "s")
		})
	}
}
