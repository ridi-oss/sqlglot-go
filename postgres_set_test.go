package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Postgres SET special-forms parse into Set{SetItem{kind: ...}} — a grammar extension beyond
// pinned upstream, which degrades each to a raw Command. A consumer reads SetItem.kind to tell a
// privileged SET (ROLE, SESSION AUTHORIZATION) from a benign one (TIME ZONE, NAMES, CONSTRAINTS,
// SESSION CHARACTERISTICS) without string-scanning. Verified against PostgreSQL 17.6.

func setItemKind(t *testing.T, e exp.Expression) string {
	t.Helper()
	if e.Kind() != exp.KindSet {
		t.Fatalf("root = %v, want Set:\n%s", exp.ClassName(e.Kind()), e.ToS())
	}
	items := e.Expressions()
	if len(items) != 1 {
		t.Fatalf("Set items = %d, want 1:\n%s", len(items), e.ToS())
	}
	return items[0].Text("kind")
}

func TestPostgresSetSpecialForms(t *testing.T) {
	cases := []struct {
		sql      string
		wantKind string
	}{
		{"SET ROLE admin", "ROLE"},
		{"SET ROLE NONE", "ROLE"},
		{"SET ROLE 'admin'", "ROLE"},
		{"SET SESSION AUTHORIZATION bob", "SESSION AUTHORIZATION"},
		{"SET SESSION AUTHORIZATION DEFAULT", "SESSION AUTHORIZATION"},
		{"SET TIME ZONE 'UTC'", "TIME ZONE"},
		{"SET TIME ZONE LOCAL", "TIME ZONE"},
		{"SET TIME ZONE DEFAULT", "TIME ZONE"},
		{"SET TIME ZONE INTERVAL '+00:00' HOUR TO MINUTE", "TIME ZONE"},
		{"SET TIME ZONE 7", "TIME ZONE"},
		{"SET TIME ZONE -5", "TIME ZONE"},  // signed numeric offset
		{"SET TIME ZONE UTC", "TIME ZONE"}, // bare zone name
		{"SET NAMES 'utf8'", "NAMES"},
		{"SET NAMES DEFAULT", "NAMES"},
		{"SET CONSTRAINTS ALL DEFERRED", "CONSTRAINTS"},
		{"SET CONSTRAINTS ALL IMMEDIATE", "CONSTRAINTS"},
		{"SET CONSTRAINTS a, b DEFERRED", "CONSTRAINTS"},
		{`SET CONSTRAINTS "ALL" DEFERRED`, "CONSTRAINTS"},      // quoted "ALL" is a constraint name
		{"SET CONSTRAINTS public.foo DEFERRED", "CONSTRAINTS"}, // schema-qualified name
		{"SET SESSION CHARACTERISTICS AS TRANSACTION ISOLATION LEVEL SERIALIZABLE", "SESSION CHARACTERISTICS"},
		{"SET SESSION CHARACTERISTICS AS TRANSACTION READ WRITE", "SESSION CHARACTERISTICS"},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if kind := setItemKind(t, e); kind != tc.wantKind {
				t.Fatalf("SetItem kind = %q, want %q:\n%s", kind, tc.wantKind, e.ToS())
			}
			out, gerr := sqlglot.Generate(e, "postgres", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != tc.sql {
				t.Fatalf("round-trip = %q, want %q", out, tc.sql)
			}
		})
	}
}

// The discriminator a consumer keys on: privileged vs benign, readable straight off SetItem.kind.
func TestPostgresSetKindDiscriminator(t *testing.T) {
	privileged := map[string]bool{"ROLE": true, "SESSION AUTHORIZATION": true}
	for _, tc := range []struct {
		sql      string
		wantPriv bool
	}{
		{"SET ROLE admin", true},
		{"SET SESSION AUTHORIZATION bob", true},
		{"SET TIME ZONE 'UTC'", false},
		{"SET NAMES 'utf8'", false},
		{"SET CONSTRAINTS ALL DEFERRED", false},
		{"SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY", false},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := privileged[setItemKind(t, e)]; got != tc.wantPriv {
				t.Fatalf("privileged=%v, want %v for %q", got, tc.wantPriv, tc.sql)
			}
		})
	}
}

// Postgres also exposes `role` and `session_authorization` as ordinary GUCs, so the assignment
// spellings perform the same privilege change but carry a scope-only/blank kind — the privileged
// signal is the assignment's LHS variable name, NOT the SetItem.kind. A consumer must deny on the
// LHS name too (see DEVIATIONS). This test guards that the LHS name is structurally reachable.
func TestPostgresSetGUCAliasLHSReachable(t *testing.T) {
	for _, tc := range []struct {
		sql     string
		wantVar string
	}{
		{"SET SESSION role = attacker", "role"},
		{"SET session_authorization = attacker", "session_authorization"},
		{"SET LOCAL session_authorization = attacker", "session_authorization"},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindSet || len(e.Expressions()) != 1 {
				t.Fatalf("want single-item Set:\n%s", e.ToS())
			}
			assign, ok := e.Expressions()[0].Arg("this").(exp.Expression)
			if !ok || assign.Kind() != exp.KindEQ {
				t.Fatalf("SetItem.this not an EQ assignment:\n%s", e.ToS())
			}
			lhs, ok := assign.Arg("this").(exp.Expression)
			if !ok || lhs.Name() != tc.wantVar {
				t.Fatalf("LHS var = %q, want %q:\n%s", func() string {
					if lhs != nil {
						return lhs.Name()
					}
					return "<nil>"
				}(), tc.wantVar, e.ToS())
			}
		})
	}
}

func TestPostgresSetFailClosedAndRegressions(t *testing.T) {
	// Ordinary assignments and the existing TRANSACTION form are unchanged.
	t.Run("assignments unchanged", func(t *testing.T) {
		for _, sql := range []string{
			"SET search_path = public",
			"SET SESSION search_path = public",
			"SET LOCAL search_path TO public",
			"SET TRANSACTION ISOLATION LEVEL SERIALIZABLE",
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if e.Kind() != exp.KindSet {
				t.Fatalf("%q: kind = %v, want Set:\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})

	// A special form missing its required value fails closed to Command (never a degenerate Set),
	// as do the SESSION/LOCAL-scoped variants this extension deliberately does not model.
	t.Run("malformed and unmodeled forms fail closed", func(t *testing.T) {
		for _, sql := range []string{
			"SET ROLE",                                              // missing role
			"SET TIME ZONE",                                         // missing value
			"SET SESSION AUTHORIZATION",                             // missing user
			"SET CONSTRAINTS ALL",                                   // missing mode
			"SET SESSION TIME ZONE 'UTC'",                           // scoped TIME ZONE stays unmodeled (benign)
			"SET CONSTRAINTS ALL GARBAGE",                           // mode not DEFERRED/IMMEDIATE
			"SET SESSION CHARACTERISTICS READ ONLY",                 // missing AS TRANSACTION
			"SET SESSION CHARACTERISTICS AS TRANSACTION",            // missing mode
			"SET SESSION CHARACTERISTICS AS TRANSACTION DEFERRABLE", // unmodeled characteristic (no crash)
			"SET NAMES 'utf8' COLLATE 'x'",                          // COLLATE is not valid Postgres SET NAMES
			"SET NAMES utf8",                                        // unquoted charset is invalid Postgres
			"SET TIME ZONE 'UTC', ROLE admin",                       // comma-combined multi-item (Postgres rejects)
			"SET a = 1, b = 2",                                      // multi-item Postgres SET
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("%q: kind = %v, want Command (fail-closed):\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})

	// The special forms are Postgres-only; base/MySQL leave them as Command.
	t.Run("special forms are postgres-gated", func(t *testing.T) {
		// The Postgres SET special-forms are postgres-gated: base leaves `SET ROLE admin` a raw Command.
		// (MySQL now structures its own `SET ROLE` — a distinct grammar; see TestMySQLSetRole.)
		e, err := sqlglot.ParseOne("SET ROLE admin", "")
		if err != nil {
			t.Fatalf("parse [base]: %v", err)
		}
		if e.Kind() != exp.KindCommand {
			t.Fatalf("[base] SET ROLE admin = %v, want Command:\n%s", exp.ClassName(e.Kind()), e.ToS())
		}
	})
}

// Postgres allows a single GUC to take a comma-separated VALUE list (`SET search_path = a, b`), which
// pinned upstream degrades to a Command; this port structures it so a consumer sees a benign assignment
// (readable LHS via SetItem->EQ->this) instead of an opaque Command that a uniform fail-close would
// wrongly deny. Verified against PostgreSQL 17.6 (ledger id pg-set-multi-value). A comma value list is a
// grammar extension; a second `name = value` (`SET a = 1, b = 2`) is a PG SYNTAX error and fails closed.
func TestPostgresSetMultiValue(t *testing.T) {
	// Valid multi-value forms structure and round-trip. `TO` normalizes to `=` (systemic, upstream-faithful).
	for _, tc := range []struct{ sql, want string }{
		{"SET search_path = a, b", "SET search_path = a, b"},
		{"SET search_path = a, b, c", "SET search_path = a, b, c"},
		{"SET search_path TO a, b", "SET search_path = a, b"},
		{"SET LOCAL search_path = a, b", "SET LOCAL search_path = a, b"},
		{"SET SESSION search_path = a, b", "SET SESSION search_path = a, b"},
		{"SET x = 1, 2", "SET x = 1, 2"},   // syntactically valid (PG rejects only semantically)
		{"SET x = ON, a", "SET x = ON, a"}, // ON is a reserved keyword but a valid opt_boolean_or_string var_value
		{"SET x = a, ON", "SET x = a, ON"},
		{"SET x = ON, OFF", "SET x = ON, OFF"},
		// a QUOTED "DEFAULT" is an ordinary identifier value (valid PG), distinct from the bare DEFAULT
		// keyword (fail-closed below) — the §1.12 quote preservation must survive the value-list path.
		{`SET search_path = "DEFAULT", public`, `SET search_path = "DEFAULT", public`},
		{`SET search_path = "$user", "DEFAULT"`, `SET search_path = "$user", "DEFAULT"`},
	} {
		e, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Errorf("%q: %v", tc.sql, err)
			continue
		}
		if e.Kind() != exp.KindSet || len(e.Expressions()) != 1 {
			t.Errorf("%q: want single-item Set\n%s", tc.sql, e.ToS())
			continue
		}
		// The variable name must be reachable off SetItem->EQ->this (the consumer's gate signal).
		item := e.Expressions()[0]
		eq, ok := item.Arg("this").(exp.Expression)
		if !ok || eq.Kind() != exp.KindEQ {
			t.Errorf("%q: SetItem.this not an EQ\n%s", tc.sql, e.ToS())
		}
		if out, _ := sqlglot.Generate(e, "postgres", generator.Options{}); out != tc.want {
			t.Errorf("%q: round-trip = %q, want %q", tc.sql, out, tc.want)
		}
	}

	// §1 correctness: a QUOTED identifier value keeps its quotes — upstream drops them (rendering the
	// invalid unquoted `$user`), but real PostgreSQL rejects `SET search_path = $user` (syntax at `$`)
	// and accepts `"$user"`. So the port matches the DB, not upstream.
	for _, tc := range []struct{ sql, want string }{
		{`SET search_path = "$user"`, `SET search_path = "$user"`},
		{`SET search_path = "$user", public`, `SET search_path = "$user", public`},
		// A DOTTED single value (`a."b"`) is invalid Postgres (syntax error at `.`); only a single-part
		// quoted identifier is preserved verbatim. The lone dotted form flattens to the bare trailing name
		// (lossy but syntactically valid, matching upstream) — it must NOT round-trip to the invalid
		// `a."b"`. (A dotted value inside a LIST fails closed instead — see the fail-closed cases below.)
		{`SET search_path = a."b"`, `SET search_path = b`},
	} {
		e, err := sqlglot.ParseOne(tc.sql, "postgres")
		if err != nil {
			t.Errorf("%q: %v", tc.sql, err)
			continue
		}
		if out, _ := sqlglot.Generate(e, "postgres", generator.Options{}); out != tc.want {
			t.Errorf("%q: round-trip = %q, want %q (quoting must be preserved)", tc.sql, out, tc.want)
		}
	}

	// A value list admits only simple var_values (identifier/string/number/bool/signed). Anything PG
	// rejects as a syntax error in a list fails closed: a second `name = value` assignment, a trailing/
	// leading/double comma, an expression, a cast, or the DEFAULT keyword (valid only as the sole value).
	for _, sql := range []string{
		"SET a = 1, b = 2",     // second name = value (multi-assignment)
		"SET x = a, b = c",     // embedded assignment
		"SET x = a,",           // trailing comma
		"SET search_path = a,", // trailing comma
		"SET x = a + b, c",     // expression element
		"SET x = a::text, b",   // cast element
		"SET x = DEFAULT, a",   // DEFAULT is not a var_list element
		"SET search_path = a, DEFAULT",
		"SET search_path = a.b, c",         // dotted element (unquoted) — invalid in a list
		`SET search_path = "$user", a."b"`, // dotted element (quoted trailing) — invalid in a list
	} {
		e, err := sqlglot.ParseOne(sql, "postgres")
		if err != nil {
			t.Errorf("%q: %v", sql, err)
			continue
		}
		if e.Kind() != exp.KindCommand {
			t.Errorf("%q: want Command (fail-closed), got %s\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
		}
	}

	// The value list renders FLAT even in Pretty mode (no stray per-item indentation/newlines).
	if e, err := sqlglot.ParseOne("SET search_path = a, b, c", "postgres"); err == nil {
		if out, _ := sqlglot.Generate(e, "postgres", generator.Options{Pretty: true}); out != "SET search_path = a, b, c" {
			t.Errorf("pretty multi-value = %q, want flat %q", out, "SET search_path = a, b, c")
		}
	}

	// MySQL/base commas separate SET ITEMS, not values — the value-list is Postgres-only. `SET a = 1, b = 2`
	// on MySQL stays two items (unchanged), not a single value-list assignment.
	e, err := sqlglot.ParseOne("SET a = 1, b = 2", "mysql")
	if err != nil || e.Kind() != exp.KindSet || len(e.Expressions()) != 2 {
		t.Errorf("mysql SET a = 1, b = 2: want 2-item Set (comma = item separator), got %v / %d items", err, len(e.Expressions()))
	}
}
