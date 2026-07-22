package parser

import (
	"fmt"

	"github.com/ridi-oss/sqlglot-go/dialects"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

type Parser struct {
	errorLevel          sqlerrors.ErrorLevel
	errorMessageContext int
	maxErrors           int
	maxNodes            int
	dialect             *dialects.Dialect
	parserOverrideName  string
	strictCast          bool
	sql                 string
	errors              []*sqlerrors.ParseError

	tokens     []tokens.Token
	tokensSize int
	index      int
	curr       tokens.Token
	next       tokens.Token
	prev       tokens.Token

	prevComments []string
	chunks       [][]tokens.Token
	chunkIndex   int
	nodeCount    int
}

func New(d *dialects.Dialect) *Parser {
	return NewWithErrorLevel(d, sqlerrors.IMMEDIATE)
}

func NewWithErrorLevel(d *dialects.Dialect, level sqlerrors.ErrorLevel) *Parser {
	return newWithErrorLevelAndOverrideName(d, level, "")
}

func newWithErrorLevelAndOverrideName(d *dialects.Dialect, level sqlerrors.ErrorLevel, overrideName string) *Parser {
	if d == nil {
		d = dialects.Base()
	}
	if overrideName == "" {
		overrideName = d.Name
	}
	p := &Parser{
		errorLevel:          level,
		errorMessageContext: 100,
		maxErrors:           3,
		maxNodes:            -1,
		dialect:             d,
		parserOverrideName:  overrideName,
		strictCast:          d.StrictCast,
		curr:                tokens.SentinelNone,
		next:                tokens.SentinelNone,
		prev:                tokens.SentinelNone,
	}
	return p
}

func (p *Parser) Reset() {
	p.sql = ""
	p.errors = nil
	p.tokens = nil
	p.tokensSize = 0
	p.index = 0
	p.curr = tokens.SentinelNone
	p.next = tokens.SentinelNone
	p.prev = tokens.SentinelNone
	p.prevComments = nil
	p.chunks = nil
	p.chunkIndex = 0
	p.nodeCount = 0
}

func (p *Parser) Errors() []*sqlerrors.ParseError { return p.errors }

func (p *Parser) advance(times ...int) {
	t := 1
	if len(times) > 0 {
		t = times[0]
	}
	index := p.index + t
	p.index = index
	if index >= 0 && index < p.tokensSize {
		p.curr = p.tokens[index]
	} else {
		p.curr = tokens.SentinelNone
	}
	if index+1 >= 0 && index+1 < p.tokensSize {
		p.next = p.tokens[index+1]
	} else {
		p.next = tokens.SentinelNone
	}
	if index > 0 && index-1 < p.tokensSize {
		prev := p.tokens[index-1]
		p.prev = prev
		p.prevComments = prev.Comments
	} else {
		p.prev = tokens.SentinelNone
		p.prevComments = nil
	}
}

func (p *Parser) advanceChunk() {
	p.index = -1
	p.tokens = p.chunks[p.chunkIndex]
	p.tokensSize = len(p.tokens)
	p.chunkIndex++
	p.advance()
}

func (p *Parser) retreat(index int) {
	if index != p.index {
		p.advance(index - p.index)
	}
}

func (p *Parser) addComments(expression exp.Expression) {
	if expression != nil && len(p.prevComments) > 0 {
		expression.AddComments(p.prevComments, false)
		p.prevComments = nil
	}
}

func (p *Parser) match(tokenType tokens.TokenType, advance ...bool) bool {
	shouldAdvance := true
	if len(advance) > 0 {
		shouldAdvance = advance[0]
	}
	if p.curr.TokenType == tokenType {
		if shouldAdvance {
			p.advance()
		}
		return true
	}
	return false
}

func (p *Parser) matchSet(types map[tokens.TokenType]bool, advance ...bool) bool {
	shouldAdvance := true
	if len(advance) > 0 {
		shouldAdvance = advance[0]
	}
	if types[p.curr.TokenType] {
		if shouldAdvance {
			p.advance()
		}
		return true
	}
	return false
}

func (p *Parser) matchPair(a, b tokens.TokenType, advance bool) bool {
	if p.curr.TokenType == a && p.next.TokenType == b {
		if advance {
			p.advance(2)
		}
		return true
	}
	return false
}

func (p *Parser) matchTexts(texts map[string]bool, advance ...bool) bool {
	shouldAdvance := true
	if len(advance) > 0 {
		shouldAdvance = advance[0]
	}
	if p.curr.TokenType != tokens.STRING && texts[stringsUpper(p.curr.Text)] {
		if shouldAdvance {
			p.advance()
		}
		return true
	}
	return false
}

func (p *Parser) matchTextSeq(texts ...string) bool {
	index := p.index
	for _, text := range texts {
		if p.curr.TokenType != tokens.STRING && stringsUpper(p.curr.Text) == text {
			p.advance()
		} else {
			p.retreat(index)
			return false
		}
	}
	return true
}

func (p *Parser) isConnected() bool {
	return p.prev.IsValid() && p.curr.IsValid() && p.prev.End+1 == p.curr.Start
}

func (p *Parser) findSQL(start, end tokens.Token) string {
	runes := []rune(p.sql)
	if start.Start < 0 || end.End >= len(runes) || start.Start > end.End {
		return ""
	}
	return string(runes[start.Start : end.End+1])
}

func (p *Parser) raiseError(message string, tok ...tokens.Token) {
	token := tokens.SentinelNone
	if len(tok) > 0 {
		token = tok[0]
	}
	if !token.IsValid() {
		if p.curr.IsValid() {
			token = p.curr
		} else if p.prev.IsValid() {
			token = p.prev
		} else {
			token = tokens.StringToken("")
		}
	}
	formattedSQL, startContext, highlight, endContext := sqlerrors.HighlightSQL(
		p.sql,
		[][2]int{{token.Start, token.End}},
		p.errorMessageContext,
	)
	formattedMessage := fmt.Sprintf("%s. Line %d, Col: %d.\n  %s", message, token.Line, token.Col, formattedSQL)
	err := sqlerrors.NewParseErrorWithLocation(formattedMessage, message, token.Line, token.Col, startContext, highlight, endContext)
	if p.errorLevel == sqlerrors.IMMEDIATE {
		panic(err)
	}
	p.errors = append(p.errors, err)
}

// exprArgs adapts a parsed function-argument slice to the []any that
// validateExpression/ErrorMessages expects for the upstream over-arity check
// (parser.py passes the raw args list to validate_expression).
func exprArgs(args []exp.Expression) []any {
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = a
	}
	return out
}

func (p *Parser) validateExpression(expression exp.Expression, args []any) exp.Expression {
	if p.maxNodes > -1 {
		p.nodeCount++
		if p.nodeCount > p.maxNodes {
			p.raiseError(fmt.Sprintf("Maximum number of AST nodes (%d) exceeded", p.maxNodes))
		}
	}
	if p.errorLevel != sqlerrors.IGNORE {
		for _, message := range expression.ErrorMessages(args) {
			p.raiseError(message)
		}
	}
	return expression
}

func (p *Parser) tryParse(parseMethod func() exp.Expression, retreat bool) (out exp.Expression) {
	index := p.index
	errorLevel := p.errorLevel
	p.errorLevel = sqlerrors.IMMEDIATE
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*sqlerrors.ParseError); ok {
				out = nil
			} else {
				panic(r)
			}
		}
		if out == nil || retreat {
			p.retreat(index)
		}
		p.errorLevel = errorLevel
	}()
	out = parseMethod()
	return out
}

func (p *Parser) Parse(rawTokens []tokens.Token, sql string) (expressions []exp.Expression, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case *sqlerrors.ParseError:
				err = e
			case error:
				err = e
			default:
				panic(r)
			}
		}
	}()
	if p.isAthenaRouter() {
		return p.parseAthena(rawTokens, sql)
	}
	return p.parse(p.parseStatement, rawTokens, sql), nil
}

func (p *Parser) ParseInto(rawTokens []tokens.Token, sql string, into exp.Kind) (out []exp.Expression, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case *sqlerrors.ParseError:
				err = e
			case error:
				err = e
			default:
				panic(r)
			}
		}
	}()
	if p.isAthenaRouter() {
		return p.parseIntoAthena(rawTokens, sql, into)
	}
	var method func() exp.Expression
	switch into {
	case exp.KindDataType:
		method = func() exp.Expression { return p.parseTypes(false, true, false, false) }
	case exp.KindTable:
		method = func() exp.Expression { return p.parseTableParts(false, false, false, false) }
	case exp.KindIdentifier:
		method = func() exp.Expression { return p.parseIdVar(true, nil) }
	case exp.KindHint:
		method = func() exp.Expression { return p.parseHintBody() }
	default:
		return nil, fmt.Errorf("no parser registered for kind %v", into)
	}
	return p.parse(method, rawTokens, sql), nil
}

func (p *Parser) checkErrors() {
	if p.errorLevel == sqlerrors.RAISE && len(p.errors) > 0 {
		errs := make([]error, len(p.errors))
		for i, err := range p.errors {
			errs[i] = err
		}
		panic(&sqlerrors.ParseError{Msg: sqlerrors.ConcatMessages(errs, p.maxErrors), Errors: sqlerrors.MergeErrors(p.errors)})
	}
}

func (p *Parser) Expression(instance exp.Expression) exp.Expression {
	return p.expression(instance, nil, nil)
}

func (p *Parser) expression(instance exp.Expression, token *tokens.Token, comments []string) exp.Expression {
	if instance == nil {
		return nil
	}
	if len(comments) > 0 {
		instance.AddComments(comments, false)
	} else {
		p.addComments(instance)
	}
	if !instance.IsPrimitive() {
		instance = p.validateExpression(instance, nil)
	}
	return instance
}

func (p *Parser) parseBatchStatements(parseMethod func() exp.Expression, sepFirstStatement bool) []exp.Expression {
	expressions := []exp.Expression{}
	if sepFirstStatement {
		p.match(tokens.BEGIN)
		expressions = append(expressions, parseMethod())
	}
	chunksLength := len(p.chunks)
	for p.chunkIndex < chunksLength {
		p.advanceChunk()
		stmt := parseMethod()
		expressions = append(expressions, stmt)
		// MySQL file-write INTO placement is validated per top-level statement so it also covers
		// non-SELECT roots (UPDATE/DELETE/INSERT/CREATE) and EXPLAIN, not just parseExpressionStatement.
		p.validateFileWriteIntoPlacement(stmt)
		if p.index < p.tokensSize {
			p.raiseError("Invalid expression / Unexpected token")
		}
		p.checkErrors()
	}
	return expressions
}

func (p *Parser) parse(parseMethod func() exp.Expression, rawTokens []tokens.Token, sql string) []exp.Expression {
	p.Reset()
	p.sql = sql
	total := len(rawTokens)
	chunks := [][]tokens.Token{{}}
	for i, token := range rawTokens {
		if token.TokenType == tokens.SEMICOLON {
			if len(token.Comments) > 0 {
				chunks = append(chunks, []tokens.Token{token})
			}
			if i < total-1 {
				chunks = append(chunks, []tokens.Token{})
			}
		} else {
			chunks[len(chunks)-1] = append(chunks[len(chunks)-1], token)
		}
	}
	p.chunks = chunks
	return p.parseBatchStatements(parseMethod, false)
}

func (p *Parser) parseStatement() exp.Expression {
	if !p.curr.IsValid() {
		return nil
	}
	if parse := p.statementParser(p.curr.TokenType); parse != nil {
		p.advance()
		comments := p.prevComments
		stmt := parse(p)
		if stmt != nil {
			stmt.AddComments(comments, true)
		}
		return stmt
	}
	// Ports parser.py:2293-2294: any leading token in the dialect's tokenizer Commands
	// set (CALL/EXPLAIN/OPTIMIZE/PREPARE/VACUUM/... - see parseCommand's doc comment)
	// degrades the whole statement to a raw-text exp.Command via the tokenizer's
	// already-packed trailing STRING token.
	if p.matchSet(p.dialect.TokenizerConfig.Commands) {
		return p.parseCommand()
	}
	return p.parseExpressionStatement()
}

// parseExpressionStatement is the ordinary (non-statement-parser, non-Command) tail of
// parseStatement: a top-level expression or SELECT with query modifiers. Extracted so a
// dialect-gated statement parser can fall back to it (see parseEndTransaction).
func (p *Parser) parseExpressionStatement() exp.Expression {
	expression := p.parseExpression()
	if expression != nil {
		expression = p.parseSetOperations(expression)
	} else {
		expression = p.parseSelect(false, false, true, true)
	}
	return p.parseQueryModifiers(expression)
}

// validateFileWriteIntoPlacement enforces MySQL's rule that a file-write INTO (OUTFILE/DUMPFILE)
// may appear only at the end of a top-level query expression — never inside a subquery/derived
// table, on a non-final UNION branch, or nested in an UPDATE/DELETE/INSERT/CREATE (MySQL error
// 3954/1064). parseInto attaches the Into wherever the keyword appears and parseSetOperations
// hoists a *final* set-op branch's write to the set-op node, so the only valid position is the
// top-level query root's own `into` arg: the statement itself when it is a SELECT/UNION, or — for a
// leading EXPLAIN/DESCRIBE — the query it explains. A file-write Into anywhere else is misplaced and
// fails closed. Detection is never lost either way; this keeps the accepted grammar aligned with
// MySQL. Called once per top-level statement (parseBatchStatements); MySQL-only.
func (p *Parser) validateFileWriteIntoPlacement(stmt exp.Expression) {
	if p.dialect.Name != "mysql" || stmt == nil {
		return
	}
	root := stmt
	if root.Kind() == exp.KindDescribe { // EXPLAIN/DESCRIBE <query>: the explained query is the root
		root = asExpr(root.Arg("this"))
	}
	var rootInto exp.Expression
	if root != nil && (root.Kind() == exp.KindSelect || isSetOperation(root.Kind())) {
		rootInto = asExpr(root.Arg("into"))
	}
	for _, node := range stmt.FindAll(exp.KindInto) {
		if isOutfileKeyword(node.Text("kind")) && node != rootInto {
			p.raiseError("Misplaced INTO OUTFILE/DUMPFILE: it must be at the end of the top-level query")
			return
		}
	}
}

func (p *Parser) parseSetOperations(this exp.Expression) exp.Expression {
	for this != nil {
		setop := p.parseSetOperation(this, false)
		if setop == nil {
			break
		}
		this = setop
	}
	if this != nil && isSetOperation(this.Kind()) {
		expression := this.Expr()
		if expression != nil {
			for _, arg := range []string{"order", "limit", "offset"} {
				if e := asExpr(expression.Arg(arg)); e != nil {
					this.Set(arg, e.Pop())
				}
			}
			// A MySQL file-write INTO on the last branch applies to the whole query expression
			// and must render at the trailing position (after ORDER BY/LIMIT), so hoist it to the
			// set-operation node like the modifiers above. Only the file-write kinds are hoisted; a
			// plain INTO (kind unset) is left on the branch, matching prior behavior.
			if into := asExpr(expression.Arg("into")); into != nil && isOutfileKeyword(into.Text("kind")) {
				this.Set("into", into.Pop())
			}
		}
	}
	return this
}

func (p *Parser) parseSetOperation(this exp.Expression, consumePipe bool) exp.Expression {
	start := p.index
	_, sideToken, kindToken := p.parseJoinParts()
	var side, kind any
	if sideToken != nil {
		side = sideToken.Text
	}
	if kindToken != nil {
		kind = kindToken.Text
	}
	if !p.matchSet(setOperationTokens) {
		p.retreat(start)
		return nil
	}
	tokenType := p.prev.TokenType
	constructor := exp.Union
	if tokenType == tokens.EXCEPT {
		constructor = exp.Except
	} else if tokenType == tokens.INTERSECT {
		constructor = exp.Intersect
	}
	comments := p.prev.Comments
	distinct := true // TODO(slice 5): dialect SET_OP_DISTINCT_BY_DEFAULT overrides.
	if p.match(tokens.DISTINCT) {
		distinct = true
	} else if p.match(tokens.ALL) {
		distinct = false
	}
	// Upstream stores by_name only when matched (parser.py:5738-5742 uses `... or None`),
	// so leave it nil when absent (like symmetric/siblings) rather than storing False.
	var byName any
	if p.matchTextSeq("BY", "NAME") {
		byName = true
	}
	expression := p.parseSelect(true, false, true, false)
	return p.expression(constructor(exp.Args{
		"this":       this,
		"distinct":   distinct,
		"by_name":    byName,
		"expression": expression,
		"side":       side,
		"kind":       kind,
	}), nil, comments)
}

func (p *Parser) parseSelect(opts ...bool) exp.Expression {
	nested := false
	table := false
	parseSubqueryAlias := true
	parseSetOperation := true
	if len(opts) > 0 {
		nested = opts[0]
	}
	if len(opts) > 1 {
		table = opts[1]
	}
	if len(opts) > 2 {
		parseSubqueryAlias = opts[2]
	}
	if len(opts) > 3 {
		parseSetOperation = opts[3]
	}

	cte := p.parseWith(false)
	if cte != nil {
		this := p.parseStatement()
		if this == nil {
			p.raiseError("Failed to parse any statement following CTE")
			return cte
		}
		for this.Kind() == exp.KindSubquery && exp.IsWrapper(this) {
			this = this.This()
		}
		if exp.SupportsArg(this.Kind(), "with_") {
			if innerCTE := asExpr(this.Arg("with_")); innerCTE != nil {
				cteExpressions := append([]exp.Expression{}, cte.Expressions()...)
				cteExpressions = append(cteExpressions, innerCTE.Expressions()...)
				cte.Set("expressions", cteExpressions)
				if innerCTE.Arg("recursive") != nil {
					cte.Set("recursive", true)
				}
			}
			this.Set("with_", cte)
		} else {
			p.raiseError(fmt.Sprintf("%s does not support CTE", this.Name()))
			this = cte
		}
		return this
	}

	var this exp.Expression
	if p.match(tokens.SELECT) {
		comments := p.prevComments
		hint := p.parseHint()
		var all bool
		matchedDistinct := false
		if p.next.IsValid() && p.next.TokenType != tokens.DOT {
			all = p.match(tokens.ALL)
			matchedDistinct = p.matchSet(distinctTokens)
		}
		var distinct exp.Expression
		if matchedDistinct {
			args := exp.Args{}
			if p.match(tokens.ON) {
				args["on"] = p.parseValue(false)
			}
			distinct = p.expression(exp.Distinct(args), nil, nil)
		}
		if all && distinct != nil {
			p.raiseError("Cannot specify both ALL and DISTINCT after SELECT")
		}
		// operationModifiers ports parser.py:3929-3931's `while self._curr and
		// self._match_texts(self.OPERATION_MODIFIERS)` loop; see mysqlOperationModifiers
		// (parser_hint.go) for why this is gated by dialect name rather than a shared set.
		var operationModifiers []exp.Expression
		if p.dialect.Name == "mysql" {
			for p.curr.IsValid() && p.matchTexts(mysqlOperationModifiers) {
				operationModifiers = append(operationModifiers, exp.Var(exp.Args{"this": stringsUpper(p.prev.Text)}))
			}
		}
		// SELECT TOP is dead for M1 because TOP is not a base tokenizer keyword,
		// but wire the parser shape to match upstream.
		top := p.parseLimit(nil, true, false)
		projections := p.parseExpressions()
		selectArgs := exp.Args{"distinct": distinct, "expressions": projections, "hint": hint}
		if top != nil {
			selectArgs["limit"] = top
		}
		if len(operationModifiers) > 0 {
			selectArgs["operation_modifiers"] = operationModifiers
		}
		this = p.expression(exp.Select(selectArgs), nil, nil)
		if len(comments) > 0 {
			this.AddComments(comments, true)
		}
		if into := p.parseInto(); into != nil {
			this.Set("into", into)
		}
		if from := p.parseFrom(false, false, false); from != nil {
			this.Set("from_", from)
		}
		this = p.parseQueryModifiers(this)
	} else if (table || nested) && p.match(tokens.L_PAREN) {
		comments := p.prevComments
		this = p.parseWrappedSelect(table)
		if this != nil {
			this.AddComments(comments, true)
		}
		p.matchRParen(this)
		return p.parseSubquery(this, parseSubqueryAlias)
	} else if p.match(tokens.VALUES, false) {
		this = p.parseDerivedTableValues()
	} else {
		this = nil
	}
	if parseSetOperation {
		return p.parseSetOperations(this)
	}
	return this
}

func (p *Parser) parseQueryModifiers(this exp.Expression) exp.Expression {
	if this != nil && (this.Is(exp.TraitQuery) || this.Kind() == exp.KindTable) {
		for {
			join := p.parseJoin(false, false, nil)
			if join == nil {
				break
			}
			this.Append("joins", join)
		}
		// Bare LATERAL / LATERAL VIEW (or CROSS/OUTER APPLY) after the joins loop
		// (parser.py:4180-4185). parseLateral peeks and returns nil without consuming
		// unless it actually sees one of those tokens, so this is a no-op otherwise.
		for {
			lateral := p.parseLateral()
			if lateral == nil {
				break
			}
			this.Append("laterals", lateral)
		}
		for queryModifierTokens[p.curr.TokenType] {
			parse := queryModifierParsers[p.curr.TokenType]
			modTok := p.curr
			key, expression := parse(p)
			if expression == nil {
				break
			}
			if this.Arg(key) != nil {
				p.raiseError(fmt.Sprintf("Found multiple '%s' clauses", stringsUpper(modTok.Text)), modTok)
			}
			this.Set(key, expression)
			if key == "limit" {
				limitExpr := asExpr(expression)
				if limitExpr == nil {
					continue
				}
				if off := asExpr(limitExpr.Arg("offset")); off != nil {
					limitExpr.Set("offset", nil)
					offset := exp.Offset(exp.Args{"expression": off})
					this.Set("offset", offset)
					if by := limitExpr.Arg("expressions"); by != nil {
						limitExpr.Set("expressions", nil)
						offset.Set("expressions", by)
					}
				}
			}
		}
		// MySQL allows the file-write INTO at the trailing position too, e.g.
		// `SELECT ... FROM t INTO OUTFILE '/path'` (the canonical spot). Upstream only
		// parses INTO *before* FROM (parser.py:3967), so this grabs just the OUTFILE/
		// DUMPFILE forms here (grammar extension); a plain trailing `INTO tbl/@var` is
		// left alone, matching upstream. Guarded to Select nodes without an existing into.
		if p.dialect.Name == "mysql" && this.Kind() == exp.KindSelect && this.Arg("into") == nil &&
			p.curr.TokenType == tokens.INTO && isOutfileKeywordToken(p.next) {
			if into := p.parseInto(); into != nil {
				this.Set("into", into)
				// A trailing file-write INTO must be the last clause except for a locking clause
				// (FOR UPDATE / LOCK IN SHARE MODE, which MySQL accepts on either side). ORDER BY /
				// LIMIT / WHERE / GROUP BY etc. after it is invalid (1064); fail closed rather than
				// letting a second parseQueryModifiers pass silently reorder them ahead of the INTO.
				if queryModifierTokens[p.curr.TokenType] &&
					p.curr.TokenType != tokens.FOR && p.curr.TokenType != tokens.LOCK {
					p.raiseError("Only a locking clause may follow a trailing INTO OUTFILE/DUMPFILE")
				}
			}
		}
	}
	return this
}

// isOutfileKeyword reports whether text is one of the MySQL INTO file-write keywords.
func isOutfileKeyword(text string) bool {
	switch stringsUpper(text) {
	case "OUTFILE", "DUMPFILE":
		return true
	}
	return false
}

// isOutfileKeywordToken reports whether tok is an *unquoted* OUTFILE/DUMPFILE keyword. A
// backtick-quoted identifier (tokens.IDENTIFIER) with the same text is an ordinary INTO
// target name (e.g. `INTO ` + "`outfile`" + ` = a variable`), which MySQL accepts, so it must
// NOT be treated as the file-write keyword (divergence would regress a legitimate query).
func isOutfileKeywordToken(tok tokens.Token) bool {
	return tok.TokenType != tokens.IDENTIFIER && isOutfileKeyword(tok.Text)
}

func (p *Parser) parseInto() exp.Expression {
	if !p.match(tokens.INTO) {
		return nil
	}
	// MySQL server-side FILE WRITES: `INTO OUTFILE '/path' [CHARACTER SET ..] [export_options]`
	// and `INTO DUMPFILE '/path'`. The `kind` marks it as a write (vs a normal INTO table/var,
	// which leaves kind unset). Grammar extension beyond upstream, which parse-errors these.
	// Only an *unquoted* OUTFILE/DUMPFILE is the keyword; a backtick-quoted `` `outfile` `` is an
	// ordinary INTO target name that MySQL accepts, so it falls through to the plain-INTO path.
	if p.dialect.Name == "mysql" && p.curr.TokenType != tokens.IDENTIFIER {
		if p.matchTextSeq("OUTFILE") {
			return p.parseIntoOutfile("OUTFILE")
		}
		if p.matchTextSeq("DUMPFILE") {
			return p.parseIntoOutfile("DUMPFILE")
		}
	}
	temp := p.match(tokens.TEMPORARY)
	unlogged := p.matchTextSeq("UNLOGGED")
	p.match(tokens.TABLE)
	tbl := p.parseTable(true, false, nil, false, false, false, false)
	return p.expression(exp.Into(exp.Args{"this": tbl, "temporary": temp, "unlogged": unlogged}), nil, nil)
}

// parseIntoOutfile parses the MySQL `INTO OUTFILE '/path' [CHARACTER SET cs] [{FIELDS|COLUMNS} …]
// [LINES …]` / `INTO DUMPFILE '/path'` file-write targets into an Into node with kind =
// "OUTFILE"/"DUMPFILE" and this = the path Literal.
//
// The Into node is ALWAYS returned with `kind` set, even when the path/options are malformed:
// having matched OUTFILE/DUMPFILE we are committed to the file-write grammar, so a malformed tail
// raises a parse error (a hard fail at the IMMEDIATE error level — the default and what the
// sqlglot.ParseOne/Parse wrappers use) yet still yields a structurally detectable file-write, so a
// lenient error level (WARN/IGNORE) can never silently degrade it to a plain SELECT with the INTO
// dropped.
func (p *Parser) parseIntoOutfile(kind string) exp.Expression {
	args := exp.Args{"kind": kind}
	// The path must be a plain string literal: MySQL requires a quoted 'file_name' constant here
	// and rejects a placeholder/parameter/hex/bit value, so parseString's placeholder fallback is
	// deliberately bypassed (it would violate the this = Literal contract).
	if p.curr.TokenType != tokens.STRING {
		p.raiseError("Expected a file path string after INTO " + kind)
		return p.expression(exp.Into(args), nil, nil)
	}
	args["this"] = p.parseString()
	if kind == "OUTFILE" {
		p.parseOutfileExportOptions(args)
	}
	return p.expression(exp.Into(args), nil, nil)
}

// parseOutfileExportOptions parses the optional `CHARACTER SET cs`, `{FIELDS|COLUMNS} …`, and
// `LINES …` export clauses of INTO OUTFILE, matching MySQL: CHARACTER SET precedes the FIELDS/LINES
// blocks; within each block the sub-options may appear in ANY order (last wins on repetition); each
// matched introducer REQUIRES its operand and a bare `FIELDS`/`COLUMNS`/`LINES` with no sub-option
// is rejected — all fail closed via raiseError rather than silently dropping the malformed clause.
func (p *Parser) parseOutfileExportOptions(args exp.Args) {
	// "CHARACTER SET" tokenizes as a single CHARACTER_SET token (tokenizer.go:160). MySQL's
	// charset operand is a bare charset name (utf8mb4) or a quoted string ('utf8'); a number,
	// NULL, a parenthesized/function expression, or a placeholder is rejected (fail closed).
	if p.match(tokens.CHARACTER_SET) {
		var cs exp.Expression
		// A charset name is a quoted string or a bare identifier keyword (utf8mb4, binary). The
		// token type is checked first so parseCharsetName's placeholder/parameter fallback can't
		// admit `?`/`@cs`/`:cs` (MySQL rejects those here with 1064).
		switch p.curr.TokenType {
		case tokens.STRING:
			cs = p.parseString()
		case tokens.VAR, tokens.IDENTIFIER, tokens.BINARY:
			cs = p.parseCharsetName()
		}
		if cs == nil {
			p.raiseError("Expected a charset name after CHARACTER SET")
			return
		}
		args["charset"] = cs
	}
	if p.matchTexts(map[string]bool{"FIELDS": true, "COLUMNS": true}) {
		if stringsUpper(p.prev.Text) == "COLUMNS" {
			args["columns"] = true
		}
		matched := 0
		for {
			if p.matchTextSeq("TERMINATED", "BY") {
				if v := p.parseExportValue(); v != nil {
					args["fields_terminated"] = v
				} else {
					p.raiseError("Expected a string after TERMINATED BY")
					return
				}
				matched++
				continue
			}
			optionally := p.matchTextSeq("OPTIONALLY")
			if p.matchTextSeq("ENCLOSED", "BY") {
				if v := p.parseExportValue(); v != nil {
					args["enclosed"] = v
					args["optionally_enclosed"] = optionally
				} else {
					p.raiseError("Expected a string after ENCLOSED BY")
					return
				}
				matched++
				continue
			}
			if optionally {
				// OPTIONALLY is only valid as an immediate prefix of ENCLOSED BY.
				p.raiseError("Expected ENCLOSED BY after OPTIONALLY")
				return
			}
			if p.matchTextSeq("ESCAPED", "BY") {
				if v := p.parseExportValue(); v != nil {
					args["escaped"] = v
				} else {
					p.raiseError("Expected a string after ESCAPED BY")
					return
				}
				matched++
				continue
			}
			break
		}
		if matched == 0 {
			p.raiseError("Expected TERMINATED BY / ENCLOSED BY / ESCAPED BY after FIELDS")
			return
		}
	}
	if p.matchTextSeq("LINES") {
		matched := 0
		for {
			if p.matchTextSeq("STARTING", "BY") {
				if v := p.parseExportValue(); v != nil {
					args["lines_starting"] = v
				} else {
					p.raiseError("Expected a string after STARTING BY")
					return
				}
				matched++
				continue
			}
			if p.matchTextSeq("TERMINATED", "BY") {
				if v := p.parseExportValue(); v != nil {
					args["lines_terminated"] = v
				} else {
					p.raiseError("Expected a string after TERMINATED BY")
					return
				}
				matched++
				continue
			}
			break
		}
		if matched == 0 {
			p.raiseError("Expected STARTING BY or TERMINATED BY after LINES")
			return
		}
	}
}

// parseExportValue parses the MySQL string constant used as a FIELDS/LINES export-option value:
// a single quoted string, or a hex/bit-string literal (X'2c' / 0x2c, b'101' / 0b101). Anything
// else yields nil (fail closed) — MySQL rejects a number/column/expression here, and also rejects
// the national-string form `N'…'` and adjacent string literals in this slot. A single constant is
// parsed deliberately: parsePrimary would fold adjacent strings into a CONCAT (`'a' 'b'` →
// CONCAT('a', 'b')), so the plain-string form goes through parseString (one literal, no concat),
// leaving any trailing token to fail closed.
func (p *Parser) parseExportValue() exp.Expression {
	switch p.curr.TokenType {
	case tokens.HEX_STRING, tokens.BIT_STRING:
		return p.parsePrimary()
	case tokens.STRING:
		return p.parseString()
	}
	return nil
}

func (p *Parser) parseFrom(joins bool, skipFromToken bool, consumePipe bool) exp.Expression {
	if !skipFromToken && !p.match(tokens.FROM) {
		return nil
	}
	comments := p.prevComments
	return p.expression(exp.From(exp.Args{"this": p.parseTable(false, joins, nil, false, false, false, consumePipe)}), nil, comments)
}

func (p *Parser) parseTable(schema bool, joins bool, aliasTokens map[tokens.TokenType]bool, parseBracket bool, isDBReference bool, parsePartition bool, consumePipe bool) exp.Expression {
	// Divergence from upstream: skip _parse_table's fast path in this slice so
	// subquery detection always runs before table-part parsing.
	if l := p.parseLateral(); l != nil {
		return l
	}
	if u := p.parseUnnest(true); u != nil {
		return u
	}
	if v := p.parseDerivedTableValues(); v != nil {
		return v
	}
	if !schema && !isDBReference {
		if subquery := p.parseSelect(false, true, true, true); subquery != nil {
			if subquery.Arg("pivots") == nil {
				if pivots := p.parsePivots(); pivots != nil {
					subquery.Set("pivots", pivots)
				}
			}
			// A TABLESAMPLE can trail a pivoted subquery. parseSubquery (parser.go:1944)
			// only attaches `sample` BEFORE pivots, so when a PIVOT sits between the
			// subquery and the sample it lands here instead — mirroring upstream
			// _parse_subquery's pivots-then-sample order (parser.py:4141-4143).
			if subquery.Arg("sample") == nil {
				if s := p.parseTableSample(false); s != nil {
					subquery.Set("sample", s)
				}
			}
			if joins {
				for {
					join := p.parseJoin(false, false, aliasTokens)
					if join == nil {
						break
					}
					subquery.Append("joins", join)
				}
			}
			return subquery
		}
	}
	// Postgres multi-function ROW FROM source (parser.py:4867-4874), e.g. `FROM ROWS FROM
	// (FUNC1(col1), FUNC2(col2)) WITH ORDINALITY`: a Table whose "rows_from" arg holds the
	// nested per-function tables (each with its own optional alias, parsed the same way any
	// table-valued function's alias is). this is populated here so the ONLY/STAR/PARTITION/
	// alias/hints/pivots/joins/ORDINALITY handling below applies uniformly.
	var this exp.Expression
	if p.matchTextSeq("ROWS", "FROM") {
		rowsFromTables := p.parseWrappedCsv(func() exp.Expression {
			return p.parseTable(false, false, nil, false, false, false, false)
		})
		if len(rowsFromTables) > 0 {
			this = p.expression(exp.Table(exp.Args{"rows_from": rowsFromTables}), nil, nil)
		}
	}
	// Postgres FROM ONLY <table> (parser.py:4876-4888): only relevant when the ONLY
	// token isn't schema/db-reference position; excludes the table's descendant
	// partitions from the scan. Set on the resulting Table below.
	only := p.match(tokens.ONLY)
	if this == nil {
		this = p.parseTableParts(schema, isDBReference, false, false)
	}
	if this == nil {
		return nil
	}
	if only {
		this.Set("only", only)
	}
	// Postgres supports a wildcard (table) suffix operator, e.g. `ONLY t1, t2*` in
	// TRUNCATE / a FROM list - a no-op in this context, so consume and discard it
	// (parser.py:4890-4891).
	p.match(tokens.STAR)
	// Hive/Spark PARTITION(...) table-partition selector (parser.py:4893-4895), e.g.
	// `INSERT INTO a.b PARTITION(ds = '...') ...` / `INSERT OVERWRITE TABLE a.b
	// PARTITION(ds) ...`. parseInsertTable passes parsePartition=true; mysql additionally
	// enables it for every FROM source via SUPPORTS_PARTITION_SELECTION (parser.py:4893
	// `parse_partition = parse_partition or self.SUPPORTS_PARTITION_SELECTION`), e.g.
	// `SELECT * FROM t1 PARTITION(p0)`.
	if (parsePartition || p.dialect.SupportsPartitionSelection) && p.match(tokens.PARTITION, false) {
		this.Set("partition", p.parsePartition())
	}
	if schema {
		return p.parseSchema(this)
	}
	// A join-starting keyword (CROSS/INNER/OUTER/STRAIGHT_JOIN/ASOF/NATURAL/POSITIONAL/...)
	// must never be swallowed as a bare (non-AS) alias, e.g. `a STRAIGHT_JOIN b` or
	// `a CROSS JOIN b`: upstream avoids this via _parse_table's fast path, whose
	// TABLE_TERMINATORS check (parser.py:968-986, which spreads *JOIN_KINDS/METHODS/SIDES)
	// short-circuits before alias parsing is ever attempted (parser.py:4824-4842). This
	// port's parseTable intentionally skips that fast path (see the divergence note atop
	// this function), so the equivalent protection is applied here instead: an explicit
	// `AS` still parses normally (parseTableAlias's own any_token path already accepts
	// these words as identifiers once AS is present, matching upstream).
	//
	// MySQL additionally removes TABLE_INDEX_HINT_TOKENS (FORCE/IGNORE/USE) from its
	// TABLE_ALIAS_TOKENS (parsers/mysql.py:83-85), so e.g. `t1 USE INDEX (i1)` parses
	// USE as the start of an index hint rather than t1's alias; base/Postgres keep them
	// alias-eligible (matching upstream, where an unqualified `USE INDEX (...)` after a
	// table is a MySQL-only construct).
	mysqlIndexHint := p.dialect.Name == "mysql" && tableIndexHintTokens[p.curr.TokenType]
	if p.curr.TokenType == tokens.ALIAS || (!mysqlIndexHint && !joinKinds[p.curr.TokenType] && !joinMethods[p.curr.TokenType] && !joinSides[p.curr.TokenType]) {
		if alias := p.parseTableAlias(aliasTokens); alias != nil {
			this.Set("alias", alias)
		}
	}
	if hints := p.parseTableHints(); len(hints) > 0 {
		this.Set("hints", hints)
	}
	if this.Arg("pivots") == nil {
		if pivots := p.parsePivots(); pivots != nil {
			this.Set("pivots", pivots)
		}
	}
	// TABLESAMPLE modifier (parser.py:4925-4926): most dialects attach `sample` here,
	// before the JOINs; ALIAS_POST_TABLESAMPLE dialects (none of base/mysql/postgres) would
	// attach it earlier instead, alongside the alias.
	if !p.dialect.AliasPostTablesample {
		if s := p.parseTableSample(false); s != nil {
			this.Set("sample", s)
		}
	}
	if joins {
		for {
			join := p.parseJoin(false, false, aliasTokens)
			if join == nil {
				break
			}
			this.Append("joins", join)
		}
	}
	// WITH ORDINALITY numbers the rows of a set-returning function source, e.g.
	// `FROM JSON_ARRAY_ELEMENTS(...) WITH ORDINALITY AS t(a, b)` (parser.py:4935-4937).
	// It carries its own alias, parsed after the keyword.
	if p.matchPair(tokens.WITH, tokens.ORDINALITY, true) {
		this.Set("ordinality", true)
		if alias := p.parseTableAlias(aliasTokens); alias != nil {
			this.Set("alias", alias)
		}
	}
	return this
}

func (p *Parser) parseTableParts(schema bool, isDBReference bool, wildcard bool, fast bool) exp.Expression {
	var catalog exp.Expression
	// schemaPart holds the ANSI-schema-level qualifier (upstream's "db" arg — the port renames
	// this arg to "schema"; the local can't be named "schema" here as that is already the
	// schema-position bool param).
	var schemaPart exp.Expression
	table := p.parseTablePart(schema)
	for p.match(tokens.DOT) {
		if catalog != nil {
			table = p.expression(exp.Dot(exp.Args{"this": table, "expression": p.parseTablePart(schema)}), nil, nil)
		} else {
			catalog = schemaPart
			schemaPart = table
			table = p.parseTablePart(schema)
			if table == nil {
				table = exp.Identifier(exp.Args{"this": "", "quoted": false})
			}
		}
	}
	if isDBReference {
		catalog = schemaPart
		schemaPart = table
		table = nil
	}
	if table == nil && !isDBReference {
		p.raiseError(fmt.Sprintf("Expected table name but got %s", p.curr.String()))
	}
	if schemaPart == nil && isDBReference {
		p.raiseError(fmt.Sprintf("Expected database name but got %s", p.curr.String()))
	}
	return p.expression(exp.Table(exp.Args{"this": table, "schema": schemaPart, "catalog": catalog}), nil, nil)
}

func (p *Parser) parseTablePart(schema bool) exp.Expression {
	// Table-valued function sources (parser.py:4664-4670), e.g. `FROM generate_series(1, 10)`
	// or `FROM JSON_TABLE(...)`: outside schema position, a function call takes priority over
	// a plain identifier so the parsed Func node becomes exp.Table's "this" arg.
	if !schema {
		if expression := p.parseFunction(nil, false, false, false); expression != nil {
			return expression
		}
	}
	if expression := p.parseIdVar(false, nil); expression != nil {
		return expression
	}
	if expression := p.parseStringAsIdentifier(); expression != nil {
		return expression
	}
	return p.parsePlaceholder()
}

func (p *Parser) parseTableAlias(aliasTokensArg map[tokens.TokenType]bool) exp.Expression {
	if p.canParseLimitOrOffset() {
		return nil
	}
	// STRAIGHT_JOIN must never be consumed as a table alias, in any dialect: upstream
	// base/mysql exclude it from TABLE_ALIAS_TOKENS (parsers/base.py:16-17) even though it
	// is otherwise a member of ID_VAR_TOKENS (tableAliasTokens here). The Go port compensates
	// in this guard because it dropped _parse_table's fast-path terminator check
	// (ROADMAP.md:142-144).
	if p.curr.TokenType == tokens.PIVOT || p.curr.TokenType == tokens.UNPIVOT || p.curr.TokenType == tokens.STRAIGHT_JOIN {
		return nil
	}
	anyToken := p.match(tokens.ALIAS)
	toks := aliasTokensArg
	if toks == nil {
		toks = tableAliasTokens
	}
	alias := p.parseIdVar(anyToken, toks)
	if alias == nil {
		alias = p.parseStringAsIdentifier()
	}
	var columns []exp.Expression
	idx := p.index
	if p.match(tokens.L_PAREN) {
		// Column aliases are parsed as column defs so typed forms like `AS y("rank" INT)`
		// (JSON_TO_RECORDSET etc.) are supported; parseColumnDef returns the bare id_var
		// unchanged when no type follows (_parse_function_parameter, parser.py:7111-7112).
		// any_token=true mirrors _parse_id_var's default (parser.py:8507), so keyword-like
		// alias columns such as `AS y(from INT)` are accepted.
		columns = p.parseCsv(func() exp.Expression { return p.parseColumnDef(p.parseIdVar(true, nil)) })
		if len(columns) > 0 {
			p.matchRParen(nil)
		} else {
			p.retreat(idx)
		}
	}
	if alias == nil && len(columns) == 0 {
		return nil
	}
	tableAlias := p.expression(exp.TableAlias(exp.Args{"this": alias, "columns": columns}), nil, nil)
	if alias != nil && alias.Kind() == exp.KindIdentifier {
		tableAlias.AddComments(alias.PopComments(), false)
	}
	return tableAlias
}

var joinMethods = map[tokens.TokenType]bool{tokens.ASOF: true, tokens.NATURAL: true, tokens.POSITIONAL: true}
var joinSides = map[tokens.TokenType]bool{tokens.FULL: true, tokens.LEFT: true, tokens.RIGHT: true}
var joinKinds = map[tokens.TokenType]bool{tokens.ANTI: true, tokens.CROSS: true, tokens.INNER: true, tokens.OUTER: true, tokens.SEMI: true, tokens.STRAIGHT_JOIN: true}

func (p *Parser) parseJoinParts() (method, side, kind *tokens.Token) {
	if p.matchSet(joinMethods) {
		prev := p.prev
		method = &prev
	}
	if p.matchSet(joinSides) {
		prev := p.prev
		side = &prev
	}
	if p.matchSet(joinKinds) {
		prev := p.prev
		kind = &prev
	}
	return method, side, kind
}

func (p *Parser) parseJoin(skipJoinToken bool, parseBracket bool, aliasTokensArg map[tokens.TokenType]bool) exp.Expression {
	if p.match(tokens.COMMA) {
		table := p.tryParse(func() exp.Expression { return p.parseTable(false, false, aliasTokensArg, false, false, false, false) }, false)
		if table == nil {
			return nil
		}
		join := p.expression(exp.Join(exp.Args{"this": table}), nil, nil)
		if p.dialect.JoinsHaveEqualPrecedence {
			join.Set("kind", "CROSS")
		}
		return join
	}
	index := p.index
	method, side, kind := p.parseJoinParts()
	join := p.match(tokens.JOIN) || (kind != nil && kind.TokenType == tokens.STRAIGHT_JOIN)
	joinComments := p.prevComments
	if !skipJoinToken && !join {
		p.retreat(index)
		method, side, kind = nil, nil, nil
	}
	// CROSS/OUTER APPLY (parser.py:4491-4494): non-consuming peeks so parseTable's
	// parseLateral consumes the pair and builds the Lateral (the join's `this`).
	outerApply := p.matchPair(tokens.OUTER, tokens.APPLY, false)
	crossApply := p.matchPair(tokens.CROSS, tokens.APPLY, false)
	if !skipJoinToken && !join && !outerApply && !crossApply {
		return nil
	}
	kwargs := exp.Args{"this": p.parseTable(false, false, aliasTokensArg, parseBracket, false, false, false)}
	if method != nil {
		kwargs["method"] = stringsUpper(method.Text)
	}
	if side != nil {
		kwargs["side"] = stringsUpper(side.Text)
	}
	if kind != nil {
		kwargs["kind"] = stringsUpper(kind.Text)
	}
	joinThis := asExpr(kwargs["this"])
	if p.match(tokens.ON) {
		kwargs["on"] = p.parseDisjunction()
	} else if p.match(tokens.USING) {
		kwargs["using"] = p.parseUsingIdentifiers()
	} else if method == nil && !outerApply && !crossApply && (joinThis == nil || joinThis.Kind() != exp.KindUnnest) && !(kind != nil && kind.TokenType == tokens.CROSS) {
		// Nested-join fallback (parser.py:4524-4541): a trailing ON/USING may belong to an
		// outer join wrapping further joins nested on this join's `this`, e.g.
		// `a JOIN b JOIN c USING (id) USING (id)` attaches the second USING to the `a JOIN b`
		// join while `b JOIN c USING (id)` is stored as a nested join on `b`.
		retreatIndex := p.index
		var nested []exp.Expression
		for {
			join := p.parseJoin(false, false, aliasTokensArg)
			if join == nil {
				break
			}
			nested = append(nested, join)
		}
		if len(nested) > 0 && p.match(tokens.ON) {
			kwargs["on"] = p.parseDisjunction()
		} else if len(nested) > 0 && p.match(tokens.USING) {
			kwargs["using"] = p.parseUsingIdentifiers()
		} else {
			nested = nil
			p.retreat(retreatIndex)
		}
		if joinThis != nil {
			if len(nested) > 0 {
				joinThis.Set("joins", nested)
			} else {
				joinThis.Set("joins", nil)
			}
		}
	}
	joinKind, _ := kwargs["kind"].(string)
	if p.dialect.AddJoinOnTrue && kwargs["on"] == nil && kwargs["using"] == nil && kwargs["method"] == nil &&
		(joinKind == "" || joinKind == "INNER" || joinKind == "OUTER") {
		kwargs["on"] = exp.Boolean(exp.Args{"this": true})
	}
	comments := append([]string(nil), joinComments...)
	for _, token := range []*tokens.Token{method, side, kind} {
		if token != nil {
			comments = append(comments, token.Comments...)
		}
	}
	return p.expression(exp.Join(kwargs), nil, comments)
}

func (p *Parser) parseWhere(skipWhereToken bool) exp.Expression {
	if !skipWhereToken && !p.match(tokens.WHERE) {
		return nil
	}
	comments := p.prevComments
	return p.expression(exp.Where(exp.Args{"this": p.parseDisjunction()}), nil, comments)
}

func (p *Parser) parseLocks() []exp.Expression {
	locks := []exp.Expression{}
	for {
		var update bool
		var key any
		if p.matchTextSeq("FOR", "UPDATE") {
			update = true
		} else if p.matchTextSeq("FOR", "SHARE") || p.matchTextSeq("LOCK", "IN", "SHARE", "MODE") {
			update = false
		} else if p.matchTextSeq("FOR", "KEY", "SHARE") {
			update = false
			key = true
		} else if p.matchTextSeq("FOR", "NO", "KEY", "UPDATE") {
			update = true
			key = true
		} else {
			break
		}

		var expressions []exp.Expression
		if p.matchTextSeq("OF") {
			expressions = p.parseCsv(func() exp.Expression {
				return p.parseTable(true, false, nil, false, false, false, false)
			})
		}

		var wait any
		if p.matchTextSeq("NOWAIT") {
			wait = true
		} else if p.matchTextSeq("WAIT") {
			wait = p.parsePrimary()
		} else if p.matchTextSeq("SKIP", "LOCKED") {
			wait = false
		}

		locks = append(locks, p.expression(exp.Lock(exp.Args{"update": update, "expressions": expressions, "wait": wait, "key": key}), nil, nil))
	}
	return locks
}

func (p *Parser) parsePrewhere() exp.Expression {
	if !p.match(tokens.PREWHERE) {
		return nil
	}
	comments := p.prevComments
	return p.expression(exp.PreWhere(exp.Args{"this": p.parseDisjunction()}), nil, comments)
}

func (p *Parser) parseCluster() exp.Expression {
	if !p.match(tokens.CLUSTER_BY) {
		return nil
	}
	return p.expression(exp.Cluster(exp.Args{"expressions": p.parseCsv(p.parseColumn)}), nil, nil)
}

func (p *Parser) parseSort(kind exp.Kind, tok tokens.TokenType) exp.Expression {
	if !p.match(tok) {
		return nil
	}
	return p.expression(exp.New(kind, exp.Args{"expressions": p.parseCsv(func() exp.Expression { return p.parseOrdered(nil) })}), nil, nil)
}

func (p *Parser) parseExpression() exp.Expression { return p.parseAlias(p.parseAssignment(), false) }

var assignmentTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.COLON_EQ: exp.PropertyEQ,
}

// parseAssignment ports _parse_assignment (parser.py:5790-5810): the ASSIGNMENT `:=`
// operator, e.g. mysql `SELECT @var1 := 1, @var2 := COUNT(*)`.
func (p *Parser) parseAssignment() exp.Expression {
	this := p.parseDisjunction()
	if this == nil && assignmentTokens[p.next.TokenType] != nil {
		// `<non-identifier token> := <expr>` (parser.py:5793-5796): lets a token that
		// otherwise fails to parse as a standalone expression still act as the LHS.
		if tok := p.advanceAny(true); tok != nil {
			this = exp.Column(exp.Args{"this": exp.ToIdentifier(tok.Text)})
		}
	}
	for constructor, ok := assignmentTokens[p.curr.TokenType]; ok; constructor, ok = assignmentTokens[p.curr.TokenType] {
		if this != nil && this.Kind() == exp.KindColumn && len(this.Parts()) == 1 {
			this = this.This()
		}
		p.advance()
		comments := p.prevComments
		this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseAssignment()}), nil, comments)
	}
	return this
}

// parseDisjunction ports _parse_disjunction (parser.py:5812-5822), matching the DISJUNCTION
// token set. Upstream's DISJUNCTION is per-dialect: base has only OR (parser.py:885-887);
// mysql additionally maps DPIPE -> exp.Or (parsers/mysql.py:78-80), since mysql's
// DPIPE_IS_STRING_CONCAT=False frees `||` up to mean logical OR instead of concatenation.
func (p *Parser) parseDisjunction() exp.Expression {
	this := p.parseConjunction()
	for p.match(tokens.OR) || (p.dialect.Name == "mysql" && p.match(tokens.DPIPE)) {
		comments := p.prevComments
		this = p.expression(exp.Or(exp.Args{"this": this, "expression": p.parseConjunction()}), nil, comments)
	}
	return this
}

// parseConjunction ports _parse_conjunction (parser.py:5824-5834), matching the CONJUNCTION
// token set. Upstream's CONJUNCTION is per-dialect: base has only AND (parser.py:877-879);
// mysql additionally maps DAMP (`&&`) -> exp.And and XOR -> exp.Xor (parsers/mysql.py:72-76).
func (p *Parser) parseConjunction() exp.Expression {
	this := p.parseEquality()
	for {
		var constructor func(exp.Args) exp.Expression
		switch {
		case p.match(tokens.AND):
			constructor = exp.And
		case p.dialect.Name == "mysql" && p.match(tokens.DAMP):
			constructor = exp.And
		case p.dialect.Name == "mysql" && p.match(tokens.XOR):
			constructor = exp.Xor
		default:
			return this
		}
		comments := p.prevComments
		this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseEquality()}), nil, comments)
	}
}

var equalityTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.EQ:          exp.EQ,
	tokens.NEQ:         exp.NEQ,
	tokens.NULLSAFE_EQ: exp.NullSafeEQ,
}

func (p *Parser) parseEquality() exp.Expression {
	this := p.parseComparison()
	for constructor, ok := equalityTokens[p.curr.TokenType]; ok; constructor, ok = equalityTokens[p.curr.TokenType] {
		p.advance()
		comments := p.prevComments
		this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseComparison()}), nil, comments)
	}
	return this
}

var comparisonTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.GT:  exp.GT,
	tokens.GTE: exp.GTE,
	tokens.LT:  exp.LT,
	tokens.LTE: exp.LTE,
}

func (p *Parser) parseComparison() exp.Expression {
	this := p.parseRange(nil)
	for constructor, ok := comparisonTokens[p.curr.TokenType]; ok; constructor, ok = comparisonTokens[p.curr.TokenType] {
		p.advance()
		comments := p.prevComments
		this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseRange(nil)}), nil, comments)
	}
	return this
}

func (p *Parser) parseRange(this exp.Expression) exp.Expression {
	if this == nil {
		this = p.parseBitwise()
	}
	negate := p.match(tokens.NOT)
	switch p.curr.TokenType {
	case tokens.BETWEEN:
		p.advance()
		this = p.parseBetween(this)
	case tokens.IN:
		p.advance()
		this = p.parseIn(this, false)
	case tokens.LIKE:
		p.advance()
		// Mirror _negate_range (parser.py): a plain LIKE has no negate key; only
		// NOT LIKE sets negate=True on the Like node (rather than wrapping in Not).
		args := exp.Args{"this": this, "expression": p.parseBitwise()}
		if negate {
			args["negate"] = true
		}
		this = p.parseEscape(p.expression(exp.Like(args), nil, nil))
		negate = false
	case tokens.ILIKE:
		p.advance()
		args := exp.Args{"this": this, "expression": p.parseBitwise()}
		if negate {
			args["negate"] = true
		}
		this = p.parseEscape(p.expression(exp.ILike(args), nil, nil))
		negate = false
	case tokens.SIMILAR_TO:
		// binary_range_parser(exp.SimilarTo) (parser.py:62-71): unlike LIKE/ILIKE above,
		// SimilarTo has no "negate" arg, so `negate` stays set and the generic
		// `Not`-wrapping below (_negate_range's fallback) applies to `NOT ... SIMILAR TO`.
		p.advance()
		this = p.parseEscape(p.expression(exp.SimilarTo(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.ISNULL:
		p.advance()
		this = p.expression(exp.Is(exp.Args{"this": this, "expression": exp.Null()}), nil, nil)
	case tokens.GLOB:
		// binary_range_parser(exp.Glob) (parser.py:1188).
		p.advance()
		this = p.parseEscape(p.expression(exp.Glob(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.OVERLAPS:
		// binary_range_parser(exp.Overlaps) (parser.py:1195).
		p.advance()
		this = p.parseEscape(p.expression(exp.Overlaps(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.RLIKE:
		// binary_range_parser(exp.RegexpLike) (parser.py:1196). RLIKE is only ever
		// tokenized by dialects that remap it (e.g. postgres' bare `~`), so this case is
		// effectively dialect-gated at the tokenizer without needing a check here.
		p.advance()
		this = p.parseEscape(p.expression(exp.RegexpLike(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.IRLIKE:
		// binary_range_parser(exp.RegexpILike) (parser.py:1191): base `~*` keyword.
		p.advance()
		this = p.parseEscape(p.expression(exp.RegexpILike(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.AT_GT:
		// binary_range_parser(exp.ArrayContainsAll) (parser.py:1186): postgres `@>`.
		p.advance()
		this = p.parseEscape(p.expression(exp.ArrayContainsAll(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.LT_AT:
		// binary_range_parser(exp.ArrayContainedBy) (parser.py:1194): postgres `<@`.
		p.advance()
		this = p.parseEscape(p.expression(exp.ArrayContainedBy(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.QMARK_AMP:
		// binary_range_parser(exp.JSONBContainsAllTopKeys) (parser.py:1199): postgres `?&`.
		p.advance()
		this = p.parseEscape(p.expression(exp.JSONBContainsAllTopKeys(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.QMARK_PIPE:
		// binary_range_parser(exp.JSONBContainsAnyTopKeys) (parser.py:1200): postgres `?|`.
		p.advance()
		this = p.parseEscape(p.expression(exp.JSONBContainsAnyTopKeys(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.HASH_DASH:
		// binary_range_parser(exp.JSONBDeleteAtPath) (parser.py:1201): postgres `#-`.
		p.advance()
		this = p.parseEscape(p.expression(exp.JSONBDeleteAtPath(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.AT_QMARK:
		// binary_range_parser(exp.JSONBPathExists) (parser.py:1202): postgres `@?`.
		p.advance()
		this = p.parseEscape(p.expression(exp.JSONBPathExists(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.ADJACENT:
		// binary_range_parser(exp.Adjacent) (parser.py:1203): postgres range `-|-`.
		p.advance()
		this = p.parseEscape(p.expression(exp.Adjacent(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
	case tokens.DAMP:
		// postgres-only: parsers/postgres.py RANGE_PARSERS adds
		// TokenType.DAMP: binary_range_parser(exp.ArrayOverlaps) on top of the base
		// Parser.RANGE_PARSERS. mysql's `&&` (TokenType.DAMP -> exp.And) is a
		// CONJUNCTION-level entry instead, owned by parseConjunction; base has no `&&`
		// support upstream either, so this case does nothing (leaves DAMP unconsumed)
		// for every other dialect.
		if p.dialect.Name == "postgres" {
			p.advance()
			this = p.parseEscape(p.expression(exp.ArrayOverlaps(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil))
		}
	case tokens.DAT:
		// parsers/postgres.py RANGE_PARSERS[TokenType.DAT]: postgres `x @@ y` full-text
		// match, built as exp.MatchAgainst(this=<rhs>, expressions=[<lhs>]) - not a
		// dedicated JSONB-path-match node (v30.12.0 has no exp.JSONBPathMatch). DAT is
		// only ever tokenized by postgres' `"@@": DAT` keyword remap, so (like DAMP's
		// non-postgres branches) this case is unreachable for other dialects.
		p.advance()
		this = p.expression(exp.MatchAgainst(exp.Args{"this": p.parseBitwise(), "expressions": []exp.Expression{this}}), nil, nil)
	case tokens.OPERATOR:
		// _parse_operator (parser.py:10122-10139): postgres `x OPERATOR(schema.op) y`,
		// chainable (`x OPERATOR(op1) y OPERATOR(op2) z`).
		p.advance()
		for {
			if !p.match(tokens.L_PAREN) {
				break
			}
			op := ""
			for p.curr.IsValid() && !p.match(tokens.R_PAREN) {
				op += p.curr.Text
				p.advance()
			}
			comments := p.prevComments
			this = p.expression(exp.Operator(exp.Args{"this": this, "operator": op, "expression": p.parseBitwise()}), nil, comments)
			if !p.match(tokens.OPERATOR) {
				break
			}
		}
	case tokens.MEMBER_OF:
		// mysql RANGE_PARSERS[TokenType.MEMBER_OF] (parsers/mysql.py:97-99): `x MEMBER
		// OF(y)` -> exp.JSONArrayContains(this=x, expression=y) (no dedicated exp.MemberOf
		// class upstream). MEMBER_OF is mysql-only tokenized, so this case is unreachable
		// elsewhere.
		p.advance()
		this = p.expression(exp.JSONArrayContains(exp.Args{"this": this, "expression": p.parseWrapped(p.parseExpression, false)}), nil, nil)
	case tokens.SOUNDS_LIKE:
		// mysql RANGE_PARSERS[TokenType.SOUNDS_LIKE] (parsers/mysql.py:91-96): `x SOUNDS
		// LIKE y` desugars to EQ(Soundex(x), Soundex(y)) (no dedicated exp.SoundsLike
		// class upstream). SOUNDS_LIKE is mysql-only tokenized.
		p.advance()
		this = p.expression(exp.EQ(exp.Args{
			"this":       p.expression(exp.Soundex(exp.Args{"this": this}), nil, nil),
			"expression": p.expression(exp.Soundex(exp.Args{"this": p.parseTerm()}), nil, nil),
		}), nil, nil)
	case tokens.IS:
		// RANGE_PARSERS[TokenType.IS] = lambda self, this: self._parse_is(this)
		// (parser.py:1192): matched here in addition to the unconditional `if
		// self._match(TokenType.IS)` check below (parser.py:5880-5881) so a chained
		// `x IS TRUE IS TRUE` gets two chances to consume an IS - parseIs's own trailing
		// parseColumnOps call doesn't re-enter IS itself.
		p.advance()
		this = p.parseIs(this)
	}
	if negate && p.match(tokens.NULL) {
		this = p.expression(exp.Is(exp.Args{"this": this, "expression": exp.Null()}), nil, nil)
		negate = false
	}
	// Postgres ISNULL/NOTNULL postfix predicates ("Postgres supports ISNULL and NOTNULL
	// for conditions", parser.py comment above _parse_range's NOTNULL check).
	if p.match(tokens.NOTNULL) {
		this = p.expression(exp.Is(exp.Args{"this": this, "expression": exp.Null()}), nil, nil)
		this = p.expression(exp.Not(exp.Args{"this": this}), nil, nil)
	}
	if p.match(tokens.IS) {
		this = p.parseIs(this)
	}
	if negate && this != nil {
		this = p.expression(exp.Not(exp.Args{"this": this}), nil, nil)
	}
	return this
}

// parseEscape ports _parse_escape (parser.py:5966-5971): `<this> ESCAPE <string|NULL>`.
func (p *Parser) parseEscape(this exp.Expression) exp.Expression {
	if !p.match(tokens.ESCAPE) {
		return this
	}
	expression := p.parseString()
	if expression == nil {
		expression = p.parseNull()
	}
	return p.expression(exp.Escape(exp.Args{"this": this, "expression": expression}), nil, nil)
}

// isJSONPredicateKind is IS_JSON_PREDICATE_KIND (parser.py:1707): the optional kind word
// following `IS JSON` - `x IS JSON [VALUE|SCALAR|ARRAY|OBJECT] ...`.
var isJSONPredicateKind = map[string]bool{"VALUE": true, "SCALAR": true, "ARRAY": true, "OBJECT": true}

func (p *Parser) parseIs(this exp.Expression) exp.Expression {
	index := p.index - 1
	negate := p.match(tokens.NOT)
	if p.matchTextSeq("DISTINCT", "FROM") {
		constructor := exp.NullSafeNEQ
		if negate {
			constructor = exp.NullSafeEQ
		}
		return p.expression(constructor(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil)
	}
	var expression exp.Expression
	if p.match(tokens.JSON) {
		// `x IS JSON [VALUE|SCALAR|ARRAY|OBJECT] [WITH|WITHOUT] [UNIQUE [KEYS]]`
		// (parser.py:5904-5916).
		var kind any
		if p.matchTexts(isJSONPredicateKind) {
			kind = stringsUpper(p.prev.Text)
		}
		var with_ any
		if p.matchTextSeq("WITH") {
			with_ = true
		} else if p.matchTextSeq("WITHOUT") {
			with_ = false
		}
		unique := p.match(tokens.UNIQUE)
		p.matchTextSeq("KEYS")
		expression = p.expression(exp.JSON(exp.Args{"this": kind, "with_": with_, "unique": unique}), nil, nil)
	} else {
		expression = p.parseNull()
		if expression == nil {
			expression = p.parseBoolean()
		}
		if expression == nil {
			expression = p.parseBitwise()
		}
		if expression == nil {
			p.retreat(index)
			return nil
		}
	}
	this = p.expression(exp.Is(exp.Args{"this": this, "expression": expression}), nil, nil)
	if negate {
		this = p.expression(exp.Not(exp.Args{"this": this}), nil, nil)
	}
	return p.parseColumnOps(this)
}

func (p *Parser) parseIn(this exp.Expression, alias bool) exp.Expression {
	if unnest := p.parseUnnest(false); unnest != nil {
		return p.expression(exp.In(exp.Args{"this": this, "unnest": unnest}), nil, nil)
	}
	if p.match(tokens.L_PAREN) {
		expressions := p.parseCsv(func() exp.Expression { return p.parseSelectOrExpression(alias) })
		if len(expressions) == 1 && expressions[0] != nil && expressions[0].Is(exp.TraitQuery) {
			query := p.parseQueryModifiers(expressions[0])
			this = p.expression(exp.In(exp.Args{"this": this, "query": exp.Subquery(exp.Args{"this": query})}), nil, nil)
		} else {
			this = p.expression(exp.In(exp.Args{"this": this, "expressions": expressions}), nil, nil)
		}
		p.matchRParen(this)
	} else {
		this = p.expression(exp.In(exp.Args{"this": this, "field": p.parseColumn()}), nil, nil)
	}
	return this
}

func (p *Parser) parseBetween(this exp.Expression) exp.Expression {
	var symmetric any
	if p.matchTextSeq("SYMMETRIC") {
		symmetric = true
	} else if p.matchTextSeq("ASYMMETRIC") {
		symmetric = false
	}
	low := p.parseBitwise()
	p.match(tokens.AND)
	high := p.parseBitwise()
	return p.expression(exp.Between(exp.Args{"this": this, "low": low, "high": high, "symmetric": symmetric}), nil, nil)
}

var bitwiseTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.AMP:   exp.BitwiseAnd,
	tokens.PIPE:  exp.BitwiseOr,
	tokens.CARET: exp.BitwiseXor,
}

// parseBitwise ports _parse_bitwise (parser.py:6064-6095): AMP/PIPE/CARET plus, when
// dialect-enabled, DPIPE-as-concat, DQMARK-as-Coalesce, and the two-token `<<`/`>>` shift
// operators (matchPair, since LT LT / GT GT are two adjacent single-char tokens, not one).
func (p *Parser) parseBitwise() exp.Expression {
	this := p.parseTerm()
	for {
		if constructor, ok := bitwiseTokens[p.curr.TokenType]; ok {
			p.advance()
			this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseTerm()}), nil, nil)
		} else if p.dialect.DPipeIsStringConcat && p.match(tokens.DPIPE) {
			this = p.expression(exp.DPipe(exp.Args{"this": this, "expression": p.parseTerm(), "safe": !p.dialect.StrictStringConcat}), nil, nil)
		} else if p.match(tokens.DQMARK) {
			this = p.expression(exp.Coalesce(exp.Args{"this": this, "expressions": []exp.Expression{p.parseTerm()}}), nil, nil)
		} else if p.matchPair(tokens.LT, tokens.LT, true) {
			this = p.expression(exp.BitwiseLeftShift(exp.Args{"this": this, "expression": p.parseTerm()}), nil, nil)
		} else if p.matchPair(tokens.GT, tokens.GT, true) {
			this = p.expression(exp.BitwiseRightShift(exp.Args{"this": this, "expression": p.parseTerm()}), nil, nil)
		} else {
			break
		}
	}
	return this
}

var termTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.DASH:    exp.Sub,
	tokens.PLUS:    exp.Add,
	tokens.MOD:     exp.Mod,
	tokens.COLLATE: exp.Collate,
}

// parseTerm ports _parse_term (parser.py:6097-6117). The COLLATE post-build step mirrors
// upstream's normalization (parser.py:6107-6115): a single-part column collation operand
// (e.g. `x COLLATE utf8_bin`) becomes a bare Var (or the Identifier itself, if quoted) instead
// of staying a Column, while a qualified one (e.g. `x COLLATE pg_catalog."default"`) is left
// as-is so it still round-trips.
func (p *Parser) parseTerm() exp.Expression {
	this := p.parseFactor()
	for constructor, ok := termTokens[p.curr.TokenType]; ok; constructor, ok = termTokens[p.curr.TokenType] {
		p.advance()
		comments := p.prevComments
		this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseFactor()}), nil, comments)

		if this != nil && this.Kind() == exp.KindCollate {
			if expr := this.Expr(); expr != nil && expr.Kind() == exp.KindColumn && len(expr.Parts()) == 1 {
				if ident := expr.This(); ident != nil && ident.Kind() == exp.KindIdentifier {
					if quoted, _ := ident.Arg("quoted").(bool); quoted {
						this.Set("expression", ident)
					} else {
						this.Set("expression", exp.Var(exp.Args{"this": ident.Name()}))
					}
				}
			}
		}
	}
	return this
}

var factorTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.SLASH:      exp.Div,
	tokens.STAR:       exp.Mul,
	tokens.DIV:        exp.Div,
	tokens.LR_ARROW:   exp.Distance,
	tokens.LLRR_ARROW: exp.DistanceNd,
}

func (p *Parser) parseFactor() exp.Expression {
	// EXPONENT is empty for base/mysql/postgres, so parse_method is always _parse_unary
	// (parser.py:6119-6121); only the first parse_method() call is wrapped in
	// _parse_at_time_zone, not the ones inside the loop below.
	this := p.parseAtTimeZone(p.parseUnary())
	for constructor, ok := factorTokens[p.curr.TokenType]; ok; constructor, ok = factorTokens[p.curr.TokenType] {
		tokenType := p.curr.TokenType
		p.advance()
		comments := p.prevComments
		args := exp.Args{"this": this, "expression": p.parseUnary()}
		if tokenType == tokens.SLASH || tokenType == tokens.DIV {
			args["typed"] = p.dialect.TypedDivision
			args["safe"] = p.dialect.SafeDivision
		}
		this = p.expression(constructor(args), nil, comments)
	}
	return this
}

func (p *Parser) parseUnary() exp.Expression {
	if p.match(tokens.DASH) {
		return p.expression(exp.Neg(exp.Args{"this": p.parseUnary()}), nil, p.prevComments)
	}
	if p.match(tokens.PLUS) {
		return p.parseUnary()
	}
	if p.match(tokens.NOT) {
		// Upstream UNARY_PARSERS binds NOT to _parse_equality, not _parse_unary
		// (parser.py:1115), so NOT is lower-precedence than comparison/range ops:
		// `NOT a = b` -> Not(EQ(a, b)), `NOT a IN (..)` -> Not(In(a, ..)).
		return p.expression(exp.Not(exp.Args{"this": p.parseEquality()}), nil, p.prevComments)
	}
	if p.match(tokens.TILDE) {
		return p.expression(exp.BitwiseNot(exp.Args{"this": p.parseUnary()}), nil, p.prevComments)
	}
	if p.dialect.Name == "postgres" && p.match(tokens.RLIKE) {
		// Postgres remaps the single-char `~` lexeme from TILDE to RLIKE, since `~` also
		// doubles as the binary REGEXP-LIKE operator there (parsers/postgres.py:185-188);
		// UNARY_PARSERS still builds BitwiseNot when RLIKE appears in prefix position.
		return p.expression(exp.BitwiseNot(exp.Args{"this": p.parseUnary()}), nil, p.prevComments)
	}
	if p.match(tokens.PIPE_SLASH) {
		return p.expression(exp.Sqrt(exp.Args{"this": p.parseUnary()}), nil, p.prevComments)
	}
	if p.match(tokens.DPIPE_SLASH) {
		return p.expression(exp.Cbrt(exp.Args{"this": p.parseUnary()}), nil, p.prevComments)
	}
	return p.parseType(true, false)
}

// parseType ports _parse_type (parser.py:6155-6217): try a plain atom first (unless
// fallbackToIdentifier), then INTERVAL, then a type-token prefix. A parsed type immediately
// followed by a literal builds a typed literal (`DATE '2020-01-01'`, and generally
// `<TYPE> '<lit>'` for any type token, not just a hardcoded DATE/DATETIME/TIMESTAMP/
// TIMESTAMPTZ set) as a Cast; a type that consumed more than its own keyword (e.g.
// `DECIMAL(38, 0)`) but isn't followed by a literal is itself the result; otherwise this
// wasn't a type after all and falls back to a plain column/identifier reference.
func (p *Parser) parseType(parseInterval, fallbackToIdentifier bool) exp.Expression {
	// parsers/mysql.py:545-558 MySQLParser._parse_type: "mysql binary is special and can
	// work anywhere, even in order by operations; it operates like a no paren func" - a bare
	// BINARY keyword prefix casts whatever follows, e.g. `ORDER BY BINARY a` ->
	// `ORDER BY CAST(a AS BINARY)`. Checked before the base logic below, matching upstream's
	// override calling super()._parse_type() only when this doesn't match.
	if p.dialect.Name == "mysql" && p.curr.TokenType == tokens.BINARY {
		index := p.index
		dataType := p.parseTypes(true, false, false, false)
		if dataType != nil && dataType.Kind() == exp.KindDataType {
			return p.expression(exp.Cast(exp.Args{"this": p.parseColumn(), "to": dataType}), nil, nil)
		}
		p.retreat(index)
	}
	if !fallbackToIdentifier {
		if atom := p.parseAtom(); atom != nil {
			return atom
		}
	}

	if parseInterval {
		if interval := p.parseInterval(true); interval != nil {
			return p.parseColumnOps(interval)
		}
	}

	index := p.index
	dataType := p.parseTypes(true, false, false, false)

	// parseTypes can return a Cast when it parses BQ's inline constructor <type>(<values>),
	// e.g. STRUCT<a INT, b STRING>(1, 'foo') - deferred (see parseTypes' TODO), but this
	// dispatch is kept ready for when that lands.
	if dataType != nil && dataType.Kind() == exp.KindCast {
		return p.parseColumnOps(dataType)
	}

	if dataType != nil {
		index2 := p.index
		this := p.parsePrimary()

		if this != nil && this.Kind() == exp.KindLiteral {
			// literal is captured from the original Literal before parseColumnOps rewrites
			// `this`, mirroring upstream `literal = this.name` (parser.py:6180).
			literal := this.Name()
			this = p.parseColumnOps(this)

			// TYPE_LITERAL_PARSERS (parser.py:1562-1564) maps only DType.JSON -> ParseJSON in
			// the base parser; ParseJSON isn't modeled by this port yet (out of this parity
			// slice's scope). ZONE_AWARE_TIMESTAMP_CONSTRUCTOR (parser.py:6186-6191) promotes
			// `TIMESTAMP '<zoned literal>'` -> TIMESTAMPTZ, gated on the Presto-only dialect
			// flag; base/mysql/postgres leave it false, so this branch is a no-op for them.
			if p.dialect.ZoneAwareTimestampConstructor && exp.DataTypeIsType(dataType, false, exp.DTypeTimestamp) && timeZoneRE.MatchString(literal) {
				dataType = exp.DataType(exp.Args{"this": exp.DTypeTimestampTz})
			}

			return p.expression(exp.Cast(exp.Args{"this": this, "to": dataType}), nil, nil)
		}

		if len(dataType.Expressions()) > 0 && index2-index > 1 {
			p.retreat(index2)
			return p.parseColumnOps(dataType)
		}

		p.retreat(index)
	}

	if fallbackToIdentifier {
		return p.parseIdVar(false, nil)
	}

	return p.parseColumn()
}

// primaryParsers ports the base PRIMARY_PARSERS (parser.py:1122-1173: STRING_PARSERS +
// NUMERIC_PARSERS + NULL/TRUE/FALSE/STAR). BIT_STRING/BYTE_STRING/HEX_STRING/INTRODUCER/
// SESSION_PARAMETER entries are omitted: those node kinds aren't modeled by this port yet.
// NATIONAL_STRING (-> National, slice-strings cluster) and UNICODE_STRING (-> UnicodeString,
// e.g. Presto's `U&'...'`) are included below. The UESCAPE clause of UNICODE_STRING
// (parser.py:1135-1140) is deferred - `escape` stays unset.
//
// Declared as an empty map + func init() assignment (rather than a map literal) because a
// literal here would create a Go initialization-cycle: primaryParsers' STAR entry calls
// parseStarOps, which transitively calls back into code that references primaryParsers
// (parseType -> ... -> parseAtom), and Go's initialization-order analysis doesn't allow that
// for package-level var literals (mirrors generator/dispatch.go's identical workaround).
var primaryParsers = map[tokens.TokenType]func(*Parser, tokens.Token) exp.Expression{}

func init() {
	primaryParsers[tokens.STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.LiteralString(token.Text), &token, nil)
	}
	primaryParsers[tokens.HEREDOC_STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.RawString(exp.Args{"this": token.Text}), &token, nil)
	}
	primaryParsers[tokens.RAW_STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.RawString(exp.Args{"this": token.Text}), &token, nil)
	}
	primaryParsers[tokens.NATIONAL_STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.National(exp.Args{"this": token.Text}), &token, nil)
	}
	primaryParsers[tokens.UNICODE_STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.UnicodeString(exp.Args{"this": token.Text}), &token, nil)
	}
	primaryParsers[tokens.NUMBER] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.LiteralNumber(token.Text), &token, nil)
	}
	primaryParsers[tokens.NULL] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.Null(), &token, nil)
	}
	primaryParsers[tokens.TRUE] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.Boolean(exp.Args{"this": true}), &token, nil)
	}
	primaryParsers[tokens.FALSE] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.Boolean(exp.Args{"this": false}), &token, nil)
	}
	primaryParsers[tokens.STAR] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.parseStarOps(token)
	}
	// NUMERIC_PARSERS (parser.py:1143-1162) BIT_STRING/HEX_STRING/BYTE_STRING entries: the
	// is_integer/is_bytes flags mirror `self.dialect.HEX_STRING_IS_INTEGER_TYPE or None` /
	// `self.dialect.BYTE_STRING_IS_BYTES_TYPE or None` - always false for base/mysql/postgres
	// (see dialects.Dialect.HexStringIsIntegerType's doc), so the arg is always the zero value
	// here, kept only for 1:1 shape with hexstringSQL/bytestringSQL's upstream signature.
	primaryParsers[tokens.BIT_STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.BitString(exp.Args{"this": token.Text}), &token, nil)
	}
	primaryParsers[tokens.HEX_STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.HexString(exp.Args{"this": token.Text, "is_integer": p.dialect.HexStringIsIntegerType}), &token, nil)
	}
	primaryParsers[tokens.BYTE_STRING] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.expression(exp.ByteString(exp.Args{"this": token.Text, "is_bytes": p.dialect.ByteStringIsBytesType}), &token, nil)
	}
	// PRIMARY_PARSERS (parser.py:1171): SESSION_PARAMETER -> _parse_session_parameter().
	primaryParsers[tokens.SESSION_PARAMETER] = func(p *Parser, token tokens.Token) exp.Expression {
		return p.parseSessionParameter()
	}
}

// parseSessionParameter ports _parse_session_parameter (parser.py:7168-7176): mysql
// `@@GLOBAL.max_connections` (kind="GLOBAL", this=Var(max_connections)) or bare `@@x`
// (kind=nil, this=Var(x)/Identifier(x)).
func (p *Parser) parseSessionParameter() exp.Expression {
	this := p.parseIdVar(true, nil)
	if this == nil {
		this = p.parsePrimary()
	}
	var kind any
	if this != nil && p.match(tokens.DOT) {
		kind = this.Name()
		this = p.parseVar(false, nil, false)
		if this == nil {
			this = p.parsePrimary()
		}
	}
	return p.expression(exp.SessionParameter(exp.Args{"this": this, "kind": kind}), nil, nil)
}

// parseAtom ports _parse_atom (parser.py:6560-6583): a simple column reference, or a
// PRIMARY_PARSERS literal that isn't immediately followed by a column operator/postfix token
// (that case is deferred to parseColumn -> parseColumnOps instead, so e.g. `1::int` gets cast
// handling rather than the first literal being consumed here with no further column-op
// processing). The STRING-followed-by-STRING bail below mirrors upstream, which routes
// adjacent string literals to CONCAT; that adjacency->CONCAT rewrite is separately deferred in
// this port (ROADMAP), so for now `'a' 'b'` falls through to plain alias handling instead - the
// bail is neutral either way (a STRING is not a column op, so parseColumnOps adds nothing).
func (p *Parser) parseAtom() exp.Expression {
	if identifierTokens[p.curr.TokenType] {
		if column := p.parseColumn(); column != nil {
			return column
		}
	}

	token := p.curr
	tokenType := token.TokenType

	primaryParser, ok := primaryParsers[tokenType]
	if !ok {
		return nil
	}

	// columnFastBailTokens is exactly COLUMN_OPERATORS ∪ COLUMN_POSTFIX_TOKENS (parser.py:
	// 804-812, 1008-1035; see parser/sets.go's definition for the cross-reference).
	nextType := p.next.TokenType
	if columnFastBailTokens[nextType] || (tokenType == tokens.STRING && nextType == tokens.STRING) {
		return nil
	}

	p.advance()
	return primaryParser(p, token)
}

// parseAtTimeZone ports _parse_at_time_zone (parser.py:6553-6558): `<this> AT TIME ZONE
// <zone>`, right-associative-by-recursion so `x AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Tokyo'`
// nests as AtTimeZone(AtTimeZone(x, 'UTC'), 'Asia/Tokyo').
func (p *Parser) parseAtTimeZone(this exp.Expression) exp.Expression {
	if !p.matchTextSeq("AT", "TIME", "ZONE") {
		return this
	}
	return p.parseAtTimeZone(p.expression(exp.AtTimeZone(exp.Args{"this": this, "zone": p.parseUnary()}), nil, nil))
}

func (p *Parser) parseColumn() exp.Expression {
	column := p.parseColumnPartsFast()
	if column == nil {
		// Mirror _parse_column (parser.py:6591): `_parse_column_ops(this) if this else this`.
		// When parseColumnReference returns nil (e.g. while a NO_PAREN_FUNCTION speculative
		// parse like IF backs out), a column operator such as DOT would build a Dot with a
		// nil `this` and fail required-arg validation instead of cleanly declining. Skip
		// parseColumnOps in that case - except for a leading bracket, which this port routes
		// through parseColumnOps to build a bare array literal (`[1, 2]`) with a nil `this`.
		this := p.parseColumnReference()
		if this == nil && !bracketsTokens[p.curr.TokenType] {
			return nil
		}
		column = p.parseColumnOps(this)
	}
	return column
}

func (p *Parser) parseColumnPartsFast() exp.Expression {
	index := p.index
	var parts []exp.Expression
	var allComments []string
	for p.matchSet(identifierTokens) {
		token := p.prev
		comments := p.prevComments
		if len(parts) == 0 && p.noParenFunctionParserFor(stringsUpper(token.Text)) != nil {
			p.retreat(index)
			return nil
		}
		hasDot := p.match(tokens.DOT)
		currTT := p.curr.TokenType
		if !hasDot {
			if columnFastBailTokens[currTT] {
				p.retreat(index)
				return nil
			}
		} else if !identifierTokens[currTT] {
			p.retreat(index)
			return nil
		}
		if len(comments) > 0 {
			allComments = append(allComments, comments...)
			p.prevComments = nil
		}
		parts = append(parts, p.expression(exp.Identifier(exp.Args{"this": token.Text, "quoted": token.TokenType == tokens.IDENTIFIER}), &token, nil))
		if !hasDot {
			break
		}
	}
	if len(parts) == 0 {
		return nil
	}
	var column exp.Expression
	switch len(parts) {
	case 1:
		column = exp.Column(exp.Args{"this": parts[0]})
	case 2:
		column = exp.Column(exp.Args{"this": parts[1], "table": parts[0]})
	case 3:
		column = exp.Column(exp.Args{"this": parts[2], "table": parts[1], "schema": parts[0]})
	default:
		column = exp.Column(exp.Args{"this": parts[3], "table": parts[2], "schema": parts[1], "catalog": parts[0]})
		for _, part := range parts[4:] {
			column = exp.Dot(exp.Args{"this": column, "expression": part})
		}
	}
	if len(allComments) > 0 {
		column.AddComments(allComments, false)
	}
	return column
}

// parseColumnReference ports _parse_column_reference (parser.py:6674-6688). A bare VALUES
// not immediately followed by "(" is reparsed as a plain identifier (e.g. `SELECT values`,
// `values.c`) when the dialect allows it (see Dialect.ValuesFollowedByParen).
func (p *Parser) parseColumnReference() exp.Expression {
	this := p.parseField(false, nil, false)
	if this == nil && p.match(tokens.VALUES, false) && p.dialect.ValuesFollowedByParen &&
		(!p.next.IsValid() || p.next.TokenType != tokens.L_PAREN) {
		this = p.parseIdVar(true, nil)
	}
	if this != nil && this.Kind() == exp.KindIdentifier {
		this = p.expression(exp.Column(exp.Args{"this": this}), nil, this.PopComments())
	}
	return this
}

type columnOpFunc func(p *Parser, this, field exp.Expression) exp.Expression

// arrowJSONExtractArgs builds the args for a JSON `->`/`->>` operator node (exp.JSONExtract/
// JSONExtractScalar). build_json_extract_path (dialect.py:2092-2124) is not ported: this port
// keeps the raw literal RHS instead of converting it to a JSONPath (see ROADMAP - JSONPath
// deferral). The only signal that decision needs from that upstream builder is its
// arrow_req_json_type branch, which sets only_json_types when the dialect requires it (postgres
// only, JSONArrowsRequireJSONType) and the RHS is a bare Literal (so e.g. `x -> -1`'s Neg RHS
// does NOT set only_json_types - only a bare literal does). The generator reads only_json_types
// to choose operator-form (`->`/`->>`) over function-form (JSON_EXTRACT_PATH[_TEXT]) rendering.
func arrowJSONExtractArgs(p *Parser, this, path exp.Expression) exp.Args {
	args := exp.Args{"this": this, "expression": path}
	if p.dialect.JSONArrowsRequireJSONType && path != nil && path.Kind() == exp.KindLiteral {
		args["only_json_types"] = true
	}
	return args
}

var columnOperators = map[tokens.TokenType]columnOpFunc{
	tokens.DOT: nil,
	tokens.DCOLON: func(p *Parser, this, to exp.Expression) exp.Expression {
		return p.buildCast(p.strictCast, exp.Args{"this": this, "to": to})
	},
	tokens.DOTCOLON: func(p *Parser, this, to exp.Expression) exp.Expression {
		return p.expression(exp.JSONCast(exp.Args{"this": this, "to": to}), nil, nil)
	},
	// ARROW/DARROW keep the raw literal RHS and gate only_json_types via arrowJSONExtractArgs
	// (see its doc for the JSONPath-deferral rationale).
	tokens.ARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONExtract(arrowJSONExtractArgs(p, this, path)), nil, nil)
	},
	tokens.DARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONExtractScalar(arrowJSONExtractArgs(p, this, path)), nil, nil)
	},
	tokens.HASH_ARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONBExtract(exp.Args{"this": this, "expression": path}), nil, nil)
	},
	tokens.DHASH_ARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONBExtractScalar(exp.Args{"this": this, "expression": path}), nil, nil)
	},
	// COLUMN_OPERATORS[TokenType.PLACEHOLDER] (parser.py:1033-1035): postgres `x ? key`
	// jsonb-contains-key operator, reusing the base "?" -> PLACEHOLDER SINGLE_TOKENS
	// mapping (tokens.py:161) rather than a dedicated keyword.
	tokens.PLACEHOLDER: func(p *Parser, this, key exp.Expression) exp.Expression {
		return p.expression(exp.JSONBContains(exp.Args{"this": this, "expression": key}), nil, nil)
	},
}

var castColumnOperators = map[tokens.TokenType]bool{tokens.DCOLON: true, tokens.DOTCOLON: true}

func (p *Parser) parseColumnOps(this exp.Expression) exp.Expression {
	for bracketsTokens[p.curr.TokenType] {
		this = p.parseBracket(this)
	}
	for p.curr.IsValid() {
		opToken := p.curr.TokenType
		op, ok := columnOperators[opToken]
		if !ok {
			break
		}
		p.advance()
		var field exp.Expression
		if castColumnOperators[opToken] {
			field = p.parseDcolon()
			if field == nil {
				p.raiseError("Expected type")
			}
		} else if op != nil && p.curr.IsValid() {
			field = p.parseColumnReference()
			if field == nil {
				field = p.parseBitwise()
			}
			if field != nil && field.Kind() == exp.KindColumn && p.match(tokens.DOT, false) {
				field = p.parseColumnOps(field)
			}
		} else {
			// Upstream _parse_column_ops takes the plain-DOT (op is None) branch here,
			// which uses _parse_field(any_token=True, anonymous_func=True) so a following
			// "(" lets the field parse as a function call (e.g. x.y.FOO()).
			field = p.parseField(true, nil, true)
		}

		// Function calls can be qualified, e.g. x.y.FOO(). Convert the accumulated
		// column into a series of Dots leading to the call (parser.py:6793-6799).
		if field != nil && (field.Is(exp.TraitFunc) || field.Kind() == exp.KindWindow) && this != nil {
			this = exp.ColumnsToDot(this)
		}

		if op != nil {
			this = op(p, this, field)
		} else if this != nil && this.Kind() == exp.KindColumn && this.Arg("catalog") == nil {
			this = p.expression(exp.Column(exp.Args{"this": field, "table": this.This(), "schema": this.Arg("table"), "catalog": this.Arg("schema")}), nil, this.Comments())
		} else if field != nil && field.Kind() == exp.KindWindow {
			// Move the Dot into the window's function (parser.py:6813-6817).
			windowFunc := p.expression(exp.Dot(exp.Args{"this": this, "expression": field.This()}), nil, nil)
			field.Set("this", windowFunc)
			this = field
		} else {
			this = p.expression(exp.Dot(exp.Args{"this": this, "expression": field}), nil, nil)
		}
		if field != nil && len(field.Comments()) > 0 {
			this.AddComments(field.PopComments(), false)
		}
		this = p.parseBracket(this)
	}
	return this
}

func (p *Parser) parseParen() exp.Expression {
	if !p.match(tokens.L_PAREN) {
		return nil
	}
	comments := p.prevComments
	query := p.parseSelect(false, false, true, true)
	var expressions []exp.Expression
	if query != nil {
		expressions = []exp.Expression{query}
	} else {
		expressions = p.parseExpressions()
	}
	var this exp.Expression
	if len(expressions) > 0 {
		this = expressions[0]
	}
	if this == nil && p.curr.TokenType == tokens.R_PAREN {
		this = p.expression(exp.Tuple(nil), nil, nil)
	} else if isUnwrappedQuery(this) {
		this = p.parseSubquery(this, false)
	} else if this != nil && this.Kind() == exp.KindSubquery {
		this = p.parseSubquery(p.parseQueryModifiers(p.parseSetOperations(this)), false)
	} else if len(expressions) > 1 || p.prev.TokenType == tokens.COMMA {
		this = p.expression(exp.Tuple(exp.Args{"expressions": expressions}), nil, nil)
	} else {
		this = p.expression(exp.Paren(exp.Args{"this": this}), nil, nil)
	}
	if this != nil {
		this.AddComments(comments, false)
	}
	p.matchRParen(this)
	if this != nil && this.Kind() == exp.KindParen && this.This() != nil && this.This().Is(exp.TraitAggFunc) {
		return p.parseWindow(this, false)
	}
	return this
}

func (p *Parser) parsePrimary() exp.Expression {
	token := p.curr
	// Both _parse_primary and _parse_atom (parser.py:6560-6583) dispatch through the single
	// PRIMARY_PARSERS table; primaryParsers is that shared port (STRING/NUMBER/NULL/TRUE/
	// FALSE literals, HEREDOC_STRING/RAW_STRING -> RawString, STAR -> parseStarOps).
	if primaryParser, ok := primaryParsers[token.TokenType]; ok {
		p.advance()
		primary := primaryParser(p, token)
		if token.TokenType == tokens.STRING {
			// _parse_primary's adjacent-string-literal rewrite (parser.py:6871-6885):
			// 'a' 'b' 'c' -> Concat('a', 'b', 'c'). ADJACENT_STRINGS_CANNOT_BE_CONNECTED
			// defaults false and isn't overridden by base/mysql/postgres, so the
			// _is_connected() raise-error branch never fires and is omitted here.
			concatExpressions := []exp.Expression{primary}
			for p.curr.TokenType == tokens.STRING {
				p.advance()
				concatExpressions = append(concatExpressions, exp.LiteralString(p.prev.Text))
			}
			if len(concatExpressions) > 1 {
				return p.expression(exp.Concat(exp.Args{"expressions": concatExpressions, "coalesce": p.dialect.ConcatCoalesce}), nil, nil)
			}
		}
		return primary
	}
	if p.matchPair(tokens.DOT, tokens.NUMBER, true) {
		return exp.LiteralNumber("0." + p.prev.Text)
	}
	// Ports the `if not self._match(TokenType.L_PAREN): return self._parse_placeholder()`
	// tail of _parse_primary (parser.py): a non-paren primary may be a placeholder/parameter
	// (`?`, `@var`). Peek rather than consume so parseParen still matches the L_PAREN itself.
	if p.curr.TokenType != tokens.L_PAREN {
		return p.parsePlaceholder()
	}
	return p.parseParen()
}

func (p *Parser) primaryFromToken(token tokens.Token) exp.Expression {
	switch token.TokenType {
	case tokens.STRING:
		return p.expression(exp.LiteralString(token.Text), &token, nil)
	case tokens.NUMBER:
		return p.expression(exp.LiteralNumber(token.Text), &token, nil)
	case tokens.NULL:
		return p.expression(exp.Null(), &token, nil)
	case tokens.TRUE:
		return p.expression(exp.Boolean(exp.Args{"this": true}), &token, nil)
	case tokens.FALSE:
		return p.expression(exp.Boolean(exp.Args{"this": false}), &token, nil)
	case tokens.STAR:
		return p.expression(exp.Star(nil), &token, nil)
	}
	return nil
}

func (p *Parser) parseField(anyToken bool, toks map[tokens.TokenType]bool, anonymousFunc bool) exp.Expression {
	var field exp.Expression
	if anonymousFunc {
		field = p.parseFunction(nil, anonymousFunc, true, anyToken)
		if field == nil {
			field = p.parsePrimary()
		}
	} else {
		field = p.parsePrimary()
		if field == nil {
			field = p.parseFunction(nil, false, true, anyToken)
		}
	}
	if field == nil {
		field = p.parseIdVar(anyToken, toks)
	}
	return field
}

func (p *Parser) parseAlias(this exp.Expression, explicit bool) exp.Expression {
	if p.canParseLimitOrOffset() {
		return this
	}
	if p.canParseNamedWindow() {
		return this
	}
	anyToken := p.match(tokens.ALIAS)
	comments := p.prevComments
	if explicit && !anyToken {
		return this
	}
	if p.match(tokens.L_PAREN) {
		aliases := p.expression(exp.Aliases(exp.Args{"this": this, "expressions": p.parseCsv(func() exp.Expression { return p.parseIdVar(anyToken, nil) })}), nil, comments)
		p.matchRParen(aliases)
		return aliases
	}
	alias := p.parseIdVar(anyToken, aliasTokens)
	if alias == nil {
		alias = p.parseStringAsIdentifier()
	}
	if alias != nil {
		comments = append(comments, alias.PopComments()...)
		this = p.expression(exp.AliasNode(exp.Args{"this": this, "alias": alias}), nil, comments)
	}
	return this
}

func (p *Parser) parseIdVar(anyToken bool, toks map[tokens.TokenType]bool) exp.Expression {
	expression := p.parseIdentifier()
	if expression == nil {
		if (anyToken && p.advanceAny(false) != nil) || p.matchSet(tokensOrDefault(toks, idVarTokens)) {
			quoted := p.prev.TokenType == tokens.STRING
			expression = p.identifierExpression(quoted)
		}
	}
	return expression
}

func (p *Parser) parseStringAsIdentifier() exp.Expression {
	if !p.match(tokens.STRING) {
		return nil
	}
	return exp.ToIdentifier(p.prev.Text, true)
}

func (p *Parser) parseIdentifier() exp.Expression {
	if p.match(tokens.IDENTIFIER) {
		return p.identifierExpression(true)
	}
	return p.parsePlaceholder()
}

func (p *Parser) identifierExpression(quoted bool) exp.Expression {
	return p.expression(exp.Identifier(exp.Args{"this": p.prev.Text, "quoted": quoted}), &p.prev, nil)
}

func (p *Parser) advanceAny(ignoreReserved bool) *tokens.Token {
	if p.curr.IsValid() && (ignoreReserved || !reservedTokens[p.curr.TokenType]) {
		p.advance()
		return &p.prev
	}
	return nil
}

func (p *Parser) parseNull() exp.Expression {
	if p.curr.TokenType == tokens.NULL || p.curr.TokenType == tokens.UNKNOWN {
		token := p.curr
		p.advance()
		return p.primaryFromToken(token)
	}
	return p.parsePlaceholder()
}

func (p *Parser) parseBoolean() exp.Expression {
	if p.curr.TokenType == tokens.TRUE || p.curr.TokenType == tokens.FALSE {
		token := p.curr
		p.advance()
		return p.primaryFromToken(token)
	}
	return p.parsePlaceholder()
}

func (p *Parser) parseStar() exp.Expression {
	if p.curr.TokenType == tokens.STAR {
		token := p.curr
		p.advance()
		return p.parseStarOps(token)
	}
	return p.parsePlaceholder()
}

func (p *Parser) parseStarOps(starToken tokens.Token) exp.Expression {
	var ilike exp.Expression
	if p.match(tokens.ILIKE) {
		if p.match(tokens.STRING) {
			ilike = p.expression(exp.LiteralString(p.prev.Text), &p.prev, nil)
		} else {
			ilike = p.parseIdVar(false, nil)
		}
	}
	args := exp.Args{
		"ilike":   ilike,
		"except_": p.parseStarOp("EXCEPT", "EXCLUDE"),
		"replace": p.parseStarOp("REPLACE"),
		"rename":  p.parseStarOp("RENAME"),
	}
	return p.expression(exp.Star(args), &starToken, nil)
}

func (p *Parser) parseStarOp(keywords ...string) []exp.Expression {
	matched := false
	for _, keyword := range keywords {
		if p.curr.TokenType != tokens.STRING && stringsUpper(p.curr.Text) == keyword {
			p.advance()
			matched = true
			break
		}
	}
	if !matched {
		return nil
	}
	if p.curr.TokenType == tokens.L_PAREN {
		return p.parseWrappedCsv(p.parseExpression)
	}
	expression := p.parseAlias(p.parseDisjunction(), true)
	if expression != nil {
		return []exp.Expression{expression}
	}
	return nil
}

// parsePlaceholder ports PLACEHOLDER_PARSERS (parser.py:1175-1183) + _parse_placeholder
// (parser.py:8590-8597). PARAMETER (`@var`) routes to parseParameter; COLON (`:name`)
// builds a named Placeholder from the following ID_VAR_TOKENS token (e.g. `:hello`),
// retreating past the COLON if no such token follows (mirrors the generic
// _parse_placeholder's `self._advance(-1)` on a falsy PLACEHOLDER_PARSERS result).
// Postgres overrides both PLACEHOLDER (jdbc=True, so `?` round-trips as `?` even under
// PARAMETER_TOKEN="%") and adds MOD ("%") -> parseQueryParameter for its pyformat/psycopg
// `%s` / `%(name)s` placeholders (parsers/postgres.py:94-97).
func (p *Parser) parsePlaceholder() exp.Expression {
	if p.match(tokens.PLACEHOLDER) {
		if p.dialect.Name == "postgres" {
			return p.expression(exp.Placeholder(exp.Args{"jdbc": true}), &p.prev, nil)
		}
		return p.expression(exp.Placeholder(nil), &p.prev, nil)
	}
	if p.match(tokens.PARAMETER) {
		return p.parseParameter()
	}
	if p.dialect.Name == "postgres" && p.match(tokens.MOD) {
		return p.parseQueryParameter()
	}
	if p.match(tokens.COLON) {
		if p.matchSet(idVarTokens) {
			return p.expression(exp.Placeholder(exp.Args{"this": p.prev.Text}), &p.prev, nil)
		}
		p.retreat(p.index - 1)
	}
	return nil
}

// parseQueryParameter ports parsers/postgres.py:294-301 PostgresParser._parse_query_parameter:
// the psycopg pyformat placeholder, either bare `%s` or named `%(name)s`. The leading `%`
// (MOD token) is already consumed by parsePlaceholder.
func (p *Parser) parseQueryParameter() exp.Expression {
	var this exp.Expression
	if p.curr.TokenType == tokens.L_PAREN {
		// _parse_wrapped(self._parse_id_var): default any_token=True, so a non-reserved keyword
		// or number is a valid pyformat name (`%(from)s`, `%(1)s`), not just a bare identifier.
		this = p.parseWrapped(func() exp.Expression { return p.parseIdVar(true, nil) }, false)
	}
	p.matchTextSeq("S")
	return p.expression(exp.Placeholder(exp.Args{"this": this}), nil, nil)
}

// parseParameter ports _parse_parameter (parser.py:8586-8588): the leading `@` is already
// consumed; the following identifier/var becomes the parameter's `this` (mysql `@var1`).
func (p *Parser) parseParameter() exp.Expression {
	this := p.parseIdentifier()
	if this == nil {
		this = p.parsePrimaryOrVar()
	}
	return p.expression(exp.Parameter(exp.Args{"this": this}), nil, nil)
}

// parsePrimaryOrVar ports _parse_primary_or_var (parser.py:8566-8567).
func (p *Parser) parsePrimaryOrVar() exp.Expression {
	if primary := p.parsePrimary(); primary != nil {
		return primary
	}
	return p.parseVar(true, nil, false)
}

func (p *Parser) parseCsv(parseMethod func() exp.Expression, sep ...tokens.TokenType) []exp.Expression {
	separator := tokens.COMMA
	if len(sep) > 0 {
		separator = sep[0]
	}
	parseResult := parseMethod()
	items := []exp.Expression{}
	if parseResult != nil {
		items = append(items, parseResult)
	}
	for p.match(separator) {
		if parseResult != nil {
			p.addComments(parseResult)
		}
		parseResult = parseMethod()
		if parseResult != nil {
			items = append(items, parseResult)
		}
	}
	return items
}

func (p *Parser) parseExpressions() []exp.Expression { return p.parseCsv(p.parseExpression) }

func (p *Parser) parseWith(skipWithToken bool) exp.Expression {
	if !skipWithToken && !p.match(tokens.WITH) {
		return nil
	}
	comments := p.prevComments
	recursive := p.match(tokens.RECURSIVE)
	var lastComments []string
	expressions := []exp.Expression{}
	for {
		cte := p.parseCTE()
		if cte != nil && cte.Kind() == exp.KindCTE {
			expressions = append(expressions, cte)
			if len(lastComments) > 0 {
				cte.AddComments(lastComments, false)
			}
		}
		if !p.match(tokens.COMMA) && !p.match(tokens.WITH) {
			break
		}
		p.match(tokens.WITH)
		lastComments = p.prevComments
	}
	args := exp.Args{"expressions": expressions, "search": p.parseRecursiveWithSearch()}
	if recursive {
		args["recursive"] = true
	}
	return p.expression(exp.With(args), nil, comments)
}

func (p *Parser) parseCTE() exp.Expression {
	alias := p.parseTableAlias(idVarTokens)
	if alias == nil || alias.This() == nil {
		p.raiseError("Expected CTE to have alias")
	}
	p.match(tokens.ALIAS)
	comments := p.prevComments
	var materialized any
	if p.matchTextSeq("NOT", "MATERIALIZED") {
		materialized = false
	} else if p.matchTextSeq("MATERIALIZED") {
		materialized = true
	}
	return p.expression(exp.CTE(exp.Args{
		"this":         p.parseWrapped(func() exp.Expression { return p.parseStatement() }, false),
		"alias":        alias,
		"materialized": materialized,
	}), nil, comments)
}

func (p *Parser) parseRecursiveWithSearch() exp.Expression {
	p.matchTextSeq("SEARCH")
	kind := ""
	if p.matchTexts(recursiveCteSearchKind) {
		kind = stringsUpper(p.prev.Text)
	}
	if kind == "" {
		return nil
	}
	p.matchTextSeq("FIRST", "BY")
	args := exp.Args{"kind": kind, "this": p.parseIdVar(false, nil), "expression": false}
	if p.matchTextSeq("SET") {
		args["expression"] = p.parseIdVar(false, nil)
	}
	if p.matchTextSeq("USING") {
		args["using"] = p.parseIdVar(false, nil)
	}
	return p.expression(exp.RecursiveWithSearch(args), nil, nil)
}

func (p *Parser) parseWrapped(parseMethod func() exp.Expression, optional bool) exp.Expression {
	wrapped := p.match(tokens.L_PAREN)
	if !wrapped && !optional {
		p.raiseError("Expecting (")
	}
	parseResult := parseMethod()
	if wrapped {
		p.matchRParen(parseResult)
	}
	return parseResult
}

func (p *Parser) parseWrappedCsv(parseMethod func() exp.Expression, optional ...bool) []exp.Expression {
	opt := false
	if len(optional) > 0 {
		opt = optional[0]
	}
	wrapped := p.match(tokens.L_PAREN)
	if !wrapped && !opt {
		p.raiseError("Expecting (")
	}
	parseResult := p.parseCsv(parseMethod)
	if wrapped {
		p.matchRParen(nil)
	}
	return parseResult
}

func (p *Parser) parseValue(values bool) exp.Expression {
	if p.match(tokens.L_PAREN) {
		expressions := p.parseCsv(p.parseValueExpression)
		p.matchRParen(nil)
		return p.expression(exp.Tuple(exp.Args{"expressions": expressions}), nil, nil)
	}
	// In some dialects VALUES 1, 2 results in 1 column & 2 rows (parser.py:3794): the
	// no-parens branch stays plain _parse_expression (no SUPPORTS_VALUES_DEFAULT).
	expression := p.parseExpression()
	if expression != nil {
		return p.expression(exp.Tuple(exp.Args{"expressions": []exp.Expression{expression}}), nil, nil)
	}
	return nil
}

// parseValueExpression ports _parse_value's local _parse_value_expression closure
// (parser.py:3784-3787): MySQL/Postgres (SUPPORTS_VALUES_DEFAULT=True, the base default;
// dialect.py:670) accept `VALUES (DEFAULT)`, parsed as exp.var("DEFAULT").
func (p *Parser) parseValueExpression() exp.Expression {
	if p.dialect.SupportsValuesDefault && p.match(tokens.DEFAULT) {
		return exp.Var(exp.Args{"this": stringsUpper(p.prev.Text)})
	}
	return p.parseExpression()
}

func (p *Parser) parseWrappedSelect(table bool) exp.Expression {
	var this exp.Expression
	if table {
		this = p.parseTable(false, false, nil, false, false, false, true)
	} else {
		this = p.parseSelect(true, false, true, false)
	}
	// _parse_wrapped_select (parser.py:3828-3832): a bare VALUES(...) parsed as a table
	// source gets rewrapped as an aliased exp.Table right before parseQueryModifiers, so
	// e.g. a following JOIN can attach to it (`((VALUES (1)) AS v(id) LEFT JOIN t ON ...)`).
	if table && this != nil && this.Kind() == exp.KindValues {
		if aliasExpr, ok := this.Arg("alias").(exp.Expression); ok && aliasExpr != nil {
			aliasExpr.Pop()
			this = exp.Table(exp.Args{"this": this, "alias": aliasExpr})
		}
	}
	return p.parseQueryModifiers(p.parseSetOperations(this))
}

func (p *Parser) parseSubquery(this exp.Expression, parseAlias bool) exp.Expression {
	if this == nil {
		return nil
	}
	args := exp.Args{"this": this}
	if parseAlias {
		args["alias"] = p.parseTableAlias(nil)
	}
	// _parse_subquery (parser.py:4132-4146) attaches a trailing TABLESAMPLE to the
	// Subquery itself, e.g. `(SELECT ...) [AS x] TABLESAMPLE (100 ROWS)`; subquerySQL
	// already renders this "sample" arg (generator/sql.go:934, afterLimitModifiers).
	if sample := p.parseTableSample(false); sample != nil {
		args["sample"] = sample
	}
	return p.expression(exp.Subquery(args), nil, nil)
}

func (p *Parser) parseFunction(functions map[string]func([]exp.Expression) exp.Expression, anonymous bool, optionalParens bool, anyToken bool) exp.Expression {
	// TODO(1d): parse ODBC {fn ...} wrapper syntax.
	return p.parseFunctionCall(functions, anonymous, optionalParens, anyToken)
}

func (p *Parser) parseFunctionCall(functions map[string]func([]exp.Expression) exp.Expression, anonymous bool, optionalParens bool, anyToken bool) exp.Expression {
	if !p.curr.IsValid() {
		return nil
	}
	comments := p.curr.Comments
	prev := p.prev
	token := p.curr
	tokenType := token.TokenType
	this := token.Text
	upper := stringsUpper(token.Text)
	afterDot := prev.TokenType == tokens.DOT
	if parser := p.noParenFunctionParserFor(upper); optionalParens && parser != nil && tokenType != tokens.IDENTIFIER && tokenType != tokens.STRING && !afterDot {
		p.advance()
		return p.parseWindow(parser(p), false)
	}
	if p.next.TokenType != tokens.L_PAREN {
		// NO_PAREN_FUNCTIONS (parser.py:6973-6975): a bare CURRENT_DATE/CURRENT_TIME keyword
		// (no trailing "(", not after a DOT) builds the corresponding zero-arg node, e.g.
		// postgres `date_add(current_date, ...)` -> CurrentDate rendering CURRENT_DATE. Only
		// shared tokens whose Kinds this port models are wired in noParenFunctions, with
		// dialect-local additions resolved by noParenFunctionFor. CURRENT_TIMESTAMP/USER/ROLE
		// keep falling through to a bare column, which already round-trips for base/mysql/postgres.
		if optionalParens && !afterDot {
			if build := p.noParenFunctionFor(tokenType); build != nil {
				p.advance()
				return p.expression(build(exp.Args{}), &token, comments)
			}
		}
		return nil
	}
	if anyToken {
		if reservedTokens[tokenType] {
			return nil
		}
		// MySQL FUNC_TOKENS += TokenType.VALUES (parsers/mysql.py:63-70): see
		// dialects.Dialect.ValuesIsFunction. MySQL FUNC_TOKENS also += DATABASE/MOD/SCHEMA
		// (parsers/mysql.py:63-70): rather than a real per-dialect FUNC_TOKENS table (deferred
		// to slice 5b, see dialects.Dialect.ValuesIsFunction's doc), a name registered in
		// exp.FunctionByName or the dialect's own Functions overlay is itself sufficient
		// evidence that this token should be treated as a function-call start, so it's checked
		// here directly instead of adding three more single-purpose bool flags. This only ever
		// affects tokens whose text is ALSO a keyword in this dialect's tokenizer (e.g. mysql's
		// MOD/DATABASE/SCHEMA) - everywhere else the token is already tokens.VAR and funcTokens
		// (which includes VAR) already covers it, so this is a no-op for the vast majority of
		// FunctionByName/Functions entries.
	} else if !funcTokens[tokenType] && !(tokenType == tokens.VALUES && p.dialect.ValuesIsFunction) &&
		!(tokenType == tokens.CHARACTER_SET && p.dialect.CharsetIsFunction) &&
		p.dialect.Functions[upper] == nil && exp.FunctionByName[upper] == nil {
		return nil
	}
	p.advance(2)
	var result exp.Expression
	parser := p.functionParser(upper)
	if parser != nil && !anonymous {
		result = parser(p)
	} else {
		if subqueryPredicate := subqueryPredicates[tokenType]; subqueryPredicate != nil {
			var expr exp.Expression
			if subqueryTokens[p.curr.TokenType] {
				expr = p.parseSelect(false, false, true, true)
				p.matchRParen(expr)
			} else if prev.TokenType == tokens.LIKE || prev.TokenType == tokens.ILIKE {
				p.advance(-1)
				expr = p.parseBitwise()
			}
			if expr != nil {
				return p.expression(subqueryPredicate(exp.Args{"this": expr}), nil, comments)
			}
		}
		if functions == nil {
			functions = mergedDialectFunctions(p.dialect)
		}
		function := functions[upper]
		knownFunction := function != nil && !anonymous
		alias := !knownFunction
		args := p.parseFunctionArgs(alias)
		if knownFunction {
			result = p.validateExpression(function(args), exprArgs(args))
		} else {
			var name any = this
			if tokenType == tokens.IDENTIFIER {
				name = exp.Identifier(exp.Args{"this": this, "quoted": true})
			}
			result = p.expression(exp.Anonymous(exp.Args{"this": name, "expressions": args}), nil, nil)
		}
	}
	if result != nil && len(comments) > 0 {
		result.AddComments(comments, false)
	}
	p.matchRParen(result)
	return p.parseWindow(result, false)
}

func (p *Parser) parseFunctionArgs(alias bool) []exp.Expression {
	return p.parseCsv(func() exp.Expression { return p.parseLambda(alias) })
}

// parseLambda ports _parse_lambda (parser.py:7181-7226). It is the per-argument parser
// used by parseFunctionArgs, so `x -> body` / `(a, b) -> body` are recognized as an
// exp.Lambda before falling through to the ordinary argument grammar below. ARROW stays
// wired into columnOperators for JSON `->` OUTSIDE function-call args (parser.go:1341,
// parseColumnOps) - this lambda handling only runs inside parseFunctionArgs, so a top-level
// `a -> b` (and postgres `x::JSON -> 'd' ->> -1`) is unaffected and keeps parsing as
// JSONExtract.
//
// LAMBDAS here only maps TokenType.ARROW (the base LAMBDAS entry that also matters for us);
// FARROW/Kwarg (named-argument `=>`) isn't ported by this part, so the Go LAMBDAS table
// mirrors only the exp.Lambda-producing half of upstream's map.
func (p *Parser) parseLambda(alias bool) exp.Expression {
	// Fast path: a simple atom (column, literal, null, bool) followed by , or ) is common
	// and never a lambda head, so skip the L_PAREN/single-arg lambda probing below entirely
	// (parser.py:7184-7189).
	if lambdaArgTerminators[p.next.TokenType] {
		if atom := p.parseAtom(); atom != nil {
			return atom
		}
	}

	index := p.index

	if p.match(tokens.L_PAREN) {
		params := p.parseCsv(p.parseLambdaArg)
		if !p.match(tokens.R_PAREN) {
			p.retreat(index)
		} else if builder, ok := lambdas[p.curr.TokenType]; ok {
			p.advance()
			return builder(p, params)
		} else {
			p.retreat(index)
		}
	} else if _, ok := lambdas[p.next.TokenType]; ok {
		// TYPED_LAMBDA_ARGS (Snowflake/Materialize's `x INT -> ...`) is always false for the
		// dialects in this part's scope, so the upstream `self.TYPED_LAMBDA_ARGS or` half of
		// this condition is dropped.
		params := []exp.Expression{p.parseLambdaArg()}
		if builder, ok := lambdas[p.curr.TokenType]; ok {
			p.advance()
			return builder(p, params)
		}
		p.retreat(index)
	}

	var this exp.Expression
	if p.match(tokens.DISTINCT) {
		this = p.expression(exp.Distinct(exp.Args{"expressions": p.parseCsv(p.parseDisjunction)}), nil, nil)
	} else {
		p.match(tokens.ALL) // ALL is the default/no-op aggregate modifier (SQL-92).
		this = p.parseSelectOrExpression(alias)
	}

	// Upstream also threads this through _parse_having_max (BigQuery's ARRAY_AGG(x HAVING
	// MAX y)) between the two _parse_respect_or_ignore_nulls calls; that's out of this
	// part's scope (no exp.HavingMax Kind ported), so it's omitted here - a no-op for every
	// dialect that doesn't parse a HAVING MAX/MIN clause at this position anyway.
	this = p.parseRespectOrIgnoreNulls(this)
	this = p.parseOrder(this, false)
	this = p.parseRespectOrIgnoreNulls(this)
	return p.parseLimit(this, false, false)
}

func (p *Parser) parseSelectOrExpression(alias bool) exp.Expression {
	var expression exp.Expression
	if alias {
		expression = p.parseAlias(p.parseAssignment(), true)
	} else {
		expression = p.parseAssignment()
	}
	if expression = p.parseSetOperations(expression); expression != nil {
		return expression
	}
	return p.parseSelect(false, false, true, true)
}

func (p *Parser) parseCase() exp.Expression {
	if p.match(tokens.DOT, false) {
		p.retreat(p.index - 1)
		return nil
	}
	ifs := []exp.Expression{}
	comments := p.prevComments
	expression := p.parseDisjunction()
	var defaultExpr exp.Expression
	for p.match(tokens.WHEN) {
		condition := p.parseDisjunction()
		p.match(tokens.THEN)
		then := p.parseDisjunction()
		ifs = append(ifs, p.expression(exp.If(exp.Args{"this": condition, "true": then}), nil, nil))
	}
	if p.match(tokens.ELSE) {
		defaultExpr = p.parseDisjunction()
	}
	if !p.match(tokens.END) {
		// Reference repair (parser.py:7763-7770): `ELSE interval END` misparses as an
		// exp.Interval whose "this" swallowed the closing END as a bare-identifier unit
		// (e.g. "CASE WHEN TRUE THEN 1 ELSE interval END" has no token left for the real
		// END to match against). If that's what happened, unwind it back into the column
		// reference `interval` instead of raising.
		fixed := false
		if defaultExpr != nil && defaultExpr.Kind() == exp.KindInterval {
			if inner, _ := defaultExpr.Arg("this").(exp.Expression); inner != nil {
				if sql, err := inner.SQL(exp.GenerateOptions{}); err == nil && stringsUpper(sql) == "END" {
					defaultExpr = exp.Column(exp.Args{"this": exp.ToIdentifier("interval")})
					fixed = true
				}
			}
		}
		if !fixed {
			p.raiseError("Expected END after CASE", p.prev)
		}
	}
	return p.expression(exp.Case(exp.Args{"this": expression, "ifs": ifs, "default": defaultExpr}), nil, comments)
}

func (p *Parser) parseIf() exp.Expression {
	if p.match(tokens.L_PAREN) {
		args := p.parseCsv(func() exp.Expression { return p.parseAlias(p.parseAssignment(), true) })
		this := p.validateExpression(exp.FromArgList(exp.KindIf, args), exprArgs(args))
		p.matchRParen(this)
		return this
	}
	index := p.index - 1
	condition := p.parseDisjunction()
	if condition == nil {
		p.retreat(index)
		return nil
	}
	p.match(tokens.THEN)
	trueExpr := p.parseDisjunction()
	var falseExpr exp.Expression
	if p.match(tokens.ELSE) {
		falseExpr = p.parseDisjunction()
	}
	p.match(tokens.END)
	return p.expression(exp.If(exp.Args{"this": condition, "true": trueExpr, "false": falseExpr}), nil, nil)
}

func (p *Parser) parseGroup(skipGroupByToken bool) exp.Expression {
	if !skipGroupByToken && !p.match(tokens.GROUP_BY) {
		return nil
	}
	comments := p.prevComments
	elements := exp.Args{"expressions": []exp.Expression{}, "grouping_sets": []exp.Expression{}, "cube": []exp.Expression{}, "rollup": []exp.Expression{}}
	if p.match(tokens.ALL) {
		elements["all"] = true
	} else if p.match(tokens.DISTINCT) {
		elements["all"] = false
	}
	if queryModifierTokens[p.curr.TokenType] {
		return p.expression(exp.Group(elements), nil, comments)
	}
	for {
		index := p.index
		expressions := p.parseCsv(func() exp.Expression {
			if p.matchSet(map[tokens.TokenType]bool{tokens.CUBE: true, tokens.ROLLUP: true}, false) {
				return nil
			}
			return p.parseDisjunction()
		})
		elements["expressions"] = append(elements["expressions"].([]exp.Expression), expressions...)
		beforeWithIndex := p.index
		withPrefix := p.match(tokens.WITH)
		if cubeOrRollup := p.parseCubeOrRollup(withPrefix); cubeOrRollup != nil {
			key := "cube"
			if cubeOrRollup.Kind() == exp.KindRollup {
				key = "rollup"
			}
			elements[key] = append(elements[key].([]exp.Expression), cubeOrRollup)
		} else if groupingSets := p.parseGroupingSets(); groupingSets != nil {
			elements["grouping_sets"] = append(elements["grouping_sets"].([]exp.Expression), groupingSets)
		} else if p.matchTextSeq("TOTALS") {
			elements["totals"] = true
		}
		if beforeWithIndex <= p.index && p.index <= beforeWithIndex+1 {
			p.retreat(beforeWithIndex)
			break
		}
		if index == p.index {
			break
		}
	}
	return p.expression(exp.Group(elements), nil, comments)
}

func (p *Parser) parseCubeOrRollup(withPrefix bool) exp.Expression {
	var constructor func(exp.Args) exp.Expression
	if p.match(tokens.CUBE) {
		constructor = exp.Cube
	} else if p.match(tokens.ROLLUP) {
		constructor = exp.Rollup
	} else {
		return nil
	}
	expressions := []exp.Expression{}
	if !withPrefix {
		expressions = p.parseWrappedCsv(p.parseBitwise)
	}
	return p.expression(constructor(exp.Args{"expressions": expressions}), nil, nil)
}

func (p *Parser) parseGroupingSets() exp.Expression {
	if p.match(tokens.GROUPING_SETS) {
		return p.expression(exp.GroupingSets(exp.Args{"expressions": p.parseWrappedCsv(p.parseGroupingSet)}), nil, nil)
	}
	return nil
}

func (p *Parser) parseGroupingSet() exp.Expression {
	if groupingSets := p.parseGroupingSets(); groupingSets != nil {
		return groupingSets
	}
	if cubeOrRollup := p.parseCubeOrRollup(false); cubeOrRollup != nil {
		return cubeOrRollup
	}
	return p.parseBitwise()
}

func (p *Parser) parseHaving(skipHavingToken bool) exp.Expression {
	if !skipHavingToken && !p.match(tokens.HAVING) {
		return nil
	}
	comments := p.prevComments
	return p.expression(exp.Having(exp.Args{"this": p.parseDisjunction()}), nil, comments)
}

func (p *Parser) parseQualify() exp.Expression {
	if !p.match(tokens.QUALIFY) {
		return nil
	}
	return p.expression(exp.Qualify(exp.Args{"this": p.parseDisjunction()}), nil, nil)
}

func (p *Parser) parseOrder(this exp.Expression, skipOrderToken bool) exp.Expression {
	var siblings any
	if !skipOrderToken && !p.match(tokens.ORDER_BY) {
		if !p.match(tokens.ORDER_SIBLINGS_BY) {
			return this
		}
		siblings = true
	}
	comments := p.prevComments
	return p.expression(exp.Order(exp.Args{"this": this, "expressions": p.parseCsv(func() exp.Expression { return p.parseOrdered(nil) }), "siblings": siblings}), nil, comments)
}

func (p *Parser) parseOrdered(parseMethod func() exp.Expression) exp.Expression {
	var this exp.Expression
	if parseMethod != nil {
		this = parseMethod()
	} else {
		this = p.parseDisjunction()
	}
	if this == nil {
		return nil
	}
	if stringsUpper(this.Name()) == "ALL" && p.dialect.SupportsOrderByAll {
		this = exp.Var(exp.Args{"this": "ALL"})
	}
	asc := p.match(tokens.ASC)
	var desc any
	if p.match(tokens.DESC) {
		desc = true
	} else if asc {
		desc = false
	}
	isNullsFirst := p.matchTextSeq("NULLS", "FIRST")
	isNullsLast := p.matchTextSeq("NULLS", "LAST")
	nullsFirst := isNullsFirst || false
	explicitlyNullOrdered := isNullsFirst || isNullsLast
	descBool, descSet := desc.(bool)
	if !explicitlyNullOrdered && ((!descSet || !descBool) && p.dialect.NullOrdering == "nulls_are_small" || (descSet && descBool) && p.dialect.NullOrdering != "nulls_are_small") && p.dialect.NullOrdering != "nulls_are_last" {
		nullsFirst = true
	}
	// TODO(1d): WITH FILL.
	return p.expression(exp.Ordered(exp.Args{"this": this, "desc": desc, "nulls_first": nullsFirst}), nil, nil)
}

func (p *Parser) parseLimit(this exp.Expression, top bool, skipLimitToken bool) exp.Expression {
	limitToken := tokens.LIMIT
	if top {
		limitToken = tokens.TOP
	}
	if skipLimitToken || p.match(limitToken) {
		comments := p.prevComments
		// LIMIT ALL is a no-op that leaves the query unchanged (parser.py:5569-5570).
		if p.dialect.SupportsLimitAll && p.match(tokens.ALL) {
			return this
		}
		// Parsing LIMIT x% (i.e. x PERCENT) as a term leads to an error, since we try to
		// build an exp.Mod expr. For that matter, we backtrack and instead consume the
		// factor plus parse the percentage separately.
		var expression exp.Expression
		index := p.index
		expression = p.tryParse(p.parseTerm, false)
		if expression != nil && expression.Kind() == exp.KindMod {
			p.retreat(index)
			expression = p.parseFactor()
		} else if expression == nil {
			expression = p.parseFactor()
		}
		limitOptions := p.parseLimitOptions()
		var offset exp.Expression
		if p.match(tokens.COMMA) {
			offset = expression
			expression = p.parseTerm()
		}
		return p.expression(exp.Limit(exp.Args{"this": this, "expression": expression, "offset": offset, "limit_options": limitOptions, "expressions": p.parseLimitBy()}), nil, comments)
	}
	if p.match(tokens.FETCH) {
		direction := "FIRST"
		if p.matchSet(map[tokens.TokenType]bool{tokens.FIRST: true, tokens.NEXT: true}) {
			direction = stringsUpper(p.prev.Text)
		}
		count := p.parseField(false, fetchTokens, false)
		return p.expression(exp.Fetch(exp.Args{"direction": direction, "count": count, "limit_options": p.parseLimitOptions()}), nil, nil)
	}
	return this
}

func (p *Parser) parseLimitOptions() exp.Expression {
	percent := p.matchSet(map[tokens.TokenType]bool{tokens.PERCENT: true, tokens.MOD: true})
	rows := p.matchSet(map[tokens.TokenType]bool{tokens.ROW: true, tokens.ROWS: true})
	p.matchTextSeq("ONLY")
	withTies := p.matchTextSeq("WITH", "TIES")
	if !(percent || rows || withTies) {
		return nil
	}
	return p.expression(exp.LimitOptions(exp.Args{"percent": percent, "rows": rows, "with_ties": withTies}), nil, nil)
}

func (p *Parser) parseLimitBy() []exp.Expression {
	if p.matchTextSeq("BY") {
		return p.parseCsv(p.parseBitwise)
	}
	return nil
}

func (p *Parser) parseOffset(this exp.Expression) exp.Expression {
	if !p.match(tokens.OFFSET) {
		return this
	}
	count := p.parseTerm()
	p.matchSet(map[tokens.TokenType]bool{tokens.ROW: true, tokens.ROWS: true})
	return p.expression(exp.Offset(exp.Args{"this": this, "expression": count, "expressions": p.parseLimitBy()}), nil, nil)
}

func (p *Parser) canParseLimitOrOffset() bool {
	if !p.matchSet(ambiguousAliasTokens, false) {
		return false
	}
	index := p.index
	result := p.tryParse(func() exp.Expression { return p.parseLimit(nil, false, false) }, true) != nil || p.tryParse(func() exp.Expression { return p.parseOffset(nil) }, true) != nil
	p.retreat(index)
	if p.next.TokenType == tokens.MATCH_CONDITION {
		result = false
	}
	return result
}

func (p *Parser) canParseNamedWindow() bool {
	if !p.match(tokens.WINDOW, false) {
		return false
	}
	if p.index+3 >= p.tokensSize {
		return false
	}
	name := p.tokens[p.index+1]
	if !idVarTokens[name.TokenType] {
		return false
	}
	aliasTok := p.tokens[p.index+2]
	if aliasTok.TokenType != tokens.ALIAS {
		return false
	}
	body := p.tokens[p.index+3]
	return body.TokenType == tokens.L_PAREN
}

func (p *Parser) parseWindowClause() []exp.Expression {
	if p.match(tokens.WINDOW) {
		return p.parseCsv(p.parseNamedWindow)
	}
	return nil
}

func (p *Parser) parseNamedWindow() exp.Expression {
	return p.parseWindow(p.parseIdVar(false, nil), true)
}

func (p *Parser) parseWindow(this exp.Expression, alias bool) exp.Expression {
	funcExpr := this
	var comments []string
	if funcExpr != nil {
		comments = funcExpr.Comments()
	}
	if p.matchTextSeq("WITHIN", "GROUP") {
		order := p.parseWrapped(func() exp.Expression { return p.parseOrder(nil, false) }, false)
		this = p.expression(exp.WithinGroup(exp.Args{"this": this, "expression": order}), nil, nil)
	}
	if p.matchPair(tokens.FILTER, tokens.L_PAREN, true) {
		p.match(tokens.WHERE)
		this = p.expression(exp.Filter(exp.Args{"this": this, "expression": p.parseWhere(true)}), nil, nil)
		p.matchRParen(this)
	}
	this = p.parseRespectOrIgnoreNulls(this)
	var over any
	if alias {
		p.match(tokens.ALIAS)
	} else if !p.match(tokens.OVER) {
		return this
	} else {
		over = stringsUpper(p.prev.Text)
	}
	if funcExpr != nil && len(comments) > 0 {
		funcExpr.PopComments()
	}
	if !p.match(tokens.L_PAREN) {
		return p.expression(exp.Window(exp.Args{"this": this, "alias": p.parseIdVar(false, nil), "over": over}), nil, comments)
	}
	windowAlias := p.parseIdVar(false, windowAliasTokens)
	var first any
	if p.match(tokens.FIRST) {
		first = true
	}
	if p.matchTextSeq("LAST") {
		first = false
	}
	partition, order := p.parsePartitionAndOrder()
	var kind any
	if p.matchSet(map[tokens.TokenType]bool{tokens.ROWS: true, tokens.RANGE: true}) || p.matchTextSeq("GROUPS") {
		kind = p.prev.Text
	}
	var spec exp.Expression
	if kind != nil {
		p.match(tokens.BETWEEN)
		start := p.parseWindowSpec()
		end := map[string]any{}
		if p.match(tokens.AND) {
			end = p.parseWindowSpec()
		}
		var exclude exp.Expression
		if p.matchTextSeq("EXCLUDE") {
			// upstream _parse_window uses the default raise_unmatched=True (parser.py:8405),
			// so a malformed EXCLUDE option errors rather than silently retreating.
			exclude = p.parseVarFromOptions(windowExcludeOptions, true)
		}
		spec = p.expression(exp.WindowSpec(exp.Args{"kind": kind, "start": start["value"], "start_side": start["side"], "end": end["value"], "end_side": end["side"], "exclude": exclude}), nil, nil)
	}
	p.matchRParen(this)
	window := p.expression(exp.Window(exp.Args{"this": this, "partition_by": partition, "order": order, "spec": spec, "alias": windowAlias, "over": over, "first": first}), nil, comments)
	if p.matchSet(map[tokens.TokenType]bool{tokens.OVER: true}, false) {
		return p.parseWindow(window, alias)
	}
	return window
}

func (p *Parser) parseRespectOrIgnoreNulls(this exp.Expression) exp.Expression {
	if p.curr.TokenType == tokens.VAR {
		if p.matchTextSeq("IGNORE", "NULLS") {
			return p.expression(exp.IgnoreNulls(exp.Args{"this": this}), nil, nil)
		}
		if p.matchTextSeq("RESPECT", "NULLS") {
			return p.expression(exp.RespectNulls(exp.Args{"this": this}), nil, nil)
		}
	}
	return this
}

func (p *Parser) parsePartitionAndOrder() ([]exp.Expression, exp.Expression) {
	return p.parsePartitionBy(), p.parseOrder(nil, false)
}

func (p *Parser) parsePartitionBy() []exp.Expression {
	if p.match(tokens.PARTITION_BY) {
		return p.parseCsv(p.parseDisjunction)
	}
	return []exp.Expression{}
}

func (p *Parser) parseWindowSpec() map[string]any {
	p.match(tokens.BETWEEN)
	var value any
	if p.matchTextSeq("UNBOUNDED") {
		value = "UNBOUNDED"
	} else if p.matchTextSeq("CURRENT", "ROW") {
		value = "CURRENT ROW"
	} else {
		value = p.parseBitwise()
	}
	var side any
	if p.matchTexts(windowSides) {
		side = p.prev.Text
	}
	return map[string]any{"value": value, "side": side}
}

var noParenFunctionParsers map[string]func(*Parser) exp.Expression

// noParenFunctions ports NO_PAREN_FUNCTIONS (parser.py:431-439): keyword tokens that, when
// not followed by "(", build a zero-arg function node. Only the subset whose Kinds this port
// models is included - CurrentDate (CURRENT_DATE/CURRENT_DATETIME) and CurrentTime
// (CURRENT_TIME); CURRENT_TIMESTAMP/CURRENT_USER/CURRENT_ROLE are omitted (no
// CurrentTimestamp/CurrentUser/CurrentRole Kind yet), still parsing as bare columns which
// round-trip unchanged for base/mysql/postgres. currentDateSQL renders CurrentDate without the
// empty parens the generic fallback would emit; CurrentTime keeps the parenthesized fallback,
// matching upstream (currentdate_sql exists, currenttime_sql does not).
var noParenFunctions = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.CURRENT_DATE:     exp.CurrentDate,
	tokens.CURRENT_DATETIME: exp.CurrentDate,
	tokens.CURRENT_TIME:     exp.CurrentTime,
}

var subqueryPredicates = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.ANY:    exp.Any,
	tokens.ALL:    exp.All,
	tokens.EXISTS: exp.Exists,
	tokens.SOME:   exp.Any,
}

var functionParsers = map[string]func(*Parser) exp.Expression{}

// statementParsers is declared with an empty-map var initializer (rather than left nil and
// only ever assigned inside init()) so it's guaranteed to be a valid, non-nil map as soon
// as package-level var initialization completes and BEFORE any init() func runs (Go var
// initializers always precede init() funcs, regardless of file). That lets each statement-
// family part register its own entries from its own func init() via plain key assignment,
// safely, regardless of init() run order across files (plan's "seam" for parallel,
// disjoint-file family parts). Embedding the entries directly in this initializer instead
// (a map literal with (*Parser).parseInsert etc. as values) is not viable: those methods
// transitively call parseStatement, which reads statementParsers, and Go's package
// dependency analysis flags that as an initialization cycle - so, like the pre-existing
// functionParsers below, the entries are populated by assignment in init() instead.
var statementParsers = map[tokens.TokenType]func(*Parser) exp.Expression{}

var queryModifierParsers map[tokens.TokenType]func(*Parser) (string, any)

func init() {
	statementParsers[tokens.INSERT] = (*Parser).parseInsert
	statementParsers[tokens.UPDATE] = (*Parser).parseUpdate
	statementParsers[tokens.DELETE] = (*Parser).parseDelete
	statementParsers[tokens.MERGE] = (*Parser).parseMerge
	statementParsers[tokens.CREATE] = (*Parser).parseCreate
	// Upstream base STATEMENT_PARSERS has no TokenType.REPLACE entry (parser.py:1081-1111):
	// CREATE consumes its own OR REPLACE modifier, while statement-leading REPLACE(...) is
	// an ordinary function expression. The mysql-replace extension registers a dialect
	// statement override instead; it retreats on `(` and structures only supported MySQL
	// statement forms now that dialects/mysql.py:154's tokenizer Command behavior is removed.
	statementParsers[tokens.PRAGMA] = (*Parser).parsePragma

	noParenFunctionParsers = map[string]func(*Parser) exp.Expression{
		"ANY": func(p *Parser) exp.Expression {
			return p.expression(exp.Any(exp.Args{"this": p.parseBitwise()}), nil, nil)
		},
		"CASE": func(p *Parser) exp.Expression { return p.parseCase() },
		"IF":   func(p *Parser) exp.Expression { return p.parseIf() },
	}
	functionParsers = map[string]func(*Parser) exp.Expression{
		"CAST":        func(p *Parser) exp.Expression { return p.parseCast(p.strictCast, nil) },
		"TRY_CAST":    func(p *Parser) exp.Expression { return p.parseCast(false, true) },
		"SAFE_CAST":   func(p *Parser) exp.Expression { return p.parseCast(false, true) },
		"CONVERT":     func(p *Parser) exp.Expression { return p.parseConvert(p.strictCast, nil) },
		"TRY_CONVERT": func(p *Parser) exp.Expression { return p.parseConvert(false, true) },
		// exp.Chr._sql_names = ["CHR", "CHAR"] (string.py:26): both spellings route to the
		// same special-grammar parser (an optional trailing `USING <charset>` clause), not
		// FunctionByName.
		"CHAR":    func(p *Parser) exp.Expression { return p.parseChar() },
		"CHR":     func(p *Parser) exp.Expression { return p.parseChar() },
		"CEIL":    func(p *Parser) exp.Expression { return p.parseCeilFloor(exp.KindCeil) },
		"FLOOR":   func(p *Parser) exp.Expression { return p.parseCeilFloor(exp.KindFloor) },
		"EXTRACT": func(p *Parser) exp.Expression { return p.parseExtract() },
		// Registered unconditionally in the base singleton, but functionParser exposes it only
		// through the Postgres fallback (parsers/postgres.py:156 FUNCTION_PARSERS). Base/MySQL
		// have no DATE_PART entry, so DATE_PART(...) there parses as a plain Anonymous call.
		"DATE_PART":      func(p *Parser) exp.Expression { return p.parseDatePart() },
		"POSITION":       func(p *Parser) exp.Expression { return p.parsePosition() },
		"SUBSTRING":      func(p *Parser) exp.Expression { return p.parseSubstring() },
		"TRIM":           func(p *Parser) exp.Expression { return p.parseTrim() },
		"STRING_AGG":     func(p *Parser) exp.Expression { return p.parseStringAgg() },
		"JSON_TABLE":     func(p *Parser) exp.Expression { return p.parseJSONTable() },
		"XMLELEMENT":     func(p *Parser) exp.Expression { return p.parseXMLElement() },
		"XMLTABLE":       func(p *Parser) exp.Expression { return p.parseXMLTable() },
		"JSON_OBJECT":    func(p *Parser) exp.Expression { return p.parseJSONObject(false) },
		"JSON_OBJECTAGG": func(p *Parser) exp.Expression { return p.parseJSONObject(true) },
		// Registered unconditionally in the base singleton, but functionParser exposes it only
		// through the MySQL fallback: among base/MySQL/Postgres, only MySQL's FUNCTION_PARSERS
		// registers _parse_json_value (parsers/mysql.py:161). Base/Postgres have no JSON_VALUE
		// entry, so JSON_VALUE(...) there parses as a plain Anonymous call.
		"JSON_VALUE": func(p *Parser) exp.Expression { return p.parseJSONValue() },
		// MySQL-only in practice: only reachable when ValuesIsFunction gates TokenType.VALUES
		// into the function-call path (parsers/mysql.py:158-160). `VALUES(col)` inside
		// `ON DUPLICATE KEY UPDATE` refers to the row that would have been inserted.
		"VALUES": func(p *Parser) exp.Expression {
			return p.expression(exp.Anonymous(exp.Args{"this": "VALUES", "expressions": []exp.Expression{p.parseIdVar(true, nil)}}), nil, nil)
		},
	}
	queryModifierParsers = map[tokens.TokenType]func(*Parser) (string, any){
		tokens.WHERE:    func(p *Parser) (string, any) { return "where", p.parseWhere(false) },
		tokens.GROUP_BY: func(p *Parser) (string, any) { return "group", p.parseGroup(false) },
		tokens.HAVING:   func(p *Parser) (string, any) { return "having", p.parseHaving(false) },
		tokens.QUALIFY:  func(p *Parser) (string, any) { return "qualify", p.parseQualify() },
		tokens.WINDOW:   func(p *Parser) (string, any) { return "windows", p.parseWindowClause() },
		tokens.ORDER_BY: func(p *Parser) (string, any) { return "order", p.parseOrder(nil, false) },
		tokens.LIMIT:    func(p *Parser) (string, any) { return "limit", p.parseLimit(nil, false, false) },
		tokens.FETCH:    func(p *Parser) (string, any) { return "limit", p.parseLimit(nil, false, false) },
		tokens.OFFSET:   func(p *Parser) (string, any) { return "offset", p.parseOffset(nil) },
		tokens.FOR: func(p *Parser) (string, any) {
			locks := p.parseLocks()
			if len(locks) == 0 {
				return "locks", nil
			}
			return "locks", locks
		},
		tokens.LOCK: func(p *Parser) (string, any) {
			locks := p.parseLocks()
			if len(locks) == 0 {
				return "locks", nil
			}
			return "locks", locks
		},
		tokens.PREWHERE:   func(p *Parser) (string, any) { return "prewhere", p.parsePrewhere() },
		tokens.CLUSTER_BY: func(p *Parser) (string, any) { return "cluster", p.parseCluster() },
		tokens.DISTRIBUTE_BY: func(p *Parser) (string, any) {
			return "distribute", p.parseSort(exp.KindDistribute, tokens.DISTRIBUTE_BY)
		},
		tokens.SORT_BY: func(p *Parser) (string, any) { return "sort", p.parseSort(exp.KindSort, tokens.SORT_BY) },
	}
}

func (p *Parser) matchLParen(expression exp.Expression) {
	if !p.match(tokens.L_PAREN) {
		p.raiseError("Expecting (")
	}
}

// matchRParen ports _match_r_paren (parser.py:9432-9434), which itself is
// `_match(R_PAREN, expression=expression)`: on a successful match, a comment trailing the
// just-consumed ")" on the same line (e.g. `CAST(x AS INT) /* c */`) is attached directly to
// the caller-supplied expression - typically the node the parens just closed - instead of
// lingering in p.prevComments to bubble up and land on whatever outer node happens to call
// p.expression(...) next (e.g. the enclosing Select). expression may be nil, matching every
// upstream call site that passes none.
func (p *Parser) matchRParen(expression exp.Expression) {
	if !p.match(tokens.R_PAREN) {
		p.raiseError("Expecting )")
		return
	}
	p.addComments(expression)
}

func isSetOperation(k exp.Kind) bool {
	return k == exp.KindUnion || k == exp.KindExcept || k == exp.KindIntersect
}

func isUnwrappedQuery(expression exp.Expression) bool {
	return expression != nil && (expression.Kind() == exp.KindSelect || isSetOperation(expression.Kind()))
}

func asExpr(value any) exp.Expression {
	if expression, ok := value.(exp.Expression); ok {
		return expression
	}
	return nil
}

func firstExpression(expressions ...exp.Expression) exp.Expression {
	for _, expression := range expressions {
		if expression != nil {
			return expression
		}
	}
	return nil
}

func tokensOrDefault(value map[tokens.TokenType]bool, defaultValue map[tokens.TokenType]bool) map[tokens.TokenType]bool {
	if value != nil {
		return value
	}
	return defaultValue
}

func stringsUpper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - ('a' - 'A')
		}
	}
	return string(out)
}
