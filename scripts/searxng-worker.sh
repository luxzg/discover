#!/usr/bin/env bash
# Unprivileged operations only. Called using an administrator-controlled copy.
set -euo pipefail
action=$1
home=$2
release=$3
revision=$4
checker=$5
[[ $(id -u) != 0 ]] || { echo 'Never run the SearXNG worker as root.' >&2; exit 1; }
export HOME="$home" PIP_CONFIG_FILE=/dev/null PIP_INDEX_URL=https://pypi.org/simple
export PIP_DISABLE_PIP_VERSION_CHECK=1 PYTHONDONTWRITEBYTECODE=1
export GIT_OPTIONAL_LOCKS=0
export SEARXNG_SETTINGS_PATH="$home/searx-settings.yml"
unset PYTHONPATH PIP_EXTRA_INDEX_URL
step() { echo; echo "==> SearXNG: $*"; }
case "$action" in
  check)
    [[ -z $(git -C "$home/searxng" status --porcelain) ]] || { echo 'SearXNG source is dirty; preserve/review changes before updating.' >&2; exit 1; }
    "$home/searx-venv/bin/python" --version
    "$home/searx-venv/bin/python" -m pip check
    "$home/searx-venv/bin/python" "$checker" settings "$home/searxng"
    git -C "$home/searxng" log -1 --format='Installed source: %h %ci'
    ;;
  prepare)
    step 'Downloading candidate source; installed source/environment stay untouched'
    git clone --depth 1 --branch master https://github.com/searxng/searxng.git "$release/source"
    if [[ $revision != master ]]; then
      git -C "$release/source" fetch --depth 1 origin "$revision"
      git -C "$release/source" checkout --detach "$revision"
    fi
    git -C "$release/source" rev-parse HEAD > "$release/revision"
    step 'Building candidate environment with the existing base Python (no OS/pyenv upgrade)'
    "$home/searx-venv/bin/python" -m venv "$release/venv"
    "$release/venv/bin/python" -m pip install --upgrade pip setuptools wheel pyyaml msgspec typing_extensions pybind11
    "$release/venv/bin/python" -m pip install --no-build-isolation -e "$release/source"
    "$release/venv/bin/python" -m pip check
    step 'Checking candidate with unchanged private settings before stopping anything'
    "$release/venv/bin/python" "$checker" settings "$release/source"
    "$release/venv/bin/python" -m pip freeze > "$release/packages.txt"
    ;;
  activate)
    step 'Retaining previous paths and linking the checked candidate'
    [[ ! -e $release/previous-source && ! -L $release/previous-source && ! -e $release/previous-venv && ! -L $release/previous-venv ]] || exit 1
    mv -T -- "$home/searxng" "$release/previous-source"
    mv -T -- "$home/searx-venv" "$release/previous-venv"
    ln -s -- "$release/source" "$home/searxng"
    ln -s -- "$release/venv" "$home/searx-venv"
    ;;
  previous-check)
    [[ -e $release/previous-source && -e $release/previous-venv/bin/python ]] || { echo 'Previous installation unavailable.' >&2; exit 1; }
    "$release/previous-venv/bin/python" "$checker" settings "$release/previous-source"
    ;;
  restore)
    step 'Restoring only previous source/environment paths; settings are NOT restored'
    for pair in 'searxng:previous-source:source' 'searx-venv:previous-venv:venv'; do
      IFS=: read -r live previous candidate <<< "$pair"
      if [[ -e $release/$previous || -L $release/$previous ]]; then
        if [[ -L $home/$live && $(readlink -- "$home/$live") == "$release/$candidate" ]]; then
          unlink -- "$home/$live"
        fi
        [[ ! -e $home/$live && ! -L $home/$live ]] || { echo 'Unexpected live path; refusing overwrite during recovery.' >&2; exit 1; }
        mv -T -- "$release/$previous" "$home/$live"
      fi
    done
    ;;
  health|search)
    "$home/searx-venv/bin/python" "$checker" "$action"
    ;;
  *) echo 'Unknown worker operation.' >&2; exit 2 ;;
esac
