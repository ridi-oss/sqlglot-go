#!/usr/bin/env python3
"""Extract the dialect validate_identity() round-trip corpus (Scope B) from upstream
tobymao/sqlglot's dialect test suite into testdata/dialect_identity.jsonl.

Why this exists: tests/dialects/test_mysql.py and tests/dialects/test_postgres.py call
Validator.validate_identity(sql, write_sql=None, pretty=False, ...) hundreds of times, including
inside loops and f-strings that expand into many distinct cases at runtime (e.g. the
BIGINT/INT/... UNSIGNED loop in test_mysql.py, the pretty ARRAY[...] case and GRANT/REVOKE lists
in test_postgres.py). A static regex over the test source cannot recover those expanded cases. So
this script monkeypatches Validator.validate_identity to record every (dialect, sql, want, pretty)
tuple it is called with, then runs the selected dialect test classes under unittest so every call
site — including loop/template expansions — fires for real. The output feeds the Go parity harness
(corpus_test.go); it is not run by `go test` itself.
MySQL/Postgres records carry hand-added cases, so re-extract those dialects deliberately. For the
Presto-family dialects the same-dialect write of validate_all is recorded too.

Run manually (after `scripts/fetch-reference.sh`, since .reference/ is gitignored):

    PYTHONPATH=.reference/sqlglot-v30.17.0 python3 scripts/extract_dialect_corpus.py

Use --dialects athena,trino,presto,hive to replace only those dialects in the existing file.
Without --dialects, all six dialects are extracted and the file is fully rewritten.

Seongjin.
"""

import argparse
import json
import sys
import unittest
from pathlib import Path

REFERENCE_HINT = (
    "Could not import sqlglot. Run scripts/fetch-reference.sh first, then invoke this script as:\n"
    "  PYTHONPATH=.reference/sqlglot-v30.17.0 python3 scripts/extract_dialect_corpus.py"
)

# Upstream gates some validate_identity cases behind
# @unittest.skipUnless(sys.version_info >= (3, 11)) (tests/dialects/test_mysql.py:912). On an
# older interpreter unittest would silently skip them, truncating the corpus, yet still exit 0.
# Refuse to run below 3.11 so the re-runnable extractor cannot quietly drop cases.
if sys.version_info < (3, 11):
    sys.exit(
        f"extract_dialect_corpus.py requires Python >= 3.11 (running {sys.version.split()[0]}); "
        "older interpreters skip the >= 3.11-gated cases and would truncate the corpus."
    )

try:
    import sqlglot  # noqa: F401
except ImportError:
    print(REFERENCE_HINT, file=sys.stderr)
    raise

from tests.dialects.test_dialect import Validator  # noqa: E402
from tests.dialects.test_athena import TestAthena  # noqa: E402
from tests.dialects.test_trino import TestTrino  # noqa: E402
from tests.dialects.test_presto import TestPresto  # noqa: E402
from tests.dialects.test_hive import TestHive  # noqa: E402
from tests.dialects.test_mysql import TestMySQL  # noqa: E402
from tests.dialects.test_postgres import TestPostgres  # noqa: E402

DIALECTS = ("mysql", "postgres", "athena", "trino", "presto", "hive")
SAME_DIALECT_WRITES = {"athena", "trino", "presto", "hive"}

captured = []

orig_validate_identity = Validator.validate_identity
orig_validate_all = Validator.validate_all


def patched_validate_identity(
    self, sql, write_sql=None, pretty=False, check_command_warning=False, identify=False
):
    captured.append(
        {
            "dialect": self.dialect,
            "sql": sql,
            "want": write_sql or sql,
            "pretty": bool(pretty),
            "identify": bool(identify),
        }
    )
    try:
        return orig_validate_identity(
            self,
            sql,
            write_sql=write_sql,
            pretty=pretty,
            check_command_warning=check_command_warning,
            identify=identify,
        )
    except Exception:
        return None


def patched_validate_all(self, sql, read=None, write=None, pretty=False, identify=False):
    # Cross-dialect writes are out of scope, but write[self.dialect] is a same-dialect
    # expectation for sql. Recorded for the Presto-family dialects only; mysql/postgres keep
    # their hand-curated records. Best effort only, so an unrelated validate_all failure never
    # aborts a test method before its later validate_identity calls fire.
    if self.dialect in SAME_DIALECT_WRITES and write and self.dialect in write:
        want = write[self.dialect]
        if isinstance(want, str):
            captured.append(
                {
                    "dialect": self.dialect,
                    "sql": sql,
                    "want": want,
                    "pretty": bool(pretty),
                    "identify": bool(identify),
                }
            )
    try:
        # Presto checks transpilation logs before later identity cases (test_presto.py:754).
        if self.dialect == "presto":
            return orig_validate_all(self, sql, read, write, pretty, identify)
        self.parse_one(sql)
    except Exception:
        pass
    return None


Validator.validate_identity = patched_validate_identity
Validator.validate_all = patched_validate_all


# Test methods we expect to fail *because of* the monkeypatches above, keyed by
# (class, method). test_array_offset asserts on the "Applying array index offset"
# log lines emitted by the real cross-dialect validate_all transpile, which
# patched_validate_all neutralizes; it makes zero validate_identity calls, so its
# failure loses nothing. Any failure NOT listed here could have truncated a test
# method before its later validate_identity calls fired (leaving the corpus
# silently incomplete), so it aborts extraction before anything is written.
KNOWN_FAILURES = {
    ("TestPostgres", "test_array_offset"),
}


def _test_key(test):
    # test.id() is e.g. "tests.dialects.test_postgres.TestPostgres.test_array_offset";
    # the last two dotted parts are (class, method).
    parts = test.id().split(".")
    if len(parts) >= 2:
        return (parts[-2], parts[-1])
    return ("", test.id())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dialects", help="comma-separated dialects to replace; default: all six, full rewrite"
    )
    args = parser.parse_args()
    selected = set(DIALECTS if args.dialects is None else args.dialects.split(","))
    unknown = selected.difference(DIALECTS)
    if unknown:
        parser.error("unknown dialect(s): " + ", ".join(sorted(unknown)))

    runner = unittest.TextTestRunner(verbosity=0)
    unexpected = []
    for cls in (TestMySQL, TestPostgres, TestAthena, TestTrino, TestPresto, TestHive):
        if cls.dialect not in selected:
            continue
        result = runner.run(unittest.defaultTestLoader.loadTestsFromTestCase(cls))
        for test, _ in list(result.failures) + list(result.errors):
            key = _test_key(test)
            if key not in KNOWN_FAILURES:
                unexpected.append(".".join(key))
        # With Python >= 3.11 enforced above, the sole skip guard (test_mysql.py:912)
        # no longer fires, so NO test in the selected classes should skip. A skip here means a
        # method — and its validate_identity calls — silently dropped out of the corpus,
        # so treat it as a blocker just like a failure.
        for test, _ in list(result.skipped):
            unexpected.append(".".join(_test_key(test)) + " (skipped)")

    if unexpected:
        print(
            "Refusing to write corpus: the unexpected unittest failure(s)/skip(s) below may "
            "have truncated a test method before its validate_identity calls fired. Fix the "
            "extractor (or, once verified corpus-irrelevant, add to KNOWN_FAILURES) and "
            "re-run:\n  " + "\n  ".join(sorted(set(unexpected))),
            file=sys.stderr,
        )
        sys.exit(1)

    records = {}
    for rec in captured:
        if rec["dialect"] not in selected:
            continue
        key = (rec["dialect"], rec["sql"], rec["want"], rec["pretty"], rec.get("identify", False))
        records[key] = rec

    deduped = sorted(
        records.values(), key=lambda r: (r["dialect"], r["sql"], r["want"])
    )

    out_path = "testdata/dialect_identity.jsonl"
    preserved = {}
    if args.dialects is not None and Path(out_path).exists():
        for line in Path(out_path).read_text().splitlines():
            if line.strip():
                dialect = json.loads(line)["dialect"]
                if dialect not in selected:
                    preserved.setdefault(dialect, []).append(line)
    output = {}
    for rec in deduped:
        output.setdefault(rec["dialect"], []).append(json.dumps(rec))
    output.update(preserved)
    with open(out_path, "w") as f:
        for dialect in sorted(d for d in output if d):
            for line in output[dialect]:
                f.write(line + "\n")
        for line in output.get("", []):
            f.write(line + "\n")

    print(f"captured {len(captured)} validate_identity call(s)", file=sys.stderr)
    for dialect in sorted(output):
        print(f"  {dialect or 'base'}: {len(output[dialect])}", file=sys.stderr)
    count = sum(len(lines) for lines in output.values())
    print(f"wrote {count} deduped record(s) to {out_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
