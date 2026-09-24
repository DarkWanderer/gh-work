package check

import (
	"testing"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestReviewThreadsNameAndNeeds(t *testing.T) {
	c := ReviewThreads{}
	if c.Name() != "review-threads" {
		t.Fatalf("Name() = %q", c.Name())
	}
	if c.Needs() != scan.ReviewThreads {
		t.Fatalf("Needs() = %v", c.Needs())
	}
}

func TestReviewThreadsDropsResolved(t *testing.T) {
	pr := scan.PullRequest{
		URL: "https://github.com/o/r/pull/1",
		ReviewThreads: []scan.ReviewThread{
			{ID: "PRRT_1", Path: "a.go", Line: 14, IsResolved: false, Comments: []work.Comment{{Author: "bob", URL: "https://x/r1"}}},
			{ID: "PRRT_2", Path: "a.go", Line: 20, IsResolved: true, Comments: []work.Comment{{Author: "bob", URL: "https://x/r2"}}},
		},
	}
	items := ReviewThreads{}.Inspect(pr)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1: %#v", len(items), items)
	}
	rt, ok := items[0].(work.ReviewThread)
	if !ok || rt.ID != "PRRT_1" {
		t.Fatalf("item = %#v", items[0])
	}
	if rt.Thread.URL != "https://x/r1" {
		t.Fatalf("URL = %q, want the first comment's URL", rt.Thread.URL)
	}
}

func TestReviewThreadsURLFallsBackToPRWhenNoComments(t *testing.T) {
	pr := scan.PullRequest{
		URL:           "https://github.com/o/r/pull/1",
		ReviewThreads: []scan.ReviewThread{{ID: "PRRT_1", IsResolved: false}},
	}
	items := ReviewThreads{}.Inspect(pr)
	rt := items[0].(work.ReviewThread)
	if rt.Thread.URL != pr.URL {
		t.Fatalf("URL = %q, want PR URL %q", rt.Thread.URL, pr.URL)
	}
}

func TestReviewThreadsNoUnresolvedReturnsNil(t *testing.T) {
	pr := scan.PullRequest{ReviewThreads: []scan.ReviewThread{{ID: "PRRT_1", IsResolved: true}}}
	if items := (ReviewThreads{}).Inspect(pr); len(items) != 0 {
		t.Fatalf("items = %#v, want none", items)
	}
}
