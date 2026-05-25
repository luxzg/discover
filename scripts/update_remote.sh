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
echo "==> Updating repository in $APP_DIR"
git pull --ff-only

echo
echo "==> Syncing Go modules (go mod tidy)"
go mod tidy

echo
echo "==> Building binary at $BIN_PATH"
go build -o "$BIN_PATH" ./cmd/discover

echo
echo "==> Build phase complete"
