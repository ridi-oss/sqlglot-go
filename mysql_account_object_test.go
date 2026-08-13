package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// MySQL account-management statements — CREATE/ALTER/DROP {USER|ROLE} — are a grammar extension beyond
// upstream (which leaves them as a raw Command). sqlglot-go structures the ROOT as Create/Alter/Drop
// carrying the object type as the canonical `kind` arg ("USER"/"ROLE", same discriminator TABLE uses),
// while preserving the unmodeled body verbatim so it round-trips exactly as the plain Command fallback
// does. Registered in testdata/upstream_extensions.jsonl; see DEVIATIONS "Grammar extensions beyond upstream".
func TestMySQLCreateAlterDropUserRole(t *testing.T) {
	cases := []struct {
		sql        string
		wantKind   exp.Kind
		wantObject string
	}{
		{"CREATE USER 'u'@'h'", exp.KindCreate, "USER"}, // minimal: no IDENTIFIED clause
		{"CREATE USER 'u'@'h' IDENTIFIED BY RANDOM PASSWORD", exp.KindCreate, "USER"},
		{"CREATE  USER  'u'@'h'", exp.KindCreate, "USER"}, // multi-space preserved verbatim
		{"CREATE USER IF NOT EXISTS 'u'@'h' IDENTIFIED BY 'x'", exp.KindCreate, "USER"},
		{"CREATE USER u IDENTIFIED WITH mysql_native_password BY 'p' REQUIRE SSL", exp.KindCreate, "USER"},
		{"CREATE ROLE 'r'", exp.KindCreate, "ROLE"},
		{"CREATE ROLE IF NOT EXISTS 'a', 'b'", exp.KindCreate, "ROLE"},
		{"ALTER USER 'u'@'h' IDENTIFIED BY 'y'", exp.KindAlter, "USER"},
		{"ALTER USER IF EXISTS 'u'@'h' ACCOUNT LOCK", exp.KindAlter, "USER"},
		{"DROP USER 'u'@'h'", exp.KindDrop, "USER"},
		{"DROP USER IF EXISTS 'a'@'h', 'b'@'h'", exp.KindDrop, "USER"},
		{"DROP ROLE 'r'", exp.KindDrop, "ROLE"},
		{"DROP ROLE IF EXISTS 'a', 'b'", exp.KindDrop, "ROLE"},
	}
	for _, c := range cases {
		e := assertMySQLKindRoundTrip(t, c.sql, c.wantKind)
		// The consumer classifies by root Kind + the canonical `kind` arg (read via Text("kind"), the
		// same arg + casing as "TABLE"). Lock the exact object string.
		if got := e.Text("kind"); got != c.wantObject {
			t.Fatalf("%q: kind arg = %q, want %q", c.sql, got, c.wantObject)
		}
		// AST shape contract: the unmodeled body is a Command child — held as `this` for Create/Drop and
		// as the sole `actions` element for Alter. Storing raw text directly in `this` must not pass.
		body := e.This()
		if c.wantKind == exp.KindAlter {
			acts, _ := e.Arg("actions").([]exp.Expression)
			if len(acts) != 1 {
				t.Fatalf("%q: Alter.actions = %d elements, want 1", c.sql, len(acts))
			}
			body = acts[0]
		}
		if body == nil || body.Kind() != exp.KindCommand {
			t.Fatalf("%q: body child kind = %v, want Command", c.sql, exp.ClassName(exp.KindCommand))
		}
	}
}

// Statements the extension deliberately does NOT structure stay a raw Command (fail-closed). Covers the
// agreed out-of-scope forms, object types the port does not model, and the boundary cases: a quoted
// string object (`'USER'`), an ICEBERG prefix, a bare object with no body, and non-account CREATE
// prefixes (OR REPLACE) / the non-existent ALTER ROLE / a DROP prefix.
func TestMySQLAccountObjectUnstructuredStayCommand(t *testing.T) {
	for _, sql := range []string{
		"RENAME USER 'a'@'h' TO 'b'@'h'",
		"CREATE EVENT e ON SCHEDULE EVERY 1 HOUR DO SET @x = 1",
		"DROP EVENT e",
		"CREATE 'USER' x",      // string-literal object, not the keyword
		"ALTER ICEBERG USER u", // consumed ICEBERG prefix rules out the account form
		"CREATE USER",          // no body
		"ALTER USER",           // no body
		"DROP ROLE",            // no body
		"CREATE OR REPLACE USER u",
		"ALTER ROLE r", // MySQL has no ALTER ROLE
		"DROP TEMPORARY USER u",
	} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		if e.Kind() != exp.KindCommand {
			t.Fatalf("%q kind = %v, want Command", sql, exp.ClassName(e.Kind()))
		}
	}
}

// The structuring is MySQL-only: Postgres CREATE USER/ROLE keeps upstream's Command fallback (the
// MySQL access-control consumer is the only one that needs the account discriminator).
func TestAccountObjectStructuringIsMySQLOnly(t *testing.T) {
	for _, sql := range []string{"CREATE USER u", "CREATE ROLE r", "DROP USER u"} {
		e, err := sqlglot.ParseOne(sql, "postgres")
		if err != nil {
			t.Fatalf("parse %q (postgres): %v", sql, err)
		}
		if e.Kind() != exp.KindCommand {
			t.Fatalf("postgres %q kind = %v, want Command (MySQL-only extension)", sql, exp.ClassName(e.Kind()))
		}
	}
}

// A table (or other object) literally named `user`/`role`, and the schema-object CREATE/DROP forms,
// must not be mis-detected as the account extension: the object token is matched only unquoted at
// statement head, and real Create/Alter/Drop keep their own kind.
func TestAccountObjectDoesNotShadowTableForms(t *testing.T) {
	for _, c := range []struct {
		sql, wantObject string
		kind            exp.Kind
	}{
		{"CREATE TABLE t (a INT)", "TABLE", exp.KindCreate},
		{"DROP TABLE t", "TABLE", exp.KindDrop},
		{"ALTER TABLE t ADD COLUMN c INT", "TABLE", exp.KindAlter},
		{"DROP TABLE `user`", "TABLE", exp.KindDrop},
	} {
		e := assertMySQLKindRoundTrip(t, c.sql, c.kind)
		if got := e.Text("kind"); got != c.wantObject {
			t.Fatalf("%q: kind arg = %q, want %q", c.sql, got, c.wantObject)
		}
	}
}
