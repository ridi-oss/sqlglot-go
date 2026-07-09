package expressions

// XMLElement/XMLTable port exp.XMLElement/exp.XMLTable (Expression, Func)
// (functions.py:439,453). XMLNamespace ports exp.XMLNamespace (query.py:2081-2082), a
// plain Expression subclass.
func XMLElement(args Args) Expression   { return newNode(KindXMLElement, args) }
func XMLTable(args Args) Expression     { return newNode(KindXMLTable, args) }
func XMLNamespace(args Args) Expression { return newNode(KindXMLNamespace, args) }

// PathColumnConstraint ports exp.PathColumnConstraint (constraints.py:185), the `PATH '<p>'`
// column constraint (parsed via the shared constraintParsers "PATH" entry; see the
// KindPathColumnConstraint comment in kinds.go).
func PathColumnConstraint(args Args) Expression { return newNode(KindPathColumnConstraint, args) }
