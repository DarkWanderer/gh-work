package collect_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DarkWanderer/gh-work/internal/collect"
	"github.com/DarkWanderer/gh-work/internal/github/fake"
	"github.com/DarkWanderer/gh-work/internal/render"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/source/branchchecks"
	"github.com/DarkWanderer/gh-work/internal/source/mergeconflicts"
	"github.com/DarkWanderer/gh-work/internal/source/prchecks"
	"github.com/DarkWanderer/gh-work/internal/source/reviewthreads"
	"github.com/DarkWanderer/gh-work/internal/target"
)

// BenchmarkCollectAndRender drives the full collect+render pipeline (all
// four sources, through the fake GraphQL client) over a synthetic org with
// 1000 open PRs (GitHub search's own result cap) spread across 20 repos,
// each contributing one unresolved review thread, one failing check and a
// merge conflict, plus one failing default-branch check per repo.
func BenchmarkCollectAndRender(b *testing.B) {
	const (
		owner              = "bench"
		totalPRs           = 1000
		pageSize           = 50
		mergeConflictsPage = 100
		repoCount          = 20
	)

	c := fake.NewClient()
	registerReviewThreadsPages(c, owner, totalPRs, pageSize, repoCount)
	registerPRChecksPages(c, owner, totalPRs, pageSize, repoCount)
	registerMergeConflictsPages(c, owner, totalPRs, mergeConflictsPage, repoCount)
	registerOrgRepos(c, owner, repoCount)

	srcs := []source.Source{reviewthreads.New(), prchecks.New(), branchchecks.New(), mergeconflicts.New()}
	tgt := target.Org{Owner: owner}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := collect.Run(ctx, c, tgt, srcs, source.Options{})
		if len(res.Errors) != 0 {
			b.Fatalf("unexpected source errors: %#v", res.Errors)
		}
		env := render.NewEnvelope(tgt, res.Items, res.Warnings, nil)

		var jsonBuf bytes.Buffer
		if err := render.JSON(&jsonBuf, env); err != nil {
			b.Fatal(err)
		}
		var textBuf bytes.Buffer
		if err := render.Text(&textBuf, env, false); err != nil {
			b.Fatal(err)
		}
	}
}

func cursorName(page int) string { return fmt.Sprintf("CURSOR_%d", page) }

func registerReviewThreadsPages(c *fake.Client, owner string, total, pageSize, repoCount int) {
	q := "is:pr is:open archived:false user:" + owner
	for page := 0; page*pageSize < total; page++ {
		start := page*pageSize + 1
		end := min(start+pageSize-1, total)
		var after any
		if page > 0 {
			after = cursorName(page)
		}
		hasNext := end < total
		nextCursor := ""
		if hasNext {
			nextCursor = cursorName(page + 1)
		}

		nodes := make([]any, 0, end-start+1)
		for n := start; n <= end; n++ {
			repo := fmt.Sprintf("%s/repo%d", owner, n%repoCount)
			nodes = append(nodes, map[string]any{
				"id": fmt.Sprintf("PR_%d", n), "number": n, "title": fmt.Sprintf("PR %d", n),
				"url":     fmt.Sprintf("https://github.com/%s/pull/%d", repo, n),
				"isDraft": false, "headRefName": fmt.Sprintf("branch-%d", n),
				"repository": map[string]any{"nameWithOwner": repo},
				"reviewThreads": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
					"nodes": []any{
						map[string]any{
							"id": fmt.Sprintf("PRRT_%d", n), "isResolved": false, "isOutdated": false,
							"path": "main.go", "line": 1,
							"comments": map[string]any{"nodes": []any{
								map[string]any{"author": map[string]any{"login": "bot"}, "body": "issue",
									"url": fmt.Sprintf("https://github.com/%s/pull/%d#r1", repo, n), "createdAt": "2026-01-01T00:00:00Z"},
							}},
						},
					},
				},
			})
		}
		resp, _ := json.Marshal(map[string]any{
			"search": map[string]any{
				"issueCount": total,
				"pageInfo":   map[string]any{"hasNextPage": hasNext, "endCursor": nextCursor},
				"nodes":      nodes,
			},
		})
		c.SetFixture("SearchReviewThreads", map[string]any{"q": q, "after": after}, resp)
	}
}

func registerPRChecksPages(c *fake.Client, owner string, total, pageSize, repoCount int) {
	q := "is:pr is:open archived:false user:" + owner
	for page := 0; page*pageSize < total; page++ {
		start := page*pageSize + 1
		end := min(start+pageSize-1, total)
		var after any
		if page > 0 {
			after = cursorName(page)
		}
		hasNext := end < total
		nextCursor := ""
		if hasNext {
			nextCursor = cursorName(page + 1)
		}

		nodes := make([]any, 0, end-start+1)
		for n := start; n <= end; n++ {
			repo := fmt.Sprintf("%s/repo%d", owner, n%repoCount)
			nodes = append(nodes, map[string]any{
				"id": fmt.Sprintf("PR_%d", n), "number": n, "title": fmt.Sprintf("PR %d", n),
				"url":     fmt.Sprintf("https://github.com/%s/pull/%d", repo, n),
				"isDraft": false, "headRefName": fmt.Sprintf("branch-%d", n),
				"repository": map[string]any{"nameWithOwner": repo},
				"commits": map[string]any{
					"nodes": []any{
						map[string]any{"commit": map[string]any{
							"id": fmt.Sprintf("C_%d", n), "oid": fmt.Sprintf("%07x", n),
							"statusCheckRollup": map[string]any{
								"contexts": map[string]any{
									"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
									"nodes": []any{
										map[string]any{
											"__typename": "CheckRun", "id": fmt.Sprintf("CR_%d", n),
											"name": "build", "conclusion": "FAILURE", "status": "COMPLETED",
											"databaseId": n, "detailsUrl": fmt.Sprintf("https://github.com/%s/pull/%d/checks/1", repo, n),
											"checkSuite": map[string]any{"workflowRun": map[string]any{
												"databaseId": n + 1_000_000, "workflow": map[string]any{"name": "CI"},
											}},
										},
									},
								},
							},
						}},
					},
				},
			})
		}
		resp, _ := json.Marshal(map[string]any{
			"search": map[string]any{
				"issueCount": total,
				"pageInfo":   map[string]any{"hasNextPage": hasNext, "endCursor": nextCursor},
				"nodes":      nodes,
			},
		})
		c.SetFixture("SearchPRChecks", map[string]any{"q": q, "after": after}, resp)
	}
}

func registerMergeConflictsPages(c *fake.Client, owner string, total, pageSize, repoCount int) {
	q := "is:pr is:open archived:false user:" + owner
	for page := 0; page*pageSize < total; page++ {
		start := page*pageSize + 1
		end := min(start+pageSize-1, total)
		var after any
		if page > 0 {
			after = cursorName(page)
		}
		hasNext := end < total
		nextCursor := ""
		if hasNext {
			nextCursor = cursorName(page + 1)
		}

		nodes := make([]any, 0, end-start+1)
		for n := start; n <= end; n++ {
			repo := fmt.Sprintf("%s/repo%d", owner, n%repoCount)
			nodes = append(nodes, map[string]any{
				"id": fmt.Sprintf("PR_%d", n), "number": n, "title": fmt.Sprintf("PR %d", n),
				"url":         fmt.Sprintf("https://github.com/%s/pull/%d", repo, n),
				"isDraft":     false,
				"headRefName": fmt.Sprintf("branch-%d", n), "baseRefName": "main",
				"mergeable":  "CONFLICTING",
				"repository": map[string]any{"nameWithOwner": repo},
			})
		}
		resp, _ := json.Marshal(map[string]any{
			"search": map[string]any{
				"issueCount": total,
				"pageInfo":   map[string]any{"hasNextPage": hasNext, "endCursor": nextCursor},
				"nodes":      nodes,
			},
		})
		c.SetFixture("SearchMergeConflicts", map[string]any{"q": q, "after": after}, resp)
	}
}

func registerOrgRepos(c *fake.Client, owner string, repoCount int) {
	nodes := make([]any, repoCount)
	for i := 0; i < repoCount; i++ {
		repo := fmt.Sprintf("%s/repo%d", owner, i)
		nodes[i] = map[string]any{
			"nameWithOwner": repo, "isArchived": false, "isFork": false,
			"defaultBranchRef": map[string]any{
				"name": "main",
				"target": map[string]any{
					"id": fmt.Sprintf("C_branch_%d", i), "oid": fmt.Sprintf("%07x", i),
					"statusCheckRollup": map[string]any{
						"contexts": map[string]any{
							"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
							"nodes": []any{
								map[string]any{
									"__typename": "StatusContext", "id": fmt.Sprintf("SC_branch_%d", i),
									"context": "ci/status", "state": "FAILURE",
									"targetUrl": fmt.Sprintf("https://ci.example.com/%s", repo),
								},
							},
						},
					},
				},
			},
		}
	}
	resp, _ := json.Marshal(map[string]any{
		"repositoryOwner": map[string]any{
			"repositories": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes":    nodes,
			},
		},
	})
	c.SetFixture("OrgRepositories", map[string]any{"login": owner, "after": nil}, resp)
}
