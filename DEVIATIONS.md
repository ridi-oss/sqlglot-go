# Deviations from upstream sqlglot

sqlglot-go is a faithful ~1:1 port of **tobymao/sqlglot v30.12.0**. This file records every place the
port *intentionally* behaves differently from the Python original, so downstream consumers and future
porters know exactly where — and why — the two disagree. It complements the per-site code comments
(grep `divergence`/`Unlike upstream`) and the `ROADMAP.md` "known divergences" + "resolved-findings"
ledgers, which carry the fine detail.

Deviations are grouped by how *observable* they are. Only **§1 changes same-dialect parse→generate
output** vs upstream; everything else is either cross-dialect-only, output-preserving, a
not-yet-ported boundary, or a Go-only analysis API / scope extension.

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
   - `MySQLCaseSensitiveTableNames` — **role-aware**: relation-level identifiers stay case-sensitive;
     column-level identifiers fold. Models `lower_case_table_names = 0` (Linux default), where MySQL
     resolves **table/db names, table aliases, CTE names, and column qualifiers** case-sensitively but
     **column names** case-insensitively. Role is decided by the identifier's parent + arg key (see
     `isRelationLevelIdentifier`): **preserved** for an `exp.Table` `this`/`db`/`catalog`, an
     `exp.Column` `table`/`db`/`catalog` (a *qualifier* — it references a relation/alias), and an
     `exp.TableAlias` `this` (a table alias or CTE name); **folded** for everything else — an
     `exp.Column` `this` (the leaf column name), an `exp.TableAlias` `columns` entry (a CTE
     output-column), an `exp.Alias` `alias` (a column alias), JOIN USING columns, etc. This matches
     MySQL 8.4 exactly: `SELECT users.rrn FROM Users` errors because the qualifier is case-sensitive,
     and `WITH Users AS (…) … FROM users` misses the CTE because CTE names are case-sensitive, while
     column names fold. (Folding `Column.table` — as if a qualifier were column-level — makes a
     qualified column against an unaliased mixed-case table, or a mixed-case CTE reference, resolve to
     the wrong relation.) When the parent is absent (a `Copy()`, a schema identifier, or
     `parse_identifier`), the identifier folds — the standalone-name default.

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

**Caveat — MariaDB CTE names diverge.** These strategies model **MySQL**. Empirically (live probes,
lctn=0): MySQL 5.7 / 8.0.33 / 8.4 and MariaDB 10.11 / 11.4 all agree that table/db names, column
qualifiers, and table aliases are case-**sensitive** and column names case-**insensitive** — but MariaDB
resolves **CTE names case-INSENSITIVELY even at lctn=0**, whereas MySQL treats them case-sensitively
(`WITH Users AS (…) … FROM users` errors on MySQL, binds on MariaDB). So `MySQLCaseSensitiveTableNames`
is exact for MySQL but **over-preserves CTE names on MariaDB** (a mixed-case CTE reference vs definition
would get distinct normalized keys — the same class of mask-miss this strategy otherwise closes, but for
CTE-derived columns). MariaDB is not a ported dialect; a faithful MariaDB variant would fold
`TableAlias.this` when it is a CTE name. If you key security off normalized identifiers on **MariaDB**,
treat CTE-derived columns with care.

**Default is unchanged (faithful to upstream).** MySQL's default `NormalizationStrategy` stays
`CASE_SENSITIVE` (upstream `mysql.py:25`) — no folding. The two MySQL strategies are **opt-in** via the
settings string (§1.3). Under the default, MySQL columns are *under-normalized* (`CAFÉ` ≠ `café`) — a
mask-evasion risk that the opt-in strategy closes. Postgres continues to use the ASCII fold of §1.1
(correct for Postgres).

**Which strategy to choose (identifier normalization for analysis/lineage/security keying):**

| dialect / situation | strategy | why |
|---|---|---|
| **PostgreSQL** | default (`LOWERCASE`, ASCII fold — §1.1) | PG folds unquoted names ASCII-only; quoted names stay case-sensitive. Nothing to change. |
| **MySQL on Linux** (or `lower_case_table_names=0`) | `mysql_case_sensitive_table_names` | Column *names* are case-insensitive on every platform (fold them); table/db names, table aliases, CTE names, and column *qualifiers* are case-sensitive on Linux (keep them). Closes the column mask-evasion gap while matching lctn=0 relation resolution exactly. |
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

The dialect-accepting entry points (`dialects.GetOrRaise`, `optimizer.NormalizeIdentifiers`,
`optimizer.QualifyOpts.Dialect`) now accept a **DialectType-style value** — `nil` | a string (bare name
or the settings form) | a `*dialects.Dialect` — mirroring upstream's polymorphic `DialectType =
Union[str, Dialect, Type[Dialect], None]` (`dialect.py:1171`). This *restores* upstream API
compatibility the earlier string-only port had narrowed. A passed `*Dialect` is threaded through the
optimizer and `schema.EnsureSchema` **unchanged** — `EnsureSchema(...).Dialect()` returns the caller's
instance — so every instance field the qualify passes read (`NormalizationStrategy`,
`ForceEarlyAliasRefExpansion`, `TablesReferenceableAsColumns`, `DefaultFunctionsColumnNames`, …) is
honored, matching upstream `qualify.py:78`. Only the schema's per-name fold re-resolution and
identifier parsing still take a string; for those the dialect is reduced to its canonical settings
string (`(*Dialect).SettingsString()` / `dialects.CanonicalString`), which round-trips its name +
`NormalizationStrategy` through `GetOrRaise`.

### 1.4 MySQL `--` line comment requires a trailing space — *fixes an upstream tokenizer bug*

**What upstream does:** sqlglot's tokenizer treats `--` as a line-comment start unconditionally in every
dialect, so it tokenizes MySQL `SELECT 1--2` as `SELECT 1` (dropping `--2` as a comment). Verified on the
pinned reference: `tokenize("SELECT 1--2", dialect="mysql")` → `[SELECT, 1]`.

**What sqlglot-go does:** for MySQL, `--` begins a line comment only when the next character is
**ASCII whitespace/control or EOF**; otherwise it is two `-` operators. `SELECT 1--2` → `SELECT 1 - -2`
(tokens `[SELECT, 1, -, -, 2]`). Implemented via `TokenizerConfig.LineCommentRequiresSpace{"--": true}` on
the MySQL dialect + a guard in `tokens.TokenizerCore.lineCommentSuppressed`; base and Postgres are
untouched (Postgres `--` stays an unconditional comment, per the SQL standard). Verified against MySQL 8.4:
`SELECT 1--` (marker at EOF) is a comment; `SELECT 1--<NBSP>2` errors (a non-ASCII space like U+00A0 does
**not** trigger the comment — only ASCII whitespace/control does), so the trigger is ASCII-restricted.

**Why we diverge (correctness):** this matches the real server. MySQL's manual: *"the `--` comment style
requires the second dash to be followed by at least one whitespace or control character (such as a space,
tab, newline, and so on)."* So `1--2` evaluates to `1 - (-2) = 3` on a real MySQL, not `1`. Upstream
over-eagerly comments it out; a consumer that relies on the token stream to distinguish `SELECT 1--2` from
`SELECT 1` would otherwise conflate them. Regression test: `tokenizer_mysql_comment_test.go`.

### 1.5 MySQL executable/version comment activation (opt-in, `mysql_version`)

**What upstream and the default do:** pinned sqlglot v30.12.0 strips MySQL executable comments
(`/*! ... */` and `/*!NNNNN ... */`) from the token stream exactly like ordinary block comments. The body
is retained only as comment metadata; it is never parsed as SQL, regardless of the gate. sqlglot-go's bare
`mysql` behavior remains identical. Activation is explicitly opt-in through a dialect setting such as
`"mysql, mysql_version=80035"`; leaving `mysql_version` unset preserves upstream behavior and corpus parity.

**Version and gate semantics:** `mysql_version` is MySQL's `MYSQL_VERSION_ID` integer — the comparable
value `major*10000 + minor*100 + patch` (`80035` for MySQL 8.0.35). This is exactly the `/*!NNNNN` gate form
and precisely what the C API `mysql_get_server_version()` returns, so a client passes the integer it already
has; a dotted version string is intentionally **not** accepted (it silently mis-parsed as a major version and
over-activated near-boundary gates). A bare `/*! ... */` body always activates when the setting is present.
For `/*!NNNNN ... */`, the first five digits are the gate: the body activates when the configured
`MYSQL_VERSION_ID` is greater than or equal to it (`50000` and `80033` activate at `80035`; `80036` and
`99999` do not). Active bodies are tokenized as SQL and the wrapper plus gate disappear; inactive bodies
remain comment metadata, including their leading `!` and digits.

**Scope:** only `/*!` version comments are activated. MySQL optimizer-hint comments (`/*+ ... */`) are left
as ordinary comments (stripped), matching upstream — hints do not change the set of columns/tables a
statement reads, so this is correct for the lineage/grant-hash consumers this extension serves.

Only the MySQL dialect advertises the executable-comment capability. `mysql_version` is nevertheless a
recognized setting for every dialect string so shared configuration can pass it uniformly: base and
Postgres accept it but leave the body inactive/comment-only. Malformed versions still error for every
dialect. `SettingsString` intentionally omits this tokenizer-only, per-call state because that method
serializes identifier-resolution/qualify state.

**Generation caveat:** activation is semantic, not a byte-preserving comment rewrite. An active
`SELECT 1 /*!50000 + 100 */` parses and regenerates as `SELECT 1 + 100`; likewise a hidden select item such
as `SELECT 1 /*!50000, rrn */ FROM t` regenerates as `SELECT 1, rrn FROM t`. Inactive/default wrappers pass
through the existing comment sanitizer, normally rendering with a space after `/*`, for example
`SELECT 1 /* !99999 + 100 */`. Do not expect the original `/*!...*/` bytes to survive regeneration.

This behavior was checked against MySQL 8.4.9: gate `50000` executes, gate `99999` does not, and the hidden
column form executes and exposes the extra select item. The implementation lives in `dialects/dialect.go`,
`dialects/mysql.go`, `tokens/tokenizer.go`, and `tokens/tokenizer_core.go`; low-level tokenizer tests live
beside those packages, and the public regression is `mysql_version_comment_test.go`.

**Why this is an opt-in behavioral extension:** the accepted SQL grammar is unchanged — both implementations
already recognize the wrapper as a comment, and its body uses the existing SQL grammar when explicitly
activated. The extension changes whether comment-contained SQL participates in the token stream for a
configured server version. Therefore no `testdata/upstream_extensions.jsonl` row is appropriate; that ledger
tracks grammar accepted beyond the pinned upstream parser, not opt-in executable-comment semantics.

---

## Opt-in behavioral extensions beyond upstream

### Search-path-aware table qualification

Pinned upstream `qualify_tables` accepts one fixed `db`/`catalog` and stamps those parts without an
existence check (`.reference/sqlglot-v30.12.0/sqlglot/optimizer/qualify_tables.py:16-23,62-75`). The
Go-only `optimizer.QualifyOpts.SearchPath` adds a separate, opt-in resolution mode. The mode switch is
exact: a nil or empty `SearchPath` uses the existing upstream-faithful fixed `DB`/`Catalog` path
unchanged, so all existing fixture output remains unchanged; only a non-empty `SearchPath` enables the
extension.

In non-empty mode, candidates are dialect-normalized and checked in order against the supplied schema.
A candidate database is stamped only when `schema.Find(..., false, false)` returns a mapping for that
candidate, and the first proven candidate wins. There is no fallback to the fixed `DB`, and no catalog
is added. If no candidate is proven, the table remains unqualified so downstream policy can fail
closed. Absent, ambiguous, empty, flat, or otherwise db-incapable schemas therefore produce no stamp.
Already-qualified tables and CTE references are preserved and are not rewritten.

The lookup requires a schema whose `SupportedTableArgs` includes `db`. An empty schema or a flat,
table-only mapping is intentionally insufficient because it cannot prove that a table exists in a
specific database; probing it as though it could would allow a mapping implementation to truncate the
db-qualified lookup and return an unrelated unscoped table.

This boundary is security-relevant: guessing a database can bind access analysis to a table that the
actual database would not resolve, creating a wrong-ALLOW decision. Leaving the database absent when
its resolution cannot be proven is the safe result.

**Dimension resolved (schema/db only; catalog is the caller's).** `SearchPath` resolves the **schema/db**
part of a table name; it never stamps `catalog`. The probe is a two-part `Table{this, db}`, and this
resolves correctly against a **three-level `catalog.schema.table`** schema too (verified: with a
`{cat:{schema:{table}}}` mapping, `SELECT * FROM t` under `SearchPath=[schema…]` stamps `db=<schema>` and
leaves `catalog` unstamped) — `schema.Find` matches a db-qualified probe by schema-level existence, not
requiring the catalog level, so a depth-3 consumer is **not** silently over-denied. The stamp is a
schema-level existence superset: a multi-catalog consumer supplies its own `catalog` (via `opts.Catalog`
or downstream) and performs the full `catalog.schema.table` resolution, which fail-closes if the table
does not exist in *its* catalog. So the division is: **R1 fixes the schema/db dimension by proven
schema-existence; the caller fixes and enforces the catalog.**

This is not a parse-grammar construct, so it is neither registered in
[`testdata/upstream_extensions.jsonl`](./testdata/upstream_extensions.jsonl) nor governed by the
grammar-extension tripwire.

---

## Grammar extensions beyond upstream

Grammar extensions are output-round-tripping AST extensions: they preserve valid same-dialect SQL output
but intentionally produce a more useful structured AST than pinned upstream. They are governed by the
extension ledger in [`testdata/upstream_extensions.jsonl`](./testdata/upstream_extensions.jsonl), not by
the §1 discipline for correctness fixes against real-engine bugs. The always-on
`TestUpstreamExtensionsGoSide` checks the recorded Go root Kind, and the `.reference`-gated
`TestUpstreamExtensionsTripwire` re-checks pinned upstream's behavior so a future reference bump cannot
silently collide with an extension.

The first registered construct is ledger id
[`pg-explain`](./testdata/upstream_extensions.jsonl). For Postgres, literal `EXPLAIN` tokenizes through the
`DESCRIBE` statement path and builds a `Describe` root with structured `CopyParameter` option children, a
parsed inner statement, and an internal `kind = "EXPLAIN"` discriminator. The Postgres generator uses that
discriminator to render either parenthesized comma-separated options or legacy space-separated options;
the existing base/MySQL literal `DESCRIBE` generation remains unchanged. Unsupported `EXPLAIN` forms
fail closed to
`Command` and round-trip verbatim. Pinned sqlglot v30.12.0 also returns `Command` for the ledgered example;
use the stable ledger id and tripwire for its reconciliation lifecycle rather than copying the row's
mutable reconciliation instructions here.

Ledger id [`mysql-insert-set`](./testdata/upstream_extensions.jsonl) registers MySQL `INSERT ... SET`,
which pinned upstream rejects with a parse error. The MySQL parser intentionally normalizes assignments
such as `INSERT INTO t SET a = 1, b = 2` directly to the existing `Insert` shape whose target is
`Schema(Table, columns)` and whose source is one-row `Values(Tuple(values))`; it therefore renders as
`INSERT INTO t (a, b) VALUES (1, 2)`, and that canonical form is idempotent across subsequent
parse/generate cycles. The extension is MySQL-only. Use the stable ledger id for its reconciliation
lifecycle.

Ledger id [`mysql-replace`](./testdata/upstream_extensions.jsonl) registers structural MySQL `REPLACE`,
which pinned upstream packs as a tokenizer-level `Command`. The MySQL tokenizer no longer performs that
packing, and the parser represents supported statements as `Insert` with the Go-only optional
`replace = true` marker. Only MySQL generation consults the marker and renders `REPLACE`; unmarked
`Insert` nodes and other dialects retain their existing behavior. A leading `REPLACE(` retreats to
ordinary expression parsing to disambiguate function calls from statements, while unsupported or
partially consumed statement forms fail closed to a source-preserving `Command`. Use the stable ledger
id for its reconciliation lifecycle.

---

## Opt-in behavioral extensions beyond upstream

### Qualify resolution report

The Go-only `optimizer.SourceKind`, `optimizer.ResolvedSource`, and
`optimizer.QualifyOpts.ResolutionReport` API exposes the source resolution already performed by the
qualify scope pass. It is additive: upstream's `Callable[[exp.Table], None]` callback shape remains
unchanged as `QualifyOpts.OnQualify`, rather than being enriched with Go-only report data.

Classification follows resolved scope relationships, not table-shaped syntax or name guessing. A
selected source's dynamic type and `ScopeType` distinguish physical tables, CTEs, derived tables, and
other scopes; physical identity comes from the resolved table's catalog, db, and name. `Unresolved` is
intentionally the zero value so missing or unclassified results fail closed. Scalar and predicate
subqueries are emitted from their existing `ScopeTypeSubquery` scopes.

For a DML root (`UPDATE`/`DELETE`/`MERGE`), the report is populated from the R3 **analysis** traversal
(`TraverseScope`), which carries the DML-root scope — so the DML target *and* its `FROM`/`USING`/`JOIN`
read-sources classify (a physical source → Physical, a CTE/derived source → CTE/Derived), not just the
target. Column qualification itself still uses the upstream-faithful optimizer traversal, which omits
those DML-root scopes. A malformed/incomplete DML for which R3 omits the root scope falls back to
supplementing only the exact grammatical target as Physical and leaving other root-level references
Unresolved (fail-closed).

A nil `ResolutionReport` performs no population and preserves all existing SQL and AST behavior. A
caller-supplied map is populated during `Qualify`'s existing scope pass: a non-DML root reuses the
`QualifyColumns` optimizer traversal (no second pass); a DML root uses the analysis traversal as above;
and a minimal scope pass covers the column-qualification-disabled case. This is an analysis API, not a
parse construct, so it does not add or change an entry in `testdata/upstream_extensions.jsonl` and no
upstream-extension tripwire applies.

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

## 6. Go-only analysis API extensions

### 6.1 Top-level UPDATE/DELETE/MERGE scopes

**What upstream does:** in v30.12.0, `traverse_scope` traverses the CTE and nested-query scopes under
an `Update`, `Delete`, or `Merge`, then returns without yielding a scope for the DML root itself
(`.reference/sqlglot-v30.12.0/sqlglot/optimizer/scope.py:700-706`).

**What sqlglot-go does:** the public analysis APIs, `TraverseScope` and `BuildScope`, and the
package-private `traverseScope` analysis traversal additionally yield a root `Scope` over the original
`Update`, `Delete`, or `Merge` AST, but only after resolving its complete source graph. Sources are registered in
deterministic order and comprise the physical write target plus `FROM` / `USING` sources and
recursively attached `JOIN` `Table` / `Subquery` sources. Existing CTE and nested-query scopes are
preserved and attached: unqualified read-side table references bind to `WITH` CTE scopes, and source
subqueries bind to their child scopes.

**Fail-closed guarantee:** if a source has a malformed or unsupported shape, traversal logs a warning
and omits only the DML-root scope. It retains all CTE and nested-query child scopes already traversed,
never emits a root with a partial source set, and never panics. A missing source can hide columns from
lineage or access analysis and produce a wrong-ALLOW result, so this deliberate Go extension favors
complete-or-none DML-root scope emission.

**Optimizer containment:** `QualifyTables`, `QualifyColumns`, `ValidateQualifyColumns`, and
`IsolateTableSelects` use a separate internal compatibility traversal that reproduces pinned
v30.12.0 behavior. It excludes the entire R3-only augmentation: both the DML-root scope and any
source-query scopes traversed solely to bind that root. Filtering only the root is incorrect because
upstream's generic DML walk is shape-dependent, as verified by differential tests: `UPDATE` `FROM` /
`JOIN` source subqueries can remain unvisited, while `DELETE` / `MERGE` `USING` subqueries can be
traversed and qualified. Preserving that asymmetry avoids same-dialect output drift and
correlated-subquery validation panics. This optimizer-only compatibility route does not weaken the
public analysis path's complete-or-none source-set guarantee for lineage and access analysis.

**Tests and parse/generate status:** DML scope and source-subquery optimizer-parity regressions are
covered in `optimizer/scope_dml_test.go`. No `upstream_extensions.jsonl` entry applies because this
extension changes neither parsing nor SQL generation; it exposes additional analysis scopes over the
existing AST and grammar.

---

## Not deviations (called out to avoid confusion)

- Where a reviewer flagged an "upstream bug," the port generally **keeps upstream's behavior 1:1** (e.g.
  a qualify_columns edge case, `optimizer/qualify_columns.go`) rather than silently "fixing" it — that
  is *faithfulness*, the opposite of a deviation. §1.1 is the deliberate exception, made only because it
  is a genuine correctness/safety issue against the modeled engine.
