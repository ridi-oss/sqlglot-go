package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestJSONObjectRoundTrip covers the base-dialect JSON_OBJECT(...) round-trip cases from
// testdata/identity.sql:829-837 directly (parser-level, in addition to the corpus harness):
// star/empty forms, colon-syntax key/value pairs, NULL-handling, WITH/WITHOUT UNIQUE KEYS,
// and RETURNING <type> [FORMAT JSON] ENCODING <var>.
func TestJSONObjectRoundTrip(t *testing.T) {
	cases := []string{
		"JSON_OBJECT()",
		"JSON_OBJECT(*)",
		"JSON_OBJECT('key1': 1, 'key2': TRUE)",
		"JSON_OBJECT('id': '5', 'fld1': 'bla', 'fld2': 'bar')",
		"JSON_OBJECT('x': NULL, 'y': 1 NULL ON NULL)",
		"JSON_OBJECT('x': NULL, 'y': 1 WITH UNIQUE KEYS)",
		"JSON_OBJECT('x': NULL, 'y': 1 ABSENT ON NULL WITH UNIQUE KEYS)",
		"JSON_OBJECT('x': 1 RETURNING VARCHAR(100))",
		"JSON_OBJECT('x': 1 RETURNING VARBINARY FORMAT JSON ENCODING UTF8)",
	}
	for _, sql := range cases {
		expression := parseOne(t, sql)
		if expression.Kind() != exp.KindJSONObject {
			t.Fatalf("ParseOne(%q).Kind() = %v, want JSONObject", sql, expression.Kind())
		}
		got, err := generateSQL(t, expression, "")
		if err != nil {
			t.Fatalf("generate(%q) error: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("round-trip mismatch:\n  sql:  %s\n  want: %s\n  got:  %s", sql, sql, got)
		}
	}
}

// TestJSONObjectAggRoundTrip mirrors TestJSONObjectRoundTrip for JSON_OBJECTAGG. It also
// covers the "KEY <k> VALUE <v>" key/value spelling (parser.py:8043-8047), which parses to
// the same JSONKeyValue shape as the colon syntax but - since the generator always emits
// the canonical `<key>: <value>` form (jsonkeyvalue_sql, generator.py:3746-3747) - doesn't
// round-trip byte-for-byte, so it's checked against the colon-syntax spelling instead.
func TestJSONObjectAggRoundTrip(t *testing.T) {
	sql := "JSON_OBJECTAGG('key1': 1)"
	expression := parseOne(t, sql)
	if expression.Kind() != exp.KindJSONObjectAgg {
		t.Fatalf("ParseOne(%q).Kind() = %v, want JSONObjectAgg", sql, expression.Kind())
	}
	got, err := generateSQL(t, expression, "")
	if err != nil {
		t.Fatalf("generate(%q) error: %v", sql, err)
	}
	if got != sql {
		t.Fatalf("round-trip mismatch:\n  sql:  %s\n  want: %s\n  got:  %s", sql, sql, got)
	}

	keyValueSpelling := parseOne(t, "JSON_OBJECTAGG(KEY 'key1' VALUE 1)")
	if keyValueSpelling.Kind() != exp.KindJSONObjectAgg {
		t.Fatalf("ParseOne(KEY/VALUE spelling).Kind() = %v, want JSONObjectAgg", keyValueSpelling.Kind())
	}
	got, err = generateSQL(t, keyValueSpelling, "")
	if err != nil {
		t.Fatalf("generate(KEY/VALUE spelling) error: %v", err)
	}
	if got != sql {
		t.Fatalf("KEY/VALUE spelling should generate identically to colon syntax:\n  want: %s\n  got:  %s", sql, got)
	}
}

// TestJSONObjectAggPostgresRename pins the postgres-only JSONObjectAgg spelling: postgres renders
// exp.JSONObjectAgg as JSON_OBJECT_AGG (generators/postgres.py:389), while base/mysql keep
// JSON_OBJECTAGG. Verified against the pinned oracle.
func TestJSONObjectAggPostgresRename(t *testing.T) {
	expression := parseOneDialect(t, "JSON_OBJECTAGG('a': 1)", "postgres")
	if expression.Kind() != exp.KindJSONObjectAgg {
		t.Fatalf("postgres JSON_OBJECTAGG kind = %v, want JSONObjectAgg", expression.Kind())
	}
	got, err := generateSQL(t, expression, "postgres")
	if err != nil {
		t.Fatalf("generate error: %v", err)
	}
	if want := "JSON_OBJECT_AGG('a': 1)"; got != want {
		t.Fatalf("postgres rename:\n  want: %s\n  got:  %s", want, got)
	}
}

// TestJSONValueMySQLRoundTrip covers testdata/dialect_identity.jsonl:244-249: MySQL's
// JSON_OBJECT comma-syntax (JSON_KEY_VALUE_PAIR_SEP=",", generators/mysql.py:144) and
// JSON_VALUE's RETURNING <type> [DEFAULT <expr>|<value>] ON EMPTY/ON ERROR on_condition
// trailer (with EMPTY always rendered before ERROR - ON_CONDITION_EMPTY_BEFORE_ERROR is
// true for base/mysql/postgres alike, dialect.py:654).
func TestJSONValueMySQLRoundTrip(t *testing.T) {
	cases := []string{
		"SELECT JSON_OBJECT('id', 87, 'name', 'carrot')",
		`SELECT JSON_VALUE('{"item": "shoes", "price": "49.95"}', '$.price' RETURNING DECIMAL(4, 2) DEFAULT 1 ON EMPTY DEFAULT 1 ON ERROR) AS price`,
		`SELECT JSON_VALUE('{"item": "shoes", "price": "49.95"}', '$.price' RETURNING DECIMAL(4, 2) ERROR ON EMPTY ERROR ON ERROR) AS price`,
		`SELECT JSON_VALUE('{"item": "shoes", "price": "49.95"}', '$.price' RETURNING DECIMAL(4, 2) NULL ON EMPTY NULL ON ERROR) AS price`,
		`SELECT JSON_VALUE('{"item": "shoes", "price": "49.95"}', '$.price' RETURNING DECIMAL(4, 2))`,
		`SELECT JSON_VALUE('{"item": "shoes", "price": "49.95"}', '$.price')`,
	}
	for _, sql := range cases {
		expression := parseOneDialect(t, sql, "mysql")
		got, err := generateSQL(t, expression, "mysql")
		if err != nil {
			t.Fatalf("generate(%q) error: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("round-trip mismatch:\n  sql:  %s\n  want: %s\n  got:  %s", sql, sql, got)
		}
	}
}

// TestJSONValueDialectGate checks the mysql-only FUNCTION_PARSERS gate added to
// parseFunctionCall: base/postgres have no _parse_json_value entry upstream (only
// parsers/mysql.py:161 registers one among base/MySQL/Postgres), so JSON_VALUE(...) falls
// through to a plain Anonymous call there instead of building a JSONValue node.
func TestJSONValueDialectGate(t *testing.T) {
	for _, dialect := range []string{"", "postgres"} {
		expression := parseOneDialect(t, "SELECT JSON_VALUE(x, '$.a')", dialect)
		projection := expression.Expressions()[0]
		if projection.Kind() != exp.KindAnonymous {
			t.Fatalf("[%s] JSON_VALUE kind = %v, want Anonymous (JSONValue is mysql-only):\n%s", dialect, projection.Kind(), projection.ToS())
		}
	}

	expression := parseOneDialect(t, "SELECT JSON_VALUE(x, '$.a')", "mysql")
	projection := expression.Expressions()[0]
	if projection.Kind() != exp.KindJSONValue {
		t.Fatalf("[mysql] JSON_VALUE kind = %v, want JSONValue:\n%s", projection.Kind(), projection.ToS())
	}
}

// TestOnConditionEmptyBeforeError pins the hardcoded ON_CONDITION_EMPTY_BEFORE_ERROR=true
// ordering (dialect.py:654) directly against the OnCondition node, independent of the
// JSON_VALUE wrapper.
func TestOnConditionEmptyBeforeError(t *testing.T) {
	sql := `SELECT JSON_VALUE(x, '$.a' NULL ON EMPTY ERROR ON ERROR)`
	expression := parseOneDialect(t, sql, "mysql")
	projection := expression.Expressions()[0]
	onCondition := exprArg(t, projection, "on_condition")
	if onCondition.Kind() != exp.KindOnCondition {
		t.Fatalf("on_condition kind = %v, want OnCondition:\n%s", onCondition.Kind(), projection.ToS())
	}
	got, err := generateSQL(t, expression, "mysql")
	if err != nil {
		t.Fatalf("generate error: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip mismatch:\n  sql:  %s\n  got:  %s", sql, got)
	}
}
