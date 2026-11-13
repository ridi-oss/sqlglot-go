package parser

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// BigQuery parser overrides port parsers/bigquery.py:189-799.

func (p *Parser) isBigQuery() bool { return p.dialect.Name == "bigquery" }

// dashedTablePartFollowTokens ports DASHED_TABLE_PART_FOLLOW_TOKENS (parsers/bigquery.py:372-376).
var dashedTablePartFollowTokens = map[tokens.TokenType]bool{tokens.DOT: true, tokens.L_PAREN: true, tokens.R_PAREN: true}

// bigqueryParseTablePart ports _parse_table_part (parsers/bigquery.py:402-431): hyphenated
// names (`my-project`) and parts that start with a digit (`foo.bar.25x`).
func (p *Parser) bigqueryParseTablePart(schema bool) exp.Expression {
	this := p.parseTablePartBase(schema)
	if this == nil {
		this = p.parseNumber()
	}
	if this == nil {
		return nil
	}
	switch this.Kind() {
	case exp.KindIdentifier:
		tableName := this.Name()
		for p.match(tokens.DASH, false) && p.next.IsValid() {
			start, startIndex := p.curr, p.index
			for p.isConnected() && !p.matchSet(dashedTablePartFollowTokens, false) {
				p.advance()
			}
			if startIndex == p.index {
				break
			}
			tableName += p.findSQL(start, p.prev)
		}
		return exp.Identifier(exp.Args{"this": tableName, "quoted": this.Arg("quoted")})
	case exp.KindLiteral:
		tableName := this.Name()
		if p.isConnected() && p.parseVar(true, nil, false) != nil {
			tableName += p.prev.Text
		}
		return exp.Identifier(exp.Args{"this": tableName, "quoted": true})
	}
	return this
}

// bigquerySplitQualifiedName ports _split_qualified_name (parsers/bigquery.py:166-183): dots
// before a `domain.com:project` colon belong to the domain.
func bigquerySplitQualifiedName(name string, minNumWords int) []string {
	const domainDot = "\x00"
	if colon := strings.Index(name, ":"); colon != -1 && strings.Contains(name[:colon], ".") {
		name = strings.ReplaceAll(name[:colon], ".", domainDot) + name[colon:]
		parts := splitNumWordsFromStart(name, ".", minNumWords)
		for i := range parts {
			parts[i] = strings.ReplaceAll(parts[i], domainDot, ".")
		}
		return parts
	}
	return splitNumWordsFromStart(name, ".", minNumWords)
}

// splitNumWordsFromStart ports helper.split_num_words(fill_from_start=True); "" stands for None.
func splitNumWordsFromStart(value, sep string, minNumWords int) []string {
	words := strings.Split(value, sep)
	if missing := minNumWords - len(words); missing > 0 {
		return append(make([]string, missing), words...)
	}
	return words
}

func quotedIdentifiers(names []string) []exp.Expression {
	out := make([]exp.Expression, len(names))
	for i, name := range names {
		if name != "" {
			out[i] = exp.Identifier(exp.Args{"this": name, "quoted": true})
		}
	}
	return out
}

// bigqueryParseTableParts ports _parse_table_parts (parsers/bigquery.py:433-518).
func (p *Parser) bigqueryParseTableParts(schema, isDBReference, fast bool) exp.Expression {
	table := p.parseTablePartsBase(schema, isDBReference, true, fast)
	if table == nil || table.Kind() != exp.KindTable {
		return table
	}

	// proj-1.db.tbl -- `1.` is tokenized as a float so we need to unravel it here.
	if !truthyArg(table.Arg("catalog")) {
		if db := identArg(table, "schema"); db != nil {
			parts := strings.Split(db.Name(), ".")
			if len(parts) == 2 && !truthyArg(db.Arg("quoted")) {
				table.Set("catalog", exp.Identifier(exp.Args{"this": parts[0]}))
				table.Set("schema", exp.Identifier(exp.Args{"this": parts[1]}))
			}
		} else if this := identArg(table, "this"); this != nil {
			parts := strings.Split(this.Name(), ".")
			if len(parts) == 2 && !truthyArg(this.Arg("quoted")) {
				table.Set("schema", exp.Identifier(exp.Args{"this": parts[0]}))
				table.Set("this", exp.Identifier(exp.Args{"this": parts[1]}))
			}
		}
	}

	var alias exp.Expression
	if this := identArg(table, "this"); this != nil && anyPartHasDot(table.Parts()) {
		alias = this
		names := []string{}
		for _, part := range table.Parts() {
			names = append(names, part.Name())
		}
		parts := quotedIdentifiers(bigquerySplitQualifiedName(strings.Join(names, "."), 3))
		var newThis exp.Expression = parts[2]
		if len(parts) > 3 && newThis != nil {
			newThis = exp.DotBuild(append([]exp.Expression{newThis}, parts[3:]...))
		}
		pivots := table.Arg("pivots")
		table = exp.Table(exp.Args{"this": newThis, "schema": parts[1], "catalog": parts[0], "pivots": pivots})
		table.Meta()["quoted_table"] = true
	}

	// INFORMATION_SCHEMA views must be qualified by a region or dataset (parsers/bigquery.py:484-516).
	tableParts := table.Parts()
	if n := len(tableParts); n > 1 && strings.ToUpper(tableParts[n-2].Name()) == "INFORMATION_SCHEMA" {
		aliasID := alias
		if aliasID == nil {
			aliasID = tableParts[n-1]
		}
		table.Set("alias", exp.TableAlias(exp.Args{"this": exp.Identifier(exp.Args{"this": aliasID.Name(), "quoted": aliasID.Arg("quoted")})}))
		view := tableParts[n-2].Name() + "." + tableParts[n-1].Name()
		table.Set("this", exp.Identifier(exp.Args{"this": view, "quoted": true}))
		table.Set("schema", partAt(tableParts, n-3))
		table.Set("catalog", partAt(tableParts, n-4))
	}
	return table
}

func partAt(parts []exp.Expression, i int) exp.Expression {
	if i >= 0 && i < len(parts) {
		return parts[i]
	}
	return nil
}

func anyPartHasDot(parts []exp.Expression) bool {
	for _, part := range parts {
		if strings.Contains(part.Name(), ".") {
			return true
		}
	}
	return false
}

func identArg(e exp.Expression, key string) exp.Expression {
	if v, ok := e.Arg(key).(exp.Expression); ok && v != nil && v.Kind() == exp.KindIdentifier {
		return v
	}
	return nil
}

func truthyArg(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case exp.Expression:
		return x != nil
	}
	return true
}

// bigqueryParseColumn ports _parse_column (parsers/bigquery.py:520-537): a quoted dotted path
// such as `dataset.table`.col is split into its parts.
func (p *Parser) bigqueryParseColumn(column exp.Expression) exp.Expression {
	if column == nil || column.Kind() != exp.KindColumn {
		return column
	}
	parts := column.Parts()
	if !anyPartHasDot(parts) {
		return column
	}
	names := []string{}
	for _, part := range parts {
		names = append(names, part.Name())
	}
	ids := quotedIdentifiers(bigquerySplitQualifiedName(strings.Join(names, "."), 4))
	var this exp.Expression = ids[3]
	if len(ids) > 4 && this != nil {
		this = exp.DotBuild(append([]exp.Expression{this}, ids[4:]...))
	}
	column = exp.Column(exp.Args{"this": this, "table": ids[2], "schema": ids[1], "catalog": ids[0]})
	column.Meta()["quoted_column"] = true
	return column
}

// matchIDToken applies BigQuery's ID_VAR/ALIAS/TABLE_ALIAS token-set edits
// (parsers/bigquery.py:196-222): GRANT is usable as a name, ASC/DESC are not.
func (p *Parser) matchIDToken(set map[tokens.TokenType]bool) bool {
	if p.isBigQuery() {
		switch p.curr.TokenType {
		case tokens.GRANT:
			return p.match(tokens.GRANT)
		case tokens.ASC, tokens.DESC:
			return false
		}
	}
	return p.matchSet(set)
}
