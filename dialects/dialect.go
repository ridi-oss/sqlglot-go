package dialects

import (
	"fmt"
	"strings"
	"unicode"

	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

type NormalizationStrategy int

const (
	Lowercase NormalizationStrategy = iota
	Uppercase
	CaseSensitive
	CaseInsensitive
	CaseInsensitiveUppercase
)

type Dialect struct {
	Name                     string
	QuoteStart               string
	QuoteEnd                 string
	IdentifierStart          string
	IdentifierEnd            string
	TokenizerConfig          tokens.TokenizerConfig
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
	// ValuesIsFunction ports the MySQL parser's FUNC_TOKENS addition of TokenType.VALUES
	// (parsers/mysql.py:63-70) paired with its FUNCTION_PARSERS["VALUES"] override
	// (parsers/mysql.py:158-160): `VALUES(col)` in `ON DUPLICATE KEY UPDATE` is an
	// Anonymous function call, not the VALUES table-constructor keyword. Divergence: this
	// port has no per-dialect FUNC_TOKENS/FUNCTION_PARSERS table (deferred to slice 5b), so
	// a single dialect flag gates the shared parseFunctionCall/functionParsers["VALUES"]
	// entry instead of a real per-dialect override.
	ValuesIsFunction bool
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
		SupportsLimitAll:         false,
		// dialect.py:670 SUPPORTS_VALUES_DEFAULT = True (base); Presto/Dremio override to
		// False (out of scope: only base/mysql/postgres are ported).
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
		// parsers/postgres.py:191 JSON_ARROWS_REQUIRE_JSON_TYPE = False (base/mysql); postgres
		// overrides to True.
		JSONArrowsRequireJSONType: false,
		// generator.py:488 JSON_TYPE_REQUIRED_FOR_EXTRACTION = False (base); mysql
		// (generators/mysql.py:142) and postgres (generators/postgres.py:245) both override to
		// True.
		JSONTypeRequiredForExtraction: false,
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

func GetOrRaise(name string) (*Dialect, error) {
	switch strings.ToLower(name) {
	case "", "base":
		return Base(), nil
	case "mysql":
		return MySQL(), nil
	case "postgres":
		return Postgres(), nil
	default:
		return nil, fmt.Errorf("unknown dialect %q", name)
	}
}

func (d *Dialect) NewTokenizer() *tokens.Tokenizer {
	return tokens.NewTokenizerWithConfig(d.TokenizerConfig)
}

func (d *Dialect) NormalizeIdentifier(e exp.Expression) exp.Expression {
	if e != nil && e.Kind() == exp.KindIdentifier && d.NormalizationStrategy != CaseSensitive {
		quoted, _ := e.Arg("quoted").(bool)
		if !quoted || d.NormalizationStrategy == CaseInsensitive || d.NormalizationStrategy == CaseInsensitiveUppercase {
			this, _ := e.Arg("this").(string)
			if d.NormalizationStrategy == Uppercase || d.NormalizationStrategy == CaseInsensitiveUppercase {
				e.Set("this", strings.ToUpper(this))
			} else {
				e.Set("this", strings.ToLower(this))
			}
		}
	}
	return e
}

func (d *Dialect) CaseSensitive(text string) bool {
	if d.NormalizationStrategy == CaseInsensitive {
		return false
	}
	unsafe := unicode.IsUpper
	if d.NormalizationStrategy == Uppercase {
		unsafe = unicode.IsLower
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
