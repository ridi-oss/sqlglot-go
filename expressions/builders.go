package expressions

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

var safeIdentifierRE = regexp.MustCompile(`^[_a-zA-Z]\w*$`)

func IsSafeIdentifier(name string) bool { return safeIdentifierRE.MatchString(name) }

var MaybeParseFunc func(sql string, dialect string) (Expression, error)
var ParseIntoFunc func(sql string, dialect string, into Kind, ignoreErrors bool) (Expression, error)

func ToIdentifier(name any, quoted ...bool) Expression {
	if name == nil {
		return nil
	}
	if expr, ok := name.(Expression); ok {
		if expr.Kind() == KindIdentifier {
			return expr
		}
	}
	text, ok := name.(string)
	if !ok {
		return nil
	}
	quote := !safeIdentifierRE.MatchString(text)
	if len(quoted) > 0 {
		quote = quoted[0]
	}
	return Identifier(Args{"this": text, "quoted": quote})
}

func AliasExpr(expression any, alias any, table bool, quoted ...bool) Expression {
	expr := MaybeParse(expression, "", false)
	aliasExpr := ToIdentifier(alias, quoted...)
	if table {
		expr.Set("alias", TableAlias(Args{"this": aliasExpr}))
		return expr
	}
	if supportsArg(expr.Kind(), "alias") && expr.Kind() != KindExpression {
		expr.Set("alias", aliasExpr)
		return expr
	}
	return AliasNode(Args{"this": expr, "alias": aliasExpr})
}

func Column_(col any, table any, db any, catalog any, fields []any, quoted ...bool) Expression {
	var colExpr Expression
	if expr, ok := col.(Expression); ok && expr.Kind() == KindStar {
		colExpr = expr
	} else {
		colExpr = ToIdentifier(col, quoted...)
	}
	this := Column(Args{
		"this":    colExpr,
		"table":   ToIdentifier(table, quoted...),
		"db":      ToIdentifier(db, quoted...),
		"catalog": ToIdentifier(catalog, quoted...),
	})
	if len(fields) > 0 {
		parts := []Expression{this}
		for _, field := range fields {
			parts = append(parts, ToIdentifier(field, quoted...))
		}
		return DotBuild(parts)
	}
	return this
}

func Convert(value any, copyValue bool) Expression {
	if expr, ok := value.(Expression); ok {
		if copyValue {
			return expr.Copy()
		}
		return expr
	}
	switch v := value.(type) {
	case string:
		return LiteralString(v)
	case bool:
		return Boolean(Args{"this": v})
	case nil:
		return Null()
	case int:
		return LiteralNumber(v)
	case int64:
		return LiteralNumber(v)
	case float64:
		if math.IsNaN(v) {
			return Null()
		}
		return LiteralNumber(v)
	case float32:
		return LiteralNumber(v)
	}
	panic(fmt.Sprintf("Cannot convert %v", value))
}

func MaybeParse(sqlOrExpression any, dialect string, copyValue bool) Expression {
	if expr, ok := sqlOrExpression.(Expression); ok {
		if copyValue {
			return expr.Copy()
		}
		return expr
	}
	if sqlOrExpression == nil {
		panic("SQL cannot be None")
	}
	if MaybeParseFunc == nil {
		return Convert(sqlOrExpression, copyValue)
	}
	expr, err := MaybeParseFunc(fmt.Sprint(sqlOrExpression), dialect)
	if err != nil {
		panic(err)
	}
	return expr
}

func maybeParseInto(sqlOrExpr any, dialect string, into Kind, copyValue bool) (Expression, error) {
	if expr, ok := sqlOrExpr.(Expression); ok {
		return maybeCopy(expr, copyValue), nil
	}
	if ParseIntoFunc == nil {
		return nil, fmt.Errorf("expressions.ParseIntoFunc is not configured")
	}
	return ParseIntoFunc(fmt.Sprint(sqlOrExpr), dialect, into, false)
}

func presentPart(v any) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return s != ""
	}
	return true
}

func Table_(table, db, catalog any, quoted *bool, alias any) Expression {
	args := Args{}
	if presentPart(table) {
		if quoted != nil {
			args["this"] = ToIdentifier(table, *quoted)
		} else {
			args["this"] = ToIdentifier(table)
		}
	}
	if presentPart(db) {
		if quoted != nil {
			args["db"] = ToIdentifier(db, *quoted)
		} else {
			args["db"] = ToIdentifier(db)
		}
	}
	if presentPart(catalog) {
		if quoted != nil {
			args["catalog"] = ToIdentifier(catalog, *quoted)
		} else {
			args["catalog"] = ToIdentifier(catalog)
		}
	}
	if presentPart(alias) {
		args["alias"] = TableAlias(Args{"this": ToIdentifier(alias)})
	}
	return Table(args)
}

func ToTable(sqlPath any, dialect string, copyValue bool, kwargs Args) (Expression, error) {
	if expr, ok := sqlPath.(Expression); ok && expr.Kind() == KindTable {
		return maybeCopy(expr, copyValue), nil
	}
	table, err := maybeParseInto(sqlPath, dialect, KindTable, true)
	if err != nil {
		s := fmt.Sprint(sqlPath)
		parts := splitNumWords(s, ".", 3, true)
		catalog, db, this := parts[0], parts[1], parts[2]
		if this == "" {
			return nil, err
		}
		table = Table_(this, db, catalog, nil, nil)
	}
	return applyKwargs(table, kwargs), nil
}

func ParseIdentifier(name any, dialect string) Expression {
	if s, ok := name.(string); ok && safeIdentifierRE.MatchString(s) {
		return Identifier(Args{"this": s, "quoted": false})
	}
	expr, err := maybeParseInto(name, dialect, KindIdentifier, false)
	if err != nil {
		return ToIdentifier(name)
	}
	return expr
}

func splitNumWords(value, sep string, minNumWords int, fillFromStart bool) []string {
	words := strings.Split(value, sep)
	missing := minNumWords - len(words)
	if missing <= 0 {
		return words
	}
	padding := make([]string, missing)
	if fillFromStart {
		return append(padding, words...)
	}
	return append(words, padding...)
}

func Condition(expression any, dialect string, copyValue bool) Expression {
	return MaybeParse(expression, dialect, copyValue)
}

func And_(expressions ...Expression) Expression {
	return combine(expressions, KindAnd, true)
}

func Or_(expressions ...Expression) Expression {
	return combine(expressions, KindOr, true)
}

func Not_(expression Expression, copyValue bool) Expression {
	if copyValue && expression != nil {
		expression = expression.Copy()
	}
	return Not(Args{"this": wrap(expression, TraitConnector)})
}

func combine(expressions []Expression, kind Kind, wrapOperands bool) Expression {
	filtered := make([]Expression, 0, len(expressions))
	for _, expression := range expressions {
		if expression != nil {
			filtered = append(filtered, expression)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	this := filtered[0]
	if len(filtered) > 1 && wrapOperands {
		this = wrap(this, TraitConnector)
	}
	for _, expression := range filtered[1:] {
		if wrapOperands {
			expression = wrap(expression, TraitConnector)
		}
		args := Args{"this": this, "expression": expression}
		if kind == KindAnd {
			this = And(args)
		} else {
			this = Or(args)
		}
	}
	return this
}

func wrap(expression Expression, trait Trait) Expression {
	if expression != nil && expression.Is(trait) {
		return Paren(Args{"this": expression})
	}
	return expression
}

func SupportsArg(kind Kind, key string) bool { return supportsArg(kind, key) }

func IsWrapper(e Expression) bool {
	n, ok := e.(*Node)
	if !ok || n.kind != KindSubquery {
		return false
	}
	// newNode doesn't store nil args, so truthy matches upstream's "v is not None" wrapper check.
	for _, key := range n.argOrder {
		if key != "this" && truthy(n.args[key]) {
			return false
		}
	}
	return true
}

// ColumnsToDot mirrors the transform in _parse_column_ops (parser.py:6796-6799):
//
//	this.transform(lambda n: n.to_dot(include_dots=False) if isinstance(n, exp.Column) else n)
//
// Every Column descendant (not just the root) is rewritten to its dot form via
// Column.to_dot(include_dots=False): a Dot chain of the column's ordered parts, or the
// single part when there is only one. It returns a transformed copy; the input is unmodified.
func ColumnsToDot(root Expression) Expression {
	if root == nil {
		return nil
	}
	return transformColumnsToDot(root.Copy())
}

func transformColumnsToDot(node Expression) Expression {
	n, ok := node.(*Node)
	if !ok {
		return node
	}
	// Post-order (children before self), matching Expression.transform's bottom-up walk.
	for _, key := range append([]string(nil), n.argOrder...) {
		switch v := n.args[key].(type) {
		case Expression:
			if rewritten := transformColumnsToDot(v); rewritten != v {
				n.Set(key, rewritten)
			}
		case []Expression:
			for i, child := range v {
				if rewritten := transformColumnsToDot(child); rewritten != child {
					n.SetAt(key, rewritten, i, true)
				}
			}
		}
	}
	if n.kind == KindColumn {
		return columnPartsToDot(n)
	}
	return n
}

func columnPartsToDot(col *Node) Expression {
	parts := col.Parts()
	switch len(parts) {
	case 0:
		return col
	case 1:
		return parts[0]
	default:
		copies := make([]Expression, len(parts))
		for i, part := range parts {
			copies[i] = part.Copy()
		}
		return DotBuild(copies)
	}
}

func supportsArg(kind Kind, key string) bool {
	for _, spec := range argTypesFor(kind) {
		if spec.Key == key {
			return true
		}
	}
	return false
}
