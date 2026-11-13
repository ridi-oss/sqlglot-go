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
		newThis := parts[2]
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
	this := ids[3]
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

// implicitUnnestsToExplicit ports _implicit_unnests_to_explicit (parser.py:4305-4330):
// `FROM t, t.arr` joins the array column t.arr, i.e. `CROSS JOIN UNNEST(t.arr)`.
func (p *Parser) implicitUnnestsToExplicit(this exp.Expression) exp.Expression {
	from := asExpr(this.Arg("from_"))
	// divergence (DEVIATIONS §1.25): range variable names compare case-insensitively; upstream
	// compares the normalized names, which keep a table's case, so `FROM Sales.Users JOIN
	// Users.tags` missed. An explicit UNNEST alias is a range variable too.
	refs := map[string]bool{}
	addRef := func(source exp.Expression) {
		if name := source.AliasOrName(); name != "" {
			refs[strings.ToLower(name)] = true
		}
		if source.Kind() == exp.KindUnnest {
			for _, name := range source.AliasColumnNames() {
				refs[strings.ToLower(name)] = true
			}
		}
	}
	if from != nil && from.This() != nil {
		addRef(from.This())
	}
	joins, _ := this.Arg("joins").([]exp.Expression)
	for _, join := range joins {
		table := join.This()
		if table == nil {
			continue
		}
		// divergence: upstream (parser.py:4315) skips joins with ON, so `JOIN u.tags AS t ON TRUE`
		// stays a table; BigQuery resolves u as the range variable either way. The right side of
		// a RIGHT or FULL join cannot see the left side, so there a path stays a table. Only a
		// plain name path converts; upstream also turns `t.f()` into UNNEST(f()), dropping t.
		side := strings.ToUpper(join.Text("side"))
		if isNamePath(table) && side != "RIGHT" && side != "FULL" {
			if parts := table.Parts(); len(parts) > 1 && refs[strings.ToLower(parts[0].Name())] {
				rangeVariableUnnest(table)
				table = join.This()
			}
		}
		addRef(table)
	}
	return this
}

// bigqueryParseColumnOps ports _parse_column_ops (parsers/bigquery.py:773-799): SAFE.f(...) and
// NET.f(...) wrap the known function; AI.f(...) / ML.f(...) re-parse f with its function parser.
func (p *Parser) bigqueryParseColumnOps(this exp.Expression) exp.Expression {
	funcIndex := p.index + 1
	this = p.parseColumnOpsBase(this)
	if this == nil || this.Kind() != exp.KindDot {
		return this
	}
	expr := this.Expr()
	if expr == nil || !expr.Is(exp.TraitFunc) || this.This() == nil {
		return this
	}
	switch prefix := strings.ToUpper(this.This().Name()); prefix {
	case "NET", "SAFE":
		kind := exp.KindSafeFunc
		if prefix == "NET" {
			kind = exp.KindNetFunc
		}
		p.retreat(funcIndex)
		return p.expression(exp.New(kind, exp.Args{"this": p.parseFunction(nil, false, false, true)}), nil, nil)
	case "AI", "ML":
		p.retreat(funcIndex)
		if parsed := p.parseFunction(nil, false, false, true); parsed != nil {
			return p.expression(exp.Dot(exp.Args{"this": this.This(), "expression": parsed}), nil, nil)
		}
	}
	return this
}

// bracketOffsets ports BRACKET_OFFSETS (parsers/bigquery.py:386-391): name -> (offset, safe).
var bracketOffsets = map[string]struct {
	offset int
	safe   bool
}{"OFFSET": {0, false}, "ORDINAL": {1, false}, "SAFE_OFFSET": {0, true}, "SAFE_ORDINAL": {1, true}}

// bigqueryParseBracket ports _parse_bracket (parsers/bigquery.py:579-602).
func (p *Parser) bigqueryParseBracket(this exp.Expression) exp.Expression {
	bracket := p.parseBracketBase(this)
	if bracket == nil || bracket == this {
		return bracket
	}
	if bracket.Kind() == exp.KindArray {
		bracket.Set("struct_name_inheritance", true)
	}
	if bracket.Kind() != exp.KindBracket {
		return bracket
	}
	for _, e := range bracket.Expressions() {
		spec, ok := bracketOffsets[strings.ToUpper(e.Name())]
		args := e.Expressions()
		if e.Kind() != exp.KindAnonymous || !ok || len(args) == 0 {
			break
		}
		bracket.Set("offset", spec.offset)
		bracket.Set("safe", spec.safe)
		e.Replace(args[0])
	}
	return bracket
}

// isNamePath reports whether a FROM table is a plain, unbackticked name path such as u.tags.
func isNamePath(table exp.Expression) bool {
	if table.Kind() != exp.KindTable || table.This() == nil || table.MetaGet("quoted_table") != nil {
		return false
	}
	// A path longer than three names keeps the rest in a Dot under this.
	for _, part := range table.Parts() {
		if part.Kind() != exp.KindIdentifier {
			return false
		}
	}
	return true
}

// rangeVariableUnnest replaces `FROM u.s.arr [AS a]` with `UNNEST(u.s.arr) [AS a]`, where u is a
// range variable: the path reads column s of u, then field arr.
// divergence: upstream's Table.to_column reads u.s.arr as schema u, table s, column arr.
func rangeVariableUnnest(table exp.Expression) {
	parts := table.Parts()
	column := exp.Column(exp.Args{"this": parts[1].Copy(), "table": parts[0].Copy()})
	if len(parts) > 2 {
		chain := []exp.Expression{column}
		for _, part := range parts[2:] {
			chain = append(chain, part.Copy())
		}
		column = exp.DotBuild(chain)
	}
	// An unaliased path's implicit alias is its last name (BigQuery "Implicit aliases"); upstream
	// drops it, so `JOIN u.tags` then `SELECT tags` bound the array, not the element.
	aliasName := parts[len(parts)-1].Copy()
	if alias := asExpr(table.Arg("alias")); alias != nil && alias.Kind() == exp.KindTableAlias && alias.This() != nil {
		aliasName = alias.This()
	}
	unnest := exp.Unnest(exp.Args{
		"expressions": []exp.Expression{column},
		"alias":       exp.TableAlias(exp.Args{"columns": []exp.Expression{aliasName}}),
	})
	table.Replace(unnest)
}

// bigqueryCorrelatedUnnests turns `FROM o.arr` inside a subquery into `UNNEST(o.arr)` when o names
// a source of an enclosing query: BigQuery resolves a path's first name against the range
// variables in scope, including correlated ones, before datasets.
// divergence: upstream only checks the joins of the same SELECT (DEVIATIONS §1.25).
func bigqueryCorrelatedUnnests(statement exp.Expression) {
	if statement == nil {
		return
	}
	for _, table := range statement.FindAll(exp.KindTable) {
		parent := table.Parent()
		// A backticked path `o.arr` names a table, unlike `o`.`arr` (generators/bigquery.py:614-627).
		if !isNamePath(table) || table.Arg("schema") == nil || parent == nil {
			continue
		}
		if parent.Kind() != exp.KindFrom && parent.Kind() != exp.KindJoin {
			continue
		}
		owner := parent.Parent()
		if owner == nil || owner.Kind() != exp.KindSelect {
			continue
		}
		parts := table.Parts()
		if !enclosingSourceNames(owner)[strings.ToLower(parts[0].Name())] {
			continue
		}
		rangeVariableUnnest(table)
	}
}

// isArgOf reports whether child is n's value, or an element of n's list, for one of keys.
func isArgOf(n, child exp.Expression, keys ...string) bool {
	for _, key := range keys {
		switch v := n.Arg(key).(type) {
		case exp.Expression:
			if v == child {
				return true
			}
		case []exp.Expression:
			for _, e := range v {
				if e == child {
					return true
				}
			}
		}
	}
	return false
}

// enclosingSourceNames collects the lowercase range-variable names visible from sel's FROM through
// enclosing queries, following BigQuery's scoping by the clause each step comes up through:
//   - SELECT: a subquery or table function in FROM/JOIN sees none of that FROM's items; an UNNEST
//     sees the items before it; a JOIN's ON sees the items up to and
//     including that join; any other clause sees all items.
//   - UPDATE/DELETE: SET/WHERE see the target and its FROM/USING items and joins.
//   - MERGE: the USING source sees neither side; ON and WHEN MATCHED see both; WHEN NOT MATCHED
//     [BY TARGET] sees the source; WHEN NOT MATCHED BY SOURCE sees the target.
//
// The walk stops at a CTE (a CTE body cannot correlate).
func enclosingSourceNames(sel exp.Expression) map[string]bool {
	names := map[string]bool{}
	var add func(source exp.Expression)
	add = func(source exp.Expression) {
		if source == nil {
			return
		}
		if source.Kind() == exp.KindFrom {
			source = source.This()
		}
		if source == nil {
			return
		}
		if name := source.AliasOrName(); name != "" {
			names[strings.ToLower(name)] = true
		}
		// An UNNEST range variable is named by its alias column.
		if source.Kind() == exp.KindUnnest {
			for _, name := range source.AliasColumnNames() {
				names[strings.ToLower(name)] = true
			}
		}
		// UPDATE/DELETE attach their joins to the target table.
		joins, _ := source.Arg("joins").([]exp.Expression)
		for _, join := range joins {
			add(join.This())
		}
	}
	addList := func(v any) {
		switch x := v.(type) {
		case exp.Expression:
			add(x)
		case []exp.Expression:
			for _, e := range x {
				add(e)
			}
		}
	}
	// child is n's direct child on the path from sel; grand is child's child on that path.
	var child, grand exp.Expression = sel, nil
	for n := sel.Parent(); n != nil && n.Kind() != exp.KindCTE; n = n.Parent() {
		switch n.Kind() {
		case exp.KindSelect:
			from := asExpr(n.Arg("from_"))
			joins, _ := n.Arg("joins").([]exp.Expression)
			switch {
			case child == from:
				// The first FROM item has no items before it.
			case child.Kind() == exp.KindJoin && child.Parent() == n:
				k := 0
				for k < len(joins) && joins[k] != child {
					k++
				}
				item := child.This()
				if grand == item && item.Kind() != exp.KindUnnest {
					// A derived table or a table function sees none of this FROM's items; only an
					// UNNEST correlates with the items before it.
				} else {
					last := k
					if grand != item {
						last = k + 1 // the ON also sees the join's own item
					}
					add(from)
					for _, join := range joins[:min(last, len(joins))] {
						add(join.This())
					}
				}
			default:
				add(from)
				for _, join := range joins {
					add(join.This())
				}
			}
		case exp.KindUpdate, exp.KindDelete:
			if isArgOf(n, child, "this", "from_", "using", "joins", "tables") {
				break // a source item does not see the target
			}
			add(asExpr(n.Arg("this")))
			add(asExpr(n.Arg("from_")))
			addList(n.Arg("using"))
			addList(n.Arg("joins"))
			addList(n.Arg("tables")) // BigQuery `DELETE ds.t AS t WHERE ...` keeps its target here
		case exp.KindMerge:
			target, source := asExpr(n.Arg("this")), asExpr(n.Arg("using"))
			switch {
			case child == source || child == target:
			case child == asExpr(n.Arg("whens")):
				// grand is the When the path comes through.
				matched, bySource := truthyArg(grand.Arg("matched")), truthyArg(grand.Arg("source"))
				if matched || bySource {
					add(target)
				}
				if matched || !bySource {
					add(source)
				}
			default:
				add(target)
				add(source)
			}
		}
		grand, child = child, n
	}
	return names
}
