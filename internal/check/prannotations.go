package check

import (
	"strconv"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// PRAnnotations reports failure-level CheckRun annotations on a PR's head
// commit, regardless of the run's own conclusion: a run can pass (e.g. a
// continue-on-error lint step) while still pinning errors to lines.
type PRAnnotations struct{}

func (PRAnnotations) Name() string       { return "pr-annotations" }
func (PRAnnotations) Needs() scan.Fields { return scan.HeadAnnotations }

func (PRAnnotations) Inspect(pr scan.PullRequest) []work.PRItem {
	if pr.Head == nil {
		return nil
	}
	var items []work.PRItem
	for _, ctx := range pr.Head.Contexts {
		for i, a := range ctx.Annotations() {
			if !work.IsFailingAnnotationLevel(a.Level) {
				continue
			}
			items = append(items, work.PRAnnotation{
				ID:     annotationID(ctx.ID(), a, i),
				Commit: pr.Head.Oid,
				Check:  ctx.Check(),
				Annotation: work.Annotation{
					Level: a.Level, Path: a.Path, StartLine: a.StartLine, EndLine: a.EndLine,
					Title: a.Title, Message: a.Message, URL: a.URL,
				},
			})
		}
	}
	return items
}

// annotationID scopes the annotation's databaseId to its CheckRun, since
// CheckAnnotation has no GraphQL node ID of its own. The positional
// fallback only exists because the schema marks databaseId nullable; it is
// stable only as long as GitHub returns annotations in the same order.
func annotationID(checkRunID string, a scan.Annotation, index int) string {
	if a.DatabaseID != 0 {
		return checkRunID + ":" + strconv.FormatInt(a.DatabaseID, 10)
	}
	return checkRunID + ":#" + strconv.Itoa(index)
}
