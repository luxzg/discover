#!/usr/bin/env bash
set -euo pipefail

# Run this script LOCALLY to execute remote update script over SSH.

REMOTE_SCRIPT_PATH="/home/discover/apps/discover/scripts/update_remote.sh"
REMOTE_APP_DIR="/home/discover/apps/discover"

read -r -p "Remote host/IP: " REMOTE_HOST
if [[ -z "${REMOTE_HOST}" ]]; then
  echo "error: remote host is required" >&2
  exit 1
fi

read -r -p "Remote SSH user: " REMOTE_USER
if [[ -z "${REMOTE_USER}" ]]; then
  echo "error: remote ssh user is required" >&2
  exit 1
fi

REMOTE="${REMOTE_USER}@${REMOTE_HOST}"

echo
echo "==> Connecting to $REMOTE"
echo "==> This flow uses sudo on the remote host for service/log commands"
ssh -tt "$REMOTE" "bash -lc '
set -euo pipefail
echo
echo \"==> Stopping discover service\"
sudo systemctl stop discover
echo
echo \"==> Running update/build script as discover user\"
sudo -u discover bash -lc \"cd '$REMOTE_APP_DIR' && git pull --ff-only && bash '$REMOTE_SCRIPT_PATH'\"
echo
echo \"==> Starting discover service\"
sudo systemctl start discover
echo
echo \"==> Service status (latest 25 lines)\"
sudo systemctl status discover --no-pager -n 25
echo
echo \"==> Recent logs (latest 50 lines)\"
sudo journalctl -u discover -n 50 --no-pager
'"

echo
echo "==> Update sequence finished"
