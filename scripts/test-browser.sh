#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/scripts/tool-env.sh"
cd "$ROOT"
case "${1:-}" in
  ''|--production) modes=(production) ;;
  --development) modes=(development) ;;
  --all) modes=(production development) ;;
  *) echo 'Usage: bash scripts/test-browser.sh [--production|--development|--all]' >&2; exit 2 ;;
esac
[[ $# -le 1 ]] || exit 2
[[ -x node_modules/.bin/playwright ]] || { echo 'Run npm ci, then npx --no-install playwright install chromium.' >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
for mode in "${modes[@]}"; do
  echo "==> Browser validation: $mode build, synthetic loopback services only"
  if [[ $mode == production ]]; then
    bash scripts/build.sh
    binary="$ROOT/discover"
  else
    binary="$work/discover-dev"
    go build -mod=readonly -o "$binary" ./cmd/discover
  fi
  DISCOVER_TEST_BINARY="$binary" DISCOVER_BROWSER_MODE="$mode" node_modules/.bin/playwright test
done
