# gh-work

A `gh` CLI extension. See [`README.md`](README.md) for usage and
[`docs/output.md`](docs/output.md) for the `--json` contract.

## Dev commands

```sh
go build -o gh-work .                                  # build the binary
go test ./...                                           # unit tests
go test -run=^$ -bench=. -benchtime=1x ./...             # benchmark smoke test
go test -race -covermode=atomic -coverprofile=coverage.out ./...  # what CI runs
gofmt -l .                                               # formatting check (CI fails if non-empty)
go vet ./...
golangci-lint run                                        # if installed locally
```

To try it against real data without installing the extension:

```sh
go build -o gh-work . && ./gh-work owner/repo
```

## Layout

- `internal/target` — sealed `Target` (`Org | Repo | PR`), its parser, and
  the `Visitor` interface callers implement instead of a type switch.
- `internal/work` — sealed `Group` (`PullRequest | Branch`) and sealed
  `PRItem` (`ReviewThread | PRCheckFailure | MergeConflict`) nested inside a
  `PullRequest`, plus their `GroupVisitor`/`PRItemVisitor` interfaces and the
  shared `Check`/conclusion types.
- `internal/github` — the `Client` interface (one method: `Query`) over
  go-gh's GraphQL client, plus `Paginate`/`PaginateFrom`.
- `internal/github/fake` — a test double for `Client` that dispatches
  fixtures by GraphQL operation name + exact variables.
- `internal/scan` — the only package that speaks GraphQL: `PullRequests`
  pages a target's open PRs once (fetching only the field groups a selected
  check needs), `Branches` pages default-branch tips once, regardless of how
  many checks consume either.
- `internal/check` — `PRCheck`/`BranchCheck`: pure functions from
  `scan.PullRequest`/`scan.Branch` to `work.PRItem`/`work.CheckFailure`, no
  I/O. `check.All` is the registry.
- `internal/collect` — runs the PR scan (if any PR check is selected) and
  the branch scan (if any branch check is selected) concurrently, applies
  every selected check, and sorts the resulting `work.Group`s.
- `internal/render` — the `Envelope` type; `JSON` flattens groups into the
  flat v1 `--json` contract, `Text` renders grouped, optionally colored text.
- `internal/cmd` — cobra wiring, flag validation, the `--watch` loop, exit
  codes.

## Adding a check

1. Add a new `work.PRItem` variant in `internal/work` (or, for a branch
   check, reuse `work.CheckFailure`) and document its `--json` shape in
   `docs/output.md`.
2. If it needs data `scan.PullRequest`/`scan.Branch` doesn't already carry,
   add it to `internal/scan`: a new `scan.Fields` bit and its GraphQL
   fragment (PR checks only — branch scans always fetch the full rollup).
3. Implement `check.PRCheck` or `check.BranchCheck` in `internal/check`: a
   pure function, no I/O — `Inspect` just reads the typed `scan.PullRequest`/
   `scan.Branch` it's given.
4. Add table-driven tests in `internal/check` (no fixtures needed, since
   `Inspect` takes typed data directly) covering its classification logic.
5. Register it in `internal/check.All` — that's the one place `--source`
   and the default "run everything" set come from.
6. Add a case to `internal/render`'s `v1Builder`/`itemLineWriter` (PRCheck)
   or `textWriter.VisitBranch` (BranchCheck) for the new shape.

## Design notes

- `Target`, `Group` and `PRItem` are sealed interfaces (unexported marker
  methods) so the type system rules out, e.g., a check failure with no repo,
  or a PR target combined with `--filter`. Variant-specific behavior goes
  through each interface's `Visitor` (`Accept(v Visitor)`), never a type
  switch, so adding a variant is a compile error in every visitor until it's
  updated.
- PR checks share one `scan.PullRequests` walk and branch checks share one
  `scan.Branches` walk — each target is fetched exactly once no matter how
  many checks are selected. `scan.Fields` lets a check declare only the
  field groups it needs, so `--source merge-conflicts` alone doesn't pay for
  review-thread or check-rollup data.
- Groups are pre-sorted by `internal/collect` (repo, then PR number or
  branch name, then — within a PR — kind then id) so `render` never
  re-sorts; it just walks `Group`/`PRItem` in the order it was given.
- The `--json` contract stays the flat v1 shape (`docs/output.md`) even
  though the internal model is grouped: `render.v1Builder` flattens a
  `work.Group` back into one item per `PRItem`/`CheckFailure`, repeating the
  enclosing PR/branch fields the way v1 always did.
