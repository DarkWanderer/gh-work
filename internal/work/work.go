// Package work defines the unit of actionable work `gh work` reports on,
// and the GitHub check conclusions that count as failures. Item is a sealed
// interface (ReviewThread | PRCheckFailure | BranchCheckFailure |
// MergeConflict) so that, for example, a check failure can never be
// constructed without the branch or PR it belongs to.
package work

import (
	"encoding/json"
	"time"
)

// Kind identifies which concrete Item variant a value holds. It is also the
// "kind" discriminator written into JSON output.
type Kind string

const (
	KindReviewThread       Kind = "review-thread"
	KindPRCheckFailure     Kind = "pr-check-failure"
	KindBranchCheckFailure Kind = "branch-check-failure"
	KindMergeConflict      Kind = "merge-conflict"
)

// PRRef identifies the pull request an item belongs to.
type PRRef struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	HeadRef string `json:"headRef"`
	IsDraft bool   `json:"isDraft"`
}

// Comment is a single review comment within a thread.
type Comment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

// Thread is an unresolved PR review thread.
type Thread struct {
	Path       string    `json:"path"`
	Line       int       `json:"line,omitempty"`
	IsOutdated bool      `json:"isOutdated"`
	URL        string    `json:"url"`
	Comments   []Comment `json:"comments"`
}

// Check is a single failing CheckRun or StatusContext. RunID/JobID are only
// populated for GitHub Actions checks (CheckRun), never for commit statuses.
type Check struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"url"`
	Workflow   string `json:"workflow,omitempty"`
	RunID      int64  `json:"runId,omitempty"`
	JobID      int64  `json:"jobId,omitempty"`
}

// CheckRun conclusions that count as a failure. CANCELLED, NEUTRAL, SKIPPED,
// STALE and SUCCESS do not.
const (
	ConclusionFailure        = "FAILURE"
	ConclusionTimedOut       = "TIMED_OUT"
	ConclusionStartupFailure = "STARTUP_FAILURE"
	ConclusionActionRequired = "ACTION_REQUIRED"
)

// StatusContext states that count as a failure. SUCCESS, PENDING and
// EXPECTED do not.
const (
	StatusStateFailure = "FAILURE"
	StatusStateError   = "ERROR"
)

// IsFailingCheckRunConclusion reports whether a CheckRun conclusion counts
// as work needing attention.
func IsFailingCheckRunConclusion(conclusion string) bool {
	switch conclusion {
	case ConclusionFailure, ConclusionTimedOut, ConclusionStartupFailure, ConclusionActionRequired:
		return true
	default:
		return false
	}
}

// IsFailingStatusState reports whether a StatusContext state counts as work
// needing attention.
func IsFailingStatusState(state string) bool {
	switch state {
	case StatusStateFailure, StatusStateError:
		return true
	default:
		return false
	}
}

// Item is implemented by ReviewThread, PRCheckFailure, BranchCheckFailure
// and MergeConflict. The unexported method seals the interface to this
// package.
type Item interface {
	ItemID() string
	ItemKind() Kind
	ItemRepo() string
	json.Marshaler
	sealed()
}

// ReviewThread is an unresolved PR review thread.
type ReviewThread struct {
	ID     string
	Repo   string
	PR     PRRef
	Thread Thread
}

func (r ReviewThread) ItemID() string   { return r.ID }
func (r ReviewThread) ItemKind() Kind   { return KindReviewThread }
func (r ReviewThread) ItemRepo() string { return r.Repo }
func (ReviewThread) sealed()            {}

func (r ReviewThread) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID     string `json:"id"`
		Kind   Kind   `json:"kind"`
		Repo   string `json:"repo"`
		PR     PRRef  `json:"pr"`
		Thread Thread `json:"thread"`
	}{r.ID, KindReviewThread, r.Repo, r.PR, r.Thread})
}

// PRCheckFailure is a failing check on a PR's head commit.
type PRCheckFailure struct {
	ID     string
	Repo   string
	PR     PRRef
	Commit string
	Check  Check
}

func (c PRCheckFailure) ItemID() string   { return c.ID }
func (c PRCheckFailure) ItemKind() Kind   { return KindPRCheckFailure }
func (c PRCheckFailure) ItemRepo() string { return c.Repo }
func (PRCheckFailure) sealed()            {}

func (c PRCheckFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID     string `json:"id"`
		Kind   Kind   `json:"kind"`
		Repo   string `json:"repo"`
		PR     PRRef  `json:"pr"`
		Commit string `json:"commit"`
		Check  Check  `json:"check"`
	}{c.ID, KindPRCheckFailure, c.Repo, c.PR, c.Commit, c.Check})
}

// BranchCheckFailure is a failing check on a repo's default branch tip.
type BranchCheckFailure struct {
	ID     string
	Repo   string
	Branch string
	Commit string
	Check  Check
}

func (c BranchCheckFailure) ItemID() string   { return c.ID }
func (c BranchCheckFailure) ItemKind() Kind   { return KindBranchCheckFailure }
func (c BranchCheckFailure) ItemRepo() string { return c.Repo }
func (BranchCheckFailure) sealed()            {}

func (c BranchCheckFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID     string `json:"id"`
		Kind   Kind   `json:"kind"`
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
		Commit string `json:"commit"`
		Check  Check  `json:"check"`
	}{c.ID, KindBranchCheckFailure, c.Repo, c.Branch, c.Commit, c.Check})
}

// MergeConflict is an open PR whose mergeable state is CONFLICTING.
type MergeConflict struct {
	ID      string
	Repo    string
	PR      PRRef
	BaseRef string
}

func (m MergeConflict) ItemID() string   { return m.ID }
func (m MergeConflict) ItemKind() Kind   { return KindMergeConflict }
func (m MergeConflict) ItemRepo() string { return m.Repo }
func (MergeConflict) sealed()            {}

func (m MergeConflict) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID      string `json:"id"`
		Kind    Kind   `json:"kind"`
		Repo    string `json:"repo"`
		PR      PRRef  `json:"pr"`
		BaseRef string `json:"baseRef"`
	}{m.ID, KindMergeConflict, m.Repo, m.PR, m.BaseRef})
}
