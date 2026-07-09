package dialects

import "github.com/sjincho/sqlglot-go/tokens"

func Postgres() *Dialect {
	d := Base()
	d.Name = "postgres"
	d.QuoteStart = "'"
	d.QuoteEnd = "'"
	d.IdentifierStart = "\""
	d.IdentifierEnd = "\""
	d.IndexOffset = 1
	d.TypedDivision = true
	d.NullOrdering = "nulls_are_large"
	d.SupportsLimitAll = true
	d.TablesReferenceableAsColumns = true
	// generators/postgres.py:234 RENAME_TABLE_WITH_DB = False.
	d.RenameTableWithDB = false
	// generators/postgres.py:233 SINGLE_STRING_INTERVAL = True.
	d.SingleStringInterval = true
	// generators/postgres.py:240 PARAMETER_TOKEN = "$".
	d.ParameterToken = "$"
	// generators/postgres.py:235 LOCKING_READS_SUPPORTED = True.
	d.LockingReadsSupported = true
	// dialects/postgres.py:20 TABLESAMPLE_SIZE_IS_PERCENT = True.
	d.TablesampleSizeIsPercent = true
	// generators/postgres.py:242 TABLESAMPLE_SIZE_IS_ROWS = False.
	d.TablesampleSizeIsRows = false
	// generators/postgres.py:243 TABLESAMPLE_SEED_KEYWORD = "REPEATABLE".
	d.TablesampleSeedKeyword = "REPEATABLE"
	// generators/postgres.py:251 COPY_HAS_INTO_KEYWORD = False.
	d.CopyHasIntoKeyword = false
	// parsers/postgres.py:191 JSON_ARROWS_REQUIRE_JSON_TYPE = True.
	d.JSONArrowsRequireJSONType = true
	// generators/postgres.py:245 JSON_TYPE_REQUIRED_FOR_EXTRACTION = True.
	d.JSONTypeRequiredForExtraction = true
	// TODO(slice 5b): DEFAULT_FUNCTIONS_COLUMN_NAMES (needs KindExplodingGenerateSeries + FUNCTIONS override).
	// TODO(slice 5b): typing/{mysql,postgres}.py EXPRESSION_METADATA — feeds annotate_types only, off probe's path (ROADMAP 4c).

	cfg := tokens.BaseConfig()
	cfg.FormatStrings["b'"] = tokens.FormatString{End: "'", TokenType: tokens.BIT_STRING}
	cfg.FormatStrings["B'"] = tokens.FormatString{End: "'", TokenType: tokens.BIT_STRING}
	cfg.FormatStrings["x'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["X'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["e'"] = tokens.FormatString{End: "'", TokenType: tokens.BYTE_STRING}
	cfg.FormatStrings["E'"] = tokens.FormatString{End: "'", TokenType: tokens.BYTE_STRING}
	cfg.ByteStringEscapes = map[rune]bool{'\'': true, '\\': true}
	cfg.FormatStrings["$"] = tokens.FormatString{End: "$", TokenType: tokens.HEREDOC_STRING}
	cfg.SingleTokens['$'] = tokens.HEREDOC_STRING
	cfg.VarSingleTokens['$'] = true
	cfg.HeredocTagIsIdentifier = true
	cfg.HeredocStringAlternative = tokens.PARAMETER

	for keyword, tokenType := range map[string]tokens.TokenType{
		"~":             tokens.RLIKE,
		"@@":            tokens.DAT,
		"@?":            tokens.AT_QMARK,
		"@>":            tokens.AT_GT,
		"<@":            tokens.LT_AT,
		"?&":            tokens.QMARK_AMP,
		"?|":            tokens.QMARK_PIPE,
		"#-":            tokens.HASH_DASH,
		"|/":            tokens.PIPE_SLASH,
		"||/":           tokens.DPIPE_SLASH,
		"BEGIN":         tokens.BEGIN,
		"BIGSERIAL":     tokens.BIGSERIAL,
		"CSTRING":       tokens.PSEUDO_TYPE,
		"DECLARE":       tokens.COMMAND,
		"DO":            tokens.COMMAND,
		"EXEC":          tokens.COMMAND,
		"HSTORE":        tokens.HSTORE,
		"INT8":          tokens.BIGINT,
		"MONEY":         tokens.MONEY,
		"NAME":          tokens.NAME,
		"OID":           tokens.OBJECT_IDENTIFIER,
		"ONLY":          tokens.ONLY,
		"POINT":         tokens.POINT,
		"REFRESH":       tokens.COMMAND,
		"REINDEX":       tokens.COMMAND,
		"RESET":         tokens.COMMAND,
		"SERIAL":        tokens.SERIAL,
		"SMALLSERIAL":   tokens.SMALLSERIAL,
		"TEMP":          tokens.TEMPORARY,
		"TYPE":          tokens.TYPE,
		"REGCLASS":      tokens.OBJECT_IDENTIFIER,
		"REGCOLLATION":  tokens.OBJECT_IDENTIFIER,
		"REGCONFIG":     tokens.OBJECT_IDENTIFIER,
		"REGDICTIONARY": tokens.OBJECT_IDENTIFIER,
		"REGNAMESPACE":  tokens.OBJECT_IDENTIFIER,
		"REGOPER":       tokens.OBJECT_IDENTIFIER,
		"REGOPERATOR":   tokens.OBJECT_IDENTIFIER,
		"REGPROC":       tokens.OBJECT_IDENTIFIER,
		"REGPROCEDURE":  tokens.OBJECT_IDENTIFIER,
		"REGROLE":       tokens.OBJECT_IDENTIFIER,
		"REGTYPE":       tokens.OBJECT_IDENTIFIER,
		"FLOAT":         tokens.DOUBLE,
		"XML":           tokens.XML,
		"VARIADIC":      tokens.VARIADIC,
		"INOUT":         tokens.INOUT,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	delete(cfg.Keywords, "/*+")
	delete(cfg.Keywords, "DIV")
	delete(cfg.Comments, "/*+")
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}
