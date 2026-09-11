package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// trinoParserOverrideSet returns fresh maps for each parser class. AthenaTrinoParser inherits the
// same callbacks but owns an independent statement map for its USING extension.
func trinoParserOverrideSet() dialectParserOverrideSet {
	return dialectParserOverrideSet{
		FunctionParsers: map[string]parserOverrideFunc{
			"TRIM":       (*Parser).parseTrim,
			"JSON_QUERY": (*Parser).parseJSONQuery,
			"JSON_VALUE": (*Parser).parseJSONValue,
			"LISTAGG":    (*Parser).parseTrinoStringAgg,
		},
		StatementParsers: map[tokens.TokenType]parserOverrideFunc{
			tokens.REFRESH: (*Parser).parseRefresh,
			tokens.DECLARE: (*Parser).parseDeclareStatement,
		},
		// parser.py:1399 STORED is a base PROPERTY_PARSERS entry that the shared Go table keeps
		// fail-closed for mysql/postgres; the Presto family registers it.
		PropertyParsers: map[string]propertyParserFunc{
			"STORED": func(p *Parser, _ bool) exp.Expression { return p.parseStored() },
		},
		NoParenFunctions: map[tokens.TokenType]func(exp.Args) exp.Expression{
			tokens.CURRENT_CATALOG: exp.CurrentCatalog,
			tokens.LOCALTIME:       exp.Localtime,
			tokens.LOCALTIMESTAMP:  exp.Localtimestamp,
		},
	}
}

func init() {
	registerDialectParserOverrides("trino", trinoParserOverrideSet())

	athena := trinoParserOverrideSet()
	athena.StatementParsers[tokens.DESCRIBE] = (*Parser).parseAthenaExplainStatement
	athena.StatementParsers[tokens.UNLOAD] = (*Parser).parseAthenaUnloadStatement
	athena.StatementParsers[tokens.USING] = func(p *Parser) exp.Expression {
		return p.parseAsCommand(p.prev)
	}
	registerDialectParserOverrides("athena", athena)
}

// parseTrinoStringAgg is a local copy of _parse_string_agg (parser.py:7911-7963). Keeping the
// overflow grammar in Trino's callback table avoids changing STRING_AGG/LISTAGG behavior in every
// existing dialect.
func (p *Parser) parseTrinoStringAgg() exp.Expression {
	var args []exp.Expression
	if p.match(tokens.DISTINCT) {
		args = []exp.Expression{
			p.expression(exp.Distinct(exp.Args{"expressions": []exp.Expression{p.parseDisjunction()}}), nil, nil),
		}
		if p.match(tokens.COMMA) {
			args = append(args, p.parseCsv(p.parseDisjunction)...)
		}
	} else {
		args = p.parseCsv(p.parseDisjunction)
	}

	var onOverflow exp.Expression
	if p.matchTextSeq("ON", "OVERFLOW") {
		if p.matchTextSeq("ERROR") {
			onOverflow = exp.Var(exp.Args{"this": "ERROR"})
		} else {
			p.matchTextSeq("TRUNCATE")
			filler := p.parseString()
			withCount := false
			if p.matchTextSeq("WITH", "COUNT") {
				withCount = true
			} else if !p.matchTextSeq("WITHOUT", "COUNT") {
				withCount = true
			}
			onOverflow = p.expression(exp.OverflowTruncateBehavior(exp.Args{
				"this":       filler,
				"with_count": withCount,
			}), nil, nil)
		}
	}

	index := p.index
	if !p.match(tokens.R_PAREN) && len(args) > 0 {
		args[0] = p.parseLimit(p.parseOrder(args[0], false), false, false)
		return p.expression(exp.GroupConcat(exp.Args{
			"this":      args[0],
			"separator": seqGet(args, 1),
		}), nil, nil)
	}

	if !p.matchTextSeq("WITHIN", "GROUP") {
		p.retreat(index)
		return p.validateExpression(exp.FromArgList(exp.KindGroupConcat, args), exprArgs(args))
	}

	// parseFunctionCall consumes the corresponding closing parenthesis after this callback.
	p.matchLParen(nil)
	return p.expression(exp.GroupConcat(exp.Args{
		"this":        p.parseOrder(seqGet(args, 0), false),
		"separator":   seqGet(args, 1),
		"on_overflow": onOverflow,
	}), nil, nil)
}

// parseDeclareItem ports _parse_declareitem (parser.py:10324-10337).
func (p *Parser) parseDeclareItem() exp.Expression {
	p.matchTexts(map[string]bool{"VAR": true, "VARIABLE": true})
	vars := p.parseCsv(func() exp.Expression { return p.parseIdVar(false, nil) })
	if len(vars) == 0 {
		return nil
	}
	p.match(tokens.ALIAS)
	var kind exp.Expression
	if p.match(tokens.TABLE) {
		kind = p.parseSchema(nil)
	} else {
		kind = p.parseTypes(true, false, true, false)
	}
	var dflt exp.Expression
	if p.match(tokens.DEFAULT) || p.match(tokens.EQ) {
		dflt = p.parseBitwise()
	}
	return p.expression(exp.DeclareItem(exp.Args{"this": vars, "kind": kind, "default": dflt}), nil, nil)
}

// parseDeclareStatement ports _parse_declare (parser.py:10339-10347).
func (p *Parser) parseDeclareStatement() exp.Expression {
	start := p.prev
	replace := p.matchTextSeq("OR", "REPLACE")
	var items []exp.Expression
	p.tryParse(func() exp.Expression {
		items = p.parseCsv(p.parseDeclareItem)
		return exp.Tuple(exp.Args{"expressions": items})
	}, false)
	if len(items) == 0 || p.curr.IsValid() {
		return p.parseAsCommand(start)
	}
	return p.expression(exp.Declare(exp.Args{"expressions": items, "replace": replace}), nil, nil)
}
