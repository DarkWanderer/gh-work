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

- `internal/target` — sealed `Target` (`Org | Repo | PR`) and its parser.
- `internal/work` — sealed `Item` (`ReviewThread | PRCheckFailure |
  BranchCheckFailure | MergeConflict`), the `Check`/conclusion types, and
  JSON marshaling.
- `internal/github` — the `Client` interface (one method: `Query`) over
  go-gh's GraphQL client, plus `Paginate`/`PaginateFrom`.
- `internal/github/fake` — a test double for `Client` that dispatches
  fixtures by GraphQL operation name + exact variables.
- `internal/source` — the `Source` plugin interface.
- `internal/source/{reviewthreads,prchecks,branchchecks,mergeconflicts}` —
  one source per package, each owning its own GraphQL queries and fixtures.
- `internal/collect` — runs the sources that support a target concurrently,
  merges items/warnings/errors, sorts items deterministically.
- `internal/render` — the `Envelope` type and its JSON/text renderers.
- `internal/cmd` — cobra wiring, flag validation, the `--watch` loop, exit
  codes.

## Adding a work source

1. Add a new `work.Item` variant in `internal/work` (a struct with the
   fields the JSON contract needs, plus a `MarshalJSON` that adds `"kind"`)
   and document its shape in `docs/output.md`.
2. Create `internal/source/<name>/` implementing `source.Source`:
   `Name()`, `Supports(target.Target) bool`, and `Collect(...)` — own your
   GraphQL query and pagination; don't share query strings with other
   sources even if they look similar (decoupling over DRY here is
   deliberate, so one source's schema change can't break another's tests).
3. Add fixture-driven tests under `internal/source/<name>/testdata/`
   covering pagination and whatever failure/classification logic applies.
4. Register it in `internal/cmd/sources.go`'s `allSources` slice — that's
   the one place `--source` and the default "run everything" set come from.
5. Add a case to `internal/render`'s text grouping/formatting for the new
   item's shape.

## Design notes

- `Target` and `Item` are sealed interfaces (unexported marker methods) so
  the type system rules out, e.g., a check failure with no repo, or a PR
  target combined with `--filter`.
- Sources run concurrently and are independent by design: `review-threads`,
  `pr-checks` and `merge-conflicts` each run their own PR search rather than
  sharing one, so a source can be changed or reworked without touching its
  neighbors.
- Item sort order (repo, then PR number or branch, then kind, then id) is
  produced once in `internal/collect`, not by callers — `render` assumes
  its input is already in that order.
