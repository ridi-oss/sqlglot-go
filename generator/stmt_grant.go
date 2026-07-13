package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindGrant] = (*Generator).grantSQL
	dispatch[expressions.KindRevoke] = (*Generator).revokeSQL
	dispatch[expressions.KindGrantPrivilege] = (*Generator).grantPrivilegeSQL
	dispatch[expressions.KindGrantPrincipal] = (*Generator).grantPrincipalSQL
}

// grantOrRevokeSQL ports generator.py:5681-5706 (_grant_or_revoke_sql), the shared
// rendering for exp.Grant and exp.Revoke.
func (g *Generator) grantOrRevokeSQL(e expressions.Expression, keyword, preposition, grantOptionPrefix, grantOptionSuffix string) string {
	privilegesSQL := g.expressions(exprsOptions{expression: e, key: "privileges", flat: true})

	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind = " " + kind
	}

	securable := g.sqlKey(e, "securable")
	if securable != "" {
		securable = " " + securable
	}

	principals := g.expressions(exprsOptions{expression: e, key: "principals", flat: true})

	if !truthy(e.Arg("grant_option")) {
		grantOptionPrefix = ""
		grantOptionSuffix = ""
	}

	// cascade for revoke only
	cascade := g.sqlKey(e, "cascade")
	if cascade != "" {
		cascade = " " + cascade
	}

	return keyword + " " + grantOptionPrefix + privilegesSQL + " ON" + kind + securable + " " + preposition + " " + principals + grantOptionSuffix + cascade
}

// grantSQL ports generator.py:5708-5714 (grant_sql).
func (g *Generator) grantSQL(e expressions.Expression) string {
	return g.grantOrRevokeSQL(e, "GRANT", "TO", "", " WITH GRANT OPTION")
}

// revokeSQL ports generator.py:5716-5722 (revoke_sql).
func (g *Generator) revokeSQL(e expressions.Expression) string {
	return g.grantOrRevokeSQL(e, "REVOKE", "FROM", "GRANT OPTION FOR ", "")
}

// grantPrivilegeSQL ports generator.py:5724-5729 (grantprivilege_sql).
func (g *Generator) grantPrivilegeSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	columns := g.expressions(exprsOptions{expression: e, flat: true})
	if columns != "" {
		columns = "(" + columns + ")"
	}

	return this + columns
}

// grantPrincipalSQL ports generator.py:5731-5737 (grantprincipal_sql).
func (g *Generator) grantPrincipalSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")

	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind = kind + " "
	}

	return kind + this
}
