package dialects

import (
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// Hive ports the Hive dialect's parser and tokenizer surface. Generator transforms, type
// mappings, INITCAP behavior, and typing metadata remain intentionally out of scope.
func Hive() *Dialect {
	d := Base()
	d.Name = "hive"
	d.QuoteStart = "'"
	d.QuoteEnd = "'"
	d.IdentifierStart = "`"
	d.IdentifierEnd = "`"

	// dialects/hive.py:18-29 class attributes.
	d.AliasPostTablesample = true
	d.SupportsUserDefinedTypes = false
	d.SafeDivision = true
	d.NormalizationStrategy = CaseInsensitive
	d.AlterTableSupportsCascade = true
	// parsers/hive.py:53-61 parser flags.
	d.AlterTablePartitions = true
	d.ValuesFollowedByParen = false
	d.StrictCast = false
	d.LogDefaultsToLn = true
	d.JoinsHaveEqualPrecedence = true
	d.AddJoinOnTrue = true
	// dialects/hive.py:25 and dialect.py:682-683.
	d.RegexpExtractDefaultGroup = 1
	d.RegexpExtractPositionOverflowReturnsNull = true

	// parsers/hive.py:70-120 FUNCTIONS overlay. The callbacks build the canonical expression
	// Kinds directly because this port keeps the base registry and dialect overlays separate.
	d.Functions = map[string]func([]exp.Expression) exp.Expression{
		"BASE64":             exp.FromArgListFunc(exp.KindToBase64),
		"COLLECT_LIST":       hiveBuildCollectList,
		"COLLECT_SET":        exp.FromArgListFunc(exp.KindArrayUniqueAgg),
		"DATE_ADD":           hiveBuildDateAdd,
		"DATE_FORMAT":        hiveBuildDateFormat,
		"DATE_SUB":           hiveBuildDateSub,
		"DATEDIFF":           hiveBuildDateDiff,
		"DAY":                hiveBuildDay,
		"FIRST":              hiveBuildWithIgnoreNulls(exp.KindFirst),
		"FIRST_VALUE":        hiveBuildWithIgnoreNulls(exp.KindFirstValue),
		"FROM_UNIXTIME":      hiveBuildFromUnixTime,
		"GET_JSON_OBJECT":    hiveBuildGetJSONObject,
		"LAST":               hiveBuildWithIgnoreNulls(exp.KindLast),
		"LAST_VALUE":         hiveBuildWithIgnoreNulls(exp.KindLastValue),
		"LOG":                hiveBuildLogarithm(d),
		"MAP":                hiveBuildVarMap,
		"MONTH":              hiveBuildMonth,
		"NAMED_STRUCT":       hiveBuildNamedStruct,
		"REGEXP_EXTRACT":     hiveBuildRegexpExtract(exp.KindRegexpExtract, d),
		"REGEXP_EXTRACT_ALL": hiveBuildRegexpExtract(exp.KindRegexpExtractAll, d),
		"SEQUENCE":           exp.FromArgListFunc(exp.KindGenerateSeries),
		"SIZE":               exp.FromArgListFunc(exp.KindArraySize),
		"SPLIT":              exp.FromArgListFunc(exp.KindRegexpSplit),
		"STR_TO_MAP":         hiveBuildStrToMap,
		"TO_DATE":            hiveBuildToDate,
		"TO_JSON":            exp.FromArgListFunc(exp.KindJSONFormat),
		"TRUNC":              hiveBuildTimestampTrunc,
		"UNBASE64":           exp.FromArgListFunc(exp.KindFromBase64),
		"UNIX_TIMESTAMP":     hiveBuildUnixTimestamp,
		"YEAR":               hiveBuildYear,
	}

	cfg := tokens.BaseConfig()
	// dialects/hive.py:89-97 Tokenizer delimiters and single-token overlay.
	cfg.Quotes = map[string]string{"'": "'", "\"": "\""}
	cfg.Identifiers = map[rune]string{'`': "`"}
	cfg.StringEscapes = map[rune]bool{'\\': true}
	cfg.SingleTokens['$'] = tokens.PARAMETER
	cfg.IdentifiersCanStartWithDigit = true
	// dialects/hive.py:99-113 keyword overlay.
	for keyword, tokenType := range map[string]tokens.TokenType{
		"ADD ARCHIVE":     tokens.COMMAND,
		"ADD ARCHIVES":    tokens.COMMAND,
		"ADD FILE":        tokens.COMMAND,
		"ADD FILES":       tokens.COMMAND,
		"ADD JAR":         tokens.COMMAND,
		"ADD JARS":        tokens.COMMAND,
		"MINUS":           tokens.EXCEPT,
		"MSCK REPAIR":     tokens.COMMAND,
		"REFRESH":         tokens.REFRESH,
		"TIMESTAMP AS OF": tokens.TIMESTAMP_SNAPSHOT,
		"VERSION AS OF":   tokens.VERSION_SNAPSHOT,
		"SERDEPROPERTIES": tokens.SERDE_PROPERTIES,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	// dialects/hive.py:115-122 numeric literal suffixes.
	cfg.NumericLiterals = map[string]string{
		"L":  "BIGINT",
		"S":  "SMALLINT",
		"Y":  "TINYINT",
		"D":  "DOUBLE",
		"F":  "FLOAT",
		"BD": "DECIMAL",
	}
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}

// hiveSeqGet ports sqlglot.helper.seq_get: args[i] or nil when out of range.
func hiveSeqGet(args []exp.Expression, i int) exp.Expression {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return nil
}

func hiveBuildCollectList(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindArrayAgg, exp.Args{
		"this":           hiveSeqGet(args, 0),
		"nulls_excluded": true,
	})
}

func hiveBuildDateAdd(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindTsOrDsAdd, exp.Args{
		"this":       hiveSeqGet(args, 0),
		"expression": hiveSeqGet(args, 1),
		"unit":       exp.Var(exp.Args{"this": "DAY"}),
	})
}

func hiveBuildDateSub(args []exp.Expression) exp.Expression {
	delta := hiveSeqGet(args, 1)
	if delta != nil {
		delta = exp.Mul(exp.Args{
			"this":       delta,
			"expression": exp.LiteralNumber(-1),
		})
	}
	return exp.New(exp.KindTsOrDsAdd, exp.Args{
		"this":       hiveSeqGet(args, 0),
		"expression": delta,
		"unit":       exp.Var(exp.Args{"this": "DAY"}),
	})
}

func hiveBuildDateDiff(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindDateDiff, exp.Args{
		"this": exp.New(exp.KindTsOrDsToDate, exp.Args{
			"this": hiveSeqGet(args, 0),
		}),
		"expression": exp.New(exp.KindTsOrDsToDate, exp.Args{
			"this": hiveSeqGet(args, 1),
		}),
	})
}

func hiveBuildDay(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindDay, exp.Args{
		"this": exp.New(exp.KindTsOrDsToDate, exp.Args{"this": hiveSeqGet(args, 0)}),
	})
}

func hiveBuildMonth(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindMonth, exp.Args{
		"this": exp.FromArgList(exp.KindTsOrDsToDate, args),
	})
}

func hiveBuildYear(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindYear, exp.Args{
		"this": exp.FromArgList(exp.KindTsOrDsToDate, args),
	})
}

func hiveBuildWithIgnoreNulls(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		value := exp.New(kind, exp.Args{"this": hiveSeqGet(args, 0)})
		ignoreNulls := hiveSeqGet(args, 1)
		if ignoreNulls != nil && ignoreNulls.Equal(exp.Boolean(exp.Args{"this": true})) {
			return exp.IgnoreNulls(exp.Args{"this": value})
		}
		return value
	}
}

func hiveBuildNamedStruct(args []exp.Expression) exp.Expression {
	properties := make([]exp.Expression, 0, len(args)/2)
	for i := 0; i < len(args)-1; i += 2 {
		properties = append(properties, exp.PropertyEQ(exp.Args{
			"this":       exp.ToIdentifier(args[i].Name()),
			"expression": args[i+1],
		}))
	}
	return exp.New(exp.KindStruct, exp.Args{"expressions": properties})
}

func hiveBuildVarMap(args []exp.Expression) exp.Expression {
	if len(args) == 1 && args[0] != nil && args[0].IsStar() {
		return exp.New(exp.KindStarMap, exp.Args{"this": args[0]})
	}

	keys := make([]exp.Expression, 0, (len(args)+1)/2)
	values := make([]exp.Expression, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		keys = append(keys, args[i])
		values = append(values, args[i+1])
	}
	return exp.New(exp.KindVarMap, exp.Args{
		"keys":   exp.Array(exp.Args{"expressions": keys}),
		"values": exp.Array(exp.Args{"expressions": values}),
	})
}

func hiveBuildRegexpExtract(kind exp.Kind, d *Dialect) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		group := hiveSeqGet(args, 2)
		if group == nil {
			group = exp.LiteralNumber(d.RegexpExtractDefaultGroup)
		}
		resultArgs := exp.Args{
			"this":       hiveSeqGet(args, 0),
			"expression": hiveSeqGet(args, 1),
			"group":      group,
			"parameters": hiveSeqGet(args, 3),
		}
		if kind == exp.KindRegexpExtract {
			resultArgs["null_if_pos_overflow"] = d.RegexpExtractPositionOverflowReturnsNull
		}
		return exp.New(kind, resultArgs)
	}
}

func hiveBuildStrToMap(args []exp.Expression) exp.Expression {
	pairDelimiter := hiveSeqGet(args, 1)
	if pairDelimiter == nil {
		pairDelimiter = exp.LiteralString(",")
	}
	keyValueDelimiter := hiveSeqGet(args, 2)
	if keyValueDelimiter == nil {
		keyValueDelimiter = exp.LiteralString(":")
	}
	return exp.New(exp.KindStrToMap, exp.Args{
		"this":            hiveSeqGet(args, 0),
		"pair_delim":      pairDelimiter,
		"key_value_delim": keyValueDelimiter,
	})
}

func hiveBuildLogarithm(d *Dialect) func([]exp.Expression) exp.Expression {
	// parser.py:74-84 build_logarithm. The base callback is not dialect-aware in this port,
	// so this Hive-local adapter consumes LogDefaultsToLn while preserving two-argument LOG.
	return func(args []exp.Expression) exp.Expression {
		if expression := hiveSeqGet(args, 1); expression != nil {
			return exp.New(exp.KindLog, exp.Args{
				"this":       hiveSeqGet(args, 0),
				"expression": expression,
			})
		}
		kind := exp.KindLog
		if d.LogDefaultsToLn {
			kind = exp.KindLn
		}
		return exp.New(kind, exp.Args{"this": hiveSeqGet(args, 0)})
	}
}

func hiveBuildGetJSONObject(args []exp.Expression) exp.Expression {
	// parsers/hive.py:94-96 calls dialect.to_json_path. This repository intentionally defers
	// the JSONPath subsystem, so preserve the parsed path expression raw rather than silently
	// approximating JSONPath semantics.
	return exp.New(exp.KindJSONExtractScalar, exp.Args{
		"this":       hiveSeqGet(args, 0),
		"expression": hiveSeqGet(args, 1),
	})
}

func hiveBuildTimestampTrunc(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindTimestampTrunc, exp.Args{
		"this": hiveSeqGet(args, 0),
		"unit": hiveTimeUnit(hiveSeqGet(args, 1)),
	})
}

// hiveUnabbreviatedUnitName mirrors TimeUnit.UNABBREVIATED_UNIT_NAME (core.py:2023-2036).
// Its raw-name lookup is intentionally case-sensitive; TimeUnit uppercases only after lookup.
var hiveUnabbreviatedUnitName = map[string]string{
	"D":  "DAY",
	"H":  "HOUR",
	"M":  "MINUTE",
	"MS": "MILLISECOND",
	"NS": "NANOSECOND",
	"Q":  "QUARTER",
	"S":  "SECOND",
	"US": "MICROSECOND",
	"W":  "WEEK",
	"Y":  "YEAR",
}

func hiveTimeUnit(unit exp.Expression) exp.Expression {
	if unit == nil {
		return nil
	}

	var name string
	switch unit.Kind() {
	case exp.KindColumn:
		if len(unit.Parts()) != 1 {
			return unit
		}
		name = unit.Name()
	case exp.KindLiteral, exp.KindVar:
		name = unit.Name()
	default:
		return unit
	}

	if expanded := hiveUnabbreviatedUnitName[name]; expanded != "" {
		name = expanded
	}
	return exp.Var(exp.Args{"this": strings.ToUpper(name)})
}

func hiveBuildDateFormat(args []exp.Expression) exp.Expression {
	this := exp.New(exp.KindTimeStrToTime, exp.Args{"this": hiveSeqGet(args, 0)})
	return hiveBuildFormattedTime(exp.KindTimeToStr, this, hiveSeqGet(args, 1), false)
}

func hiveBuildFromUnixTime(args []exp.Expression) exp.Expression {
	return hiveBuildFormattedTime(
		exp.KindUnixToStr,
		hiveSeqGet(args, 0),
		hiveSeqGet(args, 1),
		true,
	)
}

func hiveBuildToDate(args []exp.Expression) exp.Expression {
	result := hiveBuildFormattedTime(
		exp.KindTsOrDsToDate,
		hiveSeqGet(args, 0),
		hiveSeqGet(args, 1),
		false,
	)
	result.Set("safe", true)
	return result
}

func hiveBuildUnixTimestamp(args []exp.Expression) exp.Expression {
	if len(args) == 0 {
		args = []exp.Expression{exp.New(exp.KindCurrentTimestamp, nil)}
	}
	return hiveBuildFormattedTime(
		exp.KindStrToUnix,
		hiveSeqGet(args, 0),
		hiveSeqGet(args, 1),
		true,
	)
}

func hiveBuildFormattedTime(
	kind exp.Kind,
	this exp.Expression,
	format exp.Expression,
	defaultTime bool,
) exp.Expression {
	if format == nil && defaultTime {
		format = exp.LiteralString(hiveTimeFormat[1 : len(hiveTimeFormat)-1])
	}
	format = hiveFormatTime(format)

	args := exp.Args{"this": this}
	if format != nil {
		args["format"] = format
	}
	return exp.New(kind, args)
}

// dialects/hive.py:46-77 TIME_MAPPING and :81 TIME_FORMAT. The mapping converts Hive
// date-pattern tokens into sqlglot's generic Python-strftime representation.
var hiveTimeMapping = map[string]string{
	"y":      "%Y",
	"Y":      "%Y",
	"YYYY":   "%Y",
	"yyyy":   "%Y",
	"YY":     "%y",
	"yy":     "%y",
	"MMMM":   "%B",
	"MMM":    "%b",
	"MM":     "%m",
	"M":      "%-m",
	"dd":     "%d",
	"d":      "%-d",
	"HH":     "%H",
	"H":      "%-H",
	"hh":     "%I",
	"h":      "%-I",
	"mm":     "%M",
	"m":      "%-M",
	"ss":     "%S",
	"s":      "%-S",
	"SSSSSS": "%f",
	"a":      "%p",
	"DD":     "%j",
	"D":      "%-j",
	"E":      "%a",
	"EE":     "%a",
	"EEE":    "%a",
	"EEEE":   "%A",
	"z":      "%Z",
	"Z":      "%z",
}

const hiveTimeFormat = "'yyyy-MM-dd HH:mm:ss'"

type hiveTimeTrieNode struct {
	children map[rune]*hiveTimeTrieNode
	exists   bool
}

var hiveTimeTrie = func() *hiveTimeTrieNode {
	root := &hiveTimeTrieNode{children: map[rune]*hiveTimeTrieNode{}}
	for pattern := range hiveTimeMapping {
		current := root
		for _, char := range pattern {
			if current.children[char] == nil {
				current.children[char] = &hiveTimeTrieNode{children: map[rune]*hiveTimeTrieNode{}}
			}
			current = current.children[char]
		}
		current.exists = true
	}
	return root
}()

func hiveFormatTime(format exp.Expression) exp.Expression {
	if format == nil || !format.IsString() {
		return format
	}
	converted, ok := hiveConvertTimeFormat(format.Text("this"))
	if !ok {
		// time.py:27 returns None for an empty format; Literal.string(None) stores "None".
		return exp.LiteralString("None")
	}
	return exp.LiteralString(converted)
}

// hiveConvertTimeFormat ports time.py:10-62. In particular, it remembers the longest
// completed trie match and reprocesses the first character after that match on failure.
func hiveConvertTimeFormat(value string) (string, bool) {
	if value == "" {
		return "", false
	}

	characters := []rune(value)
	start, end := 0, 1
	current := hiveTimeTrie
	chunks := make([]string, 0, len(characters))
	matchedSymbol := ""

	for end <= len(characters) {
		chars := string(characters[start:end])
		next, found := current.children[characters[end-1]]
		failed := !found
		if failed {
			if matchedSymbol != "" {
				end--
				chars = matchedSymbol
				matchedSymbol = ""
			} else {
				chars = string(characters[start])
				end = start + 1
			}
			start += len([]rune(chars))
			chunks = append(chunks, chars)
			current = hiveTimeTrie
		} else {
			current = next
			if current.exists {
				matchedSymbol = chars
			}
		}

		end++
		if !failed && end > len(characters) {
			chunks = append(chunks, chars)
		}
	}

	var converted strings.Builder
	for _, chunk := range chunks {
		if replacement, ok := hiveTimeMapping[chunk]; ok {
			converted.WriteString(replacement)
		} else {
			converted.WriteString(chunk)
		}
	}
	return converted.String(), true
}
