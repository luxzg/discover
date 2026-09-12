#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/scripts/tool-env.sh"
output="$ROOT/discover"
case "${1:-}" in
  --output) [[ $# == 2 ]] || exit 2; output=$2 ;;
  -h|--help) echo 'Usage: bash scripts/build.sh [--output PATH]'; exit 0 ;;
  '') ;;
  *) echo 'Usage: bash scripts/build.sh [--output PATH]' >&2; exit 2 ;;
esac
cd "$ROOT"
commit=$(git rev-parse --verify HEAD)
built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
output=$(realpath -m -- "$output")
[[ ! -d "$output" && ! -L "$output" ]] || { echo 'Output must not be a directory or symlink.' >&2; exit 1; }
tmp=$(mktemp "$(dirname -- "$output")/.discover-build.XXXXXXXX")
trap 'rm -f -- "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
go build -mod=readonly -trimpath -ldflags "-X discover/internal/buildinfo.Commit=$commit -X discover/internal/buildinfo.BuildTime=$built_at" -o "$tmp" ./cmd/discover
version=$(timeout 10 "$tmp" --version)
[[ "$version" == *"commit=$commit"* && "$version" == *"built=$built_at"* ]] || {
  echo 'Built artifact did not report the expected commit/date through --version.' >&2
  exit 1
}
chmod 0755 "$tmp"
mv -f -- "$tmp" "$output"
printf '%s\n' "$version"
