#!/usr/bin/env python3
"""Read-only candidate pairs from a reader's recorded opens AND exposures."""
import argparse
import hashlib
from collections import Counter, defaultdict
from contextlib import closing
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from heapq import nlargest, heappush, heapreplace
from math import log
from pathlib import Path
import re
import sqlite3
import sys
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


EVENTS = {
    "delay": r"\b(delay(?:ed|s)?|postpone[ds]?|slips? (?:past|to|until)|pushed (?:back|to))\b",
    "availability": r"\b(launch(?:es|ed)?|ships?|shipping|out now|now available|bring(?:s)? .{1,65} to)\b",
    "benchmarks": r"\bbenchmarks?\b",
    "improvement": r"\b(improves?|improved|boost(?:s|ed)?|enhance[ds]?)\b",
    "preview": r"\b(teases?|teased|teaser|previews?)\b",
    "recall": r"\brecall(?:s|ed)?\b",
    "review": r"\b(review|reviewed|hands.on)\b",
}
EVENT_PATTERNS = {k: re.compile(v) for k, v in EVENTS.items()}
WEAK = STOP | set("""gpu cpu cpus gpus gpu graphics card chip chips company tech news
models ai next gen desktop desktops prices report reports reportedly claims says first
time quote day scientific launches launch out bring benchmark benchmarks shares nextgen
for is announcement release by review new latest revealed official reveals""".split())
NUM = re.compile(r"\b\d+(?:\.\d+)?\b")
UNIT = re.compile(r"\b(\d+(?:\.\d+)?)\+?\s*(%|(?:[a-z]+\s+)?(?:cores|watts|gigabytes|kilometers)\b|gb\b|tb\b)")
MODEL = re.compile(r"\b([a-z]{1,16})[- ]?(\d{1,6}[a-z]{0,2})\b")
UNCERTAIN = re.compile(r"\b(rumou?rs?|reportedly|unconfirmed|allegedly)\b")
CONFIRMED = re.compile(r"\b(confirms?|confirmed|officially)\b")
NEGATED = re.compile(r"\b(no|not|never|won't|cannot|can't|cancels?|cancelled|denies|denied)\b")
CLOCK = re.compile(r"\b(\d{1,8}(?:\.\d{1,6})?)\s*(ghz|mhz)\b")
BOOST_CLOCK = re.compile(CLOCK.pattern + r"\s+boost\b|\bboost\s+(?:clock|frequency)\b")
LAUNCH_SUBJECT = re.compile(r"^\s*([A-Z][\w-]*)\s+(?i:launches|unveils|debuts)\b")
REVIEW_CODE = re.compile(r"\b[a-z]+\d+[a-z\d]*\b")
VARIANT = re.compile(r"\b([A-Za-z]{2,})[- ]([A-Z])\b")


def product_details(title, text):
    # Use only actors/codes in explicit roles. A shared component, outlet name
    # or unrelated number elsewhere in the headline is not a product identity.
    actor = LAUNCH_SUBJECT.match(title)
    lead, review_word, _ = text.partition(" review")
    variants = defaultdict(set)
    for family, suffix in VARIANT.findall(title):
        if family.casefold() not in WEAK and suffix not in {"A", "I"}:
            variants[family.casefold()].add(suffix.lower())
    clocks = {Decimal(n) * (1000 if unit == "ghz" else 1) for n, unit in CLOCK.findall(text)}
    classes = {kind for kind in ("mobile", "desktop") if re.search(r"\b" + kind + r"\b", text)}
    return {
        "actor": actor[1].casefold() if actor else "",
        "review_codes": set(REVIEW_CODE.findall(lead)) if review_word else set(),
        "variants": variants, "clocks": clocks, "classes": classes,
        "sample": bool(re.search(r"\bengineering sample\b", text)),
    }


def product_conflict(a, b):
    if a["actor"] and b["actor"] and a["actor"] != b["actor"]:
        return "different launch subjects"
    if a["review_codes"] and b["review_codes"] and a["review_codes"].isdisjoint(b["review_codes"]):
        return "different reviewed models"
    for family in a["variants"].keys() & b["variants"].keys():
        if a["variants"][family].isdisjoint(b["variants"][family]):
            return "different product variants"
    if a["clocks"] and b["clocks"] and a["clocks"].isdisjoint(b["clocks"]):
        return "conflicting clock specifications"
    for broad, narrow in ((a, b), (b, a)):
        if len(broad["classes"]) == 2 and len(narrow["classes"]) < 2 and (narrow["classes"] or narrow["sample"]):
            return "different coverage scope: family overview versus specific report"
    return ""


def claim(title):
    text = re.sub(r"(?<=\d),(?=\d{3}\b)", "", title.casefold().replace("’", "'"))
    tokens = re.findall(r"[^\W_]+", text)[:64]
    strong = [w for w in tokens if len(w) > 1 and w not in WEAK]
    def slots(matches, reverse=False):
        out = defaultdict(set)
        for x, y in matches:
            key, value = (y, x) if reverse else (x, y)
            out[key].add(value)
        return out
    # Extract number-bearing identifiers and named numeric milestones from the
    # headlines, not from a product catalog. Avoid incidental "to 30" / "at 5".
    models = [(k, v) for k, v in MODEL.findall(text)
              if k not in STOP | {"past", "until", "than", "over", "under", "around"}]
    return {
        "tokens": tokens, "strong": set(strong),
        "phrases": set(zip(tokens, tokens[1:])),
        "kinds": {k for k, pattern in EVENT_PATTERNS.items()
                  if pattern.search(BOOST_CLOCK.sub(" ", text) if k == "improvement" else text)},
        "product": product_details(title, text),
        "units": slots(UNIT.findall(text), True), "models": slots(models),
        "rumor": bool(UNCERTAIN.search(text)), "confirmed": bool(CONFIRMED.search(text)),
        "negative": bool(NEGATED.search(text)), "numbers": set(NUM.findall(text)),
        "quote": "quote of the day" in text,
        "quote_words": re.findall(r"[^\W_]+", text.partition(":")[2]) if ":" in text else [],
    }


def same_event(a, b):
    """Headline evidence only. No clicks, domains, activity times or article IDs."""
    conflict = product_conflict(a["product"], b["product"])
    if conflict:
        return False, conflict
    if a["quote"] or b["quote"]:
        quoted = (set(a["quote_words"]) & set(b["quote_words"])) - STOP
        if not (a["quote"] and b["quote"] and len(quoted) >= 4):
            return False, "not enough quoted words to identify the quotation"
    if a["negative"] != b["negative"] or (a["rumor"] and b["confirmed"]) or (b["rumor"] and a["confirmed"]):
        return False, "different certainty or polarity"
    for slot in ("units", "models"):
        for key in a[slot].keys() & b[slot].keys():
            if a[slot][key].isdisjoint(b[slot][key]):
                return False, "conflicting " + key
    left, right = a["kinds"], b["kinds"]
    common = left & right
    if left and right and not common:
        return False, "different development or coverage angle"
    strong = a["strong"] & b["strong"]
    pa, pb = a["product"], b["product"]
    same_variant = any(v & pb["variants"].get(k, set()) for k, v in pa["variants"].items())
    if same_variant and pa["clocks"] & pb["clocks"] and len(strong) >= 3:
        return True, "same product variant and clock specification"
    phrases = a["phrases"] & b["phrases"]
    distinctive_phrase = any(x in strong and y in strong for x, y in phrases)
    prefix = 0
    for x, y in zip(a["tokens"], b["tokens"]):
        if x != y:
            break
        prefix += 1
    if prefix >= 8 and len(set(a["tokens"][:prefix]) & strong) >= 3:
        return True, "same long distinctive headline opening"
    if common and len(strong) >= 3 and (distinctive_phrase or len(strong) >= 4 or a["numbers"] & b["numbers"]):
        return True, "shared " + "/".join(sorted(common)) + " claim and subject details"
    quantity = any(values & b["units"].get(unit, set()) for unit, values in a["units"].items())
    if quantity and len(strong - a["numbers"]) >= 3 and distinctive_phrase:
        return True, "same measured specification and distinctive subject"
    if len(strong) >= 5 and len(phrases) >= 3 and distinctive_phrase:
        coverage = len(strong) / max(1, min(len(a["strong"]), len(b["strong"])))
        if coverage >= .8:
            return True, "substantial distinctive wording and phrase agreement"
    return False, "insufficient shared claim; topic overlap alone"


def candidate_pairs(rows, limit, *, strict=False, excluded=frozenset()):
    claims = [claim(r["title"]) for r in rows] if strict else []
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
        if excluded and rows[i]["id"] in excluded:
            continue
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
            if strict and not same_event(claims[i], claims[j])[0]:
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


def shortlist_id(pairs):
    key = ",".join(f"{a}:{b}" for a, b in sorted(tuple(sorted(p)) for p in pairs))
    return hashlib.sha256(key.encode()).hexdigest()[:16]


def replay(stream):
    """Private TSV from stdin; blank line/EOF ends. Do not echo headlines."""
    totals = Counter()
    pairs = []
    for n, line in enumerate(stream, 1):
        if not line.strip():
            break
        if n > 200 or len(line) > 4096:
            raise ValueError("replay limited to 200 pairs, 4096 characters per line")
        fields = line.rstrip("\r\n").split("\t")
        if len(fields) == 5:
            a, b = int(fields[1]), int(fields[3])
            if min(a, b) < 1 or a == b:
                raise ValueError(f"line {n}: expected distinct positive IDs")
            pairs.append((a, b))
            fields = [fields[0], fields[2], fields[4]]
        if len(fields) != 3 or fields[0] not in {"same", "different"} or not all(fields[1:]):
            raise ValueError(f"line {n}: expected same/different, title, title (tab-separated)")
        actual, reason = same_event(claim(fields[1]), claim(fields[2]))
        expected = fields[0] == "same"
        totals[expected, actual] += 1
        print(f"{n}: {fields[0]} -> {'match' if actual else 'separate'} ({reason})")
    if not totals:
        raise ValueError("no labeled pairs supplied")
    print(f"Recovered {totals[True, True]} of {totals[True, True]+totals[True, False]} labeled repeats; "
          f"rejected {totals[False, False]} of {totals[False, False]+totals[False, True]} labeled nonmatches.")
    if len(pairs) == sum(totals.values()):
        print("Reviewed shortlist ID:", shortlist_id(pairs))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--database", type=Path, help="existing Discover SQLite file; never created")
    source.add_argument("--replay-labels", action="store_true", help="evaluate labeled tab-separated pairs on stdin, without a DB")
    parser.add_argument("--holdout", metavar="SHORTLIST_ID", help="strict preview excluding every article in a verified previous broad shortlist")
    parser.add_argument("--strict", action="store_true", help="apply the experimental headline-only decision to broad candidates")
    parser.add_argument("--days", type=int, default=7, choices=range(1, 32), metavar="1..31")
    parser.add_argument("--pairs", type=int, default=5, choices=range(1, 21), metavar="1..20", help="max headline pairs per section")
    parser.add_argument("--until", help="optional UTC window endpoint, e.g. 2026-11-22T19:00:00Z")
    parser.add_argument("--list-headlines", action="store_true", help="also print all selected history; private, potentially long")
    args = parser.parse_args()
    if args.replay_labels:
        if args.holdout or args.strict or args.list_headlines:
            parser.error("--replay-labels cannot be combined with database report modes")
        try:
            replay(sys.stdin)
        except ValueError as exc:
            parser.error(str(exc))
        return
    if args.holdout and (not args.until or args.list_headlines):
        parser.error("--holdout needs a fixed --until and cannot list reviewed headlines")
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
    baseline = list(candidate_pairs(rows, 5))
    reference = [(rows[i]["id"], rows[j]["id"]) for _, _, pairs in baseline for i, j, _ in pairs]
    identity = shortlist_id(reference)
    reviewed = {article for pair in reference for article in pair}
    print("Broad five-per-section shortlist ID:", identity)
    if args.holdout and args.holdout != identity:
        parser.exit(1, "Broad shortlist changed; cannot verify the original sample. No holdout produced.\n")
    strict = bool(args.strict or args.holdout)
    if strict:
        print("Strict offline trial: candidates require headline evidence for a common claim. No live feed change.")
    if args.holdout:
        print(f"Fresh article sample: excluded all {len(reviewed)} articles in the verified broad shortlist.")
    report = baseline if not strict and args.pairs == 5 else candidate_pairs(
        rows, args.pairs, strict=strict, excluded=reviewed if args.holdout else frozenset())
    for kind, count, pairs in report:
        print(f"\n{kind}: {count} candidate pairs; showing {len(pairs)}")
        for i, j, shared in pairs:
            for row in (rows[i], rows[j]):
                action = "opened" if row["opened"] else "shown"
                print(f"  #{row['id']} {action} {printable(row['opened'] or row['shown'])} | {printable(row['title'])}")
            print("  Shared words:", ", ".join(shared))
            if strict:
                print("  Event evidence:", same_event(claim(rows[i]["title"]), claim(rows[j]["title"]))[1])
    if args.list_headlines:
        print("\nAll selected headlines, most recent activity first:")
        for row in rows:
            print(f"  #{row['id']} {'opened' if row['opened'] else 'shown'} | {printable(row['title'])}")


if __name__ == "__main__":
    main()
