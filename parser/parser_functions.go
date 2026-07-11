package parser

import (
	"github.com/sjincho/sqlglot-go/dialects"
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

func seqGet[T any](s []T, i int) T {
	var zero T
	if i < 0 || i >= len(s) {
		return zero
	}
	return s[i]
}

// mergedDialectFunctions is the parseFunctionCall functions==nil default: exp.FunctionByName
// overlaid with the current dialect's own Functions additions/overrides (dialects.Dialect.
// Functions, e.g. mysql's CURDATE/DAY_OF_MONTH/LCASE/... cluster in dialects/mysql.go, or
// postgres's CHAR_LENGTH/CHARACTER_LENGTH in dialects/postgres.go). Mirrors upstream's
// per-dialect `FUNCTIONS = {**parser.Parser.FUNCTIONS, ...}` class-attribute pattern, just
// merged on demand instead of once at class-definition time, since this port has one shared
// exp.FunctionByName base map rather than a per-dialect-class copy. A dialect with no
// Functions overlay (the common case) skips the merge entirely and returns the base map
// as-is.
func mergedDialectFunctions(d *dialects.Dialect) map[string]func([]exp.Expression) exp.Expression {
	if len(d.Functions) == 0 {
		return exp.FunctionByName
	}
	merged := make(map[string]func([]exp.Expression) exp.Expression, len(exp.FunctionByName)+len(d.Functions))
	for name, builder := range exp.FunctionByName {
		merged[name] = builder
	}
	for name, builder := range d.Functions {
		merged[name] = builder
	}
	return merged
}

func (p *Parser) parseConvert(strict bool, safe any) exp.Expression {
	this := p.parseBitwise()
	var to exp.Expression
	if p.match(tokens.USING) {
		// parser.py:7969: CONVERT(x USING <charset>) -> CAST(x AS CHAR CHARACTER SET <charset>),
		// captured via the synthetic CHARACTER_SET data type (mirrors the CAST(x AS CHAR
		// CHARACTER SET ...) construction in parser_types.go's parseCast).
		to = exp.DTypeCharacterSet.IntoExpr(exp.Args{"kind": p.parseCharsetName()})
	} else if p.match(tokens.COMMA) {
		to = p.parseTypes(false, false, true, false)
	}
	args := exp.Args{"this": this, "to": to}
	if safe != nil {
		args["safe"] = safe
	}
	return p.buildCast(strict, args)
}

// parseChar ports _parse_char (parser.py:7836-7842): CHR/CHAR(<expr>, ... [USING <charset>]).
func (p *Parser) parseChar() exp.Expression {
	expressions := p.parseCsv(p.parseAssignment)
	var charset exp.Expression
	if p.match(tokens.USING) {
		charset = p.parseCharsetName()
	}
	return p.expression(exp.Chr(exp.Args{"expressions": expressions, "charset": charset}), nil, nil)
}

// parseCharsetName ports _parse_charset_name (parser.py:7844-7851) and MySQL's override
// (parsers/mysql.py:523-535, mirrored inline below via the dialect check rather than a
// separate dialect-flag/override table). MySQL parses a possibly-quoted identifier and
// preserves quoting for charset names that need it (e.g. spaces, as allowed for custom
// XML-registered charsets), unwrapping "safe" names to a bare Var so the common case
// round-trips unquoted; other dialects just parse a VAR/BINARY/IDENTIFIER token as a bare
// Var (quoting dropped, matching the base upstream behavior).
func (p *Parser) parseCharsetName() exp.Expression {
	if p.dialect.Name == "mysql" {
		if identifier := p.parseIdentifier(); identifier != nil {
			name := identifier.Name()
			if exp.IsSafeIdentifier(name) {
				return exp.Var(exp.Args{"this": name})
			}
			return identifier
		}
		return p.parseVar(false, map[tokens.TokenType]bool{tokens.BINARY: true}, false)
	}
	return p.parseVar(false, map[tokens.TokenType]bool{tokens.BINARY: true, tokens.IDENTIFIER: true}, false)
}

func (p *Parser) parseCeilFloor(kind exp.Kind) exp.Expression {
	args := p.parseCsv(func() exp.Expression { return p.parseLambda(false) })
	this := seqGet(args, 0)
	decimals := seqGet(args, 1)
	var to exp.Expression
	if p.matchTextSeq("TO") {
		to = p.parseVar(false, nil, false)
	}
	return p.expression(exp.New(kind, exp.Args{"this": this, "decimals": decimals, "to": to}), nil, nil)
}

func (p *Parser) parseExtract() exp.Expression {
	// Mirror upstream `self._parse_function() or self._parse_var_or_string(...)`:
	// short-circuit so the second alternative never runs (and never advances past
	// FROM via parseVar's advanceAny) once the first succeeds.
	this := p.parseFunction(nil, false, true, false)
	if this == nil {
		this = p.parseVarOrString(true)
	}
	if p.match(tokens.FROM) {
		return p.expression(exp.Extract(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil)
	}
	if !p.match(tokens.COMMA) {
		p.raiseError("Expected FROM or comma after EXTRACT", p.prev)
	}
	return p.expression(exp.Extract(exp.Args{"this": this, "expression": p.parseBitwise()}), nil, nil)
}

// parseDatePart ports postgres's FUNCTION_PARSERS["DATE_PART"] -> _parse_date_part
// (parsers/postgres.py:303-311): `DATE_PART(<type-or-part>, <value>)` desugars to
// `EXTRACT(<part> FROM <value>)`. A Column/Literal part (the common `DATE_PART('year', x)`
// shape) is normalized to a bare Var so it renders unquoted like an EXTRACT part name; a
// Cast (e.g. `'isodow'::varchar(6)`) is left as-is.
func (p *Parser) parseDatePart() exp.Expression {
	part := p.parseType(true, false)
	p.match(tokens.COMMA)
	value := p.parseBitwise()
	if part != nil && (part.Kind() == exp.KindColumn || part.Kind() == exp.KindLiteral) {
		part = exp.Var(exp.Args{"this": part.Name()})
	}
	return p.expression(exp.Extract(exp.Args{"this": part, "expression": value}), nil, nil)
}

func (p *Parser) parsePosition() exp.Expression {
	args := p.parseCsv(p.parseBitwise)
	if p.match(tokens.IN) {
		return p.expression(exp.StrPosition(exp.Args{"this": p.parseBitwise(), "substr": seqGet(args, 0)}), nil, nil)
	}
	return p.expression(exp.StrPosition(exp.Args{"this": seqGet(args, 1), "substr": seqGet(args, 0), "position": seqGet(args, 2)}), nil, nil)
}

// parseOverlay ports _parse_overlay (parser.py:9786-9801): OVERLAY(<this> PLACING <expr>
// [FROM <from>] [FOR <for>]); PLACING/FROM/FOR are each optional and interchangeable with a
// plain comma in the same position (base FUNCTION_PARSERS entry, parser.py:1511 - not
// dialect-gated, closes parity_gaps.txt gaps 196-197).
func (p *Parser) parseOverlay() exp.Expression {
	parseArg := func(text string) exp.Expression {
		if p.match(tokens.COMMA) || p.matchTextSeq(text) {
			return p.parseBitwise()
		}
		return nil
	}
	return p.expression(exp.Overlay(exp.Args{
		"this":       p.parseBitwise(),
		"expression": parseArg("PLACING"),
		"from_":      parseArg("FROM"),
		"for_":       parseArg("FOR"),
	}), nil, nil)
}

func (p *Parser) parseSubstring() exp.Expression {
	args := p.parseCsv(p.parseBitwise)
	var start exp.Expression
	var length exp.Expression
	for p.curr.IsValid() {
		if p.match(tokens.FROM) {
			start = p.parseBitwise()
		} else if p.match(tokens.FOR) {
			if start == nil {
				start = exp.LiteralNumber(1)
			}
			length = p.parseBitwise()
		} else {
			break
		}
	}
	if start != nil {
		args = append(args, start)
	}
	if length != nil {
		args = append(args, length)
	}
	return p.validateExpression(exp.FromArgList(exp.KindSubstring, args), exprArgs(args))
}

// parseConcat ports the FUNCTIONS "CONCAT" builder (parser.py:345-349):
//
//	"CONCAT": lambda args, dialect: exp.Concat(
//	    expressions=args, safe=not dialect.STRICT_STRING_CONCAT, coalesce=dialect.CONCAT_COALESCE)
//
// The dialect-dependent safe/coalesce args are why this is a dialect-aware FUNCTION_PARSERS entry
// rather than a plain FunctionByName builder. Building a real exp.Concat (not an Anonymous call)
// is what lets it transpile across dialects, e.g. `CONCAT(a, b)` base->postgres -> `a || b` and
// postgres->mysql wraps each arg in an empty-string COALESCE (see concatSQL/convertConcatArgs).
// CONCAT_WS keeps round-tripping via Anonymous: it needs a distinct exp.ConcatWs node whose
// concatws_sql wraps a NULL-coalescing dialect in a CASE, which is out of this port's scope; it
// only diverges cross-dialect (never in the same-read==write corpus), so leaving it Anonymous is
// no regression.
func (p *Parser) parseConcat() exp.Expression {
	args := p.parseFunctionArgs(false)
	return p.validateExpression(exp.Concat(exp.Args{
		"expressions": args,
		"safe":        !p.dialect.StrictStringConcat,
		"coalesce":    p.dialect.ConcatCoalesce,
	}), exprArgs(args))
}

func (p *Parser) parseTrim() exp.Expression {
	var position any
	var collation exp.Expression
	var expression exp.Expression
	if p.matchTexts(trimTypes) {
		position = stringsUpper(p.prev.Text)
	}
	this := p.parseBitwise()
	if p.matchSet(map[tokens.TokenType]bool{tokens.FROM: true, tokens.COMMA: true}) {
		invert := p.prev.TokenType == tokens.FROM
		expression = p.parseBitwise()
		if invert {
			this, expression = expression, this
		}
	}
	if p.match(tokens.COLLATE) {
		collation = p.parseBitwise()
	}
	return p.expression(exp.Trim(exp.Args{"this": this, "position": position, "expression": expression, "collation": collation}), nil, nil)
}

func (p *Parser) parseStringAgg() exp.Expression {
	var args []exp.Expression
	if p.match(tokens.DISTINCT) {
		args = []exp.Expression{p.expression(exp.Distinct(exp.Args{"expressions": []exp.Expression{p.parseDisjunction()}}), nil, nil)}
		if p.match(tokens.COMMA) {
			args = append(args, p.parseCsv(p.parseDisjunction)...)
		}
	} else {
		args = p.parseCsv(p.parseDisjunction)
	}
	index := p.index
	if !p.match(tokens.R_PAREN) && len(args) > 0 {
		ordered := p.parseOrder(seqGet(args, 0), false)
		args[0] = p.parseLimit(ordered, false, false)
		return p.expression(exp.GroupConcat(exp.Args{"this": args[0], "separator": seqGet(args, 1)}), nil, nil)
	}
	if !p.matchTextSeq("WITHIN", "GROUP") {
		p.retreat(index)
		return p.validateExpression(exp.FromArgList(exp.KindGroupConcat, args), exprArgs(args))
	}
	p.matchLParen(nil)
	return p.expression(exp.GroupConcat(exp.Args{"this": p.parseOrder(seqGet(args, 0), false), "separator": seqGet(args, 1)}), nil, nil)
}

// parseFormatJson ports _parse_format_json (parser.py:8054-8058): wraps `this` in
// exp.FormatJson if followed by the literal "FORMAT JSON" text sequence.
func (p *Parser) parseFormatJson(this exp.Expression) exp.Expression {
	if this == nil || !p.matchTextSeq("FORMAT", "JSON") {
		return this
	}
	return p.expression(exp.FormatJson(exp.Args{"this": this}), nil, nil)
}

// parseOnHandling ports _parse_on_handling (parser.py:8076-8090): parses the
// "<value> ON <on>" or "DEFAULT <expr> ON <on>" syntax (e.g. "NULL ON NULL",
// "ERROR ON ERROR"). Returns a string, an exp.Expression (the DEFAULT case), or nil.
func (p *Parser) parseOnHandling(on string, values ...string) any {
	for _, value := range values {
		if p.matchTextSeq(value, "ON", on) {
			return value + " ON " + on
		}
	}
	index := p.index
	if p.match(tokens.DEFAULT) {
		defaultValue := p.parseBitwise()
		if p.matchTextSeq("ON", on) {
			return defaultValue
		}
		p.retreat(index)
	}
	return nil
}

// parseJSONColumnDef ports _parse_json_column_def (parser.py:8131-8156). Note: like
// upstream, this only implements the "JSON_value_column" part of the grammar.
func (p *Parser) parseJSONColumnDef() exp.Expression {
	var this, kind, nestedSchema exp.Expression
	var ordinality any
	nested := false
	if !p.matchTextSeq("NESTED") {
		// any_token=true mirrors upstream _parse_json_column_def's _parse_id_var()
		// (parser.py:8131), so keyword-like column names are accepted.
		this = p.parseIdVar(true, nil)
		ordinality = p.matchPair(tokens.FOR, tokens.ORDINALITY, true)
		kind = p.parseTypes(false, false, false, false)
	} else {
		nested = true
	}
	formatJSON := p.matchTextSeq("FORMAT", "JSON")
	var path exp.Expression
	if p.matchTextSeq("PATH") {
		path = p.parseString()
	}
	if nested {
		nestedSchema = p.parseJSONSchema()
	}
	return p.expression(exp.JSONColumnDef(exp.Args{
		"this":          this,
		"kind":          kind,
		"path":          path,
		"nested_schema": nestedSchema,
		"ordinality":    ordinality,
		"format_json":   formatJSON,
	}), nil, nil)
}

// parseJSONSchema ports _parse_json_schema (parser.py:8158-8164): `[COLUMNS] (col_def, ...)`.
func (p *Parser) parseJSONSchema() exp.Expression {
	p.matchTextSeq("COLUMNS")
	return p.expression(exp.JSONSchema(exp.Args{
		"expressions": p.parseWrappedCsv(p.parseJSONColumnDef, true),
	}), nil, nil)
}

// parseJSONTable ports _parse_json_table (parser.py:8166-8179):
// JSON_TABLE(<doc> [FORMAT JSON], <path> [<on-error>] [<on-empty>] <schema>).
// Mirrors upstream in returning the raw exp.JSONTable node (not wrapped via
// p.expression), so it isn't parser-node-count/error-message validated here.
func (p *Parser) parseJSONTable() exp.Expression {
	this := p.parseFormatJson(p.parseBitwise())
	var path exp.Expression
	if p.match(tokens.COMMA) {
		path = p.parseString()
	}
	errorHandling := p.parseOnHandling("ERROR", "ERROR", "NULL")
	emptyHandling := p.parseOnHandling("EMPTY", "ERROR", "NULL")
	schema := p.parseJSONSchema()
	return exp.JSONTable(exp.Args{
		"this":           this,
		"schema":         schema,
		"path":           path,
		"error_handling": errorHandling,
		"empty_handling": emptyHandling,
	})
}

// parseMatchAgainst ports _parse_match_against (parser.py:8181): MATCH(col, ...) AGAINST
// ('expr' [modifier]) full-text search, the base FUNCTION_PARSERS["MATCH"] entry
// (parser.py:1508). Called positioned inside `MATCH(` (parseFunctionCall already consumed
// the name + `(`); the caller consumes the closing `)` of the AGAINST group. The base
// generator renders MATCH(...) AGAINST(...); postgres' matchAgainstSQL override renders `@@`.
func (p *Parser) parseMatchAgainst() exp.Expression {
	var expressions []exp.Expression
	if p.matchTextSeq("TABLE") {
		// SingleStore MATCH(TABLE ...) syntax, kept 1:1 with upstream (not exercised by
		// base/mysql/postgres).
		if table := p.parseTable(false, false, nil, false, false, false, false); table != nil {
			expressions = []exp.Expression{table}
		}
	} else {
		expressions = p.parseCsv(p.parseColumn)
	}

	p.matchTextSeq(")", "AGAINST", "(")

	this := p.parseString()

	var modifier any
	switch {
	case p.matchTextSeq("IN", "NATURAL", "LANGUAGE", "MODE"):
		modifier = "IN NATURAL LANGUAGE MODE"
		if p.matchTextSeq("WITH", "QUERY", "EXPANSION") {
			modifier = "IN NATURAL LANGUAGE MODE WITH QUERY EXPANSION"
		}
	case p.matchTextSeq("IN", "BOOLEAN", "MODE"):
		modifier = "IN BOOLEAN MODE"
	case p.matchTextSeq("WITH", "QUERY", "EXPANSION"):
		modifier = "WITH QUERY EXPANSION"
	}

	args := exp.Args{"this": this, "expressions": expressions}
	if modifier != nil {
		args["modifier"] = modifier
	}
	return p.expression(exp.MatchAgainst(args), nil, nil)
}

// init registers this file's base FUNCTION_PARSERS/NO_PAREN_FUNCTION_PARSERS entries by plain
// key assignment into the shared functionParsers/noParenFunctionParsers package vars (see the
// package-var doc comments on statementParsers/dispatch for why this is safe regardless of
// init() run order across files: parser.go's own init() only ever does a full map-literal
// REASSIGNMENT of these two vars, and Go runs same-package init() funcs in lexical filename
// order - "parser.go" sorts before "parser_functions.go" - so that reassignment always
// completes before this file's key assignments run).
func init() {
	// OVERLAY is a base FUNCTION_PARSERS entry (parser.py:1511) - not dialect-gated.
	functionParsers["OVERLAY"] = (*Parser).parseOverlay
	// MATCH is a base FUNCTION_PARSERS entry (parser.py:1508) - not dialect-gated. Used
	// mainly in MySQL full-text search; the base generator renders MATCH(...) AGAINST(...)
	// and postgres overrides matchAgainstSQL to the `@@` form.
	functionParsers["MATCH"] = (*Parser).parseMatchAgainst
	// SUBSTR is MySQL-only upstream (parsers/mysql.py:162). The base singleton retains this
	// pre-existing entry for zero behavior change; functionParser applies the MySQL-only
	// compatibility filter after consulting parser-side dialect overrides.
	functionParsers["SUBSTR"] = (*Parser).parseSubstring
	// CONCAT is a base FUNCTIONS builder (parser.py:345-349), but unlike the FunctionByName
	// builders it needs the dialect (safe/coalesce), so it lives here in the dialect-aware
	// FUNCTION_PARSERS map instead. CONCAT_WS is left Anonymous - see parseConcat's note.
	functionParsers["CONCAT"] = (*Parser).parseConcat

	// VARIADIC is postgres-only (parsers/postgres.py:142 NO_PAREN_FUNCTION_PARSERS). The base
	// singleton retains this pre-existing entry for zero behavior change;
	// noParenFunctionParserFor applies the Postgres-only compatibility filter after consulting
	// parser-side dialect overrides, so in base/mysql a bare `VARIADIC` stays a column while
	// `VARIADIC(x)` parses as an ordinary function call.
	noParenFunctionParsers["VARIADIC"] = func(p *Parser) exp.Expression {
		return p.expression(exp.Variadic(exp.Args{"this": p.parseBitwise()}), nil, nil)
	}
}
