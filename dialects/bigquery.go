package dialects

import (
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// BigQuery ports dialects/bigquery.py, parsers/bigquery.py and generators/bigquery.py.
func BigQuery() *Dialect {
	d := Base()
	d.Name = "bigquery"
	d.IdentifierStart, d.IdentifierEnd = "`", "`"
	// HEX_START/BYTE_START take the first HEX_STRINGS/BYTE_STRINGS tuple (dialects/dialect.py:293).
	d.HexStart, d.HexEnd = "0x", ""
	d.ByteStart, d.ByteEnd = "b'", "'"
	// dialects/bigquery.py:23-50 class attributes.
	d.UnnestColumnOnly = true
	d.SupportsUserDefinedTypes = false
	d.ForceEarlyAliasRefExpansion = true
	d.ExpandOnlyGroupAliasRef = true
	d.HexStringIsIntegerType = true
	d.ByteStringIsBytesType = true
	d.AnnotateAllScopes = true
	d.ProjectionAliasesShadowSourceNames = true
	d.TablesReferenceableAsColumns = true
	d.SupportsStructStarExpansion = true
	d.ExcludesPseudocolumnsFromStar = true
	d.QueryResultsAreStructs = true
	d.SetOpDistinctRequired = true
	d.AliasPostVersion = false
	d.NormalizationStrategy = CaseInsensitive
	d.NormalizeFunctions = ""
	d.Pseudocolumns = map[string]bool{
		"_PARTITIONTIME":     true,
		"_PARTITIONDATE":     true,
		"_TABLE_SUFFIX":      true,
		"_FILE_NAME":         true,
		"_DBT_MAX_PARTITION": true,
	}
	// parsers/bigquery.py:190-194.
	d.LogDefaultsToLn = true
	d.JoinsHaveEqualPrecedence = true
	// generators/bigquery.py:252.
	d.IntervalAllowsPluralForm = false
	d.RenameTableWithDB = false
	d.Functions = bigqueryFunctions()

	cfg := tokens.BaseConfig()
	// dialects/bigquery.py:176-189 Tokenizer delimiters.
	cfg.Quotes = map[string]string{"'": "'", `"`: `"`, `"""`: `"""`, "'''": "'''"}
	cfg.Comments["#"] = ""
	cfg.Identifiers = map[rune]string{'`': "`"}
	cfg.StringEscapes = map[rune]bool{'\\': true}
	cfg.HasHexStrings = true
	for _, q := range []string{"'", `"`, `"""`, "'''"} {
		for _, prefix := range []string{"b", "B"} {
			cfg.FormatStrings[prefix+q] = tokens.FormatString{End: q, TokenType: tokens.BYTE_STRING}
		}
		for _, prefix := range []string{"r", "R"} {
			cfg.FormatStrings[prefix+q] = tokens.FormatString{End: q, TokenType: tokens.RAW_STRING}
		}
	}
	cfg.NestedComments = false
	// dialects/bigquery.py:191-215 keyword overlay.
	for keyword, tokenType := range map[string]tokens.TokenType{
		"ANY TYPE":          tokens.VARIANT,
		"BEGIN":             tokens.COMMAND,
		"BEGIN TRANSACTION": tokens.BEGIN,
		"BYTEINT":           tokens.INT,
		"BYTES":             tokens.BINARY,
		"CURRENT_DATETIME":  tokens.CURRENT_DATETIME,
		"DATETIME":          tokens.TIMESTAMP,
		"DECLARE":           tokens.DECLARE,
		"ELSEIF":            tokens.COMMAND,
		"EXCEPTION":         tokens.COMMAND,
		"EXPORT":            tokens.EXPORT,
		"FLOAT64":           tokens.DOUBLE,
		"LOOP":              tokens.COMMAND,
		"MODEL":             tokens.MODEL,
		"RECORD":            tokens.STRUCT,
		"REPEAT":            tokens.COMMAND,
		"TIMESTAMP":         tokens.TIMESTAMPTZ,
		"WHILE":             tokens.COMMAND,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	delete(cfg.Keywords, "DIV")
	delete(cfg.Keywords, "VALUES")
	delete(cfg.Keywords, "/*+")
	delete(cfg.Comments, "/*+")
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}
