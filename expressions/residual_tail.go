package expressions

// BitString ports exp.BitString (expressions/query.py:471-472, is_primitive=True): a
// bit-string literal, e.g. `b'101'` (mysql/postgres) or mysql's bare `0b101`.
func BitString(args Args) Expression { return newNode(KindBitString, args) }

// HexString ports exp.HexString (expressions/query.py:480-482, is_primitive=True): a
// hex-string literal, e.g. `x'FF'` (mysql/postgres) or mysql's bare `0xFF`.
func HexString(args Args) Expression { return newNode(KindHexString, args) }

// ByteString ports exp.ByteString (expressions/query.py:485-487, is_primitive=True): a
// byte-string literal, e.g. postgres `e'\n'`.
func ByteString(args Args) Expression { return newNode(KindByteString, args) }

// SessionParameter ports exp.SessionParameter (expressions/core.py:1837-1838): mysql
// `@@GLOBAL.max_connections` / bare `@@x` session-variable reference.
func SessionParameter(args Args) Expression { return newNode(KindSessionParameter, args) }

// PropertyEQ ports exp.PropertyEQ (expressions/core.py:2150-2151): the ASSIGNMENT `:=`
// operator (parser.py:881-883), e.g. mysql `SELECT @var1 := 1`.
func PropertyEQ(args Args) Expression { return newNode(KindPropertyEQ, args) }

// Distance ports exp.Distance (expressions/core.py:2154-2155): the postgres `<->` point
// distance operator.
func Distance(args Args) Expression { return newNode(KindDistance, args) }

// DistanceNd ports exp.DistanceNd (expressions/core.py:2158-2159): the postgres `<<->>`
// box distance operator.
func DistanceNd(args Args) Expression { return newNode(KindDistanceNd, args) }

// Lag ports exp.Lag (expressions/aggregate.py:150-151): the LAG(this[, offset[, default]])
// window function.
func Lag(args Args) Expression { return newNode(KindLag, args) }

// Lead ports exp.Lead (expressions/aggregate.py:162-163): the LEAD(this[, offset[,
// default]]) window function.
func Lead(args Args) Expression { return newNode(KindLead, args) }

// Concat ports exp.Concat (expressions/string.py:29-31): only built by _parse_primary's
// adjacent-string-literal rewrite (`'a' 'b'` -> Concat) - see KindConcat's doc.
func Concat(args Args) Expression { return newNode(KindConcat, args) }
