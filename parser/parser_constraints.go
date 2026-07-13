package parser

import (
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// constraintParsers etc. below are declared without initializers and populated inside
// init() (rather than via `var x = map[string]func(*Parser) exp.Expression{...}` literals)
// on purpose: a map literal whose VALUES are closures makes the Go compiler's var-
// initializer dependency analysis walk into those closures' bodies, and since they
// transitively (through the whole recursive-descent call graph) call back into
// constraintParsers()/constraintParserKeys() - which read these same package vars - that
// gets flagged as an initialization cycle, even though nothing is actually invoked at init
// time (the closures are just stored, not called). Plain assignment statements inside
// init() aren't subject to that analysis, so the map contents are built there instead.
var (
	// constraintParsers/mysqlConstraintParsers port CONSTRAINT_PARSERS (parser.py:1343-1406)
	// + parsers/mysql.py:243-251. The base registry includes the column/schema constraints
	// needed by the fidelity corpus, including COMPRESS/EXCLUDE/FORMAT/INLINE/LIKE/TITLE/
	// UPPERCASE; mysql extends it with FULLTEXT/INDEX/KEY/SPATIAL/ZEROFILL/INVISIBLE.
	// The remaining upstream base keys (CLUSTERED/NONCLUSTERED/ENCODE/EPHEMERAL/PERIOD/TTL/
	// WITH/BUCKET/TRUNCATE) stay deferred.
	constraintParsers      map[string]func(*Parser) exp.Expression
	constraintParserKeySet map[string]bool

	mysqlConstraintParsers      map[string]func(*Parser) exp.Expression
	mysqlConstraintParserKeySet map[string]bool

	// schemaUnnamedConstraints/mysqlSchemaUnnamedConstraints port the base subset of
	// SCHEMA_UNNAMED_CONSTRAINTS (parser.py:1458-1468) that this port has parsers for
	// (CHECK/EXCLUDE/FOREIGN KEY/LIKE/PRIMARY KEY/UNIQUE), plus mysql's FULLTEXT/INDEX/
	// KEY/SPATIAL (parsers/mysql.py:265-271). PERIOD/BUCKET/TRUNCATE stay deferred.
	schemaUnnamedConstraints      map[string]bool
	mysqlSchemaUnnamedConstraints map[string]bool
)

func init() {
	constraintParsers = map[string]func(*Parser) exp.Expression{
		"AUTOINCREMENT":  func(p *Parser) exp.Expression { return p.parseAutoIncrement() },
		"AUTO_INCREMENT": func(p *Parser) exp.Expression { return p.parseAutoIncrement() },
		"CASESPECIFIC": func(p *Parser) exp.Expression {
			return p.expression(exp.CaseSpecificColumnConstraint(exp.Args{"not_": false}), nil, nil)
		},
		"CHARACTER SET": func(p *Parser) exp.Expression {
			return p.expression(exp.CharacterSetColumnConstraint(exp.Args{"this": p.parseVarOrString(false)}), nil, nil)
		},
		"CHECK": func(p *Parser) exp.Expression { return p.parseCheckConstraint() },
		"COLLATE": func(p *Parser) exp.Expression {
			this := p.parseIdentifier()
			if this == nil {
				this = p.parseColumn()
			}
			return p.expression(exp.CollateColumnConstraint(exp.Args{"this": this}), nil, nil)
		},
		"COMMENT": func(p *Parser) exp.Expression {
			return p.expression(exp.CommentColumnConstraint(exp.Args{"this": p.parseString()}), nil, nil)
		},
		"COMPRESS": func(p *Parser) exp.Expression { return p.parseCompress() },
		"DEFAULT": func(p *Parser) exp.Expression {
			// A bare CURRENT_TIMESTAMP-family default value (e.g. mysql's `DEFAULT
			// CURRENT_TIMESTAMP`) needs the small local NO_PAREN_FUNCTIONS workaround below
			// (see parseNoParenCurrentFunc) before falling back to the ordinary expression
			// parser, which - lacking that support (parser.go:1952-1955 TODO(1d)) - would
			// otherwise leave it as a bare column reference instead of a callable function.
			this := p.parseNoParenCurrentFunc()
			if this == nil {
				this = p.parseBitwise()
			}
			return p.expression(exp.DefaultColumnConstraint(exp.Args{"this": this}), nil, nil)
		},
		"EXCLUDE": func(p *Parser) exp.Expression {
			return p.expression(exp.ExcludeColumnConstraint(exp.Args{"this": p.parseIndexParams()}), nil, nil)
		},
		"FOREIGN KEY": func(p *Parser) exp.Expression { return p.parseForeignKey() },
		"FORMAT": func(p *Parser) exp.Expression {
			return p.expression(exp.DateFormatColumnConstraint(exp.Args{"this": p.parseVarOrString(false)}), nil, nil)
		},
		"GENERATED": func(p *Parser) exp.Expression { return p.parseGeneratedAsIdentity() },
		"IDENTITY":  func(p *Parser) exp.Expression { return p.parseAutoIncrement() },
		"INLINE":    func(p *Parser) exp.Expression { return p.parseInline() },
		"LIKE":      func(p *Parser) exp.Expression { return p.parseCreateLike() },
		"NOT":       func(p *Parser) exp.Expression { return p.parseNotConstraint() },
		"NULL": func(p *Parser) exp.Expression {
			return p.expression(exp.NotNullColumnConstraint(exp.Args{"allow_null": true}), nil, nil)
		},
		// "ON" only ports the ON UPDATE <func> half of parser.py:1384-1390's lambda: the
		// other half (bare `ON <id>` -> exp.OnProperty) has no Kind in this port (out of
		// scope), so a non-UPDATE "ON" simply returns nil, which parseColumnConstraint's
		// caller retreats past.
		"ON": func(p *Parser) exp.Expression {
			index := p.index
			if p.match(tokens.UPDATE) {
				// `this` is required on OnUpdateColumnConstraint. The common no-paren value
				// form (ON UPDATE CURRENT_TIMESTAMP) needs the small local NO_PAREN_FUNCTIONS
				// workaround below (parseNoParenCurrentFunc) before falling back to
				// parseFunction, which - lacking that support (parser.go:1952-1955 TODO(1d)) -
				// returns nil there. Building the node with a nil `this` would fail validation
				// - swallowed as a Command by CREATE's tryParse, but a hard error on the
				// non-tryParse ALTER MODIFY/CHANGE path. So if even the workaround comes up
				// empty, retreat and decline the constraint, degrading gracefully to a Command
				// (matching CREATE's existing behavior for this deferred case).
				this := p.parseNoParenCurrentFunc()
				if this == nil {
					this = p.parseFunction(nil, false, true, false)
				}
				if this != nil {
					return p.expression(exp.OnUpdateColumnConstraint(exp.Args{"this": this}), nil, nil)
				}
			}
			p.retreat(index)
			return nil
		},
		// "PATH" ports parser.py:1391 (exp.PathColumnConstraint(this=_parse_string())). It is
		// reached generically here — via parseColumnConstraint's loop — so it composes with any
		// following constraints (e.g. XMLTABLE `COLUMNS id INT PATH '@id' NOT NULL`), matching
		// upstream. The only base/mysql/postgres surface that uses it is XMLTABLE's COLUMNS list
		// (parseXMLTable -> parseFieldDef), but registering it here rather than special-casing that
		// call site keeps the constraint loop the single source of truth for column constraints.
		"PATH": func(p *Parser) exp.Expression {
			return p.expression(exp.PathColumnConstraint(exp.Args{"this": p.parseString()}), nil, nil)
		},
		"PRIMARY KEY": func(p *Parser) exp.Expression { return p.parsePrimaryKey(false, false) },
		"REFERENCES":  func(p *Parser) exp.Expression { return p.parseReferences(false) },
		"TITLE": func(p *Parser) exp.Expression {
			return p.expression(exp.TitleColumnConstraint(exp.Args{"this": p.parseVarOrString(false)}), nil, nil)
		},
		"UNIQUE": func(p *Parser) exp.Expression { return p.parseUnique() },
		"UPPERCASE": func(p *Parser) exp.Expression {
			return p.expression(exp.UppercaseColumnConstraint(nil), nil, nil)
		},
	}
	constraintParserKeySet = funcMapKeys(constraintParsers)

	schemaUnnamedConstraints = map[string]bool{
		"CHECK":       true,
		"EXCLUDE":     true,
		"FOREIGN KEY": true,
		"LIKE":        true,
		"PRIMARY KEY": true,
		"UNIQUE":      true,
	}

	// mysqlConstraintParsers ports parsers/mysql.py:243-251 MySQLParser.CONSTRAINT_PARSERS:
	// `**parser.Parser.CONSTRAINT_PARSERS` plus FULLTEXT/INDEX/KEY/SPATIAL (index
	// constraints) and ZEROFILL/INVISIBLE (bare column-attribute flags).
	mysqlConstraintParsers = make(map[string]func(*Parser) exp.Expression, len(constraintParsers)+6)
	for k, v := range constraintParsers {
		mysqlConstraintParsers[k] = v
	}
	mysqlConstraintParsers["FULLTEXT"] = func(p *Parser) exp.Expression { return p.parseIndexConstraint("FULLTEXT") }
	mysqlConstraintParsers["INDEX"] = func(p *Parser) exp.Expression { return p.parseIndexConstraint("") }
	mysqlConstraintParsers["KEY"] = func(p *Parser) exp.Expression { return p.parseIndexConstraint("") }
	mysqlConstraintParsers["SPATIAL"] = func(p *Parser) exp.Expression { return p.parseIndexConstraint("SPATIAL") }
	mysqlConstraintParsers["ZEROFILL"] = func(p *Parser) exp.Expression {
		return p.expression(exp.ZeroFillColumnConstraint(nil), nil, nil)
	}
	mysqlConstraintParsers["INVISIBLE"] = func(p *Parser) exp.Expression {
		return p.expression(exp.InvisibleColumnConstraint(nil), nil, nil)
	}
	mysqlConstraintParserKeySet = funcMapKeys(mysqlConstraintParsers)

	// mysqlSchemaUnnamedConstraints ports parsers/mysql.py:265-271
	// MySQLParser.SCHEMA_UNNAMED_CONSTRAINTS: the base set plus FULLTEXT/INDEX/KEY/SPATIAL.
	mysqlSchemaUnnamedConstraints = make(map[string]bool, len(schemaUnnamedConstraints)+4)
	for k, v := range schemaUnnamedConstraints {
		mysqlSchemaUnnamedConstraints[k] = v
	}
	mysqlSchemaUnnamedConstraints["FULLTEXT"] = true
	mysqlSchemaUnnamedConstraints["INDEX"] = true
	mysqlSchemaUnnamedConstraints["KEY"] = true
	mysqlSchemaUnnamedConstraints["SPATIAL"] = true
}

// funcMapKeys materializes a map[string]func(*Parser) exp.Expression's keys as a bool set,
// for matchTexts (mirrors Python's `text in some_dict` key-membership check).
func funcMapKeys(m map[string]func(*Parser) exp.Expression) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

func (p *Parser) constraintParsers() map[string]func(*Parser) exp.Expression {
	if p.dialect.Name == "mysql" {
		return mysqlConstraintParsers
	}
	return constraintParsers
}

func (p *Parser) constraintParserKeys() map[string]bool {
	if p.dialect.Name == "mysql" {
		return mysqlConstraintParserKeySet
	}
	return constraintParserKeySet
}

func (p *Parser) schemaUnnamedConstraints() map[string]bool {
	if p.dialect.Name == "mysql" {
		return mysqlSchemaUnnamedConstraints
	}
	return schemaUnnamedConstraints
}

// peekTextSeq mirrors upstream's `_match_text_seq(..., advance=False)`: checks whether the
// given word sequence appears next, without consuming it.
func (p *Parser) peekTextSeq(texts ...string) bool {
	index := p.index
	matched := p.matchTextSeq(texts...)
	p.retreat(index)
	return matched
}

// parseAutoIncrement ports _parse_auto_increment (parser.py:7322-7347).
func (p *Parser) parseAutoIncrement() exp.Expression {
	var start, increment exp.Expression
	var order any
	if p.match(tokens.L_PAREN, false) {
		args := p.parseWrappedCsv(p.parseBitwise)
		start = seqGet(args, 0)
		increment = seqGet(args, 1)
	} else if p.matchTextSeq("START") {
		start = p.parseBitwise()
		p.matchTextSeq("INCREMENT")
		increment = p.parseBitwise()
		if p.matchTextSeq("ORDER") {
			order = true
		} else if p.matchTextSeq("NOORDER") {
			order = false
		}
	}
	if start != nil && increment != nil {
		return p.expression(exp.GeneratedAsIdentityColumnConstraint(exp.Args{
			"start": start, "increment": increment, "this": false, "order": order,
		}), nil, nil)
	}
	return p.expression(exp.AutoIncrementColumnConstraint(nil), nil, nil)
}

// parseCheckConstraint ports _parse_check_constraint (parser.py:7349-7358).
func (p *Parser) parseCheckConstraint() exp.Expression {
	if !p.match(tokens.L_PAREN, false) {
		return nil
	}
	return p.expression(exp.CheckColumnConstraint(exp.Args{
		"this":     p.parseWrapped(p.parseAssignment, false),
		"enforced": p.matchTextSeq("ENFORCED"),
	}), nil, nil)
}

// parseCompress ports _parse_compress (parser.py:7366-7372): COMPRESS accepts either a
// scalar bitwise expression or a parenthesized CSV list.
func (p *Parser) parseCompress() exp.Expression {
	if p.match(tokens.L_PAREN, false) {
		return p.expression(exp.CompressColumnConstraint(exp.Args{
			"this": p.parseWrappedCsv(p.parseBitwise),
		}), nil, nil)
	}
	return p.expression(exp.CompressColumnConstraint(exp.Args{"this": p.parseBitwise()}), nil, nil)
}

// parseInline ports _parse_inline (parser.py:7427-7429).
func (p *Parser) parseInline() exp.Expression {
	p.matchTextSeq("LENGTH")
	return p.expression(exp.InlineLengthColumnConstraint(exp.Args{"this": p.parseBitwise()}), nil, nil)
}

// parseGeneratedAsIdentityBase ports the base _parse_generated_as_identity
// (parser.py:7374-7425).
func (p *Parser) parseGeneratedAsIdentityBase() exp.Expression {
	var this exp.Expression
	if p.matchTextSeq("BY", "DEFAULT") {
		onNull := p.matchPair(tokens.ON, tokens.NULL, true)
		this = p.expression(exp.GeneratedAsIdentityColumnConstraint(exp.Args{"this": false, "on_null": onNull}), nil, nil)
	} else {
		p.matchTextSeq("ALWAYS")
		this = p.expression(exp.GeneratedAsIdentityColumnConstraint(exp.Args{"this": true}), nil, nil)
	}
	p.match(tokens.ALIAS)

	if p.matchTextSeq("ROW") {
		start := p.matchTextSeq("START")
		if !start {
			p.match(tokens.END)
		}
		hidden := p.matchTextSeq("HIDDEN")
		return p.expression(exp.GeneratedAsRowColumnConstraint(exp.Args{"start": start, "hidden": hidden}), nil, nil)
	}

	identity := p.matchTextSeq("IDENTITY")

	if p.match(tokens.L_PAREN) {
		if p.match(tokens.START_WITH) {
			this.Set("start", p.parseBitwise())
		}
		if p.matchTextSeq("INCREMENT", "BY") {
			this.Set("increment", p.parseBitwise())
		}
		if p.matchTextSeq("MINVALUE") {
			this.Set("minvalue", p.parseBitwise())
		}
		if p.matchTextSeq("MAXVALUE") {
			this.Set("maxvalue", p.parseBitwise())
		}
		if p.matchTextSeq("CYCLE") {
			this.Set("cycle", true)
		} else if p.matchTextSeq("NO", "CYCLE") {
			this.Set("cycle", false)
		}
		if !identity {
			this.Set("expression", p.parseRange(nil))
		} else if this.Arg("start") == nil && p.match(tokens.NUMBER, false) {
			args := p.parseCsv(p.parseBitwise)
			this.Set("start", seqGet(args, 0))
			this.Set("increment", seqGet(args, 1))
		}
		p.matchRParen(nil)
	}

	return this
}

// parseGeneratedAsIdentity dispatches to the dialect-specific override of
// _parse_generated_as_identity that remaps a trailing STORED/VIRTUAL keyword onto a
// ComputedColumnConstraint. MySQL (parsers/mysql.py:341-360) matches STORED or VIRTUAL and
// carries persisted (handling both the already-Computed and the GeneratedAsIdentity result).
// Postgres (parsers/postgres.py:325-337) matches STORED only and unconditionally produces a
// ComputedColumnConstraint with persisted left unset. Base has no such override
// (parser.py:7374) and returns the raw base result.
func (p *Parser) parseGeneratedAsIdentity() exp.Expression {
	this := p.parseGeneratedAsIdentityBase()
	switch p.dialect.Name {
	case "mysql":
		if p.matchTexts(map[string]bool{"STORED": true, "VIRTUAL": true}) {
			persisted := stringsUpper(p.prev.Text) == "STORED"
			switch this.Kind() {
			case exp.KindComputedColumnConstraint:
				this.Set("persisted", persisted)
			case exp.KindGeneratedAsIdentityColumnConstraint:
				this = p.expression(exp.ComputedColumnConstraint(exp.Args{"this": this.Expr(), "persisted": persisted}), nil, nil)
			}
		}
	case "postgres":
		if p.matchTextSeq("STORED") {
			this = p.expression(exp.ComputedColumnConstraint(exp.Args{"this": this.Expr()}), nil, nil)
		}
	}
	return this
}

// parseNotConstraint ports _parse_not_constraint (parser.py:7431-7441).
func (p *Parser) parseNotConstraint() exp.Expression {
	if p.matchTextSeq("NULL") {
		return p.expression(exp.NotNullColumnConstraint(nil), nil, nil)
	}
	if p.matchTextSeq("CASESPECIFIC") {
		return p.expression(exp.CaseSpecificColumnConstraint(exp.Args{"not_": true}), nil, nil)
	}
	if p.matchTextSeq("FOR", "REPLICATION") {
		return p.expression(exp.NotForReplicationColumnConstraint(nil), nil, nil)
	}
	p.retreat(p.index - 1)
	return nil
}

// parseUniqueKey ports _parse_unique_key (parser.py:7500-7507), overridden to always
// return nil on postgres (parsers/postgres.py:313-314).
func (p *Parser) parseUniqueKey() exp.Expression {
	if p.dialect.Name == "postgres" {
		return nil
	}
	if p.curr.IsValid() && p.curr.TokenType != tokens.IDENTIFIER && p.constraintParserKeys()[stringsUpper(p.curr.Text)] {
		return nil
	}
	return p.parseIdVar(false, nil)
}

// parseUnique ports _parse_unique (parser.py:7509-7519).
func (p *Parser) parseUnique() exp.Expression {
	p.matchTexts(map[string]bool{"KEY": true, "INDEX": true})
	// Sequenced as separate statements (rather than inline within the Args literal below)
	// to guarantee upstream's exact evaluation order (parser.py:7509-7519: nulls, this,
	// index_type, on_conflict, options, in that order) - nulls/this must be parsed before
	// index_type's USING check runs, matching a real `UNIQUE NULLS NOT DISTINCT (a, b)
	// USING ...` clause shape.
	nulls := p.matchTextSeq("NULLS", "NOT", "DISTINCT")
	this := p.parseSchema(p.parseUniqueKey())
	// Upstream: index_type=self._match(USING) and self._advance_any() and self._prev.text.
	// The `and` chain yields the bool False (not None) when USING is absent, which
	// repr()/ToS() renders as index_type=False; default to false to match.
	var indexType any = false
	if p.match(tokens.USING) {
		if tok := p.advanceAny(false); tok != nil {
			indexType = tok.Text
		}
	}
	return p.expression(exp.UniqueColumnConstraint(exp.Args{
		"nulls":       nulls,
		"this":        this,
		"index_type":  indexType,
		"on_conflict": p.parseOnConflict(),
		"options":     p.parseKeyConstraintOptions(),
	}), nil, nil)
}

// parseKeyConstraintOptions ports _parse_key_constraint_options (parser.py:7521-7553): the
// trailing option list shared by FOREIGN KEY/PRIMARY KEY/UNIQUE/REFERENCES.
func (p *Parser) parseKeyConstraintOptions() []string {
	var options []string
	for {
		if !p.curr.IsValid() {
			break
		}
		if p.match(tokens.ON) {
			on := ""
			if tok := p.advanceAny(false); tok != nil {
				on = tok.Text
			}
			var action string
			switch {
			case p.matchTextSeq("NO", "ACTION"):
				action = "NO ACTION"
			case p.matchTextSeq("CASCADE"):
				action = "CASCADE"
			case p.matchTextSeq("RESTRICT"):
				action = "RESTRICT"
			case p.matchPair(tokens.SET, tokens.NULL, true):
				action = "SET NULL"
			case p.matchPair(tokens.SET, tokens.DEFAULT, true):
				action = "SET DEFAULT"
			default:
				p.raiseError("Invalid key constraint")
			}
			options = append(options, "ON "+on+" "+action)
		} else {
			v := p.parseVarFromOptions(keyConstraintOptions, false)
			if v == nil {
				break
			}
			options = append(options, v.Name())
		}
	}
	return options
}

// parseReferences ports _parse_references (parser.py:7555-7562).
func (p *Parser) parseReferences(match bool) exp.Expression {
	if match && !p.match(tokens.REFERENCES) {
		return nil
	}
	return p.expression(exp.Reference(exp.Args{
		"this":        p.parseTable(true, false, nil, false, false, false, false),
		"expressions": nil,
		"options":     p.parseKeyConstraintOptions(),
	}), nil, nil)
}

// parseForeignKey ports _parse_foreign_key (parser.py:7564-7597).
func (p *Parser) parseForeignKey() exp.Expression {
	var expressions []exp.Expression
	if !p.match(tokens.REFERENCES, false) {
		expressions = p.parseWrappedIdVars()
	}
	reference := p.parseReferences(true)
	args := exp.Args{"expressions": expressions, "reference": reference}
	for p.match(tokens.ON) {
		if !p.matchSet(map[tokens.TokenType]bool{tokens.DELETE: true, tokens.UPDATE: true}) {
			p.raiseError("Expected DELETE or UPDATE")
		}
		kind := strings.ToLower(p.prev.Text)
		var action string
		switch {
		case p.matchTextSeq("NO", "ACTION"):
			action = "NO ACTION"
		case p.match(tokens.SET):
			p.matchSet(map[tokens.TokenType]bool{tokens.NULL: true, tokens.DEFAULT: true})
			action = "SET " + stringsUpper(p.prev.Text)
		default:
			p.advance()
			action = stringsUpper(p.prev.Text)
		}
		args[kind] = action
	}
	args["options"] = p.parseKeyConstraintOptions()
	return p.expression(exp.ForeignKey(args), nil, nil)
}

// parsePrimaryKeyPart ports the base _parse_primary_key_part (parser.py:7599-7600),
// overridden by mysql (parsers/mysql.py:362-369) to support the `col(N)` index-prefix-
// length form (-> exp.ColumnPrefix).
func (p *Parser) parsePrimaryKeyPart() exp.Expression {
	if p.dialect.Name != "mysql" {
		return p.parseField(false, nil, false)
	}
	this := p.parseIdVar(true, nil)
	if !p.match(tokens.L_PAREN) {
		return this
	}
	expression := p.parseNumber()
	p.matchRParen(nil)
	return p.expression(exp.ColumnPrefix(exp.Args{"this": this, "expression": expression}), nil, nil)
}

// parsePrimaryKey ports _parse_primary_key (parser.py:7614-7653). named_primary_key is
// derived from the dialect rather than accepted as a parameter, mirroring mysql's override
// (parsers/mysql.py:630-638) which always forces it true regardless of the caller.
func (p *Parser) parsePrimaryKey(wrappedOptional bool, inProps bool) exp.Expression {
	namedPrimaryKey := p.dialect.Name == "mysql"

	var desc any
	if p.matchSet(map[tokens.TokenType]bool{tokens.ASC: true, tokens.DESC: true}) {
		desc = p.prev.TokenType == tokens.DESC
	}

	var this exp.Expression
	if namedPrimaryKey && p.curr.IsValid() && !p.constraintParserKeys()[stringsUpper(p.curr.Text)] && p.next.IsValid() && p.next.TokenType == tokens.L_PAREN {
		this = p.parseIdVar(true, nil)
	}

	if !inProps && !p.match(tokens.L_PAREN, false) {
		return p.expression(exp.PrimaryKeyColumnConstraint(exp.Args{
			"desc":    desc,
			"options": p.parseKeyConstraintOptions(),
		}), nil, nil)
	}

	expressions := p.parseWrappedCsv(p.parsePrimaryKeyPart, wrappedOptional)
	return p.expression(exp.PrimaryKey(exp.Args{
		"this":        this,
		"expressions": expressions,
		"include":     p.parseIndexParams(),
		"options":     p.parseKeyConstraintOptions(),
	}), nil, nil)
}

// parseIndexConstraint ports parsers/mysql.py:371-416 MySQLParser._parse_index_constraint:
// the FULLTEXT/INDEX/KEY/SPATIAL column/schema constraint (-> exp.IndexColumnConstraint).
func (p *Parser) parseIndexConstraint(kind string) exp.Expression {
	if kind != "" {
		p.matchTexts(map[string]bool{"INDEX": true, "KEY": true})
	}
	this := p.parseIdVar(false, nil)
	var indexType any
	if p.match(tokens.USING) {
		if tok := p.advanceAny(false); tok != nil {
			indexType = tok.Text
		}
	}
	expressions := p.parseWrappedCsv(func() exp.Expression { return p.parseOrdered(nil) })

	var options []exp.Expression
	for {
		var opt exp.Expression
		switch {
		case p.matchTextSeq("KEY_BLOCK_SIZE"):
			p.match(tokens.EQ)
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"key_block_size": p.parseNumber()}), nil, nil)
		case p.match(tokens.USING):
			var using any
			if tok := p.advanceAny(false); tok != nil {
				using = tok.Text
			}
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"using": using}), nil, nil)
		case p.matchTextSeq("WITH", "PARSER"):
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"parser": p.parseVar(true, nil, false)}), nil, nil)
		case p.match(tokens.COMMENT):
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"comment": p.parseString()}), nil, nil)
		case p.matchTextSeq("VISIBLE"):
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"visible": true}), nil, nil)
		case p.matchTextSeq("INVISIBLE"):
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"visible": false}), nil, nil)
		case p.matchTextSeq("ENGINE_ATTRIBUTE"):
			p.match(tokens.EQ)
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"engine_attr": p.parseString()}), nil, nil)
		case p.matchTextSeq("SECONDARY_ENGINE_ATTRIBUTE"):
			p.match(tokens.EQ)
			opt = p.expression(exp.IndexConstraintOption(exp.Args{"secondary_engine_attr": p.parseString()}), nil, nil)
		}
		if opt == nil {
			break
		}
		options = append(options, opt)
	}

	var kindArg any
	if kind != "" {
		kindArg = kind
	}
	return p.expression(exp.IndexColumnConstraint(exp.Args{
		"this": this, "expressions": expressions, "kind": kindArg, "index_type": indexType, "options": options,
	}), nil, nil)
}

// parseWrappedIdVars ports _parse_wrapped_id_vars (parser.py:8622-8623).
func (p *Parser) parseWrappedIdVars(optional ...bool) []exp.Expression {
	opt := false
	if len(optional) > 0 {
		opt = optional[0]
	}
	return p.parseWrappedCsv(func() exp.Expression { return p.parseIdVar(true, nil) }, opt)
}

// parseNumber is a minimal port of _parse_number (parser.py:8531-8534): only the plain
// NUMBER token case is needed by this slice (mysql's PRIMARY KEY `col(N)` prefix length,
// and IndexConstraintOption's KEY_BLOCK_SIZE), so NUMERIC_PARSERS' other entries
// (BIT_STRING/BYTE_STRING/HEX_STRING) are omitted.
func (p *Parser) parseNumber() exp.Expression {
	if p.match(tokens.NUMBER) {
		return p.expression(exp.LiteralNumber(p.prev.Text), &p.prev, nil)
	}
	return p.parsePlaceholder()
}

// parseIndexParams ports _parse_index_params (parser.py:4573-4604): the trailing index-
// parameter block shared by PRIMARY KEY, CREATE INDEX, and EXCLUDE. Like upstream it always
// returns an IndexParameters node (empty when no clause follows), and parses storage properties
// between PARTITION BY and USING INDEX TABLESPACE.
func (p *Parser) parseIndexParams() exp.Expression {
	var using exp.Expression
	if p.match(tokens.USING) {
		using = p.parseVar(true, nil, false)
	}
	var columns []exp.Expression
	if p.match(tokens.L_PAREN, false) {
		columns = p.parseWrappedCsv(p.parseWithOperator)
	}
	var include []exp.Expression
	if p.matchTextSeq("INCLUDE") {
		include = p.parseWrappedIdVars()
	}
	partitionBy := p.parsePartitionBy()
	var withStorage any = false
	if p.match(tokens.WITH) {
		withStorage = p.parseWrappedProperties()
	}
	var tablespace exp.Expression
	if p.matchTextSeq("USING", "INDEX", "TABLESPACE") {
		tablespace = p.parseVar(true, nil, false)
	}
	where := p.parseWhere(false)
	var on exp.Expression
	if p.match(tokens.ON) {
		on = p.parseField(false, nil, false)
	}
	return p.expression(exp.IndexParameters(exp.Args{
		"using":        using,
		"columns":      columns,
		"include":      include,
		"partition_by": partitionBy,
		"where":        where,
		"with_storage": withStorage,
		"tablespace":   tablespace,
		"on":           on,
	}), nil, nil)
}

// opclassFollowKeywords/optypeFollowTokens mirror OPCLASS_FOLLOW_KEYWORDS/
// OPTYPE_FOLLOW_TOKENS (parser.py:1669,1671), used by parseOpclass below.
var opclassFollowKeywords = map[string]bool{"ASC": true, "DESC": true, "NULLS": true, "WITH": true}
var optypeFollowTokens = map[tokens.TokenType]bool{tokens.COMMA: true, tokens.R_PAREN: true}

// parseIndexedColumn ports _parse_indexed_column (parser.py:9517-9518): an ordered opclass
// column inside a CREATE INDEX column list.
func (p *Parser) parseIndexedColumn() exp.Expression {
	return p.parseOrdered(p.parseOpclass)
}

// parseWithOperator ports _parse_with_operator (parser.py:9520-9528). The operator parser
// explicitly accepts reserved operator tokens (`=`, `>=`, `&&`, `||`, `@>`, `<@`, etc.) while
// preserving the indexed column's opclass, ordering, and null-order modifiers.
func (p *Parser) parseWithOperator() exp.Expression {
	this := p.parseIndexedColumn()
	if !p.match(tokens.WITH) {
		return this
	}
	return p.expression(exp.WithOperator(exp.Args{
		"this": this,
		"op":   p.parseVar(true, reservedTokens, false),
	}), nil, nil)
}

// parseOpclass ports _parse_opclass (parser.py:4562-4571): disambiguates a plain ordered
// column from `column opclass_name` (e.g. `title public.gin_trgm_ops`), used by postgres
// CREATE INDEX column lists.
func (p *Parser) parseOpclass() exp.Expression {
	this := p.parseDisjunction()
	if p.matchTexts(opclassFollowKeywords, false) {
		return this
	}
	if !p.matchSet(optypeFollowTokens, false) {
		return p.expression(exp.Opclass(exp.Args{
			"this":       this,
			"expression": p.parseTableParts(false, false, false, false),
		}), nil, nil)
	}
	return this
}

// noParenFunctionTokens is the small subset of NO_PAREN_FUNCTIONS (parser.py:431-438) this
// port's DEFAULT/ON UPDATE column-constraint values need: CURRENT_DATE/CURRENT_DATETIME/
// CURRENT_TIME/CURRENT_TIMESTAMP/CURRENT_USER/CURRENT_ROLE used without trailing parens
// (e.g. mysql's `DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP`). Wiring the full
// NO_PAREN_FUNCTIONS dict into the general expression/function-call parser
// (parser.go:1952-1955, explicitly marked `TODO(1d): NO_PAREN_FUNCTIONS CURRENT_*`) is out
// of this slice's scope; parseNoParenCurrentFunc below is a narrow, local application to
// just these two constraint values.
var noParenFunctionTokens = map[tokens.TokenType]bool{
	tokens.CURRENT_DATE: true, tokens.CURRENT_DATETIME: true, tokens.CURRENT_TIME: true,
	tokens.CURRENT_TIMESTAMP: true, tokens.CURRENT_USER: true, tokens.CURRENT_ROLE: true,
}

// parseNoParenCurrentFunc builds the node a bare CURRENT_*-family keyword should produce in a
// DEFAULT/ON UPDATE column-constraint value (see noParenFunctionTokens above), or returns nil
// (consuming nothing) when curr isn't one of those tokens or is immediately followed by "(".
// CURRENT_DATE/CURRENT_TIME map to their dedicated Kinds via noParenFunctions (matching the
// no-paren keyword path in parseFunctionCall) so `DEFAULT CURRENT_DATE` renders bare
// CURRENT_DATE and `DEFAULT CURRENT_TIME` renders CURRENT_TIME() - byte-for-byte with the
// pinned oracle. The remaining family members (CURRENT_TIMESTAMP/CURRENT_USER/CURRENT_ROLE)
// have no dedicated Kind yet, so they keep the zero-arg exp.Anonymous shape parseFunctionCall's
// anonymous fallback also builds (e.g. `DEFAULT CURRENT_TIMESTAMP` -> CURRENT_TIMESTAMP()).
func (p *Parser) parseNoParenCurrentFunc() exp.Expression {
	if !noParenFunctionTokens[p.curr.TokenType] || p.next.TokenType == tokens.L_PAREN {
		return nil
	}
	tok := p.curr
	p.advance()
	if build := noParenFunctions[tok.TokenType]; build != nil {
		return p.expression(build(exp.Args{}), &tok, nil)
	}
	return p.expression(exp.Anonymous(exp.Args{"this": tok.Text, "expressions": []exp.Expression{}}), nil, nil)
}
