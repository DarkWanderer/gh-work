package check

import (
	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// BranchChecks reports failing checks on a repo's default branch tip.
type BranchChecks struct{}

func (BranchChecks) Name() string { return "branch-checks" }

func (BranchChecks) Inspect(b scan.Branch) []work.CheckFailure {
	if b.Tip == nil {
		return nil
	}
	var out []work.CheckFailure
	for _, ctx := range b.Tip.Contexts {
		if !ctx.Failing() {
			continue
		}
		out = append(out, work.CheckFailure{ID: ctx.ID(), Commit: b.Tip.Oid, Check: ctx.Check()})
	}
	return out
}
