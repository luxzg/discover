# Discover Agent Instructions

Project-specific instructions for AI coding agents working in this repository.

These rules are derived from the global skill rulebook and the current project
workflow documented in `README.md`, `INSTALL.md`, `USAGE.md`, and
`CHANGELOG.md`.

## Rule Precedence

1. Direct user instructions in the current chat.
2. This file (`AGENTS.md`).
3. Project docs (`README.md`, `INSTALL.md`, `USAGE.md`, `TODO.md`, latest `CHANGELOG.md`).
4. Global skill/reference rules (including `AGENT_RULES.md` from the skill).

## Read First (Every New Session)

Read these before substantial work:

1. `AGENTS.md` (this file)
2. `README.md`
3. Latest entries in `CHANGELOG.md`
4. `INSTALL.md`
5. `USAGE.md`
6. `TODO.md` (active ideas only)
7. `SQLITE_DEBUG.md` when debugging DB/data behavior
8. `SEARXNG.md` when touching SearXNG setup assumptions
9. `DEVELOPMENT.md` and `MAINTENANCE.md` for tooling, dependency or validation work

Historical context docs (read-only unless explicitly requested):

- `INITIAL_PROMPT.md`
- `IDEA.md`
- `JSON_SAMPLES.md`
- `GITHUB.md`

## Scope / Safety

- Do not change app code when the user asks for docs-only work.
- Preserve unrelated working tree changes.
- Never print or commit real secrets, cookies, tokens, or private paths that
  identify user machines/services.
- Treat production logs and DB contents as sensitive; summarize when possible.
- Use destructive commands only with explicit user approval.

## Project Versioning And Changelog

- Version scheme in this repo is app versions like `v2.12` in `CHANGELOG.md`.
- Code/runtime/UI behavior changes: bump to next version and add changelog entry.
- Docs-only changes: no app version bump required, but add a docs changelog entry.
- Changelog entries must include date/time and timezone (for example
  `2026-02-22 09:47 CET`).
- Append entries; do not rewrite previous history unless user asks.

## Build / Validation Defaults

- Full validation: `./scripts/check.sh --race` (vet, tests, scripts, build artifact).
- Vulnerability scan: `bash scripts/security-check.sh`.
- Browser validation: `bash scripts/test-browser.sh --all` after the one-time
  `bash scripts/setup-browser.sh`; repeat setup after lockfile/browser updates.
- Release security check: `bash scripts/security-check.sh --binary` after build.
- Build deployable artifact: `./scripts/build.sh`, then `./discover --version`.
- Formatting: `./scripts/format.sh`.
- If sandbox blocks default Go cache writes, use local cache:
  `GOCACHE=$(pwd)/.gocache go build ./...`
- Remove temporary `.gocache` after validation if created.
- For docs-only changes, skip build unless user requests build/test.

## Periodic Tooling Maintenance

- Follow `MAINTENANCE.md`; when its last dated review is about a month old during
  normal work, remind the user and propose checking tools/dependencies. This does
  not authorize unattended upgrades, OS installation or production deployment.
- Verify supported Go patches with official sources. Respect operator PATH and
  do not raise the module minimum only because the installed compiler changed.
- Include development dependencies in npm audits, keep lockfiles synchronized,
  use `npm ci` for reproduction and matching project-local Playwright browsers.
- Ask the user to install missing system tools/browser OS libraries. Do not assume
  sudo or run browser installation with automatic OS dependency installation.
- Separate reachable Go findings from uncalled advisories; record dated scan
  results and failures. Offline/failed scans are not clean results.
- Browser checks must use synthetic disposable data and test-owned servers, never
  private login state. Keep artifacts ignored, stop owned servers, review screenshots
  and distinguish local deterministic coverage from external-service checks.
- Rebuild committed artifacts after compiler/dependency changes. Remote scripts
  build remotely; ask the operator to deploy and verify version/login/feed rather
  than claiming a laptop compiler update patched the live service.

## Git Workflow Defaults

- Review before commit: `git status --short` and `git diff`.
- Stage intended changes together (project preference has been `git add -A`
  unless user asks otherwise).
- Use concise multiline commit messages that describe the full change set:
  summary line first, then short detail lines when useful.
- Push after commit when user requests it and remote is reachable.

## Command Execution Discipline

- Do not stack commands with shell chaining/operators for normal workflow
  steps (for example `&&`, `||`, `;`) when steps are logically sequential.
- Run sequential operations as separate commands and wait for each command
  result before running the next one.
- Do not run steps in parallel when there is an order dependency between them
  (for example stage -> commit -> push, or stop service -> build -> start
  service).
- Prefer one command per tool call unless there is a clear, safe reason to
  combine independent read-only checks.

## Editing Tools And Permissions

- Use native `apply_patch` for ordinary file edits and creation. Through
  `functions.exec`, call `tools.apply_patch` with a patch string; call
  `tools.exec_command` with an argument object for reads and commands. Do not
  assume a shell executable named `apply_patch` exists.
- Use `rg`, `cat`, or `sed` for reading/searching. Do not use ad hoc `python3`
  commands, shell redirection, or escalated shell scripts for routine edits
  that native patches can handle. Group related edits into coherent patches.
- Check the effective session sandbox and writable roots, not just Linux file
  ownership or the UI permission label. In this project's September 2026
  troubleshooting, one session was read-only while another was workspace-write;
  the same native editing tool was subject to different approval requirements.
- A writable repository does not imply writable Git metadata or unrestricted
  network access. Use ordinary workspace permissions first and request scoped
  escalation only when the operation actually requires it under tool policy.
- If ordinary edits unexpectedly require repeated approvals, pause editing and
  agent writes, identify the exact tool result and effective permission profile,
  and report the blocker. Distinguish sandbox restrictions from filesystem
  permissions, invalid patch syntax, and unmatched patch context. Do not suggest
  `chmod`, `chown`, or Full Access without evidence that it is necessary.
- Do not bypass denied edits by switching to Python, shell writes, or another
  agent. Do not promise approval-free execution when the session policy still
  requires approval, or alter Codex permission settings without user direction.
- Keep one writer by default: the coordinating agent applies edits; subagents
  inspect, review, and propose changes read-only unless the user explicitly
  approves multiple writers. Pass these constraints to every subagent.
- Use reviewed helper scripts in `scripts/` for repeated tests/builds/checks.
  Where approval is necessary, prefer a narrowly scoped reusable command rule;
  never request blanket approval for an interpreter or arbitrary shell code.
- Verify edits with `git diff` and `git diff --check` before reporting success.

## Commit Discipline

- Default behavior: after any code/runtime/UI change that includes version bump
  + changelog update, commit in the same turn automatically.
- Exception: do not commit only when user explicitly says `no commit`,
  `don't commit`, or equivalent.
- When the user says `commit`, perform the commit in that same turn unless a
  hard environment blocker prevents it.
- If commit fails, report the exact error and retry when user asks; do not
  claim completion until commit actually succeeds.
- After commit, always report the commit hash and what was included.

## Versioning Verification Discipline

- For any code/runtime change requiring version bump, update all version touch
  points in the same change:
  - `CHANGELOG.md` new version entry
  - `internal/buildinfo/buildinfo.go` `Version`
- Before marking task done, verify version consistency by checking both files.
- Never announce a new version complete if runtime/buildinfo version still
  points to previous release.

## Discover-Specific Operational Notes

- Existing `config.json` must never be overwritten on startup.
- Admin and user auth are separate:
  - feed uses `user_name` + `user_secret`
  - admin uses `admin_secret`
- Ingest and dedupe behavior are user-visible and should be reflected in docs
  whenever changed.
- Keep install/update/uninstall/diagnostics commands in `INSTALL.md` aligned
  with actual workflow.
- Keep SQL/debug tips aligned in `SQLITE_DEBUG.md` when schema/behavior changes.
- Automatic duplicate hides are derived state, not evidence that a story was
  handled. Rule/headline/key changes must reconsider the representative without
  resetting the all-time dedupe counter or undoing deliberate hides.
- Preserve legacy scores as a baseline; repeated identical results must not
  inflate scores. Never substitute ingestion time for an unknown publication date.
- Field-tested databases have tens of thousands of unread articles. Validate
  interactive scoring changes with `bash scripts/test-hide-scale.sh`; do not
  rematch every rule when only one rule changed or hide slow logic behind a
  larger HTTP timeout. Long-running accepted mutations must be service-owned,
  bounded, observable and independent of browser request cancellation, with
  clear acceptance versus completion messages and transactional rollback.
  The user accepts cancellation on rare service restarts; do not add a durable
  hide-job queue without a new requirement.
- Privileged deployment orchestration must come from an administrator-controlled
  copy. Never run service-owned scripts or candidate binaries as root. Run build,
  candidate inspection and service-checkout file operations as the service user.

## Documentation Alignment Rule

When workflow or behavior changes, update relevant docs in the same task:

- `README.md` for high-level behavior/features.
- `INSTALL.md` for deployment/ops/diagnostics.
- `DEVELOPMENT.md` and `MAINTENANCE.md` for validation, periodic checks and results.
- `USAGE.md` for feed/admin usage.
- `SQLITE_DEBUG.md` for DB debugging workflows.
- `TODO.md` for active future work only.
- `FINISHED_TASKS.md` for completed, skipped, dropped, or intentionally closed
  TODO items.
- `CHANGELOG.md` for timeline entries.

When the user closes a TODO item without implementation, remove it from
`TODO.md`, add a short rationale to `FINISHED_TASKS.md`, and record the docs
change in `CHANGELOG.md`.
