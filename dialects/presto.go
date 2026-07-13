package dialects

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// Presto ports the Presto/Trino dialect (dialects/presto.py). This slice covers the parser +
// tokenizer only (the class flags, the Tokenizer config, and the FUNCTIONS overlay) - the
// generator TRANSFORMS/TYPE_MAPPING and typing.EXPRESSION_METADATA are deliberately out of
// scope (see ROADMAP known-divergences), so structured functions whose canonical class-name
// differs from the Presto spelling round-trip to that canonical name.
func Presto() *Dialect {
	d := Base()
	d.Name = "presto"
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
	// dialects/presto.py:35 NORMALIZATION_STRATEGY = NormalizationStrategy.CASE_INSENSITIVE;
	// first in-scope consumer of dialects.CaseInsensitive.
	d.NormalizationStrategy = CaseInsensitive
	// parsers/presto.py:60 VALUES_FOLLOWED_BY_PAREN = False (mysql.go:41-42 precedent).
	d.ValuesFollowedByParen = false
	// parsers/presto.py:61 ZONE_AWARE_TIMESTAMP_CONSTRUCTOR = True (read at parser.py:6186-6191).
	d.ZoneAwareTimestampConstructor = true

	// parsers/presto.py:74-135 FUNCTIONS overlay, ported 1:1 with the slice policy applied: the
	// helper-dependent entries DATE_FORMAT/DATE_PARSE/DATE_TRUNC/TO_CHAR (need build_formatted_time
	// + TIME_MAPPING / date_trunc_to_time) and REGEXP_EXTRACT/REGEXP_EXTRACT_ALL/REGEXP_REPLACE
	// (need build_regexp_extract + REGEXP_EXTRACT_DEFAULT_GROUP; the injected default-group arg
	// diverges round-trip) are deferred - they stay Anonymous and are intentionally NOT in this
	// overlay (ROADMAP known-divergences). Everything else registers via exp.FromArgListFunc or a
	// custom closure below. NOW -> CurrentTimestamp is registered here only for the parenthesized
	// NOW() call form; it is deliberately NOT added to the parser's global no-paren-function map
	// (bare NOW stays a lineage-safe column), so base floors are unaffected.
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
		"WEEK":                 exp.FromArgListFunc(exp.KindWeekOfYear),
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

// prestoBuildSHA256/prestoBuildSHA512 port SHA256/SHA512 -> SHA2(this, length=256|512)
// (parsers/presto.py:132-133).
func prestoBuildSHA256(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindSHA2, exp.Args{
		"this":   prestoSeqGet(args, 0),
		"length": exp.LiteralNumber(256),
	})
}

func prestoBuildSHA512(args []exp.Expression) exp.Expression {
	return exp.New(exp.KindSHA2, exp.Args{
		"this":   prestoSeqGet(args, 0),
		"length": exp.LiteralNumber(512),
	})
}

// prestoBuildDateAdd/prestoBuildDateDiff port the unit/expression/this argument reorder in
// parsers/presto.py:85-90 (Presto spells DATE_ADD(unit, value, ts), sqlglot's DateAdd/DateDiff
// carry {this, expression, unit}).
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
