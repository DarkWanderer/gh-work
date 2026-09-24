package scan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/github/fake"
	"github.com/DarkWanderer/gh-work/internal/target"
)

func TestBuildSearchQueryFieldsControlContent(t *testing.T) {
	cases := []struct {
		name   string
		fields Fields
		want   []string
		wantNo []string
	}{
		{"none", 0, []string{"baseRefName", "repository { nameWithOwner }"}, []string{"reviewThreads", "commits", "mergeable"}},
		{"reviewThreads only", ReviewThreads, []string{"reviewThreads"}, []string{"commits", "mergeable"}},
		{"headRollup only", HeadRollup, []string{"commits", "statusCheckRollup"}, []string{"reviewThreads", "mergeable"}},
		{"mergeable only", Mergeable, []string{"mergeable"}, []string{"reviewThreads", "commits"}},
		{"all", ReviewThreads | HeadRollup | Mergeable, []string{"reviewThreads", "commits", "mergeable"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := buildSearchQuery(c.fields)
			for _, want := range c.want {
				if !strings.Contains(q, want) {
					t.Errorf("query missing %q:\n%s", want, q)
				}
			}
			for _, notWant := range c.wantNo {
				if strings.Contains(q, notWant) {
					t.Errorf("query should not contain %q:\n%s", notWant, q)
				}
			}
			if !strings.HasPrefix(q, "query SearchPullRequests(") {
				t.Errorf("operation name changed: %s", q)
			}
		})
	}
}

func TestBuildPRQueryFieldsControlContent(t *testing.T) {
	q := buildPRQuery(ReviewThreads)
	if !strings.Contains(q, "reviewThreads") {
		t.Errorf("query missing reviewThreads:\n%s", q)
	}
	if strings.Contains(q, "mergeable") || strings.Contains(q, "commits") {
		t.Errorf("query should only contain requested fields:\n%s", q)
	}
	if strings.Contains(q, "repository {") {
		t.Errorf("PR-target query should not re-request the repo it already knows:\n%s", q)
	}
	if !strings.HasPrefix(q, "query PullRequest(") {
		t.Errorf("operation name changed: %s", q)
	}
}

func allFieldsClient(t *testing.T) *fake.Client {
	t.Helper()
	c := fake.NewClient()
	q := "is:pr is:open archived:false user:o"
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.SetFixtureFile("SearchPullRequests", map[string]any{"q": q, "after": nil}, "testdata/search_page1.json"))
	must(c.SetFixtureFile("SearchPullRequests", map[string]any{"q": q, "after": "SEARCH_C1"}, "testdata/search_page2.json"))
	must(c.SetFixtureFile("PullRequestReviewThreadsPage", map[string]any{"id": "PR_2", "after": "THREADS_C1"}, "testdata/threads_overflow.json"))
	must(c.SetFixtureFile("CommitCheckContextsPage", map[string]any{"id": "C_1", "after": "CTX_C1"}, "testdata/contexts_overflow.json"))
	return c
}

func TestPullRequestsSearchPaginationAndOverflow(t *testing.T) {
	c := allFieldsClient(t)
	var prs []PullRequest
	warnings, err := PullRequests(context.Background(), c, target.Org{Owner: "o"}, ReviewThreads|HeadRollup|Mergeable, nil, func(pr PullRequest) {
		prs = append(prs, pr)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none (issueCount 3 <= 1000)", warnings)
	}
	if len(prs) != 3 {
		t.Fatalf("got %d PRs, want 3: %#v", len(prs), prs)
	}

	pr1 := prs[0]
	if pr1.ID != "PR_1" || pr1.Repo != "o/r" || pr1.Number != 1 || pr1.Mergeable != "MERGEABLE" {
		t.Fatalf("pr1 = %#v", pr1)
	}
	if len(pr1.ReviewThreads) != 2 {
		t.Fatalf("pr1 threads = %#v, want 2 (scan does not filter resolved threads)", pr1.ReviewThreads)
	}
	if pr1.Head == nil || len(pr1.Head.Contexts) != 2 {
		t.Fatalf("pr1 head = %#v, want 2 contexts (1 first page + 1 overflow)", pr1.Head)
	}
	if pr1.Head.Contexts[1].ID() != "CR_1b" {
		t.Fatalf("pr1 overflow context = %#v", pr1.Head.Contexts[1])
	}

	pr2 := prs[1]
	if pr2.Mergeable != "CONFLICTING" || !pr2.IsDraft {
		t.Fatalf("pr2 = %#v", pr2)
	}
	if len(pr2.ReviewThreads) != 2 {
		t.Fatalf("pr2 threads = %#v, want 2 (1 first page + 1 overflow)", pr2.ReviewThreads)
	}
	if pr2.ReviewThreads[1].ID != "PRRT_21" {
		t.Fatalf("pr2 overflow thread = %#v", pr2.ReviewThreads[1])
	}
	if pr2.ReviewThreads[0].Line != 0 {
		t.Fatalf("pr2 thread[0] line = %d, want 0 (null in fixture)", pr2.ReviewThreads[0].Line)
	}
	if pr2.ReviewThreads[0].Comments[0].Author != "" {
		t.Fatalf("pr2 thread[0] author = %q, want empty (null author in fixture)", pr2.ReviewThreads[0].Comments[0].Author)
	}
	if pr2.Head != nil {
		t.Fatalf("pr2 head = %#v, want nil (statusCheckRollup null in fixture)", pr2.Head)
	}

	pr10 := prs[2]
	if pr10.Number != 10 || pr10.Mergeable != "UNKNOWN" {
		t.Fatalf("pr10 = %#v", pr10)
	}
	// search_page2.json's contexts include one recognized StatusContext and
	// one node with an unrecognized __typename; the registry must silently
	// drop the latter rather than crash or produce a broken CheckContext.
	if len(pr10.Head.Contexts) != 1 || pr10.Head.Contexts[0].ID() != "SC_10" {
		t.Fatalf("pr10 contexts = %#v, want exactly the recognized StatusContext", pr10.Head.Contexts)
	}
}

func TestPullRequestsUnknownTypenameIsSkippedNotDecoded(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false repo:o/r", "after": nil},
		[]byte(`{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"id":"PR_1","number":1,"title":"t","url":"u","isDraft":false,"headRefName":"h","baseRefName":"main",
			"repository":{"nameWithOwner":"o/r"},
			"commits":{"nodes":[{"commit":{"id":"C_1","oid":"abc","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},
				"nodes":[{"__typename":"SomeFutureType","id":"XX_1"}]}}}}]}
		}]}}`))
	var prs []PullRequest
	if _, err := PullRequests(context.Background(), c, target.Repo{Owner: "o", Name: "r"}, HeadRollup, nil, func(pr PullRequest) { prs = append(prs, pr) }); err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].Head == nil || len(prs[0].Head.Contexts) != 0 {
		t.Fatalf("want the unrecognized context dropped entirely, got %#v", prs)
	}
}

func TestPullRequestsFieldsControlWhatsPopulated(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false repo:o/r", "after": nil},
		[]byte(`{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"id":"PR_1","number":1,"title":"t","url":"u","isDraft":false,"headRefName":"h","baseRefName":"main",
			"repository":{"nameWithOwner":"o/r"}
		}]}}`))
	var prs []PullRequest
	if _, err := PullRequests(context.Background(), c, target.Repo{Owner: "o", Name: "r"}, 0, nil, func(pr PullRequest) { prs = append(prs, pr) }); err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	pr := prs[0]
	if pr.ReviewThreads != nil || pr.Head != nil || pr.Mergeable != "" {
		t.Fatalf("unrequested fields should stay zero: %#v", pr)
	}
	if pr.ID != "PR_1" || pr.BaseRef != "main" {
		t.Fatalf("base fields should always be populated: %#v", pr)
	}
}

func TestPullRequestsIssueCountOverCapWarns(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil},
		[]byte(`{"search":{"issueCount":1500,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	warnings, err := PullRequests(context.Background(), c, target.Org{Owner: "o"}, 0, nil, func(PullRequest) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", warnings)
	}
}

func TestPullRequestsForPRTarget(t *testing.T) {
	c := fake.NewClient()
	if err := c.SetFixtureFile("PullRequest", map[string]any{"owner": "o", "repo": "r", "number": 9}, "testdata/pr_target.json"); err != nil {
		t.Fatal(err)
	}
	var prs []PullRequest
	if _, err := PullRequests(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 9}, ReviewThreads|HeadRollup|Mergeable, nil, func(pr PullRequest) { prs = append(prs, pr) }); err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	pr := prs[0]
	if pr.ID != "PR_9" || pr.Repo != "o/r" || pr.Number != 9 || pr.Mergeable != "CONFLICTING" {
		t.Fatalf("pr = %#v", pr)
	}
	if len(pr.ReviewThreads) != 1 || pr.ReviewThreads[0].ID != "PRRT_90" {
		t.Fatalf("pr threads = %#v", pr.ReviewThreads)
	}
}

func TestPullRequestsForPRTargetNotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("PullRequest", map[string]any{"owner": "o", "repo": "r", "number": 404}, []byte(`{"repository":{"pullRequest":null}}`))
	_, err := PullRequests(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 404}, 0, nil, func(PullRequest) {})
	if err == nil {
		t.Fatal("expected error for missing PR")
	}
}

func TestPullRequestsPropagatesQueryError(t *testing.T) {
	c := fake.NewClient()
	want := errors.New("boom")
	c.SetFixtureError("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false repo:o/r", "after": nil}, want)
	_, err := PullRequests(context.Background(), c, target.Repo{Owner: "o", Name: "r"}, 0, nil, func(PullRequest) {})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestPullRequestsOwnerNotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:ghost", "after": nil}, []byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	// SearchPullRequests never fails for an unknown owner (search just
	// returns zero results); only the org/repo listing used by Branches
	// distinguishes "owner not found". This test documents that PRs and
	// Branches diverge here deliberately (search has no such signal).
	if _, err := PullRequests(context.Background(), c, target.Org{Owner: "ghost"}, 0, nil, func(PullRequest) {}); err != nil {
		t.Fatal(err)
	}
}

func TestPullRequestsSearchQueryScopeAndFilters(t *testing.T) {
	cases := []struct {
		name    string
		t       target.Target
		filters []string
		want    string
	}{
		{"org", target.Org{Owner: "acme"}, nil, "is:pr is:open archived:false user:acme"},
		{"repo", target.Repo{Owner: "acme", Name: "widgets"}, nil, "is:pr is:open archived:false repo:acme/widgets"},
		{"org with filter", target.Org{Owner: "acme"}, []string{"involves:@me"}, "is:pr is:open archived:false user:acme involves:@me"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := fake.NewClient()
			client.SetFixture("SearchPullRequests", map[string]any{"q": c.want, "after": nil},
				[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
			if _, err := PullRequests(context.Background(), client, c.t, 0, c.filters, func(PullRequest) {}); err != nil {
				t.Fatalf("query built with unexpected scope/filters, fixture not matched: %v", err)
			}
		})
	}
}

func TestBranchesOrgFiltersArchivedAndForksAndPagination(t *testing.T) {
	c := fake.NewClient()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.SetFixtureFile("OwnerRepositories", map[string]any{"login": "o", "after": nil}, "testdata/owner_repos_page1.json"))
	must(c.SetFixtureFile("OwnerRepositories", map[string]any{"login": "o", "after": "REPOS_C1"}, "testdata/owner_repos_page2.json"))
	must(c.SetFixtureFile("CommitCheckContextsPage", map[string]any{"id": "C_active", "after": "CTX_B1"}, "testdata/contexts_overflow.json"))

	var branches []Branch
	if err := Branches(context.Background(), c, target.Org{Owner: "o"}, func(b Branch) { branches = append(branches, b) }); err != nil {
		t.Fatal(err)
	}

	// o/archived and o/forked must be skipped; o/empty (no defaultBranchRef)
	// and o/nocommits (no target) and o/norollup (no rollup) all visit
	// nothing since there's no check data to inspect.
	if len(branches) != 1 {
		t.Fatalf("got %d branches, want 1: %#v", len(branches), branches)
	}
	b := branches[0]
	if b.Repo != "o/active" || b.Name != "main" {
		t.Fatalf("branch = %#v", b)
	}
	if b.Tip == nil || len(b.Tip.Contexts) != 2 {
		t.Fatalf("tip = %#v, want 2 contexts (1 first page + 1 overflow)", b.Tip)
	}
	if b.Tip.Contexts[1].ID() != "CR_1b" {
		t.Fatalf("overflow context = %#v", b.Tip.Contexts[1])
	}
}

func TestBranchesOwnerNotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("OwnerRepositories", map[string]any{"login": "ghost", "after": nil}, []byte(`{"repositoryOwner":null}`))
	err := Branches(context.Background(), c, target.Org{Owner: "ghost"}, func(Branch) {})
	if err == nil {
		t.Fatal("expected error for missing owner")
	}
}

func TestBranchesRepoTargetOverflowAndNoFilter(t *testing.T) {
	c := fake.NewClient()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Archived, but this is an explicit repo target: unlike Branches for an
	// org, the archived/fork status must not exclude it.
	must(c.SetFixtureFile("RepositoryDefaultBranch", map[string]any{"owner": "o", "name": "billing"}, "testdata/repo_target_overflow.json"))
	must(c.SetFixtureFile("CommitCheckContextsPage", map[string]any{"id": "C_billing", "after": "CTX_BILL1"}, "testdata/contexts_overflow.json"))

	var branches []Branch
	if err := Branches(context.Background(), c, target.Repo{Owner: "o", Name: "billing"}, func(b Branch) { branches = append(branches, b) }); err != nil {
		t.Fatal(err)
	}
	if len(branches) != 1 || branches[0].Repo != "o/billing" || len(branches[0].Tip.Contexts) != 2 {
		t.Fatalf("branches = %#v", branches)
	}
}

func TestBranchesRepoNotFound(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("RepositoryDefaultBranch", map[string]any{"owner": "o", "name": "ghost"}, []byte(`{"repository":null}`))
	err := Branches(context.Background(), c, target.Repo{Owner: "o", Name: "ghost"}, func(Branch) {})
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestBranchesPRTargetIsNoOp(t *testing.T) {
	c := fake.NewClient()
	called := false
	if err := Branches(context.Background(), c, target.PR{Owner: "o", Name: "r", Number: 1}, func(Branch) { called = true }); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("a PR target has no default branch of its own")
	}
	if len(c.Calls) != 0 {
		t.Fatalf("PR target should not issue any query, got %#v", c.Calls)
	}
}

func TestBranchesPropagatesQueryError(t *testing.T) {
	c := fake.NewClient()
	want := errors.New("boom")
	c.SetFixtureError("RepositoryDefaultBranch", map[string]any{"owner": "o", "name": "r"}, want)
	err := Branches(context.Background(), c, target.Repo{Owner: "o", Name: "r"}, func(Branch) {})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestCheckContextFailingAndCheck(t *testing.T) {
	failing := checkRun{id: "CR_1", conclusion: "FAILURE", name: "build", detailsURL: "u", runID: 1, databaseID: 2, workflow: "CI"}
	if !failing.Failing() {
		t.Fatal("FAILURE conclusion should be failing")
	}
	if got := failing.Check(); got.RunID != 1 || got.JobID != 2 || got.Workflow != "CI" {
		t.Fatalf("Check() = %#v", got)
	}
	passing := checkRun{id: "CR_2", conclusion: "SUCCESS"}
	if passing.Failing() {
		t.Fatal("SUCCESS conclusion should not be failing")
	}

	sc := statusContext{id: "SC_1", context: "ci/x", state: "ERROR", targetURL: "u"}
	if !sc.Failing() {
		t.Fatal("ERROR state should be failing")
	}
	if got := sc.Check(); got.Name != "ci/x" || got.Conclusion != "ERROR" {
		t.Fatalf("Check() = %#v", got)
	}
}

func TestFieldsHas(t *testing.T) {
	f := ReviewThreads | Mergeable
	if !f.Has(ReviewThreads) || !f.Has(Mergeable) {
		t.Fatal("Has should report true for included fields")
	}
	if f.Has(HeadRollup) {
		t.Fatal("Has should report false for an excluded field")
	}
	if !f.Has(ReviewThreads | Mergeable) {
		t.Fatal("Has should report true for the exact combination it holds")
	}
	if f.Has(ReviewThreads | HeadRollup) {
		t.Fatal("Has should report false when any requested field is missing")
	}
}
