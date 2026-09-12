# Development And Validation

## Environment

- Go 1.26.8 or newer, Git and Bash.
- Node.js 22+ and npm for JavaScript/deployment-script tests and developer-only
  Playwright browser tests. Unit/script tests themselves use Node built-ins.
- SQLite CLI for operational backups; shellcheck is optional locally.
- `govulncheck` for the separate dependency/security check: install with
  `go install golang.org/x/vuln/cmd/govulncheck@latest`.

Direct Go dependencies are `modernc.org/sqlite` (pure-Go SQLite) and
`golang.org/x/net/html` (HTML tokenizer for article metadata). Other module
requirements are transitive. Builds use `-mod=readonly`; only deliberate dependency
maintenance uses `go mod tidy`/`go get`. Go may download the toolchain selected by
`go.mod` under its normal automatic toolchain policy. No production Go compilation
is run as root.

See `MAINTENANCE.md` for monthly tool/dependency reviews, dated scan results,
download prerequisites and the policy for upgrading compilers without needlessly
raising the module minimum. Helpers preserve the compiler selected by PATH.

SQLite is compiled into the binary: `modernc.org/sqlite` v1.58.0 bundles engine
3.53.4 and requires `modernc.org/libc` v1.75.6. Keep that exact pair together.
The installed `sqlite3` CLI is a separate backup/debug tool, not the app engine.

## Repeatable Commands

Run each command separately from the repository root:

```bash
./scripts/format.sh
./scripts/check.sh --race
bash scripts/setup-browser.sh
bash scripts/test-browser.sh --all
bash scripts/security-check.sh --binary
```

`check.sh` checks diffs/formatting, runs `go vet`, Go tests, shell syntax and
deployment-command doubles, JavaScript syntax/behavior tests, then builds and
verifies `discover` using `--version`, then runs isolated CLI/config/listener
smoke tests against that binary. Race testing requires a native C compiler
and CGO enabled for Go's race detector; the application itself does not require CGO.

Browser setup is needed initially and after lockfile/browser upgrades, not on every
test run. It downloads locked npm dependencies and matching Chromium without
installing OS packages. Browser tests and network vulnerability scans are separate
from the default deterministic checks. Test artifacts are ignored; see
`MAINTENANCE.md` for scope and cleanup guarantees.

For narrower iterations:

```bash
./scripts/test.sh
./scripts/build.sh
./discover --version
```

For repeatable hide/scoring latency measurements without race instrumentation:

```bash
bash scripts/test-hide-scale.sh
```

This builds a disposable 40,000-article/80-rule fixture and times rule creation,
penalty edits, deletion and full Hide Domain (400 matching articles). The regular
suite also runs this regression test. Race timings are intentionally not treated
as production performance measurements; content sizes, match rates, disk and CPU
affect real service latency.

For SQLite dependency changes, also run:

```bash
bash scripts/test-sqlite-upgrade.sh
```

It builds the current DB package against the v2.25 module locks and current locks
in temporary directories, then creates/reads/writes a synthetic file with both
engines. It checks WAL/foreign keys, persisted scores/dates/hide state/counters,
rollback and integrity. It requires the v2.25 commit in local Git history and may
download locked modules; it is separate from ordinary offline unit tests. It
never copies, restores or opens production data. The normal suite separately
checks legacy schema migration and full store/scoring behavior.

The build uses a temporary artifact and atomic rename, embeds commit/build time,
and never starts the server as a test. A dirty-tree build is a development artifact;
rebuild after the final commit before deployment to embed the committed revision.
No test opens the private `config.json` or production database. Database tests use
temporary fixtures, including an old-schema upgrade. Deployment tests use fake
service/SSH commands and never contact a real server.

## Architecture And Invariants

- `config`: exclusive default-file creation and read-only validation.
- `auth`: independent in-memory admin/feed sessions, CSRF and login throttling;
  malformed admin allowlists fail closed.
- `scheduler`: one ingestion at a time, asynchronous manual acceptance, 15-second
  completion cooldown, service-owned cancellation and shutdown waiting.
- `ingest`: fixed bounded SearXNG harvest, partial-result preservation and bounded
  error identifiers. Article metadata uses a separate public-only DNS-pinned
  transport, including redirects; local SearXNG is intentionally allowed.
- `db`/`store`: enforced foreign keys; transactional per-topic evidence and rule
  effects; baseline-preserving upgrades; derived story identity and hide reasons.
- `server`: authenticated/CSRF-protected mutation routes, rolling cookies,
  no-store responses, restrictive script policy, escaped DOM rendering.
  Rule-based hides use a single service-owned job with immediate HTTP 202
  acceptance, authenticated in-memory status polling, a 32-result retry cache
  and shutdown cancellation/waiting. Browser disconnects do not cancel accepted
  work; process restarts do, with transactional rollback. Each job has a ten-minute
  safety deadline, not a target latency. Only visible-batch IDs are cached.

Rule edits scan candidates for that rule only, write changed effects using prepared
statements, recompute only changed scores from their ledger and reconcile affected
derived story groups. Topic edits reuse the current rule ledger. Avoid restoring the former per-article,
per-rule database loop in interactive mutation paths.

Do not treat an automatically hidden duplicate as proof the story was handled.
Do not infer a publication date from ingestion time. Do not strip arbitrary URL
queries: they can distinguish articles. Never double-apply a negative rule to the
clicked article. Keep URL/article identity distinct from conservative story keys.

## Validation Boundaries

Automated tests cover SSRF addressing/redirects, auth separation, CIDRs/CSRF,
request validation, asynchronous jobs, date storage/display, stable scoring,
transaction rollback, dedupe counters, legacy migration, JS actions and deployment
ordering/recovery. Unit JS tests use an isolated DOM model. The separate
Playwright suite renders actual embedded production and plain development builds
in desktop/mobile Chromium, checks assets/console/layout and writes screenshots.
It uses only synthetic fixtures and a fake local SearXNG. Root-path deployment is
tested; subpath deployments, WebGL and workers are not applicable to this app.

After remote deployment, the operator should confirm login restoration after a
fresh sign-in, card/menu/mobile layout, known publication dates, Other sources,
manual ingest completion/warnings, and service version/logs. Actual search quality,
publisher metadata and remote systemd/TLS behavior require field testing.

`TODO.md` tracks future features, not an assertion that all possible bugs are gone.
External image requests remain a documented browser privacy tradeoff. Upstream
engine filtering, missing dates and rate limits cannot be guaranteed by Discover.
