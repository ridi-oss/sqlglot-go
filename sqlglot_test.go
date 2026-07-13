package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
)

func TestTranspileEmpty(t *testing.T) {
	got, err := sqlglot.Transpile("", "", "", generator.Options{})
	if err != nil {
		t.Fatalf("Transpile empty error: %v", err)
	}
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("Transpile empty = %#v, want []string{\"\"}", got)
	}
}

// The identity.sql round-trip (formerly TestIdentity) now lives in TestCorpus
// (corpus_test.go), which covers both the base-dialect corpus (Scope A,
// identity.sql) and the per-dialect validate_identity corpus (Scope B,
// dialect_identity.jsonl) through one parse->generate->compare core, so
// identity.sql is exercised exactly once.
