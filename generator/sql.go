package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

var safeIdentifierRE = regexp.MustCompile(`^[_a-zA-Z]\w*$`)

var typeMapping = map[expressions.DType]string{
	expressions.DTypeDatetime2:     "TIMESTAMP",
	expressions.DTypeNChar:         "CHAR",
	expressions.DTypeNVarchar:      "VARCHAR",
	expressions.DTypeMediumText:    "TEXT",
	expressions.DTypeLongText:      "TEXT",
	expressions.DTypeTinyText:      "TEXT",
	expressions.DTypeBlob:          "VARBINARY",
	expressions.DTypeMediumBlob:    "BLOB",
	expressions.DTypeLongBlob:      "BLOB",
	expressions.DTypeTinyBlob:      "BLOB",
	expressions.DTypeInet:          "INET",
	expressions.DTypeRowVersion:    "VARBINARY",
	expressions.DTypeSmallDatetime: "TIMESTAMP",
}

// mysqlTypeMapping is a FULL REPLACEMENT of typeMapping for the mysql dialect (generators/
// mysql.py:255-273): unlike base, it does NOT remap LONGTEXT/MEDIUMTEXT/TINYTEXT/*BLOB to
// TEXT/BLOB/VARBINARY - those stay as their own MySQL-native type names. It also folds in
// mysql's TIMESTAMP_TYPE_MAPPING (mysql.py:246-253), used by plain (non-CAST) DataType
// rendering, e.g. a column typed TIMESTAMP renders as DATETIME.
var mysqlTypeMapping = map[expressions.DType]string{
	expressions.DTypeNChar:         "CHAR",
	expressions.DTypeNVarchar:      "VARCHAR",
	expressions.DTypeInet:          "INET",
	expressions.DTypeRowVersion:    "VARBINARY",
	expressions.DTypeUBigInt:       "BIGINT",
	expressions.DTypeUInt:          "INT",
	expressions.DTypeUMediumInt:    "MEDIUMINT",
	expressions.DTypeUSmallInt:     "SMALLINT",
	expressions.DTypeUTinyInt:      "TINYINT",
	expressions.DTypeUDecimal:      "DECIMAL",
	expressions.DTypeUDouble:       "DOUBLE",
	expressions.DTypeDatetime2:     "DATETIME",
	expressions.DTypeSmallDatetime: "DATETIME",
	expressions.DTypeTimestamp:     "DATETIME",
	expressions.DTypeTimestampNtz:  "DATETIME",
	expressions.DTypeTimestampTz:   "TIMESTAMP",
	expressions.DTypeTimestampLtz:  "TIMESTAMP",
}

// postgresTypeMapping is typeMapping (base) plus the postgres TYPE_MAPPING delta
// (generators/postgres.py:271-284).
var postgresTypeMapping = mergeTypeMappings(typeMapping, map[expressions.DType]string{
	expressions.DTypeTinyInt:      "SMALLINT",
	expressions.DTypeFloat:        "REAL",
	expressions.DTypeDouble:       "DOUBLE PRECISION",
	expressions.DTypeBinary:       "BYTEA",
	expressions.DTypeVarBinary:    "BYTEA",
	expressions.DTypeRowVersion:   "BYTEA",
	expressions.DTypeDatetime:     "TIMESTAMP",
	expressions.DTypeTimestampNtz: "TIMESTAMP",
	expressions.DTypeBlob:         "BYTEA",
})

func mergeTypeMappings(base map[expressions.DType]string, delta map[expressions.DType]string) map[expressions.DType]string {
	out := make(map[expressions.DType]string, len(base)+len(delta))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range delta {
		out[k] = v
	}
	return out
}

// mysqlCastMapping ports MySQL's narrow CAST_MAPPING (generators/mysql.py:315-331): MySQL's
// CAST only supports a handful of target types, so text/blob-ish types collapse to CHAR and
// signed-integer-ish types (plus BOOLEAN) collapse to SIGNED; UBIGINT is the one type that
// maps to UNSIGNED instead. Values are plain override strings (not further looked up in
// mysqlTypeMapping), matching upstream's rename-then-render-raw semantics.
var mysqlCastMapping = map[expressions.DType]string{
	expressions.DTypeLongText:   "CHAR",
	expressions.DTypeLongBlob:   "CHAR",
	expressions.DTypeMediumBlob: "CHAR",
	expressions.DTypeMediumText: "CHAR",
	expressions.DTypeText:       "CHAR",
	expressions.DTypeTinyBlob:   "CHAR",
	expressions.DTypeTinyText:   "CHAR",
	expressions.DTypeVarchar:    "CHAR",
	expressions.DTypeBigInt:     "SIGNED",
	expressions.DTypeBoolean:    "SIGNED",
	expressions.DTypeInt:        "SIGNED",
	expressions.DTypeSmallInt:   "SIGNED",
	expressions.DTypeTinyInt:    "SIGNED",
	expressions.DTypeMediumInt:  "SIGNED",
	expressions.DTypeUBigInt:    "UNSIGNED",
}

// mysqlTimestampFuncTypes ports MySQL's TIMESTAMP_FUNC_TYPES (generators/mysql.py:333-336):
// casting to either of these renders as a TIMESTAMP(...) function call rather than CAST(...
// AS ...), since MySQL's CAST doesn't accept a timezone-aware timestamp target type.
var mysqlTimestampFuncTypes = map[expressions.DType]bool{
	expressions.DTypeTimestampTz:  true,
	expressions.DTypeTimestampLtz: true,
}

// timePartSingulars ports TIME_PART_SINGULARS (generator.py:644-654): used by intervalSQL
// when the dialect doesn't allow plural interval unit forms (e.g. mysql).
var timePartSingulars = map[string]string{
	"MICROSECONDS": "MICROSECOND",
	"SECONDS":      "SECOND",
	"MINUTES":      "MINUTE",
	"HOURS":        "HOUR",
	"DAYS":         "DAY",
	"WEEKS":        "WEEK",
	"MONTHS":       "MONTH",
	"QUARTERS":     "QUARTER",
	"YEARS":        "YEAR",
}

const initcapDefaultDelimiterChars = " \t\n\r\f\v!\"#$%&'()*+,\\-./:;<=>?@\\[\\]^_`{|}~"

func (g *Generator) expressionSQL(e expressions.Expression) string { return g.sqlKey(e, "this") }

func (g *Generator) nullSQL(e expressions.Expression) string { return "NULL" }

func (g *Generator) booleanSQL(e expressions.Expression) string {
	if boolValue(e.Arg("this")) {
		return "TRUE"
	}
	return "FALSE"
}

func (g *Generator) varSQL(e expressions.Expression) string { return g.sqlKey(e, "this") }

func (g *Generator) starSQL(e expressions.Expression) string {
	except := g.expressions(exprsOptions{expression: e, key: "except_", flat: true})
	if except != "" {
		except = g.seg("EXCEPT") + " (" + except + ")"
	}
	replace := g.expressions(exprsOptions{expression: e, key: "replace", flat: true})
	if replace != "" {
		replace = g.seg("REPLACE") + " (" + replace + ")"
	}
	rename := g.expressions(exprsOptions{expression: e, key: "rename", flat: true})
	if rename != "" {
		rename = g.seg("RENAME") + " (" + rename + ")"
	}
	ilike := g.sqlKey(e, "ilike")
	if ilike != "" {
		ilike = g.seg("ILIKE") + " " + ilike
	}
	return "*" + ilike + except + replace + rename
}

func (g *Generator) parenSQL(e expressions.Expression) string {
	sql := g.seg(g.indent(g.sqlKey(e, "this"), 0, nil, false, false), "")
	return "(" + sql + g.seg(")", "")
}

func (g *Generator) negSQL(e expressions.Expression) string {
	thisSQL := g.sqlKey(e, "this")
	sep := ""
	if strings.HasPrefix(thisSQL, "-") {
		sep = " "
	}
	return "-" + sep + thisSQL
}

func (g *Generator) notSQL(e expressions.Expression) string { return "NOT " + g.sqlKey(e, "this") }

func (g *Generator) dotSQL(e expressions.Expression) string {
	return g.sqlKey(e, "this") + "." + g.sqlKey(e, "expression")
}

func (g *Generator) columnParts(e expressions.Expression) string {
	parts := []string{}
	for _, key := range []string{"catalog", "db", "table", "this"} {
		if v := e.Arg(key); truthy(v) {
			parts = append(parts, g.gen(v))
		}
	}
	return strings.Join(parts, ".")
}

func (g *Generator) columnSQL(e expressions.Expression) string {
	joinMark := ""
	if truthy(e.Arg("join_mark")) {
		joinMark = " (+)"
	}
	if joinMark != "" && !g.dialect.SupportsColumnJoinMarks {
		joinMark = ""
		g.unsupported("Outer join syntax using the (+) operator is not supported.")
	}
	return g.columnParts(e) + joinMark
}

func (g *Generator) identifierSQL(e expressions.Expression) string {
	text := e.Name()
	lower := strings.ToLower(text)
	quoted := boolValue(e.Arg("quoted"))
	if g.normalize && !quoted {
		text = lower
	}
	text = strings.ReplaceAll(text, g.identifierEnd, g.escapedIdentifierEnd)
	if quoted || g.canQuoteIdentifier(e) || g.dialect.ReservedKeywords[lower] ||
		(!g.dialect.TokenizerConfig.IdentifiersCanStartWithDigit && startsWithDigit(text)) {
		text = g.identifierStart + g.replaceLineBreaks(text) + g.identifierEnd
	}
	return text
}

func (g *Generator) canQuoteIdentifier(e expressions.Expression) bool {
	identify := g.identify
	if identify == nil {
		return false
	}
	if b, ok := identify.(bool); ok && !b {
		return false
	}
	if parent := e.Parent(); parent != nil && parent.Is(expressions.TraitFunc) {
		return false
	}
	if b, ok := identify.(bool); ok && b {
		return true
	}
	// The case-sensitivity and SAFE_IDENTIFIER_RE checks run against the ORIGINAL
	// identifier text (identifier.this), not the normalized/escaped output text
	// (dialect.py:1089-1091).
	original := e.Name()
	caseSensitive := false
	for _, r := range original {
		if r >= 'A' && r <= 'Z' {
			caseSensitive = true
			break
		}
	}
	isSafe := !caseSensitive && safeIdentifierRE.MatchString(original)
	switch identify {
	case "safe":
		return isSafe
	case "unsafe":
		return !isSafe
	}
	return false
}

func startsWithDigit(text string) bool {
	if text == "" {
		return false
	}
	r := []rune(text)[0]
	return r >= '0' && r <= '9'
}

func (g *Generator) literalSQL(e expressions.Expression) string {
	text := e.Text("this")
	if e.IsString() {
		text = g.quoteStart + g.escapeStr(text) + g.quoteEnd
	}
	return text
}

// escapeStr is the common-case entry point, equivalent to upstream escape_str's defaults
// (escape_backslash=True, delimiter=None, escaped_delimiter=None, is_byte_string=False,
// generator.py:2983-2989). By far the hottest caller (every string literal), so it stays a
// plain single-arg function rather than requiring every call site to spell out defaults.
func (g *Generator) escapeStr(text string) string {
	return g.escapeStrOpts(text, escapeStrOptions{escapeBackslash: true})
}

// escapeStrOptions parameterizes escapeStrOpts, mirroring escape_str's non-default kwargs.
// Go's bool zero value is false, NOT upstream's escape_backslash=True default - callers
// that want that default must set EscapeBackslash explicitly (see escapeStr above); the
// only current caller of a non-default value is bytestringSQL (escape_backslash=False).
type escapeStrOptions struct {
	escapeBackslash  bool
	delimiter        string
	escapedDelimiter string
	isByteString     bool
}

// escapeStrOpts ports escape_str (generator.py:2983-3005) in full, parameterized form.
func (g *Generator) escapeStrOpts(text string, opts escapeStrOptions) string {
	supportsEscapeSequences := g.stringsSupportEscapedSequences
	if opts.isByteString {
		supportsEscapeSequences = g.byteStringsSupportEscapedSequences
	}
	if supportsEscapeSequences {
		// Mirrors the default ESCAPED_SEQUENCES dialect table (dialects/dialect.py:66-90,
		// 302-312): the inverse of UNESCAPED_SEQUENCES, filtered down to non-printable
		// control characters plus backslash itself (printable characters round-trip as-is).
		// No target dialect here overrides UNESCAPED_SEQUENCES (only clickhouse/snowflake
		// do, out of scope), so the table is hardcoded rather than threading a new
		// per-dialect field just for this.
		var b strings.Builder
		for i := 0; i < len(text); i++ {
			c := text[i]
			if !opts.escapeBackslash && c == '\\' {
				b.WriteByte(c)
				continue
			}
			if esc, ok := escapedSequences[c]; ok {
				b.WriteString(esc)
			} else {
				b.WriteByte(c)
			}
		}
		text = b.String()
	}
	delimiter := opts.delimiter
	if delimiter == "" {
		delimiter = g.quoteEnd
	}
	escapedDelimiter := opts.escapedDelimiter
	if escapedDelimiter == "" {
		escapedDelimiter = g.escapedQuoteEnd
	}
	return strings.ReplaceAll(g.replaceLineBreaks(text), delimiter, escapedDelimiter)
}

// escapedSequences is the default ESCAPED_SEQUENCES table (dialects/dialect.py:66-90,
// 302-312 UNESCAPED_SEQUENCES inverted, filtered to `not v.isprintable() or v == "\\"`).
// See escapeStr for why this is hardcoded rather than a per-dialect field.
var escapedSequences = map[byte]string{
	'\a': `\a`,
	'\b': `\b`,
	'\f': `\f`,
	'\n': `\n`,
	'\r': `\r`,
	'\t': `\t`,
	'\v': `\v`,
	'\\': `\\`,
}

// placeholderSQL ports placeholder_sql (generator.py:3417-3418) and postgres's override
// (generators/postgres.py:541-546). Postgres's `?` placeholder parses with jdbc=True
// (parsers/postgres.py:96) and always renders "?"; everything else there is its pyformat/
// psycopg placeholder: bare "%s", or named "%(name)s" (parseQueryParameter).
func (g *Generator) placeholderSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		if truthy(e.Arg("jdbc")) {
			return "?"
		}
		this := ""
		if truthy(e.Arg("this")) {
			this = "(" + e.Name() + ")"
		}
		return "%" + this + "s"
	}
	if truthy(e.Arg("this")) {
		// A named placeholder (:name); base/mysql keep :name (NAMED_PLACEHOLDER_TOKEN=":").
		return ":" + e.Name()
	}
	// A bare `?` placeholder.
	return "?"
}

// parameterSQL ports parameter_sql (generator.py:3406-3408). PARAMETER_TOKEN is "@" for
// base/mysql, "$" for postgres (generators/postgres.py:240, e.g. `CAST($1 AS TEXT)`).
func (g *Generator) parameterSQL(e expressions.Expression) string {
	return g.parameterToken + g.sqlKey(e, "this")
}

// rawStringSQL ports rawstring_sql (generator.py:1653-1659): a raw/heredoc string is
// re-emitted as an ordinary quoted string literal (e.g. postgres `$$doc$$` -> `'doc'`).
func (g *Generator) rawStringSQL(e expressions.Expression) string {
	return g.quoteStart + g.escapeStr(e.Text("this")) + g.quoteEnd
}

// fileFormatPropertySQL renders the FileFormatProperty node as `FORMAT=<fmt>`, matching
// upstream's generic property_sql path (generator.py:2083-2092, PROPERTY_TO_NAME lookup).
func (g *Generator) fileFormatPropertySQL(e expressions.Expression) string {
	return "FORMAT=" + g.sqlKey(e, "this")
}

func (g *Generator) aliasSQL(e expressions.Expression) string {
	alias := g.sqlKey(e, "alias")
	if alias != "" {
		alias = " AS " + alias
	}
	return g.sqlKey(e, "this") + alias
}

func (g *Generator) aliasesSQL(e expressions.Expression) string {
	return g.sqlKey(e, "this") + " AS (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
}

func (g *Generator) pivotAliasSQL(e expressions.Expression) string {
	alias := asExpression(e.Arg("alias"))
	parent := e.Parent()
	var pivot expressions.Expression
	if parent != nil {
		pivot = parent.Parent()
	}
	if pivot != nil && pivot.Kind() == expressions.KindPivot && boolValue(pivot.Arg("unpivot")) && alias != nil {
		if alias.Kind() != expressions.KindIdentifier && alias.Kind() == expressions.KindLiteral {
			alias.Replace(expressions.ToIdentifier(alias.OutputName()))
		}
	}
	return g.aliasSQL(e)
}

func (g *Generator) binary(e expressions.Expression, op string) string {
	sqls := []string{}
	stack := []any{e}
	binaryKind := e.Kind()
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if expr, ok := node.(expressions.Expression); ok && !isNilExpression(expr) && expr.Kind() == binaryKind {
			if opFunc := expr.Arg("operator"); truthy(opFunc) {
				op = "OPERATOR(" + g.gen(opFunc) + ")"
			}
			stack = append(stack, expr.Arg("expression"))
			stack = append(stack, " "+g.maybeComment(op, nil, expr.Comments(), false)+" ")
			stack = append(stack, expr.Arg("this"))
		} else {
			sqls = append(sqls, g.gen(node))
		}
	}
	return strings.Join(sqls, "")
}

func (g *Generator) addSQL(e expressions.Expression) string        { return g.binary(e, "+") }
func (g *Generator) subSQL(e expressions.Expression) string        { return g.binary(e, "-") }
func (g *Generator) mulSQL(e expressions.Expression) string        { return g.binary(e, "*") }
func (g *Generator) divSQL(e expressions.Expression) string        { return g.binary(e, "/") }
func (g *Generator) modSQL(e expressions.Expression) string        { return g.binary(e, "%") }
func (g *Generator) eqSQL(e expressions.Expression) string         { return g.binary(e, "=") }
func (g *Generator) neqSQL(e expressions.Expression) string        { return g.binary(e, "<>") }
func (g *Generator) gtSQL(e expressions.Expression) string         { return g.binary(e, ">") }
func (g *Generator) gteSQL(e expressions.Expression) string        { return g.binary(e, ">=") }
func (g *Generator) ltSQL(e expressions.Expression) string         { return g.binary(e, "<") }
func (g *Generator) lteSQL(e expressions.Expression) string        { return g.binary(e, "<=") }
func (g *Generator) bitwiseAndSQL(e expressions.Expression) string { return g.binary(e, "&") }
func (g *Generator) bitwiseOrSQL(e expressions.Expression) string  { return g.binary(e, "|") }
func (g *Generator) bitwiseXorSQL(e expressions.Expression) string { return g.binary(e, "^") }
func (g *Generator) dpipeSQL(e expressions.Expression) string      { return g.binary(e, "||") }
func (g *Generator) nullSafeEQSQL(e expressions.Expression) string {
	return g.binary(e, "IS NOT DISTINCT FROM")
}
func (g *Generator) nullSafeNEQSQL(e expressions.Expression) string {
	return g.binary(e, "IS DISTINCT FROM")
}
func (g *Generator) isSQL(e expressions.Expression) string    { return g.binary(e, "IS") }
func (g *Generator) likeSQL(e expressions.Expression) string  { return g.likeSQLWithOp(e, "LIKE") }
func (g *Generator) ilikeSQL(e expressions.Expression) string { return g.likeSQLWithOp(e, "ILIKE") }

func (g *Generator) likeSQLWithOp(e expressions.Expression, op string) string {
	if boolValue(e.Arg("negate")) {
		op = "NOT " + op
	}
	return g.binary(e, op)
}

// similarToSQL ports generator.py:4496 similarto_sql. Unlike Like/ILike, SimilarTo has no
// "negate" arg (expressions/core.py:2226): `NOT x SIMILAR TO y` is represented as
// Not(SimilarTo(...)) by parseRange, so notSQL renders the NOT prefix instead.
func (g *Generator) similarToSQL(e expressions.Expression) string { return g.binary(e, "SIMILAR TO") }

// escapeSQL ports generator.py:4411 escape_sql, minus the LIKE-ANY/ALL-quantifier special
// case (SUPPORTS_LIKE_QUANTIFIERS is not modeled in this port).
func (g *Generator) escapeSQL(e expressions.Expression) string { return g.binary(e, "ESCAPE") }

func (g *Generator) andSQL(e expressions.Expression) string { return g.connectorSQL(e, "AND", nil) }
func (g *Generator) orSQL(e expressions.Expression) string  { return g.connectorSQL(e, "OR", nil) }

// connectorOp maps a TraitConnector node to its SQL operator token, so the
// flatten walk in connectorSQL renders each nested connector with its own
// operator (mirroring upstream's getattr(self, f"{node.key}_sql") dispatch,
// generator.py:4046) instead of assuming AND/OR. Only And/Or/Xor carry
// TraitConnector; the default covers Or.
func connectorOp(k expressions.Kind) string {
	switch k {
	case expressions.KindAnd:
		return "AND"
	case expressions.KindXor:
		return "XOR"
	default:
		return "OR"
	}
}

func (g *Generator) connectorSQL(e expressions.Expression, op string, stack *[]any) string {
	if stack != nil {
		if exprs := g.expressions(exprsOptions{expression: e, sep: " " + op + " "}); exprs != "" && truthy(e.Arg("expressions")) {
			*stack = append(*stack, exprs)
		} else {
			*stack = append(*stack, e.Right())
			if len(e.Comments()) > 0 && g.comments {
				op = g.maybeComment(op, nil, e.Comments(), false)
			}
			*stack = append(*stack, op, e.Left())
		}
		return op
	}
	work := []any{e}
	sqls := []string{}
	ops := map[string]bool{}
	for len(work) > 0 {
		node := work[len(work)-1]
		work = work[:len(work)-1]
		if expr, ok := node.(expressions.Expression); ok && !isNilExpression(expr) && expr.Is(expressions.TraitConnector) {
			emitted := g.connectorSQL(expr, connectorOp(expr.Kind()), &work)
			ops[emitted] = true
		} else {
			sql := g.gen(node)
			if len(sqls) > 0 && ops[sqls[len(sqls)-1]] {
				sqls[len(sqls)-1] += " " + sql
			} else {
				sqls = append(sqls, sql)
			}
		}
	}
	sep := " "
	if g.pretty && g.tooWide(sqls) {
		sep = "\n"
	}
	return strings.Join(sqls, sep)
}

func (g *Generator) prependCtes(e expressions.Expression, sql string) string {
	with := g.sqlKey(e, "with_")
	if with != "" {
		sql = with + g.sep() + sql
	}
	return sql
}

func (g *Generator) queryModifiers(e expressions.Expression, sqls ...string) string {
	limit := asExpression(e.Arg("limit"))
	fetch := limit != nil && limit.Kind() == expressions.KindFetch
	parts := append([]string{}, sqls...)
	for _, join := range listFromValue(e.Arg("joins")) {
		parts = append(parts, g.gen(join))
	}
	parts = append(parts, g.sqlKey(e, "match"))
	for _, lateral := range listFromValue(e.Arg("laterals")) {
		parts = append(parts, g.gen(lateral))
	}
	parts = append(parts, g.sqlKey(e, "prewhere"), g.sqlKey(e, "where"), g.sqlKey(e, "connect"), g.sqlKey(e, "group"), g.sqlKey(e, "having"))
	parts = append(parts, g.sqlKey(e, "cluster"), g.sqlKey(e, "distribute"), g.sqlKey(e, "sort"))
	if truthy(e.Arg("windows")) {
		parts = append(parts, g.seg("WINDOW ")+g.expressions(exprsOptions{expression: e, key: "windows", flat: true}))
	}
	parts = append(parts, g.sqlKey(e, "qualify"), g.sqlKey(e, "order"))
	parts = append(parts, g.offsetLimitModifiers(e, fetch, limit)...)
	parts = append(parts, g.afterLimitModifiers(e)...)
	parts = append(parts, g.optionsModifier(e), g.sqlKey(e, "for_"))
	return joinNonEmpty("", parts...)
}

func (g *Generator) offsetLimitModifiers(e expressions.Expression, fetch bool, limit expressions.Expression) []string {
	if fetch {
		return []string{g.sqlKey(e, "offset"), g.gen(limit)}
	}
	return []string{g.gen(limit), g.sqlKey(e, "offset")}
}

func (g *Generator) afterLimitModifiers(e expressions.Expression) []string {
	locks := g.expressions(exprsOptions{expression: e, key: "locks", sep: " "})
	if locks != "" {
		locks = " " + locks
	}
	return []string{locks, g.sqlKey(e, "sample")}
}

func (g *Generator) optionsModifier(e expressions.Expression) string {
	options := g.expressions(exprsOptions{expression: e, key: "options"})
	if options != "" {
		return " " + options
	}
	return ""
}

func (g *Generator) selectSQL(e expressions.Expression) string {
	hint := g.sqlKey(e, "hint")
	distinct := g.sqlKey(e, "distinct")
	if distinct != "" {
		distinct = " " + distinct
	}
	kind := g.sqlKey(e, "kind")
	top := ""
	expressionsSQL := g.expressions(exprsOptions{expression: e})
	if kind != "" {
		if kind == "STRUCT" || kind == "VALUE" {
			kind = " AS " + kind
		} else {
			kind = ""
		}
	}
	operationModifiers := g.expressions(exprsOptions{expression: e, key: "operation_modifiers", sep: " "})
	if operationModifiers != "" {
		operationModifiers = g.sep() + operationModifiers
	}
	if expressionsSQL != "" {
		expressionsSQL = g.sep() + expressionsSQL
	}
	// SELECT ... INTO (generator.py:3302-3304, 3374-3381): postgres (SUPPORTS_SELECT_INTO)
	// keeps the INTO inline; every other dialect drops it here and instead wraps the finished
	// SELECT in `CREATE TABLE <into.this> AS ...` below. The keyed sql(expr,"into",...) call
	// still recurses with comments on, so we use sqlKey when we do render it inline.
	into := asExpression(e.Arg("into"))
	intoInline := ""
	if into != nil && g.dialect.SupportsSelectInto {
		intoInline = g.sqlKey(e, "into")
	}
	sql := g.queryModifiers(e,
		"SELECT"+top+hint+distinct+operationModifiers+kind+expressionsSQL,
		intoInline,
		g.sqlKey(e, "from_"),
	)
	if truthy(e.Arg("with_")) {
		sql = g.maybeComment(sql, e, nil, false)
		e.PopComments()
	}
	sql = g.prependCtes(e, sql)
	if into != nil && !g.dialect.SupportsSelectInto {
		// generator.py:3374-3381: temporary -> " TEMPORARY", else "" (UNLOGGED is only kept
		// under SUPPORTS_UNLOGGED_TABLES, which no base/mysql dialect sets, so it is dropped).
		tableKind := ""
		if boolValue(into.Arg("temporary")) {
			tableKind = " TEMPORARY"
		}
		sql = "CREATE" + tableKind + " TABLE " + g.sqlKey(into, "this") + " AS " + sql
	}
	// Deferred to slice 5 (dialects): base has STAR_EXCLUDE_REQUIRES_DERIVED_TABLE=true, which
	// upstream (generator.py:3368-3372) rewrites a select-level `exclude` arg into a derived
	// table `SELECT * EXCLUDE (...) FROM (<subquery>)`. The base parser rejects select-level
	// EXCLUDE (only Star EXCEPT is produced, handled by starSQL), so `exclude` is never
	// populated and this path is unreachable; deferred with the SELECT INTO rewrite above.
	return sql
}

func (g *Generator) fromSQL(e expressions.Expression) string {
	return g.seg("FROM") + " " + g.sqlKey(e, "this")
}

func (g *Generator) whereSQL(e expressions.Expression) string {
	this := g.indent(g.sqlKey(e, "this"), 0, nil, false, false)
	return g.seg("WHERE") + g.sep() + this
}

func (g *Generator) havingSQL(e expressions.Expression) string {
	this := g.indent(g.sqlKey(e, "this"), 0, nil, false, false)
	return g.seg("HAVING") + g.sep() + this
}

func (g *Generator) qualifySQL(e expressions.Expression) string {
	this := g.indent(g.sqlKey(e, "this"), 0, nil, false, false)
	return g.seg("QUALIFY") + g.sep() + this
}

func (g *Generator) groupingSetsSQL(e expressions.Expression) string {
	groupingSets := g.expressions(exprsOptions{expression: e, noIndent: true})
	return "GROUPING SETS " + g.wrap(groupingSets)
}

func (g *Generator) rollupSQL(e expressions.Expression) string {
	expressionsSQL := g.expressions(exprsOptions{expression: e, noIndent: true})
	if expressionsSQL != "" {
		return "ROLLUP " + g.wrap(expressionsSQL)
	}
	return "WITH ROLLUP"
}

func (g *Generator) cubeSQL(e expressions.Expression) string {
	expressionsSQL := g.expressions(exprsOptions{expression: e, noIndent: true})
	if expressionsSQL != "" {
		return "CUBE " + g.wrap(expressionsSQL)
	}
	return "WITH CUBE"
}

func (g *Generator) groupSQL(e expressions.Expression) string {
	modifier := ""
	if v, ok := e.Arg("all").(bool); ok {
		if v {
			modifier = " ALL"
		} else {
			modifier = " DISTINCT"
		}
	}
	groupBy := g.opExpressions("GROUP BY"+modifier, e)
	groupingSets := g.expressions(exprsOptions{expression: e, key: "grouping_sets"})
	cube := g.expressions(exprsOptions{expression: e, key: "cube"})
	rollup := g.expressions(exprsOptions{expression: e, key: "rollup"})
	groupings := joinNonEmpty(",",
		func() string {
			if groupingSets != "" {
				return g.seg(groupingSets)
			}
			return ""
		}(),
		func() string {
			if cube != "" {
				return g.seg(cube)
			}
			return ""
		}(),
		func() string {
			if rollup != "" {
				return g.seg(rollup)
			}
			return ""
		}(),
		func() string {
			if boolValue(e.Arg("totals")) {
				return g.seg("WITH TOTALS")
			}
			return ""
		}(),
	)
	if len(listFromValue(e.Arg("expressions"))) > 0 && groupings != "" {
		trimmed := strings.TrimSpace(groupings)
		if trimmed != "WITH CUBE" && trimmed != "WITH ROLLUP" {
			groupBy += ","
		}
	}
	return groupBy + groupings
}

func (g *Generator) orderSQL(e expressions.Expression) string { return g.orderSQLFlat(e, false) }

func (g *Generator) orderSQLFlat(e expressions.Expression, flat bool) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this += " "
	}
	siblings := ""
	if boolValue(e.Arg("siblings")) {
		siblings = "SIBLINGS "
	}
	return g.opExpressions(this+"ORDER "+siblings+"BY", e, this != "" || flat)
}

func (g *Generator) orderedSQL(e expressions.Expression) string {
	desc, descSet := e.Arg("desc").(bool)
	asc := !desc
	nullsFirst := boolValue(e.Arg("nulls_first"))
	nullsLast := !nullsFirst
	nullsAreLarge := g.dialect.NullOrdering == "nulls_are_large"
	nullsAreSmall := g.dialect.NullOrdering == "nulls_are_small"
	nullsAreLast := g.dialect.NullOrdering == "nulls_are_last"
	this := g.sqlKey(e, "this")
	sortOrder := ""
	if desc {
		sortOrder = " DESC"
	} else if descSet {
		sortOrder = " ASC"
	}
	nullsSortChange := ""
	if nullsFirst && ((asc && nullsAreLarge) || (desc && nullsAreSmall) || nullsAreLast) {
		nullsSortChange = " NULLS FIRST"
	} else if nullsLast && ((asc && nullsAreSmall) || (desc && nullsAreLarge)) && !nullsAreLast {
		nullsSortChange = " NULLS LAST"
	}
	withFill := g.sqlKey(e, "with_fill")
	if withFill != "" {
		withFill = " " + withFill
	}
	return this + sortOrder + nullsSortChange + withFill
}

func (g *Generator) limitSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	args := []string{}
	for _, key := range []string{"offset", "expression"} {
		if v := e.Arg(key); truthy(v) {
			args = append(args, g.gen(v))
		}
	}
	argsSQL := strings.Join(args, ", ")
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = " BY " + exprs
	}
	limitOptions := g.sqlKey(e, "limit_options")
	return this + g.seg("LIMIT") + " " + argsSQL + limitOptions + exprs
}

func (g *Generator) offsetSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = " BY " + exprs
	}
	return this + g.seg("OFFSET") + " " + g.sqlKey(e, "expression") + exprs
}

func (g *Generator) fetchSQL(e expressions.Expression) string {
	direction := e.Text("direction")
	if direction != "" {
		direction = " " + direction
	}
	count := g.sqlKey(e, "count")
	if count != "" {
		count = " " + count
	}
	limitOptions := g.sqlKey(e, "limit_options")
	if limitOptions == "" {
		limitOptions = " ROWS ONLY"
	}
	return g.seg("FETCH") + direction + count + limitOptions
}

func (g *Generator) limitOptionsSQL(e expressions.Expression) string {
	percent := ""
	if boolValue(e.Arg("percent")) {
		percent = " PERCENT"
	}
	rows := ""
	if boolValue(e.Arg("rows")) {
		rows = " ROWS"
	}
	withTies := ""
	if boolValue(e.Arg("with_ties")) {
		withTies = " WITH TIES"
	}
	if withTies == "" && rows != "" {
		withTies = " ONLY"
	}
	return percent + rows + withTies
}

func (g *Generator) joinSQL(e expressions.Expression) string {
	side := e.Text("side")
	kind := e.Text("kind")
	opSQL := joinNonEmpty(" ", e.Text("method"), func() string {
		if boolValue(e.Arg("global_")) {
			return "GLOBAL"
		}
		return ""
	}(), side, kind, e.Text("hint"))
	matchCond := g.sqlKey(e, "match_condition")
	if matchCond != "" {
		matchCond = " MATCH_CONDITION (" + matchCond + ")"
	}
	onSQL := g.sqlKey(e, "on")
	using := listFromValue(e.Arg("using"))
	if onSQL == "" && len(using) > 0 {
		cols := make([]string, 0, len(using))
		for _, column := range using {
			cols = append(cols, g.gen(column))
		}
		onSQL = strings.Join(cols, ", ")
	}
	this := asExpression(e.Arg("this"))
	thisSQL := g.gen(this)
	exprs := g.expressions(exprsOptions{expression: e})
	if exprs != "" {
		thisSQL = thisSQL + "," + g.seg(exprs)
	}
	if onSQL != "" {
		onSQL = g.indent(onSQL, 0, nil, true, false)
		space := " "
		if g.pretty {
			space = g.seg(strings.Repeat(" ", g.pad))
		}
		if len(using) > 0 {
			onSQL = space + "USING (" + onSQL + ")"
		} else {
			onSQL = space + "ON " + onSQL
		}
	} else if opSQL == "" {
		// generator.py:2850: CROSS/OUTER APPLY joins carry the `cross_apply` flag on the
		// Lateral child (the join's `this`, set in parser_pivot.go parseLateral), NOT on the
		// Join node — so read it from `this`. Without this, APPLY joins emit a spurious ", ".
		if this != nil && this.Kind() == expressions.KindLateral && this.Arg("cross_apply") != nil {
			return " " + thisSQL
		}
		return ", " + thisSQL
	}
	if opSQL != "STRAIGHT_JOIN" {
		if opSQL != "" {
			opSQL += " JOIN"
		} else {
			opSQL = "JOIN"
		}
	}
	pivots := g.expressions(exprsOptions{expression: e, key: "pivots", emptySep: true, flat: true})
	return g.seg(opSQL) + " " + thisSQL + matchCond + onSQL + pivots
}

func (g *Generator) clusterSQL(e expressions.Expression) string {
	return g.opExpressions("CLUSTER BY", e)
}
func (g *Generator) distributeSQL(e expressions.Expression) string {
	return g.opExpressions("DISTRIBUTE BY", e)
}
func (g *Generator) sortSQL(e expressions.Expression) string     { return g.opExpressions("SORT BY", e) }
func (g *Generator) preWhereSQL(e expressions.Expression) string { return "" }

func (g *Generator) lockSQL(e expressions.Expression) string {
	if !g.dialect.LockingReadsSupported {
		g.unsupported("Locking reads using 'FOR UPDATE/SHARE' are not supported")
		return ""
	}

	update := boolValue(e.Arg("update"))
	key := boolValue(e.Arg("key"))
	var lockType string
	if update {
		if key {
			lockType = "FOR NO KEY UPDATE"
		} else {
			lockType = "FOR UPDATE"
		}
	} else {
		if key {
			lockType = "FOR KEY SHARE"
		} else {
			lockType = "FOR SHARE"
		}
	}
	exprs := g.expressions(exprsOptions{expression: e, flat: true})
	if exprs != "" {
		exprs = " OF " + exprs
	}

	wait := ""
	if w := e.Arg("wait"); w != nil {
		if we := asExpression(w); we != nil && we.Kind() == expressions.KindLiteral {
			wait = " WAIT " + g.gen(we)
		} else if boolValue(w) {
			wait = " NOWAIT"
		} else {
			wait = " SKIP LOCKED"
		}
	}

	return lockType + exprs + wait
}

func (g *Generator) setOperationSQL(e expressions.Expression) string {
	opNames := map[expressions.Kind]string{expressions.KindUnion: "UNION", expressions.KindExcept: "EXCEPT", expressions.KindIntersect: "INTERSECT"}
	opName := opNames[e.Kind()]
	distinctArg := e.Arg("distinct")
	distinct, hasDistinct := distinctArg.(bool)
	if !hasDistinct && distinctArg == nil {
		distinct = true
	}
	distinctOrAll := ""
	if hasDistinct && !distinct {
		distinctOrAll = " ALL"
	}
	sideKind := joinNonEmpty(" ", e.Text("side"), e.Text("kind"))
	if sideKind != "" {
		sideKind += " "
	}
	byName := ""
	if boolValue(e.Arg("by_name")) {
		byName = " BY NAME"
	}
	on := g.expressions(exprsOptions{expression: e, key: "on", flat: true})
	if on != "" {
		on = " ON (" + on + ")"
	}
	return sideKind + opName + distinctOrAll + byName + on
}

func (g *Generator) setOperationsSQL(e expressions.Expression) string {
	sqls := []string{}
	stack := []any{e}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if expr, ok := node.(expressions.Expression); ok && !isNilExpression(expr) && (expr.Kind() == expressions.KindUnion || expr.Kind() == expressions.KindExcept || expr.Kind() == expressions.KindIntersect) {
			stack = append(stack, expr.Arg("expression"))
			stack = append(stack, g.maybeComment(g.setOperationSQL(expr), nil, expr.Comments(), true))
			stack = append(stack, expr.Arg("this"))
		} else {
			sqls = append(sqls, g.gen(node))
		}
	}
	this := strings.Join(sqls, g.sep())
	this = g.queryModifiers(e, this)
	return g.prependCtes(e, this)
}

func (g *Generator) withSQL(e expressions.Expression) string {
	sql := g.expressions(exprsOptions{expression: e, flat: true})
	recursive := ""
	if boolValue(e.Arg("recursive")) {
		recursive = "RECURSIVE "
	}
	search := g.sqlKey(e, "search")
	if search != "" {
		search = " " + search
	}
	return "WITH " + recursive + sql + search
}

func (g *Generator) cteSQL(e expressions.Expression) string {
	alias := asExpression(e.Arg("alias"))
	if alias != nil {
		alias.AddComments(e.PopComments(), false)
	}
	aliasSQL := g.sqlKey(e, "alias")
	materialized := ""
	if v, ok := e.Arg("materialized").(bool); ok {
		if v {
			materialized = "MATERIALIZED "
		} else {
			materialized = "NOT MATERIALIZED "
		}
	}
	keyExpressions := g.expressions(exprsOptions{expression: e, key: "key_expressions", flat: true})
	if keyExpressions != "" {
		keyExpressions = " USING KEY (" + keyExpressions + ")"
	}
	return aliasSQL + keyExpressions + " AS " + materialized + g.wrap(e)
}

func (g *Generator) recursiveWithSearchSQL(e expressions.Expression) string {
	kind := g.sqlKey(e, "kind")
	this := g.sqlKey(e, "this")
	set := g.sqlKey(e, "expression")
	using := g.sqlKey(e, "using")
	if using != "" {
		using = " USING " + using
	}
	kindSQL := kind
	if kind != "CYCLE" {
		kindSQL = "SEARCH " + kind + " FIRST BY"
	}
	return kindSQL + " " + this + " SET " + set + using
}

func (g *Generator) subquerySQL(e expressions.Expression) string {
	alias := g.sqlKey(e, "alias")
	if alias != "" {
		alias = " AS " + alias
	}
	sample := g.sqlKey(e, "sample")
	if g.dialect.AliasPostTablesample && sample != "" {
		alias = sample + alias
		e.Set("sample", nil)
	}
	pivots := g.expressions(exprsOptions{expression: e, key: "pivots", emptySep: true, flat: true})
	sql := g.queryModifiers(e, g.wrap(e), alias, pivots)
	return g.prependCtes(e, sql)
}

func (g *Generator) betweenSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	low := g.sqlKey(e, "low")
	high := g.sqlKey(e, "high")
	if boolValue(e.Arg("symmetric")) {
		return "(" + this + " BETWEEN " + low + " AND " + high + " OR " + this + " BETWEEN " + high + " AND " + low + ")"
	}
	return this + " BETWEEN " + low + " AND " + high
}

func (g *Generator) inSQL(e expressions.Expression) string {
	query := e.Arg("query")
	unnest := asExpression(e.Arg("unnest"))
	field := e.Arg("field")
	isGlobal := ""
	if boolValue(e.Arg("is_global")) {
		isGlobal = " GLOBAL"
	}
	var inSQL string
	if truthy(query) {
		inSQL = g.gen(query)
	} else if unnest != nil {
		inSQL = g.inUnnestOp(unnest)
	} else if truthy(field) {
		inSQL = g.gen(field)
	} else {
		inSQL = "(" + g.expressions(exprsOptions{expression: e, dynamic: true, newLine: true, skipFirst: true, skipLast: true}) + ")"
	}
	return g.sqlKey(e, "this") + isGlobal + " IN " + inSQL
}

func (g *Generator) inUnnestOp(unnest expressions.Expression) string {
	return "(SELECT " + g.gen(unnest) + ")"
}

func (g *Generator) existsSQL(e expressions.Expression) string { return "EXISTS" + g.wrap(e) }

func (g *Generator) anySQL(e expressions.Expression) string {
	thisExpr := asExpression(e.Arg("this"))
	this := g.sqlKey(e, "this")
	if thisExpr != nil && (isUnwrappedQuery(thisExpr.Kind()) || thisExpr.Kind() == expressions.KindParen) {
		if isUnwrappedQuery(thisExpr.Kind()) {
			this = g.wrap(this)
		}
		return "ANY" + this
	}
	return "ANY " + this
}

func (g *Generator) allSQL(e expressions.Expression) string {
	thisExpr := asExpression(e.Arg("this"))
	this := g.sqlKey(e, "this")
	if thisExpr == nil || (thisExpr.Kind() != expressions.KindTuple && thisExpr.Kind() != expressions.KindParen) {
		this = g.wrap(this)
	}
	return "ALL " + this
}

func (g *Generator) caseSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	statements := []string{}
	if this != "" {
		statements = append(statements, "CASE "+this)
	} else {
		statements = append(statements, "CASE")
	}
	for _, item := range listFromValue(e.Arg("ifs")) {
		ifExpr := asExpression(item)
		statements = append(statements, "WHEN "+g.sqlKey(ifExpr, "this"), "THEN "+g.sqlKey(ifExpr, "true"))
	}
	defaultSQL := g.sqlKey(e, "default")
	if defaultSQL != "" {
		statements = append(statements, "ELSE "+defaultSQL)
	}
	statements = append(statements, "END")
	if g.pretty && g.tooWide(statements) {
		return g.indent(strings.Join(statements, "\n"), 0, nil, true, true)
	}
	return strings.Join(statements, " ")
}

func (g *Generator) ifSQL(e expressions.Expression) string {
	return g.caseSQL(expressions.Case(expressions.Args{"ifs": []expressions.Expression{e}, "default": e.Arg("false")}))
}

func (g *Generator) windowSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	partition := g.partitionBySQL(e)
	orderExpr := asExpression(e.Arg("order"))
	order := ""
	if orderExpr != nil {
		order = g.orderSQLFlat(orderExpr, true)
	}
	spec := g.sqlKey(e, "spec")
	alias := g.sqlKey(e, "alias")
	over := g.sqlKey(e, "over")
	if over == "" {
		over = "OVER"
	}
	if e.ArgKey() == "windows" {
		this += " AS"
	} else {
		this += " " + over
	}
	first := ""
	if v, ok := e.Arg("first").(bool); ok {
		if v {
			first = "FIRST"
		} else {
			first = "LAST"
		}
	}
	if partition == "" && order == "" && spec == "" && alias != "" {
		return this + " " + alias
	}
	args := []any{}
	for _, arg := range []string{alias, first, partition, order, spec} {
		if arg != "" {
			args = append(args, arg)
		}
	}
	return this + " (" + g.formatArgs(args, " ") + ")"
}

func (g *Generator) partitionBySQL(e expressions.Expression) string {
	partition := g.expressions(exprsOptions{expression: e, key: "partition_by", flat: true})
	if partition != "" {
		return "PARTITION BY " + partition
	}
	return ""
}

func (g *Generator) windowSpecSQL(e expressions.Expression) string {
	kind := g.sqlKey(e, "kind")
	start := joinNonEmpty(" ", g.sqlKey(e, "start"), g.sqlKey(e, "start_side"))
	end := joinNonEmpty(" ", g.sqlKey(e, "end"), g.sqlKey(e, "end_side"))
	if end == "" {
		end = "CURRENT ROW"
	}
	windowSpec := kind + " BETWEEN " + start + " AND " + end
	exclude := g.sqlKey(e, "exclude")
	if exclude != "" {
		// SUPPORTS_WINDOW_EXCLUDE (generator.py:512, generators/postgres.py:250): base
		// defaults to unsupported (dropping EXCLUDE with a warning); postgres (like
		// sqlite/oracle/duckdb upstream, out of scope here) renders it.
		if g.dialect.Name == "postgres" {
			windowSpec += " EXCLUDE " + exclude
		} else {
			g.unsupported("EXCLUDE clause is not supported in the WINDOW clause")
		}
	}
	return windowSpec
}

func (g *Generator) filterSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	where := strings.TrimSpace(g.sqlKey(e, "expression"))
	return this + " FILTER(" + where + ")"
}

func (g *Generator) withinGroupSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	// order (the expression) has a leading separator (space when flat, "\n" when
	// pretty); strip the first char unconditionally to mirror Python's [1:].
	expressionSQL := g.sqlKey(e, "expression")
	if expressionSQL != "" {
		expressionSQL = expressionSQL[1:]
	}
	return this + " WITHIN GROUP (" + expressionSQL + ")"
}

func (g *Generator) distinctSQL(e expressions.Expression) string {
	this := g.expressions(exprsOptions{expression: e, flat: true})
	if this != "" {
		this = " " + this
	}
	on := g.sqlKey(e, "on")
	if on != "" {
		on = " ON " + on
	}
	return "DISTINCT" + this + on
}

func (g *Generator) ignoreNullsSQL(e expressions.Expression) string {
	return g.embedIgnoreNulls(e, "IGNORE NULLS")
}
func (g *Generator) respectNullsSQL(e expressions.Expression) string {
	return g.embedIgnoreNulls(e, "RESPECT NULLS")
}

func (g *Generator) embedIgnoreNulls(e expressions.Expression, text string) string {
	return g.sqlKey(e, "this") + " " + text
}

func (g *Generator) insertSQL(e expressions.Expression) string {
	keyword := "INSERT"
	if g.dialect.Name == "mysql" && boolValue(e.Arg("replace")) {
		keyword = "REPLACE"
	}
	hint := g.sqlKey(e, "hint")
	thisKeyword := " INTO"
	if boolValue(e.Arg("overwrite")) {
		// A Directory target (Hive/Spark INSERT OVERWRITE [LOCAL] DIRECTORY ...)
		// renders " OVERWRITE" without the TABLE keyword (generator.py insert_sql's
		// isinstance(this, exp.Directory) special-case); an ordinary table target
		// keeps " OVERWRITE TABLE".
		if this := asExpression(e.Arg("this")); this != nil && this.Kind() == expressions.KindDirectory {
			thisKeyword = " OVERWRITE"
		} else {
			thisKeyword = " OVERWRITE TABLE"
		}
	}
	stored := g.sqlKey(e, "stored")
	if stored != "" {
		stored = " " + stored
	}
	alternative := e.Text("alternative")
	if alternative != "" {
		alternative = " OR " + alternative
	}
	ignore := ""
	if boolValue(e.Arg("ignore")) {
		ignore = " IGNORE"
	}
	if boolValue(e.Arg("is_function")) {
		thisKeyword += " FUNCTION"
	}
	this := thisKeyword + " " + g.sqlKey(e, "this")
	exists := ""
	if boolValue(e.Arg("exists")) {
		exists = " IF EXISTS"
	}
	where := g.sqlKey(e, "where")
	if where != "" {
		where = g.sep() + "REPLACE WHERE " + where
	}
	expressionSQL := g.sep() + g.sqlKey(e, "expression")
	onConflict := g.sqlKey(e, "conflict")
	if onConflict != "" {
		onConflict = " " + onConflict
	}
	byName := ""
	if boolValue(e.Arg("by_name")) {
		byName = " BY NAME"
	}
	defaultValues := ""
	if boolValue(e.Arg("default")) {
		defaultValues = "DEFAULT VALUES"
	}
	returning := g.sqlKey(e, "returning")
	expressionSQL = expressionSQL + onConflict + defaultValues + returning
	partitionBy := g.sqlKey(e, "partition")
	if partitionBy != "" {
		partitionBy = " " + partitionBy
	}
	settings := g.sqlKey(e, "settings")
	if settings != "" {
		settings = " " + settings
	}
	source := g.sqlKey(e, "source")
	if source != "" {
		source = "TABLE " + source
	}
	sql := keyword + hint + alternative + ignore + this + stored + byName + exists + partitionBy + settings + where + expressionSQL + source
	return g.prependCtes(e, sql)
}

func (g *Generator) updateFromJoinsSQL(e expressions.Expression) (string, string) {
	return "", g.sqlKey(e, "from_")
}

func (g *Generator) updateSQL(e expressions.Expression) string {
	hint := g.sqlKey(e, "hint")
	this := g.sqlKey(e, "this")
	joinSQL, fromSQL := g.updateFromJoinsSQL(e)
	setSQL := g.expressions(exprsOptions{expression: e, flat: true})
	whereSQL := g.sqlKey(e, "where")
	returning := g.sqlKey(e, "returning")
	order := g.sqlKey(e, "order")
	limit := g.sqlKey(e, "limit")
	expressionSQL := fromSQL + whereSQL + returning
	options := g.expressions(exprsOptions{expression: e, key: "options"})
	if options != "" {
		options = " OPTION(" + options + ")"
	}
	sql := "UPDATE" + hint + " " + this + joinSQL + " SET " + setSQL + expressionSQL + order + limit + options
	return g.prependCtes(e, sql)
}

func (g *Generator) deleteSQL(e expressions.Expression) string {
	hint := g.sqlKey(e, "hint")
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " FROM " + this
	}
	using := g.expressions(exprsOptions{expression: e, key: "using"})
	if using != "" {
		using = " USING " + using
	}
	cluster := g.sqlKey(e, "cluster")
	if cluster != "" {
		cluster = " " + cluster
	}
	where := g.sqlKey(e, "where")
	returning := g.sqlKey(e, "returning")
	order := g.sqlKey(e, "order")
	limit := g.sqlKey(e, "limit")
	tables := g.expressions(exprsOptions{expression: e, key: "tables"})
	if tables != "" {
		tables = " " + tables
	}
	expressionSQL := this + using + cluster + where + returning + order + limit
	return g.prependCtes(e, "DELETE"+hint+tables+expressionSQL)
}

// mergeWithoutTarget ports merge_without_target_sql (dialects/dialect.py:2057-2090), postgres's
// exp.Merge generator transform (generators/postgres.py:337). It removes the target table (and
// its alias) qualifier from column refs on the LHS of each WHEN ... UPDATE SET assignment and
// from the INSERT column list, while leaving them intact in conditions and RHS values. It works
// on a Copy so the caller's AST is not mutated (upstream mutates in place via .replace()).
func (g *Generator) mergeWithoutTarget(e expressions.Expression) expressions.Expression {
	e = e.Copy()
	table := asExpression(e.Arg("this"))
	if table == nil {
		return e
	}
	// normalize returns the dialect-normalized name of an identifier for comparison only; it
	// copies first because NormalizeIdentifier mutates its argument in place.
	normalize := func(id expressions.Expression) string {
		if id == nil {
			return ""
		}
		return g.dialect.NormalizeIdentifier(id.Copy()).Name()
	}
	stripColumn := func(col expressions.Expression) expressions.Expression {
		return expressions.Column(expressions.Args{"this": col.Arg("this")})
	}
	isTarget := func(col expressions.Expression) bool {
		return col != nil && col.Kind() == expressions.KindColumn
	}

	targets := map[string]bool{normalize(asExpression(table.Arg("this"))): true}
	if alias := asExpression(table.Arg("alias")); alias != nil {
		targets[normalize(asExpression(alias.Arg("this")))] = true
	}

	whens := asExpression(e.Arg("whens"))
	if whens == nil {
		return e
	}
	for _, when := range whens.Expressions() {
		then := asExpression(when.Arg("then"))
		if then == nil {
			continue
		}
		switch then.Kind() {
		case expressions.KindUpdate:
			for _, eq := range then.FindAll(expressions.KindEQ) {
				lhs := asExpression(eq.Arg("this"))
				if isTarget(lhs) && targets[normalize(asExpression(lhs.Arg("table")))] {
					eq.Set("this", stripColumn(lhs))
				}
			}
		case expressions.KindInsert:
			columnList := asExpression(then.Arg("this"))
			if columnList == nil || columnList.Kind() != expressions.KindTuple {
				continue
			}
			cols := columnList.Expressions()
			newCols := make([]expressions.Expression, len(cols))
			changed := false
			for i, col := range cols {
				if isTarget(col) && targets[normalize(asExpression(col.Arg("table")))] {
					newCols[i] = stripColumn(col)
					changed = true
				} else {
					newCols[i] = col
				}
			}
			if changed {
				columnList.Set("expressions", newCols)
			}
		}
	}
	return e
}

func (g *Generator) mergeSQL(e expressions.Expression) string {
	// Postgres removes the target table qualifier from WHEN clause columns (see
	// mergeWithoutTarget); other dialects render the assignments verbatim.
	if g.dialect.Name == "postgres" {
		e = g.mergeWithoutTarget(e)
	}
	table := asExpression(e.Arg("this"))
	this := g.gen(table)
	using := "USING " + g.sqlKey(e, "using")
	whens := g.sqlKey(e, "whens")
	on := g.sqlKey(e, "on")
	if on != "" {
		on = "ON " + on
	} else {
		on = g.expressions(exprsOptions{expression: e, key: "using_cond"})
		if on != "" {
			on = "USING (" + on + ")"
		}
	}
	returning := g.sqlKey(e, "returning")
	if returning != "" {
		whens += returning
	}
	sep := g.sep()
	return g.prependCtes(e, "MERGE INTO "+this+sep+using+sep+on+sep+whens)
}

func (g *Generator) whenSQL(e expressions.Expression) string {
	matched := "NOT MATCHED"
	if boolValue(e.Arg("matched")) {
		matched = "MATCHED"
	}
	source := ""
	if boolValue(e.Arg("source")) {
		source = " BY SOURCE"
	}
	condition := g.sqlKey(e, "condition")
	if condition != "" {
		condition = " AND " + condition
	}
	thenExpression := asExpression(e.Arg("then"))
	then := ""
	if thenExpression != nil && thenExpression.Kind() == expressions.KindInsert {
		this := g.sqlKey(thenExpression, "this")
		if this != "" {
			this = "INSERT " + this
		} else {
			this = "INSERT"
		}
		then = g.sqlKey(thenExpression, "expression")
		if then != "" {
			then = this + " VALUES " + then
		} else {
			then = this
		}
	} else if thenExpression != nil && thenExpression.Kind() == expressions.KindUpdate {
		// `WHEN MATCHED THEN UPDATE *` (a bare Star, not a SET list) renders `UPDATE *`
		// (generator.py:4755-4756), not `UPDATE SET *`.
		if star := asExpression(thenExpression.Arg("expressions")); star != nil && star.Kind() == expressions.KindStar {
			then = "UPDATE " + g.sqlKey(thenExpression, "expressions")
		} else {
			expressionsSQL := g.expressions(exprsOptions{expression: thenExpression})
			if expressionsSQL != "" {
				then = "UPDATE SET" + g.sep() + expressionsSQL
			} else {
				then = "UPDATE"
			}
		}
	} else {
		then = g.gen(thenExpression)
	}
	if thenExpression != nil && (thenExpression.Kind() == expressions.KindInsert || thenExpression.Kind() == expressions.KindUpdate) {
		then += g.sqlKey(thenExpression, "where")
	}
	return "WHEN " + matched + source + condition + " THEN " + then
}

func (g *Generator) whensSQL(e expressions.Expression) string {
	return g.expressions(exprsOptions{expression: e, sep: " ", noIndent: true})
}

func (g *Generator) onConflictSQL(e expressions.Expression) string {
	conflict := "ON CONFLICT"
	if boolValue(e.Arg("duplicate")) {
		conflict = "ON DUPLICATE KEY"
	}
	constraint := g.sqlKey(e, "constraint")
	if constraint != "" {
		constraint = " ON CONSTRAINT " + constraint
	}
	conflictKeys := g.expressions(exprsOptions{expression: e, key: "conflict_keys", flat: true})
	if conflictKeys != "" {
		conflictKeys = "(" + conflictKeys + ")"
	}
	indexPredicate := g.sqlKey(e, "index_predicate")
	conflictKeys = conflictKeys + indexPredicate + " "
	action := g.sqlKey(e, "action")
	expressionsSQL := g.expressions(exprsOptions{expression: e, flat: true})
	if expressionsSQL != "" {
		// generator.py:2339 DUPLICATE_KEY_UPDATE_WITH_SET (True by default; MySQL's own
		// generator omits SET, since real MySQL syntax is `ON DUPLICATE KEY UPDATE col = ...`).
		setKeyword := ""
		if g.dialect.DuplicateKeyUpdateWithSet {
			setKeyword = "SET "
		}
		expressionsSQL = " " + setKeyword + expressionsSQL
	}
	where := g.sqlKey(e, "where")
	return conflict + constraint + conflictKeys + action + expressionsSQL + where
}

func (g *Generator) returningSQL(e expressions.Expression) string {
	return g.seg("RETURNING") + " " + g.expressions(exprsOptions{expression: e, flat: true})
}

func (g *Generator) createSQL(e expressions.Expression) string {
	kind := g.sqlKey(e, "kind")
	properties := asExpression(e.Arg("properties"))

	// generator.py:1306-1313: a CONSTRAINT TRIGGER stores the modifier on the first
	// TriggerProperties node and restores it in front of the creatable kind here.
	if kind == "TRIGGER" && properties != nil {
		if propertyExpressions := properties.Expressions(); len(propertyExpressions) > 0 {
			first := propertyExpressions[0]
			if first.Kind() == expressions.KindTriggerProperties && boolValue(first.Arg("constraint")) {
				kind = "CONSTRAINT " + kind
			}
		}
	}

	propertyLocations := map[propertyLocation][]expressions.Expression{}
	if properties != nil {
		propertyLocations = g.locateProperties(properties)
	}
	this := g.sqlKey(e, "this")

	propertiesSQL := ""
	postSchema := propertyLocations[propertyLocationPostSchema]
	postWith := propertyLocations[propertyLocationPostWith]
	if len(postSchema) > 0 || len(postWith) > 0 {
		combined := make([]expressions.Expression, 0, len(postSchema)+len(postWith))
		combined = append(combined, postSchema...)
		combined = append(combined, postWith...)
		propertiesSQL = g.gen(g.propertiesExpression(combined, e))
		if len(postSchema) > 0 {
			propertiesSQL = g.sep() + propertiesSQL
		} else if !g.pretty {
			propertiesSQL = " " + propertiesSQL
		}
	}

	begin := ""
	if boolValue(e.Arg("begin")) {
		begin = " BEGIN"
	}
	expressionSQL := g.sqlKey(e, "expression")
	if expressionSQL != "" {
		expressionSQL = begin + g.sep() + expressionSQL
		postAliasSQL := g.renderProperties(
			g.propertiesExpression(propertyLocations[propertyLocationPostAlias], e),
			"", ", ", "", false,
		)
		if postAliasSQL != "" {
			postAliasSQL = " " + postAliasSQL
		}
		expressionSQL = " AS" + postAliasSQL + expressionSQL
	}

	postIndexSQL := g.renderProperties(
		g.propertiesExpression(propertyLocations[propertyLocationPostIndex], e),
		" ", ", ", "", false,
	)
	indexes := g.expressions(exprsOptions{expression: e, key: "indexes", noIndent: true, sep: " "})
	if indexes != "" {
		indexes = " " + indexes
	}
	indexSQL := indexes + postIndexSQL

	clustered := ""
	if clusteredArg := e.Arg("clustered"); clusteredArg != nil {
		if boolValue(clusteredArg) {
			clustered = " CLUSTERED COLUMNSTORE"
		} else {
			clustered = " NONCLUSTERED COLUMNSTORE"
		}
	}
	replace := ""
	if boolValue(e.Arg("replace")) {
		replace = " OR REPLACE"
	}
	refresh := ""
	if boolValue(e.Arg("refresh")) {
		refresh = " OR REFRESH"
	}
	unique := ""
	if boolValue(e.Arg("unique")) {
		unique = " UNIQUE"
	}
	postCreateSQL := g.renderProperties(
		g.propertiesExpression(propertyLocations[propertyLocationPostCreate], e),
		" ", " ", "", false,
	)
	modifiers := clustered + replace + refresh + unique + postCreateSQL

	postExpressionSQL := g.renderProperties(
		g.propertiesExpression(propertyLocations[propertyLocationPostExpression], e),
		" ", " ", "", false,
	)
	concurrently := ""
	if boolValue(e.Arg("concurrently")) {
		concurrently = " CONCURRENTLY"
	}
	exists := ""
	if boolValue(e.Arg("exists")) {
		exists = " IF NOT EXISTS"
	}
	noSchemaBinding := ""
	if boolValue(e.Arg("no_schema_binding")) {
		noSchemaBinding = " WITH NO SCHEMA BINDING"
	}
	clone := g.sqlKey(e, "clone")
	if clone != "" {
		clone = " " + clone
	}

	sql := "CREATE" + modifiers + " " + kind + concurrently + exists + " " + this + propertiesSQL + expressionSQL + postExpressionSQL + indexSQL + noSchemaBinding + clone
	return g.prependCtes(e, sql)
}

func (g *Generator) schemaSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	sql := g.schemaColumnsSQL(e)
	if this != "" && sql != "" {
		return this + " " + sql
	}
	return this + sql
}

func (g *Generator) schemaColumnsSQL(e expressions.Expression) string {
	if len(listFromValue(e.Arg("expressions"))) > 0 {
		return "(" + g.sep("") + g.expressions(exprsOptions{expression: e}) + g.seg(")", "")
	}
	return ""
}

func (g *Generator) columnDefSQL(e expressions.Expression) string {
	column := g.sqlKey(e, "this")
	kind := g.sqlKey(e, "kind")

	// Postgres places a function parameter mode (IN/OUT/INOUT/VARIADIC) BEFORE the
	// parameter name and omits it from the trailing constraints
	// (generators/postgres.py:409-419). Unlike upstream, which pop()s the constraint,
	// we render around it so Generate() stays side-effect free.
	modePrefix := ""
	if g.dialect.Name == "postgres" {
		for _, item := range listFromValue(e.Arg("constraints")) {
			if c, ok := item.(expressions.Expression); ok && c != nil && c.Kind() == expressions.KindInOutColumnConstraint {
				modePrefix = g.gen(c) + " "
				break
			}
		}
	}

	var constraints string
	if modePrefix != "" {
		parts := make([]string, 0)
		for _, item := range listFromValue(e.Arg("constraints")) {
			c, ok := item.(expressions.Expression)
			if !ok || c == nil || c.Kind() == expressions.KindInOutColumnConstraint {
				continue
			}
			parts = append(parts, g.gen(c))
		}
		constraints = strings.Join(parts, " ")
	} else {
		constraints = g.expressions(exprsOptions{expression: e, key: "constraints", sep: " ", flat: true})
	}
	exists := ""
	if boolValue(e.Arg("exists")) {
		exists = "IF NOT EXISTS "
	}
	if kind != "" {
		kind = " " + kind
	}
	if constraints != "" {
		constraints = " " + constraints
	}
	position := g.sqlKey(e, "position")
	if position != "" {
		position = " " + position
	}

	if e.Find(expressions.KindComputedColumnConstraint) != nil && !g.dialect.ComputedColumnWithType {
		kind = ""
	}

	return modePrefix + exists + column + kind + constraints + position
}

func (g *Generator) commandSQL(e expressions.Expression) string {
	return strings.TrimSpace(g.sqlKey(e, "this") + " " + strings.TrimSpace(e.Text("expression")))
}

// mysqlUnsignedTypeMapping ports MySQL's UNSIGNED_TYPE_MAPPING (generators/mysql.py:236-244):
// each unsigned integer/decimal DType renders as its signed base name followed by " UNSIGNED"
// (e.g. UINT -> "INT UNSIGNED", UBIGINT(20) -> "BIGINT(20) UNSIGNED"). This is the self-contained
// subset of MySQL's TYPE_MAPPING needed for the DDL slice; the broader per-dialect TYPE_MAPPING
// table (ROADMAP 5b) is still out of scope, but these seven fixed entries don't require it.
var mysqlUnsignedTypeMapping = map[expressions.DType]string{
	expressions.DTypeUBigInt:    "BIGINT",
	expressions.DTypeUInt:       "INT",
	expressions.DTypeUMediumInt: "MEDIUMINT",
	expressions.DTypeUSmallInt:  "SMALLINT",
	expressions.DTypeUTinyInt:   "TINYINT",
	expressions.DTypeUDecimal:   "DECIMAL",
	expressions.DTypeUDouble:    "DOUBLE",
}

// typeMappingTable selects the TYPE_MAPPING table datatype_sql's generic lookup consults for
// the current dialect: mysqlTypeMapping is a full replacement (generators/mysql.py:255-273),
// postgresTypeMapping is base typeMapping plus postgres's delta (generators/postgres.py:271-284),
// and every other dialect (including base) uses the base typeMapping as-is.
func (g *Generator) typeMappingTable() map[expressions.DType]string {
	switch g.dialect.Name {
	case "mysql":
		return mysqlTypeMapping
	case "postgres":
		return postgresTypeMapping
	default:
		return typeMapping
	}
}

// dataTypeSQL ports datatype_sql (generator.py:1706-...) plus dialect-specific overrides:
// MySQL's VARCHAR_REQUIRES_SIZE -> TEXT rewrite (generators/mysql.py:670-677), MySQL's
// UNSIGNED_TYPE_MAPPING suffix (generators/mysql.py:678-684, via mysqlUnsignedTypeMapping
// above), Postgres's ARRAY/ENUM/FLOAT-with-precision rendering (generators/postgres.py:
// 477-489), and the dialect-aware TYPE_MAPPING table lookup (typeMappingTable above).
func (g *Generator) dataTypeSQL(e expressions.Expression) string {
	if g.dialect.Name == "mysql" {
		if g.dialect.VarcharRequiresSize && expressions.IsType(e, expressions.DTypeVarchar) && len(listFromValue(e.Arg("expressions"))) == 0 {
			// VARCHAR must always have a size - if it doesn't, we always generate TEXT.
			return "TEXT"
		}
	} else if g.dialect.Name == "postgres" {
		if expressions.IsType(e, expressions.DTypeArray) {
			if len(listFromValue(e.Arg("expressions"))) > 0 {
				valuesSQL := g.expressions(exprsOptions{expression: e, key: "values", flat: true})
				return g.expressions(exprsOptions{expression: e, flat: true}) + "[" + valuesSQL + "]"
			}
			return "ARRAY"
		}
		if expressions.IsType(e, expressions.DTypeEnum) {
			return "ENUM (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
		}
		if (expressions.IsType(e, expressions.DTypeDouble) || expressions.IsType(e, expressions.DTypeFloat)) && len(listFromValue(e.Arg("expressions"))) > 0 {
			// Postgres doesn't support precision for REAL and DOUBLE PRECISION types.
			return "FLOAT(" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
		}
	}

	nested := ""
	values := ""
	exprNested := truthy(e.Arg("nested"))
	typeValue := e.Arg("this")
	// MySQL renders unsigned numeric types as "<signed base> UNSIGNED" (see mysqlUnsignedTypeMapping).
	mysqlUnsignedBase := ""
	if g.dialect.Name == "mysql" {
		if dt, ok := typeValue.(expressions.DType); ok {
			mysqlUnsignedBase = mysqlUnsignedTypeMapping[dt]
		}
	}
	var interior string
	if exprNested && g.pretty {
		interior = g.expressions(exprsOptions{expression: e, dynamic: true, newLine: true, skipFirst: true, skipLast: true})
	} else {
		interior = g.expressions(exprsOptions{expression: e, flat: true})
	}
	var typeSQL string
	switch tv := typeValue.(type) {
	case expressions.DType:
		if mysqlUnsignedBase != "" {
			typeSQL = mysqlUnsignedBase
		} else if tv == expressions.DTypeUserDefined && truthy(e.Arg("kind")) {
			typeSQL = g.sqlKey(e, "kind")
		} else if tv == expressions.DTypeCharacterSet {
			return "CHAR CHARACTER SET " + g.sqlKey(e, "kind")
		} else if mapped, ok := g.typeMappingTable()[tv]; ok {
			typeSQL = mapped
		} else {
			typeSQL = string(tv)
		}
	case expressions.Expression:
		typeSQL = g.gen(tv)
	case string:
		typeSQL = tv
	default:
		typeSQL = fmt.Sprint(tv)
	}
	if interior != "" {
		if exprNested {
			nested = "<" + interior + ">"
			if e.Arg("values") != nil {
				delims := [2]string{"(", ")"}
				if typeValue == expressions.DTypeArray {
					delims = [2]string{"[", "]"}
				}
				valuesSQL := g.expressions(exprsOptions{expression: e, key: "values", flat: true})
				values = delims[0] + valuesSQL + delims[1]
			}
		} else if typeValue == expressions.DTypeInterval {
			nested = " " + interior
		} else {
			nested = "(" + interior + ")"
		}
	}
	typeSQL = typeSQL + nested + values
	if mysqlUnsignedBase != "" {
		// Append the UNSIGNED suffix after the base name + any size/params (mysql.py:681-683
		// does `f"{super().datatype_sql(...)} UNSIGNED"`), e.g. "BIGINT(20) UNSIGNED".
		typeSQL += " UNSIGNED"
	}
	collate := g.sqlKey(e, "collate")
	if collate != "" {
		typeSQL += " COLLATE " + collate
	}
	return typeSQL
}

func (g *Generator) dataTypeParamSQL(e expressions.Expression) string {
	return g.sqlKey(e, "this")
}

// pseudoTypeSQL/objectIdentifierSQL port generator.py:2316-2320 pseudotype_sql/
// objectidentifier_sql: both just re-emit the uppercased token text stored in "this"
// (see parser_types.go parseTypes' PSEUDO_TYPE/OBJECT_IDENTIFIER branches).
func (g *Generator) pseudoTypeSQL(e expressions.Expression) string { return e.Name() }

func (g *Generator) objectIdentifierSQL(e expressions.Expression) string { return e.Name() }

// atTimeZoneSQL ports generator.py:3995-3998 attimezone_sql, plus mysql's override
// (generators/mysql.py:796-798): MySQL has no AT TIME ZONE syntax, so it drops the zone
// entirely and flags the query unsupported, keeping just the wrapped expression.
func (g *Generator) atTimeZoneSQL(e expressions.Expression) string {
	if g.dialect.Name == "mysql" {
		g.unsupported("AT TIME ZONE is not supported by MySQL")
		return g.sqlKey(e, "this")
	}
	this := g.sqlKey(e, "this")
	zone := g.sqlKey(e, "zone")
	return this + " AT TIME ZONE " + zone
}

func (g *Generator) castSQL(e expressions.Expression) string { return g.castSQLWithPrefix(e, "") }

func (g *Generator) castSQLWithPrefix(e expressions.Expression, safePrefix string) string {
	formatSQL := g.sqlKey(e, "format")
	if formatSQL != "" {
		formatSQL = " FORMAT " + formatSQL
	}

	toExpr := asExpression(e.Arg("to"))

	// MySQL's cast_sql override (generators/mysql.py:689-697): a CAST to a timezone-aware
	// timestamp type renders as a TIMESTAMP(...) function call (bypassing CAST entirely, and
	// any FORMAT/DEFAULT/action clauses along with it, matching upstream's early return), and
	// a CAST to a type outside MySQL's narrow CAST-target vocabulary is rewritten to
	// CHAR/SIGNED/UNSIGNED (mysqlCastMapping) before the ordinary CAST(... AS ...) below. The
	// rename is computed on a copy of the "to" DataType so the caller's expression tree (which
	// Generate already Copy()'d once at the top) isn't mutated a second time here.
	if g.dialect.Name == "mysql" && toExpr != nil {
		if dt, ok := toExpr.Arg("this").(expressions.DType); ok {
			if mysqlTimestampFuncTypes[dt] {
				return g.funcCall("TIMESTAMP", []any{e.Arg("this")}, "(", ")", true)
			}
			if renamed, ok := mysqlCastMapping[dt]; ok {
				toCopy := toExpr.Copy()
				toCopy.Set("this", renamed)
				toExpr = toCopy
			}
		}
	}

	toSQL := ""
	if toExpr != nil {
		toSQL = " " + g.gen(toExpr)
	}
	action := g.sqlKey(e, "action")
	if action != "" {
		action = " " + action
	}
	defaultSQL := g.sqlKey(e, "default")
	if defaultSQL != "" {
		defaultSQL = " DEFAULT " + defaultSQL + " ON CONVERSION ERROR"
	}
	return safePrefix + "CAST(" + g.sqlKey(e, "this") + " AS" + toSQL + defaultSQL + formatSQL + action + ")"
}

func (g *Generator) tryCastSQL(e expressions.Expression) string {
	// MySQL and Postgres have no TRY_CAST; upstream routes exp.TryCast through no_trycast_sql
	// (dialects/dialect.py:1261), which renders a plain CAST - so a MySQL cast-target rename
	// (e.g. TEXT -> CHAR) applies without a stray TRY_ prefix. See generators/mysql.py:223 and
	// generators/postgres.py:377. Every other dialect keeps TRY_CAST (generator.py:4523).
	if g.dialect.Name == "mysql" || g.dialect.Name == "postgres" {
		return g.castSQLWithPrefix(e, "")
	}
	return g.castSQLWithPrefix(e, "TRY_")
}
func (g *Generator) jsonCastSQL(e expressions.Expression) string { return g.castSQLWithPrefix(e, "") }

// formatJSONSQL ports generator.py:3778 formatjson_sql.
func (g *Generator) formatJSONSQL(e expressions.Expression) string {
	return g.sqlKey(e, "this") + " FORMAT JSON"
}

// jsonColumnDefSQL ports generator.py:3838 jsoncolumndef_sql.
func (g *Generator) jsonColumnDefSQL(e expressions.Expression) string {
	path := g.sqlKey(e, "path")
	if path != "" {
		path = " PATH " + path
	}
	nestedSchema := g.sqlKey(e, "nested_schema")
	if nestedSchema != "" {
		return "NESTED" + path + " " + nestedSchema
	}
	this := g.sqlKey(e, "this")
	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind = " " + kind
	}
	formatJSON := ""
	if boolValue(e.Arg("format_json")) {
		formatJSON = " FORMAT JSON"
	}
	ordinality := ""
	if boolValue(e.Arg("ordinality")) {
		ordinality = " FOR ORDINALITY"
	}
	return this + kind + formatJSON + path + ordinality
}

// jsonSchemaSQL ports generator.py:3854 jsonschema_sql.
func (g *Generator) jsonSchemaSQL(e expressions.Expression) string {
	return g.funcCall("COLUMNS", listFromValue(e.Arg("expressions")), "(", ")", true)
}

// jsonTableSQL ports generator.py:3857 jsontable_sql. error_handling/empty_handling are
// rendered via sqlKey (rather than upstream's raw f-string) so they work whether
// parseOnHandling produced a literal string ("ERROR ON ERROR") or a DEFAULT <expr> node.
func (g *Generator) jsonTableSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	path := g.sqlKey(e, "path")
	if path != "" {
		path = ", " + path
	}
	errorHandling := g.sqlKey(e, "error_handling")
	if errorHandling != "" {
		errorHandling = " " + errorHandling
	}
	emptyHandling := g.sqlKey(e, "empty_handling")
	if emptyHandling != "" {
		emptyHandling = " " + emptyHandling
	}
	schema := g.sqlKey(e, "schema")
	suffix := path + errorHandling + emptyHandling + " " + schema + ")"
	return g.funcCall("JSON_TABLE", []any{this}, "(", suffix, true)
}

func (g *Generator) extractSQL(e expressions.Expression) string {
	this := g.gen(e.Arg("this"))
	expressionSQL := g.sqlKey(e, "expression")
	return "EXTRACT(" + this + " FROM " + expressionSQL + ")"
}

// trimSQL ports trim_sql (generator.py:3624-3633): the base LTRIM/RTRIM/TRIM(this, expr) form.
// mysql and postgres both override exp.Trim to trimSQLStandard instead (dialect.py:1782-1797,
// wired at generators/mysql.py:221 and generators/postgres.py:376).
func (g *Generator) trimSQL(e expressions.Expression) string {
	if g.dialect.Name == "mysql" || g.dialect.Name == "postgres" {
		return g.trimSQLStandard(e)
	}
	return g.trimSQLBase(e)
}

func (g *Generator) trimSQLBase(e expressions.Expression) string {
	trimType := g.sqlKey(e, "position")
	funcName := "TRIM"
	if trimType == "LEADING" {
		funcName = "LTRIM"
	} else if trimType == "TRAILING" {
		funcName = "RTRIM"
	}
	var this any = e.Arg("this")
	// Upstream's _parse_bitwise folds a trailing COLLATE into a Collate node wrapping the operand
	// it follows (verified against sqlglot v30.12.0), so upstream's trim_sql re-renders the
	// COLLATE for free out of `this` and never reads a separate collation arg. This port has no
	// Collate node (expressions has no KindCollate) and keeps "collation" as Trim's own arg
	// (parser/parser_functions.go:107-109), so splice it back onto the target here whenever it's
	// present - with OR without a remove-chars operand. Examples (base dialect):
	//   TRIM(LEADING ' XXX ' COLLATE "de_DE")          -> LTRIM(' XXX ' COLLATE "de_DE")
	//   TRIM(BOTH 'bla' FROM ' XXX ' COLLATE utf8_bin) -> TRIM(' XXX ' COLLATE utf8_bin, 'bla')
	if collation := g.sqlKey(e, "collation"); collation != "" {
		this = g.sqlKey(e, "this") + " COLLATE " + collation
	}
	return g.funcCall(funcName, []any{this, e.Arg("expression")}, "(", ")", true)
}

// trimSQLStandard ports trim_sql (dialect.py:1782-1797): the SQL-standard TRIM(<position>
// <remove_chars> FROM <target> COLLATE <collation>) form shared by mysql and postgres. When
// there's no "remove chars" expression, it falls back to the LTRIM/RTRIM/TRIM(this, expr) form
// (trimSQLBase) instead, since that's the more idiomatic rendering for a plain whitespace trim.
func (g *Generator) trimSQLStandard(e expressions.Expression) string {
	removeChars := g.sqlKey(e, "expression")
	if removeChars == "" {
		return g.trimSQLBase(e)
	}

	target := g.sqlKey(e, "this")
	trimType := g.sqlKey(e, "position")
	collation := g.sqlKey(e, "collation")

	if trimType != "" {
		trimType += " "
	}
	removeChars += " "
	fromPart := ""
	if trimType != "" || removeChars != "" {
		fromPart = "FROM "
	}
	if collation != "" {
		collation = " COLLATE " + collation
	}
	return "TRIM(" + trimType + removeChars + fromPart + target + collation + ")"
}

func (g *Generator) ceilFloorSQL(e expressions.Expression) string {
	toClause := g.sqlKey(e, "to")
	if toClause != "" {
		return g.sqlName(e.Kind()) + "(" + g.sqlKey(e, "this") + " TO " + toClause + ")"
	}
	return g.functionFallbackSQL(e)
}

func (g *Generator) anonymousSQL(e expressions.Expression) string {
	parent := e.Parent()
	isQualified := parent != nil && parent.Kind() == expressions.KindDot && asExpression(parent.Arg("expression")) == e
	return g.funcCall(g.sqlKey(e, "this"), listFromValue(e.Arg("expressions")), "(", ")", !isQualified)
}

func (g *Generator) hexSQL(e expressions.Expression) string {
	return g.funcCall("HEX", []any{e.Arg("this")}, "(", ")", true)
}

func (g *Generator) arrayAggSQL(e expressions.Expression) string { return g.functionFallbackSQL(e) }

// arraySizeSQL ports arraysize_sql (generator.py:5798-5811). ARRAY_SIZE_DIM_REQUIRED
// (generator.py:588, generators/postgres.py:254) is nil/false for base (drop a `1`
// dimension arg, warn on anything else), true for postgres (keep it, defaulting to `1`
// when omitted) - modeled here as a dialect-name check, matching the existing
// g.dialect.Name-gated overrides elsewhere in this package (e.g. varianceSQL).
func (g *Generator) arraySizeSQL(e expressions.Expression) string {
	dim := asExpression(e.Arg("expression"))
	if g.dialect.Name == "postgres" {
		if dim == nil {
			dim = expressions.LiteralNumber(1)
		}
	} else if dim != nil {
		if !(dim.IsInt() && dim.Name() == "1") {
			g.unsupported("Cannot transpile dimension argument for ARRAY_LENGTH")
		}
		dim = nil
	}
	return g.funcCall("ARRAY_LENGTH", []any{e.Arg("this"), dim}, "(", ")", true)
}

func (g *Generator) initcapSQL(e expressions.Expression) string {
	delimiters := asExpression(e.Arg("expression"))
	if delimiters != nil && delimiters.IsString() && delimiters.Name() == initcapDefaultDelimiterChars {
		delimiters = nil
	}
	return g.funcCall("INITCAP", []any{e.Arg("this"), delimiters}, "(", ")", true)
}

// dateAddSQL ports the base generator's generic dateadd_sql (generator.py:5158-5164) for
// exp.DateAdd, plus MySQL's and Postgres's per-dialect overrides
// (generators/mysql.py:74-83 date_add_sql("ADD"), generators/postgres.py:55-77
// _date_add_sql("+")). Divergence from those overrides: upstream's dialect-specific
// FUNCTIONS parser entry (build_date_delta_with_interval, parsers/mysql.py:114) flattens
// `DATE_ADD(x, INTERVAL y UNIT)` at parse time into DateAdd(this=x, expression=y,
// unit=Var(UNIT)); this port's DATE_ADD FUNCTIONS entry is still the generic positional
// builder (expressions/functions.go genericFunction), so "expression" stays the whole
// parsed Interval node and "unit" is never set on DateAdd directly. mysqlDateAddSQL/
// postgresDateAddSQL below detect and unwrap that unflattened shape so the rendered SQL
// still matches upstream byte-for-byte, without requiring the parser-side flattening
// (out of this part's scope: dialect FUNCTIONS registries belong to a sibling part).
func (g *Generator) dateAddSQL(e expressions.Expression) string {
	switch g.dialect.Name {
	case "mysql":
		return g.mysqlDateAddSQL("ADD", e)
	case "postgres":
		return g.postgresDateAddSQL("+", e)
	}
	return g.funcCall("DATE_ADD", []any{e.Arg("this"), e.Arg("expression"), unitToStr(e)}, "(", ")", true)
}

// mysqlDateAddSQL ports date_add_sql (generators/mysql.py:74-83): reconstructs
// `DATE_<kind>(this, INTERVAL <value> <unit>)` from the DateAdd's flattened
// expression/unit. See dateAddSQL's comment for why this port's parse doesn't actually
// flatten - dateIntervalArg below recovers the equivalent (this, unit) pair either way.
func (g *Generator) mysqlDateAddSQL(kind string, e expressions.Expression) string {
	value, unit := dateIntervalArg(e)
	interval := expressions.Interval(expressions.Args{"this": value, "unit": unitToVarValue(unit)})
	return g.funcCall("DATE_"+kind, []any{e.Arg("this"), interval}, "(", ")", true)
}

// postgresDateAddSQL ports _date_add_sql (generators/postgres.py:55-77), restricted to the
// isinstance(e, exp.Interval) branch: the other branches simplify a non-Interval second
// argument (a bare numeric day offset) via the optimizer's simplify() pass, which is out
// of scope for the gap this part targets (no round-trip corpus case exercises it for
// base/mysql/postgres). If the second argument isn't already an Interval, fall back to the
// generic `DATE_ADD(...)` rendering rather than guessing at upstream's simplify output.
func (g *Generator) postgresDateAddSQL(kind string, e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	value, unit := dateIntervalArg(e)
	if value == nil {
		return g.funcCall("DATE_ADD", []any{e.Arg("this"), e.Arg("expression"), unitToStr(e)}, "(", ")", true)
	}
	interval := expressions.Interval(expressions.Args{"this": value, "unit": unit})
	return this + " " + kind + " " + g.gen(interval)
}

// dateIntervalArg recovers the (value, unit) pair a DateAdd's "expression"/"unit" args
// represent, regardless of whether they were flattened at parse time (upstream's shape:
// unit set directly on DateAdd) or left as a single nested Interval (this port's shape,
// see dateAddSQL's comment). Returns (nil, nil) if "expression" isn't an Interval and no
// top-level "unit" was set, i.e. this port's parse produced neither shape.
func dateIntervalArg(e expressions.Expression) (value any, unit expressions.Expression) {
	if u := asExpression(e.Arg("unit")); u != nil {
		return e.Arg("expression"), u
	}
	if iv := asExpression(e.Arg("expression")); iv != nil && iv.Kind() == expressions.KindInterval {
		return iv.Arg("this"), asExpression(iv.Arg("unit"))
	}
	return nil, nil
}

func unitToStr(e expressions.Expression) expressions.Expression {
	return unitToStrValue(asExpression(e.Arg("unit")))
}

func unitToStrValue(unit expressions.Expression) expressions.Expression {
	if unit == nil {
		return expressions.LiteralString("DAY")
	}
	if unit.Kind() == expressions.KindPlaceholder || (unit.Kind() != expressions.KindVar && unit.Kind() != expressions.KindLiteral) {
		return unit
	}
	return expressions.LiteralString(unit.Name())
}

// unitToVarValue ports unit_to_var (dialects/dialect.py:2017-2023): unlike unitToStrValue,
// a Var/Placeholder/Column unit is kept as-is (not stringified), and a nil unit defaults to
// a bare Var("DAY") rather than a string literal.
func unitToVarValue(unit expressions.Expression) expressions.Expression {
	if unit != nil {
		switch unit.Kind() {
		case expressions.KindVar, expressions.KindPlaceholder, expressions.KindColumn:
			return unit
		}
	}
	value := "DAY"
	if unit != nil {
		value = unit.Name()
	}
	if value == "" {
		return nil
	}
	return expressions.Var(expressions.Args{"this": value})
}

func (g *Generator) logSQL(e expressions.Expression) string {
	return g.funcCall("LOG", []any{e.Arg("this"), e.Arg("expression")}, "(", ")", true)
}

func (g *Generator) pivotInValueAliases(e expressions.Expression) []expressions.Expression {
	// Base parse and generate use the same pivot naming settings; parser_class attributes are slice-5 work.
	return nil
}

func (g *Generator) pivotSQL(e expressions.Expression) string {
	expressionsSQL := g.expressions(exprsOptions{expression: e, flat: true})
	direction := "PIVOT"
	if boolValue(e.Arg("unpivot")) {
		direction = "UNPIVOT"
	}
	group := g.sqlKey(e, "group")
	if truthy(e.Arg("this")) {
		this := g.sqlKey(e, "this")
		var sql string
		if expressionsSQL == "" {
			sql = "UNPIVOT " + this
		} else {
			on := g.seg("ON") + " " + expressionsSQL
			into := g.sqlKey(e, "into")
			if into != "" {
				into = g.seg("INTO") + " " + into
			}
			using := g.expressions(exprsOptions{expression: e, key: "using", flat: true})
			if using != "" {
				using = g.seg("USING") + " " + using
			}
			sql = direction + " " + this + on + into + using + group
		}
		return g.prependCtes(e, sql)
	}
	alias := g.sqlKey(e, "alias")
	if alias != "" {
		alias = " AS " + alias
	}
	fields := g.expressions(exprsOptions{expression: e, key: "fields", sep: " ", dynamic: true, newLine: true, skipFirst: true, skipLast: true})
	nulls := ""
	if v, ok := e.Arg("include_nulls").(bool); ok {
		if v {
			nulls = " INCLUDE NULLS "
		} else {
			nulls = " EXCLUDE NULLS "
		}
	}
	defaultOnNull := g.sqlKey(e, "default_on_null")
	if defaultOnNull != "" {
		defaultOnNull = " DEFAULT ON NULL (" + defaultOnNull + ")"
	}
	sql := g.seg(direction) + nulls + "(" + expressionsSQL + " FOR " + fields + defaultOnNull + group + ")" + alias
	return g.prependCtes(e, sql)
}

func (g *Generator) lateralOp(e expressions.Expression) string {
	crossApply, ok := e.Arg("cross_apply").(bool)
	if ok {
		if crossApply {
			return "INNER JOIN LATERAL"
		}
		return "LEFT JOIN LATERAL"
	}
	return "LATERAL"
}

func (g *Generator) lateralSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if boolValue(e.Arg("view")) {
		alias := asExpression(e.Arg("alias"))
		columns := g.expressions(exprsOptions{expression: alias, key: "columns", flat: true})
		table := ""
		if alias != nil && alias.Name() != "" {
			table = " " + alias.Name()
		}
		if columns != "" {
			columns = " AS " + columns
		}
		opSQL := g.seg("LATERAL VIEW")
		if boolValue(e.Arg("outer")) {
			opSQL = g.seg("LATERAL VIEW OUTER")
		}
		return opSQL + g.sep() + this + table + columns
	}
	alias := g.sqlKey(e, "alias")
	if alias != "" {
		alias = " AS " + alias
	}
	ordinality := e.Text("ordinality")
	if truthy(e.Arg("ordinality")) {
		ordinality = " WITH ORDINALITY" + alias
		alias = ""
	}
	return g.lateralOp(e) + " " + this + alias + ordinality
}

func (g *Generator) valuesSQL(e expressions.Expression) string {
	// generator.py:2676-2718 values_sql. A VALUES node stays a table constructor when the
	// dialect sets VALUES_AS_TABLE, or when it is an INSERT source (not under FROM/JOIN);
	// otherwise (MySQL, generators/mysql.py:139) it is rewritten into SELECT unions.
	if g.dialect.ValuesAsTable || e.FindAncestor(expressions.KindFrom, expressions.KindJoin) == nil {
		args := g.expressions(exprsOptions{expression: e})
		alias := g.sqlKey(e, "alias")
		values := "VALUES" + g.seg("") + args
		parent := e.Parent()
		// generator.py:2684-2689 wraps a derived VALUES only when WRAP_DERIVED_VALUES is set
		// (MySQL clears it, so `VALUES (1, 2) AS t` stays bare — generators/mysql.py:148).
		if g.dialect.WrapDerivedValues && (alias != "" || (parent != nil && (parent.Kind() == expressions.KindFrom || parent.Kind() == expressions.KindTable))) {
			values = "(" + values + ")"
		}
		values = g.queryModifiers(e, values)
		if alias != "" {
			return values + " AS " + alias
		}
		return values
	}

	// generator.py:2693-2718 — convert `VALUES (...), (...)` under FROM/JOIN into a series of
	// SELECT unions, aliasing the first row's values from the table alias's column list.
	aliasNode := asExpression(e.Arg("alias"))
	var columnNames []any
	if aliasNode != nil {
		columnNames = listFromValue(aliasNode.Arg("columns"))
	}

	tuples := listFromValue(e.Arg("expressions"))
	selects := make([]expressions.Expression, 0, len(tuples))
	for i, tupleValue := range tuples {
		rowValues := listFromValue(asExpression(tupleValue).Arg("expressions"))
		row := make([]expressions.Expression, len(rowValues))
		for j, value := range rowValues {
			row[j] = asExpression(value)
			if i == 0 && j < len(columnNames) {
				row[j] = expressions.AliasExpr(row[j], columnNames[j], false)
			}
		}
		selects = append(selects, expressions.Select(expressions.Args{"expressions": row}))
	}

	if g.pretty && len(selects) > 0 {
		// Fold into an exp.Union tree so the pretty-printer can format the branches
		// (generator.py:2709-2714).
		query := selects[0]
		for _, sel := range selects[1:] {
			query = expressions.Union(expressions.Args{"this": query, "expression": sel, "distinct": false})
		}
		sub := expressions.Args{"this": query}
		if aliasNode != nil {
			sub["alias"] = expressions.TableAlias(expressions.Args{"this": aliasNode.Arg("this")})
		}
		return g.subquerySQL(expressions.Subquery(sub))
	}

	// generator.py:2716-2718 non-pretty path: join the SELECT branches with UNION ALL.
	alias := ""
	if aliasNode != nil {
		alias = " AS " + g.sqlKey(aliasNode, "this")
	}
	unions := make([]string, len(selects))
	for i, sel := range selects {
		unions[i] = g.gen(sel)
	}
	return "(" + strings.Join(unions, " UNION ALL ") + ")" + alias
}

func (g *Generator) unnestSQL(e expressions.Expression) string {
	args := g.expressions(exprsOptions{expression: e, flat: true})
	aliasExpr := asExpression(e.Arg("alias"))
	offset := e.Arg("offset")

	// Base UNNEST_WITH_ORDINALITY = true: fold a WITH OFFSET AS <col> (offset is an
	// Expression) into the alias column list and clear the offset (generator.py:3444-3447).
	if offsetExpr := asExpression(offset); aliasExpr != nil && offsetExpr != nil {
		aliasExpr.Append("columns", offsetExpr)
		// Clear only the ARG, not the local: upstream keeps `offset` truthy so WITH
		// ORDINALITY still emits after folding WITH OFFSET AS <col> into the alias
		// (generator.py:3444-3447 vs 3456-3457).
		e.Set("offset", nil)
	}

	// Base UNNEST_COLUMN_ONLY = false, so the else branch is taken for base.
	var alias string
	if aliasExpr != nil && g.dialect.UnnestColumnOnly {
		if columns := listFromValue(aliasExpr.Arg("columns")); len(columns) > 0 {
			alias = g.gen(columns[0])
		}
	} else {
		alias = g.gen(aliasExpr)
	}
	if alias != "" {
		alias = " AS " + alias
	}

	// WITH ORDINALITY is emitted whenever offset is set, independent of the alias
	// (generator.py:3456-3457) -- keep this outside the alias check.
	suffix := alias
	if truthy(offset) {
		suffix = " WITH ORDINALITY" + alias
	}
	return "UNNEST(" + args + ")" + suffix
}

func (g *Generator) bracketSQL(e expressions.Expression) string {
	// Base IndexOffset is 0, so apply_index_offset is unnecessary for slice 2.
	exprs := listFromValue(e.Arg("expressions"))
	sqls := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		sqls = append(sqls, g.gen(expr))
	}
	return g.sqlKey(e, "this") + "[" + strings.Join(sqls, ", ") + "]"
}

// inlineArraySQL ports inline_array_sql (dialects/dialect.py:1218-1219): the postgres
// bracket-notation array body, e.g. `[1, 2, 3]`, with the same dynamic pretty line-wrap as
// any other comma list. Consumed by arraySQL (generator/residual_tail.go); the postgres
// ARRAY[...] literal now parses to a dedicated exp.Array node (parser _parse_bracket's
// ARRAY_CONSTRUCTORS swap), so bracketSQL itself no longer special-cases it.
func (g *Generator) inlineArraySQL(e expressions.Expression) string {
	return "[" + g.expressions(exprsOptions{expression: e, dynamic: true, newLine: true, skipFirst: true, skipLast: true}) + "]"
}

// intervalSQL ports interval_sql (generator.py:3910-3930). unit_expression carries the
// full unit sub-expression (e.g. an IntervalSpan for "DAY TO SECOND") so it's read before
// TIME_PART_SINGULARS/pluralization collapses it down to a bare unit string.
func (g *Generator) intervalSQL(e expressions.Expression) string {
	unitExpression := e.Arg("unit")
	unit := ""
	if truthy(unitExpression) {
		unit = g.gen(unitExpression)
	}
	if !g.intervalAllowsPluralForm {
		if singular, ok := timePartSingulars[unit]; ok {
			unit = singular
		}
	}
	if unit != "" {
		unit = " " + unit
	}

	if g.singleStringInterval {
		this := ""
		if thisExpr := asExpression(e.Arg("this")); thisExpr != nil {
			this = thisExpr.Name()
		}
		if this != "" {
			if unitExprAsExpression := asExpression(unitExpression); unitExprAsExpression != nil && unitExprAsExpression.Kind() == expressions.KindIntervalSpan {
				return "INTERVAL '" + this + "'" + unit
			}
			return "INTERVAL '" + this + unit + "'"
		}
		return "INTERVAL" + unit
	}

	this := g.sqlKey(e, "this")
	if this != "" {
		thisExpr := asExpression(e.Arg("this"))
		unwrapped := thisExpr != nil && (thisExpr.Kind() == expressions.KindColumn || thisExpr.Kind() == expressions.KindLiteral || thisExpr.Kind() == expressions.KindNeg || thisExpr.Kind() == expressions.KindParen)
		if unwrapped {
			this = " " + this
		} else {
			this = " (" + this + ")"
		}
	}
	return "INTERVAL" + this + unit
}

func (g *Generator) tableParts(e expressions.Expression) string {
	parts := []string{}
	for _, key := range []string{"catalog", "db", "this"} {
		if v := e.Arg(key); v != nil {
			parts = append(parts, g.gen(v))
		}
	}
	return strings.Join(parts, ".")
}

func (g *Generator) tableColumnSQL(e expressions.Expression) string { return g.sqlKey(e, "this") }

func (g *Generator) tableSQL(e expressions.Expression) string {
	table := g.tableParts(e)
	only := ""
	if boolValue(e.Arg("only")) {
		only = "ONLY "
	}
	partition := g.sqlKey(e, "partition")
	if partition != "" {
		partition = " " + partition
	}
	version := g.sqlKey(e, "version")
	if version != "" {
		version = " " + version
	}
	alias := g.sqlKey(e, "alias")
	if alias != "" {
		alias = " AS " + alias
	}
	sample := g.sqlKey(e, "sample")
	postAlias := ""
	preAlias := ""
	if g.dialect.AliasPostTablesample {
		preAlias = sample
	} else {
		postAlias = sample
	}
	if g.dialect.AliasPostVersion {
		preAlias += version
	} else {
		postAlias += version
	}
	hints := g.expressions(exprsOptions{expression: e, key: "hints", sep: " "})
	if hints != "" {
		hints = " " + hints
	}
	pivots := g.expressions(exprsOptions{expression: e, key: "pivots", emptySep: true, flat: true})
	joins := g.indent(g.expressions(exprsOptions{expression: e, key: "joins", emptySep: true, flat: true}), 0, nil, true, false)
	laterals := g.expressions(exprsOptions{expression: e, key: "laterals", emptySep: true})
	fileFormat := g.sqlKey(e, "format")
	pattern := g.sqlKey(e, "pattern")
	if fileFormat != "" {
		if pattern != "" {
			pattern = ", PATTERN => " + pattern
		}
		fileFormat = " (FILE_FORMAT => " + fileFormat + pattern + ")"
	} else if pattern != "" {
		fileFormat = " (PATTERN => " + pattern + ")"
	}
	ordinality := ""
	if truthy(e.Arg("ordinality")) {
		ordinality = " WITH ORDINALITY" + alias
		alias = ""
	}
	when := g.sqlKey(e, "when")
	if when != "" {
		table = table + " " + when
	}
	changes := g.sqlKey(e, "changes")
	if changes != "" {
		changes = " " + changes
	}
	rowsFrom := g.expressions(exprsOptions{expression: e, key: "rows_from"})
	if rowsFrom != "" {
		table = "ROWS FROM " + g.wrap(rowsFrom)
	}
	indexed := ""
	if v := e.Arg("indexed"); v != nil {
		if b, ok := v.(bool); ok && !b {
			indexed = " NOT INDEXED"
		} else {
			indexed = " INDEXED BY " + g.gen(v)
		}
	}
	return only + table + changes + partition + fileFormat + preAlias + alias + indexed + hints + pivots + postAlias + joins + laterals + ordinality
}

func (g *Generator) tableAliasSQL(e expressions.Expression) string {
	alias := g.sqlKey(e, "this")
	columns := g.expressions(exprsOptions{expression: e, key: "columns", flat: true})
	if columns != "" {
		columns = "(" + columns + ")"
	}
	if alias == "" && !g.dialect.UnnestColumnOnly {
		alias = g.nextNameSQL()
	}
	return alias + columns
}

func (g *Generator) tupleSQL(e expressions.Expression) string {
	return "(" + g.expressions(exprsOptions{expression: e, dynamic: true, newLine: true, skipFirst: true, skipLast: true}) + ")"
}

func (g *Generator) blockSQL(e expressions.Expression) string {
	return g.expressions(exprsOptions{expression: e, sep: "; ", flat: true})
}

func (g *Generator) hintSQL(e expressions.Expression) string {
	// QUERY_HINT_SEP: mysql joins hint items with a space (generators/mysql.py:138),
	// base/postgres with ", " (generator.py:368).
	sep := ", "
	if g.dialect.Name == "mysql" {
		sep = " "
	}
	return " /*+ " + strings.TrimSpace(g.expressions(exprsOptions{expression: e, sep: sep})) + " */"
}
