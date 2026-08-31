package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// The MySQL FUNCTION_PARSERS["GROUP_CONCAT"] and STATEMENT_PARSERS[REPLACE] entries are
// registered on the shared "mysql" dialectParserOverrideSet in parser_ddl.go (alongside the
// MySQL PropertyParsers), since registerDialectParserOverrides allows only one registration per
// dialect.

// parseInsertSetValues ports the INSERT ... SET normalization (parser.py:3616-3647,
// v30.17.0): each `col = value` assignment feeds the ordinary Schema + one-row Values
// shape, so the generator needs no SET-specific path. A DEFAULT value becomes exp.Var
// when the dialect supports it (SUPPORTS_VALUES_DEFAULT).
func (p *Parser) parseInsertSetValues(this exp.Expression) (exp.Expression, exp.Expression) {
	var columns []exp.Expression
	var values []exp.Expression
	p.parseCsv(func() exp.Expression {
		target := p.parseColumn()
		if target != nil && target.Kind() == exp.KindColumn && p.match(tokens.EQ) {
			var value exp.Expression
			if p.dialect.SupportsValuesDefault && p.match(tokens.DEFAULT) {
				value = p.expression(exp.Var(exp.Args{"this": stringsUpper(p.prev.Text)}), nil, nil)
			} else {
				value = p.parseDisjunction()
			}
			if value != nil {
				columns = append(columns, target.This())
				values = append(values, value)
				return value
			}
		}
		p.raiseError("Expected column assignment in INSERT ... SET")
		return nil
	})
	schema := p.expression(exp.Schema(exp.Args{"this": this, "expressions": columns}), nil, nil)
	setValues := p.expression(exp.Values(exp.Args{
		"expressions": []exp.Expression{p.expression(exp.Tuple(exp.Args{"expressions": values}), nil, nil)},
		"alias":       p.parseTableAlias(nil),
	}), nil, nil)
	return schema, setValues
}

// parseMySQLReplace is the MySQL statement override for the mysql-replace extension. Pinned
// dialects/mysql.py:154 leaves REPLACE statements as Commands; this port structures only the
// supported INSERT-equivalent grammar and fails closed for every other statement form.
func (p *Parser) parseMySQLReplace() exp.Expression {
	start := p.prev
	if p.curr.TokenType == tokens.L_PAREN {
		// A leading REPLACE(...) is the base function, not a statement. Retreat over the token
		// consumed by parseStatement, matching stmt_transaction.go's expression fallback pattern.
		p.retreat(p.index - 1)
		return p.parseExpressionStatement()
	}

	if insert := p.tryParse(p.parseMySQLReplaceInsert, false); insert != nil {
		return insert
	}
	return p.parseAsCommand(start)
}

func (p *Parser) parseMySQLReplaceInsert() exp.Expression {
	// parseInsert consumes LOCAL unconditionally because it belongs to Hive's DIRECTORY grammar,
	// but REPLACE LOCAL is not part of this extension. Reject it before that information is lost
	// so the caller can preserve the complete statement as a Command.
	if p.curr.TokenType != tokens.STRING && stringsUpper(p.curr.Text) == "LOCAL" {
		p.raiseError("Unsupported modifier in MySQL REPLACE statement")
		return nil
	}

	insert := p.parseInsert()
	if insert == nil || insert.Kind() != exp.KindInsert || p.curr.IsValid() {
		p.raiseError("Unsupported MySQL REPLACE statement")
		return nil
	}

	target := insert.This()
	if target != nil && target.Kind() == exp.KindSchema {
		target = target.This()
	}
	if target == nil || target.Kind() != exp.KindTable {
		p.raiseError("MySQL REPLACE requires a table target")
		return nil
	}

	source := insert.Expr()
	if source == nil || (source.Kind() != exp.KindValues && !source.Is(exp.TraitQuery)) {
		p.raiseError("MySQL REPLACE requires VALUES, SET, or a query source")
		return nil
	}

	boolArg := func(key string) bool {
		value, _ := insert.Arg(key).(bool)
		return value
	}
	if insert.Arg("hint") != nil || boolArg("overwrite") || boolArg("ignore") ||
		insert.Arg("alternative") != nil || boolArg("is_function") ||
		insert.Arg("returning") != nil || boolArg("by_name") || boolArg("exists") ||
		insert.Arg("where") != nil || boolArg("default") || insert.Arg("conflict") != nil {
		p.raiseError("Unsupported modifier in MySQL REPLACE statement")
		return nil
	}

	return insert
}

// parseGroupConcat ports _parse_group_concat (parser.py:10074), the MySQL
// FUNCTION_PARSERS["GROUP_CONCAT"] entry (parsers/mysql.py:156):
//
//	GROUP_CONCAT([DISTINCT] expr [, expr ...] [ORDER BY ...] [SEPARATOR str])
//
// Multiple value expressions collapse into a CONCAT (or, under DISTINCT, the DISTINCT's
// expressions do); an ORDER BY parsed by parseLambda becomes an exp.Order wrapping the
// (possibly concatenated) value. The MySQL generator renders
// GROUP_CONCAT(<this> SEPARATOR <sep or ','>) — see generator/aggregate.go.
func (p *Parser) parseGroupConcat() exp.Expression {
	// concatExprs mirrors the upstream closure: a DISTINCT with >1 expressions has those
	// wrapped in a single CONCAT; otherwise a single arg passes through and multiple args
	// collapse into a CONCAT. safe/coalesce follow the dialect (parser.py:10075-10092).
	concatExprs := func(node exp.Expression, exprs []exp.Expression) exp.Expression {
		if node != nil && node.Kind() == exp.KindDistinct {
			if distinctExprs, _ := node.Arg("expressions").([]exp.Expression); len(distinctExprs) > 1 {
				concat := p.expression(exp.Concat(exp.Args{
					"expressions": distinctExprs,
					"safe":        true,
					"coalesce":    p.dialect.ConcatCoalesce,
				}), nil, nil)
				node.Set("expressions", []exp.Expression{concat})
				return node
			}
		}
		if len(exprs) == 1 {
			return exprs[0]
		}
		return p.expression(exp.Concat(exp.Args{
			"expressions": exprs,
			"safe":        true,
			"coalesce":    p.dialect.ConcatCoalesce,
		}), nil, nil)
	}

	args := p.parseCsv(func() exp.Expression { return p.parseLambda(false) })

	var this exp.Expression
	if len(args) > 0 {
		var order exp.Expression
		if last := args[len(args)-1]; last != nil && last.Kind() == exp.KindOrder {
			order = last
		}
		if order != nil {
			// ORDER BY is the last (or only) expression and has consumed the 'expr' before
			// it; remove 'expr' from the Order and add it back to args, then re-attach the
			// concatenated value as the Order's target.
			orderThis := order.This()
			args[len(args)-1] = orderThis
			order.Set("this", concatExprs(orderThis, args))
			this = order
		} else {
			this = concatExprs(args[0], args)
		}
	}

	var separator exp.Expression
	if p.match(tokens.SEPARATOR) {
		separator = p.parseField(false, nil, false)
	}

	return p.expression(exp.GroupConcat(exp.Args{"this": this, "separator": separator}), nil, nil)
}
