package parser

import (
	"strings"

	"github.com/ridi-oss/sqlglot-go/dialects"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

func (p *Parser) isAthenaRouter() bool {
	return p.dialect != nil && strings.EqualFold(p.dialect.Name, "athena")
}

// athenaSubParser consumes the tokenizer's Hive marker (parsers/athena.py:59-74).
func (p *Parser) athenaSubParser(rawTokens []tokens.Token) (*Parser, []tokens.Token) {
	hive := len(rawTokens) > 0 && rawTokens[0].TokenType == tokens.HIVE_TOKEN_STREAM
	d := dialects.Trino()
	if hive {
		d = dialects.Hive()
		rawTokens = rawTokens[1:]
	}
	d.OpaqueFunctions = p.dialect.OpaqueFunctions
	d.NormalizationStrategy = p.dialect.NormalizationStrategy
	var subParser *Parser
	if hive {
		subParser = newWithErrorLevelAndOverrideName(d, p.errorLevel, "athena-hive")
	} else {
		subParser = newWithErrorLevelAndOverrideName(d, p.errorLevel, "athena")
	}

	subParser.errorMessageContext = p.errorMessageContext
	subParser.maxErrors = p.maxErrors
	subParser.maxNodes = p.maxNodes
	return subParser, rawTokens
}

func (p *Parser) parseAthena(rawTokens []tokens.Token, sql string) ([]exp.Expression, error) {
	return p.parseAthenaChunks(rawTokens, sql, func(sub *Parser, chunk []tokens.Token) ([]exp.Expression, error) {
		return sub.Parse(chunk, sql)
	})
}

func (p *Parser) parseIntoAthena(rawTokens []tokens.Token, sql string, into exp.Kind) ([]exp.Expression, error) {
	return p.parseAthenaChunks(rawTokens, sql, func(sub *Parser, chunk []tokens.Token) ([]exp.Expression, error) {
		return sub.ParseInto(chunk, sql, into)
	})
}

func (p *Parser) parseAthenaChunks(rawTokens []tokens.Token, sql string, parse func(*Parser, []tokens.Token) ([]exp.Expression, error)) ([]exp.Expression, error) {
	p.Reset()
	p.sql = sql
	var expressions []exp.Expression
	var firstErr error
	start := 0
	budget := p.maxNodes
	for end := 0; end <= len(rawTokens); end++ {
		if end < len(rawTokens) && rawTokens[end].TokenType != tokens.SEMICOLON {
			continue
		}
		limit := end
		if end < len(rawTokens) {
			limit++
		} else if start == end && end > 0 {
			break
		}
		// Keep the delimiter so the base parser retains its comment-only chunks.
		sub, chunk := p.athenaSubParser(rawTokens[start:limit])
		sub.maxNodes = budget
		parsed, err := parse(sub, chunk)
		p.errors = append(p.errors, sub.Errors()...)
		if budget > -1 {
			// One node budget for the whole batch, as the base parser counts it.
			budget -= sub.nodeCount
			if budget < 0 {
				budget = 0
			}
		}
		if err != nil {
			if p.errorLevel == sqlerrors.IMMEDIATE || p.errorLevel == sqlerrors.RAISE {
				return nil, err
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		expressions = append(expressions, parsed...)
		start = limit
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return expressions, nil
}
