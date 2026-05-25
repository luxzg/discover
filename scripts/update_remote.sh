#!/usr/bin/env bash
set -euo pipefail

# Run this script ON THE REMOTE SERVER as discover user (or via sudo -u discover).
# It updates repo and rebuilds binary only.
# Service control is handled by scripts/run_remote_update.sh using caller's sudo rights.

SERVICE_NAME="${SERVICE_NAME:-discover}"
APP_DIR="${APP_DIR:-/home/discover/apps/discover}"
BIN_PATH="${BIN_PATH:-$APP_DIR/discover}"

cd "$APP_DIR"

echo
echo "==> [1/4] Updating repository in $APP_DIR"
echo "==> Updating repository in $APP_DIR"
git pull --ff-only

echo
echo "==> [2/4] Syncing Go modules (go mod tidy)"
echo "==> Syncing Go modules"
go mod tidy

echo
echo "==> [3/4] Building binary at $BIN_PATH"
echo "==> Building binary: $BIN_PATH"
go build -o "$BIN_PATH" ./cmd/discover

echo
echo "==> [4/4] Build phase complete"
echo "==> Build done"
