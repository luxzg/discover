#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/scripts/tool-env.sh"
cd "$ROOT"
scanner=${GOVULNCHECK:-govulncheck}
command -v "$scanner" >/dev/null || {
  echo 'Install: go install golang.org/x/vuln/cmd/govulncheck@latest' >&2
  exit 1
}
case "${1:-}" in
  ''|--binary) ;;
  *) echo 'Usage: bash scripts/security-check.sh [--binary]' >&2; exit 2 ;;
esac
[[ $# -le 1 ]] || exit 2
echo '==> Dated security check (requires external vulnerability/registry access)'
date '+%Y-%m-%d %H:%M %Z'
go version
"$scanner" -version
echo '==> Go source reachability, including uncalled advisory details'
"$scanner" -show verbose ./...
if [[ ${1:-} == --binary ]]; then
  [[ -x ./discover ]] || { echo 'Build discover before a binary scan.' >&2; exit 1; }
  echo '==> Built binary metadata and vulnerability scan'
  go version -m ./discover
  "$scanner" -mode binary -show verbose ./discover
fi
echo '==> npm audit, including development dependencies'
npm audit --include=dev
echo '==> Shared Playwright installation audit (read-only; managed by operator)'
playwright_module=${PLAYWRIGHT_MODULE:-$HOME/dev/playwright-tools/node_modules/playwright}
tool_root=$(dirname -- "$(dirname -- "$playwright_module")")
[[ -f "$tool_root/package-lock.json" ]] || { echo 'Shared tooling lockfile unavailable; audit not completed.' >&2; exit 1; }
npm --prefix "$tool_root" audit --include=dev
