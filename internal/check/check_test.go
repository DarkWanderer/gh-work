package check

import (
	"reflect"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// fakeContext is a scan.CheckContext test double, so check tests never
// depend on scan's unexported checkRun/statusContext types.
type fakeContext struct {
	id      string
	failing bool
	check   work.Check
}

func (f fakeContext) ID() string        { return f.id }
func (f fakeContext) Failing() bool     { return f.failing }
func (f fakeContext) Check() work.Check { return f.check }

func TestSetNames(t *testing.T) {
	got := All.Names()
	want := []string{"review-threads", "pr-checks", "merge-conflicts", "branch-checks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestSetSelectSubset(t *testing.T) {
	sel, unknown := All.Select([]string{"review-threads", "branch-checks"})
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none", unknown)
	}
	if len(sel.PR) != 1 || sel.PR[0].Name() != "review-threads" {
		t.Fatalf("sel.PR = %v", sel.PR)
	}
	if len(sel.Branch) != 1 || sel.Branch[0].Name() != "branch-checks" {
		t.Fatalf("sel.Branch = %v", sel.Branch)
	}
}

func TestSetSelectPreservesRegistrationOrder(t *testing.T) {
	sel, _ := All.Select([]string{"merge-conflicts", "review-threads"})
	got := sel.Names()
	want := []string{"review-threads", "merge-conflicts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Select order = %v, want %v (registry order, not request order)", got, want)
	}
}

func TestSetSelectUnknownNames(t *testing.T) {
	_, unknown := All.Select([]string{"review-threads", "bogus", "also-bogus"})
	want := []string{"also-bogus", "bogus"}
	if !reflect.DeepEqual(unknown, want) {
		t.Fatalf("unknown = %v, want %v (sorted)", unknown, want)
	}
}

func TestSetSelectEmptyMeansEmpty(t *testing.T) {
	sel, unknown := All.Select(nil)
	if len(sel.PR) != 0 || len(sel.Branch) != 0 || len(unknown) != 0 {
		t.Fatalf("Select(nil) = %#v, %v, want empty set and no unknowns", sel, unknown)
	}
}

func TestSetFieldsUnionsSelectedChecksNeeds(t *testing.T) {
	sel, _ := All.Select([]string{"merge-conflicts", "review-threads"})
	want := scan.Mergeable | scan.ReviewThreads
	if got := sel.Fields(); got != want {
		t.Fatalf("Fields() = %v, want %v", got, want)
	}
}

func TestSetFieldsEmptyWhenNoPRChecksSelected(t *testing.T) {
	sel, _ := All.Select([]string{"branch-checks"})
	if got := sel.Fields(); got != 0 {
		t.Fatalf("Fields() = %v, want 0", got)
	}
}
