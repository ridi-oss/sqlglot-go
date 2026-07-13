package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
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

// TestParseAnalyzeHistogram ports mysql's ANALYZE_EXPRESSION_PARSERS DROP/UPDATE
// branches and _parse_analyze_histogram (parser.py:1731-1740,9113-9150).
func TestParseAnalyzeHistogram(t *testing.T) {
	cases := []struct {
		sql           string
		action        string
		hasInner      bool
		innerKind     exp.Kind
		withPhrases   []string
		usingData     string
		updateOptions string
	}{
		{sql: "ANALYZE tbl DROP HISTOGRAM ON col1", action: "DROP"},
		{sql: "ANALYZE tbl UPDATE HISTOGRAM ON col1", action: "UPDATE"},
		{sql: "ANALYZE tbl UPDATE HISTOGRAM ON col1 USING DATA 'json_data'", action: "UPDATE", hasInner: true, innerKind: exp.KindUsingData, usingData: "json_data"},
		{sql: "ANALYZE tbl UPDATE HISTOGRAM ON col1 WITH 5 BUCKETS", action: "UPDATE", hasInner: true, innerKind: exp.KindAnalyzeWith, withPhrases: []string{"5 BUCKETS"}},
		{sql: "ANALYZE tbl UPDATE HISTOGRAM ON col1 WITH 5 BUCKETS AUTO UPDATE", action: "UPDATE", hasInner: true, innerKind: exp.KindAnalyzeWith, withPhrases: []string{"5 BUCKETS"}, updateOptions: "AUTO"},
		{sql: "ANALYZE tbl UPDATE HISTOGRAM ON col1 WITH 5 BUCKETS MANUAL UPDATE", action: "UPDATE", hasInner: true, innerKind: exp.KindAnalyzeWith, withPhrases: []string{"5 BUCKETS"}, updateOptions: "MANUAL"},
	}

	for _, c := range cases {
		analyze := parseOneDialect(t, c.sql, "mysql")
		if analyze.Kind() != exp.KindAnalyze {
			t.Fatalf("%q: kind = %v, want Analyze:\n%s", c.sql, analyze.Kind(), analyze.ToS())
		}
		if kind := analyze.Arg("kind"); kind != nil {
			t.Fatalf("%q: kind = %#v, want nil:\n%s", c.sql, kind, analyze.ToS())
		}
		if options := analyze.Arg("options"); options != nil {
			t.Fatalf("%q: options = %#v, want nil:\n%s", c.sql, options, analyze.ToS())
		}
		table := exprArg(t, analyze, "this")
		if table.Kind() != exp.KindTable || table.Name() != "tbl" {
			t.Fatalf("%q: analyze target mismatch:\n%s", c.sql, analyze.ToS())
		}

		histogram := exprArg(t, analyze, "expression")
		if histogram.Kind() != exp.KindAnalyzeHistogram || histogram.Text("this") != c.action {
			t.Fatalf("%q: histogram action mismatch:\n%s", c.sql, analyze.ToS())
		}
		columns := expressionsForArg(histogram, "expressions")
		if len(columns) != 1 || columns[0].Kind() != exp.KindColumn || columns[0].Name() != "col1" {
			t.Fatalf("%q: histogram columns mismatch:\n%s", c.sql, analyze.ToS())
		}
		if c.updateOptions == "" {
			if got := histogram.Arg("update_options"); got != nil {
				t.Fatalf("%q: update_options = %#v, want nil:\n%s", c.sql, got, analyze.ToS())
			}
		} else if got := histogram.Text("update_options"); got != c.updateOptions {
			t.Fatalf("%q: update_options = %q, want %q:\n%s", c.sql, got, c.updateOptions, analyze.ToS())
		}

		inner, _ := histogram.Arg("expression").(exp.Expression)
		if !c.hasInner {
			if inner != nil {
				t.Fatalf("%q: unexpected histogram expression:\n%s", c.sql, analyze.ToS())
			}
		} else if inner == nil || inner.Kind() != c.innerKind {
			t.Fatalf("%q: histogram expression kind mismatch:\n%s", c.sql, analyze.ToS())
		} else if c.innerKind == exp.KindUsingData {
			if data := exprArg(t, inner, "this"); data.Kind() != exp.KindLiteral || data.Text("this") != c.usingData {
				t.Fatalf("%q: USING DATA mismatch:\n%s", c.sql, analyze.ToS())
			}
		} else {
			phrases, ok := inner.Arg("expressions").([]string)
			if !ok || len(phrases) != len(c.withPhrases) {
				t.Fatalf("%q: WITH phrases = %#v, want %#v:\n%s", c.sql, inner.Arg("expressions"), c.withPhrases, analyze.ToS())
			}
			for i := range phrases {
				if phrases[i] != c.withPhrases[i] {
					t.Fatalf("%q: WITH phrases = %#v, want %#v:\n%s", c.sql, phrases, c.withPhrases, analyze.ToS())
				}
			}
		}

		if commands := analyze.FindAll(exp.KindCommand); len(commands) != 0 {
			t.Fatalf("%q: found %d Command descendants:\n%s", c.sql, len(commands), analyze.ToS())
		}
		got, err := generateSQL(t, analyze, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", c.sql, err)
		}
		if got != c.sql {
			t.Fatalf("%q: round-trip = %q", c.sql, got)
		}
	}
}
