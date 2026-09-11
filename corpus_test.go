package sqlglot_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// corpusRecord is one round-trip case: parse Sql under Dialect, generate with
// Pretty, and expect the output to equal Want. Scope A (identity.sql) sets
// Dialect="" and Want==Sql (the line itself); Scope B (dialect_identity.jsonl,
// produced by the P1 extraction script) sets Dialect to the upstream test dialect and
// carries an independent Want (upstream's validate_identity write_sql).
//
// JSON field names match the P1 contract: {"dialect":..,"sql":..,"want":..,"pretty":..}.
type corpusRecord struct {
	Dialect string `json:"dialect"`
	Sql     string `json:"sql"`
	Want    string `json:"want"`
	Pretty  bool   `json:"pretty"`
	// Identify mirrors validate_identity(identify=True): quote every identifier on output.
	Identify bool `json:"identify,omitempty"`
}

// gapKey identifies a tracked round-trip gap. It intentionally excludes the
// category: a gap's label can be freely re-categorized without the subset
// check (see TestCorpus) treating that as a new/untracked failure.
type gapKey struct {
	Dialect string
	Sql     string
}

// gapRecord is the on-disk shape of one line of testdata/parity_gaps.txt.
type gapRecord struct {
	Dialect  string `json:"dialect"`
	Category string `json:"category"`
	Sql      string `json:"sql"`
}

const parityGapsPath = "testdata/parity_gaps.txt"

// loadIdentityCorpus reproduces the exact comparison semantics of the old
// TestIdentity (base dialect, ""): trim each line, skip blank/"--" lines, and
// require the generated SQL to equal the trimmed source line verbatim.
func loadIdentityCorpus(t *testing.T) []corpusRecord {
	t.Helper()
	data, err := os.ReadFile("testdata/identity.sql")
	if err != nil {
		t.Fatalf("read identity fixture: %v", err)
	}
	var records []corpusRecord
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		records = append(records, corpusRecord{Dialect: "", Sql: line, Want: line, Pretty: false})
	}
	return records
}

// parseCorpusJSONL parses one corpusRecord per non-empty line of JSONL data.
// It is factored out of loadDialectCorpus so it can be exercised directly in
// a self-contained test without depending on P1's committed fixture.
func parseCorpusJSONL(data []byte) ([]corpusRecord, error) {
	var records []corpusRecord
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var rec corpusRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("parse dialect corpus line %q: %w", line, err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// loadDialectCorpus reads Scope B (testdata/dialect_identity.jsonl), produced
// by the P1 extraction script. That file is owned by a sibling builder and is
// not committed by this part, so its absence must not fail the build: log and
// run Scope A only.
func loadDialectCorpus(t *testing.T) []corpusRecord {
	t.Helper()
	data, err := os.ReadFile("testdata/dialect_identity.jsonl")
	if err != nil {
		if os.IsNotExist(err) {
			t.Log("scope-B corpus absent: testdata/dialect_identity.jsonl not found, running Scope A only")
			return nil
		}
		t.Fatalf("read dialect corpus: %v", err)
	}
	records, err := parseCorpusJSONL(data)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return records
}

// assertNoDuplicateRecords enforces the Scope B extractor's dedup invariant
// (dialect_identity.jsonl is deduped on the full record). It hardens the
// record-count floors: without it, the floors could be satisfied by padding the
// corpus with duplicate lines while real cases were dropped. corpusRecord is
// all comparable fields, so it can key the set directly.
func assertNoDuplicateRecords(t *testing.T, records []corpusRecord) {
	t.Helper()
	seen := make(map[corpusRecord]struct{}, len(records))
	for _, rec := range records {
		if _, dup := seen[rec]; dup {
			t.Errorf("duplicate scope-B corpus record (dialect_identity.jsonl must be deduped): %+v", rec)
		}
		seen[rec] = struct{}{}
	}
}

// roundTrip parses rec.Sql under rec.Dialect and, on parse success, generates
// it back with rec.Pretty. A case passes iff both steps succeed and the
// output equals rec.Want; the individual errors are kept so categorize can
// bucket the reason.
func roundTrip(rec corpusRecord) (got string, perr, gerr error) {
	expression, perr := sqlglot.ParseOne(rec.Sql, rec.Dialect)
	if perr != nil {
		return "", perr, nil
	}
	opts := generator.Options{Pretty: rec.Pretty}
	if rec.Identify {
		opts.Identify = true
	}
	got, gerr = sqlglot.Generate(expression, rec.Dialect, opts)
	return got, nil, gerr
}

// categorize buckets a failing case for the parity_gaps.txt worklist. It is
// coarse and deterministic; the integrator hand-refines the top buckets'
// labels for the report. Categories are not part of the subset-check identity
// (see gapKey), so relabeling here never breaks the test.
func categorize(rec corpusRecord, got string, perr, gerr error) string {
	if perr != nil {
		return "parse: " + firstPhrase(perr.Error())
	}
	if gerr != nil {
		return "generate: error"
	}
	if strings.Contains(rec.Want, "CAST(") || strings.Contains(rec.Sql, "::") {
		return "gen: cast/type"
	}
	if strings.HasPrefix(rec.Sql, "CREATE ") || strings.HasPrefix(rec.Sql, "ALTER ") {
		return "gen: ddl/properties"
	}
	return "gen: canonicalization"
}

// firstPhrase trims a parse error message down to its leading sentence,
// dropping the "Line N, Col: M.\n  <context>" suffix that raiseError appends
// (parser/parser.go raiseError). Cutting at the first '.', '(' or ':' -
// whichever comes first - isolates the dominant message, e.g. "Invalid
// expression / Unexpected token" (parser/parser.go:356).
func firstPhrase(msg string) string {
	if i := strings.IndexAny(msg, ".(:"); i >= 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}

// loadGaps reads the committed worklist of known/expected round-trip
// failures. Only (dialect, sql) is used as the map key - the category column
// is metadata for humans, not part of the identity.
func loadGaps(t *testing.T) map[gapKey]bool {
	t.Helper()
	gaps := map[gapKey]bool{}
	data, err := os.ReadFile(parityGapsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return gaps
		}
		t.Fatalf("read %s: %v", parityGapsPath, err)
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var rec gapRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("parse %s line %q: %v", parityGapsPath, line, err)
		}
		gaps[gapKey{Dialect: rec.Dialect, Sql: rec.Sql}] = true
	}
	return gaps
}

// writeGaps rewrites testdata/parity_gaps.txt from the current failure set,
// sorted by (dialect, category, sql) for a stable, diffable file.
func writeGaps(fails map[gapKey]string) error {
	entries := make([]gapRecord, 0, len(fails))
	for k, category := range fails {
		entries = append(entries, gapRecord{Dialect: k.Dialect, Category: category, Sql: k.Sql})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dialect != entries[j].Dialect {
			return entries[i].Dialect < entries[j].Dialect
		}
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Sql < entries[j].Sql
	})
	var b strings.Builder
	// Use an Encoder with HTML escaping disabled so operators like "->" and
	// "<" stay literal, keeping the file readable and line-diffable.
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return os.WriteFile(parityGapsPath, []byte(b.String()), 0o644)
}

// Scope B includes MySQL, Postgres, and the Presto-family dialects Athena, Trino,
// Presto, and Hive. Their gaps in testdata/parity_gaps.txt are the generator-port
// burndown list. Pass and total floors are monotonic: raise them, never lower them.
const (
	minPassBase     = 980
	minPassMySQL    = 428
	minPassPostgres = 468
	minPassAthena   = 51
	minPassTrino    = 67
	minPassPresto   = 170
	minPassHive     = 141
)

// Total floors catch dropped failing records that pass floors cannot detect.
const (
	minTotalBase     = 980
	minTotalMySQL    = 428
	minTotalPostgres = 468
	minTotalAthena   = 51
	minTotalTrino    = 115
	minTotalPresto   = 170
	minTotalHive     = 141
)

// TestCorpus is the single round-trip harness for both the base-dialect
// corpus (Scope A, identity.sql) and the per-dialect validate_identity corpus
// (Scope B, dialect_identity.jsonl), replacing the old TestIdentity plus the
// hand-curated dialect identity tables.
//
// Three checks guard the corpus, each catching a distinct way coverage could
// silently erode:
//   - subset check: every failing case must already be listed in
//     testdata/parity_gaps.txt, so a new/untracked failure fails the build.
//   - per-corpus pass-floor ratchet: the pass count per dialect bucket must
//     stay >= its recorded minimum, so a regression can't be hidden by simply
//     ALSO adding it to parity_gaps.txt (the pass count would still drop).
//   - per-corpus completeness floor: the number of cases exercised per bucket
//     must stay >= its recorded minimum, so dropping a *failing* case — which
//     lowers neither the pass count nor trips the subset check — still fails.
//
// Together the corpus can only grow and the gaps list can only shrink: a real
// gap may be added deliberately, but a silent regression or a truncated corpus
// is caught by one of the three checks.
func TestCorpus(t *testing.T) {
	base := loadIdentityCorpus(t)
	dialect := loadDialectCorpus(t)
	assertNoDuplicateRecords(t, dialect)
	records := append(base, dialect...)

	total := map[string]int{}
	pass := map[string]int{}
	fails := map[gapKey]string{}

	for _, rec := range records {
		total[rec.Dialect]++
		got, perr, gerr := roundTrip(rec)
		if perr == nil && gerr == nil && got == rec.Want {
			pass[rec.Dialect]++
			continue
		}
		fails[gapKey{Dialect: rec.Dialect, Sql: rec.Sql}] = categorize(rec, got, perr, gerr)
	}

	floors := []struct {
		dialect  string
		minPass  int
		minTotal int
	}{
		{"", minPassBase, minTotalBase},
		{"mysql", minPassMySQL, minTotalMySQL},
		{"postgres", minPassPostgres, minTotalPostgres},
		{"athena", minPassAthena, minTotalAthena},
		{"trino", minPassTrino, minTotalTrino},
		{"presto", minPassPresto, minTotalPresto},
		{"hive", minPassHive, minTotalHive},
	}
	var counts []string
	for _, floor := range floors {
		name := floor.dialect
		if name == "" {
			name = "base"
		}
		counts = append(counts, fmt.Sprintf("%s pass=%d/%d fail=%d", name,
			pass[floor.dialect], total[floor.dialect], total[floor.dialect]-pass[floor.dialect]))
	}

	if os.Getenv("SQLGLOT_CORPUS_UPDATE") != "" {
		if err := writeGaps(fails); err != nil {
			t.Fatalf("write %s: %v", parityGapsPath, err)
		}
		// Emit the observed pass counts on stderr (not t.Log). The integrator
		// reads these to set the minPass* ratchets. stderr surfaces in directory
		// mode and under -v, whereas t.Log needs -v; but note that package-list
		// mode (`go test ./...`) still buffers a passing test's output, so run:
		//   SQLGLOT_CORPUS_UPDATE=1 go test . -run TestCorpus -v
		fmt.Fprintf(os.Stderr,
			"updated %s: N=%d; %s\n", parityGapsPath, len(records), strings.Join(counts, "; "))
		return
	}

	gaps := loadGaps(t)
	var untracked []string
	for k, category := range fails {
		if !gaps[k] {
			untracked = append(untracked, fmt.Sprintf("[%s] %s (%s)", k.Dialect, k.Sql, category))
		}
	}
	if len(untracked) > 0 {
		sort.Strings(untracked)
		t.Errorf("untracked round-trip failures (fix them, or record as known gaps in %s with\n"+
			"  SQLGLOT_CORPUS_UPDATE=1 go test . -run TestCorpus -v\n):\n%s",
			parityGapsPath, strings.Join(untracked, "\n"))
	}

	for _, floor := range floors {
		name := floor.dialect
		if name == "" {
			name = "base"
		}
		if pass[floor.dialect] < floor.minPass {
			t.Errorf("%s corpus round-trip regressed: pass=%d, want >= %d", name, pass[floor.dialect], floor.minPass)
		}
		if total[floor.dialect] < floor.minTotal {
			t.Errorf("%s corpus shrank: %d records exercised, want >= %d (a corpus case was dropped)", name, total[floor.dialect], floor.minTotal)
		}
	}
	t.Logf("corpus totals: N=%d; %s", len(records), strings.Join(counts, "; "))
}

// TestParseCorpusJSONL validates the JSONL loader independently of P1's
// generated fixture: write a couple of inline records to a temp file and
// assert loadDialectCorpus-style parsing round-trips them faithfully.
func TestParseCorpusJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dialect_identity.jsonl")
	content := "" +
		`{"dialect":"mysql","sql":"SELECT 1","want":"SELECT 1","pretty":false}` + "\n" +
		`{"dialect":"postgres","sql":"SELECT 1::int","want":"SELECT CAST(1 AS INT)","pretty":true}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp jsonl: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp jsonl: %v", err)
	}
	got, err := parseCorpusJSONL(data)
	if err != nil {
		t.Fatalf("parseCorpusJSONL: %v", err)
	}

	want := []corpusRecord{
		{Dialect: "mysql", Sql: "SELECT 1", Want: "SELECT 1", Pretty: false},
		{Dialect: "postgres", Sql: "SELECT 1::int", Want: "SELECT CAST(1 AS INT)", Pretty: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCorpusJSONL = %#v, want %#v", got, want)
	}
}
