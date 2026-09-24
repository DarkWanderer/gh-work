// Package mergeconflicts implements the "merge-conflicts" work source: open
// PRs GitHub reports as CONFLICTING.
package mergeconflicts

import (
	"context"
	"fmt"
	"strings"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// Source collects open PRs whose mergeable state is CONFLICTING. GitHub
// search has no qualifier for this, so it lists open PRs and filters on
// mergeable client-side.
type Source struct{}

var _ source.Source = Source{}

// New returns a merge-conflicts Source.
func New() Source { return Source{} }

func (Source) Name() string { return "merge-conflicts" }

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
		return source.Result{}, fmt.Errorf("merge-conflicts: unsupported target %T", t)
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
	var resp prTargetResponse
	if err := gh.Query(ctx, prQuery, map[string]any{
		"owner": p.Owner, "repo": p.Name, "number": p.Number,
	}, &resp); err != nil {
		return nil, err
	}
	if resp.Repository.PullRequest == nil {
		return nil, fmt.Errorf("merge-conflicts: %s#%d not found", p.RepoString(), p.Number)
	}
	item, ok := itemFrom(p.RepoString(), *resp.Repository.PullRequest)
	if !ok {
		return nil, nil
	}
	return []work.Item{item}, nil
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
			if item, ok := itemFrom(repo, pr); ok {
				items = append(items, item)
			}
		}
		return resp.Search.PageInfo, nil
	})
	if err != nil {
		return source.Result{}, err
	}
	var warnings []string
	if issueCount > 1000 {
		warnings = append(warnings, fmt.Sprintf("merge-conflicts: search matched %d PRs; GitHub search only returns the first 1000", issueCount))
	}
	return source.Result{Items: items, Warnings: warnings}, nil
}

// itemFrom returns ok=false for anything other than a confirmed conflict.
// MERGEABLE (no conflict) and UNKNOWN (GitHub hasn't finished computing it
// yet) are both skipped silently, with no warning — UNKNOWN resolves itself
// on a later run.
func itemFrom(repo string, n prNode) (work.MergeConflict, bool) {
	if n.Mergeable != "CONFLICTING" {
		return work.MergeConflict{}, false
	}
	return work.MergeConflict{ID: n.ID, Repo: repo, PR: prRefFrom(n), BaseRef: n.BaseRefName}, true
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
