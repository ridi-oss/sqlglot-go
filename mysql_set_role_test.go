package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// MySQL `SET ROLE …` / `SET DEFAULT ROLE … TO …` are privilege operations pinned upstream Commands.
// They now parse to Set{SetItem{kind:"ROLE"|"DEFAULT ROLE"}} — so a consumer gates a MySQL role change
// like the Postgres SET ROLE form (shared kind="ROLE") instead of blanket-denying every MySQL
// Command-SET — and round-trip byte-for-byte. Verified against MySQL 8.0.33 (grammar extension; ledger
// ids mysql-set-role / mysql-set-default-role).
func TestMySQLSetRole(t *testing.T) {
	cases := []struct {
		sql, kind string
	}{
		{"SET ROLE admin", "ROLE"},
		{"SET ROLE admin, dev", "ROLE"},
		{"SET ROLE ALL", "ROLE"},
		{"SET ROLE NONE", "ROLE"},
		{"SET ROLE DEFAULT", "ROLE"},
		{"SET ROLE ALL EXCEPT admin", "ROLE"},
		{"SET ROLE ALL EXCEPT admin, dev", "ROLE"},
		{"SET ROLE 'admin'@'%'", "ROLE"},
		{"SET ROLE admin @localhost", "ROLE"}, // MySQL tolerates a space BEFORE @ (parsed); round-trips verbatim
		// nonreserved MySQL keywords are valid unquoted account names, even those the tokenizer lexes as a
		// dedicated token (BEGIN/SESSION/FORMAT) — accepted via the reserved-keyword table, not degraded.
		{"SET ROLE BEGIN", "ROLE"},
		{"SET ROLE SESSION, FORMAT", "ROLE"},
		{"SET DEFAULT ROLE admin TO BEGIN", "DEFAULT ROLE"},
		{"SET DEFAULT ROLE admin TO ridi", "DEFAULT ROLE"},
		{"SET DEFAULT ROLE NONE TO ridi", "DEFAULT ROLE"},
		{"SET DEFAULT ROLE ALL TO u1, u2", "DEFAULT ROLE"},
		{"SET DEFAULT ROLE a, b TO ridi", "DEFAULT ROLE"},
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne(tc.sql, "mysql")
		if err != nil {
			t.Errorf("%q: parse: %v", tc.sql, err)
			continue
		}
		if e.Kind() != exp.KindSet || len(e.Expressions()) != 1 {
			t.Errorf("%q: want single-item Set\n%s", tc.sql, e.ToS())
			continue
		}
		if got := e.Expressions()[0].Text("kind"); got != tc.kind {
			t.Errorf("%q: SetItem kind = %q, want %q\n%s", tc.sql, got, tc.kind, e.ToS())
		}
		if out, _ := sqlglot.Generate(e, "mysql", generator.Options{}); out != tc.sql {
			t.Errorf("%q: round-trip = %q", tc.sql, out)
		}
	}

	// A quoted keyword is a role NAME, not the keyword (matches MySQL: `SET ROLE 'ALL'` sets a role
	// literally named ALL, not the ALL form).
	e, err := sqlglot.ParseOne(`SET ROLE 'ALL'`, "mysql")
	if err != nil || e.Kind() != exp.KindSet {
		t.Fatalf(`SET ROLE 'ALL': want Set, got %v / %v`, kindOrErr(e, err), err)
	}
	item := e.Expressions()[0]
	if item.Arg("this") != nil {
		t.Errorf(`SET ROLE 'ALL': quoted 'ALL' must be a role in expressions, not the ALL keyword in this\n%s`, e.ToS())
	}

	// Malformed / bare forms fail closed to Command (a consumer denies the Command). Every case here is
	// ERROR 1064 on MySQL 8.0.33 — parsing it into a structured, regenerable role mutation would launder
	// engine-invalid SQL past a gating consumer, so it must stay a Command. (dual-review 2026-07-23:
	// Sol + Codex both flagged these three over-accept classes; all verified against the live container.)
	for _, sql := range []string{
		"SET ROLE",                  // no spec
		"SET DEFAULT ROLE admin",    // missing TO
		"SET DEFAULT ROLE admin TO", // missing users
		// (1) trailing / leading / doubled comma in the role or user list — parseCsv would silently drop
		// the dangling separator and regenerate valid SQL; the strict list fails closed instead.
		"SET ROLE admin,",
		"SET ROLE ,admin",
		"SET ROLE admin,,dev",
		"SET ROLE ALL EXCEPT, admin",
		"SET ROLE ALL EXCEPT admin,",
		"SET DEFAULT ROLE NONE TO u,",
		"SET DEFAULT ROLE a,,b TO ridi",
		// (2) ROLE / DEFAULT ROLE are standalone statements — never comma-combined with another SET item
		// in either position (the reverse-order forms also exercise the atomic-dispatch retreat).
		"SET NAMES utf8, ROLE admin",
		"SET CHARACTER SET utf8, ROLE admin",
		"SET NAMES utf8, DEFAULT ROLE admin TO ridi",
		"SET @x = 1, ROLE admin",
		"SET ROLE ALL, @x = 1",
		"SET DEFAULT ROLE NONE, @x = 1", // the lossy path: must not drop the role part and keep @x = 1
		// (3) reserved keywords / CURRENT_USER are not valid role or user names past the first alternative.
		"SET ROLE TRUE",
		"SET ROLE CURRENT_USER",
		"SET ROLE admin, NONE",
		"SET ROLE NONE, admin",
		"SET ROLE admin, ALL",
		"SET ROLE admin, DEFAULT",
		"SET DEFAULT ROLE admin TO CURRENT_USER",
		"SET DEFAULT ROLE admin TO CURRENT_USER()",
		"SET DEFAULT ROLE admin, ALL TO ridi",
		// the host must be adjacent to @ — a space AFTER @ is invalid (a space before @ is fine, above).
		"SET ROLE admin@ localhost",
		"SET ROLE admin @ localhost",
		// MySQL has no TO assignment delimiter, and SET DEFAULT ROLE has no `= value` form.
		"SET ROLE TO admin",
		"SET DEFAULT ROLE = admin",
		"SET DEFAULT ROLE := admin",
		// a trailing comma after a keyword alternative (not a role list) must fail closed, not be eaten
		// by the outer CSV and regenerated as the different valid `SET ROLE ALL`.
		"SET ROLE ALL,",
		"SET ROLE NONE,",
		"SET ROLE DEFAULT,",
		// MySQL RESERVED words are invalid as unquoted role/user names, even those this tokenizer lexes
		// as a plain VAR (TO/ACCESSIBLE/ADD) — rejected via the pinned MySQL reserved-keyword table.
		"SET ROLE TO",
		"SET ROLE ACCESSIBLE",
		"SET ROLE ADD",
		"SET ROLE ALL EXCEPT TO",
		"SET DEFAULT ROLE r TO TO",
		"SET DEFAULT ROLE r TO ACCESSIBLE",
		// nonreserved keywords MySQL's role_keyword grammar nonetheless excludes as an unquoted name
		// (ident_keywords_ambiguous_1/3): EVENT/EXECUTE/FILE/PROCESS/PROXY/RELOAD/REPLICATION/RESOURCE/
		// RESTART/SHUTDOWN/SUPER. Not in the reserved table, so caught by mysqlNonRoleNameWords.
		"SET ROLE EVENT",
		"SET ROLE SUPER",
		"SET ROLE admin, RESTART",
		"SET ROLE ALL EXCEPT EVENT, SUPER",
		"SET DEFAULT ROLE admin TO EVENT",
		"SET DEFAULT ROLE EVENT TO ridi",
		// a role/user name is a single plain-identifier word — a placeholder, hex/bit literal, operator,
		// number, or MULTIWORD keyword token (CHARACTER SET / GROUP BY) is not a name and fails closed.
		"SET ROLE ?",
		"SET ROLE x'6162'",
		"SET ROLE 0x6162",
		"SET ROLE b'01'",
		"SET ROLE CHARACTER SET",
		"SET ROLE GROUP BY",
		"SET DEFAULT ROLE r TO ?",
	} {
		if e, err := sqlglot.ParseOne(sql, "mysql"); err == nil && e.Kind() != exp.KindCommand {
			t.Errorf("%q: want Command (fail-closed), got %s\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
		}
	}

	// `SET ROLE = x` / `SET ROLE := x` is the generic assignment production — MySQL parses it as an
	// attempt to set a run-time variable literally named `role` (ERROR 1193 Unknown system variable —
	// syntactically accepted), NOT the privileged `SET ROLE <name>` form. The ROLE dispatch must fall
	// back to a plain assignment on a `=`/`:=` delimiter (mirroring the Postgres precedent), not swallow
	// it to Command — a regression the atomic dispatch introduced until the delimiter probe was added.
	// It must also NOT carry kind="ROLE" (it is a variable assignment, not a role activation).
	for _, tc := range []struct{ sql, want string }{
		{"SET ROLE = admin", "SET ROLE = admin"},
		{"SET ROLE := admin", "SET ROLE = admin"}, // := normalizes to = (systemic across SET assignments)
	} {
		e, err := sqlglot.ParseOne(tc.sql, "mysql")
		if err != nil {
			t.Errorf("%q: parse: %v", tc.sql, err)
			continue
		}
		if e.Kind() != exp.KindSet || len(e.Expressions()) != 1 {
			t.Errorf("%q: want single-item Set\n%s", tc.sql, e.ToS())
			continue
		}
		if got := e.Expressions()[0].Text("kind"); got == "ROLE" {
			t.Errorf("%q: must NOT be the privileged ROLE form (it is a variable assignment)\n%s", tc.sql, e.ToS())
		}
		if out, _ := sqlglot.Generate(e, "mysql", generator.Options{}); out != tc.want {
			t.Errorf("%q: round-trip = %q, want %q", tc.sql, out, tc.want)
		}
	}

	// The Postgres SET ROLE form is untouched (shares kind="ROLE", but its own parser/generator path).
	pg, err := sqlglot.ParseOne("SET ROLE admin", "postgres")
	if err != nil || pg.Expressions()[0].Text("kind") != "ROLE" {
		t.Errorf("postgres SET ROLE regressed: %v", err)
	}
}
