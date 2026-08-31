package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
)

// Tests for the NON-UPSTREAM EngineCatalog identity resolution (engine_catalog.go), companion to
// the opaque_functions setting. The catalog is a pure consumer input; the tiers under test are the
// live-verified engine algorithms (MySQL native priority; PG search_path with pg_catalog
// implicitly first unless explicitly listed; fail-closed Unknown residue).

func names(list ...string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, n := range list {
		m[n] = true
	}
	return m
}

func pgCatalog() *optimizer.EngineCatalog {
	return &optimizer.EngineCatalog{
		BuiltinFunctions: names("abs", "lower", "set_config", "count"),
		SystemFunctionSchemas: map[string]map[string]bool{
			"pg_catalog":         names("abs", "lower", "set_config", "count"),
			"information_schema": names("_pg_expandarray"),
		},
		UDFSchemas: map[string]map[string]bool{
			"app": names("f", "abs"),
			"sh":  names("f"),
		},
		SystemRelations: map[string]map[string]bool{
			"pg_catalog":         names("pg_tables", "pg_shadow"),
			"information_schema": names("tables"),
		},
		UserRelations: map[string]map[string]bool{
			"app":    names("orders"),
			"public": names("tables", "pg_tables"),
		},
	}
}

func resolveOne(t *testing.T, dialect, sql string, catalog *optimizer.EngineCatalog, searchPath []string) (map[exp.Expression]optimizer.ResolvedCall, map[exp.Expression]optimizer.ResolvedRelation, exp.Expression) {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q): %v", sql, err)
	}
	calls := map[exp.Expression]optimizer.ResolvedCall{}
	relations := map[exp.Expression]optimizer.ResolvedRelation{}
	optimizer.ResolveEngineIdentities(e, catalog, dialect, searchPath, calls, relations)
	return calls, relations, e
}

func onlyCall(t *testing.T, calls map[exp.Expression]optimizer.ResolvedCall) optimizer.ResolvedCall {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("want exactly 1 call entry, got %d: %v", len(calls), calls)
	}
	for _, c := range calls {
		return c
	}
	panic("unreachable")
}

func TestEngineCatalogPGFunctionTiers(t *testing.T) {
	dialect := "postgres, opaque_functions=true"
	for _, tt := range []struct {
		name, sql    string
		searchPath   []string
		wantKind     optimizer.CallKind
		wantIdentity string
	}{
		{"builtin implicit pg_catalog first", "SELECT abs(-1)", []string{"sh", "public"}, optimizer.CallBuiltin, "pg_catalog.abs"},
		{"udf via search path, no builtin name", "SELECT f(1)", []string{"app", "public"}, optimizer.CallUDF, "app.f"},
		// f exists in both sh and app: at name level we cannot prove identical signatures, and a
		// type-dependent winner is possible (live-verified: f(4.2) picked the LATER schema's
		// f(numeric)) — the residue fails closed.
		{"name in two path schemas -> Unknown", "SELECT f(1)", []string{"sh", "app"}, optimizer.CallUnknown, ""},
		{"demoted pg_catalog + shadow -> Unknown", "SELECT abs(-1)", []string{"app", "pg_catalog"}, optimizer.CallUnknown, ""},
		{"demoted pg_catalog, no shadow -> builtin", "SELECT lower(x)", []string{"app", "pg_catalog"}, optimizer.CallBuiltin, "pg_catalog.lower"},
		{"name in builtin AND path udf -> Unknown", "SELECT abs(-1)", []string{"app"}, optimizer.CallUnknown, ""},
		{"qualified system schema", "SELECT pg_catalog.set_config(a, b, c)", nil, optimizer.CallBuiltin, "pg_catalog.set_config"},
		{"qualified udf schema", "SELECT app.f(1)", nil, optimizer.CallUDF, "app.f"},
		{"qualified unknown schema", "SELECT nowhere.f(1)", nil, optimizer.CallUnknown, ""},
		{"unqualified absent everywhere", "SELECT mystery(1)", []string{"app", "public"}, optimizer.CallUnknown, ""},
		{"aggregate is a builtin call", "SELECT count(x) FROM t", nil, optimizer.CallBuiltin, "pg_catalog.count"},
		{"case-insensitive name fold", "SELECT ABS(-1)", nil, optimizer.CallBuiltin, "pg_catalog.abs"},
		{"quoted name is verbatim, not folded", `SELECT "ABS"(-1)`, nil, optimizer.CallUnknown, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls, _, _ := resolveOne(t, dialect, tt.sql, pgCatalog(), tt.searchPath)
			got := onlyCall(t, calls)
			if got.Kind != tt.wantKind || got.Identity != tt.wantIdentity {
				t.Errorf("%q = {%v %q}, want {%v %q}", tt.sql, got.Kind, got.Identity, tt.wantKind, tt.wantIdentity)
			}
		})
	}
}

func TestEngineCatalogMySQLFunctionTiers(t *testing.T) {
	dialect := "mysql, opaque_functions=true"
	catalog := &optimizer.EngineCatalog{
		BuiltinFunctions: names("abs", "pi", "ifnull"),
		SystemFunctionSchemas: map[string]map[string]bool{
			"sys": names("format_bytes"),
		},
		UDFSchemas: map[string]map[string]bool{
			"shop":  names("pi", "myfn"),
			"other": names("elsewhere_fn"),
		},
		CurrentDatabase:   "shop",
		LoadableFunctions: names("plugin_fn", "abs"),
	}
	for _, tt := range []struct {
		name, sql    string
		wantKind     optimizer.CallKind
		wantIdentity string
	}{
		// Native priority: a stored shop.pi never wins unqualified (live-verified on mysql:8.0).
		{"native beats stored", "SELECT pi()", optimizer.CallBuiltin, "pi"},
		{"qualified reaches stored", "SELECT shop.pi()", optimizer.CallUDF, "shop.pi"},
		{"stored in current db", "SELECT myfn()", optimizer.CallUDF, "shop.myfn"},
		{"stored in other db unqualified -> Unknown", "SELECT elsewhere_fn()", optimizer.CallUnknown, ""},
		{"qualified other db", "SELECT other.elsewhere_fn()", optimizer.CallUDF, "other.elsewhere_fn"},
		{"system schema qualified", "SELECT sys.format_bytes(n)", optimizer.CallBuiltin, "sys.format_bytes"},
		{"absent everywhere", "SELECT mystery()", optimizer.CallUnknown, ""},
		// Loadable (plugin) functions: global namespace, below natives, above stored — CallUDF.
		{"loadable is UDF, global", "SELECT plugin_fn(1)", optimizer.CallUDF, "plugin_fn"},
		{"native beats loadable", "SELECT abs(1)", optimizer.CallBuiltin, "abs"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls, _, _ := resolveOne(t, dialect, tt.sql, catalog, nil)
			got := onlyCall(t, calls)
			if got.Kind != tt.wantKind || got.Identity != tt.wantIdentity {
				t.Errorf("%q = {%v %q}, want {%v %q}", tt.sql, got.Kind, got.Identity, tt.wantKind, tt.wantIdentity)
			}
		})
	}
}

func TestEngineCatalogRelations(t *testing.T) {
	dialect := "postgres, opaque_functions=true"
	for _, tt := range []struct {
		name, sql    string
		searchPath   []string
		wantKind     optimizer.RelationKind
		wantIdentity string
	}{
		// public.pg_tables exists in the user catalog, but pg_catalog is implicitly FIRST when
		// not explicitly listed — the system view wins (matches the engine).
		{"unqualified system view via implicit pg_catalog", "SELECT * FROM pg_tables", []string{"public"}, optimizer.SystemRelation, "pg_catalog.pg_tables"},
		// With pg_catalog explicitly demoted, the earlier user table shadows the system name.
		{"demoted pg_catalog: user table shadows system view", "SELECT * FROM pg_tables", []string{"public", "pg_catalog"}, optimizer.UserRelation, "public.pg_tables"},
		// A user table earlier on the path shadows information_schema's later entry.
		{"user table shadows later path schema", "SELECT * FROM tables", []string{"public", "information_schema"}, optimizer.UserRelation, "public.tables"},
		{"system schema hit later on explicit path", "SELECT * FROM tables", []string{"app", "information_schema"}, optimizer.SystemRelation, "information_schema.tables"},
		// Qualified names absent from BOTH catalogs are Unknown — never asserted UserRelation.
		{"qualified name missing from system catalog -> Unknown", "SELECT * FROM pg_catalog.not_a_real_table", nil, optimizer.RelationUnknown, ""},
		{"qualified unknown schema -> Unknown", "SELECT * FROM nowhere.orders", nil, optimizer.RelationUnknown, ""},
		{"qualified information_schema", "SELECT * FROM information_schema.tables", nil, optimizer.SystemRelation, "information_schema.tables"},
		{"qualified sensitive catalog", "SELECT * FROM pg_catalog.pg_shadow", nil, optimizer.SystemRelation, "pg_catalog.pg_shadow"},
		{"qualified user schema", "SELECT * FROM app.orders", nil, optimizer.UserRelation, "app.orders"},
		{"unqualified user table via path", "SELECT * FROM orders", []string{"app"}, optimizer.UserRelation, "app.orders"},
		{"unqualified absent from path schemas -> Unknown", "SELECT * FROM orders", []string{"public"}, optimizer.RelationUnknown, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, relations, _ := resolveOne(t, dialect, tt.sql, pgCatalog(), tt.searchPath)
			if len(relations) != 1 {
				t.Fatalf("want 1 relation entry, got %d", len(relations))
			}
			for _, got := range relations {
				if got.Kind != tt.wantKind || got.Identity != tt.wantIdentity {
					t.Errorf("%q = {%v %q}, want {%v %q}", tt.sql, got.Kind, got.Identity, tt.wantKind, tt.wantIdentity)
				}
			}
		})
	}
}

func TestEngineCatalogFailClosedEdges(t *testing.T) {
	dialect := "postgres, opaque_functions=true"

	// Unqualified pg_catalog builtin/UDF-map collision fails closed (not just qualified).
	collide := pgCatalog()
	collide.UDFSchemas["pg_catalog"] = names("abs")
	calls, _, _ := resolveOne(t, dialect, "SELECT abs(x)", collide, []string{"public"})
	if got := onlyCall(t, calls); got.Kind != optimizer.CallUnknown {
		t.Errorf("unqualified pg_catalog collision = %v, want Unknown", got.Kind)
	}

	// Engine-true folding: PG folds ASCII-only, so CAFÉ() must NOT match a café catalog entry.
	unicodeCat := &optimizer.EngineCatalog{BuiltinFunctions: names("café")}
	calls, _, _ = resolveOne(t, dialect, "SELECT CAFÉ(x)", unicodeCat, nil)
	if got := onlyCall(t, calls); got.Kind != optimizer.CallUnknown {
		t.Errorf("CAFÉ folded non-ASCII: %v, want Unknown (engine folds ASCII-only)", got.Kind)
	}

	// Dotted quoted components would make the dot-joined identity ambiguous — Unknown.
	dotted := &optimizer.EngineCatalog{UDFSchemas: map[string]map[string]bool{"a": names("b.c")}}
	calls, _, _ = resolveOne(t, dialect, `SELECT "a"."b.c"(x)`, dotted, nil)
	if got := onlyCall(t, calls); got.Kind != optimizer.CallUnknown {
		t.Errorf("dotted identity component = %v, want Unknown", got.Kind)
	}

	// CTE aliases shadow catalog relations — fail-closed, never a catalog identity.
	_, relations, _ := resolveOne(t, dialect, "WITH pg_shadow AS (SELECT 1 AS a) SELECT * FROM pg_shadow", pgCatalog(), []string{"public"})
	for _, rr := range relations {
		if rr.Kind != optimizer.RelationUnknown {
			t.Errorf("CTE-named relation = %v %q, want Unknown", rr.Kind, rr.Identity)
		}
	}

	// Three-part catalog.schema.table has no catalog dimension in the input — Unknown.
	_, relations, _ = resolveOne(t, dialect, "SELECT * FROM other.pg_catalog.pg_shadow", pgCatalog(), nil)
	for _, rr := range relations {
		if rr.Kind != optimizer.RelationUnknown {
			t.Errorf("catalog-qualified relation = %v, want Unknown", rr.Kind)
		}
	}

	// PG temp relations shadow pg_catalog (implicit temp schema is searched first).
	temp := pgCatalog()
	temp.TempRelations = names("pg_tables")
	_, relations, _ = resolveOne(t, dialect, "SELECT * FROM pg_tables", temp, []string{"public"})
	for _, rr := range relations {
		if rr.Kind != optimizer.UserRelation || rr.Identity != "pg_temp.pg_tables" {
			t.Errorf("temp shadow = %v %q, want UserRelation pg_temp.pg_tables", rr.Kind, rr.Identity)
		}
	}

	// Unsupported dialects never borrow PG semantics — everything Unknown.
	calls, relations, _ = resolveOne(t, ", opaque_functions=true", "SELECT abs(x) FROM pg_tables", pgCatalog(), []string{"public"})
	for _, c := range calls {
		if c.Kind != optimizer.CallUnknown {
			t.Errorf("base dialect call = %v, want Unknown", c.Kind)
		}
	}
	for _, rr := range relations {
		if rr.Kind != optimizer.RelationUnknown {
			t.Errorf("base dialect relation = %v, want Unknown", rr.Kind)
		}
	}
}

func TestEngineCatalogQualifierSpellingPreserved(t *testing.T) {
	// Through Qualify under mysql lctn=0 strategy, a function's database qualifier keeps its
	// spelling (Shop != shop are different databases) — NormalizeIdentifiers skips opaque call
	// identifiers; the resolver folds for lookup itself (relation-role: no fold at lctn=0).
	dialect := "mysql, normalization_strategy=mysql_case_sensitive_table_names, opaque_functions=true"
	e, err := sqlglot.ParseOne("SELECT Shop.myfn(x)", dialect)
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = dialect
	opts.QualifyColumns = false
	opts.ValidateQualifyColumns = false
	opts.QuoteIdentifiers = false
	catalog := &optimizer.EngineCatalog{UDFSchemas: map[string]map[string]bool{"Shop": names("myfn")}}
	calls := map[exp.Expression]optimizer.ResolvedCall{}
	opts.EngineCatalog = catalog
	opts.CallReport = calls
	out := optimizer.Qualify(e, opts)
	got, err := sqlglot.Generate(out, dialect, generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "SELECT Shop.myfn(x)" {
		t.Errorf("qualifier spelling changed: %q", got)
	}
	if c := onlyCall(t, calls); c.Kind != optimizer.CallUDF || c.Identity != "Shop.myfn" {
		t.Errorf("resolution = {%v %q}, want {UDF Shop.myfn}", c.Kind, c.Identity)
	}
}

func TestEngineCatalogPurityAndFailSafe(t *testing.T) {
	dialect := "postgres, opaque_functions=true"
	// Empty catalog: everything Unknown (fail-safe).
	calls, relations, _ := resolveOne(t, dialect, "SELECT abs(x) FROM pg_tables", &optimizer.EngineCatalog{}, []string{"public"})
	for _, c := range calls {
		if c.Kind != optimizer.CallUnknown {
			t.Errorf("empty catalog call = %v, want Unknown", c.Kind)
		}
	}
	for _, r := range relations {
		if r.Kind != optimizer.RelationUnknown {
			t.Errorf("empty catalog relation = %v, want Unknown", r.Kind)
		}
	}
	// Determinism: same query + catalog -> same report.
	a, _, _ := resolveOne(t, dialect, "SELECT abs(x)", pgCatalog(), []string{"public"})
	b, _, _ := resolveOne(t, dialect, "SELECT abs(x)", pgCatalog(), []string{"public"})
	if onlyCall(t, a) != onlyCall(t, b) {
		t.Error("resolution not deterministic")
	}
	// Zero values fail closed.
	var c optimizer.ResolvedCall
	if c.Kind != optimizer.CallUnknown || c.Kind.String() != "Unknown" {
		t.Error("zero ResolvedCall must be Unknown")
	}
	var r optimizer.ResolvedRelation
	if r.Kind != optimizer.RelationUnknown || r.Kind.String() != "Unknown" {
		t.Error("zero ResolvedRelation must be Unknown")
	}

	// Catalog collisions (consumer input error) fail closed, never prefer the trusted class.
	collide := pgCatalog()
	collide.UDFSchemas["pg_catalog"] = names("abs")
	calls, _, _ = resolveOne(t, dialect, "SELECT pg_catalog.abs(x)", collide, nil)
	if got := onlyCall(t, calls); got.Kind != optimizer.CallUnknown {
		t.Errorf("system+udf collision = %v, want Unknown", got.Kind)
	}
	collideRel := pgCatalog()
	collideRel.UserRelations["pg_catalog"] = names("pg_tables")
	_, relations, _ = resolveOne(t, dialect, "SELECT * FROM pg_catalog.pg_tables", collideRel, nil)
	for _, rr := range relations {
		if rr.Kind != optimizer.RelationUnknown {
			t.Errorf("system+user relation collision = %v, want Unknown", rr.Kind)
		}
	}
}

func TestEngineCatalogViaQualify(t *testing.T) {
	// End-to-end through Qualify: reports populate, AST/output unchanged (no rewrite).
	dialect := "postgres, opaque_functions=true"
	sql := "SELECT abs(t.a), myschema.f(t.b) FROM sch.t"
	e, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatal(err)
	}
	catalog := pgCatalog()
	catalog.UDFSchemas["myschema"] = names("f")
	calls := map[exp.Expression]optimizer.ResolvedCall{}
	relations := map[exp.Expression]optimizer.ResolvedRelation{}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = dialect
	opts.QualifyColumns = false
	opts.ValidateQualifyColumns = false
	opts.QuoteIdentifiers = false
	opts.EngineCatalog = catalog
	opts.CallReport = calls
	opts.RelationReport = relations
	out := optimizer.Qualify(e, opts)
	if len(calls) != 2 {
		t.Fatalf("want 2 call entries, got %d", len(calls))
	}
	kinds := map[string]optimizer.CallKind{}
	for node, c := range calls {
		name, _ := node.Arg("this").(string)
		kinds[name] = c.Kind
		_ = c
	}
	if kinds["abs"] != optimizer.CallBuiltin || kinds["f"] != optimizer.CallUDF {
		t.Errorf("kinds = %v, want abs:Builtin f:UDF", kinds)
	}
	got, err := sqlglot.Generate(out, dialect, generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Qualify's own passes may alias-canonicalize (AS t); the resolver itself must not
	// touch call names/qualifiers.
	want := "SELECT abs(t.a), myschema.f(t.b) FROM sch.t AS t"
	if got != want {
		t.Errorf("unexpected qualify output: %q, want %q", got, want)
	}
}
