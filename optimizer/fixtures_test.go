package optimizer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/schema"
)

type sqlFixturePair struct {
	Meta     map[string]string
	SQL      string
	Expected string
}

func filterComments(s string) string {
	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		if line != "" && !strings.HasPrefix(line, "--") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func extractMeta(sql string) (string, map[string]string) {
	meta := map[string]string{}
	lines := strings.Split(sql, "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(lines[i], "#") {
		parts := strings.SplitN(lines[i], ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(strings.TrimPrefix(parts[0], "#"))
			meta[key] = strings.TrimSpace(parts[1])
		}
		i++
	}
	return strings.TrimSpace(strings.Join(lines[i:], "\n")), meta
}

func loadSQLFixturePairs(t *testing.T, filename string) []sqlFixturePair {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("read fixture %s: %v", filename, err)
	}
	statements := strings.Split(filterComments(string(data)), ";")
	pairs := []sqlFixturePair{}
	for i := 0; i+1 < len(statements); i += 2 {
		sql, meta := extractMeta(strings.TrimSpace(statements[i]))
		pairs = append(pairs, sqlFixturePair{
			Meta:     meta,
			SQL:      sql,
			Expected: strings.TrimSpace(statements[i+1]),
		})
	}
	return pairs
}

func loadSQLFixtures(t *testing.T, filename string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("read fixture %s: %v", filename, err)
	}
	statements := strings.Split(filterComments(string(data)), "\n")
	out := []string{}
	for _, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement != "" {
			out = append(out, statement)
		}
	}
	return out
}

func assertASTInvariants(t *testing.T, root exp.Expression) {
	t.Helper()
	for _, node := range root.Walk() {
		for _, key := range exp.ArgKeys(node.Kind()) {
			switch v := node.Arg(key).(type) {
			case exp.Expression:
				if v != nil && (v.ArgKey() != key || v.Parent() != node) {
					t.Fatalf("AST invariant: %v.%s", node.Kind(), key)
				}
			case []exp.Expression:
				for _, child := range v {
					if child != nil && (child.ArgKey() != key || child.Parent() != node) {
						t.Fatalf("AST invariant: %v.%s", node.Kind(), key)
					}
				}
			}
		}
	}
}

func boolPtr(value bool) *bool { return &value }

func stringToBool(value string) bool {
	switch strings.ToLower(value) {
	case "true", "1":
		return true
	default:
		return false
	}
}

func dialectInScope(meta map[string]string) bool {
	switch meta["dialect"] {
	case "", "mysql", "postgres":
		return true
	default:
		return false
	}
}

func optimizerTestSchema() *schema.Mapping {
	return schema.M(
		"x", schema.M("a", "INT", "b", "INT"),
		"y", schema.M("b", "INT", "c", "INT"),
		"z", schema.M("b", "INT", "c", "INT"),
		"w", schema.M("d", "TEXT", "e", "TEXT"),
		"temporal", schema.M("d", "DATE", "t", "DATETIME"),
		"structs", schema.M(
			"one", "STRUCT<a_1 INT, b_1 VARCHAR>",
			"nested_0", "STRUCT<a_1 INT, nested_1 STRUCT<a_2 INT, nested_2 STRUCT<a_3 INT>>>",
			"quoted", "STRUCT<\"foo bar\" INT>",
		),
		"t_bool", schema.M("a", "BOOLEAN"),
	)
}
