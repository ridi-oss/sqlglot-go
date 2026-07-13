package generator

import "github.com/ridi/sqlglot-go/expressions"

// intoSQL ports into_sql (generator.py:2724-2727): `INTO [TEMPORARY|UNLOGGED] <this>`, e.g.
// postgres's `SELECT * INTO UNLOGGED foo FROM ...` (unlogged, kinds.go:472). Upstream also
// reads "bulk_collect"/"expressions" (Oracle `SELECT ... BULK COLLECT INTO`), out of scope
// for base/mysql/postgres, so this port only renders temporary/unlogged/this, matching
// every base/mysql/postgres round-trip case.
func (g *Generator) intoSQL(e expressions.Expression) string {
	temporary := ""
	if boolValue(e.Arg("temporary")) {
		temporary = " TEMPORARY"
	}
	unlogged := ""
	if boolValue(e.Arg("unlogged")) {
		unlogged = " UNLOGGED"
	}
	suffix := temporary
	if suffix == "" {
		suffix = unlogged
	}
	return g.seg("INTO") + suffix + " " + g.sqlKey(e, "this")
}

func init() {
	dispatch[expressions.KindInto] = (*Generator).intoSQL
}
