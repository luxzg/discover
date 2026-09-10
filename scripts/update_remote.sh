#!/usr/bin/env bash
set -euo pipefail

# Run this script ON THE REMOTE SERVER as discover user (or via sudo -u discover).
# It updates repo and builds/validates a candidate without replacing the live binary.
# Service control is handled by scripts/run_remote_update.sh using caller's sudo rights.

APP_DIR="${APP_DIR:-/home/discover/apps/discover}"
BIN_PATH="$APP_DIR/discover.next"

cd "$APP_DIR"

echo
echo "==> Updating repository in $APP_DIR"
if [[ -n $(git status --porcelain) ]]; then
  echo 'Working tree is dirty; resolve local changes before updating.' >&2
  exit 1
fi
git pull --ff-only

echo
echo "==> Building candidate with pinned modules (live binary unchanged)"
bash scripts/build.sh --output "$BIN_PATH"

echo
echo "==> Validating config and TLS access as service user"
"$BIN_PATH" --check-config -config "$APP_DIR/config.json"

echo
echo "==> Candidate ready; deploy.sh handles backup and service replacement"
