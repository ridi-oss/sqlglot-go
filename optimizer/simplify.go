package optimizer

import (
	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func SimplifyParens(expression exp.Expression, dialect *dialects.Dialect) exp.Expression {
	if expression == nil || expression.Kind() != exp.KindParen {
		return expression
	}
	if dialect == nil {
		dialect = dialects.Base()
	}

	this := expression.This()
	parent := expression.Parent()
	parentIsPredicate := parent != nil && parent.Is(exp.TraitPredicate)

	if this != nil && this.Kind() == exp.KindSelect {
		return expression
	}

	if parent != nil && (isSubqueryPredicate(parent) || parent.Kind() == exp.KindBracket) {
		return expression
	}

	if dialect.RequiresParenthesizedStructAccess && parent != nil && parent.Kind() == exp.KindDot {
		right := parent.Right()
		if right != nil && (right.Kind() == exp.KindIdentifier || right.Kind() == exp.KindStar) {
			return expression
		}
	}

	if this != nil && this.Is(exp.TraitPredicate) && !(parentIsPredicate || (parent != nil && parent.Kind() == exp.KindNeg) || (parent != nil && parent.Is(exp.TraitBinary) && !parent.Is(exp.TraitConnector))) {
		return this
	}

	if parent == nil || !parent.Is(exp.TraitCondition) && !parent.Is(exp.TraitBinary) || parent.Kind() == exp.KindParen || (this != nil && !this.Is(exp.TraitBinary) && !((this.Kind() == exp.KindNot || this.Kind() == exp.KindIs) && parentIsPredicate)) || (this != nil && this.Kind() == exp.KindAdd && parent.Kind() == exp.KindAdd) || (this != nil && this.Kind() == exp.KindMul && parent.Kind() == exp.KindMul) || (this != nil && this.Kind() == exp.KindMul && (parent.Kind() == exp.KindAdd || parent.Kind() == exp.KindSub)) {
		return this
	}

	return expression
}

func isSubqueryPredicate(expression exp.Expression) bool {
	if expression == nil {
		return false
	}
	switch expression.Kind() {
	case exp.KindIn, exp.KindExists, exp.KindAny, exp.KindAll:
		return true
	default:
		return false
	}
}
