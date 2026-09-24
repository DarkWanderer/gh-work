package mergeconflicts

import "github.com/DarkWanderer/gh-work/internal/github"

// The page size is 100, not GitHub search's default 50, because this query
// has no nested connections (unlike prchecks' commits->contexts chain), so
// its per-node cost stays low even at that width.
var searchQuery = `query SearchMergeConflicts($q: String!, $after: String) {
  search(type: ISSUE, query: $q, first: 100, after: $after) {
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
        baseRefName
        mergeable
        repository { nameWithOwner }
      }
    }
  }
}`

var prQuery = `query PRMergeConflict($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      id
      number
      title
      url
      isDraft
      headRefName
      baseRefName
      mergeable
    }
  }
}`

type prNode struct {
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
