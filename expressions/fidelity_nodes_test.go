package expressions

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type fidelityMetadataCase struct {
	name     string
	kind     Kind
	build    func(Args) Expression
	argKeys  []string
	required Args
}

func fidelityScalar() Expression {
	return LiteralString("x")
}

func fidelityExpressions() []Expression {
	return []Expression{fidelityScalar()}
}

func copyFidelityArgs(args Args) Args {
	copied := Args{}
	for key, value := range args {
		copied[key] = value
	}
	return copied
}

func testFidelityMetadata(t *testing.T, cases []fidelityMetadataCase) {
	t.Helper()

	traits := []Trait{
		TraitCondition,
		TraitPredicate,
		TraitBinary,
		TraitConnector,
		TraitFunc,
		TraitAggFunc,
		TraitUnary,
		TraitQuery,
		TraitDDL,
		TraitDML,
		TraitUDTF,
		TraitDerivedTable,
		TraitSetOperation,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := tc.build(copyFidelityArgs(tc.required))
			if got := node.Kind(); got != tc.kind {
				t.Fatalf("Kind() = %v, want %v", got, tc.kind)
			}
			if got := ArgKeys(tc.kind); !reflect.DeepEqual(got, tc.argKeys) {
				t.Fatalf("ArgKeys(%v) = %v, want %v", tc.kind, got, tc.argKeys)
			}
			if got := ClassName(tc.kind); got != tc.name {
				t.Fatalf("ClassName(%v) = %q, want %q", tc.kind, got, tc.name)
			}
			toSName, _, ok := strings.Cut(node.ToS(), "(")
			if !ok || toSName != tc.name {
				t.Fatalf("ToS() class name = %q, want %q; full output: %s", toSName, tc.name, node.ToS())
			}
			if messages := node.ErrorMessages(nil); len(messages) != 0 {
				t.Fatalf("ErrorMessages() = %v, want no errors", messages)
			}
			for _, trait := range traits {
				if node.Is(trait) {
					t.Fatalf("node unexpectedly has trait %v", trait)
				}
			}
			if node.IsPrimitive() {
				t.Fatal("node unexpectedly marked primitive")
			}

			for key := range tc.required {
				missing := copyFidelityArgs(tc.required)
				delete(missing, key)
				want := fmt.Sprintf("Required keyword: '%s' missing for %s", key, tc.name)
				if got := tc.build(missing).ErrorMessages(nil); !reflect.DeepEqual(got, []string{want}) {
					t.Fatalf("ErrorMessages() without %q = %v, want [%q]", key, got, want)
				}
			}
		})
	}
}

func TestFidelityPropertyMetadata(t *testing.T) {
	testFidelityMetadata(t, []fidelityMetadataCase{
		{"Property", KindProperty, Property, []string{"this", "value"}, Args{"this": fidelityScalar(), "value": fidelityScalar()}},
		{"AlgorithmProperty", KindAlgorithmProperty, AlgorithmProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"AutoIncrementProperty", KindAutoIncrementProperty, AutoIncrementProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"CollateProperty", KindCollateProperty, CollateProperty, []string{"this", "default"}, Args{"this": fidelityScalar()}},
		{"DefinerProperty", KindDefinerProperty, DefinerProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"EngineProperty", KindEngineProperty, EngineProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"InheritsProperty", KindInheritsProperty, InheritsProperty, []string{"expressions"}, Args{"expressions": fidelityExpressions()}},
		{"LikeProperty", KindLikeProperty, LikeProperty, []string{"this", "expressions"}, Args{"this": fidelityScalar()}},
		{"LockProperty", KindLockProperty, LockProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"LockingProperty", KindLockingProperty, LockingProperty, []string{"this", "kind", "for_or_in", "lock_type", "override"}, Args{"kind": fidelityScalar(), "lock_type": fidelityScalar()}},
		{"MaterializedProperty", KindMaterializedProperty, MaterializedProperty, []string{"this"}, Args{}},
		{"NoPrimaryIndexProperty", KindNoPrimaryIndexProperty, NoPrimaryIndexProperty, []string{}, Args{}},
		{"OnCommitProperty", KindOnCommitProperty, OnCommitProperty, []string{"delete"}, Args{}},
		{"OnProperty", KindOnProperty, OnProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"PartitionedByProperty", KindPartitionedByProperty, PartitionedByProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"PartitionByRangeProperty", KindPartitionByRangeProperty, PartitionByRangeProperty, []string{"partition_expressions", "create_expressions"}, Args{"partition_expressions": fidelityExpressions(), "create_expressions": fidelityExpressions()}},
		{"PartitionByListProperty", KindPartitionByListProperty, PartitionByListProperty, []string{"partition_expressions", "create_expressions"}, Args{"partition_expressions": fidelityExpressions(), "create_expressions": fidelityExpressions()}},
		{"PartitionList", KindPartitionList, PartitionList, []string{"this", "expressions"}, Args{"this": fidelityScalar(), "expressions": fidelityExpressions()}},
		{"PartitionBoundSpec", KindPartitionBoundSpec, PartitionBoundSpec, []string{"this", "expression", "from_expressions", "to_expressions"}, Args{}},
		{"PartitionedOfProperty", KindPartitionedOfProperty, PartitionedOfProperty, []string{"this", "expression"}, Args{"this": fidelityScalar(), "expression": fidelityScalar()}},
		{"SchemaCommentProperty", KindSchemaCommentProperty, SchemaCommentProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"SqlReadWriteProperty", KindSqlReadWriteProperty, SqlReadWriteProperty, []string{"this"}, Args{"this": fidelityScalar()}},
		{"TemporaryProperty", KindTemporaryProperty, TemporaryProperty, []string{"this"}, Args{}},
		{"UnloggedProperty", KindUnloggedProperty, UnloggedProperty, []string{}, Args{}},
		{"WithDataProperty", KindWithDataProperty, WithDataProperty, []string{"no", "statistics"}, Args{"no": true}},
	})
}

func TestFidelityConstraintMetadata(t *testing.T) {
	testFidelityMetadata(t, []fidelityMetadataCase{
		{"CompressColumnConstraint", KindCompressColumnConstraint, CompressColumnConstraint, []string{"this"}, Args{}},
		{"DateFormatColumnConstraint", KindDateFormatColumnConstraint, DateFormatColumnConstraint, []string{"this"}, Args{"this": fidelityScalar()}},
		{"ExcludeColumnConstraint", KindExcludeColumnConstraint, ExcludeColumnConstraint, []string{"this"}, Args{"this": fidelityScalar()}},
		{"InlineLengthColumnConstraint", KindInlineLengthColumnConstraint, InlineLengthColumnConstraint, []string{"this"}, Args{"this": fidelityScalar()}},
		{"TitleColumnConstraint", KindTitleColumnConstraint, TitleColumnConstraint, []string{"this"}, Args{"this": fidelityScalar()}},
		{"UppercaseColumnConstraint", KindUppercaseColumnConstraint, UppercaseColumnConstraint, []string{}, Args{}},
		{"WithOperator", KindWithOperator, WithOperator, []string{"this", "op"}, Args{"this": fidelityScalar(), "op": fidelityScalar()}},
		{"InOutColumnConstraint", KindInOutColumnConstraint, InOutColumnConstraint, []string{"input_", "output", "variadic"}, Args{}},
	})
}

func TestFidelityQueryMetadata(t *testing.T) {
	testFidelityMetadata(t, []fidelityMetadataCase{
		{"PartitionRange", KindPartitionRange, PartitionRange, []string{"this", "expression", "expressions"}, Args{"this": fidelityScalar()}},
		{"AnalyzeHistogram", KindAnalyzeHistogram, AnalyzeHistogram, []string{"this", "expressions", "expression", "update_options"}, Args{"this": fidelityScalar(), "expressions": fidelityExpressions()}},
		{"AnalyzeWith", KindAnalyzeWith, AnalyzeWith, []string{"expressions"}, Args{"expressions": fidelityExpressions()}},
		{"UsingData", KindUsingData, UsingData, []string{"this"}, Args{"this": fidelityScalar()}},
	})
}
