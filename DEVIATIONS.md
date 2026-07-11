# Deviations from upstream sqlglot

sqlglot-go is a faithful ~1:1 port of **tobymao/sqlglot v30.12.0**. This file records every place the
port *intentionally* behaves differently from the Python original, so downstream consumers and future
porters know exactly where — and why — the two disagree. It complements the per-site code comments
(grep `divergence`/`Unlike upstream`) and the `ROADMAP.md` "known divergences" + "resolved-findings"
ledgers, which carry the fine detail.

Deviations are grouped by how *observable* they are. Only **§1 changes same-dialect parse→generate
output** vs upstream; everything else is either cross-dialect-only, output-preserving, or a
not-yet-ported scope boundary.

---

## 1. Behavioral deviations (same input → different output than upstream)

### 1.1 ASCII-only identifier case-folding  — *the one that changes same-dialect output*

**What upstream does:** `Dialect.normalize_identifier` folds unquoted identifiers with Python
`str.lower()` / `str.upper()`, and `Dialect.case_sensitive` tests with `str.isupper()`/`str.islower()`
— all **full-Unicode** (`.reference/sqlglot-v30.12.0/sqlglot/dialects/dialect.py:1042-1050,1055-1064`).
So upstream normalizes unquoted `CAFÉ` → `café` (it also lowercases the `É`).

**What sqlglot-go does:** folds **ASCII-only** — `A-Z`↔`a-z` (bytes `0x41-0x5A`/`0x61-0x7A`), leaving
every byte `≥ 0x80` untouched (`dialects/dialect.go` `asciiLower`/`asciiUpper` + `CaseSensitive`). So
unquoted `CAFÉ` → `cafÉ` (the ASCII `C,A,F` fold; `É` is left alone). `CaseSensitive` likewise treats
an identifier that differs only by non-ASCII case (e.g. `cafÉ`) as already-normalized, not
needing-quotes.

**Why we diverge (correctness):** upstream over-folds — it does not match the database it models. Real
engines case-fold identifiers as an **ASCII** operation on multibyte encodings:

- **PostgreSQL** — `downcase_identifier()` in `src/backend/parser/scansup.c`. Per the PG commit
  *"Don't downcase non-ascii identifier chars in multi-byte encoding"*: *"Long-standing code has called
  `tolower()` on identifier character bytes with the high bit set. This is clearly an error and produces
  junk output when the encoding is multi-byte. This patch therefore restricts this activity to cases
  where there is a character with the high bit set AND the encoding is single-byte."* On UTF-8
  (`server_encoding=UTF8`) it degenerates to plain ASCII casing — only `A-Z` fold; multibyte sequences
  pass through unchanged. Empirically verified: `CREATE TABLE t (CAFÉ int)` →
  `information_schema.columns.column_name` = `cafÉ`, not `café`.
  The docs (§4.1.1 *Identifiers and Key Words*) state only that *"unquoted names are always folded to
  lower case"*; the ASCII-only detail lives in the source above.

ASCII-only is **exact for UTF-8** (the dominant server_encoding) and a **safe under-fold** otherwise.
The port has no DB connection and cannot know the actual encoding/locale, so it does not chase
per-encoding/locale rules (e.g. a single-byte-encoding `tolower()` under a specific locale). The same
ASCII-only fold applies to every strategy that folds (Lowercase → PostgreSQL/base; CaseInsensitive →
Presto/Trino/Athena/Hive; and the currently-unused Uppercase/CaseInsensitiveUppercase).

**Why it matters:** a downstream consumer keys column-level security policy off the *normalized*
identifier. If sqlglot-go folds `É` but the backend does not, the normalized key resolves to a different
column than the database binds — a correctness/safety bug. Beyond that consumer, it is simply modeling
the dialect correctly.

**Scope of the change:** `dialects/dialect.go` only — `NormalizeIdentifier` (fold) and `CaseSensitive`
(needs-quoting test). Quoted-identifier handling is unchanged (quoted names are never folded, which is
correct). Regression test: `identifier_casefold_test.go` (root, an original non-ported test). Full
`go test ./...` stayed green — no existing test had encoded the old full-Unicode fold (fixtures are
ASCII), so the blast radius was zero.

**Upstream status:** this is a real upstream bug (the Go port faithfully mirrored it before this fix).
Worth an upstream issue/PR to sqlglot proposing ASCII-restricted folding for the LOWERCASE/UPPERCASE
strategies; until/unless upstream changes, this stays a deliberate divergence.

### 1.2 MySQL-exact identifier folding — two non-upstream normalization strategies

**Background.** §1.1 makes the *base/Postgres* fold ASCII-only, which is exactly right for PostgreSQL.
**MySQL is different**: it folds identifiers **Unicode-simple and accent-PRESERVING**, under the fixed
`utf8mb3_general_ci` collation (`system_charset_info`) via that collation's `.tolower` map — verified
against the MySQL 8.0 source (`strings/ctype-utf8.cc` `my_unicase_default`; `sql/sql_base.cc`
`find_field_in_table` uses `my_strcasecmp(system_charset_info, …)`, with a literal `// Ñ != N` comment).
So MySQL treats `CAFÉ` == `café` (É→é, same column) and `NIÑO` == `niño`, but `CAFÉ` ≠ `CAFE` and
`Ñ` ≠ `N` (accents kept), `ß` stays `ß` (no `ss`). Quoting (backticks) does **not** affect case. Column
names are case-insensitive on every platform; database/table names are case-sensitive only when
`lower_case_table_names = 0` (the Linux default).

**Two things upstream does not have:**

1. **Two new `NormalizationStrategy` members** (`dialects/dialect.go`), used only by opt-in (see below):
   - `MySQLCaseInsensitive` — folds **every** identifier (columns and table/db names) with MySQL's map,
     regardless of quoting. Models `lower_case_table_names = 1/2`.
   - `MySQLCaseSensitiveTableNames` — **role-aware**: database/table name identifiers stay
     case-sensitive; every other identifier folds. Models `lower_case_table_names = 0` (Linux default).
     Role is decided by the identifier's parent — case-sensitive **only** for an `exp.Table`'s
     `this`/`db`/`catalog` child. `Column.table` is *excluded* (it is a table **alias** reference, not a
     table name, and must fold with the column so a qualified column keeps matching its lowercased
     source alias). When the parent is absent (a `Copy()`, a schema identifier, or `parse_identifier`),
     the identifier folds — the safe default for a security-keyed identity (never leaves a
     would-be-folded name un-folded).

2. **The MySQL fold algorithm itself** — the exported **`dialects.MySQLLower`**
   (`dialects/mysql_casefold.go`) folds via `mysqlLowerMap`, a byte-exact port of MySQL's `.tolower`
   map (696 BMP entries) **baked into generated Go code** (`dialects/mysql_casefold_table.go`, via
   `scripts/gen_mysql_casefold.py`; no runtime data file). This is deliberate and load-bearing:
   **neither Go's `strings.ToLower` (simple mapping) nor the JVM's `String.lowercase()` (full mapping)
   reproduces MySQL's table, and the two diverge from each other** on characters like `İ` (U+0130) and
   Greek final-sigma (empirically measured). `MySQLLower` is **exported so it is the single fold
   implementation across languages**: a consumer that must reproduce the normalized identifier
   byte-for-byte (e.g. the JVM proxy that keys column masking off it) should **call `MySQLLower`
   through a native binding** — one implementation, zero drift — or, failing that, regenerate the same
   table. Never substitute a stdlib case function.

**Default is unchanged (faithful to upstream).** MySQL's default `NormalizationStrategy` stays
`CASE_SENSITIVE` (upstream `mysql.py:25`) — no folding. The two MySQL strategies are **opt-in** via the
settings string (§1.3). Under the default, MySQL columns are *under-normalized* (`CAFÉ` ≠ `café`) — a
mask-evasion risk that the opt-in strategy closes. Postgres continues to use the ASCII fold of §1.1
(correct for Postgres).

**Which strategy to choose (identifier normalization for analysis/lineage/security keying):**

| dialect / situation | strategy | why |
|---|---|---|
| **PostgreSQL** | default (`LOWERCASE`, ASCII fold — §1.1) | PG folds unquoted names ASCII-only; quoted names stay case-sensitive. Nothing to change. |
| **MySQL on Linux** (or `lower_case_table_names=0`) | `mysql_case_sensitive_table_names` | Columns are case-insensitive on every platform (fold them); db/table names are case-sensitive on Linux (keep them). Closes the column mask-evasion gap while matching table semantics. |
| **MySQL on macOS/Windows** (or `lower_case_table_names=1` or `2`) | `mysql_case_insensitive` | There, db/table names are *also* case-insensitive, so fold everything. |
| **MySQL, must match upstream sqlglot exactly** (transpile/round-trip, not security keying) | default (`CASE_SENSITIVE`) | Upstream folds nothing for MySQL; keep parity. But be aware columns are under-normalized (`Foo` ≠ `foo`). |
| **Presto / Trino / Athena / Hive** | default (`CASE_INSENSITIVE`, ASCII fold) | Case-insensitive dialects; folded to lower. (ASCII fold is an approximation of engine-exact folding — see §2.) |
| **any dialect, must not fold at all** | `case_sensitive` | Treats every identifier as case-sensitive (no normalization). |

Rule of thumb for a **column-masking / security key**: pick the strategy that matches the *engine's
actual resolution* so two spellings of the same column produce the same key — for MySQL that means one
of the two MySQL strategies (never the `CASE_SENSITIVE` default), because MySQL resolves columns
case-insensitively regardless of quoting.

### 1.3 Overridable dialect settings (`normalization_strategy`)

`dialects.GetOrRaise` accepts upstream sqlglot's comma-separated settings string form —
`"mysql, normalization_strategy = mysql_case_sensitive_table_names"` — mirroring upstream's
`Dialect.get_or_raise` (`dialect.py:914-971`) + `SUPPORTED_SETTINGS`. This is a **feature the Go port
previously lacked** (a gap from upstream, now closed), not a behavioral divergence; the bare-name form
is unchanged. Only `normalization_strategy` is supported (upstream also has `version`, which this port
does not model); unknown settings/strategy values error, as upstream.

---

## 2. Cross-dialect-only deviations (never affect same-dialect round-trip)

The port's verified goal is **same-dialect round-trip** (read X → write X). Cross-dialect
transpilation (read X → write Y) is explicitly out of scope and only partially correct. These differ
from upstream only on the cross-dialect path:

- **Generator `TRANSFORMS` / `TYPE_MAPPING` not ported** for presto/trino/hive/athena (the
  parser+tokenizer side is ported for lineage; generators are skipped). Same-dialect functions
  round-trip via the base generator + `Anonymous` fallback. See `ROADMAP.md` "Athena support".
- **Functions left `Anonymous` instead of structured** where structuring would only matter for
  canonicalization/transpile: Presto/Trino `DATE_FORMAT`/`DATE_PARSE`/`TO_CHAR`/`DATE_TRUNC`/`REGEXP_*`/
  `LOCALTIME[STAMP]`, and `CONCAT_WS` (`parser/parser_functions.go`, `dialects/presto.go`). Lineage
  still sees their column args; same-dialect `.sql()` echoes them verbatim.

---

## 3. Output-preserving, Go-necessitated divergences (same output, internal difference)

Go's static typing / lack of metaclasses forces these; none change `.sql()` output:

- **Arg ordering:** `newNode` orders args by the per-Kind `argTypes` declaration order, not caller
  insertion order (Python dict preserves insertion). Equality/hash/find/walk are unaffected; the
  generator iterates `argTypes` order too. (`expressions/core.go`; `ROADMAP.md` known-divergences.)
- **`parseTable` fast-path skip** — a parse-order optimization divergence, same result
  (`parser/parser.go`).
- **`IsWrapper` uses the `truthy` helper** rather than Python's `v is None` (the port doesn't store nil
  args); equivalent for stored args.
- **`matchTextSeq` retreat has no logger** vs upstream's debug log (`parser/parser_stmt_common.go`).

---

## 4. Cosmetic AST-shape divergences (round-trip output identical, `.ToS()`/repr differs)

- **Comment bubbling:** a trailing comment can attach to a slightly different node than upstream; the
  comment still renders in the same textual position, so round-trip output matches (`parser` — see
  `ROADMAP.md`).
- **MySQL `PARTITION BY RANGE (YEAR|MONTH(col))`:** upstream wraps the 11 MySQL date functions in
  `TsOrDsToDate` in the parser and removes it in the generator; the port elides both consistently.
  Round-trips to identical SQL; only the incidental partition-expression arg shape differs
  (`generator/dialect_funcs.go`; capped in `fidelity_test.go` `maxASTDivergences`).

---

## 5. Deferred / fail-closed (parse to `exp.Command` where a future slice would structure)

These currently produce a raw-text `Command` (round-trips verbatim, fails closed) pending future work:

- **Postgres `CREATE FUNCTION ... AS $$...$$`** dollar-quoted (heredoc) bodies — `exp.Heredoc` unmodeled
  (`parser/parser_ddl.go`).
- **Hive CREATE-DDL property callbacks** (`CLUSTERED`/`EXTERNAL`/`LOCATION`/`ROW`/`STORED`/
  `TBLPROPERTIES`/`USING`) live in Hive's `PropertyParsers` overlay, deliberately kept out of the shared
  base registry until a paired parser+generator slice, so base/mysql/postgres/presto keep failing them
  closed (`parser/dialect_hive_overrides.go`).

---

## Not deviations (called out to avoid confusion)

- Where a reviewer flagged an "upstream bug," the port generally **keeps upstream's behavior 1:1** (e.g.
  a qualify_columns edge case, `optimizer/qualify_columns.go`) rather than silently "fixing" it — that
  is *faithfulness*, the opposite of a deviation. §1.1 is the deliberate exception, made only because it
  is a genuine correctness/safety issue against the modeled engine.
