# Development And Validation

## Environment

- Go 1.26.8 or newer, Git and Bash.
- Node.js 22+ for dependency-free JavaScript and deployment-script tests.
- SQLite CLI for operational backups; shellcheck is optional locally.
- `govulncheck` for the separate dependency/security check: install with
  `go install golang.org/x/vuln/cmd/govulncheck@latest`.

Direct Go dependencies are `modernc.org/sqlite` (pure-Go SQLite) and
`golang.org/x/net/html` (HTML tokenizer for article metadata). Other module
requirements are transitive. Builds use `-mod=readonly`; only deliberate dependency
maintenance uses `go mod tidy`/`go get`. Go may download the toolchain selected by
`go.mod` under its normal automatic toolchain policy. No production Go compilation
is run as root.

## Repeatable Commands

Run each command separately from the repository root:

```bash
./scripts/format.sh
./scripts/check.sh --race
./scripts/security-check.sh
```

`check.sh` checks diffs/formatting, runs `go vet`, Go tests, shell syntax and
deployment-command doubles, JavaScript syntax/behavior tests, then builds and
verifies `discover` using `--version`, then runs isolated CLI/config/listener
smoke tests against that binary. Race testing requires a native C compiler
and CGO enabled for Go's race detector; the application itself does not require CGO.

For narrower iterations:

```bash
./scripts/test.sh
./scripts/build.sh
./discover --version
```

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

Do not treat an automatically hidden duplicate as proof the story was handled.
Do not infer a publication date from ingestion time. Do not strip arbitrary URL
queries: they can distinguish articles. Never double-apply a negative rule to the
clicked article. Keep URL/article identity distinct from conservative story keys.

## Validation Boundaries

Automated tests cover SSRF addressing/redirects, auth separation, CIDRs/CSRF,
request validation, asynchronous jobs, date storage/display, stable scoring,
transaction rollback, dedupe counters, legacy migration, JS actions and deployment
ordering/recovery. JS tests use an isolated DOM model, not a real rendered browser.

After remote deployment, the operator should confirm login restoration after a
fresh sign-in, card/menu/mobile layout, known publication dates, Other sources,
manual ingest completion/warnings, and service version/logs. Actual search quality,
publisher metadata and remote systemd/TLS behavior require field testing.

`TODO.md` tracks future features, not an assertion that all possible bugs are gone.
External image requests remain a documented browser privacy tradeoff. Upstream
engine filtering, missing dates and rate limits cannot be guaranteed by Discover.
