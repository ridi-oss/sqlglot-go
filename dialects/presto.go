package dialects

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
	"strings"
)

// Presto ports dialects/presto.py and parsers/presto.py.
func Presto() *Dialect {
	d := Base()
	d.Name = "presto"
	// generators/presto.py:439-500.
	d.ReservedKeywords = map[string]bool{
		"alter":             true,
		"and":               true,
		"as":                true,
		"between":           true,
		"by":                true,
		"case":              true,
		"cast":              true,
		"constraint":        true,
		"create":            true,
		"cross":             true,
		"current_time":      true,
		"current_timestamp": true,
		"deallocate":        true,
		"delete":            true,
		"describe":          true,
		"distinct":          true,
		"drop":              true,
		"else":              true,
		"end":               true,
		"escape":            true,
		"except":            true,
		"execute":           true,
		"exists":            true,
		"extract":           true,
		"false":             true,
		"for":               true,
		"from":              true,
		"full":              true,
		"group":             true,
		"having":            true,
		"in":                true,
		"inner":             true,
		"insert":            true,
		"intersect":         true,
		"into":              true,
		"is":                true,
		"join":              true,
		"left":              true,
		"like":              true,
		"natural":           true,
		"not":               true,
		"null":              true,
		"on":                true,
		"or":                true,
		"order":             true,
		"outer":             true,
		"prepare":           true,
		"right":             true,
		"select":            true,
		"table":             true,
		"then":              true,
		"true":              true,
		"union":             true,
		"using":             true,
		"values":            true,
		"when":              true,
		"where":             true,
		"with":              true,
	}
	// dialects/presto.py:18-35 class attributes. Delimiters inherit base ANSI '/" (presto.py
	// declares no Tokenizer QUOTES/IDENTIFIERS override), so QuoteStart/IdentifierStart stay as
	// Base() set them; DPipeIsStringConcat likewise stays at the base True (no override).
	d.IndexOffset = 1
	d.NullOrdering = "nulls_are_last"
	d.StrictStringConcat = true
	d.TypedDivision = true
	d.TablesampleSizeIsPercent = true
	d.SupportsLimitAll = true
	d.SupportsValuesDefault = false
	// generators/presto.py:260 INTERVAL_ALLOWS_PLURAL_FORM = False.
	d.IntervalAllowsPluralForm = false
	// dialects/presto.py:35 NORMALIZATION_STRATEGY = NormalizationStrategy.CASE_INSENSITIVE;
	// first in-scope consumer of dialects.CaseInsensitive.
	d.NormalizationStrategy = CaseInsensitive
	// parsers/presto.py:60 VALUES_FOLLOWED_BY_PAREN = False (mysql.go:41-42 precedent).
	d.ValuesFollowedByParen = false
	// parsers/presto.py:61 ZONE_AWARE_TIMESTAMP_CONSTRUCTOR = True (read at parser.py:6186-6191).
	d.ZoneAwareTimestampConstructor = true

	// parsers/presto.py:74-139.
	d.Functions = map[string]func([]exp.Expression) exp.Expression{
		"ARBITRARY":            exp.FromArgListFunc(exp.KindAnyValue),
		"APPROX_DISTINCT":      exp.FromArgListFunc(exp.KindApproxDistinct),
		"APPROX_PERCENTILE":    prestoBuildApproxPercentile,
		"BITWISE_AND":          prestoBinaryFromFunction(exp.KindBitwiseAnd),
		"BITWISE_NOT":          exp.FromArgListFunc(exp.KindBitwiseNot),
		"BITWISE_OR":           prestoBinaryFromFunction(exp.KindBitwiseOr),
		"BITWISE_XOR":          prestoBinaryFromFunction(exp.KindBitwiseXor),
		"CARDINALITY":          exp.FromArgListFunc(exp.KindArraySize),
		"CONTAINS":             exp.FromArgListFunc(exp.KindArrayContains),
		"DATE_FORMAT":          prestoBuildFormattedTime(exp.KindTimeToStr, false),
		"DATE_PARSE":           prestoBuildFormattedTime(exp.KindStrToTime, false),
		"TO_CHAR":              prestoBuildFormattedTime(exp.KindTimeToStr, true),
		"DATE_TRUNC":           prestoBuildDateTrunc,
		"REGEXP_EXTRACT":       prestoBuildRegexpExtract(exp.KindRegexpExtract),
		"REGEXP_EXTRACT_ALL":   prestoBuildRegexpExtract(exp.KindRegexpExtractAll),
		"REGEXP_REPLACE":       prestoBuildRegexpReplace,
		"DATE_ADD":             prestoBuildDateAdd,
		"DATE_DIFF":            prestoBuildDateDiff,
		"DAY_OF_WEEK":          exp.FromArgListFunc(exp.KindDayOfWeekIso),
		"DOW":                  exp.FromArgListFunc(exp.KindDayOfWeekIso),
		"DOY":                  exp.FromArgListFunc(exp.KindDayOfYear),
		"ELEMENT_AT":           prestoBuildElementAt,
		"FROM_HEX":             exp.FromArgListFunc(exp.KindUnhex),
		"FROM_UNIXTIME":        prestoBuildFromUnixtime,
		"FROM_UTF8":            prestoBuildFromUtf8,
		"JSON_FORMAT":          prestoBuildJSONFormat,
		"LEVENSHTEIN_DISTANCE": exp.FromArgListFunc(exp.KindLevenshtein),
		"NOW":                  exp.FromArgListFunc(exp.KindCurrentTimestamp),
		"REPLACE":              prestoBuildReplace,
		"ROW":                  exp.FromArgListFunc(exp.KindStruct),
		"SEQUENCE":             exp.FromArgListFunc(exp.KindGenerateSeries),
		"SET_AGG":              exp.FromArgListFunc(exp.KindArrayUniqueAgg),
		"SPLIT_TO_MAP":         exp.FromArgListFunc(exp.KindStrToMap),
		"STRPOS":               prestoBuildStrpos,
		"SLICE":                exp.FromArgListFunc(exp.KindArraySlice),
		"TO_UNIXTIME":          exp.FromArgListFunc(exp.KindTimeToUnix),
		"TO_UTF8":              prestoBuildToUtf8,
		"MD5":                  exp.FromArgListFunc(exp.KindMD5Digest),
		"SHA256":               prestoBuildSHA256,
		"SHA512":               prestoBuildSHA512,
		// Func classes are registered by name upstream (parser.py:373), so the parenthesized
		// niladic forms build the same nodes as the bare keywords.
		"CURRENT_TIME":      exp.FromArgListFunc(exp.KindCurrentTime),
		"CURRENT_TIMESTAMP": exp.FromArgListFunc(exp.KindCurrentTimestamp),
		"CURRENT_USER":      exp.FromArgListFunc(exp.KindCurrentUser),
		"LOCALTIME":         exp.FromArgListFunc(exp.KindLocaltime),
		"LOCALTIMESTAMP":    exp.FromArgListFunc(exp.KindLocaltimestamp),
		"WEEK":              exp.FromArgListFunc(exp.KindWeekOfYear),
	}

	cfg := tokens.BaseConfig()
	// dialects/presto.py:45 HEX_STRINGS = [("x'", "'"), ("X'", "'")]; has_hex_strings = True
	// (tokens.py:582) also enables the number scanner's bare `0x` form. Presto declares no
	// BIT_STRINGS, so HasBitStrings stays false.
	cfg.HasHexStrings = true
	cfg.FormatStrings["x'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	cfg.FormatStrings["X'"] = tokens.FormatString{End: "'", TokenType: tokens.HEX_STRING}
	// HEX_START/HEX_END take the FIRST HEX_STRINGS tuple (dialects/dialect.py:293).
	d.HexStart, d.HexEnd = "x'", "'"
	// dialects/presto.py:46-50 UNICODE_STRINGS = [(prefix + q, q) for q in QUOTES for prefix in
	// ("U&", "u&")]; base QUOTES = ["'"], so U&'...'/u&'...' delimit a UNICODE_STRING literal.
	// Both cases are registered because the tokenizer resolves a format string by its
	// original-case matched text (postgres x'/X', e'/E' precedent).
	cfg.FormatStrings["U&'"] = tokens.FormatString{End: "'", TokenType: tokens.UNICODE_STRING}
	cfg.FormatStrings["u&'"] = tokens.FormatString{End: "'", TokenType: tokens.UNICODE_STRING}
	// dialects/presto.py:52 NESTED_COMMENTS = False.
	cfg.NestedComments = false

	// dialects/presto.py:54-67 KEYWORDS overrides.
	for keyword, tokenType := range map[string]tokens.TokenType{
		"DEALLOCATE PREPARE": tokens.COMMAND,
		"DESCRIBE INPUT":     tokens.COMMAND,
		"DESCRIBE OUTPUT":    tokens.COMMAND,
		"RESET SESSION":      tokens.COMMAND,
		"START":              tokens.BEGIN,
		"MATCH_RECOGNIZE":    tokens.MATCH_RECOGNIZE,
		"ROW":                tokens.STRUCT,
		"IPADDRESS":          tokens.IPADDRESS,
		"IPPREFIX":           tokens.IPPREFIX,
		"TDIGEST":            tokens.TDIGEST,
		"HYPERLOGLOG":        tokens.HLLSKETCH,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	// dialects/presto.py:68-69 KEYWORDS.pop("/*+") / KEYWORDS.pop("QUALIFY"): Presto has no
	// optimizer-hint comment and no QUALIFY keyword. Dropping "/*+" from KEYWORDS (and hence from
	// the derived hint Comment recomputed by CompileConfig) makes `/*+ ... */` scan as an ordinary
	// block comment; dropping QUALIFY lets a bare `qualify` parse as an identifier
	// (postgres.go:135-137 precedent).
	delete(cfg.Keywords, "/*+")
	delete(cfg.Comments, "/*+")
	delete(cfg.Keywords, "QUALIFY")
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}

// prestoSeqGet ports sqlglot.helper.seq_get: args[i] or nil when out of range.
func prestoSeqGet(args []exp.Expression, i int) exp.Expression {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return nil
}

// prestoBinaryFromFunction ports binary_from_function (dialects/dialect.py:1862): a two-arg
// builder mapping seq_get(0)/seq_get(1) onto {this, expression} of a Binary-node Kind (used by
// BITWISE_AND/OR/XOR, parsers/presto.py:79-82).
func prestoBinaryFromFunction(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		return exp.New(kind, exp.Args{
			"this":       prestoSeqGet(args, 0),
			"expression": prestoSeqGet(args, 1),
		})
	}
}

// prestoBuildApproxPercentile ports _build_approx_percentile (parsers/presto.py:20-32).
func prestoBuildApproxPercentile(args []exp.Expression) exp.Expression {
	switch len(args) {
	case 4:
		return exp.New(exp.KindApproxQuantile, exp.Args{
			"this":     prestoSeqGet(args, 0),
			"weight":   prestoSeqGet(args, 1),
			"quantile": prestoSeqGet(args, 2),
			"accuracy": prestoSeqGet(args, 3),
		})
	case 3:
		return exp.New(exp.KindApproxQuantile, exp.Args{
			"this":     prestoSeqGet(args, 0),
			"quantile": prestoSeqGet(args, 1),
			"accuracy": prestoSeqGet(args, 2),
		})
	default:
		return exp.FromArgList(exp.KindApproxQuantile, args)
	}
}

// prestoBuildFromUnixtime ports _build_from_unixtime (parsers/presto.py:35-45).
func prestoBuildFromUnixtime(args []exp.Expression) exp.Expression {
	switch len(args) {
	case 3:
		return exp.New(exp.KindUnixToTime, exp.Args{
			"this":    prestoSeqGet(args, 0),
			"hours":   prestoSeqGet(args, 1),
			"minutes": prestoSeqGet(args, 2),
		})
	case 2:
		return exp.New(exp.KindUnixToTime, exp.Args{
			"this": prestoSeqGet(args, 0),
			"zone": prestoSeqGet(args, 1),
		})
	default:
		return exp.FromArgList(exp.KindUnixToTime, args)
	}
}

// prestoBuildFromUtf8 ports FROM_UTF8 -> Decode(this, replace, charset="utf-8")
// (parsers/presto.py:102-104).
func prestoBuildFromUtf8(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindDecode, exp.Args{
		"this":    prestoSeqGet(args, 0),
		"replace": prestoSeqGet(args, 1),
		"charset": exp.LiteralString("utf-8"),
	})
}

// prestoBuildToUtf8 ports TO_UTF8 -> Encode(this, charset="utf-8") (parsers/presto.py:128-130).
func prestoBuildToUtf8(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindEncode, exp.Args{
		"this":    prestoSeqGet(args, 0),
		"charset": exp.LiteralString("utf-8"),
	})
}

// prestoBuildJSONFormat ports JSON_FORMAT -> JSONFormat(this, options, is_json=True)
// (parsers/presto.py:105-107).
func prestoBuildJSONFormat(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindJSONFormat, exp.Args{
		"this":    prestoSeqGet(args, 0),
		"options": prestoSeqGet(args, 1),
		"is_json": true,
	})
}

// prestoBuildSHA256/prestoBuildSHA512 port SHA256/SHA512 -> SHA2Digest(this, length=256|512)
// (parsers/presto.py:132-135).
func prestoBuildSHA256(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindSHA2Digest, exp.Args{
		"this":   prestoSeqGet(args, 0),
		"length": exp.LiteralNumber(256),
	})
}

func prestoBuildSHA512(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindSHA2Digest, exp.Args{
		"this":   prestoSeqGet(args, 0),
		"length": exp.LiteralNumber(512),
	})
}

// prestoBuildDateAdd/prestoBuildDateDiff port the unit/expression/this argument reorder in
// parsers/presto.py:85-90 (Presto spells DATE_ADD(unit, value, ts), sqlglot's DateAdd/DateDiff
// carry {this, expression, unit}).
// prestoBuildRegexpReplace ports parsers/presto.py:112-116 (replacement defaults to ”).
func prestoBuildRegexpReplace(args []exp.Expression) exp.Expression {
	replacement := prestoSeqGet(args, 2)
	if replacement == nil {
		replacement = exp.LiteralString("")
	}
	return exp.New(exp.KindRegexpReplace, exp.Args{"this": prestoSeqGet(args, 0), "expression": prestoSeqGet(args, 1), "replacement": replacement})
}

func prestoBuildDateAdd(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindDateAdd, exp.Args{
		"this":       prestoSeqGet(args, 2),
		"expression": prestoSeqGet(args, 1),
		"unit":       prestoSeqGet(args, 0),
	})
}

func prestoBuildDateDiff(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindDateDiff, exp.Args{
		"this":       prestoSeqGet(args, 2),
		"expression": prestoSeqGet(args, 1),
		"unit":       prestoSeqGet(args, 0),
	})
}

// prestoBuildElementAt ports ELEMENT_AT -> Bracket(this, [expr], offset=1, safe=True)
// (parsers/presto.py:97-99). offset/safe are stored as the plain int/bool upstream carries.
func prestoBuildElementAt(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindBracket, exp.Args{
		"this":        prestoSeqGet(args, 0),
		"expressions": []exp.Expression{prestoSeqGet(args, 1)},
		"offset":      1,
		"safe":        true,
	})
}

// prestoBuildStrpos ports STRPOS -> StrPosition(this, substr, occurrence)
// (parsers/presto.py:122-124).
func prestoBuildStrpos(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindStrPosition, exp.Args{
		"this":       prestoSeqGet(args, 0),
		"substr":     prestoSeqGet(args, 1),
		"occurrence": prestoSeqGet(args, 2),
	})
}

// prestoBuildReplace ports build_replace_with_optional_replacement (dialects/dialect.py:2486-2491,
// wired at parsers/presto.py:117): REPLACE(this, expression[, replacement]) with an empty-string
// default replacement.
func prestoBuildReplace(args []exp.Expression) exp.Expression {
	replacement := prestoSeqGet(args, 2)
	if replacement == nil {
		replacement = exp.LiteralString("")
	}
	return exp.New(exp.KindReplace, exp.Args{
		"this":        prestoSeqGet(args, 0),
		"expression":  prestoSeqGet(args, 1),
		"replacement": replacement,
	})
}

var prestoTimeMapping = map[string]string{
	"%M": "%B", "%c": "%-m", "%e": "%-d", "%h": "%I", "%i": "%M", "%s": "%S",
	"%u": "%W", "%k": "%-H", "%l": "%-I", "%T": "%H:%M:%S", "%W": "%A",
}

var prestoTeradataTimeMapping = map[string]string{
	"YY": "%y", "Y4": "%Y", "YYYY": "%Y", "M4": "%B", "M3": "%b", "M": "%-M", "MI": "%M",
	"MM": "%m", "MMM": "%b", "MMMM": "%B", "D": "%-d", "DD": "%d", "D3": "%j", "DDD": "%j",
	"H": "%-H", "HH": "%H", "HH24": "%H", "S": "%-S", "SS": "%S", "SSSSSS": "%f",
	"E": "%a", "EE": "%a", "E3": "%a", "E4": "%A", "EEE": "%a", "EEEE": "%A",
}

type prestoTimeTrieNode struct {
	children map[rune]*prestoTimeTrieNode
	exists   bool
}

func prestoTimeTrie(mapping map[string]string) *prestoTimeTrieNode {
	root := &prestoTimeTrieNode{children: map[rune]*prestoTimeTrieNode{}}
	for pattern := range mapping {
		current := root
		for _, char := range pattern {
			if current.children[char] == nil {
				current.children[char] = &prestoTimeTrieNode{children: map[rune]*prestoTimeTrieNode{}}
			}
			current = current.children[char]
		}
		current.exists = true
	}
	return root
}

var prestoForwardTimeTrie = prestoTimeTrie(prestoTimeMapping)
var prestoTeradataTimeTrie = prestoTimeTrie(prestoTeradataTimeMapping)
var prestoInverseTimeMapping = func() map[string]string {
	inverse := map[string]string{}
	for key, value := range prestoTimeMapping {
		inverse[value] = key
	}
	return inverse
}()
var prestoInverseTimeTrie = prestoTimeTrie(prestoInverseTimeMapping)

// PrestoFormatTime ports Generator.format_time with Presto's inverse TIME_MAPPING.
func PrestoFormatTime(format string) string {
	converted, _ := prestoConvertTimeFormatWith(format, prestoInverseTimeMapping, prestoInverseTimeTrie)
	return converted
}

const PrestoTimeFormat = "%Y-%m-%d %T"

func prestoBuildFormattedTime(kind exp.Kind, teradata bool) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		format := prestoSeqGet(args, 1)
		if format != nil && format.IsString() {
			value := format.Name()
			mapping, trie := prestoTimeMapping, prestoForwardTimeTrie
			if teradata {
				value = strings.ToUpper(value)
				mapping, trie = prestoTeradataTimeMapping, prestoTeradataTimeTrie
			}
			converted, ok := prestoConvertTimeFormatWith(value, mapping, trie)
			if !ok {
				converted = "None"
			}
			format = exp.LiteralString(converted)
		}
		return exp.New(kind, exp.Args{"this": prestoSeqGet(args, 0), "format": format})
	}
}

// prestoBuildDateTrunc ports date_trunc_to_time (dialects/dialect.py:1682-1688): a DATE-typed
// cast argument builds DateTrunc, anything else TimestampTrunc.
func prestoBuildDateTrunc(args []exp.Expression) exp.Expression {
	unit := prestoSeqGet(args, 0)
	this := prestoSeqGet(args, 1)
	if this != nil && (this.Kind() == exp.KindCast || this.Kind() == exp.KindTryCast) && exp.DataTypeIsType(this, false, exp.DTypeDate) {
		return exp.DateTrunc(exp.Args{"unit": unit, "this": this})
	}
	return exp.New(exp.KindTimestampTrunc, exp.Args{"this": this, "unit": exp.TimeUnitVar(unit)})
}

func prestoBuildRegexpExtract(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		group := prestoSeqGet(args, 2)
		if group == nil {
			group = exp.LiteralNumber("0")
		}
		values := exp.Args{"this": prestoSeqGet(args, 0), "expression": prestoSeqGet(args, 1), "group": group, "parameters": prestoSeqGet(args, 3)}
		if kind == exp.KindRegexpExtract {
			values["null_if_pos_overflow"] = true
		}
		return exp.New(kind, values)
	}
}

// time.py:10-62 preserves the longest completed format match.
func prestoConvertTimeFormatWith(value string, mapping map[string]string, trie *prestoTimeTrieNode) (string, bool) {
	if value == "" {
		return "", false
	}

	characters := []rune(value)
	start, end := 0, 1
	current := trie
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
			current = trie
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
		if replacement, ok := mapping[chunk]; ok {
			converted.WriteString(replacement)
		} else {
			converted.WriteString(chunk)
		}
	}
	return converted.String(), true
}
