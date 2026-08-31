# Deviations from upstream sqlglot

Ledger of every place sqlglot-go *intentionally* differs from pinned upstream (v30.12.0), so an
upstream bump or differential check can tell an intended divergence from a regression. One entry per
deviation: upstream's behavior → ours → the guarding test. Reasoning lives in the code comments
(grep `divergence` / `Unlike upstream`), commit history, and `ROADMAP.md`'s resolved-findings ledger.

Only **§1 changes same-dialect parse→generate output**; everything else is cross-dialect-only,
output-preserving, a not-yet-ported boundary, or a Go-only API extension. Grammar the port accepts
beyond upstream is additionally registered in `testdata/upstream_extensions.jsonl` (tripwired by
`TestUpstreamExtensionsTripwire`).

---

## 1. Behavioral deviations (same input → different output than upstream)

Each fixes a place upstream disagrees with the real engine; the port matches the engine.

### 1.1 ASCII-only identifier case-folding
Upstream folds unquoted identifiers full-Unicode (`str.lower()`): `CAFÉ` → `café`. Real engines fold
ASCII-only on multibyte encodings (PG `downcase_identifier`), so the port folds only `A-Z`↔`a-z`:
`CAFÉ` → `cafÉ`. Applies to every folding strategy. `dialects/dialect.go`;
test `identifier_casefold_test.go`.

### 1.2 MySQL-exact identifier folding (opt-in strategies)
Upstream's MySQL is `CASE_SENSITIVE` (folds nothing) — unchanged by default. Two opt-in
`NormalizationStrategy` values model real MySQL resolution:
- `MySQLCaseInsensitive` (lctn=1/2): folds everything with MySQL's `utf8mb3_general_ci` map
  (`dialects.MySQLLower`, generated table in `dialects/mysql_casefold_table.go` — accent-preserving,
  not Go's `strings.ToLower`).
- `MySQLCaseSensitiveTableNames` (lctn=0): role-aware — table/db names, table aliases, CTE names, and
  column qualifiers stay case-sensitive; column names fold. `information_schema` relations fold
  regardless (MySQL matches that schema case-insensitively).

`schema.NewMappingSchema(normalize=true)` normalizes keys role-aware and **fails closed** on two raw
keys folding to one normalized key (upstream `nested_set` is last-wins) — every folding dialect.
Tests: `optimizer/qualify_tables_mysql_test.go`, `schema/` tests.

### 1.3 Dialect settings string + DialectType polymorphism
`dialects.GetOrRaise` accepts upstream's settings-string form (`"mysql, normalization_strategy=…"`)
and the dialect-accepting entry points take `nil | string | *Dialect`, mirroring upstream
`DialectType`. Gap-closure, not a divergence. Only `normalization_strategy` (plus the Go-only
`mysql_version`/`mysql_ansi_quotes`) is supported; upstream's `version` is not.

### 1.4 MySQL `--` comment requires trailing space
Upstream comments out `SELECT 1--2` in every dialect. MySQL requires ASCII whitespace/control (or
EOF) after `--`, so under mysql the port tokenizes `1--2` as `1 - -2`. Postgres/base unchanged.
Test `tokenizer_mysql_comment_test.go`.

### 1.5 MySQL executable comment activation (opt-in `mysql_version`)
Default matches upstream: `/*! … */` bodies stay comments. With `mysql_version=<MYSQL_VERSION_ID>`
set, a bare `/*! … */` body — and a gated `/*!NNNNN … */` body when version ≥ gate — is tokenized as
SQL. Activation is semantic, not byte-preserving (`SELECT 1 /*!50000 + 100 */` regenerates as
`SELECT 1 + 100`). Optimizer hints `/*+ … */` stay comments. Test `mysql_version_comment_test.go`.

### 1.6 MySQL `RESET …` → `Command`
Upstream parses `RESET MASTER` as `Alias(RESET AS MASTER)`. The port degrades all MySQL `RESET …` to
`exp.Command`. `reset` as an identifier unaffected. `dialects/mysql.go`.

### 1.7 Postgres `U&'…'` / `U&"…"` decoded
Upstream mis-tokenizes `U&'\0067'` as `U & '…'` and errors on the identifier form. The port decodes
the SQL-standard escapes into an ordinary `Literal`/`Identifier`. A custom `UESCAPE 'c'` clause fails
closed. Ledger `pg-unicode-identifier`; tests `unicode_escape_test.go`, `tokens/unicode_escape_test.go`.

### 1.8 `SAVEPOINT` / `RELEASE SAVEPOINT` → `Savepoint`
Upstream parses `SAVEPOINT s` as an `Alias` and errors on `RELEASE SAVEPOINT s`. The port builds
`exp.Savepoint{this, kind}`; bare `RELEASE <name>` (no keyword) is Postgres-only. Output normalizes
release to `RELEASE SAVEPOINT <name>`. Ledger `release-savepoint`; test `savepoint_test.go`.

### 1.9 Postgres/MySQL reject a bare string as table name/alias
Upstream folds `FROM 'foo'` and `FROM t 'x'` into identifiers in every dialect; real PG/MySQL reject
both. Gated by `Dialect.StringTableIdentifiers` (default true = upstream; false for postgres/mysql —
parse error, or `Command` inside tryParse'd DDL). Explicit `AS '…'` table alias is still accepted
(matches upstream; a pre-existing quirk). Test `parser/string_aliases_test.go`.

### 1.10 Postgres `pg_catalog.<builtin>` resolves to the builtin
Upstream leaves `pg_catalog.int4` USER-DEFINED. For postgres, a two-part `pg_catalog.<name>` whose
tail is in a pinned allowlist of real pg_catalog type names resolves to the same node as the bare
spelling (`pg_catalog.int4` → `INT`), across `::`/CAST/space-typed-literal spellings. Aliases PG
rejects (`pg_catalog.integer`), other schemas, `pg_catalog.char`/`bit` (bare keyword is a different
type), and modifier-bearing `oid(5)`-style names stay USER-DEFINED. `parser/parser_types.go`;
test `TestParsePgCatalogBuiltinType`.

### 1.11 MySQL `ANSI_QUOTES` (opt-in `mysql_ansi_quotes`)
Upstream has no support; default MySQL treats `"…"` as a string. With `mysql_ansi_quotes=true`,
`"…"` is a quoted identifier. Parse-only; the generator already emits `'`/backtick, valid in both
modes. Test `mysql_ansi_quotes_test.go`.

### 1.12 Postgres `SET var = "quoted"` keeps the quotes
Upstream rewrites a quoted-identifier SET value to a bare `Var` (`SET search_path = "$user"` →
`= $user`, invalid PG). The port keeps a quoted value verbatim. `parser/stmt_set.go`;
test `TestPostgresSetMultiValue`.

### 1.13 MySQL `START REPLICA|SLAVE|GROUP_REPLICATION` → `Command`
Upstream parses these as `Transaction{modes:[REPLICA]}` (renders invalid `BEGIN REPLICA`). The port
degrades them to `Command` at top level, MySQL-only. `parser/stmt_transaction.go`;
tests `TestMySQLStartReplicationIsNotTransaction`, `TestStartReplicationDivertIsMySQLOnly`.

### 1.14 MySQL admin command leaders → `Command`
Upstream mis-parses `STOP …`/`FLUSH …`/`UNLOCK INSTANCE`/`XA …`/`BINLOG`/`HELP`/`RESTART`/`SHUTDOWN`
as `Alias`/`Column` (some `XA` forms error). The port degrades the whole statement to `Command` —
top-level only, so the words stay usable as identifiers and nested values.
`parser/stmt_mysql_command.go`; tests `TestMySQLCommandLeaders*`.

### 1.15 MySQL `TABLE tbl` → `SELECT * FROM tbl`
Upstream parses `TABLE users` as an `Alias`. The port builds the equivalent `Select` (MySQL defines
`TABLE t` as exactly that), top-level, plain table identifier only; trailer forms
(`ORDER BY`/`UNION`/…) fail closed. Qualified `TABLE db.users` is ledgered (`mysql-table-value`).
`parser/stmt_mysql_table.go`; tests `TestMySQLTableStatement*`.

### 1.16 MySQL `DROP INDEX idx ON db.users`
Upstream errors on the db-qualified target (single-id `_parse_on_property`). The port parses the ON
target with `parseTableParts` into `OnProperty{this: Table}` — the qualifier survives; the target is
a `Table`, not upstream's bare `Identifier`. Ledger `mysql-drop-index-on-qualified`;
test `TestParseDropIndexOnTable`.

### 1.17 Postgres dollar-quote tag preserved
Upstream drops a named heredoc tag on parse and always regenerates `$$…$$`; real PostgreSQL rejects
that whenever the body itself contains `$$` (the reason a named tag exists). The port keeps the tag
on `Heredoc.tag` (`$fn$…$fn$` round-trips verbatim). `parser_ddl.go` `heredocTag`;
test `TestHeredocTagPreserved`.

### 1.18 `END` terminates a routine's `BEGIN…END` block
Upstream `_parse_block` consumes ALL remaining chunks, folding
`CREATE PROCEDURE p() … END; SELECT 2` into ONE `Create` whose Block smuggles the trailing
`Select` after `EndStatement`. Real PostgreSQL 16 — the engine that parses this form server-side
(`BEGIN ATOMIC`) — treats the trailing statement as its own statement (verified: the SELECT runs
separately). The port returns the block at its terminating `END` chunk, yielding
`[Create, Select]`; a well-formed body is unchanged (its `END` is the last chunk). Applies to
every `parseBlock` (a top-level `WHILE (…) BEGIN … END; SELECT 2` also splits). Three adjacent
divergences in the same machinery: an empty body (`BEGIN END`, valid MySQL) terminates the same
way instead of upstream's `Column(END)` mis-parse; a bare top-level `ELSE` chunk is a parse error
(real MySQL/PG reject it) instead of upstream's silent drop of every later statement; and a WHILE
body without its own `BEGIN` is the single following statement (T-SQL's actual binding), so it
cannot steal the enclosing routine's `END`. `parser.go` `parseBatchStatements`/`parseWhileBlock`;
test `TestBlockEndTerminates`.

The same rule covers every routine form, not just PROCEDURE — upstream leaves the others' `END`
as a dangling top-level `EndStatement` fragment (a truncated definition plus a bare `END`, each
"authorized/executed" as a statement the user never wrote):
- MySQL `CREATE FUNCTION … BEGIN … END` takes the block path when a `BEGIN` body is present
  (upstream: single-statement body, dangling `END`).
- A CREATE the structured parse degrades (MySQL TRIGGER/DECLARE bodies, bare
  IF/LOOP/REPEAT/WHILE/CASE routine bodies, PG `BEGIN ATOMIC` —
  the block's first statement now gets the trailing-token check, so an unconsumable body
  statement degrades the whole definition instead of a mangled structured AST) becomes a
  `Command` extended through the block's balancing `END` chunk (`parseCreateAsCommand` +
  `blockStack`: a KIND-MATCHED stack, not a counter — openers push their kind, every
  `END [kw]` must pop exactly that kind, mismatch → parse error. Opener gating is
  contextual: bare IF/LOOP/REPEAT/WHILE only at statement starts (chunk start, after
  THEN/ELSE/BEGIN/ROW/DO/label-COLON; header positions like the signature `)` and routine
  characteristics only while no block is open), a leader before `(` only when the balanced
  paren group is followed by THEN/DO, and CASE only with a WHEN ahead. Identifiers named
  if/loop/case, `IF NOT EXISTS`, `IF(…)`/`DO IF(…)` calls, and subquery aliases never
  inflate the stack, so a later matching `END <kw>` cannot rebalance a frame that was
  never opened. The remaining ambiguity direction is FAIL CLOSED, never merge). PG 16 executes `BEGIN ATOMIC … END; SELECT 2` as exactly
  [definition, SELECT].
- The stack is token-level, so ambiguity FAILS CLOSED, never merges: a non-empty or
  mismatched stack at batch end (unterminated body, a closer against the wrong kind)
  is a parse error, and a residual bare top-level `END` chunk is a parse
  error under MySQL (real MySQL 8.0 rejects it) rather than upstream's `EndStatement`
  fragment. The same rule holds on the structured path: a `BEGIN` body that exhausts the
  batch without its terminating `END` (`CREATE PROCEDURE p() BEGIN SELECT 1;`) is a parse
  error in EVERY dialect (MySQL rejects it with 1064, PG's `BEGIN ATOMIC` likewise requires
  `END`; upstream silently truncates) — a body without `BEGIN` (`CREATE PROCEDURE p()
  SELECT 1`, a valid single-statement routine) is unaffected. A bare `END` as the whole
  MySQL batch is a parse error too (real MySQL 1064; upstream mis-parses `Column(END)`). Under postgres a top-level bare `END` chunk
  is COMMIT wherever it appears (`BEGIN; SELECT 1; END` → [Transaction, Select, Commit],
  matching real PG 16; upstream leaves the mid-batch one an `EndStatement`).
Tests `TestRoutineBodyOneStatement`, `TestDegradedCreateFailsClosed`,
`TestQualifiedKeywordNotBlockToken`, `TestUnsupportedBodyStatementDegradesWhole`.

---

## Opt-in behavioral extensions beyond upstream

Additive analysis features; default behavior and fixture output unchanged.

- **Search-path table qualification** (`QualifyOpts.SearchPath`): resolves the schema part by proven
  existence against the supplied schema, in order; no fallback to `DefaultSchema`, no catalog stamp,
  unproven stays unqualified (fail-closed). Names fold role-aware (§1.2). Empty path = upstream path.
- **Qualify resolution report** (`QualifyOpts.ResolutionReport` → `SourceKind` per source): exposes
  the source classification the qualify scope pass already computes. `Unresolved` is the zero value.
  DML roots populate from the §6.1 analysis traversal.

Neither is a grammar construct — no ledger row.

---

## Grammar extensions beyond upstream

Constructs upstream Commands or parse-errors that this port structures. Each is registered in
`testdata/upstream_extensions.jsonl` by the id below (the ledger row records upstream's fallback and
reconciliation note); malformed/engine-invalid forms fail closed to `Command` or a parse error.

| ledger id | construct → node |
|---|---|
| `pg-explain` | Postgres `EXPLAIN` → `Describe{kind:"EXPLAIN"}` with structured options + parsed inner statement |
| `mysql-insert-set` | `INSERT INTO t SET a=1` → normalized `Insert` + one-row `Values` (renders as `INSERT … VALUES`) |
| `mysql-replace` | `REPLACE INTO …` → `Insert{replace:true}` (MySQL generator renders `REPLACE`) |
| `mysql-describe-column` | `DESCRIBE tbl col\|'wild'` → `Describe{this:Table, column}` (single identifier/wild only) |
| `pg-set-role`, `pg-set-session-authorization`, `pg-set-time-zone`, `pg-set-names`, `pg-set-constraints`, `pg-set-session-characteristics` | Postgres SET special forms → `Set{SetItem{kind:…}}` |
| `pg-set-transaction-deferrable` | PG `[NOT] DEFERRABLE` transaction mode (PG-only mode table) |
| `pg-set-multi-value` | `SET search_path = a, b` → one `SetItem`, extra values in `expressions` (PG-only; strict `var_value` list) |
| `pg-set-scoped-role` | `SET [SESSION\|LOCAL] ROLE\|SESSION AUTHORIZATION` → same kinds + `SetItem.scope` |
| `mysql-set-password` | `SET PASSWORD [FOR user] = v` → `SetItem{kind:"PASSWORD"}` |
| `mysql-set-role`, `mysql-set-default-role` | MySQL `SET [DEFAULT] ROLE …` → `SetItem{kind:"ROLE"\|"DEFAULT ROLE"}`; strict role-name/CSV grammar, engine-invalid forms fail closed |
| `mysql-show-create-user` | `SHOW CREATE USER u` → `Show{this:"CREATE USER"}` |
| `mysql-show-grants-user-host`, `mysql-show-grants-using` | `SHOW GRANTS [FOR user [USING roles]]` → structured `Show` |
| `pg-show-guc`, `pg-reset` | PG `SHOW`/`RESET {name\|ALL\|special}` → `Show`/`Reset` with **canonical** `this` (lowercased, unquoted; `TIME ZONE`→`timezone`, `SESSION AUTHORIZATION`→`session_authorization`) |
| `mysql-into-outfile`, `mysql-into-dumpfile` | `SELECT … INTO OUTFILE\|DUMPFILE '/path'` → `Into{kind, this:path}` + structured export options; placement/tail rules match MySQL, violations fail closed |
| `pg-user-type-typed-literal` | PG `<type-name> 'str'` (space typed-literal) → the same `Cast` as `'str'::type`; also ports the `STRING_ALIASES` flag properly (base/PG reject implicit string aliases, MySQL accepts) |
| `pg-start-transaction`, `mysql-start-transaction-snapshot` | `START TRANSACTION [modes]` → `Transaction` (PG `START`→BEGIN token; MySQL `WITH CONSISTENT SNAPSHOT`) |
| `mysql-create-user`, `mysql-create-role`, `mysql-alter-user`, `mysql-drop-user`, `mysql-drop-role` | account DDL → structured `Create`/`Alter`/`Drop` root with `kind:"USER"\|"ROLE"`; body kept verbatim in a `Command` child |

Cross-cutting rules for the SET/SHOW family:
- **Quoted dispatch keywords never match** — `SET "ROLE" x`, `` SET PASSWORD `=` 'x' ``,
  `RESET "all"` fail closed rather than regenerate as the unquoted (different) statement.
  Upstream's `_find_parser`/`_match_texts` match quoted tokens by text; the port does not.
- The privileged keyword forms are disambiguated from same-named GUC assignments by the following
  `=`/`:=`/`TO` delimiter (`SET role = x` parses as an assignment `EQ`, readable LHS).
- `parseMySQLUserSpec` enforces MySQL's `user` grammar (`name[@host]`, `CURRENT_USER[()]`); a dotted
  unquoted host under-accepts to `Command`.
- Known approximation: the port has no PG reserved-word table (upstream leaves it empty), so a few
  keyword-shaped names over/under-accept — all fail-safe (documented per-site in the parsers).

---

## 2. Cross-dialect-only deviations (never affect same-dialect round-trip)

Cross-dialect transpilation is out of scope. Presto/trino/hive/athena generator
`TRANSFORMS`/`TYPE_MAPPING` are not ported, and several of their functions parse as `Anonymous`
(`DATE_FORMAT`, `DATE_PARSE`, `TO_CHAR`, `DATE_TRUNC`, `REGEXP_*`, `LOCALTIME[STAMP]`, `CONCAT_WS`)
— lineage still sees column args; same-dialect `.sql()` echoes them.

---

## 3. Output-preserving, Go-necessitated divergences

None change `.sql()`:
- `newNode` orders args by per-Kind `argTypes` declaration order, not insertion order.
- `parseTable` fast-path skip (parse-order optimization).
- `IsWrapper` uses the `truthy` helper (no nil args stored).
- `matchTextSeq` retreat has no debug logger.

---

## 4. Cosmetic AST-shape divergences (output identical, `.ToS()` differs)

- Comment bubbling: a trailing comment can attach to a slightly different node.
- MySQL `PARTITION BY RANGE (YEAR(col))`: upstream wraps/unwraps `TsOrDsToDate`; the port elides
  both (capped in `fidelity_test.go` `maxASTDivergences`).

---

## 5. Deferred / fail-closed (`Command` where a future slice would structure)

- Hive CREATE-DDL property callbacks (kept in Hive's `PropertyParsers` overlay; base/mysql/postgres
  fail them closed).
- Upstream `_parse_heredoc`'s `$`-textseq fallback (parser.py:9372-9399, for dialects that don't lex
  dollar-quotes): pinned upstream's own base/mysql round-trip of it is broken (`… END AS $$`), so
  only the HEREDOC_STRING branch is ported (Postgres bodies parse to `exp.Heredoc`).

---

## 6. Go-only analysis API extensions

### 6.1 Top-level UPDATE/DELETE/MERGE scopes
Upstream `traverse_scope` yields no scope for a DML root. `TraverseScope`/`BuildScope` additionally
yield a root `Scope` (target + FROM/USING/JOIN sources), complete-or-none (a malformed source omits
the root scope, never emits a partial one). The optimizer passes use a separate compatibility
traversal reproducing upstream exactly. Tests `optimizer/scope_dml_test.go`.

### 6.2 Projection + statement source spans
Upstream discards token positions at parse time. The port stamps each SELECT projection (inside any
`Alias`) with `meta["span"]` (rune offsets) and `meta["spanText"]` (verbatim text — `1 +    1` keeps
its spacing), both-or-neither, via `parseSpanned`. Meta survives `Copy()`, never affects output/
`HashKey`/`Equal`; a rewrite that replaces the node drops it. Accessors in `expressions/span.go`.
Every top-level statement `Parse` returns is stamped the same way (a multi-chunk statement — a
procedure `BEGIN…END` body — spans all its chunks, inner semicolons included), so a batch consumer
can split a multi-statement input into verbatim per-statement slices without a `Generate()`
rewrite. The span is the statement's token range: separators and comment text outside the first/
last token are not covered (the slice is exact, not exhaustive). A boundary token spliced from an
activated MySQL executable comment widens to the `/*!NNNNN … */` delimiters — statement spans only
— so the slice stays executable MySQL.
Tests `expressions/span_test.go`, `parser/parser_span_test.go`, `optimizer/qualify_span_test.go`,
`statement_span_test.go`.

---

## 7. Qualifier arg renamed `db` → `schema`

Upstream calls the middle table/column qualifier `db` — a misnomer for the ANSI **schema**. The port
renames that one qualifier everywhere: arg-key `"schema"`, `SchemaName()`,
`TablePartKeys = {this, schema, catalog}`, builders `Table_(table, schema, catalog, …)`,
`QualifyOpts.DB` → `QualifyOpts.DefaultSchema`. `.sql()` output unchanged; `.ToS()` renders
`schema=` where Python renders `db=`.

**NOT renamed** (genuine databases): `Show.db` (`SHOW … FROM <db>`), `Use`, `CREATE DATABASE`,
`TruncateTable.is_database`, the `DATABASE` token, schema-mapping data keys, and 1:1-ported flag
names like `Dialect.RenameTableWithDB`. The `is_db_reference` construct reuses the (now-`schema`)
slot for a database name, as upstream reuses `db` — discriminated by `is_database`.

**Porting rule:** upstream code touching a Table/Column `db` arg / `.db` → translate to `schema`;
leave genuine-database `db` alone. Python-captured `fidelity_cases.txt` oracles: apply
`s/\bdb=/schema=/` (qualifier only).

---

## Not deviations

Where a reviewer flags an "upstream bug", the port generally keeps upstream 1:1 (faithfulness);
§1 entries are the deliberate exceptions, each matching the real engine.
