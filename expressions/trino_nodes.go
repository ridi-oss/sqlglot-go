package expressions

// trino_nodes.go provides the minimal expression builders shared by Trino and Athena.
// Each builder mirrors the pinned sqlglot v30.12.0 class declaration cited below.

// CurrentCatalog ports expressions/functions.py:249-250.
func CurrentCatalog(args Args) Expression { return newNode(KindCurrentCatalog, args) }

// CurrentVersion ports expressions/functions.py:317-318.
func CurrentVersion(args Args) Expression { return newNode(KindCurrentVersion, args) }

// JSONExtractQuote ports expressions/query.py:2062-2066.
func JSONExtractQuote(args Args) Expression { return newNode(KindJSONExtractQuote, args) }

// OverflowTruncateBehavior ports expressions/query.py:1961-1962.
func OverflowTruncateBehavior(args Args) Expression {
	return newNode(KindOverflowTruncateBehavior, args)
}

// Refresh ports expressions/core.py:1596-1597.
func Refresh(args Args) Expression { return newNode(KindRefresh, args) }

// ArrayFirst ports expressions/array.py:146-147.
func ArrayFirst(args Args) Expression { return newNode(KindArrayFirst, args) }
