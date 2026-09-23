package prchecks

import (
	"context"
	"errors"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/github/fake"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestCollectViaSearch_ClassificationAndSkips(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Org{Owner: "o"}, nil)
	if err := c.SetFixtureFile("SearchPRChecks", map[string]any{"q": q, "after": nil}, "testdata/search_page1.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// CR_2 (SUCCESS), CR_3 (pending, null conclusion) and SC_2 (SUCCESS)
	// must be dropped; CR_1 (FAILURE) and SC_1 (ERROR) must survive.
	if len(res.Items) != 2 {
		t.Fatalf("got %d items, want 2: %#v", len(res.Items), res.Items)
	}

	cr, ok := res.Items[0].(work.PRCheckFailure)
	if !ok {
		t.Fatalf("item 0 is not a PRCheckFailure: %#v", res.Items[0])
	}
	if cr.ID != "CR_1" || cr.Check.Conclusion != "FAILURE" || cr.Check.RunID != 100 || cr.Check.JobID != 200 || cr.Check.Workflow != "CI" {
		t.Fatalf("CheckRun item = %#v", cr)
	}
	if cr.Commit != "abc1234" || cr.Repo != "o/r" || cr.PR.Number != 1 {
		t.Fatalf("CheckRun item = %#v", cr)
	}

	sc, ok := res.Items[1].(work.PRCheckFailure)
	if !ok {
		t.Fatalf("item 1 is not a PRCheckFailure: %#v", res.Items[1])
	}
	if sc.ID != "SC_1" || sc.Check.Conclusion != "ERROR" || sc.Check.Name != "ci/legacy" {
		t.Fatalf("StatusContext item = %#v", sc)
	}
	if sc.Check.RunID != 0 || sc.Check.JobID != 0 {
		t.Fatalf("StatusContext item should have no runId/jobId: %#v", sc.Check)
	}
}

func TestCollectForPRTarget_ContextsOverflow(t *testing.T) {
	c := fake.NewClient()
	if err := c.SetFixtureFile("PRChecks", map[string]any{"owner": "o", "repo": "r", "number": 9}, "testdata/pr_target_overflow.json"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetFixtureFile("CommitCheckContextsPage", map[string]any{"id": "C_commit9", "after": "CTX_C1"}, "testdata/contexts_overflow.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 9}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("got %d items, want 2 (one per page): %#v", len(res.Items), res.Items)
	}
	first := res.Items[0].(work.PRCheckFailure)
	second := res.Items[1].(work.PRCheckFailure)
	if first.ID != "CR_9a" || second.ID != "CR_9b" {
		t.Fatalf("ids = %s, %s", first.ID, second.ID)
	}
}

func TestCollectForPRTarget_NotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("PRChecks", map[string]any{"owner": "o", "repo": "r", "number": 404},
		[]byte(`{"repository":{"pullRequest":null}}`))

	_, err := New().Collect(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 404}, source.Options{})
	if err == nil {
		t.Fatal("expected error for missing PR")
	}
}

func TestCollectViaSearch_IssueCountOverCapWarns(t *testing.T) {
	c := fake.NewClient()
	q := searchQueryString(target.Repo{Owner: "o", Name: "r"}, nil)
	c.SetFixture("SearchPRChecks", map[string]any{"q": q, "after": nil},
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
	c.SetFixtureError("SearchPRChecks", map[string]any{"q": q, "after": nil}, want)

	_, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestNoCommitsOrNoRollup(t *testing.T) {
	items, err := checkItemsForPR(context.Background(), fake.NewClient(), "o/r", prNode{})
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Fatalf("expected nil items for PR with no commits, got %#v", items)
	}
}

func TestSupportsAndName(t *testing.T) {
	s := New()
	if s.Name() != "pr-checks" {
		t.Fatalf("Name() = %q", s.Name())
	}
	for _, tg := range []target.Target{target.Org{Owner: "o"}, target.Repo{Owner: "o", Name: "r"}, target.PR{Owner: "o", Name: "r", Number: 1}} {
		if !s.Supports(tg) {
			t.Errorf("Supports(%#v) = false, want true", tg)
		}
	}
}
