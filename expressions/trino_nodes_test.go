package expressions

import (
	"fmt"
	"reflect"
	"testing"
)

type trinoNodeMetadataCase struct {
	name       string
	kind       Kind
	build      func(Args) Expression
	argKeys    []string
	required   Args
	funcTraits bool
	toSArgs    Args
	wantToS    string
}

func copyTrinoArgs(args Args) Args {
	copied := Args{}
	for key, value := range args {
		copied[key] = value
	}
	return copied
}

// TestTrinoNodeMetadata pins the class names, arg_types declaration order, and
// required arguments from the pinned functions.py:249-250,317-318,
// query.py:1961-1962,2062-2066, core.py:1596-1597, and array.py:146-147.
func TestTrinoNodeMetadata(t *testing.T) {
	cases := []trinoNodeMetadataCase{
		{
			name:       "CurrentCatalog",
			kind:       KindCurrentCatalog,
			build:      CurrentCatalog,
			argKeys:    []string{},
			required:   Args{},
			funcTraits: true,
			toSArgs:    Args{},
			wantToS:    "CurrentCatalog()",
		},
		{
			name:       "CurrentVersion",
			kind:       KindCurrentVersion,
			build:      CurrentVersion,
			argKeys:    []string{},
			required:   Args{},
			funcTraits: true,
			toSArgs:    Args{},
			wantToS:    "CurrentVersion()",
		},
		{
			name:     "JSONExtractQuote",
			kind:     KindJSONExtractQuote,
			build:    JSONExtractQuote,
			argKeys:  []string{"option", "scalar"},
			required: Args{"option": "WITHOUT"},
			toSArgs:  Args{"option": "WITHOUT", "scalar": true},
			wantToS:  "JSONExtractQuote(option=WITHOUT, scalar=True)",
		},
		{
			name:     "OverflowTruncateBehavior",
			kind:     KindOverflowTruncateBehavior,
			build:    OverflowTruncateBehavior,
			argKeys:  []string{"this", "with_count"},
			required: Args{"with_count": true},
			toSArgs:  Args{"this": "END", "with_count": true},
			wantToS:  "OverflowTruncateBehavior(this=END, with_count=True)",
		},
		{
			name:     "Refresh",
			kind:     KindRefresh,
			build:    Refresh,
			argKeys:  []string{"this", "kind"},
			required: Args{"this": "orders", "kind": "MATERIALIZED VIEW"},
			toSArgs:  Args{"this": "orders", "kind": "MATERIALIZED VIEW"},
			wantToS:  "Refresh(this=orders, kind=MATERIALIZED VIEW)",
		},
		{
			name:       "ArrayFirst",
			kind:       KindArrayFirst,
			build:      ArrayFirst,
			argKeys:    []string{"this", "expression"},
			required:   Args{"this": "items"},
			funcTraits: true,
			toSArgs:    Args{"this": "items", "expression": "predicate"},
			wantToS:    "ArrayFirst(this=items, expression=predicate)",
		},
	}

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
			node := tc.build(copyTrinoArgs(tc.required))
			if got := node.Kind(); got != tc.kind {
				t.Fatalf("Kind() = %v, want %v", got, tc.kind)
			}
			if got := ArgKeys(tc.kind); !reflect.DeepEqual(got, tc.argKeys) {
				t.Fatalf("ArgKeys(%v) = %v, want %v", tc.kind, got, tc.argKeys)
			}
			if got := ClassName(tc.kind); got != tc.name {
				t.Fatalf("ClassName(%v) = %q, want %q", tc.kind, got, tc.name)
			}
			if got := tc.build(copyTrinoArgs(tc.toSArgs)).ToS(); got != tc.wantToS {
				t.Fatalf("ToS() = %q, want %q", got, tc.wantToS)
			}
			if messages := node.ErrorMessages(nil); len(messages) != 0 {
				t.Fatalf("ErrorMessages() = %v, want no errors", messages)
			}

			for _, trait := range traits {
				want := tc.funcTraits && (trait == TraitCondition || trait == TraitFunc)
				if got := node.Is(trait); got != want {
					t.Fatalf("Is(%v) = %v, want %v", trait, got, want)
				}
			}
			if node.IsPrimitive() || primitive[tc.kind] {
				t.Fatal("node unexpectedly marked primitive")
			}
			if varLenArgs[tc.kind] {
				t.Fatal("node unexpectedly marked as variadic")
			}

			for key := range tc.required {
				missing := copyTrinoArgs(tc.required)
				delete(missing, key)
				want := fmt.Sprintf("Required keyword: '%s' missing for %s", key, tc.name)
				if got := tc.build(missing).ErrorMessages(nil); !reflect.DeepEqual(got, []string{want}) {
					t.Fatalf("ErrorMessages() without %q = %v, want [%q]", key, got, want)
				}
			}
		})
	}
}
