# `--json` output contract

`gh work --json` prints one JSON object (schemaVersion 1) to stdout:

```json
{
  "schemaVersion": 1,
  "target": { "kind": "org|repo|pr", "owner": "…", "repo": "…", "number": 1 },
  "items": [ /* see below */ ],
  "warnings": [ "…" ],
  "errors": [ { "source": "branch-checks", "message": "…" } ]
}
```

- `target` describes what was scanned. `repo` and `number` are omitted where
  not applicable (`repo` for an org target, `number` unless `kind` is `pr`).
- `items` is always present, sorted by repo, then PR number or branch name,
  then kind, then id — the order is deterministic across runs given the
  same input.
- `warnings` are non-fatal notices (e.g. GitHub search's 1000-result cap
  was hit) that don't affect the exit code.
- `errors` are per-source failures. A failing source does not drop other
  sources' items — its failure is reported here instead, and the process
  exits 1 (see the "Exit codes" section of the README).

## Item shapes

Every item has `id` (a GitHub GraphQL node ID — stable, so agents can dedupe
across runs), `kind`, and `repo` (`"owner/name"`).

### `review-thread`

An unresolved PR review thread.

```json
{
  "id": "PRRT_…",
  "kind": "review-thread",
  "repo": "owner/repo",
  "pr": { "number": 1, "title": "…", "url": "…", "headRef": "…", "isDraft": false },
  "thread": {
    "path": "src/main.go",
    "line": 14,
    "isOutdated": false,
    "url": "…",
    "comments": [ { "author": "…", "body": "…", "url": "…", "createdAt": "…" } ]
  }
}
```

`thread.line` is omitted for file-level comments. `isOutdated` marks a
thread whose diff context is stale but is still unresolved — it's included,
not filtered out. `thread.url` is the first comment's permalink (review
threads themselves have no URL of their own).

### `pr-check-failure`

A failing check on a PR's head commit.

```json
{
  "id": "CR_…",
  "kind": "pr-check-failure",
  "repo": "owner/repo",
  "pr": { "number": 1, "title": "…", "url": "…", "headRef": "…", "isDraft": false },
  "commit": "sha",
  "check": { "name": "…", "conclusion": "FAILURE", "url": "…", "workflow": "CI", "runId": 123, "jobId": 456 }
}
```

### `branch-check-failure`

A failing check on a repo's default branch tip.

```json
{
  "id": "CR_…",
  "kind": "branch-check-failure",
  "repo": "owner/repo",
  "branch": "main",
  "commit": "sha",
  "check": { "name": "…", "conclusion": "ERROR", "url": "…" }
}
```

### `merge-conflict`

An open PR whose mergeable state is CONFLICTING.

```json
{
  "id": "PR_…",
  "kind": "merge-conflict",
  "repo": "owner/repo",
  "pr": { "number": 1, "title": "…", "url": "…", "headRef": "…", "isDraft": false },
  "baseRef": "main"
}
```

GitHub search has no qualifier for merge conflicts, so this source lists
open PRs and checks each one's mergeable state itself. A PR GitHub hasn't
finished computing mergeability for (`UNKNOWN`), or one it hasn't recomputed
yet after a push, is omitted; it shows up on a later run once GitHub
resolves it.

### `check`

`check.conclusion` is the raw GitHub value: for a GitHub Actions CheckRun,
one of `FAILURE`, `TIMED_OUT`, `STARTUP_FAILURE`, `ACTION_REQUIRED`; for a
commit StatusContext (e.g. a legacy CI integration), `FAILURE` or `ERROR`.
Passing, pending, cancelled, skipped and neutral checks are never reported.

`workflow`, `runId` and `jobId` are present only for GitHub Actions checks
(CheckRun), never for a commit status. `runId` is the Actions run ID (feeds
`gh run view <runId> --log-failed`); `jobId` is the specific job's ID
(`gh run view --job=<jobId> --log`).
