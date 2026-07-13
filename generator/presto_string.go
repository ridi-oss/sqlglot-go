package generator

import "github.com/ridi/sqlglot-go/expressions"

// unicodeStringSQL ports unicodestring_sql (generator.py:1630) for Presto, whose
// UNICODE_START/UNICODE_END are "U&'" and "'" (see expressions/query.py:494 UnicodeString
// and dialects/presto.py). Presto has UNICODE_START set, so the reference takes the
// left_quote/right_quote branch and emits `U&'<this>'` with the raw text passed through
// verbatim (no \uXXXX substitution). The UESCAPE clause is deferred.
func (g *Generator) unicodeStringSQL(e expressions.Expression) string {
	return "U&'" + e.Name() + "'"
}

func init() {
	dispatch[expressions.KindUnicodeString] = (*Generator).unicodeStringSQL
}
