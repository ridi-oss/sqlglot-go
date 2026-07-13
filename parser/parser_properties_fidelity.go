package parser

import (
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// parsePropertyBefore ports _parse_property_before (parser.py:2767-2790). The property
// subset in this port only consumes the DEFAULT prefix; the other Teradata-only modifiers
// are left untouched so unsupported forms fail closed through CREATE's Command fallback.
func (p *Parser) parsePropertyBefore() exp.Expression {
	index := p.index
	p.match(tokens.COMMA)

	if p.matchTextSeq("NO") || p.matchTextSeq("DUAL") || p.matchTextSeq("BEFORE") ||
		p.matchTextSeq("LOCAL") || p.matchTextSeq("NOT", "LOCAL") || p.matchTextSeq("AFTER") ||
		p.matchTexts(map[string]bool{"MIN": true, "MINIMUM": true, "MAX": true, "MAXIMUM": true}) {
		p.retreat(index)
		return nil
	}

	isDefault := p.matchTextSeq("DEFAULT")
	if !p.matchTexts(p.propertyParserKeys()) {
		p.retreat(index)
		return nil
	}
	return p.propertyParser(stringsUpper(p.prev.Text))(p, isDefault)
}

// parseWrappedProperties ports _parse_wrapped_properties (parser.py:2792-2793), flattening
// temporary Properties wrappers used by list-returning registry entries.
func (p *Parser) parseWrappedProperties() []exp.Expression {
	parsed := p.parseWrappedCsv(p.parseProperty)
	properties := make([]exp.Expression, 0, len(parsed))
	for _, property := range parsed {
		if property != nil && property.Kind() == exp.KindProperties {
			properties = append(properties, property.Expressions()...)
		} else if property != nil {
			properties = append(properties, property)
		}
	}
	return properties
}

// parseProperty ports _parse_property (parser.py:2795-2815) for the registered property
// subset, followed by the generic key=value fallback. Unsupported registry entries are not
// registered, so they consume nothing and the enclosing CREATE degrades to Command.
func (p *Parser) parseProperty() exp.Expression {
	if p.matchTexts(p.propertyParserKeys()) {
		return p.propertyParser(stringsUpper(p.prev.Text))(p, false)
	}
	if p.match(tokens.DEFAULT) && p.matchTexts(p.propertyParserKeys()) {
		return p.propertyParser(stringsUpper(p.prev.Text))(p, true)
	}
	return p.parseKeyValueProperty(nil)
}

// parseKeyValueProperty ports _parse_key_value_property (parser.py:2817-2841), including
// Column-to-Dot/Var normalization for keys and Column-to-Var normalization for values.
func (p *Parser) parseKeyValueProperty(parseValue func() exp.Expression) exp.Expression {
	index := p.index
	key := p.parseColumn()
	if !p.match(tokens.EQ) {
		p.retreat(index)
		return nil
	}

	if key != nil && key.Kind() == exp.KindColumn {
		if len(key.Parts()) > 1 {
			key = exp.ColumnsToDot(key)
		} else {
			key = exp.Var(exp.Args{"this": key.Name()})
		}
	}

	var value exp.Expression
	if parseValue != nil {
		value = parseValue()
	} else {
		value = p.parseBitwise()
		if value == nil {
			value = p.parseVar(true, nil, false)
		}
	}
	if value != nil && value.Kind() == exp.KindColumn {
		value = exp.Var(exp.Args{"this": value.Name()})
	}

	return p.expression(exp.Property(exp.Args{"this": key, "value": value}), nil, nil)
}

// parseStored ports _parse_stored (parser.py:2843-2864).
func (p *Parser) parseStored() exp.Expression {
	if p.matchTextSeq("BY") {
		return p.expression(exp.StorageHandlerProperty(exp.Args{"this": p.parseVarOrString(false)}), nil, nil)
	}

	p.match(tokens.ALIAS)
	var inputFormat exp.Expression
	if p.matchTextSeq("INPUTFORMAT") {
		inputFormat = p.parseString()
	}
	var outputFormat exp.Expression
	if p.matchTextSeq("OUTPUTFORMAT") {
		outputFormat = p.parseString()
	}

	var this exp.Expression
	if inputFormat != nil || outputFormat != nil {
		this = p.expression(exp.InputOutputFormat(exp.Args{
			"input_format":  inputFormat,
			"output_format": outputFormat,
		}), nil, nil)
	} else {
		this = p.parseVarOrString(false)
		if this == nil {
			this = p.parseNumber()
		}
		if this == nil {
			this = p.parseIdVar(true, nil)
		}
	}

	return p.expression(exp.FileFormatProperty(exp.Args{"this": this, "hive_format": true}), nil, nil)
}

// parseClusteredBy ports _parse_clustered_by (parser.py:3108-3128).
func (p *Parser) parseClusteredBy() exp.Expression {
	p.matchTextSeq("BY")

	p.matchLParen(nil)
	expressions := p.parseCsv(p.parseColumn)
	p.matchRParen(nil)

	var sortedBy []exp.Expression
	if p.matchTextSeq("SORTED", "BY") {
		p.matchLParen(nil)
		sortedBy = p.parseCsv(func() exp.Expression { return p.parseOrdered(nil) })
		p.matchRParen(nil)
	}

	p.match(tokens.INTO)
	buckets := p.parseNumber()
	p.matchTextSeq("BUCKETS")

	return p.expression(exp.ClusteredByProperty(exp.Args{
		"expressions": expressions,
		"sorted_by":   sortedBy,
		"buckets":     buckets,
	}), nil, nil)
}

// parseDefiner ports _parse_definer (parser.py:3058-3069): DEFINER[=]user@host.
func (p *Parser) parseDefiner() exp.Expression {
	p.match(tokens.EQ)
	user := p.parseIdVar(true, nil)
	p.match(tokens.PARAMETER)
	host := p.parseIdVar(true, nil)

	var hostText string
	if host != nil {
		hostText = propertyIdentifierText(host)
	} else if p.match(tokens.MOD) {
		hostText = p.prev.Text
	}
	if user == nil || hostText == "" {
		return nil
	}
	return exp.DefinerProperty(exp.Args{"this": propertyIdentifierText(user) + "@" + hostText})
}

func propertyIdentifierText(identifier exp.Expression) string {
	name := identifier.Name()
	if quoted, _ := identifier.Arg("quoted").(bool); quoted {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return name
}

// parseWithProperty ports the worklist-relevant branches of _parse_with_property
// (parser.py:3002-3043): WITH (...), WITH DATA, and WITH NO DATA. The wrapped branch uses a
// temporary Properties node because propertyParserFunc returns one expression.
func (p *Parser) parseWithProperty() exp.Expression {
	if p.match(tokens.L_PAREN, false) {
		return p.expression(exp.Properties(exp.Args{"expressions": p.parseWrappedProperties()}), nil, nil)
	}
	if p.matchTextSeq("DATA") {
		return p.parseWithData(false)
	}
	if p.matchTextSeq("NO", "DATA") {
		return p.parseWithData(true)
	}
	if p.match(tokens.SERDE_PROPERTIES, false) {
		return p.parseSerdeProperties(true)
	}
	return nil
}

// parseLocking ports _parse_locking (parser.py:3205-3250).
func (p *Parser) parseLocking() exp.Expression {
	var kind string
	switch {
	case p.match(tokens.TABLE):
		kind = "TABLE"
	case p.match(tokens.VIEW):
		kind = "VIEW"
	case p.match(tokens.ROW):
		kind = "ROW"
	case p.matchTextSeq("DATABASE"):
		kind = "DATABASE"
	}

	var this exp.Expression
	if kind == "DATABASE" || kind == "TABLE" || kind == "VIEW" {
		this = p.parseTableParts(false, false, false, false)
	}

	var forOrIn string
	if p.match(tokens.FOR) {
		forOrIn = "FOR"
	} else if p.match(tokens.IN) {
		forOrIn = "IN"
	}

	var lockType string
	switch {
	case p.matchTextSeq("ACCESS"):
		lockType = "ACCESS"
	case p.matchTexts(map[string]bool{"EXCL": true, "EXCLUSIVE": true}):
		lockType = "EXCLUSIVE"
	case p.matchTextSeq("SHARE"):
		lockType = "SHARE"
	case p.matchTextSeq("READ"):
		lockType = "READ"
	case p.matchTextSeq("WRITE"):
		lockType = "WRITE"
	case p.matchTextSeq("CHECKSUM"):
		lockType = "CHECKSUM"
	}

	return p.expression(exp.LockingProperty(exp.Args{
		"this": this, "kind": kind, "for_or_in": forOrIn,
		"lock_type": lockType, "override": p.matchTextSeq("OVERRIDE"),
	}), nil, nil)
}

// parsePartitionBoundSpec ports _parse_partition_bound_spec (parser.py:3257-3291).
func (p *Parser) parsePartitionBoundSpec() exp.Expression {
	parseBound := func() exp.Expression {
		if p.matchTextSeq("MINVALUE") {
			return exp.Var(exp.Args{"this": "MINVALUE"})
		}
		if p.matchTextSeq("MAXVALUE") {
			return exp.Var(exp.Args{"this": "MAXVALUE"})
		}
		return p.parseBitwise()
	}

	args := exp.Args{}
	switch {
	case p.match(tokens.IN):
		args["this"] = p.parseWrappedCsv(p.parseBitwise)
	case p.match(tokens.FROM):
		args["from_expressions"] = p.parseWrappedCsv(parseBound)
		p.matchTextSeq("TO")
		args["to_expressions"] = p.parseWrappedCsv(parseBound)
	case p.matchTextSeq("WITH", "(", "MODULUS"):
		args["this"] = p.parseNumber()
		p.matchTextSeq(",", "REMAINDER")
		args["expression"] = p.parseNumber()
		p.matchRParen(nil)
	default:
		p.raiseError("Failed to parse partition bound spec.")
	}
	return p.expression(exp.PartitionBoundSpec(args), nil, nil)
}

// parsePartitionedOf ports _parse_partitioned_of (parser.py:3293-3308).
func (p *Parser) parsePartitionedOf() exp.Expression {
	if !p.matchTextSeq("OF") {
		p.retreat(p.index - 1)
		return nil
	}
	this := p.parseTable(true, false, nil, false, false, false, false)

	var expression exp.Expression
	if p.match(tokens.DEFAULT) {
		expression = exp.Var(exp.Args{"this": "DEFAULT"})
	} else if p.matchTextSeq("FOR", "VALUES") {
		expression = p.parsePartitionBoundSpec()
	} else {
		p.raiseError("Expecting either DEFAULT or FOR VALUES clause.")
	}
	return p.expression(exp.PartitionedOfProperty(exp.Args{"this": this, "expression": expression}), nil, nil)
}

// parsePartitionedBy ports _parse_partitioned_by (parser.py:3310-3316).
func (p *Parser) parsePartitionedBy() exp.Expression {
	p.match(tokens.EQ)
	this := p.parseSchema(nil)
	if this == nil {
		this = p.parseBracket(p.parseField(false, nil, false))
	}
	return p.expression(exp.PartitionedByProperty(exp.Args{"this": this}), nil, nil)
}

// parseWithData ports _parse_withdata (parser.py:3318-3326).
func (p *Parser) parseWithData(no bool) exp.Expression {
	var statistics any
	if p.matchTextSeq("AND", "STATISTICS") {
		statistics = true
	} else if p.matchTextSeq("AND", "NO", "STATISTICS") {
		statistics = false
	}
	return p.expression(exp.WithDataProperty(exp.Args{"no": no, "statistics": statistics}), nil, nil)
}

func (p *Parser) parseContainsProperty() exp.Expression {
	if p.matchTextSeq("SQL") {
		return p.expression(exp.SqlReadWriteProperty(exp.Args{"this": "CONTAINS SQL"}), nil, nil)
	}
	return nil
}

func (p *Parser) parseModifiesProperty() exp.Expression {
	if p.matchTextSeq("SQL", "DATA") {
		return p.expression(exp.SqlReadWriteProperty(exp.Args{"this": "MODIFIES SQL DATA"}), nil, nil)
	}
	return nil
}

func (p *Parser) parseNoProperty() exp.Expression {
	if p.matchTextSeq("PRIMARY", "INDEX") {
		return exp.NoPrimaryIndexProperty(nil)
	}
	if p.matchTextSeq("SQL") {
		return p.expression(exp.SqlReadWriteProperty(exp.Args{"this": "NO SQL"}), nil, nil)
	}
	return nil
}

// parseOnProperty ports the ON COMMIT branches of _parse_on_property
// (parser.py:3345-3350). The generic OnProperty branch is outside the AST worklist and is
// left unconsumed by returning nil, so its enclosing statement fails closed.
func (p *Parser) parseOnProperty() exp.Expression {
	if p.matchTextSeq("COMMIT", "PRESERVE", "ROWS") {
		return exp.OnCommitProperty(nil)
	}
	if p.matchTextSeq("COMMIT", "DELETE", "ROWS") {
		return exp.OnCommitProperty(exp.Args{"delete": true})
	}
	return nil
}

func (p *Parser) parseReadsProperty() exp.Expression {
	if p.matchTextSeq("SQL", "DATA") {
		return p.expression(exp.SqlReadWriteProperty(exp.Args{"this": "READS SQL DATA"}), nil, nil)
	}
	return nil
}

// parseCreateLike ports _parse_create_like (parser.py:3360-3375).
func (p *Parser) parseCreateLike() exp.Expression {
	table := p.parseTable(true, false, nil, false, false, false, false)
	options := []exp.Expression{}
	for p.matchTexts(map[string]bool{"INCLUDING": true, "EXCLUDING": true}) {
		this := stringsUpper(p.prev.Text)
		idVar := p.parseIdVar(true, nil)
		if idVar == nil {
			return nil
		}
		options = append(options, p.expression(exp.Property(exp.Args{
			"this":  this,
			"value": exp.Var(exp.Args{"this": stringsUpper(idVar.Name())}),
		}), nil, nil))
	}
	return p.expression(exp.LikeProperty(exp.Args{"this": table, "expressions": options}), nil, nil)
}
