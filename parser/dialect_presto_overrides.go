package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
	"regexp"
)

// timeZoneRE ports TIME_ZONE_RE (parser.py:41 `re.compile(r":.*?[a-zA-Z\+\-]")`): a naive
// check for a time-zone suffix on a timestamp literal - a colon followed later by a letter
// or a sign, e.g. `... 05:00 Europe/Prague` or `... +05:00`. Presto's
// ZONE_AWARE_TIMESTAMP_CONSTRUCTOR path (parseType in parser.go) uses it to promote
// `TIMESTAMP '<zoned literal>'` to TIMESTAMPTZ.
var timeZoneRE = regexp.MustCompile(`:.*?[a-zA-Z+\-]`)

// Presto builds FUNCTION_PARSERS as the base table minus TRIM (parsers/presto.py:137:
// `FUNCTION_PARSERS = {k: v for k, v in parser.Parser.FUNCTION_PARSERS.items() if k != "TRIM"}`).
// Disabling TRIM here makes `TRIM(x)` fall through to the Anonymous-function path
// (functionParser resolution, dialect_parser_overrides.go:58-90) instead of building an
// exp.Trim node, matching upstream Presto.
func init() {
	registerDialectParserOverrides("presto", dialectParserOverrideSet{
		NoParenFunctions:        map[tokens.TokenType]func(exp.Args) exp.Expression{tokens.LOCALTIME: exp.Localtime, tokens.LOCALTIMESTAMP: exp.Localtimestamp},
		DisabledFunctionParsers: map[string]bool{"TRIM": true},
		// parsers/presto.py:69-72 TABLE_ALIAS_TOKENS |= {ANTI, SEMI}.
		AliasTokens: map[tokens.TokenType]bool{tokens.ANTI: true, tokens.SEMI: true},
		// parser.py:1399 STORED is a base PROPERTY_PARSERS entry; the shared Go table keeps it
		// fail-closed for mysql/postgres, so Presto registers it here.
		PropertyParsers: map[string]propertyParserFunc{
			"STORED": func(p *Parser, _ bool) exp.Expression { return p.parseStored() },
		},
	})
}
