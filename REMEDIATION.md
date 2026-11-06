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
| F02 | P1 | Installed Go compiler lacks current security patches | IN PROGRESS | SDK 1.26.10 selected; full race/build/smoke passed; source/binary scans zero reachable, seven uncalled x/net advisories; commit/push next |
| F03 | P2 | Privileged deployment backup root lacks ancestry validation | PENDING | |
| F04 | P2 | Publication extraction accepts unrelated/partial HTML | PENDING | |
| F05 | P2 | Obsolete auth responses can undo successful sign-in | PENDING | |
| F06 | P2 | Batch replacement can race a rule-based hide | PENDING | |
| F07 | P2 | Admin accepts malformed successful JSON as an empty object | PENDING | |
| F08 | P2 | Save completion clears subsequent editor changes | PENDING | |
| F09 | P2 | Score-hidden rows silently ignore positive actions | PENDING | |
| F10 | P2 | Retroactive dedupe lacks service-owned lifecycle/exclusion | PENDING | |
| F11 | P2 | Redundant scoring writes and per-card source queries | PENDING | Measure before optimizing; retain historical story clocks |
| F12 | P2 | SQLite alternative-extension sidecars not ignored | PENDING | |
| F13 | P2 | SearXNG inspection does not reject UID zero | PENDING | |
| F14 | P3 | Middle-click opens are not recorded | PENDING | |
| F15 | P3 | Hidden articles lose recorded-exposure contribution | PENDING | |
| F16 | P3 | Editor labels and menu accessibility state incomplete | PENDING | |

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

Implementation and validation evidence will be appended per finding. No production
deployment is authorized by this remediation task.

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
