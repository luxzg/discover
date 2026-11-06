# November 2026 Audit Remediation

Development-only work authorized 2026-11-06. Production services, private data,
configuration and backups are not test fixtures. The starting production release
is v2.31 (`4ae2a84`). Existing scores, first-seen dates, manual hides and config
must survive. No released migrations will be rewritten.

## Tracking

VERIFIED requires regression checks, documentation, commit and successful push.
Commit references are recorded in the next tracking update to avoid self-hashes.
The normal branch has no automatic deployment; deployment is operator-run.

| ID | Priority | Finding / root cause | Status | Evidence / commit / push |
| --- | --- | --- | --- | --- |
| F01 | P1 | Build canonicalizes aliases; deployment copies into predictable paths | VERIFIED | 48ef106; targeted 8/8 and full race checks; pushed |
| F02 | P1 | Installed Go compiler lacks current security patches | VERIFIED | 07d0d2b; SDK 1.26.10 full checks/scans; pushed; remote deployment separate |
| F03 | P2 | Privileged deployment backup root lacks ancestry validation | VERIFIED | 7c37a50; full checks, 16 targeted cases and independent review; pushed |
| F04 | P2 | Publication extraction accepts unrelated/partial HTML | VERIFIED | 9b45ce8; targeted/full checks and clean source/binary/npm scans; pushed |
| F05 | P2 | Obsolete auth responses can undo successful sign-in | VERIFIED | c49a2a4; 25 JS, twelve browser and full checks passed; pushed |
| F06 | P2 | Batch replacement can race a rule-based hide | VERIFIED | effa887; 30 JS, twelve browser and full checks passed; pushed |
| F07 | P2 | Admin accepts malformed successful JSON as an empty object | VERIFIED | fdd1e32; 34 JS, twelve browser and full checks passed; pushed |
| F08 | P2 | Save completion clears subsequent editor changes | VERIFIED | bff5e73; 46 JS, sixteen browser and full checks passed; pushed |
| F09 | P2 | Score-hidden rows silently ignore positive actions | VERIFIED | 580b50c; full race/vet/script/build/smoke passed; pushed |
| F10 | P2 | Retroactive dedupe lacks service-owned lifecycle/exclusion | VERIFIED | fe16948; full checks, 51 JS and twenty browser checks; pushed |
| F11 | P2 | Redundant scoring writes and per-card source queries | VERIFIED | 553a71b; benchmarks/plans, full race checks and 40k scale passed; pushed |
| F12 | P2 | SQLite alternative-extension sidecars not ignored | VERIFIED | 96af635; matcher and 37 script tests passed; pushed |
| F13 | P2 | SearXNG inspection does not reject UID zero | VERIFIED | c67b8b0; targeted and full checks passed; pushed |
| F14 | P3 | Middle-click opens are not recorded | VERIFIED | 93d13e2; full checks, 56 JS and twenty-four browser checks passed; pushed |
| F15 | P3 | Hidden articles lose recorded-exposure contribution | VERIFIED | 9aeac5a; targeted and full race/vet/script/build/smoke passed; pushed |
| F16 | P3 | Editor labels and menu accessibility state incomplete | VERIFIED | 5835f45; full checks, 57 JS and twenty-eight browser checks; clean source/binary/npm scans; pushed |
| R01 | User | Use operator-managed central Playwright | VERIFIED | 5591044; four browser runs, full checks and project/shared npm audits passed; pushed |
| R02 | User | Prefer newest stable installed Go SDK under HOME/sdk | VERIFIED | d3d9409; selection and full checks passed; pushed |

## Validation And Compatibility

- Run targeted baseline tests before each fix and reproduce with synthetic
  regressions where feasible; record any environmental limitations explicitly.
- Use existing `scripts/check.sh --race`, browser, security and scale helpers.
- Test fresh and legacy synthetic databases; never open the private database.
- Keep deployment orchestration administrator-controlled, service file operations
  unprivileged, backups retained and database restore explicitly operator-owned.
- Final source/binary scans, production/development browser checks and rebuild
  follow the individual fixes. Local checks do not establish remote health.

## Results

All sixteen audit findings and both shared-tooling requests are verified locally,
committed and pushed. The final application version is **v2.48**. Detailed evidence
follows; final dated checks are also recorded in MAINTENANCE.md. No production
deployment was performed or authorized by this remediation task.

After the interrupted VS Code session and laptop restart, the expected pending
F16 changes were preserved. Full checks and both browser build modes were rerun;
the interrupted browser run was not counted. Synthetic old/new SQLite file
compatibility and the 40k scale fixture passed. The compatibility helper initially
hit read-only Go cache permissions, then passed with scoped helper approval.

No remaining audit item is deferred to TODO. Its advanced product ideas remain
separate future scope. This does not establish permanent safety or real-server
health: publisher date coverage, upstream engine availability, production latency,
systemd/TLS and physical-browser behavior still require operator field observation.
Historical suspect publication dates and lost exposure timestamps were not guessed
or rewritten. See INSTALL.md section 7 for deployment and upgrade checks.

### F01

- Original real build helper accepted an output symlink instead of rejecting it;
  failed-start recovery overwrote a synthetic sentinel through a stale link.
  Both regressions now pass, including the original build/backup/start failures.
- Existing Go race tests passed before runtime changes. The initial broad command
  reached script tests during edits and caught a temporary executable-mode
  regression, corrected by chmod on the fresh temporary binary. A separate full
  run passed before commit; do not call that mixed run a clean baseline.
- No database/schema/config changes. Synthetic existing-installation recovery
  verifies old binary and database preservation; fresh builds use the same helper.

### F02

- `PATH` selected Go 1.26.8, but the operator already installed SDK 1.26.10.
  The helper confirms 1.26.10 without any install or global profile change.
  Official downloads/release history checked 2026-11-06: 1.26.10 / 1.27.3.
- Initial patched-compiler race run exceeded the existing aggregate 120-second
  fixture deadline (not a production timeout). The identical non-race fixture
  passed in 3.57s: backfill 1.43s, feed 704ms, archive 174ms, report 72ms, full
  hide 282ms. Measurements are local observations, not production guarantees.
  The test now uses Go's overall test deadline and keeps all data assertions;
  no application timeouts changed. Full race rerun passed (store 96.6s), scripts,
  vet, build and synthetic fresh/config/listener smoke passed. The running helper
  was edited during one run, causing a shell read-offset error after its checks;
  a stable unedited rerun passed. Do not edit a running script.
- Source/binary govulncheck 1.8.0 (DB 2026-11-05 23:49:57 UTC) found zero reachable
  vulnerabilities, two uncalled package and five uncalled module x/net advisories.
  Track their compatible patch with F04 before changing parser APIs. npm audit
  including developer dependencies: zero. These are dated observations only.
- Existing legacy migration tests passed; no module/schema/config changes.
  Remote compiler selection and deployed binary remain operator verification.

### F03

- Confirmed root-shell snapshot writes trusted an unvalidated path; synthetic
  non-root owner, writable ancestor and real symlink were accepted before the
  fix and rejected afterward, with no service stop or backup creation.
- Guard stays inline in administrator-controlled orchestration transmitted by
  the SSH wrapper. Original lexical components are inspected before any path
  resolution; root-owned non-writable ancestry prevents unprivileged replacement.
  Default paths and existing failure-recovery tests pass. Review found the old
  multi-missing-parent traversal limitation; newly created parents now use 0711,
  while existing ancestry is never widened. Targeted suite: 16 tests. Static
  independent review found no remaining F03 bypass; actual UID separation is
  simulated, not a claim of a real-root deployment test.
- No schema/config/data changes. Real server permissions are intentionally
  uninspected; unsafe custom layouts will now fail explicitly, not be repaired.

### F12

- Git's real ignore matcher rejected `synthetic-ignore-check.sqlite-wal` before
  the fix; now all twelve extension/journal variants match, installation temps
  are ignored and config.example.json remains trackable. No files with those
  synthetic database names were created or opened; no private data inspected.
- No runtime, configuration, schema or upgrade impact. Script-suite integration
  makes the regression repeatable without requiring a new tool.

### F13

- Two failing synthetic owner cases (root and another UID-zero name) now reject
  before the runuser/application double. The valid non-root inventory still
  reports revision/Python without reading settings. Noninteractive Git options
  also match the updater's existing operational contract. Eight targeted tests
  passed. Full checks before commit; no schema/config/upgrade changes.

### R01 - Shared Browser Tools

- Operator preference overrides earlier per-repo pinning. Default resolution is
  the shared Playwright package, including the test runner it already contains;
  no @playwright/test install is needed. No changes to the external tools tree.
- Shared 1.64.1, Node 22.23.4: production/development desktop/mobile browser suite
  passed (four runs). Setup confirms matching Chromium without downloading it.
  Synthetic selector tests cover explicit installation choice, missing Chromium
  and relative-path rejection. Full race/vet/script/build/smoke passed. Source/
  binary scan remains zero reachable with the seven tracked uncalled advisories;
  project and shared tooling npm audits both report zero. Config/runtime/database
  upgrade behavior unchanged.

### F04

- Six scope cases and an over-limit HTML case failed before the fix and pass
  afterward. Head conflicts remain unknown; one article scope excludes nested
  articles and navigation/sidebar/footer. Complete structured data still works.
  Test sizes: 1MiB-1, exactly 1MiB, 1MiB+1. Oversized documents retain an early
  valid image but no guessed date. Extraction depth is bounded and fails unknown.
- x/net 0.62.0 fixes the seven previously uncalled advisories before use of the
  tree parser. Required x/sys is 0.49.0. Upstream go.mod requires Go 1.26.0;
  application minimum remains 1.26.8, SQLite/libc unchanged. Dependency helper
  applies only this reviewed version and tidies sums. No database migration or
  historical-date rewriting; fresh and legacy fixture checks remain required.
  Full race suite passed, including legacy upgrades; Go scans now show no
  reachable or uncalled advisories, project/shared npm audits zero. Scan database
  timestamp: 2026-11-05 23:49:57 UTC. Historical publication provenance is unknown;
  prevention is fixed, but existing suspect dates require evidence before repair.

### R02 - Shared Go SDKs

- Reproduced PATH overriding a newer shared SDK in a synthetic fixture. Selection
  now uses version ordering, excludes prereleases, sets GOTOOLCHAIN=local for the
  selected SDK and preserves PATH fallback without SDKs. No SDK files changed.
- Added formatting to the same selector and removed hard-coded 1.26.10 overrides.
  Real selected compiler remains 1.26.10. Synthetic command doubles now isolate
  HOME: the first regression run exposed a mock bypass into real module lookup,
  corrected before rerun, not concealed by allowing external test traffic.
- No runtime/schema/config/dependency changes; shared SDK upgrades still require
  tests/rebuild and operator deployment. Full checks before commit.

### F05

- Eight deterministic regressions reproduced delayed startup 401/success/JSON/
  network failures for feed/Admin. Actual login handlers now retain new auth,
  CSRF and status when obsolete responses arrive. Requests are epoch-fenced,
  pending network fetches aborted at transitions, explicit auth serialized, and
  stale catches/finally blocks cannot clear newer operation locks.
- Existing failure/logout/current-401/hide tests retained; 25 JS tests passed.
  Backend APIs/TTL/cookies unchanged. Browser cookie ordering and full checks
  passed: twelve production/development desktop/mobile browser checks, including
  real cookie/CSRF ordering for both roles, plus full race/vet/script/build/smoke.
  Inspected the synthetic mobile feed screenshot; no private login state used.
  No persistent data/config or migration changes.

### F06

- Four controlled races reproduced Hide This/Domain overlapping seen POST or
  feed GET. Negative actions now stop before prompts/submission while the batch
  lock is held. The helper independently enforces that boundary. Reverse hide/
  next exclusion and startup recovery lock tests pass (30 JS tests total).
- No API, persistent-data, schema or config changes. No automatic retry/queue;
  accepted hides remain service-owned and positive actions keep cards visible.
  Full race/vet/script/build/smoke and twelve shared-browser checks passed.

### F07

- Reproduced malformed-success acceptance and token-less login. Successful replies
  now require an object and auth replies require a usable CSRF token. Malformed
  401/500 bodies preserve the HTTP status; current 401 still expires auth.
  Tests retain drafts/prior lists on uncertain replies; one fake-DOM setup error
  was corrected, not counted as a successful reproduction. JS suite: 34 passed.
- Existing endpoints return JSON objects, including logout/deletes; no valid 204
  contract removed. No backend/schema/config changes or automatic retries.
  Full race/vet/script/build/smoke and twelve shared-browser checks passed.
  Inspected the synthetic mobile Admin screenshot; layout remains bounded.

### F08

- Ten failing cases (typing/toggle/select/clear/duplicate save, both editors) now
  pass; unchanged drafts still reset. Revision plus submitted payload checks
  preserve newer intent, and each editor serializes submissions. JS suite: 46.
- Confirmed save and subsequent list refresh failure have distinct messages.
  Epoch-owned cleanup respects F05; auth reset releases stale busy flags. No
  backend API/schema/config or migration changes. Full race/vet/script/build/
  smoke and sixteen production/development desktop/mobile browser checks passed.
  A brief operator network interruption did not affect local checks; GitHub
  connectivity and the previous pushed revision were verified afterward.

### F09

- Both score-hidden Read/Useful regressions failed before the fix. The guard now
  permits positive actions only for known duplicate/score filters, not manual,
  legacy, blank or unknown hide origins. Repeated votes remain single-counted;
  handled cards are not returned as unread. No schema/config changes.
- Full race/vet/script/build/smoke passed, including all twelve hide-origin/action
  combinations and idempotent votes. Existing 40k store regression passed.

### F10

- Legacy dedupe returned success during active ingestion before the fix. Scheduler
  gating now excludes dedupe/ingest/checks, waits for due automatic ingestion and
  owns cancellation/shutdown. New Admin job endpoints use immediate acceptance
  and memory-only snapshots. Existing synchronous API response remains unchanged.
- Concurrent callers join one job. Recent results are bounded at 32; no durable
  queue or schema/config change. Counter snapshots are inside the transaction.
  Tests cover auth/CSRF/CIDR, browser cancellation, blocked SQLite/status, legacy
  joining, rollback, shutdown, time limits, eviction and stale UI responses.
  A first legacy-join test released work before the waiter joined, corrected with
  an explicit synchronization signal; this was a fixture race, not a clean pass.
- Independent review found idle dedupe polls could clear pending archive/report
  button state. Two controlled regressions reproduced it; explicit epoch-owned
  busy flags now survive polling. Cached scheduler state also preserves run/check
  cooldown guards when rendering job status. Full race/vet/script/build/smoke,
  51 JS and twenty production/development desktop/mobile browser checks passed.
  Independent review reported no other blocking backend findings in this pass.

### F11

- Baseline unchanged-effect write reproduced; existing handled-legacy/new-rule
  counting remains tested. Effects now bulk-load per article and no-op writes are
  skipped without freezing changed evidence or deliberate edits. Dedupe skips
  singleton groups/unchanged pointers and repeated old/new-key reconciliation.
- Feed ranking loads payloads after selection; one indexed source query preserves
  per-card caps, eligibility, order, dates and image fallback. Clock tests retain
  hidden/archived history outside the source cap. A test's initial nanosecond
  comparison was corrected to SQLite's existing millisecond display precision.
- Three-sample local benchmark medians and fixture/version details are recorded
  in DEVELOPMENT.md. The first join plan was too expensive and was rejected;
  index/order correction has a regression test. No schema/config/API changes,
  caches, extra connections or production measurements. Full race/vet/script/
  build/smoke passed. Non-race 40k integration passed: feed 426ms, full hide 208ms
  on that different fixture. Independent review found no blocking regression.

### F14

- Missing auxiliary-click handling reproduced. Normal and middle-click share one
  authenticated/CSRF tracking path, retain selected source IDs and leave native
  navigation/cards intact. Other auxiliary buttons do not count as opens.
- Added five controlled event cases and real synthetic publisher-tab browser
  interactions. Context-menu navigation remains an explicit browser limitation;
  no invasive navigation interception or tracking library added.
- The first browser case incorrectly required a new-page event for middle-click;
  this headless Chromium did not emit one. Coverage now checks its trusted native
  event, uncanceled default, selected source and real API 200, while normal-click
  publisher-tab navigation is separately exercised. This limitation is documented.
- Full race/vet/script/build/smoke, 56 JS and twenty-four production/development
  desktop/mobile browser checks passed after correcting that fixture assumption.

### F15

- Read/Useful then hide and historical read-timestamp cases lost Shown contribution
  before the fix. Positive actions now retain last_seen_at, and report fallback
  includes read_at. Current deliberate hides still override positive interest;
  hide-only rows do not invent exposure. No migrations, guessed backfill or new
  tracking/event model. Lost legacy Useful-only history remains unrecoverable.
- Targeted actions and full race/vet/script/build/smoke passed, including legacy
  state and the large synthetic store fixture. No production backfill performed.

### F16

- Missing accessible trigger/state reproduced. Added field labels, controlled
  panels, expanded-state synchronization and Escape/Useful focus restoration.
  Existing visual language and native buttons remain; no menu framework added.
- Added actual browser keyboard/label/layout regressions. Final documentation
  review found an obsolete inline cache-build fallback and an overbroad RFC3339
  storage claim; both corrected. After the laptop restart, reran full race/vet/
  script/build/smoke and all twenty-eight production/development desktop/mobile
  browser checks successfully; reviewed label/menu screenshots at both sizes.
  Source/binary scans found no reachable or uncalled advisories, and project/
  shared npm audits reported zero. The interrupted browser run is not counted.
