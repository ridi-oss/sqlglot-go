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

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
)

// corpusRecord is one round-trip case: parse Sql under Dialect, generate with
// Pretty, and expect the output to equal Want. Scope A (identity.sql) sets
// Dialect="" and Want==Sql (the line itself); Scope B (dialect_identity.jsonl,
// produced by the P1 extraction script) sets Dialect to "mysql"/"postgres" and
// carries an independent Want (upstream's validate_identity write_sql).
//
// JSON field names match the P1 contract: {"dialect":..,"sql":..,"want":..,"pretty":..}.
type corpusRecord struct {
	Dialect string `json:"dialect"`
	Sql     string `json:"sql"`
	Want    string `json:"want"`
	Pretty  bool   `json:"pretty"`
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
	got, gerr = sqlglot.Generate(expression, rec.Dialect, generator.Options{Pretty: rec.Pretty})
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
	return "gen: mismatch"
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

// Scope B (dialect_identity.jsonl). These floors reflect the combined parity slices
// merged into main: the TYPE/CAST/`::`/AT TIME ZONE slice, the FROM/TABLE-modifier/
// LOCK/STATEMENT slice, and the FUNCTION + JSON-operator slice. Coverage now spans:
//   - TYPE/CAST: `1::int`-style literal casts, bare `ARRAY<...>`/`STRUCT<...>` type
//     expressions, `x AT TIME ZONE zone` (chainable), postgres PSEUDO_TYPE/
//     OBJECT_IDENTIFIER round-trips, postgres UDT CAST targets, the mysql SET(...) enum
//     CAST target and `CHAR CHARACTER SET <cs>` suffix (mysql attimezone_sql drops the
//     zone + flags unsupported, matching upstream).
//   - FROM/stmt/lock: TABLESAMPLE (`(BUCKET .. OUT OF ..)`, `(N ROWS)`, `(N PERCENT)`,
//     postgres `SYSTEM (N) REPEATABLE (N)`); LATERAL VIEW attach; row-locking reads
//     FOR UPDATE/SHARE/KEY SHARE/NO KEY UPDATE (with OF/NOWAIT/SKIP LOCKED/WAIT, gated
//     on LockingReadsSupported; mysql `LOCK IN SHARE MODE` -> `FOR SHARE`); CACHE/UNCACHE
//     TABLE; postgres COPY (degrading to a raw Command for shapes upstream doesn't model);
//     the STRAIGHT_JOIN-as-alias guard.
//   - functions/JSON: JSON_OBJECT/JSON_OBJECTAGG/JSON_VALUE (with OnCondition/JSONKeyValue),
//     CHR/CHAR + `CONVERT ... USING <charset>`, the STR_TO_*/TIME_STR_TO_* temporal family,
//     XMLELEMENT/XMLTABLE/XMLNAMESPACE, and the JSON arrow operators (-> ->> #> #>>) with
//     the only_json_types gate choosing operator vs. JSON_EXTRACT_PATH[_TEXT] function form
//     for postgres (mysql/base emit JSON_EXTRACT).
//
// The residual parity tail (testdata/parity_gaps.txt's last 25/26 entries: byte/hex/bit
// string literals + postgres e-strings, mysql session parameters/`:=` assignment/BINARY
// cast/CHARSET(...)/CHAR(0x.. USING ..), postgres Distance operators/VALUES-join grouping/
// DATE_PART/adjacent-string CONCAT/empty ARRAY[]::type[]/VARIADIC, the LEAD/LAG AggFunc
// window-reparse gate, and matchRParen's dropped trailing-comment attachment) is now fully
// closed - parity_gaps.txt is empty and every corpus record passes.
// These are monotonic pass floors — raise them as coverage improves, never lower them to
// mask a regression. A drop below any floor fails the build even if the regressing case is
// also (illegitimately) added to parity_gaps.txt. Values below are reconciled by a full
// SQLGLOT_CORPUS_UPDATE=1 run on the merged tree.
const (
	minPassBase     = 955
	minPassMySQL    = 424
	minPassPostgres = 468
)

// Minimum record counts per corpus, from the committed fixtures (identity.sql:
// 955 lines; dialect_identity.jsonl: 424 mysql + 468 postgres). Unlike the pass
// floors, these ratchet how many cases are *exercised*, independent of how many
// pass. Without them, silently dropping a FAILING record — a truncated
// re-extraction, or deleting dialect_identity.jsonl entirely — would leave the
// suite green: the pass-floor ratchet only notices dropped *passing* cases, and
// the subset check tolerates now-stale gap entries. Raise these when the corpus
// legitimately grows; a count below any floor means the corpus shrank.
const (
	minTotalBase     = 955
	minTotalMySQL    = 424
	minTotalPostgres = 468
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

	passBase, passMySQL, passPostgres := pass[""], pass["mysql"], pass["postgres"]

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
			"updated %s: N=%d base pass=%d/%d mysql pass=%d/%d postgres pass=%d/%d\n",
			parityGapsPath, len(records), passBase, total[""], passMySQL, total["mysql"], passPostgres, total["postgres"])
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

	if passBase < minPassBase {
		t.Errorf("base corpus round-trip regressed: pass=%d, want >= %d", passBase, minPassBase)
	}
	if passMySQL < minPassMySQL {
		t.Errorf("mysql corpus round-trip regressed: pass=%d, want >= %d", passMySQL, minPassMySQL)
	}
	if passPostgres < minPassPostgres {
		t.Errorf("postgres corpus round-trip regressed: pass=%d, want >= %d", passPostgres, minPassPostgres)
	}

	// Completeness floors: catch a corpus that shrank (a dropped/omitted case),
	// which neither the subset check nor the pass floors would notice on their own.
	if total[""] < minTotalBase {
		t.Errorf("base corpus shrank: %d records exercised, want >= %d (a corpus case was dropped)", total[""], minTotalBase)
	}
	if total["mysql"] < minTotalMySQL {
		t.Errorf("mysql corpus shrank: %d records exercised, want >= %d (dialect_identity.jsonl truncated or missing?)", total["mysql"], minTotalMySQL)
	}
	if total["postgres"] < minTotalPostgres {
		t.Errorf("postgres corpus shrank: %d records exercised, want >= %d (dialect_identity.jsonl truncated or missing?)", total["postgres"], minTotalPostgres)
	}

	t.Logf("corpus totals: N=%d; base pass=%d/%d fail=%d; mysql pass=%d/%d fail=%d; postgres pass=%d/%d fail=%d",
		len(records),
		passBase, total[""], total[""]-passBase,
		passMySQL, total["mysql"], total["mysql"]-passMySQL,
		passPostgres, total["postgres"], total["postgres"]-passPostgres)
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
