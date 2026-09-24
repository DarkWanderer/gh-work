package collect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DarkWanderer/gh-work/internal/check"
	"github.com/DarkWanderer/gh-work/internal/collect"
	"github.com/DarkWanderer/gh-work/internal/github/fake"
	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// blockingPRCheck lets a test observe that the PR and branch scans run
// concurrently, by blocking Inspect until released.
type blockingPRCheck struct {
	started chan struct{}
	release chan struct{}
}

func (blockingPRCheck) Name() string       { return "blocking-pr" }
func (blockingPRCheck) Needs() scan.Fields { return 0 }
func (b blockingPRCheck) Inspect(scan.PullRequest) []work.PRItem {
	b.started <- struct{}{}
	<-b.release
	return nil
}

type blockingBranchCheck struct {
	started chan struct{}
	release chan struct{}
}

func (blockingBranchCheck) Name() string { return "blocking-branch" }
func (b blockingBranchCheck) Inspect(scan.Branch) []work.CheckFailure {
	b.started <- struct{}{}
	<-b.release
	return nil
}

func emptySearchAndOwnerFixtures(c *fake.Client, owner string) {
	q := "is:pr is:open archived:false user:" + owner
	c.SetFixture("SearchPullRequests", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	c.SetFixture("OwnerRepositories", map[string]any{"login": owner, "after": nil},
		[]byte(`{"repositoryOwner":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}`))
}

func TestRunScansConcurrently(t *testing.T) {
	c := fake.NewClient()
	emptySearchAndOwnerFixtures(c, "o")

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	checks := check.Set{
		PR:     []check.PRCheck{blockingPRCheck{started: started, release: release}},
		Branch: []check.BranchCheck{blockingBranchCheck{started: started, release: release}},
	}

	go func() {
		<-started
		<-started
		close(release)
	}()

	done := make(chan collect.Result, 1)
	go func() {
		done <- collect.Run(context.Background(), c, target.Org{Owner: "o"}, checks, nil)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not complete; PR and branch scans likely ran sequentially and deadlocked on the release barrier")
	}
}

func TestRunOnlySelectedChecksScan(t *testing.T) {
	c := fake.NewClient()
	// Only the PR search fixture is registered; if branch checks were
	// (wrongly) selected, the missing OwnerRepositories fixture would
	// surface as an error.
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))

	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.Set{PR: []check.PRCheck{check.ReviewThreads{}}}, nil)
	if len(res.Errors) != 0 {
		t.Fatalf("errors = %#v, want none", res.Errors)
	}
}

func TestRunEmptyCheckSetScansNothing(t *testing.T) {
	c := fake.NewClient()
	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.Set{}, nil)
	if len(res.Groups) != 0 || len(res.Errors) != 0 {
		t.Fatalf("res = %#v, want empty", res)
	}
	if len(c.Calls) != 0 {
		t.Fatalf("no scan should have run, got calls %#v", c.Calls)
	}
}

func TestRunGroupsMultiplePRChecksUnderOnePullRequest(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil},
		[]byte(`{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"id":"PR_1","number":1,"title":"t","url":"https://x/1","isDraft":false,"headRefName":"h","baseRefName":"main",
			"mergeable":"CONFLICTING","repository":{"nameWithOwner":"o/r"},
			"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
				{"id":"PRRT_1","isResolved":false,"isOutdated":false,"path":"a.go","line":1,"comments":{"nodes":[{"author":{"login":"bob"},"body":"x","url":"https://x/1#r1","createdAt":"2026-01-01T00:00:00Z"}]}}
			]},
			"commits":{"nodes":[{"commit":{"id":"C_1","oid":"abc","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
				{"__typename":"CheckRun","id":"CR_1","name":"build","conclusion":"FAILURE","databaseId":1,"detailsUrl":"u","checkSuite":null}
			]}}}}]}
		}]}}`))

	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.All, nil)
	if len(res.Groups) != 1 {
		t.Fatalf("got %d groups, want 1 (all 3 PR checks fire on the same PR): %#v", len(res.Groups), res.Groups)
	}
	pr, ok := res.Groups[0].(work.PullRequest)
	if !ok {
		t.Fatalf("group is not a PullRequest: %#v", res.Groups[0])
	}
	if len(pr.Items) != 3 {
		t.Fatalf("got %d items, want 3 (thread + check + conflict): %#v", len(pr.Items), pr.Items)
	}
	// Only one SearchPullRequests call, regardless of 3 PR checks sharing it.
	searchCalls := 0
	for _, call := range c.Calls {
		if call.Operation == "SearchPullRequests" {
			searchCalls++
		}
	}
	if searchCalls != 1 {
		t.Fatalf("SearchPullRequests calls = %d, want 1 (checks must share one fetch)", searchCalls)
	}
}

func TestRunPRWithNoMatchingItemsProducesNoGroup(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil},
		[]byte(`{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"id":"PR_1","number":1,"title":"t","url":"https://x/1","isDraft":false,"headRefName":"h","baseRefName":"main",
			"repository":{"nameWithOwner":"o/r"},
			"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}
		}]}}`))
	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.Set{PR: []check.PRCheck{check.ReviewThreads{}}}, nil)
	if len(res.Groups) != 0 {
		t.Fatalf("groups = %#v, want none", res.Groups)
	}
}

func TestRunPRScanFailureReportsOneErrorPerSelectedPRCheck(t *testing.T) {
	c := fake.NewClient()
	want := errors.New("rate limited")
	c.SetFixtureError("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil}, want)
	c.SetFixture("OwnerRepositories", map[string]any{"login": "o", "after": nil},
		[]byte(`{"repositoryOwner":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"nameWithOwner":"o/r","isArchived":false,"isFork":false,
			"defaultBranchRef":{"name":"main","target":{"id":"C_1","oid":"def","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
				{"__typename":"StatusContext","id":"SC_1","context":"ci/status","state":"FAILURE","targetUrl":"u"}
			]}}}}}]}}}`))

	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.All, nil)

	if len(res.Errors) != 3 {
		t.Fatalf("errors = %#v, want 3 (one per selected PR check)", res.Errors)
	}
	wantSources := map[string]bool{"review-threads": true, "pr-checks": true, "merge-conflicts": true}
	for _, e := range res.Errors {
		if !wantSources[e.Source] {
			t.Errorf("unexpected error source %q", e.Source)
		}
		if e.Message != want.Error() {
			t.Errorf("message = %q, want %q", e.Message, want.Error())
		}
	}
	// Branch check must still have contributed its group despite the PR
	// scan failing.
	if len(res.Groups) != 1 {
		t.Fatalf("groups = %#v, want 1 (branch-checks unaffected by the PR scan failure)", res.Groups)
	}
}

func TestRunSortsGroupsByRepoThenPRNumberOrBranchName(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil},
		[]byte(`{"search":{"issueCount":3,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
			{"id":"PR_10","number":10,"title":"t","url":"u","isDraft":false,"headRefName":"h","baseRefName":"main","repository":{"nameWithOwner":"o/r"},
			 "reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"PRRT_10","isResolved":false,"isOutdated":false,"path":"a","line":1,"comments":{"nodes":[]}}]}},
			{"id":"PR_2","number":2,"title":"t","url":"u","isDraft":false,"headRefName":"h","baseRefName":"main","repository":{"nameWithOwner":"o/r"},
			 "reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"PRRT_2","isResolved":false,"isOutdated":false,"path":"a","line":1,"comments":{"nodes":[]}}]}},
			{"id":"PR_1z","number":1,"title":"t","url":"u","isDraft":false,"headRefName":"h","baseRefName":"main","repository":{"nameWithOwner":"a/first"},
			 "reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"PRRT_1z","isResolved":false,"isOutdated":false,"path":"a","line":1,"comments":{"nodes":[]}}]}}
		]}}`))
	c.SetFixture("OwnerRepositories", map[string]any{"login": "o", "after": nil},
		[]byte(`{"repositoryOwner":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
			{"nameWithOwner":"o/r","isArchived":false,"isFork":false,"defaultBranchRef":{"name":"main","target":{"id":"C_m","oid":"a","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"__typename":"StatusContext","id":"SC_main","context":"x","state":"FAILURE","targetUrl":"u"}]}}}}},
			{"nameWithOwner":"o/r","isArchived":false,"isFork":false,"defaultBranchRef":{"name":"develop","target":{"id":"C_d","oid":"b","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"__typename":"StatusContext","id":"SC_dev","context":"x","state":"FAILURE","targetUrl":"u"}]}}}}}
		]}}}`))

	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.All, nil)

	var got []string
	for _, g := range res.Groups {
		got = append(got, g.GroupRepo()+"|"+g.SortKey())
	}
	want := []string{"a/first|00000000001", "o/r|00000000002", "o/r|00000000010", "o/r|1develop", "o/r|1main"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestRunPRItemsWithinAGroupSortByKindThenID(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil},
		[]byte(`{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"id":"PR_1","number":1,"title":"t","url":"u","isDraft":false,"headRefName":"h","baseRefName":"main",
			"mergeable":"CONFLICTING","repository":{"nameWithOwner":"o/r"},
			"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
				{"id":"PRRT_1","isResolved":false,"isOutdated":false,"path":"a","line":1,"comments":{"nodes":[]}}
			]},
			"commits":{"nodes":[{"commit":{"id":"C_1","oid":"abc","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
				{"__typename":"CheckRun","id":"CR_1","name":"build","conclusion":"FAILURE","databaseId":1,"detailsUrl":"u","checkSuite":null}
			]}}}}]}
		}]}}`))

	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.All, nil)
	pr := res.Groups[0].(work.PullRequest)
	var ids []string
	for _, it := range pr.Items {
		ids = append(ids, string(it.ItemKind())+":"+it.ItemID())
	}
	want := []string{"merge-conflict:PR_1", "pr-check-failure:CR_1", "review-thread:PRRT_1"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("got %v, want %v", ids, want)
		}
	}
}

func TestRunMergesPRScanWarnings(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("SearchPullRequests", map[string]any{"q": "is:pr is:open archived:false user:o", "after": nil},
		[]byte(`{"search":{"issueCount":1500,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	res := collect.Run(context.Background(), c, target.Org{Owner: "o"}, check.Set{PR: []check.PRCheck{check.ReviewThreads{}}}, nil)
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", res.Warnings)
	}
}

func TestHasWork(t *testing.T) {
	if (collect.Result{}).HasWork() {
		t.Fatal("empty result should not have work")
	}
	if !(collect.Result{Groups: []work.Group{work.Branch{Repo: "o/r", Name: "main"}}}).HasWork() {
		t.Fatal("non-empty result should have work")
	}
}
