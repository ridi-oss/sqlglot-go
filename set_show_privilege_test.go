package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Postgres SCOPED privileged SET forms — `SET [SESSION|LOCAL] ROLE r` and
// `SET [SESSION|LOCAL] SESSION AUTHORIZATION u` — parse to Set{SetItem{kind, scope}} with the SAME
// kind as the bare form (so a consumer reads the privilege identically), the scope preserved for a
// faithful round-trip. Pinned upstream Commands these. Verified against PostgreSQL 17.6.
func TestPostgresScopedPrivilegedSet(t *testing.T) {
	cases := []struct {
		sql, kind, scope string
	}{
		{"SET ROLE admin", "ROLE", ""},
		{"SET LOCAL ROLE admin", "ROLE", "LOCAL"},
		{"SET SESSION ROLE admin", "ROLE", "SESSION"},
		{"SET SESSION AUTHORIZATION bob", "SESSION AUTHORIZATION", ""},
		{"SET LOCAL SESSION AUTHORIZATION bob", "SESSION AUTHORIZATION", "LOCAL"},
		{"SET SESSION SESSION AUTHORIZATION bob", "SESSION AUTHORIZATION", "SESSION"},
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Errorf("%q: parse: %v", tc.sql, err)
			continue
		}
		if e.Kind() != exp.KindSet || len(e.Expressions()) != 1 {
			t.Errorf("%q: want single-item Set\n%s", tc.sql, e.ToS())
			continue
		}
		item := e.Expressions()[0]
		if got := item.Text("kind"); got != tc.kind {
			t.Errorf("%q: SetItem kind = %q, want %q", tc.sql, got, tc.kind)
		}
		if got := item.Text("scope"); got != tc.scope {
			t.Errorf("%q: SetItem scope = %q, want %q", tc.sql, got, tc.scope)
		}
		if out, _ := sqlglot.Generate(e, "postgres", generator.Options{}); out != tc.sql {
			t.Errorf("%q: round-trip = %q", tc.sql, out)
		}
	}
}

// SECURITY: the GUC-alias assignment forms (`SET role = x`, where `role`/`session_authorization` are
// ordinary GUCs) must NOT be captured as the privileged ROLE/SESSION AUTHORIZATION forms — they stay
// EQ assignments so a consumer can read the LHS var name. The disambiguator is the assignment
// delimiter following the keyword. The BARE (unscoped) `SET role = x` is load-bearing: `role` is a
// dispatch key, so without the delimiter check it would fail closed to a raw Command — and a consumer
// whose Command-SET fallback is a benign passthrough would then ALLOW the role change (a privilege
// escalation). It must parse to a readable EQ. Verified against PostgreSQL 17.6.
func TestPostgresScopedSetGUCAliasStaysAssignment(t *testing.T) {
	for _, tc := range []struct{ sql, lhs string }{
		{"SET role = attacker", "role"},         // BARE unscoped — must be EQ, not Command
		{"SET role TO attacker", "role"},        // ...with TO
		{"SET ROLE = attacker", "ROLE"},         // ...uppercase keyword (LHS keeps source case)
		{"SET SESSION role = attacker", "role"}, // scoped
		{"SET LOCAL role = attacker", "role"},
		{"SET SESSION role TO attacker", "role"},
	} {
		e, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Errorf("%q: parse: %v", tc.sql, err)
			continue
		}
		if e.Kind() != exp.KindSet {
			t.Errorf("%q: Kind = %s, want Set (a bare Command here is a privesc bypass)\n%s", tc.sql, exp.ClassName(e.Kind()), e.ToS())
			continue
		}
		item := e.Expressions()[0]
		assign, ok := item.Arg("this").(exp.Expression)
		if !ok || assign.Kind() != exp.KindEQ {
			t.Errorf("%q: SetItem.this must be an EQ assignment (GUC-alias reachable)\n%s", tc.sql, e.ToS())
			continue
		}
		if lhs, ok := assign.Arg("this").(exp.Expression); !ok || lhs.Name() != tc.lhs {
			t.Errorf("%q: EQ LHS var must be readable as %q\n%s", tc.sql, tc.lhs, e.ToS())
		}
	}
}

// SECURITY: a role/user name that text-collides with an assignment delimiter keyword — e.g. a role
// literally named "to" (`SET SESSION ROLE "to"`, valid Postgres) — must still classify as the
// privileged ROLE form, not be dropped. The delimiter peek keys on token TYPE, so a quoted IDENTIFIER
// whose text is "to"/"=" is not mistaken for the TO/= delimiter. A trailing garbage token fails
// closed to Command rather than silently misclassifying and discarding the role.
func TestPostgresScopedSetDelimiterCollision(t *testing.T) {
	for _, sql := range []string{`SET SESSION ROLE "to"`, `SET LOCAL ROLE "to"`, `SET SESSION ROLE "="`} {
		e, err := sqlglot.ParseOne(sql, "postgres")
		if err != nil {
			t.Errorf("%q: parse: %v", sql, err)
			continue
		}
		if e.Kind() != exp.KindSet || e.Expressions()[0].Text("kind") != "ROLE" {
			t.Errorf("%q: want SetItem kind=ROLE (a role named after a delimiter word)\n%s", sql, e.ToS())
		}
		if out, _ := sqlglot.Generate(e, "postgres", generator.Options{}); out != sql {
			t.Errorf("%q: round-trip = %q", sql, out)
		}
	}
	// Trailing garbage after a collision-named role must NOT silently misclassify — fail closed.
	if e, err := sqlglot.ParseOne(`SET SESSION ROLE "to" extra`, "postgres"); err == nil && e.Kind() != exp.KindCommand {
		t.Errorf("`SET SESSION ROLE \"to\" extra`: want Command (fail-closed), got %s", exp.ClassName(e.Kind()))
	}
	// A *quoted* keyword position (`SET SESSION "role" x`) is NOT the ROLE keyword — a quoted
	// identifier is never a keyword, so it must not be structured as the privileged form.
	for _, sql := range []string{`SET SESSION "role" attacker`, `SET LOCAL "role" attacker`} {
		if e, err := sqlglot.ParseOne(sql, "postgres"); err == nil && e.Kind() != exp.KindCommand {
			t.Errorf("%q: quoted keyword must not be the ROLE form; want Command, got %s", sql, exp.ClassName(e.Kind()))
		}
	}
}

// MySQL fail-closed cases the reviewers flagged: a bare (unquoted) `%` host is invalid MySQL, and
// SET PASSWORD cannot be comma-combined with other SET items.
func TestMySQLSetPasswordFailClosed(t *testing.T) {
	for _, sql := range []string{
		"SET PASSWORD FOR u@% = 'x'",        // bare unquoted % host — invalid MySQL
		"SET PASSWORD = 'y', @x = 1",        // PASSWORD cannot be comma-combined
		"SET @x = 1, PASSWORD = 'y'",        // ...in either position
		"SET PASSWORD FOR @'h' = 'x'",       // missing user name
		"SET PASSWORD FOR 'u'@'h' TO 'x'",   // TO takes only RANDOM, not a string
		"SET PASSWORD = RANDOM",             // bare RANDOM (no TO) is invalid
		"SET PASSWORD FOR 'u'@'h' = RANDOM", // = takes a string, not a bare word
	} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err == nil && e.Kind() != exp.KindCommand {
			t.Errorf("%q: want Command (fail-closed), got %s\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
		}
	}
	// CURRENT_USER() (with parens) is a valid user spec for both features.
	for _, sql := range []string{"SET PASSWORD FOR CURRENT_USER() = 'x'", "SHOW CREATE USER CURRENT_USER()"} {
		if _, err := sqlglot.ParseOne(sql, "mysql"); err != nil {
			t.Errorf("%q: CURRENT_USER() should parse: %v", sql, err)
		}
	}
}

// MySQL `SET PASSWORD [FOR user] = value` parses to Set{SetItem{kind:"PASSWORD"}} (an account
// mutation a consumer reads structurally), the user@host and value round-tripping byte-for-byte.
// Beyond pinned upstream, which Commands the FOR form. Verified valid syntax on MySQL 8.0.33.
func TestMySQLSetPassword(t *testing.T) {
	for _, sql := range []string{
		"SET PASSWORD = 'x'",
		"SET PASSWORD FOR 'u'@'h' = 'x'",
		"SET PASSWORD FOR 'admin'@'%' = 'secret'",
		"SET PASSWORD FOR u = 'x'",
		"SET PASSWORD FOR 'u'@'h' TO RANDOM",
		"SET PASSWORD TO RANDOM",
	} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Errorf("%q: parse: %v", sql, err)
			continue
		}
		if e.Kind() != exp.KindSet || len(e.Expressions()) != 1 {
			t.Errorf("%q: want single-item Set\n%s", sql, e.ToS())
			continue
		}
		if got := e.Expressions()[0].Text("kind"); got != "PASSWORD" {
			t.Errorf("%q: SetItem kind = %q, want PASSWORD", sql, got)
		}
		if out, _ := sqlglot.Generate(e, "mysql", generator.Options{}); out != sql {
			t.Errorf("%q: round-trip = %q", sql, out)
		}
	}
	// Malformed (no assignment) fails closed to Command.
	if e, err := sqlglot.ParseOne("SET PASSWORD", "mysql"); err == nil && e.Kind() != exp.KindCommand {
		t.Errorf("`SET PASSWORD`: want Command (fail-closed), got %s", exp.ClassName(e.Kind()))
	}
}

// MySQL `SHOW CREATE USER <user>` parses to Show{this:"CREATE USER"} (like the sibling SHOW CREATE
// * forms), the user spec (name[@host]) round-tripping. Beyond pinned upstream, which Commands it.
func TestMySQLShowCreateUser(t *testing.T) {
	for _, sql := range []string{
		"SHOW CREATE USER 'u'",
		"SHOW CREATE USER 'u'@'h'",
		"SHOW CREATE USER 'root'@'localhost'",
		"SHOW CREATE USER u",
		"SHOW CREATE USER CURRENT_USER",
	} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Errorf("%q: parse: %v", sql, err)
			continue
		}
		if e.Kind() != exp.KindShow || e.Text("this") != "CREATE USER" {
			t.Errorf("%q: want Show{this:CREATE USER}\n%s", sql, e.ToS())
		}
		if out, _ := sqlglot.Generate(e, "mysql", generator.Options{}); out != sql {
			t.Errorf("%q: round-trip = %q", sql, out)
		}
	}
}

// MySQL account-grammar over-acceptance the reviewers flagged: parseMySQLUserSpec / the PASSWORD value
// grammar must reject forms MySQL 8.0.33 itself rejects (ERROR 1064), degrading to Command rather than
// building a structured privileged node.
func TestMySQLAccountGrammarFailClosed(t *testing.T) {
	for _, sql := range []string{
		"SHOW CREATE USER foo()",                    // () valid only on CURRENT_USER
		"SET PASSWORD FOR foo() = 'x'",              // ...ditto
		"SHOW CREATE USER CURRENT_USER@'localhost'", // CURRENT_USER takes no host
		"SET PASSWORD FOR 123 = 'x'",                // a number is not a user
		"SET PASSWORD = ?",                          // placeholder is not an auth string
		"SET PASSWORD = 5",                          // number is not an auth string
		"SET PASSWORD `=` 'x'",                      // backtick-quoted `=` is not the delimiter
	} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err == nil && e.Kind() != exp.KindCommand {
			t.Errorf("%q: want Command (fail-closed), got %s\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
		}
	}
	// Valid MySQL account names/hosts must NOT be rejected (regression guard): a keyword-shaped
	// unquoted name and a numeric host are both accepted by MySQL 8.0.33.
	for _, sql := range []string{"SHOW CREATE USER session", "SHOW CREATE USER u@123", "SHOW CREATE USER 'u'@'h'"} {
		if e, err := sqlglot.ParseOne(sql, "mysql"); err != nil || e.Kind() != exp.KindShow {
			t.Errorf("%q: want Show (valid MySQL), got %v / %v", sql, kindOrErr(e, err), err)
		}
	}
}

func kindOrErr(e exp.Expression, err error) string {
	if err != nil {
		return "error"
	}
	return exp.ClassName(e.Kind())
}

// SECURITY: a quoted dispatch keyword is never an unquoted keyword — the shared findParser must not
// match it, so `SET "ROLE" x`/`SET "LOCAL" ROLE x`/`SHOW `+"`CREATE`"+` USER` (all engine-rejected) do
// NOT build a structured privileged node (they fail closed to Command), and `SET `+"`PASSWORD`"+` = x`
// parses as the ordinary variable assignment MySQL treats it as (an EQ, kind unset), not the PASSWORD
// account mutation. Verified against PostgreSQL 17.6 and MySQL 8.0.33.
func TestQuotedDispatchKeywordFailClosed(t *testing.T) {
	for _, tc := range []struct{ sql, dialect string }{
		{`SET "ROLE" admin`, "postgres"},
		{`SET "LOCAL" ROLE admin`, "postgres"},
		{`SET "SESSION" ROLE admin`, "postgres"},
		// A quoted keyword in a NON-dispatch position (the second word of a phrase, or a keyword
		// matched by matchTextSeq rather than findParser) must also fail closed — all engine-rejected.
		{`SET SESSION "AUTHORIZATION" bob`, "postgres"},
		{`SET LOCAL SESSION "AUTHORIZATION" bob`, "postgres"},
		{`SET SESSION "CHARACTERISTICS" AS TRANSACTION READ ONLY`, "postgres"},
		{`SET SESSION CHARACTERISTICS "AS" TRANSACTION READ ONLY`, "postgres"},
		{`SET SESSION "TRANSACTION" READ ONLY`, "postgres"},
		{"SHOW `CREATE` USER 'u'", "mysql"},
		{"SET PASSWORD TO `RANDOM`", "mysql"},
		{"SET PASSWORD `FOR` 'u' = 'x'", "mysql"},
	} {
		e, err := sqlglot.ParseOne(tc.sql, tc.dialect)
		if err == nil && e.Kind() != exp.KindCommand {
			t.Errorf("%q [%s]: want Command (fail-closed), got %s\n%s", tc.sql, tc.dialect, exp.ClassName(e.Kind()), e.ToS())
		}
	}
	// `SET `+"`PASSWORD`"+` = 0` is a benign variable assignment (MySQL: unknown variable at runtime),
	// so it must NOT carry the privileged kind="PASSWORD".
	e, err := sqlglot.ParseOne("SET `PASSWORD` = 0", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Kind() != exp.KindSet {
		t.Fatalf("Kind = %s, want Set", exp.ClassName(e.Kind()))
	}
	if kind := e.Expressions()[0].Text("kind"); kind == "PASSWORD" {
		t.Errorf("quoted `PASSWORD` must not be the PASSWORD account form (got kind=%q)", kind)
	}
}
