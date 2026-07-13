package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestXMLElement ports _parse_xml_element (parser.py:7977-7992) plus test_postgres.py:
// 1687-1698. XMLATTRIBUTES/XMLCOMMENT aren't registered functions (per the CONTRACT), so they
// fall through to the generic Anonymous alias-args path (parseFunctionCall, parser.go:1910).
func TestXMLElement(t *testing.T) {
	root := parseOneDialect(t, "SELECT XMLELEMENT(NAME foo, XMLATTRIBUTES('xyz' AS bar))", "postgres")
	xmlElement := root.Expressions()[0]
	if xmlElement.Kind() != exp.KindXMLElement {
		t.Fatalf("kind = %v, want XMLElement:\n%s", xmlElement.Kind(), root.ToS())
	}
	if xmlElement.Arg("evalname") != nil {
		t.Fatalf("evalname should be unset for NAME form:\n%s", root.ToS())
	}
	this := xmlElement.This()
	if this == nil || this.Kind() != exp.KindIdentifier || this.Name() != "foo" {
		t.Fatalf("this should be Identifier(foo):\n%s", root.ToS())
	}
	args := expressionsForArg(xmlElement, "expressions")
	if len(args) != 1 || args[0].Kind() != exp.KindAnonymous || args[0].Arg("this") != "XMLATTRIBUTES" {
		t.Fatalf("expressions should be one Anonymous XMLATTRIBUTES call:\n%s", root.ToS())
	}
	attr := expressionsForArg(args[0], "expressions")
	if len(attr) != 1 || attr[0].Kind() != exp.KindAlias {
		t.Fatalf("XMLATTRIBUTES arg should be an Alias:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := "SELECT XMLELEMENT(NAME foo, XMLATTRIBUTES('xyz' AS bar))"
	if got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestXMLElementEvalname covers the EVALNAME branch (`this = self._parse_bitwise()`, distinct
// from the NAME branch's `self._parse_id_var()`), and nested XMLELEMENT/XMLCOMMENT args -
// mirrors testdata/parity_gaps.txt:201.
func TestXMLElementEvalname(t *testing.T) {
	sql := "SELECT XMLELEMENT(EVALNAME 'foo' || 'bar')"
	root := parseOneDialect(t, sql, "postgres")
	xmlElement := root.Expressions()[0]
	if xmlElement.Arg("evalname") != true {
		t.Fatalf("evalname should be true:\n%s", root.ToS())
	}
	this := xmlElement.This()
	if this == nil || this.Kind() != exp.KindDPipe {
		t.Fatalf("EVALNAME this should parse as a bitwise expr (DPipe), got %v:\n%s", this.Kind(), root.ToS())
	}

	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}

	nested := parseOneDialect(t, "SELECT XMLELEMENT(NAME foo, XMLELEMENT(NAME abc), XMLCOMMENT('test'))", "postgres")
	outer := nested.Expressions()[0]
	nestedArgs := expressionsForArg(outer, "expressions")
	if len(nestedArgs) != 2 {
		t.Fatalf("expected 2 nested args:\n%s", nested.ToS())
	}
	if nestedArgs[0].Kind() != exp.KindXMLElement {
		t.Fatalf("first nested arg should be XMLElement:\n%s", nested.ToS())
	}
	if nestedArgs[1].Kind() != exp.KindAnonymous || nestedArgs[1].Arg("this") != "XMLCOMMENT" {
		t.Fatalf("second nested arg should be Anonymous XMLCOMMENT:\n%s", nested.ToS())
	}
}

// TestXMLTable ports _parse_xml_table (parser.py:7994-8019) and test_postgres.py:119-123: a
// PASSING clause plus per-column PATH constraints (CONSTRAINT_PARSERS["PATH"], parser.py:1391).
func TestXMLTable(t *testing.T) {
	sql := "SELECT id, name FROM xml_data AS t, XMLTABLE('/root/user' PASSING t.xml COLUMNS id INT PATH '@id', name TEXT PATH 'name/text()') AS x"
	root := parseOneDialect(t, sql, "postgres")
	joins := expressionsForArg(root, "joins")
	if len(joins) != 1 {
		t.Fatalf("join count = %d, want 1:\n%s", len(joins), root.ToS())
	}
	table := joins[0].This()
	if table == nil || table.Kind() != exp.KindTable {
		t.Fatalf("join.this should be Table:\n%s", root.ToS())
	}
	xmlTable := table.This()
	if xmlTable == nil || xmlTable.Kind() != exp.KindXMLTable {
		t.Fatalf("table.this should be XMLTable, got %v:\n%s", table.Kind(), root.ToS())
	}
	if this := xmlTable.This(); this == nil || this.Name() != "/root/user" {
		t.Fatalf("XMLTable.this should be the xpath literal:\n%s", root.ToS())
	}
	passing := expressionsForArg(xmlTable, "passing")
	if len(passing) != 1 || passing[0].Kind() != exp.KindColumn {
		t.Fatalf("passing should be one Column:\n%s", root.ToS())
	}
	columns := expressionsForArg(xmlTable, "columns")
	if len(columns) != 2 {
		t.Fatalf("column count = %d, want 2:\n%s", len(columns), root.ToS())
	}
	for i, want := range []struct{ name, path string }{{"id", "@id"}, {"name", "name/text()"}} {
		col := columns[i]
		if col.Kind() != exp.KindColumnDef || col.This() == nil || col.This().Name() != want.name {
			t.Fatalf("column %d should be ColumnDef(%s), got %#v:\n%s", i, want.name, col, root.ToS())
		}
		constraints := expressionsForArg(col, "constraints")
		if len(constraints) != 1 || constraints[0].Kind() != exp.KindColumnConstraint {
			t.Fatalf("column %d should have one ColumnConstraint:\n%s", i, root.ToS())
		}
		kind := constraints[0].Arg("kind")
		kindExpr, ok := kind.(exp.Expression)
		if !ok || kindExpr.Kind() != exp.KindPathColumnConstraint || kindExpr.This() == nil || kindExpr.This().Name() != want.path {
			t.Fatalf("column %d constraint kind should be PathColumnConstraint(%s), got %#v:\n%s", i, want.path, kind, root.ToS())
		}
	}

	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}

// TestXMLTableNamespaces ports the XMLNAMESPACES(...) prefix (_parse_xml_namespace,
// parser.py:8021-8033) and test_postgres.py:121-123.
func TestXMLTableNamespaces(t *testing.T) {
	sql := "SELECT id, value FROM xml_content AS t, XMLTABLE(XMLNAMESPACES('http://example.com/ns1' AS ns1, 'http://example.com/ns2' AS ns2), '/root/data' PASSING t.xml COLUMNS id INT PATH '@ns1:id', value TEXT PATH 'ns2:value/text()') AS x"
	root := parseOneDialect(t, sql, "postgres")
	xmlTable := expressionsForArg(root, "joins")[0].This().This()
	if xmlTable == nil || xmlTable.Kind() != exp.KindXMLTable {
		t.Fatalf("expected XMLTable:\n%s", root.ToS())
	}
	namespaces := expressionsForArg(xmlTable, "namespaces")
	if len(namespaces) != 2 {
		t.Fatalf("namespace count = %d, want 2:\n%s", len(namespaces), root.ToS())
	}
	for i, want := range []string{"ns1", "ns2"} {
		ns := namespaces[i]
		if ns.Kind() != exp.KindXMLNamespace {
			t.Fatalf("namespace %d should be XMLNamespace:\n%s", i, root.ToS())
		}
		alias := ns.This()
		if alias == nil || alias.Kind() != exp.KindAlias {
			t.Fatalf("namespace %d this should be an Alias (prefix AS <uri>):\n%s", i, root.ToS())
		}
		if aliasID := alias.Arg("alias"); aliasID == nil || aliasID.(exp.Expression).Name() != want {
			t.Fatalf("namespace %d alias should be %q:\n%s", i, want, root.ToS())
		}
	}

	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}

// TestXMLTableDefaultNamespace covers the DEFAULT branch of _parse_xml_namespace
// (`if self._match(TokenType.DEFAULT): uri = self._parse_string()`), which has no alias so
// xmlnamespace_sql renders it as `DEFAULT '<uri>'` (generator.py:5975-5977).
func TestXMLTableDefaultNamespace(t *testing.T) {
	sql := "SELECT * FROM XMLTABLE(XMLNAMESPACES(DEFAULT 'http://example.com/ns'), '/root' COLUMNS id INT) AS x"
	root := parseOneDialect(t, sql, "postgres")
	xmlTable := exprArg(t, root, "from_").This().This()
	namespaces := expressionsForArg(xmlTable, "namespaces")
	if len(namespaces) != 1 || namespaces[0].This() == nil || namespaces[0].This().Kind() == exp.KindAlias {
		t.Fatalf("DEFAULT namespace should not carry an Alias:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}

// TestXMLTableReturningSequenceByRef covers the by_ref flag (`RETURNING SEQUENCE BY REF`).
func TestXMLTableReturningSequenceByRef(t *testing.T) {
	sql := "SELECT * FROM XMLTABLE('/root' RETURNING SEQUENCE BY REF COLUMNS id INT) AS x"
	root := parseOneDialect(t, sql, "postgres")
	xmlTable := exprArg(t, root, "from_").This().This()
	if xmlTable.Arg("by_ref") != true {
		t.Fatalf("by_ref should be true:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}
