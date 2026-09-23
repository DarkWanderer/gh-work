// Package branchchecks implements the "branch-checks" work source: failing
// checks on a repo's default branch tip. It does not apply to a PR target.
package branchchecks

import (
	"context"
	"fmt"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// Source collects failing checks on default branch tips.
type Source struct{}

var _ source.Source = Source{}

// New returns a branch-checks Source.
func New() Source { return Source{} }

func (Source) Name() string { return "branch-checks" }

func (Source) Supports(t target.Target) bool {
	switch t.(type) {
	case target.Org, target.Repo:
		return true
	default:
		return false
	}
}

func (s Source) Collect(ctx context.Context, gh github.Client, t target.Target, _ source.Options) (source.Result, error) {
	switch v := t.(type) {
	case target.Org:
		return collectForOrg(ctx, gh, v)
	case target.Repo:
		items, err := collectForRepo(ctx, gh, v)
		if err != nil {
			return source.Result{}, err
		}
		return source.Result{Items: items}, nil
	default:
		return source.Result{}, fmt.Errorf("branch-checks: unsupported target %T", t)
	}
}

// collectForOrg scans every non-archived, non-fork repo owned by an org or
// user. Archived/fork status is filtered client-side (see queries.go).
func collectForOrg(ctx context.Context, gh github.Client, o target.Org) (source.Result, error) {
	var items []work.Item
	err := github.Paginate(ctx, func(ctx context.Context, cursor string) (github.PageInfo, error) {
		var after any
		if cursor != "" {
			after = cursor
		}
		var resp orgReposResponse
		if err := gh.Query(ctx, orgReposQuery, map[string]any{"login": o.Owner, "after": after}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		if resp.RepositoryOwner == nil {
			return github.PageInfo{}, fmt.Errorf("branch-checks: owner %q not found", o.Owner)
		}
		for _, repo := range resp.RepositoryOwner.Repositories.Nodes {
			if repo.IsArchived || repo.IsFork {
				continue
			}
			repoItems, err := branchItemsForRepo(ctx, gh, repo)
			if err != nil {
				return github.PageInfo{}, err
			}
			items = append(items, repoItems...)
		}
		return resp.RepositoryOwner.Repositories.PageInfo, nil
	})
	if err != nil {
		return source.Result{}, err
	}
	return source.Result{Items: items}, nil
}

// collectForRepo checks a single, explicitly named repo, regardless of its
// archived/fork status (the user asked for it by name).
func collectForRepo(ctx context.Context, gh github.Client, r target.Repo) ([]work.Item, error) {
	var resp repoResponse
	if err := gh.Query(ctx, repoQuery, map[string]any{"owner": r.Owner, "name": r.Name}, &resp); err != nil {
		return nil, err
	}
	if resp.Repository == nil {
		return nil, fmt.Errorf("branch-checks: repository %s not found", r.String())
	}
	return branchItemsForRepo(ctx, gh, *resp.Repository)
}

func branchItemsForRepo(ctx context.Context, gh github.Client, repo repoNode) ([]work.Item, error) {
	if repo.DefaultBranchRef == nil || repo.DefaultBranchRef.Target == nil {
		return nil, nil
	}
	commit := repo.DefaultBranchRef.Target
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

	var items []work.Item
	for _, n := range contexts {
		chk, isFailure := checkFrom(n)
		if !isFailure {
			continue
		}
		items = append(items, work.BranchCheckFailure{
			ID: n.ID, Repo: repo.NameWithOwner, Branch: repo.DefaultBranchRef.Name, Commit: commit.Oid, Check: chk,
		})
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
