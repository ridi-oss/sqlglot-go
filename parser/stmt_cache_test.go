package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseCache/TestParseUncache port _parse_cache/_parse_uncache (parser.py:3743-3770):
// Spark's `CACHE [LAZY] TABLE x [OPTIONS(k = v)] [AS] <query>` / `UNCACHE TABLE
// [IF EXISTS] x`. Unlike e.g. _parse_analyze, upstream has no dedicated
// test_cache.py/test_uncache-style unit-test oracle for exp.Cache/exp.Uncache's
// structure — the only oracle is the tests/fixtures/identity.sql round-trip corpus
// (testdata/identity.sql:657-665,717-718), reused verbatim here as the correctness
// check, plus direct structural assertions against the arg shape documented at
// expressions/core.py:1583-1594.
func TestParseCache(t *testing.T) {
	cache := parseOne(t, "CACHE TABLE x")
	if cache.Kind() != exp.KindCache {
		t.Fatalf("kind = %v, want Cache:\n%s", cache.Kind(), cache.ToS())
	}
	if lazy := cache.Arg("lazy"); lazy != nil && lazy != false {
		t.Fatalf("lazy = %#v, want false/nil:\n%s", lazy, cache.ToS())
	}
	if this := exprArg(t, cache, "this"); this.Kind() != exp.KindTable || this.Name() != "x" {
		t.Fatalf("this mismatch:\n%s", cache.ToS())
	}
	if opts, ok := cache.Arg("options").([]exp.Expression); ok && len(opts) != 0 {
		t.Fatalf("options = %v, want empty:\n%s", opts, cache.ToS())
	}
	if expr := cache.Arg("expression"); expr != nil {
		t.Fatalf("expression = %#v, want nil:\n%s", expr, cache.ToS())
	}
	got, err := generateSQL(t, cache, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "CACHE TABLE x"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}

	cache = parseOne(t, "CACHE LAZY TABLE x")
	if lazy, ok := cache.Arg("lazy").(bool); !ok || !lazy {
		t.Fatalf("lazy = %#v, want true:\n%s", cache.Arg("lazy"), cache.ToS())
	}

	cache = parseOne(t, "CACHE LAZY TABLE x OPTIONS('storageLevel' = 'value')")
	opts, ok := cache.Arg("options").([]exp.Expression)
	if !ok || len(opts) != 2 {
		t.Fatalf("options = %#v, want a 2-element raw list:\n%s", cache.Arg("options"), cache.ToS())
	}
	if opts[0].Text("this") != "storageLevel" || opts[1].Text("this") != "value" {
		t.Fatalf("options mismatch: %s = %s:\n%s", opts[0].ToS(), opts[1].ToS(), cache.ToS())
	}
	if expr := cache.Arg("expression"); expr != nil {
		t.Fatalf("expression = %#v, want nil (no AS query):\n%s", expr, cache.ToS())
	}

	cache = parseOne(t, "CACHE LAZY TABLE x OPTIONS('storageLevel' = 'value') AS SELECT 1")
	if expr := exprArg(t, cache, "expression"); expr.Kind() != exp.KindSelect {
		t.Fatalf("expression should be Select:\n%s", cache.ToS())
	}

	cache = parseOne(t, "CACHE TABLE x AS WITH a AS (SELECT 1) SELECT a.* FROM a")
	expr := exprArg(t, cache, "expression")
	// The AS-query is a bare (nested=true) parseSelect: the leading WITH is folded
	// into the resulting Select's own with_ arg (parser.go parseSelect's CTE
	// handling), not wrapped in a separate With/CTE node.
	if expr.Kind() != exp.KindSelect || expr.Arg("with_") == nil {
		t.Fatalf("expression should be a Select carrying with_:\n%s", cache.ToS())
	}

	cache = parseOne(t, "CACHE TABLE x AS (SELECT 1 AS y)")
	if expr := exprArg(t, cache, "expression"); expr.Kind() != exp.KindSubquery {
		t.Fatalf("expression should be Subquery (parenthesized AS-query):\n%s", cache.ToS())
	}

	// Full round-trip sweep over the identity.sql CACHE cases this feature closes
	// (testdata/identity.sql:657-665; the N'...' OPTIONS key variant stays a
	// registered gap in testdata/parity_gaps.txt since it needs the unported
	// National-string node, out of scope here).
	for _, sql := range []string{
		"CACHE TABLE x",
		"CACHE LAZY TABLE x",
		"CACHE LAZY TABLE x OPTIONS('storageLevel' = 'value')",
		"CACHE LAZY TABLE x OPTIONS('storageLevel' = 'value') AS SELECT 1",
		"CACHE LAZY TABLE x OPTIONS('storageLevel' = 'value') AS WITH a AS (SELECT 1) SELECT a.* FROM a",
		"CACHE LAZY TABLE x AS WITH a AS (SELECT 1) SELECT a.* FROM a",
		"CACHE TABLE x AS WITH a AS (SELECT 1) SELECT a.* FROM a",
		"CACHE TABLE x AS (SELECT 1 AS y)",
	} {
		e := parseOne(t, sql)
		if e.Kind() != exp.KindCache {
			t.Fatalf("%q: kind = %v, want Cache:\n%s", sql, e.Kind(), e.ToS())
		}
		got, err := generateSQL(t, e, "")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

func TestParseUncache(t *testing.T) {
	uncache := parseOne(t, "UNCACHE TABLE x")
	if uncache.Kind() != exp.KindUncache {
		t.Fatalf("kind = %v, want Uncache:\n%s", uncache.Kind(), uncache.ToS())
	}
	if this := exprArg(t, uncache, "this"); this.Kind() != exp.KindTable || this.Name() != "x" {
		t.Fatalf("this mismatch:\n%s", uncache.ToS())
	}
	if exists := uncache.Arg("exists"); exists != nil && exists != false {
		t.Fatalf("exists = %#v, want false/nil:\n%s", exists, uncache.ToS())
	}
	got, err := generateSQL(t, uncache, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "UNCACHE TABLE x"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}

	uncache = parseOne(t, "UNCACHE TABLE IF EXISTS x")
	if exists, ok := uncache.Arg("exists").(bool); !ok || !exists {
		t.Fatalf("exists = %#v, want true:\n%s", uncache.Arg("exists"), uncache.ToS())
	}
	got, err = generateSQL(t, uncache, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "UNCACHE TABLE IF EXISTS x"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}
