package reviewthreads

import (
	"context"
	"errors"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/github/fake"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestSearchQueryString(t *testing.T) {
	cases := []struct {
		t       target.Target
		filters []string
		want    string
	}{
		{target.Org{Owner: "acme"}, nil, "is:pr is:open archived:false user:acme"},
		{target.Repo{Owner: "acme", Name: "widgets"}, nil, "is:pr is:open archived:false repo:acme/widgets"},
		{target.Org{Owner: "acme"}, []string{"involves:@me"}, "is:pr is:open archived:false user:acme involves:@me"},
	}
	for _, c := range cases {
		if got := searchQueryString(c.t, c.filters); got != c.want {
			t.Errorf("searchQueryString(%#v, %v) = %q, want %q", c.t, c.filters, got, c.want)
		}
	}
}

func TestCollectViaSearch_PaginationAndOverflow(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Org{Owner: "o"}, nil)
	if err := c.SetFixtureFile("SearchReviewThreads", map[string]any{"q": q, "after": nil}, "testdata/search_page1.json"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetFixtureFile("SearchReviewThreads", map[string]any{"q": q, "after": "SEARCH_C1"}, "testdata/search_page2.json"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetFixtureFile("PRReviewThreadsPage", map[string]any{"id": "PR_kwDOA2", "after": "THREADS_C1"}, "testdata/threads_overflow.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// PRRT_2 is resolved and must be dropped; the rest (from both search
	// pages plus the overflow page) must survive.
	wantIDs := []string{"PRRT_1", "PRRT_3", "PRRT_10", "PRRT_11"}
	if len(res.Items) != len(wantIDs) {
		t.Fatalf("got %d items, want %d: %#v", len(res.Items), len(wantIDs), res.Items)
	}
	for i, id := range wantIDs {
		rt, ok := res.Items[i].(work.ReviewThread)
		if !ok {
			t.Fatalf("item %d is not a ReviewThread: %#v", i, res.Items[i])
		}
		if rt.ID != id {
			t.Fatalf("item %d ID = %q, want %q", i, rt.ID, id)
		}
	}

	outdated := res.Items[1].(work.ReviewThread)
	if !outdated.Thread.IsOutdated {
		t.Fatal("PRRT_3 should be marked outdated")
	}
	if outdated.Thread.Line != 0 {
		t.Fatalf("PRRT_3 line = %d, want 0 (null in fixture)", outdated.Thread.Line)
	}
	if outdated.Thread.Comments[0].Author != "" {
		t.Fatalf("PRRT_3 author = %q, want empty (null author in fixture)", outdated.Thread.Comments[0].Author)
	}

	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
}

func TestCollectViaSearch_IssueCountOverCapWarns(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Org{Owner: "o"}, nil)
	c.SetFixture("SearchReviewThreads", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":1500,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))

	res, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 warning about the 1000 result cap", res.Warnings)
	}
}

func TestCollectForPRTarget(t *testing.T) {
	c := fake.NewClient()
	if err := c.SetFixtureFile("PRReviewThreads", map[string]any{"owner": "o", "repo": "r", "number": 9, "after": nil}, "testdata/pr_target.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 9}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("got %d items, want 1: %#v", len(res.Items), res.Items)
	}
	rt := res.Items[0].(work.ReviewThread)
	if rt.ID != "PRRT_90" || rt.Repo != "o/r" || rt.PR.Number != 9 {
		t.Fatalf("item = %#v", rt)
	}
}

func TestCollectForPRTarget_NotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("PRReviewThreads", map[string]any{"owner": "o", "repo": "r", "number": 404, "after": nil},
		[]byte(`{"repository":{"pullRequest":null}}`))

	_, err := New().Collect(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 404}, source.Options{})
	if err == nil {
		t.Fatal("expected error for missing PR")
	}
}

func TestCollectPropagatesQueryError(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Repo{Owner: "o", Name: "r"}, nil)
	want := errors.New("boom")
	c.SetFixtureError("SearchReviewThreads", map[string]any{"q": q, "after": nil}, want)

	_, err := New().Collect(context.Background(), c, target.Repo{Owner: "o", Name: "r"}, source.Options{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestSupportsAndName(t *testing.T) {
	s := New()
	if s.Name() != "review-threads" {
		t.Fatalf("Name() = %q", s.Name())
	}
	for _, tg := range []target.Target{target.Org{Owner: "o"}, target.Repo{Owner: "o", Name: "r"}, target.PR{Owner: "o", Name: "r", Number: 1}} {
		if !s.Supports(tg) {
			t.Errorf("Supports(%#v) = false, want true", tg)
		}
	}
}
