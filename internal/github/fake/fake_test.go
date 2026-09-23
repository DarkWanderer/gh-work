package fake

import (
	"context"
	"errors"
	"testing"
)

const sampleQuery = `query SearchReviewThreads($q: String!, $after: String) { search(query: $q) { issueCount } }`

func TestQueryDispatchesByOperationAndVars(t *testing.T) {
	c := NewClient()
	c.SetFixture("SearchReviewThreads", map[string]any{"q": "is:pr", "after": nil}, []byte(`{"search":{"issueCount":3}}`))

	var out struct {
		Search struct {
			IssueCount int `json:"issueCount"`
		} `json:"search"`
	}
	if err := c.Query(context.Background(), sampleQuery, map[string]any{"q": "is:pr", "after": nil}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Search.IssueCount != 3 {
		t.Fatalf("issueCount = %d, want 3", out.Search.IssueCount)
	}
	if len(c.Calls) != 1 || c.Calls[0].Operation != "SearchReviewThreads" {
		t.Fatalf("Calls = %#v", c.Calls)
	}
}

func TestQueryDifferentVarsDifferentFixture(t *testing.T) {
	c := NewClient()
	c.SetFixture("SearchReviewThreads", map[string]any{"q": "is:pr", "after": nil}, []byte(`{"search":{"issueCount":1}}`))
	c.SetFixture("SearchReviewThreads", map[string]any{"q": "is:pr", "after": "CURSOR"}, []byte(`{"search":{"issueCount":2}}`))

	var out struct {
		Search struct {
			IssueCount int `json:"issueCount"`
		} `json:"search"`
	}
	if err := c.Query(context.Background(), sampleQuery, map[string]any{"q": "is:pr", "after": "CURSOR"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Search.IssueCount != 2 {
		t.Fatalf("issueCount = %d, want 2 (wrong page fixture matched)", out.Search.IssueCount)
	}
}

func TestQueryMissingFixtureErrors(t *testing.T) {
	c := NewClient()
	var out any
	err := c.Query(context.Background(), sampleQuery, map[string]any{"q": "nope"}, &out)
	if err == nil {
		t.Fatal("expected error for missing fixture")
	}
}

func TestQueryInjectedError(t *testing.T) {
	c := NewClient()
	want := errors.New("rate limited")
	c.SetFixtureError("SearchReviewThreads", map[string]any{"q": "is:pr"}, want)

	var out any
	err := c.Query(context.Background(), sampleQuery, map[string]any{"q": "is:pr"}, &out)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestOperationNameExtraction(t *testing.T) {
	cases := map[string]string{
		`query SearchReviewThreads($q: String!) { search { issueCount } }`:          "SearchReviewThreads",
		`query  OrgRepositories($login:String!){repositoryOwner(login:$login){id}}`: "OrgRepositories",
		`{ viewer { login } }`: "",
	}
	for q, want := range cases {
		if got := operationName(q); got != want {
			t.Errorf("operationName(%q) = %q, want %q", q, got, want)
		}
	}
}
