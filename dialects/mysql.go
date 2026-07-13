package dialects

import (
	"fmt"
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// parseMySQLVersion parses the mysql_version setting into MySQL's MYSQL_VERSION_ID —
// the comparable integer major*10000 + minor*100 + patch (e.g. 80035 for MySQL 8.0.35).
// This is exactly the /*!NNNNN version-comment gate form and what the C API
// mysql_get_server_version() returns, so a client passes the integer it already has.
// The value must be a plain decimal integer; a dotted version string is not accepted.
func parseMySQLVersion(value string) (int, error) {
	value = strings.TrimSpace(value)
	invalid := func(reason string) (int, error) {
		return 0, fmt.Errorf("invalid mysql_version %q: %s", value, reason)
	}
	if value == "" {
		return invalid("expected a MYSQL_VERSION_ID integer")
	}

	maxInt := int(^uint(0) >> 1)
	id := 0
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return invalid("expected a MYSQL_VERSION_ID integer, e.g. 80035 for MySQL 8.0.35")
		}
		digit := int(value[i] - '0')
		if id > (maxInt-digit)/10 {
			return invalid("MYSQL_VERSION_ID overflows int")
		}
		id = id*10 + digit
	}
	return id, nil
}

func MySQL() *Dialect {
	d := Base()
	d.Name = "mysql"
	d.QuoteStart = "'"
	d.QuoteEnd = "'"
	d.IdentifierStart = "`"
	d.IdentifierEnd = "`"
	// mysql.py:25 NORMALIZATION_STRATEGY = CASE_SENSITIVE (upstream default, kept for
	// faithfulness — no folding). For correct+safe identifier folding (columns are
	// case-insensitive on every MySQL platform), a consumer opts into a MySQL strategy via the
	// settings string, e.g.
	//   GetOrRaise("mysql, normalization_strategy=mysql_case_sensitive_table_names")   (lctn=0)
	//   GetOrRaise("mysql, normalization_strategy=mysql_case_insensitive")             (lctn=1/2)
	// Those strategies fold with MySQL's exact .tolower map (exported as MySQLLower).
	d.NormalizationStrategy = CaseSensitive
	d.DPipeIsStringConcat = false
	d.SupportsUserDefinedTypes = false
	d.SafeDivision = true
	// parsers/mysql.py:68 FUNC_TOKENS adds TokenType.VALUES (see ValuesIsFunction doc).
	d.ValuesIsFunction = true
	// parsers/mysql.py:69 FUNC_TOKENS adds TokenType.CHARACTER_SET (see CharsetIsFunction doc).
	d.CharsetIsFunction = true
	// generators/mysql.py:137 DUPLICATE_KEY_UPDATE_WITH_SET = False.
	d.DuplicateKeyUpdateWithSet = false
	// generators/mysql.py:148 WRAP_DERIVED_VALUES = False: aliased VALUES is emitted bare.
	d.WrapDerivedValues = false
	// generators/mysql.py:139 VALUES_AS_TABLE = False: a table-source VALUES is rewritten
	// into `(SELECT ... UNION ALL ...) AS t` instead of a bare VALUES constructor.
	d.ValuesAsTable = false
	// generators/mysql.py:127-128 SUPPORTS_MODIFY_COLUMN/SUPPORTS_CHANGE_COLUMN = True.
	d.SupportsModifyColumn = true
	d.SupportsChangeColumn = true
	// generators/mysql.py:149 VARCHAR_REQUIRES_SIZE = True.
	d.VarcharRequiresSize = true
	// generators/mysql.py:132 INTERVAL_ALLOWS_PLURAL_FORM = False.
	d.IntervalAllowsPluralForm = false
	// generators/mysql.py:133 LOCKING_READS_SUPPORTED = True.
	d.LockingReadsSupported = true
	// generators/mysql.py:142 JSON_TYPE_REQUIRED_FOR_EXTRACTION = True.
	d.JSONTypeRequiredForExtraction = true
	// parsers/mysql.py:303 VALUES_FOLLOWED_BY_PAREN = False.
	d.ValuesFollowedByParen = false
	// parsers/mysql.py:304 SUPPORTS_PARTITION_SELECTION = True: allow `FROM t PARTITION(p0)`.
	d.SupportsPartitionSelection = true
	// generators/mysql.py:339 RESERVED_KEYWORDS: quote reserved words used as identifiers
	// (e.g. `SELECT 1 AS row` -> SELECT 1 AS `row`). See dialects/mysql_reserved.go.
	d.ReservedKeywords = mysqlReservedKeywords

	// parsers/mysql.py:106-166 FUNCTIONS overlay. The DayOfMonth/DayOfWeek/DayOfYear/
	// WeekOfYear and LCASE/UCASE spellings are base-scope upstream (auto-registered via each
	// class's _sql_names), so they now live in the shared exp.FunctionByName singleton (base
	// and postgres canonicalize them too); MySQL's own spelling is applied by the generator
	// overrides (generator/dialect_funcs.go). What remains here is genuinely mysql-only:
	// CURDATE/CURTIME (:110-112) and DATABASE/SCHEMA -> CurrentSchema (:134-135) - verified
	// against the pinned oracle that base does NOT register those. INSTR -> StrPosition
	// (parser.py:405) and TIME_STR_TO_UNIX (temporal.py:472) are also base-scope upstream but
	// kept mysql-local for now (not needed to close any base/postgres parity gap; deferred to
	// avoid changing base STR_POSITION/TIME_STR_TO_UNIX rendering that no fixture exercises).
	d.Functions = map[string]func([]exp.Expression) exp.Expression{
		"CURDATE":          exp.FromArgListFunc(exp.KindCurrentDate),
		"CURTIME":          exp.FromArgListFunc(exp.KindCurrentTime),
		"DATABASE":         exp.FromArgListFunc(exp.KindCurrentSchema),
		"SCHEMA":           exp.FromArgListFunc(exp.KindCurrentSchema),
		"INSTR":            exp.FromArgListFunc(exp.KindStrPosition),
		"TIME_STR_TO_UNIX": exp.FromArgListFunc(exp.KindTimeStrToUnix),
	}

	for _, unit := range []string{
		"SECOND_MICROSECOND",
		"MINUTE_MICROSECOND",
		"MINUTE_SECOND",
		"HOUR_MICROSECOND",
		"HOUR_SECOND",
		"HOUR_MINUTE",
		"DAY_MICROSECOND",
		"DAY_SECOND",
		"DAY_MINUTE",
		"DAY_HOUR",
		"YEAR_MONTH",
	} {
		d.ValidIntervalUnits[unit] = true
	}

	cfg := tokens.BaseConfig()
	cfg.Quotes["\""] = "\""
	cfg.Identifiers = map[rune]string{'`': "`"}
	cfg.Comments["#"] = ""
	cfg.MySQLExecutableComments = true
	// MySQL requires `--` to be followed by whitespace/control (or EOF) to start a line
	// comment; otherwise it is two `-` operators (`1--2` == `1 - -2`). Upstream sqlglot
	// mis-tokenizes this — see DEVIATIONS §1.4. `#` needs no such guard (`#comment` is a
	// full-line comment in MySQL).
	cfg.LineCommentRequiresSpace = map[string]bool{"--": true}
	cfg.StringEscapes['"'] = true
	cfg.StringEscapes['\\'] = true
	for _, r := range []rune{'0', 'b', 'n', 'r', 't', 'Z', '%', '_'} {
		cfg.EscapeFollowChars[r] = true
	}
	cfg.HasBitStrings = true
	cfg.HasHexStrings = true
	cfg.FormatStrings["b'"] = tokens.FormatString{End: "'", TokenType: tokens.BIT_STRING}
	cfg.FormatStrings["B'"] = tokens.FormatString{End: "'", TokenType: tokens.BIT_STRING}
	cfg.FormatStrings["x'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["X'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	// dialects/mysql.py:71-72 BIT_STRINGS=[("b'","'"),("B'","'"),("0b","")] / HEX_STRINGS=
	// [("x'","'"),("X'","'"),("0x","")]; BIT_START/HEX_START take the FIRST tuple.
	d.BitStart, d.BitEnd = "b'", "'"
	d.HexStart, d.HexEnd = "x'", "'"
	cfg.NestedComments = false
	cfg.IdentifiersCanStartWithDigit = true

	for keyword, tokenType := range map[string]tokens.TokenType{
		"BLOB":             tokens.BLOB,
		"CHARSET":          tokens.CHARACTER_SET,
		"DISTINCTROW":      tokens.DISTINCT,
		"EXPLAIN":          tokens.DESCRIBE,
		"FORCE":            tokens.FORCE,
		"IGNORE":           tokens.IGNORE,
		"KEY":              tokens.KEY,
		"LOCK TABLES":      tokens.COMMAND,
		"LONGBLOB":         tokens.LONGBLOB,
		"LONGTEXT":         tokens.LONGTEXT,
		"MEDIUMBLOB":       tokens.MEDIUMBLOB,
		"MEDIUMINT":        tokens.MEDIUMINT,
		"MEDIUMTEXT":       tokens.MEDIUMTEXT,
		"MEMBER OF":        tokens.MEMBER_OF,
		"MOD":              tokens.MOD,
		"SEPARATOR":        tokens.SEPARATOR,
		"SERIAL":           tokens.SERIAL,
		"SIGNED":           tokens.BIGINT,
		"SIGNED INTEGER":   tokens.BIGINT,
		"SOUNDS LIKE":      tokens.SOUNDS_LIKE,
		"START":            tokens.BEGIN,
		"TIMESTAMP":        tokens.TIMESTAMPTZ,
		"TINYBLOB":         tokens.TINYBLOB,
		"TINYTEXT":         tokens.TINYTEXT,
		"UNLOCK TABLES":    tokens.COMMAND,
		"UNSIGNED":         tokens.UBIGINT,
		"UNSIGNED INTEGER": tokens.UBIGINT,
		"YEAR":             tokens.YEAR,
		"_ARMSCII8":        tokens.INTRODUCER,
		"_ASCII":           tokens.INTRODUCER,
		"_BIG5":            tokens.INTRODUCER,
		"_BINARY":          tokens.INTRODUCER,
		"_CP1250":          tokens.INTRODUCER,
		"_CP1251":          tokens.INTRODUCER,
		"_CP1256":          tokens.INTRODUCER,
		"_CP1257":          tokens.INTRODUCER,
		"_CP850":           tokens.INTRODUCER,
		"_CP852":           tokens.INTRODUCER,
		"_CP866":           tokens.INTRODUCER,
		"_CP932":           tokens.INTRODUCER,
		"_DEC8":            tokens.INTRODUCER,
		"_EUCJPMS":         tokens.INTRODUCER,
		"_EUCKR":           tokens.INTRODUCER,
		"_GB18030":         tokens.INTRODUCER,
		"_GB2312":          tokens.INTRODUCER,
		"_GBK":             tokens.INTRODUCER,
		"_GEOSTD8":         tokens.INTRODUCER,
		"_GREEK":           tokens.INTRODUCER,
		"_HEBREW":          tokens.INTRODUCER,
		"_HP8":             tokens.INTRODUCER,
		"_KEYBCS2":         tokens.INTRODUCER,
		"_KOI8R":           tokens.INTRODUCER,
		"_KOI8U":           tokens.INTRODUCER,
		"_LATIN1":          tokens.INTRODUCER,
		"_LATIN2":          tokens.INTRODUCER,
		"_LATIN5":          tokens.INTRODUCER,
		"_LATIN7":          tokens.INTRODUCER,
		"_MACCE":           tokens.INTRODUCER,
		"_MACROMAN":        tokens.INTRODUCER,
		"_SJIS":            tokens.INTRODUCER,
		"_SWE7":            tokens.INTRODUCER,
		"_TIS620":          tokens.INTRODUCER,
		"_UCS2":            tokens.INTRODUCER,
		"_UJIS":            tokens.INTRODUCER,
		"_UTF8":            tokens.INTRODUCER,
		"_UTF16":           tokens.INTRODUCER,
		"_UTF16LE":         tokens.INTRODUCER,
		"_UTF32":           tokens.INTRODUCER,
		"_UTF8MB3":         tokens.INTRODUCER,
		"_UTF8MB4":         tokens.INTRODUCER,
		"@@":               tokens.SESSION_PARAMETER,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	// The pinned .reference/sqlglot-v30.12.0/sqlglot/dialects/mysql.py:154 adds
	// REPLACE as a Command, but the mysql-replace extension preserves its suffix
	// tokens for statement parsing.
	delete(cfg.Commands, tokens.REPLACE)
	delete(cfg.Commands, tokens.SHOW)
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}
