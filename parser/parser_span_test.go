package parser_test

import (
	"testing"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// Projection source spans (DEVIATIONS.md §6.2): each top-level SELECT-list expression carries
// the rune span of its verbatim source text, set on the expression inside any alias.
func TestProjectionSpans(t *testing.T) {
	cases := []struct {
		sql     string
		dialect string
		want    []string // verbatim source text per projection
	}{
		{"SELECT database(), 1+1, a FROM t", "mysql", []string{"database()", "1+1", "a"}},
		{"SELECT DATABASE( ) AS x, `weird col` FROM t", "mysql", []string{"DATABASE( )", "`weird col`"}},
		{"select current_schema(), upper(name) from users", "postgres", []string{"current_schema()", "upper(name)"}},
		{"SELECT a + /* c */ b FROM t", "", []string{"a + /* c */ b"}},
		{"SELECT '데이터', x FROM t", "mysql", []string{"'데이터'", "x"}},
		{"SELECT (SELECT max(x) FROM u), t.* FROM t", "", []string{"(SELECT max(x) FROM u)", "t.*"}},
	}
	for _, tc := range cases {
		var stmt exp.Expression
		if tc.dialect == "" {
			stmt = parseOne(t, tc.sql)
		} else {
			stmt = parseOneDialect(t, tc.sql, tc.dialect)
		}
		selects := stmt.Selects()
		if len(selects) != len(tc.want) {
			t.Fatalf("%s: %d projections, want %d", tc.sql, len(selects), len(tc.want))
		}
		for i, projection := range selects {
			node := projection
			if node.Kind() == exp.KindAlias {
				node = node.This()
			}
			text, ok := exp.SpanText(tc.sql, node)
			if !ok {
				t.Errorf("%s: projection %d has no span", tc.sql, i)
				continue
			}
			if text != tc.want[i] {
				t.Errorf("%s: projection %d span text = %q, want %q", tc.sql, i, text, tc.want[i])
			}
		}
	}
}

// Every Select node's projections get spans — CTE bodies, derived-table bodies, and scalar
// subqueries recurse through the same parseSelect projection path, not only the outermost list.
func TestProjectionSpansNestedSelects(t *testing.T) {
	sql := "WITH c AS (SELECT database(), 1+1 FROM t) SELECT upper(x), (SELECT count(*) FROM u) FROM (SELECT lower(y) FROM z) d"
	stmt := parseOneDialect(t, sql, "mysql")
	want := map[string]bool{
		"database()": true, "1+1": true, "upper(x)": true,
		"(SELECT count(*) FROM u)": true, "count(*)": true, "lower(y)": true,
	}
	got := map[string]bool{}
	for _, sel := range stmt.FindAll(exp.KindSelect) {
		for _, projection := range sel.Selects() {
			node := projection
			if node.Kind() == exp.KindAlias {
				node = node.This()
			}
			text, ok := exp.SpanText(sql, node)
			if !ok {
				t.Errorf("projection %s has no span", node.ToS())
				continue
			}
			got[text] = true
		}
	}
	for text := range want {
		if !got[text] {
			t.Errorf("missing span text %q; got %v", text, got)
		}
	}
}

// The span points at the expression node inside an Alias wrapper, so it survives a rewrite
// that re-wraps or copies projections (e.g. qualify aliasing unaliased expressions).
func TestProjectionSpanInsideAliasAndCopy(t *testing.T) {
	sql := "SELECT 1+1 AS total FROM t"
	stmt := parseOne(t, sql)
	alias := stmt.Selects()[0]
	if alias.Kind() != exp.KindAlias {
		t.Fatalf("projection kind = %v, want Alias", alias.Kind())
	}
	if _, _, ok := alias.Span(); ok {
		t.Fatal("Alias wrapper itself must not carry the expression span")
	}
	inner := alias.This().Copy()
	if text, ok := exp.SpanText(sql, inner); !ok || text != "1+1" {
		t.Fatalf("copied inner span text = %q,%v; want \"1+1\",true", text, ok)
	}
}
