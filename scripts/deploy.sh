#!/usr/bin/env bash
set -euo pipefail
# Privileged orchestrator: execute ONLY from an administrator-controlled copy.
# The SSH wrapper sends its local reviewed copy, not the service user's checkout.
APP_DIR=${APP_DIR:-/home/discover/apps/discover}
SERVICE_USER=${SERVICE_USER:-discover}
SERVICE_NAME=${SERVICE_NAME:-discover}
[[ $(id -u) == 0 ]] || { echo 'Run this script with sudo.' >&2; exit 1; }
[[ "$APP_DIR" == /* ]] || { echo 'APP_DIR must be absolute.' >&2; exit 1; }
cd "$APP_DIR"
exec 9>/run/lock/discover-deploy.lock
flock -n 9 || { echo 'Another deployment is running.' >&2; exit 1; }
service() { runuser -u "$SERVICE_USER" -- "$@"; }
echo
echo '==> Building and checking candidate as service user; service stays running'
service git -C "$APP_DIR" pull --ff-only
service env APP_DIR="$APP_DIR" bash "$APP_DIR/scripts/update_remote.sh"
echo
echo '==> Retaining candidate, previous binary and config in a root-owned snapshot'
backup_root=${BACKUP_ROOT:-/var/backups/discover}
[[ "$backup_root" == /* && "$backup_root" != *"'"* && "$backup_root" != *$'\n'* ]] || { echo 'Invalid backup root.' >&2; exit 1; }
umask 077
mkdir -p -- "$backup_root"
# Traversal lets the unprivileged candidate run; directory listing remains private.
chmod 0711 "$backup_root"
snapshot=$(mktemp -d "$backup_root/release-XXXXXXXX")
chmod 0711 "$snapshot"
service cat ./discover.next > "$snapshot/candidate"
service cat ./discover > "$snapshot/discover.previous"
service cat ./config.json > "$snapshot/config.json"
chmod 0755 "$snapshot/candidate" "$snapshot/discover.previous"
service "$snapshot/candidate" --check-config -config "$APP_DIR/config.json"
database=$(service "$snapshot/candidate" --database-path -config "$APP_DIR/config.json")
[[ "$database" == /* ]] || database="$APP_DIR/$database"
command -v sqlite3 >/dev/null || { echo 'Install sqlite3 before deploying.' >&2; exit 1; }
[[ -f "$database" ]] || { echo 'Configured database does not exist; use first-install instructions.' >&2; exit 1; }
mkdir "$snapshot/data"
chown "$SERVICE_USER" "$snapshot/data"
was_active=0
if systemctl is-active --quiet "$SERVICE_NAME"; then was_active=1; fi
stopped=0
swapped=0
recover() {
  code=$?
  trap - EXIT
  if [[ $code != 0 && $stopped == 1 ]]; then
    echo
    echo '==> Deployment failed; recovering previous binary (database is NOT restored)' >&2
    if [[ $swapped == 1 ]]; then
      systemctl stop "$SERVICE_NAME" || true
      service cp -- "$snapshot/discover.previous" ./discover.recovery
      service mv -fT -- ./discover.recovery ./discover
    fi
    if [[ $was_active == 1 ]]; then systemctl start "$SERVICE_NAME" || true; fi
  fi
  chmod 0700 "$snapshot"
  exit "$code"
}
trap recover EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
echo
echo '==> Stopping service for consistent backup and atomic binary replacement'
stopped=1
systemctl stop "$SERVICE_NAME"
echo
echo '==> Backing up SQLite, including committed WAL data; snapshots are never deleted'
service sqlite3 -batch -bail -readonly "$database" '.timeout 10000' ".backup '$snapshot/data/discover.sqlite3'"
integrity=$(service sqlite3 -batch -bail -readonly "$snapshot/data/discover.sqlite3" 'PRAGMA quick_check;')
[[ "$integrity" == ok ]] || { echo 'Backup integrity check failed; snapshot retained.' >&2; exit 1; }
echo
echo '==> Installing candidate and starting service'
service cp -- "$snapshot/candidate" ./discover.next
service mv -fT -- ./discover.next ./discover
swapped=1
systemctl start "$SERVICE_NAME"
sleep 3
systemctl is-active --quiet "$SERVICE_NAME"
echo
echo '==> Installed version and service status'
service ./discover --version
systemctl status "$SERVICE_NAME" --no-pager -n 25
journalctl -u "$SERVICE_NAME" -n 50 --no-pager
echo
echo "==> Deployment complete; recovery files retained at $snapshot"
