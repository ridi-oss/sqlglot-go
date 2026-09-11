package generator

import (
	"github.com/ridi-oss/sqlglot-go/dialects"
	"github.com/ridi-oss/sqlglot-go/expressions"
)

// athenaGeneratesAsHive ports _generate_as_hive (generators/athena.py:47-69): DDL renders with
// the Hive generator, queries and query-bearing DDL with the Trino one.
func athenaGeneratesAsHive(e expressions.Expression) bool {
	switch e.Kind() {
	case expressions.KindCreate:
		if e.Text("kind") == "TABLE" {
			if properties := asExpression(e.Arg("properties")); properties != nil && properties.Find(expressions.KindExternalProperty) != nil {
				return true
			}
			expression := e.Expr()
			return expression == nil || !expression.Is(expressions.TraitQuery)
		}
		return e.Text("kind") != "VIEW"
	case expressions.KindDrop:
		return e.Text("kind") != "VIEW"
	case expressions.KindDescribe:
		// Structured EXPLAIN (ledger athena-explain) is a Trino statement; DESCRIBE t is Hive.
		return e.Text("kind") != "EXPLAIN"
	case expressions.KindAlter, expressions.KindShow:
		return true
	}
	return false
}

// athenaChild picks the routed sub-generator for one statement (generators/athena.py:174-180).
func (g *Generator) athenaChild(e expressions.Expression) *Generator {
	if athenaGeneratesAsHive(e) {
		return g.child(dialects.Hive(), "athena-hive")
	}
	return g.child(dialects.Trino(), "athena-trino")
}
