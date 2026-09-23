// Package github is the thin GraphQL boundary between `gh work` and the
// GitHub API. Sources depend only on the Client interface, so tests can
// swap in internal/github/fake instead of talking to a real host.
package github

import (
	"context"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Client runs a single named GraphQL query. query must be a full document
// with an operation name (e.g. "query SearchReviewThreads($q: String!) {
// ... }") — the fake client used in tests dispatches fixtures by that name.
type Client interface {
	Query(ctx context.Context, query string, vars map[string]any, out any) error
}

type client struct {
	gql *api.GraphQLClient
}

// NewClient builds a Client using gh's own resolved auth token and host
// (respects GH_HOST, GH_TOKEN, gh auth login, etc).
func NewClient() (Client, error) {
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	return &client{gql: gql}, nil
}

func (c *client) Query(ctx context.Context, query string, vars map[string]any, out any) error {
	return c.gql.DoWithContext(ctx, query, vars, out)
}
