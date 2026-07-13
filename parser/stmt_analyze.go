package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.ANALYZE] = (*Parser).parseAnalyze
}

// analyzeExpressionParsers is the worklist-scoped subset of
// ANALYZE_EXPRESSION_PARSERS (parser.py:1731-1740). DROP and UPDATE both parse
// MySQL's HISTOGRAM form; the remaining analyze sub-families stay deferred.
var analyzeExpressionParsers = map[string]func(*Parser) exp.Expression{
	"DROP":   (*Parser).parseAnalyzeHistogram,
	"UPDATE": (*Parser).parseAnalyzeHistogram,
}

var analyzeExpressionParserKeys = funcMapKeys(analyzeExpressionParsers)

// parseAnalyze ports the top level of parser.py:8975-9038 (_parse_analyze).
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
	var innerExpression exp.Expression

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
	// Try matching inner expression keywords before falling back to parsing a table.
	case p.matchTexts(analyzeExpressionParserKeys):
		kind = ""
		innerExpression = analyzeExpressionParsers[stringsUpper(p.prev.Text)](p)
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

	if p.matchTexts(analyzeExpressionParserKeys) {
		innerExpression = analyzeExpressionParsers[stringsUpper(p.prev.Text)](p)
	}

	properties := p.parseProperties()

	// The other ANALYZE_EXPRESSION_PARSERS entries remain out of scope. Preserve the
	// existing fail-closed Command fallback for any unconsumed analyze grammar.
	if p.curr.IsValid() {
		p.retreat(index)
		return p.parseAsCommand(start)
	}

	args := exp.Args{
		"this":       this,
		"partition":  partition,
		"properties": properties,
		"expression": innerExpression,
	}
	if kind != "" {
		args["kind"] = kind
	}
	if mode != "" {
		args["mode"] = mode
	}
	if len(options) > 0 {
		args["options"] = options
	}
	return p.expression(exp.Analyze(args), nil, nil)
}

// parseAnalyzeHistogram ports _parse_analyze_histogram (parser.py:9113-9150).
func (p *Parser) parseAnalyzeHistogram() exp.Expression {
	this := stringsUpper(p.prev.Text)
	var expression exp.Expression
	var expressions []exp.Expression
	var updateOptions string

	if p.matchTextSeq("HISTOGRAM", "ON") {
		expressions = p.parseCsv(p.parseColumnReference)
		var withExpressions []string
		for p.match(tokens.WITH) {
			// https://docs.starrocks.io/docs/sql-reference/sql-statements/cbo_stats/ANALYZE_TABLE/
			if p.matchTexts(map[string]bool{"SYNC": true, "ASYNC": true}) {
				mode := stringsUpper(p.prev.Text)
				if p.matchTextSeq("MODE") {
					withExpressions = append(withExpressions, mode+" MODE")
				}
			} else {
				buckets := p.parseNumber()
				if p.matchTextSeq("BUCKETS") && buckets != nil {
					withExpressions = append(withExpressions, buckets.Text("this")+" BUCKETS")
				}
			}
		}
		if len(withExpressions) > 0 {
			expression = p.expression(exp.AnalyzeWith(exp.Args{"expressions": withExpressions}), nil, nil)
		}

		if p.matchTexts(map[string]bool{"MANUAL": true, "AUTO": true}) {
			if p.match(tokens.UPDATE, false) {
				updateOptions = stringsUpper(p.prev.Text)
				p.advance()
			}
		} else if p.matchTextSeq("USING", "DATA") {
			expression = p.expression(exp.UsingData(exp.Args{"this": p.parseString()}), nil, nil)
		}
	}

	args := exp.Args{
		"this":        this,
		"expressions": expressions,
		"expression":  expression,
	}
	if updateOptions != "" {
		args["update_options"] = updateOptions
	}
	return p.expression(exp.AnalyzeHistogram(args), nil, nil)
}
