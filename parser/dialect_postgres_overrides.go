package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
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

	var inner exp.Expression
	if p.curr.TokenType == tokens.TABLE {
		// PG `TABLE t` is a valid EXPLAIN target (sql-explain lists SELECT-family statements;
		// TABLE t is the SELECT * FROM t shorthand, engine-verified on PG 16/17). Without this
		// arm the generic statement parse produced a bogus Alias(Column(TABLE) AS t).
		p.advance()
		if p.matchTextSeq("ONLY") {
			// `TABLE ONLY t [*]` narrows/widens inheritance scope; the Table node cannot carry
			// that yet — fail closed rather than under-report relations.
			return nil
		}
		inner = p.parseTable(true, false, nil, false, false, false, false)
		if inner == nil || inner.Kind() != exp.KindTable {
			return nil
		}
		if p.prev.TokenType == tokens.STAR || p.curr.TokenType == tokens.STAR {
			// `TABLE t *` includes inheritance children; parseTable consumes the star as a
			// no-op, so the AST would name only t — fail closed instead of a wrong shape.
			return nil
		}
	} else {
		inner = p.parseStatement()
	}
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
