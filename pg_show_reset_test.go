package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Postgres `SHOW { name | ALL | special }` and `RESET { name | ALL }` parse to structured
// Show{this} / Reset{this} instead of the raw Command pinned upstream produces. `this` is a
// CANONICAL identity — ASCII-lowercased, unquoted, with the special multi-word phrases mapped to
// their underscore-GUC name — so a consumer gates on one value per setting regardless of spelling
// (see TestPostgresShowResetCanonicalIdentity). The accepted grammar matches PostgreSQL 17.6;
// trailing tokens, reserved words, and bare forms fail closed to Command. See DEVIATIONS 'Grammar
// extensions beyond upstream', ledger ids pg-show-guc / pg-reset.
func TestPostgresShowGUC(t *testing.T) {
	cases := []struct {
		sql  string
		name string // canonical parameter identity (this)
	}{
		{"SHOW search_path", "search_path"},
		{"SHOW ALL", "ALL"},
		{"SHOW TIME ZONE", "timezone"},                          // phrase -> GUC name
		{"SHOW timezone", "timezone"},                           // already the GUC name
		{"SHOW SESSION AUTHORIZATION", "session_authorization"}, // phrase -> GUC name
		{"SHOW TRANSACTION ISOLATION LEVEL", "transaction_isolation"},
		{"SHOW myext.myparam", "myext.myparam"},
		{`SHOW "search_path"`, "search_path"}, // quoted GUC name -> folded, unquoted
		{`SHOW "all"`, "ALL"},                 // SHOW: quoted "all" is the ALL form
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Errorf("%q: parse: %v", tc.sql, err)
			continue
		}
		if e.Kind() != exp.KindShow {
			t.Errorf("%q: Kind = %s, want Show\n%s", tc.sql, exp.ClassName(e.Kind()), e.ToS())
			continue
		}
		if got := e.Text("this"); got != tc.name {
			t.Errorf("%q: this = %q, want %q", tc.sql, got, tc.name)
		}
		if got, _ := sqlglot.Generate(e, "postgres", generator.Options{}); got != "SHOW "+tc.name {
			t.Errorf("%q: round-trip = %q, want %q", tc.sql, got, "SHOW "+tc.name)
		}
	}
}

func TestPostgresReset(t *testing.T) {
	cases := []struct {
		sql  string
		name string
	}{
		{"RESET search_path", "search_path"},
		{"RESET ALL", "ALL"},
		{"RESET TIME ZONE", "timezone"},
		{"RESET SESSION AUTHORIZATION", "session_authorization"},
		{"RESET TRANSACTION ISOLATION LEVEL", "transaction_isolation"},
		{"RESET myext.myparam", "myext.myparam"},
		{`RESET "search_path"`, "search_path"},
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Errorf("%q: parse: %v", tc.sql, err)
			continue
		}
		if e.Kind() != exp.KindReset {
			t.Errorf("%q: Kind = %s, want Reset\n%s", tc.sql, exp.ClassName(e.Kind()), e.ToS())
			continue
		}
		if got := e.Text("this"); got != tc.name {
			t.Errorf("%q: this = %q, want %q", tc.sql, got, tc.name)
		}
		if got, _ := sqlglot.Generate(e, "postgres", generator.Options{}); got != "RESET "+tc.name {
			t.Errorf("%q: round-trip = %q, want %q", tc.sql, got, "RESET "+tc.name)
		}
	}
}

// TestPostgresShowResetCanonicalIdentity is the security-relevant test: every case/quote/phrase
// spelling of one privilege-relevant GUC must fold to the SAME canonical `this`, so a consumer that
// gates on `this` cannot be tricked by a spelling variant. PostgreSQL's GUC lookup is
// case/quote-insensitive (verified on PG 17.6: `RESET ROLE` == `reset role` == `RESET "RoLe"`;
// `RESET SESSION AUTHORIZATION` == `RESET session_authorization`), so the AST must not expose the
// raw spelling.
func TestPostgresShowResetCanonicalIdentity(t *testing.T) {
	groups := []struct {
		want      string
		spellings []string
	}{
		{"role", []string{"RESET ROLE", "RESET role", "RESET RoLe", `RESET "RoLe"`, `RESET "role"`}},
		{"session_authorization", []string{
			"RESET SESSION AUTHORIZATION", "reset session authorization", "RESET Session Authorization",
			"RESET session_authorization", `RESET "Session_Authorization"`,
		}},
		{"timezone", []string{"SHOW TIME ZONE", "SHOW time zone", "SHOW timezone", "SHOW TimeZone", `SHOW "TimeZone"`}},
		{"search_path", []string{"SHOW search_path", "SHOW SEARCH_PATH", `SHOW "Search_Path"`, `SHOW "search_path"`}},
	}
	for _, g := range groups {
		for _, sql := range g.spellings {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Errorf("%q: parse: %v", sql, err)
				continue
			}
			if e.Kind() != exp.KindShow && e.Kind() != exp.KindReset {
				t.Errorf("%q: Kind = %s, want Show/Reset\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
				continue
			}
			if got := e.Text("this"); got != g.want {
				t.Errorf("%q: canonical this = %q, want %q (all spellings must fold to one identity)", sql, got, g.want)
			}
		}
	}
}

// Forms PostgreSQL rejects — a trailing token, a bare SHOW/RESET, a non-name token, a quoted
// multi-word "keyword" phrase, or a RESERVED word that cannot be a var_name (NULL/TRUE/FALSE/DEFAULT/
// CURRENT_USER/SESSION_USER) — must fail closed to a raw Command, never a half-built Show/Reset, and
// still round-trip byte-for-byte.
func TestPostgresShowResetFailClosed(t *testing.T) {
	for _, sql := range []string{
		"SHOW search_path extra",
		"SHOW",
		"SHOW 1",
		"SHOW (search_path)",
		`SHOW "TIME" "ZONE"`,
		"SHOW a.b.c",
		"SHOW NULL",         // reserved
		"SHOW TRUE",         // reserved
		"SHOW DEFAULT",      // reserved
		"SHOW SESSION_USER", // reserved function-keyword
		// A quoted parameter name whose folded text is not a plain identifier must fail closed rather
		// than launder: `RESET "all"` (an unknown param) would else regenerate as `RESET all` (= reset
		// everything); `RESET "SESSION AUTHORIZATION"` (space) would become the privileged phrase form.
		`RESET "all"`,
		`RESET "SESSION AUTHORIZATION"`,
		`SHOW "SESSION AUTHORIZATION"`,
		`RESET "a.b"`,
		"RESET search_path extra",
		"RESET",
		"RESET 1",
		"RESET CURRENT_USER", // reserved function-keyword
		"RESET DEFAULT",      // reserved
	} {
		e, err := sqlglot.ParseOne(sql, "postgres")
		if err != nil {
			t.Errorf("%q: parse: %v", sql, err)
			continue
		}
		if e.Kind() != exp.KindCommand {
			t.Errorf("%q: Kind = %s, want Command (fail closed)\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			continue
		}
		if got, _ := sqlglot.Generate(e, "postgres", generator.Options{}); got != sql {
			t.Errorf("%q: round-trip = %q", sql, got)
		}
	}
}

// RESET is a plain VAR token in Postgres (not COMMAND), so it stays usable as an ordinary
// identifier everywhere except statement start — `SELECT reset FROM t` must still parse as a Select.
func TestPostgresResetUsableAsIdentifier(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT reset FROM t", "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Kind() != exp.KindSelect {
		t.Fatalf("Kind = %s, want Select\n%s", exp.ClassName(e.Kind()), e.ToS())
	}
	if got, _ := sqlglot.Generate(e, "postgres", generator.Options{}); got != "SELECT reset FROM t" {
		t.Errorf("round-trip = %q", got)
	}
}

// The leading statement comment is preserved on the Reset root (the VAR-led dispatch must capture it
// just like SAVEPOINT does).
func TestPostgresResetPreservesLeadingComment(t *testing.T) {
	e, err := sqlglot.ParseOne("/* lead */ RESET search_path", "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cs := e.Comments(); len(cs) != 1 || cs[0] != " lead " {
		t.Errorf("leading comment = %v, want [\" lead \"]\n%s", cs, e.ToS())
	}
}

// MySQL RESET stays a raw Command (RESET MASTER/REPLICA is a privileged replication-admin op,
// semantically distinct from the Postgres GUC reset) — the Postgres extension must not leak to it.
func TestMySQLResetStaysCommand(t *testing.T) {
	for _, sql := range []string{"RESET MASTER", "RESET REPLICA", "RESET search_path"} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Errorf("%q: parse: %v", sql, err)
			continue
		}
		if e.Kind() != exp.KindCommand {
			t.Errorf("%q [mysql]: Kind = %s, want Command\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
		}
	}
}
