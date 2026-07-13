package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// roundTrip parses sql under dialect and regenerates it, failing the test if either step
// errors or the output doesn't equal want.
func roundTripRangeOps(t *testing.T, sql, dialect, want string) exp.Expression {
	t.Helper()
	expression := parseOneDialect(t, sql, dialect)
	got, err := generateSQL(t, expression, dialect)
	if err != nil {
		t.Fatalf("generate(%q, %q) error: %v", sql, dialect, err)
	}
	if got != want {
		t.Fatalf("round-trip(%q, %q) = %q, want %q", sql, dialect, got, want)
	}
	return expression
}

// TestRangeOpsBinaryOperators covers the base-dialect binary range operators added to
// parseRange's switch: GLOB, OVERLAPS and postgres' Adjacent (-|-), which is base-tokenized
// (parity_gaps.txt gaps 44/63-65/218).
func TestRangeOpsBinaryOperators(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"x GLOB '??-*'", exp.KindGlob},
		{"x GLOB y", exp.KindGlob},
		{"a OVERLAPS b", exp.KindOverlaps},
		{"SELECT NUMRANGE(1.1, 2.2) -|- NUMRANGE(2.2, 3.3)", exp.KindAdjacent},
	}
	for _, tc := range cases {
		roundTripRangeOps(t, tc.sql, "", tc.sql)
		expression := parseOne(t, tc.sql)
		if expression.Kind() == exp.KindSelect {
			expression = expression.Expressions()[0]
		}
		if expression.Kind() != tc.kind {
			t.Fatalf("%s: kind = %v, want %v:\n%s", tc.sql, expression.Kind(), tc.kind, expression.ToS())
		}
	}
}

// TestRangeOpsEscape covers gaps 36/66/68: LIKE/ILIKE now wrap in parseEscape like
// SIMILAR_TO already did.
func TestRangeOpsEscape(t *testing.T) {
	cases := []string{
		"x LIKE '%y%' ESCAPE '\\'",
		"x ILIKE '%y%' ESCAPE '\\'",
		"SELECT 'Ac' ILIKE 'a%c' ESCAPE NULL",
	}
	for _, sql := range cases {
		roundTripRangeOps(t, sql, "", sql)
	}

	expression := parseOne(t, "x LIKE '%y%' ESCAPE '\\'")
	if expression.Kind() != exp.KindEscape {
		t.Fatalf("kind = %v, want Escape:\n%s", expression.Kind(), expression.ToS())
	}
	if exprArg(t, expression, "this").Kind() != exp.KindLike {
		t.Fatalf("Escape.this kind = %v, want Like:\n%s", exprArg(t, expression, "this").Kind(), expression.ToS())
	}
}

// TestRangeOpsChainedIs covers gap 67 (x IS TRUE IS TRUE): parseRange's new `case
// tokens.IS` plus the pre-existing trailing `if p.match(tokens.IS)` check give a chain two
// chances to consume an IS, matching upstream's RANGE_PARSERS[IS] + the unconditional
// post-negate IS check (parser.py:5862-5881).
func TestRangeOpsChainedIs(t *testing.T) {
	sql := "x IS TRUE IS TRUE"
	roundTripRangeOps(t, sql, "", sql)

	expression := parseOne(t, sql)
	if expression.Kind() != exp.KindIs {
		t.Fatalf("kind = %v, want Is:\n%s", expression.Kind(), expression.ToS())
	}
	inner := exprArg(t, expression, "this")
	if inner.Kind() != exp.KindIs {
		t.Fatalf("outer Is.this kind = %v, want Is:\n%s", inner.Kind(), expression.ToS())
	}
}

// TestRangeOpsNotnull covers postgres gap 226: `x NOTNULL` -> Not(Is(x, Null())).
func TestRangeOpsNotnull(t *testing.T) {
	sql := "SELECT id, email, CAST(deleted AS TEXT) FROM users WHERE deleted NOTNULL"
	want := "SELECT id, email, CAST(deleted AS TEXT) FROM users WHERE NOT deleted IS NULL"
	roundTripRangeOps(t, sql, "postgres", want)
}

// TestRangeOpsIsJson covers postgres gaps 229/230: `x IS JSON [kind] [WITH|WITHOUT]
// [UNIQUE [KEYS]]`.
func TestRangeOpsIsJson(t *testing.T) {
	cases := []string{
		`SELECT js, js IS JSON AS "json?", js IS JSON VALUE AS "scalar?", js IS JSON SCALAR AS "scalar?", js IS JSON OBJECT AS "object?", js IS JSON ARRAY AS "array?" FROM t`,
		`SELECT js, js IS JSON ARRAY WITH UNIQUE KEYS AS "array w. UK?", js IS JSON ARRAY WITHOUT UNIQUE KEYS AS "array w/o UK?", js IS JSON ARRAY UNIQUE KEYS AS "array w UK 2?" FROM t`,
	}
	for _, sql := range cases {
		roundTripRangeOps(t, sql, "postgres", sql)
	}

	expression := parseOneDialect(t, `SELECT js IS JSON VALUE FROM t`, "postgres")
	projection := expression.Expressions()[0]
	if projection.Kind() != exp.KindIs {
		t.Fatalf("kind = %v, want Is:\n%s", projection.Kind(), projection.ToS())
	}
	kindNode := exprArg(t, projection, "expression")
	if kindNode.Kind() != exp.KindJSON {
		t.Fatalf("Is.expression kind = %v, want JSON:\n%s", kindNode.Kind(), projection.ToS())
	}
	if kindNode.Arg("this") != "VALUE" {
		t.Fatalf("JSON.this = %#v, want \"VALUE\":\n%s", kindNode.Arg("this"), projection.ToS())
	}
}

// TestRangeOpsPostgresJsonbAndArrayOperators covers postgres gaps 206/209-211/214/217/232-241:
// jsonb/array binary operators and the OPERATOR(...)/regexp family, all round-tripping
// byte-for-byte.
func TestRangeOpsPostgresJsonbAndArrayOperators(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"ARRAY[1, 2, 3] && ARRAY[1, 2]", exp.KindArrayOverlaps},
		{"SELECT ARRAY[1, 2, 3] <@ ARRAY[1, 2]", exp.KindArrayContainedBy},
		{"a #- b", exp.KindJSONBDeleteAtPath},
		{"a ?& b", exp.KindJSONBContainsAllTopKeys},
		{"a ?| b", exp.KindJSONBContainsAnyTopKeys},
		{"doc @? '$.a[*] ? (@ > 2)'", exp.KindJSONBPathExists},
		{"x ? 'x'", exp.KindJSONBContains},
		{"x @@ y", exp.KindMatchAgainst},
		{"x ~ 'y'", exp.KindRegexpLike},
		{"x ~* 'y'", exp.KindRegexpILike},
	}
	for _, tc := range cases {
		expression := roundTripRangeOps(t, tc.sql, "postgres", tc.sql)
		if expression.Kind() == exp.KindSelect {
			expression = expression.Expressions()[0]
		}
		if expression.Kind() != tc.kind {
			t.Fatalf("%s: kind = %v, want %v:\n%s", tc.sql, expression.Kind(), tc.kind, expression.ToS())
		}
	}

	// NOT-negated regexp forms desugar to Not(RegexpLike/RegexpILike) (parser.py
	// _negate_range's generic Not fallback, since RegexpLike/ILike aren't Like/ILike).
	notCases := []struct {
		sql  string
		want string
	}{
		{"x !~ 'y'", "NOT x ~ 'y'"},
		{"x !~* 'y'", "NOT x ~* 'y'"},
	}
	for _, tc := range notCases {
		expression := roundTripRangeOps(t, tc.sql, "postgres", tc.want)
		if expression.Kind() != exp.KindNot {
			t.Fatalf("%s: kind = %v, want Not:\n%s", tc.sql, expression.Kind(), expression.ToS())
		}
	}
}

// TestRangeOpsOperator covers postgres gap 220's OPERATOR(...) half: `x OPERATOR(op) y`,
// chainable, rendered back via the existing g.binary "operator" special-case.
func TestRangeOpsOperator(t *testing.T) {
	sql := "x OPERATOR(pg_catalog.~) 'y'"
	expression := roundTripRangeOps(t, sql, "postgres", sql)
	if expression.Kind() != exp.KindOperator {
		t.Fatalf("kind = %v, want Operator:\n%s", expression.Kind(), expression.ToS())
	}
	if expression.Arg("operator") != "pg_catalog.~" {
		t.Fatalf("operator = %#v, want \"pg_catalog.~\":\n%s", expression.Arg("operator"), expression.ToS())
	}
}

// TestRangeOpsMysqlMemberOfAndSoundsLike covers mysql gaps 130-135/148/152/155/131-132:
// MEMBER OF -> JSONArrayContains, SOUNDS LIKE -> EQ(Soundex, Soundex) (upstream has no
// dedicated exp.MemberOf/exp.SoundsLike classes - parsers/mysql.py RANGE_PARSERS).
func TestRangeOpsMysqlMemberOfAndSoundsLike(t *testing.T) {
	cases := []string{
		`SELECT 'ab' MEMBER OF('[23, "abc", 17, "ab", 10]')`,
		`SELECT * FROM foo WHERE 'ab' MEMBER OF(content)`,
		`SELECT CAST('[4,5]' AS JSON) MEMBER OF('[[3,4],[4,5]]')`,
		`SELECT JSON_ARRAY(4, 5) MEMBER OF('[[3,4],[4,5]]')`,
		`SELECT @a MEMBER OF(@c), @b MEMBER OF(@c)`,
	}
	for _, sql := range cases {
		roundTripRangeOps(t, sql, "mysql", sql)
	}

	memberOf := parseOneDialect(t, `SELECT 'ab' MEMBER OF('[23]')`, "mysql").Expressions()[0]
	if memberOf.Kind() != exp.KindJSONArrayContains {
		t.Fatalf("kind = %v, want JSONArrayContains:\n%s", memberOf.Kind(), memberOf.ToS())
	}

	roundTripRangeOps(t, `SELECT 'foo' SOUNDS LIKE 'bar'`, "mysql", `SELECT SOUNDEX('foo') = SOUNDEX('bar')`)
	notSounds := roundTripRangeOps(t, `SELECT 'foo' NOT SOUNDS LIKE 'bar'`, "mysql", `SELECT NOT SOUNDEX('foo') = SOUNDEX('bar')`)
	if notSounds.Expressions()[0].Kind() != exp.KindNot {
		t.Fatalf("NOT SOUNDS LIKE kind = %v, want Not:\n%s", notSounds.Expressions()[0].Kind(), notSounds.ToS())
	}

	eq := parseOneDialect(t, `SELECT 'foo' SOUNDS LIKE 'bar'`, "mysql").Expressions()[0]
	if eq.Kind() != exp.KindEQ {
		t.Fatalf("kind = %v, want EQ:\n%s", eq.Kind(), eq.ToS())
	}
	if exprArg(t, eq, "this").Kind() != exp.KindSoundex {
		t.Fatalf("EQ.this kind = %v, want Soundex:\n%s", eq.ToS(), eq.ToS())
	}
}
