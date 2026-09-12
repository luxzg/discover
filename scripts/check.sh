#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/scripts/tool-env.sh"
cd "$ROOT"
case "${1:-}" in
  --race|'') ;;
  -h|--help) echo 'Usage: bash scripts/check.sh [--race]'; exit 0 ;;
  *) echo 'Usage: bash scripts/check.sh [--race]' >&2; exit 2 ;;
esac
[[ $# -le 1 ]] || exit 2
git diff --check
git diff --cached --check
mapfile -d '' go_files < <(git ls-files -z --cached --others --exclude-standard -- '*.go')
if [[ ${#go_files[@]} != 0 ]]; then
  unformatted=$(gofmt -l "${go_files[@]}")
  if [[ -n "$unformatted" ]]; then
    printf 'Run gofmt on these files:\n%s\n' "$unformatted" >&2
    exit 1
  fi
fi
export GOFLAGS="${GOFLAGS:-} -mod=readonly"
go vet ./...
bash scripts/test.sh "$@"
go build ./...
bash scripts/build.sh
bash scripts/smoke-test.sh
