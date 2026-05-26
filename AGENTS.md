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

- Build command: `go build ./...`
- Formatting: `gofmt -w <changed_go_files>`
- If sandbox blocks default Go cache writes, use local cache:
  `GOCACHE=$(pwd)/.gocache go build ./...`
- Remove temporary `.gocache` after validation if created.
- For docs-only changes, skip build unless user requests build/test.

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

## Documentation Alignment Rule

When workflow or behavior changes, update relevant docs in the same task:

- `README.md` for high-level behavior/features.
- `INSTALL.md` for deployment/ops/diagnostics.
- `USAGE.md` for feed/admin usage.
- `SQLITE_DEBUG.md` for DB debugging workflows.
- `TODO.md` for active future work only.
- `CHANGELOG.md` for timeline entries.
