package sqlglot_test

import (
	"os"
	"strings"
	"testing"

	sqlglot "github.com/sjincho/sqlglot-go"
	"github.com/sjincho/sqlglot-go/generator"
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

func TestIdentity(t *testing.T) {
	data, err := os.ReadFile("testdata/identity.sql")
	if err != nil {
		t.Fatalf("read identity fixture: %v", err)
	}
	pass := 0
	parseDeferred := 0
	genMismatch := 0
	var mismatchSamples []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		t.Run(line, func(t *testing.T) {
			expression, err := sqlglot.ParseOne(line, "")
			if err != nil {
				parseDeferred++
				t.Skipf("parser-deferred: %v", err)
				return
			}
			got, err := sqlglot.Generate(expression, "", generator.Options{})
			if err != nil {
				genMismatch++
				if len(mismatchSamples) < 20 {
					mismatchSamples = append(mismatchSamples, line+" -> error: "+err.Error())
				}
				return
			}
			want := strings.TrimSpace(line)
			if got != want {
				genMismatch++
				if len(mismatchSamples) < 20 {
					mismatchSamples = append(mismatchSamples, line+" -> "+got)
				}
				return
			}
			pass++
		})
	}
	if len(mismatchSamples) > 0 {
		t.Logf("identity mismatch samples: %s", strings.Join(mismatchSamples, " || "))
	}
	t.Logf("identity round-trip: pass=%d parser-deferred=%d gen-mismatch=%d", pass, parseDeferred, genMismatch)

	// Baselines pinned from the slice-2 generator build; they turn the round-trip
	// tally into a real ship-gate. Ratchet them tighter as coverage improves (raise
	// the pass floor, lower the mismatch ceiling); never loosen them to mask a
	// regression. A drop in pass or a rise in gen-mismatch fails the build.
	const (
		minPass        = 745
		maxGenMismatch = 20
	)
	if pass < minPass {
		t.Errorf("identity round-trip regressed: pass=%d, want >= %d", pass, minPass)
	}
	if genMismatch > maxGenMismatch {
		t.Errorf("identity round-trip regressed: gen-mismatch=%d, want <= %d", genMismatch, maxGenMismatch)
	}
}
