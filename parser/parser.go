package parser

import (
	"fmt"

	"github.com/sjincho/sqlglot-go/dialects"
	sqlerrors "github.com/sjincho/sqlglot-go/errors"
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

type Parser struct {
	errorLevel          sqlerrors.ErrorLevel
	errorMessageContext int
	maxErrors           int
	maxNodes            int
	dialect             *dialects.Dialect
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
	if d == nil {
		d = dialects.Base()
	}
	p := &Parser{
		errorLevel:          level,
		errorMessageContext: 100,
		maxErrors:           3,
		maxNodes:            -1,
		dialect:             d,
		strictCast:          true,
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
	var method func() exp.Expression
	switch into {
	case exp.KindDataType:
		method = func() exp.Expression { return p.parseTypes(false, true, false, false) }
	case exp.KindTable:
		method = func() exp.Expression { return p.parseTableParts(false, false, false, false) }
	case exp.KindIdentifier:
		method = func() exp.Expression { return p.parseIdVar(true, nil) }
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
		expressions = append(expressions, parseMethod())
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
	if parse := statementParsers[p.curr.TokenType]; parse != nil {
		p.advance()
		comments := p.prevComments
		stmt := parse(p)
		if stmt != nil {
			stmt.AddComments(comments, true)
		}
		return stmt
	}
	expression := p.parseExpression()
	if expression != nil {
		expression = p.parseSetOperations(expression)
	} else {
		expression = p.parseSelect(false, false, true, true)
	}
	return p.parseQueryModifiers(expression)
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
		// SELECT TOP is dead for M1 because TOP is not a base tokenizer keyword,
		// but wire the parser shape to match upstream.
		top := p.parseLimit(nil, true, false)
		projections := p.parseExpressions()
		selectArgs := exp.Args{"distinct": distinct, "expressions": projections}
		if top != nil {
			selectArgs["limit"] = top
		}
		this = p.expression(exp.Select(selectArgs), nil, nil)
		if len(comments) > 0 {
			this.AddComments(comments, true)
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
	}
	return this
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
	this := p.parseTableParts(schema, isDBReference, false, false)
	if this == nil {
		return nil
	}
	if schema {
		return p.parseSchema(this)
	}
	if alias := p.parseTableAlias(aliasTokens); alias != nil {
		this.Set("alias", alias)
	}
	if this.Arg("pivots") == nil {
		if pivots := p.parsePivots(); pivots != nil {
			this.Set("pivots", pivots)
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
	return this
}

func (p *Parser) parseTableParts(schema bool, isDBReference bool, wildcard bool, fast bool) exp.Expression {
	var catalog exp.Expression
	var db exp.Expression
	table := p.parseTablePart(schema)
	for p.match(tokens.DOT) {
		if catalog != nil {
			table = p.expression(exp.Dot(exp.Args{"this": table, "expression": p.parseTablePart(schema)}), nil, nil)
		} else {
			catalog = db
			db = table
			table = p.parseTablePart(schema)
			if table == nil {
				table = exp.Identifier(exp.Args{"this": "", "quoted": false})
			}
		}
	}
	if isDBReference {
		catalog = db
		db = table
		table = nil
	}
	if table == nil && !isDBReference {
		p.raiseError(fmt.Sprintf("Expected table name but got %s", p.curr.String()))
	}
	if db == nil && isDBReference {
		p.raiseError(fmt.Sprintf("Expected database name but got %s", p.curr.String()))
	}
	return p.expression(exp.Table(exp.Args{"this": table, "db": db, "catalog": catalog}), nil, nil)
}

func (p *Parser) parseTablePart(schema bool) exp.Expression {
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
	if p.curr.TokenType == tokens.PIVOT || p.curr.TokenType == tokens.UNPIVOT {
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
		columns = p.parseCsv(func() exp.Expression { return p.parseIdVar(false, nil) })
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
		if table != nil {
			return p.expression(exp.Join(exp.Args{"this": table}), nil, nil)
		}
		return nil
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
	if p.match(tokens.ON) {
		kwargs["on"] = p.parseDisjunction()
	} else if p.match(tokens.USING) {
		kwargs["using"] = p.parseUsingIdentifiers()
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

func (p *Parser) parseAssignment() exp.Expression { return p.parseDisjunction() }

func (p *Parser) parseDisjunction() exp.Expression {
	this := p.parseConjunction()
	for p.match(tokens.OR) {
		comments := p.prevComments
		this = p.expression(exp.Or(exp.Args{"this": this, "expression": p.parseConjunction()}), nil, comments)
	}
	return this
}

func (p *Parser) parseConjunction() exp.Expression {
	this := p.parseEquality()
	for p.match(tokens.AND) {
		comments := p.prevComments
		this = p.expression(exp.And(exp.Args{"this": this, "expression": p.parseEquality()}), nil, comments)
	}
	return this
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
		this = p.expression(exp.Like(args), nil, nil)
		negate = false
	case tokens.ILIKE:
		p.advance()
		args := exp.Args{"this": this, "expression": p.parseBitwise()}
		if negate {
			args["negate"] = true
		}
		this = p.expression(exp.ILike(args), nil, nil)
		negate = false
	case tokens.ISNULL:
		p.advance()
		this = p.expression(exp.Is(exp.Args{"this": this, "expression": exp.Null()}), nil, nil)
	}
	if negate && p.match(tokens.NULL) {
		this = p.expression(exp.Is(exp.Args{"this": this, "expression": exp.Null()}), nil, nil)
		negate = false
	}
	if p.match(tokens.IS) {
		this = p.parseIs(this)
	}
	if negate && this != nil {
		this = p.expression(exp.Not(exp.Args{"this": this}), nil, nil)
	}
	return this
}

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
	expression := p.parseNull()
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

func (p *Parser) parseBitwise() exp.Expression {
	this := p.parseTerm()
	for {
		if constructor, ok := bitwiseTokens[p.curr.TokenType]; ok {
			p.advance()
			this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseTerm()}), nil, nil)
		} else if p.dialect.DPipeIsStringConcat && p.match(tokens.DPIPE) {
			this = p.expression(exp.DPipe(exp.Args{"this": this, "expression": p.parseTerm(), "safe": !p.dialect.StrictStringConcat}), nil, nil)
		} else {
			break
		}
	}
	return this
}

var termTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.DASH: exp.Sub,
	tokens.PLUS: exp.Add,
	tokens.MOD:  exp.Mod,
}

func (p *Parser) parseTerm() exp.Expression {
	this := p.parseFactor()
	for constructor, ok := termTokens[p.curr.TokenType]; ok; constructor, ok = termTokens[p.curr.TokenType] {
		p.advance()
		comments := p.prevComments
		this = p.expression(constructor(exp.Args{"this": this, "expression": p.parseFactor()}), nil, comments)
	}
	return this
}

var factorTokens = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.SLASH: exp.Div,
	tokens.STAR:  exp.Mul,
	tokens.DIV:   exp.Div,
}

func (p *Parser) parseFactor() exp.Expression {
	this := p.parseUnary()
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
	return p.parseType()
}

func (p *Parser) parseType() exp.Expression {
	if typed := p.parseTypedStringLiteral(); typed != nil {
		return typed
	}
	if atom := p.parseAtom(); atom != nil {
		return atom
	}
	if interval := p.parseInterval(true); interval != nil {
		return p.parseColumnOps(interval)
	}
	return p.parseColumn()
}

func (p *Parser) parseTypedStringLiteral() exp.Expression {
	switch p.curr.TokenType {
	case tokens.DATE, tokens.DATETIME, tokens.TIMESTAMP, tokens.TIMESTAMPTZ:
		if p.next.TokenType != tokens.STRING {
			return nil
		}
		typeToken := p.curr
		p.advance()
		stringToken := p.curr
		p.advance()
		to, err := exp.DataTypeBuild(stringsUpper(typeToken.Text), "", p.dialect.SupportsUserDefinedTypes, true, nil)
		if err != nil {
			p.raiseError(err.Error(), typeToken)
			return nil
		}
		literal := p.expression(exp.LiteralString(stringToken.Text), &stringToken, nil)
		return p.expression(exp.Cast(exp.Args{"this": literal, "to": to}), &typeToken, nil)
	default:
		return nil
	}
}

func (p *Parser) parseAtom() exp.Expression {
	if identifierTokens[p.curr.TokenType] {
		if column := p.parseColumn(); column != nil {
			return column
		}
	}
	token := p.curr
	switch token.TokenType {
	case tokens.STRING, tokens.NUMBER, tokens.NULL, tokens.TRUE, tokens.FALSE:
		p.advance()
		return p.primaryFromToken(token)
	case tokens.STAR:
		p.advance()
		return p.parseStarOps(token)
	}
	return nil
}

func (p *Parser) parseColumn() exp.Expression {
	column := p.parseColumnPartsFast()
	if column == nil {
		column = p.parseColumnOps(p.parseColumnReference())
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
		if len(parts) == 0 && noParenFunctionParsers[stringsUpper(token.Text)] != nil {
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
		column = exp.Column(exp.Args{"this": parts[2], "table": parts[1], "db": parts[0]})
	default:
		column = exp.Column(exp.Args{"this": parts[3], "table": parts[2], "db": parts[1], "catalog": parts[0]})
		for _, part := range parts[4:] {
			column = exp.Dot(exp.Args{"this": column, "expression": part})
		}
	}
	if len(allComments) > 0 {
		column.AddComments(allComments, false)
	}
	return column
}

func (p *Parser) parseColumnReference() exp.Expression {
	this := p.parseField(false, nil, false)
	if this != nil && this.Kind() == exp.KindIdentifier {
		this = p.expression(exp.Column(exp.Args{"this": this}), nil, this.PopComments())
	}
	return this
}

type columnOpFunc func(p *Parser, this, field exp.Expression) exp.Expression

var columnOperators = map[tokens.TokenType]columnOpFunc{
	tokens.DOT: nil,
	tokens.DCOLON: func(p *Parser, this, to exp.Expression) exp.Expression {
		return p.buildCast(p.strictCast, exp.Args{"this": this, "to": to})
	},
	tokens.DOTCOLON: func(p *Parser, this, to exp.Expression) exp.Expression {
		return p.expression(exp.JSONCast(exp.Args{"this": this, "to": to}), nil, nil)
	},
	// TODO(slice 6): convert -> and ->> RHS expressions to JSONPath.
	tokens.ARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONExtract(exp.Args{"this": this, "expression": path}), nil, nil)
	},
	tokens.DARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONExtractScalar(exp.Args{"this": this, "expression": path}), nil, nil)
	},
	tokens.HASH_ARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONBExtract(exp.Args{"this": this, "expression": path}), nil, nil)
	},
	tokens.DHASH_ARROW: func(p *Parser, this, path exp.Expression) exp.Expression {
		return p.expression(exp.JSONBExtractScalar(exp.Args{"this": this, "expression": path}), nil, nil)
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
			this = p.expression(exp.Column(exp.Args{"this": field, "table": this.This(), "db": this.Arg("table"), "catalog": this.Arg("db")}), nil, this.Comments())
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
	switch token.TokenType {
	case tokens.STRING, tokens.NUMBER, tokens.NULL, tokens.TRUE, tokens.FALSE:
		p.advance()
		return p.primaryFromToken(token)
	case tokens.STAR:
		p.advance()
		return p.parseStarOps(token)
	}
	if p.matchPair(tokens.DOT, tokens.NUMBER, true) {
		return exp.LiteralNumber("0." + p.prev.Text)
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

func (p *Parser) parsePlaceholder() exp.Expression {
	if p.match(tokens.PLACEHOLDER) {
		return p.expression(exp.Placeholder(nil), &p.prev, nil)
	}
	return nil
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
		expressions := p.parseCsv(p.parseExpression)
		p.matchRParen(nil)
		return p.expression(exp.Tuple(exp.Args{"expressions": expressions}), nil, nil)
	}
	expression := p.parseExpression()
	if expression != nil {
		return p.expression(exp.Tuple(exp.Args{"expressions": []exp.Expression{expression}}), nil, nil)
	}
	return nil
}

func (p *Parser) parseWrappedSelect(table bool) exp.Expression {
	var this exp.Expression
	if table {
		this = p.parseTable(false, false, nil, false, false, false, true)
	} else {
		this = p.parseSelect(true, false, true, false)
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
	if parser := noParenFunctionParsers[upper]; optionalParens && parser != nil && tokenType != tokens.IDENTIFIER && tokenType != tokens.STRING && !afterDot {
		p.advance()
		return p.parseWindow(parser(p), false)
	}
	if p.next.TokenType != tokens.L_PAREN {
		// TODO(1d): NO_PAREN_FUNCTIONS CURRENT_*.
		return nil
	}
	if anyToken {
		if reservedTokens[tokenType] {
			return nil
		}
	} else if !funcTokens[tokenType] {
		return nil
	}
	p.advance(2)
	var result exp.Expression
	if parser := functionParsers[upper]; parser != nil && !anonymous {
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
			functions = exp.FunctionByName
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

func (p *Parser) parseLambda(alias bool) exp.Expression {
	if p.match(tokens.DISTINCT) {
		return p.expression(exp.Distinct(exp.Args{"expressions": p.parseCsv(p.parseDisjunction)}), nil, nil)
	}
	p.match(tokens.ALL)
	return p.parseSelectOrExpression(alias)
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
		p.raiseError("Expected END after CASE", p.prev)
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

var subqueryPredicates = map[tokens.TokenType]func(exp.Args) exp.Expression{
	tokens.ANY:    exp.Any,
	tokens.ALL:    exp.All,
	tokens.EXISTS: exp.Exists,
	tokens.SOME:   exp.Any,
}

var functionParsers = map[string]func(*Parser) exp.Expression{}

var statementParsers map[tokens.TokenType]func(*Parser) exp.Expression

var queryModifierParsers map[tokens.TokenType]func(*Parser) (string, any)

func init() {
	noParenFunctionParsers = map[string]func(*Parser) exp.Expression{
		"ANY": func(p *Parser) exp.Expression {
			return p.expression(exp.Any(exp.Args{"this": p.parseBitwise()}), nil, nil)
		},
		"CASE": func(p *Parser) exp.Expression { return p.parseCase() },
		"IF":   func(p *Parser) exp.Expression { return p.parseIf() },
	}
	statementParsers = map[tokens.TokenType]func(*Parser) exp.Expression{
		tokens.INSERT:  (*Parser).parseInsert,
		tokens.UPDATE:  (*Parser).parseUpdate,
		tokens.DELETE:  (*Parser).parseDelete,
		tokens.MERGE:   (*Parser).parseMerge,
		tokens.CREATE:  (*Parser).parseCreate,
		tokens.REPLACE: (*Parser).parseCreate,
	}
	functionParsers = map[string]func(*Parser) exp.Expression{
		"CAST":        func(p *Parser) exp.Expression { return p.parseCast(p.strictCast, nil) },
		"TRY_CAST":    func(p *Parser) exp.Expression { return p.parseCast(false, true) },
		"SAFE_CAST":   func(p *Parser) exp.Expression { return p.parseCast(false, true) },
		"CONVERT":     func(p *Parser) exp.Expression { return p.parseConvert(p.strictCast, nil) },
		"TRY_CONVERT": func(p *Parser) exp.Expression { return p.parseConvert(false, true) },
		"CEIL":        func(p *Parser) exp.Expression { return p.parseCeilFloor(exp.KindCeil) },
		"FLOOR":       func(p *Parser) exp.Expression { return p.parseCeilFloor(exp.KindFloor) },
		"EXTRACT":     func(p *Parser) exp.Expression { return p.parseExtract() },
		"POSITION":    func(p *Parser) exp.Expression { return p.parsePosition() },
		"SUBSTRING":   func(p *Parser) exp.Expression { return p.parseSubstring() },
		"TRIM":        func(p *Parser) exp.Expression { return p.parseTrim() },
		"STRING_AGG":  func(p *Parser) exp.Expression { return p.parseStringAgg() },
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

func (p *Parser) matchRParen(expression exp.Expression) {
	if !p.match(tokens.R_PAREN) {
		p.raiseError("Expecting )")
	}
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
