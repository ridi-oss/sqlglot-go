package dialects

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
	"strings"
)

// BigQuery ports dialects/bigquery.py, parsers/bigquery.py and generators/bigquery.py.
func BigQuery() *Dialect {
	d := Base()
	d.Name = "bigquery"
	d.IdentifierStart, d.IdentifierEnd = "`", "`"
	// HEX_START/BYTE_START take the first HEX_STRINGS/BYTE_STRINGS tuple (dialects/dialect.py:293).
	d.HexStart, d.HexEnd = "0x", ""
	d.ByteStart, d.ByteEnd = "b'", "'"
	// dialects/bigquery.py:23-50 class attributes.
	d.UnnestColumnOnly = true
	d.SupportsUserDefinedTypes = false
	d.ForceEarlyAliasRefExpansion = true
	d.ExpandOnlyGroupAliasRef = true
	d.HexStringIsIntegerType = true
	d.ByteStringIsBytesType = true
	d.AnnotateAllScopes = true
	d.ProjectionAliasesShadowSourceNames = true
	d.TablesReferenceableAsColumns = true
	d.SupportsStructStarExpansion = true
	d.ExcludesPseudocolumnsFromStar = true
	d.QueryResultsAreStructs = true
	d.SetOpDistinctRequired = true
	d.CastFormatToTime = BigQueryParseFormat
	d.AliasPostVersion = false
	d.NormalizationStrategy = CaseInsensitive
	d.NormalizeFunctions = ""
	d.Pseudocolumns = map[string]bool{
		"_PARTITIONTIME":     true,
		"_PARTITIONDATE":     true,
		"_TABLE_SUFFIX":      true,
		"_FILE_NAME":         true,
		"_DBT_MAX_PARTITION": true,
	}
	// parsers/bigquery.py:190-194.
	d.LogDefaultsToLn = true
	d.JoinsHaveEqualPrecedence = true
	// generators/bigquery.py:252.
	d.IntervalAllowsPluralForm = false
	d.RenameTableWithDB = false
	// generators/bigquery.py:484-581 RESERVED_KEYWORDS.
	d.ReservedKeywords = map[string]bool{}
	for _, w := range strings.Fields(`all and any array as asc assert_rows_modified at between by case cast collate contains create cross cube current default define desc distinct else end enum escape except exclude exists extract false fetch following for from full group grouping groups hash having if ignore in inner intersect interval into is join lateral left like limit lookup merge natural new no not null nulls of on or order outer over partition preceding proto qualify range recursive respect right rollup rows select set some struct tablesample then to treat true unbounded union unnest using when where window with within`) {
		d.ReservedKeywords[w] = true
	}
	d.Functions = bigqueryFunctions()

	cfg := tokens.BaseConfig()
	// dialects/bigquery.py:176-189 Tokenizer delimiters.
	cfg.Quotes = map[string]string{"'": "'", `"`: `"`, `"""`: `"""`, "'''": "'''"}
	cfg.Comments["#"] = ""
	cfg.Identifiers = map[rune]string{'`': "`"}
	cfg.StringEscapes = map[rune]bool{'\\': true}
	cfg.HasHexStrings = true
	for _, q := range []string{"'", `"`, `"""`, "'''"} {
		for _, prefix := range []string{"b", "B"} {
			cfg.FormatStrings[prefix+q] = tokens.FormatString{End: q, TokenType: tokens.BYTE_STRING}
		}
		for _, prefix := range []string{"r", "R"} {
			cfg.FormatStrings[prefix+q] = tokens.FormatString{End: q, TokenType: tokens.RAW_STRING}
		}
	}
	cfg.NestedComments = false
	// dialects/bigquery.py:191-215 keyword overlay.
	for keyword, tokenType := range map[string]tokens.TokenType{
		"ANY TYPE":          tokens.VARIANT,
		"BEGIN":             tokens.COMMAND,
		"BEGIN TRANSACTION": tokens.BEGIN,
		"BYTEINT":           tokens.INT,
		"BYTES":             tokens.BINARY,
		"CURRENT_DATETIME":  tokens.CURRENT_DATETIME,
		"DATETIME":          tokens.TIMESTAMP,
		"DECLARE":           tokens.DECLARE,
		"ELSEIF":            tokens.COMMAND,
		"EXCEPTION":         tokens.COMMAND,
		"EXPORT":            tokens.EXPORT,
		"FLOAT64":           tokens.DOUBLE,
		"LOOP":              tokens.COMMAND,
		"MODEL":             tokens.MODEL,
		"RECORD":            tokens.STRUCT,
		"REPEAT":            tokens.COMMAND,
		"TIMESTAMP":         tokens.TIMESTAMPTZ,
		"WHILE":             tokens.COMMAND,
	} {
		cfg.Keywords[keyword] = tokenType
	}
	delete(cfg.Keywords, "DIV")
	delete(cfg.Keywords, "VALUES")
	delete(cfg.Keywords, "/*+")
	delete(cfg.Comments, "/*+")
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}

// bigqueryNormalizeIdentifier ports BigQuery.normalize_identifier (dialects/bigquery.py:153-174):
// CTEs and columns are case-insensitive, but UDF and table names are case-sensitive. A table is
// detected by being qualified with at least a dataset.
func bigqueryNormalizeIdentifier(e exp.Expression) exp.Expression {
	parent := e.Parent()
	for parent != nil && parent.Kind() == exp.KindDot {
		parent = parent.Parent()
	}
	caseSensitive := metaTrue(e, "is_table")
	if parent != nil {
		switch parent.Kind() {
		case exp.KindUserDefinedFunction:
			caseSensitive = true
		case exp.KindTable:
			if parent.Text("schema") != "" && (metaTrue(parent, "quoted_table") || !metaTrue(parent, "maybe_column")) {
				caseSensitive = true
			}
		}
	}
	if !caseSensitive {
		this, _ := e.Arg("this").(string)
		e.Set("this", asciiLower(this))
	}
	return e
}

func metaTrue(e exp.Expression, key string) bool {
	v, ok := e.MetaGet(key).(bool)
	return ok && v
}
