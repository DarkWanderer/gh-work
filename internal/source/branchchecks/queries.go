package branchchecks

import "github.com/DarkWanderer/gh-work/internal/github"

// checkContextsFragment mirrors the one in internal/source/prchecks; kept
// as its own copy per source per the project's decoupling choice (see
// AGENTS.md).
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

// orgReposQuery pages non-archived, non-fork repos owned by an org or user
// and their default branch's latest check rollup. isArchived/isFork are
// re-checked client-side rather than passed as connection arguments, since
// those filter args are not reliably present on the RepositoryOwner
// interface across both Organization and User.
var orgReposQuery = `query OrgRepositories($login: String!, $after: String) {
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

var repoQuery = `query RepoDefaultBranchChecks($owner: String!, $name: String!) {
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
              ` + checkContextsFragment + `
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

type commitTarget struct {
	ID                string             `json:"id"`
	Oid               string             `json:"oid"`
	StatusCheckRollup *statusCheckRollup `json:"statusCheckRollup"`
}

type repoNode struct {
	NameWithOwner    string `json:"nameWithOwner"`
	IsArchived       bool   `json:"isArchived"`
	IsFork           bool   `json:"isFork"`
	DefaultBranchRef *struct {
		Name   string        `json:"name"`
		Target *commitTarget `json:"target"`
	} `json:"defaultBranchRef"`
}

type orgReposResponse struct {
	RepositoryOwner *struct {
		Repositories struct {
			PageInfo github.PageInfo `json:"pageInfo"`
			Nodes    []repoNode      `json:"nodes"`
		} `json:"repositories"`
	} `json:"repositoryOwner"`
}

type repoResponse struct {
	Repository *repoNode `json:"repository"`
}

type contextsPageResponse struct {
	Node struct {
		StatusCheckRollup *statusCheckRollup `json:"statusCheckRollup"`
	} `json:"node"`
}
