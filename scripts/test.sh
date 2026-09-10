#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
export PATH="$HOME/toolchains/go1.26.8/bin:$HOME/go/bin:/usr/local/go/bin:$PATH"
cd "$ROOT"
race=()
case "${1:-}" in
  --race) [[ $# == 1 ]] || exit 2; race=(-race) ;;
  -h|--help) echo 'Usage: bash scripts/test.sh [--race]'; exit 0 ;;
  '') ;;
  *) echo 'Usage: bash scripts/test.sh [--race]' >&2; exit 2 ;;
esac
if [[ ${#race[@]} != 0 && $(go env CGO_ENABLED) != 1 ]]; then
  echo '--race requires CGO_ENABLED=1 and a supported native Go target/C compiler.' >&2
  exit 1
fi
export GOFLAGS="${GOFLAGS:-} -mod=readonly"
go test "${race[@]}" ./...
bash scripts/test-scripts.sh
while IFS= read -r -d '' file; do
  node --check "$file"
done < <(find internal/server/web -type f \( -name '*.js' -o -name '*.mjs' -o -name '*.cjs' \) -print0)
if [[ -d tests ]]; then
  node --test tests/*.test.cjs
fi
