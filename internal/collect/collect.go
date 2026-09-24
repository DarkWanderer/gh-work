// Package collect runs the PR and branch scans applicable to a check.Set
// concurrently — each scan exactly once, no matter how many checks consume
// it — applies every selected check to what they find, and merges the
// result into one deterministically ordered list of work.Group.
package collect

import (
	"context"
	"sort"
	"sync"

	"github.com/DarkWanderer/gh-work/internal/check"
	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// SourceError is a per-check failure that did not prevent other checks from
// contributing groups.
type SourceError struct {
	Source  string
	Message string
}

// Result is the merged output of every selected check.
type Result struct {
	Groups   []work.Group
	Warnings []string
	Errors   []SourceError
}

// HasWork reports whether any group was found; --exit-status and --watch
// key off this.
func (r Result) HasWork() bool { return len(r.Groups) > 0 }

// Run scans t for every field group checks.PR needs (if any PR check is
// selected) and every branch (if any branch check is selected), running
// both scans concurrently, then applies each selected check to what was
// found. A PR (or branch) that no selected check flags contributes no
// group.
//
// A scan failure drops every group that scan would have produced and is
// reported once per check that depended on it — so errors[].source keeps
// meaning "this named check didn't run" even though PR checks now share one
// fetch — but does not prevent the other scan's checks from contributing
// their groups. PR-check errors are always reported before branch-check
// errors (registration order), regardless of which scan's goroutine
// finishes first, so output stays deterministic across runs.
func Run(ctx context.Context, gh github.Client, t target.Target, checks check.Set, filters []string) Result {
	var wg sync.WaitGroup

	// Each goroutine below writes only to its own dedicated variables, so
	// there's no shared mutable state to guard: wg.Wait() is the only
	// synchronization needed before these are read.
	var prGroups, branchGroups []work.Group
	var warnings []string
	var prErr, branchErr error

	if len(checks.PR) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prGroups, warnings, prErr = runPRChecks(ctx, gh, t, checks.PR, filters)
		}()
	}
	if len(checks.Branch) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			branchGroups, branchErr = runBranchChecks(ctx, gh, t, checks.Branch)
		}()
	}
	wg.Wait()

	var errs []SourceError
	if prErr != nil {
		for _, c := range checks.PR {
			errs = append(errs, SourceError{Source: c.Name(), Message: prErr.Error()})
		}
	}
	if branchErr != nil {
		for _, c := range checks.Branch {
			errs = append(errs, SourceError{Source: c.Name(), Message: branchErr.Error()})
		}
	}

	groups := append(prGroups, branchGroups...)
	sortGroups(groups)
	return Result{Groups: groups, Warnings: warnings, Errors: errs}
}

func runPRChecks(ctx context.Context, gh github.Client, t target.Target, checks []check.PRCheck, filters []string) ([]work.Group, []string, error) {
	fields := check.Set{PR: checks}.Fields()
	var groups []work.Group
	warnings, err := scan.PullRequests(ctx, gh, t, fields, filters, func(pr scan.PullRequest) {
		var items []work.PRItem
		for _, c := range checks {
			items = append(items, c.Inspect(pr)...)
		}
		if len(items) == 0 {
			return
		}
		sortPRItems(items)
		groups = append(groups, work.PullRequest{
			ID: pr.ID, Repo: pr.Repo, Number: pr.Number, Title: pr.Title,
			URL: pr.URL, HeadRef: pr.HeadRef, IsDraft: pr.IsDraft, Items: items,
		})
	})
	if err != nil {
		return nil, nil, err
	}
	return groups, warnings, nil
}

func runBranchChecks(ctx context.Context, gh github.Client, t target.Target, checks []check.BranchCheck) ([]work.Group, error) {
	var groups []work.Group
	err := scan.Branches(ctx, gh, t, func(b scan.Branch) {
		var failures []work.CheckFailure
		for _, c := range checks {
			failures = append(failures, c.Inspect(b)...)
		}
		if len(failures) == 0 {
			return
		}
		sortCheckFailures(failures)
		groups = append(groups, work.Branch{Repo: b.Repo, Name: b.Name, Commit: commitOid(b), Failures: failures})
	})
	if err != nil {
		return nil, err
	}
	return groups, nil
}

func commitOid(b scan.Branch) string {
	if b.Tip == nil {
		return ""
	}
	return b.Tip.Oid
}

func sortGroups(groups []work.Group) {
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.GroupRepo() != b.GroupRepo() {
			return a.GroupRepo() < b.GroupRepo()
		}
		return a.SortKey() < b.SortKey()
	})
}

// sortPRItems orders a PR's items by kind (alphabetically: merge-conflict,
// pr-check-failure, review-thread) then id, matching the order the
// original flat-item collect.Run produced.
func sortPRItems(items []work.PRItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ItemKind() != items[j].ItemKind() {
			return items[i].ItemKind() < items[j].ItemKind()
		}
		return items[i].ItemID() < items[j].ItemID()
	})
}

func sortCheckFailures(failures []work.CheckFailure) {
	sort.Slice(failures, func(i, j int) bool { return failures[i].ID < failures[j].ID })
}
