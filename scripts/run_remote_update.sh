#!/usr/bin/env bash
set -euo pipefail

# Run this script LOCALLY to execute remote update script over SSH.

REMOTE_SCRIPT_PATH="/home/discover/apps/discover/scripts/update_remote.sh"
REMOTE_APP_DIR="/home/discover/apps/discover"

REMOTE_HOST=""
REMOTE_USER=""

usage() {
  cat <<'EOF'
Usage:
  ./scripts/run_remote_update.sh [-ip <host_or_ip>] [-user <ssh_user>] [-h|--help]

Examples:
  ./scripts/run_remote_update.sh
  ./scripts/run_remote_update.sh -ip 10.10.10.10 -user myusername
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -ip)
      if [[ $# -lt 2 ]]; then
        echo "error: -ip requires a value" >&2
        exit 1
      fi
      REMOTE_HOST="$2"
      shift 2
      ;;
    -user)
      if [[ $# -lt 2 ]]; then
        echo "error: -user requires a value" >&2
        exit 1
      fi
      REMOTE_USER="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "${REMOTE_HOST}" ]]; then
  read -r -p "Remote host/IP: " REMOTE_HOST
fi

if [[ -z "${REMOTE_HOST}" ]]; then
  echo "error: remote host is required" >&2
  exit 1
fi

if [[ -z "${REMOTE_USER}" ]]; then
  read -r -p "Remote SSH user: " REMOTE_USER
fi

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
