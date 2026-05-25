#!/usr/bin/env bash
set -euo pipefail

# Run this script ON THE REMOTE SERVER (as discover user).
# It updates repo, rebuilds binary, restarts service, and prints status.

SERVICE_NAME="${SERVICE_NAME:-discover}"
APP_DIR="${APP_DIR:-/home/discover/apps/discover}"
BIN_PATH="${BIN_PATH:-$APP_DIR/discover}"

cd "$APP_DIR"

echo "==> Updating repository in $APP_DIR"
git pull --ff-only

echo "==> Syncing Go modules"
go mod tidy

echo "==> Building binary: $BIN_PATH"
go build -o "$BIN_PATH" ./cmd/discover

echo "==> Restarting service: $SERVICE_NAME"
sudo systemctl stop "$SERVICE_NAME"
sudo systemctl start "$SERVICE_NAME"

echo "==> Service status"
sudo systemctl status "$SERVICE_NAME" --no-pager -n 25

echo "==> Recent logs"
sudo journalctl -u "$SERVICE_NAME" -n 50 --no-pager

echo "==> Done"
