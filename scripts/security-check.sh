#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
export PATH="$HOME/go/bin:/usr/local/go/bin:$PATH"
cd "$ROOT"
command -v govulncheck >/dev/null || {
  echo 'Install: go install golang.org/x/vuln/cmd/govulncheck@latest' >&2
  exit 1
}
govulncheck ./...
