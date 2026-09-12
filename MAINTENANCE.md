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

## Follow-up Improvements

- Review the SQLite update in a focused dependency change rather than coupling
  a large database-engine upgrade to browser tooling.
- Consider CI using these pinned checks and reviewed dependency-update PRs;
  do not automatically merge or deploy runtime dependency changes.
- Rehearse restore on a disposable database copy with explicit approval before
  copying sensitive production data. Include TLS expiry and off-host backup
  retention in operator checks.
- Add Firefox/WebKit or physical-device checks if browser-specific usage problems
  justify the additional downloads and testing cost.
