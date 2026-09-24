// Package collect runs the applicable sources for a target concurrently and
// merges their output into one deterministically ordered result.
package collect

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// SourceError is a per-source failure that did not prevent other sources
// from contributing items.
type SourceError struct {
	Source  string
	Message string
}

// Result is the merged output of every applicable source.
type Result struct {
	Items    []work.Item
	Warnings []string
	Errors   []SourceError
}

// HasWork reports whether any item was found; --exit-status and --watch key
// off this.
func (r Result) HasWork() bool { return len(r.Items) > 0 }

// Run calls Collect on every source that supports t, concurrently, then
// merges their items (sorted by repo, PR-or-branch, kind, id), warnings and
// errors. A source that returns an error contributes no items but does not
// prevent the others from contributing theirs; its failure is reported in
// Result.Errors instead. Sources and their errors/warnings appear in the
// order they were registered, not completion order, so output stays
// deterministic regardless of scheduling.
func Run(ctx context.Context, gh github.Client, t target.Target, srcs []source.Source, opts source.Options) Result {
	var applicable []source.Source
	for _, s := range srcs {
		if s.Supports(t) {
			applicable = append(applicable, s)
		}
	}

	results := make([]source.Result, len(applicable))
	errs := make([]error, len(applicable))
	var wg sync.WaitGroup
	for i, s := range applicable {
		wg.Add(1)
		go func(i int, s source.Source) {
			defer wg.Done()
			results[i], errs[i] = s.Collect(ctx, gh, t, opts)
		}(i, s)
	}
	wg.Wait()

	var items []work.Item
	var warnings []string
	var sourceErrs []SourceError
	for i, s := range applicable {
		if err := errs[i]; err != nil {
			sourceErrs = append(sourceErrs, SourceError{Source: s.Name(), Message: err.Error()})
			continue
		}
		items = append(items, results[i].Items...)
		warnings = append(warnings, results[i].Warnings...)
	}

	sortItems(items)
	return Result{Items: items, Warnings: warnings, Errors: sourceErrs}
}

func sortItems(items []work.Item) {
	sort.Slice(items, func(i, j int) bool {
		return sortKey(items[i]) < sortKey(items[j])
	})
}

// sortKey builds a single comparable string encoding (repo, PR-or-branch
// group, kind, id) using NUL as a field separator that cannot appear in any
// of the underlying values.
func sortKey(it work.Item) string {
	return it.ItemRepo() + "\x00" + groupKey(it) + "\x00" + string(it.ItemKind()) + "\x00" + it.ItemID()
}

// groupKey sorts PR-numbered items (zero-padded, so numeric order is
// preserved) before branch items, then branches alphabetically.
func groupKey(it work.Item) string {
	switch v := it.(type) {
	case work.ReviewThread:
		return fmt.Sprintf("0%010d", v.PR.Number)
	case work.PRCheckFailure:
		return fmt.Sprintf("0%010d", v.PR.Number)
	case work.MergeConflict:
		return fmt.Sprintf("0%010d", v.PR.Number)
	case work.BranchCheckFailure:
		return "1" + v.Branch
	default:
		return ""
	}
}
