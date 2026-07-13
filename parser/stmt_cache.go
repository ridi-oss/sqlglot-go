package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.CACHE] = (*Parser).parseCache
	statementParsers[tokens.UNCACHE] = (*Parser).parseUncache
}

// parseUncache ports _parse_uncache (parser.py:3743-3749): `UNCACHE TABLE
// [IF EXISTS] x`.
func (p *Parser) parseUncache() exp.Expression {
	if !p.match(tokens.TABLE) {
		p.raiseError("Expecting TABLE after UNCACHE")
	}

	return p.expression(exp.Uncache(exp.Args{
		"exists": p.parseExists(false),
		"this":   p.parseTable(true, false, nil, false, false, false, false),
	}), nil, nil)
}

// parseCache ports _parse_cache (parser.py:3751-3770): `CACHE [LAZY] TABLE x
// [OPTIONS(k = v)] [AS] <query>`.
func (p *Parser) parseCache() exp.Expression {
	lazy := p.matchTextSeq("LAZY")
	p.match(tokens.TABLE)
	table := p.parseTable(true, false, nil, false, false, false, false)

	var options []exp.Expression
	if p.matchTextSeq("OPTIONS") {
		p.matchLParen(nil)
		k := p.parseString()
		p.match(tokens.EQ)
		v := p.parseString()
		options = []exp.Expression{k, v}
		p.matchRParen(nil)
	}

	p.match(tokens.ALIAS)
	return p.expression(exp.Cache(exp.Args{
		"this":       table,
		"lazy":       lazy,
		"options":    options,
		"expression": p.parseSelect(true),
	}), nil, nil)
}
