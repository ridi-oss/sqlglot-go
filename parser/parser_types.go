package parser

import (
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

func dataTypeArgs(dtype any, expressions []exp.Expression, nested bool) exp.Args {
	args := exp.Args{"this": dtype}
	if len(expressions) > 0 {
		args["expressions"] = expressions
	}
	if nested {
		args["nested"] = true
	}
	return args
}

func (p *Parser) parseTypes(checkFunc, schema, allowIdentifiers, withCollation bool) exp.Expression {
	index := p.index
	var this exp.Expression

	if !p.matchSet(typeTokens) {
		// TODO(1d/slice3): UDT identifier fallback via DataType.from_str(udt=True).
		return nil
	}
	typeToken := p.prev.TokenType

	// https://materialize.com/docs/sql/types/map/
	if typeToken == tokens.MAP && p.match(tokens.L_BRACKET) {
		keyType := p.parseTypes(checkFunc, schema, allowIdentifiers, false)
		if !p.match(tokens.FARROW) {
			p.retreat(index)
			return nil
		}
		valueType := p.parseTypes(checkFunc, schema, allowIdentifiers, false)
		if !p.match(tokens.R_BRACKET) {
			p.retreat(index)
			return nil
		}
		return p.expression(exp.DataType(exp.Args{"this": exp.DTypeMap, "expressions": []exp.Expression{keyType, valueType}, "nested": true}), nil, nil)
	}

	nested := nestedTypeTokens[typeToken]
	isStruct := structTypeTokens[typeToken]
	isAggregate := aggregateTypeTokens[typeToken]
	expressions := []exp.Expression(nil)
	maybeFunc := false

	if p.match(tokens.L_PAREN) {
		if isStruct {
			expressions = p.parseCsv(p.parseStructTypes)
		} else if nested {
			expressions = p.parseCsv(func() exp.Expression {
				return p.parseTypes(checkFunc, schema, allowIdentifiers, false)
			})
			if typeToken == tokens.NULLABLE && len(expressions) == 1 {
				this = expressions[0]
				this.Set("nullable", true)
				p.matchRParen(this)
				return this
			}
		} else if enumTypeTokens[typeToken] {
			expressions = p.parseCsv(p.parseEquality)
		} else if isAggregate {
			funcOrIdent := p.parseFunction(nil, true, true, false)
			if funcOrIdent == nil {
				funcOrIdent = p.parseIdVar(false, map[tokens.TokenType]bool{tokens.VAR: true, tokens.ANY: true})
			}
			if funcOrIdent == nil {
				p.retreat(index)
				return nil
			}
			expressions = []exp.Expression{funcOrIdent}
			if p.match(tokens.COMMA) {
				expressions = append(expressions, p.parseCsv(func() exp.Expression {
					return p.parseTypes(checkFunc, schema, allowIdentifiers, false)
				})...)
			}
		} else {
			// TODO(1d): ClickHouse JSON type args and VECTOR expression normalization.
			expressions = p.parseCsv(p.parseTypeSize)
		}

		if !p.match(tokens.R_PAREN) {
			p.retreat(index)
			return nil
		}
		maybeFunc = true
	}

	if nested && p.match(tokens.LT) {
		if isStruct {
			expressions = p.parseCsv(p.parseStructTypes)
		} else {
			expressions = p.parseCsv(func() exp.Expression {
				return p.parseTypes(checkFunc, schema, allowIdentifiers, true)
			})
		}
		if !p.match(tokens.GT) {
			p.raiseError("Expecting >")
		}
		// TODO(1d/slice3): inline constructor suffix STRUCT<..>(..) -> Cast.
	}

	if timestampsTokens[typeToken] {
		if p.matchTextSeq("WITH", "TIME", "ZONE") {
			maybeFunc = false
			tzType := exp.DTypeTimestampTz
			if timesTokens[typeToken] {
				tzType = exp.DTypeTimeTz
			}
			this = p.expression(exp.DataType(dataTypeArgs(tzType, expressions, false)), nil, nil)
		} else if p.matchTextSeq("WITH", "LOCAL", "TIME", "ZONE") {
			maybeFunc = false
			this = p.expression(exp.DataType(dataTypeArgs(exp.DTypeTimestampLtz, expressions, false)), nil, nil)
		} else if p.matchTextSeq("WITHOUT", "TIME", "ZONE") {
			maybeFunc = false
		}
	} else if typeToken == tokens.INTERVAL {
		if p.dialect.ValidIntervalUnits[stringsUpper(p.curr.Text)] {
			unit := p.parseVar(false, nil, true)
			if p.matchTextSeq("TO") {
				unit = p.expression(exp.IntervalSpan(exp.Args{"this": unit, "expression": p.parseVar(false, nil, true)}), nil, nil)
			}
			this = p.expression(exp.DataType(exp.Args{"this": p.expression(exp.Interval(exp.Args{"unit": unit}), nil, nil)}), nil, nil)
		} else {
			this = p.expression(exp.DataType(exp.Args{"this": exp.DTypeInterval}), nil, nil)
		}
	} else if typeToken == tokens.VOID {
		this = p.expression(exp.DataType(exp.Args{"this": exp.DTypeNull}), nil, nil)
	}

	// TODO(1d): check_func typed-literal peek path.
	_ = maybeFunc

	if this == nil {
		if p.matchTextSeq("UNSIGNED") {
			unsignedTypeToken, ok := signedToUnsigned[typeToken]
			if !ok {
				p.raiseError("Cannot convert " + tokens.TypeName(typeToken) + " to unsigned.")
			} else {
				typeToken = unsignedTypeToken
			}
		}

		// NULLABLE without parentheses can be a column (Presto/Trino).
		if typeToken == tokens.NULLABLE && len(expressions) == 0 {
			p.retreat(index)
			return nil
		}

		dtype, ok := exp.DTypeFromName(tokens.TypeName(typeToken))
		if !ok {
			p.retreat(index)
			return nil
		}
		this = p.expression(exp.DataType(dataTypeArgs(dtype, expressions, nested)), nil, nil)
	} else if len(expressions) > 0 {
		this.Set("expressions", expressions)
	}

	// https://materialize.com/docs/sql/types/list/#type-name
	for p.match(tokens.LIST) {
		this = p.expression(exp.DataType(exp.Args{"this": exp.DTypeList, "expressions": []exp.Expression{this}, "nested": true}), nil, nil)
	}

	index = p.index
	matchedArray := p.match(tokens.ARRAY)
	for p.curr.IsValid() {
		datatypeToken := p.prev.TokenType
		matchedLBracket := p.match(tokens.L_BRACKET)
		if (!matchedLBracket && !matchedArray) || (datatypeToken == tokens.ARRAY && p.match(tokens.R_BRACKET)) {
			break
		}
		matchedArray = false
		values := p.parseCsv(p.parseDisjunction)
		if len(values) > 0 && !schema && (!p.dialect.SupportsFixedSizeArrays || datatypeToken == tokens.ARRAY || !p.match(tokens.R_BRACKET, false)) {
			p.retreat(index)
			break
		}
		args := exp.Args{"this": exp.DTypeArray, "expressions": []exp.Expression{this}, "nested": true}
		if len(values) > 0 {
			args["values"] = values
		}
		this = p.expression(exp.DataType(args), nil, nil)
		p.match(tokens.R_BRACKET)
	}

	if withCollation && this.Kind() == exp.KindDataType && p.match(tokens.COLLATE) {
		collate := p.parseIdentifier()
		if collate == nil {
			collate = p.parseColumn()
		}
		this.Set("collate", collate)
	}

	return this
}

func (p *Parser) parseStructTypes() exp.Expression {
	// Simplified vs parser.py:6523: parse a field name plus a following type, or a positional type.
	index := p.index
	name := p.parseIdVar(false, nil)
	if name != nil {
		p.match(tokens.COLON)
		kind := p.parseTypes(false, false, true, false)
		if kind != nil {
			return p.expression(exp.ColumnDef(exp.Args{"this": name, "kind": kind}), nil, nil)
		}
		p.retreat(index)
	}
	return p.parseTypes(false, false, true, false)
}

func (p *Parser) parseTypeSize() exp.Expression {
	this := p.parseType()
	if this == nil {
		return nil
	}
	if this.Kind() == exp.KindColumn && this.Arg("table") == nil {
		this = exp.Var(exp.Args{"this": stringsUpper(this.Name())})
	}
	return p.expression(exp.DataTypeParam(exp.Args{"this": this, "expression": p.parseVar(true, nil, false)}), nil, nil)
}

func (p *Parser) parseCast(strict bool, safe any) exp.Expression {
	this := p.parseAssignment()
	if !p.match(tokens.ALIAS) {
		if p.match(tokens.COMMA) {
			return p.expression(exp.CastToStrType(exp.Args{"this": this, "to": p.parseString()}), nil, nil)
		}
		p.raiseError("Expected AS after CAST")
	}
	// Mirror upstream _parse_cast (parser.py:7863): with_collation=True so that
	// CAST(x AS <type> COLLATE ...) parses instead of hard-erroring on COLLATE.
	to := p.parseTypes(false, false, true, true)
	var default_ exp.Expression
	if p.match(tokens.DEFAULT) {
		default_ = p.parseBitwise()
		p.matchTextSeq("ON", "CONVERSION", "ERROR")
	}
	if to == nil {
		p.raiseError("Expected TYPE after CAST")
	}
	args := exp.Args{"this": this, "to": to, "default": default_, "action": p.parseVarFromOptions(castActions, false)}
	if safe != nil {
		args["safe"] = safe
	}
	return p.buildCast(strict, args)
}

func (p *Parser) buildCast(strict bool, args exp.Args) exp.Expression {
	kind := exp.KindCast
	if !strict {
		kind = exp.KindTryCast
		if p.dialect.TryCastRequiresString != nil {
			args["requires_string"] = *p.dialect.TryCastRequiresString
		}
	}
	return p.Expression(exp.New(kind, args))
}

func (p *Parser) parseDcolon() exp.Expression {
	return p.parseTypes(false, false, true, false)
}

// parseString ports _parse_string + STRING_PARSERS (parser.py:1122-1136, 8519-8523):
// plain STRING -> Literal, and HEREDOC_STRING (postgres `$$...$$`) / RAW_STRING ->
// RawString. NATIONAL_STRING/UNICODE_STRING are not modeled yet (their nodes aren't
// ported), so those still fall through to parsePlaceholder.
func (p *Parser) parseString() exp.Expression {
	if p.match(tokens.STRING) {
		return p.expression(exp.LiteralString(p.prev.Text), &p.prev, nil)
	}
	if p.match(tokens.HEREDOC_STRING) || p.match(tokens.RAW_STRING) {
		return p.expression(exp.RawString(exp.Args{"this": p.prev.Text}), &p.prev, nil)
	}
	return p.parsePlaceholder()
}

func (p *Parser) parseVar(anyToken bool, toks map[tokens.TokenType]bool, upper bool) exp.Expression {
	if (anyToken && p.advanceAny(false) != nil) || p.match(tokens.VAR) || (toks != nil && p.matchSet(toks)) {
		text := p.prev.Text
		if upper {
			text = stringsUpper(text)
		}
		return p.expression(exp.Var(exp.Args{"this": text}), nil, nil)
	}
	return p.parsePlaceholder()
}

func (p *Parser) parseVarOrString(upper bool) exp.Expression {
	// Mirror upstream `self._parse_string() or self._parse_var(...)`: short-circuit
	// so parseVar's advanceAny doesn't eagerly consume the token after a matched
	// string literal (e.g. the FROM in EXTRACT('lit' FROM x)).
	if s := p.parseString(); s != nil {
		return s
	}
	return p.parseVar(true, nil, upper)
}

func (p *Parser) parseBracket(this exp.Expression) exp.Expression {
	if !p.match(tokens.L_BRACKET) {
		return this
	}
	expressions := p.parseCsv(p.parseDisjunction)
	if !p.match(tokens.R_BRACKET) {
		p.raiseError("Expected ]")
	}
	if this == nil {
		this = exp.Array(exp.Args{"expressions": expressions})
	} else {
		this = p.expression(exp.Bracket(exp.Args{"this": this, "expressions": expressions}), nil, this.PopComments())
	}
	p.addComments(this)
	return p.parseBracket(this)
}
