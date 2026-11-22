#!/usr/bin/env python3
"""Read-only candidate pairs from a reader's recorded opens AND exposures."""
import argparse
from collections import Counter, defaultdict
from contextlib import closing
from datetime import datetime, timedelta, timezone
from heapq import nlargest, heappush, heapreplace
from math import log
from pathlib import Path
import re
import sqlite3
import unicodedata

CAP = 3000
KINDS = ("shown / shown", "opened / shown", "opened / opened")
STOP = set("""a an and are as at be by for from has have how in into is it its
new of on or out says that the their these this to what when why will with your
after about now could just here's than more over up you""".split())


def printable(value):
    # Untrusted publisher text must not insert terminal controls or fake rows.
    return " ".join("".join(" " if unicodedata.category(c).startswith("C") else c
                            for c in value).split())


def recent(db_path, start, end):
    # Do not use immutable=1: Discover's newest committed reads may be in WAL.
    uri = db_path.resolve(strict=True).as_uri() + "?mode=ro"
    with closing(sqlite3.connect(uri, uri=True, timeout=3)) as db:
        db.execute("PRAGMA query_only=ON")
        db.row_factory = sqlite3.Row
        return db.execute("""
          WITH activity AS (
            SELECT id,substr(title,1,480) AS title,story_key,event_id,
              CASE WHEN julianday(read_at)>julianday(:start)
                    AND julianday(read_at)<=julianday(:end) THEN read_at END AS opened,
              CASE WHEN julianday(last_seen_at)>julianday(:start)
                    AND julianday(last_seen_at)<=julianday(:end) THEN last_seen_at END AS shown
            FROM articles
          )
          SELECT *,COUNT(*) OVER() AS total FROM activity
          WHERE opened IS NOT NULL OR shown IS NOT NULL
          ORDER BY MAX(COALESCE(julianday(opened),0),COALESCE(julianday(shown),0)) DESC,id DESC
          LIMIT :cap
        """, {"start": start.isoformat(), "end": end.isoformat(), "cap": CAP}).fetchall()


def candidate_pairs(rows, limit):
    terms = [re.findall(r"[^\W_]+", r["title"].casefold())[:64] for r in rows]
    words = [set(w for w in line if w not in STOP and len(w) > 1) for line in terms]
    phrases = [set(zip(line, line[1:])) for line in terms]
    df = Counter(w for ws in words for w in ws)
    weight = {w: 1 + log((1 + len(rows)) / (1 + n)) for w, n in df.items()}
    sizes = [sum(weight[w] for w in sorted(ws)) for ws in words]
    postings = defaultdict(list)
    best = {k: [] for k in KINDS}
    counts = Counter()
    for i, ws in enumerate(words):
        # Stop very common terms from generating all-pairs work. They can still
        # corroborate pairs retrieved by a more distinctive word or model.
        keys = sorted((w for w in ws if df[w] <= 128), key=lambda w: (df[w], w))
        candidates = {j for w in keys for j in postings[w]}
        for j in candidates:
            a, b = rows[i], rows[j]
            if ((a["story_key"] and a["story_key"] == b["story_key"]) or
                    (a["event_id"] is not None and a["event_id"] == b["event_id"])):
                continue
            shared = ws & words[j]
            if len(shared) < 2:
                continue
            has_model = any(any(c.isdigit() for c in w) and
                            (any(c.isalpha() for c in w) or len(w) >= 3) for w in shared)
            phrase = any(x in shared and y in shared for x, y in phrases[i] & phrases[j])
            overlap = sum(weight[w] for w in sorted(shared)) / min(sizes[i], sizes[j])
            if overlap < .24 or not (phrase or has_model or len(shared) >= 3):
                continue
            kind = KINDS[int(bool(a["opened"])) + int(bool(b["opened"]))]
            counts[kind] += 1
            score = overlap + .13 * phrase + .08 * has_model + .02 * min(5, len(shared))
            item = (score, -i, -j)
            heap = best[kind]
            if len(heap) < limit * 40:
                heappush(heap, item)
            elif item > heap[0]:
                heapreplace(heap, item)
        for w in keys:
            postings[w].append(i)
    for kind in KINDS:
        used = set()
        selected = []
        for _, ni, nj in sorted(best[kind], reverse=True):
            i, j = -ni, -nj
            if i in used or j in used:
                continue
            evidence = nlargest(6, words[i] & words[j], key=lambda w: (weight[w], w))
            selected.append((i, j, evidence))
            used.update((i, j))
            if len(selected) == limit:
                break
        yield kind, counts[kind], selected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", required=True, type=Path, help="existing Discover SQLite file; never created")
    parser.add_argument("--days", type=int, default=7, choices=range(1, 32), metavar="1..31")
    parser.add_argument("--pairs", type=int, default=5, choices=range(1, 21), metavar="1..20", help="max headline pairs per section")
    parser.add_argument("--until", help="optional UTC window endpoint, e.g. 2026-11-22T19:00:00Z")
    parser.add_argument("--list-headlines", action="store_true", help="also print all selected history; private, potentially long")
    args = parser.parse_args()
    end = datetime.now(timezone.utc).replace(microsecond=0)
    if args.until:
        try:
            end = datetime.strptime(args.until, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
        except ValueError:
            parser.error("--until must be YYYY-MM-DDTHH:MM:SSZ")
    start = end-timedelta(days=args.days)
    try:
        rows = recent(args.database, start, end)
    except (OSError, sqlite3.Error) as exc:
        parser.exit(1, f"Cannot read Discover history: {exc}\n")
    total = rows[0]["total"] if rows else 0
    opened = sum(bool(r["opened"]) for r in rows)
    print(f"Recorded activity (UTC): {start:%Y-%m-%dT%H:%M:%SZ} to {end:%Y-%m-%dT%H:%M:%SZ}")
    print(f"Analyzed {len(rows)} of {total} articles: {opened} recorded opens, {len(rows)-opened} shown without a recorded open in this window.")
    if total > CAP:
        print(f"Truncated to the latest {CAP} by activity. Use shorter windows with --until to investigate the rest.")
    print("Shown includes Load Next and positive actions; not literal viewport tracking.")
    print("Possible misses only: same exact-story key or assigned event excluded. Shared subject is not proof of the same event.")
    for kind, count, pairs in candidate_pairs(rows, args.pairs):
        print(f"\n{kind}: {count} candidate pairs; showing {len(pairs)}")
        for i, j, shared in pairs:
            for row in (rows[i], rows[j]):
                action = "opened" if row["opened"] else "shown"
                print(f"  #{row['id']} {action} {printable(row['opened'] or row['shown'])} | {printable(row['title'])}")
            print("  Shared words:", ", ".join(shared))
    if args.list_headlines:
        print("\nAll selected headlines, most recent activity first:")
        for row in rows:
            print(f"  #{row['id']} {'opened' if row['opened'] else 'shown'} | {printable(row['title'])}")


if __name__ == "__main__":
    main()
