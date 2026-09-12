#!/usr/bin/env bash
# Source this helper. Respect the operator's selected PATH; fallback locations
# support non-login runuser shells and the documented private server toolchain.
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin:$HOME/toolchains/go1.26.8/bin"
