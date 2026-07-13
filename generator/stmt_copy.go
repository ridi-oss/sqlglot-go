package generator

import (
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

func init() {
	dispatch[expressions.KindCopy] = (*Generator).copySQL
	dispatch[expressions.KindCopyParameter] = (*Generator).copyParameterSQL
	dispatch[expressions.KindCredentials] = (*Generator).credentialsSQL
}

// copyParameterSQL ports copyparameter_sql (generator.py:5343-5363).
func (g *Generator) copyParameterSQL(e expressions.Expression) string {
	option := g.sqlKey(e, "this")

	if truthy(e.Arg("expressions")) {
		upper := strings.ToUpper(option)

		// Snowflake FILE_FORMAT options are separated by whitespace.
		sep := ", "
		if upper == "FILE_FORMAT" {
			sep = " "
		}

		// Databricks copy/format options do not set their list of values with EQ.
		op := " = "
		if upper == "COPY_OPTIONS" || upper == "FORMAT_OPTIONS" {
			op = " "
		}

		values := g.expressions(exprsOptions{expression: e, flat: true, sep: sep})
		return option + op + "(" + values + ")"
	}

	value := g.sqlKey(e, "expression")
	if value == "" {
		return option
	}

	op := " "
	if g.dialect.CopyParamsEqRequired {
		op = " = "
	}

	return option + op + value
}

// credentialsSQL ports credentials_sql (generator.py:5365-5388). This port's target
// dialects (base/mysql/postgres) never populate any of Credentials' args (see
// parser.parseCredentials), so it always renders "" in practice for those dialects.
func (g *Generator) credentialsSQL(e expressions.Expression) string {
	credArg := e.Arg("credentials")

	var credentials string
	if credExpr := asExpression(credArg); credExpr != nil && credExpr.Kind() == expressions.KindLiteral {
		// Redshift case: CREDENTIALS <string>.
		credentials = g.sqlKey(e, "credentials")
		if credentials != "" {
			credentials = "CREDENTIALS " + credentials
		}
	} else {
		// Snowflake case: CREDENTIALS = (...).
		credentials = g.expressions(exprsOptions{expression: e, key: "credentials", flat: true, sep: " "})
		if credArg != nil {
			credentials = "CREDENTIALS = (" + credentials + ")"
		} else {
			credentials = ""
		}
	}

	storage := g.sqlKey(e, "storage")
	if storage != "" {
		storage = "STORAGE_INTEGRATION = " + storage
	}

	encryption := g.expressions(exprsOptions{expression: e, key: "encryption", flat: true, sep: " "})
	if encryption != "" {
		encryption = " ENCRYPTION = (" + encryption + ")"
	}

	iamRole := g.sqlKey(e, "iam_role")
	if iamRole != "" {
		iamRole = "IAM_ROLE " + iamRole
	}

	region := g.sqlKey(e, "region")
	if region != "" {
		region = " REGION " + region
	}

	return credentials + storage + encryption + iamRole + region
}

// copySQL ports copy_sql (generator.py:5390-5416).
func (g *Generator) copySQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if g.dialect.CopyHasIntoKeyword {
		this = " INTO " + this
	} else {
		this = " " + this
	}

	credentials := g.sqlKey(e, "credentials")
	if credentials != "" {
		credentials = g.seg(credentials)
	}

	files := g.expressions(exprsOptions{expression: e, key: "files", flat: true})

	var kind string
	if files != "" {
		if truthy(e.Arg("kind")) {
			kind = g.seg("FROM")
		} else {
			kind = g.seg("TO")
		}
	}

	sep := " "
	if g.dialect.CopyParamsAreCsv {
		sep = ", "
	}

	params := g.expressions(exprsOptions{
		expression: e,
		key:        "params",
		sep:        sep,
		newLine:    true,
		skipLast:   true,
		skipFirst:  true,
		noIndent:   !g.dialect.CopyParamsAreWrapped,
	})

	if params != "" {
		if g.dialect.CopyParamsAreWrapped {
			params = " WITH (" + params + ")"
		} else if !g.pretty && (files != "" || credentials != "") {
			params = " " + params
		}
	}

	return "COPY" + this + kind + " " + files + credentials + params
}
