package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
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
		},
		NoParenFunctions: map[tokens.TokenType]func(exp.Args) exp.Expression{
			tokens.CURRENT_CATALOG: exp.CurrentCatalog,
		},
	}
}

func init() {
	registerDialectParserOverrides("trino", trinoParserOverrideSet())

	athena := trinoParserOverrideSet()
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
