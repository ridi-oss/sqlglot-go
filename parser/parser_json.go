package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// jsonKeyValueSeparatorTokens ports JSON_KEY_VALUE_SEPARATOR_TOKENS (parser.py:1681):
// the token that binds a JSON_OBJECT key to its value, e.g. `'key1': 1` / `'key1', 1` /
// `'key1' IS 1`.
var jsonKeyValueSeparatorTokens = map[tokens.TokenType]bool{
	tokens.COLON: true,
	tokens.COMMA: true,
	tokens.IS:    true,
}

// onConditionTokens ports ON_CONDITION_TOKENS (parser.py:1711): the value tokens accepted
// before "ON EMPTY"/"ON ERROR"/"ON NULL" in _parse_on_condition.
var onConditionTokens = []string{"ERROR", "NULL", "TRUE", "FALSE", "EMPTY"}

// parseJSONKeyValue ports _parse_json_key_value (parser.py:8043-8052): one `[KEY] <key>
// [<sep>] [VALUE] <value>` pair inside JSON_OBJECT(...)/JSON_OBJECTAGG(...). Returns nil
// (rather than a JSONKeyValue with nil this/expression) when nothing at all was consumed,
// so parseCsv's caller can distinguish "empty pair" from "no more pairs".
func (p *Parser) parseJSONKeyValue() exp.Expression {
	p.matchTextSeq("KEY")
	key := p.parseColumn()
	p.matchSet(jsonKeyValueSeparatorTokens)
	p.matchTextSeq("VALUE")
	value := p.parseBitwise()

	if key == nil && value == nil {
		return nil
	}
	return p.expression(exp.JSONKeyValue(exp.Args{"this": key, "expression": value}), nil, nil)
}

// parseOnCondition ports _parse_on_condition (parser.py:8060-8074). Upstream branches on
// dialect.ON_CONDITION_EMPTY_BEFORE_ERROR to decide whether EMPTY or ERROR is parsed first;
// that flag is true for base/mysql/postgres alike (dialect.py:654), so EMPTY always comes
// first here.
func (p *Parser) parseOnCondition() exp.Expression {
	onEmpty := p.parseOnHandling("EMPTY", onConditionTokens...)
	onError := p.parseOnHandling("ERROR", onConditionTokens...)
	onNull := p.parseOnHandling("NULL", onConditionTokens...)

	if onEmpty == nil && onError == nil && onNull == nil {
		return nil
	}
	return p.expression(exp.OnCondition(exp.Args{"empty": onEmpty, "error": onError, "null": onNull}), nil, nil)
}

// parseJSONObject ports _parse_json_object (parser.py:8092-8128): JSON_OBJECT(...) /
// JSON_OBJECTAGG(...), built from either a bare `*` or a CSV list of `[FORMAT JSON]`
// key/value pairs, followed by NULL-handling, WITH/WITHOUT UNIQUE KEYS, a
// `RETURNING <type> [FORMAT JSON]` clause, and an `ENCODING <var>` clause.
func (p *Parser) parseJSONObject(agg bool) exp.Expression {
	star := p.parseStar()
	var itemExpressions []exp.Expression
	if star != nil {
		itemExpressions = []exp.Expression{star}
	} else {
		itemExpressions = p.parseCsv(func() exp.Expression {
			return p.parseFormatJson(p.parseJSONKeyValue())
		})
	}
	nullHandling := p.parseOnHandling("NULL", "NULL", "ABSENT")

	var uniqueKeys any
	if p.matchTextSeq("WITH", "UNIQUE") {
		uniqueKeys = true
	} else if p.matchTextSeq("WITHOUT", "UNIQUE") {
		uniqueKeys = false
	}

	p.matchTextSeq("KEYS")

	var returnType exp.Expression
	if p.matchTextSeq("RETURNING") {
		returnType = p.parseFormatJson(p.parseType(true, false))
	}
	var encoding exp.Expression
	if p.matchTextSeq("ENCODING") {
		encoding = p.parseVar(false, nil, false)
	}

	args := exp.Args{
		"expressions":   itemExpressions,
		"null_handling": nullHandling,
		"unique_keys":   uniqueKeys,
		"return_type":   returnType,
		"encoding":      encoding,
	}
	if agg {
		return p.expression(exp.JSONObjectAgg(args), nil, nil)
	}
	return p.expression(exp.JSONObject(args), nil, nil)
}

// parseJSONValue ports _parse_json_value (parser.py:10058-10072): JSON_VALUE(<doc>,
// <path> [RETURNING <type>] [<on-condition>]). Unlike upstream, `path` is stored as the
// raw parsed expression with no dialect.to_json_path normalization - this port doesn't
// have a JSONPath subsystem (see the plan's JSONPath resolution note); the only signal the
// corpus needs is literal-vs-non-literal RHS, which the raw expression already preserves.
func (p *Parser) parseJSONValue() exp.Expression {
	this := p.parseBitwise()
	p.match(tokens.COMMA)
	path := p.parseBitwise()

	var returning exp.Expression
	if p.match(tokens.RETURNING) {
		returning = p.parseType(true, false)
	}

	return p.expression(exp.JSONValue(exp.Args{
		"this":         this,
		"path":         path,
		"returning":    returning,
		"on_condition": p.parseOnCondition(),
	}), nil, nil)
}

// trinoJSONQueryOptions ports TrinoParser.JSON_QUERY_OPTIONS (parsers/trino.py:28-40), including
// the reference's CONDITIONAL ARRAY WRAPPED spelling.
var trinoJSONQueryOptions = optionsType{
	"WITH": {
		{"WRAPPER"},
		{"ARRAY", "WRAPPER"},
		{"CONDITIONAL", "WRAPPER"},
		{"CONDITIONAL", "ARRAY", "WRAPPED"},
		{"UNCONDITIONAL", "WRAPPER"},
		{"UNCONDITIONAL", "ARRAY", "WRAPPER"},
	},
	"WITHOUT": {
		{"WRAPPER"},
		{"ARRAY", "WRAPPER"},
		{"CONDITIONAL", "WRAPPER"},
		{"CONDITIONAL", "ARRAY", "WRAPPED"},
		{"UNCONDITIONAL", "WRAPPER"},
		{"UNCONDITIONAL", "ARRAY", "WRAPPER"},
	},
}

func (p *Parser) parseJSONQueryQuote() exp.Expression {
	if !(p.matchTextSeq("KEEP", "QUOTES") || p.matchTextSeq("OMIT", "QUOTES")) {
		return nil
	}

	return p.expression(exp.JSONExtractQuote(exp.Args{
		"option": stringsUpper(p.tokens[p.index-2].Text),
		"scalar": p.matchTextSeq("ON", "SCALAR", "STRING"),
	}), nil, nil)
}

// parseJSONQuery ports TrinoParser._parse_json_query (parsers/trino.py:53-63). The path remains
// the raw parsed expression because this port does not yet have dialect.to_json_path normalization.
func (p *Parser) parseJSONQuery() exp.Expression {
	this := p.parseBitwise()
	var expression exp.Expression
	if p.match(tokens.COMMA) {
		expression = p.parseBitwise()
	}

	return p.expression(exp.JSONExtract(exp.Args{
		"this":         this,
		"expression":   expression,
		"option":       p.parseVarFromOptions(trinoJSONQueryOptions, false),
		"json_query":   true,
		"quote":        p.parseJSONQueryQuote(),
		"on_condition": p.parseOnCondition(),
	}), nil, nil)
}
