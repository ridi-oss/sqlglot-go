package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.COMMENT] = (*Parser).parseComment
	statementParsers[tokens.TRUNCATE] = (*Parser).parseTruncateTable
}

// commentTableAliasTokens ports upstream COMMENT_TABLE_ALIAS_TOKENS (parser.py:792):
// TABLE_ALIAS_TOKENS minus IS, so `COMMENT ON TABLE t IS '...'` doesn't misparse the
// trailing `IS '...'` as a no-AS table alias.
var commentTableAliasTokens = buildCommentTableAliasTokens()

func buildCommentTableAliasTokens() map[tokens.TokenType]bool {
	m := make(map[tokens.TokenType]bool, len(tableAliasTokens))
	for tt := range tableAliasTokens {
		m[tt] = true
	}
	delete(m, tokens.IS)
	return m
}

// parseComment ports _parse_comment (parser.py:2192-2222): `COMMENT ON <kind> <target>
// IS <string>`. It degrades to a Command via parseAsCommand only when ON isn't followed
// by a recognized creatable or when the comment body cannot be parsed structurally.
func (p *Parser) parseComment() exp.Expression {
	start := p.prev
	exists := p.parseExists(false)

	p.match(tokens.ON)

	materialized := p.matchTextSeq("MATERIALIZED")
	if !p.matchSet(creatables) {
		return p.parseAsCommand(start)
	}
	kind := p.prev

	var this exp.Expression
	switch kind.TokenType {
	case tokens.FUNCTION, tokens.PROCEDURE:
		this = p.parseUserDefinedFunction()
	case tokens.TABLE:
		this = p.parseTable(false, false, commentTableAliasTokens, false, false, false, false)
	case tokens.COLUMN:
		this = p.parseColumn()
	default:
		this = p.parseTableParts(true, false, false, false)
	}

	p.match(tokens.IS)

	// The comment body must be a string literal. This port only models plain STRING
	// literals (no National/RawString node yet), so a non-plain body - base N'...'
	// (NATIONAL_STRING) or postgres $$...$$ (HEREDOC_STRING) - makes parseString return
	// nil. Degrade the whole statement to a raw Command rather than hard-erroring on the
	// required `expression` arg (upstream models these via _parse_string's National/
	// RawString parsers, unported here). The Command re-emits the original source, so
	// identity cases (e.g. the base N'...' form) still round-trip.
	expression := p.parseString()
	if expression == nil {
		return p.parseAsCommand(start)
	}

	return p.expression(exp.Comment(exp.Args{
		"this":         this,
		"kind":         kind.Text,
		"expression":   expression,
		"exists":       exists,
		"materialized": materialized,
	}), nil, nil)
}

// parseTruncateTable ports _parse_truncate_table (parser.py:9466-9515). The ON-cluster
// branch (ClickHouse `ON CLUSTER ...`, parser.py:9484) is omitted - no corpus case
// exercises it, and a leftover `ON ...` correctly falls through to the trailing-token
// check below and degrades to Command instead.
func (p *Parser) parseTruncateTable() exp.Expression {
	start := p.prev

	// Not to be confused with the TRUNCATE(number, decimals) function call: retreat past
	// TRUNCATE and parse it as an ordinary function call. Degrading to Command here instead
	// (as the rest of this parser does) would mis-round-trip: commandSQL always inserts a
	// space between the keyword and the rest ("TRUNCATE (3.14159, 2)"), but the source has
	// none ("TRUNCATE(3.14159, 2)").
	if p.match(tokens.L_PAREN) {
		p.retreat(p.index - 2)
		return p.parseFunction(nil, false, true, false)
	}

	// ClickHouse also supports TRUNCATE DATABASE.
	isDatabase := p.match(tokens.DATABASE)

	p.match(tokens.TABLE)

	exists := p.parseExists(false)

	expressions := p.parseCsv(func() exp.Expression {
		return p.parseTable(true, false, nil, false, isDatabase, false, false)
	})

	var identity any
	if p.matchTextSeq("RESTART", "IDENTITY") {
		identity = "RESTART"
	} else if p.matchTextSeq("CONTINUE", "IDENTITY") {
		identity = "CONTINUE"
	}

	var option any
	if p.matchTextSeq("CASCADE") || p.matchTextSeq("RESTRICT") {
		option = p.prev.Text
	}

	partition := p.parsePartition()

	// Fallback case: e.g. postgres `TRUNCATE TABLE ONLY t1, t2*, ...` (the trailing `*`
	// isn't parsed by parseTable/parseTableParts), or any other unconsumed trailer.
	if p.curr.IsValid() {
		return p.parseAsCommand(start)
	}

	return p.expression(exp.TruncateTable(exp.Args{
		"expressions": expressions,
		"is_database": isDatabase,
		"exists":      exists,
		"identity":    identity,
		"option":      option,
		"partition":   partition,
	}), nil, nil)
}
