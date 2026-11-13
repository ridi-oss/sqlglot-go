package dialects

import (
	"regexp/syntax"
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// dialects/bigquery.py:56-76.
var (
	bigqueryTimeMapping = map[string]string{
		"%x": "%m/%d/%y", "%D": "%m/%d/%y", "%E6S": "%S.%f", "%e": "%-d", "%F": "%Y-%m-%d",
		"%T": "%H:%M:%S", "%c": "%a %b %e %H:%M:%S %Y",
	}
	// INVERSE_TIME_MAPPING as computed by the Dialect metaclass (dialect.py:265-271).
	bigqueryInverseTimeMapping = map[string]string{
		"%m/%d/%y": "%D", "%S.%f": "%E6S", "%-d": "%e", "%Y-%m-%d": "%F", "%H:%M:%S": "%T",
		"%a %b %e %H:%M:%S %Y": "%c", "%H:%M:%S.%f": "%H:%M:%E6S",
		"%mstrict": "%m", "%dstrict": "%d", "%Hstrict": "%H", "%Istrict": "%I", "%Mstrict": "%M", "%Sstrict": "%S",
	}
	bigqueryFormatMapping = map[string]string{
		"dd": "%d", "DD": "%d", "mm": "%m", "MM": "%m", "mon": "%b", "MON": "%b", "month": "%B",
		"MONTH": "%B", "yyyy": "%Y", "YYYY": "%Y", "yy": "%y", "YY": "%y", "HH": "%I", "HH12": "%I",
		"hh24": "%H", "HH24": "%H", "mi": "%M", "MI": "%M", "ss": "%S", "SS": "%S", "SSSSS": "%f",
		"tzh": "%z", "TZH": "%z",
	}
	bigqueryInverseFormatMapping = map[string]string{
		"%d": "DD", "%m": "MM", "%b": "MON", "%B": "MONTH", "%Y": "YYYY", "%y": "YY", "%I": "HH12",
		"%H": "HH24", "%M": "MI", "%S": "SS", "%f": "SSSSS", "%z": "TZH",
		"%mstrict": "MM", "%dstrict": "DD", "%Hstrict": "HH24", "%Istrict": "HH12", "%Mstrict": "MI", "%Sstrict": "SS",
	}
	bigqueryTimeTrie          = prestoTimeTrie(bigqueryTimeMapping)
	bigqueryInverseTimeTrie   = prestoTimeTrie(bigqueryInverseTimeMapping)
	bigqueryFormatTrie        = prestoTimeTrie(bigqueryFormatMapping)
	bigqueryInverseFormatTrie = prestoTimeTrie(bigqueryInverseFormatMapping)
)

// BigQueryFormatTime ports Generator.format_time with BigQuery's INVERSE_TIME_MAPPING, or with
// INVERSE_FORMAT_MAPPING when cast is set (the CAST ... FORMAT element syntax).
func BigQueryFormatTime(format string, cast bool) string {
	mapping, trie := bigqueryInverseTimeMapping, bigqueryInverseTimeTrie
	if cast {
		mapping, trie = bigqueryInverseFormatMapping, bigqueryInverseFormatTrie
	}
	converted, _ := prestoConvertTimeFormatWith(format, mapping, trie)
	return converted
}

// BigQueryParseFormat ports Dialect.format_time with FORMAT_MAPPING (dialect.py:1014-1025 via
// the FORMAT_TRIE parse path of _parse_cast).
func BigQueryParseFormat(format string) string {
	converted, _ := prestoConvertTimeFormatWith(format, bigqueryFormatMapping, bigqueryFormatTrie)
	return converted
}

func bigqueryFormatTime(format exp.Expression) exp.Expression {
	if format != nil && format.IsString() {
		converted, _ := prestoConvertTimeFormatWith(format.Name(), bigqueryTimeMapping, bigqueryTimeTrie)
		return exp.LiteralString(converted)
	}
	return format
}

// buildFormattedTime ports build_formatted_time (dialect.py:1592-1619) for BigQuery.
func bigqueryBuildFormattedTime(kind exp.Kind, args []exp.Expression) exp.Expression {
	return exp.New(kind, exp.Args{"this": seqGet(args, 0), "format": bigqueryFormatTime(seqGet(args, 1))})
}

func seqGet(args []exp.Expression, i int) exp.Expression { return prestoSeqGet(args, i) }

func fromArgList(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return exp.FromArgListFunc(kind)
}

// parsers/bigquery.py:31-37.
func bigqueryNormalizeBareWeek(e exp.Expression) exp.Expression {
	unit := asExpr(e.Arg("unit"))
	if unit != nil && (unit.Kind() == exp.KindLiteral || unit.Kind() == exp.KindVar) && strings.ToUpper(unit.Name()) == "WEEK" {
		e.Set("unit", exp.New(exp.KindWeekStart, exp.Args{"this": exp.Var(exp.Args{"this": "SUNDAY"})}))
	}
	return e
}

// bigqueryDateDeltaWithInterval ports build_date_delta_with_interval (dialect.py:1658-1679).
func bigqueryDateDeltaWithInterval(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		if len(args) < 2 {
			return nil
		}
		interval := args[1]
		if interval == nil || interval.Kind() != exp.KindInterval {
			// dialect.py:1670 raises ParseError; a nil result fails the parse the same way.
			return nil
		}
		unit := exp.LiteralString("DAY")
		if u := asExpr(interval.Arg("unit")); u != nil {
			if u.Kind() == exp.KindVar || u.Kind() == exp.KindLiteral {
				unit = exp.LiteralString(u.Name())
			} else {
				unit = u
			}
		}
		return exp.New(kind, exp.Args{"this": args[0], "expression": interval.Arg("this"), "unit": unit})
	}
}

func bigqueryDateDiff(kind exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		return bigqueryNormalizeBareWeek(exp.New(kind, exp.Args{
			"this": seqGet(args, 0), "expression": seqGet(args, 1), "unit": seqGet(args, 2), "date_part_boundary": true,
		}))
	}
}

func bigqueryExtractJSON(kind exp.Kind, defaultPath bool) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		if defaultPath && len(args) == 1 {
			args = append(args, exp.LiteralString("$"))
		}
		e := exp.New(kind, exp.Args{"this": seqGet(args, 0), "expression": seqGet(args, 1)})
		if len(args) > 2 && kind == exp.KindJSONExtract {
			e.Set("expressions", args[2:])
		}
		if kind == exp.KindJSONExtractScalar {
			e.Set("scalar_only", true)
		}
		return e
	}
}

func bigqueryFormatTimeBuilder(inner exp.Kind) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		e := bigqueryBuildFormattedTime(exp.KindTimeToStr, []exp.Expression{exp.New(inner, exp.Args{"this": seqGet(args, 1)}), seqGet(args, 0)})
		e.Set("zone", seqGet(args, 2))
		return e
	}
}

// parsers/bigquery.py:124-144.
func bigqueryRegexpExtract(kind exp.Kind, defaultGroup exp.Expression) func([]exp.Expression) exp.Expression {
	return func(args []exp.Expression) exp.Expression {
		group := defaultGroup
		if pattern := seqGet(args, 1); pattern != nil {
			if re, err := syntax.Parse(pattern.Name(), syntax.Perl); err == nil && re.MaxCap() == 1 {
				group = exp.LiteralNumber("1")
			}
		}
		values := exp.Args{
			"this": seqGet(args, 0), "expression": seqGet(args, 1), "position": seqGet(args, 2),
			"occurrence": seqGet(args, 3), "group": group,
		}
		if kind == exp.KindRegexpExtract {
			values["null_if_pos_overflow"] = false
		}
		return exp.New(kind, values)
	}
}

// bigqueryFunctions ports parsers/bigquery.py:224-323 FUNCTIONS.
func bigqueryFunctions() map[string]func([]exp.Expression) exp.Expression {
	return map[string]func([]exp.Expression) exp.Expression{
		// parsers/bigquery.py:225 drops SEARCH from the base table.
		"SEARCH":           nil,
		"APPROX_TOP_COUNT": fromArgList(exp.KindApproxTopK),
		"BIT_AND":          fromArgList(exp.KindBitwiseAndAgg),
		"BIT_OR":           fromArgList(exp.KindBitwiseOrAgg),
		"BIT_XOR":          fromArgList(exp.KindBitwiseXorAgg),
		"BIT_COUNT":        fromArgList(exp.KindBitwiseCount),
		"BOOL":             fromArgList(exp.KindJSONBool),
		"CONTAINS_SUBSTR": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindContains, exp.Args{
				"this":       exp.New(exp.KindLower, exp.Args{"this": seqGet(args, 0)}),
				"expression": exp.New(exp.KindLower, exp.Args{"this": seqGet(args, 1)}),
				"json_scope": seqGet(args, 2),
			})
		},
		"DATE": func(args []exp.Expression) exp.Expression {
			if len(args) == 3 {
				return exp.FromArgList(exp.KindDateFromParts, args)
			}
			return exp.FromArgList(exp.KindDate, args)
		},
		"DATE_ADD":  bigqueryDateDeltaWithInterval(exp.KindDateAdd),
		"DATE_DIFF": bigqueryDateDiff(exp.KindDateDiff),
		"DATE_SUB":  bigqueryDateDeltaWithInterval(exp.KindDateSub),
		"DATE_TRUNC": func(args []exp.Expression) exp.Expression {
			return bigqueryNormalizeBareWeek(exp.DateTrunc(exp.Args{"unit": seqGet(args, 1), "this": seqGet(args, 0), "zone": seqGet(args, 2)}))
		},
		"DATETIME": func(args []exp.Expression) exp.Expression {
			switch len(args) {
			case 1:
				return exp.FromArgList(exp.KindTsOrDsToDatetime, args)
			case 2:
				return exp.FromArgList(exp.KindDatetime, args)
			}
			return exp.FromArgList(exp.KindTimestampFromParts, args)
		},
		"DATETIME_ADD":  bigqueryDateDeltaWithInterval(exp.KindDatetimeAdd),
		"DATETIME_DIFF": bigqueryDateDiff(exp.KindDatetimeDiff),
		"DATETIME_SUB":  bigqueryDateDeltaWithInterval(exp.KindDatetimeSub),
		"DATETIME_TRUNC": func(args []exp.Expression) exp.Expression {
			return bigqueryNormalizeBareWeek(exp.FromArgList(exp.KindDatetimeTrunc, args))
		},
		"DIV":            prestoBinaryFromFunction(exp.KindIntDiv),
		"EDIT_DISTANCE":  bigqueryBuildLevenshtein,
		"EMBED":          fromArgList(exp.KindAIEmbed),
		"FORMAT_DATE":    bigqueryFormatTimeBuilder(exp.KindTsOrDsToDate),
		"GENERATE":       fromArgList(exp.KindAIGenerate),
		"GENERATE_ARRAY": fromArgList(exp.KindGenerateSeries),
		// JSON paths stay raw literals: this port does not parse JSON paths (Dialect.to_json_path).
		"JSON_EXTRACT_SCALAR":       bigqueryExtractJSON(exp.KindJSONExtractScalar, true),
		"JSON_EXTRACT_ARRAY":        bigqueryExtractJSON(exp.KindJSONExtractArray, true),
		"JSON_EXTRACT_STRING_ARRAY": bigqueryExtractJSON(exp.KindJSONValueArray, true),
		"JSON_KEYS":                 fromArgList(exp.KindJSONKeysAtDepth),
		"JSON_QUERY":                bigqueryExtractJSON(exp.KindJSONExtract, false),
		"JSON_QUERY_ARRAY":          bigqueryExtractJSON(exp.KindJSONExtractArray, true),
		"JSON_STRIP_NULLS":          bigqueryBuildJSONStripNulls,
		"JSON_VALUE":                bigqueryExtractJSON(exp.KindJSONExtractScalar, true),
		"JSON_VALUE_ARRAY":          bigqueryExtractJSON(exp.KindJSONValueArray, true),
		"LAST_DAY": func(args []exp.Expression) exp.Expression {
			return bigqueryNormalizeBareWeek(exp.FromArgList(exp.KindLastDay, args))
		},
		// build_logarithm (parser.py:78-88) with LOG_BASE_FIRST = False (dialects/bigquery.py:26).
		"LOG": func(args []exp.Expression) exp.Expression {
			if len(args) > 1 {
				return exp.FromArgList(exp.KindLog, []exp.Expression{args[1], args[0]})
			}
			return exp.FromArgList(exp.KindLn, args)
		},
		"LENGTH": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindLength, exp.Args{"this": seqGet(args, 0), "binary": true})
		},
		"MD5":  fromArgList(exp.KindMD5Digest),
		"SHA1": fromArgList(exp.KindSHA1Digest),
		"NORMALIZE_AND_CASEFOLD": func(args []exp.Expression) exp.Expression {
			// The form is a keyword (NFKC), not a column; upstream keeps it a Column here.
			form := seqGet(args, 1)
			if form != nil && form.Kind() == exp.KindColumn && len(form.Parts()) == 1 {
				form = exp.Var(exp.Args{"this": form.Name()})
			}
			return exp.New(exp.KindNormalize, exp.Args{"this": seqGet(args, 0), "form": form, "is_casefold": true})
		},
		"OCTET_LENGTH": fromArgList(exp.KindByteLength),
		"TO_HEX": func(args []exp.Expression) exp.Expression {
			arg := seqGet(args, 0)
			if arg != nil && arg.Kind() == exp.KindMD5Digest {
				return exp.New(exp.KindMD5, exp.Args{"this": arg.Arg("this")})
			}
			return exp.New(exp.KindLowerHex, exp.Args{"this": arg})
		},
		"PARSE_DATE": func(args []exp.Expression) exp.Expression {
			e := bigqueryBuildFormattedTime(exp.KindStrToDate, []exp.Expression{seqGet(args, 1), seqGet(args, 0)})
			e.Set("default_year", exp.LiteralNumber("1970"))
			return e
		},
		"PARSE_TIME": func(args []exp.Expression) exp.Expression {
			return bigqueryBuildFormattedTime(exp.KindParseTime, []exp.Expression{seqGet(args, 1), seqGet(args, 0)})
		},
		"PARSE_TIMESTAMP": func(args []exp.Expression) exp.Expression {
			e := bigqueryBuildFormattedTime(exp.KindStrToTime, []exp.Expression{seqGet(args, 1), seqGet(args, 0)})
			e.Set("zone", seqGet(args, 2))
			e.Set("default_year", exp.LiteralNumber("1970"))
			return e
		},
		"PARSE_DATETIME": func(args []exp.Expression) exp.Expression {
			e := bigqueryBuildFormattedTime(exp.KindParseDatetime, []exp.Expression{seqGet(args, 1), seqGet(args, 0)})
			e.Set("default_year", exp.LiteralNumber("1970"))
			return e
		},
		"REGEXP_CONTAINS":    fromArgList(exp.KindRegexpLike),
		"REGEXP_EXTRACT":     bigqueryRegexpExtract(exp.KindRegexpExtract, nil),
		"REGEXP_SUBSTR":      bigqueryRegexpExtract(exp.KindRegexpExtract, nil),
		"REGEXP_EXTRACT_ALL": bigqueryRegexpExtract(exp.KindRegexpExtractAll, exp.LiteralNumber("0")),
		"SHA256": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindSHA2Digest, exp.Args{"this": seqGet(args, 0), "length": exp.LiteralNumber("256")})
		},
		"SHA512": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindSHA2Digest, exp.Args{"this": seqGet(args, 0), "length": exp.LiteralNumber("512")})
		},
		"SIMILARITY": fromArgList(exp.KindAISimilarity),
		"SPLIT": func(args []exp.Expression) exp.Expression {
			expression := seqGet(args, 1)
			if expression == nil {
				expression = exp.LiteralString(",")
			}
			return exp.New(exp.KindSplit, exp.Args{"this": seqGet(args, 0), "expression": expression})
		},
		"STRPOS": fromArgList(exp.KindStrPosition),
		"TIME": func(args []exp.Expression) exp.Expression {
			switch len(args) {
			case 1:
				return exp.New(exp.KindTsOrDsToTime, exp.Args{"this": args[0]})
			case 2:
				return exp.FromArgList(exp.KindTime, args)
			}
			return exp.FromArgList(exp.KindTimeFromParts, args)
		},
		"TIME_ADD": bigqueryDateDeltaWithInterval(exp.KindTimeAdd),
		"TIME_SUB": bigqueryDateDeltaWithInterval(exp.KindTimeSub),
		"TIMESTAMP": func(args []exp.Expression) exp.Expression {
			e := exp.FromArgList(exp.KindTimestamp, args)
			e.Set("with_tz", true)
			return e
		},
		"TIMESTAMP_ADD": bigqueryDateDeltaWithInterval(exp.KindTimestampAdd),
		"TIMESTAMP_SUB": bigqueryDateDeltaWithInterval(exp.KindTimestampSub),
		"TIMESTAMP_MICROS": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindUnixToTime, exp.Args{"this": seqGet(args, 0), "scale": exp.LiteralNumber("6")})
		},
		"TIMESTAMP_MILLIS": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindUnixToTime, exp.Args{"this": seqGet(args, 0), "scale": exp.LiteralNumber("3")})
		},
		"TIMESTAMP_SECONDS": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindUnixToTime, exp.Args{"this": seqGet(args, 0)})
		},
		"TIMESTAMP_TRUNC": func(args []exp.Expression) exp.Expression {
			return bigqueryNormalizeBareWeek(exp.FromArgList(exp.KindTimestampTrunc, args))
		},
		"TO_JSON": func(args []exp.Expression) exp.Expression {
			return exp.New(exp.KindJSONFormat, exp.Args{"this": seqGet(args, 0), "options": seqGet(args, 1), "to_json": true})
		},
		"TO_JSON_STRING":   fromArgList(exp.KindJSONFormat),
		"FORMAT_DATETIME":  bigqueryFormatTimeBuilder(exp.KindTsOrDsToDatetime),
		"FORMAT_TIMESTAMP": bigqueryFormatTimeBuilder(exp.KindTsOrDsToTimestamp),
		"FORMAT_TIME":      bigqueryFormatTimeBuilder(exp.KindTsOrDsToTime),
		"FROM_HEX":         fromArgList(exp.KindUnhex),
		"WEEK": func(args []exp.Expression) exp.Expression {
			var this exp.Expression
			if a := seqGet(args, 0); a != nil {
				this = exp.Var(exp.Args{"this": a.Name()})
			}
			return exp.New(exp.KindWeekStart, exp.Args{"this": this})
		},
	}
}

// parsers/bigquery.py:86-93.
func bigqueryBuildJSONStripNulls(args []exp.Expression) exp.Expression {
	e := exp.New(exp.KindJSONStripNulls, exp.Args{"this": seqGet(args, 0)})
	for _, arg := range args[1:] {
		if arg != nil && arg.Kind() == exp.KindKwarg {
			e.Set(strings.ToLower(asExpr(arg.Arg("this")).Name()), arg)
		} else {
			e.Set("expression", arg)
		}
	}
	return e
}

// parsers/bigquery.py:96-102.
func bigqueryBuildLevenshtein(args []exp.Expression) exp.Expression {
	var maxDist any
	if m := seqGet(args, 2); m != nil {
		maxDist = m.Arg("expression")
	}
	return exp.New(exp.KindLevenshtein, exp.Args{"this": seqGet(args, 0), "expression": seqGet(args, 1), "max_dist": maxDist})
}

func asExpr(v any) exp.Expression {
	e, _ := v.(exp.Expression)
	return e
}
