package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

// intoSQL ports into_sql (generator.py:2724-2727): `INTO [TEMPORARY|UNLOGGED] <this>`, e.g.
// postgres's `SELECT * INTO UNLOGGED foo FROM ...` (unlogged, kinds.go:472). Upstream also
// reads "bulk_collect"/"expressions" (Oracle `SELECT ... BULK COLLECT INTO`), out of scope
// for base/mysql/postgres, so this port renders temporary/unlogged/this, matching every
// base/mysql/postgres round-trip case — plus MySQL's `INTO {OUTFILE|DUMPFILE} '/path'
// [export_options]` file-write forms (kind marker + export args), a grammar extension beyond
// upstream (DEVIATIONS.md "Grammar extensions beyond upstream").
// isOutfileKeyword reports whether kind is one of the MySQL INTO file-write markers.
func isOutfileKeyword(kind string) bool {
	return kind == "OUTFILE" || kind == "DUMPFILE"
}

func (g *Generator) intoSQL(e expressions.Expression) string {
	// MySQL file-write targets: `INTO {OUTFILE|DUMPFILE} '/path' [CHARACTER SET ..] [export_options]`.
	if kind := e.Text("kind"); isOutfileKeyword(kind) {
		out := g.seg("INTO") + " " + kind + " " + g.sqlKey(e, "this")
		if charset := g.sqlKey(e, "charset"); charset != "" {
			out += " CHARACTER SET " + charset
		}
		fieldsKeyword := "FIELDS"
		if boolValue(e.Arg("columns")) {
			fieldsKeyword = "COLUMNS"
		}
		fieldOpts := ""
		if v := g.sqlKey(e, "fields_terminated"); v != "" {
			fieldOpts += " TERMINATED BY " + v
		}
		if boolValue(e.Arg("optionally_enclosed")) {
			fieldOpts += " OPTIONALLY"
		}
		if v := g.sqlKey(e, "enclosed"); v != "" {
			fieldOpts += " ENCLOSED BY " + v
		}
		if v := g.sqlKey(e, "escaped"); v != "" {
			fieldOpts += " ESCAPED BY " + v
		}
		if fieldOpts != "" {
			out += " " + fieldsKeyword + fieldOpts
		}
		lineOpts := ""
		if v := g.sqlKey(e, "lines_starting"); v != "" {
			lineOpts += " STARTING BY " + v
		}
		if v := g.sqlKey(e, "lines_terminated"); v != "" {
			lineOpts += " TERMINATED BY " + v
		}
		if lineOpts != "" {
			out += " LINES" + lineOpts
		}
		return out
	}

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
