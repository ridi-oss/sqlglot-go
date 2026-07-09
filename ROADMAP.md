# sqlglot-go — roadmap

Goal: a faithful Go port of sqlglot v30.12.0 with behavioral parity for base + MySQL + Postgres —
tokenizer, AST, parser, generator, schema, and the qualify/scope optimizer passes. Anything upstream
sqlglot parses should parse here. Port 1:1 from .reference/sqlglot-v30.12.0/ file-by-file; port the
matching upstream tests as the oracle.

The initial slices below were sequenced to first stand up a working parse→qualify→scope pipeline
(that ordering was driven by an external consumer's needs); the CURRENT focus is closing parser/
feature parity with upstream — see "Remaining work" after the slice ledger. Slices 0–5 are done and
committed; each landed `go test ./...` green.

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

## Remaining work — parser/feature parity with upstream

Anything upstream sqlglot parses should parse here. Known gaps to close (port 1:1 from `.reference/`,
port the matching upstream tests, differential-check `.sql()` against the pinned Python):

- PARSER TAIL: table-valued function sources (`generate_series(...)`, `JSON_TABLE(...)`, `FOO(bar)`)
  as FROM/JOIN sources; `SIMILAR TO`; `FROM ONLY` (Postgres); `CONNECT BY` / `START WITH`; MySQL
  `ON DUPLICATE KEY UPDATE ... VALUES(col)`; the long `FUNCTIONS` / `FUNCTION_PARSERS` registry
  tail; DDL detail (properties, column + table CONSTRAINT_PARSERS, indexes, clone/sequence).
  (`ARRAY[...]` literals + `UNNEST(ARRAY[...])` are DONE as of the residual-tail slice below -
  parseBracket's ARRAY_CONSTRUCTORS swap builds a real exp.Array instead of a Bracket-shaped
  workaround.)
- FULL annotate_types (coercion tables + per-node type rules) — currently a minimal constructible
  stub; not yet a faithful port.
- Per-dialect parser/generator override tables (function + type-name remaps must land paired to
  avoid round-trip regressions), MySQL `||`/`&&`/`XOR` logical operators, MySQL
  `CAST(x AS TIMESTAMP/BLOB)` round-trip.
- Optional/larger: dialects beyond base + MySQL + Postgres; canonicalize/normalize/simplify beyond
  the pieces qualify needs.

Historical note: earlier entries below tagged items "off probe's critical path" / "fail-closed" —
that framing referred to the removed consumer. For this repo they are simply parser-parity gaps.

Cross-cutting deferred from foundation (tracked as TODOs in code):
- Expr→SQL (generator) — blocks all .sql() asserts.
- Reflection registries EXPR_CLASSES / FUNCTION_BY_NAME (expressions/__init__.py:47-51) →
  explicit Go registration tables (slice 1).
- Full schema/type annotation hierarchy beyond the parser's minimal DataType/DType nodes (slice 3).
- highlight_sql-rich parse errors already ported in foundation; parse_into(into=) deferred.

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
