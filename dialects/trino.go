package dialects

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// Trino ports the dialect deltas in
// .reference/sqlglot-v30.12.0/sqlglot/dialects/trino.py:9-20 and the function
// registry additions in .reference/sqlglot-v30.12.0/sqlglot/parsers/trino.py:15-18.
// Each call starts from a fresh Presto instance so no inherited map is shared with
// a separately constructed dialect.
func Trino() *Dialect {
	d := Presto()
	d.Name = "trino"
	d.SupportsUserDefinedTypes = false
	d.Functions["VERSION"] = exp.FromArgListFunc(exp.KindCurrentVersion)
	// ARRAY_FIRST is inherited from the upstream base function registry, but that
	// entry is not in the Go base port yet. Backfill it only for Trino/Athena so the
	// established base, MySQL, Postgres, Presto, and Hive registries stay unchanged.
	d.Functions["ARRAY_FIRST"] = exp.FromArgListFunc(exp.KindArrayFirst)

	cfg := d.TokenizerConfig
	cfg.Keywords["REFRESH"] = tokens.REFRESH
	d.TokenizerConfig = tokens.CompileConfig(cfg)
	return d
}
