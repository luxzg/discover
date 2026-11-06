#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
echo '==> Checking the operator-managed shared Playwright and matching Chromium'
echo 'No installs, updates, downloads or OS package changes are performed.'
node scripts/browser-tools.cjs
