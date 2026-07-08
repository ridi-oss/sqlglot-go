package parser

import (
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.ANALYZE] = (*Parser).parseAnalyze
}

// parseAnalyze ports the top level of parser.py:8975-9038 (_parse_analyze).
// The ANALYZE_EXPRESSION_PARSERS sub-family (AnalyzeStatistics/Histogram/Delete/
// Validate/Columns/List: parser.py:1731-1740) is out of scope for this slice, so
// any input that would have taken one of those branches — or that carries other
// trailing tokens we don't structurally parse — degrades to a raw Command via
// parseAsCommand, mirroring upstream's own graceful-degradation idiom (used e.g.
// by _parse_set) and round-tripping identically for the corpus's identity cases.
func (p *Parser) parseAnalyze() exp.Expression {
	start := p.prev
	index := p.index

	// https://duckdb.org/docs/sql/statements/analyze
	if !p.curr.IsValid() {
		return p.expression(exp.Analyze(exp.Args{}), nil, nil)
	}

	var options []string
	for p.matchTexts(analyzeStyles) {
		if stringsUpper(p.prev.Text) == "BUFFER_USAGE_LIMIT" {
			text := "BUFFER_USAGE_LIMIT"
			// NUMERIC_PARSERS' NUMBER case (parser.py:8531-8534,1160-1162) is the only
			// shape this slice's corpus exercises for the option's argument; the shared
			// numeric-literal parser hasn't been ported yet, so match NUMBER directly.
			if p.match(tokens.NUMBER) {
				text += " " + p.prev.Text
			}
			options = append(options, text)
		} else {
			options = append(options, stringsUpper(p.prev.Text))
		}
	}

	var this exp.Expression
	var innerExpression exp.Expression // ANALYZE_EXPRESSION_PARSERS not ported; always nil.

	var kind string
	if p.curr.IsValid() {
		kind = stringsUpper(p.curr.Text)
	}

	switch {
	case p.match(tokens.TABLE) || p.match(tokens.INDEX):
		this = p.parseTableParts(false, false, false, false)
	case p.matchTextSeq("TABLES"):
		if p.match(tokens.FROM) || p.match(tokens.IN) {
			kind = kind + " " + stringsUpper(p.prev.Text)
			this = p.parseTable(true, false, nil, false, true, false, false)
		}
	case p.matchTextSeq("DATABASE"):
		this = p.parseTable(true, false, nil, false, true, false, false)
	case p.matchTextSeq("CLUSTER"):
		this = p.parseTable(false, false, nil, false, false, false, false)
	default:
		// Empty kind: https://prestodb.io/docs/current/sql/analyze.html
		kind = ""
		this = p.parseTableParts(false, false, false, false)
	}

	partition := p.tryParse(p.parsePartition, false)
	if partition == nil && p.matchTexts(partitionKeywords) {
		return p.parseAsCommand(start)
	}

	// https://docs.starrocks.io/docs/sql-reference/sql-statements/cbo_stats/ANALYZE_TABLE/
	var mode string
	if p.matchTextSeq("WITH", "SYNC", "MODE") || p.matchTextSeq("WITH", "ASYNC", "MODE") {
		mode = "WITH " + stringsUpper(p.tokens[p.index-2].Text) + " MODE"
	}

	properties := p.parseProperties()

	// Not upstream: upstream instead re-checks ANALYZE_EXPRESSION_PARSERS here
	// (parser.py:9024-9025), which we don't port. Any leftover token means we hit a
	// construct outside this slice's structured grammar, so degrade to a Command.
	if p.curr.IsValid() {
		p.retreat(index)
		return p.parseAsCommand(start)
	}

	return p.expression(exp.Analyze(exp.Args{
		"kind":       kind,
		"this":       this,
		"mode":       mode,
		"partition":  partition,
		"properties": properties,
		"expression": innerExpression,
		"options":    options,
	}), nil, nil)
}
