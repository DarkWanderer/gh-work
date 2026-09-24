package check

import (
	"testing"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestMergeConflictsNameAndNeeds(t *testing.T) {
	c := MergeConflicts{}
	if c.Name() != "merge-conflicts" {
		t.Fatalf("Name() = %q", c.Name())
	}
	if c.Needs() != scan.Mergeable {
		t.Fatalf("Needs() = %v", c.Needs())
	}
}

func TestMergeConflictsClassification(t *testing.T) {
	cases := []struct {
		mergeable string
		wantItem  bool
	}{
		{"CONFLICTING", true},
		{"MERGEABLE", false},
		{"UNKNOWN", false},
		{"", false},
	}
	for _, c := range cases {
		pr := scan.PullRequest{ID: "PR_1", BaseRef: "main", Mergeable: c.mergeable}
		items := MergeConflicts{}.Inspect(pr)
		if c.wantItem && len(items) != 1 {
			t.Errorf("mergeable=%q: got %d items, want 1", c.mergeable, len(items))
		}
		if !c.wantItem && len(items) != 0 {
			t.Errorf("mergeable=%q: got %d items, want 0", c.mergeable, len(items))
		}
	}
}

func TestMergeConflictsItemFields(t *testing.T) {
	pr := scan.PullRequest{ID: "PR_1", BaseRef: "main", Mergeable: "CONFLICTING"}
	items := MergeConflicts{}.Inspect(pr)
	mc, ok := items[0].(work.MergeConflict)
	if !ok || mc.ID != "PR_1" || mc.BaseRef != "main" {
		t.Fatalf("item = %#v", items[0])
	}
}
