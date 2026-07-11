package parser

import (
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

// The MySQL FUNCTION_PARSERS["GROUP_CONCAT"] entry is registered on the shared "mysql"
// dialectParserOverrideSet in parser_ddl.go (alongside the mysql PropertyParsers), since
// registerDialectParserOverrides allows only one registration per dialect.

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
