"""Synthetic exposure history, never an operator database."""
from datetime import datetime, timedelta, timezone
from contextlib import redirect_stdout, redirect_stderr, closing
import importlib.util
import io
from pathlib import Path
import sqlite3
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("coverage_review", Path(__file__).parents[1] / "review-coverage.py")
review = importlib.util.module_from_spec(spec)
spec.loader.exec_module(review)


class CoverageReviewTests(unittest.TestCase):
    def test_product_scope_and_measurements(self):
        cases = [
            (False, "system launch",
             "River Works brings sixth generation Lumen Prism processors to market at rack scale",
             "Sixth generation Lumen Prism now available from Lumen"),
            (False, "different launch subjects",
             "Orchid launches Zephyr Wave mini PC for coastal labs",
             "Juniper launches Zephyr Wave mini PC for coastal labs"),
            (False, "different reviewed models",
             "Orchid Prism T16 review: Zephyr Wave workstation",
             "Orchid Prism GR1X review: Zephyr Wave workstation"),
            (False, "different coverage scope",
             "Orchid Prism mobile and desktop details emerge with 28 cores",
             "Orchid Prism-S engineering sample spotted with 28 cores"),
            (True, "clock",
             "Orchid Prism-H mobile processor specs reveal 4.8 GHz boost",
             "Orchid Prism H leak reveals 16 cores with 4800MHz boost"),
            (False, "conflicting clock",
             "Orchid Prism-H mobile processor specs reveal 4.8 GHz boost",
             "Orchid Prism-H mobile processor specs reveal 4.6 GHz boost"),
            (False, "variant",
             "Orchid Prism-H mobile processor specs reveal 4.8 GHz boost",
             "Orchid Prism-S mobile processor specs reveal 4.8 GHz boost"),
            (True, "availability",
             "Orchid launches Zephyr Wave mini PC for coastal labs",
             "Zephyr Wave mini PC from Orchid now available for coastal labs"),
        ]
        for expected, reason, a, b in cases:
            for x, y in ((a, b), (b, a)):
                with self.subTest(a=x, b=y):
                    actual, why = review.same_event(review.claim(x), review.claim(y))
                    self.assertEqual(actual, expected, why)
                    self.assertIn(reason, why)

    def test_strict_claim_decisions(self):
        cases = [
            (True, "Orchid Lira generator reportedly delayed past Polaris 2030",
             "Polaris 2030: Lira generator from Orchid reportedly postponed"),
            (True, "Nimbus quartz reactor has 28,000 photonic cores",
             "Helios reveals Nimbus quartz reactor packing 28000+ photonic cores"),
            (False, "Orchid Lira generator reportedly delayed past Polaris 2030",
             "Orchid confirms Lira generator delayed past Polaris 2030"),
            (False, "Orchid unveils Lira 7 tidal generator at summit",
             "Orchid unveils Lira 8 tidal generator at summit"),
            (False, "Orchid unveils Lira quartz reactor with 420 photonic cores",
             "Orchid unveils Lira quartz reactor with 840 photonic cores"),
            (False, "Report on tablet prices and subsidies", "Tablet prices in Eastern Europe"),
            (False, "Quote of the day by wind pioneer Ada: turbines keep turning",
             "Quote of the day by wind pioneer Ada: the ..."),
        ]
        for expected, a, b in cases:
            with self.subTest(a=a,b=b):
                self.assertEqual(review.same_event(review.claim(a), review.claim(b))[0], expected)
                self.assertEqual(review.same_event(review.claim(b), review.claim(a))[0], expected)

    def test_label_replay_and_fresh_article_holdout(self):
        output = io.StringIO()
        with redirect_stdout(output):
            review.replay(io.StringIO("different\t22\tRiver rises\t11\tBattery test\n"))
        self.assertIn("rejected 1 of 1", output.getvalue())
        self.assertIn(review.shortlist_id([(11,22)]), output.getvalue())
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "test.db"
            when = "2030-01-07T00:00:00Z"
            with closing(sqlite3.connect(path)) as db:
                db.execute("CREATE TABLE articles(id INTEGER PRIMARY KEY,title,story_key,event_id,read_at,last_seen_at)")
                db.executemany("INSERT INTO articles VALUES(?,?,?,NULL,NULL,?)",
                               [(i, f"Orchid launches tidal quartz generator | dispatch {chr(96+i)}", str(i), when)
                                for i in range(1,25)])
                db.commit()
            end = datetime(2030,1,7,tzinfo=timezone.utc)
            rows = review.recent(path,end-timedelta(days=7),end)
            broad = list(review.candidate_pairs(rows,5))
            pairs = [(rows[i]["id"],rows[j]["id"]) for _,_,selected in broad for i,j,_ in selected]
            # The same candidate generator and thresholds are used in the CLI.
            args = ["review","--database",str(path),"--until",when,"--holdout",review.shortlist_id(pairs)]
            with patch.object(review.sys,"argv",args), redirect_stdout(io.StringIO()) as out:
                review.main()
            self.assertIn("excluded all 10 articles",out.getvalue())
            self.assertIn("Event evidence:",out.getvalue())
            for i,j in pairs:
                for article in (i,j):
                    self.assertNotIn(f"#{article} shown",out.getvalue())
            args[-1]="bad-fingerprint"
            with patch.object(review.sys,"argv",args),redirect_stderr(io.StringIO()),redirect_stdout(io.StringIO()),self.assertRaises(SystemExit):
                review.main()

    def test_passed_and_opened_from_live_wal(self):
        end = datetime(2026, 11, 22, 12, tzinfo=timezone.utc)
        start = end - timedelta(days=7)
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "history ?#.db"
            writer = sqlite3.connect(path)
            self.addCleanup(writer.close)
            writer.execute("PRAGMA journal_mode=WAL")
            writer.execute("CREATE TABLE articles(id INTEGER PRIMARY KEY,title,story_key,event_id,read_at,last_seen_at)")
            def insert(i, title, opened, shown):
                writer.execute("INSERT INTO articles VALUES(?,?,?,NULL,?,?)", (i,title,str(i),opened,shown))
            for i in range(1, 5):
                insert(i, f"Quasar 7090 headlines from outlet {i}", end.isoformat() if i > 2 else None, end.isoformat())
            insert(5, "Only discovered, never shown", None, None)
            insert(6, "Before window", None, start.isoformat())
            insert(7, "After window", None, (end + timedelta(seconds=1)).isoformat())
            # Old first open but a new recorded exposure: don't infer a recent click.
            insert(8, "Seen again", start.isoformat(), end.isoformat())
            writer.commit()
            before = writer.execute("PRAGMA data_version").fetchone()
            rows = review.recent(path,start,end)
            self.assertEqual({r["id"] for r in rows}, {1,2,3,4,8})
            self.assertEqual(sum(bool(r["opened"]) for r in rows),2)
            kinds = {k:n for k,n,_ in review.candidate_pairs(rows,5)}
            self.assertEqual(kinds, {"shown / shown":1,"opened / shown":4,"opened / opened":1})
            self.assertEqual(before,writer.execute("PRAGMA data_version").fetchone())
            self.assertFalse((Path(tmp)/"missing.db").exists())
            with self.assertRaises(FileNotFoundError):
                review.recent(Path(tmp)/"missing.db",start,end)
            self.assertFalse((Path(tmp)/"missing.db").exists())

    def test_excludes_existing_assignments_and_sanitizes_terminal(self):
        base = {"title":"Cerium 9000 capsule touches down","story_key":"x","event_id":4,"opened":None}
        rows = [base,dict(base,story_key="y"),dict(base,event_id=None)]
        pairs = list(review.candidate_pairs(rows,2))[0]
        self.assertEqual(pairs[1],1)
        self.assertEqual(set(pairs[2][0][:2]),{1,2})
        self.assertEqual(review.printable("A\x1b\nB\u202eC"), "A B C")
