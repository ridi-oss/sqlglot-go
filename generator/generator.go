package generator

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	"github.com/ridi/sqlglot-go/expressions"
)

const sentinelLineBreak = "__SQLGLOT_GO_SENTINEL_LINE_BREAK__"

type Options struct {
	Pretty             bool
	Identify           any
	Normalize          bool
	Pad                int
	Indent             int
	NormalizeFunctions any
	UnsupportedLevel   *sqlerrors.ErrorLevel
	MaxUnsupported     int
	LeadingComma       bool
	MaxTextWidth       int
	Comments           *bool
}

type Generator struct {
	pretty              bool
	identify            any
	normalize           bool
	pad                 int
	indentSize          int
	normalizeFunctions  any
	unsupportedLevel    sqlerrors.ErrorLevel
	maxUnsupported      int
	leadingComma        bool
	maxTextWidth        int
	comments            bool
	dialect             *dialects.Dialect
	unsupportedMessages []string
	nextName            int

	identifierStart string
	identifierEnd   string
	quoteStart      string
	quoteEnd        string

	escapedQuoteEnd                string
	escapedIdentifierEnd           string
	stringsSupportEscapedSequences bool
	// escapedByteQuoteEnd ports _escaped_byte_quote_end (generator.py:906-910): like
	// escapedQuoteEnd, but built from the dialect's BYTE_END (empty when the dialect has no
	// byte-string family, e.g. base/mysql).
	escapedByteQuoteEnd string
	// byteStringsSupportEscapedSequences ports BYTE_STRINGS_SUPPORT_ESCAPED_SEQUENCES
	// (dialects/dialect.py:298-300, 434): "\\" in the dialect's BYTE_STRING_ESCAPES. Only
	// postgres sets ByteStringEscapes['\\'] among base/mysql/postgres.
	byteStringsSupportEscapedSequences bool

	// singleStringInterval ports SINGLE_STRING_INTERVAL (generator.py:335, postgres.py:233):
	// whether intervalSQL renders as a single quoted "<value> <unit>" string (postgres) rather
	// than the base's separate `INTERVAL <this> <unit>` tokens.
	singleStringInterval bool
	// intervalAllowsPluralForm ports INTERVAL_ALLOWS_PLURAL_FORM (generator.py:335, mysql.py:132):
	// whether intervalSQL keeps a plural unit (e.g. "DAYS") as-is, or singularizes it via
	// timePartSingulars first (mysql doesn't allow the plural form).
	intervalAllowsPluralForm bool
	// parameterToken ports PARAMETER_TOKEN (generator.py:667, postgres.py:240): the sigil
	// parameterSQL prefixes a Parameter's name with ("@" base/mysql, "$" postgres).
	parameterToken string
}

func New(d *dialects.Dialect, o Options) *Generator {
	if d == nil {
		d = dialects.Base()
	}
	pad := o.Pad
	if pad == 0 {
		pad = 2
	}
	indent := o.Indent
	if indent == 0 {
		indent = 2
	}
	unsupportedLevel := sqlerrors.WARN
	if o.UnsupportedLevel != nil {
		unsupportedLevel = *o.UnsupportedLevel
	}
	maxUnsupported := o.MaxUnsupported
	if maxUnsupported == 0 {
		maxUnsupported = 3
	}
	maxTextWidth := o.MaxTextWidth
	if maxTextWidth == 0 {
		maxTextWidth = 80
	}
	comments := true
	if o.Comments != nil {
		comments = *o.Comments
	}
	normalizeFunctions := any(d.NormalizeFunctions)
	if o.NormalizeFunctions != nil {
		normalizeFunctions = o.NormalizeFunctions
	}

	identifierStart, identifierEnd := identifierDelimiters(d)
	quoteStart, quoteEnd := quoteDelimiters(d)
	stringEscape := "'"
	if !d.TokenizerConfig.StringEscapes['\''] {
		for r := range d.TokenizerConfig.StringEscapes {
			stringEscape = string(r)
			break
		}
	}

	// _escaped_byte_quote_end (generator.py:906-910): empty when the dialect has no
	// byte-string family (BYTE_END unset), else STRING_ESCAPES[0] + BYTE_END.
	escapedByteQuoteEnd := ""
	if d.ByteEnd != "" {
		escapedByteQuoteEnd = stringEscape + d.ByteEnd
	}

	return &Generator{
		pretty:                             o.Pretty,
		identify:                           o.Identify,
		normalize:                          o.Normalize,
		pad:                                pad,
		indentSize:                         indent,
		normalizeFunctions:                 normalizeFunctions,
		unsupportedLevel:                   unsupportedLevel,
		maxUnsupported:                     maxUnsupported,
		leadingComma:                       o.LeadingComma,
		maxTextWidth:                       maxTextWidth,
		comments:                           comments,
		dialect:                            d,
		identifierStart:                    identifierStart,
		identifierEnd:                      identifierEnd,
		quoteStart:                         quoteStart,
		quoteEnd:                           quoteEnd,
		escapedQuoteEnd:                    stringEscape + quoteEnd,
		escapedIdentifierEnd:               identifierEnd + identifierEnd,
		stringsSupportEscapedSequences:     d.TokenizerConfig.StringEscapes['\\'],
		escapedByteQuoteEnd:                escapedByteQuoteEnd,
		byteStringsSupportEscapedSequences: d.TokenizerConfig.ByteStringEscapes['\\'],
		singleStringInterval:               d.SingleStringInterval,
		intervalAllowsPluralForm:           d.IntervalAllowsPluralForm,
		parameterToken:                     d.ParameterToken,
	}
}

func identifierDelimiters(d *dialects.Dialect) (string, string) {
	if d.IdentifierStart != "" {
		return d.IdentifierStart, d.IdentifierEnd
	}
	return "\"", "\""
}

func quoteDelimiters(d *dialects.Dialect) (string, string) {
	if d.QuoteStart != "" {
		return d.QuoteStart, d.QuoteEnd
	}
	return "'", "'"
}

func (g *Generator) Generate(e expressions.Expression) (sql string, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch v := r.(type) {
			case *sqlerrors.UnsupportedError:
				err = v
			case error:
				err = v
			default:
				err = fmt.Errorf("%v", v)
			}
		}
	}()
	if e == nil {
		return "", nil
	}
	e = e.Copy()
	e = g.preprocess(e)
	g.unsupportedMessages = nil
	sql = strings.TrimSpace(g.gen(e))
	if g.pretty {
		sql = strings.ReplaceAll(sql, sentinelLineBreak, "\n")
	}
	if g.unsupportedLevel == sqlerrors.IGNORE {
		return sql, nil
	}
	if g.unsupportedLevel == sqlerrors.WARN {
		for _, msg := range g.unsupportedMessages {
			log.Printf("%s", msg)
		}
	} else if g.unsupportedLevel == sqlerrors.RAISE && len(g.unsupportedMessages) > 0 {
		errs := make([]error, len(g.unsupportedMessages))
		for i, msg := range g.unsupportedMessages {
			errs[i] = fmt.Errorf("%s", msg)
		}
		return "", &sqlerrors.UnsupportedError{Msg: sqlerrors.ConcatMessages(errs, g.maxUnsupported)}
	}
	return sql, nil
}

func (g *Generator) preprocess(e expressions.Expression) expressions.Expression {
	// Base EXPRESSIONS_WITHOUT_NESTED_CTES is empty and ENSURE_BOOLS is false, so preprocessing is a no-op for slice 2.
	return e
}

func (g *Generator) unsupported(message string) {
	if g.unsupportedLevel == sqlerrors.IMMEDIATE {
		panic(&sqlerrors.UnsupportedError{Msg: message})
	}
	g.unsupportedMessages = append(g.unsupportedMessages, message)
}

func (g *Generator) sep(sep ...string) string {
	s := " "
	if len(sep) > 0 {
		s = sep[0]
	}
	if g.pretty {
		return strings.TrimSpace(s) + "\n"
	}
	return s
}

func (g *Generator) seg(sql string, sep ...string) string {
	s := " "
	if len(sep) > 0 {
		s = sep[0]
	}
	return g.sep(s) + sql
}

func (g *Generator) indent(sql string, level int, pad *int, skipFirst bool, skipLast bool) string {
	if !g.pretty || sql == "" {
		return sql
	}
	padValue := g.pad
	if pad != nil {
		padValue = *pad
	}
	lines := strings.Split(sql, "\n")
	for i, line := range lines {
		if (skipFirst && i == 0) || (skipLast && i == len(lines)-1) {
			continue
		}
		lines[i] = strings.Repeat(" ", level*g.indentSize+padValue) + line
	}
	return strings.Join(lines, "\n")
}

func (g *Generator) wrap(v any) string {
	var thisSQL string
	if e, ok := v.(expressions.Expression); ok && !isNilExpression(e) && isUnwrappedQuery(e.Kind()) {
		thisSQL = g.gen(e)
	} else if e, ok := v.(expressions.Expression); ok && !isNilExpression(e) {
		thisSQL = g.sqlKey(e, "this")
	} else {
		thisSQL = g.gen(v)
	}
	if thisSQL == "" {
		return "()"
	}
	zero := 0
	thisSQL = g.indent(thisSQL, 1, &zero, false, false)
	return "(" + g.sep("") + thisSQL + g.seg(")", "")
}

func isUnwrappedQuery(k expressions.Kind) bool {
	return k == expressions.KindSelect || k == expressions.KindUnion || k == expressions.KindExcept || k == expressions.KindIntersect
}

func (g *Generator) gen(v any) string { return g.genWithComment(v, true) }

func (g *Generator) genNoComment(v any) string { return g.genWithComment(v, false) }

func (g *Generator) genWithComment(v any, comment bool) string {
	if v == nil {
		return ""
	}
	switch tv := v.(type) {
	case string:
		return tv
	case expressions.Expression:
		if isNilExpression(tv) {
			return ""
		}
		h := dispatch[tv.Kind()]
		var sql string
		if h != nil {
			sql = h(g, tv)
		} else if tv.Is(expressions.TraitFunc) {
			sql = g.functionFallbackSQL(tv)
		} else {
			panic(&sqlerrors.UnsupportedError{Msg: fmt.Sprintf("Unsupported expression type %s", expressions.ClassName(tv.Kind()))})
		}
		if g.comments && comment {
			return g.maybeComment(sql, tv, nil, false)
		}
		return sql
	default:
		return fmt.Sprint(tv)
	}
}

func isNilExpression(e expressions.Expression) bool {
	if e == nil {
		return true
	}
	v := reflect.ValueOf(e)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func (g *Generator) sqlKey(e expressions.Expression, key string) string {
	if e == nil {
		return ""
	}
	v := e.Arg(key)
	if !truthy(v) {
		return ""
	}
	return g.gen(v)
}

func (g *Generator) maybeComment(sql string, e expressions.Expression, comments []string, separated bool) string {
	if !g.comments {
		return sql
	}
	if comments == nil && e != nil {
		comments = e.Comments()
	}
	if len(comments) == 0 || (e != nil && excludeComments(e)) {
		return sql
	}
	commentsList := make([]string, 0, len(comments))
	for _, comment := range comments {
		if comment == "" {
			continue
		}
		commentsList = append(commentsList, "/*"+g.replaceLineBreaks(g.sanitizeComment(comment))+"*/")
	}
	if len(commentsList) == 0 {
		return sql
	}
	if separated || (e != nil && withSeparatedComments(e.Kind())) {
		commentsSQL := strings.Join(commentsList, g.sep())
		if sql == "" || strings.TrimSpace(sql[:1]) == "" {
			return g.sep() + commentsSQL + sql
		}
		return commentsSQL + g.sep() + sql
	}
	return sql + " " + strings.Join(commentsList, " ")
}

func (g *Generator) sanitizeComment(comment string) string {
	if comment == "" {
		return comment
	}
	if strings.TrimSpace(comment[:1]) != "" {
		comment = " " + comment
	}
	if strings.TrimSpace(comment[len(comment)-1:]) != "" {
		comment += " "
	}
	comment = strings.ReplaceAll(comment, "*/", "* /")
	comment = strings.ReplaceAll(comment, "/*", "/ *")
	return comment
}

func (g *Generator) replaceLineBreaks(s string) string {
	if g.pretty {
		return strings.ReplaceAll(s, "\n", sentinelLineBreak)
	}
	return s
}

func excludeComments(e expressions.Expression) bool {
	if e.Is(expressions.TraitBinary) {
		return true
	}
	switch e.Kind() {
	case expressions.KindUnion, expressions.KindExcept, expressions.KindIntersect:
		return true
	}
	return false
}

func withSeparatedComments(k expressions.Kind) bool {
	switch k {
	case expressions.KindCommand, expressions.KindCreate, expressions.KindDescribe, expressions.KindDelete,
		expressions.KindDrop, expressions.KindFrom, expressions.KindJoin, expressions.KindOrder, expressions.KindGroup,
		expressions.KindHaving, expressions.KindSelect, expressions.KindUnion, expressions.KindExcept,
		expressions.KindIntersect, expressions.KindUpdate, expressions.KindWhere, expressions.KindWith,
		expressions.KindInsert:
		return true
	}
	return false
}

type exprsOptions struct {
	expression expressions.Expression
	key        string
	sqls       []any
	flat       bool
	noIndent   bool
	skipFirst  bool
	skipLast   bool
	// sep is the separator between rendered items. Because Go's zero value ("")
	// cannot distinguish "unset" from "explicitly empty", the zero value means the
	// upstream default ", "; set emptySep to request a genuine empty separator
	// (mirrors Python callers that pass sep="").
	sep      string
	emptySep bool
	prefix   string
	dynamic  bool
	newLine  bool
}

func (g *Generator) expressions(opts exprsOptions) string {
	sep := opts.sep
	if sep == "" && !opts.emptySep {
		sep = ", "
	}
	indent := !opts.noIndent
	items := opts.sqls
	if opts.expression != nil {
		key := opts.key
		if key == "" {
			key = "expressions"
		}
		items = listFromValue(opts.expression.Arg(key))
	}
	if len(items) == 0 {
		return ""
	}
	if opts.flat {
		sqls := make([]string, 0, len(items))
		for _, item := range items {
			if sql := g.gen(item); sql != "" {
				sqls = append(sqls, sql)
			}
		}
		return strings.Join(sqls, sep)
	}
	numSQLs := len(items)
	resultSQLs := make([]string, 0, numSQLs)
	for i, item := range items {
		sql := g.genNoComment(item)
		if sql == "" {
			continue
		}
		comments := ""
		if expr, ok := item.(expressions.Expression); ok && !isNilExpression(expr) {
			comments = g.maybeComment("", expr, nil, false)
		}
		if g.pretty {
			if g.leadingComma {
				prefix := opts.prefix
				if i > 0 {
					prefix = sep + prefix
				}
				resultSQLs = append(resultSQLs, prefix+sql+comments)
			} else {
				tail := ""
				if i+1 < numSQLs {
					if comments != "" {
						tail = strings.TrimRight(sep, " \t\n\r")
					} else {
						tail = sep
					}
				}
				resultSQLs = append(resultSQLs, opts.prefix+sql+tail+comments)
			}
		} else {
			tail := ""
			if i+1 < numSQLs {
				tail = sep
			}
			resultSQLs = append(resultSQLs, opts.prefix+sql+comments+tail)
		}
	}
	var resultSQL string
	if g.pretty && (!opts.dynamic || g.tooWide(resultSQLs)) {
		if opts.newLine {
			resultSQLs = append([]string{""}, resultSQLs...)
			resultSQLs = append(resultSQLs, "")
		}
		for i, sql := range resultSQLs {
			resultSQLs[i] = strings.TrimRight(sql, " \t\n\r")
		}
		resultSQL = strings.Join(resultSQLs, "\n")
	} else {
		resultSQL = strings.Join(resultSQLs, "")
	}
	if indent {
		return g.indent(resultSQL, 0, nil, opts.skipFirst, opts.skipLast)
	}
	return resultSQL
}

func listFromValue(value any) []any {
	switch v := value.(type) {
	case nil:
		return nil
	case []any:
		return v
	case []string:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out
	case []expressions.Expression:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		return out
	case expressions.Expression:
		if isNilExpression(v) {
			return nil
		}
		return []any{v}
	default:
		return []any{v}
	}
}

func (g *Generator) opExpressions(op string, e expressions.Expression, flat ...bool) string {
	isFlat := len(flat) > 0 && flat[0]
	expressionsSQL := g.expressions(exprsOptions{expression: e, flat: isFlat})
	if isFlat {
		return op + " " + expressionsSQL
	}
	sep := ""
	if expressionsSQL != "" {
		sep = g.sep()
	}
	return g.seg(op) + sep + expressionsSQL
}

func (g *Generator) funcCall(name string, args []any, prefix string, suffix string, normalize bool) string {
	if prefix == "" {
		prefix = "("
	}
	if suffix == "" {
		suffix = ")"
	}
	if normalize {
		name = g.normalizeFunc(name)
	}
	return name + prefix + g.formatArgs(args, ", ") + suffix
}

func (g *Generator) formatArgs(args []any, sep string) string {
	if sep == "" {
		sep = ", "
	}
	argSQLs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == nil {
			continue
		}
		if _, ok := arg.(bool); ok {
			continue
		}
		argSQLs = append(argSQLs, g.gen(arg))
	}
	if g.pretty && g.tooWide(argSQLs) {
		return g.indent("\n"+strings.Join(argSQLs, strings.TrimSpace(sep)+"\n")+"\n", 0, nil, true, true)
	}
	return strings.Join(argSQLs, sep)
}

func (g *Generator) tooWide(args []string) bool {
	total := 0
	for _, arg := range args {
		total += len(arg)
	}
	return total > g.maxTextWidth
}

func (g *Generator) normalizeFunc(name string) string {
	switch v := g.normalizeFunctions.(type) {
	case string:
		if v == "upper" {
			return strings.ToUpper(name)
		}
		if v == "lower" {
			return strings.ToLower(name)
		}
	case bool:
		if v {
			return strings.ToUpper(name)
		}
	}
	return name
}

func (g *Generator) functionFallbackSQL(e expressions.Expression) string {
	return g.funcCall(g.sqlName(e.Kind()), g.fallbackArgs(e), "(", ")", true)
}

// fallbackArgs gathers an expression's argument values in ArgKeys (declared) order,
// flattening slice-valued args, so a function-style node can be rendered with the same
// argument list functionFallbackSQL produces. Shared by the dialect rename handlers
// (generator/rename.go varianceSQL/variancePopSQL, generator/aggregate.go logicalOrSQL) that
// re-emit a Func under a different name, mirroring upstream rename_func's
// flatten(expression.args.values()).
func (g *Generator) fallbackArgs(e expressions.Expression) []any {
	args := []any{}
	for _, key := range expressions.ArgKeys(e.Kind()) {
		argValue := e.Arg(key)
		switch v := argValue.(type) {
		case []expressions.Expression:
			for _, value := range v {
				args = append(args, value)
			}
		case []any:
			args = append(args, v...)
		default:
			if argValue != nil {
				args = append(args, argValue)
			}
		}
	}
	return args
}

func (g *Generator) nextNameSQL() string {
	name := fmt.Sprintf("_t%d", g.nextName)
	g.nextName++
	return name
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case []expressions.Expression:
		return len(v) > 0
	case []any:
		return len(v) > 0
	}
	return true
}

func boolValue(value any) bool {
	b, _ := value.(bool)
	return b
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func joinNonEmpty(sep string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, sep)
}

func asExpression(value any) expressions.Expression {
	if expr, ok := value.(expressions.Expression); ok && !isNilExpression(expr) {
		return expr
	}
	return nil
}
