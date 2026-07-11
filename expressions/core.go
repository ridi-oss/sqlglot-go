package expressions

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Args = map[string]any

type GenerateOptions struct {
	Dialect            string
	Pretty             bool
	Identify           any
	Normalize          bool
	NormalizeFunctions any
	LeadingComma       bool
	MaxTextWidth       int
	Comments           *bool
}

var GenerateFunc func(Expression, GenerateOptions) (string, error)

type Expression interface {
	Kind() Kind
	Arg(key string) any
	Set(key string, val any)
	SetAt(key string, val any, index int, overwrite bool)
	Append(key string, val Expression)
	This() Expression
	Expr() Expression
	Expressions() []Expression
	Text(key string) string
	Name() string
	Alias() string
	AliasOrName() string
	OutputName() string
	Parent() Expression
	ArgKey() string
	Index() int
	SetParent(p Expression, key string, idx int)
	Comments() []string
	AddComments(cs []string, prepend bool)
	PopComments() []string
	Meta() map[string]any
	MetaGet(key string) any
	Is(Trait) bool
	IsPrimitive() bool
	IsLeaf() bool
	Find(t Target) Expression
	FindAll(t Target, bfs ...bool) []Expression
	FindAncestor(kinds ...Kind) Expression
	ParentSelect() Expression
	Root() Expression
	Walk(bfs ...bool) []Expression
	WalkWithPrune(bfs bool, prune func(Expression) bool) []Expression
	Copy() Expression
	Replace(Expression) Expression
	Pop() Expression
	HashKey() uint64
	Equal(Expression) bool
	SQL(opts GenerateOptions) (string, error)
	ToS() string
	ErrorMessages(args []any) []string
	IsString() bool
	IsNumber() bool
	IsInt() bool
	IsStar() bool
	Unnest() Expression
	Unwrap() Expression
	SameParent() bool
	AliasColumnNames() []string
	Selects() []Expression
	NamedSelects() []string
	ToPy() any
	Parts() []Expression
	TableName() string
	DbName() string
	CatalogName() string
	Left() Expression
	Right() Expression
}

type Target interface{ matches(Expression) bool }

func (k Kind) matches(e Expression) bool  { return e != nil && e.Kind() == k }
func (t Trait) matches(e Expression) bool { return e != nil && e.Is(t) }

type ArgError struct{ Msg string }

func (e *ArgError) Error() string { return e.Msg }

type Node struct {
	kind     Kind
	args     map[string]any
	argOrder []string
	parent   Expression
	argKey   string
	index    int
	comments []string
	meta     map[string]any
	hash     *uint64
}

func New(k Kind, args Args) Expression {
	return newNode(k, args)
}

func newNode(k Kind, args Args) *Node {
	if args == nil {
		args = Args{}
	}
	n := &Node{kind: k, args: map[string]any{}, index: -1}
	seen := map[string]bool{}
	for _, spec := range argTypesFor(k) {
		if value, ok := args[spec.Key]; ok {
			n.args[spec.Key] = value
			n.argOrder = append(n.argOrder, spec.Key)
			seen[spec.Key] = true
		}
	}
	var extras []string
	for key := range args {
		if !seen[key] {
			extras = append(extras, key)
		}
	}
	sort.Strings(extras)
	for _, key := range extras {
		n.args[key] = args[key]
		n.argOrder = append(n.argOrder, key)
	}
	if !primitive[k] {
		for _, key := range n.argOrder {
			n.setParent(key, n.args[key], -1)
		}
	}
	return n
}

func newEmptyNode(k Kind) *Node {
	return &Node{kind: k, args: map[string]any{}, index: -1}
}

func argTypesFor(k Kind) []argSpec {
	if specs, ok := argTypes[k]; ok {
		return specs
	}
	return defaultArgTypes
}

func (n *Node) Kind() Kind { return n.kind }

func (n *Node) Arg(key string) any { return n.args[key] }

func (n *Node) Set(key string, val any) { n.set(key, val, nil, true) }

func (n *Node) SetAt(key string, val any, index int, overwrite bool) {
	n.set(key, val, &index, overwrite)
}

func (n *Node) Append(key string, val Expression) {
	n.invalidateHash()
	values, _ := n.args[key].([]Expression)
	if values == nil {
		values = []Expression{}
		if !n.hasArgKey(key) {
			n.argOrder = append(n.argOrder, key)
		}
	}
	if val != nil {
		val.SetParent(n, key, len(values))
	}
	values = append(values, val)
	n.args[key] = values
}

func (n *Node) set(key string, val any, index *int, overwrite bool) {
	n.invalidateHash()

	if index != nil {
		expressions := n.ExpressionsFor(key)
		idx := *index
		if idx < 0 || idx >= len(expressions) {
			return
		}
		if val == nil {
			expressions = append(expressions[:idx], expressions[idx+1:]...)
			for i := idx; i < len(expressions); i++ {
				expressions[i].SetParent(n, key, i)
			}
			n.args[key] = expressions
			return
		}
		if list, ok := val.([]Expression); ok {
			expressions = append(expressions[:idx], append(list, expressions[idx+1:]...)...)
		} else if expr, ok := val.(Expression); ok {
			if overwrite {
				expressions[idx] = expr
			} else {
				expressions = append(expressions[:idx], append([]Expression{expr}, expressions[idx:]...)...)
			}
		}
		val = expressions
	} else if val == nil {
		delete(n.args, key)
		n.dropArgKey(key)
		return
	}

	if !n.hasArgKey(key) {
		n.argOrder = append(n.argOrder, key)
	}
	n.args[key] = val
	n.setParent(key, val, -1)
}

func (n *Node) setParent(key string, value any, index int) {
	switch v := value.(type) {
	case Expression:
		if v != nil {
			v.SetParent(n, key, index)
		}
	case []Expression:
		for i, child := range v {
			if child != nil {
				child.SetParent(n, key, i)
			}
		}
	case []any:
		for i, child := range v {
			if expr, ok := child.(Expression); ok && expr != nil {
				expr.SetParent(n, key, i)
			}
		}
	}
}

func (n *Node) invalidateHash() {
	var node Expression = n
	for node != nil {
		if nn, ok := node.(*Node); ok {
			if nn.hash == nil {
				break
			}
			nn.hash = nil
			node = nn.parent
		} else {
			break
		}
	}
}

func (n *Node) hasArgKey(key string) bool {
	for _, existing := range n.argOrder {
		if existing == key {
			return true
		}
	}
	return false
}

func (n *Node) dropArgKey(key string) {
	for i, existing := range n.argOrder {
		if existing == key {
			n.argOrder = append(n.argOrder[:i], n.argOrder[i+1:]...)
			return
		}
	}
}

func (n *Node) This() Expression { return asExpression(n.args["this"]) }

func (n *Node) Expr() Expression { return asExpression(n.args["expression"]) }

func (n *Node) Expressions() []Expression { return n.ExpressionsFor("expressions") }

func (n *Node) ExpressionsFor(key string) []Expression {
	value := n.args[key]
	switch v := value.(type) {
	case []Expression:
		return v
	case []any:
		out := make([]Expression, 0, len(v))
		for _, item := range v {
			if expr, ok := item.(Expression); ok {
				out = append(out, expr)
			}
		}
		return out
	case Expression:
		if v == nil {
			return nil
		}
		return []Expression{v}
	default:
		return nil
	}
}

func (n *Node) Text(key string) string {
	field := n.args[key]
	switch v := field.(type) {
	case string:
		return v
	case Expression:
		if v == nil {
			return ""
		}
		switch v.Kind() {
		case KindIdentifier, KindLiteral, KindVar:
			if s, ok := v.Arg("this").(string); ok {
				return s
			}
			return fmt.Sprint(v.Arg("this"))
		case KindStar, KindNull:
			return v.Name()
		}
	}
	return ""
}

func (n *Node) Name() string {
	switch n.kind {
	case KindStar:
		return "*"
	case KindNull:
		return "NULL"
	case KindPlaceholder:
		if text := n.Text("this"); text != "" {
			return text
		}
		return "?"
	case KindDot:
		if expr := n.Expr(); expr != nil {
			return expr.Name()
		}
	case KindFrom, KindOrdered:
		if this := n.This(); this != nil {
			return this.Name()
		}
	case KindJoin:
		if this := n.This(); this != nil {
			return this.AliasOrName()
		}
	case KindCast, KindTryCast, KindCastToStrType:
		if this := n.This(); this != nil {
			return this.Name()
		}
	}
	return n.Text("this")
}

func (n *Node) Alias() string {
	alias := n.args["alias"]
	if expr, ok := alias.(Expression); ok && expr != nil {
		return expr.Name()
	}
	return n.Text("alias")
}

func (n *Node) AliasOrName() string {
	if alias := n.Alias(); alias != "" {
		return alias
	}
	return n.Name()
}

func (n *Node) OutputName() string {
	switch n.kind {
	case KindColumn, KindLiteral, KindIdentifier, KindStar, KindDot, KindTableColumn:
		return n.Name()
	case KindAlias:
		return n.Alias()
	case KindParen:
		if this := n.This(); this != nil {
			return this.Name()
		}
	case KindSubquery:
		return n.Alias()
	case KindCast, KindTryCast, KindCastToStrType:
		return n.Name()
	}
	return ""
}

func (n *Node) Parent() Expression { return n.parent }
func (n *Node) ArgKey() string     { return n.argKey }
func (n *Node) Index() int         { return n.index }

func (n *Node) SetParent(p Expression, key string, idx int) {
	n.parent = p
	n.argKey = key
	n.index = idx
}

func (n *Node) Comments() []string { return n.comments }

func (n *Node) AddComments(comments []string, prepend bool) {
	if len(comments) == 0 {
		return
	}
	if prepend {
		n.comments = append(append([]string(nil), comments...), n.comments...)
	} else {
		n.comments = append(n.comments, comments...)
	}
}

func (n *Node) PopComments() []string {
	comments := n.comments
	n.comments = nil
	return comments
}

// Meta returns the node's mutable metadata map, allocating it on first access. Ports upstream
// Expression.meta (core.py:991). Metadata (e.g. "is_table", "case_sensitive") travels with the
// node but is never part of its generated SQL or its ToS() repr.
func (n *Node) Meta() map[string]any {
	if n.meta == nil {
		n.meta = map[string]any{}
	}
	return n.meta
}

// MetaGet reads a metadata value without allocating the map (unlike Meta). Ports upstream
// Expression.meta_get (core.py:996); returns nil when the key (or the map) is unset.
func (n *Node) MetaGet(key string) any {
	if n.meta == nil {
		return nil
	}
	return n.meta[key]
}

func (n *Node) Is(trait Trait) bool { return traitsOf[n.kind]&trait != 0 }

func (n *Node) IsPrimitive() bool { return primitive[n.kind] }

func (n *Node) IsLeaf() bool {
	for _, value := range n.args {
		switch v := value.(type) {
		case Expression:
			if v != nil {
				return false
			}
		case []Expression:
			if len(v) > 0 {
				return false
			}
		case []any:
			if len(v) > 0 {
				return false
			}
		case []string:
			// Upstream is_leaf (core.py:988) treats ANY non-empty list as making
			// a node non-leaf (`type(v) is list`), including a list of strings
			// (e.g. AnalyzeWith.expressions). Without this, such nodes inline in ToS().
			if len(v) > 0 {
				return false
			}
		}
	}
	return true
}

func (n *Node) iterExpressions(reverse bool) []Expression {
	order := n.argOrder
	out := []Expression{}
	if reverse {
		for i := len(order) - 1; i >= 0; i-- {
			out = append(out, expressionsFromValue(n.args[order[i]], true)...)
		}
		return out
	}
	for _, key := range order {
		out = append(out, expressionsFromValue(n.args[key], false)...)
	}
	return out
}

func expressionsFromValue(value any, reverse bool) []Expression {
	switch v := value.(type) {
	case []Expression:
		out := make([]Expression, 0, len(v))
		if reverse {
			for i := len(v) - 1; i >= 0; i-- {
				if v[i] != nil {
					out = append(out, v[i])
				}
			}
		} else {
			for _, expr := range v {
				if expr != nil {
					out = append(out, expr)
				}
			}
		}
		return out
	case []any:
		out := make([]Expression, 0, len(v))
		if reverse {
			for i := len(v) - 1; i >= 0; i-- {
				if expr, ok := v[i].(Expression); ok && expr != nil {
					out = append(out, expr)
				}
			}
		} else {
			for _, item := range v {
				if expr, ok := item.(Expression); ok && expr != nil {
					out = append(out, expr)
				}
			}
		}
		return out
	case Expression:
		if v != nil {
			return []Expression{v}
		}
	}
	return nil
}

func (n *Node) Find(t Target) Expression {
	all := n.FindAll(t)
	if len(all) == 0 {
		return nil
	}
	return all[0]
}

func (n *Node) FindAll(t Target, bfsOpt ...bool) []Expression {
	bfs := true
	if len(bfsOpt) > 0 {
		bfs = bfsOpt[0]
	}
	var out []Expression
	for _, expression := range n.Walk(bfs) {
		if t.matches(expression) {
			out = append(out, expression)
		}
	}
	return out
}

func (n *Node) FindAncestor(kinds ...Kind) Expression {
	wanted := map[Kind]bool{}
	for _, kind := range kinds {
		wanted[kind] = true
	}
	ancestor := n.parent
	for ancestor != nil && !wanted[ancestor.Kind()] {
		ancestor = ancestor.Parent()
	}
	return ancestor
}

func (n *Node) ParentSelect() Expression { return n.FindAncestor(KindSelect) }

func (n *Node) Root() Expression {
	var expression Expression = n
	for expression.Parent() != nil {
		expression = expression.Parent()
	}
	return expression
}

func (n *Node) Walk(bfsOpt ...bool) []Expression {
	bfs := true
	if len(bfsOpt) > 0 {
		bfs = bfsOpt[0]
	}
	if bfs {
		return n.bfs()
	}
	return n.dfs()
}

func (n *Node) WalkWithPrune(bfs bool, prune func(Expression) bool) []Expression {
	if bfs {
		queue := []Expression{n}
		out := []Expression{}
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			out = append(out, node)
			if prune != nil && prune(node) {
				continue
			}
			if nn, ok := node.(*Node); ok {
				queue = append(queue, nn.iterExpressions(false)...)
			}
		}
		return out
	}

	stack := []Expression{n}
	out := []Expression{}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, node)
		if prune != nil && prune(node) {
			continue
		}
		if nn, ok := node.(*Node); ok {
			for _, child := range nn.iterExpressions(true) {
				stack = append(stack, child)
			}
		}
	}
	return out
}

func (n *Node) dfs() []Expression {
	stack := []Expression{n}
	out := []Expression{}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, node)
		if nn, ok := node.(*Node); ok {
			for _, child := range nn.iterExpressions(true) {
				stack = append(stack, child)
			}
		}
	}
	return out
}

func (n *Node) bfs() []Expression {
	queue := []Expression{n}
	out := []Expression{}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		out = append(out, node)
		if nn, ok := node.(*Node); ok {
			queue = append(queue, nn.iterExpressions(false)...)
		}
	}
	return out
}

func (n *Node) Copy() Expression {
	root := newEmptyNode(n.kind)
	stack := [][2]*Node{{n, root}}
	for len(stack) > 0 {
		pair := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node, copyNode := pair[0], pair[1]
		if node.comments != nil {
			copyNode.comments = append([]string(nil), node.comments...)
		}
		if node.meta != nil {
			// Ports upstream copy._meta = deepcopy(node._meta) (core.py:1013): the copy's
			// metadata must not alias the original's composite values (e.g. a []string like
			// upstream's "dot_parts").
			copyNode.meta = make(map[string]any, len(node.meta))
			for k, v := range node.meta {
				copyNode.meta[k] = deepCopyMetaValue(v)
			}
		}
		if node.hash != nil {
			h := *node.hash
			copyNode.hash = &h
		}
		for _, key := range node.argOrder {
			value := node.args[key]
			switch v := value.(type) {
			case Expression:
				if child, ok := v.(*Node); ok {
					childCopy := newEmptyNode(child.kind)
					copyNode.Set(key, childCopy)
					stack = append(stack, [2]*Node{child, childCopy})
				} else {
					copyNode.Set(key, v)
				}
			case []Expression:
				if !copyNode.hasArgKey(key) {
					copyNode.argOrder = append(copyNode.argOrder, key)
				}
				copyNode.args[key] = []Expression{}
				for _, childExpr := range v {
					if child, ok := childExpr.(*Node); ok {
						childCopy := newEmptyNode(child.kind)
						copyNode.Append(key, childCopy)
						stack = append(stack, [2]*Node{child, childCopy})
					} else if childExpr != nil {
						copyNode.Append(key, childExpr)
					}
				}
			default:
				copyNode.Set(key, cloneScalar(value))
			}
		}
	}
	return root
}

func (n *Node) Replace(expression Expression) Expression {
	parent := n.parent
	if parent == nil || parent == expression {
		return expression
	}
	key := n.argKey
	if key != "" {
		// Upstream calls parent.set(key, expression, self.index), where self.index is
		// None for single-value args and an int for list elements. We encode "no index"
		// as index<0, so route single-value args through Set (the index-nil path:
		// plain assignment, or delete when expression is nil). SetAt's list guard would
		// otherwise reject index<0 and silently drop the mutation — the tree-rewrite
		// primitive every optimizer pass depends on.
		if n.index < 0 {
			parent.Set(key, expression)
		} else {
			parent.SetAt(key, expression, n.index, true)
		}
	}
	if expression != n {
		n.parent = nil
		n.argKey = ""
		n.index = -1
	}
	return expression
}

func (n *Node) Pop() Expression {
	n.Replace(nil)
	return n
}

func (n *Node) HashKey() uint64 {
	if n.hash == nil {
		nodes := []*Node{}
		stack := []*Node{n}
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			nodes = append(nodes, node)
			for _, value := range node.args {
				for _, child := range expressionsFromValue(value, false) {
					if cn, ok := child.(*Node); ok && cn.hash == nil {
						stack = append(stack, cn)
					}
				}
			}
		}
		for i := len(nodes) - 1; i >= 0; i-- {
			node := nodes[i]
			hash := hashString(className[node.kind])
			keys := make([]string, 0, len(node.args))
			for key := range node.args {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				value := node.args[key]
				if hashRaw[node.kind] {
					if truthy(value) {
						hash = hashCombine(hash, key, stableHashAny(value, true))
					}
					continue
				}
				if list, ok := value.([]Expression); ok {
					for _, item := range list {
						if item != nil {
							hash = hashCombine(hash, key, stableHashAny(item, false))
						} else {
							hash = hashCombine(hash, key)
						}
					}
				} else if list, ok := value.([]any); ok {
					for _, item := range list {
						if item != nil && !isFalse(item) {
							hash = hashCombine(hash, key, stableHashAny(item, false))
						} else {
							hash = hashCombine(hash, key)
						}
					}
				} else if value != nil && !isFalse(value) {
					hash = hashCombine(hash, key, stableHashAny(value, false))
				}
			}
			node.hash = &hash
		}
	}
	return *n.hash
}

func (n *Node) Equal(other Expression) bool {
	if other == nil {
		return false
	}
	if on, ok := other.(*Node); ok && n == on {
		return true
	}
	return n.kind == other.Kind() && n.HashKey() == other.HashKey()
}

func (n *Node) SQL(opts GenerateOptions) (string, error) {
	if GenerateFunc == nil {
		return "", fmt.Errorf("expressions.GenerateFunc is not configured")
	}
	return GenerateFunc(n, opts)
}

func (n *Node) ToS() string { return toS(n, false, 0, false) }

func (n *Node) ErrorMessages(args []any) []string {
	allowed := map[string]bool{}
	for _, spec := range argTypesFor(n.kind) {
		allowed[spec.Key] = true
	}
	for _, key := range n.argOrder {
		if !allowed[key] {
			panic(&ArgError{Msg: fmt.Sprintf("Unexpected keyword: '%s' for %s", key, className[n.kind])})
		}
	}
	var messages []string
	argTypes := argTypesFor(n.kind)
	for _, spec := range argTypes {
		if !spec.Required {
			continue
		}
		value := n.args[spec.Key]
		if value == nil || isEmptyList(value) {
			messages = append(messages, fmt.Sprintf("Required keyword: '%s' missing for %s", spec.Key, className[n.kind]))
		}
	}
	// Upstream error_messages (core.py:1320) rejects a Func given more positional args
	// than it has arg_types, unless it is var-len. `args` is the raw list passed to the
	// function builder (validate_expression); nil means the caller opted out of the check.
	if len(args) > len(argTypes) && n.Is(TraitFunc) && !varLenArgs[n.kind] {
		messages = append(messages, fmt.Sprintf(
			"The number of provided arguments (%d) is greater than the maximum number of supported arguments (%d)",
			len(args), len(argTypes)))
	}
	return messages
}

func (n *Node) IsString() bool {
	value, _ := n.args["is_string"].(bool)
	return n.kind == KindLiteral && value
}

func (n *Node) IsNumber() bool {
	if n.kind == KindLiteral {
		isString, _ := n.args["is_string"].(bool)
		return !isString
	}
	return n.kind == KindNeg && n.This() != nil && n.This().IsNumber()
}

func (n *Node) IsInt() bool {
	if !n.IsNumber() {
		return false
	}
	_, ok := n.ToPy().(int64)
	return ok
}

func (n *Node) IsStar() bool {
	switch n.kind {
	case KindStar:
		return true
	case KindColumn:
		return n.This() != nil && n.This().Kind() == KindStar
	case KindDot:
		return n.Expr() != nil && n.Expr().IsStar()
	}
	return false
}

func (n *Node) Unnest() Expression {
	var expr Expression = n
	if n.kind == KindSubquery {
		for expr != nil && expr.Kind() == KindSubquery {
			next := expr.This()
			if next == nil {
				break
			}
			expr = next
		}
		return expr
	}
	for expr != nil && expr.Kind() == KindParen {
		next := expr.This()
		if next == nil {
			break
		}
		expr = next
	}
	return expr
}

func (n *Node) Unwrap() Expression {
	var expr Expression = n
	for expr != nil && expr.SameParent() && IsWrapper(expr) {
		expr = expr.Parent()
	}
	return expr
}

func (n *Node) SameParent() bool {
	return n.parent != nil && n.parent.Kind() == n.kind
}

func (n *Node) AliasColumnNames() []string {
	alias := asExpression(n.args["alias"])
	aliasNode, ok := alias.(*Node)
	if !ok || aliasNode == nil {
		return nil
	}
	columns := aliasNode.ExpressionsFor("columns")
	out := make([]string, 0, len(columns))
	for _, column := range columns {
		out = append(out, column.Name())
	}
	return out
}

func (n *Node) Selects() []Expression {
	switch n.kind {
	case KindSelect:
		return n.Expressions()
	case KindUnion, KindExcept, KindIntersect:
		var expr Expression = n
		for expr != nil && expr.Is(TraitSetOperation) {
			expr = expr.This()
			if expr != nil {
				expr = expr.Unnest()
			}
		}
		if expr != nil {
			return expr.Selects()
		}
	case KindSubquery:
		if this := n.This(); this != nil && this.Is(TraitQuery) {
			return this.Selects()
		}
	case KindValues, KindUnnest, KindLateral:
		if alias, ok := asExpression(n.args["alias"]).(*Node); ok && alias != nil {
			return alias.ExpressionsFor("columns")
		}
	}
	return nil
}

func (n *Node) NamedSelects() []string {
	switch n.kind {
	case KindSelect:
		var out []string
		for _, e := range n.Expressions() {
			if e.AliasOrName() != "" {
				out = append(out, e.OutputName())
			} else if e.Kind() == KindAliases {
				if aliases, ok := e.(*Node); ok {
					for _, alias := range aliases.ExpressionsFor("expressions") {
						out = append(out, alias.Name())
					}
				}
			}
		}
		return out
	case KindUnion, KindExcept, KindIntersect:
		// Mirror SetOperation.named_selects (query.py:1050): unnest down to the
		// leftmost query, then return _named_selects (query.py:90) — the unfiltered
		// output_name of every projection, with no Aliases expansion. This differs
		// from Select.named_selects, which filters unnamed projections and expands
		// Aliases.
		var expr Expression = n
		for expr != nil && expr.Is(TraitSetOperation) {
			expr = expr.This()
			if expr != nil {
				expr = expr.Unnest()
			}
		}
		if expr != nil {
			out := make([]string, 0, len(expr.Selects()))
			for _, e := range expr.Selects() {
				out = append(out, e.OutputName())
			}
			return out
		}
	case KindSubquery, KindValues, KindUnnest, KindLateral:
		var out []string
		for _, e := range n.Selects() {
			out = append(out, e.OutputName())
		}
		return out
	}
	return nil
}

func (n *Node) ToPy() any {
	switch n.kind {
	case KindLiteral:
		text := n.Text("this")
		if n.IsNumber() {
			if i, err := strconv.ParseInt(text, 10, 64); err == nil {
				return i
			}
			if f, err := strconv.ParseFloat(text, 64); err == nil {
				return f
			}
		}
		return text
	case KindNull:
		return nil
	case KindBoolean:
		return n.args["this"]
	case KindNeg:
		if n.This() != nil && n.This().IsNumber() {
			switch v := n.This().ToPy().(type) {
			case int64:
				return -v
			case float64:
				return -v
			}
		}
	}
	panic(fmt.Sprintf("%s cannot be converted to a Go object", n.ToS()))
}

func (n *Node) Parts() []Expression {
	switch n.kind {
	case KindColumn:
		parts := []Expression{}
		for _, key := range []string{"catalog", "db", "table", "this"} {
			if expr := asExpression(n.args[key]); expr != nil {
				parts = append(parts, expr)
			}
		}
		return parts
	case KindTable:
		parts := []Expression{}
		for _, key := range []string{"catalog", "db", "this"} {
			part := asExpression(n.args[key])
			if part == nil {
				continue
			}
			if part.Kind() == KindDot {
				parts = append(parts, part.Parts()...)
			} else {
				parts = append(parts, part)
			}
		}
		return parts
	case KindDot:
		parts := []Expression{}
		if this := n.This(); this != nil {
			if this.Kind() == KindDot {
				parts = append(parts, this.Parts()...)
			} else {
				parts = append(parts, this)
			}
		}
		if expr := n.Expr(); expr != nil {
			parts = append(parts, expr)
		}
		return parts
	}
	return nil
}

var TablePartKeys = []string{"this", "db", "catalog"}

func applyKwargs(e Expression, kwargs Args) Expression {
	// kwargs is empty or single-key in the M1 surface, so map iteration order is immaterial.
	for k, v := range kwargs {
		e.Set(k, v)
	}
	return e
}

func (n *Node) TableName() string   { return n.Text("table") }
func (n *Node) DbName() string      { return n.Text("db") }
func (n *Node) CatalogName() string { return n.Text("catalog") }
func (n *Node) Left() Expression    { return n.This() }
func (n *Node) Right() Expression   { return n.Expr() }

func asExpression(value any) Expression {
	if expr, ok := value.(Expression); ok {
		return expr
	}
	return nil
}

func cloneScalar(value any) any {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	default:
		return v
	}
}

// deepCopyMetaValue recursively copies a Node.meta value so a copied node never aliases the
// original's mutable metadata (ports the deepcopy in upstream Expression.copy, core.py:1013).
// Scalars are immutable and returned as-is; the composite shapes metadata actually uses
// (slices, string-keyed maps) are cloned recursively.
func deepCopyMetaValue(value any) any {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = deepCopyMetaValue(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = deepCopyMetaValue(e)
		}
		return out
	default:
		return v
	}
}

func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func hashCombine(seed uint64, parts ...any) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.FormatUint(seed, 10)))
	for _, part := range parts {
		_, _ = h.Write([]byte("|"))
		_, _ = h.Write([]byte(fmt.Sprintf("%T:%v", part, part)))
	}
	return h.Sum64()
}

func stableHashAny(value any, raw bool) uint64 {
	switch v := value.(type) {
	case Expression:
		if v == nil {
			return 0
		}
		return v.HashKey()
	case string:
		if !raw {
			v = strings.ToLower(v)
		}
		return hashString(v)
	case bool:
		if v {
			return hashString("true")
		}
		return hashString("false")
	case int:
		return hashString(strconv.Itoa(v))
	case int64:
		return hashString(strconv.FormatInt(v, 10))
	case float64:
		return hashString(strconv.FormatFloat(v, 'g', -1, 64))
	case []Expression:
		h := hashString("list")
		for _, item := range v {
			h = hashCombine(h, stableHashAny(item, raw))
		}
		return h
	default:
		return hashString(fmt.Sprintf("%v", v))
	}
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case []Expression:
		return len(v) > 0
	case []any:
		return len(v) > 0
	}
	return true
}

func isFalse(value any) bool {
	b, ok := value.(bool)
	return ok && !b
}

func isEmptyList(value any) bool {
	switch v := value.(type) {
	case []Expression:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case []string:
		// Upstream _to_s filters `v != []` for every list type; an empty list of
		// strings (e.g. an unset UniqueColumnConstraint.options) must be dropped too.
		return len(v) == 0
	}
	return false
}

func toS(node any, verbose bool, level int, reprStr bool) string {
	indent := "\n" + strings.Repeat("  ", level+1)
	delim := "," + indent

	if expr, ok := node.(*Node); ok {
		keys := make([]string, 0, len(expr.argOrder)+2)
		for _, key := range expr.argOrder {
			value := expr.args[key]
			if verbose || (value != nil && !isEmptyList(value)) {
				keys = append(keys, key)
			}
		}
		if verbose || len(expr.comments) > 0 {
			keys = append(keys, "_comments")
		}
		if expr.IsLeaf() {
			indent = ""
			delim = ", "
		}
		reprStr = expr.IsString() || (expr.kind == KindIdentifier && boolArg(expr.args["quoted"]))
		items := make([]string, 0, len(keys))
		for _, key := range keys {
			value := any(nil)
			if key == "_comments" {
				value = expr.comments
			} else {
				value = expr.args[key]
			}
			items = append(items, fmt.Sprintf("%s=%s", key, toSAny(value, verbose, level+1, reprStr)))
		}
		return fmt.Sprintf("%s(%s%s)", className[expr.kind], indent, strings.Join(items, delim))
	}
	if expr, ok := node.(Expression); ok && expr != nil {
		if nn, ok := expr.(*Node); ok {
			return toS(nn, verbose, level, reprStr)
		}
	}
	return toSAny(node, verbose, level, reprStr)
}

func toSAny(node any, verbose bool, level int, reprStr bool) string {
	indent := "\n" + strings.Repeat("  ", level+1)
	delim := "," + indent
	switch v := node.(type) {
	case *Node:
		return toS(v, verbose, level, reprStr)
	case Expression:
		if v == nil {
			return "None"
		}
		if nn, ok := v.(*Node); ok {
			return toS(nn, verbose, level, reprStr)
		}
	case []Expression:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, toSAny(item, verbose, level+1, false))
		}
		if len(items) > 0 {
			return "[" + indent + strings.Join(items, delim) + "]"
		}
		return "[]"
	case []string:
		// Upstream _to_s (core.py) renders a list of strings (e.g. _comments) like
		// any list node: multi-line with per-item indentation, dedented, and
		// unquoted (repr_str defaults to False for list elements) -> not `[' c']`.
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, toSAny(item, verbose, level+1, false))
		}
		if len(items) > 0 {
			return "[" + indent + strings.Join(items, delim) + "]"
		}
		return "[]"
	case string:
		if reprStr {
			return pyStringRepr(v)
		}
		return indentMultiline(v, indent)
	case bool:
		if v {
			return "True"
		}
		return "False"
	case DType:
		// Upstream stores DataType.this as the DType enum, whose str() is
		// "DType.<MEMBER_NAME>" (Enum default). ToS()/repr() renders that verbatim,
		// so a bare Go string("INT") must become "DType.INT". The enum member NAME
		// (not its value) is used: USERDEFINED is the sole member whose value
		// ("USER-DEFINED") differs from its name in v30.12.0.
		name := string(v)
		if v == DTypeUserDefined {
			name = "USERDEFINED"
		}
		return "DType." + name
	case nil:
		return "None"
	}
	return indentMultiline(fmt.Sprint(node), indent)
}

func indentMultiline(s, indent string) string {
	// Port of upstream's final _to_s branch:
	//   indent.join(textwrap.dedent(str(node).strip("\n")).splitlines())
	s = pyDedent(strings.Trim(s, "\n"))
	lines := strings.Split(s, "\n")
	return strings.Join(lines, indent)
}

// pyDedent mirrors Python's textwrap.dedent: it removes the longest common
// leading whitespace (spaces/tabs) shared by all non-whitespace-only lines, and
// normalizes whitespace-only lines to empty.
func pyDedent(s string) string {
	lines := strings.Split(s, "\n")
	margin := ""
	haveMargin := false
	for _, line := range lines {
		stripped := strings.TrimLeft(line, " \t")
		if stripped == "" {
			// whitespace-only lines don't contribute to the common margin
			continue
		}
		indent := line[:len(line)-len(stripped)]
		if !haveMargin {
			margin = indent
			haveMargin = true
		} else {
			margin = commonPrefix(margin, indent)
		}
		if margin == "" {
			break
		}
	}
	for i, line := range lines {
		if strings.TrimLeft(line, " \t") == "" {
			lines[i] = ""
		} else if strings.HasPrefix(line, margin) {
			lines[i] = line[len(margin):]
		}
	}
	return strings.Join(lines, "\n")
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

func boolArg(value any) bool {
	b, _ := value.(bool)
	return b
}

func pyStringRepr(s string) string {
	quote := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		quote = "\""
	}
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	if quote == "'" {
		escaped = strings.ReplaceAll(escaped, "'", "\\'")
	} else {
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	}
	return quote + escaped + quote
}

func Column(args Args) Expression      { return newNode(KindColumn, args) }
func Literal(args Args) Expression     { return newNode(KindLiteral, args) }
func Identifier(args Args) Expression  { return newNode(KindIdentifier, args) }
func Star(args Args) Expression        { return newNode(KindStar, args) }
func AliasNode(args Args) Expression   { return newNode(KindAlias, args) }
func Aliases(args Args) Expression     { return newNode(KindAliases, args) }
func Dot(args Args) Expression         { return newNode(KindDot, args) }
func Null() Expression                 { return newNode(KindNull, nil) }
func Boolean(args Args) Expression     { return newNode(KindBoolean, args) }
func Var(args Args) Expression         { return newNode(KindVar, args) }
func Paren(args Args) Expression       { return newNode(KindParen, args) }
func Neg(args Args) Expression         { return newNode(KindNeg, args) }
func Not(args Args) Expression         { return newNode(KindNot, args) }
func Add(args Args) Expression         { return newNode(KindAdd, args) }
func Sub(args Args) Expression         { return newNode(KindSub, args) }
func Mul(args Args) Expression         { return newNode(KindMul, args) }
func Div(args Args) Expression         { return newNode(KindDiv, args) }
func Mod(args Args) Expression         { return newNode(KindMod, args) }
func EQ(args Args) Expression          { return newNode(KindEQ, args) }
func NEQ(args Args) Expression         { return newNode(KindNEQ, args) }
func NullSafeEQ(args Args) Expression  { return newNode(KindNullSafeEQ, args) }
func GT(args Args) Expression          { return newNode(KindGT, args) }
func GTE(args Args) Expression         { return newNode(KindGTE, args) }
func LT(args Args) Expression          { return newNode(KindLT, args) }
func LTE(args Args) Expression         { return newNode(KindLTE, args) }
func And(args Args) Expression         { return newNode(KindAnd, args) }
func Or(args Args) Expression          { return newNode(KindOr, args) }
func BitwiseAnd(args Args) Expression  { return newNode(KindBitwiseAnd, args) }
func BitwiseOr(args Args) Expression   { return newNode(KindBitwiseOr, args) }
func BitwiseXor(args Args) Expression  { return newNode(KindBitwiseXor, args) }
func DPipe(args Args) Expression       { return newNode(KindDPipe, args) }
func Between(args Args) Expression     { return newNode(KindBetween, args) }
func Is(args Args) Expression          { return newNode(KindIs, args) }
func Like(args Args) Expression        { return newNode(KindLike, args) }
func ILike(args Args) Expression       { return newNode(KindILike, args) }
func SimilarTo(args Args) Expression   { return newNode(KindSimilarTo, args) }
func Escape(args Args) Expression      { return newNode(KindEscape, args) }
func In(args Args) Expression          { return newNode(KindIn, args) }
func Case(args Args) Expression        { return newNode(KindCase, args) }
func If(args Args) Expression          { return newNode(KindIf, args) }
func Exists(args Args) Expression      { return newNode(KindExists, args) }
func Any(args Args) Expression         { return newNode(KindAny, args) }
func All(args Args) Expression         { return newNode(KindAll, args) }
func NullSafeNEQ(args Args) Expression { return newNode(KindNullSafeNEQ, args) }
func Anonymous(args Args) Expression   { return newNode(KindAnonymous, args) }
func Hint(args Args) Expression        { return newNode(KindHint, args) }
func Placeholder(args Args) Expression { return newNode(KindPlaceholder, args) }
func Parameter(args Args) Expression   { return newNode(KindParameter, args) }
func RawString(args Args) Expression   { return newNode(KindRawString, args) }
func AtTimeZone(args Args) Expression  { return newNode(KindAtTimeZone, args) }

func LiteralNumber(number any) Expression {
	text := fmt.Sprint(number)
	if f, ok := number.(float64); ok && math.Trunc(f) == f {
		text = strconv.FormatFloat(f, 'f', 0, 64)
	}
	if strings.HasPrefix(text, "-") {
		lit := Literal(Args{"this": strings.TrimPrefix(text, "-"), "is_string": false})
		return Neg(Args{"this": lit})
	}
	return Literal(Args{"this": text, "is_string": false})
}

func LiteralString(s any) Expression {
	return Literal(Args{"this": fmt.Sprint(s), "is_string": true})
}

func DotBuild(expressions []Expression) Expression {
	if len(expressions) < 2 {
		panic("Dot requires >= 2 expressions.")
	}
	current := expressions[0]
	for _, expression := range expressions[1:] {
		current = Dot(Args{"this": current, "expression": expression})
	}
	return current
}
