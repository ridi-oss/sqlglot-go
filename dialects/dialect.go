package dialects

import (
	"fmt"
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

type NormalizationStrategy int

const (
	Lowercase NormalizationStrategy = iota
	Uppercase
	CaseSensitive
	CaseInsensitive
	CaseInsensitiveUppercase
	// The two MySQL strategies below are NON-UPSTREAM (DEVIATIONS.md §1.2). They fold with
	// MySQL's exact identifier lowercase (MySQLLower — my_unicase_default .tolower, Unicode
	// simple + accent-preserving: É->é but Ñ!=N), NOT the ASCII fold used by the other
	// strategies. That fold is exported (MySQLLower) so a consumer can call the same
	// implementation via a native binding, guaranteeing a byte-identical normalized identifier.

	// MySQLCaseInsensitive folds EVERY identifier (columns AND table/db names), regardless of
	// quoting, with MySQLLower. Models MySQL with lower_case_table_names=1/2 (all names
	// case-insensitive).
	MySQLCaseInsensitive
	// MySQLCaseSensitiveTableNames is role-aware: table/database name identifiers are
	// case-sensitive (never folded); every other identifier (columns, aliases, CTE names, ...)
	// is folded with MySQLLower, regardless of quoting. Models MySQL's default on a
	// case-sensitive filesystem (lower_case_table_names=0): columns case-insensitive on every
	// platform, database/table names case-sensitive on Linux.
	MySQLCaseSensitiveTableNames
)

type Dialect struct {
	Name            string
	QuoteStart      string
	QuoteEnd        string
	IdentifierStart string
	IdentifierEnd   string
	// TokenizerFactory supports Athena's classify-and-re-tokenize router while preserving
	// Dialect.NewTokenizer's concrete *tokens.Tokenizer return type. A nil factory uses the
	// ordinary compiled TokenizerConfig path.
	TokenizerFactory func() *tokens.Tokenizer
	TokenizerConfig  tokens.TokenizerConfig
	// MySQLVersion is NON-UPSTREAM tokenizer state (DEVIATIONS.md §1.5): the parsed
	// MySQL five-digit comparable value. Nil is the default-off sentinel.
	MySQLVersion             *int
	NormalizationStrategy    NormalizationStrategy
	DPipeIsStringConcat      bool
	StrictStringConcat       bool
	TypedDivision            bool
	SafeDivision             bool
	SupportsColumnJoinMarks  bool
	ColonIsVariantExtract    bool
	NullOrdering             string
	SupportsOrderByAll       bool
	TryCastRequiresString    *bool
	DatePartMapping          map[string]string
	ValidIntervalUnits       map[string]bool
	SupportsUserDefinedTypes bool
	SupportsFixedSizeArrays  bool
	SupportsLimitAll         bool
	SupportsValuesDefault    bool
	// SupportsSelectInto ports the generator flag SUPPORTS_SELECT_INTO (generator.py:466):
	// when false (base/mysql), the generator rewrites `SELECT ... INTO x` into
	// `CREATE TABLE x AS SELECT ...`; postgres (and tsql) keep the inline INTO.
	SupportsSelectInto bool
	// ValuesIsFunction ports the MySQL parser's FUNC_TOKENS addition of TokenType.VALUES
	// (parsers/mysql.py:63-70), paired upstream with its FUNCTION_PARSERS["VALUES"] override
	// (parsers/mysql.py:158-160): `VALUES(col)` in `ON DUPLICATE KEY UPDATE` is a function
	// call, not the VALUES table-constructor keyword. Callback overrides now live in the
	// parser-side dialect registry, but FUNC_TOKENS admission remains a separate,
	// not-yet-generalized problem: this flag lets parseFunctionCall admit VALUES before
	// callback dispatch.
	ValuesIsFunction bool
	// CharsetIsFunction likewise ports MySQL's FUNC_TOKENS addition of
	// TokenType.CHARACTER_SET (parsers/mysql.py:63-70): `CHARSET(...)`/`CHARACTER SET(...)`
	// is an ordinary (Anonymous) function call, not the CHARACTER SET/CHARSET keyword.
	// The parser-side callback registry does not control that earlier admission decision,
	// so this flag remains until FUNC_TOKENS itself is generalized per dialect.
	CharsetIsFunction bool
	// DuplicateKeyUpdateWithSet ports the generator flag of the same name
	// (generator.py:374, generators/mysql.py:137): base emits `ON DUPLICATE KEY UPDATE SET
	// ...`, MySQL's own generator omits the SET keyword (`ON DUPLICATE KEY UPDATE ...`).
	DuplicateKeyUpdateWithSet bool
	// WrapDerivedValues ports the generator flag of the same name (generator.py:320,
	// generators/mysql.py:148): base wraps an aliased/derived VALUES table constructor in
	// parentheses (`(VALUES (1, 2)) AS t`), MySQL emits it bare (`VALUES (1, 2) AS t`).
	WrapDerivedValues bool
	// ValuesAsTable ports the generator flag of the same name (generator.py:397,
	// generators/mysql.py:139): base renders a table-source VALUES as a VALUES constructor,
	// MySQL rewrites it into a series of SELECT unions (`(SELECT 1 AS a, ...) AS t`). A VALUES
	// used as an INSERT source (not under FROM/JOIN) is unaffected by this flag.
	ValuesAsTable                      bool
	IntervalSpans                      bool
	NormalizeFunctions                 string
	DefaultFunctionsColumnNames        map[exp.Kind][]string
	AliasPostTablesample               bool
	AliasPostVersion                   bool
	UnnestColumnOnly                   bool
	Pseudocolumns                      map[string]bool
	PreferCTEAliasColumn               bool
	ForceEarlyAliasRefExpansion        bool
	ExpandOnlyGroupAliasRef            bool
	AnnotateAllScopes                  bool
	DisablesAliasRefExpansion          bool
	SupportsAliasRefsInJoinConditions  bool
	ProjectionAliasesShadowSourceNames bool
	TablesReferenceableAsColumns       bool
	SupportsStructStarExpansion        bool
	ExcludesPseudocolumnsFromStar      bool
	QueryResultsAreStructs             bool
	RequiresParenthesizedStructAccess  bool
	IndexOffset                        int
	// RenameTableWithDB ports the generator flag of the same name (generator.py:344): base
	// allows `ALTER TABLE db.a RENAME TO db.b`, some dialects (e.g. postgres, generators/
	// postgres.py:234) only allow renaming within the same db and drop the db qualifier.
	RenameTableWithDB bool
	// SupportsModifyColumn ports the generator flag of the same name (generator.py:475):
	// whether `ALTER TABLE ... MODIFY COLUMN <column-redefinition>` syntax is supported.
	SupportsModifyColumn bool
	// SupportsChangeColumn ports the generator flag of the same name (generator.py:478):
	// whether `ALTER TABLE ... CHANGE COLUMN <rename-and-redefine>` syntax is supported.
	SupportsChangeColumn bool
	// VarcharRequiresSize ports the MySQL-generator-family flag of the same name
	// (generators/mysql.py:149): MySQL requires VARCHAR to carry an explicit size.
	VarcharRequiresSize bool
	// AlterTableSupportsCascade ports the dialect flag of the same name (dialects/dialect.py:
	// 701): whether ALTER TABLE ... CASCADE is supported to propagate a column change to
	// existing partitions (Hive-family only).
	AlterTableSupportsCascade bool
	// AlterTableAddRequiredForEachColumn ports the dialect flag of the same name (dialects/
	// dialect.py:709): whether ADD must be repeated for each column added by ALTER TABLE.
	AlterTableAddRequiredForEachColumn bool
	// AlterRenameRequiresColumn ports the parser flag of the same name (parser.py:1819):
	// whether renaming a column with ALTER requires the presence of the COLUMN keyword.
	AlterRenameRequiresColumn bool
	// AlterTablePartitions ports the parser flag of the same name (parser.py:1822): whether
	// ALTER statements are allowed to contain PARTITION specifications.
	AlterTablePartitions bool
	// AlterTableIncludeColumnKeyword ports the generator flag of the same name (generator.py:
	// 400): whether the word COLUMN is included when adding a column with ALTER TABLE.
	AlterTableIncludeColumnKeyword bool
	// ComputedColumnWithType ports the generator flag of the same name (generator.py:412):
	// whether to include the type of a computed column in the CREATE DDL.
	ComputedColumnWithType bool
	// AlterSetWrapped ports the generator flag of the same name (generator.py:568): whether
	// to wrap AlterSet's properties in parens, e.g. `ALTER ... SET (<props>)`.
	AlterSetWrapped bool
	// AlterSetType ports the generator flag of the same name (generator.py:582): the syntax
	// to use when altering the type of a column via ALTER SET.
	AlterSetType string
	// SupportsDropAlterIcebergProperty ports the generator flag of the same name (generator.py:
	// 619): whether DROP/ALTER can carry the ICEBERG keyword (e.g. Snowflake's `DROP ICEBERG
	// TABLE a.b`; DuckDB overrides to False and just emits `DROP TABLE a.b`).
	SupportsDropAlterIcebergProperty bool
	// SingleStringInterval ports the generator flag of the same name (generator.py:335); postgres
	// overrides to True (generators/postgres.py:233): render INTERVAL as a single quoted
	// "<value> <unit>" string instead of the base's separate `INTERVAL <this> <unit>` tokens.
	SingleStringInterval bool
	// IntervalAllowsPluralForm ports the generator flag of the same name (generator.py:335);
	// mysql overrides to False (generators/mysql.py:132): whether a plural interval unit (e.g.
	// "DAYS") is kept as-is, or singularized via TIME_PART_SINGULARS first.
	IntervalAllowsPluralForm bool
	// ParameterToken ports the generator flag PARAMETER_TOKEN (generator.py:667); postgres
	// overrides to "$" (generators/postgres.py:240): the sigil parameterSQL prefixes a
	// Parameter's name with (base/mysql use "@", postgres uses "$" for its positional $1/$2/...
	// placeholders).
	ParameterToken string
	// LockingReadsSupported ports the generator flag LOCKING_READS_SUPPORTED (generator.py:314);
	// mysql (generators/mysql.py:133) and postgres (generators/postgres.py:235) override to True:
	// whether row-locking read modifiers (FOR UPDATE/SHARE/...) are rendered instead of raising
	// "unsupported".
	LockingReadsSupported bool
	// TablesampleSizeIsPercent ports the dialect flag of the same name (dialects/dialect.py:342);
	// postgres overrides to true (dialects/postgres.py:20): whether a bare numeric size in
	// TABLESAMPLE means percent (rather than rows).
	TablesampleSizeIsPercent bool
	// TablesampleSizeIsRows ports the generator flag TABLESAMPLE_SIZE_IS_ROWS (generator.py:421);
	// postgres overrides to false (generators/postgres.py:242): whether a rendered size needs a
	// trailing ROWS keyword.
	TablesampleSizeIsRows bool
	// TablesampleSeedKeyword ports the generator flag TABLESAMPLE_SEED_KEYWORD (generator.py:430);
	// postgres overrides to "REPEATABLE" (generators/postgres.py:243): the keyword preceding a
	// TABLESAMPLE clause's seed value.
	TablesampleSeedKeyword string
	// TablesampleRequiresParens ports the generator flag TABLESAMPLE_REQUIRES_PARENS
	// (generator.py:418): whether parentheses are required around the table sample's expression.
	TablesampleRequiresParens bool
	// TablesampleWithMethod ports the generator flag TABLESAMPLE_WITH_METHOD (generator.py:427):
	// whether the TABLESAMPLE clause supports a method name, like BERNOULLI.
	TablesampleWithMethod bool
	// TablesampleKeywords ports the generator flag TABLESAMPLE_KEYWORDS (generator.py:424): the
	// keyword(s) to use when generating a sample clause.
	TablesampleKeywords string
	// CopyHasIntoKeyword ports the generator flag COPY_HAS_INTO_KEYWORD (generator.py:526);
	// postgres overrides to False (generators/postgres.py:251): whether copySQL renders
	// `COPY INTO <this>` (base) or `COPY <this>` (postgres).
	CopyHasIntoKeyword bool
	// CopyParamsAreWrapped ports the generator flag COPY_PARAMS_ARE_WRAPPED (generator.py:
	// 520): whether copySQL's WITH-clause params are wrapped in parens (`WITH (...)`,
	// indented when pretty) or emitted bare.
	CopyParamsAreWrapped bool
	// CopyParamsAreCsv ports the dialect flag COPY_PARAMS_ARE_CSV (dialects/dialect.py:363):
	// whether COPY's WITH-clause params are comma-separated (parser: the separator consumed
	// between options; generator: the separator emitted between rendered options).
	CopyParamsAreCsv bool
	// CopyParamsEqRequired ports the generator flag COPY_PARAMS_EQ_REQUIRED (generator.py:
	// 523): whether copyparameterSQL always renders `option = value` (True) or `option value`
	// (False) when a param has a scalar value.
	CopyParamsEqRequired bool
	// JSONArrowsRequireJSONType ports the parser flag JSON_ARROWS_REQUIRE_JSON_TYPE
	// (parsers/postgres.py:191); base/mysql leave it False. When set, a JSON `->`/`->>` RHS
	// that is a literal marks the built JSONExtract/JSONExtractScalar node's only_json_types
	// arg (mirroring build_json_extract_path's arrow_req_json_type branch, dialect.py:
	// 2092-2124), which the generator uses to choose operator-form (`->`/`->>`) over
	// function-form (JSON_EXTRACT_PATH/JSON_EXTRACT_PATH_TEXT) rendering.
	JSONArrowsRequireJSONType bool
	// JSONTypeRequiredForExtraction ports the generator flag JSON_TYPE_REQUIRED_FOR_EXTRACTION
	// (generator.py:488); mysql (generators/mysql.py:142) and postgres (generators/postgres.py:
	// 245) both override to True. Consumed by arrow_json_extract_sql (dialect.py:1210-1215):
	// when set, a string-literal `this` is wrapped in `CAST(... AS JSON)` before rendering the
	// JSON_EXTRACT_SCALAR arrow form.
	JSONTypeRequiredForExtraction bool
	// StrictCast ports parser.py:1755 STRICT_CAST: whether CAST and :: build Cast (true)
	// or the dialect's permissive TryCast form (false).
	StrictCast bool
	// LogDefaultsToLn ports parser.py:1765 LOG_DEFAULTS_TO_LN: whether one-argument LOG(x)
	// canonicalizes to Ln instead of Log.
	LogDefaultsToLn bool
	// JoinsHaveEqualPrecedence ports parser.py:1824-1828: whether keyword and comma joins build
	// the same left-deep tree instead of keyword JOIN binding more tightly.
	JoinsHaveEqualPrecedence bool
	// AddJoinOnTrue ports parser.py:1842-1844: whether a JOIN without criteria receives ON TRUE
	// so its semantics remain explicit when transpiled to dialects that require a condition.
	AddJoinOnTrue bool
	// RegexpExtractDefaultGroup ports dialect.py:679 and build_regexp_extract at dialect.py:
	// 2343-2360: the capture group inserted when REGEXP_EXTRACT omits its group argument.
	RegexpExtractDefaultGroup int
	// RegexpExtractPositionOverflowReturnsNull ports dialect.py:682-683 and
	// build_regexp_extract at dialect.py:2343-2360: the canonical RegexpExtract node records
	// whether an overflowing position returns NULL rather than an empty string.
	RegexpExtractPositionOverflowReturnsNull bool
	// Functions ports the per-dialect FUNCTIONS class attribute (parsers/mysql.py,
	// parsers/postgres.py, etc.): a dialect-specific overlay of function-name -> builder
	// entries, merged OVER exp.FunctionByName at parse time (see parser/parser.go's
	// functions==nil merge site in parseFunctionCall). Unlike upstream - where every
	// dialect's FUNCTIONS is `{**parser.Parser.FUNCTIONS, ...overrides}`, a single
	// pre-merged class-level dict - this port keeps exp.FunctionByName as the one base map
	// and each Dialect only carries its OWN additions/overrides here, merged in on demand.
	// A nil/empty map (the base Dialect's zero value) means "no per-dialect overrides",
	// so p.dialect.Functions is safe to index even when unset.
	Functions map[string]func([]exp.Expression) exp.Expression
	// ValuesFollowedByParen ports the parser flag of the same name (parser.py:1801): whether a
	// bare VALUES keyword not immediately followed by "(" can be reparsed as a plain identifier
	// (e.g. `SELECT values`, `values.c`). MySQL overrides to False (parsers/mysql.py:303),
	// since MySQL treats VALUES specially even without a following paren.
	ValuesFollowedByParen bool
	// ZoneAwareTimestampConstructor ports the parser flag ZONE_AWARE_TIMESTAMP_CONSTRUCTOR
	// (parser.py:1831, default False; parsers/presto.py:61 overrides to True). Consumed at
	// parser.py:6186-6191 when parsing a typed-literal CAST: for a TIMESTAMP-typed literal whose
	// text carries a time-zone offset (TIME_ZONE_RE), the target type is promoted to TIMESTAMPTZ
	// (`TIMESTAMP '2020-01-01 00:00:00+00'` -> CAST(... AS TIMESTAMPTZ)). Presto is the first
	// in-scope override; base/mysql/postgres leave it at the False default.
	ZoneAwareTimestampConstructor bool
	// SupportsPartitionSelection ports the parser flag SUPPORTS_PARTITION_SELECTION
	// (parser.py:1810, default False): whether a table source in FROM may carry a trailing
	// PARTITION(...) selector, e.g. `SELECT * FROM t1 PARTITION(p0)`. MySQL overrides to True
	// (parsers/mysql.py:304); base/postgres leave it False.
	SupportsPartitionSelection bool
	// ReservedKeywords ports the generator class var RESERVED_KEYWORDS (generator.py:790,
	// empty for base/postgres; generators/mysql.py:339 for MySQL): unquoted identifiers whose
	// lowercased name is in this set are quoted on generation (identifier_sql, generator.py:
	// 1983). A nil/empty map (base's zero value) means "quote nothing extra".
	ReservedKeywords map[string]bool
	// BitStart/BitEnd, HexStart/HexEnd, ByteStart/ByteEnd port BIT_START/BIT_END,
	// HEX_START/HEX_END, BYTE_START/BYTE_END (dialects/dialect.py:292-295): the
	// bitstringSQL/hexstringSQL/bytestringSQL delimiters, derived upstream from the FIRST
	// entry of the tokenizer's BIT_STRINGS/HEX_STRINGS/BYTE_STRINGS class attribute (e.g.
	// mysql/postgres BIT_STRINGS=[("b'","'"),...] -> BIT_START="b'", BIT_END="'"). Unlike
	// upstream's `_FORMAT_STRINGS` dict (Python dict insertion order), this port's
	// TokenizerConfig.FormatStrings is a plain Go map with no reliable iteration order, so
	// these are explicit per-dialect fields (set alongside QuoteStart/IdentifierStart above)
	// rather than derived from FormatStrings at Dialect-construction time. Empty string
	// (base's zero value) means "dialect has no such string family", matching upstream's
	// None default.
	BitStart, BitEnd   string
	HexStart, HexEnd   string
	ByteStart, ByteEnd string
	// HexStringIsIntegerType ports HEX_STRING_IS_INTEGER_TYPE (dialects/dialect.py:676) and
	// ByteStringIsBytesType ports BYTE_STRING_IS_BYTES_TYPE (dialects/dialect.py:724): neither
	// base, mysql, nor postgres overrides either flag away from its False default, so both
	// stay at the Go zero value for all three dialects here - present for 1:1 fidelity with
	// hexstringSQL/bytestringSQL's upstream signature, not because any in-scope dialect sets
	// them true.
	HexStringIsIntegerType bool
	ByteStringIsBytesType  bool
	// ConcatCoalesce ports CONCAT_COALESCE (dialects/dialect.py:404, postgres.py:15): whether
	// the dialect's native CONCAT function already coalesces NULL args to empty strings.
	// Consumed by parsePrimary's adjacent-string-literal rewrite (`'a' 'b'` -> Concat) to set
	// the built node's own "coalesce" arg to match; postgres is the only override (true) among
	// base/mysql/postgres.
	ConcatCoalesce bool
}

func Base() *Dialect {
	datePartMapping := baseDatePartMapping()
	return &Dialect{
		Name:                     "base",
		QuoteStart:               "'",
		QuoteEnd:                 "'",
		IdentifierStart:          "\"",
		IdentifierEnd:            "\"",
		TokenizerConfig:          tokens.BaseConfig(),
		NormalizationStrategy:    Lowercase,
		DPipeIsStringConcat:      true,
		NullOrdering:             "nulls_are_small",
		DatePartMapping:          datePartMapping,
		ValidIntervalUnits:       validIntervalUnits(datePartMapping),
		SupportsUserDefinedTypes: true,
		SupportsFixedSizeArrays:  false,
		// dialect.py:664 SUPPORTS_LIMIT_ALL = False (base); postgres (dialects/postgres.py) and
		// presto (dialects/presto.py:25) both override to True.
		SupportsLimitAll: false,
		// dialect.py:670 SUPPORTS_VALUES_DEFAULT = True (base); presto overrides to False
		// (dialects/presto.py:26).
		SupportsValuesDefault: true,
		// generator.py:374 DUPLICATE_KEY_UPDATE_WITH_SET = True (base); MySQL overrides to
		// False (generators/mysql.py:137).
		DuplicateKeyUpdateWithSet: true,
		// generator.py:320 WRAP_DERIVED_VALUES = True (base); MySQL overrides to False
		// (generators/mysql.py:148).
		WrapDerivedValues: true,
		// generator.py:397 VALUES_AS_TABLE = True (base); MySQL overrides to False
		// (generators/mysql.py:139), rewriting table-source VALUES into SELECT unions.
		ValuesAsTable:        true,
		IntervalSpans:        true,
		NormalizeFunctions:   "upper",
		AliasPostTablesample: false,
		AliasPostVersion:     true,
		UnnestColumnOnly:     false,
		Pseudocolumns:        map[string]bool{},
		IndexOffset:          0,
		// generator.py:344 RENAME_TABLE_WITH_DB = True (base); postgres overrides to False
		// (generators/postgres.py:234).
		RenameTableWithDB: true,
		// generator.py:475/478 SUPPORTS_MODIFY_COLUMN/SUPPORTS_CHANGE_COLUMN = False (base);
		// MySQL overrides both to True (generators/mysql.py:127-128).
		SupportsModifyColumn: false,
		SupportsChangeColumn: false,
		// generators/mysql.py:149 VARCHAR_REQUIRES_SIZE = True; not a base Generator
		// attribute at all (only referenced within the MySQL generator family), so base
		// (and postgres, which doesn't override it) is False.
		VarcharRequiresSize: false,
		// dialects/dialect.py:701 ALTER_TABLE_SUPPORTS_CASCADE = False (base); only the
		// Hive-family dialects (out of scope here) override to True.
		AlterTableSupportsCascade: false,
		// dialects/dialect.py:709 ALTER_TABLE_ADD_REQUIRED_FOR_EACH_COLUMN = True (base).
		AlterTableAddRequiredForEachColumn: true,
		// parser.py:1819 ALTER_RENAME_REQUIRES_COLUMN = True (base).
		AlterRenameRequiresColumn: true,
		// parser.py:1822 ALTER_TABLE_PARTITIONS = False (base); only Hive overrides to True.
		AlterTablePartitions: false,
		// generator.py:400 ALTER_TABLE_INCLUDE_COLUMN_KEYWORD = True (base).
		AlterTableIncludeColumnKeyword: true,
		// generator.py:412 COMPUTED_COLUMN_WITH_TYPE = True (base).
		ComputedColumnWithType: true,
		// generator.py:568 ALTER_SET_WRAPPED = False (base).
		AlterSetWrapped: false,
		// generator.py:582 ALTER_SET_TYPE = "SET DATA TYPE" (base).
		AlterSetType: "SET DATA TYPE",
		// generator.py:619 SUPPORTS_DROP_ALTER_ICEBERG_PROPERTY = True (base).
		SupportsDropAlterIcebergProperty: true,
		// generator.py:335 SINGLE_STRING_INTERVAL = False (base); postgres overrides to True
		// (generators/postgres.py:233).
		SingleStringInterval: false,
		// generator.py:335 INTERVAL_ALLOWS_PLURAL_FORM = True (base); mysql overrides to False
		// (generators/mysql.py:132).
		IntervalAllowsPluralForm: true,
		// generator.py:667 PARAMETER_TOKEN = "@" (base); postgres overrides to "$"
		// (generators/postgres.py:240).
		ParameterToken: "@",
		// generator.py:314 LOCKING_READS_SUPPORTED = False (base).
		LockingReadsSupported: false,
		// dialects/dialect.py:342 TABLESAMPLE_SIZE_IS_PERCENT = False (base); postgres
		// overrides to True (dialects/postgres.py:20).
		TablesampleSizeIsPercent: false,
		// generator.py:421 TABLESAMPLE_SIZE_IS_ROWS = True (base); postgres overrides to
		// False (generators/postgres.py:242).
		TablesampleSizeIsRows: true,
		// generator.py:430 TABLESAMPLE_SEED_KEYWORD = "SEED" (base); postgres overrides to
		// "REPEATABLE" (generators/postgres.py:243).
		TablesampleSeedKeyword: "SEED",
		// generator.py:418 TABLESAMPLE_REQUIRES_PARENS = True (base); no dialect override
		// within base/mysql/postgres.
		TablesampleRequiresParens: true,
		// generator.py:427 TABLESAMPLE_WITH_METHOD = True (base); no dialect override
		// within base/mysql/postgres.
		TablesampleWithMethod: true,
		// generator.py:424 TABLESAMPLE_KEYWORDS = "TABLESAMPLE" (base); no dialect override
		// within base/mysql/postgres.
		TablesampleKeywords: "TABLESAMPLE",
		// generator.py:526 COPY_HAS_INTO_KEYWORD = True (base); postgres overrides to False
		// (generators/postgres.py:251).
		CopyHasIntoKeyword: true,
		// generator.py:520 COPY_PARAMS_ARE_WRAPPED = True (base).
		CopyParamsAreWrapped: true,
		// dialects/dialect.py:363 COPY_PARAMS_ARE_CSV = True (base).
		CopyParamsAreCsv: true,
		// generator.py:523 COPY_PARAMS_EQ_REQUIRED = False (base).
		CopyParamsEqRequired: false,
		// parsers/postgres.py:191 JSON_ARROWS_REQUIRE_JSON_TYPE = False (base/mysql); postgres
		// overrides to True.
		JSONArrowsRequireJSONType: false,
		// generator.py:488 JSON_TYPE_REQUIRED_FOR_EXTRACTION = False (base); mysql
		// (generators/mysql.py:142) and postgres (generators/postgres.py:245) both override to
		// True.
		JSONTypeRequiredForExtraction: false,
		// parser.py:1755 STRICT_CAST = True (base); Hive overrides to False.
		StrictCast: true,
		// parser.py:1765 LOG_DEFAULTS_TO_LN = False (base); Hive overrides to True.
		LogDefaultsToLn: false,
		// parser.py:1824-1828 JOINS_HAVE_EQUAL_PRECEDENCE = False (base); Hive overrides
		// to True.
		JoinsHaveEqualPrecedence: false,
		// parser.py:1842-1844 ADD_JOIN_ON_TRUE = False (base); Hive overrides to True.
		AddJoinOnTrue: false,
		// dialect.py:679 REGEXP_EXTRACT_DEFAULT_GROUP = 0 (base); Hive overrides to 1.
		RegexpExtractDefaultGroup: 0,
		// dialect.py:682-683 REGEXP_EXTRACT_POSITION_OVERFLOW_RETURNS_NULL = True.
		RegexpExtractPositionOverflowReturnsNull: true,
		// parser.py:1801 VALUES_FOLLOWED_BY_PAREN = True (base); MySQL overrides to False
		// (parsers/mysql.py:303).
		ValuesFollowedByParen: true,
		// parser.py:1831 ZONE_AWARE_TIMESTAMP_CONSTRUCTOR = False (base); presto overrides to
		// True (parsers/presto.py:61).
		ZoneAwareTimestampConstructor: false,
	}
}

var datePartMapping = map[string]string{
	"Y":                  "YEAR",
	"YY":                 "YEAR",
	"YYY":                "YEAR",
	"YYYY":               "YEAR",
	"YR":                 "YEAR",
	"YEARS":              "YEAR",
	"YRS":                "YEAR",
	"MM":                 "MONTH",
	"MON":                "MONTH",
	"MONS":               "MONTH",
	"MONTHS":             "MONTH",
	"D":                  "DAY",
	"DD":                 "DAY",
	"DAYS":               "DAY",
	"DAYOFMONTH":         "DAY",
	"DAY OF WEEK":        "DAYOFWEEK",
	"WEEKDAY":            "DAYOFWEEK",
	"DOW":                "DAYOFWEEK",
	"DW":                 "DAYOFWEEK",
	"WEEKDAY_ISO":        "DAYOFWEEKISO",
	"DOW_ISO":            "DAYOFWEEKISO",
	"DW_ISO":             "DAYOFWEEKISO",
	"DAYOFWEEK_ISO":      "DAYOFWEEKISO",
	"DAY OF YEAR":        "DAYOFYEAR",
	"DOY":                "DAYOFYEAR",
	"DY":                 "DAYOFYEAR",
	"W":                  "WEEK",
	"WK":                 "WEEK",
	"WEEKOFYEAR":         "WEEK",
	"WOY":                "WEEK",
	"WY":                 "WEEK",
	"WEEK_ISO":           "WEEKISO",
	"WEEKOFYEARISO":      "WEEKISO",
	"WEEKOFYEAR_ISO":     "WEEKISO",
	"Q":                  "QUARTER",
	"QTR":                "QUARTER",
	"QTRS":               "QUARTER",
	"QUARTERS":           "QUARTER",
	"H":                  "HOUR",
	"HH":                 "HOUR",
	"HR":                 "HOUR",
	"HOURS":              "HOUR",
	"HRS":                "HOUR",
	"M":                  "MINUTE",
	"MI":                 "MINUTE",
	"MIN":                "MINUTE",
	"MINUTES":            "MINUTE",
	"MINS":               "MINUTE",
	"S":                  "SECOND",
	"SEC":                "SECOND",
	"SECONDS":            "SECOND",
	"SECS":               "SECOND",
	"MS":                 "MILLISECOND",
	"MSEC":               "MILLISECOND",
	"MSECS":              "MILLISECOND",
	"MSECOND":            "MILLISECOND",
	"MSECONDS":           "MILLISECOND",
	"MILLISEC":           "MILLISECOND",
	"MILLISECS":          "MILLISECOND",
	"MILLISECON":         "MILLISECOND",
	"MILLISECONDS":       "MILLISECOND",
	"US":                 "MICROSECOND",
	"USEC":               "MICROSECOND",
	"USECS":              "MICROSECOND",
	"MICROSEC":           "MICROSECOND",
	"MICROSECS":          "MICROSECOND",
	"USECOND":            "MICROSECOND",
	"USECONDS":           "MICROSECOND",
	"MICROSECONDS":       "MICROSECOND",
	"NS":                 "NANOSECOND",
	"NSEC":               "NANOSECOND",
	"NANOSEC":            "NANOSECOND",
	"NSECOND":            "NANOSECOND",
	"NSECONDS":           "NANOSECOND",
	"NANOSECS":           "NANOSECOND",
	"EPOCH_SECOND":       "EPOCH",
	"EPOCH_SECONDS":      "EPOCH",
	"EPOCH_MILLISECONDS": "EPOCH_MILLISECOND",
	"EPOCH_MICROSECONDS": "EPOCH_MICROSECOND",
	"EPOCH_NANOSECONDS":  "EPOCH_NANOSECOND",
	"TZH":                "TIMEZONE_HOUR",
	"TZM":                "TIMEZONE_MINUTE",
	"DEC":                "DECADE",
	"DECS":               "DECADE",
	"DECADES":            "DECADE",
	"MIL":                "MILLENNIUM",
	"MILS":               "MILLENNIUM",
	"MILLENIA":           "MILLENNIUM",
	"C":                  "CENTURY",
	"CENT":               "CENTURY",
	"CENTS":              "CENTURY",
	"CENTURIES":          "CENTURY",
}

func baseDatePartMapping() map[string]string {
	out := make(map[string]string, len(datePartMapping))
	for key, value := range datePartMapping {
		out[key] = value
	}
	return out
}

func validIntervalUnits(mapping map[string]string) map[string]bool {
	out := map[string]bool{}
	for key, value := range mapping {
		out[key] = true
		out[value] = true
	}
	return out
}

// dialectByName constructs a fresh dialect from its bare name (no settings).
func dialectByName(name string) (*Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "base":
		return Base(), nil
	case "mysql":
		return MySQL(), nil
	case "postgres":
		return Postgres(), nil
	case "presto":
		return Presto(), nil
	case "trino":
		return Trino(), nil
	case "hive":
		return Hive(), nil
	case "athena":
		return Athena(), nil
	default:
		return nil, fmt.Errorf("unknown dialect %q", name)
	}
}

// parseNormalizationStrategy maps a canonical strategy name (case-insensitive; the
// upstream UPPER_SNAKE spellings, plus this port's two MySQL strategies) to the enum.
func parseNormalizationStrategy(v string) (NormalizationStrategy, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "LOWERCASE":
		return Lowercase, nil
	case "UPPERCASE":
		return Uppercase, nil
	case "CASE_SENSITIVE":
		return CaseSensitive, nil
	case "CASE_INSENSITIVE":
		return CaseInsensitive, nil
	case "CASE_INSENSITIVE_UPPERCASE":
		return CaseInsensitiveUppercase, nil
	case "MYSQL_CASE_INSENSITIVE":
		return MySQLCaseInsensitive, nil
	case "MYSQL_CASE_SENSITIVE_TABLE_NAMES":
		return MySQLCaseSensitiveTableNames, nil
	default:
		return 0, fmt.Errorf("unknown normalization_strategy %q", v)
	}
}

// normalizationStrategyToken is the reverse of parseNormalizationStrategy: it maps a
// strategy back to the settings-string token GetOrRaise accepts. Used by SettingsString so a
// typed *Dialect round-trips through the string-based settings resolver.
func normalizationStrategyToken(s NormalizationStrategy) (string, bool) {
	switch s {
	case Lowercase:
		return "lowercase", true
	case Uppercase:
		return "uppercase", true
	case CaseSensitive:
		return "case_sensitive", true
	case CaseInsensitive:
		return "case_insensitive", true
	case CaseInsensitiveUppercase:
		return "case_insensitive_uppercase", true
	case MySQLCaseInsensitive:
		return "mysql_case_insensitive", true
	case MySQLCaseSensitiveTableNames:
		return "mysql_case_sensitive_table_names", true
	default:
		return "", false
	}
}

// SettingsString serializes the dialect to the canonical "name, normalization_strategy=..."
// form that GetOrRaise round-trips. It captures the state that participates in identifier
// resolution — the dialect name (tokenizer/quoting) and normalization_strategy — which is
// the complete set of *Dialect state the qualify pipeline consults. Other tweaked fields are
// not serialized (they don't affect qualify), so a *Dialect handed to CanonicalString is
// honored for those two facets.
func (d *Dialect) SettingsString() string {
	if d == nil {
		return ""
	}
	if token, ok := normalizationStrategyToken(d.NormalizationStrategy); ok {
		return d.Name + ", normalization_strategy=" + token
	}
	return d.Name
}

// CanonicalString reduces a DialectType-style value (nil | string | *Dialect) to a string the
// string-based resolvers understand, mirroring upstream sqlglot's polymorphic DialectType at
// the boundaries that still thread a dialect as a string (schema, identifier parsing).
func CanonicalString(dialect any) (string, error) {
	switch v := dialect.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case *Dialect:
		return v.SettingsString(), nil
	case Dialect:
		return v.SettingsString(), nil
	default:
		return "", fmt.Errorf("invalid dialect %T (want string or *dialects.Dialect)", dialect)
	}
}

// GetOrRaise resolves a DialectType-style value to a *Dialect, mirroring upstream sqlglot's
// Dialect.get_or_raise(DialectType): it accepts nil (base), a string (bare name or the
// comma-separated settings form, e.g. "mysql, normalization_strategy=mysql_case_insensitive"),
// or an already-built *Dialect (returned as-is). Each string resolution returns a fresh
// *Dialect, so a per-call setting override cannot leak across callers. The supported string
// settings are normalization_strategy and mysql_version (upstream also has "version", not
// modeled here).
func GetOrRaise(dialect any) (*Dialect, error) {
	switch v := dialect.(type) {
	case nil:
		return getOrRaiseString("")
	case string:
		return getOrRaiseString(v)
	case *Dialect:
		if v == nil {
			return getOrRaiseString("")
		}
		return v, nil
	case Dialect:
		return &v, nil
	default:
		return nil, fmt.Errorf("invalid dialect %T (want string or *dialects.Dialect)", dialect)
	}
}

func getOrRaiseString(name string) (*Dialect, error) {
	base, rest, hasSettings := strings.Cut(name, ",")
	d, err := dialectByName(base)
	if err != nil {
		return nil, err
	}
	if !hasSettings {
		return d, nil
	}
	for _, part := range strings.Split(rest, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		key, val, hasVal := strings.Cut(part, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "normalization_strategy":
			if !hasVal {
				return nil, fmt.Errorf("dialect setting %q requires a value", key)
			}
			ns, err := parseNormalizationStrategy(val)
			if err != nil {
				return nil, err
			}
			d.NormalizationStrategy = ns
		case "mysql_version":
			if !hasVal {
				return nil, fmt.Errorf("dialect setting %q requires a value", key)
			}
			version, err := parseMySQLVersion(val)
			if err != nil {
				return nil, err
			}
			d.MySQLVersion = &version
		default:
			return nil, fmt.Errorf("unsupported dialect setting %q (supported: normalization_strategy, mysql_version)", key)
		}
	}
	return d, nil
}

func (d *Dialect) NewTokenizer() *tokens.Tokenizer {
	if d.TokenizerFactory != nil {
		return d.TokenizerFactory()
	}
	cfg := d.TokenizerConfig
	cfg.MySQLVersion = d.MySQLVersion
	return tokens.NewTokenizerWithConfig(cfg)
}

// asciiLower / asciiUpper case-fold ONLY the ASCII letters A-Z <-> a-z
// (bytes 0x41-0x5A / 0x61-0x7A), leaving every byte >= 0x80 unchanged. This
// models how real engines case-fold *unquoted* identifiers: PostgreSQL's
// downcase_identifier (src/backend/parser/scansup.c) folds ASCII-only on any
// multibyte (e.g. UTF-8) server_encoding, so `CAFÉ` -> `cafÉ` (C,A,F folded,
// É left alone), NOT `café`. UTF-8 lead/continuation bytes are always >= 0x80,
// so the plain byte scan never touches a multibyte sequence.
//
// DIVERGENCE FROM UPSTREAM (intentional): sqlglot's Dialect.normalize_identifier
// folds with Python str.lower()/str.upper() (full-Unicode) — dialects/dialect.py
// v30.12.0:1042-1050 — which over-folds non-ASCII case (É->é) and so diverges
// from the database it models. We fold ASCII-only: exact for UTF-8 and a safe
// under-fold otherwise (sqlglot has no DB connection, can't know the encoding/
// locale). Matches PostgreSQL downcase_identifier (scansup.c), which folds
// ASCII-only on multibyte encodings. Full detail + citations: DEVIATIONS.md §1.1.
func asciiLower(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c + ('a' - 'A')
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

func asciiUpper(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'a' && c <= 'z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c - ('a' - 'A')
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

// isRelationLevelIdentifier reports whether e names or references a table/relation (as opposed to a
// column). Under MySQL lower_case_table_names=0 (the MySQLCaseSensitiveTableNames strategy) these are
// case-SENSITIVE and must NOT fold, while column-level identifiers stay case-INSENSITIVE and fold. The
// relation-level positions are:
//   - a this/db/catalog child of an exp.Table    — a physical table/db/catalog name
//   - a table/db/catalog child of an exp.Column  — a column QUALIFIER (it references a relation/alias)
//   - the this child of an exp.TableAlias        — a table alias OR a CTE name
//
// Everything else folds: exp.Column.this (the leaf column name), exp.TableAlias.columns (a CTE
// output-column list), exp.Alias.alias (a SELECT column alias), JOIN USING columns — all
// case-insensitive on every MySQL platform. This matches MySQL lctn=0 exactly: `SELECT users.rrn FROM
// Users` errors because the qualifier is case-sensitive, and `WITH Users AS (…) … FROM users` misses
// the CTE because CTE/alias names are case-sensitive, while column names fold. A nil parent
// (Copy()/parse-identifier paths) returns false so a detached identifier folds (the standalone-name
// default; the NormalizeIdentifiers tree walk always has parents).
func isRelationLevelIdentifier(e exp.Expression) bool {
	p := e.Parent()
	if p == nil {
		return false
	}
	switch p.Kind() {
	case exp.KindTable:
		switch e.ArgKey() {
		case "this", "db", "catalog":
			return true
		}
	case exp.KindColumn:
		switch e.ArgKey() {
		case "table", "db", "catalog":
			return true
		}
	case exp.KindTableAlias:
		return e.ArgKey() == "this"
	}
	return false
}

// FoldIdentifierName applies the dialect's normalization strategy to a bare identifier NAME
// (a plain string with no AST node/parent), returning the folded form. It is the string-level
// counterpart to NormalizeIdentifier — which derives an identifier's role from its parent — for
// callers that must fold a detached name (e.g. a catalog/schema key) where no AST context
// exists. isTable selects the role for the role-aware MySQLCaseSensitiveTableNames strategy
// (lctn=0): true = a relation-level name (table/db/catalog/alias/CTE) that stays case-sensitive;
// false = a column-level name that folds. It is ignored by the other strategies.
//
// The input is treated as an UNQUOTED name (folding is unconditional): callers pass a name they
// have already decided is unquoted, matching how schema normalization builds identifiers with
// quoted=false. A quoted name must not be folded and should not be passed here.
func (d *Dialect) FoldIdentifierName(name string, isTable bool) string {
	switch d.NormalizationStrategy {
	case CaseSensitive:
		return name
	case MySQLCaseSensitiveTableNames:
		if isTable {
			return name
		}
		return MySQLLower(name)
	case MySQLCaseInsensitive:
		return MySQLLower(name)
	case Uppercase, CaseInsensitiveUppercase:
		return asciiUpper(name)
	default: // Lowercase, CaseInsensitive
		return asciiLower(name)
	}
}

func (d *Dialect) NormalizeIdentifier(e exp.Expression) exp.Expression {
	s := d.NormalizationStrategy
	if e == nil || e.Kind() != exp.KindIdentifier || s == CaseSensitive {
		return e
	}

	// MySQL strategies (non-upstream): fold with MySQL's exact .tolower map, regardless of
	// quoting (MySQL identifier case-insensitivity ignores backticks). See DEVIATIONS.md §1.2.
	switch s {
	case MySQLCaseSensitiveTableNames:
		// Role-aware (models lctn=0): relation-level identifiers — table/db/catalog names, column
		// QUALIFIERS, and table-alias/CTE names — stay case-sensitive; column-level identifiers (leaf
		// column names, CTE output-column lists, column aliases) fold with MySQLLower.
		if !isRelationLevelIdentifier(e) {
			this, _ := e.Arg("this").(string)
			e.Set("this", MySQLLower(this))
		}
		return e
	case MySQLCaseInsensitive:
		this, _ := e.Arg("this").(string)
		e.Set("this", MySQLLower(this))
		return e
	}

	// Upstream strategies: ASCII-only fold; quoted identifiers are protected unless the
	// strategy is case-insensitive (see DEVIATIONS.md §1.1).
	quoted, _ := e.Arg("quoted").(bool)
	if !quoted || s == CaseInsensitive || s == CaseInsensitiveUppercase {
		this, _ := e.Arg("this").(string)
		if s == Uppercase || s == CaseInsensitiveUppercase {
			e.Set("this", asciiUpper(this))
		} else {
			e.Set("this", asciiLower(this))
		}
	}
	return e
}

// IsReservedKeyword reports whether word is a reserved keyword in this dialect — a word that
// cannot be an unquoted identifier. It is the case-safe accessor for ReservedKeywords (which is
// stored lowercase-keyed): a consumer folding keyword case must fold ONLY reserved words, since a
// non-reserved keyword can be a case-sensitive identifier (e.g. on MySQL lower_case_table_names=0).
// Only MySQL populates a reserved set today; other dialects return false.
func (d *Dialect) IsReservedKeyword(word string) bool {
	if d.ReservedKeywords == nil {
		return false
	}
	return d.ReservedKeywords[strings.ToLower(word)]
}

func (d *Dialect) CaseSensitive(text string) bool {
	// Strategies that fold every identifier unconditionally never need quoting to
	// preserve case: nothing is preserved.
	if d.NormalizationStrategy == CaseInsensitive || d.NormalizationStrategy == MySQLCaseInsensitive {
		return false
	}
	// ASCII-only, to stay consistent with the ASCII fold (asciiLower/asciiUpper): an
	// identifier is "case sensitive" (would be changed by folding, so must be quoted to
	// preserve it) only if it contains an ASCII letter of the case the fold would flip. A
	// non-ASCII letter like `É` is not considered here — for the ASCII-folding strategies it
	// is never folded, and for the MySQL strategies (MySQLCaseSensitiveTableNames) this stays
	// an ASCII approximation used only for output quoting (CanQuote), not the normalized
	// identity; being conservative here at worst under-quotes a non-ASCII name on output.
	// (Upstream uses full-Unicode str.isupper/str.islower — dialects/dialect.py
	// v30.12.0:1055-1064; we diverge to match the ASCII fold. See DEVIATIONS.md §1.1.)
	unsafe := func(r rune) bool { return r >= 'A' && r <= 'Z' } // lower-folding strategies
	if d.NormalizationStrategy == Uppercase {
		unsafe = func(r rune) bool { return r >= 'a' && r <= 'z' }
	}
	for _, r := range text {
		if unsafe(r) {
			return true
		}
	}
	return false
}

func (d *Dialect) CanQuote(identifier exp.Expression, identify any) bool {
	if identifier == nil || identifier.Kind() != exp.KindIdentifier {
		return false
	}
	quoted, _ := identifier.Arg("quoted").(bool)
	if quoted {
		return true
	}
	if identify == nil {
		return false
	}
	if b, ok := identify.(bool); ok && !b {
		return false
	}
	if parent := identifier.Parent(); parent != nil && parent.Is(exp.TraitFunc) {
		return false
	}
	if b, ok := identify.(bool); ok && b {
		return true
	}
	name := identifier.Name()
	isSafe := !d.CaseSensitive(name) && exp.IsSafeIdentifier(name)
	switch identify {
	case "safe":
		return isSafe
	case "unsafe":
		return !isSafe
	}
	panic(fmt.Sprintf("Unexpected argument for identify: '%v'", identify))
}

func (d *Dialect) QuoteIdentifier(identifier exp.Expression, identify bool) exp.Expression {
	if identifier != nil && identifier.Kind() == exp.KindIdentifier {
		mode := any(identify)
		if !identify {
			mode = "unsafe"
		}
		identifier.Set("quoted", d.CanQuote(identifier, mode))
	}
	return identifier
}

func (d *Dialect) GenerateValuesAliases(values exp.Expression) []exp.Expression {
	expressions := values.Expressions()
	if len(expressions) == 0 || expressions[0] == nil {
		return nil
	}
	columns := expressions[0].Expressions()
	aliases := make([]exp.Expression, 0, len(columns))
	for i := range columns {
		aliases = append(aliases, exp.ToIdentifier(fmt.Sprintf("_col_%d", i)))
	}
	return aliases
}
