package parser

import "github.com/ridi/sqlglot-go/tokens"

// The option-sets below port parser.py:1553-1743 (the STATEMENT_PARSERS-adjacent class
// vars consumed by the SET/SHOW/USE/DESCRIBE/TRANSACTION/ANALYZE/GRANT/TRUNCATE family
// parsers), plus mysql's PROFILE_TYPES (parsers/mysql.py:273-278). Each family builder
// (a sibling part) reads these directly; fnd doesn't consume them itself.

// describeStyles mirrors parser.py:1710 DESCRIBE_STYLES.
var describeStyles = map[string]bool{"ANALYZE": true, "EXTENDED": true, "FORMATTED": true, "HISTORY": true}

// usables mirrors parser.py:1618-1620 USABLES.
var usables = optionsType{
	"ROLE":      nil,
	"WAREHOUSE": nil,
	"DATABASE":  nil,
	"SCHEMA":    nil,
	"CATALOG":   nil,
}

// transactionKind mirrors parser.py:1571 TRANSACTION_KIND.
var transactionKind = map[string]bool{"DEFERRED": true, "IMMEDIATE": true, "EXCLUSIVE": true}

// transactionCharacteristics mirrors parser.py:1572-1580 TRANSACTION_CHARACTERISTICS.
var transactionCharacteristics = optionsType{
	"ISOLATION": {
		{"LEVEL", "REPEATABLE", "READ"},
		{"LEVEL", "READ", "COMMITTED"},
		{"LEVEL", "READ", "UNCOMITTED"},
		{"LEVEL", "SERIALIZABLE"},
	},
	"READ": {{"WRITE"}, {"ONLY"}},
}

// setAssignmentDelimiters mirrors parser.py:1713 SET_ASSIGNMENT_DELIMITERS.
var setAssignmentDelimiters = map[string]bool{"=": true, ":=": true, "TO": true}

// analyzeStyles mirrors parser.py:1716-1723 ANALYZE_STYLES.
var analyzeStyles = map[string]bool{
	"BUFFER_USAGE_LIMIT": true,
	"FULL":               true,
	"LOCAL":              true,
	"NO_WRITE_TO_BINLOG": true,
	"SAMPLE":             true,
	"SKIP_LOCKED":        true,
	"VERBOSE":            true,
}

// privilegeFollowTokens mirrors parser.py:1707 PRIVILEGE_FOLLOW_TOKENS.
var privilegeFollowTokens = map[tokens.TokenType]bool{tokens.ON: true, tokens.COMMA: true, tokens.L_PAREN: true}

// partitionKeywords mirrors parser.py:1741 PARTITION_KEYWORDS.
var partitionKeywords = map[string]bool{"PARTITION": true, "SUBPARTITION": true}

// profileTypes mirrors mysql PROFILE_TYPES (parsers/mysql.py:273-278): the argument to
// `SHOW PROFILE <type>` (e.g. `SHOW PROFILE BLOCK IO, PAGE FAULTS FOR QUERY 1`).
var profileTypes = optionsType{
	"ALL":     nil,
	"CPU":     nil,
	"IPC":     nil,
	"MEMORY":  nil,
	"SOURCE":  nil,
	"SWAPS":   nil,
	"BLOCK":   {{"IO"}},
	"CONTEXT": {{"SWITCHES"}},
	"PAGE":    {{"FAULTS"}},
}
