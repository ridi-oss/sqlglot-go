package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

var intervalStringRE = regexp.MustCompile(`\s*(-?[0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]+)\s*`)

// intervalUnitNoParenTokens lets a bare interval unit position accept the "current
// date/time" pseudo-functions (e.g. `INTERVAL '-1' CURRENT_DATE`), matching upstream's
// NO_PAREN_FUNCTIONS dispatch inside _parse_function_call (parser.py:6973-6978:
// `token_type in self.NO_PAREN_FUNCTIONS` -> exp.CurrentDate()/CurrentTime()/...). That
// token-type-keyed table isn't ported in general yet (parser.go:1953 TODO(1d); see also
// parser_constraints.go's "ON UPDATE CURRENT_TIMESTAMP" comment for another call site
// blocked on the same gap) - a full port needs new Kinds (CurrentDate/CurrentTime/...)
// plus the shared parseFunctionCall change, both out of this file's scope. This is a
// narrow, interval-scoped accommodation: it renders the unit as a bare Var (e.g.
// Var("CURRENT_DATE")) rather than upstream's dedicated CurrentDate() node.
//
// Only tokens.CURRENT_DATE is listed: it's the only NO_PAREN_FUNCTIONS token that (a) this
// port's tokenizer actually produces (CURRENT_ROLE/CURRENT_DATETIME have no tokenizer
// keyword entry in tokens/tokenizer.go, so those TokenTypes are unreachable) and (b)
// upstream renders bare, with no trailing "()" (CurrentDate has no ported
// currentdate_sql/no_paren_current_date_sql override needed - it's the class default).
// CURRENT_TIME/CURRENT_TIMESTAMP/CURRENT_USER *are* reachable but upstream renders those
// with "()" (e.g. "INTERVAL '1' CURRENT_TIMESTAMP()" - confirmed against the pinned
// oracle), which the bare-Var rendering here can't reproduce faithfully; omitted rather
// than emit a subtly wrong round-trip (no corpus case exercises them as an interval unit).
var intervalUnitNoParenTokens = map[tokens.TokenType]bool{
	tokens.CURRENT_DATE: true,
}

// pyNumberString renders a Go number the way Python's str() would, so INTERVAL
// literal canonicalization matches upstream (e.g. 1.0 -> "1.0", not "1").
func pyNumberString(v any) string {
	switch n := v.(type) {
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		s := strconv.FormatFloat(n, 'g', -1, 64)
		// Python renders integer-valued floats with a trailing ".0".
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	default:
		return fmt.Sprint(v)
	}
}

func (p *Parser) parseIntervalSpan(this exp.Expression) exp.Expression {
	unit := p.parseFunction(nil, false, true, false)
	if unit == nil && (p.curr.TokenType == tokens.VAR || p.dialect.ValidIntervalUnits[stringsUpper(p.curr.Text)] || intervalUnitNoParenTokens[p.curr.TokenType]) {
		unit = p.parseVar(true, nil, true)
	}

	// Most dialects support, e.g., the form INTERVAL '5' day, thus we try to parse
	// each INTERVAL expression into this canonical form so it's easy to transpile.
	if this != nil && this.IsNumber() {
		// Mirror upstream `exp.Literal.string(this.to_py())`: Python's str() of the
		// number, which keeps decimals (1.0 -> "1.0") and the sign for Neg literals.
		// fmt.Sprint on a float64 would drop the trailing ".0", so format explicitly.
		this = exp.LiteralString(pyNumberString(this.ToPy()))
	} else if this != nil && this.IsString() {
		parts := intervalStringRE.FindAllStringSubmatch(this.Name(), -1)
		if len(parts) > 0 && unit != nil {
			unit = nil
			p.retreat(p.index - 1)
		}
		if len(parts) == 1 {
			this = exp.LiteralString(parts[0][1])
			unit = p.expression(exp.Var(exp.Args{"this": stringsUpper(parts[0][2])}), nil, nil)
		}
	}

	if p.dialect.IntervalSpans && p.matchTextSeq("TO") {
		expression := p.parseFunction(nil, false, true, false)
		if expression == nil {
			expression = p.parseVar(true, nil, true)
		}
		unit = p.expression(exp.IntervalSpan(exp.Args{"this": unit, "expression": expression}), nil, nil)
	}

	return p.expression(exp.Interval(exp.Args{"this": this, "unit": unit}), nil, nil)
}

func (p *Parser) parseInterval(requireInterval bool) exp.Expression {
	index := p.index
	if !p.match(tokens.INTERVAL) && requireInterval {
		return nil
	}

	var this exp.Expression
	if p.match(tokens.STRING, false) {
		this = p.parsePrimary()
	} else {
		this = p.parseTerm()
	}

	if this == nil || isBareIntervalColumnWithInvalidUnit(p, this) {
		p.retreat(index)
		return nil
	}

	interval := p.parseIntervalSpan(this)

	index = p.index
	p.match(tokens.PLUS)
	if p.matchSet(map[tokens.TokenType]bool{tokens.STRING: true, tokens.NUMBER: true}, false) {
		return p.expression(exp.Add(exp.Args{"this": interval, "expression": p.parseInterval(false)}), nil, nil)
	}

	p.retreat(index)
	return interval
}

func isBareIntervalColumnWithInvalidUnit(p *Parser, this exp.Expression) bool {
	if this.Kind() != exp.KindColumn || this.Arg("table") != nil || !p.curr.IsValid() {
		return false
	}
	identifier := this.This()
	quoted := false
	if identifier != nil {
		quoted, _ = identifier.Arg("quoted").(bool)
	}
	return !quoted && !p.dialect.ValidIntervalUnits[stringsUpper(p.curr.Text)]
}
