#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
echo '==> Installing locked development dependencies (npm registry access)'
npm ci
echo '==> Installing matching Chromium (external browser download/cache access)'
echo 'No OS packages are installed. Ask the operator if native libraries are missing.'
timeout 180 node_modules/.bin/playwright install chromium
