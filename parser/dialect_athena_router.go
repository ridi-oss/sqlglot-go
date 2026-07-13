package parser

import (
	"strings"

	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func (p *Parser) isAthenaRouter() bool {
	return p.dialect != nil && strings.EqualFold(p.dialect.Name, "athena")
}

// athenaSubParser mirrors AthenaParser's Hive/Trino dispatch in parsers/athena.py:24-74. The
// Athena tokenizer has already classified the statement; this seam only consumes its leading
// HIVE_TOKEN_STREAM marker. Query statements use a concrete Trino dialect with Athena's parser
// class overlay so parser flags continue to see Trino.
func (p *Parser) athenaSubParser(rawTokens []tokens.Token) (*Parser, []tokens.Token) {
	var subParser *Parser
	if len(rawTokens) > 0 && rawTokens[0].TokenType == tokens.HIVE_TOKEN_STREAM {
		subParser = NewWithErrorLevel(dialects.Hive(), p.errorLevel)
		rawTokens = rawTokens[1:]
	} else {
		subParser = newWithErrorLevelAndOverrideName(dialects.Trino(), p.errorLevel, "athena")
	}

	subParser.errorMessageContext = p.errorMessageContext
	subParser.maxErrors = p.maxErrors
	subParser.maxNodes = p.maxNodes
	return subParser, rawTokens
}

func (p *Parser) preserveAthenaErrors(subParser *Parser) {
	p.errors = append([]*sqlerrors.ParseError(nil), subParser.Errors()...)
}

func (p *Parser) parseAthena(rawTokens []tokens.Token, sql string) ([]exp.Expression, error) {
	p.Reset()
	subParser, routedTokens := p.athenaSubParser(rawTokens)
	expressions, err := subParser.Parse(routedTokens, sql)
	p.preserveAthenaErrors(subParser)
	return expressions, err
}

func (p *Parser) parseIntoAthena(rawTokens []tokens.Token, sql string, into exp.Kind) ([]exp.Expression, error) {
	p.Reset()
	subParser, routedTokens := p.athenaSubParser(rawTokens)
	expressions, err := subParser.ParseInto(routedTokens, sql, into)
	p.preserveAthenaErrors(subParser)
	return expressions, err
}
