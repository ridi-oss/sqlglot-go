package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
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

// parseUserDefinedType ports the base _parse_user_defined_type (parser.py:6231-6237) and
// the postgres override (parsers/postgres.py:339-347). Base joins the dotted parts into a
// single string and re-parses it via DataType.from_str(name, dialect, udt=True): on the UDT
// fallback (the name doesn't re-parse as a real type) "kind" ends up that plain string.
// Postgres instead builds an Identifier/Dot chain and calls DataType.build(chain, udt=True)
// directly, so "kind" is an expression that preserves each part's quoting - needed for
// `CAST(5 AS "MyType")`/`CAST(5 AS "MySchema"."MyType")` to round-trip exactly (dataTypeSQL's
// USERDEFINED branch already renders "kind" generically via sqlKey, whether it's a string or
// an expression).
func (p *Parser) parseUserDefinedType(identifier exp.Expression) exp.Expression {
	if p.dialect.Name == "postgres" {
		udtType := identifier
		for p.match(tokens.DOT) {
			if part := p.parseIdVar(true, nil); part != nil {
				udtType = p.expression(exp.Dot(exp.Args{"this": udtType, "expression": part}), nil, nil)
			}
		}
		result, err := exp.DataTypeBuild(udtType, "", true, false, nil)
		if err != nil {
			return nil
		}
		return result
	}

	typeName := identifier.Name()
	for p.match(tokens.DOT) {
		if tok := p.advanceAny(false); tok != nil {
			typeName += "." + tok.Text
		}
	}
	result, err := exp.DataTypeBuild(typeName, p.dialect.Name, true, false, nil)
	if err != nil {
		return nil
	}
	return result
}

func (p *Parser) parseTypes(checkFunc, schema, allowIdentifiers, withCollation bool) exp.Expression {
	if parser := p.typeParserOverride(); parser != nil {
		return parser(p, checkFunc, schema, allowIdentifiers, withCollation)
	}
	return p.parseTypesBase(checkFunc, schema, allowIdentifiers, withCollation)
}

func (p *Parser) parseTypesBase(checkFunc, schema, allowIdentifiers, withCollation bool) exp.Expression {
	index := p.index
	var this exp.Expression
	var typeToken tokens.TokenType // zero value is "no token", since real TokenTypes start at 1

	// mysql adds TokenType.SET to TYPE_TOKENS/ENUM_TYPE_TOKENS (mysql.py:280-287); rather
	// than mutating the global typeTokens (which would leak SET-as-type into base/postgres),
	// match it inline for the mysql dialect only, then treat it as an enum below.
	if p.matchSet(typeTokens) || (p.dialect.Name == "mysql" && p.match(tokens.SET)) {
		typeToken = p.prev.TokenType
	} else {
		var identifier exp.Expression
		if allowIdentifiers {
			identifier = p.parseIdVar(false, map[tokens.TokenType]bool{tokens.VAR: true})
		}
		if identifier == nil || identifier.Kind() != exp.KindIdentifier {
			return nil
		}

		subTokens, tokErr := p.dialect.NewTokenizer().Tokenize(identifier.Name())
		if tokErr != nil {
			subTokens = nil
		}

		// mysql adds TokenType.SET to TYPE_TOKENS (mysql.py:280-287) but we keep it out of
		// the global typeTokens map (see the direct-match note above), so recognize it inline
		// here too - otherwise a back-ticked SET type, e.g. CAST(x AS `SET`('a', 'b')), fails
		// to re-parse as a type and falls through to the UDT/retreat path.
		firstIsType := len(subTokens) > 0 &&
			(typeTokens[subTokens[0].TokenType] ||
				(p.dialect.Name == "mysql" && subTokens[0].TokenType == tokens.SET))
		if firstIsType {
			typeToken = subTokens[0].TokenType
			if len(subTokens) > 1 {
				// A quoted multi-word type name (e.g. `"double precision"`), re-parsed
				// without udt=True (parser.py:6264: `exp.DataType.from_str(identifier.name,
				// dialect=self.dialect)`), so upstream lets a ParseError here propagate as a
				// hard failure rather than falling back to a UDT. This port degrades to a
				// benign nil (-> the caller's normal retreat/fallback path) instead, which is
				// safer and not corpus-visible.
				result, err := exp.DataTypeBuild(identifier.Name(), p.dialect.Name, false, true, nil)
				if err != nil {
					return nil
				}
				return result
			}
			// len(subTokens) == 1: fall through using the discovered typeToken, exactly as
			// if it had been matched directly (parser.py:6262-6263).
		} else if p.dialect.SupportsUserDefinedTypes {
			this = p.parseUserDefinedType(identifier)
		} else {
			p.retreat(p.index - 1)
			return nil
		}
	}

	if typeToken == tokens.PSEUDO_TYPE {
		return p.expression(exp.PseudoType(exp.Args{"this": stringsUpper(p.prev.Text)}), nil, nil)
	}

	if typeToken == tokens.OBJECT_IDENTIFIER {
		return p.expression(exp.ObjectIdentifier(exp.Args{"this": stringsUpper(p.prev.Text)}), nil, nil)
	}

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
	// values captures a trailing inline constructor, e.g. ARRAY<INT>[1, 2] or
	// STRUCT<a INT>(1) (parser.py:6375-6381); nil means "no constructor suffix".
	var values []exp.Expression

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
		} else if enumTypeTokens[typeToken] || (p.dialect.Name == "mysql" && typeToken == tokens.SET) {
			expressions = p.parseCsv(p.parseEquality)
		} else if isAggregate {
			funcOrIdent := p.parseFunction(nil, true, true, false)
			if funcOrIdent == nil {
				funcOrIdent = p.parseIdVar(false, map[tokens.TokenType]bool{tokens.VAR: true, tokens.ANY: true})
			}
			if funcOrIdent == nil {
				// Upstream returns bare (parser.py:6331-6332), without retreating.
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

		// Inline constructor suffix (parser.py:6375-6381): ARRAY<INT>[1, 2] /
		// STRUCT<a INT>(1, 'foo') capture their values here and get wrapped in a Cast
		// below. An empty pair on a struct (STRUCT<..>()) is not a constructor, so we
		// retreat past the opening bracket and leave it for the caller.
		if p.match(tokens.L_BRACKET) || p.match(tokens.L_PAREN) {
			values = p.parseCsv(p.parseDisjunction)
			if len(values) == 0 && isStruct {
				values = nil
				p.retreat(p.index - 1)
			} else if !p.match(tokens.R_BRACKET) {
				p.match(tokens.R_PAREN)
			}
		}
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

	if maybeFunc && checkFunc {
		index2 := p.index
		peek := p.parseString()
		if peek == nil {
			p.retreat(index)
			return nil
		}
		p.retreat(index2)
	}

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
		// Upstream's scalar branch (parser.py:6429-6433) always sets nested=nested
		// (True or False), unlike the TZ/interval/null branches which omit it. The
		// shared dataTypeArgs helper drops a false nested, so build args inline here
		// to preserve the explicit nested=False that repr()/ToS() expects.
		scalarArgs := exp.Args{"this": dtype, "nested": nested}
		if len(expressions) > 0 {
			scalarArgs["expressions"] = expressions
		}
		this = p.expression(exp.DataType(scalarArgs), nil, nil)

		// Empty arrays/structs are allowed (parser.py:6436-6438): a captured inline
		// constructor becomes CAST(<ARRAY|STRUCT>(values) AS <type>). parseType's
		// KindCast fast-path (parser.go) then returns this straight through parseColumnOps.
		if values != nil {
			ctor := exp.Array(exp.Args{"expressions": values})
			if isStruct {
				ctor = exp.Struct(exp.Args{"expressions": values})
			}
			this = p.expression(exp.Cast(exp.Args{"this": ctor, "to": this}), nil, nil)
		}
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
		values = p.parseCsv(p.parseDisjunction)
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

// parseStructTypes ports _parse_struct_types (parser.py:6523-6551). Both call sites in
// parseTypes pass type_required=True, so that parameter is inlined rather than threaded
// through as an argument.
func (p *Parser) parseStructTypes() exp.Expression {
	index := p.index

	var this exp.Expression
	if p.curr.IsValid() && p.next.IsValid() && typeTokens[p.curr.TokenType] && typeTokens[p.next.TokenType] {
		// Handles cases like `STRUCT<list ARRAY<...>>` where the field name is itself a type
		// token: without this, "list" would be parsed as a (positional) type and crash.
		this = p.parseIdVar(true, nil)
	} else {
		this = p.parseType(false, true)
		if this == nil {
			this = p.parseIdVar(true, nil)
		}
	}

	p.match(tokens.COLON)

	if (this == nil || this.Kind() != exp.KindDataType) && !p.matchSet(typeTokens, false) {
		p.retreat(index)
		return p.parseTypes(false, false, true, false)
	}

	return p.parseColumnDef(this)
}

func (p *Parser) parseTypeSize() exp.Expression {
	this := p.parseType(true, false)
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
	} else if exp.IsType(to, exp.DTypeChar) && p.match(tokens.CHARACTER_SET) {
		// parser.py:7900-7901 (elif branch): CAST(x AS CHAR CHARACTER SET <cs>) captures the
		// charset via the synthetic CHARACTER_SET data type; the generator renders it as
		// `CHAR CHARACTER SET <kind>` (generator/sql.go:1472-1473). FORMAT/COMMA and the
		// isinstance(to, Identifier) elif branches (parser.py:7870-7897) are deferred - no
		// corpus case in this slice exercises them.
		to = exp.DTypeCharacterSet.IntoExpr(exp.Args{"kind": p.parseVarOrString(false)})
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

// parseString ports _parse_string + STRING_PARSERS (parser.py:1122-1141, 8519-8523):
// plain STRING -> Literal, HEREDOC_STRING (postgres `$$...$$`) / RAW_STRING -> RawString,
// NATIONAL_STRING -> National, and UNICODE_STRING (e.g. Presto's `U&'...'`) -> UnicodeString.
// The UESCAPE clause of UNICODE_STRING (parser.py:1135-1140) is deferred - `escape` stays unset.
func (p *Parser) parseString() exp.Expression {
	if p.match(tokens.STRING) {
		return p.expression(exp.LiteralString(p.prev.Text), &p.prev, nil)
	}
	if p.match(tokens.HEREDOC_STRING) || p.match(tokens.RAW_STRING) {
		return p.expression(exp.RawString(exp.Args{"this": p.prev.Text}), &p.prev, nil)
	}
	if p.match(tokens.NATIONAL_STRING) {
		return p.expression(exp.National(exp.Args{"this": p.prev.Text}), &p.prev, nil)
	}
	if p.match(tokens.UNICODE_STRING) {
		return p.expression(exp.UnicodeString(exp.Args{"this": p.prev.Text}), &p.prev, nil)
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
	expressions := p.parseCsv(p.parseBracketKeyValue)
	if !p.match(tokens.R_BRACKET) {
		p.raiseError("Expected ]")
	}
	if this == nil {
		this = exp.Array(exp.Args{"expressions": expressions})
	} else if stringsUpper(this.Name()) == "ARRAY" {
		// _parse_bracket's ARRAY_CONSTRUCTORS swap (parser.py:7713-7721, table at :787-790): a
		// bracket subscripting a bare "ARRAY" reference (e.g. `ARRAY[1, 2, 3]`, tokenized as an
		// ordinary column/var since ARRAY is ID_VAR_TOKENS-eligible) is really an array
		// literal, not indexing - build exp.Array and discard `this`. Matches upstream's own
		// name-based check exactly (no extra unquoted/no-table guard), including its
		// limitation: a genuine column named "array" would be misread the same way upstream
		// misreads it. duckdb's "LIST" ARRAY_CONSTRUCTORS entry isn't modeled (out of base/
		// mysql/postgres scope).
		this = exp.Array(exp.Args{"expressions": expressions})
	} else {
		this = p.expression(exp.Bracket(exp.Args{"this": this, "expressions": expressions}), nil, this.PopComments())
	}
	p.addComments(this)
	return p.parseBracket(this)
}

// parseBracketKeyValue ports _parse_bracket_key_value (parser.py:7655-7657). The is_map
// parameter (used for MAP_KEYS_ARE_ARBITRARY_EXPRESSIONS dialects building key:value
// props) is not modeled - no dialect in this port sets that flag, so it's inlined away.
func (p *Parser) parseBracketKeyValue() exp.Expression {
	return p.parseSlice(p.parseAlias(p.parseDisjunction(), true))
}

// parseSlice ports _parse_slice (parser.py:7745-7754): the `start:end:step` triple inside
// a bracket subscript, e.g. x[1:2], x[:2], x[1:], x[:], x[-4:-1]. The DASH+COLON special
// case handles a bare `-` end/step sentinel meaning -1 (e.g. duckdb `arr[:-:-1]` ->
// `arr[:-1:-1]`, tests/dialects/test_duckdb.py:23).
func (p *Parser) parseSlice(this exp.Expression) exp.Expression {
	if !p.match(tokens.COLON) {
		return this
	}

	var end exp.Expression
	if p.matchPair(tokens.DASH, tokens.COLON, false) {
		p.advance()
		end = exp.Neg(exp.Args{"this": exp.LiteralNumber("1")})
	} else {
		end = p.parseAssignment()
	}
	var step exp.Expression
	if p.match(tokens.COLON) {
		step = p.parseUnary()
	}
	return p.expression(exp.Slice(exp.Args{"this": this, "expression": end, "step": step}), nil, nil)
}
