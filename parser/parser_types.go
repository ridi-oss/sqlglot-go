package parser

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
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
		// divergence (DEVIATIONS §1.10): `pg_catalog.<builtin>` IS the builtin. pg_catalog is
		// PostgreSQL's system schema, so `pg_catalog.int4` is definitionally `int4` (real PG
		// 17.6: pg_typeof('5'::pg_catalog.int4) = pg_typeof('5'::int4) = integer). Resolve a
		// pg_catalog-qualified real builtin to the same node the bare spelling produces, so a
		// consumer classifying the cast target sees the builtin. Pinned upstream is
		// over-conservative here (leaves it USER-DEFINED). A tail that is not a real pg_catalog
		// name (`pg_catalog.myt`, `pg_catalog.integer`) or any other schema (`public.myt`,
		// `myschema.int4`) stays USER-DEFINED, matching real PG.
		if builtin := p.resolvePgCatalogBuiltin(udtType); builtin != nil {
			return builtin
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

// pgCatalogTypeNames is the pinned set of real PostgreSQL `pg_catalog` type names — the
// authoritative source of truth for the §1.10 divergence. It is the base/pseudo/range/multirange
// type set (typtype in b/p/r/m, non-array) of pg_catalog, captured from PostgreSQL 17.6:
//
//	SELECT typname FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
//	WHERE n.nspname = 'pg_catalog' AND t.typtype IN ('b','p','r','m') AND typname NOT LIKE '\_%';
//
// Membership here — NOT the type parser's generic keyword recognition — decides whether a
// `pg_catalog.<name>` qualifier names a real builtin. The tokenizer knows many aliases that are
// NOT pg_catalog names (`integer`/`bigint`/`boolean`/`decimal`/`smallint`/`real`/`double` are
// grammar spellings; `tinyint`/`datetime`/`mediumint`/`nvarchar` are other dialects'), and real
// PG rejects `pg_catalog.integer`, `pg_catalog.tinyint`, etc. — the catalog names are
// `int4`/`int8`/`bool`/`numeric`/`int2`/`float4`/`float8`, not their aliases. Keying on this
// list avoids that false-resolution.
var pgCatalogTypeNames = func() map[string]struct{} {
	names := strings.Fields(`
		aclitem any anyarray anycompatible anycompatiblearray anycompatiblemultirange
		anycompatiblenonarray anycompatiblerange anyelement anyenum anymultirange anynonarray
		anyrange bit bool box bpchar bytea char cid cidr circle cstring date daterange
		datemultirange event_trigger fdw_handler float4 float8 gtsvector index_am_handler inet int2
		int2vector int4 int4range int4multirange int8 int8range int8multirange internal interval
		json jsonb jsonpath language_handler line lseg macaddr macaddr8 money name numeric numrange
		nummultirange oid oidvector path pg_brin_bloom_summary pg_brin_minmax_multi_summary
		pg_ddl_command pg_dependencies pg_lsn pg_mcv_list pg_ndistinct pg_node_tree pg_snapshot point
		polygon record refcursor regclass regcollation regconfig regdictionary regnamespace regoper
		regoperator regproc regprocedure regrole regtype table_am_handler text tid time timestamp
		timestamptz timetz trigger tsm_handler tsmultirange tsquery tsrange tstzrange tstzmultirange
		tsvector txid_snapshot unknown uuid varbit varchar void xid xid8 xml`)
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}()

// pgCatalogSemanticMismatch are real pg_catalog names that ARE modeled as a bare builtin but whose
// bare SQL spelling is NOT the same type, so resolving them would silently change semantics — they
// stay USER-DEFINED instead (verified against PostgreSQL 17.6):
//   - char: `pg_catalog.char` is the 1-byte "char" (OID 18); the bare CHAR keyword is character(1)
//     / bpchar (OID 1042) — a different type (`65::pg_catalog.char` = 'A', `65::char` = '6').
//   - bit:  `pg_catalog.bit` has no implicit length; the bare BIT keyword is bit(1), which truncates
//     (`'101'::pg_catalog.bit` = '101', `'101'::bit` = '1').
//
// Both remain in pgCatalogTypeNames (they are real pg_catalog names) but are excluded here so the
// qualified form round-trips losslessly as USER-DEFINED, matching upstream's conservative behavior.
var pgCatalogSemanticMismatch = map[string]struct{}{
	"char": {},
	"bit":  {},
}

// resolvePgCatalogBuiltin implements the DEVIATIONS §1.10 correctness divergence (postgres
// only): a two-part `pg_catalog.<name>` type name whose tail is a real pg_catalog builtin is
// returned as the exact node the bare `<name>` spelling produces — a builtin DataType
// (int4->INT, int8->BIGINT, ...), an ObjectIdentifier (oid, regclass, reg*), or a PseudoType
// (cstring) — instead of the USER-DEFINED node upstream leaves it as. Membership is decided by
// the pinned pgCatalogTypeNames set (real pg_catalog names), not the type parser's generic
// keyword recognition, so tokenizer aliases that are not pg_catalog names (`integer`, `tinyint`,
// `nvarchar`, ...) stay USER-DEFINED — matching real PG, which rejects `pg_catalog.integer` etc.
//
// Returns nil (caller keeps the USER-DEFINED build) when: the type name is not a two-part Dot
// (a 3-part `a.pg_catalog.int4` stays USER-DEFINED); the schema part is not `pg_catalog` under
// PG's identifier folding; the tail (folded the same way) is not in the pinned set; or the tail
// is a real pg_catalog name the port does not model as a builtin (e.g. `macaddr`/`varbit`, which
// resolve back to USER-DEFINED) — those keep the qualified USER-DEFINED so the schema qualifier
// is preserved.
func (p *Parser) resolvePgCatalogBuiltin(udtType exp.Expression) exp.Expression {
	if udtType == nil || udtType.Kind() != exp.KindDot {
		return nil
	}
	schema := udtType.This()
	tail := udtType.Expr()
	if schema == nil || tail == nil ||
		schema.Kind() != exp.KindIdentifier || tail.Kind() != exp.KindIdentifier {
		return nil
	}
	// The schema part must be `pg_catalog` under PostgreSQL's identifier folding: an unquoted
	// name folds to lowercase (so `PG_CATALOG` is the system schema), but a quoted name is
	// literal (`"PG_CATALOG"` is a different, case-sensitive schema that real PG rejects here).
	if schemaQuoted, _ := schema.Arg("quoted").(bool); schemaQuoted {
		if schema.Name() != "pg_catalog" {
			return nil
		}
	} else if !strings.EqualFold(schema.Name(), "pg_catalog") {
		return nil
	}
	// Fold the tail the same way PG does: an unquoted name folds to lowercase; a quoted name is
	// literal, so `pg_catalog."int4"` (already lowercase) matches the catalog name but
	// `pg_catalog."INT4"` does not (real PG rejects the latter — "type does not exist").
	name := tail.Name()
	if tailQuoted, _ := tail.Arg("quoted").(bool); !tailQuoted {
		// ASCII-only fold (stringsLower), matching PostgreSQL's identifier folding rather than Go's
		// full-Unicode strings.ToLower — see DEVIATIONS §1.1. For the all-ASCII catalog names this is
		// identical to strings.ToLower, but it will not over-fold a non-ASCII spelling PG rejects.
		name = stringsLower(name)
	}
	if _, ok := pgCatalogTypeNames[name]; !ok {
		return nil
	}
	// A real pg_catalog name whose bare spelling is a semantically different type (char, bit) stays
	// USER-DEFINED — resolving it would silently change the type (see pgCatalogSemanticMismatch).
	if _, mismatch := pgCatalogSemanticMismatch[name]; mismatch {
		return nil
	}
	// Route the folded name through the same bare-type resolution `int4`/`oid`/`cstring` take,
	// and return whatever node it yields: a builtin DataType, an ObjectIdentifier (oid/reg*), or
	// a PseudoType (cstring). If the port does not model this pg_catalog type as a builtin (the
	// tail resolves back to a USER-DEFINED DataType, e.g. `macaddr`/`varbit`), keep the qualified
	// name USER-DEFINED so the schema qualifier is preserved.
	built, err := exp.DataTypeBuild(name, p.dialect.Name, false, false, nil)
	if err != nil || built == nil {
		return nil
	}
	switch built.Kind() {
	case exp.KindObjectIdentifier, exp.KindPseudoType:
		// oid/reg*/cstring never take a type modifier (real PG: `pg_catalog.oid(5)` -> "type modifier
		// is not allowed for type"). A pending `(` here is such a modifier; since ObjectIdentifier /
		// PseudoType cannot carry `expressions`, the trailing (...) parseTypesBase would attach is
		// silently dropped on output. Keep the qualified name USER-DEFINED instead, so the modifier is
		// preserved on round-trip and an engine-invalid form is not laundered into a valid-looking one.
		if p.curr.TokenType == tokens.L_PAREN {
			return nil
		}
		return built
	case exp.KindDataType:
		if built.Arg("this") != exp.DTypeUserDefined {
			return built
		}
	}
	return nil
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
	} else if exp.IsType(to, exp.DTypeChar) && (p.match(tokens.CHARACTER_SET) || p.matchTextSeq("CHARACTER", "SET")) {
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
