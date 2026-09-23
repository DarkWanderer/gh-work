package branchchecks

import (
	"context"
	"errors"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/github/fake"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestCollectForOrg_FiltersArchivedAndForksAndPagination(t *testing.T) {
	c := fake.NewClient()
	if err := c.SetFixtureFile("OrgRepositories", map[string]any{"login": "o", "after": nil}, "testdata/org_repos_page1.json"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetFixtureFile("OrgRepositories", map[string]any{"login": "o", "after": "REPO_C1"}, "testdata/org_repos_page2.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// archived-repo and forked-repo must be skipped entirely; pages must
	// be a no-op (nil rollup); grafana-iac's only check is SUCCESS so it
	// contributes nothing; only billing's FAILURE survives.
	if len(res.Items) != 1 {
		t.Fatalf("got %d items, want 1: %#v", len(res.Items), res.Items)
	}
	bc := res.Items[0].(work.BranchCheckFailure)
	if bc.ID != "SC_billing1" || bc.Repo != "o/billing" || bc.Branch != "main" || bc.Commit != "cafebabe" {
		t.Fatalf("item = %#v", bc)
	}
	if bc.Check.Conclusion != "FAILURE" {
		t.Fatalf("check = %#v", bc.Check)
	}
}

func TestCollectForOrg_OwnerNotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("OrgRepositories", map[string]any{"login": "ghost", "after": nil}, []byte(`{"repositoryOwner":null}`))

	_, err := New().Collect(context.Background(), c, target.Org{Owner: "ghost"}, source.Options{})
	if err == nil {
		t.Fatal("expected error for missing owner")
	}
}

func TestCollectForRepo_ContextsOverflowAndNoFilter(t *testing.T) {
	c := fake.NewClient()
	if err := c.SetFixtureFile("RepoDefaultBranchChecks", map[string]any{"owner": "o", "name": "billing"}, "testdata/repo_target_overflow.json"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetFixtureFile("CommitCheckContextsPage", map[string]any{"id": "C_billing2", "after": "CTX_C1"}, "testdata/contexts_overflow.json"); err != nil {
		t.Fatal(err)
	}

	res, err := New().Collect(context.Background(), c, target.Repo{Owner: "o", Name: "billing"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("got %d items, want 2 (one per page): %#v", len(res.Items), res.Items)
	}
	first := res.Items[0].(work.BranchCheckFailure)
	second := res.Items[1].(work.BranchCheckFailure)
	if first.ID != "CR_b1" || second.ID != "SC_b2" {
		t.Fatalf("ids = %s, %s", first.ID, second.ID)
	}
	if first.Check.RunID != 350 || first.Check.JobID != 400 {
		t.Fatalf("CheckRun ids = %#v", first.Check)
	}
}

func TestCollectForRepo_NotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("RepoDefaultBranchChecks", map[string]any{"owner": "o", "name": "missing"}, []byte(`{"repository":null}`))

	_, err := New().Collect(context.Background(), c, target.Repo{Owner: "o", Name: "missing"}, source.Options{})
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestCollectPropagatesQueryError(t *testing.T) {
	c := fake.NewClient()
	want := errors.New("boom")
	c.SetFixtureError("OrgRepositories", map[string]any{"login": "o", "after": nil}, want)

	_, err := New().Collect(context.Background(), c, target.Org{Owner: "o"}, source.Options{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestSupportsAndName(t *testing.T) {
	s := New()
	if s.Name() != "branch-checks" {
		t.Fatalf("Name() = %q", s.Name())
	}
	if !s.Supports(target.Org{Owner: "o"}) {
		t.Error("should support org target")
	}
	if !s.Supports(target.Repo{Owner: "o", Name: "r"}) {
		t.Error("should support repo target")
	}
	if s.Supports(target.PR{Owner: "o", Name: "r", Number: 1}) {
		t.Error("should not support PR target")
	}
}
