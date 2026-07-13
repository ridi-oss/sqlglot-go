package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestJSONOperators(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"SELECT a -> 'b'", exp.KindJSONExtract},
		{"SELECT a ->> 'b'", exp.KindJSONExtractScalar},
		{"SELECT a #> '{b}'", exp.KindJSONBExtract},
		{"SELECT a #>> '{b}'", exp.KindJSONBExtractScalar},
	}
	for _, tc := range cases {
		projection := parseOne(t, tc.sql).Expressions()[0]
		if projection.Kind() != tc.kind {
			t.Fatalf("%s: projection kind = %v, want %v:\n%s", tc.sql, projection.Kind(), tc.kind, projection.ToS())
		}
		if rhs := exprArg(t, projection, "expression"); rhs.Kind() != exp.KindLiteral {
			t.Fatalf("%s: RHS kind = %v, want Literal:\n%s", tc.sql, rhs.Kind(), projection.ToS())
		}
	}

	projection := parseOne(t, "SELECT a -> 'b' -> 'c'").Expressions()[0]
	if projection.Kind() != exp.KindJSONExtract || projection.This() == nil || projection.This().Kind() != exp.KindJSONExtract {
		t.Fatalf("nested JSON extract mismatch:\n%s", projection.ToS())
	}

	projection = parseOne(t, "SELECT x::INT").Expressions()[0]
	if projection.Kind() != exp.KindCast {
		t.Fatalf("dcolon cast regression: kind = %v, want Cast:\n%s", projection.Kind(), projection.ToS())
	}
}

// TestJSONArrowsOnlyJSONTypes covers only_json_types gating (parser.go's columnOperators
// ARROW/DARROW, dialects.Dialect.JSONArrowsRequireJSONType): base never sets it; postgres sets
// it iff the RHS is a Literal (parsers/postgres.py:191 + dialect.py's build_json_extract_path
// arrow_req_json_type branch), so a non-literal RHS like `-1` (which parses as Neg(Literal),
// not itself a Literal) leaves it unset.
func TestJSONArrowsOnlyJSONTypes(t *testing.T) {
	// Base: JSONArrowsRequireJSONType is false, so only_json_types is never set even for a
	// literal RHS.
	baseProjection := parseOne(t, "SELECT a -> 'b'").Expressions()[0]
	if v, _ := baseProjection.Arg("only_json_types").(bool); v {
		t.Fatalf("base: only_json_types = true, want unset/false:\n%s", baseProjection.ToS())
	}

	// Postgres cast-chain: '...'::JSON -> 'duration' ->> -1. The inner JSONExtract's RHS
	// ('duration') is a Literal, so only_json_types is set; the outer JSONExtractScalar's RHS
	// (-1, a Neg) is not a Literal, so only_json_types stays unset.
	outer := parseOneDialect(t, "SELECT x::JSON -> 'duration' ->> -1", "postgres").Expressions()[0]
	if outer.Kind() != exp.KindJSONExtractScalar {
		t.Fatalf("outer kind = %v, want JSONExtractScalar:\n%s", outer.Kind(), outer.ToS())
	}
	if v, _ := outer.Arg("only_json_types").(bool); v {
		t.Fatalf("outer (Neg RHS): only_json_types = true, want unset/false:\n%s", outer.ToS())
	}
	inner := exprArg(t, outer, "this")
	if inner.Kind() != exp.KindJSONExtract {
		t.Fatalf("inner kind = %v, want JSONExtract:\n%s", inner.Kind(), inner.ToS())
	}
	if v, _ := inner.Arg("only_json_types").(bool); !v {
		t.Fatalf("inner (literal RHS 'duration'): only_json_types = false, want true:\n%s", inner.ToS())
	}

	// Postgres: literal-key `#>`/`#>>` (JSONBExtract/JSONBExtractScalar) never carry
	// only_json_types - that arg isn't even declared for those Kinds (kinds.go:509-510).
	jsonb := parseOneDialect(t, "SELECT a #> '{b}'", "postgres").Expressions()[0]
	if _, ok := jsonb.Arg("only_json_types").(bool); ok {
		t.Fatalf("JSONBExtract carries only_json_types, want it undeclared:\n%s", jsonb.ToS())
	}
}
