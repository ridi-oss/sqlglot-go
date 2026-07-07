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
	TokenizerConfig                    tokens.TokenizerConfig
	NormalizationStrategy              NormalizationStrategy
	DPipeIsStringConcat                bool
	StrictStringConcat                 bool
	TypedDivision                      bool
	SafeDivision                       bool
	SupportsColumnJoinMarks            bool
	ColonIsVariantExtract              bool
	NullOrdering                       string
	SupportsOrderByAll                 bool
	TryCastRequiresString              *bool
	DatePartMapping                    map[string]string
	ValidIntervalUnits                 map[string]bool
	SupportsUserDefinedTypes           bool
	SupportsFixedSizeArrays            bool
	SupportsLimitAll                   bool
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
}

func Base() *Dialect {
	datePartMapping := baseDatePartMapping()
	return &Dialect{
		TokenizerConfig:          tokens.BaseConfig(),
		NormalizationStrategy:    Lowercase,
		DPipeIsStringConcat:      true,
		NullOrdering:             "nulls_are_small",
		DatePartMapping:          datePartMapping,
		ValidIntervalUnits:       validIntervalUnits(datePartMapping),
		SupportsUserDefinedTypes: true,
		SupportsFixedSizeArrays:  false,
		SupportsLimitAll:         false,
		IntervalSpans:            true,
		NormalizeFunctions:       "upper",
		AliasPostTablesample:     false,
		AliasPostVersion:         true,
		UnnestColumnOnly:         false,
		Pseudocolumns:            map[string]bool{},
		IndexOffset:              0,
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
	case "mysql", "postgres":
		// TODO(slice 5): wire real MySQL and Postgres dialect behavior.
		return Base(), nil
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
