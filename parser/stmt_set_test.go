package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseSetBase ports the base (non-mysql) structural cases: SET x = 1, SET GLOBAL
// var = value, SET LOCAL var = value, and a multi-item SET x = 1, y = 2. These all close
// testdata/parity_gaps.txt's base "SET ..." lines.
func TestParseSetBase(t *testing.T) {
	for _, sql := range []string{
		"SET x = 1",
		"SET GLOBAL variable = value",
		"SET LOCAL variable = value",
		"SET variable = value",
	} {
		set := parseOneDialect(t, sql, "")
		if set.Kind() != exp.KindSet {
			t.Fatalf("%q: kind = %v, want Set:\n%s", sql, set.Kind(), set.ToS())
		}
		items := expressionsForArg(set, "expressions")
		if len(items) != 1 || items[0].Kind() != exp.KindSetItem {
			t.Fatalf("%q: expressions = %#v, want a single SetItem:\n%s", sql, items, set.ToS())
		}
		if eq := exprArg(t, items[0], "this"); eq.Kind() != exp.KindEQ {
			t.Fatalf("%q: item.this should be EQ:\n%s", sql, set.ToS())
		}
		got, err := generateSQL(t, set, "")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}

	// SET x = 1, y = 2: two comma-separated SetItems (test_mysql.py:1478-1479).
	set := parseOneDialect(t, "SET x = 1, y = 2", "")
	items := expressionsForArg(set, "expressions")
	if len(items) != 2 {
		t.Fatalf("expressions = %d items, want 2:\n%s", len(items), set.ToS())
	}
	got, err := generateSQL(t, set, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "SET x = 1, y = 2" {
		t.Fatalf("round-trip = %q", got)
	}
}

// TestParseSetMySQL ports the structural (non-@/@@) cases of test_mysql.py:1452
// test_set_variable, plus the CHARACTER SET/CHARSET/NAMES corpus gaps
// (testdata/parity_gaps.txt). The @/@@ user/system-variable forms are covered by
// TestParseSetMySQLCorpus below.
func TestParseSetMySQL(t *testing.T) {
	// SET SESSION x = 1 -> kind "SESSION", this = EQ(x, 1).
	set := parseOneDialect(t, "SET SESSION x = 1", "mysql")
	items := expressionsForArg(set, "expressions")
	if len(items) != 1 {
		t.Fatalf("expressions = %#v, want 1 item:\n%s", items, set.ToS())
	}
	item0 := items[0]
	if item0.Text("kind") != "SESSION" {
		t.Fatalf("kind = %q, want SESSION:\n%s", item0.Text("kind"), set.ToS())
	}
	eq := exprArg(t, item0, "this")
	if eq.Kind() != exp.KindEQ {
		t.Fatalf("item.this should be EQ:\n%s", set.ToS())
	}
	if left := exprArg(t, eq, "this"); left.Name() != "x" {
		t.Fatalf("left.name = %q, want x:\n%s", left.Name(), set.ToS())
	}
	if right := exprArg(t, eq, "expression"); right.Name() != "1" {
		t.Fatalf("right.name = %q, want 1:\n%s", right.Name(), set.ToS())
	}
	got, err := generateSQL(t, set, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "SET SESSION x = 1" {
		t.Fatalf("round-trip = %q", got)
	}

	// SET NAMES 'charset_name' COLLATE 'collation_name'.
	set = parseOneDialect(t, "SET NAMES 'charset_name' COLLATE 'collation_name'", "mysql")
	items = expressionsForArg(set, "expressions")
	if len(items) != 1 {
		t.Fatalf("expressions = %#v, want 1 item:\n%s", items, set.ToS())
	}
	item0 = items[0]
	if item0.Text("kind") != "NAMES" {
		t.Fatalf("kind = %q, want NAMES:\n%s", item0.Text("kind"), set.ToS())
	}
	if item0.Name() != "charset_name" {
		t.Fatalf("name = %q, want charset_name:\n%s", item0.Name(), set.ToS())
	}
	if item0.Text("collate") != "collation_name" {
		t.Fatalf("collate = %q, want collation_name:\n%s", item0.Text("collate"), set.ToS())
	}
	got, err = generateSQL(t, set, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "SET NAMES 'charset_name' COLLATE 'collation_name'" {
		t.Fatalf("round-trip = %q", got)
	}

	// SET CHARSET DEFAULT -> kind normalizes to "CHARACTER SET", this = Var(DEFAULT).
	set = parseOneDialect(t, "SET CHARSET DEFAULT", "mysql")
	items = expressionsForArg(set, "expressions")
	if len(items) != 1 {
		t.Fatalf("expressions = %#v, want 1 item:\n%s", items, set.ToS())
	}
	item0 = items[0]
	if item0.Text("kind") != "CHARACTER SET" {
		t.Fatalf("kind = %q, want \"CHARACTER SET\":\n%s", item0.Text("kind"), set.ToS())
	}
	this := exprArg(t, item0, "this")
	if this.Name() != "DEFAULT" {
		t.Fatalf("this.name = %q, want DEFAULT:\n%s", this.Name(), set.ToS())
	}
	// Not a round-trip: CHARSET always normalizes to the "CHARACTER SET" kind on
	// generation (test_mysql.py:1473-1476 only checks structural fields, not identity;
	// the corpus's own gap list only exercises the full "CHARACTER SET" spelling).
	got, err = generateSQL(t, set, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "SET CHARACTER SET DEFAULT" {
		t.Fatalf("generate = %q, want %q", got, "SET CHARACTER SET DEFAULT")
	}
}

// TestParseSetMySQLCorpus round-trips the remaining structural mysql SET gaps from
// testdata/parity_gaps.txt (PERSIST/PERSIST_ONLY/CHARACTER SET/NAMES unquoted forms/
// TRANSACTION characteristics), plus the @@ SessionParameter forms.
func TestParseSetMySQLCorpus(t *testing.T) {
	structured := []string{
		"SET CHARACTER SET 'utf8'",
		"SET CHARACTER SET DEFAULT",
		"SET CHARACTER SET utf8",
		"SET GLOBAL TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ WRITE",
		"SET GLOBAL TRANSACTION ISOLATION LEVEL SERIALIZABLE",
		"SET GLOBAL max_connections = 1000",
		"SET GLOBAL max_connections = 1000, sort_buffer_size = 1000000",
		"SET GLOBAL sort_buffer_size = 1000000, SESSION sort_buffer_size = 1000000",
		"SET LOCAL sql_mode = 'TRADITIONAL'",
		"SET NAMES 'utf8'",
		"SET NAMES 'utf8' COLLATE 'utf8_unicode_ci'",
		"SET NAMES DEFAULT",
		"SET NAMES utf8 COLLATE utf8_unicode_ci",
		"SET PERSIST max_connections = 1000",
		"SET PERSIST_ONLY back_log = 100",
		"SET SESSION sql_mode = 'TRADITIONAL'",
		"SET TRANSACTION READ ONLY",
		"SET sql_mode = 'TRADITIONAL'",
	}
	for _, sql := range structured {
		set := parseOneDialect(t, sql, "mysql")
		if set.Kind() != exp.KindSet {
			t.Fatalf("%q: kind = %v, want Set:\n%s", sql, set.Kind(), set.ToS())
		}
		got, err := generateSQL(t, set, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}

	// @@ system-variable forms build a structured SessionParameter (residual-tail cluster:
	// primaryParsers[SESSION_PARAMETER] -> parseSessionParameter, parser.py:7168-7176) nested
	// inside the ordinary SET/SetItem/EQ shape, verified against the pinned oracle.
	sessionParameterForms := []string{
		"SET @@GLOBAL.max_connections = 1000",
		"SET @@GLOBAL.sort_buffer_size = 1000000, @@LOCAL.sort_buffer_size = 1000000",
		"SET @@PERSIST.max_connections = 1000",
	}
	for _, sql := range sessionParameterForms {
		set := parseOneDialect(t, sql, "mysql")
		if set.Kind() != exp.KindSet {
			t.Fatalf("%q: kind = %v, want Set:\n%s", sql, set.Kind(), set.ToS())
		}
		got, err := generateSQL(t, set, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}

	// @ user-variable forms parse into a Parameter node (parser.py:8586). `SET @var1 := 1`
	// is non-identity: the `:=` assignment delimiter normalizes to `=`.
	for _, tc := range []struct{ sql, want string }{
		{"SET @name = 43", "SET @name = 43"},
		{"SET @x = 1, SESSION sql_mode = ''", "SET @x = 1, SESSION sql_mode = ''"},
		{"SET @var1 := 1", "SET @var1 = 1"},
	} {
		set := parseOneDialect(t, tc.sql, "mysql")
		if set.Kind() != exp.KindSet {
			t.Fatalf("%q: kind = %v, want Set:\n%s", tc.sql, set.Kind(), set.ToS())
		}
		got, err := generateSQL(t, set, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", tc.sql, err)
		}
		if got != tc.want {
			t.Fatalf("%q: round-trip = %q, want %q", tc.sql, got, tc.want)
		}
	}
}
