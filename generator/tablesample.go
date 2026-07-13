package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindTableSample] = (*Generator).tablesampleSQL
}

// tablesampleSQL ports tablesample_sql (generator.py:2464-2491). Upstream's optional
// tablesample_keyword parameter is only ever supplied by the pipe-syntax caller (out of
// scope for this port), so the trailing keyword always falls back to TablesampleKeywords.
func (g *Generator) tablesampleSQL(e expressions.Expression) string {
	method := g.sqlKey(e, "method")
	if method != "" && g.dialect.TablesampleWithMethod {
		method += " "
	} else {
		method = ""
	}
	numerator := g.sqlKey(e, "bucket_numerator")
	denominator := g.sqlKey(e, "bucket_denominator")
	field := g.sqlKey(e, "bucket_field")
	if field != "" {
		field = " ON " + field
	}
	bucket := ""
	if numerator != "" {
		bucket = "BUCKET " + numerator + " OUT OF " + denominator + field
	}
	seed := g.sqlKey(e, "seed")
	if seed != "" {
		seed = " " + g.dialect.TablesampleSeedKeyword + " (" + seed + ")"
	}

	size := g.sqlKey(e, "size")
	if size != "" && g.dialect.TablesampleSizeIsRows {
		size += " ROWS"
	}

	percent := g.sqlKey(e, "percent")
	if percent != "" && !g.dialect.TablesampleSizeIsPercent {
		percent += " PERCENT"
	}

	expr := bucket + percent + size
	if g.dialect.TablesampleRequiresParens {
		expr = "(" + expr + ")"
	}

	return " " + g.dialect.TablesampleKeywords + " " + method + expr + seed
}
