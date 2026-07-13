package dialects

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func Postgres() *Dialect {
	d := Base()
	d.Name = "postgres"
	d.QuoteStart = "'"
	d.QuoteEnd = "'"
	d.IdentifierStart = "\""
	d.IdentifierEnd = "\""
	d.IndexOffset = 1
	d.TypedDivision = true
	// dialects/postgres.py:15 CONCAT_COALESCE = True.
	d.ConcatCoalesce = true
	d.NullOrdering = "nulls_are_large"
	d.SupportsLimitAll = true
	d.TablesReferenceableAsColumns = true
	// generator.py:466 SUPPORTS_SELECT_INTO is overridden True for postgres: `SELECT ... INTO x`
	// stays inline instead of being rewritten to `CREATE TABLE x AS SELECT ...`.
	d.SupportsSelectInto = true
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

	// CHAR_LENGTH/CHARACTER_LENGTH close parity_gaps.txt gaps 177-178. Length._sql_names =
	// ["LENGTH", "LEN", "CHAR_LENGTH", "CHARACTER_LENGTH"] (string.py:69-71) is base-scope
	// upstream, but functions.go:100-105 explicitly warns against unifying LENGTH globally
	// (MySQL's LENGTH is byte-length, CHAR_LENGTH is char-length - a real semantic split,
	// deferred to ROADMAP 5b), so only postgres (which has no such split: LENGTH,
	// CHAR_LENGTH and CHARACTER_LENGTH all render identically as LENGTH(...), no generator
	// override needed - see the default className fallback) gets these two spellings here.
	// "LENGTH" itself is left unregistered: it already round-trips correctly as Anonymous.
	d.Functions = map[string]func([]exp.Expression) exp.Expression{
		"CHAR_LENGTH":      exp.FromArgListFunc(exp.KindLength),
		"CHARACTER_LENGTH": exp.FromArgListFunc(exp.KindLength),
	}

	cfg := tokens.BaseConfig()
	// has_bit_strings/has_hex_strings = bool(BIT_STRINGS)/bool(HEX_STRINGS) (tokens.py:581-582);
	// postgres BIT_STRINGS/HEX_STRINGS are non-empty (dialects/postgres.py:65-66), which also
	// enables the number scanner's bare `0b`/`0x` forms (`SELECT 0xFF` -> `SELECT x'FF'`).
	cfg.HasBitStrings = true
	cfg.HasHexStrings = true
	cfg.FormatStrings["b'"] = tokens.FormatString{End: "'", TokenType: tokens.BIT_STRING}
	cfg.FormatStrings["B'"] = tokens.FormatString{End: "'", TokenType: tokens.BIT_STRING}
	cfg.FormatStrings["x'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["X'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["e'"] = tokens.FormatString{End: "'", TokenType: tokens.BYTE_STRING}
	cfg.FormatStrings["E'"] = tokens.FormatString{End: "'", TokenType: tokens.BYTE_STRING}
	cfg.ByteStringEscapes = map[rune]bool{'\'': true, '\\': true}
	// dialects/postgres.py:65-67 BIT_STRINGS=[("b'","'"),("B'","'")] / HEX_STRINGS=
	// [("x'","'"),("X'","'")] / BYTE_STRINGS=[("e'","'"),("E'","'")]; *_START take the
	// FIRST tuple of each.
	d.BitStart, d.BitEnd = "b'", "'"
	d.HexStart, d.HexEnd = "x'", "'"
	d.ByteStart, d.ByteEnd = "e'", "'"
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
		"EXPLAIN":       tokens.DESCRIBE, // Ledgered pg-explain: non-COMMAND prevents swallowing remaining SQL as raw text.
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
