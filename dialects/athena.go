package dialects

import (
	"maps"
	"strings"

	"github.com/ridi-oss/sqlglot-go/tokens"
)

// Athena routes each statement through Hive or Athena-extended Trino.
func Athena() *Dialect {
	d := Base()
	d.Name = "athena"
	// divergence: Athena folds quoted names too, matching Trino (presto.py:35).
	d.NormalizationStrategy = CaseInsensitive
	d.TokenizerFactory = newAthenaTokenizer
	return d
}

// newAthenaTokenizer ports Athena.Tokenizer from
// .reference/sqlglot-v30.17.0/sqlglot/dialects/athena.py:51-82,114-119.
func newAthenaTokenizer() *tokens.Tokenizer {
	classifier := tokens.NewTokenizerWithConfig(athenaClassifierConfig())
	// Athena-only grammar extensions (ledger athena-show-*, athena-explain, athena-unload) need
	// real tokens where upstream packs the tail into one raw STRING.
	hiveConfig := Hive().TokenizerConfig
	hiveConfig.Commands = maps.Clone(hiveConfig.Commands)
	delete(hiveConfig.Commands, tokens.SHOW)
	hiveTokenizer := tokens.NewTokenizerWithConfig(tokens.CompileConfig(hiveConfig))

	trinoConfig := Trino().TokenizerConfig
	trinoConfig.Keywords = maps.Clone(trinoConfig.Keywords)
	trinoConfig.Keywords["UNLOAD"] = tokens.UNLOAD
	trinoConfig.Keywords["EXPLAIN"] = tokens.DESCRIBE
	trinoTokenizer := tokens.NewTokenizerWithConfig(tokens.CompileConfig(trinoConfig))

	return tokens.NewTokenizerWithFunc(func(sql string) ([]tokens.Token, error) {
		classified, err := classifier.Tokenize(sql)
		if err != nil {
			return nil, err
		}

		// divergence: athena.py:89-111 routes whole batches; Athena batches can mix DDL and DML.
		runes := []rune(sql)
		var result []tokens.Token
		start := 0
		position, line, col := 0, 1, 0
		for end := 0; end < len(classified); end++ {
			if classified[end].TokenType != tokens.SEMICOLON && end != len(classified)-1 {
				continue
			}
			if classified[end].TokenType == tokens.SEMICOLON && !athenaBoundaryAgrees(hiveTokenizer, runes, classified, start, end) {
				// The classifier reads "…" as an identifier but Hive reads it as a string with
				// backslash escapes, so a `;` inside such a string is not a statement boundary.
				continue
			}
			chunk := classified[start : end+1]
			base, limit := chunk[0].Start, chunk[len(chunk)-1].End+1
			if start == 0 {
				base = 0
			}
			if end == len(classified)-1 {
				limit = len(runes)
			}
			tokenizer := trinoTokenizer
			hive := tokenizeAthenaAsHive(chunk)
			if hive {
				tokenizer = hiveTokenizer
			}
			routed, err := tokenizer.Tokenize(string(runes[base:limit]))
			if err != nil {
				return nil, err
			}
			for position < base {
				r := runes[position]
				if r == '\n' || r == '\r' && (position+1 == len(runes) || runes[position+1] != '\n') {
					line++
					col = 0
				} else {
					col++
				}
				position++
			}
			for i := range routed {
				token := &routed[i]
				if token.Line == 1 {
					token.Col += col
				}
				token.Line += line - 1
				token.Start += base
				token.End += base
				if token.WrapEnd > 0 {
					token.WrapStart += base
					token.WrapEnd += base
				}
			}
			if len(routed) > 0 {
				// Comments before the chunk's first token lie outside the re-tokenized slice.
				if start > 0 {
					routed[0].Comments = append([]string{}, chunk[0].Comments...)
				}
				// A same-line comment after the `;` belongs to it (tokenizer_core.py:862) but lies
				// past the slice; re-read it with the routed engine so `/*+` keeps its payload.
				if end < len(classified)-1 && chunk[len(chunk)-1].TokenType == tokens.SEMICOLON && len(chunk[len(chunk)-1].Comments) > 0 {
					trailing := string(runes[classified[end].End+1 : classified[end+1].Start])
					if commentTokens, err := tokenizer.Tokenize(";" + trailing); err == nil && len(commentTokens) == 1 {
						routed[len(routed)-1].Comments = append([]string{}, commentTokens[0].Comments...)
					}
				}
			}
			if hive {
				result = append(result, tokens.NewToken(tokens.HIVE_TOKEN_STREAM, ""))
			}
			result = append(result, routed...)
			start = end + 1
		}
		return result, nil
	})
}

// athenaBoundaryAgrees reports whether Hive also tokenizes the classifier's candidate `;` at
// classified[end] as a statement-final SEMICOLON for the chunk starting at classified[start].
func athenaBoundaryAgrees(hive *tokens.Tokenizer, runes []rune, classified []tokens.Token, start, end int) bool {
	base := classified[start].Start
	if start == 0 {
		base = 0
	}
	hiveTokens, err := hive.Tokenize(string(runes[base : classified[end].End+1]))
	if err != nil || len(hiveTokens) == 0 {
		return false
	}
	last := hiveTokens[len(hiveTokens)-1]
	return last.TokenType == tokens.SEMICOLON && last.Start+base == classified[end].Start
}

// athenaClassifierConfig combines only the tokenizer class attributes overridden
// by Athena upstream. In particular, Hive's double-quote QUOTES entry is not
// imported: double quotes remain identifiers during classification and become
// strings only in statements routed through Hive.
func athenaClassifierConfig() tokens.TokenizerConfig {
	cfg := tokens.BaseConfig()
	trinoConfig := Trino().TokenizerConfig
	hiveConfig := Hive().TokenizerConfig

	for identifier, end := range trinoConfig.Identifiers {
		cfg.Identifiers[identifier] = end
	}
	for identifier, end := range hiveConfig.Identifiers {
		cfg.Identifiers[identifier] = end
	}

	for escape := range trinoConfig.StringEscapes {
		cfg.StringEscapes[escape] = true
	}
	for escape := range hiveConfig.StringEscapes {
		cfg.StringEscapes[escape] = true
	}

	for start, format := range trinoConfig.FormatStrings {
		if format.TokenType == tokens.HEX_STRING || format.TokenType == tokens.UNICODE_STRING {
			cfg.FormatStrings[start] = format
		}
	}
	for start, format := range hiveConfig.FormatStrings {
		if format.TokenType == tokens.HEX_STRING || format.TokenType == tokens.UNICODE_STRING {
			cfg.FormatStrings[start] = format
		}
	}
	cfg.HasHexStrings = trinoConfig.HasHexStrings || hiveConfig.HasHexStrings

	for literal, dataType := range trinoConfig.NumericLiterals {
		cfg.NumericLiterals[literal] = dataType
	}
	for literal, dataType := range hiveConfig.NumericLiterals {
		cfg.NumericLiterals[literal] = dataType
	}

	for keyword, tokenType := range hiveConfig.Keywords {
		cfg.Keywords[keyword] = tokenType
	}
	for keyword, tokenType := range trinoConfig.Keywords {
		cfg.Keywords[keyword] = tokenType
	}
	cfg.Keywords["UNLOAD"] = tokens.COMMAND

	return tokens.CompileConfig(cfg)
}

// tokenizeAthenaAsHive ports _tokenize_as_hive verbatim from
// .reference/sqlglot-v30.17.0/sqlglot/dialects/athena.py:89-111. The predicate
// searches only tokens after the first two for SELECT.
func tokenizeAthenaAsHive(tokenStream []tokens.Token) bool {
	if len(tokenStream) < 2 {
		return false
	}

	first := tokenStream[0]
	second := tokenStream[1]
	rest := tokenStream[2:]

	firstType := first.TokenType
	firstText := strings.ToUpper(first.Text)
	secondType := second.TokenType
	secondText := strings.ToUpper(second.Text)

	if firstType == tokens.DESCRIBE || firstType == tokens.SHOW || firstText == "MSCK REPAIR" {
		return true
	}

	if firstType == tokens.ALTER || firstType == tokens.CREATE || firstType == tokens.DROP {
		if secondText == "DATABASE" || secondText == "EXTERNAL" || secondText == "SCHEMA" {
			return true
		}
		if secondType == tokens.VIEW {
			return false
		}

		for _, token := range rest {
			if token.TokenType == tokens.SELECT {
				return false
			}
		}
		return true
	}

	return false
}
