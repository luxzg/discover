#!/usr/bin/env bash
# Privileged orchestration for the inventoried source/pyenv installation only.
# Invoke from an administrator-owned checkout, NEVER the Discover service checkout.
set -euo pipefail
home=/usr/local/searxng
service=searxng
revision=master
mode=check
rollback=
search_check=0
backup_root=/var/backups/searxng
usage() {
  echo 'Usage: sudo bash scripts/searxng-update.sh [--check|--apply|--rollback SNAPSHOT] [--home PATH] [--service NAME] [--revision FULL_COMMIT] [--search-check]'
  echo 'Default --check is read-only/offline. --apply downloads Git/Python packages, retains backups, stops/swaps/restarts SearXNG. Search checks are opt-in.'
}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check|--apply) mode=${1#--}; shift ;;
    --search-check) search_check=1; shift ;;
    --home|--service|--revision|--rollback)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      case "$1" in
        --home) home=$2 ;;
        --service) service=$2 ;;
        --revision) revision=$2 ;;
        --rollback) mode=rollback; rollback=$2 ;;
      esac
      shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
die() { echo "$*" >&2; exit 1; }
step() { echo; echo "==> $*"; }
[[ $(id -u) == 0 ]] || die 'Run from a reviewed administrator-owned checkout using sudo.'
[[ $home =~ ^/[a-zA-Z0-9_./-]+$ && $home != / && $home == "$(realpath -m -- "$home")" ]] || die 'Expected a canonical absolute home path.'
[[ $service =~ ^[a-zA-Z0-9_-]+$ ]] || die 'Invalid service name.'
[[ $revision == master || $revision =~ ^[a-f0-9]{40}$ ]] || die 'Revision must be a full lowercase Git commit hash.'
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
# All privileged instructions/helpers must come from a non-service-writable
# administrator tree. Check ancestors too, not just the script's own mode.
controlled() {
  local p=$1 uid permissions
  [[ ! -L $p ]] || die 'Administrator input must not be a symlink.'
  while :; do
    [[ ! -L $p ]] || die 'Administrator input ancestor must not be a symlink.'
    uid=$(stat -c %u -- "$p")
    permissions=$(stat -c %a -- "$p")
    [[ $uid == 0 || $uid == "${SUDO_UID:-0}" ]] || die 'Use an administrator-owned checkout, not a service-owned one.'
    (( (8#$permissions & 8#022) == 0 )) || die 'Administrator input/ancestor is group/other writable.'
    [[ $p != / ]] || break
    p=$(dirname -- "$p")
  done
}
controlled "$script_dir/searxng-update.sh"
controlled "$script_dir/searxng-worker.sh"
controlled "$script_dir/searxng-check.py"
# The administrator's home can be private. Copy reviewed helpers into a
# root-owned disposable directory; do not relax anyone's home permissions.
helpers=$(mktemp -d /run/searxng-update.XXXXXXXX)
chmod 0755 "$helpers"
install -m 0644 "$script_dir/searxng-worker.sh" "$helpers/worker.sh"
install -m 0644 "$script_dir/searxng-check.py" "$helpers/check.py"
cleanup() {
  rm -f -- "$helpers/worker.sh" "$helpers/check.py"
  rmdir -- "$helpers"
}
trap cleanup EXIT
if [[ $mode == rollback ]]; then
  [[ $rollback =~ ^$backup_root/release-[a-zA-Z0-9]+$ ]] || die 'Expected a snapshot printed by this updater.'
  controlled "$rollback/manifest"
  mapfile -t state < "$rollback/manifest"
  [[ ${#state[@]} == 4 ]] || die 'Invalid recovery manifest.'
  home=${state[0]}; service=${state[1]}; release=${state[3]}
  [[ $home =~ ^/[a-zA-Z0-9_./-]+$ && $home == "$(realpath -m -- "$home")" && $service =~ ^[a-zA-Z0-9_-]+$ && $release == "$home"/update-* ]] || die 'Invalid recovery paths.'
fi
step 'Checking the existing dedicated-user service contract (no secret values printed)'
owner=$(systemctl show "$service" --property=User --value)
[[ $owner =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ && $owner != root && $(id -u "$owner") != 0 && $(id -u "$owner") != "${SUDO_UID:-0}" ]] || die 'Expected a separate non-root service user.'
[[ $(systemctl show "$service" --property=WorkingDirectory --value) == "$home/searxng" ]] || die 'WorkingDirectory differs from the inventoried layout.'
launch=$(systemctl show "$service" --property=ExecStart --value)
[[ $launch == *"argv[]=$home/searx-venv/bin/python -m searx.webapp ;"* ]] || die 'Unsupported ExecStart; review the launch contract locally before adapting this helper.'
[[ $(systemctl show "$service" --property=Environment --value) == "SEARXNG_SETTINGS_PATH=$home/searx-settings.yml" ]] || die 'Unsupported environment overrides; review locally (do not paste secret values).'
[[ -z $(systemctl show "$service" --property=EnvironmentFiles --value) ]] || die 'Environment files require a separate compatibility review.'
unit=$(systemctl show "$service" --property=FragmentPath --value)
[[ $unit == /etc/systemd/system/"$service".service && -f $unit && ! -L $unit ]] || die 'Unexpected unit path.'
controlled "$unit"
[[ -d $home && ! -L $home && -f $home/searx-settings.yml && ! -L $home/searx-settings.yml ]] || die 'Unexpected home/settings paths.'
[[ $(stat -c %u -- "$home") == "$(id -u "$owner")" ]] || die 'Home must belong to the dedicated service user.'
worker() {
  # Empty environment avoids caller proxy/Python/pip overrides; no package or
  # application code is executed as root.
  local limit=120
  if [[ $1 == prepare ]]; then limit=1800; fi
  timeout --kill-after=10s "$limit" runuser -u "$owner" -- env -i HOME="$home" USER="$owner" PATH=/usr/bin:/bin LANG=C.UTF-8 \
    bash "$helpers/worker.sh" "$1" "$home" "${release:-}" "$revision" "$helpers/check.py"
}
if [[ $mode == check ]]; then
  worker check
  step 'Preflight complete; only disposable helper copies, no downloads/backups/service changes'
  exit 0
fi
umask 077
# Fixed root-owned backup/lock locations; no service-owned script is sourced.
if [[ ! -d $backup_root ]]; then mkdir -- "$backup_root"; fi
controlled "$backup_root"
[[ $(stat -c %u -- "$backup_root") == 0 ]] || die 'Backup directory must be root-owned.'
chmod 0711 "$backup_root"
exec 9>"$backup_root/update.lock"
flock -n 9 || die 'Another SearXNG update is running.'
if [[ $mode == rollback ]]; then
  [[ $owner == "${state[2]}" ]] || die 'Service owner differs from recovery manifest.'
  [[ -L $home/searxng && $(readlink -- "$home/searxng") == "$release/source" && -L $home/searx-venv && $(readlink -- "$home/searx-venv") == "$release/venv" ]] || die 'Not the active release; rollback only the most recent update first.'
  worker previous-check
  step 'Rolling back source/environment only; current settings/unit remain unchanged'
  systemctl stop "$service"
  worker restore
  systemctl start "$service"
  worker health
  systemctl is-active --quiet "$service"
  step 'Rollback complete; all retained releases/backups remain on disk'
  exit 0
fi
systemctl is-active --quiet "$service" || die 'Expected an active service; investigate before updating.'
worker check
available=$(df -Pk "$home" | awk 'NR==2 {print $4}')
[[ $available =~ ^[0-9]+$ && $available -ge 2097152 ]] || die 'At least 2 GiB free space is required for candidate and retained files.'
step 'Retaining restricted settings/unit snapshots (neither will be overwritten)'
snapshot=$(mktemp -d "$backup_root/release-XXXXXXXX")
chmod 0711 "$snapshot"
runuser -u "$owner" -- cat "$home/searx-settings.yml" > "$snapshot/settings.yml"
cp -- "$unit" "$snapshot/service.unit"
settings_hash=$(sha256sum "$snapshot/settings.yml" | cut -d ' ' -f 1)
unit_hash=$(sha256sum "$snapshot/service.unit" | cut -d ' ' -f 1)
release=$(runuser -u "$owner" -- mktemp -d "$home/update-XXXXXXXX")
printf '%s\n' "$home" "$service" "$owner" "$release" > "$snapshot/manifest"
echo "Snapshot retained at: $snapshot"
echo "Candidate/previous installation retained at: $release"
worker prepare
# Detect concurrent operator edits before service stop. Do not restore settings.
[[ $(runuser -u "$owner" -- sha256sum "$home/searx-settings.yml" | cut -d ' ' -f 1) == "$settings_hash" && $(sha256sum "$unit" | cut -d ' ' -f 1) == "$unit_hash" ]] || die 'Settings/unit changed during preparation; candidate retained, service untouched.'
stopped=0
recover() {
  code=$?
  trap - EXIT
  if [[ $code != 0 && $stopped == 1 ]]; then
    step 'Update failed; attempting previous source/environment recovery (no settings restore)'
    if systemctl stop "$service" && worker restore && systemctl start "$service" && worker health && systemctl is-active --quiet "$service"; then
      echo 'Previous service recovered.' >&2
    else
      echo 'Automatic recovery failed; retained paths need administrator inspection.' >&2
    fi
  fi
  cleanup
  exit "$code"
}
trap recover EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
step 'Stopping SearXNG briefly; switching candidate without changing settings/unit'
stopped=1
systemctl stop "$service"
worker activate
systemctl start "$service"
worker health
systemctl is-active --quiet "$service"
stopped=0
step 'Installed revision and retained recovery paths'
runuser -u "$owner" -- git -C "$home/searxng" log -1 --format='%h %ci'
echo "Snapshot: $snapshot"
echo "Retained source/environment: $release"
echo "Rollback: sudo bash scripts/searxng-update.sh --rollback $snapshot"
if [[ $search_check == 1 ]]; then
  step 'Opt-in two spaced upstream searches; warnings do not trigger rollback'
  worker search
fi
step 'Update complete; upstream blocking may persist; no backups were deleted'
