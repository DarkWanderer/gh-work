package github_test

import (
	"context"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/github/fake"
)

const pageQuery = `query PageThings($after: String) {
  things(first: 2, after: $after) {
    pageInfo { hasNextPage endCursor }
    nodes { name }
  }
}`

type pageResponse struct {
	Things struct {
		PageInfo github.PageInfo `json:"pageInfo"`
		Nodes    []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"things"`
}

func TestPaginateAccumulatesAcrossPages(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("PageThings", map[string]any{"after": nil},
		[]byte(`{"things":{"pageInfo":{"hasNextPage":true,"endCursor":"C1"},"nodes":[{"name":"a"},{"name":"b"}]}}`))
	c.SetFixture("PageThings", map[string]any{"after": "C1"},
		[]byte(`{"things":{"pageInfo":{"hasNextPage":true,"endCursor":"C2"},"nodes":[{"name":"c"}]}}`))
	c.SetFixture("PageThings", map[string]any{"after": "C2"},
		[]byte(`{"things":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"name":"d"}]}}`))

	var names []string
	err := github.Paginate(context.Background(), func(ctx context.Context, cursor string) (github.PageInfo, error) {
		var after any
		if cursor != "" {
			after = cursor
		}
		var resp pageResponse
		if err := c.Query(ctx, pageQuery, map[string]any{"after": after}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		for _, n := range resp.Things.Nodes {
			names = append(names, n.Name)
		}
		return resp.Things.PageInfo, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c", "d"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
}

func TestPaginateStopsAtOnePageWhenNoNextPage(t *testing.T) {
	c := fake.NewClient()
	c.SetFixture("PageThings", map[string]any{"after": nil},
		[]byte(`{"things":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"name":"only"}]}}`))

	calls := 0
	err := github.Paginate(context.Background(), func(ctx context.Context, cursor string) (github.PageInfo, error) {
		calls++
		var after any
		if cursor != "" {
			after = cursor
		}
		var resp pageResponse
		if err := c.Query(ctx, pageQuery, map[string]any{"after": after}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		return resp.Things.PageInfo, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestPaginatePropagatesError(t *testing.T) {
	c := fake.NewClient() // no fixtures registered
	err := github.Paginate(context.Background(), func(ctx context.Context, cursor string) (github.PageInfo, error) {
		var resp pageResponse
		err := c.Query(ctx, pageQuery, map[string]any{"after": nil}, &resp)
		return resp.Things.PageInfo, err
	})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}
