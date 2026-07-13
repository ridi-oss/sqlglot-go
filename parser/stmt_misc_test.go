package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseUse ports the USE cases from parser.py:3735-3741 (_parse_use):
// a bare table/db name, and each USABLES kind keyword ahead of it.
func TestParseUse(t *testing.T) {
	for _, sql := range []string{
		"USE db",
		"USE ROLE x",
		"USE WAREHOUSE x",
		"USE DATABASE x",
		"USE SCHEMA x.y",
		"USE CATALOG abc",
	} {
		use := parseOne(t, sql)
		if use.Kind() != exp.KindUse {
			t.Fatalf("%q: kind = %v, want Use:\n%s", sql, use.Kind(), use.ToS())
		}
		got, err := generateSQL(t, use, "")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

// TestParseKill ports _parse_kill (parser.py:3550-3553): a bare primary
// target, and each of the CONNECTION/QUERY kind keywords ahead of it.
func TestParseKill(t *testing.T) {
	for _, sql := range []string{
		"KILL '123'",
		"KILL CONNECTION 123",
		"KILL QUERY '123'",
	} {
		kill := parseOne(t, sql)
		if kill.Kind() != exp.KindKill {
			t.Fatalf("%q: kind = %v, want Kill:\n%s", sql, kill.Kind(), kill.ToS())
		}
		got, err := generateSQL(t, kill, "")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

// TestParseDescribe ports _parse_describe (parser.py:3416-3445): a bare
// table, DESCRIBE_STYLES ahead of a qualified table name, and a bare SELECT
// (which needs the local parseDescribeThis workaround; see
// parser/stmt_misc.go).
func TestParseDescribe(t *testing.T) {
	for _, sql := range []string{
		"DESCRIBE x",
		"DESCRIBE EXTENDED a.b",
		"DESCRIBE FORMATTED a.b",
		"DESCRIBE SELECT 1",
	} {
		describe := parseOne(t, sql)
		if describe.Kind() != exp.KindDescribe {
			t.Fatalf("%q: kind = %v, want Describe:\n%s", sql, describe.Kind(), describe.ToS())
		}
		got, err := generateSQL(t, describe, "")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}

	extended := parseOne(t, "DESCRIBE EXTENDED a.b")
	if style := extended.Text("style"); style != "EXTENDED" {
		t.Fatalf("style = %q, want EXTENDED:\n%s", style, extended.ToS())
	}
}

// TestParseDescribeMySQLExplainAnalyze ports test_mysql.py:1599 test_explain:
// mysql tokenizes EXPLAIN as DESCRIBE, and ANALYZE is a DESCRIBE_STYLES
// keyword, so `EXPLAIN ANALYZE SELECT * FROM t` is a structured Describe
// whose "this" is a bare Select - the parseDescribeThis workaround this
// slice adds (parser/stmt_misc.go) for the schema=true SELECT fallback that
// the shared parseTable can't reach.
func TestParseDescribeMySQLExplainAnalyze(t *testing.T) {
	sql := "EXPLAIN ANALYZE SELECT * FROM t"
	want := "DESCRIBE ANALYZE SELECT * FROM t"

	describe := parseOneDialect(t, sql, "mysql")
	if describe.Kind() != exp.KindDescribe {
		t.Fatalf("kind = %v, want Describe:\n%s", describe.Kind(), describe.ToS())
	}
	if style := describe.Text("style"); style != "ANALYZE" {
		t.Fatalf("style = %q, want ANALYZE:\n%s", style, describe.ToS())
	}
	if this := exprArg(t, describe, "this"); this.Kind() != exp.KindSelect {
		t.Fatalf("this should be Select:\n%s", describe.ToS())
	}

	got, err := generateSQL(t, describe, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestParseLoadData ports the DATA branch of _parse_load (parser.py:3653-3678).
func TestParseLoadData(t *testing.T) {
	for _, sql := range []string{
		"LOAD DATA INPATH 'x' INTO TABLE y PARTITION(ds = 'yyyy')",
		"LOAD DATA LOCAL INPATH 'x' INTO TABLE y PARTITION(ds = 'yyyy')",
		"LOAD DATA LOCAL INPATH 'x' INTO TABLE y PARTITION(ds = 'yyyy') INPUTFORMAT 'y'",
		"LOAD DATA LOCAL INPATH 'x' INTO TABLE y PARTITION(ds = 'yyyy') INPUTFORMAT 'y' SERDE 'z'",
		"LOAD DATA INPATH 'x' INTO TABLE y INPUTFORMAT 'y' SERDE 'z'",
		"LOAD DATA INPATH 'x' INTO TABLE y.b INPUTFORMAT 'y' SERDE 'z'",
	} {
		load := parseOne(t, sql)
		if load.Kind() != exp.KindLoadData {
			t.Fatalf("%q: kind = %v, want LoadData:\n%s", sql, load.Kind(), load.ToS())
		}
		got, err := generateSQL(t, load, "")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

// TestParseDescribeFormat covers mysql's `FORMAT=<fmt>` DESCRIBE option, parsed
// structurally into a Describe carrying a FileFormatProperty (parser.py:3425). Because
// describe_sql always emits the DESCRIBE keyword, the EXPLAIN/DESC aliases (mysql
// tokenizes EXPLAIN as DESCRIBE) normalize to DESCRIBE on round-trip.
func TestParseDescribeFormat(t *testing.T) {
	for _, format := range []string{"JSON", "TRADITIONAL", "TREE"} {
		sql := "DESCRIBE FORMAT=" + format + " UPDATE test SET test_col = 'abc'"
		describe := parseOneDialect(t, sql, "mysql")
		if describe.Kind() != exp.KindDescribe {
			t.Fatalf("%q: kind = %v, want Describe:\n%s", sql, describe.Kind(), describe.ToS())
		}
		got, err := generateSQL(t, describe, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}

	// EXPLAIN/DESC aliases normalize to the canonical DESCRIBE spelling.
	for _, tc := range []struct{ sql, want string }{
		{"EXPLAIN FORMAT=JSON UPDATE test SET test_col = 'abc'", "DESCRIBE FORMAT=JSON UPDATE test SET test_col = 'abc'"},
		{"DESC FORMAT=TREE UPDATE test SET test_col = 'abc'", "DESCRIBE FORMAT=TREE UPDATE test SET test_col = 'abc'"},
	} {
		describe := parseOneDialect(t, tc.sql, "mysql")
		if describe.Kind() != exp.KindDescribe {
			t.Fatalf("%q: kind = %v, want Describe:\n%s", tc.sql, describe.Kind(), describe.ToS())
		}
		got, err := generateSQL(t, describe, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", tc.sql, err)
		}
		if got != tc.want {
			t.Fatalf("%q: round-trip = %q, want %q", tc.sql, got, tc.want)
		}
	}
}
