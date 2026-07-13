package optimizer

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
)

func parseOne(t *testing.T, sql string) exp.Expression {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, "")
	if err != nil {
		t.Fatalf("ParseOne(%q): %v", sql, err)
	}
	return expression
}

func sqlOf(t *testing.T, expression exp.Expression) string {
	t.Helper()
	sql, err := sqlglot.Generate(expression, "", generator.Options{})
	if err != nil {
		t.Fatalf("Generate(%s): %v", expression.ToS(), err)
	}
	return sql
}

func sourceNameSet(scope *Scope) map[string]bool {
	out := map[string]bool{}
	for name := range scope.Sources {
		out[name] = true
	}
	return out
}

func assertStringSet(t *testing.T, got map[string]bool, want ...string) {
	t.Helper()
	wantSet := map[string]bool{}
	for _, name := range want {
		wantSet[name] = true
	}
	if len(got) != len(wantSet) {
		t.Fatalf("set = %v, want %v", sortedKeys(got), sortedKeys(wantSet))
	}
	for name := range wantSet {
		if !got[name] {
			t.Fatalf("set = %v, want %v", sortedKeys(got), sortedKeys(wantSet))
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func columnSQLSet(t *testing.T, columns []exp.Expression) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, column := range columns {
		out[sqlOf(t, column)] = true
	}
	return out
}

func columnTableSet(columns []exp.Expression) map[string]bool {
	out := map[string]bool{}
	for _, column := range columns {
		out[column.Text("table")] = true
	}
	return out
}

func TestScope(t *testing.T) {
	ast := parseOne(t, "SELECT IF(a IN UNNEST(b), 1, 0) AS c FROM t")
	columns := buildScope(ast).Columns()
	wantColumns := []exp.Expression{exp.Column_("a", nil, nil, nil, nil), exp.Column_("b", nil, nil, nil, nil)}
	if len(columns) != len(wantColumns) {
		t.Fatalf("columns length = %d, want %d", len(columns), len(wantColumns))
	}
	for i, want := range wantColumns {
		if !columns[i].Equal(want) {
			t.Fatalf("columns[%d] = %s, want %s", i, columns[i].ToS(), want.ToS())
		}
	}

	parts := make([]string, 10000)
	for i := range parts {
		parts[i] = "SELECT x FROM t"
	}
	manyUnions := parseOne(t, strings.Join(parts, " UNION ALL "))
	scopesUsingTraverse := buildScope(manyUnions).Traverse()
	scopesUsingTraverseScope := traverseScope(manyUnions)
	if len(scopesUsingTraverse) != len(scopesUsingTraverseScope) {
		t.Fatalf("traverse len = %d, traverseScope len = %d", len(scopesUsingTraverse), len(scopesUsingTraverseScope))
	}
	for i := range scopesUsingTraverse {
		if scopesUsingTraverse[i].Expression != scopesUsingTraverseScope[i].Expression {
			t.Fatalf("scope expression %d differs", i)
		}
	}

	sql := `
        WITH q AS (
          SELECT x.b FROM x
        ), r AS (
          SELECT y.b FROM y
        ), z as (
          SELECT cola, colb FROM (VALUES(1, 'test')) AS tab(cola, colb)
        )
        SELECT
          r.b,
          s.b
        FROM r
        JOIN (
          SELECT y.c AS b FROM y
        ) s
        ON s.b = r.b
        WHERE s.b > (SELECT MAX(x.a) FROM x WHERE x.b = s.b)
        `
	expression := parseOne(t, sql)
	for label, scopes := range map[string][]*Scope{
		"traverseScope": traverseScope(expression),
		"traverse":      buildScope(expression).Traverse(),
	} {
		t.Run("complex "+label, func(t *testing.T) {
			if len(scopes) != 7 {
				t.Fatalf("len(scopes) = %d, want 7", len(scopes))
			}
			wants := []string{
				"SELECT x.b FROM x",
				"SELECT y.b FROM y",
				"(VALUES (1, 'test')) AS tab(cola, colb)",
				"SELECT cola, colb FROM (VALUES (1, 'test')) AS tab(cola, colb)",
				"SELECT y.c AS b FROM y",
				"SELECT MAX(x.a) FROM x WHERE x.b = s.b",
				sqlOf(t, parseOne(t, sql)),
			}
			for i, want := range wants {
				if got := sqlOf(t, scopes[i].Expression); got != want {
					t.Fatalf("scope[%d].expression = %q, want %q", i, got, want)
				}
			}
			assertStringSet(t, sourceNameSet(scopes[6]), "q", "z", "r", "s")
			if got := len(scopes[6].Columns()); got != 6 {
				t.Fatalf("root columns len = %d, want 6", got)
			}
			assertStringSet(t, columnTableSet(scopes[6].Columns()), "r", "s")
			if got := scopes[6].SourceColumns("q"); len(got) != 0 {
				t.Fatalf("SourceColumns(q) = %v, want empty", got)
			}
			if got := len(scopes[6].SourceColumns("r")); got != 2 {
				t.Fatalf("SourceColumns(r) len = %d, want 2", got)
			}
			assertStringSet(t, columnTableSet(scopes[6].SourceColumns("r")), "r")

			assertStringSet(t, columnSQLSet(t, scopes[len(scopes)-1].FindAll(exp.KindColumn)), "r.b", "s.b")
			if got := sqlOf(t, scopes[len(scopes)-1].Find(exp.KindColumn)); got != "r.b" {
				t.Fatalf("root first column = %q, want r.b", got)
			}
			assertStringSet(t, columnSQLSet(t, scopes[0].FindAll(exp.KindColumn)), "x.b")
		})
	}

	where := expression.Find(exp.KindWhere)
	whereColumns := map[string]bool{}
	for _, node := range walkInScope(where, nil) {
		if node.Kind() == exp.KindColumn {
			whereColumns[sqlOf(t, node)] = true
		}
	}
	assertStringSet(t, whereColumns, "s.b")

	sql = "SELECT * FROM (((SELECT * FROM (t1 JOIN t2) AS t3) JOIN (SELECT * FROM t4)))"
	expression = parseOne(t, sql)
	for label, scopes := range map[string][]*Scope{
		"traverseScope": traverseScope(expression),
		"traverse":      buildScope(expression).Traverse(),
	} {
		t.Run("parens "+label, func(t *testing.T) {
			if len(scopes) != 4 {
				t.Fatalf("len(scopes) = %d, want 4", len(scopes))
			}
			wantSQLs := []string{
				"t1, t2",
				"SELECT * FROM (t1, t2) AS t3",
				"SELECT * FROM t4",
				"SELECT * FROM (((SELECT * FROM (t1, t2) AS t3), (SELECT * FROM t4)))",
			}
			wantSources := [][]string{{"t1", "t2"}, {"t3"}, {"t4"}, {""}}
			for i := range wantSQLs {
				if got := sqlOf(t, scopes[i].Expression); got != wantSQLs[i] {
					t.Fatalf("scope[%d].expression = %q, want %q", i, got, wantSQLs[i])
				}
				assertStringSet(t, sourceNameSet(scopes[i]), wantSources[i]...)
			}
		})
	}

	innerQuery := "SELECT bar FROM baz"
	for _, udtf := range []string{fmt.Sprintf("UNNEST((%s))", innerQuery), fmt.Sprintf("LATERAL (%s)", innerQuery)} {
		sql = fmt.Sprintf("SELECT a FROM foo CROSS JOIN %s", udtf)
		expression = parseOne(t, sql)
		for label, scopes := range map[string][]*Scope{
			"traverseScope": traverseScope(expression),
			"traverse":      buildScope(expression).Traverse(),
		} {
			t.Run("udtf "+udtf+" "+label, func(t *testing.T) {
				if len(scopes) != 3 {
					t.Fatalf("len(scopes) = %d, want 3", len(scopes))
				}
				if got := sqlOf(t, scopes[0].Expression); got != innerQuery {
					t.Fatalf("scope[0].expression = %q, want %q", got, innerQuery)
				}
				assertStringSet(t, sourceNameSet(scopes[0]), "baz")
				if got := sqlOf(t, scopes[1].Expression); got != udtf {
					t.Fatalf("scope[1].expression = %q, want %q", got, udtf)
				}
				assertStringSet(t, sourceNameSet(scopes[1]), "", "foo")
				if got := sqlOf(t, scopes[2].Expression); got != sql {
					t.Fatalf("scope[2].expression = %q, want %q", got, sql)
				}
				assertStringSet(t, sourceNameSet(scopes[2]), "", "foo")
			})
		}
	}

	// Upstream v30.12.0 omits the DML root. R3 contributes exactly one
	// successful target-only root while preserving all existing nested scopes.
	dmlCases := []struct {
		sql  string
		want int
	}{
		{"UPDATE customers SET total_spent = (SELECT 1 FROM t1) WHERE EXISTS (SELECT 1 FROM t2)", 4},
		{"UPDATE tbl1 SET col = 1 WHERE EXISTS (SELECT 1 FROM tbl2 WHERE tbl1.id = tbl2.id)", 2},
		{"UPDATE tbl1 SET col = 0", 1},
	}
	for _, tc := range dmlCases {
		if got := len(traverseScope(parseOne(t, tc.sql))); got != tc.want {
			t.Fatalf("len(traverseScope(%q)) = %d, want %d", tc.sql, got, tc.want)
		}
	}

	sql = "SELECT * FROM t LEFT JOIN UNNEST(a) AS a1 LEFT JOIN UNNEST(a1.a) AS a2"
	scope := buildScope(parseOne(t, sql))
	selected := map[string]bool{}
	for name := range scope.SelectedSources() {
		selected[name] = true
	}
	assertStringSet(t, selected, "t", "a1", "a2")

	correlated := []string{
		"WITH x AS (SELECT 1 AS id) SELECT x.id, (SELECT MAX(x2.id) FROM x AS x2 WHERE x2.id = x.id) AS mx FROM x",
		"WITH x AS (SELECT 1 AS id), y AS (SELECT 2 AS id) SELECT (SELECT y.id FROM y WHERE y.id = x.id) FROM x",
		"WITH x AS (SELECT 1 AS id) SELECT (SELECT x.id FROM (SELECT * FROM x) AS sub) FROM x",
	}
	for _, sql := range correlated {
		scopes := traverseScope(parseOne(t, sql))
		var subqueryScope *Scope
		for _, scope := range scopes {
			if scope.IsSubquery() {
				subqueryScope = scope
				break
			}
		}
		if subqueryScope == nil {
			t.Fatalf("no subquery scope for %q", sql)
		}
		if !subqueryScope.IsCorrelatedSubquery() {
			t.Fatalf("subquery for %q is not correlated", sql)
		}
		if !columnSQLSet(t, subqueryScope.ExternalColumns())["x.id"] {
			t.Fatalf("external columns for %q = %v, want x.id", sql, sortedKeys(columnSQLSet(t, subqueryScope.ExternalColumns())))
		}
	}
}

func TestScopeWarning(t *testing.T) {
	old := logWarning
	called := false
	logWarning = func(format string, args ...any) {
		called = true
		if format != "Cannot traverse scope %s with type '%s'" {
			t.Fatalf("warning format = %q", format)
		}
	}
	defer func() { logWarning = old }()

	scopes := traverseScope(parseOne(t, "WITH q AS (1) SELECT * FROM q"))
	if len(scopes) != 1 {
		t.Fatalf("len(scopes) = %d, want 1", len(scopes))
	}
	if !called {
		t.Fatalf("expected scope warning")
	}
}

// TestScopeLateralUnnest guards the walk_in_scope boundary gate: a UDTF is not a
// Query, so a UDTF nested directly inside another UDTF (Postgres LATERAL UNNEST,
// where Lateral.this is an Unnest) must not be treated as a scope boundary. If it
// is, every column reference inside the UNNEST is dropped from both the UDTF scope
// and the enclosing scope. Verified column-for-column against the reference
// (sqlglot v30.12.0 scope.py).
func TestScopeLateralUnnest(t *testing.T) {
	sortedColSQL := func(columns []exp.Expression) []string {
		out := make([]string, 0, len(columns))
		for _, column := range columns {
			out = append(out, sqlOf(t, column))
		}
		sort.Strings(out)
		return out
	}
	assertEqual := func(label string, got, want []string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}

	cases := []struct {
		sql        string
		wantCols   []string
		source     string
		wantSource []string
	}{
		{
			sql:        "SELECT v FROM t CROSS JOIN LATERAL UNNEST(t.arr) AS f(v)",
			wantCols:   []string{"t.arr", "t.arr", "v"},
			source:     "t",
			wantSource: []string{"t.arr", "t.arr"},
		},
		{
			sql:        "SELECT val FROM o CROSS JOIN LATERAL UNNEST(o.items) AS e(val)",
			wantCols:   []string{"o.items", "o.items", "val"},
			source:     "o",
			wantSource: []string{"o.items", "o.items"},
		},
	}
	for _, tc := range cases {
		expression, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Fatalf("ParseOne(%q): %v", tc.sql, err)
		}
		scope := buildScope(expression)
		assertEqual(fmt.Sprintf("Columns() for %q", tc.sql), sortedColSQL(scope.Columns()), tc.wantCols)
		assertEqual(fmt.Sprintf("SourceColumns(%q) for %q", tc.source, tc.sql), sortedColSQL(scope.SourceColumns(tc.source)), tc.wantSource)
	}
}

func TestRecursiveCTE(t *testing.T) {
	query := parseOne(t, `
            with recursive t(n) AS
            (
              select 1
              union all
              select n + 1
              FROM t
              where n < 3
            ), y AS (
              select n
              FROM t
              union all
              select n + 1
              FROM y
              where n < 2
            )
            select * from y
            `)

	scope := buildScope(query)
	if len(scope.CTEScopes) != 2 {
		t.Fatalf("len(cte scopes) = %d, want 2", len(scope.CTEScopes))
	}
	assertStringSet(t, mapFromAnyKeys(scope.CTEScopes[0].CTESources), "t")
	assertStringSet(t, mapFromAnyKeys(scope.CTEScopes[1].CTESources), "t", "y")
}

func mapFromAnyKeys(m map[string]any) map[string]bool {
	out := map[string]bool{}
	for key := range m {
		out[key] = true
	}
	return out
}
