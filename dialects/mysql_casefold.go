package dialects

import (
	_ "embed"
	"strconv"
	"strings"
)

// mysql_casefold.tsv is the canonical, byte-exact MySQL identifier case-fold
// table (MySQL 8.0 my_unicase_default `.tolower`, utf8mb3_general_ci =
// system_charset_info — the collation MySQL resolves identifiers under). It is
// a simple 1:1, accent-PRESERVING, BMP-only lowercase: É->é but Ñ!=N and ß
// unchanged. It is the SINGLE SOURCE OF TRUTH shared with any consumer (e.g. the
// JVM proxy), which must embed the identical file so the normalized identifier
// is byte-for-byte the same across languages — neither Go's strings.ToLower nor
// the JVM's lowercase() reproduces this map (they diverge; see DEVIATIONS.md).
// Regenerate via scripts/gen_mysql_casefold.py against the pinned MySQL source.
//
//go:embed mysql_casefold.tsv
var mysqlCasefoldTSV string

// mysqlLowerMap maps a code point to its MySQL lowercase, only where they differ.
var mysqlLowerMap = func() map[rune]rune {
	m := make(map[rune]rune, 700)
	for _, line := range strings.Split(mysqlCasefoldTSV, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cp, lo, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		c, err1 := strconv.ParseInt(strings.TrimSpace(cp), 16, 32)
		l, err2 := strconv.ParseInt(strings.TrimSpace(lo), 16, 32)
		if err1 == nil && err2 == nil {
			m[rune(c)] = rune(l)
		}
	}
	return m
}()

// mysqlLower folds an identifier exactly as MySQL folds identifiers for
// case-insensitive resolution: per code point via the MySQL .tolower table
// (accent-preserving; leaves a code point unchanged when the table has no entry
// for it, incl. all non-BMP runes). This is the MySQL-compatible fold to use for
// any MySQL folding strategy — NOT strings.ToLower.
func mysqlLower(s string) string {
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
