#!/usr/bin/env python3
"""Regenerate dialects/mysql_casefold.tsv — the canonical MySQL identifier case-fold table.

Source of truth: MySQL's `my_unicase_default` `.tolower` map (the case map used by
`utf8mb3_general_ci`, which is `system_charset_info` — the collation MySQL resolves
identifiers under). It is a simple 1:1, accent-PRESERVING, BMP-only lowercase:
    É -> é   (0x00C9 -> 0x00E9)   but   Ñ != N   and   ß unchanged.

Neither Go's strings.ToLower nor the JVM's String.lowercase() reproduces this map, and they
diverge from each other on characters like U+0130 / final sigma. So sqlglot-go and any JVM
consumer that must produce a byte-identical normalized identifier both embed THIS file.

Usage:
    # 1. get the pinned MySQL source (branch 8.0):
    #    curl -sL https://raw.githubusercontent.com/mysql/mysql-server/8.0/strings/ctype-utf8.cc -o /tmp/ctype-utf8.cc
    # 2. regenerate:
    python3 scripts/gen_mysql_casefold.py /tmp/ctype-utf8.cc dialects/mysql_casefold.tsv

Verified anchors (mysql-server @ 8.0): the identifier collation
`my_charset_utf8mb3_general_ci` uses `caseinfo = &my_unicase_default`; that table's page map
`my_unicase_pages_default[256]` selects the per-page `MY_UNICASE_CHARACTER planeXX[]` tables,
each entry `{toupper, .tolower, sort}` — identifier folding reads `.tolower` (NOT `.sort`,
which is the accent-folding collation weight).
"""
import re
import sys

TERM = r"\}\s*;"  # array close "};"; inner triples "{a,b,c}," never contain it


def parse_plane(src: str, name: str) -> list[int]:
    m = re.search(r"MY_UNICASE_CHARACTER " + re.escape(name) + r"\[\]\s*=\s*\{(.*?)" + TERM, src, re.S)
    if not m:
        raise SystemExit(f"plane not found: {name}")
    triples = re.findall(r"\{\s*0x([0-9A-Fa-f]+)\s*,\s*0x([0-9A-Fa-f]+)\s*,\s*0x([0-9A-Fa-f]+)\s*\}", m.group(1))
    if len(triples) != 256:
        raise SystemExit(f"{name}: expected 256 entries, got {len(triples)}")
    return [int(t[1], 16) for t in triples]  # .tolower (2nd field)


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("usage: gen_mysql_casefold.py <ctype-utf8.cc> <out.tsv>")
    src = open(sys.argv[1]).read()

    pm = re.search(r"my_unicase_pages_default\[256\]\s*=\s*\{(.*?)" + TERM, src, re.S).group(1)
    tokens = [t.strip() for t in pm.split(",") if t.strip()]
    if len(tokens) != 256:
        raise SystemExit(f"page map: expected 256, got {len(tokens)}")

    planes: dict[str, list[int]] = {}
    mapping: dict[int, int] = {}
    for page, tok in enumerate(tokens):
        if tok in ("nullptr", "NULL", "0"):
            continue
        if tok not in planes:
            planes[tok] = parse_plane(src, tok)
        low = planes[tok]
        base = page << 8
        for off in range(256):
            cp, lo = base + off, low[off]
            if lo != cp and lo != 0:
                mapping[cp] = lo

    with open(sys.argv[2], "w") as f:
        f.write("# MySQL identifier case-fold table — canonical, byte-exact.\n")
        f.write("# Source of truth: MySQL 8.0 my_unicase_default `.tolower` (utf8mb3_general_ci = system_charset_info),\n")
        f.write("# strings/ctype-utf8.cc. Simple 1:1 lowercase, accent-PRESERVING, BMP-only (utf8mb3).\n")
        f.write("# Line format: <codepoint-hex>\\t<lowercase-hex>, only for codepoints whose fold differs from identity.\n")
        f.write("# sqlglot-go AND any consumer (e.g. the JVM proxy) MUST embed THIS file verbatim so the normalized\n")
        f.write("# identifier is byte-identical across languages. Do NOT regenerate from a different Unicode version\n")
        f.write("# or a stdlib case function (they diverge — see DEVIATIONS.md). Regenerate via scripts/gen_mysql_casefold.py.\n")
        for cp in sorted(mapping):
            f.write(f"{cp:04X}\t{mapping[cp]:04X}\n")
    print(f"wrote {sys.argv[2]} ({len(mapping)} entries; planes {sorted(planes)})")


if __name__ == "__main__":
    main()
