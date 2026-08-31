package optimizer

import (
	"strings"

	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// EngineCatalog is the NON-UPSTREAM, opt-in identity catalog for function/relation resolution
// (DEVIATIONS: opaque_functions companion). It is a PURE consumer input, introspected from the
// live target instance — sqlglot-go ships no name sets, only the resolution algorithm. All name
// keys are expected pre-folded: function names lowercase (function identifiers are
// case-insensitive in both engines), relation names folded per the dialect's strategy.
// A name missing from the catalog resolves Unknown (fail-closed).
type EngineCatalog struct {
	// BuiltinFunctions is the priority builtin name set: MySQL natives (builtin > loadable >
	// stored — unshadowable), or PG's pg_catalog functions (implicitly first on the search
	// path unless pg_catalog is explicitly listed).
	BuiltinFunctions map[string]bool
	// SystemFunctionSchemas classifies QUALIFIED calls: schema -> function names
	// (e.g. pg_catalog, information_schema).
	SystemFunctionSchemas map[string]map[string]bool
	// UDFSchemas is the user function catalog: schema (PG) / database (MySQL) -> names.
	UDFSchemas map[string]map[string]bool
	// LoadableFunctions is MySQL's loadable (plugin/UDF) function tier: global names ranking
	// below natives but above stored functions. Classified CallUDF (consumer-installed code,
	// grant-gated), never CallBuiltin.
	LoadableFunctions map[string]bool
	// SystemRelations: schema -> system relation names (pg_catalog/information_schema tables
	// and views; MySQL's mysql/information_schema/performance_schema/sys).
	SystemRelations map[string]map[string]bool
	// UserRelations is the user table/view catalog: schema (PG) / database (MySQL) -> names.
	// A qualified or path-resolved relation found in NEITHER SystemRelations nor UserRelations
	// is Unknown (fail-closed) — never asserted UserRelation without catalog evidence.
	UserRelations map[string]map[string]bool
	// TempRelations names the session's PG temporary relations. The engine searches the
	// implicit temporary schema BEFORE pg_catalog for relations (not functions), so a temp
	// table shadows a same-named system view — a consumer that cannot track temp objects must
	// deny temp-creating statements upstream or accept that shadow as unmodeled.
	TempRelations map[string]bool
	// CurrentDatabase scopes MySQL's unqualified stored functions and relations, which resolve
	// against the current database only.
	CurrentDatabase string
}

// CallKind classifies a resolved function call. Unknown is the zero value (fail-closed).
type CallKind int

const (
	CallUnknown CallKind = iota
	CallBuiltin
	CallUDF
)

func (k CallKind) String() string {
	switch k {
	case CallBuiltin:
		return "Builtin"
	case CallUDF:
		return "UDF"
	default:
		return "Unknown"
	}
}

// RelationKind classifies a resolved relation. Unknown is the zero value (fail-closed).
type RelationKind int

const (
	RelationUnknown RelationKind = iota
	SystemRelation
	UserRelation
)

func (k RelationKind) String() string {
	switch k {
	case SystemRelation:
		return "SystemRelation"
	case UserRelation:
		return "UserRelation"
	default:
		return "Unknown"
	}
}

// ResolvedCall reports one function call's identity. Identity is the canonical
// "schema.name" (folded) — the hard contract consumers key policy on — empty when Unknown.
type ResolvedCall struct {
	Kind     CallKind
	Identity string
}

// ResolvedRelation reports one relation's identity ("schema.name", folded; empty when Unknown).
type ResolvedRelation struct {
	Kind     RelationKind
	Identity string
}

// ResolveEngineIdentities classifies every Anonymous function call and every Table reference in
// the expression against the consumer-supplied catalog, writing into the non-nil report maps
// (keyed by the Anonymous / Table node). The AST is never modified. Through Qualify it runs AFTER
// the table passes, so a DefaultSchema stamp (applied without existence proof) becomes the
// relation's qualifier before classification — a stamped name absent from the catalogs reports
// Unknown; a consumer wanting engine-true relation resolution should pass SearchPath (proof-based)
// rather than DefaultSchema, or resolve on the unqualified AST. The per-dialect rules are the
// verified engine algorithms (see DEVIATIONS): MySQL native-priority + current-database scoping;
// PG search_path union with pg_catalog implicitly first unless explicitly listed, with the
// type-dependent residue (a name in more than one candidate schema) fail-closed to Unknown.
func ResolveEngineIdentities(expression exp.Expression, catalog *EngineCatalog, dialect dialects.DialectType, searchPath []string, calls map[exp.Expression]ResolvedCall, relations map[exp.Expression]ResolvedRelation) {
	if expression == nil || catalog == nil || (calls == nil && relations == nil) {
		return
	}
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		panic(err)
	}
	name := strings.ToLower(d.Name)
	supported := name == "mysql" || name == "postgres"
	r := &engineResolver{catalog: catalog, dialect: d, searchPath: searchPath, ctes: cteNames(expression)}
	for _, node := range expression.Walk() {
		switch node.Kind() {
		case exp.KindAnonymous:
			if calls != nil {
				if !supported {
					// Only the verified MySQL/PG algorithms are modeled — any other dialect
					// fails closed to Unknown rather than borrowing PG semantics.
					calls[node] = ResolvedCall{}
					continue
				}
				calls[node] = r.resolveCall(node)
			}
		case exp.KindTable:
			if relations != nil && node.This() != nil && node.This().Kind() == exp.KindIdentifier {
				if !supported {
					relations[node] = ResolvedRelation{}
					continue
				}
				relations[node] = r.resolveRelation(node)
			}
		}
	}
}

type engineResolver struct {
	catalog    *EngineCatalog
	dialect    *dialects.Dialect
	searchPath []string
	ctes       map[string]bool
}

func (r *engineResolver) isMySQL() bool { return strings.ToLower(r.dialect.Name) == "mysql" }

func quotedFlag(value any) bool {
	b, _ := value.(bool)
	return b
}

// foldFunctionName folds an UNQUOTED function name engine-true: MySQL routine/native names are
// case-insensitive regardless of lower_case_table_names (MySQLLower); PG folds per the dialect
// strategy (ASCII-only lower by default — never strings.ToLower, which over-folds non-ASCII).
func (r *engineResolver) foldFunctionName(name string) string {
	if r.isMySQL() {
		return dialects.MySQLLower(name)
	}
	return r.dialect.FoldIdentifierName(name, false)
}

// foldSchemaQualifier folds an UNQUOTED schema/database qualifier engine-true: relation-role
// folding (MySQL lctn=0 keeps database names case-sensitive; PG ASCII-lowers).
func (r *engineResolver) foldSchemaQualifier(name string) string {
	return r.dialect.FoldIdentifierName(name, true)
}

// identityComponentOK rejects components that would make the canonical dot-joined identity
// ambiguous ("a.b"."c" vs "a"."b.c") — such names fail closed to Unknown.
func identityComponentOK(name string) bool {
	return !strings.ContainsAny(name, ".\"")
}

// callName extracts an Anonymous call's lookup name: unquoted names fold engine-true via
// foldFunctionName; quoted names are verbatim. ok is false for a non-string, non-Identifier
// "this" (never built by the parser).
func (r *engineResolver) callName(node exp.Expression) (string, bool) {
	switch v := node.Arg("this").(type) {
	case string:
		return r.foldFunctionName(v), true
	case exp.Expression:
		if v != nil && v.Kind() == exp.KindIdentifier {
			if quotedFlag(v.Arg("quoted")) {
				return v.Name(), true
			}
			return r.foldFunctionName(v.Name()), true
		}
	}
	return "", false
}

// callQualifier returns the single schema qualifier of Dot(Identifier(schema), Anonymous), "" for
// an unqualified call, and ok=false for a deeper/non-identifier qualifier chain (fail-closed).
func (r *engineResolver) callQualifier(node exp.Expression) (string, bool) {
	parent := node.Parent()
	if parent == nil || parent.Kind() != exp.KindDot || asExpression(parent.Arg("expression")) != node {
		return "", true
	}
	qualifier := parent.This()
	if qualifier == nil || qualifier.Kind() != exp.KindIdentifier {
		return "", false
	}
	if quotedFlag(qualifier.Arg("quoted")) {
		return qualifier.Name(), true
	}
	return r.foldSchemaQualifier(qualifier.Name()), true
}

func (r *engineResolver) resolveCall(node exp.Expression) ResolvedCall {
	name, ok := r.callName(node)
	if !ok || name == "" || !identityComponentOK(name) {
		return ResolvedCall{}
	}
	qualifier, ok := r.callQualifier(node)
	if !ok || !identityComponentOK(qualifier) {
		return ResolvedCall{}
	}

	if qualifier != "" {
		system := r.catalog.SystemFunctionSchemas[qualifier][name]
		udf := r.catalog.UDFSchemas[qualifier][name]
		if system && udf {
			// Catalog collision: the same qualifier.name in both maps is a consumer input
			// error — fail closed rather than prefer the trusted class.
			return ResolvedCall{}
		}
		if system {
			return ResolvedCall{Kind: CallBuiltin, Identity: qualifier + "." + name}
		}
		if udf {
			return ResolvedCall{Kind: CallUDF, Identity: qualifier + "." + name}
		}
		return ResolvedCall{}
	}

	if r.isMySQL() {
		// Natives have priority and are unshadowable: builtin > loadable > stored.
		if r.catalog.BuiltinFunctions[name] {
			return ResolvedCall{Kind: CallBuiltin, Identity: name}
		}
		// Loadable (plugin) functions rank below natives, above stored; global namespace.
		if r.catalog.LoadableFunctions[name] {
			return ResolvedCall{Kind: CallUDF, Identity: name}
		}
		// Unqualified stored functions resolve against the current database only.
		if db := r.catalog.CurrentDatabase; db != "" && r.catalog.UDFSchemas[db][name] {
			return ResolvedCall{Kind: CallUDF, Identity: db + "." + name}
		}
		return ResolvedCall{}
	}

	// PG/ANSI: candidates are unioned across the effective search path (pg_catalog implicitly
	// FIRST unless explicitly listed); with a name-level catalog, more than one candidate schema
	// means the winner is type-dependent — fail-closed to Unknown.
	type candidate struct {
		kind     CallKind
		identity string
	}
	var candidates []candidate
	pgCatalogListed := false
	for _, schema := range r.searchPath {
		if strings.EqualFold(schema, "pg_catalog") {
			pgCatalogListed = true
		}
	}
	pgCatalogCandidates := func() {
		// A pg_catalog builtin/UDF-map collision is a consumer input error: emit both
		// candidates so the >1 rule fails the call closed.
		if r.catalog.BuiltinFunctions[name] {
			candidates = append(candidates, candidate{CallBuiltin, "pg_catalog." + name})
		}
		if r.catalog.UDFSchemas["pg_catalog"][name] {
			candidates = append(candidates, candidate{CallUDF, "pg_catalog." + name})
		}
	}
	if !pgCatalogListed {
		pgCatalogCandidates()
	}
	for _, schema := range r.searchPath {
		if strings.EqualFold(schema, "pg_catalog") {
			pgCatalogCandidates()
			continue
		}
		if r.catalog.UDFSchemas[schema][name] {
			candidates = append(candidates, candidate{CallUDF, schema + "." + name})
		}
	}
	if len(candidates) == 1 {
		return ResolvedCall{Kind: candidates[0].kind, Identity: candidates[0].identity}
	}
	return ResolvedCall{}
}

// relationName folds a table identifier for catalog lookup: quoted verbatim, unquoted via the
// dialect's relation-role folding.
func (r *engineResolver) relationName(identifier exp.Expression) string {
	if quotedFlag(identifier.Arg("quoted")) {
		return identifier.Name()
	}
	return r.dialect.FoldIdentifierName(identifier.Name(), true)
}

// cteNames collects every CTE alias in the statement. A table reference matching ANY CTE alias is
// fail-closed to Unknown rather than classified against the engine catalog — over-deny beats
// misattributing a WITH-local source to a catalog identity (precise per-scope visibility would
// need traverseScope; not worth an over-allow risk here).
func cteNames(expression exp.Expression) map[string]bool {
	names := map[string]bool{}
	for _, cte := range expression.FindAll(exp.KindCTE) {
		if alias := asExpression(cte.Arg("alias")); alias != nil && alias.This() != nil {
			names[alias.This().Name()] = true
		}
	}
	return names
}

func (r *engineResolver) resolveRelation(node exp.Expression) ResolvedRelation {
	name := r.relationName(node.This())
	if name == "" || !identityComponentOK(name) {
		return ResolvedRelation{}
	}
	if node.Arg("catalog") != nil {
		// Three-part catalog.schema.table: the catalog input has no catalog dimension —
		// fail-closed rather than silently dropping the first qualifier.
		return ResolvedRelation{}
	}
	if r.ctes[name] {
		return ResolvedRelation{}
	}
	schemaName := ""
	if s := asExpression(node.Arg("schema")); s != nil && s.Kind() == exp.KindIdentifier {
		schemaName = r.relationName(s)
		if !identityComponentOK(schemaName) {
			return ResolvedRelation{}
		}
	} else if node.Arg("schema") != nil {
		return ResolvedRelation{} // non-identifier qualifier: fail-closed
	}

	if schemaName != "" {
		return r.relationInSchema(schemaName, name)
	}

	if r.isMySQL() {
		// Unqualified relations resolve against the current database only.
		if db := r.catalog.CurrentDatabase; db != "" {
			return r.relationInSchema(db, name)
		}
		return ResolvedRelation{}
	}

	// PG: relations have no overloads, so first match on the effective path wins — the implicit
	// temporary schema is searched FIRST (relations only, not functions), then pg_catalog unless
	// explicitly listed. Both catalogs are checked per schema so a user table earlier on the path
	// shadows a system name later on it, exactly like the engine.
	if r.catalog.TempRelations[name] {
		return ResolvedRelation{Kind: UserRelation, Identity: "pg_temp." + name}
	}
	pgCatalogListed := false
	for _, schema := range r.searchPath {
		if strings.EqualFold(schema, "pg_catalog") {
			pgCatalogListed = true
		}
	}
	if !pgCatalogListed {
		if hit := r.relationInSchema("pg_catalog", name); hit.Kind != RelationUnknown {
			return hit
		}
	}
	for _, schema := range r.searchPath {
		if hit := r.relationInSchema(schema, name); hit.Kind != RelationUnknown {
			return hit
		}
	}
	return ResolvedRelation{}
}

// relationInSchema classifies one schema.name against the catalogs: system, user, a collision in
// both fails closed, and a name in neither is Unknown — never asserted UserRelation without
// catalog evidence.
func (r *engineResolver) relationInSchema(schemaName, name string) ResolvedRelation {
	system := r.catalog.SystemRelations[schemaName][name]
	user := r.catalog.UserRelations[schemaName][name]
	if system && user {
		return ResolvedRelation{}
	}
	if system {
		return ResolvedRelation{Kind: SystemRelation, Identity: schemaName + "." + name}
	}
	if user {
		return ResolvedRelation{Kind: UserRelation, Identity: schemaName + "." + name}
	}
	return ResolvedRelation{}
}
