#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
[[ -x ./discover ]] || { echo 'Run scripts/build.sh first.' >&2; exit 1; }
node --test tests/cli.smoke.cjs
