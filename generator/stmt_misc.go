package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindUse] = (*Generator).useSQL
	dispatch[expressions.KindKill] = (*Generator).killSQL
	dispatch[expressions.KindDescribe] = (*Generator).describeSQL
	dispatch[expressions.KindLoadData] = (*Generator).loadDataSQL
}

// useSQL ports use_sql (generator.py:4550-4555).
func (g *Generator) useSQL(e expressions.Expression) string {
	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind = " " + kind
	}
	this := g.sqlKey(e, "this")
	if this == "" {
		this = g.expressions(exprsOptions{expression: e, flat: true})
	}
	if this != "" {
		this = " " + this
	}
	return "USE" + kind + this
}

// killSQL ports kill_sql (generator.py:2309-2314).
func (g *Generator) killSQL(e expressions.Expression) string {
	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind = " " + kind
	}
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	return "KILL" + kind + this
}

// describeSQL ports describe_sql (generator.py:1499-1508).
func (g *Generator) describeSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" && e.Text("kind") == "EXPLAIN" {
		// This ledgered Postgres extension intentionally bypasses FileFormatProperty formatting.
		wrapped := boolValue(e.Arg("wrapped"))
		sep := " "
		if wrapped {
			sep = ", "
		}
		options := g.expressions(exprsOptions{expression: e, flat: true, sep: sep})
		if options != "" {
			if wrapped {
				options = " (" + options + ")"
			} else {
				options = " " + options
			}
		}
		return "EXPLAIN" + options + " " + g.sqlKey(e, "this")
	}

	style := g.sqlKey(e, "style")
	if style != "" {
		style = " " + style
	}
	partition := g.sqlKey(e, "partition")
	if partition != "" {
		partition = " " + partition
	}
	format := g.sqlKey(e, "format")
	if format != "" {
		format = " " + format
	}
	asJSON := ""
	if boolValue(e.Arg("as_json")) {
		asJSON = " AS JSON"
	}
	// MySQL `DESCRIBE tbl_name [col_name | wild]` — the column/wildcard filter renders right
	// after the table (see parseDescribeStructured).
	column := g.sqlKey(e, "column")
	if column != "" {
		column = " " + column
	}
	return "DESCRIBE" + style + format + " " + g.sqlKey(e, "this") + column + partition + asJSON
}

// loadDataSQL ports loaddata_sql (generator.py:3007-3033), minus the `files`
// branch (generator.py:3012-3022): parseLoad never sets "files" (see
// parser/stmt_misc.go), so that branch is unreachable here.
func (g *Generator) loadDataSQL(e expressions.Expression) string {
	overwrite := ""
	if boolValue(e.Arg("overwrite")) {
		overwrite = " OVERWRITE"
	}
	local := ""
	if boolValue(e.Arg("local")) {
		local = " LOCAL"
	}
	inpath := " INPATH " + g.sqlKey(e, "inpath")
	this := " INTO TABLE " + g.sqlKey(e, "this")
	partition := g.sqlKey(e, "partition")
	if partition != "" {
		partition = " " + partition
	}
	inputFormat := g.sqlKey(e, "input_format")
	if inputFormat != "" {
		inputFormat = " INPUTFORMAT " + inputFormat
	}
	serde := g.sqlKey(e, "serde")
	if serde != "" {
		serde = " SERDE " + serde
	}
	return "LOAD DATA" + local + inpath + overwrite + this + partition + inputFormat + serde
}
