package generator

import (
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

var sqlNameOverrides = map[expressions.Kind]string{
	expressions.KindPow:                "POWER",
	expressions.KindDateDiff:           "DATEDIFF",
	expressions.KindMD5:                "MD5",
	expressions.KindJSONExtract:        "JSON_EXTRACT",
	expressions.KindJSONExtractScalar:  "JSON_EXTRACT_SCALAR",
	expressions.KindJSONBExtract:       "JSONB_EXTRACT",
	expressions.KindJSONBExtractScalar: "JSONB_EXTRACT_SCALAR",
	// presto cluster: acronym/multi-capital class names whose upstream _sql_names[0]
	// diverges from the camelToSnake split (which would emit J_S_O_N_FORMAT etc.). These
	// mirror the classes' explicit _sql_names (verified against .reference base-write):
	// JSONFormat->JSON_FORMAT (json.py:145), MD5Digest->MD5_DIGEST (string.py:540),
	// SHA2->SHA2 (string.py:562), DayOfWeekIso->DAYOFWEEK_ISO (temporal.py:217).
	expressions.KindJSONFormat:   "JSON_FORMAT",
	expressions.KindMD5Digest:    "MD5_DIGEST",
	expressions.KindSHA2:         "SHA2",
	expressions.KindDayOfWeekIso: "DAYOFWEEK_ISO",
}

func (g *Generator) sqlName(kind expressions.Kind) string {
	if name, ok := sqlNameOverrides[kind]; ok {
		return name
	}
	return camelToSnake(expressions.ClassName(kind))
}

func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}
