package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func (p *Parser) parseLateral() exp.Expression {
	var crossApply any
	if p.matchPair(tokens.CROSS, tokens.APPLY, true) {
		crossApply = true
	} else if p.matchPair(tokens.OUTER, tokens.APPLY, true) {
		crossApply = false
	}

	var this exp.Expression
	var view any
	var outer any
	if crossApply != nil {
		this = p.parseSelect(false, true, true, true)
	} else if p.match(tokens.LATERAL) {
		this = p.parseSelect(false, true, true, true)
		view = p.match(tokens.VIEW)
		outer = p.match(tokens.OUTER)
	} else {
		return nil
	}

	if this == nil {
		// Mirror upstream `_parse_unnest() or _parse_function() or _parse_id_var(any_token=False)`:
		// short-circuit so a later alternative never runs (and never advances) once an
		// earlier one succeeds. Eager evaluation here would let parseIdVar swallow a
		// trailing bare table alias after parseFunction consumes a `func(...)` source
		// (e.g. `CROSS APPLY func(y) t`), silently dropping the alias.
		this = p.parseUnnest(true)
		if this == nil {
			this = p.parseFunction(nil, false, true, false)
		}
		if this == nil {
			this = p.parseIdVar(false, nil)
		}
		for p.match(tokens.DOT) {
			field := p.parseFunction(nil, false, true, false)
			if field == nil {
				field = p.parseIdVar(false, nil)
			}
			this = p.expression(exp.Dot(exp.Args{"this": this, "expression": field}), nil, nil)
		}
	}

	var ordinality any
	var tableAlias exp.Expression
	if viewValue, _ := view.(bool); viewValue {
		table := p.parseIdVar(false, nil)
		var columns []exp.Expression
		if p.match(tokens.ALIAS) {
			columns = p.parseCsv(func() exp.Expression { return p.parseIdVar(false, nil) })
		}
		tableAlias = p.expression(exp.TableAlias(exp.Args{"this": table, "columns": columns}), nil, nil)
	} else if this != nil && (this.Kind() == exp.KindSubquery || this.Kind() == exp.KindUnnest) && this.Arg("alias") != nil {
		tableAlias = asExpr(this.Arg("alias"))
		this.Set("alias", nil)
	} else {
		if p.matchPair(tokens.WITH, tokens.ORDINALITY, true) {
			ordinality = true
		}
		tableAlias = p.parseTableAlias(nil)
	}

	return p.expression(exp.Lateral(exp.Args{"this": this, "view": view, "outer": outer, "alias": tableAlias, "cross_apply": crossApply, "ordinality": ordinality}), nil, nil)
}

func (p *Parser) parseUnnest(withAlias bool) exp.Expression {
	if !p.matchPair(tokens.UNNEST, tokens.L_PAREN, false) {
		return nil
	}
	p.advance()
	expressions := p.parseWrappedCsv(p.parseEquality)
	var offset any
	if p.matchPair(tokens.WITH, tokens.ORDINALITY, true) {
		offset = true
	}
	var alias exp.Expression
	if withAlias {
		alias = p.parseTableAlias(nil)
	}
	if offset == nil && p.matchPair(tokens.WITH, tokens.OFFSET, true) {
		p.match(tokens.ALIAS)
		offset = p.parseIdVar(false, nil)
		if offset == nil {
			offset = exp.ToIdentifier("offset", false)
		}
	}
	return p.expression(exp.Unnest(exp.Args{"expressions": expressions, "alias": alias, "offset": offset}), nil, nil)
}

func (p *Parser) parseDerivedTableValues() exp.Expression {
	isDerived := p.matchPair(tokens.L_PAREN, tokens.VALUES, true)
	if !isDerived && !p.matchTextSeq("VALUES") && !p.matchTextSeq("FORMAT", "VALUES") {
		return nil
	}
	expressions := p.parseCsv(func() exp.Expression { return p.parseValue(true) })
	alias := p.parseTableAlias(nil)
	if isDerived {
		p.matchRParen(nil)
	}
	return p.expression(exp.Values(exp.Args{"expressions": expressions, "alias": firstExpression(alias, p.parseTableAlias(nil))}), nil, nil)
}

func (p *Parser) parsePivots() []exp.Expression {
	if p.curr.TokenType != tokens.PIVOT && p.curr.TokenType != tokens.UNPIVOT {
		return nil
	}
	pivots := []exp.Expression{}
	for {
		pivot := p.parsePivot()
		if pivot == nil {
			break
		}
		pivots = append(pivots, pivot)
	}
	if len(pivots) == 0 {
		return nil
	}
	return pivots
}

func (p *Parser) parsePivot() exp.Expression {
	index := p.index
	var includeNulls any
	var unpivot bool
	if p.match(tokens.PIVOT) {
		unpivot = false
	} else if p.match(tokens.UNPIVOT) {
		unpivot = true
		if p.matchTextSeq("INCLUDE", "NULLS") {
			includeNulls = true
		} else if p.matchTextSeq("EXCLUDE", "NULLS") {
			includeNulls = false
		}
	} else {
		return nil
	}
	if !p.match(tokens.L_PAREN) {
		p.retreat(index)
		return nil
	}
	var expressions []exp.Expression
	if unpivot {
		expressions = p.parseCsv(p.parseColumn)
	} else {
		expressions = p.parseCsv(p.parsePivotAggregation)
	}
	if len(expressions) == 0 {
		p.raiseError("Failed to parse PIVOT's aggregation list")
	}
	if !p.match(tokens.FOR) {
		p.raiseError("Expecting FOR")
	}
	fields := []exp.Expression{}
	for {
		field := p.tryParse(p.parsePivotIn, false)
		if field == nil {
			break
		}
		fields = append(fields, field)
	}
	var defaultOnNull exp.Expression
	if p.matchTextSeq("DEFAULT", "ON", "NULL") {
		defaultOnNull = p.parseWrapped(p.parseBitwise, false)
	}
	group := p.parseGroup(false)
	p.matchRParen(nil)
	pivot := p.expression(exp.Pivot(exp.Args{"expressions": expressions, "fields": fields, "unpivot": unpivot, "include_nulls": includeNulls, "default_on_null": defaultOnNull, "group": group}), nil, nil)
	if unpivot {
		for i, expression := range expressions {
			expressions[i] = unpivotTarget(expression)
		}
		pivot.Set("expressions", expressions)
		for _, field := range fields {
			if field != nil && field.Kind() == exp.KindIn {
				field.Set("this", unpivotTarget(asExpr(field.Arg("this"))))
			}
		}
	}
	// TODO(slice 2): PIVOT column-name inference needs generator SQL output.
	if !p.matchSet(map[tokens.TokenType]bool{tokens.PIVOT: true, tokens.UNPIVOT: true}, false) {
		pivot.Set("alias", p.parseTableAlias(nil))
	}
	return pivot
}

func (p *Parser) parsePivotAggregation() exp.Expression {
	f := p.parseFunction(nil, false, true, false)
	if f == nil {
		if p.prev.TokenType == tokens.COMMA {
			return nil
		}
		p.raiseError("Expecting an aggregation function in PIVOT")
	}
	return p.parseAlias(f, false)
}

func (p *Parser) parsePivotIn() exp.Expression {
	value := p.parseColumn()
	if !p.match(tokens.IN) {
		p.raiseError("Expecting IN")
	}
	if p.match(tokens.L_PAREN) {
		var exprs []exp.Expression
		if p.match(tokens.ANY) {
			exprs = []exp.Expression{p.expression(exp.PivotAny(exp.Args{"this": p.parseOrder(nil, false)}), nil, nil)}
		} else {
			exprs = p.parseCsv(p.parseAliasedPivotExpression)
		}
		p.matchRParen(nil)
		return p.expression(exp.In(exp.Args{"this": value, "expressions": exprs}), nil, nil)
	}
	return p.expression(exp.In(exp.Args{"this": value, "field": p.parseIdVar(false, nil)}), nil, nil)
}

func (p *Parser) parseAliasedPivotExpression() exp.Expression {
	this := p.parseSelectOrExpression(false)
	p.match(tokens.ALIAS)
	alias := p.parseBitwise()
	if alias != nil {
		if alias.Kind() == exp.KindColumn && alias.Arg("db") == nil {
			alias = alias.This()
		}
		return p.expression(exp.PivotAlias(exp.Args{"this": this, "alias": alias}), nil, nil)
	}
	return this
}

func unpivotTarget(expression exp.Expression) exp.Expression {
	if expression == nil {
		return nil
	}
	if expression.Kind() == exp.KindColumn && expression.Arg("table") == nil {
		return expression.This()
	}
	if expression.Kind() == exp.KindTuple {
		expressions := expression.Expressions()
		for i, child := range expressions {
			expressions[i] = unpivotTarget(child)
		}
		expression.Set("expressions", expressions)
	}
	return expression
}
