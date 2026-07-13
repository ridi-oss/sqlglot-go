package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/parser"
)

func TestGroupByVariants(t *testing.T) {
	expression := parseOne(t, "SELECT a GROUP BY a")
	group := expression.Arg("group").(exp.Expression)
	if group.Kind() != exp.KindGroup || len(group.Expressions()) != 1 {
		t.Fatalf("GROUP BY shape = %s, want one expression", group.ToS())
	}

	expression = parseOne(t, "SELECT a GROUP BY CUBE(a)")
	group = expression.Arg("group").(exp.Expression)
	if cube := expressionsForArg(group, "cube"); len(cube) != 1 || cube[0].Kind() != exp.KindCube {
		t.Fatalf("GROUP BY CUBE shape = %s, want one Cube", group.ToS())
	}

	expression = parseOne(t, "SELECT a GROUP BY ROLLUP(a)")
	group = expression.Arg("group").(exp.Expression)
	if rollup := expressionsForArg(group, "rollup"); len(rollup) != 1 || rollup[0].Kind() != exp.KindRollup {
		t.Fatalf("GROUP BY ROLLUP shape = %s, want one Rollup", group.ToS())
	}

	expression = parseOne(t, "SELECT a GROUP BY GROUPING SETS((a),(b))")
	group = expression.Arg("group").(exp.Expression)
	sets := expressionsForArg(group, "grouping_sets")
	if len(sets) != 1 || sets[0].Kind() != exp.KindGroupingSets || len(sets[0].Expressions()) != 2 {
		t.Fatalf("GROUPING SETS shape = %s, want one GroupingSets with two entries", group.ToS())
	}
}

func TestQueryClauses(t *testing.T) {
	expression := parseOne(t, "SELECT a HAVING a > 1")
	if having, ok := expression.Arg("having").(exp.Expression); !ok || having.Kind() != exp.KindHaving {
		t.Fatalf("having = %#v, want Having", expression.Arg("having"))
	}

	expression = parseOne(t, "SELECT a QUALIFY a > 1")
	if qualify, ok := expression.Arg("qualify").(exp.Expression); !ok || qualify.Kind() != exp.KindQualify {
		t.Fatalf("qualify = %#v, want Qualify", expression.Arg("qualify"))
	}

	expression = parseOne(t, "SELECT a ORDER BY a DESC")
	order := expression.Arg("order").(exp.Expression)
	ordered := order.Expressions()[0]
	if ordered.Kind() != exp.KindOrdered || ordered.Arg("desc") != true || ordered.Arg("nulls_first") != false {
		t.Fatalf("ordered = %s, want desc nulls_last", ordered.ToS())
	}

	expression = parseOne(t, "SELECT a LIMIT 5 OFFSET 2")
	if limit, ok := expression.Arg("limit").(exp.Expression); !ok || limit.Kind() != exp.KindLimit {
		t.Fatalf("limit = %#v, want Limit", expression.Arg("limit"))
	}
	if offset, ok := expression.Arg("offset").(exp.Expression); !ok || offset.Kind() != exp.KindOffset {
		t.Fatalf("offset = %#v, want Offset", expression.Arg("offset"))
	}

	expression = parseOne(t, "SELECT DISTINCT ON (a) a FROM t")
	distinct := expression.Arg("distinct").(exp.Expression)
	if distinct.Kind() != exp.KindDistinct || distinct.Arg("on") == nil {
		t.Fatalf("distinct = %s, want DISTINCT ON", distinct.ToS())
	}
}

// TestLimitPercentModRetreat locks in _parse_limit's `exp.Mod` retreat (parser.py:5576-5579):
// `LIMIT x%` must not parse the count as a Mod term. Real forms are unaffected; the
// never-valid `LIMIT 10 % 3` must error on the trailing operand rather than build Mod(10, 3).
func TestLimitPercentModRetreat(t *testing.T) {
	for _, sql := range []string{"SELECT a LIMIT 10", "SELECT a LIMIT 10 PERCENT", "SELECT a LIMIT 10%"} {
		limit, ok := parseOne(t, sql).Arg("limit").(exp.Expression)
		if !ok || limit.Kind() != exp.KindLimit {
			t.Fatalf("%q: limit = %#v, want Limit", sql, limit)
		}
		if expr := limit.Expr(); expr != nil && expr.Kind() == exp.KindMod {
			t.Fatalf("%q: limit count parsed as Mod, want a plain count:\n%s", sql, limit.ToS())
		}
	}
	if _, err := sqlglot.ParseOne("SELECT a LIMIT 10 % 3", ""); err == nil {
		t.Fatal("LIMIT 10 % 3 should error on the trailing operand (upstream parity)")
	}
}

func TestWindowAndFilterClauses(t *testing.T) {
	expression := parseOne(t, "SELECT SUM(x) OVER (PARTITION BY a ORDER BY b)")
	window := expression.Expressions()[0]
	if window.Kind() != exp.KindWindow {
		t.Fatalf("projection kind = %v, want Window", window.Kind())
	}
	if partitionBy := expressionsForArg(window, "partition_by"); len(partitionBy) != 1 {
		t.Fatalf("partition_by count = %d, want 1", len(partitionBy))
	}
	if order, ok := window.Arg("order").(exp.Expression); !ok || order.Kind() != exp.KindOrder {
		t.Fatalf("window order = %#v, want Order", window.Arg("order"))
	}

	expression = parseOne(t, "SELECT SUM(x) FILTER (WHERE a > 0)")
	filter := expression.Expressions()[0]
	if filter.Kind() != exp.KindFilter {
		t.Fatalf("projection kind = %v, want Filter", filter.Kind())
	}

	expression = parseOne(t, "SELECT a WINDOW w AS (PARTITION BY x)")
	windows := expressionsForArg(expression, "windows")
	if len(windows) != 1 || windows[0].Kind() != exp.KindWindow {
		t.Fatalf("windows = %#v, want one Window", windows)
	}
}

func TestLocks(t *testing.T) {
	cases := []struct {
		sql    string
		update bool
		key    any
		wait   any
		ofLen  int
	}{
		{"SELECT * FROM t FOR UPDATE", true, nil, nil, 0},
		{"SELECT * FROM t FOR SHARE", false, nil, nil, 0},
		{"SELECT * FROM t LOCK IN SHARE MODE", false, nil, nil, 0},
		{"SELECT * FROM t FOR NO KEY UPDATE OF a NOWAIT", true, true, true, 1},
		{"SELECT * FROM t FOR SHARE OF t1, t2 SKIP LOCKED", false, nil, false, 2},
		{"SELECT * FROM t FOR KEY SHARE", false, true, nil, 0},
		{"SELECT * FROM t FOR NO KEY UPDATE", true, true, nil, 0},
	}
	for _, tc := range cases {
		expression := parseOne(t, tc.sql)
		locks := expressionsForArg(expression, "locks")
		if len(locks) != 1 || locks[0].Kind() != exp.KindLock {
			t.Fatalf("%s: locks = %#v, want one Lock:\n%s", tc.sql, locks, expression.ToS())
		}
		lock := locks[0]
		if lock.Arg("update") != tc.update || lock.Arg("key") != tc.key || lock.Arg("wait") != tc.wait {
			t.Fatalf("%s: lock args update/key/wait = %v/%v/%v, want %v/%v/%v:\n%s", tc.sql, lock.Arg("update"), lock.Arg("key"), lock.Arg("wait"), tc.update, tc.key, tc.wait, lock.ToS())
		}
		if got := len(expressionsForArg(lock, "expressions")); got != tc.ofLen {
			t.Fatalf("%s: OF expressions = %d, want %d:\n%s", tc.sql, got, tc.ofLen, lock.ToS())
		}
	}

	// Multiple lock clauses in sequence produce multiple Lock nodes in "locks".
	multi := "SELECT * FROM t FOR SHARE OF t1 NOWAIT FOR UPDATE OF t2, t3 SKIP LOCKED"
	expression := parseOne(t, multi)
	locks := expressionsForArg(expression, "locks")
	if len(locks) != 2 || locks[0].Kind() != exp.KindLock || locks[1].Kind() != exp.KindLock {
		t.Fatalf("%s: locks = %#v, want two Lock nodes:\n%s", multi, locks, expression.ToS())
	}
	first, second := locks[0], locks[1]
	if first.Arg("update") != false || first.Arg("key") != nil || first.Arg("wait") != true {
		t.Fatalf("%s: first lock args update/key/wait = %v/%v/%v, want false/nil/true:\n%s", multi, first.Arg("update"), first.Arg("key"), first.Arg("wait"), first.ToS())
	}
	if got := len(expressionsForArg(first, "expressions")); got != 1 {
		t.Fatalf("%s: first lock OF expressions = %d, want 1:\n%s", multi, got, first.ToS())
	}
	if second.Arg("update") != true || second.Arg("key") != nil || second.Arg("wait") != false {
		t.Fatalf("%s: second lock args update/key/wait = %v/%v/%v, want true/nil/false:\n%s", multi, second.Arg("update"), second.Arg("key"), second.Arg("wait"), second.ToS())
	}
	if got := len(expressionsForArg(second, "expressions")); got != 2 {
		t.Fatalf("%s: second lock OF expressions = %d, want 2:\n%s", multi, got, second.ToS())
	}
}

func TestClusterDistributeSort(t *testing.T) {
	cases := []struct {
		sql  string
		arg  string
		kind exp.Kind
	}{
		{"SELECT a FROM t CLUSTER BY x", "cluster", exp.KindCluster},
		{"SELECT a FROM t DISTRIBUTE BY x", "distribute", exp.KindDistribute},
		{"SELECT a FROM t SORT BY x DESC", "sort", exp.KindSort},
	}
	for _, tc := range cases {
		expression := parseOne(t, tc.sql)
		clause, ok := expression.Arg(tc.arg).(exp.Expression)
		if !ok || clause.Kind() != tc.kind {
			t.Fatalf("%s: %s = %#v, want %v:\n%s", tc.sql, tc.arg, expression.Arg(tc.arg), tc.kind, expression.ToS())
		}
	}
}

func TestWindowExtras(t *testing.T) {
	expression := parseOne(t, "SELECT PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY x) OVER ()")
	window := expression.Expressions()[0]
	if window.Kind() != exp.KindWindow || window.This() == nil || window.This().Kind() != exp.KindWithinGroup {
		t.Fatalf("WITHIN GROUP window mismatch:\n%s", expression.ToS())
	}

	expression = parseOne(t, "SELECT LAST_VALUE(x) IGNORE NULLS OVER (PARTITION BY y)")
	window = expression.Expressions()[0]
	if window.Kind() != exp.KindWindow || window.This() == nil || window.This().Kind() != exp.KindIgnoreNulls {
		t.Fatalf("IGNORE NULLS window mismatch:\n%s", expression.ToS())
	}

	expression = parseOne(t, "SELECT SUM(x) OVER (ORDER BY y ROWS BETWEEN 1 PRECEDING AND CURRENT ROW EXCLUDE TIES)")
	window = expression.Expressions()[0]
	spec := exprArg(t, window, "spec")
	exclude := exprArg(t, spec, "exclude")
	if exclude.Kind() != exp.KindVar || exclude.Name() != "TIES" {
		t.Fatalf("EXCLUDE mismatch:\n%s", spec.ToS())
	}

	// A malformed EXCLUDE option must error (upstream _parse_window uses the default
	// raise_unmatched=True), not silently retreat.
	if _, err := sqlglot.ParseOne("SELECT SUM(x) OVER (ORDER BY y ROWS BETWEEN 1 PRECEDING AND CURRENT ROW EXCLUDE FOOBAR)", ""); err == nil {
		t.Fatal("malformed EXCLUDE option should raise Unknown option")
	}
}

func TestDuplicateWhereIgnoreKeepsLast(t *testing.T) {
	d := dialects.Base()
	toks, err := d.NewTokenizer().Tokenize("SELECT a WHERE x = 1 WHERE y = 2")
	if err != nil {
		t.Fatalf("Tokenize error: %v", err)
	}
	p := parser.NewWithErrorLevel(d, sqlerrors.IGNORE)
	expressions, err := p.Parse(toks, "SELECT a WHERE x = 1 WHERE y = 2")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(expressions) != 1 || expressions[0] == nil {
		t.Fatalf("expressions = %#v, want one expression", expressions)
	}
	where := expressions[0].Arg("where").(exp.Expression)
	if where.Find(exp.KindColumn).Name() != "y" {
		t.Fatalf("WHERE kept %s, want last duplicate y", where.ToS())
	}
}
