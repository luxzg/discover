"""Synthetic exposure history, never an operator database."""
from datetime import datetime, timedelta, timezone
import importlib.util
from pathlib import Path
import sqlite3
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("coverage_review", Path(__file__).parents[1] / "review-coverage.py")
review = importlib.util.module_from_spec(spec)
spec.loader.exec_module(review)


class CoverageReviewTests(unittest.TestCase):
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
