package github

import "context"

// PageInfo mirrors GraphQL's standard pageInfo shape.
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// Paginate calls fetch with cursor="" for the first page, then repeatedly
// with the previous page's EndCursor, until a page reports HasNextPage
// false or fetch returns an error. fetch is expected to decode the page
// into its own accumulator (a slice captured by the closure) as a side
// effect before returning that page's PageInfo.
func Paginate(ctx context.Context, fetch func(ctx context.Context, cursor string) (PageInfo, error)) error {
	return PaginateFrom(ctx, "", fetch)
}

// PaginateFrom is like Paginate but starts at a known cursor (e.g. one
// carried over from an earlier, differently-shaped query) instead of the
// first page.
func PaginateFrom(ctx context.Context, cursor string, fetch func(ctx context.Context, cursor string) (PageInfo, error)) error {
	for {
		pi, err := fetch(ctx, cursor)
		if err != nil {
			return err
		}
		if !pi.HasNextPage {
			return nil
		}
		cursor = pi.EndCursor
	}
}
