#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/scripts/tool-env.sh"
cd "$ROOT"
case "${1:-}" in
  '') ;;
  -h|--help) echo 'Usage: bash scripts/test-hide-scale.sh'; exit 0 ;;
  *) echo 'Usage: bash scripts/test-hide-scale.sh' >&2; exit 2 ;;
esac
[[ $# -le 1 ]] || exit 2
export GOFLAGS="${GOFLAGS:-} -mod=readonly"
# Uses temporary fixtures only; no configured/production database is opened.
go test ./internal/store -run '^TestRuleEditLargeUnreadDatabase$' -count=1 -v
