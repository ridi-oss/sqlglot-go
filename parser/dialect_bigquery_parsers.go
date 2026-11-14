package parser

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// parsers/bigquery.py:325-384 FUNCTION_PARSERS, NO_PAREN_FUNCTIONS, PROPERTY_PARSERS and
// STATEMENT_PARSERS.
func init() {
	registerDialectParserOverrides("bigquery", dialectParserOverrideSet{
		FunctionParsers: map[string]parserOverrideFunc{
			"ARRAY": func(p *Parser) exp.Expression {
				return p.expression(exp.New(exp.KindArray, exp.Args{"expressions": []exp.Expression{p.parseStatement()}, "struct_name_inheritance": true}), nil, nil)
			},
			"JSON_ARRAY": func(p *Parser) exp.Expression {
				return p.expression(exp.New(exp.KindJSONArray, exp.Args{"expressions": p.parseCsv(p.parseBitwise)}), nil, nil)
			},
			"MAKE_INTERVAL":           (*Parser).bigqueryParseMakeInterval,
			"PREDICT":                 func(p *Parser) exp.Expression { return p.bigqueryParseML(exp.KindPredict, false) },
			"TRANSLATE":               (*Parser).bigqueryParseTranslate,
			"FEATURES_AT_TIME":        (*Parser).bigqueryParseFeaturesAtTime,
			"GENERATE_EMBEDDING":      func(p *Parser) exp.Expression { return p.bigqueryParseML(exp.KindGenerateEmbedding, false) },
			"GENERATE_TEXT_EMBEDDING": func(p *Parser) exp.Expression { return p.bigqueryParseML(exp.KindGenerateEmbedding, true) },
			"GENERATE_TEXT":           func(p *Parser) exp.Expression { return p.bigqueryParseGenerate(exp.KindGenerateText) },
			"GENERATE_TABLE":          func(p *Parser) exp.Expression { return p.bigqueryParseGenerate(exp.KindGenerateTable) },
			"GENERATE_BOOL":           func(p *Parser) exp.Expression { return p.bigqueryParseGenerate(exp.KindGenerateBool) },
			"GENERATE_INT":            func(p *Parser) exp.Expression { return p.bigqueryParseGenerate(exp.KindGenerateInt) },
			"GENERATE_DOUBLE":         func(p *Parser) exp.Expression { return p.bigqueryParseGenerate(exp.KindGenerateDouble) },
			"VECTOR_SEARCH":           (*Parser).bigqueryParseVectorSearch,
			"FORECAST":                (*Parser).bigqueryParseForecast,
		},
		DisabledFunctionParsers: map[string]bool{"TRIM": true},
		NoParenFunctions: map[tokens.TokenType]func(exp.Args) exp.Expression{
			tokens.CURRENT_DATETIME: func(a exp.Args) exp.Expression { return exp.New(exp.KindCurrentDatetime, a) },
		},
		PropertyParsers: map[string]propertyParserFunc{
			"OPTIONS": func(p *Parser, _ bool) exp.Expression { return p.parseWithProperty() },
		},
		StatementParsers: map[tokens.TokenType]parserOverrideFunc{
			tokens.DECLARE: (*Parser).parseDeclareStatement,
			tokens.ELSE:    func(p *Parser) exp.Expression { return p.parseAsCommand(p.prev) },
			tokens.END:     func(p *Parser) exp.Expression { return p.parseAsCommand(p.prev) },
			tokens.FOR:     (*Parser).bigqueryParseForIn,
			tokens.EXPORT:  (*Parser).bigqueryParseExportData,
		},
		AliasTokens: map[tokens.TokenType]bool{tokens.ANTI: true, tokens.SEMI: true, tokens.GRANT: true},
	})
}

// parsers/bigquery.py:393-400.
func (p *Parser) bigqueryParseForIn() exp.Expression {
	index := p.index
	this := p.parseRange(nil)
	p.matchTextSeq("DO")
	if p.match(tokens.COMMAND) {
		p.retreat(index)
		return p.parseAsCommand(p.prev)
	}
	return p.expression(exp.New(exp.KindForIn, exp.Args{"this": this, "expression": p.parseStatement()}), nil, nil)
}

// parsers/bigquery.py:762-771.
func (p *Parser) bigqueryParseExportData() exp.Expression {
	p.matchTextSeq("DATA")
	var connection exp.Expression
	if p.matchTextSeq("WITH", "CONNECTION") {
		connection = p.parseTableParts(false, false, false, false)
	}
	options := p.parseProperties()
	var this exp.Expression
	if p.matchTextSeq("AS") {
		this = p.parseSelect(true)
	}
	return p.expression(exp.New(exp.KindExport, exp.Args{"connection": connection, "options": options, "this": this}), nil, nil)
}

var makeIntervalKwargs = []string{"year", "month", "day", "hour", "minute", "second"}

// parsers/bigquery.py:625-643.
func (p *Parser) bigqueryParseMakeInterval() exp.Expression {
	e := exp.New(exp.KindMakeInterval, exp.Args{})
	for _, key := range makeIntervalKwargs {
		value := p.parseLambda(false)
		if value == nil {
			break
		}
		if value.Kind() == exp.KindKwarg {
			key = value.This().Name()
		}
		e.Set(key, value)
		p.match(tokens.COMMA)
	}
	return e
}

// parsers/bigquery.py:645-663.
func (p *Parser) bigqueryParseML(kind exp.Kind, isText bool) exp.Expression {
	p.matchTextSeq("MODEL")
	this := p.parseTable(false, false, nil, false, false, false, false)
	p.match(tokens.COMMA)
	p.matchTextSeq("TABLE")
	var expression exp.Expression
	if !p.match(tokens.STRUCT, false) {
		expression = p.parseTable(false, false, nil, false, false, false, false)
	}
	p.match(tokens.COMMA)
	args := exp.Args{"this": this, "expression": expression, "params_struct": p.parseBitwise()}
	if isText {
		args["is_text"] = true
	}
	return p.expression(exp.New(kind, args), nil, nil)
}

// parsers/bigquery.py:665-682.
func (p *Parser) bigqueryParseGenerate(kind exp.Kind) exp.Expression {
	p.matchTextSeq("MODEL")
	this := p.parseTable(false, false, nil, false, false, false, false)
	p.match(tokens.COMMA)
	var expression exp.Expression
	if p.matchTextSeq("TABLE") || p.match(tokens.L_PAREN, false) {
		expression = p.parseTable(false, false, nil, false, false, false, false)
	} else {
		expression = p.parseBitwise()
	}
	var params exp.Expression
	if p.match(tokens.COMMA) {
		params = p.parseBitwise()
	}
	return p.expression(exp.New(kind, exp.Args{"this": this, "expression": expression, "params_struct": params}), nil, nil)
}

func (p *Parser) tokenBackIsML(n int) bool {
	i := p.index - n
	return i >= 0 && i < len(p.tokens) && strings.ToUpper(p.tokens[i].Text) == "ML"
}

// parsers/bigquery.py:684-690.
func (p *Parser) bigqueryParseTranslate() exp.Expression {
	if p.tokenBackIsML(4) {
		return p.bigqueryParseML(exp.KindMLTranslate, false)
	}
	return exp.FromArgList(exp.KindTranslate, p.parseFunctionArgs(false))
}

// parsers/bigquery.py:692-714.
func (p *Parser) bigqueryParseForecast() exp.Expression {
	if p.tokenBackIsML(4) {
		return p.bigqueryParseML(exp.KindMLForecast, false)
	}
	p.match(tokens.TABLE)
	this := p.parseTable(false, false, nil, false, false, false, false)
	if this == nil {
		p.raiseError("Expected table or query statement")
	}
	e := p.expression(exp.New(exp.KindAIForecast, exp.Args{"this": this}), nil, nil)
	for p.match(tokens.COMMA) {
		arg := p.parseLambda(false)
		if arg == nil || arg.Kind() != exp.KindKwarg {
			p.raiseError("Expected key => value syntax for AI.FORECAST")
			break
		}
		e.Set(arg.This().Name(), arg)
	}
	return e
}

// parsers/bigquery.py:716-730.
func (p *Parser) bigqueryParseFeaturesAtTime() exp.Expression {
	p.match(tokens.TABLE)
	this := p.parseTable(false, false, nil, false, false, false, false)
	e := p.expression(exp.New(exp.KindFeaturesAtTime, exp.Args{"this": this}), nil, nil)
	for p.match(tokens.COMMA) {
		if arg := p.parseLambda(false); arg != nil {
			e.Set(arg.This().Name(), arg)
		}
	}
	return e
}

// parsers/bigquery.py:732-760.
func (p *Parser) bigqueryParseVectorSearch() exp.Expression {
	p.match(tokens.TABLE)
	base := p.parseTable(false, false, nil, false, false, false, false)
	p.match(tokens.COMMA)
	column := p.parseBitwise()
	p.match(tokens.COMMA)
	p.match(tokens.TABLE)
	query := p.parseTable(false, false, nil, false, false, false, false)
	e := p.expression(exp.New(exp.KindVectorSearch, exp.Args{"this": base, "column_to_search": column, "query_table": query}), nil, nil)
	for p.match(tokens.COMMA) {
		if p.match(tokens.STRING, false) {
			e.Set("query_column_to_search", p.parseString())
		} else if arg := p.parseLambda(false); arg != nil {
			e.Set(arg.This().Name(), arg)
		}
	}
	return e
}
