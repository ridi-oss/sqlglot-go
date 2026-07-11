package dialects

import "strings"

// MySQLLower folds a string exactly as MySQL folds identifiers for case-insensitive
// resolution: per code point via MySQL's my_unicase_default `.tolower` map (see
// mysql_casefold_table.go, generated from the MySQL source). The fold is
// Unicode-simple and accent-PRESERVING — É->é, but Ñ stays distinct from N and ß is
// unchanged — and covers only the BMP (utf8mb3); any code point without a table entry
// (incl. all non-BMP runes) is left unchanged.
//
// This is the MySQL-compatible identifier fold. It is EXPORTED so a consumer (e.g. a
// JVM service via a native binding) can call this exact implementation rather than
// re-deriving it: neither Go's strings.ToLower nor the JVM's String.lowercase()
// reproduces this map, and they diverge from each other on characters like U+0130 and
// Greek final-sigma (see DEVIATIONS.md §1.2). Do NOT substitute a stdlib case function.
func MySQLLower(s string) string {
	var b strings.Builder
	changed := false
	for i, r := range s {
		if lo, ok := mysqlLowerMap[r]; ok && lo != r {
			if !changed {
				b.Grow(len(s))
				b.WriteString(s[:i])
				changed = true
			}
			b.WriteRune(lo)
		} else if changed {
			b.WriteRune(r)
		}
	}
	if !changed {
		return s
	}
	return b.String()
}
