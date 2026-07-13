package optimizer

import (
	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// NormalizeIdentifiers folds unquoted identifiers per the dialect's normalization strategy.
// dialect is a DialectType-style value (nil | string | *dialects.Dialect), mirroring upstream
// sqlglot's normalize_identifiers(expression, dialect: DialectType).
func NormalizeIdentifiers(expression exp.Expression, dialect any) exp.Expression {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		panic(err)
	}
	if expression == nil {
		return nil
	}
	// Ports normalize_identifiers.py:66-78: prune subtrees under a case_sensitive-marked node
	// and skip such nodes, normalizing only the rest. (store_original_column_identifiers is off
	// by default, so its dot_parts branch is not ported here.) NB: the only upstream producer of
	// the case_sensitive meta is add_comments parsing a `/* sqlglot.meta case_sensitive */`
	// annotation (core.py:1044), which this port does not yet parse — so this guard is currently
	// inert (nothing sets the flag) but kept structurally faithful for when that lands.
	caseSensitive := func(n exp.Expression) bool {
		b, _ := n.MetaGet("case_sensitive").(bool)
		return b
	}
	for _, node := range expression.WalkWithPrune(true, caseSensitive) {
		if caseSensitive(node) {
			continue
		}
		if node.Kind() == exp.KindIdentifier {
			d.NormalizeIdentifier(node)
		}
	}
	return expression
}

func NormalizeIdentifiersString(name string, dialect any) exp.Expression {
	// ParseIdentifier only needs the dialect's tokenizer/quoting (strategy-independent), so
	// resolve any->name for it; NormalizeIdentifiers still applies the full strategy.
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		panic(err)
	}
	return NormalizeIdentifiers(exp.ParseIdentifier(name, d.Name), dialect)
}
