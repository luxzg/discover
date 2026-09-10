#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
mapfile -d '' files < <(git ls-files -z --cached --others --exclude-standard -- '*.go')
if [[ ${#files[@]} -gt 0 ]]; then gofmt -w "${files[@]}"; fi
