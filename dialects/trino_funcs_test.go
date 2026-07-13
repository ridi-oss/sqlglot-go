package dialects_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestTrinoFunctionsAreExactPrestoSuperset(t *testing.T) {
	presto := dialects.Presto()
	trino := dialects.Trino()

	if len(trino.Functions) != len(presto.Functions)+2 {
		t.Fatalf("Trino function count = %d, want Presto %d + 2", len(trino.Functions), len(presto.Functions))
	}
	for name := range presto.Functions {
		if trino.Functions[name] == nil {
			t.Errorf("Trino lost inherited Presto function %q", name)
		}
	}
	for _, name := range []string{"VERSION", "ARRAY_FIRST"} {
		if trino.Functions[name] == nil {
			t.Errorf("Trino function %q = nil, want builder", name)
		}
	}
}

func TestTrinoFunctionKinds(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"SELECT VERSION()", exp.KindCurrentVersion},
		{"SELECT ARRAY_FIRST(a)", exp.KindArrayFirst},
	}
	for _, tc := range cases {
		expression, err := sqlglot.ParseOne(tc.sql, "trino")
		if err != nil {
			t.Errorf("ParseOne(%q, trino): %v", tc.sql, err)
			continue
		}
		if len(expression.FindAll(tc.kind)) != 1 {
			t.Errorf("Trino %q did not parse to exactly one %v node", tc.sql, tc.kind)
		}
	}
}
