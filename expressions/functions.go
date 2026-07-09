package expressions

func FromArgList(kind Kind, args []Expression) Expression {
	specs := argTypesFor(kind)
	m := Args{}
	if len(specs) == 0 {
		return newNode(kind, m)
	}
	if varLenArgs[kind] {
		nNon := len(specs) - 1
		for i := 0; i < nNon && i < len(args); i++ {
			m[specs[i].Key] = args[i]
		}
		rest := []Expression{}
		if nNon <= len(args) {
			rest = args[nNon:]
		}
		m[specs[nNon].Key] = rest
	} else {
		for i, spec := range specs {
			if i < len(args) {
				m[spec.Key] = args[i]
			}
		}
	}
	return newNode(kind, m)
}

var FunctionByName = map[string]func([]Expression) Expression{
	"ARRAY": func(args []Expression) Expression {
		return newNode(KindArray, Args{"expressions": args})
	},
	"ABS":          genericFunction(KindAbs),
	"AVG":          genericFunction(KindAvg),
	"SUM":          genericFunction(KindSum),
	"SQRT":         genericFunction(KindSqrt),
	"LN":           genericFunction(KindLn),
	"EXP":          genericFunction(KindExp),
	"MIN":          genericFunction(KindMin),
	"MAX":          genericFunction(KindMax),
	"ROUND":        genericFunction(KindRound),
	"LOG":          genericFunction(KindLog),
	"POW":          genericFunction(KindPow),
	"POWER":        genericFunction(KindPow),
	"SUBSTR":       genericFunction(KindSubstring),
	"CEILING":      genericFunction(KindCeil),
	"GROUP_CONCAT": genericFunction(KindGroupConcat),
	// LISTAGG is deliberately NOT registered here: upstream only maps it in Oracle/Snowflake/
	// etc.'s FUNCTIONS (never base's), so base LISTAGG(x) must fall through to Anonymous and
	// round-trip verbatim (parity_gaps.txt:10,11 - WITHIN GROUP attaches generically via the
	// same postfix parseWindow path PERCENTILE_CONT already uses).
	"STDDEV":                genericFunction(KindStddev),
	"STDEV":                 genericFunction(KindStddev),
	"STDDEV_POP":            genericFunction(KindStddevPop),
	"STDDEV_SAMP":           genericFunction(KindStddevSamp),
	"VARIANCE":              genericFunction(KindVariance),
	"VARIANCE_SAMP":         genericFunction(KindVariance),
	"VAR_SAMP":              genericFunction(KindVariance),
	"VARIANCE_POP":          genericFunction(KindVariancePop),
	"VAR_POP":               genericFunction(KindVariancePop),
	"DAY":                   genericFunction(KindDay),
	"MONTH":                 genericFunction(KindMonth),
	"YEAR":                  genericFunction(KindYear),
	"QUARTER":               genericFunction(KindQuarter),
	"APPROX_DISTINCT":       genericFunction(KindApproxDistinct),
	"APPROX_COUNT_DISTINCT": genericFunction(KindApproxDistinct),
	"HLL":                   genericFunction(KindHll),
	"COUNT_IF":              genericFunction(KindCountIf),
	"COUNTIF":               genericFunction(KindCountIf),
	"QUANTILE":              genericFunction(KindQuantile),
	"ARRAY_AGG":             genericFunction(KindArrayAgg),
	"ARRAY_SIZE":            genericFunction(KindArraySize),
	"ARRAY_LENGTH":          genericFunction(KindArraySize),
	"ARRAY_CONTAINS":        genericFunction(KindArrayContains),
	"ARRAY_HAS":             genericFunction(KindArrayContains),
	"INITCAP":               genericFunction(KindInitcap),
	"SPLIT":                 genericFunction(KindSplit),
	"REGEXP_LIKE":           genericFunction(KindRegexpLike),
	"RLIKE":                 genericFunction(KindRegexpLike),
	"REGEXP_SPLIT":          genericFunction(KindRegexpSplit),
	"STRUCT_EXTRACT":        genericFunction(KindStructExtract),
	"STANDARD_HASH":         genericFunction(KindStandardHash),
	"HEX":                   genericFunction(KindHex),
	"MD5":                   genericFunction(KindMD5),
	"ST_POINT":              genericFunction(KindStPoint),
	"ST_MAKEPOINT":          genericFunction(KindStPoint),
	"ST_DISTANCE":           genericFunction(KindStDistance),
	"GENERATE_SERIES":       genericFunction(KindGenerateSeries),
	"DATE":                  genericFunction(KindDate),
	"ADD_MONTHS":            genericFunction(KindAddMonths),
	"DATE_ADD":              genericFunction(KindDateAdd),
	"DATEADD":               genericFunction(KindDateAdd),
	"DATEDIFF":              genericFunction(KindDateDiff),
	"DATE_DIFF":             genericFunction(KindDateDiff),
	"JSON_EXTRACT":          jsonExtractFunction(KindJSONExtract),
	"JSON_EXTRACT_SCALAR":   jsonExtractFunction(KindJSONExtractScalar),
	// REPLACE(this, expression[, replacement]) (string.py:113); base has no replace_sql, so
	// it renders via functionFallbackSQL like any other unadorned Func (see KindInitcap).
	"REPLACE": genericFunction(KindReplace),
	// LENGTH/LEN/CHAR_LENGTH/CHARACTER_LENGTH are deliberately NOT registered as a single Kind:
	// upstream keeps them distinct per dialect (MySQL LENGTH=binary/byte-length vs
	// CHAR_LENGTH=char-length, parsers/mysql.py:127 + generators/mysql.py:179), which needs
	// per-dialect FUNCTIONS parse-side canonicalization (ROADMAP 5b, deferred). One global Kind
	// would render MySQL CHAR_LENGTH as LENGTH - a semantic (byte vs char) regression - so these
	// fall through to Anonymous and round-trip verbatim until 5b lands.
	// LogicalOr._sql_names (aggregate.py:172): all three spellings parse to the same Kind;
	// see generator/aggregate.go logicalOrSQL for the postgres/mysql renames.
	"LOGICAL_OR": genericFunction(KindLogicalOr),
	"BOOL_OR":    genericFunction(KindLogicalOr),
	"BOOLOR_AGG": genericFunction(KindLogicalOr),
	"COUNT": func(args []Expression) Expression {
		m := Args{"big_int": true}
		if len(args) > 0 {
			m["this"] = args[0]
		}
		if len(args) > 1 {
			m["expressions"] = args[1:]
		}
		return newNode(KindCount, m)
	},
	// Base dialect maps COALESCE/IFNULL/NVL to the same builder with is_nvl unset
	// (parser.py:329); only Oracle-family dialects set is_nvl=True (deferred).
	"COALESCE": coalesceFunction(),
	"IFNULL":   coalesceFunction(),
	"NVL":      coalesceFunction(),
	"GREATEST": func(args []Expression) Expression {
		m := Args{"ignore_nulls": true}
		if len(args) > 0 {
			m["this"] = args[0]
		}
		if len(args) > 1 {
			m["expressions"] = args[1:]
		}
		return newNode(KindGreatest, m)
	},
	"LEAST": func(args []Expression) Expression {
		m := Args{"ignore_nulls": true}
		if len(args) > 0 {
			m["this"] = args[0]
		}
		if len(args) > 1 {
			m["expressions"] = args[1:]
		}
		return newNode(KindLeast, m)
	},
	// STR_TO_*/TIME_STR_TO_* (temporal.py:452-473): all six register generically here (no
	// per-dialect FUNCTIONS override upstream), parsing for free via FromArgList/genericFunction
	// since each Kind's argTypes row (kinds.go) already mirrors its arg_types dict key order.
	"STR_TO_DATE":      genericFunction(KindStrToDate),
	"STR_TO_TIME":      genericFunction(KindStrToTime),
	"STR_TO_UNIX":      genericFunction(KindStrToUnix),
	"TIME_STR_TO_DATE": genericFunction(KindTimeStrToDate),
	"TIME_STR_TO_TIME": genericFunction(KindTimeStrToTime),
	"TIME_STR_TO_UNIX": genericFunction(KindTimeStrToUnix),
}

func genericFunction(kind Kind) func([]Expression) Expression {
	return func(args []Expression) Expression { return FromArgList(kind, args) }
}

func jsonExtractFunction(kind Kind) func([]Expression) Expression {
	return func(args []Expression) Expression {
		m := Args{}
		if len(args) > 0 {
			m["this"] = args[0]
		}
		if len(args) > 1 {
			m["expression"] = args[1]
		}
		// build_extract_json_with_path (parser.py:104-118) preserves the 3rd+ path arguments
		// only for JSONExtract, not JSONExtractScalar (scalar keeps just this + expression), so
		// e.g. JSON_EXTRACT_SCALAR(a, b, c) drops `c`.
		if len(args) > 2 && kind == KindJSONExtract {
			m["expressions"] = args[2:]
		}
		return newNode(kind, m)
	}
}

func coalesceFunction() func([]Expression) Expression {
	return func(args []Expression) Expression {
		m := Args{}
		if len(args) > 0 {
			m["this"] = args[0]
		}
		if len(args) > 1 {
			m["expressions"] = args[1:]
		}
		return newNode(KindCoalesce, m)
	}
}

func Abs(args Args) Expression            { return newNode(KindAbs, args) }
func Avg(args Args) Expression            { return newNode(KindAvg, args) }
func Sum(args Args) Expression            { return newNode(KindSum, args) }
func Sqrt(args Args) Expression           { return newNode(KindSqrt, args) }
func Ln(args Args) Expression             { return newNode(KindLn, args) }
func Exp(args Args) Expression            { return newNode(KindExp, args) }
func Min(args Args) Expression            { return newNode(KindMin, args) }
func Max(args Args) Expression            { return newNode(KindMax, args) }
func Round(args Args) Expression          { return newNode(KindRound, args) }
func Log(args Args) Expression            { return newNode(KindLog, args) }
func Pow(args Args) Expression            { return newNode(KindPow, args) }
func Stddev(args Args) Expression         { return newNode(KindStddev, args) }
func StddevPop(args Args) Expression      { return newNode(KindStddevPop, args) }
func StddevSamp(args Args) Expression     { return newNode(KindStddevSamp, args) }
func Variance(args Args) Expression       { return newNode(KindVariance, args) }
func VariancePop(args Args) Expression    { return newNode(KindVariancePop, args) }
func Day(args Args) Expression            { return newNode(KindDay, args) }
func Month(args Args) Expression          { return newNode(KindMonth, args) }
func Year(args Args) Expression           { return newNode(KindYear, args) }
func Quarter(args Args) Expression        { return newNode(KindQuarter, args) }
func ApproxDistinct(args Args) Expression { return newNode(KindApproxDistinct, args) }
func Hll(args Args) Expression            { return newNode(KindHll, args) }
func CountIf(args Args) Expression        { return newNode(KindCountIf, args) }
func Quantile(args Args) Expression       { return newNode(KindQuantile, args) }
func Count(args Args) Expression          { return newNode(KindCount, args) }
func Coalesce(args Args) Expression       { return newNode(KindCoalesce, args) }
func Greatest(args Args) Expression       { return newNode(KindGreatest, args) }
func Least(args Args) Expression          { return newNode(KindLeast, args) }
func Cast(args Args) Expression           { return newNode(KindCast, args) }
func TryCast(args Args) Expression        { return newNode(KindTryCast, args) }
func CastToStrType(args Args) Expression  { return newNode(KindCastToStrType, args) }
func Extract(args Args) Expression        { return newNode(KindExtract, args) }
func StrPosition(args Args) Expression    { return newNode(KindStrPosition, args) }
func Substring(args Args) Expression      { return newNode(KindSubstring, args) }
func Trim(args Args) Expression           { return newNode(KindTrim, args) }
func Ceil(args Args) Expression           { return newNode(KindCeil, args) }
func Floor(args Args) Expression          { return newNode(KindFloor, args) }
func GroupConcat(args Args) Expression    { return newNode(KindGroupConcat, args) }
func Replace(args Args) Expression        { return newNode(KindReplace, args) }
func LogicalOr(args Args) Expression      { return newNode(KindLogicalOr, args) }
func ArrayAgg(args Args) Expression       { return newNode(KindArrayAgg, args) }
func ArraySize(args Args) Expression      { return newNode(KindArraySize, args) }
func ArrayContains(args Args) Expression  { return newNode(KindArrayContains, args) }
func Initcap(args Args) Expression        { return newNode(KindInitcap, args) }
func Split(args Args) Expression          { return newNode(KindSplit, args) }
func RegexpLike(args Args) Expression     { return newNode(KindRegexpLike, args) }
func RegexpSplit(args Args) Expression    { return newNode(KindRegexpSplit, args) }
func StructExtract(args Args) Expression  { return newNode(KindStructExtract, args) }
func StandardHash(args Args) Expression   { return newNode(KindStandardHash, args) }
func Hex(args Args) Expression            { return newNode(KindHex, args) }
func MD5(args Args) Expression            { return newNode(KindMD5, args) }
func StPoint(args Args) Expression        { return newNode(KindStPoint, args) }
func StDistance(args Args) Expression     { return newNode(KindStDistance, args) }
func GenerateSeries(args Args) Expression { return newNode(KindGenerateSeries, args) }
func Date(args Args) Expression           { return newNode(KindDate, args) }
func AddMonths(args Args) Expression      { return newNode(KindAddMonths, args) }
func DateAdd(args Args) Expression        { return newNode(KindDateAdd, args) }
func DateDiff(args Args) Expression       { return newNode(KindDateDiff, args) }
