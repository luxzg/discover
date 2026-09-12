#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/scripts/tool-env.sh"
cd "$ROOT"
[[ $# == 0 ]] || { echo 'Usage: bash scripts/test-sqlite-upgrade.sh' >&2; exit 2; }
# v2.25 module locks: last release with modernc.org/sqlite v1.39.1.
baseline=65f1d54a4eea9e0c47e213c621f661fe8bcabe1e
git cat-file -e "$baseline:go.mod"
work=$(mktemp -d -t discover-sqlite-compat.XXXXXXXX)
trap 'rm -rf -- "$work"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$work/old" "$work/new" "$work/data"
for engine in old new; do
  echo
  echo "==> Building isolated $engine driver test (may download locked Go modules)"
  mkdir -p "$work/$engine/internal/db"
  cp internal/db/*.go "$work/$engine/internal/db/"
  for file in go.mod go.sum; do
    if [[ $engine == old ]]; then
      git show "$baseline:$file" > "$work/$engine/$file"
    else
      cp "$file" "$work/$engine/$file"
    fi
  done
  (
    cd "$work/$engine"
    go test -mod=readonly -c -o "$work/$engine.test" ./internal/db
  )
done
export DISCOVER_SQLITE_COMPAT_DIR="$work/data"
echo
echo '==> Create with old engine, verify/write with new, reopen with old and new'
DISCOVER_SQLITE_COMPAT_MODE=create "$work/old.test" -test.run '^TestSQLiteEngineCompatibility$' -test.v
export DISCOVER_SQLITE_COMPAT_MODE=verify
"$work/new.test" -test.run '^TestSQLiteEngineCompatibility$' -test.v
"$work/old.test" -test.run '^TestSQLiteEngineCompatibility$' -test.v
"$work/new.test" -test.run '^TestSQLiteEngineCompatibility$' -test.v
