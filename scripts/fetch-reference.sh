#!/usr/bin/env bash
# Fetch the pinned Python source of truth for the port into .reference/ (gitignored).
#
# This is upstream tobymao/sqlglot at the exact version being ported (v30.12.0). It is NOT
# vendored into the repo — it is the reference you port FROM and the oracle the probe parity
# harness runs against. Run this once after cloning.
set -euo pipefail

VERSION="v30.12.0"
EXPECTED_SHA="64e268a5d95cd84a87ad74ef569fc9bf356fd3fb"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$REPO_ROOT/.reference/sqlglot-$VERSION"

if [ -f "$DEST/sqlglot/__init__.py" ]; then
  echo "Reference already present at $DEST"
  exit 0
fi

echo "Cloning tobymao/sqlglot $VERSION into $DEST ..."
mkdir -p "$REPO_ROOT/.reference"
git clone --depth 1 --branch "$VERSION" https://github.com/tobymao/sqlglot.git "$DEST"

SHA="$(git -C "$DEST" rev-parse HEAD)"
echo "$SHA" > "$DEST/GIT_SHA.txt"
if [ "$SHA" != "$EXPECTED_SHA" ]; then
  echo "WARNING: cloned SHA $SHA != pinned $EXPECTED_SHA — the tag may have moved." >&2
fi

echo "Done. Reference at $DEST (sqlglot $VERSION, SHA $SHA)."
echo "Verify the Python oracle: PYTHONPATH=$DEST python3 -c 'import sqlglot; print(sqlglot.__version__)'"
