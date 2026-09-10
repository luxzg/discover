#!/usr/bin/env bash
set -euo pipefail
# Explicit paths only: never discover or open a private DB implicitly.
database=''
backup_dir=''
owner=$(id -un)
usage() { echo 'Usage: bash scripts/backup-database.sh --database PATH --backup-dir DIR [--owner USER]'; }
while [[ $# -gt 0 ]]; do
  case "$1" in
    --database|--backup-dir|--owner)
      [[ $# -ge 2 && -n "$2" ]] || { usage >&2; exit 2; }
      case "$1" in
        --database) database=$2 ;;
        --backup-dir) backup_dir=$2 ;;
        --owner) owner=$2 ;;
      esac
      shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
[[ -n "$database" && -n "$backup_dir" ]] || { usage >&2; exit 2; }
command -v sqlite3 >/dev/null || { echo 'Install sqlite3 before backing up.' >&2; exit 1; }
[[ -f "$database" ]] || { echo 'Database does not exist; refusing to create it.' >&2; exit 1; }
uid=$(id -u "$owner")
[[ "$uid" == "$(id -u)" ]] || { echo 'Backups must remain owned by the executing user; ownership transfers are not supported.' >&2; exit 1; }
umask 077
mkdir -p -- "$backup_dir"
backup_dir=$(cd -- "$backup_dir" && pwd -P)
if [[ $(id -u) == 0 ]]; then
  parent=$backup_dir
  while :; do
    mode=$(stat -c %a -- "$parent")
    [[ $(stat -c %u -- "$parent") == 0 ]] && (( (8#$mode & 0022) == 0 )) || {
      echo 'Root backups require a root-owned path without group/other-writable ancestors.' >&2; exit 1;
    }
    [[ "$parent" != / ]] || break
    parent=$(dirname -- "$parent")
  done
fi
[[ "$backup_dir" != *"'"* && "$backup_dir" != *$'\n'* && "$backup_dir" != *$'\r'* ]] || {
  echo 'Backup directory must not contain quotes or line breaks.' >&2; exit 1;
}
# Unique directories avoid collisions; even partial backups are retained.
snapshot=$(mktemp -d "$backup_dir/discover-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXXXX")
backup="$snapshot/discover.sqlite3"
sqlite3 -batch -bail -readonly "$database" ".timeout 10000" ".backup '$backup'"
result=$(sqlite3 -batch -bail -readonly "$backup" 'PRAGMA quick_check;')
[[ "$result" == ok ]] || { echo 'Backup integrity check failed; snapshot retained.' >&2; exit 1; }
chmod 0600 "$backup"
printf '%s\n' "$backup"
