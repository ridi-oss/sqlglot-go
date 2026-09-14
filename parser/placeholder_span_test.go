package parser_test

import (
	"sort"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// Bind parameters carry source spans (DEVIATIONS.md §6.2) so ordered binding can follow lexical
// order; tree walk order is not lexical across CTEs, joins and WHERE.
func TestPlaceholderSpans(t *testing.T) {
	cases := []struct {
		dialect string
		sql     string
		want    []string // verbatim source text per bind parameter, in lexical order
	}{
		{"athena", "WITH c AS (SELECT * FROM t WHERE a = ?) SELECT * FROM c JOIN u ON u.id = ? WHERE b IN (?, ?) -- ? not a param\n AND s = '?' AND d = ? /* ? */", []string{"?", "?", "?", "?", "?"}},
		{"athena", "SELECT ? AS x, :p FROM t WHERE a = :q AND b = ?", []string{"?", ":p", ":q", "?"}},
		{"mysql", "SELECT @v, ? FROM t WHERE a = ? AND b = :name AND c = @w", []string{"@v", "?", "?", ":name", "@w"}},
		{"postgres", "SELECT $1 FROM t WHERE a = $2 AND b = %(n)s AND c = ? AND d = :x", []string{"$1", "$2", "%(n)s", "?", ":x"}},
		{"postgres", "SELECT %s, $ /* c */ 1 FROM t WHERE a = ':fake' -- :fake\n AND b = :real /* :fake */", []string{"%s", "$ /* c */ 1", ":real"}},
		{"hive", "SELECT ${a:b}, ${c} FROM t WHERE d = ?", []string{"${a:b}", "${c}", "?"}},
		{"", "INSERT INTO t (a, b) VALUES (?, ?)", []string{"?", "?"}},
		{"mysql", "UPDATE t SET a = ? WHERE b = ? LIMIT ?", []string{"?", "?", "?"}},
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne(tc.sql, tc.dialect)
		if err != nil {
			t.Fatalf("%s: %v", tc.sql, err)
		}
		var nodes []exp.Expression
		for _, n := range e.Walk(false) {
			if n.Kind() == exp.KindPlaceholder || n.Kind() == exp.KindParameter {
				nodes = append(nodes, n)
			}
		}
		if len(nodes) != len(tc.want) {
			t.Fatalf("%s: %d bind parameters, want %d:\n%s", tc.sql, len(nodes), len(tc.want), e.ToS())
		}
		type at struct {
			start int
			text  string
		}
		var got []at
		runes := []rune(tc.sql)
		for _, n := range nodes {
			start, end, ok := n.Span()
			text, okText := n.SpanText()
			if !ok || !okText {
				t.Fatalf("%s: %s has no span: %s", tc.sql, exp.ClassName(n.Kind()), n.ToS())
			}
			if string(runes[start:end]) != text {
				t.Fatalf("%s: span [%d,%d) = %q, text %q", tc.sql, start, end, string(runes[start:end]), text)
			}
			got = append(got, at{start, text})
		}
		sort.Slice(got, func(i, j int) bool { return got[i].start < got[j].start })
		for i, g := range got {
			if g.text != tc.want[i] {
				t.Errorf("%s: lexical #%d = %q, want %q", tc.sql, i, g.text, tc.want[i])
			}
			if i > 0 && got[i-1].start >= g.start {
				t.Errorf("%s: spans not strictly increasing", tc.sql)
			}
		}
	}
}
