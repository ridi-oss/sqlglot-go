package optimizer

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// BigQuery engine identity rules: function names are case-insensitive (ASCII-lower); dataset
// and table names in a path keep their case; CTE names fold.

func asciiLowerName(name string) string {
	b := []byte(name)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// bigqueryTempFunctions collects the CREATE TEMP FUNCTION names defined in the statement.
func bigqueryTempFunctions(expression exp.Expression) map[string]bool {
	names := map[string]bool{}
	for _, create := range expression.FindAll(exp.KindCreate) {
		if !strings.EqualFold(create.Text("kind"), "FUNCTION") || create.Find(exp.KindTemporaryProperty) == nil {
			continue
		}
		if udf := create.This(); udf != nil {
			// BigQuery rejects a qualified temp function name, so a schema here only comes from
			// Qualify stamping the default dataset.
			if target := udf.This(); target != nil && target.Kind() == exp.KindTable {
				names[target.Name()] = true
			}
		}
	}
	return names
}

// pathParts flattens a call qualifier (Identifier or Dot of Identifiers) into names; ok is false
// for anything else.
func pathParts(e exp.Expression) ([]exp.Expression, bool) {
	switch e.Kind() {
	case exp.KindIdentifier:
		return []exp.Expression{e}, true
	case exp.KindDot:
		left, ok := pathParts(e.This())
		if !ok || e.Expr() == nil || e.Expr().Kind() != exp.KindIdentifier {
			return nil, false
		}
		return append(left, e.Expr()), true
	}
	return nil, false
}

// resolveBigQueryCall classifies an Anonymous call: an unqualified name is a temp UDF of the
// statement or a builtin; SAFE.f resolves like f; NET.f and ns.f are builtin when the dotted
// lowercase name is in BuiltinFunctions; ds.f / proj.ds.f is a persistent UDF when
// UDFSchemas[ds][f] holds with f as written and proj (if written) is DefaultCatalog. Anything else is Unknown.
func (r *engineResolver) resolveBigQueryCall(node exp.Expression) ResolvedCall {
	var names []string
	switch v := node.Arg("this").(type) {
	case string:
		names = []string{v}
	case exp.Expression:
		if v == nil || v.Kind() != exp.KindIdentifier {
			return ResolvedCall{}
		}
		// A backticked path such as `proj.ds.f` names the parts in one identifier.
		names = strings.Split(v.Name(), ".")
	default:
		return ResolvedCall{}
	}
	var qualifier []string
	if parent := node.Parent(); parent != nil {
		switch parent.Kind() {
		case exp.KindSafeFunc:
			if len(names) > 1 {
				return ResolvedCall{}
			}
		case exp.KindNetFunc:
			qualifier = []string{"net"}
		case exp.KindDot:
			// `f(x).a` is a field access on the call's result: f stays unqualified.
			if asExpression(parent.Arg("expression")) == node {
				// `FROM a.p.ds.f()` puts the leading parts on an enclosing Table: unsupported.
				if grand := parent.Parent(); grand != nil && grand.Kind() == exp.KindTable {
					return ResolvedCall{}
				}
				parts, ok := pathParts(parent.This())
				if !ok {
					return ResolvedCall{}
				}
				for _, part := range parts {
					qualifier = append(qualifier, part.Name())
				}
			}
		case exp.KindTable:
			// A table function `FROM [p.]ds.f(...)` carries its path in the Table's parts.
			if parent.This() == node {
				for _, key := range []string{"catalog", "schema"} {
					if value := parent.Arg(key); value != nil {
						part := asExpression(value)
						if part == nil || part.Kind() != exp.KindIdentifier {
							return ResolvedCall{}
						}
						qualifier = append(qualifier, part.Name())
					}
				}
			}
		}
	}
	if len(names) > 1 && len(qualifier) > 0 {
		// A backticked dotted name under a written qualifier, e.g. p.`ds.f`(): unsupported.
		return ResolvedCall{}
	}
	qualifier = append(qualifier, names[:len(names)-1]...)
	// BigQuery: built-in function names are case-insensitive, UDF names (temp and persistent)
	// case-sensitive (GoogleSQL lexical structure, "Case sensitivity").
	written := names[len(names)-1]
	name := asciiLowerName(written)
	if name == "" || !identityComponentOK(name) {
		return ResolvedCall{}
	}
	for _, q := range qualifier {
		if q == "" || !identityComponentOK(q) {
			return ResolvedCall{}
		}
	}

	switch len(qualifier) {
	case 0:
		if r.tempFunctions[written] {
			return ResolvedCall{Kind: CallUDF, Identity: written}
		}
		if r.catalog.BuiltinFunctions[name] {
			return ResolvedCall{Kind: CallBuiltin, Identity: name}
		}
	case 1:
		if dotted := asciiLowerName(qualifier[0]) + "." + name; r.catalog.BuiltinFunctions[dotted] {
			return ResolvedCall{Kind: CallBuiltin, Identity: dotted}
		}
		if r.catalog.UDFSchemas[qualifier[0]][written] {
			return ResolvedCall{Kind: CallUDF, Identity: qualifier[0] + "." + written}
		}
	case 2:
		if qualifier[0] == r.catalog.DefaultCatalog && r.catalog.UDFSchemas[qualifier[1]][written] {
			return ResolvedCall{Kind: CallUDF, Identity: qualifier[1] + "." + written}
		}
	}
	return ResolvedCall{}
}

// resolveBigQueryRelation follows the Athena three-level rule (DefaultCatalog = job project,
// CurrentDatabase = default dataset) with BigQuery folding: path parts keep their case and an
// unqualified name matching a CTE is Unknown.
func (r *engineResolver) resolveBigQueryRelation(node exp.Expression) ResolvedRelation {
	name := node.This().Name()
	if name == "" || !identityComponentOK(name) {
		return ResolvedRelation{}
	}
	if node.Arg("schema") == nil && r.ctes[asciiLowerName(name)] {
		return ResolvedRelation{}
	}
	parts := []string{r.catalog.DefaultCatalog, r.catalog.CurrentDatabase}
	for i, key := range []string{"catalog", "schema"} {
		if value := node.Arg(key); value != nil {
			identifier := asExpression(value)
			if identifier == nil || identifier.Kind() != exp.KindIdentifier {
				return ResolvedRelation{}
			}
			parts[i] = identifier.Name()
		}
		if parts[i] == "" || !identityComponentOK(parts[i]) {
			return ResolvedRelation{}
		}
	}
	project, dataset := parts[0], parts[1]
	system := r.catalog.CatalogSystemRelations[project][dataset][name]
	user := r.catalog.CatalogUserRelations[project][dataset][name]
	identity := project + "." + dataset + "." + name
	switch {
	case system && user:
		return ResolvedRelation{}
	case system:
		return ResolvedRelation{Kind: SystemRelation, Identity: identity}
	case user:
		return ResolvedRelation{Kind: UserRelation, Identity: identity}
	}
	return ResolvedRelation{}
}
