package prchecks

import "github.com/DarkWanderer/gh-work/internal/github"

// checkContextsFragment is inlined into every query below; kept as a
// comment (GraphQL has no real #include) purely so the three copies stay
// visibly identical when reading a diff:
//
//	pageInfo { hasNextPage endCursor }
//	nodes {
//	  __typename
//	  ... on CheckRun {
//	    id name conclusion status databaseId detailsUrl
//	    checkSuite { workflowRun { databaseId workflow { name } } }
//	  }
//	  ... on StatusContext { id context state targetUrl }
//	}
const checkContextsFragment = `pageInfo { hasNextPage endCursor }
          nodes {
            __typename
            ... on CheckRun {
              id
              name
              conclusion
              status
              databaseId
              detailsUrl
              checkSuite {
                workflowRun {
                  databaseId
                  workflow { name }
                }
              }
            }
            ... on StatusContext {
              id
              context
              state
              targetUrl
            }
          }`

var searchQuery = `query SearchPRChecks($q: String!, $after: String) {
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
        commits(last: 1) {
          nodes {
            commit {
              id
              oid
              statusCheckRollup {
                contexts(first: 100) {
                  ` + checkContextsFragment + `
                }
              }
            }
          }
        }
      }
    }
  }
}`

var prQuery = `query PRChecks($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      id
      number
      title
      url
      isDraft
      headRefName
      commits(last: 1) {
        nodes {
          commit {
            id
            oid
            statusCheckRollup {
              contexts(first: 100) {
                ` + checkContextsFragment + `
              }
            }
          }
        }
      }
    }
  }
}`

var contextsPageQuery = `query CommitCheckContextsPage($id: ID!, $after: String) {
  node(id: $id) {
    ... on Commit {
      statusCheckRollup {
        contexts(first: 100, after: $after) {
          ` + checkContextsFragment + `
        }
      }
    }
  }
}`

// checkContextNode is the flattened union of CheckRun and StatusContext
// fields; whichever the API actually returned is discriminated by
// Typename, and the fields belonging to the other variant stay zero.
type checkContextNode struct {
	Typename   string `json:"__typename"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	Status     string `json:"status"`
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

	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

type contextsConnection struct {
	PageInfo github.PageInfo    `json:"pageInfo"`
	Nodes    []checkContextNode `json:"nodes"`
}

type statusCheckRollup struct {
	Contexts contextsConnection `json:"contexts"`
}

type commitNode struct {
	ID                string             `json:"id"`
	Oid               string             `json:"oid"`
	StatusCheckRollup *statusCheckRollup `json:"statusCheckRollup"`
}

type commitsConnection struct {
	Nodes []struct {
		Commit commitNode `json:"commit"`
	} `json:"nodes"`
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
	Commits commitsConnection `json:"commits"`
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

type contextsPageResponse struct {
	Node struct {
		StatusCheckRollup *statusCheckRollup `json:"statusCheckRollup"`
	} `json:"node"`
}
