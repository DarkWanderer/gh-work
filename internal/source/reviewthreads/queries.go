package reviewthreads

import (
	"time"

	"github.com/DarkWanderer/gh-work/internal/github"
)

// GitHub GraphQL rejects a query whose worst-case node count (summed across
// every nesting level, not just the leaf product) exceeds 500,000. search x
// reviewThreads x comments is three levels deep, so page sizes here must
// stay much smaller than the other sources' two-level queries: at 50 x 100
// x 100 this query alone requests 505,050 nodes and GitHub refuses it
// outright on any org with enough open PRs. 50 x 30 x 10 (16,550) leaves
// generous headroom; reviewThreads' own overflow pagination (see
// fetchRemainingThreads) covers PRs with more than 30 unresolved threads.

// searchQuery finds open PRs in scope and their first page of unresolved-
// candidate review threads (resolution is filtered client-side since the
// search API can't filter on it). Operation name is used by tests (and the
// fake client) to dispatch fixtures.
const searchQuery = `query SearchReviewThreads($q: String!, $after: String) {
  search(type: ISSUE, query: $q, first: 50, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes {
      ... on PullRequest {
        id
        number
        title
        url
        isDraft
        headRefName
        repository { nameWithOwner }
        reviewThreads(first: 30) {
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
  }
}`

// prQuery fetches review threads directly for a single PR target, paging
// through reviewThreads itself via $after.
const prQuery = `query PRReviewThreads($owner: String!, $repo: String!, $number: Int!, $after: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      id
      number
      title
      url
      isDraft
      headRefName
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

// threadsPageQuery re-fetches a search-discovered PR's review threads past
// the first page, via its node ID.
const threadsPageQuery = `query PRReviewThreadsPage($id: ID!, $after: String) {
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

type reviewThreadNode struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	Line       *int   `json:"line"`
	Comments   struct {
		Nodes []commentNode `json:"nodes"`
	} `json:"comments"`
}

type commentNode struct {
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

type reviewThreadsConnection struct {
	PageInfo github.PageInfo    `json:"pageInfo"`
	Nodes    []reviewThreadNode `json:"nodes"`
}

type prNode struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
	Repository  *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	ReviewThreads reviewThreadsConnection `json:"reviewThreads"`
}

type searchResponse struct {
	Search struct {
		IssueCount int             `json:"issueCount"`
		PageInfo   github.PageInfo `json:"pageInfo"`
		Nodes      []prNode        `json:"nodes"`
	} `json:"search"`
}

type prTargetResponse struct {
	Repository struct {
		PullRequest *prNode `json:"pullRequest"`
	} `json:"repository"`
}

type threadsPageResponse struct {
	Node struct {
		ReviewThreads reviewThreadsConnection `json:"reviewThreads"`
	} `json:"node"`
}
