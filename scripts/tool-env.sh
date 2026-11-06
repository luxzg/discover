#!/usr/bin/env bash
# Source this helper. Prefer the operator's newest installed stable shared SDK.
# Ignore prereleases and never install tools or raise the module minimum here.
_discover_sdks=()
for _discover_sdk in "$HOME"/sdk/go*; do
  _discover_name=${_discover_sdk##*/}
  if [[ $_discover_name =~ ^go[0-9]+\.[0-9]+(\.[0-9]+)?$ && -x "$_discover_sdk/bin/go" ]]; then
    _discover_sdks+=("$_discover_name")
  fi
done
if [[ ${#_discover_sdks[@]} -gt 0 ]]; then
  mapfile -t _discover_sdks < <(printf '%s\n' "${_discover_sdks[@]}" | sort -V)
  _discover_selected="$HOME/sdk/${_discover_sdks[-1]}/bin"
  if [[ "$PATH" != "$_discover_selected:"* ]]; then export PATH="$_discover_selected:$PATH"; fi
  export GOTOOLCHAIN=local
fi
unset _discover_sdk _discover_name _discover_sdks _discover_selected
# Legacy server installations remain usable when no shared SDK is installed.
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin:$HOME/toolchains/go1.26.8/bin"
