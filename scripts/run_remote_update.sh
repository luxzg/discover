#!/usr/bin/env bash
set -euo pipefail

# Run this script LOCALLY to execute remote update script over SSH.

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
if [[ ! "$REMOTE_USER" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ || ! "$REMOTE_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.:-]*$ ]]; then
  echo 'Invalid SSH user or host; use a hostname, IPv4 address or unbracketed IPv6 address.' >&2
  exit 2
fi

echo
echo "==> Connecting to $REMOTE"
echo "==> This flow uses sudo on the remote host for service/log commands"
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
quote_shell() { printf "'%s'" "${1//\'/\'\\\'\'}"; }
# Send reviewed administrator-side orchestration as one quoted argument. Keep
# stdin attached to the TTY for SSH/sudo authentication, not a script pipe.
remote_command="sudo bash -c $(quote_shell "$(cat "$SCRIPT_DIR/deploy.sh")")"
ssh -tt "$REMOTE" "$remote_command"

echo
echo "==> Update sequence finished"
