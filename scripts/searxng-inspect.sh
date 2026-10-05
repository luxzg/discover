#!/usr/bin/env bash
# Read-only inventory; never reads settings contents, credentials or search history.
set -euo pipefail
home=/usr/local/searxng
service=searxng
usage() { echo 'Usage: bash scripts/searxng-inspect.sh [--home /usr/local/searxng] [--service searxng]'; }
while [[ $# -gt 0 ]]; do
  case "$1" in
    --home|--service)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      if [[ $1 == --home ]]; then home=$2; else service=$2; fi
      shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
[[ $home == /* && $service =~ ^[a-zA-Z0-9_-]+$ ]] || { echo 'Expected an absolute home and simple service name.' >&2; exit 2; }
owner=$(systemctl show "$service" --property=User --value)
[[ $owner =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]] || { echo 'Cannot identify a dedicated service user.' >&2; exit 1; }
as_service() {
  if [[ $(id -u) == 0 ]]; then
    runuser -u "$owner" -- "$@"
  elif [[ $(id -un) == "$owner" ]]; then
    "$@"
  else
    echo 'Run from a reviewed administrator-controlled checkout with sudo, or as the service user.' >&2
    return 1
  fi
}
echo
echo '==> Service location (no environment or secret values)'
systemctl show "$service" --property=User --property=WorkingDirectory --property=FragmentPath
echo
echo '==> Installed source revision and date'
as_service git -C "$home/searxng" log -1 --format='%h %ci'
as_service git -C "$home/searxng" status --short
echo
echo '==> Installed Python and dependency consistency'
as_service "$home/searx-venv/bin/python" --version
as_service "$home/searx-venv/bin/python" -m pip check
echo
echo '==> Settings file presence only'
if as_service test -f "$home/searx-settings.yml"; then
  echo 'searx-settings.yml exists (contents were not read)'
else
  echo 'Historical settings path not found; locate SEARXNG_SETTINGS_PATH locally without sharing its secret.'
fi
echo
echo '==> Inventory complete; no service, source, packages or settings changed'
