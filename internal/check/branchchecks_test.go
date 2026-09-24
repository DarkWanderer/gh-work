package check

import (
	"testing"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestBranchChecksName(t *testing.T) {
	if (BranchChecks{}).Name() != "branch-checks" {
		t.Fatalf("Name() = %q", (BranchChecks{}).Name())
	}
}

func TestBranchChecksFiltersToFailingContexts(t *testing.T) {
	b := scan.Branch{
		Repo: "o/r", Name: "main",
		Tip: &scan.Commit{Oid: "def5678", Contexts: []scan.CheckContext{
			fakeContext{id: "SC_1", failing: true, check: work.Check{Name: "ci/status", Conclusion: "ERROR"}},
			fakeContext{id: "SC_2", failing: false, check: work.Check{Name: "ci/other", Conclusion: "SUCCESS"}},
		}},
	}
	failures := BranchChecks{}.Inspect(b)
	if len(failures) != 1 {
		t.Fatalf("got %d failures, want 1: %#v", len(failures), failures)
	}
	if failures[0].ID != "SC_1" || failures[0].Commit != "def5678" || failures[0].Check.Name != "ci/status" {
		t.Fatalf("failure = %#v", failures[0])
	}
}

func TestBranchChecksNilTipReturnsNil(t *testing.T) {
	if failures := (BranchChecks{}).Inspect(scan.Branch{}); len(failures) != 0 {
		t.Fatalf("failures = %#v, want none", failures)
	}
}
