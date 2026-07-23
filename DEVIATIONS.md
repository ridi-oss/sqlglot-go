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
     the wrong relation.) When the parent is absent (a lone `Copy()` or `parse_identifier` of a single
     name), the identifier folds — the standalone-name default; bulk schema normalization does **not**
     rely on that default (see *Bulk-schema normalization* below).

     **`INFORMATION_SCHEMA` exception.** MySQL matches `INFORMATION_SCHEMA` case-**insensitively
     regardless of lctn** — uniquely among schemas — because it is a virtual (synthesized) schema, not
     an on-disk directory. Live-verified on MySQL 8.0.46 (lctn=0): `INFORMATION_SCHEMA.tables`,
     `information_schema.TABLES`, and mixed case all resolve, as do its table names in any case; but
     `PERFORMANCE_SCHEMA`, `MySQL`, and `SYS` are ordinary on-disk DBs and stay case-sensitive. So
     `MySQLCaseSensitiveTableNames` folds a relation identifier — despite being relation-level — when it
     names or qualifies `information_schema`: the schema name itself, a table name under it, and an
     `exp.Column` `table` qualifier under it (see `isInformationSchemaRelationPart`, which reads the
     sibling `db`, never the node's own kind). `performance_schema`/`mysql`/`sys` are left
     case-sensitive. Under `MySQLCaseInsensitive` (lctn=1/2) everything folds anyway, so no special case
     is needed there. Upstream models none of this (its MySQL is `CASE_SENSITIVE`, folding nothing).

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

**Bulk-schema normalization (role-aware + fail-closed).** `schema.NewMappingSchema(mapping,
normalize=true)` normalizes each catalog/schema/table key by assembling the key path into an `exp.Table`
and normalizing *that* — not by folding each bare string in isolation. This gives every relation key its
parent (so the role-aware lctn=0 strategy preserves it, instead of misreading a parentless identifier as
a foldable column — the bug this fixes) and its sibling `db` (so the `INFORMATION_SCHEMA` exception fires
on the schema side exactly as on the query side). Non-role-aware strategies ignore the parent, so their
normalized keys stay byte-identical to per-key folding. **Kind-1 injectivity:** if two distinct raw keys
fold to the same normalized key, `NewMappingSchema` **fails closed** (a `SchemaError` — `duplicate
normalized {catalog,schema,table,column} …`) instead of silently merging the two identities (upstream
`nested_set` is last-wins). This applies to **every** folding dialect (including Postgres), because a
non-injective fold under a security key is a fail-open hazard; it is the only part of §1.2 that reaches
beyond the MySQL strategies, and it only ever turns a silent merge into a loud error — never a
parse→generate change.

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

### 1.6 MySQL `RESET …` degrades to `Command` (not a bogus `Alias`)

**What upstream does:** pinned sqlglot v30.12.0 does not tokenize MySQL `RESET` as a keyword, so
`RESET MASTER` falls into the generic expression-statement path and parses as an `Alias` —
`Alias(this=Column(RESET), alias=MASTER)`, i.e. the expression `RESET` aliased `AS MASTER`. Verified on the
pinned reference: `parse_one("RESET MASTER", "mysql")` → `Alias`, `.sql()` = `RESET AS MASTER`. The sibling
`RESET BINARY LOGS AND GTIDS` parse-errors.

**What sqlglot-go does:** MySQL maps `RESET` to a `COMMAND` token (as Postgres already does), so the whole
statement degrades to a raw `exp.Command{this: "RESET"}` — `RESET MASTER` / `RESET REPLICA` /
`RESET BINARY LOGS AND GTIDS` all round-trip unchanged. `reset` as an ordinary identifier
(`SELECT reset FROM t`) is unaffected.

**Why we diverge (correctness):** `RESET …` is an administrative statement; upstream's `Alias` is a
semantically wrong structural claim (there is no alias) that a tree consumer could read as a harmless aliased
expression. A `Command` is the faithful "not structurally modelled" node and matches the real server's intent
(and Postgres's own `RESET` handling here). Verified against MySQL 8.4 (`RESET MASTER` was removed there;
`RESET REPLICA` / `RESET BINARY LOGS AND GTIDS` are valid — all degrade to `Command`). Implemented in
`dialects/mysql.go`.

### 1.7 Postgres `U&'…'` / `U&"…"` Unicode escapes are decoded

**What upstream does:** pinned sqlglot leaves the Postgres `UNICODE_STRINGS` tokenizer set empty, so it
mis-tokenizes `U&'\0067'` as `U & '\0067'` — a bitwise-AND of a column named `U` with an ordinary (undecoded)
string literal — and parse-errors the quoted-identifier form (`… FROM U&"inf\006Frmation_schema".tables`).
Verified on the pinned reference. (Where upstream *does* wire `UNICODE_STRINGS` — Presto/Oracle — it keeps the
escapes raw in a `UnicodeString` node and never decodes them.)

**What sqlglot-go does:** for Postgres, `U&'…'` (string) and `U&"…"` (quoted identifier) are recognized and
their SQL-standard backslash-Unicode escapes are decoded into the real code points — `\XXXX`, `\+XXXXXX`,
`\\`, and UTF-16 surrogate pairs — producing an ordinary decoded string `Literal` / quoted `Identifier`. So
`U&'\0067\0072\0061\0064\0065'` is the string `'grade'` and `U&"inf\006Frmation_schema"` is the identifier
`information_schema`, matching what the server executes. A trailing custom `UESCAPE 'c'` clause is not
consumed, so those rare forms fail closed (parse error) rather than decode against the wrong escape character.
Presto/Oracle `UnicodeString` handling is untouched.

**Why we diverge (correctness):** `standard_conforming_strings` is on by default in Postgres, which evaluates
`U&'…'`/`U&"…"` as decoded strings/identifiers — they are pure alternate spellings. Upstream's `U & '…'` is a
wrong parse, and for an AST-based analyzer the identifier form is a real blind spot: a name (`set_config`, a
system schema, a masked column) spelled with escapes is invisible to every name-based check while the DB runs
it. Decoding surfaces the effective name. Verified against PostgreSQL 17.6 (`U&'\0067\0072\0061\0064\0065'` →
`grade`; `U&"inf\006Frmation_schema"` resolves to the live `information_schema`). Implemented in
`tokens/unicode_escape.go`, `tokens/tokenizer.go` (`FormatString.DecodeUnicode`), `tokens/tokenizer_core.go`,
and `dialects/postgres.go`. The `U&"…"` identifier form is also registered in
`testdata/upstream_extensions.jsonl` (`pg-unicode-identifier`, upstream parse-errors); regression tests:
`unicode_escape_test.go` and `tokens/unicode_escape_test.go`.

### 1.8 `SAVEPOINT` / `RELEASE SAVEPOINT` parse to `Savepoint` (not a bogus `Alias` / parse error)

**What upstream does:** pinned sqlglot v30.12.0 has no savepoint statement node. `SAVEPOINT s`
mis-parses as an `Alias` — `Alias(this=Column(SAVEPOINT), alias=s)`, rendering `SAVEPOINT AS s` — i.e.
the expression `SAVEPOINT` aliased `AS s`, and `RELEASE SAVEPOINT s` parse-errors. Verified on the
pinned reference for both postgres and mysql. Only `ROLLBACK TO SAVEPOINT s` is modelled (as
`Rollback(savepoint=s)`).

**What sqlglot-go does:** `SAVEPOINT <name>` and `RELEASE [SAVEPOINT] <name>` build a new
`exp.Savepoint{this: name, kind}` root (`kind = "RELEASE"` for the release form). `SAVEPOINT`/`RELEASE`
stay ordinary `VAR` tokens — the statement is dispatched by leading text in `parseStatement`, so
`savepoint`/`release` remain usable as identifiers everywhere else (`SELECT savepoint FROM t` is
unchanged) and `ROLLBACK TO SAVEPOINT` is still an `exp.Rollback`. The bare `RELEASE <name>` spelling
(SAVEPOINT keyword omitted) is **Postgres-only** — real MySQL requires the keyword — so under
mysql/base a bare `RELEASE <name>` is left to the normal path (fails closed). Output normalizes the
release form to the explicit `RELEASE SAVEPOINT <name>` (which Postgres also accepts).

**Why we diverge (correctness):** `SAVEPOINT` / `RELEASE SAVEPOINT` / `ROLLBACK TO SAVEPOINT` are
standard SQL transaction-control statements — benign session/transaction state with no data access. An
AST consumer that gates on statement kind needs to route them to session passthrough like
`BEGIN`/`COMMIT`/`ROLLBACK`; upstream's `Alias`/parse-error is a wrong structural claim (like §1.6's
`RESET`) that would make a valid savepoint statement indistinguishable from an aliased column or a
parse failure. Verified against PostgreSQL 17.6 and MySQL 8.0.33 (`SAVEPOINT s`, `RELEASE SAVEPOINT s`,
plus Postgres's optional-keyword `RELEASE s`, all execute). Implemented in `parser/stmt_transaction.go`
(`parseSavepointStatement`) + `parser/parser.go` (the `parseStatement` hook) + `generator/
stmt_transaction.go` (`savepointSQL`); regression test `savepoint_test.go`. The `RELEASE SAVEPOINT`
form, which pinned upstream parse-errors, is additionally registered in
`testdata/upstream_extensions.jsonl` (`release-savepoint`) with a tripwire.

### 1.9 Postgres/MySQL reject a bare string as a table name or table alias

**What upstream does:** pinned sqlglot v30.12.0 calls `_parse_string_as_identifier` **unconditionally** in
`_parse_table_part` (`parser.py:4668`) and `_parse_table_alias` (`parser.py:4111`) — there is no flag — so
`SELECT * FROM 'foo'` folds the string into a table name and `SELECT * FROM t 'x'` folds it into a table
alias, in *every* dialect. Verified on the pinned reference (base/postgres/mysql all accept both). This is
separate from the projection string-alias `STRING_ALIASES` gate (the `Dialect.StringAliases` flag): that one only
governs `_parse_alias`.

**What sqlglot-go does:** a bare string is accepted as a table name/alias only when the dialect's
`StringTableIdentifiers` flag is true. It defaults **true** (accept — matching upstream's unconditional
behavior, so base and the presto/trino/hive/athena partial dialects are unchanged), but is **false** for
postgres and mysql, so `SELECT * FROM 'foo'` and `SELECT * FROM t 'x'` fail closed there. Ordinary
identifier and `AS`/space-aliased tables (`FROM t`, `FROM t AS x`, `FROM t x`) are unaffected.

**How it fails closed (two shapes).** For a `SELECT`/`UPDATE`/`DELETE`/`INSERT`/`DROP`/`TRUNCATE`/`ALTER`/
`COMMENT ON` statement the rejected string table part is a hard **parse error** (`ParseOne` returns an
error), because those paths have no error isolation. `CREATE`-family DDL and `GRANT`/`REVOKE`, however, wrap
their structured parse in `tryParse` and degrade to a raw-text `exp.Command` on *any* internal failure — so
`CREATE TABLE 'foo' (a INT)`, `CREATE VIEW 'foo' AS …`, and `GRANT SELECT ON 'foo' TO bob` become a
verbatim, unexecutable `Command` rather than a parse error. Both shapes are **fail closed** for a consumer
that rejects unparsed statements and opaque `Command` nodes (the port's standard "not structurally modelled"
node). Note the `Command` shape is a net improvement over the pre-flag behavior, which folded the string
into a real `Table` and thereby **normalized invalid SQL into a valid statement** (`CREATE TABLE "foo"`) — a
`Command` preserves the raw, still-invalid text instead. A tree-walking consumer must therefore treat a
`Command` as deny, not as "no tables found"; this matches how the port already surfaces other unmodelled
statements (e.g. `SHOW CREATE USER`).

**Why we diverge (correctness):** real PostgreSQL and MySQL both reject a bare string constant in
table-name or table-alias position (a table name/alias is an identifier, not a string literal); upstream's
unconditional fold is a permissive over-accept that rewrites invalid SQL into a different, executable
statement (`FROM t AS "x"` / `FROM "foo"`). An AST consumer needs the invalid form to fail closed, not be
normalized. This is orthogonal to the projection string-alias gate: **MySQL accepts** the projection `SELECT 1 'x'`
(`StringAliases=true`) yet **rejects** the table forms (`StringTableIdentifiers=false`) — the two flags are
independent. Base keeps upstream's accept (no real-engine reference). Verified against PostgreSQL 17.6 and
MySQL 8.0.33 (both reject `FROM 'foo'` and `FROM t 'x'`). Implemented in `dialects/dialect.go`
(`StringTableIdentifiers`, default true; postgres/mysql false) and the two gated call sites in
`parser/parser.go`; regression test `parser/string_aliases_test.go` (`TestStringTableIdentifiersFlag`).

**Out of scope (a separate pre-existing quirk, like the projection `AS '…'` case above):** an *explicit*
`AS` + string table alias — `SELECT * FROM t AS 'x'` — is still accepted for both dialects. This gate only
wraps the no-`AS` string fallback in `_parse_table_alias`; the `AS` form goes through `parseIdVar`'s
`advanceAny` path (which absorbs any token, including a `STRING`, whenever `AS` was matched) and is unchanged
here, matching pinned upstream even though real PG/MySQL reject it. A follow-up could gate that path too.

### 1.10 Postgres `pg_catalog.<builtin>` resolves to the builtin node (not USER-DEFINED)

**What upstream does:** pinned sqlglot v30.12.0 parses a schema-qualified Postgres type name whose schema is
`pg_catalog` and whose tail is a builtin as a **USER-DEFINED** `DataType` — `CAST('5' AS pg_catalog.int4)`
and `'5'::pg_catalog.int4` both become `DataType(this=USERDEFINED, kind=Dot(pg_catalog, int4))`. Verified on
the pinned reference for `pg_catalog.int4`/`text`/`timestamptz`/`bool`/`float8` (all USER-DEFINED). The bare
spelling `'5'::int4` already resolves to `DataType(this=INT)` because the tokenizer keyword-maps `int4`→`INT`.

**What sqlglot-go does:** for **Postgres only**, a two-part `pg_catalog.<name>` type name whose `<name>` is a
**real pg_catalog type name** resolves to the *exact node the bare `<name>` spelling produces* (byte-identical,
verified with `.Equal`): a builtin `DataType` (`pg_catalog.int4` → `INT`, `int8` → `BIGINT`, `int2` →
`SMALLINT`, `bool` → `BOOLEAN`, `numeric` → `DECIMAL`, `float8` → `DOUBLE`, `text`, `varchar`, `timestamptz`,
`name`, `money`, the six multiranges `int4multirange`/`int8multirange`/`datemultirange`/`nummultirange`/
`tsmultirange`/`tstzmultirange`, …), an **`ObjectIdentifier`** (`pg_catalog.oid`/`regclass`/`regproc`/
`regtype`/… — the `OBJECT_IDENTIFIER` family), or a **`PseudoType`** (`pg_catalog.cstring`). Both cast
spellings are covered (`CAST(x AS pg_catalog.int4)` and `x::pg_catalog.int4`), and array/collation suffixes
compose exactly as for the bare builtin.

Membership is decided by a **pinned allowlist of the real `pg_catalog` type names** (the base/pseudo/range/
multirange type set, captured from PostgreSQL 17.6 via `pg_type ⋈ pg_namespace WHERE nspname='pg_catalog' AND
typtype IN ('b','p','r','m')`), **not** by the type parser's generic keyword recognition. That distinction is
the whole point:
the tokenizer knows many spellings that are **not** `pg_catalog` type names — the SQL-standard grammar aliases
`integer`/`bigint`/`boolean`/`decimal`/`smallint`/`real`/`double` (the catalog names are
`int4`/`int8`/`bool`/`numeric`/`int2`/`float4`/`float8`) and other dialects' `tinyint`/`datetime`/`mediumint`/
`nvarchar` — and real PG **rejects** `pg_catalog.integer`, `pg_catalog.tinyint`, etc. Keying on the pinned
catalog set keeps all of those **USER-DEFINED**, so the resolution never claims a builtin PG itself refuses.

**Stays USER-DEFINED** (unchanged): a tail not in the pinned set (an alias like `pg_catalog.integer`/`real`/
`double`/`bigserial`, an extension type like `pg_catalog.hstore`/`geography`, a genuine user type
`pg_catalog.myt`); a real `pg_catalog` name the port does not model as a builtin
(`pg_catalog.macaddr`/`varbit`/`tsvector` — these resolve back to USER-DEFINED and **keep their `pg_catalog.`
qualifier**, i.e. are not silently rewritten to the bare unqualified name); any other schema (`public.myt`,
`myschema.int4`); and a three-part name (`a.pg_catalog.int4`).

**Two deliberate non-resolutions** (both stay USER-DEFINED with the qualifier preserved, so they round-trip
losslessly and match real PG):
- **`pg_catalog.char` and `pg_catalog.bit`** — real catalog names, but their bare SQL keyword is a *different*
  type, so resolving would silently change semantics. `pg_catalog.char` is the 1-byte `"char"` (OID 18) whereas
  the bare `CHAR` keyword is `character(1)`/`bpchar` (OID 1042) — `65::pg_catalog.char` = `'A'` but `65::char`
  = `'6'`; `pg_catalog.bit` has no implicit length whereas bare `BIT` is `bit(1)` — `'101'::pg_catalog.bit` =
  `'101'` but `'101'::bit` = `'1'`. Excluded via a small `pgCatalogSemanticMismatch` set. (`pg_catalog.bpchar`
  and `pg_catalog.varchar` **do** resolve — their bare spellings `BPCHAR`/`VARCHAR` are the same type.)
- **`pg_catalog.oid(5)` / `regclass(5)` / `cstring(5)`** and the other `ObjectIdentifier`/`PseudoType` names
  **with a trailing type modifier** — real PG rejects a modifier on these (`type modifier is not allowed for
  type "pg_catalog.oid"`), and the `ObjectIdentifier`/`PseudoType` node cannot carry `expressions`, so resolving
  would silently drop the `(5)`. A pending `(` therefore keeps the name USER-DEFINED (which *does* carry the
  modifier), preserving it on round-trip rather than laundering an engine-invalid form into a valid-looking one.
  (Modifier-bearing builtin `DataType`s like `pg_catalog.numeric(10,2)` are unaffected — they carry modifiers.)

Quoting is handled
under PostgreSQL's identifier folding on **both** parts: an unquoted name folds to lowercase — **ASCII-only**
(`stringsLower`, per §1.1), matching PG's ASCII identifier folding rather than Go's full-Unicode
`strings.ToLower`, so `pg_catalog.INT4`/`unKnown` resolve (real PG accepts them) but a non-ASCII spelling PG
rejects is never over-folded into a match — while a quoted name is literal — so
`pg_catalog."int4"` (already lowercase) **is** `int4` and resolves (real PG: `pg_typeof('5'::pg_catalog."int4")
= integer`), but `pg_catalog."INT4"` and `"PG_CATALOG".int4` stay USER-DEFINED (case-sensitive, not the
lowercase catalog name/schema — real PG rejects them). Base and MySQL are untouched: base still folds the
dotted name into a plain-string USER-DEFINED `kind`, and MySQL (`SUPPORTS_USER_DEFINED_TYPES=false`) errors on
the dotted type name, exactly as before — neither has a `pg_catalog` concept.

**Why we diverge (correctness):** `pg_catalog` is PostgreSQL's builtin/system schema, so `pg_catalog.<X>` names
the builtin `<X>`, not a user type — and for the resolved set it is the *same type* as the bare spelling the
port emits. Verified against PostgreSQL 17.6: `pg_typeof('5'::pg_catalog.int4) = pg_typeof('5'::int4) =
integer`, and likewise for `int8`/`int2`/`bool`/`numeric`/`float8`/`text`/`varchar`/`timestamptz`/`oid`/
`cstring`/`name`/`money`/the multiranges; the negatives all error there (`SELECT NULL::pg_catalog.integer` /
`.tinyint` / `.hstore` / `.real` / `public.myt` → `type … does not exist`), so keeping those USER-DEFINED is
correct. The equivalence is checked per-name against the bare node, **not** assumed from the `pg_catalog`
prefix — the two exceptions above (`char`/`bit`, where the bare *keyword* is a different type, and the
modifier-bearing `oid(5)`/…) show the prefix is not blindly "definitionally the bare spelling"; each resolved
name is one whose bare rendering PG treats as the identical type. Upstream is over-conservative — it leaves
the qualified builtin USER-DEFINED, forcing an AST consumer that classifies a cast target by `DataType.this`
to special-case the `pg_catalog.` prefix. Resolving it structurally lets the consumer read the same builtin
node regardless of the (semantically identical) spelling. The only §1.10 behavior change is for real catalog
builtins: `CAST('5' AS pg_catalog.int4)` now renders as `CAST('5' AS INT)` — the same type. No identity-corpus
case uses a `pg_catalog.`-qualified type name (scanned: none), so nothing in the corpus flips. Implemented in
`parser/parser_types.go` (the pinned `pgCatalogTypeNames` allowlist + `resolvePgCatalogBuiltin`, called from
the `parseUserDefinedType` postgres branch); regression test `parser/parser_types_test.go`
(`TestParsePgCatalogBuiltinType`). No upstream tripwire is needed: upstream already emits a node here (a
USER-DEFINED `DataType`, not a `Command`/parse error), so this is a §1 correctness divergence, not a
grammar-extension ledger row — if upstream later resolves it too, the tests still pass.

---

## Opt-in behavioral extensions beyond upstream

### Search-path-aware table qualification

Pinned upstream `qualify_tables` accepts one fixed `db`/`catalog` and stamps those parts without an
existence check (`.reference/sqlglot-v30.12.0/sqlglot/optimizer/qualify_tables.py:16-23,62-75`). The
Go-only `optimizer.QualifyOpts.SearchPath` adds a separate, opt-in resolution mode. The mode switch is
exact: a nil or empty `SearchPath` uses the existing upstream-faithful fixed `DefaultSchema`/`Catalog`
path unchanged, so all existing fixture output remains unchanged; only a non-empty `SearchPath` enables
the extension.

In non-empty mode, candidates are dialect-normalized and checked in order against the supplied schema.
A candidate database is stamped only when `schema.Find(..., false, false)` returns a mapping for that
candidate, and the first proven candidate wins. There is no fallback to the fixed `DefaultSchema`, and
no catalog is added. If no candidate is proven, the table remains unqualified so downstream policy can
fail closed. Absent, ambiguous, empty, flat, or otherwise schema-incapable schemas therefore produce no
stamp. Already-qualified tables and CTE references are preserved and are not rewritten.

The lookup requires a schema whose `SupportedTableArgs` includes `schema` (this is the port's renamed
qualifier arg — upstream calls it `db`; see §7). An empty schema or a flat, table-only mapping is
intentionally insufficient because it cannot prove that a table exists in a specific schema; probing it
as though it could would allow a mapping implementation to truncate the schema-qualified lookup and
return an unrelated unscoped table.

This boundary is security-relevant: guessing a database can bind access analysis to a table that the
actual database would not resolve, creating a wrong-ALLOW decision. Leaving the database absent when
its resolution cannot be proven is the safe result.

**Dimension resolved (schema only; catalog is the caller's).** `SearchPath` resolves the **schema**
part of a table name; it never stamps `catalog`. The probe is a two-part `Table{this, schema}`, and this
resolves correctly against a **three-level `catalog.schema.table`** schema too (verified: with a
`{cat:{schema:{table}}}` mapping, `SELECT * FROM t` under `SearchPath=[schema…]` stamps `schema=<schema>`
and leaves `catalog` unstamped) — `schema.Find` matches a schema-qualified probe by schema-level
existence, not requiring the catalog level, so a depth-3 consumer is **not** silently over-denied. The
stamp is a schema-level existence superset: a multi-catalog consumer supplies its own `catalog` (via
`opts.Catalog` or downstream) and performs the full `catalog.schema.table` resolution, which fail-closes
if the table does not exist in *its* catalog. So the division is: **R1 fixes the schema dimension by
proven schema-existence; the caller fixes and enforces the catalog.**

**Identifier folding (role-aware).** The `SearchPath`, `DefaultSchema`, and `Catalog` names are folded with the
dialect's normalization strategy in a **relation-role context** — each is parsed and given a Table parent
under its arg key before `NormalizeIdentifier` runs (`normalizeRelationIdentifier`) — so the role-aware
MySQL `lower_case_table_names=0` strategy **preserves** a schema name's case (a detached identifier has
no parent and would be misread as a foldable column, lowercasing `App` to `app`) and the
INFORMATION_SCHEMA exception (§1.2) applies. A caller may therefore pass the search path in its **raw**
case: under lctn=0, `SearchPath=["App"]` stamps `schema=App` (case-sensitive) and `["app"]` fails to resolve
`App`; under lctn=1/2 both fold to `app`. Non-role-aware dialects (base/Postgres, default MySQL) ignore
the parent, so their result is unchanged from a detached normalization.

This is not a parse-grammar construct, so it is neither registered in
[`testdata/upstream_extensions.jsonl`](./testdata/upstream_extensions.jsonl) nor governed by the
grammar-extension tripwire.

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

Ledger id [`mysql-describe-column`](./testdata/upstream_extensions.jsonl) registers MySQL's
`{DESCRIBE|DESC|EXPLAIN} tbl_name [col_name | wild]` column/wildcard-filtered table describe, which pinned
upstream rejects with a parse error. After a *plain* `DESCRIBE tbl` target is parsed, the MySQL parser
consumes a single trailing `col_name` — a backtick-quoted identifier, or an unquoted **non-reserved** name
(an unquoted reserved word like `NULL`/`ORDER`, which MySQL rejects here, is not consumed) — or a `wild`
string, into a Go-only optional `column` arg, so `DESCRIBE users id` / `DESCRIBE users 'i%'` build
`Describe{this: Table, column: ...}` instead of degrading to `Command`. The target stays `this` (a `Table`),
so a consumer keying on `this.Kind()` classifies these as table-describes rather than fail-closed — closing a
false-reject the `this.Kind()` discriminator would otherwise introduce for these common interactive-metadata
statements.

The col slot is deliberately a single identifier, **not** the general column-expression grammar: a
parenthesized subquery, function call, cast, qualified/multi-part column, bracket, or literal is rejected
(fail closed to `Command`), so a full `SELECT` (with its own table reads) can never be smuggled into the
`column` arg behind `this = Table`. The grab is further gated so it fires only for a *bare* describe with no
`style`/`format`/`kind` modifier and consumes no trailing clause: `EXPLAIN ANALYZE TABLE t` /
`EXPLAIN FORMAT=JSON TABLE t` (which are explains of the MySQL `TABLE t` query — row-reading scans, not
metadata), and any `PARTITION(...)` / `AS JSON` after a col, all stay `Command`. A statement target
(`DESCRIBE SELECT ...`) never has a trailing token grabbed, and other dialects are untouched. The generator
renders the column right after the table (`DESCRIBE users id`); the `DESC`/`EXPLAIN` leaders normalize to
`DESCRIBE` as they already do for the bare form. Verified against MySQL 8.4. Use the stable ledger id for its
reconciliation lifecycle.

(Separately, and NOT changed here: the plain `EXPLAIN TABLE t` form — MySQL's `TABLE t` statement, a
query-explain — already parses to `Describe{kind: "TABLE", this: Table(t)}` in both this port and pinned
upstream. A consumer distinguishing query-explain from table-describe by `this.Kind()` must also treat
`kind == "TABLE"` as a query-explain; this extension does not widen that pre-existing shape.)

Ledger ids [`pg-set-role`, `pg-set-session-authorization`, `pg-set-time-zone`, `pg-set-names`,
`pg-set-constraints`, `pg-set-session-characteristics`](./testdata/upstream_extensions.jsonl) register the
Postgres `SET` special-forms, each of which pinned upstream degrades to a raw `Command`. The ordinary
assignment forms (`SET x = v` / `SET x TO v`, with optional `SESSION`/`LOCAL`/`GLOBAL` scope) already parse
to `Set`; these six keyword forms did not. Structuring them into `Set{SetItem{kind: ...}}` lets a consumer
read `SetItem.kind` to tell a **privileged** SET (`ROLE`, `SESSION AUTHORIZATION` — which change the
effective role/user) from a **benign** one (`TIME ZONE`, `NAMES`, `CONSTRAINTS`, `SESSION CHARACTERISTICS`).
Values are modeled fully — `TIME ZONE` accepts a string, a signed number, a bare zone name,
`LOCAL`/`DEFAULT`, or an `INTERVAL '…' … TO …`; `CONSTRAINTS` holds either the unquoted `ALL` keyword (a
*quoted* `"ALL"` stays a specific constraint name so round-trip can't broaden it to every constraint) or a
comma-separated list of (optionally schema-qualified) constraint names in `expressions`, and the
`DEFERRED`/`IMMEDIATE` mode (validated against exactly those two words) in `this`; `SESSION CHARACTERISTICS`
requires `AS TRANSACTION` and reuses the shared transaction-mode options (a characteristic outside that
set — e.g. `DEFERRABLE`, or `READ UNCOMMITTED`, which the shared upstream-ported table blocks via a
typo in its `READ UNCOMMITTED` entry — fails closed to `Command` rather than raising); `NAMES` takes a string literal,
`DEFAULT`, or nothing (and, unlike MySQL's, no `COLLATE` — an unquoted charset is invalid Postgres and fails
closed). The parsers live in `parser/dialect_postgres_set.go` (dispatched via a
Postgres-specific `SET_PARSERS` table; `SESSION AUTHORIZATION`/`SESSION CHARACTERISTICS` are disambiguated
inside the `SESSION` assignment parser because the dispatch trie matches `SESSION` first), with two
generator branches in `generator/stmt_set.go` for the `CONSTRAINTS`/`SESSION CHARACTERISTICS` shapes.

**The `kind` is not sufficient on its own for a privilege check.** Postgres also exposes `role` and
`session_authorization` as *ordinary GUCs*, so `SET role = x` (bare or `SESSION`/`LOCAL`-scoped) and
`SET session_authorization = x` perform the same privilege change as the keyword forms but parse as ordinary
assignments (`SetItem.kind` `""`/`"SESSION"`/`"LOCAL"`, `this = EQ(<var>, <value>)`). A consumer must
therefore ALSO deny an assignment whose LHS variable name is `role`/`session_authorization`
(case-insensitive) — not only `kind ∈ {ROLE, SESSION AUTHORIZATION}`. The disambiguator is a following
assignment delimiter (`=`/`:=`/`TO`): only the no-delimiter spelling is the privileged keyword form.
**SECURITY — the bare `SET role = x`** is load-bearing and newly modeled here: because `ROLE` is a SET dispatch
key, without the delimiter check it fell closed to a raw `Command`, and a consumer whose Command-SET fallback is
a benign session passthrough (allow) would then let a bare `SET role = admin` through as a privilege
escalation. `parseSetItemRole` now retreats on a following delimiter and builds the readable `EQ` (the scoped
and `session_authorization` spellings already did). The alias *surface* is still not closable by
keyword-spelling detection alone — every special form has a plain-assignment alias — which is exactly why a
consumer must gate on the `EQ` LHS too.

Fail-closed to `Command`: a form missing or malforming its required value (`parseSet` also rejects a
zero-item `Set`); and a comma-combined multi-item Postgres SET (real Postgres SET is single top-level item, so
`len(items) > 1` on Postgres degrades — this is the only way a special form gets mixed into a comma list,
which the server rejects). The extension is Postgres-only; base/MySQL leave these forms as `Command` (MySQL's
own multi-item `SET a=1, b=2` is unaffected). Verified against PostgreSQL 17.6. Use the stable ledger ids for
the reconciliation lifecycle.

Ledger id [`pg-set-scoped-role`](./testdata/upstream_extensions.jsonl) extends the above to the
**`SESSION`/`LOCAL`-scoped** privileged forms — `SET [SESSION|LOCAL] ROLE r` and
`SET [SESSION|LOCAL] SESSION AUTHORIZATION u` — which pinned upstream also Commands. They parse to the SAME
`SetItem.kind` as the bare form (`ROLE` / `SESSION AUTHORIZATION`, so a consumer's privilege check is
unchanged), with the scope word preserved in a new `SetItem.scope` arg for a faithful round-trip. Critically,
the same GUC-alias caveat is honored structurally: `SET SESSION role = x` (an assignment on the `role` GUC)
is disambiguated from `SET SESSION ROLE x` (the privileged form) by the following assignment delimiter
(`=`/`:=`/`TO`) — only the no-delimiter spelling is the privileged form, so the GUC-alias assignment stays an
`EQ` whose LHS a consumer can read. Parser in `parser/stmt_set.go` (`parseSetItemAssignment`).

Ledger id [`mysql-set-password`](./testdata/upstream_extensions.jsonl) registers MySQL
`SET PASSWORD [FOR user] = value` — an account mutation pinned upstream Commands (the `FOR` form) — into
`Set{SetItem{kind:"PASSWORD", this:<user|nil>, expressions:[<value>]}}`, so a consumer reads
`kind = "PASSWORD"` instead of scanning the raw tail. The optional `FOR <user>` is a MySQL user spec
`name[@host]` captured verbatim (quoting preserved) so it round-trips byte-for-byte; the bare
`SET PASSWORD = x` (previously a plain assignment) now folds into the same `kind=PASSWORD` shape. Parser in
`parser/dialect_mysql_set_show.go` (`parseSetItemPassword` + `parseMySQLUserSpec`), generator branch in
`generator/stmt_set.go`.

Ledger id [`mysql-show-create-user`](./testdata/upstream_extensions.jsonl) registers MySQL
`SHOW CREATE USER <user>` — pinned upstream omits it from its `SHOW_PARSERS` and Commands it — modeled like
the sibling `SHOW CREATE {TABLE,VIEW,…}` forms as `Show{this:"CREATE USER"}`, via a dedicated strict
`parseShowCreateUser`: a user target is **required** and no trailing clause is allowed (MySQL rejects
`SHOW CREATE USER 'u' LIKE …`/`FROM …`/`LIMIT …` and a bare `SHOW CREATE USER`), so those fail closed to a raw
`Command`. All three are valid syntax on MySQL 8.0.33 / PostgreSQL 17.6.

The `name[@host]` user target (used by both `SET PASSWORD FOR …` and `SHOW CREATE USER`) is parsed by
`parseMySQLUserSpec`, which enforces MySQL's `user` grammar rather than accepting anything id-var-shaped: the
name must be an identifier/string (not a number or reserved keyword like `ALL`), empty parens `()` are valid
only on `CURRENT_USER` (`foo()` is not a user), and `CURRENT_USER` takes no `@host`. The `SET PASSWORD` auth
value must be a plain string literal (`= '…'`) or `TO RANDOM` — a placeholder (`= ?`), number, or bare
identifier fails closed. Every engine-invalid form thus degrades to `Command` instead of a structured
privileged node (verified against MySQL 8.0.33).

**Quoted dispatch keywords fail closed (all SET/SHOW dispatch).** A quoted keyword is never an unquoted
dispatch keyword — real engines reject `SET "ROLE" x` / `SET "LOCAL" ROLE x` / `SHOW `` `CREATE` `` USER` as
syntax errors, and read `` `PASSWORD` `` = x` as an ordinary (non-existent) variable assignment — so matching a
quoted token by text would launder engine-invalid SQL into a structured, valid-looking **privileged** statement
(e.g. regenerating `SET "ROLE" x` as the executable `SET ROLE x`). This whole *quote-blind hand-rolled matcher*
class is closed:
- the shared `findParser`/trie stops on a quoted `IDENTIFIER`/`STRING` token (the *dispatch* keyword — the
  first word), affecting every dispatched SET/SHOW keyword, not only the privileged ones;
- every keyword the privileged SET paths match with `matchTextSeq` — the second word of a phrase and the
  surrounding keywords (`SESSION AUTHORIZATION`, `SESSION CHARACTERISTICS AS TRANSACTION`, `SET [GLOBAL|SESSION]
  TRANSACTION`, MySQL `PASSWORD`'s `FOR`/`TO RANDOM`) — uses `matchUnquotedTextSeq`, so `SET SESSION
  "AUTHORIZATION" x` / `SET PASSWORD TO `` `RANDOM` ``` fail closed;
- MySQL `SET PASSWORD`'s `=`/`:=` delimiter check excludes a quoted `IDENTIFIER` (`` SET PASSWORD `=` 'x' ``
  no longer builds the mutation);
- and a **quoted config-parameter name** that would change meaning when regenerated unquoted is rejected by
  `isPlainConfigIdent`: `RESET "all"` (an unknown parameter) must not become `RESET all` (= reset everything),
  and `RESET "SESSION AUTHORIZATION"` (a quoted name with a space) must not become the privileged phrase — both
  fail closed to `Command` (the parser keeps the quotes verbatim in that raw form).

This is a §1-style correctness alignment (matches the DB); pinned upstream's `_find_parser`/`_match_texts`
match quoted tokens by text.

Ledger ids [`pg-show-guc`, `pg-reset`](./testdata/upstream_extensions.jsonl) register Postgres
`SHOW { name | ALL | special }` and `RESET { name | ALL }` — session run-time-parameter introspection and
reset that pinned upstream Commands (`postgres.py` maps `RESET`→`COMMAND` and leaves `SHOW` in the
tokenizer's Commands set, so both pack their tail into one raw `STRING`). This port parses them from real
tokens into `Show{this:<name>}` and `Reset{this:<name>}` (a new `KindReset` node), so a consumer classifies
the GUC introspection/reset structurally. Two contained tokenizer changes make the real tokens available:
`SHOW` is removed from the Postgres Commands set (exactly as `dialects/mysql.go` already does), and `RESET`
is left as a plain `VAR` instead of `COMMAND` (so it is dispatched by leading text like `SAVEPOINT`, and stays
usable as an ordinary identifier — `SELECT reset FROM t` is unaffected). A shared `parsePostgresConfigParam`
accepts exactly what PostgreSQL 17.6's grammar accepts: a one-or-two-part name (`search_path`, `ext.var`,
quoted `"search_path"`), `ALL`, or one of the special multi-word phrases `TIME ZONE` / `TRANSACTION ISOLATION
LEVEL` / `SESSION AUTHORIZATION` (each required to be **unquoted** keywords — Postgres treats `"time"` as a
parameter name, not the `TIME` keyword). A trailing token (`SHOW search_path extra`), a bare `SHOW`/`RESET`, a
non-name token (`SHOW 1`, `SHOW (x)`), or a **reserved word** Postgres rejects as a `var_name`
(`SHOW NULL`/`RESET CURRENT_USER`/`SHOW DEFAULT`/…) all **fail closed** to a raw `Command` — matching what the
engine rejects.

**`this` is a CANONICAL identity, not the raw spelling** — this is a security property. PostgreSQL's GUC lookup
is case- and quote-insensitive (`RESET ROLE` == `reset role` == `RESET "RoLe"`), and its special phrases are
exact aliases of underscore GUCs (`RESET SESSION AUTHORIZATION` == `RESET session_authorization`, `TIME ZONE`
== `timezone`, `TRANSACTION ISOLATION LEVEL` == `transaction_isolation` — all verified on PG 17.6). So the
parser folds every spelling to **one** identity: a generic name to its ASCII-lowercased, unquoted form
(§1.1), and each special phrase to its underscore-GUC name. `RESET ROLE`/`role`/`"RoLe"` all yield
`this="role"`; every spelling of the session-authorization reset yields `this="session_authorization"`. A
consumer gating a privilege-relevant reset (`RESET ROLE`/`RESET SESSION AUTHORIZATION` restore the login
identity — a potential escalation) can therefore match a single canonical `this` and cannot be defeated by a
case/quote/phrase variant. This mirrors how SET already canonicalizes `SetItem.kind`. Round-trip regenerates
the canonical spelling (`SHOW TIME ZONE` → `SHOW timezone`; `RESET "RoLe"` → `RESET role`) — semantically
identical valid Postgres, the same normalization philosophy as SET's `TO`→`=`. `SHOW` additionally treats a
quoted `"all"` as the `ALL` form (real PG: `SHOW "all"` lists every setting), whereas `RESET "all"` is an
ordinary parameter named `all` — the one place SHOW and RESET diverge, handled by a `showAll` flag.

**MySQL `RESET` is deliberately left as `Command`** (§1.6) — `RESET MASTER`/`RESET REPLICA` is a privileged
replication-admin op, semantically unrelated to the Postgres GUC reset, so a consumer must keep treating it as
opaque. Parser in `parser/dialect_postgres_show_reset.go` + the `parseShow` postgres branch in
`parser/stmt_show.go`; generators in `generator/stmt_show.go` (postgres `showSQL` branch) and
`generator/stmt_transaction.go` (`resetSQL`).

**Known limitation — the name grammars approximate the engines' keyword classification (all fail-safe).**
Exactly matching PostgreSQL's `var_name`/`ColId` and MySQL's `user`/reserved-word tables would require porting
those keyword-category tables; this port instead approximates them with its tokenizer's token classes, which
over- or under-accepts a handful of keyword-shaped names. All cases fail **safe** — a consumer gates on the
statement's structural identity (`Show.this`/`Reset.this` canonical GUC, `SetItem.kind`), never on whether an
odd name was accepted:
- *Under-acceptance* (engine-valid → `Command`, a legibility gap): a three-part GUC name (`SHOW a.b.c` /
  `RESET a.b.c`); a config name whose component tokenizes as a keyword rather than `VAR` (`SHOW schema` —
  `SCHEMA` is a valid PG `ColId`); a MySQL dotted-IP host (`u@1.2.3.4`); and MySQL
  `SET PASSWORD … REPLACE …/RETAIN CURRENT PASSWORD` clauses.
- *Over-acceptance* (engine-invalid → structured, but never a privileged **bypass** — it structures a form the
  engine rejects, which a consumer still denies): a PG reserved word the tokenizer leaves as `VAR`
  (`SHOW CURRENT_ROLE`/`SHOW USER` — PG rejects these, but there is no real `current_role`/`user` GUC, so
  nothing privileged hides behind it), and a MySQL reserved-word account name (`SET PASSWORD FOR ACCESSIBLE =
  …`). The clearly-reserved *value* keywords (`NULL`/`TRUE`/`DEFAULT`/`CURRENT_USER`/`SESSION_USER`) and a
  numeric name/host *are* rejected.

These are fidelity gaps, not safety gaps: none lets an engine-executable privileged statement parse as benign
or regenerate into a different privileged statement (that laundering class is closed — see the quoted-keyword
note above and the `isPlainConfigIdent` guard, which reject a quoted parameter name whose unquoted
regeneration would change meaning).

Ledger ids [`mysql-into-outfile`, `mysql-into-dumpfile`](./testdata/upstream_extensions.jsonl) register
MySQL's `SELECT ... INTO {OUTFILE|DUMPFILE} '/path'` server-side file writes, which pinned upstream rejects
with a parse error. These parse to the existing `Into` node (hung off the `Select`'s `into` arg) with a new
`kind` marker (`"OUTFILE"`/`"DUMPFILE"`; **unset** for a normal `INTO tbl/@var`) and `this` = the path
`Literal`, so a consumer detects the file-write — and its target path — structurally via `Into.kind` rather
than scanning raw SQL. `OUTFILE` additionally models the export grammar into dedicated args — `CHARACTER SET`
(`charset`), the `{FIELDS|COLUMNS}` clause (`fields_terminated`, `optionally_enclosed`, `enclosed`, `escaped`,
plus a `columns` marker distinguishing the `COLUMNS` synonym), and the `LINES` clause (`lines_starting`,
`lines_terminated`); `DUMPFILE` takes no options (trailing option tokens fail closed, matching MySQL). The
path is required to be a plain string `Literal` (MySQL rejects a placeholder/parameter/number here); the
`CHARACTER SET` operand is a bare charset name (`utf8mb4`) or a quoted string (`'utf8'`) — a number, `NULL`,
a parenthesized/function expression, or a placeholder is rejected; and each `FIELDS`/`LINES` option value is
a single plain-string or hex/bit literal (`X'2c'`/`0x2c`, `b'101'`/`0b101`) — the national-string form
(`N'…'`) and adjacent-string concatenation (`'a' 'b'`, which MySQL rejects in this slot) fail closed rather
than folding into a `CONCAT`. Within a `FIELDS`/`LINES` block the sub-options may appear in **any order**
(repetition: last wins), matching MySQL; the generator re-emits a canonical order, so a reordered/repeated
input normalizes rather than round-tripping byte-for-byte (semantically identical, valid MySQL). Every
matched option introducer requires its operand and a bare `FIELDS`/`COLUMNS`/`LINES` is rejected — all fail
closed.

MySQL accepts `INTO` both before `FROM` and at the trailing position; the trailing form is canonical and is
where this port re-emits it (the before-`FROM` spelling parses to the same shape and normalizes to trailing
on output). The trailing grab in `parseQueryModifiers` fires only for an **unquoted** `OUTFILE`/`DUMPFILE`
keyword on a MySQL `Select` with no existing `into`; a plain trailing `INTO tbl/@var`, and a backtick-quoted
`` `outfile` `` (an ordinary INTO target name that MySQL accepts), are left as upstream. Because
`INTO OUTFILE` is a file write, not a table materialization, the generator renders it inline via `intoSQL`
and never as the `CREATE TABLE … AS` rewrite a plain `SELECT INTO` takes on non-`SUPPORTS_SELECT_INTO`
dialects.

**Fail-closed, detection-preserving.** Having matched `OUTFILE`/`DUMPFILE` the parser is committed to the
file-write grammar, so a malformed tail (missing path, dangling option, modifier after a trailing `INTO`)
raises a parse error — a hard failure at the default `IMMEDIATE` level that `sqlglot.ParseOne`/`Parse` use.
The core guarantee is that a **well-formed** file-write is never silently lost: the `Into` node is built with
its `kind` before any tail is validated, so a malformed *plain SELECT/UNION* file-write still exposes a
kind-tagged `Into` even at the lenient `WARN`/`IGNORE` levels (reachable via the public
`parser.NewWithErrorLevel`) — it can never degrade to a plain `SELECT` with the `INTO` dropped. (The one
exception is a malformed file-write nested inside a speculatively-parsed `CREATE`/`REPLACE`, whose
`tryParse` forces `IMMEDIATE` and, on the raised error, discards the partial tree back to a raw `Command`
with no `Into`; such input is invalid MySQL and cannot execute, so this is a limit of the lenient-level
guarantee, not an executable bypass.) **Trailing position:** a trailing `INTO` must be the last clause except
for a following locking clause (`FOR UPDATE`/`LOCK IN SHARE MODE`, which MySQL accepts on either side); an
`ORDER BY`/`LIMIT`/`WHERE`/`GROUP BY` after it is rejected (MySQL 1064) rather than silently reordered ahead
of the `INTO`. **Set operations:** a file-write `INTO` on the last branch of a `UNION`/`EXCEPT`/`INTERSECT` is
hoisted to the set-operation node (like `ORDER BY`/`LIMIT`) and rendered at the trailing position, so
`… UNION … ORDER BY … INTO OUTFILE '/x'` round-trips as valid MySQL and stays detectable; the
parenthesized-last-branch spelling (`… UNION (SELECT …) INTO OUTFILE`) fails closed. **Placement:** a
per-top-level-statement check (covering `SELECT`/`UNION`, a leading `EXPLAIN`/`DESCRIBE`, and non-SELECT
roots like `UPDATE`/`DELETE`/`INSERT`) requires the file-write `Into` to sit at the top-level query root's own
`into` arg (directly, or after the final-branch hoist, or the query explained by `EXPLAIN`). A file-write
`INTO` inside a subquery/derived table, on a non-final UNION branch, or nested in an `UPDATE`/`DELETE`/`INSERT`
— all of which MySQL rejects with error 3954/1064 — fails closed. Detection is never lost in any of these
cases; the checks keep the accepted grammar aligned with MySQL. **Executable comments:** a file-write hidden
in a MySQL executable comment (`SELECT 1 /*! INTO OUTFILE '/x' */`) is invisible by default because the
default strips `/*! … */` for all content (§1.5, matching upstream); it becomes a detectable `Into` node when
`mysql_version` is set — the documented, opt-in way to see any executable-comment SQL — so this extension
composes with §1.5 rather than adding its own comment handling.

The extension is MySQL-only; other dialects do not treat `OUTFILE` specially. Verified syntactically against
MySQL 8.0.33 — the production target — via a differential accept/reject sweep (each accepted form reaches
runtime `secure-file-priv`, i.e. parses cleanly; each rejected form matches MySQL's `1064`/`3954`). Use the
stable ledger ids for the reconciliation lifecycle.

Ledger id [`pg-user-type-typed-literal`](./testdata/upstream_extensions.jsonl) registers the Postgres
user-type **space typed-literal** — `<type-name> 'string'` with no `AS`, e.g. `public.evil_domain 'x'` or
`"t" 'x'` — which pinned upstream parse-errors. In Postgres a bare string is **never** a valid column alias
(`SELECT 1 'x'` and even `SELECT 1 AS 'x'` are syntax errors), so `<name> 'string'` is unambiguously a typed
literal: the name is a type and the string is its value. This port folds it into
`Cast(Literal, DataType(user-defined))` — the **same node** `'x'::a.b` / `CAST('x' AS a.b)` already produce
(the type name derived from the parsed name via `ColumnsToDot`, preserving each part's quoting so the Cast
round-trips exactly) — so a consumer detects the user-type coercion (a `DOMAIN`/type `CHECK` runs a user
function on the shared session — a code-exec/error-oracle vector) structurally via `FindAll(KindDataType)`,
the same walk it already uses for the `::`/CAST spellings. The space form normalizes to the canonical `CAST`
spelling on output.

**Recognized at the primary-expression level** (`parseAtom`, and `parseType`'s keyword-name fallback), NOT in
alias parsing — so it folds into a `Cast` in *every* position a value can appear: a SELECT projection, a
function argument, a `WHERE`/`HAVING` predicate, an `UPDATE … SET` value, `VALUES`, a binary-operator operand,
and before a postfix `::` (`public.foo 'x'::text` → nested `Cast`). This is essential for the security use
case — the coercion is detectable as a `DataType` wherever it hides, not only in a projection. Any postfix ops
after the literal are re-applied to the `Cast`, and a trailing alias (`<type> 'x' AS bar` / `<type> 'x' bar`)
is then handled by ordinary alias parsing.

This also corrects a pre-existing port divergence: sqlglot-go's `parseAlias` previously called
`parseStringAsIdentifier` **unconditionally**, whereas upstream gates the string-as-alias behind
`STRING_ALIASES` — `False` for base/postgres (`parser.py:1780`), overridden to `True` only by
mysql/tsql/sqlite (`parsers/mysql.py:302` et al.). So the port previously accepted `public.evil_domain 'x'`
as a string **alias** and round-tripped it to a *different* statement (`… AS "x"`). `STRING_ALIASES` is now
ported as a proper per-dialect flag (`Dialect.StringAliases`, default `False`, set `True` for MySQL), and
`parseAlias`'s string-as-alias call is gated by it — so the **implicit** (no-`AS`) string-alias fails closed
for **both base and postgres**: a trailing string that is not a typed literal (`SELECT 1 'x'`, or a second
string as in `public.foo 'a' 'b'`) is an unexpected token, matching upstream base + postgres (both raise)
and the real engines (PostgreSQL rejects it; MySQL — `STRING_ALIASES = True` — accepts it, folding the
string into a backtick-quoted identifier alias). (The **explicit** `AS '…'` form — `SELECT 1 AS 'x'` — is a
separate, pre-existing quirk: it still parses as a quoted-identifier `Alias` via `parseIdVar`'s any-token
path, matching pinned upstream, even though real PG rejects it; that path is unchanged here and out of
scope.) This flag gates only the **projection/expression** alias (`parseAlias`); the string-as-identifier for
a table **name** (`_parse_table_part`, `parser.py:4668`) and a table **alias** (`_parse_table_alias`,
`parser.py:4111`) are unconditional upstream — not gated by `STRING_ALIASES` — and are handled by a separate
per-dialect flag, `Dialect.StringTableIdentifiers` (base keeps upstream's accept; postgres/mysql reject, since
their real engines do). See **§1.9** for that gate; the closing of the base/postgres *projection* string-alias
divergence here needs no new §1 entry (gated base/postgres now match upstream), but the *table* forms do
diverge from upstream to match the engines and are tracked as §1.9.

The type name is validated structurally as a real identifier chain — a `Dot` built by postfix
`.field`/`.*`/`[…]` access over an arbitrary base (`(t2.a).city`, `foo().bar`, `arr[1].foo`, `t.*`) is
rejected (Postgres rejects each as a syntax error). A **bare reserved value-function** (`current_user`,
`session_user`, `current_catalog`, …) is excluded by a check on the *leading token type* — not by blanket
keyword membership, which would false-reject the ~57 non-reserved keywords (`type`, `format`, `schema`,
`view`, `current_schema`, …) that Postgres accepts unquoted as type names. (`user` / `current_role` are also
PG-reserved but this port lexes them as `VAR`, so they fold as *accept-invalid* — harmless: Postgres rejects
them and a consumer denies the resulting `Cast` either way; a pre-existing tokenizer gap, not this feature.)
The value accepts every Postgres string-constant form — plain, escape (`E'…'` → `ByteString`), and
dollar-quoted (`$$…$$`/`$tag$…$tag$` → `RawString`) — but not national (`N'…'`) or bit/hex (`B'…'`/`X'…'`),
which Postgres rejects in this production; a comment between the name and the string is carried onto the
`Cast` (so `Cast.this` is the string-constant node — a `Literal` for a plain string, `ByteString`/`RawString`
for the escape/dollar forms). Only a `::`/`.:` cast may directly follow the literal (`public.foo 'x'::text` →
nested `Cast`); a postfix `.field`/`[…]`/`.*` (which Postgres requires parentheses for) fails closed. Verified
against PostgreSQL 17.6 with a differential accept/reject sweep across all positions (each accepted form
reaches type resolution — `type "…" does not exist` — i.e. parses as a typed literal; each rejected form is a
PG `syntax error`).

**Known limitations** (all *fail closed under the default `IMMEDIATE` error level* that `sqlglot.ParseOne`
uses — a parse error, so a consumer denies them — or produce a still-detectable `Cast`; none is a
detection bypass, they are grammar-completeness gaps left for a follow-up): a type name carrying a **type
modifier** (`public.foo(3) 'x'`) parses as a function call, not a type-name chain, so it is not folded (parse
error under `IMMEDIATE`); a **keyword-named user type** that this port lexes as a non-`VAR` statement keyword
(e.g. a type actually named `alter`) likewise doesn't reach the fold; **newline-separated string
continuation** (`public.foo 'a'\n'b'`, which Postgres concatenates) is parsed as a single string then fails
on the second; and the two PG-reserved value-keywords this port lexes as `VAR` (`user`, `current_role`) fold
as *accept-invalid* (Postgres rejects them; the resulting `Cast` is denied anyway). Use the stable ledger id
for the reconciliation lifecycle.

Ledger ids [`pg-start-transaction`, `mysql-start-transaction-snapshot`](./testdata/upstream_extensions.jsonl)
register `START TRANSACTION` transaction-control statements that pinned upstream parse-errors. Upstream maps
`START` → `BEGIN` for mysql/presto/oracle but **not** postgres, so `parse_one("START TRANSACTION", "postgres")`
mis-parses `START` as an expression and errors on the trailing `TRANSACTION`/modes; upstream also errors on
Postgres `START TRANSACTION READ ONLY` (postgres lexes `ONLY` as a dedicated token, not `VAR`) and on MySQL
`START TRANSACTION WITH CONSISTENT SNAPSHOT`. This port maps the postgres `START` keyword to the `BEGIN` token
(as the other dialects already do upstream) so `START TRANSACTION [<modes>]` routes through the shared
`parseTransaction` and builds `exp.Transaction{this, modes}`; the mode loop also consumes the `ONLY` token
(postgres `READ ONLY`) and, **for MySQL only**, the `WITH CONSISTENT SNAPSHOT` phrase (a single mode string).
`START TRANSACTION` normalizes to `BEGIN` on output, carrying the comma-separated modes. `COMMIT`/`ROLLBACK`/
`BEGIN` and `start` used as an ordinary identifier/table/CTE/alias are unaffected (`tokens.BEGIN` is already
in the id/alias token sets). The parser is permissive about mode content (matching upstream's mode loop), so a
mode a given engine rejects at runtime — MySQL's standalone `ISOLATION LEVEL`, or a `START TRANSACTION READ
ONLY` under a dialect that lacks it — may still parse to `Transaction`; only `WITH CONSISTENT SNAPSHOT` is
dialect-gated (real MySQL 8.0.33 accepts it, real PostgreSQL 17.6 rejects it). This is a transaction boundary
an AST consumer needs to route to session passthrough. Verified against PostgreSQL 17.6 and MySQL 8.0.33;
implemented in `dialects/postgres.go` and `parser/stmt_transaction.go`; regression test
`pg_start_transaction_test.go`. Use the stable ledger ids for the reconciliation lifecycle.

**Known limitation** (a grammar-completeness gap, not a detection bypass): a user-defined Postgres function
literally named `start` — `SELECT start()` — no longer parses as a function call (the `START` keyword now
tokenizes as `BEGIN`), matching how pinned upstream already treats `start()` under mysql/presto/oracle. Bare
`start` as a column/table/CTE/alias is unaffected; only the rare `start(...)` call form is lost. This is the
same trade-off upstream itself makes for its `START`→`BEGIN` dialects.

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

## 7. Table/column qualifier arg renamed `db` → `schema` (API + `.ToS()`; round-trip identical)

**What upstream does:** upstream names the middle table/column qualifier `db` — the `Table`/`Column`
`db` arg, the `.db` property, `TABLE_PARTS = ("this", "db", "catalog")` /
`COLUMN_PARTS = ("this", "table", "db", "catalog")`, and `repr()` renders it `db=…`. The name is a
**misnomer**: this level is the ANSI **schema** (the middle qualifier), *not* a database. Upstream's own
`table_` builder docstring says so — a table path is `[catalog].[schema].[table]`, and it assigns
`catalog, db, this = split_num_words(sql_path, ".", 3)`, i.e. the `db` slot **is** the schema
(`.reference/sqlglot-v30.12.0/sqlglot/expressions/builders.py:360`). The confusion is cross-engine:
Postgres's *database* is the ANSI **catalog** (→ sqlglot's `catalog` arg), and only MySQL conflates
`database == schema`. This role ambiguity is what forced the role-aware search-path/`db`/`catalog`
folding in `qualify_tables` (commit `49965a3`; see *Search-path-aware table qualification* above).

**What sqlglot-go does:** the port renames this one qualifier **everywhere** to `schema`, so the ANSI
level is explicit in the code:
- arg-key string `"db"` → `"schema"` on `KindTable` / `KindColumn` (`expressions/kinds.go`);
- accessor `Node.DbName()` → `Node.SchemaName()` (+ the `Expression` interface) (`expressions/core.go`);
- `TablePartKeys = []string{"this", "schema", "catalog"}` and the `Parts()` / generator / `qualify`
  part-order loops (`expressions/core.go`, `generator/sql.go`, `optimizer/qualify_tables.go`);
- builder params: `Table_(table, schema, catalog, …)`, `Column_(col, table, schema, catalog, …)`,
  and `QualifyTables(expression, schemaName, catalog, …)` (`schemaName`, not `schema`, because the
  `schema` package is imported there; likewise `parseTableParts` uses the local `schemaPart` because
  its `schema bool` DDL-position param already owns the name `schema`);
- the public qualify option field `QualifyOpts.DB` → `QualifyOpts.DefaultSchema` (`optimizer/qualify.go`),
  the field that becomes the stamped `schema` arg. It is `DefaultSchema`, not `Schema`, because
  `QualifyOpts.Schema` is the column-metadata mapping (upstream's `schema=` kwarg) — upstream likewise
  carries both a `schema=` mapping and a `db=` default qualifier, so the two must stay distinct here.
The ANSI **catalog** keeps its `catalog` name unchanged.

**Scope boundary — the rename covers the qualifier arg and its direct accessors/setters, not
upstream-ported symbol names that merely reference the concept.** Dialect/generator flag names ported
1:1 from upstream keep upstream's `DB` spelling to preserve grep-correspondence with `.reference/` — in
particular `Dialect.RenameTableWithDB` (← upstream's `RENAME_TABLE_WITH_DB`, `generator.py:344`), which
governs whether `ALTER TABLE … RENAME` retains the qualifier. It reads/acts on the (renamed) `schema`
arg but keeps its upstream name.

**Explicitly NOT renamed — genuine "database" sites** (here `db`/`database` really means a database):
`KindShow`'s `db` arg (`SHOW … FROM <db>`), `KindUse`, `CREATE DATABASE`, `TruncateTable.is_database`,
the `DATABASE` token, and schema-**mapping data** whose top key happens to be `"db"`. A blanket
find-replace of `"db"` would corrupt these; they are left as-is.

**One inherited overload — `is_db_reference`.** For the `is_db_reference` construct (e.g. ClickHouse
`TRUNCATE DATABASE <db>`; `parser.go` `parseTableParts`), upstream reuses the Table's `db` slot to hold
a genuine **database** name (with `this` empty). The port reuses the same slot, so that name now lives
under the Table's **`schema`** arg — i.e. `SchemaName()` returns a database there. This is upstream's
own overloading of the qualifier slot, not a separate site to special-case; the enclosing node's
`is_database` flag is the discriminator. Round-trip is unchanged (see `parser/stmt_comment_truncate_test.go`).

**Observability:** `.sql()` round-trip output is **unchanged** (identity corpus stays 1847/1847). Two
surfaces change: (1) the Go API — `SchemaName()` / arg-key `"schema"` / builder params — which is a
**breaking change** for downstream consumers (use `SchemaName()` and `Arg("schema")`); and (2) `.ToS()`
/ repr now renders `schema=…` where upstream `repr()` renders `db=…`. The fidelity goldens were updated
to match (`testdata/fidelity_cases.txt`), so `TestFidelity`'s `WantAST` is the Python oracle **with the
documented `db=` → `schema=` substitution** — see the porting rule below.

**Porting rule (READ before porting any slice that touches Table/Column qualifiers):** when porting
upstream code that reads or writes the `db` arg / `.db` property of a `Table` or `Column` (including
`TABLE_PARTS`/`COLUMN_PARTS` ordering and any `repr` oracle captured from Python), translate `db` →
`schema`. Do **not** touch the genuine-database `db` listed above. When capturing a new
`fidelity_cases.txt` `want_ast` from live Python, apply `s/\bdb=/schema=/` to the Table/Column
qualifier key.

---

## Not deviations (called out to avoid confusion)

- Where a reviewer flagged an "upstream bug," the port generally **keeps upstream's behavior 1:1** (e.g.
  a qualify_columns edge case, `optimizer/qualify_columns.go`) rather than silently "fixing" it — that
  is *faithfulness*, the opposite of a deviation. §1.1 is the deliberate exception, made only because it
  is a genuine correctness/safety issue against the modeled engine.
