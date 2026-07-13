package optimizer

import (
	"fmt"
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func seqGet[T any](s []T, i int) (T, bool) {
	var zero T
	if i < 0 {
		i = len(s) + i
	}
	if i < 0 || i >= len(s) {
		return zero, false
	}
	return s[i], true
}

func findNewName(taken map[string]any, base string) string {
	if _, ok := taken[base]; !ok {
		return base
	}
	i := 2
	name := fmt.Sprintf("%s_%d", base, i)
	for {
		if _, ok := taken[name]; !ok {
			return name
		}
		i++
		name = fmt.Sprintf("%s_%d", base, i)
	}
}

func nameSequence(prefix string) func() string {
	i := 0
	return func() string {
		name := fmt.Sprintf("%s%d", prefix, i)
		i++
		return name
	}
}

func ensureList[T any](value any) []T {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []T:
		return v
	case T:
		return []T{v}
	default:
		return nil
	}
}

func isTraversable(expression exp.Expression) bool {
	return expression != nil && (expression.Is(exp.TraitQuery) || expression.Is(exp.TraitDDL) || expression.Is(exp.TraitDML))
}

func isSetOperation(expression exp.Expression) bool {
	return expression != nil && expression.Is(exp.TraitSetOperation)
}

func isUnwrappedQueryKind(k exp.Kind) bool {
	return k == exp.KindSelect || k == exp.KindUnion || k == exp.KindExcept || k == exp.KindIntersect
}

func isUnwrappedQuery(expression exp.Expression) bool {
	return expression != nil && isUnwrappedQueryKind(expression.Kind())
}

func isDerivedTable(expression exp.Expression) bool {
	return expression != nil && expression.Kind() == exp.KindSubquery && (expression.Alias() != "" || isUnwrappedQuery(expression.This()))
}

func isFromOrJoin(expression exp.Expression) bool {
	if expression == nil {
		return false
	}
	parent := expression.Parent()
	for parent != nil && parent.Kind() == exp.KindSubquery {
		parent = parent.Parent()
	}
	return parent != nil && (parent.Kind() == exp.KindFrom || parent.Kind() == exp.KindJoin)
}

func getSourceAlias(expression exp.Expression) string {
	aliasArg := expression.Arg("alias")
	aliasName := expression.Alias()
	if aliasName == "" {
		if alias, ok := aliasArg.(exp.Expression); ok && alias != nil && alias.Kind() == exp.KindTableAlias {
			columns := expressionsFor(alias, "columns")
			if len(columns) == 1 {
				aliasName = columns[0].Name()
			}
		}
	}
	return aliasName
}

func isSemiOrAntiJoin(join exp.Expression) bool {
	if join == nil || join.Kind() != exp.KindJoin {
		return false
	}
	kind := strings.ToUpper(join.Text("kind"))
	return kind == "SEMI" || kind == "ANTI"
}

func copySources(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeSources(base map[string]any, other map[string]any) map[string]any {
	out := copySources(base)
	for k, v := range other {
		out[k] = v
	}
	return out
}

func expressionsFor(expression exp.Expression, key string) []exp.Expression {
	if expression == nil {
		return nil
	}
	if node, ok := expression.(*exp.Node); ok {
		return node.ExpressionsFor(key)
	}
	value := expression.Arg(key)
	switch v := value.(type) {
	case []exp.Expression:
		return v
	case exp.Expression:
		if v == nil {
			return nil
		}
		return []exp.Expression{v}
	default:
		return nil
	}
}

func asExpression(value any) exp.Expression {
	if expression, ok := value.(exp.Expression); ok {
		return expression
	}
	return nil
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
	case []exp.Expression:
		return len(v) > 0
	case []any:
		return len(v) > 0
	}
	return true
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func copyStringSlice(values []string) []string {
	return append([]string(nil), values...)
}
