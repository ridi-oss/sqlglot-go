package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestKnownStatementRoots(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"INSERT INTO t VALUES (1)", exp.KindInsert},
		{"UPDATE t SET a = 1", exp.KindUpdate},
		{"DELETE FROM t", exp.KindDelete},
		{"MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN DELETE", exp.KindMerge},
		{"CREATE TABLE t (a INT)", exp.KindCreate},
	}
	for _, tc := range cases {
		expression := parseOne(t, tc.sql)
		if expression.Kind() != tc.kind {
			t.Fatalf("%s kind = %v, want %v:\n%s", tc.sql, expression.Kind(), tc.kind, expression.ToS())
		}
	}
}
