package expressions

// Source spans are a Go-only extension (DEVIATIONS.md §6.2): the parser stamps selected nodes
// with the location of the source text they were parsed from, so a consumer can recover the
// verbatim spelling (e.g. MySQL's unaliased-projection column label). Upstream has no
// equivalent. Spans live in Node.meta, so they survive Copy() and never affect generated SQL,
// HashKey, or Equal. A span means "where this node's text was in the ORIGINAL source at parse
// time" — a rewrite (qualify, transforms) copies the stale span along, so read spans off the
// freshly parsed tree.
const metaSpanKey = "span"

// SetSpan records the node's source span: rune offsets into the SQL string it was parsed from,
// start inclusive, end exclusive (token Start/End are both inclusive; this is [Start, End+1)).
func (n *Node) SetSpan(start, end int) {
	n.Meta()[metaSpanKey] = [2]int{start, end}
}

// Span returns the source span recorded by SetSpan; ok is false when the node has none.
func (n *Node) Span() (start, end int, ok bool) {
	span, ok := n.MetaGet(metaSpanKey).([2]int)
	if !ok {
		return 0, 0, false
	}
	return span[0], span[1], true
}

// SpanText returns the verbatim source slice for e's span within sql — the exact original
// spelling, quotes and casing included. sql must be the same string the expression was parsed
// from; ok is false when e has no span or the span does not fit sql.
func SpanText(sql string, e Expression) (string, bool) {
	if e == nil {
		return "", false
	}
	start, end, ok := e.Span()
	if !ok || start < 0 || start >= end {
		return "", false
	}
	runes := []rune(sql)
	if end > len(runes) {
		return "", false
	}
	return string(runes[start:end]), true
}
