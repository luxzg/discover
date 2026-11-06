#!/usr/bin/env bash
# Local remediation validation only; never deploys or opens production data.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
case "${1:-check}" in
  check|build|browser|security|security-source|scale|scripts|js|ingest|store|server) mode=${1:-check} ;;
  *) echo 'Usage: bash scripts/audit-check.sh [check|build|browser|security|security-source|scale|scripts|js|ingest|store|server]' >&2; exit 2 ;;
esac
[[ $# -le 1 ]] || exit 2
# The operator already installed this SDK. Do not download or change shell files.
if [[ -x "$HOME/sdk/go1.26.10/bin/go" ]]; then
  export PATH="$HOME/sdk/go1.26.10/bin:$PATH"
fi
source "$ROOT/scripts/tool-env.sh"
go version
case "$mode" in
  check) bash scripts/check.sh --race ;;
  build) bash scripts/build.sh ;;
  browser) bash scripts/test-browser.sh --all ;;
  security) bash scripts/security-check.sh --binary ;;
  security-source) bash scripts/security-check.sh ;;
  scale) bash scripts/test-hide-scale.sh ;;
  scripts) bash scripts/test-scripts.sh ;;
  js) node --test tests/*.test.cjs ;;
  ingest) go test -race ./internal/ingest ;;
  store) go test -race ./internal/store ;;
  server) go test -race ./internal/server ./internal/scheduler ;;
esac
