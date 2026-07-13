package expressions_test

import (
	"reflect"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestJSONObjectKinds is a NEW file (rather than an addition to
// expressions_functions_test.go, per the P1 contract) to avoid sibling-builder collisions
// on that shared file. It covers the JSON_OBJECT/JSON_OBJECTAGG/JSON_VALUE family:
// JSONObject/JSONObjectAgg (expressions/json.py:160-177), JSONKeyValue/OnCondition/
// JSONValue (expressions/query.py:2026,577,2045).
func TestJSONObjectKinds(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"JSON_OBJECT('a': 1)", exp.KindJSONObject},
		{"JSON_OBJECT(*)", exp.KindJSONObject},
		{"JSON_OBJECTAGG('a': 1)", exp.KindJSONObjectAgg},
		{"JSON_OBJECTAGG(KEY 'a' VALUE 1)", exp.KindJSONObjectAgg},
	}
	for _, tc := range cases {
		expression := parseOne(t, tc.sql)
		if expression.Kind() != tc.kind {
			t.Fatalf("ParseOne(%q).Kind() = %v, want %v", tc.sql, expression.Kind(), tc.kind)
		}
	}
}

// TestJSONObjectExpressionsShape checks that JSON_OBJECT's key/value pairs parse into
// JSONKeyValue children (this=key, expression=value), mirroring _parse_json_key_value
// (parser.py:8043-8052).
func TestJSONObjectExpressionsShape(t *testing.T) {
	expression := parseOne(t, "JSON_OBJECT('a': 1, 'b': 2)")
	kvs := expression.Expressions()
	if len(kvs) != 2 {
		t.Fatalf("JSON_OBJECT expressions count = %d, want 2:\n%s", len(kvs), expression.ToS())
	}
	for _, kv := range kvs {
		if kv.Kind() != exp.KindJSONKeyValue {
			t.Fatalf("pair kind = %v, want JSONKeyValue:\n%s", kv.Kind(), expression.ToS())
		}
		if kv.This() == nil || kv.Expr() == nil {
			t.Fatalf("JSONKeyValue missing this/expression:\n%s", kv.ToS())
		}
	}
}

// TestJSONObjectArgOrder pins the arg_types key order (mirrors upstream's dict declaration
// order, consumed by ArgKeys/functionFallbackSQL) for all five new Kinds.
func TestJSONObjectArgOrder(t *testing.T) {
	jsonObjectArgs := []string{"expressions", "null_handling", "unique_keys", "return_type", "encoding"}
	cases := []struct {
		kind exp.Kind
		want []string
	}{
		{exp.KindJSONObject, jsonObjectArgs},
		{exp.KindJSONObjectAgg, jsonObjectArgs},
		{exp.KindJSONKeyValue, []string{"this", "expression"}},
		{exp.KindOnCondition, []string{"error", "empty", "null"}},
		{exp.KindJSONValue, []string{"this", "path", "returning", "on_condition"}},
	}
	for _, tc := range cases {
		if got := exp.ArgKeys(tc.kind); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ArgKeys(%v) = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

// TestJSONObjectClassNames pins the PascalCase class names used by ClassName (the generic
// TraitFunc fallback's function name, and error messages).
func TestJSONObjectClassNames(t *testing.T) {
	cases := map[exp.Kind]string{
		exp.KindJSONObject:    "JSONObject",
		exp.KindJSONObjectAgg: "JSONObjectAgg",
		exp.KindJSONKeyValue:  "JSONKeyValue",
		exp.KindOnCondition:   "OnCondition",
		exp.KindJSONValue:     "JSONValue",
	}
	for k, want := range cases {
		if got := exp.ClassName(k); got != want {
			t.Fatalf("ClassName(%v) = %q, want %q", k, got, want)
		}
	}
}

// TestJSONObjectTraits pins the Func/Condition/AggFunc trait bits: JSONObject/JSONObjectAgg
// are Func subclasses (JSONObjectAgg is also an AggFunc, json.py:171-177), while
// JSONKeyValue/OnCondition/JSONValue are plain Expression subclasses with no traits at all
// (query.py:2026,577,2045).
func TestJSONObjectTraits(t *testing.T) {
	obj := exp.JSONObject(exp.Args{})
	if !obj.Is(exp.TraitFunc) || !obj.Is(exp.TraitCondition) {
		t.Fatalf("JSONObject should carry Func|Condition traits")
	}
	if obj.Is(exp.TraitAggFunc) {
		t.Fatalf("JSONObject should not carry the AggFunc trait")
	}

	agg := exp.JSONObjectAgg(exp.Args{})
	if !agg.Is(exp.TraitFunc) || !agg.Is(exp.TraitCondition) || !agg.Is(exp.TraitAggFunc) {
		t.Fatalf("JSONObjectAgg should carry Func|Condition|AggFunc traits")
	}

	kv := exp.JSONKeyValue(exp.Args{"this": exp.LiteralString("a"), "expression": exp.LiteralNumber(1)})
	if kv.Is(exp.TraitFunc) || kv.Is(exp.TraitCondition) {
		t.Fatalf("JSONKeyValue should be a plain Expression (no Func/Condition trait)")
	}

	oc := exp.OnCondition(exp.Args{})
	if oc.Is(exp.TraitFunc) || oc.Is(exp.TraitCondition) {
		t.Fatalf("OnCondition should be a plain Expression (no Func/Condition trait)")
	}

	jv := exp.JSONValue(exp.Args{"this": exp.LiteralString("x"), "path": exp.LiteralString("$.a")})
	if jv.Is(exp.TraitFunc) || jv.Is(exp.TraitCondition) {
		t.Fatalf("JSONValue should be a plain Expression (no Func/Condition trait)")
	}
}

// TestJSONValueParses checks the base JSON_VALUE(<doc>, <path>) shape (base/postgres route
// JSON_VALUE through the plain Anonymous fallback - only MySQL registers a FUNCTION_PARSERS
// entry, parsers/mysql.py:161 - so this exercises the mysql-gated parseJSONValue directly).
func TestJSONValueParses(t *testing.T) {
	expression, err := sqlglot.ParseOne("SELECT JSON_VALUE(x, '$.a')", "mysql")
	if err != nil {
		t.Fatalf("ParseOne error: %v", err)
	}
	projection := expression.Expressions()[0]
	if projection.Kind() != exp.KindJSONValue {
		t.Fatalf("kind = %v, want JSONValue:\n%s", projection.Kind(), projection.ToS())
	}
	if projection.This() == nil {
		t.Fatalf("JSONValue.this is nil:\n%s", projection.ToS())
	}
	if path, ok := projection.Arg("path").(exp.Expression); !ok || path == nil {
		t.Fatalf("JSONValue.path is nil:\n%s", projection.ToS())
	}
}
