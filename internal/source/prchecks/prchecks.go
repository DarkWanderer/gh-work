// Package prchecks implements the "pr-checks" work source: failing checks
// on a PR's head commit.
package prchecks

import (
	"context"
	"fmt"
	"strings"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// Source collects failing checks on PR head commits.
type Source struct{}

var _ source.Source = Source{}

// New returns a pr-checks Source.
func New() Source { return Source{} }

func (Source) Name() string { return "pr-checks" }

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
		return source.Result{}, fmt.Errorf("pr-checks: unsupported target %T", t)
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
		return nil, fmt.Errorf("pr-checks: %s#%d not found", p.RepoString(), p.Number)
	}
	return checkItemsForPR(ctx, gh, p.RepoString(), *resp.Repository.PullRequest)
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
			prItems, err := checkItemsForPR(ctx, gh, repo, pr)
			if err != nil {
				return github.PageInfo{}, err
			}
			items = append(items, prItems...)
		}
		return resp.Search.PageInfo, nil
	})
	if err != nil {
		return source.Result{}, err
	}
	var warnings []string
	if issueCount > 1000 {
		warnings = append(warnings, fmt.Sprintf("pr-checks: search matched %d PRs; GitHub search only returns the first 1000", issueCount))
	}
	return source.Result{Items: items, Warnings: warnings}, nil
}

// checkItemsForPR extracts failing checks from a PR's head (last) commit,
// paging through the rollup's contexts if there are more than 100.
func checkItemsForPR(ctx context.Context, gh github.Client, repo string, pr prNode) ([]work.Item, error) {
	if len(pr.Commits.Nodes) == 0 {
		return nil, nil
	}
	commit := pr.Commits.Nodes[0].Commit
	if commit.StatusCheckRollup == nil {
		return nil, nil
	}
	contexts := commit.StatusCheckRollup.Contexts.Nodes
	if commit.StatusCheckRollup.Contexts.PageInfo.HasNextPage {
		more, err := fetchRemainingContexts(ctx, gh, commit.ID, commit.StatusCheckRollup.Contexts.PageInfo.EndCursor)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, more...)
	}

	prRef := prRefFrom(pr)
	var items []work.Item
	for _, n := range contexts {
		chk, isFailure := checkFrom(n)
		if !isFailure {
			continue
		}
		items = append(items, work.PRCheckFailure{ID: n.ID, Repo: repo, PR: prRef, Commit: commit.Oid, Check: chk})
	}
	return items, nil
}

func fetchRemainingContexts(ctx context.Context, gh github.Client, commitID, cursor string) ([]checkContextNode, error) {
	var out []checkContextNode
	err := github.PaginateFrom(ctx, cursor, func(ctx context.Context, c string) (github.PageInfo, error) {
		var resp contextsPageResponse
		if err := gh.Query(ctx, contextsPageQuery, map[string]any{"id": commitID, "after": c}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		if resp.Node.StatusCheckRollup == nil {
			return github.PageInfo{}, nil
		}
		out = append(out, resp.Node.StatusCheckRollup.Contexts.Nodes...)
		return resp.Node.StatusCheckRollup.Contexts.PageInfo, nil
	})
	return out, err
}

// checkFrom classifies a check context node, returning ok=false for
// passing/pending/skipped checks that are not work.
func checkFrom(n checkContextNode) (chk work.Check, ok bool) {
	switch n.Typename {
	case "CheckRun":
		if !work.IsFailingCheckRunConclusion(n.Conclusion) {
			return work.Check{}, false
		}
		chk = work.Check{Name: n.Name, Conclusion: n.Conclusion, URL: n.DetailsURL, JobID: n.DatabaseID}
		if n.CheckSuite != nil && n.CheckSuite.WorkflowRun != nil {
			chk.RunID = n.CheckSuite.WorkflowRun.DatabaseID
			chk.Workflow = n.CheckSuite.WorkflowRun.Workflow.Name
		}
		return chk, true
	case "StatusContext":
		if !work.IsFailingStatusState(n.State) {
			return work.Check{}, false
		}
		return work.Check{Name: n.Context, Conclusion: n.State, URL: n.TargetURL}, true
	default:
		return work.Check{}, false
	}
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
