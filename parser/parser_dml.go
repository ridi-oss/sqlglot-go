package parser

import (
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func (p *Parser) parseInsert() exp.Expression {
	keywordTok := p.prev
	hint := p.parseHint()
	overwrite := p.match(tokens.OVERWRITE)
	ignore := p.match(tokens.IGNORE)
	// LOCAL only ever precedes DIRECTORY (parser.py:3486-3502); matched unconditionally
	// here (mirroring upstream), it's simply a no-op miss for the ordinary INTO/TABLE form.
	local := p.matchTextSeq("LOCAL")
	var alternative any
	var isFunction bool
	var this exp.Expression
	var comments []string
	if p.matchTextSeq("DIRECTORY") {
		// Hive/Spark `INSERT OVERWRITE [LOCAL] DIRECTORY '<path>' [ROW FORMAT ...] <select>`
		// (parser.py:3495-3502).
		this = p.expression(exp.Directory(exp.Args{
			"this":       p.parseVarOrString(false),
			"local":      local,
			"row_format": p.parseRowFormat(true),
		}), nil, nil)
	} else {
		if p.match(tokens.OR) {
			if p.matchTexts(insertAlternatives) {
				alternative = p.prev.Text
			}
		}
		p.match(tokens.INTO)
		comments = p.prevComments
		// mysql-insert-set/mysql-replace divergence from parser.py:3511-3514: when TABLE is
		// followed by target-column or source syntax, it is the target's literal name, not the
		// optional keyword. This becomes observable once dialects/mysql.py:154 no longer tokenizes
		// REPLACE as Command. L_PAREN is listed explicitly even though selectStartTokens also
		// contains it, because here it most commonly starts the target's explicit column list.
		tableIsTarget := p.dialect.Name == "mysql" && p.curr.TokenType == tokens.TABLE &&
			(p.next.TokenType == tokens.L_PAREN || p.next.TokenType == tokens.SET ||
				p.next.TokenType == tokens.VALUES || selectStartTokens[p.next.TokenType])
		if !tableIsTarget {
			p.match(tokens.TABLE)
		}
		isFunction = p.match(tokens.FUNCTION)
		if tableIsTarget {
			// Keep the identity-corpus spelling `REPLACE INTO table SELECT ...` source-preserving:
			// a normal Identifier would be auto-quoted because MySQL reserves TABLE. A Var remains
			// a valid Table child and reparses through this same MySQL target-name branch.
			tableToken := p.curr
			p.advance()
			name := p.expression(exp.Var(exp.Args{"this": tableToken.Text}), &tableToken, nil)
			this = p.parseSchema(p.expression(exp.Table(exp.Args{"this": name}), nil, nil))
		} else if isFunction {
			this = p.parseFunction(nil, false, true, false)
		} else {
			this = p.parseInsertTable()
		}
	}
	returning := p.parseReturning()
	byName := p.matchTextSeq("BY", "NAME")
	exists := p.parseExists(false)
	var where exp.Expression
	if p.matchPair(tokens.REPLACE, tokens.WHERE, true) {
		where = p.parseDisjunction()
	}
	default_ := p.matchTextSeq("DEFAULT", "VALUES")
	var expression exp.Expression
	if p.dialect.Name == "mysql" && p.match(tokens.SET) {
		// mysql-insert-set divergence from parser.py:3486-3542: the pinned INSERT grammar
		// does not consume SET; desugar it into the ordinary Schema + one-row Values shape.
		this, expression = p.parseMySQLInsertSet(this, p.parseCsv(p.parseEquality))
	} else {
		// These firstExpression calls evaluate their alternatives eagerly (unlike a
		// short-circuit `or`), which is safe here only because the alternatives sit at
		// non-overlapping token positions: parseDerivedTableValues/parseReturning consume
		// nothing when they return nil, and the second parseReturning is a no-op once the
		// first grabbed the RETURNING clause. Contrast parseLateral, where eager evaluation
		// would drop a trailing alias.
		expression = firstExpression(p.parseDerivedTableValues(), p.parseDDLSelect())
	}
	conflict := p.parseOnConflict()
	returning = firstExpression(returning, p.parseReturning())
	args := exp.Args{
		"hint":        hint,
		"overwrite":   overwrite,
		"ignore":      ignore,
		"alternative": alternative,
		"is_function": isFunction,
		"this":        this,
		"returning":   returning,
		"by_name":     byName,
		"exists":      exists,
		"where":       where,
		"default":     default_,
		"expression":  expression,
		"conflict":    conflict,
	}
	// mysql-replace is a Go-only extension: upstream v30.12.0 leaves statement-leading
	// REPLACE raw via dialects/mysql.py:154, while this port structures the supported grammar.
	if keywordTok.TokenType == tokens.REPLACE {
		args["replace"] = true
	}
	return p.expression(exp.Insert(args), &keywordTok, comments)
}

func (p *Parser) parseInsertTable() exp.Expression {
	this := p.parseTable(true, false, nil, false, false, true, false)
	if this != nil && this.Kind() == exp.KindTable && p.match(tokens.ALIAS, false) {
		this.Set("alias", p.parseTableAlias(nil))
	}
	return this
}

func (p *Parser) parseUpdate() exp.Expression {
	hint := p.parseHint()
	kwargs := exp.Args{"hint": hint, "this": p.parseTable(false, true, updateAliasTokens, false, false, false, false)}
	for p.curr.IsValid() {
		switch {
		case p.match(tokens.SET):
			kwargs["expressions"] = p.parseCsv(p.parseEquality)
		case p.match(tokens.RETURNING, false):
			kwargs["returning"] = p.parseReturning()
		case p.match(tokens.FROM, false):
			kwargs["from_"] = p.parseFrom(true, false, false)
		case p.match(tokens.WHERE, false):
			kwargs["where"] = p.parseWhere(false)
		case p.match(tokens.ORDER_BY, false):
			kwargs["order"] = p.parseOrder(nil, false)
		case p.match(tokens.LIMIT, false):
			kwargs["limit"] = p.parseLimit(nil, false, false)
		default:
			return p.expression(exp.Update(kwargs), nil, nil)
		}
	}
	return p.expression(exp.Update(kwargs), nil, nil)
}

func (p *Parser) parseDelete() exp.Expression {
	hint := p.parseHint()
	var tables []exp.Expression
	tableNoJoin := func() exp.Expression { return p.parseTable(false, false, nil, false, false, false, false) }
	tableWithJoins := func() exp.Expression { return p.parseTable(false, true, nil, false, false, false, false) }
	if !p.match(tokens.FROM, false) {
		if parsed := p.parseCsv(tableNoJoin); len(parsed) > 0 {
			tables = parsed
		}
	}
	returning := p.parseReturning()
	var this exp.Expression
	if p.match(tokens.FROM) {
		this = p.parseTable(false, true, nil, false, false, false, false)
	}
	var using []exp.Expression
	if p.match(tokens.USING) {
		using = p.parseCsv(tableWithJoins)
	}
	where := p.parseWhere(false)
	returning = firstExpression(returning, p.parseReturning())
	order := p.parseOrder(nil, false)
	limit := p.parseLimit(nil, false, false)
	return p.expression(exp.Delete(exp.Args{
		"hint":      hint,
		"tables":    tables,
		"this":      this,
		"using":     using,
		"where":     where,
		"returning": returning,
		"order":     order,
		"limit":     limit,
	}), nil, nil)
}

func (p *Parser) parseReturning() exp.Expression {
	if !p.match(tokens.RETURNING) {
		return nil
	}
	args := exp.Args{"expressions": p.parseCsv(p.parseExpression)}
	if p.match(tokens.INTO) {
		args["into"] = p.parseTablePart(false)
	}
	return p.expression(exp.Returning(args), nil, nil)
}

func (p *Parser) parseOnConflict() exp.Expression {
	conflict := p.matchTextSeq("ON", "CONFLICT")
	duplicate := p.matchTextSeq("ON", "DUPLICATE", "KEY")
	if !conflict && !duplicate {
		return nil
	}
	var constraint exp.Expression
	var conflictKeys []exp.Expression
	if conflict {
		if p.matchTextSeq("ON", "CONSTRAINT") {
			constraint = p.parseIdVar(false, nil)
		} else if p.match(tokens.L_PAREN) {
			conflictKeys = p.parseCsv(func() exp.Expression { return p.parseOrdered(p.parseColumn) })
			p.matchRParen(nil)
		}
	}
	indexPredicate := p.parseWhere(false)
	action := p.parseVarFromOptions(conflictActions, true)
	var expressions []exp.Expression
	if p.prev.TokenType == tokens.UPDATE {
		p.match(tokens.SET)
		expressions = p.parseCsv(p.parseEquality)
	}
	return p.expression(exp.OnConflict(exp.Args{
		"duplicate":       duplicate,
		"expressions":     expressions,
		"action":          action,
		"conflict_keys":   conflictKeys,
		"index_predicate": indexPredicate,
		"constraint":      constraint,
		"where":           p.parseWhere(false),
	}), nil, nil)
}

// parseExists ports _parse_exists (parser.py). Upstream's implementation is a
// short-circuiting `and` chain over _match calls, so it returns the bool False
// (never None) when the IF [NOT] EXISTS sequence is absent. Mirroring that here —
// returning false rather than nil — is what makes downstream nodes (Alter, Drop,
// Insert, Truncate, ColumnDef, ...) carry the explicit exists=False that repr()/ToS()
// expects. Callers that need a presence test must use a truthy check, not `!= nil`.
func (p *Parser) parseExists(not_ bool) any {
	if !p.matchTextSeq("IF") {
		return false
	}
	if not_ && !p.match(tokens.NOT) {
		return false
	}
	if !p.match(tokens.EXISTS) {
		return false
	}
	return true
}

func (p *Parser) parseVarFromOptions(options optionsType, raiseUnmatched bool) exp.Expression {
	start := p.curr
	if !start.IsValid() {
		return nil
	}
	option := stringsUpper(start.Text)
	continuations, present := options[option]
	index := p.index
	p.advance()
	matched := false
	for _, kw := range continuations {
		if p.matchTextSeq(kw...) {
			option += " " + strings.Join(kw, " ")
			matched = true
			break
		}
	}
	if !matched {
		if len(continuations) > 0 || !present {
			if raiseUnmatched {
				p.raiseError("Unknown option " + option)
			}
			p.retreat(index)
			return nil
		}
	}
	return exp.Var(exp.Args{"this": option})
}

func (p *Parser) parseMerge() exp.Expression {
	p.match(tokens.INTO)
	// joins=false matches upstream _parse_merge; the next token is always ON/USING/WHEN,
	// which parseJoin can't consume anyway, but we keep the flag faithful.
	target := p.parseTable(false, false, nil, false, false, false, false)
	if target != nil && p.match(tokens.ALIAS, false) {
		target.Set("alias", p.parseTableAlias(nil))
	}
	p.match(tokens.USING)
	using := p.parseTable(false, false, nil, false, false, false, false)
	args := exp.Args{"this": target, "using": using}
	if p.match(tokens.ON) {
		args["on"] = p.parseDisjunction()
	}
	if p.match(tokens.USING) {
		args["using_cond"] = p.parseUsingIdentifiers()
	}
	args["whens"] = p.parseWhenMatched()
	if returning := p.parseReturning(); returning != nil {
		args["returning"] = returning
	}
	return p.expression(exp.Merge(args), nil, nil)
}

func (p *Parser) parseUsingIdentifiers() []exp.Expression {
	return p.parseWrappedCsv(func() exp.Expression {
		c := p.parseColumn()
		if c != nil && c.Kind() == exp.KindColumn {
			return c.This()
		}
		return c
	}, true)
}

func (p *Parser) parseWhenMatched() exp.Expression {
	whens := []exp.Expression{}
	for p.match(tokens.WHEN) {
		matched := !p.match(tokens.NOT)
		p.matchTextSeq("MATCHED")
		source := false
		if p.matchTextSeq("BY", "TARGET") {
			source = false
		} else {
			source = p.matchTextSeq("BY", "SOURCE")
		}
		var condition exp.Expression
		if p.match(tokens.AND) {
			condition = p.parseDisjunction()
		}
		p.match(tokens.THEN)
		var then exp.Expression
		if p.match(tokens.INSERT) {
			if star := p.parseStar(); star != nil {
				then = exp.Insert(exp.Args{"this": star})
			} else {
				var insThis exp.Expression
				if p.matchTextSeq("ROW") {
					insThis = exp.Var(exp.Args{"this": "ROW"})
				} else {
					insThis = p.parseValue(false)
				}
				var insExpr exp.Expression
				if p.matchTextSeq("VALUES") {
					insExpr = p.parseValue(true)
				}
				then = exp.Insert(exp.Args{"this": insThis, "expression": insExpr, "where": p.parseWhere(false)})
			}
		} else if p.match(tokens.UPDATE) {
			if exprs := p.parseStar(); exprs != nil {
				then = exp.Update(exp.Args{"expressions": exprs})
			} else {
				var setExprs []exp.Expression
				if p.match(tokens.SET) {
					setExprs = p.parseCsv(p.parseEquality)
				}
				then = exp.Update(exp.Args{"expressions": setExprs, "where": p.parseWhere(false)})
			}
		} else if p.match(tokens.DELETE) {
			then = exp.Var(exp.Args{"this": p.prev.Text})
		} else {
			then = p.parseVarFromOptions(conflictActions, true)
		}
		whens = append(whens, p.expression(exp.When(exp.Args{"matched": matched, "source": source, "condition": condition, "then": then}), nil, nil))
	}
	return p.expression(exp.Whens(exp.Args{"expressions": whens}), nil, nil)
}

// parseRow ports _parse_row (parser.py:3603-3606), the standalone CREATE TABLE
// property adapter for ROW FORMAT ... clauses.
func (p *Parser) parseRow() exp.Expression {
	if !p.match(tokens.FORMAT) {
		return nil
	}
	return p.parseRowFormat(false)
}

// parseRowFormat ports _parse_row_format (parser.py:3619-3651): the Hive `ROW FORMAT
// DELIMITED ...` / `ROW FORMAT SERDE '...'` clause. parseInsert's DIRECTORY branch reaches
// it with matchRow=true, while parseRow has already consumed ROW FORMAT.
func (p *Parser) parseRowFormat(matchRow bool) exp.Expression {
	if matchRow && !p.matchPair(tokens.ROW, tokens.FORMAT, true) {
		return nil
	}
	if p.matchTextSeq("SERDE") {
		return p.expression(exp.RowFormatSerdeProperty(exp.Args{
			"this":             p.parseString(),
			"serde_properties": p.parseSerdeProperties(false),
		}), nil, nil)
	}
	p.matchTextSeq("DELIMITED")
	args := exp.Args{}
	if p.matchTextSeq("FIELDS", "TERMINATED", "BY") {
		args["fields"] = p.parseString()
		if p.matchTextSeq("ESCAPED", "BY") {
			args["escaped"] = p.parseString()
		}
	}
	if p.matchTextSeq("COLLECTION", "ITEMS", "TERMINATED", "BY") {
		args["collection_items"] = p.parseString()
	}
	if p.matchTextSeq("MAP", "KEYS", "TERMINATED", "BY") {
		args["map_keys"] = p.parseString()
	}
	if p.matchTextSeq("LINES", "TERMINATED", "BY") {
		args["lines"] = p.parseString()
	}
	if p.matchTextSeq("NULL", "DEFINED", "AS") {
		args["null"] = p.parseString()
	}
	return p.expression(exp.RowFormatDelimitedProperty(args), nil, nil)
}

// parseSerdeProperties ports _parse_serde_properties (parser.py:3608-3617): `[WITH]
// SERDEPROPERTIES (...)`. Matching the dedicated token keeps this Hive-only syntax fail-closed
// for dialects whose tokenizers leave SERDEPROPERTIES as an ordinary identifier.
func (p *Parser) parseSerdeProperties(withKeyword bool) exp.Expression {
	index := p.index
	with_ := withKeyword || p.matchTextSeq("WITH")
	if !p.match(tokens.SERDE_PROPERTIES) {
		p.retreat(index)
		return nil
	}
	return p.expression(exp.SerdeProperties(exp.Args{
		"expressions": p.parseWrappedProperties(),
		"with_":       with_,
	}), nil, nil)
}

// tableIndexHintTokens/indexOrKeyTokens back parseTableHints below (mirroring upstream's
// TABLE_INDEX_HINT_TOKENS, parser.py:1673, and the inline `_match_set((INDEX, KEY))`,
// parser.py:4655).
var tableIndexHintTokens = map[tokens.TokenType]bool{tokens.FORCE: true, tokens.IGNORE: true, tokens.USE: true}
var indexOrKeyTokens = map[tokens.TokenType]bool{tokens.INDEX: true, tokens.KEY: true}

// parseTableHints ports _parse_table_hints (parser.py:4636-4662): T-SQL `WITH (...)` table
// hints and MySQL `USE|FORCE|IGNORE INDEX [FOR JOIN|ORDER BY|GROUP BY] (...)` index hints.
// Called from parseTable, right after alias parsing (parser.py:4920).
func (p *Parser) parseTableHints() []exp.Expression {
	var hints []exp.Expression
	if p.matchPair(tokens.WITH, tokens.L_PAREN, true) {
		hints = append(hints, p.expression(exp.WithTableHint(exp.Args{
			"expressions": p.parseCsv(func() exp.Expression {
				if fn := p.parseFunction(nil, false, false, false); fn != nil {
					return fn
				}
				return p.parseVar(true, nil, false)
			}),
		}), nil, nil))
		p.matchRParen(nil)
		return hints
	}
	for p.matchSet(tableIndexHintTokens) {
		args := exp.Args{"this": stringsUpper(p.prev.Text)}
		p.matchSet(indexOrKeyTokens)
		if p.match(tokens.FOR) {
			if tok := p.advanceAny(true); tok != nil {
				args["target"] = stringsUpper(tok.Text)
			}
		}
		args["expressions"] = p.parseWrappedIdVars()
		hints = append(hints, p.expression(exp.IndexTableHint(args), nil, nil))
	}
	return hints
}
