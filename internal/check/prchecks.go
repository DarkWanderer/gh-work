package check

import (
	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// PRChecks reports failing checks on a PR's head commit.
type PRChecks struct{}

func (PRChecks) Name() string       { return "pr-checks" }
func (PRChecks) Needs() scan.Fields { return scan.HeadRollup }

func (PRChecks) Inspect(pr scan.PullRequest) []work.PRItem {
	if pr.Head == nil {
		return nil
	}
	var items []work.PRItem
	for _, ctx := range pr.Head.Contexts {
		if !ctx.Failing() {
			continue
		}
		items = append(items, work.PRCheckFailure{
			CheckFailure: work.CheckFailure{ID: ctx.ID(), Commit: pr.Head.Oid, Check: ctx.Check()},
		})
	}
	return items
}
