package check

import (
	"testing"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestPRChecksNameAndNeeds(t *testing.T) {
	c := PRChecks{}
	if c.Name() != "pr-checks" {
		t.Fatalf("Name() = %q", c.Name())
	}
	if c.Needs() != scan.HeadRollup {
		t.Fatalf("Needs() = %v", c.Needs())
	}
}

func TestPRChecksFiltersToFailingContexts(t *testing.T) {
	pr := scan.PullRequest{
		Head: &scan.Commit{Oid: "abc1234", Contexts: []scan.CheckContext{
			fakeContext{id: "CR_1", failing: true, check: work.Check{Name: "build", Conclusion: "FAILURE"}},
			fakeContext{id: "CR_2", failing: false, check: work.Check{Name: "lint", Conclusion: "SUCCESS"}},
		}},
	}
	items := PRChecks{}.Inspect(pr)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1: %#v", len(items), items)
	}
	cf, ok := items[0].(work.PRCheckFailure)
	if !ok || cf.ID != "CR_1" || cf.Commit != "abc1234" || cf.Check.Name != "build" {
		t.Fatalf("item = %#v", items[0])
	}
}

func TestPRChecksNilHeadReturnsNil(t *testing.T) {
	if items := (PRChecks{}).Inspect(scan.PullRequest{}); len(items) != 0 {
		t.Fatalf("items = %#v, want none", items)
	}
}
