package dialects

import (
	"strings"

	"github.com/ridi/sqlglot-go/tokens"
)

// Athena is an outer routing dialect rather than a Trino subtype. Its tokenizer
// classifies the complete input once, then re-tokenizes the original SQL with a
// fresh Hive or Athena-extended Trino tokenizer.
func Athena() *Dialect {
	d := Base()
	d.Name = "athena"
	d.TokenizerFactory = newAthenaTokenizer
	return d
}

// newAthenaTokenizer ports Athena.Tokenizer from
// .reference/sqlglot-v30.12.0/sqlglot/dialects/athena.py:51-82,114-119.
func newAthenaTokenizer() *tokens.Tokenizer {
	classifier := tokens.NewTokenizerWithConfig(athenaClassifierConfig())
	hiveTokenizer := Hive().NewTokenizer()

	trino := Trino()
	trinoConfig := trino.TokenizerConfig
	trinoConfig.Keywords["UNLOAD"] = tokens.COMMAND
	trinoTokenizer := tokens.NewTokenizerWithConfig(tokens.CompileConfig(trinoConfig))

	return tokens.NewTokenizerWithFunc(func(sql string) ([]tokens.Token, error) {
		classified, err := classifier.Tokenize(sql)
		if err != nil {
			return nil, err
		}

		if tokenizeAthenaAsHive(classified) {
			hiveTokens, err := hiveTokenizer.Tokenize(sql)
			if err != nil {
				return nil, err
			}
			return append([]tokens.Token{tokens.NewToken(tokens.HIVE_TOKEN_STREAM, "")}, hiveTokens...), nil
		}

		return trinoTokenizer.Tokenize(sql)
	})
}

// athenaClassifierConfig combines only the tokenizer class attributes overridden
// by Athena upstream. In particular, Hive's double-quote QUOTES entry is not
// imported: double quotes remain identifiers during classification and become
// strings only if the complete SQL is subsequently routed through Hive.
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
// .reference/sqlglot-v30.12.0/sqlglot/dialects/athena.py:89-111. The predicate
// intentionally examines one token stream for the whole input and searches only
// tokens after the first two for SELECT.
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
