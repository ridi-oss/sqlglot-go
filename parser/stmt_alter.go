package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.ALTER] = (*Parser).parseAlter
}

// alterParsers/mysqlAlterParsers port ALTER_PARSERS (parser.py:1437-1449). Only
// ADD/AS/ALTER/DELETE/DROP/RENAME/SET are ported (CLUSTER BY and SWAP are Redshift/
// Snowflake-only, out of the base/mysql/postgres dialect scope and absent from that
// corpus). Each entry returns []exp.Expression directly (mirroring ensure_list(parser(self))
// at the single call site in parseAlter), since upstream's sub-parsers are a mix of
// list-returning (_parse_alter_table_add/_drop) and single-expression-returning
// (_parse_select/_parse_alter_table_alter/.../_parse_alter_table_set) functions.
var (
	alterParsers map[string]func(*Parser) []exp.Expression

	mysqlAlterParsers map[string]func(*Parser) []exp.Expression

	// alterAlterParsers/mysqlAlterAlterParsers port ALTER_ALTER_PARSERS (parser.py:1451-
	// 1456): base's four entries (DISTKEY/DISTSTYLE/SORTKEY/COMPOUND) are Redshift-only and
	// out of scope, so the base table is empty; mysql adds INDEX (parsers/mysql.py:260-263).
	alterAlterParsers         = map[string]func(*Parser) exp.Expression{}
	alterAlterParserKeys      = map[string]bool{}
	mysqlAlterAlterParsers    map[string]func(*Parser) exp.Expression
	mysqlAlterAlterParserKeys map[string]bool
)

func init() {
	alterParsers = map[string]func(*Parser) []exp.Expression{
		"ADD": (*Parser).parseAlterTableAdd,
		"AS":  func(p *Parser) []exp.Expression { return ensureListExpr(p.parseSelect()) },
		"ALTER": func(p *Parser) []exp.Expression {
			return ensureListExpr(p.parseAlterTableAlter())
		},
		"DELETE": func(p *Parser) []exp.Expression {
			return ensureListExpr(p.expression(exp.Delete(exp.Args{"where": p.parseWhere(false)}), nil, nil))
		},
		"DROP":   (*Parser).parseAlterTableDrop,
		"RENAME": func(p *Parser) []exp.Expression { return ensureListExpr(p.parseAlterTableRename()) },
		"SET":    func(p *Parser) []exp.Expression { return ensureListExpr(p.parseAlterTableSet()) },
	}

	// mysqlAlterParsers ports parsers/mysql.py:253-258 MySQLParser.ALTER_PARSERS:
	// `**parser.Parser.ALTER_PARSERS` plus CHANGE/MODIFY/AUTO_INCREMENT.
	mysqlAlterParsers = make(map[string]func(*Parser) []exp.Expression, len(alterParsers)+3)
	for k, v := range alterParsers {
		mysqlAlterParsers[k] = v
	}
	mysqlAlterParsers["CHANGE"] = func(p *Parser) []exp.Expression {
		return ensureListExpr(p.parseAlterTableModify(true))
	}
	mysqlAlterParsers["MODIFY"] = func(p *Parser) []exp.Expression {
		return ensureListExpr(p.parseAlterTableModify(false))
	}
	mysqlAlterParsers["AUTO_INCREMENT"] = func(p *Parser) []exp.Expression {
		return ensureListExpr(p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
			return p.expression(exp.AutoIncrementProperty(exp.Args{"this": this}), nil, nil)
		}))
	}

	mysqlAlterAlterParsers = map[string]func(*Parser) exp.Expression{
		"INDEX": func(p *Parser) exp.Expression { return p.parseAlterTableAlterIndex() },
	}
	mysqlAlterAlterParserKeys = map[string]bool{"INDEX": true}
}

func (p *Parser) alterParsers() map[string]func(*Parser) []exp.Expression {
	if p.dialect.Name == "mysql" {
		return mysqlAlterParsers
	}
	return alterParsers
}

func (p *Parser) alterAlterParsers() (map[string]func(*Parser) exp.Expression, map[string]bool) {
	if p.dialect.Name == "mysql" {
		return mysqlAlterAlterParsers, mysqlAlterAlterParserKeys
	}
	return alterAlterParsers, alterAlterParserKeys
}

// ensureListExpr mirrors upstream's ensure_list(value) (helper.py) for the single-expression
// case: nil -> empty slice, a value -> a one-element slice.
func ensureListExpr(e exp.Expression) []exp.Expression {
	if e == nil {
		return []exp.Expression{}
	}
	return []exp.Expression{e}
}

// parseAlter ports _parse_alter (parser.py:8923-8973).
func (p *Parser) parseAlter() exp.Expression {
	start := p.prev
	iceberg := p.matchTextSeq("ICEBERG")

	// Must seed with SentinelNone, not the zero Token: a zero-value tokens.Token has TokenType 0,
	// whose IsValid() is true (only SENTINEL is invalid). Using `var alterToken tokens.Token` would
	// make the "no ALTERABLE matched" guard below never fire, so e.g. `ALTER SQL SECURITY = DEFINER
	// VIEW ...` would fall into parseTable and hard-error instead of degrading to a Command (upstream
	// returns a Command: `alter_token = self._match_set(ALTERABLES) and self._prev` is falsy here).
	alterToken := tokens.SentinelNone
	if p.matchSet(alterables) {
		alterToken = p.prev
	}
	if !alterToken.IsValid() {
		return p.parseAsCommand(start)
	}
	if iceberg && alterToken.TokenType != tokens.TABLE {
		return p.parseAsCommand(start)
	}

	exists := p.parseExists(false)
	only := p.matchTextSeq("ONLY")

	var this exp.Expression
	var check any
	var cluster exp.Expression
	if alterToken.TokenType != tokens.SESSION {
		this = p.parseTable(true, false, nil, false, false, p.dialect.AlterTablePartitions, false)
		check = p.matchTextSeq("WITH", "CHECK")
		if p.match(tokens.ON) {
			cluster = p.parseOnProperty()
		}
		if p.next.IsValid() {
			p.advance()
		}
	}

	var parseAction func(*Parser) []exp.Expression
	if p.prev.IsValid() {
		parseAction = p.alterParsers()[stringsUpper(p.prev.Text)]
	}
	if parseAction != nil {
		actions := parseAction(p)
		notValid := p.matchTextSeq("NOT", "VALID")
		options := p.parseCsv(p.parseProperty)
		cascade := p.dialect.AlterTableSupportsCascade && p.matchTextSeq("CASCADE")

		if !p.curr.IsValid() && len(actions) > 0 {
			return p.expression(exp.Alter(exp.Args{
				"this":      this,
				"kind":      stringsUpper(alterToken.Text),
				"exists":    exists,
				"actions":   actions,
				"only":      only,
				"options":   options,
				"cluster":   cluster,
				"not_valid": notValid,
				"check":     check,
				"cascade":   cascade,
				"iceberg":   iceberg,
			}), nil, nil)
		}
	}

	return p.parseAsCommand(start)
}

// parseColumnDefWithExists ports _parse_column_def_with_exists (parser.py:8716-8729).
func (p *Parser) parseColumnDefWithExists() exp.Expression {
	start := p.index
	p.match(tokens.COLUMN)
	existsColumn := p.parseExists(true)
	expression := p.parseFieldDef()
	if expression == nil || expression.Kind() != exp.KindColumnDef {
		p.retreat(start)
		return nil
	}
	expression.Set("exists", existsColumn)
	return expression
}

// parseAddColumn ports _parse_add_column (parser.py:8731-8735).
func (p *Parser) parseAddColumn() exp.Expression {
	if stringsUpper(p.prev.Text) != "ADD" {
		return nil
	}
	return p.parseColumnDefWithExists()
}

// parseAddAlteration ports the `_parse_add_alteration` inner closure of
// _parse_alter_table_add (parser.py:8753-8775).
func (p *Parser) parseAddAlteration() exp.Expression {
	// Optional repeated "ADD" (BigQuery-style `ADD COLUMN a INT, ADD COLUMN b INT`): a no-op
	// on the first CSV item (already consumed by parseAlter's dispatch step), consumed here
	// on subsequent items.
	p.matchTextSeq("ADD")

	if p.matchSet(addConstraintTokens, false) {
		return p.expression(exp.AddConstraint(exp.Args{"expressions": p.parseCsv(p.parseConstraint)}), nil, nil)
	}

	if columnDef := p.parseAddColumn(); columnDef != nil && columnDef.Kind() == exp.KindColumnDef {
		return columnDef
	}

	exists := p.parseExists(true)
	if p.matchPair(tokens.PARTITION, tokens.L_PAREN, false) {
		// this must be parsed before the LOCATION peek (parser.py:8766-8772: exists, this,
		// location, in that order).
		this := p.parseField(true, nil, false)
		var location exp.Expression
		if p.peekTextSeq("LOCATION") {
			location = p.parseProperty()
		}
		return p.expression(exp.AddPartition(exp.Args{
			"exists":   exists,
			"this":     this,
			"location": location,
		}), nil, nil)
	}

	return nil
}

// parseAlterTableAdd ports _parse_alter_table_add (parser.py:8752-8789).
func (p *Parser) parseAlterTableAdd() []exp.Expression {
	if !p.matchSet(addConstraintTokens, false) && (!p.dialect.AlterTableAddRequiredForEachColumn || p.matchTextSeq("COLUMNS")) {
		schema := p.parseSchema(nil)
		if schema != nil {
			return []exp.Expression{schema}
		}
		return p.parseCsv(p.parseColumnDefWithExists)
	}
	return p.parseCsv(p.parseAddAlteration)
}

// parseAlterTableAlter ports _parse_alter_table_alter (parser.py:8791-8825).
func (p *Parser) parseAlterTableAlter() exp.Expression {
	parsers, keys := p.alterAlterParsers()
	if p.matchTexts(keys) {
		return parsers[stringsUpper(p.prev.Text)](p)
	}

	// Many dialects support the ALTER [COLUMN] syntax, so if there is no keyword after
	// ALTER we default to parsing this statement.
	p.match(tokens.COLUMN)
	column := p.parseField(true, nil, false)

	if p.matchPair(tokens.DROP, tokens.DEFAULT, true) {
		return p.expression(exp.AlterColumn(exp.Args{"this": column, "drop": true}), nil, nil)
	}
	if p.matchPair(tokens.SET, tokens.DEFAULT, true) {
		return p.expression(exp.AlterColumn(exp.Args{"this": column, "default": p.parseDisjunction()}), nil, nil)
	}
	if p.match(tokens.COMMENT) {
		return p.expression(exp.AlterColumn(exp.Args{"this": column, "comment": p.parseString()}), nil, nil)
	}
	if p.matchTextSeq("DROP", "NOT", "NULL") {
		return p.expression(exp.AlterColumn(exp.Args{"this": column, "drop": true, "allow_null": true}), nil, nil)
	}
	if p.matchTextSeq("SET", "NOT", "NULL") {
		return p.expression(exp.AlterColumn(exp.Args{"this": column, "allow_null": false}), nil, nil)
	}
	if p.matchTextSeq("SET", "VISIBLE") {
		return p.expression(exp.AlterColumn(exp.Args{"this": column, "visible": "VISIBLE"}), nil, nil)
	}
	if p.matchTextSeq("SET", "INVISIBLE") {
		return p.expression(exp.AlterColumn(exp.Args{"this": column, "visible": "INVISIBLE"}), nil, nil)
	}

	p.matchTextSeq("SET", "DATA")
	p.matchTextSeq("TYPE")
	// dtype must be parsed before collate/using are checked (parser.py:8818-8824: dtype,
	// then collate, then using, in that order) - collecting them out of order would let a
	// trailing COLLATE/USING clause get consumed as (or block) part of the type itself.
	dtype := p.parseTypes(false, false, true, false)
	var collate, using exp.Expression
	if p.match(tokens.COLLATE) {
		collate = p.parseTerm()
	}
	if p.match(tokens.USING) {
		using = p.parseDisjunction()
	}
	return p.expression(exp.AlterColumn(exp.Args{
		"this":    column,
		"dtype":   dtype,
		"collate": collate,
		"using":   using,
	}), nil, nil)
}

// parseAlterTableAlterIndex ports parsers/mysql.py:561-571
// MySQLParser._parse_alter_table_alter_index.
func (p *Parser) parseAlterTableAlterIndex() exp.Expression {
	index := p.parseField(true, nil, false)
	var visible any
	if p.matchTextSeq("VISIBLE") {
		visible = true
	} else if p.matchTextSeq("INVISIBLE") {
		visible = false
	}
	return p.expression(exp.AlterIndex(exp.Args{"this": index, "visible": visible}), nil, nil)
}

// parseAlterTableModify ports parsers/mysql.py:319-339
// MySQLParser._parse_alter_table_modify: MODIFY [COLUMN] col def, or (rename=true)
// CHANGE [COLUMN] old new def.
func (p *Parser) parseAlterTableModify(rename bool) exp.Expression {
	p.match(tokens.COLUMN)
	column := p.parseField(true, nil, false)
	if column == nil {
		return nil
	}

	var renameFrom exp.Expression
	if rename {
		renameFrom = column
		column = p.parseField(true, nil, false)
		if column == nil {
			return nil
		}
	}

	columnDef := p.parseColumnDef(column)
	if columnDef == nil || columnDef.Kind() != exp.KindColumnDef {
		return nil
	}
	return p.expression(exp.ModifyColumn(exp.Args{"this": columnDef, "rename_from": renameFrom}), nil, nil)
}

// parseDropPartition ports _parse_drop_partition (parser.py:8747-8750).
func (p *Parser) parseDropPartition(exists any) exp.Expression {
	return p.expression(exp.DropPartition(exp.Args{
		"expressions": p.parseCsv(p.parsePartition),
		"exists":      exists,
	}), nil, nil)
}

// parseDropColumn ports _parse_drop_column (parser.py:8737-8741): re-matches DROP (retreated
// into position by parseAlterTableDrop) and reuses the top-level parseDrop grammar.
func (p *Parser) parseDropColumn() exp.Expression {
	if !p.match(tokens.DROP) {
		return nil
	}
	drop := p.parseDrop()
	if drop != nil && drop.Kind() != exp.KindCommand {
		if drop.Arg("kind") == nil {
			drop.Set("kind", "COLUMN")
		}
	}
	return drop
}

// parseAlterDropAction ports _parse_alter_drop_action (parser.py:8743-8744), overridden by
// mysql (parsers/mysql.py:314-317) to recognize the bare `DROP PRIMARY KEY` action.
func (p *Parser) parseAlterDropAction() exp.Expression {
	if p.dialect.Name == "mysql" && p.matchPair(tokens.DROP, tokens.PRIMARY_KEY, true) {
		return p.expression(exp.DropPrimaryKey(nil), nil, nil)
	}
	return p.parseDropColumn()
}

// parseAlterTableDrop ports _parse_alter_table_drop (parser.py:8848-8856): the
// PARTITION-vs-column/constraint-drop retreat trick.
func (p *Parser) parseAlterTableDrop() []exp.Expression {
	index := p.index - 1
	partitionExists := p.parseExists(false)
	if p.match(tokens.PARTITION, false) {
		return p.parseCsv(func() exp.Expression { return p.parseDropPartition(partitionExists) })
	}
	p.retreat(index)
	return p.parseCsv(p.parseAlterDropAction)
}

// parseAlterTableRenameBase ports the base _parse_alter_table_rename (parser.py:8858-8873).
func (p *Parser) parseAlterTableRenameBase() exp.Expression {
	if p.match(tokens.COLUMN) || (!p.dialect.AlterRenameRequiresColumn && !p.peekTextSeq("TO")) {
		exists := p.parseExists(false)
		oldColumn := p.parseColumn()
		to := p.matchTextSeq("TO")
		newColumn := p.parseColumn()
		if oldColumn == nil || !to || newColumn == nil {
			return nil
		}
		return p.expression(exp.RenameColumn(exp.Args{"this": oldColumn, "to": newColumn, "exists": exists}), nil, nil)
	}
	p.matchTextSeq("TO")
	return p.expression(exp.AlterRename(exp.Args{"this": p.parseTable(true, false, nil, false, false, false, false)}), nil, nil)
}

// parseAlterTableRename ports parsers/mysql.py:306-312
// MySQLParser._parse_alter_table_rename (RENAME INDEX/KEY -> exp.RenameIndex), falling back
// to the base implementation.
func (p *Parser) parseAlterTableRename() exp.Expression {
	if p.dialect.Name == "mysql" && p.matchTexts(map[string]bool{"INDEX": true, "KEY": true}) {
		old := p.parseField(true, nil, false)
		p.matchTextSeq("TO")
		newField := p.parseField(true, nil, false)
		return p.expression(exp.RenameIndex(exp.Args{"this": old, "to": newField}), nil, nil)
	}
	return p.parseAlterTableRenameBase()
}

// parseAlterTableSet ports _parse_alter_table_set (parser.py:8875-8909). The
// STAGE_FILE_FORMAT/STAGE_COPY_OPTIONS branches (Snowflake-only, needing
// _parse_wrapped_options - not ported anywhere in this codebase) are omitted; unsupported
// occurrences fall through to the shared property parser and ultimately fail closed.
func (p *Parser) parseAlterTableSet() exp.Expression {
	alterSet := p.expression(exp.AlterSet(nil), nil, nil)

	switch {
	case p.match(tokens.L_PAREN, false) || p.matchTextSeq("TABLE", "PROPERTIES"):
		alterSet.Set("expressions", p.parseWrappedCsv(p.parseAssignment))
	case p.peekTextSeq("FILESTREAM_ON"):
		alterSet.Set("expressions", []exp.Expression{p.parseAssignment()})
	case p.matchTexts(map[string]bool{"LOGGED": true, "UNLOGGED": true}):
		alterSet.Set("option", p.expression(exp.Var(exp.Args{"this": stringsUpper(p.prev.Text)}), nil, nil))
	case p.matchTextSeq("WITHOUT") && p.matchTexts(map[string]bool{"CLUSTER": true, "OIDS": true}):
		alterSet.Set("option", p.expression(exp.Var(exp.Args{"this": "WITHOUT " + stringsUpper(p.prev.Text)}), nil, nil))
	case p.matchTextSeq("LOCATION"):
		alterSet.Set("location", p.parseField(false, nil, false))
	case p.matchTextSeq("ACCESS", "METHOD"):
		alterSet.Set("access_method", p.parseField(false, nil, false))
	case p.matchTextSeq("TABLESPACE"):
		alterSet.Set("tablespace", p.parseField(false, nil, false))
	case p.matchTextSeq("FILE", "FORMAT") || p.matchTextSeq("FILEFORMAT"):
		alterSet.Set("file_format", []exp.Expression{p.parseField(false, nil, false)})
	case p.matchTextSeq("TAG") || p.matchTextSeq("TAGS"):
		alterSet.Set("tag", p.parseCsv(p.parseAssignment))
	default:
		if p.matchTextSeq("SERDE") {
			alterSet.Set("serde", p.parseField(false, nil, false))
		}
		properties := p.parseWrapped(func() exp.Expression { return p.parseProperties() }, true)
		alterSet.Set("expressions", []exp.Expression{properties})
	}

	return alterSet
}
