# gh-work

`gh` CLI extension that lists actionable work — unresolved review threads,
failing PR checks, failing default-branch checks, merge-conflicted PRs — for
an org, repo, or PR.
Built for agents: stable JSON, exit codes, blocking `--watch`.

## Install

```sh
gh extension install DarkWanderer/gh-work
```

## Usage

```sh
gh work [owner|owner/repo|owner/repo#N|url] [--filter Q] [--source list] [--json] [--exit-status] [--watch]
```

Exit codes: `0` ok · `1` API/partial failure · `2` usage error · `8` work found (`--exit-status`) · `130` interrupted.

See [`docs/output.md`](docs/output.md) for the `--json` contract and
[`AGENTS.md`](AGENTS.md) for development.
