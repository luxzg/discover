#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/scripts/tool-env.sh"
cd "$ROOT"
case "${1:-}" in
  ''|--online) ;;
  *) echo 'Usage: bash scripts/tooling-report.sh [--online]' >&2; exit 2 ;;
esac
[[ $# -le 1 ]] || exit 2
date '+%Y-%m-%d %H:%M %Z'
go version
go env GOROOT GOTOOLCHAIN
node --version
npm --version
git status --short --branch
if [[ ${1:-} == --online ]]; then
  echo '==> Official stable Go releases (review before choosing an upgrade)'
  releases=$(curl -fsSL --connect-timeout 10 --max-time 30 'https://go.dev/dl/?mode=json')
  printf '%s' "$releases" | node -e 'let s="";process.stdin.on("data",c=>s+=c);process.stdin.on("end",()=>{for(const r of JSON.parse(s))if(r.stable)console.log(r.version)})'
  echo '==> Go module update candidates (not automatically applied)'
  go list -mod=readonly -m -u -f '{{.Path}} {{.Version}}{{if .Update}} -> {{.Update.Version}}{{end}}' all
  echo '==> npm update candidates (not automatically applied)'
  status=0
  outdated=$(npm outdated --include=dev --json) || status=$?
  printf '%s' "$outdated" | node -e '
    let s="";
    process.stdin.on("data", c => s += c);
    process.stdin.on("end", () => {
      const status=Number(process.argv[1]);
      const result=JSON.parse(s);
      console.log(JSON.stringify(result, null, 2));
      if (status > 1 || result.error || (status === 1 && !Object.keys(result).length)) process.exit(1);
    });
  ' "$status"
fi
