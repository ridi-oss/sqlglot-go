package tokens

import (
	"strings"

	"github.com/sjincho/sqlglot-go/trie"
)

type FormatString struct {
	End       string
	TokenType TokenType
}

type TokenizerConfig struct {
	SingleTokens  map[rune]TokenType
	Keywords      map[string]TokenType
	Quotes        map[string]string
	FormatStrings map[string]FormatString
	Identifiers   map[rune]string
	Comments      map[string]string
	// LineCommentRequiresSpace holds line-comment starts (keys of Comments) that only
	// begin a comment when immediately followed by whitespace/control or EOF. MySQL
	// requires this for `--` — `1--2` is arithmetic `1 - -2`, not `1` + comment. See
	// DEVIATIONS §1 (upstream mis-tokenizes this).
	LineCommentRequiresSpace         map[string]bool
	StringEscapes                    map[rune]bool
	ByteStringEscapes                map[rune]bool
	IdentifierEscapes                map[rune]bool
	EscapeFollowChars                map[rune]bool
	Commands                         map[TokenType]bool
	CommandPrefixTokens              map[TokenType]bool
	TokensPrecedingHint              map[TokenType]bool
	NumericLiterals                  map[string]string
	VarSingleTokens                  map[rune]bool
	UnescapedSequences               map[string]string
	NestedComments                   bool
	HintStart                        string
	HasBitStrings                    bool
	HasHexStrings                    bool
	StringEscapesAllowedInRawStrings bool
	HeredocTagIsIdentifier           bool
	HeredocStringAlternative         TokenType
	NumbersCanBeUnderscoreSeparated  bool
	NumbersCanHaveDecimals           bool
	IdentifiersCanStartWithDigit     bool
	KeywordTrie                      *trie.Node
}

func BaseConfig() TokenizerConfig {
	cfg := TokenizerConfig{
		SingleTokens: map[rune]TokenType{
			'(':  L_PAREN,
			')':  R_PAREN,
			'[':  L_BRACKET,
			']':  R_BRACKET,
			'{':  L_BRACE,
			'}':  R_BRACE,
			'&':  AMP,
			'^':  CARET,
			':':  COLON,
			',':  COMMA,
			'.':  DOT,
			'-':  DASH,
			'=':  EQ,
			'>':  GT,
			'<':  LT,
			'%':  MOD,
			'!':  NOT,
			'|':  PIPE,
			'+':  PLUS,
			';':  SEMICOLON,
			'/':  SLASH,
			'\\': BACKSLASH,
			'*':  STAR,
			'~':  TILDE,
			'?':  PLACEHOLDER,
			'@':  PARAMETER,
			'#':  HASH,
			'\'': UNKNOWN,
			'`':  UNKNOWN,
			'"':  UNKNOWN,
		},
		Keywords: map[string]TokenType{
			"!=":                NEQ,
			"#>":                HASH_ARROW,
			"#>>":               DHASH_ARROW,
			"%}":                BLOCK_END,
			"&&":                DAMP,
			"&<":                AMP_LT,
			"&>":                AMP_GT,
			"+%}":               BLOCK_END,
			"+}}":               BLOCK_END,
			"-%}":               BLOCK_END,
			"->":                ARROW,
			"->>":               DARROW,
			"-|-":               ADJACENT,
			"-}}":               BLOCK_END,
			"/*+":               HINT,
			"::":                DCOLON,
			":=":                COLON_EQ,
			"<->":               LR_ARROW,
			"<<->>":             LLRR_ARROW,
			"<=":                LTE,
			"<=>":               NULLSAFE_EQ,
			"<>":                NEQ,
			"==":                EQ,
			"=>":                FARROW,
			">=":                GTE,
			"?::":               QDCOLON,
			"??":                DQMARK,
			"ALL":               ALL,
			"ALTER":             ALTER,
			"ANALYZE":           ANALYZE,
			"AND":               AND,
			"ANTI":              ANTI,
			"ANY":               ANY,
			"APPLY":             APPLY,
			"ARRAY":             ARRAY,
			"AS":                ALIAS,
			"ASC":               ASC,
			"ASOF":              ASOF,
			"AUTOINCREMENT":     AUTO_INCREMENT,
			"AUTO_INCREMENT":    AUTO_INCREMENT,
			"BEGIN":             BEGIN,
			"BETWEEN":           BETWEEN,
			"BIGDECIMAL":        BIGDECIMAL,
			"BIGINT":            BIGINT,
			"BIGNUM":            BIGNUM,
			"BIGNUMERIC":        BIGDECIMAL,
			"BINARY":            BINARY,
			"BIT":               BIT,
			"BLOB":              VARBINARY,
			"BOOL":              BOOLEAN,
			"BOOLEAN":           BOOLEAN,
			"BPCHAR":            BPCHAR,
			"BYTE":              TINYINT,
			"BYTEA":             VARBINARY,
			"CACHE":             CACHE,
			"CALL":              COMMAND,
			"CASE":              CASE,
			"CHAR":              CHAR,
			"CHAR VARYING":      VARCHAR,
			"CHARACTER":         CHAR,
			"CHARACTER SET":     CHARACTER_SET,
			"CHARACTER VARYING": VARCHAR,
			"CLOB":              TEXT,
			"CLUSTER BY":        CLUSTER_BY,
			"COLLATE":           COLLATE,
			"COLUMN":            COLUMN,
			"COMMENT":           COMMENT,
			"COMMIT":            COMMIT,
			"CONNECT BY":        CONNECT_BY,
			"CONSTRAINT":        CONSTRAINT,
			"COPY":              COPY,
			"CREATE":            CREATE,
			"CROSS":             CROSS,
			"CUBE":              CUBE,
			"CURRENT_CATALOG":   CURRENT_CATALOG,
			"CURRENT_DATE":      CURRENT_DATE,
			"CURRENT_SCHEMA":    CURRENT_SCHEMA,
			"CURRENT_TIME":      CURRENT_TIME,
			"CURRENT_TIMESTAMP": CURRENT_TIMESTAMP,
			"CURRENT_USER":      CURRENT_USER,
			"DATABASE":          DATABASE,
			"DATE":              DATE,
			"DATEMULTIRANGE":    DATEMULTIRANGE,
			"DATERANGE":         DATERANGE,
			"DATETIME":          DATETIME,
			"DEC":               DECIMAL,
			"DECFLOAT":          DECFLOAT,
			"DECIMAL":           DECIMAL,
			"DECIMAL128":        DECIMAL128,
			"DECIMAL256":        DECIMAL256,
			"DECIMAL32":         DECIMAL32,
			"DECIMAL64":         DECIMAL64,
			"DEFAULT":           DEFAULT,
			"DELETE":            DELETE,
			"DESC":              DESC,
			"DESCRIBE":          DESCRIBE,
			"DISTINCT":          DISTINCT,
			"DISTRIBUTE BY":     DISTRIBUTE_BY,
			"DIV":               DIV,
			"DOUBLE":            DOUBLE,
			"DOUBLE PRECISION":  DOUBLE,
			"DROP":              DROP,
			"ELSE":              ELSE,
			"END":               END,
			"ENUM":              ENUM,
			"ESCAPE":            ESCAPE,
			"EXCEPT":            EXCEPT,
			"EXECUTE":           EXECUTE,
			"EXISTS":            EXISTS,
			"EXPLAIN":           COMMAND,
			"FALSE":             FALSE,
			"FETCH":             FETCH,
			"FILE":              FILE,
			"FILTER":            FILTER,
			"FIRST":             FIRST,
			"FIXED":             DECIMAL,
			"FLOAT":             FLOAT,
			"FLOAT4":            FLOAT,
			"FLOAT8":            DOUBLE,
			"FOR":               FOR,
			"FOR TIMESTAMP":     TIMESTAMP_SNAPSHOT,
			"FOR VERSION":       VERSION_SNAPSHOT,
			"FOREIGN KEY":       FOREIGN_KEY,
			"FORMAT":            FORMAT,
			"FROM":              FROM,
			"FULL":              FULL,
			"FUNCTION":          FUNCTION,
			"GEOGRAPHY":         GEOGRAPHY,
			"GEOMETRY":          GEOMETRY,
			"GLOB":              GLOB,
			"GRANT":             GRANT,
			"GROUP BY":          GROUP_BY,
			"GROUPING SETS":     GROUPING_SETS,
			"HAVING":            HAVING,
			"HUGEINT":           INT128,
			"ILIKE":             ILIKE,
			"IN":                IN,
			"INDEX":             INDEX,
			"INET":              INET,
			"INNER":             INNER,
			"INSERT":            INSERT,
			"INT":               INT,
			"INT1":              TINYINT,
			"INT128":            INT128,
			"INT16":             SMALLINT,
			"INT2":              SMALLINT,
			"INT256":            INT256,
			"INT32":             INT,
			"INT4":              INT,
			"INT4MULTIRANGE":    INT4MULTIRANGE,
			"INT4RANGE":         INT4RANGE,
			"INT64":             BIGINT,
			"INT8":              TINYINT,
			"INT8MULTIRANGE":    INT8MULTIRANGE,
			"INT8RANGE":         INT8RANGE,
			"INTEGER":           INT,
			"INTERSECT":         INTERSECT,
			"INTERVAL":          INTERVAL,
			"INTO":              INTO,
			"IS":                IS,
			"ISNULL":            ISNULL,
			"JOIN":              JOIN,
			"JSON":              JSON,
			"JSONB":             JSONB,
			"KEEP":              KEEP,
			"KILL":              KILL,
			"LATERAL":           LATERAL,
			"LEFT":              LEFT,
			"LIKE":              LIKE,
			"LIMIT":             LIMIT,
			"LIST":              LIST,
			"LOAD":              LOAD,
			"LOCALTIME":         LOCALTIME,
			"LOCALTIMESTAMP":    LOCALTIMESTAMP,
			"LOCK":              LOCK,
			"LONG":              BIGINT,
			"LONGBLOB":          LONGBLOB,
			"LONGTEXT":          LONGTEXT,
			"LONGVARCHAR":       TEXT,
			"MAP":               MAP,
			"MEDIUMBLOB":        MEDIUMBLOB,
			"MEDIUMINT":         MEDIUMINT,
			"MEDIUMTEXT":        MEDIUMTEXT,
			"MERGE":             MERGE,
			"NAMESPACE":         NAMESPACE,
			"NATURAL":           NATURAL,
			"NCHAR":             NCHAR,
			"NEXT":              NEXT,
			"NOT":               NOT,
			"NOTNULL":           NOTNULL,
			"NULL":              NULL,
			"NULLABLE":          NULLABLE,
			"NUMBER":            DECIMAL,
			"NUMERIC":           DECIMAL,
			"NUMMULTIRANGE":     NUMMULTIRANGE,
			"NUMRANGE":          NUMRANGE,
			"NVARCHAR":          NVARCHAR,
			"NVARCHAR2":         NVARCHAR,
			"OBJECT":            OBJECT,
			"OFFSET":            OFFSET,
			"ON":                ON,
			"OPERATOR":          OPERATOR,
			"OPTIMIZE":          COMMAND,
			"OR":                OR,
			"ORDER BY":          ORDER_BY,
			"ORDINALITY":        ORDINALITY,
			"OUT":               OUT,
			"OUTER":             OUTER,
			"OVER":              OVER,
			"OVERLAPS":          OVERLAPS,
			"OVERWRITE":         OVERWRITE,
			"PARTITION":         PARTITION,
			"PARTITION BY":      PARTITION_BY,
			"PARTITIONED BY":    PARTITION_BY,
			"PARTITIONED_BY":    PARTITION_BY,
			"PERCENT":           PERCENT,
			"PIVOT":             PIVOT,
			"PRAGMA":            PRAGMA,
			"PREPARE":           COMMAND,
			"PRIMARY KEY":       PRIMARY_KEY,
			"PROCEDURE":         PROCEDURE,
			"QUALIFY":           QUALIFY,
			"RANGE":             RANGE,
			"REAL":              FLOAT,
			"RECURSIVE":         RECURSIVE,
			"REFERENCES":        REFERENCES,
			"REGEXP":            RLIKE,
			"RENAME":            RENAME,
			"REPLACE":           REPLACE,
			"RETURNING":         RETURNING,
			"REVOKE":            REVOKE,
			"RIGHT":             RIGHT,
			"RLIKE":             RLIKE,
			"ROLLBACK":          ROLLBACK,
			"ROLLUP":            ROLLUP,
			"ROW":               ROW,
			"ROWS":              ROWS,
			"SCHEMA":            SCHEMA,
			"SELECT":            SELECT,
			"SEMI":              SEMI,
			"SEQUENCE":          SEQUENCE,
			"SESSION":           SESSION,
			"SESSION_USER":      SESSION_USER,
			"SET":               SET,
			"SETTINGS":          SETTINGS,
			"SHORT":             SMALLINT,
			"SHOW":              SHOW,
			"SIMILAR TO":        SIMILAR_TO,
			"SMALLINT":          SMALLINT,
			"SOME":              SOME,
			"SORT BY":           SORT_BY,
			"SQL SECURITY":      SQL_SECURITY,
			"START WITH":        START_WITH,
			"STR":               TEXT,
			"STRAIGHT_JOIN":     STRAIGHT_JOIN,
			"STRING":            TEXT,
			"STRUCT":            STRUCT,
			"TABLE":             TABLE,
			"TABLESAMPLE":       TABLE_SAMPLE,
			"TEMP":              TEMPORARY,
			"TEMPORARY":         TEMPORARY,
			"TEXT":              TEXT,
			"THEN":              THEN,
			"TIME":              TIME,
			"TIMESTAMP":         TIMESTAMP,
			"TIMESTAMPLTZ":      TIMESTAMPLTZ,
			"TIMESTAMPNTZ":      TIMESTAMPNTZ,
			"TIMESTAMPTZ":       TIMESTAMPTZ,
			"TIMESTAMP_LTZ":     TIMESTAMPLTZ,
			"TIMESTAMP_NTZ":     TIMESTAMPNTZ,
			"TIMETZ":            TIMETZ,
			"TIME_NS":           TIME_NS,
			"TINYBLOB":          TINYBLOB,
			"TINYINT":           TINYINT,
			"TINYTEXT":          TINYTEXT,
			"TRIGGER":           TRIGGER,
			"TRUE":              TRUE,
			"TRUNCATE":          TRUNCATE,
			"TSMULTIRANGE":      TSMULTIRANGE,
			"TSRANGE":           TSRANGE,
			"TSTZMULTIRANGE":    TSTZMULTIRANGE,
			"TSTZRANGE":         TSTZRANGE,
			"UHUGEINT":          UINT128,
			"UINT":              UINT,
			"UINT128":           UINT128,
			"UINT256":           UINT256,
			"UNCACHE":           UNCACHE,
			"UNION":             UNION,
			"UNIQUE":            UNIQUE,
			"UNKNOWN":           UNKNOWN,
			"UNNEST":            UNNEST,
			"UNPIVOT":           UNPIVOT,
			"UPDATE":            UPDATE,
			"USE":               USE,
			"USER-DEFINED":      USERDEFINED,
			"USING":             USING,
			"UUID":              UUID,
			"VACUUM":            COMMAND,
			"VALUES":            VALUES,
			"VARBINARY":         VARBINARY,
			"VARCHAR":           VARCHAR,
			"VARCHAR2":          VARCHAR,
			"VARIANT":           VARIANT,
			"VECTOR":            VECTOR,
			"VIEW":              VIEW,
			"VOLATILE":          VOLATILE,
			"WHEN":              WHEN,
			"WHERE":             WHERE,
			"WINDOW":            WINDOW,
			"WITH":              WITH,
			"XOR":               XOR,
			"{%":                BLOCK_START,
			"{%+":               BLOCK_START,
			"{%-":               BLOCK_START,
			"{{+":               BLOCK_START,
			"{{-":               BLOCK_START,
			"|>":                PIPE_GT,
			"||":                DPIPE,
			"~*":                IRLIKE,
			"~~":                LIKE,
			"~~*":               ILIKE,
			"~~~":               GLOB,
		},
		Quotes:            map[string]string{"'": "'"},
		FormatStrings:     map[string]FormatString{},
		Identifiers:       map[rune]string{'"': "\""},
		Comments:          map[string]string{"--": "", "/*": "*/"},
		StringEscapes:     map[rune]bool{'\'': true},
		ByteStringEscapes: map[rune]bool{},
		IdentifierEscapes: map[rune]bool{},
		EscapeFollowChars: map[rune]bool{},
		Commands: map[TokenType]bool{
			COMMAND: true,
			EXECUTE: true,
			FETCH:   true,
			RENAME:  true,
			SHOW:    true,
		},
		CommandPrefixTokens: map[TokenType]bool{
			SEMICOLON: true,
			BEGIN:     true,
		},
		NestedComments:                   true,
		HintStart:                        "/*+",
		TokensPrecedingHint:              map[TokenType]bool{SELECT: true, INSERT: true, UPDATE: true, DELETE: true},
		NumericLiterals:                  map[string]string{},
		VarSingleTokens:                  map[rune]bool{},
		StringEscapesAllowedInRawStrings: true,
		HeredocStringAlternative:         VAR,
		NumbersCanHaveDecimals:           true,
		UnescapedSequences:               map[string]string{},
	}
	return compileConfig(cfg)
}

// defaultUnescapedSequences ports the module-level UNESCAPED_SEQUENCES table
// (dialects/dialect.py:66-75): the two-character backslash escapes recognized while
// SCANNING a string (as opposed to escapedSequences in package generator, which is this
// table inverted and used while EMITTING one).
var defaultUnescapedSequences = map[string]string{
	`\a`: "\a",
	`\b`: "\b",
	`\f`: "\f",
	`\n`: "\n",
	`\r`: "\r",
	`\t`: "\t",
	`\v`: "\v",
	`\\`: `\`,
}

func compileConfig(cfg TokenizerConfig) TokenizerConfig {
	if cfg.FormatStrings == nil {
		cfg.FormatStrings = map[string]FormatString{}
	}
	// Ports the Dialect metaclass hook (dialects/dialect.py:297-306): a dialect whose
	// regular strings or byte strings support backslash escapes (StringEscapes['\\'] /
	// ByteStringEscapes['\\'] - the Go analogues of STRINGS_SUPPORT_ESCAPED_SEQUENCES /
	// BYTE_STRINGS_SUPPORT_ESCAPED_SEQUENCES) gets the default UNESCAPED_SEQUENCES table
	// merged in, so e.g. mysql's `\\` collapses to one backslash and postgres's `e'\n'`
	// scans to an actual newline. No dialect here (base/mysql/postgres) sets its own
	// UnescapedSequences, so a plain copy (guarded by len==0, since this can run twice -
	// once inside BaseConfig, again via the per-dialect CompileConfig call) suffices;
	// a real per-dialect override table would need the same `{**default, **override}`
	// merge dialect.py does, deferred until a dialect needs it.
	if (cfg.StringEscapes['\\'] || cfg.ByteStringEscapes['\\']) && len(cfg.UnescapedSequences) == 0 {
		if cfg.UnescapedSequences == nil {
			cfg.UnescapedSequences = map[string]string{}
		}
		for k, v := range defaultUnescapedSequences {
			cfg.UnescapedSequences[k] = v
		}
	}
	for start, end := range cfg.Quotes {
		cfg.FormatStrings["n"+start] = FormatString{End: end, TokenType: NATIONAL_STRING}
		cfg.FormatStrings["N"+start] = FormatString{End: end, TokenType: NATIONAL_STRING}
	}
	if cfg.Comments == nil {
		cfg.Comments = map[string]string{}
	}
	cfg.Comments["{#"] = "#}"
	if _, ok := cfg.Keywords[cfg.HintStart]; ok {
		cfg.Comments[cfg.HintStart] = "*/"
	}
	var keys []string
	addKey := func(key string) {
		if strings.Contains(key, " ") {
			keys = append(keys, strings.ToUpper(key))
			return
		}
		for single := range cfg.SingleTokens {
			if strings.ContainsRune(key, single) {
				keys = append(keys, strings.ToUpper(key))
				return
			}
		}
	}
	for key := range cfg.Keywords {
		addKey(key)
	}
	for key := range cfg.Comments {
		addKey(key)
	}
	for key := range cfg.Quotes {
		addKey(key)
	}
	for key := range cfg.FormatStrings {
		addKey(key)
	}
	cfg.KeywordTrie = trie.NewTrie(keys)
	return cfg
}

// CompileConfig recomputes derived config (national-string format strings, the
// hint comment, and the keyword trie) after a caller mutates a base config's maps.
// Exported so per-dialect config builders in package dialects can finalize configs.
func CompileConfig(cfg TokenizerConfig) TokenizerConfig { return compileConfig(cfg) }

// TokenizeFunc lets dialects with multi-stage tokenization retain the public concrete
// *Tokenizer API while routing each SQL string through a custom implementation.
type TokenizeFunc func(sql string) ([]Token, error)

type Tokenizer struct {
	core         *TokenizerCore
	tokenizeFunc TokenizeFunc
	sql          []rune
	size         int
	tokens       []Token
}

func NewTokenizer() *Tokenizer { cfg := BaseConfig(); return NewTokenizerWithConfig(cfg) }

func NewTokenizerWithConfig(cfg TokenizerConfig) *Tokenizer {
	return &Tokenizer{core: NewTokenizerCore(cfg)}
}

// NewTokenizerWithFunc constructs a tokenizer backed by a non-nil custom callback.
func NewTokenizerWithFunc(tokenizeFunc TokenizeFunc) *Tokenizer {
	if tokenizeFunc == nil {
		panic("tokens: nil tokenize function")
	}
	return &Tokenizer{tokenizeFunc: tokenizeFunc}
}

func (t *Tokenizer) Tokenize(sql string) ([]Token, error) {
	if t.tokenizeFunc == nil {
		return t.core.Tokenize(sql)
	}

	t.sql = []rune(sql)
	t.size = len(t.sql)
	t.tokens = nil
	tokenized, err := t.tokenizeFunc(sql)
	t.tokens = tokenized
	return tokenized, err
}

func (t *Tokenizer) Tokens() []Token {
	if t.tokenizeFunc == nil {
		return t.core.tokens
	}
	return t.tokens
}

func (t *Tokenizer) SQL() string {
	if t.tokenizeFunc == nil {
		return string(t.core.sql)
	}
	return string(t.sql)
}

func (t *Tokenizer) Size() int {
	if t.tokenizeFunc == nil {
		return t.core.size
	}
	return t.size
}
