"""Synthetic checker contracts; no real SearXNG, ports, settings or requests."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("checker", Path(__file__).parents[1] / "searxng-check.py")
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class CheckerTests(unittest.TestCase):
    def test_health_requires_enabled_news_and_general(self):
        valid = {"engines": [{"enabled": True, "categories": ["news", "general"]}]}
        with patch.object(checker.sys, "argv", ["checker", "health"]), patch.object(checker, "get_json", return_value=valid) as fetch, contextlib.redirect_stdout(io.StringIO()):
            checker.main()
            fetch.assert_called_once_with("/config", timeout=2)
        for value in ({"engines": []}, {"engines": [{"enabled": False, "categories": ["news", "general"]}]}, {"engines": [{"enabled": True, "categories": ["general"]}]}):
            with patch.object(checker.sys, "argv", ["checker", "health"]), patch.object(checker, "get_json", return_value=value) as fetch, patch.object(checker.time, "sleep") as sleep:
                with self.assertRaises(ValueError):
                    checker.main()
                self.assertEqual(fetch.call_count, 15)
                self.assertEqual(sleep.call_count, 14)

    def test_search_contract_accepts_empty_with_warnings_without_printing_bodies(self):
        value = {"results": [], "unresponsive_engines": [["fixture", "synthetic-private-message"]]}
        output = io.StringIO()
        with patch.object(checker.sys, "argv", ["checker", "search"]), patch.object(checker, "get_json", return_value=value) as fetch, patch.object(checker.time, "sleep") as sleep, contextlib.redirect_stdout(output):
            checker.main()
            self.assertEqual(fetch.call_count, 2)
            sleep.assert_called_once_with(5)
            for call in fetch.call_args_list:
                self.assertIn("time_range=day", call.args[0])
                self.assertIn("pageno=1", call.args[0])
        self.assertIn("engine_warnings=1", output.getvalue())
        self.assertNotIn("synthetic-private-message", output.getvalue())
        with patch.object(checker.sys, "argv", ["checker", "search"]), patch.object(checker, "get_json", return_value={"results": None}):
            with self.assertRaises(ValueError):
                checker.main()

    def test_transport_is_fixed_loopback_bounded_and_rejects_redirects(self):
        class Response:
            body = b'{"engines":[]}'
            def __enter__(self):
                return self
            def __exit__(self, *_):
                return False
            def read(self, maximum):
                self.maximum = maximum
                return self.body[:maximum]

        response = Response()
        class Opener:
            def open(self, url, timeout):
                self.url, self.timeout = url, timeout
                return response

        opener = Opener()
        with patch.object(checker.urllib.request, "build_opener", return_value=opener) as build:
            self.assertEqual(checker.get_json("/config"), {"engines": []})
            self.assertEqual(opener.url, "http://127.0.0.1:8888/config")
            self.assertEqual(response.maximum, (4 << 20) + 1)
            self.assertEqual(build.call_args.args[0].proxies, {})
            response.body = b'x' * ((4 << 20) + 1)
            with self.assertRaises(ValueError):
                checker.get_json("/config")
            response.body = json.dumps([]).encode()
            with self.assertRaises(ValueError):
                checker.get_json("/config")
        self.assertIsNone(checker.NoRedirect().redirect_request(None, None, 302, "", {}, "https://example.test"))


if __name__ == "__main__":
    unittest.main()
