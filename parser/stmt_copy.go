package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.COPY] = (*Parser).parseCopy
}

// parseCopy ports _parse_copy (parser.py:9616-9649): `COPY [INTO] <table-or-select>
// FROM|TO <files> [<credentials>] [WITH] [(<params>)]`. Any input this doesn't
// structurally recognize (e.g. dialect-specific option shapes this slice doesn't model)
// degrades to a raw Command via parseAsCommand, mirroring upstream's own fallback.
func (p *Parser) parseCopy() exp.Expression {
	start := p.prev

	p.match(tokens.INTO)

	var this exp.Expression
	if p.match(tokens.L_PAREN, false) {
		this = p.parseSelect(true, false, false)
	} else {
		this = p.parseTable(true, false, nil, false, false, false, false)
	}

	// kind = self._match(TokenType.FROM) or not self._match_text_seq("TO")
	// (parser.py:9631): only probe for TO when FROM didn't match, preserving Python's
	// short-circuit "or".
	kind := p.match(tokens.FROM)
	if !kind {
		kind = !p.matchTextSeq("TO")
	}

	files := p.parseCsv(func() exp.Expression { return p.parseField(false, nil, false) })
	if p.match(tokens.EQ, false) {
		// Backtrack one token since we've consumed the lhs of a parameter assignment here
		// (parser.py:9636-9640): this can happen for Snowflake dialect. Instead, we'd like
		// to parse the parameter list via parseCopyParameters below.
		p.retreat(p.index - 1)
		files = nil
	}

	credentials := p.parseCredentials()

	// The CREDENTIALS/ENCRYPTION/STORAGE_INTEGRATION/IAM_ROLE/REGION clauses are
	// Snowflake/Redshift-only and render faithfully only via upstream's unported
	// _parse_property grammar. None of base/mysql/postgres produce them, so when one is
	// present degrade to a raw Command (exact round-trip via parseAsCommand's source
	// recapture) rather than emit a lossy structural rewrite. This keeps every credential
	// shape consistent — cf. the multi-option case that already degrades via trailing tokens.
	if credentialsPresent(credentials) {
		return p.parseAsCommand(start)
	}

	p.matchTextSeq("WITH")

	// _parse_wrapped(self._parse_copy_parameters, optional=True) (parser.py:9646;
	// _parse_wrapped at parser.py:8635-8641): _parse_wrapped ALWAYS invokes the parse method,
	// matching the closing ")" only when an opening "(" was present. With optional=True a
	// missing "(" is not an error, so unwrapped parameter lists parse too — postgres
	// `COPY tbl FROM 'file' WITH FORMAT csv` -> `COPY tbl FROM 'file' WITH (FORMAT csv)`.
	// Go has no list-returning parseWrapped, so the wrapper is inlined here.
	wrapped := p.match(tokens.L_PAREN)
	params := p.parseCopyParameters()
	if wrapped {
		p.matchRParen(nil)
	}

	// Fallback case.
	if p.curr.IsValid() {
		return p.parseAsCommand(start)
	}

	return p.expression(exp.Copy(exp.Args{
		"this":        this,
		"kind":        kind,
		"credentials": credentials,
		"files":       files,
		"params":      params,
	}), nil, nil)
}

// parseCopyParameters ports _parse_copy_parameters (parser.py:9551-9588). The generic
// branch plus the `FORMAT AS AVRO|JSON` branch (parser.py:9573-9579) are ported; that
// branch is NOT dialect-gated, so postgres reaches it too. The COPY_INTO_VARLEN_OPTIONS
// and FILE_FORMAT branches (parser.py:9565-9572) are Snowflake/T-SQL-only and unreachable
// from this port's target dialects; any input needing one of them falls through to
// parseCopy's trailing-token Command fallback.
func (p *Parser) parseCopyParameters() []exp.Expression {
	sep := p.dialect.CopyParamsAreCsv

	var options []exp.Expression
	for p.curr.IsValid() && !p.match(tokens.R_PAREN, false) {
		startIndex := p.index
		option := p.parseVar(true, nil, false)
		prev := stringsUpper(p.prev.Text)

		// Different dialects might separate options and values by white space, "=" and "AS".
		p.match(tokens.EQ)
		matchedAlias := p.match(tokens.ALIAS)

		param := p.expression(exp.CopyParameter(exp.Args{"this": option}), nil, nil)

		if prev == "FORMAT" && matchedAlias && p.matchTexts(map[string]bool{"AVRO": true, "JSON": true}) {
			// parser.py:9573-9579: `FORMAT AS AVRO|JSON`. Fold the keyword into `this` as
			// the var "FORMAT AS <fmt>" and take the following field (if any) as the value,
			// so it round-trips as `FORMAT AS JSON` / `FORMAT AS JSON 'x'` rather than the
			// lossy `FORMAT JSON` / `FORMAT JSON, x` the generic branch would produce.
			param.Set("this", exp.Var(exp.Args{"this": "FORMAT AS " + stringsUpper(p.prev.Text)}))
			param.Set("expression", p.parseField(false, nil, false))
		} else {
			expression := p.parseUnquotedField()
			if expression == nil {
				expression = p.parseBracket(nil)
			}
			param.Set("expression", expression)
		}

		// No-progress guard: upstream's _parse_var(any_token=True) consumes any token, but
		// this port's parseVar returns nil for reserved tokens (operators, stray separators).
		// If an iteration consumed nothing, stop instead of spinning — the leftover tokens
		// reach parseCopy's trailing-token Command fallback (degrade-to-Command). This now
		// also runs on unwrapped parameter lists, so guarding termination here matters.
		if p.index == startIndex {
			break
		}

		options = append(options, param)

		if sep {
			p.match(tokens.COMMA)
		}
	}

	return options
}

// credentialsPresent reports whether parseCredentials matched any of its clauses (i.e.
// set any of Credentials' args). Used to decide whether a COPY must degrade to Command.
func credentialsPresent(e exp.Expression) bool {
	if e == nil {
		return false
	}
	for _, key := range []string{"storage", "credentials", "encryption", "iam_role", "region"} {
		if e.Arg(key) != nil {
			return true
		}
	}
	return false
}

// parseCredentials ports _parse_credentials (parser.py:9590-9611): `[STORAGE_INTEGRATION
// = <field>] [CREDENTIALS [= (<opts>)|<field>]] [ENCRYPTION (<opts>)] [IAM_ROLE
// DEFAULT|<field>] [REGION <field>]`. This port's target dialects (base/mysql/postgres)
// never exercise any of these guards (they're Snowflake/Redshift-specific), so this
// always returns an all-nil Credentials node in practice; it's still ported structurally
// so a stray CREDENTIALS/ENCRYPTION/... clause is consumed rather than falling through to
// the Command fallback.
func (p *Parser) parseCredentials() exp.Expression {
	expr := p.expression(exp.Credentials(exp.Args{}), nil, nil)

	if p.matchTextSeq("STORAGE_INTEGRATION", "=") {
		expr.Set("storage", p.parseField(false, nil, false))
	}
	if p.matchTextSeq("CREDENTIALS") {
		// Snowflake case: CREDENTIALS = (...), Redshift case: CREDENTIALS <string>.
		var credentials any
		if p.match(tokens.EQ) {
			credentials = p.parseCopyWrappedOptions()
		} else {
			credentials = p.parseField(false, nil, false)
		}
		expr.Set("credentials", credentials)
	}
	if p.matchTextSeq("ENCRYPTION") {
		expr.Set("encryption", p.parseCopyWrappedOptions())
	}
	if p.matchTextSeq("IAM_ROLE") {
		if p.match(tokens.DEFAULT) {
			expr.Set("iam_role", exp.Var(exp.Args{"this": p.prev.Text}))
		} else {
			expr.Set("iam_role", p.parseField(false, nil, false))
		}
	}
	if p.matchTextSeq("REGION") {
		expr.Set("region", p.parseField(false, nil, false))
	}

	return expr
}

// parseCopyWrappedOptions is a scoped-down port of _parse_wrapped_options
// (parser.py:9530-9548), covering only the option shapes reachable from Credentials'
// CREDENTIALS=(...)/ENCRYPTION(...) sub-clauses. Upstream's version falls back to
// _parse_property for each option, which isn't ported in this slice (it's a large,
// separate DDL-properties grammar); this instead captures each option generically as a
// bare var or `var = value` pair, which is sufficient for the unreachable-in-corpus paths
// this feeds (base/mysql/postgres never exercise Credentials' EQ sub-clauses).
func (p *Parser) parseCopyWrappedOptions() []exp.Expression {
	p.match(tokens.EQ)
	p.match(tokens.L_PAREN)

	var opts []exp.Expression
	for p.curr.IsValid() && !p.match(tokens.R_PAREN) {
		option := p.parseVar(true, nil, false)
		if option == nil {
			// Mirror _parse_wrapped_options' `if option is None: break` (parser.py:9545-9547).
			// This port lacks the full _parse_property grammar, so any option shape it can't
			// capture as a bare var / `var = value` pair — e.g. a comma separator inside a
			// Snowflake/Redshift credential list — stops the loop rather than spinning forever
			// on a token parseVar can't consume. The unconsumed tokens then reach parseCopy's
			// trailing-token Command fallback (degrade-to-Command, exactly as upstream does for
			// shapes it can't model). This guarantees termination.
			break
		}
		if p.match(tokens.EQ) {
			value := p.parseUnquotedField()
			opts = append(opts, p.expression(exp.EQ(exp.Args{"this": option, "expression": value}), nil, nil))
		} else {
			opts = append(opts, option)
		}
	}

	return opts
}
