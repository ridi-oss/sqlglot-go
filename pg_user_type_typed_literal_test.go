package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Postgres user-type space typed-literal: `<type-name> 'string'` (no AS) — e.g.
// `public.evil_domain 'x'` or `"t" 'x'` — is a typed literal, NOT a column with a string alias (a
// bare string is never a valid Postgres alias). It parses to Cast(Literal, DataType(user-defined)),
// the SAME shape `'x'::a.b` / `CAST('x' AS a.b)` produce, so a consumer detects the user-type
// coercion structurally via FindAll(KindDataType). This is an extension beyond pinned upstream,
// which parse-errors the form (STRING_ALIASES is false for postgres — see
// testdata/upstream_extensions.jsonl "pg-user-type-typed-literal"). Verified against PostgreSQL 17.6
// (each form reaches type resolution, i.e. parses as a typed literal).

func TestPGUserTypeTypedLiteral(t *testing.T) {
	// The space form normalizes to the canonical CAST spelling on output (semantically identical),
	// matching how `::` and CAST already render, and carries exactly one DataType + one Cast.
	cases := []struct{ in, want string }{
		{"SELECT public.evil_domain 'x'", "SELECT CAST('x' AS public.evil_domain)"},
		{`SELECT "t" 'x'`, `SELECT CAST('x' AS "t")`},
		{"SELECT evil_domain 'x'", "SELECT CAST('x' AS evil_domain)"},
		{`SELECT "MySchema"."MyType" 'x'`, `SELECT CAST('x' AS "MySchema"."MyType")`},
		{"SELECT a, public.foo 'x', b FROM t", "SELECT a, CAST('x' AS public.foo), b FROM t"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.in, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if n := len(e.FindAll(exp.KindDataType)); n != 1 {
				t.Fatalf("FindAll(DataType) = %d, want 1:\n%s", n, e.ToS())
			}
			if n := len(e.FindAll(exp.KindCast)); n != 1 {
				t.Fatalf("FindAll(Cast) = %d, want 1:\n%s", n, e.ToS())
			}
			out, gerr := sqlglot.Generate(e, "postgres", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != tc.want {
				t.Fatalf("normalized = %q, want %q", out, tc.want)
			}
		})
	}
}

func TestPGUserTypeTypedLiteralShape(t *testing.T) {
	// The contract a consumer keys on: the SAME node `::`/CAST build for a user type — a Cast whose
	// `this` is the string Literal and whose `to` is a DataType(user-defined). It is byte-identical
	// to the CAST spelling's AST (kind expression preserves each part's quoting).
	for _, form := range []string{
		"SELECT public.evil_domain 'x'",          // space typed-literal (this extension)
		"SELECT CAST('x' AS public.evil_domain)", // canonical CAST — must produce the same shape
		"SELECT 'x'::public.evil_domain",         // :: spelling — same shape
	} {
		t.Run(form, func(t *testing.T) {
			e, err := sqlglot.ParseOne(form, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			cast := e.Expressions()[0]
			if cast.Kind() != exp.KindCast {
				t.Fatalf("projection kind = %v, want Cast:\n%s", exp.ClassName(cast.Kind()), e.ToS())
			}
			this, ok := cast.Arg("this").(exp.Expression)
			if !ok || this.Kind() != exp.KindLiteral || this.Arg("is_string") != true || this.Text("this") != "x" {
				t.Fatalf("Cast.this = %v, want string Literal 'x':\n%s", exp.ClassName(cast.Kind()), e.ToS())
			}
			to, ok := cast.Arg("to").(exp.Expression)
			if !ok || to.Kind() != exp.KindDataType || !exp.IsType(to, exp.DTypeUserDefined) {
				t.Fatalf("Cast.to = %v, want DataType(user-defined):\n%s", exp.ClassName(cast.Kind()), e.ToS())
			}
			// The type name is reachable as a DataType node — pm's userTypeCast walker key.
			if n := len(e.FindAll(exp.KindDataType)); n != 1 {
				t.Fatalf("FindAll(DataType) = %d, want 1", n)
			}
		})
	}
}

func TestPGUserTypeTypedLiteralQuotingPreserved(t *testing.T) {
	// The kind expression preserves each part's quoting so the Cast round-trips exactly.
	e, err := sqlglot.ParseOne(`SELECT "MySchema".my_type 'x'`, "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	to := e.Expressions()[0].Arg("to").(exp.Expression)
	dot, ok := to.Arg("kind").(exp.Expression)
	if !ok || dot.Kind() != exp.KindDot {
		t.Fatalf("kind = %v, want Dot:\n%s", exp.ClassName(to.Kind()), e.ToS())
	}
	schema := dot.Arg("this").(exp.Expression)
	if schema.Arg("quoted") != true {
		t.Fatalf("schema part lost its quoting:\n%s", e.ToS())
	}
}

func TestPGUserTypeTypedLiteralTrailingAlias(t *testing.T) {
	// A typed literal may itself carry a trailing alias — `<type> 'x' AS bar` and the implicit
	// `<type> 'x' bar` are both valid Postgres — so the Cast is wrapped in an Alias, still carrying
	// its DataType. Verified against PostgreSQL 17.6.
	for _, tc := range []struct{ in, want string }{
		{"SELECT public.foo 'x' AS bar", `SELECT CAST('x' AS public.foo) AS bar`},
		{"SELECT public.foo 'x' bar", `SELECT CAST('x' AS public.foo) AS bar`},
	} {
		t.Run(tc.in, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.in, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			proj := e.Expressions()[0]
			if proj.Kind() != exp.KindAlias {
				t.Fatalf("projection kind = %v, want Alias:\n%s", exp.ClassName(proj.Kind()), e.ToS())
			}
			if n := len(e.FindAll(exp.KindDataType)); n != 1 {
				t.Fatalf("FindAll(DataType) = %d, want 1 (the type survives the alias):\n%s", n, e.ToS())
			}
			if out, _ := sqlglot.Generate(e, "postgres", generator.Options{}); out != tc.want {
				t.Fatalf("round-trip = %q, want %q", out, tc.want)
			}
		})
	}
}

func TestPGUserTypeTypedLiteralValueForms(t *testing.T) {
	// The value accepts every Postgres string-constant form except bit/hex: a plain string, an escape
	// string (E'…'), and a dollar-quoted string ($$…$$) all produce a Cast; `B'…'`/`X'…'` are rejected
	// (Postgres rejects them in this position). Verified against PostgreSQL 17.6.
	t.Run("accepted string forms", func(t *testing.T) {
		for _, sql := range []string{
			"SELECT public.foo E'x'",
			"SELECT public.foo $$x$$",
			"SELECT public.foo $tag$x$tag$",
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("%q: parse: %v", sql, err)
			}
			if n := len(e.FindAll(exp.KindDataType)); n != 1 {
				t.Fatalf("%q: FindAll(DataType) = %d, want 1:\n%s", sql, n, e.ToS())
			}
		}
	})
	t.Run("bit, hex and national strings rejected", func(t *testing.T) {
		// Postgres rejects bit/hex AND national strings in the type-name+string production (even
		// though `N'x'` is a valid string constant standalone).
		for _, sql := range []string{"SELECT public.foo B'1'", "SELECT public.foo X'a'", "SELECT public.foo N'x'"} {
			if _, err := sqlglot.ParseOne(sql, "postgres"); err == nil {
				t.Fatalf("%q: parsed without error, want fail-closed (not a PG typed-literal value)", sql)
			}
		}
	})
}

func TestPGUserTypeTypedLiteralAllPositions(t *testing.T) {
	// The typed literal is recognized at the primary-expression level, so it folds into a Cast in
	// EVERY position — not just a SELECT projection. Each of these carries a discoverable DataType
	// (the security-relevant invariant for a downstream consumer). Verified against PostgreSQL 17.6.
	cases := []struct {
		sql   string
		casts int
	}{
		{"SELECT coalesce(public.foo 'x', NULL)", 1}, // function argument
		{"SELECT 1 WHERE 1 = public.foo 'x'", 1},     // predicate
		{"UPDATE t SET x = public.foo 'x'", 1},       // UPDATE SET value
		{"SELECT public.foo 'x' + 1", 1},             // binary-operator operand
		{"SELECT public.foo 'x'::text", 2},           // postfix :: (the typed literal is itself cast)
		{"INSERT INTO t VALUES (public.foo 'x')", 1}, // VALUES
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if n := len(e.FindAll(exp.KindCast)); n != tc.casts {
				t.Fatalf("FindAll(Cast) = %d, want %d:\n%s", n, tc.casts, e.ToS())
			}
			if n := len(e.FindAll(exp.KindDataType)); n < 1 {
				t.Fatalf("FindAll(DataType) = %d, want >= 1 (user-type coercion must be detectable):\n%s", n, e.ToS())
			}
		})
	}
}

func TestPGUserTypeTypedLiteralPostfix(t *testing.T) {
	// A `::` cast may directly follow the typed literal; a postfix `.field`/`[…]`/`.*` requires
	// parentheses in Postgres, so it fails closed rather than being applied (which would emit invalid
	// SQL). Verified against PostgreSQL 17.6.
	t.Run("cast operator applies", func(t *testing.T) {
		e, err := sqlglot.ParseOne("SELECT public.foo 'x'::text", "postgres")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if n := len(e.FindAll(exp.KindCast)); n != 2 {
			t.Fatalf("FindAll(Cast) = %d, want 2 (typed literal then ::):\n%s", n, e.ToS())
		}
	})
	t.Run("field/subscript/star postfix rejected", func(t *testing.T) {
		for _, sql := range []string{
			"SELECT public.foo 'x'.bar",
			"SELECT public.foo 'x'[1]",
			"SELECT public.foo 'x'.*",
		} {
			if _, err := sqlglot.ParseOne(sql, "postgres"); err == nil {
				t.Fatalf("%q: parsed without error, want fail-closed (postfix needs parens)", sql)
			}
		}
	})
}

func TestPGUserTypeTypedLiteralKnownLimitationsFailClosed(t *testing.T) {
	// Documented grammar-completeness gaps (DEVIATIONS.md): under the default IMMEDIATE error level a
	// type modifier and a keyword-named-user-type / multiline-string-continuation form fail closed
	// (parse error → a consumer denies), so none is a detection bypass. Verified against PostgreSQL
	// 17.6 (each is valid PG that this port does not yet fold).
	for _, sql := range []string{
		"SELECT public.foo(3) 'x'",         // type modifier — parsed as a function call, not folded
		"SELECT coalesce(alter 'x', NULL)", // keyword-named user type not reachable as a column
	} {
		if _, err := sqlglot.ParseOne(sql, "postgres"); err == nil {
			t.Fatalf("%q: parsed without error — if this now folds, update DEVIATIONS known-limitations", sql)
		}
	}
}

func TestPGUserTypeTypedLiteralTypeNameStrictness(t *testing.T) {
	// The type name must be a genuine identifier chain. A Dot built by postfix field/`*`/subscript
	// access over an arbitrary base, and a bare value-function keyword pseudo-column, are NOT type
	// names — Postgres rejects each with a syntax error, so the fold must not fire (fail closed).
	t.Run("non-identifier Dot rejected", func(t *testing.T) {
		for _, sql := range []string{
			"SELECT (1).foo 'x'",
			"SELECT foo().bar 'x'",
			"SELECT arr[1].foo 'x'",
			"SELECT t.* 'x'",
		} {
			if _, err := sqlglot.ParseOne(sql, "postgres"); err == nil {
				t.Fatalf("%q: parsed without error, want fail-closed (not a type-name chain)", sql)
			}
		}
	})
	t.Run("bare reserved value-function keyword rejected", func(t *testing.T) {
		for _, sql := range []string{
			"SELECT current_user 'x'",
			"SELECT session_user 'x'",
			"SELECT current_catalog 'x'",
		} {
			if _, err := sqlglot.ParseOne(sql, "postgres"); err == nil {
				t.Fatalf("%q: parsed without error, want fail-closed (reserved value-function, not a type name)", sql)
			}
		}
	})
	// A bare NON-reserved keyword (`type`, `format`, `schema`, `current_schema`, …) IS a valid
	// unquoted type name in Postgres and must fold — the classifier keys on the specific reserved
	// value-function tokens, NOT blanket keyword membership (which would false-reject dozens of these).
	t.Run("bare non-reserved keyword type name accepted", func(t *testing.T) {
		for _, sql := range []string{
			"SELECT type 'x'",
			"SELECT format 'x'",
			"SELECT schema 'x'",
			"SELECT current_schema 'x'",
			"SELECT view 'x'",
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("%q: parse: %v", sql, err)
			}
			if n := len(e.FindAll(exp.KindDataType)); n != 1 {
				t.Fatalf("%q: FindAll(DataType) = %d, want 1 (valid keyword type name):\n%s", sql, n, e.ToS())
			}
		}
	})
	// ...and a *qualified* name whose part collides with a non-reserved keyword is likewise a type
	// reference (no over-rejection).
	t.Run("qualified keyword-collision name accepted", func(t *testing.T) {
		for _, sql := range []string{
			"SELECT public.name 'x'",
			"SELECT public.type 'x'",
			"SELECT public.value 'x'",
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("%q: parse: %v", sql, err)
			}
			if n := len(e.FindAll(exp.KindDataType)); n != 1 {
				t.Fatalf("%q: FindAll(DataType) = %d, want 1 (valid qualified type name):\n%s", sql, n, e.ToS())
			}
		}
	})
}

func TestPGUserTypeTypedLiteralCommentPreserved(t *testing.T) {
	// A comment between the type name and the string is attached to the name node, which ColumnsToDot
	// drops when it rebuilds the name — so it is carried onto the Cast rather than lost.
	e, err := sqlglot.ParseOne("SELECT public.foo /* c */ 'x'", "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, _ := sqlglot.Generate(e, "postgres", generator.Options{})
	if want := "SELECT CAST('x' AS public.foo) /* c */"; out != want {
		t.Fatalf("comment dropped: got %q, want %q", out, want)
	}
}

func TestPGHasNoStringAliases(t *testing.T) {
	// Postgres has no *implicit* (no-AS) string aliases: a trailing string that is not a typed literal
	// fails closed rather than being folded into a quoted-identifier alias, matching pinned upstream
	// (STRING_ALIASES is false) and real PG (`SELECT 1 'x'` is a syntax error). A stray second string
	// after a typed literal (`public.foo 'a' 'b'`) is likewise rejected. (The explicit `AS 'x'` form
	// stays an Alias, matching upstream's `_parse_id_var` any-token path — out of scope here.)
	for _, sql := range []string{
		"SELECT 1 'x'",              // non-type-name + string → not an alias in PG
		"SELECT public.foo 'a' 'b'", // typed literal then a stray string
	} {
		if _, err := sqlglot.ParseOne(sql, "postgres"); err == nil {
			t.Fatalf("%q: parsed without error, want fail-closed (no PG implicit string alias)", sql)
		}
	}
}

func TestPGUserTypeTypedLiteralBoundaries(t *testing.T) {
	// Normal aliases are untouched: an implicit id-var alias and an explicit AS alias both stay
	// aliases, not typed literals.
	t.Run("normal aliases unaffected", func(t *testing.T) {
		for _, sql := range []string{
			"SELECT col AS alias",    // explicit AS
			"SELECT col alias",       // implicit id-var alias
			"SELECT public.foo AS x", // qualified column + AS alias (has AS → not a typed literal)
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("%q: parse: %v", sql, err)
			}
			if n := len(e.FindAll(exp.KindDataType)); n != 0 {
				t.Fatalf("%q: FindAll(DataType) = %d, want 0 (should stay an alias):\n%s", sql, n, e.ToS())
			}
		}
	})
	// The typed-literal fold is Postgres-only: MySQL/base keep their string-as-identifier alias
	// (MySQL's `SELECT 1 'x'` / `SELECT col 'x'` are valid string aliases there).
	t.Run("mysql string alias unaffected", func(t *testing.T) {
		for _, sql := range []string{"SELECT 1 'x'", "SELECT col 'x'"} {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("%q: parse: %v", sql, err)
			}
			if n := len(e.FindAll(exp.KindDataType)); n != 0 {
				t.Fatalf("%q: FindAll(DataType) = %d, want 0 (mysql string alias):\n%s", sql, n, e.ToS())
			}
			if e.Expressions()[0].Kind() != exp.KindAlias {
				t.Fatalf("%q: want Alias in mysql:\n%s", sql, e.ToS())
			}
		}
	})
}
