// Package scan is the only package that speaks GraphQL for gh-work: it pages
// a target's open PRs and default-branch tips into typed data, once, that
// internal/check's pure checks then inspect. Keeping all pagination and
// query composition in one place means "each PR/branch is fetched once"
// is structural, not a convention every check has to honor.
package scan

import (
	"context"
	"fmt"
	"strings"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// Fields selects which optional field groups a PR scan fetches. The base
// fields (id, number, title, url, headRefName, baseRefName, isDraft) are
// always fetched; everything else costs extra GraphQL nodes and is included
// only when a selected check needs it (see internal/check.PRCheck.Needs).
type Fields uint8

const (
	ReviewThreads Fields = 1 << iota
	HeadRollup
	Mergeable
)

// Has reports whether f includes every field in x.
func (f Fields) Has(x Fields) bool { return f&x == x }

// CheckContext is a single CheckRun or StatusContext on a commit. It is an
// interface, rather than a struct with a type-discriminator field, so
// callers never need to branch on which GraphQL union member it was — see
// the __typename decoder registry in queries.go.
type CheckContext interface {
	ID() string
	Failing() bool
	Check() work.Check
}

// Commit is a commit's check-context rollup, with any overflow pages beyond
// GraphQL's first 100 already fetched and merged in.
type Commit struct {
	ID       string
	Oid      string
	Contexts []CheckContext
}

// ReviewThread is a single PR review thread, resolved or not — it's left to
// a check to decide what counts as work.
type ReviewThread struct {
	ID         string
	Path       string
	Line       int // 0 for a file-level comment
	IsOutdated bool
	IsResolved bool
	Comments   []work.Comment
}

// PullRequest is one open PR. Fields outside the requested Fields groups are
// left zero: ReviewThreads is nil unless ReviewThreads was requested, Head
// is nil unless HeadRollup was requested, Mergeable is "" unless Mergeable
// was requested.
type PullRequest struct {
	ID            string
	Repo          string
	Number        int
	Title         string
	URL           string
	HeadRef       string
	BaseRef       string
	IsDraft       bool
	Mergeable     string // raw GitHub enum: MERGEABLE, CONFLICTING, UNKNOWN
	ReviewThreads []ReviewThread
	Head          *Commit
}

// Branch is a repo's default branch and its tip commit's check rollup.
type Branch struct {
	Repo string
	Name string
	Tip  *Commit // nil if the repo has no default branch, or it has no commits yet
}

// PullRequests visits every open PR in scope for t (an org/user's PRs, a
// repo's PRs, or a single PR), fetching only the field groups named by
// fields, in one GraphQL walk no matter how many checks consume it.
// Warnings carry non-fatal notices (GitHub search's 1000-result cap).
func PullRequests(ctx context.Context, gh github.Client, t target.Target, fields Fields, filters []string, visit func(PullRequest)) ([]string, error) {
	s := &prScanner{ctx: ctx, gh: gh, fields: fields, filters: filters, visit: visit}
	if err := t.Accept(s); err != nil {
		return nil, err
	}
	return s.warnings, nil
}

// Branches visits every non-archived, non-fork repo's default branch tip in
// scope for t. A PR target has no default branch of its own, so it visits
// nothing.
func Branches(ctx context.Context, gh github.Client, t target.Target, visit func(Branch)) error {
	return t.Accept(&branchScanner{ctx: ctx, gh: gh, visit: visit})
}

// prScanner implements target.Visitor so PullRequests' target-shape
// handling is dispatch, not a type switch.
type prScanner struct {
	ctx      context.Context
	gh       github.Client
	fields   Fields
	filters  []string
	visit    func(PullRequest)
	warnings []string
}

func (s *prScanner) VisitOrg(o target.Org) error {
	return s.search("user:" + o.Owner)
}

func (s *prScanner) VisitRepo(r target.Repo) error {
	return s.search("repo:" + r.Owner + "/" + r.Name)
}

func (s *prScanner) VisitPR(p target.PR) error {
	var resp prTargetResponse
	if err := s.gh.Query(s.ctx, buildPRQuery(s.fields), map[string]any{
		"owner": p.Owner, "repo": p.Name, "number": p.Number,
	}, &resp); err != nil {
		return err
	}
	if resp.Repository.PullRequest == nil {
		return fmt.Errorf("%s#%d not found", p.RepoString(), p.Number)
	}
	pr, err := pullRequestFromRaw(s.ctx, s.gh, p.RepoString(), *resp.Repository.PullRequest)
	if err != nil {
		return err
	}
	s.visit(pr)
	return nil
}

func (s *prScanner) search(scope string) error {
	q := strings.Join(append([]string{"is:pr", "is:open", "archived:false", scope}, s.filters...), " ")
	query := buildSearchQuery(s.fields)
	issueCount := 0
	err := github.Paginate(s.ctx, func(ctx context.Context, cursor string) (github.PageInfo, error) {
		var after any
		if cursor != "" {
			after = cursor
		}
		var resp searchResponse
		if err := s.gh.Query(ctx, query, map[string]any{"q": q, "after": after}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		issueCount = resp.Search.IssueCount
		for _, n := range resp.Search.Nodes {
			repo := ""
			if n.Repository != nil {
				repo = n.Repository.NameWithOwner
			}
			pr, err := pullRequestFromRaw(ctx, s.gh, repo, n)
			if err != nil {
				return github.PageInfo{}, err
			}
			s.visit(pr)
		}
		return resp.Search.PageInfo, nil
	})
	if err != nil {
		return err
	}
	if issueCount > 1000 {
		s.warnings = append(s.warnings, fmt.Sprintf("search matched %d PRs; GitHub search only returns the first 1000", issueCount))
	}
	return nil
}

// branchScanner implements target.Visitor so Branches' target-shape
// handling is dispatch, not a type switch.
type branchScanner struct {
	ctx   context.Context
	gh    github.Client
	visit func(Branch)
}

// VisitOrg scans every non-archived, non-fork repo owned by an org or user.
func (s *branchScanner) VisitOrg(o target.Org) error {
	return github.Paginate(s.ctx, func(ctx context.Context, cursor string) (github.PageInfo, error) {
		var after any
		if cursor != "" {
			after = cursor
		}
		var resp ownerReposResponse
		if err := s.gh.Query(ctx, ownerReposQuery, map[string]any{"login": o.Owner, "after": after}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		if resp.RepositoryOwner == nil {
			return github.PageInfo{}, fmt.Errorf("owner %q not found", o.Owner)
		}
		for _, repo := range resp.RepositoryOwner.Repositories.Nodes {
			if repo.IsArchived || repo.IsFork {
				continue
			}
			b, err := branchFromRaw(ctx, s.gh, repo)
			if err != nil {
				return github.PageInfo{}, err
			}
			if b != nil {
				s.visit(*b)
			}
		}
		return resp.RepositoryOwner.Repositories.PageInfo, nil
	})
}

// VisitRepo scans a single, explicitly named repo, regardless of its
// archived/fork status (the user asked for it by name).
func (s *branchScanner) VisitRepo(r target.Repo) error {
	var resp repoResponse
	if err := s.gh.Query(s.ctx, repoDefaultBranchQuery, map[string]any{"owner": r.Owner, "name": r.Name}, &resp); err != nil {
		return err
	}
	if resp.Repository == nil {
		return fmt.Errorf("repository %s not found", r.String())
	}
	b, err := branchFromRaw(s.ctx, s.gh, *resp.Repository)
	if err != nil {
		return err
	}
	if b != nil {
		s.visit(*b)
	}
	return nil
}

// VisitPR is a no-op: a single PR target has no default branch of its own.
func (s *branchScanner) VisitPR(target.PR) error { return nil }
