package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

var rParenCommaSet = map[tokens.TokenType]bool{tokens.R_PAREN: true, tokens.COMMA: true}

func (p *Parser) parseCreate() exp.Expression {
	start := p.prev
	// Attempt a structured CREATE under error isolation, then degrade to a raw Command when
	// an unported creatable/property or trailing token prevents a clean parse. tryParse runs
	// the body at IMMEDIATE error level so a partial parse retreats without leaving stale
	// errors that would later poison checkErrors.
	if structured := p.tryParse(func() exp.Expression { return p.parseCreateStructured(start) }, false); structured != nil {
		return structured
	}
	return p.parseAsCommand(start)
}

// parseCreateStructured ports the base/mysql/postgres-relevant control flow of _parse_create
// (parser.py:2360-2644). Each property pass is appended in upstream order so location-aware
// generation can place the resulting nodes around the schema, AS alias, expression, and indexes.
func (p *Parser) parseCreateStructured(start tokens.Token) exp.Expression {
	replace := start.TokenType == tokens.REPLACE || p.matchPair(tokens.OR, tokens.REPLACE, true) || p.matchPair(tokens.OR, tokens.ALTER, true)
	refresh := p.matchPair(tokens.OR, tokens.REFRESH, true)
	unique := p.match(tokens.UNIQUE)

	properties := []exp.Expression{}
	extendProperties := func(parsed exp.Expression) {
		if parsed != nil {
			properties = append(properties, parsed.Expressions()...)
		}
	}

	if !p.matchSet(creatables) {
		// exp.Properties.Location.POST_CREATE
		parsed := p.parseProperties()
		extendProperties(parsed)
		if parsed == nil || !p.matchSet(creatables) {
			return nil
		}
	}
	createToken := p.prev
	ctt := createToken.TokenType
	concurrently := p.matchTextSeq("CONCURRENTLY")
	exists := p.parseExists(true)

	var this exp.Expression
	var expression exp.Expression
	var indexes []exp.Expression
	var noSchemaBinding any
	var begin any

	switch {
	case ctt == tokens.FUNCTION || ctt == tokens.PROCEDURE:
		var functionProperties []exp.Expression
		this, expression, functionProperties, begin = p.parseCreateFunction(ctt)
		properties = append(properties, functionProperties...)
	case ctt == tokens.INDEX:
		var index exp.Expression
		anonymous := false
		if !p.match(tokens.ON) {
			index = p.parseIdVar(true, nil)
		} else {
			anonymous = true
		}
		if index == nil && !anonymous {
			return nil
		}
		this = p.parseIndexBody(index, anonymous)
	case ctt == tokens.TRIGGER || (ctt == tokens.CONSTRAINT && p.match(tokens.TRIGGER)):
		isConstraint := ctt == tokens.CONSTRAINT
		if isConstraint {
			createToken = p.prev
		}
		var triggerProperties exp.Expression
		this, triggerProperties = p.parseCreateTrigger(isConstraint)
		if this == nil {
			return nil
		}
		if triggerProperties != nil {
			properties = append(properties, triggerProperties)
		}
	case ctt == tokens.TYPE:
		this = p.parseTableParts(true, false, false, false)
		if this == nil || !p.match(tokens.ALIAS) {
			return nil
		}
		if p.match(tokens.ENUM) {
			expression = p.expression(exp.DataType(exp.Args{
				"this":        exp.DTypeEnum,
				"expressions": p.parseWrappedCsv(p.parseString),
			}), nil, nil)
		} else if p.match(tokens.L_PAREN, false) {
			expression = p.parseSchema(nil)
		} else {
			return nil
		}
	case dbCreatables[ctt]:
		tableParts := p.parseTableParts(true, ctt == tokens.SCHEMA, false, false)

		// exp.Properties.Location.POST_NAME
		p.match(tokens.COMMA)
		extendProperties(p.parseProperties(true))

		this = p.parseSchema(tableParts)

		// exp.Properties.Location.POST_SCHEMA and POST_WITH
		extendProperties(p.parseProperties())

		hasAlias := p.match(tokens.ALIAS)
		if !p.matchSet(selectStartTokens, false) {
			// exp.Properties.Location.POST_ALIAS
			extendProperties(p.parseProperties())
		}

		expression = p.parseDDLSelect()
		if expression == nil && hasAlias {
			expression = p.tryParse(func() exp.Expression { return p.parseTableParts(false, false, false, false) }, false)
		}

		if ctt == tokens.TABLE {
			// exp.Properties.Location.POST_EXPRESSION
			extendProperties(p.parseProperties())

			indexes = []exp.Expression{}
			for {
				index := p.parseIndex()

				// exp.Properties.Location.POST_INDEX
				extendProperties(p.parseProperties())
				if index == nil {
					break
				}
				p.match(tokens.COMMA)
				indexes = append(indexes, index)
			}
		} else if ctt == tokens.VIEW && p.matchTextSeq("WITH", "NO", "SCHEMA", "BINDING") {
			noSchemaBinding = true
		}
	default:
		return nil
	}
	if p.curr.IsValid() && !p.matchSet(rParenCommaSet, false) {
		return nil
	}

	kind := stringsUpper(createToken.Text)
	var propertiesNode exp.Expression
	if len(properties) > 0 {
		propertiesNode = p.expression(exp.Properties(exp.Args{"expressions": properties}), nil, nil)
	}
	return p.expression(exp.Create(exp.Args{
		"this":              this,
		"kind":              kind,
		"replace":           replace,
		"refresh":           refresh,
		"unique":            unique,
		"expression":        expression,
		"exists":            exists,
		"properties":        propertiesNode,
		"indexes":           indexes,
		"no_schema_binding": noSchemaBinding,
		"concurrently":      concurrently,
		"begin":             begin,
	}), nil, nil)
}

// parseUserDefinedFunction ports _parse_user_defined_function (parser.py:7114-7124): a
// CREATE FUNCTION/PROCEDURE signature, `name` or `name(params...)`.
func (p *Parser) parseUserDefinedFunction() exp.Expression {
	this := p.parseTableParts(true, false, false, false)
	if !p.match(tokens.L_PAREN) {
		return this
	}
	expressions := p.parseCsv(p.parseFunctionParameter)
	p.matchRParen(nil)
	return p.expression(exp.UserDefinedFunction(exp.Args{"this": this, "expressions": expressions, "wrapped": true}), nil, nil)
}

var postgresArgModeTokens = map[tokens.TokenType]bool{
	tokens.IN: true, tokens.OUT: true, tokens.INOUT: true, tokens.VARIADIC: true,
}

// parsePostgresParameterMode ports PostgresParser._parse_parameter_mode
// (parsers/postgres.py:207-255), including both speculative type lookaheads which distinguish
// a parameter named `out` from an OUT-mode parameter.
func (p *Parser) parsePostgresParameterMode() (tokens.TokenType, bool) {
	if !p.matchSet(postgresArgModeTokens, false) || !p.next.IsValid() {
		return tokens.SENTINEL, false
	}
	mode := p.curr.TokenType

	isFollowedByBuiltinType := p.tryParse(func() exp.Expression {
		p.advance()
		return p.parseTypes(false, false, false, false)
	}, true)
	if isFollowedByBuiltinType != nil {
		return tokens.SENTINEL, false
	}
	if !idVarTokens[p.next.TokenType] {
		return tokens.SENTINEL, false
	}

	isFollowedByAnyType := p.tryParse(func() exp.Expression {
		p.advance(2)
		return p.parseTypes(false, false, true, false)
	}, true)
	if isFollowedByAnyType != nil {
		return mode, true
	}
	return tokens.SENTINEL, false
}

// parseFunctionParameter ports the base _parse_function_parameter and PostgreSQL's override
// (parsers/postgres.py:275-292). Mode constraints are raw InOutColumnConstraint children,
// prepended directly to ColumnDef.constraints just as upstream does.
func (p *Parser) parseFunctionParameter() exp.Expression {
	if p.dialect.Name != "postgres" {
		return p.parseColumnDef(p.parseIdVar(true, nil))
	}

	mode, hasMode := p.parsePostgresParameterMode()
	if hasMode {
		p.advance()
	}
	columnDef := p.parseColumnDef(p.parseIdVar(true, nil))
	if hasMode && columnDef != nil {
		constraint := p.expression(exp.InOutColumnConstraint(exp.Args{
			"input_":   mode == tokens.IN || mode == tokens.INOUT,
			"output":   mode == tokens.OUT || mode == tokens.INOUT,
			"variadic": mode == tokens.VARIADIC,
		}), nil, nil)
		constraints, _ := columnDef.Arg("constraints").([]exp.Expression)
		constraints = append([]exp.Expression{constraint}, constraints...)
		columnDef.Set("constraints", constraints)
	}
	return columnDef
}

// parseCreateFunction ports the CREATE FUNCTION/PROCEDURE branch of _parse_create
// (parser.py:2414-2472): the UDF signature, its RETURNS/LANGUAGE/... properties (both
// before and after the body), and the body itself (a string literal, a nested statement, or
// - left unsupported, see parseUserDefinedFunction - a heredoc/BEGIN block).
func (p *Parser) parseCreateFunction(ctt tokens.TokenType) (this, expression exp.Expression, properties []exp.Expression, begin any) {
	extendProperties := func(parsed exp.Expression) {
		if parsed != nil {
			properties = append(properties, parsed.Expressions()...)
		}
	}

	this = p.parseUserDefinedFunction()
	// exp.Properties.Location.POST_SCHEMA (parser.py:2417): properties parsed before AS.
	extendProperties(p.parseProperties())

	// _parse_heredoc (parser.py:9368-9370) only matches TokenType.HEREDOC_STRING - a
	// dollar-quoted UDF body (e.g. postgres `AS $$ ... $$`), which this port doesn't model
	// (no exp.Heredoc Kind - deferred, no target-gap SQL needs it). `AS` is still matched
	// here (mirroring upstream's self._match(TokenType.ALIAS) gate); a genuine heredoc body
	// is left unconsumed below (bailing out before the generic parseStatement fallback, which
	// - unlike upstream's own HEREDOC_STRING-only _parse_heredoc - would otherwise happily
	// misparse it as an ordinary string literal), so the caller's trailing-token check
	// degrades the whole CREATE to a Command.
	p.match(tokens.ALIAS)
	if p.curr.TokenType == tokens.HEREDOC_STRING {
		return this, nil, properties, begin
	}

	// upstream's table-overload/MacroOverloads detection (parser.py:2422-2436) is a
	// BigQuery/Snowflake-only feature (`CREATE FUNCTION f(...) AS (expr), (params) AS TABLE
	// body`) that always retreats to a no-op for base/mysql/postgres inputs: it only commits
	// when the parsed body is immediately followed by `, (`, which never occurs in this
	// port's corpus. Omitted (documented divergence; see parser_ddl_tail_test.go).

	// exp.Properties.Location.POST_SCHEMA (parser.py:2445): function properties parsed after
	// AS/the overload attempt, without the generic key=value fallback.
	properties = append(properties, p.parseTailProperties()...)

	if p.match(tokens.COMMAND) {
		expression = p.parseAsCommand(p.prev)
		return this, expression, properties, begin
	}
	begin = p.match(tokens.BEGIN)
	returnMatched := p.matchTextSeq("RETURN")
	if p.match(tokens.STRING, false) {
		expression = p.parseString()
		// exp.Properties.Location.POST_SCHEMA (parser.py:2458): generic properties parsed
		// after a string-literal body.
		extendProperties(p.parseProperties())
	} else if ctt == tokens.FUNCTION {
		// _parse_user_defined_function_expression (parser.py:7108-7109) is just
		// self._parse_statement(); exp.Block (the PROCEDURE body fallback, parser.py:2462)
		// isn't ported (deferred - no target-gap SQL needs it), so a PROCEDURE with a
		// non-string body simply leaves `expression` nil here, degrading to a Command via
		// the caller's trailing-token check.
		expression = p.parseStatement()
	}
	if returnMatched {
		expression = p.expression(exp.Return(exp.Args{"this": expression}), nil, nil)
	}
	return this, expression, properties, begin
}

// parseIndexBody ports the named/anonymous CREATE INDEX branch of _parse_index
// (parser.py:4606-4634).
func (p *Parser) parseIndexBody(index exp.Expression, anonymous bool) exp.Expression {
	p.match(tokens.ON)
	p.match(tokens.TABLE) // hive
	table := p.parseTableParts(true, false, false, false)
	params := p.parseIndexParams()
	return p.expression(exp.Index(exp.Args{
		"this": index, "table": table, "params": params,
	}), nil, nil)
}

// parseIndex ports the no-argument branch of _parse_index, used by CREATE TABLE's trailing
// UNIQUE/PRIMARY/AMP INDEX loop.
func (p *Parser) parseIndex() exp.Expression {
	unique := p.match(tokens.UNIQUE)
	primary := p.matchTextSeq("PRIMARY")
	amp := p.matchTextSeq("AMP")
	if !p.match(tokens.INDEX) {
		return nil
	}
	index := p.parseIdVar(true, nil)
	return p.expression(exp.Index(exp.Args{
		"this": index, "unique": unique, "primary": primary, "amp": amp,
		"params": p.parseIndexParams(),
	}), nil, nil)
}

// triggerEventTokens mirrors TRIGGER_EVENTS (parser.py:650-655).
var triggerEventTokens = map[tokens.TokenType]bool{
	tokens.INSERT: true, tokens.UPDATE: true, tokens.DELETE: true, tokens.TRUNCATE: true,
}

// triggerTimingOptions mirrors TRIGGER_TIMING (parser.py:1588-1592).
var triggerTimingOptions = optionsType{
	"INSTEAD": {{"OF"}},
	"BEFORE":  nil,
	"AFTER":   nil,
}

// triggerDeferrableOptions mirrors TRIGGER_DEFERRABLE (parser.py:1594-1597).
var triggerDeferrableOptions = optionsType{
	"NOT":        {{"DEFERRABLE"}},
	"DEFERRABLE": nil,
}

// parseCreateTrigger ports the (CONSTRAINT )?TRIGGER branch of _parse_create
// (parser.py:2483-2532): trigger name, timing, events, ON table, [FROM referenced_table],
// [DEFERRABLE ...], [REFERENCING ...], [FOR EACH ROW|STATEMENT], [WHEN (...)], EXECUTE
// FUNCTION|PROCEDURE <call>. Returns (nil, nil) at any of upstream's own
// `return self._parse_as_command(start)` bail points, which the caller (case
// ctt==TRIGGER... in parseCreateStructured) turns into an overall nil (-> Command).
func (p *Parser) parseCreateTrigger(isConstraint bool) (exp.Expression, exp.Expression) {
	triggerName := p.parseIdVar(true, nil)
	if triggerName == nil {
		return nil, nil
	}
	timingVar := p.parseVarFromOptions(triggerTimingOptions, false)
	if timingVar == nil {
		return nil, nil
	}
	timing := timingVar.Name()
	events := p.parseTriggerEvents()
	if !p.match(tokens.ON) {
		p.raiseError("Expected ON in trigger definition")
	}
	table := p.parseTableParts(false, false, false, false)
	var referencedTable exp.Expression
	if p.match(tokens.FROM) {
		referencedTable = p.parseTableParts(false, false, false, false)
	}
	deferrable, initially := p.parseTriggerDeferrable()
	referencing := p.parseTriggerReferencing()
	forEach := p.parseTriggerForEach()
	var when exp.Expression
	if p.matchTextSeq("WHEN") {
		when = p.parseWrapped(p.parseDisjunction, true)
	}
	execute := p.parseTriggerExecute()
	if execute == nil {
		return nil, nil
	}
	triggerProps := p.expression(exp.TriggerProperties(exp.Args{
		"table":            table,
		"timing":           timing,
		"events":           events,
		"execute":          execute,
		"constraint":       isConstraint,
		"referenced_table": referencedTable,
		"deferrable":       deferrable,
		"initially":        initially,
		"referencing":      referencing,
		"for_each":         forEach,
		"when":             when,
	}), nil, nil)
	return triggerName, triggerProps
}

// parseTriggerEvents ports _parse_trigger_events (parser.py:2681-2701).
func (p *Parser) parseTriggerEvents() []exp.Expression {
	var events []exp.Expression
	for {
		if !p.matchSet(triggerEventTokens) {
			p.raiseError("Expected trigger event (INSERT, UPDATE, DELETE, TRUNCATE)")
		}
		eventType := stringsUpper(p.prev.Text)
		var columns []exp.Expression
		if eventType == "UPDATE" && p.matchTextSeq("OF") {
			columns = p.parseCsv(p.parseColumn)
		}
		events = append(events, p.expression(exp.TriggerEvent(exp.Args{"this": eventType, "columns": columns}), nil, nil))
		if !p.match(tokens.OR) {
			break
		}
	}
	return events
}

// parseTriggerDeferrable ports _parse_trigger_deferrable (parser.py:2703-2717).
func (p *Parser) parseTriggerDeferrable() (any, any) {
	var deferrable any
	if deferrableVar := p.parseVarFromOptions(triggerDeferrableOptions, false); deferrableVar != nil {
		deferrable = deferrableVar.Name()
	}
	var initially any
	if deferrable != nil && p.matchTextSeq("INITIALLY") {
		if p.matchTexts(map[string]bool{"IMMEDIATE": true, "DEFERRED": true}) {
			initially = stringsUpper(p.prev.Text)
		}
	}
	return deferrable, initially
}

// parseTriggerReferencingClause ports _parse_trigger_referencing_clause (parser.py:2719-2725).
func (p *Parser) parseTriggerReferencingClause(keyword string) exp.Expression {
	if !p.matchTextSeq(keyword) {
		return nil
	}
	if !p.matchTextSeq("TABLE") {
		p.raiseError("Expected TABLE after " + keyword + " in REFERENCING clause")
	}
	p.matchTextSeq("AS")
	return p.parseIdVar(true, nil)
}

// parseTriggerReferencing ports _parse_trigger_referencing (parser.py:2727-2749).
func (p *Parser) parseTriggerReferencing() exp.Expression {
	if !p.matchTextSeq("REFERENCING") {
		return nil
	}
	var oldAlias, newAlias exp.Expression
	for {
		if alias := p.parseTriggerReferencingClause("OLD"); alias != nil {
			if oldAlias != nil {
				p.raiseError("Duplicate OLD clause in REFERENCING")
			}
			oldAlias = alias
		} else if alias := p.parseTriggerReferencingClause("NEW"); alias != nil {
			if newAlias != nil {
				p.raiseError("Duplicate NEW clause in REFERENCING")
			}
			newAlias = alias
		} else {
			break
		}
	}
	if oldAlias == nil && newAlias == nil {
		p.raiseError("REFERENCING clause requires at least OLD TABLE or NEW TABLE")
	}
	return p.expression(exp.TriggerReferencing(exp.Args{"old": oldAlias, "new": newAlias}), nil, nil)
}

// parseTriggerForEach ports _parse_trigger_for_each (parser.py:2751-2755).
func (p *Parser) parseTriggerForEach() any {
	if !p.matchTextSeq("FOR", "EACH") {
		return nil
	}
	if p.matchTexts(map[string]bool{"ROW": true, "STATEMENT": true}) {
		return stringsUpper(p.prev.Text)
	}
	return nil
}

// parseTriggerExecute ports _parse_trigger_execute (parser.py:2757-2765).
func (p *Parser) parseTriggerExecute() exp.Expression {
	if !p.match(tokens.EXECUTE) {
		return nil
	}
	if !p.matchSet(map[tokens.TokenType]bool{tokens.FUNCTION: true, tokens.PROCEDURE: true}) {
		p.raiseError("Expected FUNCTION or PROCEDURE after EXECUTE")
	}
	funcCall := p.parseColumn()
	return p.expression(exp.TriggerExecute(exp.Args{"this": funcCall}), nil, nil)
}

func (p *Parser) parseSchema(this exp.Expression) exp.Expression {
	index := p.index
	if !p.match(tokens.L_PAREN) {
		return this
	}
	if p.matchSet(selectStartTokens) {
		p.retreat(index)
		return this
	}
	args := p.parseCsv(func() exp.Expression {
		if c := p.parseConstraint(); c != nil {
			return c
		}
		return p.parseFieldDef()
	})
	p.matchRParen(nil)
	return p.expression(exp.Schema(exp.Args{"this": this, "expressions": args}), nil, nil)
}

func (p *Parser) parseFieldDef() exp.Expression {
	return p.parseColumnDef(p.parseField(true, nil, false))
}

func (p *Parser) parseColumnDef(this exp.Expression) exp.Expression {
	if this != nil && this.Kind() == exp.KindColumn {
		this = this.This()
	}
	// schema=true mirrors upstream _parse_column_def (parser.py:7255): it enables the
	// fixed-size-array form (e.g. `col INT[3]`) in column definitions.
	kind := p.parseTypes(false, true, true, false)

	// `<name> FOR ORDINALITY` (parser.py:7257): an XMLTABLE/column-def ordinality marker. The
	// flag isn't rendered by columndef_sql (generator.py:1169), so it round-trips lossily to just
	// `<name>`, matching upstream.
	if p.matchPair(tokens.FOR, tokens.ORDINALITY, true) {
		return p.expression(exp.ColumnDef(exp.Args{"this": this, "ordinality": true}), nil, nil)
	}

	constraints := []exp.Expression{}

	// `<kind> AS (<expr>) [STORED|VIRTUAL]`: the computed-column branch at parser.py:
	// 7283-7302. Gated on WRAPPED_TRANSFORM_COLUMN_CONSTRAINT (=True for base/mysql/
	// postgres; only RisingWave, out of scope, overrides False), which requires the AS to
	// be immediately followed by "(" - so `col INT AS some_expr` (no parens) is left for
	// parseColumnConstraint's own dispatch instead. The two upstream sibling branches
	// (ALIAS/MATERIALIZED-prefixed and IN/OUT parameter constraints, parser.py:7262-7281)
	// are Oracle/T-SQL/procedure-only and aren't exercised by the base/mysql/postgres
	// corpus, so they're omitted here (documented divergence).
	if kind != nil && p.matchPair(tokens.ALIAS, tokens.L_PAREN, false) {
		p.advance() // consume AS, leaving `(` as the current token for parseDisjunction below
		// `this` MUST be parsed before the STORED/VIRTUAL match: upstream (parser.py:7295-7299)
		// evaluates this=self._parse_disjunction() before persisted=self._match_texts(...), and
		// parseDisjunction consumes the `(<expr>)` so the storage keyword (if present) becomes the
		// current token. Computing `persisted` first (Go arg-map evaluation order) would test the
		// still-current `(` and leave STORED/VIRTUAL unconsumed, degrading the CREATE to a Command.
		this := p.parseDisjunction()
		persisted := p.matchTexts(map[string]bool{"STORED": true, "VIRTUAL": true}) && stringsUpper(p.prev.Text) == "STORED"
		constraints = append(constraints, p.expression(exp.ColumnConstraint(exp.Args{
			"kind": p.expression(exp.ComputedColumnConstraint(exp.Args{
				"this":      this,
				"persisted": persisted,
			}), nil, nil),
		}), nil, nil))
	}

	for {
		constraint := p.parseColumnConstraint()
		if constraint == nil {
			break
		}
		constraints = append(constraints, constraint)
	}
	if kind == nil && len(constraints) == 0 {
		return this
	}

	// Trailing FIRST/AFTER <col> position (parser.py:7313-7316), e.g. `ADD COLUMN k INT
	// FIRST` / `ADD COLUMN k INT AFTER m`.
	var position exp.Expression
	if p.matchTexts(map[string]bool{"FIRST": true, "AFTER": true}) {
		// pos must be captured before parseColumn() below, which advances p.prev past the
		// FIRST/AFTER keyword itself (mirrors upstream's `pos = self._prev.text` capture
		// preceding its own _parse_column() call, parser.py:7314-7316).
		pos := p.prev.Text
		position = p.expression(exp.ColumnPosition(exp.Args{"this": p.parseColumn(), "position": pos}), nil, nil)
	}

	return p.expression(exp.ColumnDef(exp.Args{"this": this, "kind": kind, "constraints": constraints, "position": position}), nil, nil)
}

// parseConstraint ports _parse_constraint (parser.py:7462-7468): a named `CONSTRAINT <id>
// (<unnamed constraints>)` clause, or (when CONSTRAINT isn't present) an unnamed
// schema-level constraint drawn from schemaUnnamedConstraints (CHECK/FOREIGN KEY/PRIMARY
// KEY/UNIQUE, +mysql's FULLTEXT/INDEX/KEY/SPATIAL).
func (p *Parser) parseConstraint() exp.Expression {
	if !p.match(tokens.CONSTRAINT) {
		return p.parseUnnamedConstraint(p.schemaUnnamedConstraints())
	}
	return p.expression(exp.Constraint(exp.Args{
		"this":        p.parseIdVar(true, nil),
		"expressions": p.parseUnnamedConstraints(),
	}), nil, nil)
}

// parseUnnamedConstraints ports _parse_unnamed_constraints (parser.py:7470-7478): the body
// of a named CONSTRAINT, a CSV of unnamed constraints (matched against every registered
// CONSTRAINT_PARSERS key, not just the schema-level subset) or plain function calls.
func (p *Parser) parseUnnamedConstraints() []exp.Expression {
	var constraints []exp.Expression
	for {
		constraint := p.parseUnnamedConstraint(nil)
		if constraint == nil {
			constraint = p.parseFunction(nil, false, true, false)
		}
		if constraint == nil {
			break
		}
		constraints = append(constraints, constraint)
	}
	return constraints
}

// parseUnnamedConstraint ports _parse_unnamed_constraint (parser.py:7480-7498). constraints
// filters which texts are eligible (nil means "any CONSTRAINT_PARSERS key", mirroring
// `constraints or self.CONSTRAINT_PARSERS`).
func (p *Parser) parseUnnamedConstraint(constraints map[string]bool) exp.Expression {
	index := p.index
	keys := constraints
	if keys == nil {
		keys = p.constraintParserKeys()
	}
	if p.match(tokens.IDENTIFIER, false) || !p.matchTexts(keys) {
		return nil
	}
	key := stringsUpper(p.prev.Text)
	fn, ok := p.constraintParsers()[key]
	if !ok {
		p.raiseError("No parser found for schema constraint " + key + ".")
		return nil
	}
	result := fn(p)
	if result == nil {
		p.retreat(index)
	}
	return result
}

// parseColumnConstraint ports _parse_column_constraint (parser.py:7443-7460): an optional
// `CONSTRAINT <id>` name, followed by a CONSTRAINT_PARSERS-dispatched kind.
// PROCEDURE_OPTIONS (parser.py:1636) is empty for base/mysql/postgres (only T-SQL overrides
// it), so procedure_option_follows is always false and is omitted here.
func (p *Parser) parseColumnConstraint() exp.Expression {
	var this exp.Expression
	if p.match(tokens.CONSTRAINT) {
		this = p.parseIdVar(true, nil)
	}
	if p.matchTexts(p.constraintParserKeys()) {
		key := stringsUpper(p.prev.Text)
		var constraint exp.Expression
		if fn := p.constraintParsers()[key]; fn != nil {
			constraint = fn(p)
		}
		if constraint == nil {
			p.retreat(p.index - 1)
			return nil
		}
		return p.expression(exp.ColumnConstraint(exp.Args{"this": this, "kind": constraint}), nil, nil)
	}
	return this
}

func (p *Parser) parseAsCommand(start tokens.Token) exp.Expression {
	for p.curr.IsValid() {
		p.advance()
	}
	text := p.findSQL(start, p.prev)
	runes := []rune(text)
	size := len([]rune(start.Text))
	return p.expression(exp.Command(exp.Args{"this": string(runes[:size]), "expression": string(runes[size:])}), nil, nil)
}

func (p *Parser) parseDDLSelect() exp.Expression {
	return p.parseQueryModifiers(p.parseSetOperations(p.parseSelect(true, false, false)))
}

// propertyParserFunc mirrors one PROPERTY_PARSERS entry (parser.py:1227-1341): `default`
// carries the `_match(DEFAULT) and ...` prefix upstream passes as a `default=True` kwarg
// (CHARACTER SET/CHARSET and COLLATE consult it in the ported subset).
type propertyParserFunc func(p *Parser, isDefault bool) exp.Expression

// propertyParsers contains the currently generator-safe base subset of PROPERTY_PARSERS.
// Although pinned upstream base registers CLUSTERED, EXTERNAL, LOCATION, ROW, STORED,
// TBLPROPERTIES, and USING at
// /Users/sjcho/repos/sqlglot-go-hive/.reference/sqlglot-v30.12.0/sqlglot/parser.py:
// 1243,1268,1287,1310,1326,1328,1337, this parser-only slice keeps them dialect-local until
// matching generator placement and renderers exist: a partial structured parse could silently
// drop or error instead of failing
// closed to Command. They can return here only in a future paired parser+generator slice;
// other dialect-specific additions and replacements live in the override seam.
var propertyParsers map[string]propertyParserFunc

func init() {
	propertyParsers = map[string]propertyParserFunc{
		"ALGORITHM": func(p *Parser, _ bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				return p.expression(exp.AlgorithmProperty(exp.Args{"this": this}), nil, nil)
			})
		},
		"AUTO_INCREMENT": func(p *Parser, _ bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				return p.expression(exp.AutoIncrementProperty(exp.Args{"this": this}), nil, nil)
			})
		},
		"CALLED": func(p *Parser, _ bool) exp.Expression { return p.parseCalledOnNullInputProperty() },
		"CHARSET": func(p *Parser, isDefault bool) exp.Expression {
			return p.parseCharacterSet(isDefault)
		},
		"CHARACTER SET": func(p *Parser, isDefault bool) exp.Expression {
			return p.parseCharacterSet(isDefault)
		},
		"COLLATE": func(p *Parser, isDefault bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				// Upstream builds CollateProperty via _parse_property_assignment(**kwargs);
				// the `default` kwarg is present only when a leading DEFAULT set it True
				// (parser.py:2799-2800). Omit it otherwise to match repr()/ToS() exactly,
				// unlike CharacterSetProperty whose parser always records default.
				args := exp.Args{"this": this}
				if isDefault {
					args["default"] = true
				}
				return p.expression(exp.CollateProperty(args), nil, nil)
			})
		},
		"COMMENT": func(p *Parser, _ bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				return p.expression(exp.SchemaCommentProperty(exp.Args{"this": this}), nil, nil)
			})
		},
		"CONTAINS": func(p *Parser, _ bool) exp.Expression { return p.parseContainsProperty() },
		"DEFINER":  func(p *Parser, _ bool) exp.Expression { return p.parseDefiner() },
		"DETERMINISTIC": func(p *Parser, _ bool) exp.Expression {
			return p.parseStabilityProperty("IMMUTABLE")
		},
		"ENGINE": func(p *Parser, _ bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				return p.expression(exp.EngineProperty(exp.Args{"this": this}), nil, nil)
			})
		},
		"FORMAT": func(p *Parser, _ bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				return p.expression(exp.FileFormatProperty(exp.Args{"this": this}), nil, nil)
			})
		},
		"IMMUTABLE": func(p *Parser, _ bool) exp.Expression { return p.parseStabilityProperty("IMMUTABLE") },
		"INHERITS": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.InheritsProperty(exp.Args{
				"expressions": p.parseWrappedCsv(func() exp.Expression {
					return p.parseTable(false, false, nil, false, false, false, false)
				}),
			}), nil, nil)
		},
		"LANGUAGE": func(p *Parser, _ bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				return p.expression(exp.LanguageProperty(exp.Args{"this": this}), nil, nil)
			})
		},
		"LIKE":    func(p *Parser, _ bool) exp.Expression { return p.parseCreateLike() },
		"LOCK":    func(p *Parser, _ bool) exp.Expression { return p.parseLocking() },
		"LOCKING": func(p *Parser, _ bool) exp.Expression { return p.parseLocking() },
		"MATERIALIZED": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.MaterializedProperty(nil), nil, nil)
		},
		"MODIFIES": func(p *Parser, _ bool) exp.Expression { return p.parseModifiesProperty() },
		"NO":       func(p *Parser, _ bool) exp.Expression { return p.parseNoProperty() },
		"ON":       func(p *Parser, _ bool) exp.Expression { return p.parseOnProperty() },
		"PARTITION": func(p *Parser, _ bool) exp.Expression {
			return p.parsePartitionedOf()
		},
		"PARTITION BY": func(p *Parser, _ bool) exp.Expression {
			return p.parsePartitionedBy()
		},
		"PARTITIONED BY": func(p *Parser, _ bool) exp.Expression {
			return p.parsePartitionedBy()
		},
		"PARTITIONED_BY": func(p *Parser, _ bool) exp.Expression {
			return p.parsePartitionedBy()
		},
		"READS":   func(p *Parser, _ bool) exp.Expression { return p.parseReadsProperty() },
		"RETURNS": func(p *Parser, _ bool) exp.Expression { return p.parseReturnsProperty() },
		"ROW_FORMAT": func(p *Parser, _ bool) exp.Expression {
			return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
				return p.expression(exp.RowFormatProperty(exp.Args{"this": this}), nil, nil)
			})
		},
		"SECURITY":     func(p *Parser, _ bool) exp.Expression { return p.parseSQLSecurity() },
		"SQL SECURITY": func(p *Parser, _ bool) exp.Expression { return p.parseSQLSecurity() },
		"STABLE":       func(p *Parser, _ bool) exp.Expression { return p.parseStabilityProperty("STABLE") },
		"STRICT": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.StrictProperty(nil), nil, nil)
		},
		"TEMP": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.TemporaryProperty(nil), nil, nil)
		},
		"TEMPORARY": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.TemporaryProperty(nil), nil, nil)
		},
		"UNLOGGED": func(p *Parser, _ bool) exp.Expression {
			return p.expression(exp.UnloggedProperty(nil), nil, nil)
		},
		"WITH": func(p *Parser, _ bool) exp.Expression { return p.parseWithProperty() },
	}
	registerDialectParserOverrides("mysql", dialectParserOverrideSet{
		// GROUP_CONCAT: MySQL FUNCTION_PARSERS entry (parsers/mysql.py:156); see
		// parseGroupConcat in dialect_mysql_overrides.go.
		FunctionParsers: map[string]parserOverrideFunc{
			"GROUP_CONCAT": (*Parser).parseGroupConcat,
		},
		// mysql-replace divergence from dialects/mysql.py:154: the tokenizer no longer
		// packs REPLACE as Command, so this override can disambiguate REPLACE(...).
		StatementParsers: map[tokens.TokenType]parserOverrideFunc{
			tokens.REPLACE: (*Parser).parseMySQLReplace,
		},
		PropertyParsers: map[string]propertyParserFunc{
			"LOCK": func(p *Parser, _ bool) exp.Expression {
				return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
					return p.expression(exp.LockProperty(exp.Args{"this": this}), nil, nil)
				})
			},
			"PARTITION BY": func(p *Parser, _ bool) exp.Expression {
				return p.parseMySQLPartitionProperty()
			},
		},
	})

	// parsers/postgres.py:89-91 replaces SET with SetConfigProperty(this=_parse_set()).
	// parseSet intentionally retains upstream's whole-statement Command fallback behavior.
	registerDialectParserOverrides("postgres", dialectParserOverrideSet{
		StatementParsers: map[tokens.TokenType]parserOverrideFunc{
			tokens.DESCRIBE: (*Parser).parsePostgresExplain,
		},
		PropertyParsers: map[string]propertyParserFunc{
			"SET": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.SetConfigProperty(exp.Args{"this": p.parseSet()}), nil, nil)
			},
		},
	})
}

// parseTailProperty is the singleton adapter for _parse_function_properties
// (parser.py:7091-7106), intentionally omitting the generic key=value fallback after AS.
func (p *Parser) parseTailProperty() exp.Expression {
	if p.matchTexts(p.propertyParserKeys()) {
		return p.propertyParser(stringsUpper(p.prev.Text))(p, false)
	}
	if p.match(tokens.DEFAULT) && p.matchTexts(p.propertyParserKeys()) {
		return p.propertyParser(stringsUpper(p.prev.Text))(p, true)
	}
	return nil
}

// parseTailProperties ports _parse_function_properties (parser.py:7091-7106). It returns
// the raw list so CREATE FUNCTION can merge its successive property passes in order.
func (p *Parser) parseTailProperties() []exp.Expression {
	var properties []exp.Expression
	for {
		property := p.parseTailProperty()
		if property == nil {
			break
		}
		if property.Kind() == exp.KindProperties {
			properties = append(properties, property.Expressions()...)
		} else {
			properties = append(properties, property)
		}
	}
	return properties
}

// parsePropertyAssignment ports _parse_property_assignment (parser.py:2873-2877): `[=] [AS]
// <unquoted field>`.
func (p *Parser) parsePropertyAssignment(build func(this exp.Expression) exp.Expression) exp.Expression {
	p.match(tokens.EQ)
	p.match(tokens.ALIAS)
	return build(p.parseUnquotedField())
}

// parseCharacterSet ports _parse_character_set (parser.py:3382-3386).
func (p *Parser) parseCharacterSet(isDefault bool) exp.Expression {
	p.match(tokens.EQ)
	return p.expression(exp.CharacterSetProperty(exp.Args{"this": p.parseVarOrString(false), "default": isDefault}), nil, nil)
}

// parseSQLSecurity ports _parse_sql_security (parser.py:2901-2905).
func (p *Parser) parseSQLSecurity() exp.Expression {
	var this any
	if p.matchTexts(securityPropertyKeywords) {
		this = stringsUpper(p.prev.Text)
	}
	return p.expression(exp.SqlSecurityProperty(exp.Args{"this": this}), nil, nil)
}

// securityPropertyKeywords mirrors SECURITY_PROPERTY_KEYWORDS (parser.py:1751).
var securityPropertyKeywords = map[string]bool{"DEFINER": true, "INVOKER": true, "NONE": true}

// parseCalledOnNullInputProperty ports _parse_called_on_null_input_property
// (parser.py:2913-2918). On failure it retreats past the already-consumed "CALLED" keyword
// too (self._retreat(self._index - 1)), matching upstream exactly.
func (p *Parser) parseCalledOnNullInputProperty() exp.Expression {
	if !p.matchTextSeq("ON", "NULL", "INPUT") {
		p.retreat(p.index - 1)
		return nil
	}
	return p.expression(exp.CalledOnNullInputProperty(nil), nil, nil)
}

// parseStabilityProperty builds the exp.StabilityProperty(this=Literal.string(name)) node
// shared by the IMMUTABLE/STABLE/DETERMINISTIC PROPERTY_PARSERS entries (parser.py:1253-1255,
// 1275-1277,1323-1325).
func (p *Parser) parseStabilityProperty(name string) exp.Expression {
	return p.expression(exp.StabilityProperty(exp.Args{"this": exp.LiteralString(name)}), nil, nil)
}

// parseReturnsProperty ports _parse_returns (parser.py:3394-3414), minus its `RETURNS
// TABLE(...)` branch (parser.py:3397-3407, a BigQuery/DuckDB-oriented table-valued-function
// signature - deferred, no target-gap SQL uses it: a bare TABLE token here is simply left
// unconsumed, degrading the enclosing CREATE to a Command).
func (p *Parser) parseReturnsProperty() exp.Expression {
	var value exp.Expression
	var null any
	if p.matchTextSeq("NULL", "ON", "NULL", "INPUT") {
		null = true
	} else {
		value = p.parseTypes(false, false, true, false)
	}
	return p.expression(exp.ReturnsProperty(exp.Args{"this": value, "is_table": false, "null": null}), nil, nil)
}
