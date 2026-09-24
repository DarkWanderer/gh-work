package mergeconflicts

import (
	"context"
	"errors"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/github/fake"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestCollectViaSearch_ClassificationAndPagination(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Org{Owner: "o"}, nil)
	if err := c.SetFixtureFile("SearchMergeConflicts", map[string]any{"q": q, "after": nil}, "testdata/search_page1.json"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetFixtureFile("SearchMergeConflicts", map[string]any{"q": q, "after": "CURSOR_1"}, "testdata/search_page2.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// PR 2 (MERGEABLE) and PR 3 (UNKNOWN) must be dropped; PR 1 and PR 4
	// (both CONFLICTING, across two pages) must survive.
	if len(res.Items) != 2 {
		t.Fatalf("got %d items, want 2: %#v", len(res.Items), res.Items)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", res.Warnings)
	}

	first, ok := res.Items[0].(work.MergeConflict)
	if !ok {
		t.Fatalf("item 0 is not a MergeConflict: %#v", res.Items[0])
	}
	if first.ID != "PR_kwDOA1" || first.Repo != "o/r" || first.BaseRef != "main" {
		t.Fatalf("item 0 = %#v", first)
	}
	if first.PR.Number != 1 || first.PR.Title != "Conflicting PR" || first.PR.HeadRef != "feature-1" || first.PR.IsDraft {
		t.Fatalf("item 0 PR = %#v", first.PR)
	}

	second, ok := res.Items[1].(work.MergeConflict)
	if !ok {
		t.Fatalf("item 1 is not a MergeConflict: %#v", res.Items[1])
	}
	if second.ID != "PR_kwDOA4" || second.BaseRef != "develop" || second.PR.Number != 4 || !second.PR.IsDraft {
		t.Fatalf("item 1 = %#v", second)
	}
}

func TestCollectForPRTarget_Conflicting(t *testing.T) {
	c := fake.NewClient()
	if err := c.SetFixtureFile("PRMergeConflict", map[string]any{"owner": "o", "repo": "r", "number": 9}, "testdata/pr_target.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 9}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("got %d items, want 1: %#v", len(res.Items), res.Items)
	}
	item, ok := res.Items[0].(work.MergeConflict)
	if !ok {
		t.Fatalf("item is not a MergeConflict: %#v", res.Items[0])
	}
	if item.ID != "PR_kwDOA9" || item.Repo != "o/r" || item.BaseRef != "main" || item.PR.Number != 9 {
		t.Fatalf("item = %#v", item)
	}
}

func TestCollectForPRTarget_Mergeable(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("PRMergeConflict", map[string]any{"owner": "o", "repo": "r", "number": 5},
		[]byte(`{"repository":{"pullRequest":{"id":"PR_5","number":5,"title":"Clean","url":"https://github.com/o/r/pull/5","isDraft":false,"headRefName":"feature-5","baseRefName":"main","mergeable":"MERGEABLE"}}}`))

	res, err := New().Collect(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 5}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("got %d items, want 0: %#v", len(res.Items), res.Items)
	}
}

func TestCollectForPRTarget_NotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("PRMergeConflict", map[string]any{"owner": "o", "repo": "r", "number": 404},
		[]byte(`{"repository":{"pullRequest":null}}`))

	_, err := New().Collect(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 404}, source.Options{})
	if err == nil {
		t.Fatal("expected error for missing PR")
	}
}

func TestCollectViaSearch_IssueCountOverCapWarns(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Repo{Owner: "o", Name: "r"}, nil)
	c.SetFixture("SearchMergeConflicts", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":2000,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))

	res, err := New().Collect(context.Background(), c, target.Repo{Owner: "o", Name: "r"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", res.Warnings)
	}
}

func TestCollectPropagatesQueryError(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Org{Owner: "o"}, nil)
	want := errors.New("boom")
	c.SetFixtureError("SearchMergeConflicts", map[string]any{"q": q, "after": nil}, want)

	_, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestSupportsAndName(t *testing.T) {
	s := New()
	if s.Name() != "merge-conflicts" {
		t.Fatalf("Name() = %q", s.Name())
	}
	for _, tg := range []target.Target{target.Org{Owner: "o"}, target.Repo{Owner: "o", Name: "r"}, target.PR{Owner: "o", Name: "r", Number: 1}} {
		if !s.Supports(tg) {
			t.Errorf("Supports(%#v) = false, want true", tg)
		}
	}
}
