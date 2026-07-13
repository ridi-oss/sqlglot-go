package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// tablesampleBucketOrPercentSet mirrors the (TokenType.PERCENT, TokenType.MOD) set
// checked in _parse_table_sample's `elif` branch (parser.py:5089).
var tablesampleBucketOrPercentSet = map[tokens.TokenType]bool{tokens.PERCENT: true, tokens.MOD: true}

// tablesampleMethodRowTokens mirrors the `tokens=(TokenType.ROW,)` argument to _parse_var
// (parser.py:5069): letting a bare ROW keyword double as the sampling method name.
var tablesampleMethodRowTokens = map[tokens.TokenType]bool{tokens.ROW: true}

// tablesampleSeedTexts mirrors `_match_texts(("SEED", "REPEATABLE"))` (parser.py:5099).
var tablesampleSeedTexts = map[string]bool{"SEED": true, "REPEATABLE": true}

// parseTableSample ports _parse_table_sample (parser.py:5057-5121). base/mysql/postgres all
// have TABLESAMPLE_CSV = False (parser.py:1768, no per-dialect override in any of the three),
// so the `expressions`/CSV branch is never taken here. DEFAULT_SAMPLING_METHOD is nil for all
// three dialects too, so the trailing `if not method and self.DEFAULT_SAMPLING_METHOD` fallback
// is omitted.
func (p *Parser) parseTableSample(asModifier bool) exp.Expression {
	if !p.match(tokens.TABLE_SAMPLE) && !(asModifier && p.matchTextSeq("USING", "SAMPLE")) {
		return nil
	}

	var bucketNumerator, bucketDenominator, bucketField, percent, size, seed exp.Expression

	method := p.parseVar(false, tablesampleMethodRowTokens, true)
	matchedLParen := p.match(tokens.L_PAREN)

	var num exp.Expression
	if p.match(tokens.NUMBER, false) {
		num = p.parseFactor()
	} else {
		num = p.parsePrimary()
		if num == nil {
			num = p.parsePlaceholder()
		}
	}

	if p.matchTextSeq("BUCKET") {
		bucketNumerator = p.parseNumber()
		p.matchTextSeq("OUT", "OF")
		bucketDenominator = p.parseNumber()
		p.match(tokens.ON)
		bucketField = p.parseField(false, nil, false)
	} else if p.matchSet(tablesampleBucketOrPercentSet) {
		percent = num
	} else if p.match(tokens.ROWS) || !p.dialect.TablesampleSizeIsPercent {
		size = num
	} else {
		percent = num
	}

	if matchedLParen {
		p.matchRParen(nil)
	}

	if p.match(tokens.L_PAREN) {
		method = p.parseVar(false, nil, true)
		if p.match(tokens.COMMA) {
			seed = p.parseNumber()
		}
		p.matchRParen(nil)
	} else if p.matchTexts(tablesampleSeedTexts) {
		seed = p.parseWrapped(p.parseNumber, false)
	}

	return p.expression(exp.TableSample(exp.Args{
		"method":             method,
		"bucket_numerator":   bucketNumerator,
		"bucket_denominator": bucketDenominator,
		"bucket_field":       bucketField,
		"percent":            percent,
		"size":               size,
		"seed":               seed,
	}), nil, nil)
}
