#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
mapfile -d '' files < <(find scripts -type f -name '*.sh' -print0)
for file in "${files[@]}"; do
  bash -n "$file"
done
if command -v shellcheck >/dev/null; then
  shellcheck -x "${files[@]}"
elif [[ ${REQUIRE_SHELLCHECK:-0} == 1 ]]; then
  echo 'shellcheck is required for this run.' >&2
  exit 1
else
  echo 'shellcheck unavailable; bash syntax and isolated script tests still run.' >&2
fi
shopt -s nullglob
tests=(scripts/tests/*.test.cjs)
[[ ${#tests[@]} -gt 0 ]] || { echo 'Missing script tests.' >&2; exit 1; }
node --test "${tests[@]}"
