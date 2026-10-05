"""Bounded SearXNG preflight/health checks, always run as the service user.

This helper intentionally suppresses exception text: parser/import failures can
include settings values. It does not rewrite settings or search history.
"""
import json
import contextlib
import io
import importlib.util
import logging
import os
import sys
import time
import urllib.error
import urllib.request


def check_settings(settings):
    server = settings["server"]
    if server["bind_address"] != "127.0.0.1":
        raise ValueError("listener is not loopback")
    if server["port"] != 8888 or server.get("public_instance") or settings["general"].get("debug"):
        raise ValueError("unexpected listener or debug/public mode")
    if "json" not in settings["search"]["formats"]:
        raise ValueError("JSON format is disabled")
    if not server.get("secret_key") or server["secret_key"] == "ultrasecretkey":
        raise ValueError("missing or default secret")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def get_json(path, timeout=20):
    # This helper is for the known local listener, never a caller-supplied URL.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with opener.open("http://127.0.0.1:8888" + path, timeout=timeout) as response:
        body = response.read((4 << 20) + 1)
    if len(body) > 4 << 20:
        raise ValueError("oversized response")
    value = json.loads(body)
    if not isinstance(value, dict):
        raise ValueError("response is not an object")
    return value


def main():
    mode = sys.argv[1]
    if mode == "settings":
        # The shell passes the candidate source explicitly, avoiding the old
        # checkout's editable-install path and working-directory import precedence.
        sys.path.insert(0, sys.argv[2])
        # Upstream imports/schema errors can print values before raising.
        logging.disable(logging.CRITICAL)
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            from searx import settings

            check_settings(settings)
            # Importing webapp initializes caches and engine networking upstream.
            # Check resolution/syntax only here; run the actual app after swap.
            spec = importlib.util.find_spec("searx.webapp")
            if spec is None or spec.origin is None:
                raise ValueError("missing application module")
            with open(spec.origin, encoding="utf-8") as file:
                compile(file.read(), spec.origin, "exec")

        print("Settings/module preflight passed (secret values not displayed).")
        import yaml

        with open(os.environ["SEARXNG_SETTINGS_PATH"], encoding="utf-8") as file:
            overrides = yaml.safe_load(file)
        if not overrides.get("use_default_settings"):
            print("WARNING: full legacy settings retained; engine overrides may remain stale.")
    elif mode == "health":
        for attempt in range(15):
            try:
                value = get_json("/config", timeout=2)
                engines = value.get("engines")
                if not isinstance(engines, list) or not engines:
                    raise ValueError("missing engine configuration")
                for category in ("news", "general"):
                    if not any(isinstance(engine, dict) and engine.get("enabled") and category in engine.get("categories", []) for engine in engines):
                        raise ValueError("no enabled engine for required category")
                print("Local JSON /config health passed; no upstream searches issued.")
                return
            except (OSError, ValueError, urllib.error.HTTPError):
                if attempt == 14:
                    raise
                time.sleep(2)
    elif mode == "search":
        for index, category in enumerate(("news", "general")):
            if index:
                time.sleep(5)
            value = get_json("/search?q=intel%20cpu&format=json&time_range=day&pageno=1&categories=" + category)
            if not isinstance(value.get("results"), list):
                raise ValueError("missing results array")
            errors = value.get("unresponsive_engines", [])
            if not isinstance(errors, list):
                raise ValueError("invalid engine diagnostics")
            print(f"{category}: results={len(value['results'])}, engine_warnings={len(errors)}")
        print("Search API contract passed; empty results/engine warnings do not prove engine recovery.")
    else:
        raise ValueError("unknown mode")


if __name__ == "__main__":
    try:
        main()
    except (Exception, SystemExit):
        print("SearXNG check failed; inspect private local logs/settings. Values suppressed.", file=sys.stderr)
        sys.exit(1)
