package generator_test

// Round-trip checks for xmlElementSQL/xmlTableSQL/xmlNamespaceSQL/pathColumnConstraintSQL
// (generator/xml.go), ported from xmlelement_sql/xmltable_sql/xmlnamespace_sql
// (generator.py:5873-5876,5964-5977) and the exp.PathColumnConstraint TRANSFORMS lambda
// (generator.py:233). Cases are drawn from testdata/parity_gaps.txt (postgres XMLELEMENT/
// XMLTABLE entries, :198-203,227-228) and test_postgres.py:119-123,1687-1698.

import "testing"

func TestXMLElementSQL(t *testing.T) {
	cases := []string{
		"SELECT XMLELEMENT(NAME foo)",
		"SELECT XMLELEMENT(NAME foo, XMLATTRIBUTES('xyz' AS bar))",
		"SELECT XMLELEMENT(NAME test, XMLATTRIBUTES(a, b)) FROM test",
		"SELECT XMLELEMENT(NAME foo, XMLATTRIBUTES(CURRENT_DATE AS bar), 'cont', 'ent')",
		`SELECT XMLELEMENT(NAME "foo$bar", XMLATTRIBUTES('xyz' AS "a&b"))`,
		"SELECT XMLELEMENT(NAME foo, XMLATTRIBUTES('xyz' AS bar), XMLELEMENT(NAME abc), XMLCOMMENT('test'), XMLELEMENT(NAME xyz))",
	}
	for _, sql := range cases {
		if got := roundTrip(t, "postgres", sql); got != sql {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", sql, got, sql)
		}
	}
}

// TestXMLElementNameForms pins the two non-identity XMLELEMENT NAME edge cases against the pinned
// oracle: a string-literal name folds to a quoted identifier (_parse_id_var any_token=True,
// parser.py:7985), and a FOR ORDINALITY column marker round-trips lossily (columndef_sql does not
// render the ordinality flag, generator.py:1169).
func TestXMLElementNameForms(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT XMLELEMENT(NAME 'foo', 1)", `SELECT XMLELEMENT(NAME "foo", 1)`},
		{
			"SELECT * FROM XMLTABLE('/a' COLUMNS ord FOR ORDINALITY, id INT PATH '@id')",
			"SELECT * FROM XMLTABLE('/a' COLUMNS ord, id INT PATH '@id')",
		},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

func TestXMLTableSQL(t *testing.T) {
	cases := []string{
		"SELECT id, name FROM xml_data AS t, XMLTABLE('/root/user' PASSING t.xml COLUMNS id INT PATH '@id', name TEXT PATH 'name/text()') AS x",
		"SELECT id, value FROM xml_content AS t, XMLTABLE(XMLNAMESPACES('http://example.com/ns1' AS ns1, 'http://example.com/ns2' AS ns2), '/root/data' PASSING t.xml COLUMNS id INT PATH '@ns1:id', value TEXT PATH 'ns2:value/text()') AS x",
		"SELECT * FROM XMLTABLE('/root' COLUMNS id INT) AS x",
		"SELECT * FROM XMLTABLE('/root' RETURNING SEQUENCE BY REF COLUMNS id INT) AS x",
		"SELECT * FROM XMLTABLE(XMLNAMESPACES(DEFAULT 'http://example.com/ns'), '/root' COLUMNS id INT) AS x",
		// PATH is a normal column constraint (shared constraintParsers "PATH" entry), so it
		// composes with following constraints instead of terminating the column def. Verified
		// against the pinned oracle.
		"SELECT * FROM XMLTABLE('/root' COLUMNS id INT PATH '@id' NOT NULL) AS x",
		"SELECT * FROM XMLTABLE('/root' COLUMNS id INT PATH '@id' DEFAULT 0) AS x",
		"SELECT * FROM XMLTABLE('/root' COLUMNS id TEXT PATH '@id' DEFAULT 'x') AS x",
	}
	for _, sql := range cases {
		if got := roundTrip(t, "postgres", sql); got != sql {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", sql, got, sql)
		}
	}
}
