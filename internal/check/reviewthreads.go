package check

import (
	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// ReviewThreads reports a PR's unresolved review threads.
type ReviewThreads struct{}

func (ReviewThreads) Name() string       { return "review-threads" }
func (ReviewThreads) Needs() scan.Fields { return scan.ReviewThreads }

// Inspect drops resolved threads; the thread's URL is taken from its first
// comment, since a review thread has no URL of its own, falling back to the
// PR's URL for the (theoretical) case of a thread with no comments.
func (ReviewThreads) Inspect(pr scan.PullRequest) []work.PRItem {
	var items []work.PRItem
	for _, t := range pr.ReviewThreads {
		if t.IsResolved {
			continue
		}
		threadURL := pr.URL
		if len(t.Comments) > 0 {
			threadURL = t.Comments[0].URL
		}
		items = append(items, work.ReviewThread{
			ID: t.ID,
			Thread: work.Thread{
				Path: t.Path, Line: t.Line, IsOutdated: t.IsOutdated, URL: threadURL, Comments: t.Comments,
			},
		})
	}
	return items
}
