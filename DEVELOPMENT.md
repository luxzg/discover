# Development And Validation

## Grouped Feed Trial

`internal/store/events.go` assigns membership on ingest/title/date changes
and backfills recent candidates once, in resumable batches. Compare against a
fixed anchor, never another member. Bound the candidate lookup by kind, headline
token and time; if ambiguous or crowded, abstain. Both views use the same
ranked/eligible exact-story selection in `stories.go`. Grouped applies a second
partition before LIMIT; only visible leads get normal seen writes. Event
advancement and those writes share a transaction. The matcher adds no provider
traffic. Desktop/mobile browser tests cover opt-in, switching and advancement;
use real-reading feedback before broadening matching.

The 2026-11-22 synthetic `bash scripts/audit-check.sh bench` run measured
1.02-1.07 s per 15-card Grouped query on the development laptop (40,000 URLs,
25 URLs per exact story, four exact stories per event). This measures SQLite
selection and source lookup, not match accuracy or live-server latency.

## Environment

- Go 1.26.8 or newer, Git and Bash.
- Node.js 22+ and npm for JavaScript/deployment-script tests and developer-only
  Playwright browser tests. Unit/script tests themselves use Node built-ins.
- SQLite CLI for operational backups; shellcheck is optional locally.
- Python 3 for synthetic SearXNG helper tests (standard library only). Real
  SearXNG packages/venvs are not installed or updated on the development PC.
- `govulncheck` for the separate dependency/security check: install with
  `go install golang.org/x/vuln/cmd/govulncheck@latest`.

Direct Go dependencies are `modernc.org/sqlite` (pure-Go SQLite) and
`golang.org/x/net` (bounded HTML parsing for article metadata and bundled public-suffix
grouping for domain reports). Other module
requirements are transitive. Builds use `-mod=readonly`; only deliberate dependency
maintenance uses `go mod tidy`/`go get`. Helpers prefer the newest installed stable
SDK under `$HOME/sdk` and set GOTOOLCHAIN=local in that case: a missing required
compiler is an operator action, not an automatic download. On legacy servers
without SDKs, PATH and Go's existing toolchain policy remain unchanged. No
production Go compilation is run as root.

See `MAINTENANCE.md` for monthly tool/dependency reviews, dated scan results,
download prerequisites and the policy for upgrading compilers without needlessly
raising the module minimum. Shared toolchains are operator-managed, not installed
or updated by routine validation. Synthetic compiler doubles isolate HOME as well
as PATH so a real shared SDK cannot bypass them or trigger external requests.

SQLite is compiled into the binary: `modernc.org/sqlite` v1.58.0 bundles engine
3.53.4 and requires `modernc.org/libc` v1.75.6. Keep that exact pair together.
The installed `sqlite3` CLI is a separate backup/debug tool, not the app engine.

## Repeatable Commands

The November audit uses `bash scripts/audit-check.sh`: it selects the already
operator-installed newest stable `$HOME/sdk/go*` when present, otherwise respects PATH.
No tool installation, shell/profile edits or deployment occur. Initial builds may
download locked modules, and security mode contacts advisory/registry services;
those are not offline checks. Its default is the full
race check; named bounded modes include `build`, `browser`, `security`, `scale`,
`scripts`, `js`, `ingest`, `store`, `actions`, `server`, `perf`, `plans` and `bench`. Environment/flags stay inside
the helper so recurring approvals use a stable entry point. Ordinary production
builds use the same shared-SDK selection with legacy server fallback.

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

Browser tools are centrally managed by the operator in `~/dev/playwright-tools`.
`PLAYWRIGHT_MODULE` can select a different absolute Playwright package directory;
helpers default to that shared installation even in an old IDE environment.
`setup-browser.sh` only checks the shared version and matching Chromium; it never
installs or updates anything. Discover no longer pins a separate @playwright/test.
Record the shared version used; operator upgrades intentionally affect all projects.
Browser tests and network vulnerability scans are separate
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

The regular script suite covers SearXNG preflight, candidate-before-stop ordering,
failed download/settings/start/health recovery, explicit rollback, unchanged settings,
symlink refusal and secret-safe settings validation. Privileged locations and
service/package commands are doubled in disposable directories; these tests do
not establish real systemd/pyenv/upstream package compatibility. The small Python
checker is a reviewed operational helper, not an ad hoc file-editing command.
When util-linux `script` is available, a disposable real-Git pseudo-terminal
test verifies that revision reporting bypasses an interactive pager; this one
test is explicitly skipped otherwise. Synthetic deadline tests verify phase
diagnostics and cleanup without waiting two minutes or touching services.

## Architecture And Invariants

### End-To-End Flows And State Ownership

Discover is a small layered monolith, not a crawler or recommendation service
with separately deployed workers. `cmd/discover` composes config, DB/store,
authentication, ingestion, scheduler and HTTP/UI. The store owns scoring, story
identity and state transitions; handlers/JavaScript must not invent parallel rules.

1. Ingestion reserves the scheduler, harvests paced SearXNG JSON, normalizes URL
   identity and records topic/evidence/rule effects transactionally. Dedupe,
   bounded metadata enrichment, archive maintenance and eligible retention follow.
2. A signed-in feed query selects eligible unread story representatives and
   alternate sources. Positive actions record interest/read state while retaining
   the current tile; Load Next advances the batch. Rule hides acknowledge receipt,
   complete service-owned work, then confirm visible-card removal.
3. Admin edits persist day-to-day topics/rules and selected settings in SQLite.
   Boot/TLS/credentials/search transport settings remain in local JSON. Login
   fields authenticate; they do not change server credentials. Background status
   and sessions are memory-only, unlike article history/counters/settings.

`article_topics` preserves many-to-many search provenance and powers per-topic
unread/total statistics without creating one article per query. It is distinct
from strongest per-topic relevance (`article_evidence`) and current negative
effects (`article_rule_effects`). Multiple associations are not permission to
accumulate identical rediscovery boosts indefinitely.

Feed users may create global negative rules through Hide This/Domain by design;
general topic/rule management remains Admin-only. Hidden UI panels are convenience,
not authorization. Configured SearXNG endpoints/DNS are administrator-trusted;
publisher metadata requires the separate public-only fetch boundary. Never replace
that boundary with the trusted search client to improve image/date coverage.

### Superseded Historical Assumptions

Keep the original prompt/sample files unchanged. Later user decisions and release
behavior supersede these earlier assumptions:

| Earlier Idea / Workflow | Current Decision / Reason |
| --- | --- |
| Public SearXNG/RSS and daily-only ingestion | Private/local JSON integration; configurable interval or daily mode. Public anti-bot failures motivated self-hosting, not bypasses. RSS/Atom is future work. |
| URL-only dedupe, or strip every query parameter | Preserve identity parameters; conservative title groups retain alternative URLs. Do not confuse URL identity with story identity or turn on fuzzy grouping silently. |
| Boost score on every repeated hit | Strongest per-topic evidence within the original story window; preserve legacy baselines without continuing runaway inflation. |
| Ingestion time as publication fallback | Unknown publication stays unknown; clearly labeled immutable First seen supplies discovery/age evidence only. |
| Admin URL/header secret and browser localStorage | Separate feed/Admin cookie sessions plus CSRF; no compatibility credential bypass. Never revive the early URL-secret flow. |
| Copy whichever local binary exists; stop before building | Reviewed remote build/config validation before downtime, retained snapshots and atomic swap. A generic successful build once left an old deployable artifact untouched. |
| Service user's scripts/candidate inspection run as root | Administrator-controlled orchestration; build and candidate checks as dedicated service users. Service accounts do not need sudo. |
| Manual testing only; no Node tools at all | Automated synthetic development tests are established. Node/Playwright are developer-only, never a deployed runtime requirement. |
| Version every prompt/document edit | Current AGENTS policy versions code/runtime changes; docs-only timeline entries do not bump the app. |

The documented operational target is Debian/Ubuntu Linux with systemd. Pure-Go
SQLite makes other Go-supported targets plausible, not tested release platforms.
There is no approved cross-platform release matrix or CI implementation. Do not
replace this stack, add Docker/ORM/brokers, or require a proxy to satisfy an old
discussion or generic checklist. Existing thumbnail caps, memory-only jobs and
operator-managed TLS are deliberate proportional choices; see FINISHED_TASKS.md.

### Current Implementation Constraints

The reviewed HTML parser patch can be reproduced with
`bash scripts/update-html-parser.sh apply`; its no-argument mode shows usage.
It updates only x/net to v0.62.0, required sums/transitives, and refuses an
unexpected Go minimum change. This is deliberate networked dependency maintenance,
not an automatic validation step. Run full checks/scans afterward.

- `config`: exclusive default-file creation and read-only validation.
- `auth`: independent in-memory admin/feed sessions, CSRF and login throttling;
  malformed admin allowlists fail closed.
- `scheduler`: one ingestion at a time, asynchronous manual acceptance, 15-second
  completion cooldown, service-owned cancellation and shutdown waiting.
- `ingest`: fixed bounded SearXNG harvest, partial-result preservation and bounded
  error identifiers. Article metadata uses a separate public-only DNS-pinned
  transport, including redirects; local SearXNG is intentionally allowed.
  Individual searches are paced with cancellable delay/jitter and no parallel
  burst; the scheduler bounds manual and scheduled jobs to two hours. Synthetic
  HTTP tests disable pacing explicitly and test waits via an injected waiter.
  Admin engine diagnostics reuse that transport with a fixed day/page-1 sample,
  minimum five-second pacing and bounded, classified observations; no writes.
- `scheduler`: next-run time reflects the actual interval/daily timer. Service-owned
  three-minute diagnostic jobs share ingestion exclusion, have a five-minute
  cooldown and are awaited on shutdown. Due automatic ingestion waits for a check;
  manual ingestion leaves its scheduled timer unchanged. Tests use local synthetic
  HTTP/check callbacks, not real engines or operator sessions.
- Retroactive dedupe shares the scheduler gate and shutdown lifecycle, with a
  ten-minute bound. Admin uses immediate acceptance and memory-only status;
  legacy synchronous callers join the same job. Job results include the counter
  snapshot from the hiding transaction, not a request-owned post-commit query.
  Its 32-entry result cache is transient; no durable job queue was added.
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

Feed selection ranks narrow IDs/score/clock fields before fetching large article
payloads. Other sources use one indexed query for the selected cards, preserving
the per-card 20-source cap/order and image fallback. Whole-history story clocks
remain authoritative; source caps are never clock inputs. Rule effects are read
once per article and written only on change, including first-count transitions
on handled rows. Repeated identical evidence does not rewrite its ledger.

For bounded, synthetic non-race performance comparisons, run separately:

```bash
bash scripts/audit-check.sh perf
bash scripts/audit-check.sh plans
bash scripts/audit-check.sh bench
```

`perf` checks effects/source contracts and legacy counting with the race detector.
`plans` prints plans from an empty temporary schema and asserts indexed source
lookups. `bench` creates disposable 40k rows (~2KiB content each, 25 per group)
and an 80-rule repeated-hit fixture; three samples of three iterations, five-minute
bound, no private DB/network. Allow a few hundred MiB of temporary disk space.
Fixtures are outside timing; output includes ns/op, B/op and allocs/op.

On 2026-11-06, Go 1.26.10 / SQLite driver 1.58.0, median local observations:

| Synthetic Operation | Before | After |
| --- | --- | --- |
| Full feed, 10 cards | 1.540 s | 0.911 s |
| Full feed, 100 cards | 1.577 s | 0.879 s |
| Repeated hit, 80 rules | 2.480 ms | 0.473 ms |

The initial batched join regressed source loading (~0.62s for 100 cards); its plan
scanned status rows. Explicit outer candidate order and the existing story index
reduced that component to ~14.7ms; a plan test guards this choice. These small
synthetic samples are not production guarantees. Whole-history age aggregation
still scales with retained history; no materialized cache or extra DB connection
was introduced. Profile real workload evidence before adding either.

Do not treat an automatically hidden duplicate as proof the story was handled.
Do not infer a publication date from ingestion time. Do not strip arbitrary URL
queries: they can distinguish articles. Never double-apply a negative rule to the
clicked article. Keep URL/article identity distinct from conservative story keys.

First-seen is immutable (including a SQLite update trigger), recovered from
`created_at` on upgrade, and separate from last ingestion and user exposure.
Search evidence can grow only in the configured initial story window; explicit
topic/rule/vote edits remain effective. Feed eligibility and enrichment share a
group age clock, including earlier publication evidence. Age archives are a
separate reversible field, never an implicit vote or handled-story decision.
The feed's freshness rank is derived; stored scores and minimum-score gates stay
unchanged. Admin archive applies its limit and marks in one transaction.

Metadata fetches share one secure HTTP request for date and image extraction;
date-only candidates include already illustrated cards. Per-batch candidate caps
and a persisted one-day retry delay bound work. Publisher JSON-LD/meta parsing
does not execute scripts and does not substitute `dateModified` for publication.
Reading-by-domain merges registrable domains before filtering to at least two
positive reads and limiting to 200 groups; source/rule identity is unchanged.
The bundled PSL retains private hosted-site boundaries. Reports are descriptive,
not an automatically trained ranking model. Tests also cover read-then-hide
precedence, auth/CSRF, archive restoration and 40k-row feed/archive/report timing.
Archive preview and apply share eligibility predicates; preview includes restore
counts without mutating settings or rows. Restored rows do not bypass feed rank,
minimum score, duplicate or handled-history gates.

## Validation Boundaries

Automated tests cover SSRF addressing/redirects, auth separation, CIDRs/CSRF,
request validation, asynchronous jobs, date storage/display, stable scoring,
transaction rollback, dedupe counters, legacy migration, JS actions and deployment
ordering/recovery. Unit JS tests use an isolated DOM model. The separate
Playwright suite renders actual embedded production and plain development builds
in desktop/mobile Chromium, checks assets/console/layout and writes screenshots.
It uses only synthetic fixtures and a fake local SearXNG. Root-path deployment is
tested; subpath deployments, WebGL and workers are not applicable to this app.

Click coverage exercises a real normal-click synthetic publisher tab and actual
authenticated writes for trusted middle-click events on alternate sources.
Headless Chromium did not emit a new-page event for the latter; that check verifies
the uncanceled default and `_blank` target, not physical-browser tab behavior.

After remote deployment, the operator should confirm login restoration after a
fresh sign-in, card/menu/mobile layout, known publication dates, Other sources,
manual ingest completion/warnings, and service version/logs. Actual search quality,
publisher metadata and remote systemd/TLS behavior require field testing.

`TODO.md` tracks future features, not an assertion that all possible bugs are gone.
External image requests remain a documented browser privacy tradeoff. Upstream
engine filtering, missing dates and rate limits cannot be guaranteed by Discover.

### Knowledge-Preservation Review (2026-11-13)

Read all sixteen tracked Markdown documents, the installed coding skill revision
2026-11-08.2, relevant Git history and the available Discover conversation context.
Recovery included project-relevant excerpts of the February-November session's
completed answers and reports from twenty historical child sessions, plus the
available compacted context. This was not a verbatim replay of every tool trace,
attachment or internal reasoning step. Retained compaction messages have incomplete
history flags; independent conversations, missing evidence and the current live
server cannot be assumed recovered. No old raw archive is needed for normal work.

The closed November ledger remains the implementation record. Open product ideas
and conditional scheduling questions belong in TODO.md. Suspect historical dates
and lost exposure evidence require a separate evidence-backed, authorized repair,
not guessed data or mass resets. The next operator checkpoint is README's Current
Status, not a fresh claim that November's local tests establish production health.
This documentation pass did not inspect source/config/DBs or run application validation,
builds, installations, deployment, staging, commits or pushes.
