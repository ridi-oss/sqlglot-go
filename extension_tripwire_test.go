package sqlglot_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// extensionEntry is one row of testdata/upstream_extensions.jsonl: a construct sqlglot-go
// parses STRUCTURALLY that pinned upstream (sqlglot v30.12.0) does NOT — a deliberate
// grammar extension beyond upstream (see DEVIATIONS.md "Grammar extensions beyond upstream"
// and AGENTS.md "How deviations are tracked"). Each row is a live spec:
//
//   - go_kind: sqlglot-go must parse Sql to this root Kind class name (the correctness half).
//   - upstream: what pinned upstream does — "command" (falls back to exp.Command) or
//     "parse_error" (raises). The pin-tripwire (TestUpstreamExtensionsTripwire) re-asserts
//     this against the pinned Python, so the day a reference bump makes upstream parse the
//     construct structurally, the test FAILS with the reconcile note — converting a silent
//     divergence into a located, playbook-attached failure at bump time.
type extensionEntry struct {
	ID        string `json:"id"`
	Dialect   string `json:"dialect"`
	SQL       string `json:"sql"`
	Upstream  string `json:"upstream"` // "command" | "parse_error"
	GoKind    string `json:"go_kind"`
	Reconcile string `json:"reconcile"`
}

const upstreamExtensionsPath = "testdata/upstream_extensions.jsonl"

func loadExtensionLedger(t *testing.T) []extensionEntry {
	t.Helper()
	data, err := os.ReadFile(upstreamExtensionsPath)
	if err != nil {
		t.Fatalf("read %s: %v", upstreamExtensionsPath, err)
	}
	var entries []extensionEntry
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var e extensionEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("%s line %d: %v", upstreamExtensionsPath, lineNo+1, err)
		}
		entries = append(entries, e)
	}
	return entries
}

// TestUpstreamExtensionsGoSide is the correctness half: sqlglot-go must actually parse each
// ledgered construct to its recorded go_kind. Runs everywhere (no reference/Python needed).
func TestUpstreamExtensionsGoSide(t *testing.T) {
	for _, e := range loadExtensionLedger(t) {
		t.Run(e.ID, func(t *testing.T) {
			parsed, err := sqlglot.ParseOne(e.SQL, e.Dialect)
			if err != nil {
				t.Fatalf("sqlglot-go failed to parse %q (%s): %v", e.SQL, e.Dialect, err)
			}
			if got := exp.ClassName(parsed.Kind()); got != e.GoKind {
				t.Fatalf("%q (%s) parsed to %s, ledger says go_kind=%s", e.SQL, e.Dialect, got, e.GoKind)
			}
		})
	}
}

// TestUpstreamExtensionsTripwire is the pin-tripwire: it re-asserts that pinned upstream STILL
// does not parse each ledgered construct structurally. Gated on the pinned reference + python3
// being present (the audience is whoever bumps scripts/fetch-reference.sh — they have both), so
// it auto-skips in a bare checkout. A failure here at bump time means upstream caught up: read
// the row's "reconcile" note, adopt upstream's node shape, and delete the ledger entry.
func TestUpstreamExtensionsTripwire(t *testing.T) {
	entries := loadExtensionLedger(t)
	if len(entries) == 0 {
		t.Skip("no upstream extensions ledgered yet")
	}
	if _, err := os.Stat(".reference/sqlglot-v30.12.0/sqlglot"); err != nil {
		t.Skip("pinned reference absent (run scripts/fetch-reference.sh) — tripwire only runs at bump time")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available — tripwire only runs where the pinned reference can be exercised")
	}

	type probeIn struct {
		Dialect string `json:"dialect"`
		SQL     string `json:"sql"`
	}
	inputs := make([]probeIn, len(entries))
	for i, e := range entries {
		inputs[i] = probeIn{Dialect: e.Dialect, SQL: e.SQL}
	}
	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}

	const script = `
import json, sys
import sqlglot
from sqlglot import exp
out = []
for row in json.load(sys.stdin):
    d = None if row["dialect"] == "" else row["dialect"]
    try:
        e = sqlglot.parse_one(row["sql"], read=d)
        behavior = "command" if isinstance(e, exp.Command) else ("structured:" + type(e).__name__)
    except Exception:
        behavior = "parse_error"
    out.append({"behavior": behavior})
json.dump(out, sys.stdout, separators=(",", ":"))
`
	cmd := exec.Command(python, "-c", script)
	cmd.Env = append(os.Environ(), "PYTHONPATH=.reference/sqlglot-v30.12.0")
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run pinned-upstream oracle: %v", err)
	}
	var results []struct {
		Behavior string `json:"behavior"`
	}
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("decode oracle output: %v", err)
	}
	if len(results) != len(entries) {
		t.Fatalf("oracle returned %d results for %d entries", len(results), len(entries))
	}

	for i, e := range entries {
		got := results[i].Behavior
		if got != e.Upstream {
			t.Errorf("TRIPWIRE %q: pinned upstream now does %q for %q (%s), ledger recorded %q.\n"+
				"  Upstream may have caught up — reconcile: %s",
				e.ID, got, e.SQL, e.Dialect, e.Upstream, e.Reconcile)
		}
	}
}
