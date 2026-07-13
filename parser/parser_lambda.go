package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// lambdaArgTerminators ports LAMBDA_ARG_TERMINATORS (parser.py:1006): the fast-path guard
// in parseLambda that recognizes a plain (non-lambda) argument terminated by "," or ")".
var lambdaArgTerminators = map[tokens.TokenType]bool{
	tokens.COMMA:   true,
	tokens.R_PAREN: true,
}

// lambdas ports the base LAMBDAS table (parser.py:988-1001), restricted to the ARROW ->
// exp.Lambda entry. Upstream's other entry, FARROW -> exp.Kwarg (named arguments like
// `foo(bar => 1)`), isn't ported here: exp.Kwarg has no Kind in this codebase yet, and no
// gap in this part's scope needs it.
//
// Declared empty and populated from func init() (rather than a map literal referencing the
// closure directly): the closure calls parseDisjunction, which transitively calls back
// through parseLambda -> lambdas, and a literal initializer for that cycle is rejected by
// Go's package init-order analysis - the same reason dispatch (generator/dispatch.go) and
// statementParsers (parser.go) use this two-step pattern.
var lambdas = map[tokens.TokenType]func(*Parser, []exp.Expression) exp.Expression{}

func init() {
	lambdas[tokens.ARROW] = func(p *Parser, params []exp.Expression) exp.Expression {
		body := replaceLambda(p.parseDisjunction(), params)
		return p.expression(exp.Lambda(exp.Args{"this": body, "expressions": params}), nil, nil)
	}
}

// parseLambdaArg ports _parse_lambda_arg (parser.py:7178-7179): a lambda parameter is just
// an id-var (Snowflake/Materialize override this to also accept `x INT`, out of scope here).
func (p *Parser) parseLambdaArg() exp.Expression {
	return p.parseIdVar(true, nil)
}

// replaceLambda ports _replace_lambda (parser.py:9436-9464), simplified for this part's
// scope (base/mysql/postgres): TYPED_LAMBDA_ARGS is always false for those dialects, so a
// lambda param never carries a "to" type annotation and the upstream Cast branch (wrapping
// a matched column in exp.Cast) never fires - it's dropped here accordingly.
//
// For every exp.Column in the lambda body whose leading part (parts[0], i.e. the outermost
// qualifier - catalog/db/table/this in that priority) names a parameter, the column is
// replaced by its dot form: bare identifier when the column has no table qualifier, or the
// fully flattened dot chain (walking up through any wrapping exp.Dot ancestors) otherwise.
// This mirrors Column.to_dot()/column.this and Column.parts (expressions/builders.go:290,
// 323 columnPartsToDot; core.go:990 Parts) but needs its own walk here because to_dot's
// default include_dots=True also folds in outer exp.Dot ancestors that columnPartsToDot
// (include_dots=False) does not.
func replaceLambda(node exp.Expression, params []exp.Expression) exp.Expression {
	if node == nil {
		return nil
	}

	names := make(map[string]bool, len(params))
	for _, param := range params {
		if param != nil {
			names[param.Name()] = true
		}
	}

	for _, column := range node.FindAll(exp.KindColumn) {
		parts := column.Parts()
		if len(parts) == 0 || !names[parts[0].Name()] {
			continue
		}

		var dotOrID exp.Expression
		if column.Arg("table") != nil {
			dotOrID = columnToDot(column, parts)
		} else {
			dotOrID = column.This()
		}

		parent := column.Parent()
		if parent != nil && parent.Kind() == exp.KindDot {
			for {
				grandparent := parent.Parent()
				if grandparent == nil || grandparent.Kind() != exp.KindDot {
					parent.Replace(dotOrID)
					break
				}
				parent = grandparent
			}
		} else if column == node {
			node = dotOrID
		} else {
			column.Replace(dotOrID)
		}
	}

	return node
}

// columnToDot ports Column.to_dot(include_dots=True) (expressions/core.py:1724-1734): the
// column's own parts (catalog/db/table/this, whichever are present) followed by every
// expression hanging off an ancestor chain of exp.Dot nodes, flattened into a single dot
// chain via exp.DotBuild - or the lone identifier when there's nothing to flatten.
func columnToDot(column exp.Expression, parts []exp.Expression) exp.Expression {
	for parent := column.Parent(); parent != nil && parent.Kind() == exp.KindDot; parent = parent.Parent() {
		if expr := parent.Expr(); expr != nil {
			parts = append(parts, expr)
		}
	}
	if len(parts) == 1 {
		return parts[0].Copy()
	}
	copies := make([]exp.Expression, len(parts))
	for i, part := range parts {
		copies[i] = part.Copy()
	}
	return exp.DotBuild(copies)
}
