package generator

import (
	"strconv"
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

// subsecondPrecision ports subsecond_precision (time.py:668-688): given an ISO-8601 timestamp
// literal (e.g. "2023-01-01 12:13:14.123456+00:00"), returns its subsecond precision bucketed to
// 0, 3 or 6 so callers can construct types like DATETIME(6). Upstream leans on
// datetime.datetime.fromisoformat both to validate the literal (any ValueError -> 0) and to read
// back parsed.microsecond; Go has no equivalent, so isoMicroseconds faithfully reproduces
// fromisoformat's grammar plus range/calendar validation, and the bucketing mirrors
// len(str(parsed.microsecond).rstrip("0")).
func subsecondPrecision(literal string) int {
	micro, ok := isoMicroseconds(literal)
	if !ok {
		return 0
	}
	digitCount := len(strings.TrimRight(strconv.Itoa(micro), "0"))
	switch {
	case digitCount > 3:
		return 6
	case digitCount > 0:
		return 3
	default:
		return 0
	}
}

// isoMicroseconds validates s as an ISO-8601 datetime the way Python 3.11+'s
// datetime.datetime.fromisoformat does and returns its microsecond component (fractional seconds
// scaled to 6 digits, truncating extras). ok is false for any literal fromisoformat would reject
// with ValueError - including out-of-range or non-calendar dates (2023-99-99, 2023-02-30),
// unpadded fields (2023-1-1) and mismatched basic/extended separators (2023-0101). ISO week dates
// (YYYY-Www-D) are intentionally unsupported: they never appear as SQL timestamp literals.
func isoMicroseconds(s string) (int, bool) {
	rest, ok := isoParseDate(s)
	if !ok {
		return 0, false
	}
	if rest == "" {
		return 0, true // date-only literal: no fractional seconds
	}
	// A single separator character (commonly 'T' or ' ', but fromisoformat accepts any) joins
	// the date to the time-of-day.
	return isoParseTime(rest[1:])
}

// isoParseDate consumes a leading ISO date - extended YYYY-MM-DD or basic YYYYMMDD, with the two
// separators either both present or both absent - validates it against the Gregorian calendar and
// returns the unconsumed remainder.
func isoParseDate(s string) (string, bool) {
	year, s, ok := isoDigits(s, 4)
	if !ok {
		return "", false
	}
	extended := strings.HasPrefix(s, "-")
	if extended {
		s = s[1:]
	}
	month, s, ok := isoDigits(s, 2)
	if !ok {
		return "", false
	}
	if extended {
		if !strings.HasPrefix(s, "-") {
			return "", false
		}
		s = s[1:]
	}
	day, s, ok := isoDigits(s, 2)
	if !ok {
		return "", false
	}
	if year < 1 || month < 1 || month > 12 || day < 1 || day > daysInMonth(year, month) {
		return "", false
	}
	return s, true
}

// isoParseTime validates an ISO time-of-day (basic HHMMSS or extended HH:MM:SS, optionally
// truncated after the hour or minute) followed by an optional UTC offset, requiring the whole
// string to be consumed, and returns the microsecond component of any fractional seconds.
func isoParseTime(s string) (int, bool) {
	// The offset is introduced by '+', '-' or a bare 'Z'; none can occur inside the
	// time-of-day, so the first such character splits the clock from the offset.
	clock, offset := s, ""
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '+' || c == '-' || c == 'Z' {
			clock, offset = s[:i], s[i:]
			break
		}
	}
	micro, ok := isoParseClock(clock)
	if !ok || !isoValidOffset(offset) {
		return 0, false
	}
	return micro, true
}

// isoParseClock parses HH[:MM[:SS[.frac]]] (basic or extended) and returns the microsecond value
// of the fractional part, if any.
func isoParseClock(s string) (int, bool) {
	hour, s, ok := isoDigits(s, 2)
	if !ok || hour > 23 {
		return 0, false
	}
	if s == "" {
		return 0, true
	}
	extended := strings.HasPrefix(s, ":")
	if extended {
		s = s[1:]
	}
	minute, s, ok := isoDigits(s, 2)
	if !ok || minute > 59 {
		return 0, false
	}
	if s == "" {
		return 0, true
	}
	if extended {
		if !strings.HasPrefix(s, ":") {
			return 0, false
		}
		s = s[1:]
	}
	second, s, ok := isoDigits(s, 2)
	if !ok || second > 59 {
		return 0, false
	}
	if s == "" {
		return 0, true
	}
	// Fractional seconds: '.' or ',' then 1+ digits, scaled/truncated to a 6-digit microsecond.
	if s[0] != '.' && s[0] != ',' {
		return 0, false
	}
	frac := s[1:]
	if !isoAllDigits(frac) {
		return 0, false
	}
	if len(frac) > 6 {
		frac = frac[:6]
	} else {
		frac += strings.Repeat("0", 6-len(frac))
	}
	micro, _ := strconv.Atoi(frac)
	return micro, true
}

// isoValidOffset validates an optional ISO UTC offset: "" (none), "Z", or a signed
// +HH[[:]MM[[:]SS[.f+]]] whose fields are in range.
func isoValidOffset(s string) bool {
	if s == "" || s == "Z" {
		return true
	}
	if s[0] != '+' && s[0] != '-' {
		return false
	}
	s = s[1:]
	hour, s, ok := isoDigits(s, 2)
	if !ok || hour > 23 {
		return false
	}
	if s == "" {
		return true
	}
	extended := strings.HasPrefix(s, ":")
	if extended {
		s = s[1:]
	}
	minute, s, ok := isoDigits(s, 2)
	if !ok || minute > 59 {
		return false
	}
	if s == "" {
		return true
	}
	if extended {
		if !strings.HasPrefix(s, ":") {
			return false
		}
		s = s[1:]
	}
	second, s, ok := isoDigits(s, 2)
	if !ok || second > 59 {
		return false
	}
	if s == "" {
		return true
	}
	if s[0] != '.' && s[0] != ',' {
		return false
	}
	return isoAllDigits(s[1:])
}

// isoDigits consumes exactly n leading decimal digits, returning their value and the remainder.
func isoDigits(s string, n int) (int, string, bool) {
	if len(s) < n || !isoAllDigits(s[:n]) {
		return 0, s, false
	}
	v, _ := strconv.Atoi(s[:n])
	return v, s[n:], true
}

func isoAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	}
	return 0
}

// timeStrToTimeSQL ports timestrtotime_sql (dialects/dialect.py:1729-1744): renders
// exp.TimeStrToTime as a CAST of its "this" to TIMESTAMP (or TIMESTAMPTZ, if a "zone" arg is
// set), optionally sized to the literal's subsecond precision. The zone-set/TIMESTAMPTZ branch
// and the CAST-target-name remap (TIMESTAMP -> DATETIME) are handled entirely by the existing
// mysql castSQLWithPrefix/dataTypeSQL machinery once the Cast/DataType node is built here - see
// generators/mysql.py:689-697's TIMESTAMP_FUNC_TYPES/CAST_MAPPING, already ported at
// generator/sql.go's mysqlTimestampFuncTypes/mysqlCastMapping.
func timeStrToTimeSQL(g *Generator, e expressions.Expression, includePrecision bool) string {
	dtype := expressions.DTypeTimestamp
	if truthy(e.Arg("zone")) {
		dtype = expressions.DTypeTimestampTz
	}
	datatypeArgs := expressions.Args{"this": dtype}

	this := asExpression(e.Arg("this"))
	if includePrecision && this != nil && this.Kind() == expressions.KindLiteral {
		if precision := subsecondPrecision(this.Name()); precision > 0 {
			datatypeArgs["expressions"] = []expressions.Expression{
				expressions.DataTypeParam(expressions.Args{"this": expressions.LiteralNumber(precision)}),
			}
		}
	}

	cast := expressions.Cast(expressions.Args{"this": this, "to": expressions.DataType(datatypeArgs)})
	return g.gen(cast)
}

// timeStrToTimeDispatchSQL gates timeStrToTimeSQL per-dialect: mysql passes
// include_precision=not zone (generators/mysql.py:213-217), postgres passes no
// include_precision override i.e. False (generators/postgres.py:371), and base has no
// TimeStrToTime override so it keeps the class's default name via functionFallbackSQL
// (TIME_STR_TO_TIME(...), camelToSnake(ClassName) - not corpus-tested, base/StrTo* generation
// is fallback-only in this port's scope).
func (g *Generator) timeStrToTimeDispatchSQL(e expressions.Expression) string {
	switch g.dialect.Name {
	case "mysql":
		return timeStrToTimeSQL(g, e, !truthy(e.Arg("zone")))
	case "postgres":
		return timeStrToTimeSQL(g, e, false)
	default:
		return g.functionFallbackSQL(e)
	}
}

// timeStrToUnixSQL ports exp.TimeStrToUnix: rename_func("UNIX_TIMESTAMP") (generators/
// mysql.py:212). Base (and every other dialect in this port's scope) has no override, so it
// keeps the default name via functionFallbackSQL (TIME_STR_TO_UNIX(...)).
func (g *Generator) timeStrToUnixSQL(e expressions.Expression) string {
	if g.dialect.Name == "mysql" {
		return g.funcCall("UNIX_TIMESTAMP", g.fallbackArgs(e), "(", ")", true)
	}
	return g.functionFallbackSQL(e)
}

func init() {
	dispatch[expressions.KindTimeStrToTime] = (*Generator).timeStrToTimeDispatchSQL
	dispatch[expressions.KindTimeStrToUnix] = (*Generator).timeStrToUnixSQL
}
