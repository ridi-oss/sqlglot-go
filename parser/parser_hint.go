package parser

import (
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// mysqlOperationModifiers ports parsers/mysql.py:290-299 (MySQLParser.OPERATION_MODIFIERS):
// `SELECT [ALL|DISTINCT|DISTINCTROW] [<OPERATION_MODIFIERS>] ...`. Base Parser.OPERATION_MODIFIERS
// (parser.py:1747) is an empty set and Postgres doesn't override it, so this port keeps the set
// MySQL-only and gates it by dialect name in parseSelect (this port has no per-dialect Parser
// subclass registry yet, see dialects.Dialect.Name usage elsewhere in this package).
var mysqlOperationModifiers = map[string]bool{
	"HIGH_PRIORITY":       true,
	"STRAIGHT_JOIN":       true,
	"SQL_SMALL_RESULT":    true,
	"SQL_BIG_RESULT":      true,
	"SQL_BUFFER_RESULT":   true,
	"SQL_NO_CACHE":        true,
	"SQL_CALC_FOUND_ROWS": true,
}

// parseHint ports _parse_hint (parser.py:4253-4257). The tokenizer synthesizes a HINT token
// right after SELECT/INSERT/UPDATE/DELETE when it scans a `/*+ ... */` comment there (see
// tokens.TokensPrecedingHint, tokenizer.go:423), and attaches the comment's raw body text as
// that token's leading comment. Re-parse that text as an exp.Hint through the shared
// ParseInto entry point (exp.ParseIntoFunc, wired to sqlglot.ParseInto by sqlglot.go's init,
// mirroring exp.maybe_parse(..., into=exp.Hint, dialect=self.dialect)).
func (p *Parser) parseHint() exp.Expression {
	if p.match(tokens.HINT) && len(p.prevComments) > 0 {
		expr, err := exp.ParseIntoFunc(p.prevComments[0], p.dialect.Name, exp.KindHint, false)
		if err != nil {
			panic(err)
		}
		return expr
	}
	return nil
}

// parseHintFunctionCall ports _parse_hint_function_call (parser.py:4228-4229).
func (p *Parser) parseHintFunctionCall() exp.Expression {
	return p.parseFunctionCall(nil, false, true, false)
}

// parseHintBody ports _parse_hint_body (parser.py:4231-4251): a list of hint items, either
// function calls (`INDEX(t, i)`) or bare uppercased vars (`REBALANCE`), where items can be
// separated by a comma *or* just whitespace (`BKA(t1) NO_BKA(t2)` has two items with no
// comma between them). parseCsv only splits on COMMA internally, so upstream repeatedly
// calls `_parse_csv` (via `for hint in iter(lambda: self._parse_csv(...), [])`) to also pick
// up each subsequent whitespace-separated run; it stops once a call yields nothing new. If
// any item fails to parse (a ParseError, only raised because ParseInto runs with error level
// IMMEDIATE - see parser.go's ParseInto/NewWithErrorLevel) or leftover tokens remain, the
// whole body falls back to a single raw-text hint via parseHintFallbackToString.
func (p *Parser) parseHintBody() exp.Expression {
	startIndex := p.index
	shouldFallbackToString := false

	var hints []exp.Expression
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(*sqlerrors.ParseError); ok {
					shouldFallbackToString = true
					return
				}
				panic(r)
			}
		}()
		parseHintItem := func() exp.Expression {
			if call := p.parseHintFunctionCall(); call != nil {
				return call
			}
			return p.parseVar(false, nil, true)
		}
		for {
			items := p.parseCsv(parseHintItem)
			if len(items) == 0 {
				break
			}
			hints = append(hints, items...)
		}
	}()

	if shouldFallbackToString || p.curr.IsValid() {
		p.retreat(startIndex)
		return p.parseHintFallbackToString()
	}

	return p.Expression(exp.Hint(exp.Args{"expressions": hints}))
}

// parseHintFallbackToString ports _parse_hint_fallback_to_string (parser.py:4220-4226):
// consumes every remaining token and wraps the raw hint text as a single-element Hint. Upstream
// builds this Hint directly (no self.expression() wrapper), so comments/validation are
// intentionally skipped here too.
func (p *Parser) parseHintFallbackToString() exp.Expression {
	start := p.curr
	for p.curr.IsValid() {
		p.advance()
	}
	end := p.tokens[p.index-1]
	return exp.Hint(exp.Args{"expressions": []any{p.findSQL(start, end)}})
}
