package check

import (
	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// MergeConflicts reports an open PR whose mergeable state is CONFLICTING.
// GitHub search has no qualifier for this, so the PR scan lists open PRs
// and this check filters on mergeable client-side.
type MergeConflicts struct{}

func (MergeConflicts) Name() string       { return "merge-conflicts" }
func (MergeConflicts) Needs() scan.Fields { return scan.Mergeable }

// Inspect returns ok=false for anything other than a confirmed conflict.
// MERGEABLE (no conflict) and UNKNOWN (GitHub hasn't finished computing it
// yet) are both skipped silently — UNKNOWN resolves itself on a later run.
func (MergeConflicts) Inspect(pr scan.PullRequest) []work.PRItem {
	if pr.Mergeable != "CONFLICTING" {
		return nil
	}
	return []work.PRItem{work.MergeConflict{ID: pr.ID, BaseRef: pr.BaseRef}}
}
