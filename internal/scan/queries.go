package scan

import (
	"context"
	"time"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// annotationsSelection is a CheckAnnotationConnection's selection, shared
// by the inline first page (checkContextsFragment) and annotationsPageQuery.
// CheckAnnotation has no node ID, so databaseId is what item IDs are built
// from (see internal/check.PRAnnotations).
const annotationsSelection = `pageInfo { hasNextPage endCursor }
                nodes {
                  databaseId
                  annotationLevel
                  path
                  location { start { line } end { line } }
                  title
                  message
                  blobUrl
                }`

// checkContextsFragment is shared by every query that reads a commit's
// check rollup (PR head commit, branch tip commit, and the overflow page
// query for either): CheckRun and StatusContext are GitHub's only two
// StatusCheckRollupContext union members. withAnnotations splices in each
// CheckRun's first page of annotations — only PR head commits ever ask for
// them, since it is by far the most expensive part of the rollup.
func checkContextsFragment(withAnnotations bool) string {
	annotations := ""
	if withAnnotations {
		annotations = `
              annotations(first: 50) {
                ` + annotationsSelection + `
              }`
	}
	return `pageInfo { hasNextPage endCursor }
          nodes {
            __typename
            ... on CheckRun {
              id
              name
              conclusion
              databaseId
              detailsUrl
              checkSuite {
                workflowRun {
                  databaseId
                  workflow { name }
                }
              }` + annotations + `
            }
            ... on StatusContext {
              id
              context
              state
              targetUrl
            }
          }`
}

// reviewThreadsFragment fetches a PR's first page of review threads;
// resolution is decided by a check, not filtered here.
const reviewThreadsFragment = `reviewThreads(first: 30) {
          pageInfo { hasNextPage endCursor }
          nodes {
            id
            isResolved
            isOutdated
            path
            line
            comments(first: 10) {
              nodes { author { login } body url createdAt }
            }
          }
        }`

// headRollupFragment fetches a PR's head commit and its check rollup, with
// each CheckRun's annotations if withAnnotations.
//
// GitHub GraphQL rejects a query whose worst-case node count (summed across
// every nesting level, not just the leaf product) exceeds 500,000. With
// both this and reviewThreadsFragment spliced into a 50-wide search page,
// the worst case is roughly 50 + 50*30 + 50*30*10 + 50*1 + 50*1*100 =
// 21,600 nodes, plus 50*1*100*50 = 250,000 more with annotations — 271,600
// in all, still under the cap. The annotations level dominates the query's
// rate-limit cost too, which is why it is its own Fields bit.
func headRollupFragment(withAnnotations bool) string {
	return `commits(last: 1) {
          nodes {
            commit {
              id
              oid
              statusCheckRollup {
                contexts(first: 100) {
                  ` + checkContextsFragment(withAnnotations) + `
                }
              }
            }
          }
        }`
}

// prBaseSelection is always fetched for a PR; mergeable/reviewThreads/
// commits are spliced in only when fields asks for them, so a scan that
// only needs one field group doesn't pay for the others.
func prBaseSelection(fields Fields) string {
	sel := `id
        number
        title
        url
        isDraft
        headRefName
        baseRefName`
	if fields.Has(Mergeable) {
		sel += "\n        mergeable"
	}
	if fields.Has(ReviewThreads) {
		sel += "\n        " + reviewThreadsFragment
	}
	if fields.Has(HeadRollup) || fields.Has(HeadAnnotations) {
		sel += "\n        " + headRollupFragment(fields.Has(HeadAnnotations))
	}
	return sel
}

func buildSearchQuery(fields Fields) string {
	return `query SearchPullRequests($q: String!, $after: String) {
  search(type: ISSUE, query: $q, first: 50, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes {
      ... on PullRequest {
        ` + prBaseSelection(fields) + `
        repository { nameWithOwner }
      }
    }
  }
}`
}

func buildPRQuery(fields Fields) string {
	return `query PullRequest($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      ` + prBaseSelection(fields) + `
    }
  }
}`
}

// threadsPageQuery re-fetches a PR's review threads past the first page,
// via its node ID — shared by every path that discovers a PR (search or a
// direct PR target), so overflow pagination has exactly one implementation.
const threadsPageQuery = `query PullRequestReviewThreadsPage($id: ID!, $after: String) {
  node(id: $id) {
    ... on PullRequest {
      reviewThreads(first: 30, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isResolved
          isOutdated
          path
          line
          comments(first: 10) {
            nodes { author { login } body url createdAt }
          }
        }
      }
    }
  }
}`

// buildContextsPageQuery re-fetches a commit's check contexts past the
// first page, via its node ID — shared by PR head commits and branch tip
// commits. withAnnotations must match the query that fetched the first
// page, so overflow contexts carry the same fields as the rest.
func buildContextsPageQuery(withAnnotations bool) string {
	return `query CommitCheckContextsPage($id: ID!, $after: String) {
  node(id: $id) {
    ... on Commit {
      statusCheckRollup {
        contexts(first: 100, after: $after) {
          ` + checkContextsFragment(withAnnotations) + `
        }
      }
    }
  }
}`
}

// annotationsPageQuery re-fetches a CheckRun's annotations past the first
// page, via its node ID.
const annotationsPageQuery = `query CheckRunAnnotationsPage($id: ID!, $after: String) {
  node(id: $id) {
    ... on CheckRun {
      annotations(first: 50, after: $after) {
        ` + annotationsSelection + `
      }
    }
  }
}`

// ownerReposQuery pages non-archived, non-fork repos owned by an org or
// user and their default branch's latest check rollup. isArchived/isFork
// are re-checked client-side rather than passed as connection arguments,
// since those filter args are not reliably present on the RepositoryOwner
// interface across both Organization and User.
var ownerReposQuery = `query OwnerRepositories($login: String!, $after: String) {
  repositoryOwner(login: $login) {
    repositories(first: 50, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes {
        nameWithOwner
        isArchived
        isFork
        defaultBranchRef {
          name
          target {
            ... on Commit {
              id
              oid
              statusCheckRollup {
                contexts(first: 100) {
                  ` + checkContextsFragment(false) + `
                }
              }
            }
          }
        }
      }
    }
  }
}`

var repoDefaultBranchQuery = `query RepositoryDefaultBranch($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    isArchived
    isFork
    defaultBranchRef {
      name
      target {
        ... on Commit {
          id
          oid
          statusCheckRollup {
            contexts(first: 100) {
              ` + checkContextsFragment(false) + `
            }
          }
        }
      }
    }
  }
}`

// rawCheckContextNode is the flattened union of CheckRun and StatusContext
// fields; whichever the API actually returned is discriminated by
// Typename, and the fields belonging to the other variant stay zero.
type rawCheckContextNode struct {
	Typename   string `json:"__typename"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	DatabaseID int64  `json:"databaseId"`
	DetailsURL string `json:"detailsUrl"`
	CheckSuite *struct {
		WorkflowRun *struct {
			DatabaseID int64 `json:"databaseId"`
			Workflow   struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`

	Annotations *rawAnnotationsConnection `json:"annotations"`

	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

type rawAnnotationNode struct {
	DatabaseID      *int64 `json:"databaseId"`
	AnnotationLevel string `json:"annotationLevel"`
	Path            string `json:"path"`
	Location        struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
		End struct {
			Line int `json:"line"`
		} `json:"end"`
	} `json:"location"`
	Title   *string `json:"title"`
	Message string  `json:"message"`
	BlobURL string  `json:"blobUrl"`
}

type rawAnnotationsConnection struct {
	PageInfo github.PageInfo     `json:"pageInfo"`
	Nodes    []rawAnnotationNode `json:"nodes"`
}

// checkRun and statusContext are CheckContext's two implementations,
// decoded from rawCheckContextNode by the registry below.
type checkRun struct {
	id, name, conclusion, detailsURL, workflow string
	databaseID, runID                          int64
	annotations                                []Annotation
}

func (c checkRun) ID() string                { return c.id }
func (c checkRun) Failing() bool             { return work.IsFailingCheckRunConclusion(c.conclusion) }
func (c checkRun) Annotations() []Annotation { return c.annotations }
func (c checkRun) Check() work.Check {
	return work.Check{Name: c.name, Conclusion: c.conclusion, URL: c.detailsURL, Workflow: c.workflow, RunID: c.runID, JobID: c.databaseID}
}

type statusContext struct {
	id, context, state, targetURL string
}

func (c statusContext) ID() string    { return c.id }
func (c statusContext) Failing() bool { return work.IsFailingStatusState(c.state) }

// Annotations is always nil: commit statuses have no annotations.
func (c statusContext) Annotations() []Annotation { return nil }
func (c statusContext) Check() work.Check {
	return work.Check{Name: c.context, Conclusion: c.state, URL: c.targetURL}
}

// checkContextDecoders maps a StatusCheckRollupContext union member's
// __typename to the function that builds its CheckContext, in place of a
// type-switch on n.Typename. An unrecognized __typename (a future GitHub
// union member gh-work doesn't know about) has no entry and is skipped.
var checkContextDecoders = map[string]func(rawCheckContextNode) CheckContext{
	"CheckRun": func(n rawCheckContextNode) CheckContext {
		cr := checkRun{id: n.ID, name: n.Name, conclusion: n.Conclusion, detailsURL: n.DetailsURL, databaseID: n.DatabaseID}
		if n.CheckSuite != nil && n.CheckSuite.WorkflowRun != nil {
			cr.runID = n.CheckSuite.WorkflowRun.DatabaseID
			cr.workflow = n.CheckSuite.WorkflowRun.Workflow.Name
		}
		if n.Annotations != nil {
			cr.annotations = make([]Annotation, 0, len(n.Annotations.Nodes))
			for _, a := range n.Annotations.Nodes {
				cr.annotations = append(cr.annotations, annotationFromRaw(a))
			}
		}
		return cr
	},
	"StatusContext": func(n rawCheckContextNode) CheckContext {
		return statusContext{id: n.ID, context: n.Context, state: n.State, targetURL: n.TargetURL}
	},
}

func annotationFromRaw(n rawAnnotationNode) Annotation {
	a := Annotation{
		Level: n.AnnotationLevel, Path: n.Path,
		StartLine: n.Location.Start.Line, EndLine: n.Location.End.Line,
		Message: n.Message, URL: n.BlobURL,
	}
	if n.DatabaseID != nil {
		a.DatabaseID = *n.DatabaseID
	}
	if n.Title != nil {
		a.Title = *n.Title
	}
	return a
}

func decodeCheckContext(n rawCheckContextNode) (CheckContext, bool) {
	dec, ok := checkContextDecoders[n.Typename]
	if !ok {
		return nil, false
	}
	return dec(n), true
}

type rawContextsConnection struct {
	PageInfo github.PageInfo       `json:"pageInfo"`
	Nodes    []rawCheckContextNode `json:"nodes"`
}

type rawStatusCheckRollup struct {
	Contexts rawContextsConnection `json:"contexts"`
}

type rawCommit struct {
	ID                string                `json:"id"`
	Oid               string                `json:"oid"`
	StatusCheckRollup *rawStatusCheckRollup `json:"statusCheckRollup"`
}

type rawCommentNode struct {
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

type rawThreadNode struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	Line       *int   `json:"line"`
	Comments   struct {
		Nodes []rawCommentNode `json:"nodes"`
	} `json:"comments"`
}

type rawThreadsConnection struct {
	PageInfo github.PageInfo `json:"pageInfo"`
	Nodes    []rawThreadNode `json:"nodes"`
}

type rawPR struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	Mergeable   string `json:"mergeable"`
	Repository  *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	ReviewThreads *rawThreadsConnection `json:"reviewThreads"`
	Commits       *struct {
		Nodes []struct {
			Commit rawCommit `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type searchResponse struct {
	Search struct {
		IssueCount int             `json:"issueCount"`
		PageInfo   github.PageInfo `json:"pageInfo"`
		Nodes      []rawPR         `json:"nodes"`
	} `json:"search"`
}

type prTargetResponse struct {
	Repository struct {
		PullRequest *rawPR `json:"pullRequest"`
	} `json:"repository"`
}

type threadsPageResponse struct {
	Node struct {
		ReviewThreads rawThreadsConnection `json:"reviewThreads"`
	} `json:"node"`
}

type contextsPageResponse struct {
	Node struct {
		StatusCheckRollup *rawStatusCheckRollup `json:"statusCheckRollup"`
	} `json:"node"`
}

type annotationsPageResponse struct {
	Node struct {
		Annotations *rawAnnotationsConnection `json:"annotations"`
	} `json:"node"`
}

type rawRepo struct {
	NameWithOwner    string `json:"nameWithOwner"`
	IsArchived       bool   `json:"isArchived"`
	IsFork           bool   `json:"isFork"`
	DefaultBranchRef *struct {
		Name   string     `json:"name"`
		Target *rawCommit `json:"target"`
	} `json:"defaultBranchRef"`
}

type ownerReposResponse struct {
	RepositoryOwner *struct {
		Repositories struct {
			PageInfo github.PageInfo `json:"pageInfo"`
			Nodes    []rawRepo       `json:"nodes"`
		} `json:"repositories"`
	} `json:"repositoryOwner"`
}

type repoResponse struct {
	Repository *rawRepo `json:"repository"`
}

// pullRequestFromRaw converts a decoded PR node into scan's typed shape,
// fetching any overflow pages (threads beyond 30, check contexts beyond
// 100) its connections report.
func pullRequestFromRaw(ctx context.Context, gh github.Client, repo string, n rawPR, withAnnotations bool) (PullRequest, error) {
	pr := PullRequest{
		ID: n.ID, Repo: repo, Number: n.Number, Title: n.Title, URL: n.URL,
		HeadRef: n.HeadRefName, BaseRef: n.BaseRefName, IsDraft: n.IsDraft, Mergeable: n.Mergeable,
	}

	threads, err := threadsFromRaw(ctx, gh, n.ID, n.ReviewThreads)
	if err != nil {
		return PullRequest{}, err
	}
	pr.ReviewThreads = threads

	if n.Commits != nil && len(n.Commits.Nodes) > 0 {
		commit, err := commitFromRaw(ctx, gh, &n.Commits.Nodes[0].Commit, withAnnotations)
		if err != nil {
			return PullRequest{}, err
		}
		pr.Head = commit
	}
	return pr, nil
}

func threadsFromRaw(ctx context.Context, gh github.Client, prID string, conn *rawThreadsConnection) ([]ReviewThread, error) {
	if conn == nil {
		return nil, nil
	}
	nodes := conn.Nodes
	if conn.PageInfo.HasNextPage {
		more, err := fetchRemainingThreads(ctx, gh, prID, conn.PageInfo.EndCursor)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, more...)
	}
	out := make([]ReviewThread, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, reviewThreadFromRaw(n))
	}
	return out, nil
}

func reviewThreadFromRaw(n rawThreadNode) ReviewThread {
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
	return ReviewThread{ID: n.ID, Path: n.Path, Line: line, IsOutdated: n.IsOutdated, IsResolved: n.IsResolved, Comments: comments}
}

func fetchRemainingThreads(ctx context.Context, gh github.Client, prID, cursor string) ([]rawThreadNode, error) {
	var out []rawThreadNode
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

func commitFromRaw(ctx context.Context, gh github.Client, c *rawCommit, withAnnotations bool) (*Commit, error) {
	if c == nil || c.StatusCheckRollup == nil {
		return nil, nil
	}
	contexts, err := contextsFromRaw(ctx, gh, c.ID, c.StatusCheckRollup.Contexts, withAnnotations)
	if err != nil {
		return nil, err
	}
	return &Commit{ID: c.ID, Oid: c.Oid, Contexts: contexts}, nil
}

func contextsFromRaw(ctx context.Context, gh github.Client, commitID string, conn rawContextsConnection, withAnnotations bool) ([]CheckContext, error) {
	nodes := conn.Nodes
	if conn.PageInfo.HasNextPage {
		more, err := fetchRemainingContexts(ctx, gh, commitID, conn.PageInfo.EndCursor, withAnnotations)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, more...)
	}
	out := make([]CheckContext, 0, len(nodes))
	for _, n := range nodes {
		if !withAnnotations {
			// Keeps CheckContext.Annotations' "nil unless requested"
			// promise independent of what the response happened to carry.
			n.Annotations = nil
		}
		if n.Annotations != nil && n.Annotations.PageInfo.HasNextPage {
			more, err := fetchRemainingAnnotations(ctx, gh, n.ID, n.Annotations.PageInfo.EndCursor)
			if err != nil {
				return nil, err
			}
			n.Annotations.Nodes = append(n.Annotations.Nodes, more...)
		}
		if cc, ok := decodeCheckContext(n); ok {
			out = append(out, cc)
		}
	}
	return out, nil
}

func fetchRemainingContexts(ctx context.Context, gh github.Client, commitID, cursor string, withAnnotations bool) ([]rawCheckContextNode, error) {
	query := buildContextsPageQuery(withAnnotations)
	var out []rawCheckContextNode
	err := github.PaginateFrom(ctx, cursor, func(ctx context.Context, c string) (github.PageInfo, error) {
		var resp contextsPageResponse
		if err := gh.Query(ctx, query, map[string]any{"id": commitID, "after": c}, &resp); err != nil {
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

func fetchRemainingAnnotations(ctx context.Context, gh github.Client, checkRunID, cursor string) ([]rawAnnotationNode, error) {
	var out []rawAnnotationNode
	err := github.PaginateFrom(ctx, cursor, func(ctx context.Context, c string) (github.PageInfo, error) {
		var resp annotationsPageResponse
		if err := gh.Query(ctx, annotationsPageQuery, map[string]any{"id": checkRunID, "after": c}, &resp); err != nil {
			return github.PageInfo{}, err
		}
		if resp.Node.Annotations == nil {
			return github.PageInfo{}, nil
		}
		out = append(out, resp.Node.Annotations.Nodes...)
		return resp.Node.Annotations.PageInfo, nil
	})
	return out, err
}

func branchFromRaw(ctx context.Context, gh github.Client, repo rawRepo) (*Branch, error) {
	if repo.DefaultBranchRef == nil || repo.DefaultBranchRef.Target == nil {
		return nil, nil
	}
	commit, err := commitFromRaw(ctx, gh, repo.DefaultBranchRef.Target, false)
	if err != nil {
		return nil, err
	}
	if commit == nil {
		return nil, nil
	}
	return &Branch{Repo: repo.NameWithOwner, Name: repo.DefaultBranchRef.Name, Tip: commit}, nil
}
