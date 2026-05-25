#!/usr/bin/env bash
set -euo pipefail

# Run this script LOCALLY to execute remote update script over SSH.

REMOTE_SCRIPT_PATH="/home/discover/apps/discover/scripts/update_remote.sh"

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

echo "==> Running remote update script on $REMOTE"
ssh -tt "$REMOTE" "bash '$REMOTE_SCRIPT_PATH'"
