package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

var postgresExplainBooleanOptions = map[string]bool{
	"ANALYZE":      true,
	"VERBOSE":      true,
	"COSTS":        true,
	"SETTINGS":     true,
	"GENERIC_PLAN": true,
	"BUFFERS":      true,
	"WAL":          true,
	"TIMING":       true,
	"SUMMARY":      true,
	"MEMORY":       true,
}

var postgresExplainBooleanValues = map[string]bool{
	"TRUE":  true,
	"FALSE": true,
	"ON":    true,
	"OFF":   true,
	"1":     true,
	"0":     true,
}

var postgresExplainSerializeValues = map[string]bool{
	"NONE":   true,
	"TEXT":   true,
	"BINARY": true,
}

var postgresExplainFormatValues = map[string]bool{
	"TEXT": true,
	"XML":  true,
	"JSON": true,
	"YAML": true,
}

var postgresExplainLegacyOptions = map[string]bool{
	"ANALYZE": true,
	"VERBOSE": true,
}

// parsePostgresExplain is the pg-explain extension beyond pinned upstream, which still
// parses Postgres EXPLAIN statements as raw Commands.
func (p *Parser) parsePostgresExplain() exp.Expression {
	start := p.prev
	if stringsUpper(start.Text) != "EXPLAIN" {
		return p.parseDescribe()
	}

	if structured := p.tryParse(p.parsePostgresExplainStructured, false); structured != nil {
		return structured
	}
	return p.parseAsCommand(start)
}

func (p *Parser) parsePostgresExplainStructured() exp.Expression {
	wrapped := p.match(tokens.L_PAREN)
	options := []exp.Expression{}

	if wrapped {
		if !p.curr.IsValid() || p.match(tokens.R_PAREN, false) {
			return nil
		}

		for {
			option := p.parsePostgresExplainOption()
			if option == nil {
				return nil
			}
			options = append(options, option)

			if p.match(tokens.R_PAREN) {
				break
			}
			if !p.match(tokens.COMMA) || !p.curr.IsValid() || p.match(tokens.R_PAREN, false) {
				return nil
			}
		}
	} else {
		if p.matchTexts(map[string]bool{"ANALYZE": true}) {
			options = append(options, p.postgresExplainOption("ANALYZE", ""))
		}
		if p.matchTexts(map[string]bool{"VERBOSE": true}) {
			options = append(options, p.postgresExplainOption("VERBOSE", ""))
		}
		if p.matchTexts(postgresExplainLegacyOptions, false) {
			return nil
		}
	}

	inner := p.parseStatement()
	if inner == nil || p.curr.IsValid() {
		return nil
	}

	return p.expression(exp.Describe(exp.Args{
		"this":        inner,
		"kind":        "EXPLAIN",
		"expressions": options,
		"wrapped":     wrapped,
	}), nil, nil)
}

func (p *Parser) parsePostgresExplainOption() exp.Expression {
	if !p.curr.IsValid() || p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER {
		return nil
	}

	name := stringsUpper(p.curr.Text)
	var allowedValues map[string]bool
	valueRequired := false

	switch {
	case postgresExplainBooleanOptions[name]:
		allowedValues = postgresExplainBooleanValues
	case name == "SERIALIZE":
		allowedValues = postgresExplainSerializeValues
		valueRequired = true
	case name == "FORMAT":
		allowedValues = postgresExplainFormatValues
		valueRequired = true
	default:
		return nil
	}
	p.advance()

	value := ""
	if p.curr.TokenType != tokens.STRING && p.curr.TokenType != tokens.IDENTIFIER && p.matchTexts(allowedValues) {
		value = stringsUpper(p.prev.Text)
	} else if valueRequired {
		return nil
	}

	return p.postgresExplainOption(name, value)
}

func (p *Parser) postgresExplainOption(name, value string) exp.Expression {
	args := exp.Args{"this": exp.Var(exp.Args{"this": name})}
	if value != "" {
		args["expression"] = exp.Var(exp.Args{"this": value})
	}
	return p.expression(exp.CopyParameter(args), nil, nil)
}
