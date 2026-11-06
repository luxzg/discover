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
install_binary() {
  # Never copy through predictable service-owned candidate/recovery aliases.
  local staged
  staged=$(service mktemp "$APP_DIR/.discover-install.XXXXXXXX")
  if ! service cp -- "$1" "$staged"; then
    service rm -f -- "$staged"
    return 1
  fi
  if ! service chmod 0755 "$staged"; then
    service rm -f -- "$staged"
    return 1
  fi
  if ! service mv -fT -- "$staged" "$APP_DIR/discover"; then
    service rm -f -- "$staged"
    return 1
  fi
}
echo
echo '==> Building and checking candidate as service user; service stays running'
service git -C "$APP_DIR" pull --ff-only
service env APP_DIR="$APP_DIR" bash "$APP_DIR/scripts/update_remote.sh"
echo
echo '==> Retaining candidate, previous binary and config in a root-owned snapshot'
backup_root=${BACKUP_ROOT:-/var/backups/discover}
[[ "$backup_root" == /* && "$backup_root" != *"'"* && "$backup_root" != *$'\n'* && "$backup_root" != *$'\r'* ]] || { echo 'Invalid backup root.' >&2; exit 1; }
# Inspect the original components: realpath would conceal symlink traversal.
IFS=/ read -r -a backup_parts <<< "$backup_root"
backup_root=''
for part in "${backup_parts[@]}"; do
  [[ -n "$part" ]] || continue
  [[ "$part" != . && "$part" != .. ]] || { echo 'Backup path must use trusted explicit directory names.' >&2; exit 1; }
  backup_root+="/$part"
done
[[ -n "$backup_root" ]] || { echo 'Backup root cannot be the filesystem root.' >&2; exit 1; }
trusted_backup_dir() {
  local mode uid
  [[ -d "$1" && ! -L "$1" ]] || { echo 'Backup ancestry must be trusted directories, not symlinks.' >&2; return 1; }
  uid=$(stat -c %u -- "$1")
  mode=$(stat -c %a -- "$1")
  if [[ "$uid" != 0 || ! "$mode" =~ ^[0-7]{3,4}$ ]] || (( (8#$mode & 0022) != 0 )); then
    echo 'Backup ancestry must be root-owned without group/other write access.' >&2; return 1
  fi
}
trusted_backup_dir /
prefix=''
for part in "${backup_parts[@]}"; do
  [[ -n "$part" ]] || continue
  prefix+="/$part"
  [[ ! -L "$prefix" ]] || { echo 'Backup ancestry must not contain symlinks.' >&2; exit 1; }
  if [[ -e "$prefix" ]]; then trusted_backup_dir "$prefix"; fi
done
umask 077
prefix=''
for part in "${backup_parts[@]}"; do
  [[ -n "$part" ]] || continue
  prefix+="/$part"
  # New parents need traversal for candidate checks, never directory listing.
  # Do not widen any pre-existing ancestor's permissions.
  if [[ ! -e "$prefix" ]]; then mkdir -m 0711 -- "$prefix"; fi
  trusted_backup_dir "$prefix"
done
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
      install_binary "$snapshot/discover.previous"
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
install_binary "$snapshot/candidate"
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
