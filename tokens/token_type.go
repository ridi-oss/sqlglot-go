package tokens

import "fmt"

type TokenType uint16

const (
	L_PAREN TokenType = iota + 1
	R_PAREN
	L_BRACKET
	R_BRACKET
	L_BRACE
	R_BRACE
	COMMA
	DOT
	DASH
	PLUS
	COLON
	DOTCOLON
	DOTCARET
	DCOLON
	DCOLONDOLLAR
	DCOLONPERCENT
	DCOLONQMARK
	DQMARK
	SEMICOLON
	STAR
	BACKSLASH
	SLASH
	LT
	LTE
	GT
	GTE
	NOT
	EQ
	NEQ
	NULLSAFE_EQ
	COLON_EQ
	COLON_GT
	NCOLON_GT
	AND
	OR
	AMP
	DPIPE
	PIPE_GT
	PIPE
	PIPE_SLASH
	DPIPE_SLASH
	CARET
	CARET_AT
	TILDE
	ARROW
	DARROW
	FARROW
	HASH
	HASH_ARROW
	DHASH_ARROW
	LR_ARROW
	LLRR_ARROW
	DAT
	AT_QMARK
	LT_AT
	AT_GT
	DOLLAR
	PARAMETER
	SESSION
	SESSION_PARAMETER
	SESSION_USER
	DAMP
	AMP_LT
	AMP_GT
	ADJACENT
	XOR
	DSTAR
	QMARK_AMP
	QMARK_PIPE
	HASH_DASH
	EXCLAMATION
	URI_START
	BLOCK_START
	BLOCK_END
	SPACE
	BREAK
	STRING
	NUMBER
	IDENTIFIER
	DATABASE
	COLUMN
	COLUMN_DEF
	SCHEMA
	TABLE
	WAREHOUSE
	STAGE
	STREAM
	STREAMLIT
	VAR
	BIT_STRING
	HEX_STRING
	BYTE_STRING
	NATIONAL_STRING
	RAW_STRING
	HEREDOC_STRING
	UNICODE_STRING
	BIT
	BOOLEAN
	TINYINT
	UTINYINT
	SMALLINT
	USMALLINT
	MEDIUMINT
	UMEDIUMINT
	INT
	UINT
	BIGINT
	UBIGINT
	BIGNUM
	INT128
	UINT128
	INT256
	UINT256
	FLOAT
	DOUBLE
	UDOUBLE
	DECIMAL
	DECIMAL32
	DECIMAL64
	DECIMAL128
	DECIMAL256
	DECFLOAT
	UDECIMAL
	BIGDECIMAL
	CHAR
	NCHAR
	VARCHAR
	NVARCHAR
	BPCHAR
	TEXT
	MEDIUMTEXT
	LONGTEXT
	BLOB
	MEDIUMBLOB
	LONGBLOB
	TINYBLOB
	TINYTEXT
	NAME
	BINARY
	VARBINARY
	JSON
	JSONB
	TIME
	TIMETZ
	TIME_NS
	TIMESTAMP
	TIMESTAMPTZ
	TIMESTAMPLTZ
	TIMESTAMPNTZ
	TIMESTAMP_S
	TIMESTAMP_MS
	TIMESTAMP_NS
	DATETIME
	DATETIME2
	DATETIME64
	SMALLDATETIME
	DATE
	DATE32
	INT4RANGE
	INT4MULTIRANGE
	INT8RANGE
	INT8MULTIRANGE
	NUMRANGE
	NUMMULTIRANGE
	TSRANGE
	TSMULTIRANGE
	TSTZRANGE
	TSTZMULTIRANGE
	DATERANGE
	DATEMULTIRANGE
	UUID
	GEOGRAPHY
	GEOGRAPHYPOINT
	NULLABLE
	GEOMETRY
	POINT
	RING
	LINESTRING
	LOCALTIME
	LOCALTIMESTAMP
	SYSTIMESTAMP
	MULTILINESTRING
	POLYGON
	MULTIPOLYGON
	HLLSKETCH
	HSTORE
	SUPER
	SERIAL
	SMALLSERIAL
	BIGSERIAL
	XML
	YEAR
	USERDEFINED
	MONEY
	SMALLMONEY
	ROWVERSION
	IMAGE
	VARIANT
	OBJECT
	INET
	IPADDRESS
	IPPREFIX
	IPV4
	IPV6
	ENUM
	ENUM8
	ENUM16
	FIXEDSTRING
	LOWCARDINALITY
	NESTED
	AGGREGATEFUNCTION
	SIMPLEAGGREGATEFUNCTION
	TDIGEST
	UNKNOWN
	VECTOR
	DYNAMIC
	VOID
	ALIAS
	ALTER
	ALL
	ANTI
	ANY
	APPLY
	ARRAY
	ASC
	ASOF
	ATTACH
	AUTO_INCREMENT
	BEGIN
	BETWEEN
	BULK_COLLECT_INTO
	CACHE
	CASE
	CHARACTER_SET
	CLUSTER_BY
	COLLATE
	COMMAND
	COMMENT
	COMMIT
	CONNECT_BY
	CONSTRAINT
	COPY
	CREATE
	CROSS
	CUBE
	CURRENT_DATE
	CURRENT_DATETIME
	CURRENT_SCHEMA
	CURRENT_TIME
	CURRENT_TIMESTAMP
	CURRENT_USER
	CURRENT_USER_ID
	CURRENT_ROLE
	CURRENT_CATALOG
	DECLARE
	DEFAULT
	DELETE
	DESC
	DESCRIBE
	DETACH
	DICTIONARY
	DISTINCT
	DISTRIBUTE_BY
	DIV
	DROP
	ELSE
	END
	ESCAPE
	EXCEPT
	EXECUTE
	EXISTS
	FALSE
	FETCH
	FILE
	FILE_FORMAT
	FILTER
	FINAL
	FIRST
	FOR
	FORCE
	FOREIGN_KEY
	FORMAT
	FROM
	FULL
	FUNCTION
	GET
	GLOB
	GLOBAL
	GRANT
	GROUP_BY
	GROUPING_SETS
	HAVING
	HINT
	IGNORE
	ILIKE
	IN
	INDEX
	INDEXED_BY
	INNER
	INSERT
	INSTALL
	INTEGRATION
	INTERSECT
	INTERVAL
	INTO
	INTRODUCER
	IRLIKE
	IS
	ISNULL
	JOIN
	JOIN_MARKER
	KEEP
	KEY
	KILL
	LANGUAGE
	LATERAL
	LEFT
	LIKE
	LIMIT
	LIST
	LOAD
	LOCK
	MAP
	MATCH
	MATCH_CONDITION
	MATCH_RECOGNIZE
	MEMBER_OF
	MERGE
	MOD
	MODEL
	NATURAL
	NEXT
	NOTHING
	NOTNULL
	NULL
	OBJECT_IDENTIFIER
	OFFSET
	ON
	ONLY
	OPERATOR
	ORDER_BY
	ORDER_SIBLINGS_BY
	ORDERED
	ORDINALITY
	OUT
	INOUT
	OUTER
	OVER
	OVERLAPS
	OVERWRITE
	PACKAGE
	PARTITION
	PARTITION_BY
	PERCENT
	PIVOT
	PLACEHOLDER
	POLICY
	POOL
	POSITIONAL
	PRAGMA
	PREWHERE
	PRIMARY_KEY
	PROCEDURE
	PROPERTIES
	PSEUDO_TYPE
	PUT
	QUALIFY
	QUOTE
	QDCOLON
	RANGE
	RECURSIVE
	REFRESH
	RENAME
	REPLACE
	RETURNING
	REVOKE
	REFERENCES
	RIGHT
	RLIKE
	ROLE
	ROLLBACK
	ROLLUP
	ROW
	ROWS
	RULE
	SELECT
	SEMI
	SEPARATOR
	SEQUENCE
	SERDE_PROPERTIES
	SET
	SETTINGS
	SHOW
	SIMILAR_TO
	SOME
	SORT_BY
	SOUNDS_LIKE
	SQL_SECURITY
	START_WITH
	STORAGE_INTEGRATION
	STRAIGHT_JOIN
	STRUCT
	SUMMARIZE
	TABLE_SAMPLE
	TAG
	TEMPORARY
	TOP
	THEN
	TRUE
	TRUNCATE
	TRIGGER
	TYPE
	UNCACHE
	UNDROP
	UNION
	UNLOAD
	UNNEST
	UNPIVOT
	UPDATE
	USE
	USING
	VALUES
	VARIADIC
	VIEW
	SEMANTIC_VIEW
	VOLATILE
	VOLUME
	WHEN
	WHERE
	WINDOW
	WITH
	UNIQUE
	UTC_DATE
	UTC_TIME
	UTC_TIMESTAMP
	VERSION_SNAPSHOT
	TIMESTAMP_SNAPSHOT
	OPTION
	SINK
	SOURCE
	ANALYZE
	NAMESPACE
	EXPORT
	HIVE_TOKEN_STREAM
	SENTINEL
)

var tokenTypeNames = map[TokenType]string{
	L_PAREN:                 "L_PAREN",
	R_PAREN:                 "R_PAREN",
	L_BRACKET:               "L_BRACKET",
	R_BRACKET:               "R_BRACKET",
	L_BRACE:                 "L_BRACE",
	R_BRACE:                 "R_BRACE",
	COMMA:                   "COMMA",
	DOT:                     "DOT",
	DASH:                    "DASH",
	PLUS:                    "PLUS",
	COLON:                   "COLON",
	DOTCOLON:                "DOTCOLON",
	DOTCARET:                "DOTCARET",
	DCOLON:                  "DCOLON",
	DCOLONDOLLAR:            "DCOLONDOLLAR",
	DCOLONPERCENT:           "DCOLONPERCENT",
	DCOLONQMARK:             "DCOLONQMARK",
	DQMARK:                  "DQMARK",
	SEMICOLON:               "SEMICOLON",
	STAR:                    "STAR",
	BACKSLASH:               "BACKSLASH",
	SLASH:                   "SLASH",
	LT:                      "LT",
	LTE:                     "LTE",
	GT:                      "GT",
	GTE:                     "GTE",
	NOT:                     "NOT",
	EQ:                      "EQ",
	NEQ:                     "NEQ",
	NULLSAFE_EQ:             "NULLSAFE_EQ",
	COLON_EQ:                "COLON_EQ",
	COLON_GT:                "COLON_GT",
	NCOLON_GT:               "NCOLON_GT",
	AND:                     "AND",
	OR:                      "OR",
	AMP:                     "AMP",
	DPIPE:                   "DPIPE",
	PIPE_GT:                 "PIPE_GT",
	PIPE:                    "PIPE",
	PIPE_SLASH:              "PIPE_SLASH",
	DPIPE_SLASH:             "DPIPE_SLASH",
	CARET:                   "CARET",
	CARET_AT:                "CARET_AT",
	TILDE:                   "TILDE",
	ARROW:                   "ARROW",
	DARROW:                  "DARROW",
	FARROW:                  "FARROW",
	HASH:                    "HASH",
	HASH_ARROW:              "HASH_ARROW",
	DHASH_ARROW:             "DHASH_ARROW",
	LR_ARROW:                "LR_ARROW",
	LLRR_ARROW:              "LLRR_ARROW",
	DAT:                     "DAT",
	AT_QMARK:                "AT_QMARK",
	LT_AT:                   "LT_AT",
	AT_GT:                   "AT_GT",
	DOLLAR:                  "DOLLAR",
	PARAMETER:               "PARAMETER",
	SESSION:                 "SESSION",
	SESSION_PARAMETER:       "SESSION_PARAMETER",
	SESSION_USER:            "SESSION_USER",
	DAMP:                    "DAMP",
	AMP_LT:                  "AMP_LT",
	AMP_GT:                  "AMP_GT",
	ADJACENT:                "ADJACENT",
	XOR:                     "XOR",
	DSTAR:                   "DSTAR",
	QMARK_AMP:               "QMARK_AMP",
	QMARK_PIPE:              "QMARK_PIPE",
	HASH_DASH:               "HASH_DASH",
	EXCLAMATION:             "EXCLAMATION",
	URI_START:               "URI_START",
	BLOCK_START:             "BLOCK_START",
	BLOCK_END:               "BLOCK_END",
	SPACE:                   "SPACE",
	BREAK:                   "BREAK",
	STRING:                  "STRING",
	NUMBER:                  "NUMBER",
	IDENTIFIER:              "IDENTIFIER",
	DATABASE:                "DATABASE",
	COLUMN:                  "COLUMN",
	COLUMN_DEF:              "COLUMN_DEF",
	SCHEMA:                  "SCHEMA",
	TABLE:                   "TABLE",
	WAREHOUSE:               "WAREHOUSE",
	STAGE:                   "STAGE",
	STREAM:                  "STREAM",
	STREAMLIT:               "STREAMLIT",
	VAR:                     "VAR",
	BIT_STRING:              "BIT_STRING",
	HEX_STRING:              "HEX_STRING",
	BYTE_STRING:             "BYTE_STRING",
	NATIONAL_STRING:         "NATIONAL_STRING",
	RAW_STRING:              "RAW_STRING",
	HEREDOC_STRING:          "HEREDOC_STRING",
	UNICODE_STRING:          "UNICODE_STRING",
	BIT:                     "BIT",
	BOOLEAN:                 "BOOLEAN",
	TINYINT:                 "TINYINT",
	UTINYINT:                "UTINYINT",
	SMALLINT:                "SMALLINT",
	USMALLINT:               "USMALLINT",
	MEDIUMINT:               "MEDIUMINT",
	UMEDIUMINT:              "UMEDIUMINT",
	INT:                     "INT",
	UINT:                    "UINT",
	BIGINT:                  "BIGINT",
	UBIGINT:                 "UBIGINT",
	BIGNUM:                  "BIGNUM",
	INT128:                  "INT128",
	UINT128:                 "UINT128",
	INT256:                  "INT256",
	UINT256:                 "UINT256",
	FLOAT:                   "FLOAT",
	DOUBLE:                  "DOUBLE",
	UDOUBLE:                 "UDOUBLE",
	DECIMAL:                 "DECIMAL",
	DECIMAL32:               "DECIMAL32",
	DECIMAL64:               "DECIMAL64",
	DECIMAL128:              "DECIMAL128",
	DECIMAL256:              "DECIMAL256",
	DECFLOAT:                "DECFLOAT",
	UDECIMAL:                "UDECIMAL",
	BIGDECIMAL:              "BIGDECIMAL",
	CHAR:                    "CHAR",
	NCHAR:                   "NCHAR",
	VARCHAR:                 "VARCHAR",
	NVARCHAR:                "NVARCHAR",
	BPCHAR:                  "BPCHAR",
	TEXT:                    "TEXT",
	MEDIUMTEXT:              "MEDIUMTEXT",
	LONGTEXT:                "LONGTEXT",
	BLOB:                    "BLOB",
	MEDIUMBLOB:              "MEDIUMBLOB",
	LONGBLOB:                "LONGBLOB",
	TINYBLOB:                "TINYBLOB",
	TINYTEXT:                "TINYTEXT",
	NAME:                    "NAME",
	BINARY:                  "BINARY",
	VARBINARY:               "VARBINARY",
	JSON:                    "JSON",
	JSONB:                   "JSONB",
	TIME:                    "TIME",
	TIMETZ:                  "TIMETZ",
	TIME_NS:                 "TIME_NS",
	TIMESTAMP:               "TIMESTAMP",
	TIMESTAMPTZ:             "TIMESTAMPTZ",
	TIMESTAMPLTZ:            "TIMESTAMPLTZ",
	TIMESTAMPNTZ:            "TIMESTAMPNTZ",
	TIMESTAMP_S:             "TIMESTAMP_S",
	TIMESTAMP_MS:            "TIMESTAMP_MS",
	TIMESTAMP_NS:            "TIMESTAMP_NS",
	DATETIME:                "DATETIME",
	DATETIME2:               "DATETIME2",
	DATETIME64:              "DATETIME64",
	SMALLDATETIME:           "SMALLDATETIME",
	DATE:                    "DATE",
	DATE32:                  "DATE32",
	INT4RANGE:               "INT4RANGE",
	INT4MULTIRANGE:          "INT4MULTIRANGE",
	INT8RANGE:               "INT8RANGE",
	INT8MULTIRANGE:          "INT8MULTIRANGE",
	NUMRANGE:                "NUMRANGE",
	NUMMULTIRANGE:           "NUMMULTIRANGE",
	TSRANGE:                 "TSRANGE",
	TSMULTIRANGE:            "TSMULTIRANGE",
	TSTZRANGE:               "TSTZRANGE",
	TSTZMULTIRANGE:          "TSTZMULTIRANGE",
	DATERANGE:               "DATERANGE",
	DATEMULTIRANGE:          "DATEMULTIRANGE",
	UUID:                    "UUID",
	GEOGRAPHY:               "GEOGRAPHY",
	GEOGRAPHYPOINT:          "GEOGRAPHYPOINT",
	NULLABLE:                "NULLABLE",
	GEOMETRY:                "GEOMETRY",
	POINT:                   "POINT",
	RING:                    "RING",
	LINESTRING:              "LINESTRING",
	LOCALTIME:               "LOCALTIME",
	LOCALTIMESTAMP:          "LOCALTIMESTAMP",
	SYSTIMESTAMP:            "SYSTIMESTAMP",
	MULTILINESTRING:         "MULTILINESTRING",
	POLYGON:                 "POLYGON",
	MULTIPOLYGON:            "MULTIPOLYGON",
	HLLSKETCH:               "HLLSKETCH",
	HSTORE:                  "HSTORE",
	SUPER:                   "SUPER",
	SERIAL:                  "SERIAL",
	SMALLSERIAL:             "SMALLSERIAL",
	BIGSERIAL:               "BIGSERIAL",
	XML:                     "XML",
	YEAR:                    "YEAR",
	USERDEFINED:             "USERDEFINED",
	MONEY:                   "MONEY",
	SMALLMONEY:              "SMALLMONEY",
	ROWVERSION:              "ROWVERSION",
	IMAGE:                   "IMAGE",
	VARIANT:                 "VARIANT",
	OBJECT:                  "OBJECT",
	INET:                    "INET",
	IPADDRESS:               "IPADDRESS",
	IPPREFIX:                "IPPREFIX",
	IPV4:                    "IPV4",
	IPV6:                    "IPV6",
	ENUM:                    "ENUM",
	ENUM8:                   "ENUM8",
	ENUM16:                  "ENUM16",
	FIXEDSTRING:             "FIXEDSTRING",
	LOWCARDINALITY:          "LOWCARDINALITY",
	NESTED:                  "NESTED",
	AGGREGATEFUNCTION:       "AGGREGATEFUNCTION",
	SIMPLEAGGREGATEFUNCTION: "SIMPLEAGGREGATEFUNCTION",
	TDIGEST:                 "TDIGEST",
	UNKNOWN:                 "UNKNOWN",
	VECTOR:                  "VECTOR",
	DYNAMIC:                 "DYNAMIC",
	VOID:                    "VOID",
	ALIAS:                   "ALIAS",
	ALTER:                   "ALTER",
	ALL:                     "ALL",
	ANTI:                    "ANTI",
	ANY:                     "ANY",
	APPLY:                   "APPLY",
	ARRAY:                   "ARRAY",
	ASC:                     "ASC",
	ASOF:                    "ASOF",
	ATTACH:                  "ATTACH",
	AUTO_INCREMENT:          "AUTO_INCREMENT",
	BEGIN:                   "BEGIN",
	BETWEEN:                 "BETWEEN",
	BULK_COLLECT_INTO:       "BULK_COLLECT_INTO",
	CACHE:                   "CACHE",
	CASE:                    "CASE",
	CHARACTER_SET:           "CHARACTER_SET",
	CLUSTER_BY:              "CLUSTER_BY",
	COLLATE:                 "COLLATE",
	COMMAND:                 "COMMAND",
	COMMENT:                 "COMMENT",
	COMMIT:                  "COMMIT",
	CONNECT_BY:              "CONNECT_BY",
	CONSTRAINT:              "CONSTRAINT",
	COPY:                    "COPY",
	CREATE:                  "CREATE",
	CROSS:                   "CROSS",
	CUBE:                    "CUBE",
	CURRENT_DATE:            "CURRENT_DATE",
	CURRENT_DATETIME:        "CURRENT_DATETIME",
	CURRENT_SCHEMA:          "CURRENT_SCHEMA",
	CURRENT_TIME:            "CURRENT_TIME",
	CURRENT_TIMESTAMP:       "CURRENT_TIMESTAMP",
	CURRENT_USER:            "CURRENT_USER",
	CURRENT_USER_ID:         "CURRENT_USER_ID",
	CURRENT_ROLE:            "CURRENT_ROLE",
	CURRENT_CATALOG:         "CURRENT_CATALOG",
	DECLARE:                 "DECLARE",
	DEFAULT:                 "DEFAULT",
	DELETE:                  "DELETE",
	DESC:                    "DESC",
	DESCRIBE:                "DESCRIBE",
	DETACH:                  "DETACH",
	DICTIONARY:              "DICTIONARY",
	DISTINCT:                "DISTINCT",
	DISTRIBUTE_BY:           "DISTRIBUTE_BY",
	DIV:                     "DIV",
	DROP:                    "DROP",
	ELSE:                    "ELSE",
	END:                     "END",
	ESCAPE:                  "ESCAPE",
	EXCEPT:                  "EXCEPT",
	EXECUTE:                 "EXECUTE",
	EXISTS:                  "EXISTS",
	FALSE:                   "FALSE",
	FETCH:                   "FETCH",
	FILE:                    "FILE",
	FILE_FORMAT:             "FILE_FORMAT",
	FILTER:                  "FILTER",
	FINAL:                   "FINAL",
	FIRST:                   "FIRST",
	FOR:                     "FOR",
	FORCE:                   "FORCE",
	FOREIGN_KEY:             "FOREIGN_KEY",
	FORMAT:                  "FORMAT",
	FROM:                    "FROM",
	FULL:                    "FULL",
	FUNCTION:                "FUNCTION",
	GET:                     "GET",
	GLOB:                    "GLOB",
	GLOBAL:                  "GLOBAL",
	GRANT:                   "GRANT",
	GROUP_BY:                "GROUP_BY",
	GROUPING_SETS:           "GROUPING_SETS",
	HAVING:                  "HAVING",
	HINT:                    "HINT",
	IGNORE:                  "IGNORE",
	ILIKE:                   "ILIKE",
	IN:                      "IN",
	INDEX:                   "INDEX",
	INDEXED_BY:              "INDEXED_BY",
	INNER:                   "INNER",
	INSERT:                  "INSERT",
	INSTALL:                 "INSTALL",
	INTEGRATION:             "INTEGRATION",
	INTERSECT:               "INTERSECT",
	INTERVAL:                "INTERVAL",
	INTO:                    "INTO",
	INTRODUCER:              "INTRODUCER",
	IRLIKE:                  "IRLIKE",
	IS:                      "IS",
	ISNULL:                  "ISNULL",
	JOIN:                    "JOIN",
	JOIN_MARKER:             "JOIN_MARKER",
	KEEP:                    "KEEP",
	KEY:                     "KEY",
	KILL:                    "KILL",
	LANGUAGE:                "LANGUAGE",
	LATERAL:                 "LATERAL",
	LEFT:                    "LEFT",
	LIKE:                    "LIKE",
	LIMIT:                   "LIMIT",
	LIST:                    "LIST",
	LOAD:                    "LOAD",
	LOCK:                    "LOCK",
	MAP:                     "MAP",
	MATCH:                   "MATCH",
	MATCH_CONDITION:         "MATCH_CONDITION",
	MATCH_RECOGNIZE:         "MATCH_RECOGNIZE",
	MEMBER_OF:               "MEMBER_OF",
	MERGE:                   "MERGE",
	MOD:                     "MOD",
	MODEL:                   "MODEL",
	NATURAL:                 "NATURAL",
	NEXT:                    "NEXT",
	NOTHING:                 "NOTHING",
	NOTNULL:                 "NOTNULL",
	NULL:                    "NULL",
	OBJECT_IDENTIFIER:       "OBJECT_IDENTIFIER",
	OFFSET:                  "OFFSET",
	ON:                      "ON",
	ONLY:                    "ONLY",
	OPERATOR:                "OPERATOR",
	ORDER_BY:                "ORDER_BY",
	ORDER_SIBLINGS_BY:       "ORDER_SIBLINGS_BY",
	ORDERED:                 "ORDERED",
	ORDINALITY:              "ORDINALITY",
	OUT:                     "OUT",
	INOUT:                   "INOUT",
	OUTER:                   "OUTER",
	OVER:                    "OVER",
	OVERLAPS:                "OVERLAPS",
	OVERWRITE:               "OVERWRITE",
	PACKAGE:                 "PACKAGE",
	PARTITION:               "PARTITION",
	PARTITION_BY:            "PARTITION_BY",
	PERCENT:                 "PERCENT",
	PIVOT:                   "PIVOT",
	PLACEHOLDER:             "PLACEHOLDER",
	POLICY:                  "POLICY",
	POOL:                    "POOL",
	POSITIONAL:              "POSITIONAL",
	PRAGMA:                  "PRAGMA",
	PREWHERE:                "PREWHERE",
	PRIMARY_KEY:             "PRIMARY_KEY",
	PROCEDURE:               "PROCEDURE",
	PROPERTIES:              "PROPERTIES",
	PSEUDO_TYPE:             "PSEUDO_TYPE",
	PUT:                     "PUT",
	QUALIFY:                 "QUALIFY",
	QUOTE:                   "QUOTE",
	QDCOLON:                 "QDCOLON",
	RANGE:                   "RANGE",
	RECURSIVE:               "RECURSIVE",
	REFRESH:                 "REFRESH",
	RENAME:                  "RENAME",
	REPLACE:                 "REPLACE",
	RETURNING:               "RETURNING",
	REVOKE:                  "REVOKE",
	REFERENCES:              "REFERENCES",
	RIGHT:                   "RIGHT",
	RLIKE:                   "RLIKE",
	ROLE:                    "ROLE",
	ROLLBACK:                "ROLLBACK",
	ROLLUP:                  "ROLLUP",
	ROW:                     "ROW",
	ROWS:                    "ROWS",
	RULE:                    "RULE",
	SELECT:                  "SELECT",
	SEMI:                    "SEMI",
	SEPARATOR:               "SEPARATOR",
	SEQUENCE:                "SEQUENCE",
	SERDE_PROPERTIES:        "SERDE_PROPERTIES",
	SET:                     "SET",
	SETTINGS:                "SETTINGS",
	SHOW:                    "SHOW",
	SIMILAR_TO:              "SIMILAR_TO",
	SOME:                    "SOME",
	SORT_BY:                 "SORT_BY",
	SOUNDS_LIKE:             "SOUNDS_LIKE",
	SQL_SECURITY:            "SQL_SECURITY",
	START_WITH:              "START_WITH",
	STORAGE_INTEGRATION:     "STORAGE_INTEGRATION",
	STRAIGHT_JOIN:           "STRAIGHT_JOIN",
	STRUCT:                  "STRUCT",
	SUMMARIZE:               "SUMMARIZE",
	TABLE_SAMPLE:            "TABLE_SAMPLE",
	TAG:                     "TAG",
	TEMPORARY:               "TEMPORARY",
	TOP:                     "TOP",
	THEN:                    "THEN",
	TRUE:                    "TRUE",
	TRUNCATE:                "TRUNCATE",
	TRIGGER:                 "TRIGGER",
	TYPE:                    "TYPE",
	UNCACHE:                 "UNCACHE",
	UNDROP:                  "UNDROP",
	UNION:                   "UNION",
	UNLOAD:                  "UNLOAD",
	UNNEST:                  "UNNEST",
	UNPIVOT:                 "UNPIVOT",
	UPDATE:                  "UPDATE",
	USE:                     "USE",
	USING:                   "USING",
	VALUES:                  "VALUES",
	VARIADIC:                "VARIADIC",
	VIEW:                    "VIEW",
	SEMANTIC_VIEW:           "SEMANTIC_VIEW",
	VOLATILE:                "VOLATILE",
	VOLUME:                  "VOLUME",
	WHEN:                    "WHEN",
	WHERE:                   "WHERE",
	WINDOW:                  "WINDOW",
	WITH:                    "WITH",
	UNIQUE:                  "UNIQUE",
	UTC_DATE:                "UTC_DATE",
	UTC_TIME:                "UTC_TIME",
	UTC_TIMESTAMP:           "UTC_TIMESTAMP",
	VERSION_SNAPSHOT:        "VERSION_SNAPSHOT",
	TIMESTAMP_SNAPSHOT:      "TIMESTAMP_SNAPSHOT",
	OPTION:                  "OPTION",
	SINK:                    "SINK",
	SOURCE:                  "SOURCE",
	ANALYZE:                 "ANALYZE",
	NAMESPACE:               "NAMESPACE",
	EXPORT:                  "EXPORT",
	HIVE_TOKEN_STREAM:       "HIVE_TOKEN_STREAM",
	SENTINEL:                "SENTINEL",
}

func TypeName(t TokenType) string { return tokenTypeNames[t] }

func (t TokenType) String() string {
	if name, ok := tokenTypeNames[t]; ok {
		return "TokenType." + name
	}
	return fmt.Sprintf("TokenType(%d)", t)
}
