// Package work defines the unit of actionable work `gh work` reports on,
// grouped by the PR or branch it belongs to, and the GitHub check
// conclusions that count as failures.
//
// Group is a sealed interface (PullRequest | Branch); PRItem is a sealed
// interface (ReviewThread | PRCheckFailure | MergeConflict) nested inside a
// PullRequest group. Both are sealed so that, for example, a check failure
// can never be constructed without the branch or PR it belongs to, and so
// that adding a variant is a compile error in every Visitor until it's
// updated (see GroupVisitor, PRItemVisitor) rather than a silently-ignored
// switch case.
package work

import (
	"fmt"
	"time"
)

// Kind identifies which concrete PRItem variant a value holds, or the shape
// of a branch's check failure. It is also the "kind" discriminator written
// into JSON output (see internal/render).
type Kind string

const (
	KindReviewThread       Kind = "review-thread"
	KindPRCheckFailure     Kind = "pr-check-failure"
	KindBranchCheckFailure Kind = "branch-check-failure"
	KindMergeConflict      Kind = "merge-conflict"
)

// Comment is a single review comment within a thread. JSON tags match the
// --json contract (docs/output.md) directly, since internal/render embeds
// this value as-is rather than re-shaping it.
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

var failingCheckRunConclusions = map[string]bool{
	ConclusionFailure:        true,
	ConclusionTimedOut:       true,
	ConclusionStartupFailure: true,
	ConclusionActionRequired: true,
}

var failingStatusStates = map[string]bool{
	StatusStateFailure: true,
	StatusStateError:   true,
}

// IsFailingCheckRunConclusion reports whether a CheckRun conclusion counts
// as work needing attention.
func IsFailingCheckRunConclusion(conclusion string) bool {
	return failingCheckRunConclusions[conclusion]
}

// IsFailingStatusState reports whether a StatusContext state counts as work
// needing attention.
func IsFailingStatusState(state string) bool {
	return failingStatusStates[state]
}

// CheckFailure is a single failing check together with the commit it ran
// against. Embedded by PRCheckFailure and held in Branch.Failures.
type CheckFailure struct {
	ID     string
	Commit string
	Check  Check
}

// GroupVisitor is implemented by callers that need variant-specific
// behavior for a Group (e.g. rendering), in place of a type switch.
type GroupVisitor interface {
	VisitPullRequest(PullRequest)
	VisitBranch(Branch)
}

// Group is implemented by PullRequest and Branch: the two things work items
// are grouped under. The unexported method seals the interface to this
// package.
type Group interface {
	// GroupRepo is the "owner/name" repository the group belongs to.
	GroupRepo() string
	// SortKey orders groups within a repo: PR-numbered groups (zero-padded,
	// so numeric order is preserved) before branch groups, then branches
	// alphabetically.
	SortKey() string
	// Accept dispatches to the matching GroupVisitor method.
	Accept(v GroupVisitor)
	sealed()
}

// PullRequest is an open PR carrying every PRItem found for it (unresolved
// review threads, failing head-commit checks, a merge conflict).
type PullRequest struct {
	ID      string
	Repo    string
	Number  int
	Title   string
	URL     string
	HeadRef string
	IsDraft bool
	Items   []PRItem
}

func (p PullRequest) GroupRepo() string     { return p.Repo }
func (p PullRequest) SortKey() string       { return fmt.Sprintf("0%010d", p.Number) }
func (p PullRequest) Accept(v GroupVisitor) { v.VisitPullRequest(p) }
func (PullRequest) sealed()                 {}

// Branch is a repo's default branch carrying every failing check found on
// its tip commit.
type Branch struct {
	Repo     string
	Name     string
	Commit   string
	Failures []CheckFailure
}

func (b Branch) GroupRepo() string     { return b.Repo }
func (b Branch) SortKey() string       { return "1" + b.Name }
func (b Branch) Accept(v GroupVisitor) { v.VisitBranch(b) }
func (Branch) sealed()                 {}

// PRItemVisitor is implemented by callers that need variant-specific
// behavior for a PRItem, in place of a type switch.
type PRItemVisitor interface {
	VisitReviewThread(ReviewThread)
	VisitPRCheckFailure(PRCheckFailure)
	VisitMergeConflict(MergeConflict)
}

// PRItem is implemented by ReviewThread, PRCheckFailure and MergeConflict:
// the kinds of work found on a single PR. The unexported method seals the
// interface to this package.
type PRItem interface {
	ItemID() string
	ItemKind() Kind
	// Accept dispatches to the matching PRItemVisitor method.
	Accept(v PRItemVisitor)
	sealed()
}

// ReviewThread is an unresolved PR review thread.
type ReviewThread struct {
	ID     string
	Thread Thread
}

func (r ReviewThread) ItemID() string         { return r.ID }
func (r ReviewThread) ItemKind() Kind         { return KindReviewThread }
func (r ReviewThread) Accept(v PRItemVisitor) { v.VisitReviewThread(r) }
func (ReviewThread) sealed()                  {}

// PRCheckFailure is a failing check on a PR's head commit.
type PRCheckFailure struct {
	CheckFailure
}

func (c PRCheckFailure) ItemID() string         { return c.ID }
func (c PRCheckFailure) ItemKind() Kind         { return KindPRCheckFailure }
func (c PRCheckFailure) Accept(v PRItemVisitor) { v.VisitPRCheckFailure(c) }
func (PRCheckFailure) sealed()                  {}

// MergeConflict is an open PR whose mergeable state is CONFLICTING.
type MergeConflict struct {
	ID      string
	BaseRef string
}

func (m MergeConflict) ItemID() string         { return m.ID }
func (m MergeConflict) ItemKind() Kind         { return KindMergeConflict }
func (m MergeConflict) Accept(v PRItemVisitor) { v.VisitMergeConflict(m) }
func (MergeConflict) sealed()                  {}
