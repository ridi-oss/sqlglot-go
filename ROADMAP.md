# sqlglot-go — roadmap

Goal: a faithful Go port of sqlglot v30.12.0's **parse → AST → generate** core (tokenizer, AST,
parser, generator, schema) plus the `qualify` + `scope` optimizer passes that column qualification and
**lineage** build on, for **base + MySQL + Postgres**. This is deliberately **not** a full port of
sqlglot: the rest of the optimizer (simplify/normalize/pushdown/eliminate/merge/unnest/`optimize()`),
cross-dialect transpilation, and the other 30+ dialects are out of scope for now. Port 1:1 from
.reference/sqlglot-v30.12.0/ file-by-file; port the matching upstream tests as the oracle.

Status: the parse → generate round-trip is at **100% parity on the ported upstream identity corpus** —
1847/1847 cases (base 955/955, MySQL 424/424, Postgres 468/468), enforced by a monotonic corpus floor
(`corpus_test.go` + `testdata/parity_gaps.txt`, now empty) and an AST-fidelity gate (`fidelity_test.go`
+ `testdata/fidelity_cases.txt`: no statement fakes a round-trip via a raw-text `exp.Command` where
upstream builds a structured node). The initial slices below stood up the parse→qualify→scope pipeline;
the residual-tail and CREATE-properties fidelity slices then closed the last round-trip + fidelity gaps.
Slices 0–5 are done and committed; each landed `go test ./...` green. **The CURRENT remaining work is
the optimizer** — see "Remaining work" after the slice ledger.

Slices (historical ledger; each landed `go test ./...` green before the next):

0. FOUNDATION (this run) — DONE when green:
   errors, trie, tokens (TokenType/Token/TokenizerCore/Tokenizer), expressions core
   (Node model + core/query nodes needed for SELECT), minimal SELECT parser, base Dialect.
   Tests: test_tokens.py (all but test_jinja), subset of test_expressions.py/test_parser.py.

1. PARSER CORE — split into 1a/1b so each lands green:
   - 1a: DONE (committed on branch sjcho/sqlglot-go/parser-core; 52 tests green). Grammar
     green, function long-tail + CAST deferred. Includes set operations,
     Subquery/derived tables/scalar subqueries, CTE/WITH, GROUP/HAVING/ORDER/LIMIT/
     OFFSET/FETCH/QUALIFY/DISTINCT/WINDOW/FILTER, predicates (IN/EXISTS/ANY/ALL/
     BETWEEN/IS DISTINCT FROM), CASE/IF/Paren/Tuple, function-call dispatch,
     FUNCTION_BY_NAME, Anonymous fallback, and a curated common-function set.
   - 1b: DONE when green. Landed DML/DDL statement roots, minimal CREATE/Command,
     CAST/`::`/DataType coordination, specialized FUNCTION_PARSERS, bracket
     literals/indexing, LATERAL/UNNEST/VALUES/PIVOT, and the M1 root probes.
   - 1c: DONE (committed on branch; 73 tests green). Landed LOCK/FOR, CLUSTER/DISTRIBUTE/
     SORT BY, PREWHERE, window extras (WITHIN GROUP / IGNORE-RESPECT NULLS / frame EXCLUDE),
     full parseTypes (nested/UDT/parameterized/MAP/STRUCT/ARRAY/enum/NULLABLE/COLLATE),
     INTERVAL literals, full PIVOT/UNPIVOT (Any/Alias), JSON column operators
     (-> ->> #> #>> → JSONExtract*/JSONBExtract*), SELECT TOP wiring, and a function batch.
   - 1d: defer CREATE detail (properties, column + table CONSTRAINT_PARSERS, indexes,
     clone/sequence/materialized), remaining STATEMENT_PARSERS + parse_into(into=),
     CONNECT BY / START WITH, angle-bracket inline STRUCT constructor, and the remaining
     long function tail. (None probe-critical; probe's KNOWN_ROOTS already parse.)

2. GENERATOR CORE — DONE (branch sjcho/sqlglot-go/generator; 81 tests green). Base Generator
   in a new generator/ package (table-driven Kind→sql dispatch, query-modifier clause order,
   identifier quoting for quote_identifiers, ANSI string escaping, pretty/compact). identity.sql
   round-trip: 732/955 pass, 203 need deferred 1d grammar, 20 gen-mismatch (tracked). Public
   API: Generate / Transpile + Expression.SQL. MySQL/Postgres generator overrides → slice 5.

3. SCHEMA — DONE (branch sjcho/sqlglot-go/schema; 93 tests green). New schema/ package:
   Schema iface + MappingSchema + EnsureSchema (nested-mapping normalization, ordered Mapping
   for deterministic iteration, string-part trie, dialect identifier normalization, column_names/
   get_column_type/find/supported_table_args). Completed DataType semantics in expressions/
   datatype.go: DataTypeBuild/FromStr (via a new ParseIntoFunc hook — no parser import cycle),
   DataTypeIsType + category sets (TEXT/INTEGER/FLOAT/NUMERIC/TEMPORAL/NESTED/…). time.py NOT
   needed (zero refs from schema/datatypes). UDF machinery deferred (probe/tests don't use it).
   A nested dict {table:{col:type}} → MappingSchema → column_names/get_column_type verified.

4. OPTIMIZER PASSES — split into 4a/4b so each lands green:
   - 4a: DONE (branch sjcho/sqlglot-go/scope; 103 tests green). New optimizer/ package:
     scope.py (Scope + ScopeType + build_scope + traverse_scope + walk_in_scope +
     find_all_in_scope; sources/selected_sources/cte_sources/external_columns/columns/
     is_union/union_scopes/subquery_scopes/... with lazy caching), plus the pre-passes
     qualify_tables, normalize_identifiers, isolate_table_selects. traverse_scope verified on
     JOIN/UNION/CTE/correlated-subquery. Ported scope + qualify_tables/normalize_identifiers/
     isolate_table_selects fixtures.
   - 4b: DONE (branch sjcho/sqlglot-go/qualify; 106 tests green). resolver + qualify_columns
     + validate_qualify_columns + quote_identifiers + expand_stars + the qualify() driver +
     simplify_parens. ALL 165 in-scope qualify_columns.sql fixtures pass (exact .sql() + AST
     invariant), 18 invalid cases raise. qualify() runs end-to-end (JOIN with unqualified cols
     → qualified + stars expanded + validated). Carried 4a items done: NamedSelects KindSubquery
     (+ Selects()); AST-invariant assertion added to the optimizer harness (assertASTInvariants).
   - 4c (DEFERRED, OFF probe's critical path): full TypeAnnotator/annotate_types (coercion
     tables, per-node type rules). KEY FINDING: annotate_scope is NEVER called for base/mysql/
     postgres — both call sites are gated by ANNOTATE_ALL_SCOPES / SUPPORTS_STRUCT_STAR_EXPANSION,
     both false in base (qualify_columns.py:112,788). A minimal constructible TypeAnnotator +
     no-op AnnotateScope suffices for qualify(); probe never triggers annotation. Port the full
     machinery only for annotate_types.sql test fidelity, after probe e2e. Also deferred:
     canonicalize/normalize (not on qualify's path).

5. MYSQL + POSTGRES WIRING — DONE (branch sjcho/sqlglot-go/dialects; 114 tests green).
   GetOrRaise("mysql"/"postgres") return real *Dialect values: per-dialect TokenizerConfig
   (MySQL backtick identifiers + # comments + backslash/bit/hex + keyword remaps; Postgres $$
   dollar-quotes + HSTORE [moved here from base per slice-3 TODO] + keyword/op deltas),
   NormalizationStrategy (MySQL = CASE_SENSITIVE per mysql.py:25 — NOT case-insensitive;
   Postgres = LOWERCASE), and dialect quoting (MySQL ` / Postgres "). VERIFIED end-to-end:
   parse → qualify → traverse_scope runs under BOTH dialects with correct identifier
   normalization (postgres lowercases unquoted; mysql keeps case) + dialect-correct quoting.
   Ported curated same-dialect validate_identity round-trips from test_mysql/test_postgres.
   - 5b (DEFERRED, not probe-critical): per-dialect parser FUNCTIONS ↔ generator TRANSFORMS/
     TYPE_MAPPING override tables (function + type-name remaps must land paired to avoid
     round-trip regressions; the base tables are package-global singletons). Includes MySQL
     ||/&&/XOR logical operators (DPipeIsStringConcat=false currently errors — safer than
     misparse), MySQL CAST(x AS TIMESTAMP/BLOB) round-trip, and the cross-dialect
     transpilation test cases (need the other 32 dialects — out of M1 scope).

(An external consumer's lineage analyzer was ported and parity-tested against Python during the
initial slices to drive/validate the API surface; it has since been removed — this repo is the SQL
engine only. That validation confirmed the qualify/scope pipeline matches upstream on ~94 real
queries across MySQL/Postgres.)

## Remaining work

The parse → generate round-trip is done for the ported corpus (see Status). The remaining work is
mostly the **optimizer** (port 1:1 from `.reference/`, port the matching upstream tests,
differential-check against the pinned Python):

- THE REST OF THE OPTIMIZER (the big one): `simplify` (full — only `simplify_parens` is present),
  `normalize` (CNF/DNF), `pushdown_predicates`, `pushdown_projections`, `eliminate_ctes` /
  `eliminate_joins` / `eliminate_subqueries`, `merge_subqueries`, `unnest_subqueries`,
  `optimize_joins`, `canonicalize` / `canonicalize_internal_names`, and the top-level `optimize()`
  rule orchestrator that chains them. Only `qualify` + `scope`/`traverse_scope` (+ `isolate_table_
  selects`) are ported today.
- FULL annotate_types (coercion tables + per-node type rules) — currently a minimal constructible
  stub; not yet a faithful port.
- CROSS-DIALECT transpilation: same-dialect round-trip is verified; reading one dialect and writing
  another is not. The per-dialect override tables that make it correct are partial — per-dialect
  parser `FUNCTIONS` ↔ generator `TRANSFORMS`/`TYPE_MAPPING` remaps (must land paired to avoid
  round-trip regressions), MySQL `||`/`&&`/`XOR` logical operators, MySQL `CAST(x AS TIMESTAMP/BLOB)`.
- DIALECTS beyond base + MySQL + Postgres (upstream ships 30+).
- PARSER coverage is bounded by the ported corpus: constructs upstream parses that are NOT exercised
  by the identity fixtures may still be gaps — e.g. `exp.Heredoc` for postgres `CREATE FUNCTION ... AS
  $$...$$` dollar-quoted bodies (still degrades to Command; see the fidelity-slice deferred item),
  and any long `FUNCTIONS`/`FUNCTION_PARSERS` tail or DDL detail not hit by a fixture. Treat a
  not-yet-parsed construct upstream parses as a gap to close.

## MySQL grammar + tokenizer correctness + typed dialect + Node.Meta — DONE (PR #2)

**DONE (2026-07).** Branch `sjincho/parity/mysql-grammar-tokenizer-meta`, PR
[ridi-oss/sqlglot-go#2](https://github.com/ridi-oss/sqlglot-go/pull/2) — reviewed (Codex gpt-5.6-sol xhigh +
Opus, each verified vs the pinned reference), `go test ./...` / `go vet` / `gofmt` green. Motivated by the
proxy-monster suggestions review; five slices, each a faithful port, an API restoration, or a documented
correctness fix:

- **P1 — MySQL/base `MATCH(cols) AGAINST('x' [modifier])`.** Registered the base `FUNCTION_PARSERS["MATCH"]`
  entry (parser.py:1508 → `_parse_match_against`, parser.py:8181). The `exp.MatchAgainst` node + base/PG
  generators already existed (reachable only via PG `@@`); this closed the parser gap. Ports
  `tests/dialects/test_mysql.py::test_match_against`.
- **P2 — MySQL `GROUP_CONCAT(expr [, …] [ORDER BY …] [SEPARATOR s])`.** Ported `_parse_group_concat`
  (parser.py:10074, MySQL `FUNCTION_PARSERS`, parsers/mysql.py:156) **and** the MySQL generator
  (generators/mysql.py:174) together — parser-only would re-emit the comma form (per-row concat under the
  default separator), a re-generation correctness bug. Multi-arg → `CONCAT`, `ORDER BY` → `exp.Order`.
- **N1 — typed dialect (restores upstream `DialectType`).** `dialects.GetOrRaise`,
  `optimizer.NormalizeIdentifiers`, and `optimizer.QualifyOpts.Dialect` accept `nil | string |
  *dialects.Dialect`; the dialect is threaded through the optimizer + `EnsureSchema` as an `any` so a passed
  `*Dialect` instance flows through **unchanged** (matches upstream `qualify.py:78` — verified by pointer
  identity that `EnsureSchema(...).Dialect()` is the caller's instance). The string/settings form is
  unchanged. Added `(*Dialect).SettingsString()` / `dialects.CanonicalString`.
- **N3 — port `Node.Meta` (upstream `Expression.meta`, core.py:991/996).** `Node` gains a `meta
  map[string]any` with `Meta()` (lazy) / `MetaGet()` (no-alloc) + a deep-copy in `Copy` (core.py:1013);
  wired the `is_table` (qualify_tables.py:55/59, schema.py:704) and `case_sensitive`
  (normalize_identifiers.py:66-67) consumers. **Inert** for base/MySQL/Postgres — only BigQuery reads
  `is_table`, and `case_sensitive` has no producer until `sqlglot.meta` comment parsing is ported — so the
  corpus/fidelity are unchanged; the value is upstream parity + a foundation (resolved the `Node.Meta` TODOs
  in `schema.go`, `normalize_identifiers.go`, `qualify_tables.go`).

**Deviation added:** T1 — MySQL `--` line comment requires a trailing whitespace/control/EOF; otherwise it
is two `-` operators (`1--2` == `1 - -2`), matching the real server. Upstream mis-tokenizes it. Recorded in
**DEVIATIONS.md §1.4** (a same-dialect behavioral deviation, same class as the §1.1 ASCII fold), with a
regression test. Base/Postgres unchanged.

## M1 + P6 — extension mechanism + structural Postgres EXPLAIN — DONE (branch sjincho/parity/extension-mechanism-explain)

**M1 (Class-A divergence mechanism):** `testdata/upstream_extensions.jsonl` ledgers each construct
sqlglot-go parses structurally that pinned upstream does not; `extension_tripwire_test.go` enforces both
sides — `TestUpstreamExtensionsGoSide` (always-on: we parse each row to its `go_kind`) and the
`.reference`-gated `TestUpstreamExtensionsTripwire` (re-asserts pinned upstream still returns
`command`/`parse_error`, failing at a future reference bump with the row's reconcile note). See AGENTS.md
"How deviations are tracked" + DEVIATIONS.md "Grammar extensions beyond upstream".

## P6 — structural Postgres EXPLAIN grammar extension — DONE (same branch)

Postgres `EXPLAIN` now builds a `Describe` root with `kind = "EXPLAIN"`, structured `CopyParameter`
options, a parsed inner statement, and the option-list `wrapped` flag. The Postgres generator preserves
parenthesized comma-separated and legacy space-separated forms; unsupported forms still fail closed to a
verbatim `Command`. This is the first extension governed by the `pg-explain` row in
`testdata/upstream_extensions.jsonl`: pinned v30.12.0 remains `Command`, Go remains `Describe`, and
`TestUpstreamExtensionsGoSide` plus the `.reference`-gated `TestUpstreamExtensionsTripwire` enforce both
sides until a future upstream implementation is reconciled. Dedicated AST/round-trip/fallback tests and
the full build/test/vet/gofmt gate verify the slice. No identity-corpus fixture was added, so the corpus
floors remain **1847/1847** (base 955/955, MySQL 424/424, Postgres 468/468).

## P4 — structural MySQL INSERT SET + REPLACE grammar extensions — DONE

MySQL `INSERT INTO t SET a = 1, b = 2` now parses as `Insert` and intentionally canonicalizes to
`INSERT INTO t (a, b) VALUES (1, 2)`: the target is the existing `Schema(Table, columns)` shape and the
source is one-row `Values(Tuple(values))`, making the generated form idempotent. Supported MySQL
`REPLACE` statements also use the ordinary `Insert` target/source shape plus the Go-only optional
`replace = true` marker; for example, `REPLACE INTO t VALUES (1)` remains that canonical output through
the MySQL generator. The tokenizer no longer packs `REPLACE` as a raw
`Command`; a MySQL-only optional-`TABLE` lookahead preserves a target literally named `table`, a leading
`REPLACE(` retreats to ordinary expression parsing, and unsupported or partially consumed forms fail
closed to a source-preserving `Command`. Only MySQL generation observes the marker, so other dialects
and ordinary `Insert` nodes remain unchanged.

The Class-A ledger rows `mysql-insert-set` (`parse_error` upstream, Go `Insert`) and `mysql-replace`
(`command` upstream, Go `Insert`) lock the extensions to pinned v30.12.0 and define their reconciliation
lifecycle. No corpus or fidelity fixtures or floors were changed: the existing **1847/1847** identity
corpus (base 955/955, MySQL 424/424, Postgres 468/468), including the preexisting
`REPLACE INTO table SELECT id FROM table2 WHERE cnt > 100` row, remains the ratchet.

## R1 / R3 / A1′ / R2 — opt-in analysis + tokenizer enablers — DONE (v0.5.0)

**DONE (2026-07-12, main; released as v0.5.0.)** Four PRs of opt-in, additive enabler APIs for the
downstream lineage/gating consumer — none changes default same-dialect output; each is in DEVIATIONS.md
and (where it goes beyond upstream grammar) the Class-A ledger. Corpus/fidelity floors unchanged.

- **R1 — search-path table qualification (PR #3).** `QualifyOpts.SearchPath []string` stamps an
  unqualified table's `db` from the first schema in the path that _proves_ the table exists; fail-closed
  (a flat/db-incapable schema never stamps), resolves a 2-part probe against a 3-level
  `catalog.schema.table` schema, and leaves `catalog` to the caller. `QualifyTables` gained
  `searchPath, schema.Schema` trailing args.
- **R3 — top-level UPDATE/DELETE/MERGE scopes (PR #4).** A Go-only **analysis** traversal
  (`TraverseScope`/`BuildScope`) yields a DML-root scope binding the target + `FROM`/`USING`/`JOIN`
  sources + columns; complete-or-none (a malformed DML omits the root scope rather than emitting a
  partial one). The optimizer/qualify/validate passes keep the upstream-faithful
  `traverseScopeForOptimizer`, which omits these scopes — so no same-dialect drift. Security-sensitive:
  analysis-only, fail-closed. DEVIATIONS §6.
- **A1′ + V1/V2/A2 — MySQL version comments + tokenizer-sharing enablers (PR #6).** Opt-in
  `mysql_version=<MYSQL_VERSION_ID>` activates `/*!… */` and gated `/*!NNNNN … */` bodies into the token
  stream (default-off strips, DEVIATIONS §1.5). Plus `(*Dialect).IsReservedKeyword`, and documented
  fail-closed contracts for `Tokenize` (errors, never truncates) and `Token.Start`/`End` (byte-exact
  lexeme spans).
- **R2 — Qualify resolution report (PR #7, closes #5).** `QualifyOpts.ResolutionReport` surfaces the
  per-source `SourceKind` (Physical/CTE/Derived/Subquery/Unresolved) Qualify's scope pass already
  computes, classified from the resolved source's dynamic type (never the node's `KindTable`).
  `Unresolved` is the zero value (fail-closed); nil report is a strict no-op. Composes with R3: a DML
  root classifies its target _and_ read-sources from the analysis traversal.

## Athena support (Presto/Trino/Hive parser chain), scoped to lineage — DONE

**DONE (2026-07, main @ e428a54).** All 4 slices landed + merged, each `go test ./...` green with
base/MySQL/Postgres byte-identical throughout: (1) per-dialect parser-override seam `d9474c2`,
(2) Presto `22c7fbd`, (3) Hive `a1d322e`, (4) Trino + Athena router `e428a54`. `GetOrRaise` now
returns real dialects for **presto / trino / hive / athena** (parser + tokenizer only; generators
skipped as planned). Validation on the real RIDI Athena corpus (1,974 raw queries): `athena` parses
**1909 structured / 0 Command** (= the presto baseline); a `CREATE EXTERNAL TABLE … STORED AS PARQUET
LOCATION …` routes to Hive → structured `Create`; and `ParseOne → Qualify → TraverseScope` runs under
`athena` and resolves columns (proxy-monster's exact pipeline). The seam gained `PropertyParsers`
(slice 3) + token-keyed `NoParenFunctions` + an override-key indirection (slice 4). Known deferrals
(rare, lineage-safe, Anonymous): Presto DATE_FORMAT/DATE_PARSE/REGEXP_*/LOCALTIME family,
MATCH_RECOGNIZE, U&'…' UESCAPE; the 7 Hive CREATE-DDL property callbacks live in Hive's overlay (not
the shared registry) until a future paired parser+generator slice. The original plan follows.

Motivation: proxy-monster (RIDI's query-gating proxy) is adding an Athena proxy alongside its
MySQL/Postgres ones (planned, near-term). Its probe uses `parse` + `qualify` + `traverse_scope`
(+ a same-dialect `.sql()` ONLY for `SELECT *` expansion) — it does NOT canonicalize functions or
transpile cross-dialect. Empirical basis (6,072 unique RIDI Athena queries sampled from Redash;
1,974 raw + 4,098 `{{templated}}`): the CURRENT engine (postgres dialect, zero Athena code) already
structures 1,908/1,974 = 96.7% of the raw queries — matching the pinned Python `athena` dialect
(1,915), with ZERO Command fallbacks. So Athena support is a faithfulness/robustness investment for
the last ~3% + upstream-correct semantics, not a "can't parse it" necessity.

Approach: import the **parser + tokenizer** side of upstream's Athena chain 1:1 — athena→trino→presto
for queries, hive for DDL/CTAS lineage. That is ~570 LOC of `parsers/*.py` (presto 137, hive 298,
trino 63, athena 74) + the tokenizer deltas in each `dialects/*.py`. **Skip the generators** (~1,438
LOC of TRANSFORMS/TYPE_MAPPING) — the probe never canonicalizes or transpiles; the lone same-dialect
`SELECT *`-expansion `.sql()` round-trips via the base generator + `Anonymous` fallback (functions
like URL_DECODE/FORMAT_DATETIME stay Anonymous, which lineage sees through and the generator echoes
verbatim). Athena's parser is a ROUTER: `AthenaParser.parse()` sends DDL to the Hive sub-parser and
queries to the Trino(→Presto) sub-parser.

PREREQUISITE — the per-dialect parser-override seam (parser-side of the deferred 5b): the parser's
`functionParsers` / `statementParsers` / `noParenFunctionParsers` maps and type-token sets are
package-global singletons today; only `d.Functions` (a FUNCTIONS overlay) + a few bool flags are
per-dialect. Presto's parser overrides `FUNCTION_PARSERS` (drops TRIM), adds `STATEMENT_PARSERS`
(Athena's `USING`→command), and brings ARRAY/MAP/ROW type tokens — none layerable per-dialect today.
Generalize the seam so a dialect supplies parser-registry overlays (dialect overlay ∪ base singleton).
This is reusable for every future dialect, so it is foundational, not throwaway.

Slices (each lands `go test ./...` green; regressed against the committed RIDI Athena query corpus):
1. PER-DIALECT PARSER-OVERRIDE SEAM (5b, parser-side): extend Dialect with FunctionParsers /
   StatementParsers / type-token overlays; thread them through the parser's registry lookups as
   (dialect overlay ∪ base singleton). Zero behavior change for base/MySQL/Postgres (empty overlays).
2. PRESTO: parser (FUNCTIONS + FUNCTION_PARSERS + the `_parse` methods for ARRAY/MAP/ROW, UNNEST,
   TRY, lambda `->`, casts/type tokens) + tokenizer deltas. The query-engine bulk — most of the value.
3. HIVE: parser — only the DDL surface Athena routes to Hive (CREATE [EXTERNAL] TABLE etc.) that
   CTAS/DDL lineage needs.
4. TRINO + ATHENA: thin Trino-over-Presto layer + the AthenaParser router (DDL→Hive, query→Trino) +
   athena tokenizer/dialect registration.

Out of scope (same as base/MySQL/Postgres): generator TRANSFORMS/TYPE_MAPPING, cross-dialect
transpilation, the other 30+ dialects. Corpus: RIDI's real Athena queries (sampled 2026-07 from Redash
`platform-athena` + siblings) — kept OUT of the public repo (internal SQL); used as a local
regression oracle only.

Landed (2026-07) — Presto parser + tokenizer (part of Slice 2, atop the committed parser-override
seam): `dialects.Presto()` (class flags, tokenizer config, `d.Functions` overlay of 34 entries), the
`ZONE_AWARE_TIMESTAMP_CONSTRUCTOR` cast promotion (`TIMESTAMP '<zoned literal>'` → TIMESTAMPTZ, Presto-
gated, zero base impact), the `U&'...'` UNICODE_STRING path, and the TRIM function-parser disable. On
the RIDI corpus (2,002 raw non-templated queries) Presto structures 1,935 with 0 Command fallbacks vs
Postgres 1,934/0 — matches-and-beats the baseline. Known divergences for this slice (differential-
tested vs Python 30.12.0):
- Generator TRANSFORMS/TYPE_MAPPING deferred (out of scope), so a structured function whose canonical
  class name differs from its Presto spelling round-trips to the CANONICAL name via the base generator
  (faithful to upstream presto-read + base-write): `ARBITRARY`→`ANY_VALUE`, `CARDINALITY`→`ARRAY_LENGTH`,
  `ROW`→`STRUCT`, `DAY_OF_WEEK`→`DAYOFWEEK_ISO`, `MD5`→`MD5_DIGEST`, `SHA256`→`SHA2(x, 256)`,
  `BITWISE_AND(a,b)`→`a & b`. The four acronym class names (`JSON_FORMAT`/`MD5_DIGEST`/`SHA2`/
  `DAYOFWEEK_ISO`) render via `generator/name.go` sqlNameOverrides mirroring upstream `_sql_names`
  (the camelToSnake split would otherwise emit `J_S_O_N_FORMAT` etc.).
- FUNCTIONS entries needing unported helpers stay Anonymous (fail-closed, lineage-safe, round-trip
  verbatim): DATE_FORMAT/DATE_PARSE/DATE_TRUNC/TO_CHAR (need build_formatted_time / TIME_MAPPING /
  date_trunc_to_time) and REGEXP_EXTRACT/REGEXP_EXTRACT_ALL/REGEXP_REPLACE (need build_regexp_extract
  + the default-group arg). LOCALTIME/LOCALTIMESTAMP, MATCH_RECOGNIZE grammar, TABLE_ALIAS_TOKENS
  |= {ANTI,SEMI}, and the `U&'...'` UESCAPE clause are also deferred (see plan).
- `TRIM(BOTH x FROM y)` fails to parse under Presto (Expecting `)`) — FAITHFUL to upstream, which drops
  TRIM from FUNCTION_PARSERS (presto.py:137), making the special TRIM grammar unreachable. This is the
  single corpus query Presto errors on where Postgres (which keeps the TRIM parser) succeeds.

Historical note: earlier entries below tagged items "off probe's critical path" / "fail-closed" —
that framing referred to a since-removed external consumer. For this repo they are simply parity gaps.

Cross-cutting deferred from foundation (tracked as TODOs in code):
- Expr→SQL (generator) — blocks all .sql() asserts.
- Reflection registries EXPR_CLASSES / FUNCTION_BY_NAME (expressions/__init__.py:47-51) →
  explicit Go registration tables (slice 1).
- Full schema/type annotation hierarchy beyond the parser's minimal DataType/DType nodes (slice 3).
- highlight_sql-rich parse errors already ported in foundation; parse_into(into=) deferred.

**All intentional deviations from upstream are consolidated in [DEVIATIONS.md](./DEVIATIONS.md).** The
headline behavioral one: ASCII-only identifier case-folding (`dialects/dialect.go` NormalizeIdentifier /
CaseSensitive) — a deliberate divergence from upstream's full-Unicode `str.lower()`/`str.upper()`
(dialect.py:1042-1050,1055-1064) to match how real engines fold on multibyte encodings (PostgreSQL
`downcase_identifier`, scansup.c: ASCII-only). See DEVIATIONS.md §1.1.

Known divergences from the r1–r3 adversarial review (differential-tested vs Python 30.12.0;
non-blocking for the foundation, must be resolved by the noted slice):
- arg ordering: newNode orders args by argTypes declaration order, not caller insertion
  order (expressions/core.go newNode). Cosmetic now — HashKey sorts keys, and Expression-
  valued children traverse in the same relative order, so equality/find/walk are unaffected.
  GENERATOR (slice 2): verified NOT needed — the only generic-iteration emit path,
  function_fallback_sql, iterates arg_types (class-declaration order), which argTypesFor(kind)
  already provides independent of Node.argOrder. Still MUST fix before serde (slice 6), which
  serializes the live args in order. (Upstream preserves insertion order via a dict.)
- parser-level comment bubbling: `SELECT a FROM t /* after */` attaches the trailing comment
  to the inner Identifier(t) rather than the Table node; and `_parse_alias` does not yet move
  a mid-expression comment next to the alias (upstream parser.py:8499-8501). Tokenizer-level
  attachment is correct. Cosmetic (AST-shape only) — round-trip output is unaffected since the
  comment still renders in the same textual position either way, so this doesn't fail the
  corpus; still worth fixing for AST fidelity. NOT the same bug as matchRParen's dropped
  expression-hint (see the residual-tail resolved-findings entry below, now fixed) - that one
  DID cause round-trip mismatches (`CAST(x AS INT) /* c */` bubbling all the way to the
  enclosing Select) and is closed.
- deferred-feature parse divergences (expected, un-skip as features land): `/*+ HINT */` errors
  instead of being ignored (slice 1); int64 overflow in ToPy/IsInt (latent until slice 4).
- Slice 1a intentionally drops `_parse_table`'s fast path so subquery detection runs before
  table-part parsing. This is a pure optimization divergence; revisit if parser profiling
  shows it matters.
- `IsWrapper` uses the Go AST's `truthy` helper rather than Python's `v is None` check because
  `newNode` does not store nil args. The wrapper semantics are equivalent for stored args.
- Full DataType semantics remain deferred to schema/DataType slice 3. Slice 1b only adds the
  parser-visible DType enum and DataType nodes needed for CAST/`::` and column definitions;
  generator `.sql()` and rich `.type` assertions stay deferred.

Resolved in the alias-role review pass (PM-escalated, now fixed + regression-tested):
- QualifyTables' setAlias folded the injected default alias as a PARENTLESS identifier
  (NormalizeIdentifiersString) BEFORE attaching it under the TableAlias — a faithful port of
  upstream's `normalize_identifiers(new_alias_name).name` (qualify_tables.py:93), which also omits
  the alias's role (it sets `is_table` only on db/catalog). Harmless upstream, but it starved the
  port's non-upstream role-aware MySQLCaseSensitiveTableNames strategy (lctn=0, §1.2) of the parent
  it reads: the alias for an unaliased mixed-case table was misclassified as column-level and
  lowercased (`FROM Users` → `AS users`) while the column qualifier stayed `Users`, so scope
  resolution could not bind and lineage came back empty. Fixed in optimizer/qualify_tables.go by
  normalizing the alias AFTER `alias.Set("this", ...)`, giving it the TableAlias("this") parent
  (isRelationLevelIdentifier). Output-identical to upstream for every strategy except the role-aware
  one; quoting still honored. Test: TestQualifyTablesDefaultAliasRelationRole. (Rejected the initial
  FoldIdentifierName(name, true) proposal: it folds unconditionally and would over-fold a quoted
  mixed-case alias under the ASCII strategies, diverging from upstream.)

Resolved in the foundation review pass (were latent, now fixed + regression-tested):
- Replace()/Pop() silently no-op'd on single-value (non-list) args — the core tree-rewrite
  primitive every optimizer pass depends on. Fixed in expressions/core.go Replace (route
  index<0 through Set, the index-nil path). Tests: TestReplaceSingleValueArg, TestPopSingleValueArg.
- _parse_alias built an invalid exp.Tuple{this:...} (Tuple has no `this` arg) → ArgError.
  Added exp.Aliases (this+expressions) and use it. Test: TestParseAliases.

Resolved in the slice-2 review pass:
- unnestSQL nil'd the local `offset` after folding WITH OFFSET AS <col> into the alias, so it
  dropped WITH ORDINALITY (and turned an ordinality column into a plain data column). Upstream
  clears only the offset ARG (generator.py:3444-3447 vs 3456-3457). Fixed in generator/sql.go.
  Test: TestUnnestWithOrdinality.

Resolved in the slice-1c review pass:
- parseWindow parsed the frame EXCLUDE option with raise_unmatched=false; upstream
  _parse_window (parser.py:8405) uses the default True, so a malformed EXCLUDE option must
  raise "Unknown option". Fixed in parser/parser.go. Test: TestWindowExtras (malformed case).

Resolved in the slice-4b review pass:
- expandStarsInScope reset the EXCEPT/RENAME/REPLACE maps per selection, so a modifier on a
  leading full `*` did not leak into a later bare `*` (upstream keys by id(table): full stars
  share the stable selected_sources key → leak; qualified stars use fresh keys → no leak).
  Fixed: maps declared once outside the loop; full-star keyed by source name (stable),
  qualified-star keyed by a per-selection-unique token. Test: TestExpandStarsFullStarLeak.
  All 165 in-scope fixtures still pass.

Slice-1b review disposition:
- Reviewer flagged parseValue ignoring its `values` param, claiming upstream has an
  `if not values and self._curr: return None` guard. VERIFIED against the pinned source:
  v30.12.0 `_parse_value` (parser.py:3783) declares `values=True` but never references it —
  the Go port is faithful; that guard exists in a different sqlglot version. No change.
- Genuine minor gap (deferred to dialect slice 5): parseValue does not yet honor
  SUPPORTS_VALUES_DEFAULT (`VALUES (DEFAULT)` → exp.var), a dialect flag; base is unaffected.

Resolved in the slice-1a review pass:
- parseLimit dropped upstream's `isinstance(expression, exp.Mod)` retreat (parser.py:5576-5579),
  so `LIMIT 10 % 3` built Mod(10,3) instead of erroring on the trailing operand. Restored the
  retreat in parser/parser.go parseLimit. Test: TestLimitPercentModRetreat.

Resolved in the residual-tail parity slice (closed the last 25 round-trip gaps in
testdata/parity_gaps.txt, now empty; corpus floors raised to 955/424/468 base/mysql/postgres,
all 1847 records passing):
- New node families ported 1:1 to expressions/kinds.go + expressions/residual_tail.go: BitString/
  HexString/ByteString (query.py:471-491, is_primitive=True — mysql `0x..`/`x'..'`/`b'..'`/`0b..`,
  postgres `e'..'`), SessionParameter (core.py:1837, mysql `@@GLOBAL.x`), PropertyEQ (core.py:2150,
  the `:=` ASSIGNMENT operator), Distance/DistanceNd (core.py:2154-2159, postgres `<->`/`<<->>`),
  Lag/Lead (aggregate.py:150-163), Concat (string.py:29-31, adjacent-string-literal rewrite only).
- tokens/tokenizer.go compileConfig now auto-populates UnescapedSequences from the default table
  (dialects/dialect.py:297-306) whenever StringEscapes['\\'] or ByteStringEscapes['\\'] is set —
  fixes mysql's `'\\"a'` round-trip (the tokenizer wasn't collapsing `\\` to one backslash) and
  makes postgres `e'\n'`/`e'\t'`/etc. scan to real control chars for the byte-string family above.
- generator/sql.go escapeStr split into escapeStr (unchanged default) + escapeStrOpts (full
  escape_str signature: escapeBackslash/delimiter/escapedDelimiter/isByteString,
  generator.py:2983-3005), needed by bytestringSQL's escape_backslash=False behavior (a literal
  backslash, e.g. postgres `e'\176'`'s octal escape, must round-trip unchanged).
- parseType (mysql only) ports parsers/mysql.py:545-558's BINARY-as-cast special case
  (`ORDER BY BINARY a` -> `ORDER BY CAST(a AS BINARY)`); parseFunctionCall's func-token gate
  gained CharsetIsFunction (mysql `CHARSET(...)`, parsers/mysql.py:69 FUNC_TOKENS).
  hexstringSQL/CHAR(0x.. USING ..) needed the HexString kind above to close (mysql
  `CHAR(0xC3A9 USING utf8mb4)`).
- parseAssignment now implements the ASSIGNMENT `:=` loop (parser.py:5790-5810, was a bare
  pass-through to parseDisjunction); primaryParsers[SESSION_PARAMETER] + parseSessionParameter
  added (parser.py:7168-7176) — together close mysql `@var1 := 1`/`@@GLOBAL.x`.
  factorTokens gained LR_ARROW/LLRR_ARROW -> Distance/DistanceNd (parser.py:917-918), closing
  postgres `<->`/`<<->>` and (as a side effect, per the plan's predicted prerequisite chain) the
  `LATERAL VERTICES(...) v1 <-> v2` gap, which needed no other grammar change.
- parseWrappedSelect was missing _parse_wrapped_select's Values-into-Table rewrap
  (parser.py:3828-3832): a parenthesized `(VALUES (1)) AS v(id) LEFT JOIN t ON ...` couldn't
  attach the trailing JOIN. Fixed.
- parseDatePart + a dialect-gated FUNCTION_PARSERS["DATE_PART"] entry port postgres's
  `_parse_date_part` (parsers/postgres.py:303-311): `DATE_PART(<part>, <value>)` desugars to
  `EXTRACT(<part> FROM <value>)`.
- parsePrimary's STRING dispatch was missing _parse_primary's adjacent-string-literal rewrite
  (parser.py:6871-6885): `'a' 'b' 'c'` now builds Concat (coalesce=dialect.CONCAT_COALESCE,
  a new Dialect field — dialects/dialect.py:404, postgres.py:15 override) instead of falling
  through to plain alias handling.
- parseBracket was missing _parse_bracket's ARRAY_CONSTRUCTORS swap (parser.py:7713-7721,
  table at :787-790): a bracket subscripting a bare "ARRAY" reference now builds a real
  exp.Array instead of a Bracket workaround, fixing `ARRAY[]::type[]` (empty array — Bracket's
  "expressions" arg is required, so an empty list tripped the same "Required keyword" check
  upstream's own error_messages has; Array's is optional). Needed a new arraySQL generator
  method (postgres bracket-notation via generators/postgres.py:502-509, `ARRAY(<subquery>)` for
  a query-typed sole element; base/mysql fall back to the pre-existing functionFallbackSQL paren
  form, verified against the oracle to be base's real behavior - the OLD Bracket-based
  `TestArraySizeDimDroppedBase` expectation was actually wrong, not identity-preserving, and is
  corrected). The old isArrayLiteralBracket helper + its bracketSQL special-case branch were
  removed during integration (parseBracket never produces that Bracket-with-"ARRAY"-column shape
  again; the postgres ARRAY[...] pretty-wrap path is now arraySQL -> inlineArraySQL).
- KindLag/KindLead registered (were unregistered, falling through to Anonymous, which has no
  AggFunc trait): this - not a parser change - is what actually closes
  `(LEAD(foo1, 1, 0)) OVER (...)`, since parseParen's existing `this.This().Is(TraitAggFunc)`
  window-reparse gate already handled the parenthesized-aggfunc case correctly once LEAD/LAG
  build a real AggFunc-trait node instead of Anonymous.
- matchRParen silently dropped its `expression` parameter (a no-op wrapping bare `p.match`),
  unlike upstream's `_match_r_paren(expression)` -> `_match(.., expression=expression)`, which
  attaches a same-line trailing comment after the just-matched `)` directly to that expression
  (parser.py:9432-9434, 1926-1938). Instead the comment lingered in p.prevComments and bubbled
  up to whatever outer node's next p.expression(...) call happened to consume it - e.g.
  `SELECT CAST(x AS INT) /* c */ FROM foo` attached the comment to the outer Select instead of
  the Cast. Fixed generically (matchRParen now calls p.addComments(expression) after a
  successful match); this single fix, at its ~11 non-nil call sites across the parser, also
  closed the separate `SELECT FOO(x /* c */) /* FOO */, b /* b */` gap with no further change.

Closed in the integration / review-findings pass (dialect-divergence findings that the
same-read==write corpus can't reach — cross-dialect transpile, bare/underscore literals, and
hand-built ASTs; each verified against the pinned reference and guarded by
TestReviewFindingsFixes):
- The CONCAT(...) function is now a registered builder (parser.parseConcat, porting the FUNCTIONS
  "CONCAT" lambda at parser.py:345-349) instead of an Anonymous call, so it composes with the
  generator work below: `CONCAT(a, b)` base->postgres -> `a || b`, postgres->mysql ->
  `CONCAT(COALESCE(a, ''), COALESCE(b, ''))`, same-dialect stays `CONCAT(a, b)`. It lives in the
  dialect-aware FUNCTION_PARSERS map (not FunctionByName) because safe/coalesce depend on the
  dialect. CONCAT_WS is intentionally left Anonymous: it needs a distinct exp.ConcatWs node whose
  concatws_sql wraps a NULL-coalescing dialect in a CASE (generator.py:3683+) - out of this port's
  scope, and it only diverges cross-dialect (never in the same-read==write corpus), so leaving it
  Anonymous is no regression.
- concatSQL now ports the full concat_sql (generator.py:3667-3682): the CONCAT_COALESCE branch
  transpiles a coalesce=false Concat to a `||` DPipe chain (concat_to_dpipe_sql, dialect.py:1804)
  so e.g. `'a' 'b'` base->postgres becomes `'a' || 'b'` (NULL-propagating) instead of CONCAT(...),
  and convert_concat_args (generator.py:3636-3665) wraps each non-string arg in COALESCE(e, '')
  when the write dialect's CONCAT doesn't coalesce but the node asks it to. The per-arg
  string/ARRAY-type skip still defers full annotate_types (ROADMAP 4c): a statically-known string
  literal is left unwrapped (matching upstream), any other arg is wrapped.
- hexstringSQL ports the full is_integer_type condition (generator.py:1578-1586): a
  HexString{is_integer:true} renders as its integer value even where the write dialect has a
  HEX_START (HEX_STRING_IS_INTEGER_TYPE is false for base/mysql/postgres). bytestringSQL's
  is_bytes cast now wraps the re-parsed byte-string node (exp.cast semantics) rather than the
  rendered literal, so `ByteString{is_bytes:true}` -> `CAST(e'abc' AS BYTEA)`, not
  CAST of a doubled-quote string. (No in-scope parser sets either flag; these guard the exposed
  node contract.)
- Bit/hex literal validation is now one CPython-faithful int() parser, tokens.ParseIntPython
  (*big.Int, bool), shared by the tokenizer and the generator's integer fallback so the accepted
  forms match exactly. It honors a leading +/- sign, a matching base prefix (0b/0o/0x), and single
  '_' separators between digits or right after the prefix (never leading/trailing/doubled). The
  quoted forms validate int(payload, base), so `x'0xA'`/`x'+A'`/`x'-FF'`/`x'A_B'` round-trip and
  `x'GG'` still errors; the bare forms validate int(fullText, base), so `0x_FF` tokenizes (payload
  "_FF" -> x'_FF') while `0x`/`0xGG` fall back to an identifier. The generator fallback panics
  (recovered by Generate into an error) on an invalid payload, matching CPython's ValueError - e.g.
  folding a bare `0x_FF` to base runs int("_FF", 16), which raises upstream too (an upstream
  asymmetry the port mirrors). An empty `x''`/`b''` is left as-is (upstream skips int() on it).
- The postgres pyformat placeholder name is parsed with parseIdVar(any_token=true), matching
  upstream `_parse_wrapped(self._parse_id_var)` (parsers/postgres.py:294-301), so a non-reserved
  keyword or a number is a valid name: `%(from)s` / `%(1)s`, not only a bare identifier `%(name)s`.
- postgres now sets has_bit_strings/has_hex_strings (= bool(BIT_STRINGS)/bool(HEX_STRINGS),
  tokens.py:581-582; postgres tables are non-empty, dialects/postgres.py:65-66), enabling the
  number scanner's bare `0b`/`0x` forms (`SELECT 0xFF` -> `SELECT x'FF'`), which base does not have.
- VARIADIC's postgres-only NO_PAREN_FUNCTION_PARSERS entry is now filtered per-dialect at both
  lookup sites (noParenFunctionParserFor) instead of self-gating inside the closure. The old
  self-gate returned nil after the caller had already committed to the no-paren path, so base/mysql
  `VARIADIC(x)` degraded to `VARIADIC AS (x)`; it now parses as an ordinary function call, and a
  bare `VARIADIC` stays a column in base/mysql (parsers/postgres.py:142; per-dialect table deferred
  to slice 5b).

Resolved in the CREATE-properties fidelity slice (drove a 95-row Python-oracle AST/SQL gate,
testdata/fidelity_cases.txt + fidelity_test.go; corpus stays 955/424/468 base/mysql/postgres, all
1847 records passing, parity_gaps.txt empty; the full-corpus Go-vs-Python Command audit went from
94 Go-only degradations to 1 — see the deferred Heredoc item below):
- AST (expressions/kinds.go + expressions/fidelity_*.go): added the property/query/constraint node
  families needed to model the worklist — Algorithm/AutoIncrement/Collate/Definer/Engine/Inherits/
  Like/Lock/Locking/Materialized/NoPrimaryIndex/OnCommit/PartitionedBy/PartitionByRange/
  PartitionByList/PartitionedOf/SchemaComment/SqlReadWrite/Temporary/Unlogged/WithData properties,
  PartitionList/PartitionBoundSpec/PartitionRange, AnalyzeHistogram/AnalyzeWith/UsingData, and the
  Compress/DateFormat/Exclude/InlineLength/Title/Uppercase/WithOperator/InOut column constraints.
  This is the worklist subset, NOT the full upstream 80+ PROPERTIES / all DDL constraints.
- Parser: real _parse_properties loop + POST_CREATE/NAME/SCHEMA/WITH/ALIAS/EXPRESSION/INDEX property
  placement in _parse_create; CREATE TYPE enum/composite; bare UNIQUE/PRIMARY/AMP indexes; postgres
  function parameter modes (InOutColumnConstraint); MySQL RANGE/LIST partition DDL; MySQL ALTER
  options + AUTO_INCREMENT; EXCLUDE / WITH-storage / reserved WITH-operator constraints; MySQL
  ANALYZE ... HISTOGRAM and structured COMMENT ON FUNCTION/PROCEDURE.
- Generator: PROPERTIES_LOCATION bucketing + dedicated renderers, full CREATE assembly order, MySQL
  partition rendering, postgres columndef_sql placing the parameter mode before the name (side-
  effect-free, does not pop() the constraint), and the new constraint/analyze renderers.
- Integration ToS-fidelity fixes so repr()==ToS() exactly (verified generation + Equal/hash are
  untouched — they use truthy()/isFalse and skip false/empty): arg-order rows for Create/Alter/
  UniqueColumnConstraint/IndexParameters now follow the parser's constructor-kwarg order (what
  repr's dict-insertion order reflects), not the class arg_types order; DType renders as its enum
  member name "DType.<NAME>" (USERDEFINED name != value); _parse_types sets nested on the scalar
  branch; parseExists returns the bool False (not nil) like upstream's and-chain, so Alter/Drop/
  Insert/Truncate/ColumnDef carry exists=False; Node.IsLeaf/isEmptyList handle a non-empty/empty
  []string (AnalyzeWith, UniqueColumnConstraint.options); UNIQUE index_type defaults to False;
  CollateProperty.default is set only under a leading DEFAULT (unlike CharacterSetProperty).
- Gate floors (fidelity_test.go): >=95 cases, >=93 command-free. Two command_exception rows are
  legitimate upstream nested Commands — `CREATE FUNCTION ... SET search_path TO 'public' AS ...` and
  the sibling SET-config row — whose SetConfigProperty.this is itself a Command in the pinned tree
  (_parse_set returns Command mid-CREATE, parser.py:9265-9275), so a Command-free port is impossible
  there; the root stays Create.
- Two ast_divergence rows (MySQL `PARTITION BY RANGE (YEAR|MONTH(col))`): upstream wraps all 11 MySQL
  date functions in TsOrDsToDate in the parser and removes it in the generator; this port elides
  both consistently (no exp.TsOrDsToDate Kind — generator/dialect_funcs.go). The CREATE parses to a
  full PartitionByRangeProperty and round-trips to identical SQL; only the incidental partition-
  expression arg shape differs, so want_ast stays the honest Python oracle and only the exact ToS()
  match is relaxed (capped at maxASTDivergences=2). Closing it belongs to the function-parity slice.
- Deferred (the 1 remaining full-corpus Go-only Command, outside the worklist): postgres
  `CREATE FUNCTION ... AS $$ ... $$` dollar-quoted (heredoc) UDF body degrades to Command because
  exp.Heredoc is not modeled (parser_ddl.go:258-269 fails closed; round-trips verbatim, so the
  corpus stays green). Adding exp.Heredoc is separate function/body-parsing work.
