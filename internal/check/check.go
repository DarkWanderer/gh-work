// Package check turns internal/scan's typed GraphQL data into work.PRItem
// and work.CheckFailure values. A check is a pure function: no I/O, no
// pagination — all of that already happened in internal/scan — which makes
// each one trivially unit-testable and means a check can never accidentally
// duplicate a fetch its neighbor already made.
package check

import (
	"sort"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// PRCheck inspects one scanned PR and returns whatever PRItems it finds.
type PRCheck interface {
	// Name identifies the check for --source and for envelope errors.
	Name() string
	// Needs declares which of scan.PullRequest's optional field groups
	// this check reads, so a PR scan only fetches what's selected.
	Needs() scan.Fields
	// Inspect returns pr's items for this check; nil/empty if none.
	Inspect(pr scan.PullRequest) []work.PRItem
}

// BranchCheck inspects one scanned branch and returns whatever check
// failures it finds.
type BranchCheck interface {
	// Name identifies the check for --source and for envelope errors.
	Name() string
	// Inspect returns b's failures for this check; nil/empty if none.
	Inspect(b scan.Branch) []work.CheckFailure
}

// Set is a selection of checks to run: which PR checks (needing one shared
// PR scan) and which branch checks (needing one shared branch scan).
type Set struct {
	PR     []PRCheck
	Branch []BranchCheck
}

// All is every known check, in registration order. Adding a check (see
// AGENTS.md) means: a new work.PRItem/CheckFailure-producing type here, and
// one line in this slice.
var All = Set{
	PR:     []PRCheck{ReviewThreads{}, PRChecks{}, MergeConflicts{}},
	Branch: []BranchCheck{BranchChecks{}},
}

// Names lists every check name in s, in registration order.
func (s Set) Names() []string {
	names := make([]string, 0, len(s.PR)+len(s.Branch))
	for _, c := range s.PR {
		names = append(names, c.Name())
	}
	for _, c := range s.Branch {
		names = append(names, c.Name())
	}
	return names
}

// Select returns the subset of s named by names, preserving s's
// registration order, plus any names that matched nothing.
func (s Set) Select(names []string) (Set, []string) {
	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[n] = true
	}

	var out Set
	for _, c := range s.PR {
		if wanted[c.Name()] {
			out.PR = append(out.PR, c)
			delete(wanted, c.Name())
		}
	}
	for _, c := range s.Branch {
		if wanted[c.Name()] {
			out.Branch = append(out.Branch, c)
			delete(wanted, c.Name())
		}
	}

	if len(wanted) == 0 {
		return out, nil
	}
	unknown := make([]string, 0, len(wanted))
	for n := range wanted {
		unknown = append(unknown, n)
	}
	sort.Strings(unknown)
	return out, unknown
}

// Fields is the union of every selected PR check's Needs, so the shared PR
// scan fetches exactly what's used and nothing more.
func (s Set) Fields() scan.Fields {
	var f scan.Fields
	for _, c := range s.PR {
		f |= c.Needs()
	}
	return f
}
