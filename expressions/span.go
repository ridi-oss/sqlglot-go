package expressions

// Source spans (Go-only, DEVIATIONS.md §6.2): the parser stamps selected nodes with the rune
// span and verbatim text of their source (e.g. MySQL's unaliased-projection column label).
// They live in Node.meta — survive Copy(), never affect generated SQL, HashKey, or Equal —
// and describe the source at parse time: Replace drops them, so read off the fresh tree.
const (
	metaSpanKey     = "span"
	metaSpanTextKey = "spanText"
)

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

// SetSpanText records the node's verbatim source text — the exact original spelling, quotes,
// casing, and spacing included.
func (n *Node) SetSpanText(text string) {
	n.Meta()[metaSpanTextKey] = text
}

// SpanText returns the verbatim source text recorded by SetSpanText; ok is false when the node
// has none.
func (n *Node) SpanText() (text string, ok bool) {
	text, ok = n.MetaGet(metaSpanTextKey).(string)
	return text, ok
}
