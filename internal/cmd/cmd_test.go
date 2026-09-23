package cmd

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DarkWanderer/gh-work/internal/github/fake"
)

// noWorkFixtures registers empty-but-well-formed responses for every
// source, for a given target shape, so RunContext exercises the full
// collect->render pipeline without finding any work.
func emptySearchFixtures(c *fake.Client, owner string) {
	q := "is:pr is:open archived:false user:" + owner
	c.SetFixture("SearchReviewThreads", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	c.SetFixture("SearchPRChecks", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	c.SetFixture("OrgRepositories", map[string]any{"login": owner, "after": nil},
		[]byte(`{"repositoryOwner":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}`))
}

func withWorkFixtures(c *fake.Client, owner string) {
	q := "is:pr is:open archived:false user:" + owner
	c.SetFixture("SearchReviewThreads", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	c.SetFixture("SearchPRChecks", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	c.SetFixture("OrgRepositories", map[string]any{"login": owner, "after": nil},
		[]byte(`{"repositoryOwner":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"nameWithOwner":"`+owner+`/r","isArchived":false,"isFork":false,
			"defaultBranchRef":{"name":"main","target":{"id":"C_1","oid":"deadbeef","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
				{"__typename":"StatusContext","id":"SC_1","context":"ci/status","state":"FAILURE","targetUrl":"https://ci.example.com/1"}
			]}}}}}]}}}`))
}

func TestRunContext_NoWorkExitsZero(t *testing.T) {
	c := fake.NewClient()
	emptySearchFixtures(c, "acme")
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme"}, GH: c, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No work found") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunContext_ExitStatusWithWork(t *testing.T) {
	c := fake.NewClient()
	withWorkFixtures(c, "acme")
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--exit-status"}, GH: c, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 8 {
		t.Fatalf("code = %d, want 8; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestRunContext_ExitStatusWithoutFlagStillZero(t *testing.T) {
	c := fake.NewClient()
	withWorkFixtures(c, "acme")
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme"}, GH: c, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0 (work found but --exit-status not passed)", code)
	}
}

func TestRunContext_JSONOutput(t *testing.T) {
	c := fake.NewClient()
	withWorkFixtures(c, "acme")
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--json"}, GH: c, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schemaVersion": 1`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunContext_SourceFilter(t *testing.T) {
	c := fake.NewClient()
	q := "is:pr is:open archived:false user:acme"
	// only register the review-threads fixture; if --source correctly
	// restricts to review-threads, pr-checks/branch-checks must never be
	// queried, so their missing fixtures won't cause an error.
	c.SetFixture("SearchReviewThreads", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--source", "review-threads"}, GH: c, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%s", code, stderr.String())
	}
}

func TestRunContext_UnknownSourceIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--source", "nonsense"}, GH: fake.NewClient(), Stdout: &stdout, Stderr: &stderr,
	})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown --source") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunContext_FilterWithPRTargetIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme/repo#1", "--filter", "involves:@me"}, GH: fake.NewClient(), Stdout: &stdout, Stderr: &stderr,
	})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--filter") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunContext_InvalidTargetIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"https://example.com/not/github"}, GH: fake.NewClient(), Stdout: &stdout, Stderr: &stderr,
	})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr=%s", code, stderr.String())
	}
}

func TestRunContext_IntervalBelowMinimumIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--watch", "--interval", "1s"}, GH: fake.NewClient(), Stdout: &stdout, Stderr: &stderr,
	})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr=%s", code, stderr.String())
	}
}

func TestRunContext_PartialFailureExitsOne(t *testing.T) {
	c := fake.NewClient()
	q := "is:pr is:open archived:false user:acme"
	c.SetFixture("SearchReviewThreads", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	c.SetFixture("SearchPRChecks", map[string]any{"q": q, "after": nil},
		[]byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`))
	// branch-checks fixture deliberately missing -> that source errors.
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--json"}, GH: c, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 1 {
		t.Fatalf("code = %d, want 1; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"source": "branch-checks"`) {
		t.Fatalf("expected the envelope's errors array to name branch-checks, stdout=%s", stdout.String())
	}
}

func TestRunContext_WatchReturnsOnThirdPoll(t *testing.T) {
	c := fake.NewClient()
	q := "is:pr is:open archived:false user:acme"
	empty := []byte(`{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}`)
	c.SetFixture("SearchReviewThreads", map[string]any{"q": q, "after": nil}, empty)
	c.SetFixture("SearchPRChecks", map[string]any{"q": q, "after": nil}, empty)

	var pollCount int32
	// OrgRepositories fixture is swapped out from an atomic counter so the
	// first two polls report no work and the third reports a failure.
	noWork := []byte(`{"repositoryOwner":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}`)
	withWork := []byte(`{"repositoryOwner":{"repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
		"nameWithOwner":"acme/r","isArchived":false,"isFork":false,
		"defaultBranchRef":{"name":"main","target":{"id":"C_1","oid":"deadbeef","statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
			{"__typename":"StatusContext","id":"SC_1","context":"ci/status","state":"FAILURE","targetUrl":"https://ci.example.com/1"}
		]}}}}}]}}}`)
	c.SetFixture("OrgRepositories", map[string]any{"login": "acme", "after": nil}, noWork)

	sleep := func(ctx context.Context, d time.Duration) error {
		n := atomic.AddInt32(&pollCount, 1)
		if n == 2 {
			c.SetFixture("OrgRepositories", map[string]any{"login": "acme", "after": nil}, withWork)
		}
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--watch", "--interval", "10s"}, GH: c, Sleep: sleep, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ci/status") {
		t.Fatalf("expected the eventual work item in output, stdout=%s", stdout.String())
	}
	if pollCount < 2 {
		t.Fatalf("pollCount = %d, want at least 2 (should not return on first, empty poll)", pollCount)
	}
}

func TestRunContext_WatchTimeoutExitsZeroWithEmptyResult(t *testing.T) {
	c := fake.NewClient()
	emptySearchFixtures(c, "acme")

	sleep := func(ctx context.Context, d time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}

	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--watch", "--interval", "10s", "--timeout", "50ms"}, GH: c, Sleep: sleep, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No work found") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunContext_WatchCtxCancelExits130(t *testing.T) {
	c := fake.NewClient()
	emptySearchFixtures(c, "acme")

	ctx, cancel := context.WithCancel(context.Background())
	sleep := func(ctx context.Context, d time.Duration) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}

	var stdout, stderr bytes.Buffer
	code := RunContext(ctx, Options{
		Args: []string{"acme", "--watch", "--interval", "10s"}, GH: c, Sleep: sleep, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 130 {
		t.Fatalf("code = %d, want 130; stderr=%s", code, stderr.String())
	}
}

func TestRunContext_WatchReturnsImmediatelyIfWorkAlreadyPresent(t *testing.T) {
	c := fake.NewClient()
	withWorkFixtures(c, "acme")

	called := false
	sleep := func(ctx context.Context, d time.Duration) error {
		called = true
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), Options{
		Args: []string{"acme", "--watch"}, GH: c, Sleep: sleep, Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if called {
		t.Fatal("sleep should never be called when work is present on the first poll")
	}
}
