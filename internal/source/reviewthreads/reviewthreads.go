// Package reviewthreads implements the "review-threads" work source:
// unresolved PR review threads.
package reviewthreads

import (
	"context"
	"fmt"
	"strings"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// Source collects unresolved PR review threads.
type Source struct{}

var _ source.Source = Source{}

// New returns a review-threads Source.
func New() Source { return Source{} }

func (Source) Name() string { return "review-threads" }

func (Source) Supports(target.Target) bool { return true }

func (s Source) Collect(ctx context.Context, gh github.Client, t target.Target, opts source.Options) (source.Result, error) {
	switch v := t.(type) {
	case target.PR:
		items, err := collectForPR(ctx, gh, v)
		if err != nil {
			return source.Result{}, err
		}
		return source.Result{Items: items}, nil
	case target.Org, target.Repo:
		return collectViaSearch(ctx, gh, t, opts.Filters)
	default:
		return source.Result{}, fmt.Errorf("review-threads: unsupported target %T", t)
	}
}

func searchQueryString(t target.Target, filters []string) string {
	scope := ""
	switch v := t.(type) {
	case target.Org:
		scope = "user:" + v.Owner
	case target.Repo:
		scope = "repo:" + v.Owner + "/" + v.Name
	}
	parts := append([]string{"is:pr", "is:open", "archived:false", scope}, filters...)
	return strings.Join(parts, " ")
}

func collectForPR(ctx context.Context, gh github.Client, p target.PR) ([]work.Item, error) {
	var items []work.Item
	err := github.Paginate(ctx, func(ctx context.Context, cursor string) (github.PageInfo, error) {
		var after any
		if cursor != "" {
			after = cursor
		}
		var resp prTargetResponse
		if err := gh.Query(ctx, prQuery, map[string]any{
			"owner": p.Owner, "repo": p.Name, "number": p.Number, "after": after,
		}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		if resp.Repository.PullRequest == nil {
			return github.PageInfo{}, fmt.Errorf("review-threads: %s#%d not found", p.RepoString(), p.Number)
		}
		pr := resp.Repository.PullRequest
		items = append(items, threadItems(p.RepoString(), prRefFrom(*pr), pr.ReviewThreads.Nodes)...)
		return pr.ReviewThreads.PageInfo, nil
	})
	return items, err
}

func collectViaSearch(ctx context.Context, gh github.Client, t target.Target, filters []string) (source.Result, error) {
	q := searchQueryString(t, filters)
	var items []work.Item
	issueCount := 0
	err := github.Paginate(ctx, func(ctx context.Context, cursor string) (github.PageInfo, error) {
		var after any
		if cursor != "" {
			after = cursor
		}
		var resp searchResponse
		if err := gh.Query(ctx, searchQuery, map[string]any{"q": q, "after": after}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		issueCount = resp.Search.IssueCount
		for _, pr := range resp.Search.Nodes {
			repo := repoStringFor(t, pr)
			nodes := pr.ReviewThreads.Nodes
			if pr.ReviewThreads.PageInfo.HasNextPage {
				more, err := fetchRemainingThreads(ctx, gh, pr.ID, pr.ReviewThreads.PageInfo.EndCursor)
				if err != nil {
					return github.PageInfo{}, err
				}
				nodes = append(nodes, more...)
			}
			items = append(items, threadItems(repo, prRefFrom(pr), nodes)...)
		}
		return resp.Search.PageInfo, nil
	})
	if err != nil {
		return source.Result{}, err
	}
	var warnings []string
	if issueCount > 1000 {
		warnings = append(warnings, fmt.Sprintf("review-threads: search matched %d PRs; GitHub search only returns the first 1000", issueCount))
	}
	return source.Result{Items: items, Warnings: warnings}, nil
}

func fetchRemainingThreads(ctx context.Context, gh github.Client, prID, cursor string) ([]reviewThreadNode, error) {
	var out []reviewThreadNode
	err := github.PaginateFrom(ctx, cursor, func(ctx context.Context, c string) (github.PageInfo, error) {
		var resp threadsPageResponse
		if err := gh.Query(ctx, threadsPageQuery, map[string]any{"id": prID, "after": c}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		out = append(out, resp.Node.ReviewThreads.Nodes...)
		return resp.Node.ReviewThreads.PageInfo, nil
	})
	return out, err
}

func prRefFrom(n prNode) work.PRRef {
	return work.PRRef{Number: n.Number, Title: n.Title, URL: n.URL, HeadRef: n.HeadRefName, IsDraft: n.IsDraft}
}

func repoStringFor(t target.Target, n prNode) string {
	if n.Repository != nil && n.Repository.NameWithOwner != "" {
		return n.Repository.NameWithOwner
	}
	if r, ok := t.(target.Repo); ok {
		return r.String()
	}
	return ""
}

// threadItems converts unresolved review thread nodes into work items.
// Resolved threads are dropped; the thread's URL is taken from its first
// comment, since PullRequestReviewThread itself has no url field.
func threadItems(repo string, pr work.PRRef, nodes []reviewThreadNode) []work.Item {
	var items []work.Item
	for _, n := range nodes {
		if n.IsResolved {
			continue
		}
		line := 0
		if n.Line != nil {
			line = *n.Line
		}
		comments := make([]work.Comment, 0, len(n.Comments.Nodes))
		for _, c := range n.Comments.Nodes {
			author := ""
			if c.Author != nil {
				author = c.Author.Login
			}
			comments = append(comments, work.Comment{Author: author, Body: c.Body, URL: c.URL, CreatedAt: c.CreatedAt})
		}
		threadURL := pr.URL
		if len(comments) > 0 {
			threadURL = comments[0].URL
		}
		items = append(items, work.ReviewThread{
			ID: n.ID, Repo: repo, PR: pr,
			Thread: work.Thread{Path: n.Path, Line: line, IsOutdated: n.IsOutdated, URL: threadURL, Comments: comments},
		})
	}
	return items
}
