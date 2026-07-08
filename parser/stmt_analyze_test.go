package parser_test

import (
	"testing"

	exp "github.com/sjincho/sqlglot-go/expressions"
)

// TestParseAnalyzeStructured ports the top level of _parse_analyze (parser.py:8975-9038),
// excluding the unported ANALYZE_EXPRESSION_PARSERS sub-family. Cases mirror
// testdata/parity_gaps.txt (postgres "ANALYZE TBL" gen mismatch; mysql LOCAL/
// NO_WRITE_TO_BINLOG TABLE parse failures).
func TestParseAnalyzeStructured(t *testing.T) {
	// https://github.com/tobymao/sqlglot postgres ANALYZE: bare table, no kind/options.
	analyze := parseOneDialect(t, "ANALYZE TBL", "postgres")
	if analyze.Kind() != exp.KindAnalyze {
		t.Fatalf("kind = %v, want Analyze:\n%s", analyze.Kind(), analyze.ToS())
	}
	if kind := analyze.Arg("kind"); kind != nil && kind != "" {
		t.Fatalf("kind = %#v, want empty:\n%s", kind, analyze.ToS())
	}
	if this := exprArg(t, analyze, "this"); this.Kind() != exp.KindTable {
		t.Fatalf("this should be Table:\n%s", analyze.ToS())
	}

	got, err := generateSQL(t, analyze, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "ANALYZE TBL" {
		t.Fatalf("round-trip = %q, want %q", got, "ANALYZE TBL")
	}

	// ANALYZE_STYLES options (VERBOSE, SKIP_LOCKED) precede the bare table.
	for _, sql := range []string{
		"ANALYZE VERBOSE SKIP_LOCKED TBL",
		"ANALYZE BUFFER_USAGE_LIMIT 1337 TBL",
	} {
		analyze = parseOneDialect(t, sql, "postgres")
		if analyze.Kind() != exp.KindAnalyze {
			t.Fatalf("%q: kind = %v, want Analyze:\n%s", sql, analyze.Kind(), analyze.ToS())
		}
		options, ok := analyze.Arg("options").([]string)
		if !ok || len(options) == 0 {
			t.Fatalf("%q: options = %#v, want non-empty:\n%s", sql, analyze.Arg("options"), analyze.ToS())
		}
		got, err = generateSQL(t, analyze, "postgres")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	}

	// mysql: ANALYZE_STYLES option + TABLE keyword + table name.
	for _, sql := range []string{
		"ANALYZE LOCAL TABLE tbl",
		"ANALYZE NO_WRITE_TO_BINLOG TABLE tbl",
	} {
		analyze = parseOneDialect(t, sql, "mysql")
		if analyze.Kind() != exp.KindAnalyze {
			t.Fatalf("%q: kind = %v, want Analyze:\n%s", sql, analyze.Kind(), analyze.ToS())
		}
		if kind := analyze.Arg("kind"); kind != "TABLE" {
			t.Fatalf("%q: kind = %#v, want \"TABLE\":\n%s", sql, kind, analyze.ToS())
		}
		if this := exprArg(t, analyze, "this"); this.Kind() != exp.KindTable {
			t.Fatalf("%q: this should be Table:\n%s", sql, analyze.ToS())
		}
		got, err = generateSQL(t, analyze, "mysql")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	}

	// postgres: `TBL(col1, col2)` isn't ANALYZE's own column-list grammar (which this
	// slice doesn't port) — it falls to the else branch's parse_table_parts(), whose
	// table-valued-function support (parser.py:4664-4670) reads `TBL` as a table-valued
	// function name with (col1, col2) as its call args, exactly like upstream. Verified
	// against the pinned reference: parse_one("ANALYZE TBL(col1, col2)", "postgres") ->
	// Analyze(this=Table(this=Anonymous(this=TBL, expressions=[Column(col1), Column(col2)]))).
	for _, sql := range []string{
		"ANALYZE TBL(col1, col2)",
		"ANALYZE VERBOSE SKIP_LOCKED TBL(col1, col2)",
	} {
		analyze = parseOneDialect(t, sql, "postgres")
		if analyze.Kind() != exp.KindAnalyze {
			t.Fatalf("%q: kind = %v, want Analyze:\n%s", sql, analyze.Kind(), analyze.ToS())
		}
		table := exprArg(t, analyze, "this")
		if table.Kind() != exp.KindTable {
			t.Fatalf("%q: this should be Table:\n%s", sql, analyze.ToS())
		}
		fn := exprArg(t, table, "this")
		if fn.Kind() != exp.KindAnonymous || fn.Text("this") != "TBL" {
			t.Fatalf("%q: table.this should be Anonymous(TBL):\n%s", sql, analyze.ToS())
		}
		got, err = generateSQL(t, analyze, "postgres")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	}
}

// TestParseAnalyzeCommandDegrade covers inputs whose grammar this slice doesn't
// structurally port: mysql's HISTOGRAM sub-family, i.e. ANALYZE_EXPRESSION_PARSERS'
// DROP/UPDATE branches (parser.py:1731-1740). Each degrades to a raw exp.Command that
// round-trips the original source text byte-identically (testdata/parity_gaps.txt).
func TestParseAnalyzeCommandDegrade(t *testing.T) {
	cases := []struct {
		dialect string
		sql     string
	}{
		{"mysql", "ANALYZE tbl DROP HISTOGRAM ON col1"},
		{"mysql", "ANALYZE tbl UPDATE HISTOGRAM ON col1"},
		{"mysql", "ANALYZE tbl UPDATE HISTOGRAM ON col1 USING DATA 'json_data'"},
		{"mysql", "ANALYZE tbl UPDATE HISTOGRAM ON col1 WITH 5 BUCKETS"},
		{"mysql", "ANALYZE tbl UPDATE HISTOGRAM ON col1 WITH 5 BUCKETS AUTO UPDATE"},
		{"mysql", "ANALYZE tbl UPDATE HISTOGRAM ON col1 WITH 5 BUCKETS MANUAL UPDATE"},
	}
	for _, c := range cases {
		analyze := parseOneDialect(t, c.sql, c.dialect)
		if analyze.Kind() != exp.KindCommand {
			t.Fatalf("%q (%s): kind = %v, want Command:\n%s", c.sql, c.dialect, analyze.Kind(), analyze.ToS())
		}
		if this := analyze.Arg("this"); this != "ANALYZE" {
			t.Fatalf("%q (%s): command.this = %#v, want \"ANALYZE\":\n%s", c.sql, c.dialect, this, analyze.ToS())
		}
		got, err := generateSQL(t, analyze, c.dialect)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != c.sql {
			t.Fatalf("round-trip = %q, want %q", got, c.sql)
		}
	}
}
