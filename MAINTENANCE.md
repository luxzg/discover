# Tooling And Dependency Maintenance

## Monthly Review

During normal development, check this record about once a month and ask the
operator to review toolchain/dependency updates when it is stale. Last review:
**2026-09-12**; next suggested review: **2026-10-12**. This is a workflow reminder,
not an installed scheduler or unattended upgrade service. Review sooner after
a relevant vulnerability disclosure. Append dated results; preserve history.

1. Read `AGENTS.md` and `DEVELOPMENT.md`, inspect the working tree, manifests,
   lockfiles, build scripts and deployment path. Preserve unrelated changes.
2. Run `bash scripts/tooling-report.sh`, then
   `bash scripts/tooling-report.sh --online` for official Go releases and
   dependency update candidates. Failed/offline checks are not clean results.
3. Verify supported patches against [official Go downloads](https://go.dev/dl/)
   and [release history](https://go.dev/doc/devel/release). Ask the operator to
   install system tools; never assume sudo. Do not raise `go.mod`'s minimum merely
   because a newer compiler is installed. Check the server compiler separately.
4. Review dependency release notes/compatibility before updating. Keep manifests
   and lockfiles synchronized, and test database upgrades/workloads where relevant.
   Do not apply `npm audit fix --force` or bulk Go upgrades blindly.
5. Run deterministic checks, both browser build modes and external vulnerability
   checks below. Record tool versions, date, findings, failures and limitations.
6. Update docs/version/changelog, commit, rebuild the committed revision and deploy
   through the reviewed workflow. Verify version and authenticated functionality,
   not only an active process. Installing a compiler does not patch running code.

Future additions should state scope, operator action, prerequisites, external
downloads, recurring command, failure interpretation and verification steps.
Keep runtime requirements separate from developer-only tools.

## Developer Prerequisites And Downloads

Discover still deploys as a Go binary; Node/npm/Chromium are **not required on the
production server**. Development checks use Node.js 22+, npm and Bash. A native C
compiler is needed for Go's race detector, not ordinary pure-Go SQLite builds.
`curl` is used by the online report; shellcheck is an optional shell linter.
Ask the operator to install missing system packages or browser OS libraries;
do not install them with sudo automatically.

Install or update the Go scanner deliberately:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
```

This downloads modules and writes to GOBIN (normally `$HOME/go/bin`). Helpers
respect the existing PATH, appending known Go installation/bin locations only
as fallbacks. They do not change `GOTOOLCHAIN` or install a system compiler.
Go's automatic toolchain selection may download a compiler if the selected
one is older than the module minimum.

Install the exact npm lockfile and matching Chromium on the development machine:

```bash
bash scripts/setup-browser.sh
```

This runs `npm ci` (replaces local `node_modules`) and the project-local Playwright
Chromium installer, with a bounded download time. Browser/companion binaries use
Playwright's user cache. No `--with-deps` or privileged OS installation is run.
If download times out, check connectivity and retry. Equivalent manual commands:

```bash
npm ci
npx --no-install playwright install chromium
```

Keep `@playwright/test` exactly pinned and commit `package-lock.json`. After an
intentional Playwright upgrade, install its matching browser revision again;
arbitrary system Chrome is not a reproducible substitute. See
[Playwright browser installation](https://playwright.dev/docs/browsers).

## Repeatable Validation

Run each command separately from the repository root:

```bash
./scripts/check.sh --race
bash scripts/test-browser.sh --all
bash scripts/security-check.sh --binary
```

`check.sh` covers formatting, vet, unit/regression tests, script doubles, build
metadata and isolated binary smoke tests. It does not implicitly run online
scans or download browsers. Initial Go builds may download locked dependencies
or a required toolchain; warmed-cache tests use synthetic local inputs.

Browser testing runs the actual release-script binary (`--production`, default)
and a plain Go development build (`--development`). `--all` runs both serially.
Each mode exercises desktop and mobile Chromium against temporary synthetic
SQLite/config, random credentials and a loopback fake SearXNG. Coverage includes
login, session restoration, asset URLs, version, dates, story groups, upvotes,
background domain hides, pagination/scroll, admin authentication and layout.
Unexpected browser network requests are blocked and fail the test. Test-owned
servers are stopped and temporary databases removed on ordinary failures too;
forced termination such as SIGKILL cannot run cleanup handlers.

Screenshots/failure artifacts go under ignored `.browser-artifacts/`. No private
config, production database, saved login state or browser profile is used.
Traces/video are disabled to avoid unnecessarily recording credentials. Review
desktop/mobile screenshots after UI changes, not only test assertions.

There is no frontend bundler, separate JS development server, WebGL canvas or web
worker in Discover, so those specific checks do not apply. Embedded URLs assume
deployment at `/` with `/assets/` and `/admin`; a reverse-proxy subpath is not
supported or validated. Chromium emulation is not physical-device, Firefox or
Safari coverage. Real TLS, systemd, publisher images and SearXNG quality remain
operator/opt-in checks, separate from deterministic fixtures.

## Interpreting Security Checks

The security helper prints date and versions, runs `govulncheck -show verbose ./...`,
optionally scans the already built binary, then runs `npm audit --include=dev`.
It fails on scanner/audit errors. It is fail-fast: resolve earlier failures and
rerun to ensure later checks execute. Failed/offline scans are never clean results.
Use `GOVULNCHECK` to select another installed scanner if necessary. Build first
for `--binary`; `go version -m ./discover` identifies the artifact's actual
compiler/dependencies, not just the current shell's compiler.

Distinguish reachable Go call paths from advisories on uncalled packages/modules.
Uncalled findings are not proven exploitable, but remain review/update candidates.
Binary scanning has less call-path detail than source analysis. See the
[govulncheck documentation](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck).
npm development-tool advisories can affect developers/CI even though npm is absent
from production. Neither advisory database guarantees detection of every issue,
including upstream SQLite engine vulnerabilities or local application logic flaws.

## Dated Review Results

### 2026-09-12 - v2.25 Tooling Review

- Development tools: Go **1.26.8 linux/amd64**, Node **22.23.2**, npm **10.9.8**,
  govulncheck **1.8.0**. Existing `go.mod` minimum remains **1.26.8**.
- Official release checks returned **1.26.8** and **1.27.1** as latest supported
  Go patches. No system compiler installation was needed on the laptop.
- Added developer-only Playwright **1.63.0**, matching Chromium **153.0.8010.12**,
  revision **1243**. `npm ci` and browser installation succeeded without missing
  browser OS libraries or privileged installation.
- Source and built-binary govulncheck reported no vulnerabilities; npm audit
  including development dependencies reported **0**. The scanner's vulnerability
  database timestamp was **2026-09-10 14:48:42 UTC**. These are dated observations,
  not permanent safety claims.
- Production/development browser runs passed on desktop and mobile; synthetic
  screenshots were inspected. Full regression/race, vet, script and binary smoke
  checks passed. Shellcheck was unavailable; shell syntax and executable regression
  tests passed, but the optional shellcheck lint pass was not run.
- Online inventory found SQLite **v1.39.1 -> v1.58.0** and associated transitive
  update candidates. These were not automatically applied: a separate upgrade
  needs release review, migration/recovery tests and scoring/dedupe latency checks.
  No npm update candidates remained.
- Fixed scripts that could prioritize an old private Go installation over the
  compiler explicitly selected by the operator's PATH.
- No production server, config, database or backup was accessed. Deployment
  remains an operator action using `INSTALL.md` section 7.

### 2026-09-12 - v2.26 SQLite Upgrade

- Upgraded `modernc.org/sqlite` v1.39.1 to v1.58.0 (SQLite 3.50.4 to 3.53.4),
  with its required exact `modernc.org/libc` v1.75.6 and related module updates.
  The Go minimum remains 1.26.8. Do not update libc independently to its latest
  version; the driver's matching version requirement takes precedence.
- Reviewed [upstream release notes](https://gitlab.com/cznic/sqlite/-/blob/v1.58.0/CHANGELOG.md)
  and connection-parameter changes. Existing `_pragma` options remain supported;
  no optional OFD locking, custom page cache, virtual tables or time-conversion
  modes were enabled. No application schema/config changes are required.
- Added `bash scripts/test-sqlite-upgrade.sh`: an isolated file written by the old
  engine is verified/updated by the new engine, then reopened by both. Scores,
  dates, deliberate hides, rule evidence and counters persist; WAL, foreign keys,
  transaction rollback and integrity checks pass. This is synthetic engine
  compatibility validation, not an operational restore drill or production copy.
- Shellcheck 0.11.0 is now installed. Its first run flagged a conditional in the
  backup helper; changed it to explicit `if` logic without relaxing permissions.
- Validation passed with Go 1.26.8: full vet/race/unit/script/binary-smoke checks,
  Shellcheck 0.11.0, cross-driver file checks and all four production/development
  desktop/mobile browser runs. The 40,000-article/80-rule full-hide measurement
  was about 221 ms before and 204 ms after this upgrade on the development machine;
  single synthetic measurements are not a guaranteed production speedup.
- Source and binary govulncheck 1.8.0 reported no vulnerabilities at 16:20 CEST;
  the reported database timestamp remained 2026-09-10 14:48:42 UTC. npm audit,
  including dev dependencies, reported zero vulnerabilities. These dated results
  do not guarantee absence of undisclosed or application-specific issues.
- The operator confirmed v2.25 remote deployment succeeded. v2.26 remote
  deployment/health checks remain an operator action after local validation.

### 2026-10-05 08:58 CEST - v2.27 Freshness Validation

- Kept the Go minimum, runtime dependencies and npm lockfile unchanged. Used Go
  1.26.8 and govulncheck 1.8.0. This feature pass is not a full monthly online
  release inventory; the suggested tooling review remains October 12.
- Full vet/race/unit/script checks, ShellCheck 0.11.0 and binary smoke passed.
  Production and development browser modes passed on desktop/mobile; inspected
  synthetic feed/admin screenshots, including date labels, archive controls and
  the horizontally scrollable domain report. No private state or external
  publishers were used in those deterministic checks.
- On the synthetic 40,000-article fixture, feed selection took about 571 ms,
  archiving 136 ms, the domain report 59 ms and full hide 211 ms without race
  instrumentation. A follow-up measured first-seen backfill at about 864 ms
  with comparable feed/archive/report/hide timings. These individual measurements
  are not production guarantees.
- Source and built-binary govulncheck reported no vulnerabilities; npm audit
  including development dependencies reported zero. The scanner database
  timestamp was 2026-10-01 20:24:15 UTC. Dated scans do not prove permanent safety.
- New tests cover immutable legacy discovery dates, bounded scoring windows,
  syndicated clocks, reversible archives, raw-score preservation, shared date/
  image requests and retry timing, read/hide precedence, admin auth/CSRF and UI
  escaping. No production DB, server service or SearXNG installation was changed.
- Deployment remains an operator action. Verify v2.27, preview the chosen age
  limit, inspect retained domain history and run ingestion to evaluate publisher
  date coverage. SearXNG updating is pending read-only server inventory.

### 2026-10-05 09:46 CEST - v2.28 Follow-up Validation

- Operator confirmed v2.27 remote deployment and version in both UIs, First seen
  dates, fresher ordering and the domain report. No full deployment log was
  needed to record that scope; exact private-row restoration remains uninspected.
- v2.28 restoration tests verify preview/apply agreement, no preview mutations,
  disabling/increasing age limits, duplicate/manual-hide preservation and a
  restored high raw-score story remaining eligible below a fresher card.
- Domain tests verify merge-before-filter/cap, complete subdomain sums, 200-row
  stable ordering, multi-label suffixes, separate private hosted sites and no
  source rewriting. The existing x/net module supplies the bundled PSL; no
  dependency or toolchain upgrade was made.
- Full vet/race/Go/script/JS/binary-smoke checks passed. Production/development
  desktop/mobile browser tests passed; inspected updated Admin screenshots.
  Go source/binary govulncheck 1.8.0 with Go 1.26.8 reported no vulnerabilities;
  npm audit including dev dependencies reported zero. The database timestamp
  remained 2026-10-01 20:24:15 UTC. These are dated, limited observations.
- Recorded read-only SearXNG server inventory in SEARXNG.md. No production DB,
  service, settings or SearXNG packages were changed by the agent. v2.28 deployment
  and revised UI behavior remain operator checks using the existing update script.

### 2026-10-05 11:01 CEST - v2.29 Pacing And SearXNG Updater

- Kept Go/npm runtime and development dependency locks and the Go minimum
  unchanged. Full vet/race/Go/script/JS/binary smoke passed, including 25 Node
  script regressions, three standard-library Python checker tests and ShellCheck.
  Checker tests used local Python 3.14.7; they do not install SearXNG or prove
  package compatibility on the operator's Python 3.12.12 server.
- All four production/development desktop/mobile browser runs passed with
  synthetic services/data and explicit zero pacing in fixtures. Search tests
  separately verify default inheritance, request pauses across topic/failover
  boundaries, cancellation and run deadlines. No user config was rewritten.
- Source/binary govulncheck 1.8.0 with Go 1.26.8 at 10:58 CEST reported no
  vulnerabilities; npm audit including dev dependencies reported zero. Scanner
  DB timestamp remained 2026-10-01 20:24:15 UTC. These scans cover Discover,
  not SearXNG/Python packages, and do not imply permanent safety.
- Checked current upstream installation/package/settings/webapp sources before
  implementing the updater. Current package metadata permits Python 3.10+;
  actual candidate install/settings/startup health remains a server check.
  Webapp imports initialize caches/engine networking, so offline preflight
  deliberately checks settings/module syntax without starting that code.
- Operator-confirmed v2.28 deployment/archive/report and search diagnostics
  motivate pacing and upstream maintenance. No live SearXNG, service, DB,
  private settings or OS packages were accessed/changed by the agent. Use the
  administrator-owned updater in SEARXNG.md; validate preflight then apply.
  API health, usable search results and engine recovery are separate checks.
  Full monthly tool inventory remains due October 12, not performed in this pass.

### 2026-10-05 11:19 CEST - v2.30 Updater Follow-up

- Operator preflight passed Python/dependency/settings checks, then terminated
  after a wait. Git interactivity is suspected; real-server diagnosis remains
  pending. Do not treat the legacy-settings warning or those partial checks as
  completed preflight or proceed to activation on that evidence alone.
- Full local vet/race/Go/script/JS/binary smoke and ShellCheck passed, including
  27 Node script regressions and three Python checks. A disposable real-Git
  pseudo-terminal exercises a configured pager and verifies the helper bypasses
  it; a doubled deadline verifies clear phase reporting and no service mutation.
  No live service/config was accessed, dependencies changed or new security scan
  performed in this follow-up; earlier scan results retain their dated scope.

### 2026-10-05 11:52 CEST - v2.31 Admin Diagnostics And Scheduling

- Full vet/race/Go/script/JS/binary smoke and ShellCheck passed. New regressions
  cover diagnostic authentication/CSRF/CIDR, immediate acceptance independent of
  request cancellation, bounded/escaped reports, pacing/cancellation/429 handling,
  mutual exclusion, cooldown, visible interval/daily schedules and shutdown.
- All four production/development desktop/mobile browser runs passed. Synthetic
  checks sent exactly two extra sample searches and left article counts unchanged;
  reviewed desktop/mobile screenshots, including stacked mobile report rows.
  Test-owned servers/data were cleaned up; no production credentials or engines
  were used. Live SearXNG results remain an operator check after deployment.
- Source and binary govulncheck 1.8.0 with Go 1.26.8 reported no vulnerabilities,
  npm audit including dev dependencies reported zero. Scanner DB timestamp:
  2026-10-01 20:24:15 UTC. No dependency or module-minimum changes; scans cover
  Discover/developer tooling, not the remote SearXNG Python installation.
- Recorded operator confirmation of deployed v2.30 and the SearXNG upgrade/local
  health. These do not establish upstream engine availability. The new Admin
  action is explicit active sampling, not continuous engine monitoring.

### 2026-10-05 12:07 CEST - Operator Closeout

- Operator confirmed v2.31 deployment in service and both UIs, next-run display
  for the configured three-hour interval, and readable engine diagnostics with
  cooldown. Samples returned usable news/general results alongside engine
  warnings: partial availability only. Scheduled ingestion remains unverified.
- Operator explicitly added missing config defaults and restarted; documented
  safe edits, saved age-limit precedence and interval reset on service startup.
  No app/dependency/toolchain changes, new scans, builds or live agent probes
  were performed in this documentation-only pass. The full monthly inventory
  reminder remains October 12; earlier scans retain their dated scope.

## Maintenance Scope

CI and recurring restore drills were declined as unnecessary for this small
project. TLS renewal/monitoring remains the operator's responsibility through
existing server scripts. These suggestions are closed, not pending work; normal
deployment snapshots and deliberate recovery procedures remain unchanged.

## Follow-up Improvements

- Add Firefox/WebKit or physical-device checks if browser-specific usage problems
  justify the additional downloads and testing cost.
