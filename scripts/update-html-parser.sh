#!/usr/bin/env bash
# Scoped dependency maintenance; no production or system installation effects.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
case "${1:-check}" in
  check) echo 'Usage: bash scripts/update-html-parser.sh apply'; echo 'apply updates only x/net to v0.62.0 and required module sums; then run audit-check.sh.'; exit 0 ;;
  apply) ;;
  *) exit 2 ;;
esac
[[ $# == 1 ]] || exit 2
source scripts/tool-env.sh
before=$(sed -n 's/^go //p' go.mod)
go get golang.org/x/net@v0.62.0
go mod tidy
after=$(sed -n 's/^go //p' go.mod)
[[ "$before" == "$after" ]] || { echo 'Unexpected module minimum change; review before proceeding.' >&2; exit 1; }
