// Package render turns a collected Envelope into the two output forms
// `gh work` supports: indented JSON for agents (the flat v1 --json
// contract, docs/output.md), and grouped, optionally colored text for
// humans. Both walk work.Group/work.PRItem via their Visitor, never a type
// switch, so a new variant is a compile error here until handled.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// TargetInfo is the JSON-facing description of what was scanned.
type TargetInfo struct {
	Kind   string `json:"kind"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo,omitempty"`
	Number int    `json:"number,omitempty"`
}

// targetInfoVisitor builds a TargetInfo via target.Visitor dispatch.
type targetInfoVisitor struct{ info TargetInfo }

func (v *targetInfoVisitor) VisitOrg(o target.Org) error {
	v.info = TargetInfo{Kind: "org", Owner: o.Owner}
	return nil
}

func (v *targetInfoVisitor) VisitRepo(r target.Repo) error {
	v.info = TargetInfo{Kind: "repo", Owner: r.Owner, Repo: r.Name}
	return nil
}

func (v *targetInfoVisitor) VisitPR(p target.PR) error {
	v.info = TargetInfo{Kind: "pr", Owner: p.Owner, Repo: p.Name, Number: p.Number}
	return nil
}

// TargetInfoFrom converts a target.Target into its JSON representation.
func TargetInfoFrom(t target.Target) TargetInfo {
	var v targetInfoVisitor
	_ = t.Accept(&v) // targetInfoVisitor never returns an error
	return v.info
}

// SourceError is a per-check failure that did not prevent other checks
// from reporting groups (partial failure).
type SourceError struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

// v1PRRef is the --json contract's embedded "pr" shape.
type v1PRRef struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	HeadRef string `json:"headRef"`
	IsDraft bool   `json:"isDraft"`
}

// The v1* types below are the flat --json contract's item shapes
// (docs/output.md); each mirrors a work.PRItem/CheckFailure variant plus
// the PR or branch it was found in, which the grouped work.Group no longer
// repeats per item.
type v1ReviewThread struct {
	ID     string      `json:"id"`
	Kind   work.Kind   `json:"kind"`
	Repo   string      `json:"repo"`
	PR     v1PRRef     `json:"pr"`
	Thread work.Thread `json:"thread"`
}

type v1PRCheckFailure struct {
	ID     string     `json:"id"`
	Kind   work.Kind  `json:"kind"`
	Repo   string     `json:"repo"`
	PR     v1PRRef    `json:"pr"`
	Commit string     `json:"commit"`
	Check  work.Check `json:"check"`
}

type v1MergeConflict struct {
	ID      string    `json:"id"`
	Kind    work.Kind `json:"kind"`
	Repo    string    `json:"repo"`
	PR      v1PRRef   `json:"pr"`
	BaseRef string    `json:"baseRef"`
}

type v1BranchCheckFailure struct {
	ID     string     `json:"id"`
	Kind   work.Kind  `json:"kind"`
	Repo   string     `json:"repo"`
	Branch string     `json:"branch"`
	Commit string     `json:"commit"`
	Check  work.Check `json:"check"`
}

// v1Builder flattens work.Group/work.PRItem into the v1 --json contract's
// flat items array, via GroupVisitor/PRItemVisitor dispatch instead of a
// type switch.
type v1Builder struct {
	items []any
}

func (b *v1Builder) VisitPullRequest(p work.PullRequest) {
	ref := v1PRRef{Number: p.Number, Title: p.Title, URL: p.URL, HeadRef: p.HeadRef, IsDraft: p.IsDraft}
	ib := prItemBuilder{b: b, repo: p.Repo, pr: ref}
	for _, it := range p.Items {
		it.Accept(ib)
	}
}

func (b *v1Builder) VisitBranch(br work.Branch) {
	for _, f := range br.Failures {
		b.items = append(b.items, v1BranchCheckFailure{
			ID: f.ID, Kind: work.KindBranchCheckFailure, Repo: br.Repo, Branch: br.Name, Commit: f.Commit, Check: f.Check,
		})
	}
}

type prItemBuilder struct {
	b    *v1Builder
	repo string
	pr   v1PRRef
}

func (p prItemBuilder) VisitReviewThread(t work.ReviewThread) {
	p.b.items = append(p.b.items, v1ReviewThread{ID: t.ID, Kind: work.KindReviewThread, Repo: p.repo, PR: p.pr, Thread: t.Thread})
}

func (p prItemBuilder) VisitPRCheckFailure(c work.PRCheckFailure) {
	p.b.items = append(p.b.items, v1PRCheckFailure{
		ID: c.ID, Kind: work.KindPRCheckFailure, Repo: p.repo, PR: p.pr, Commit: c.Commit, Check: c.Check,
	})
}

func (p prItemBuilder) VisitMergeConflict(m work.MergeConflict) {
	p.b.items = append(p.b.items, v1MergeConflict{ID: m.ID, Kind: work.KindMergeConflict, Repo: p.repo, PR: p.pr, BaseRef: m.BaseRef})
}

func flattenItems(groups []work.Group) []any {
	b := &v1Builder{items: []any{}}
	for _, g := range groups {
		g.Accept(b)
	}
	return b.items
}

// Envelope is the full `--json` output contract (schemaVersion: 1). Groups
// is the grouped data Text renders from; it's excluded from JSON since
// Items already carries the flattened v1 shape.
type Envelope struct {
	SchemaVersion int           `json:"schemaVersion"`
	Target        TargetInfo    `json:"target"`
	Items         []any         `json:"items"`
	Warnings      []string      `json:"warnings"`
	Errors        []SourceError `json:"errors"`
	Groups        []work.Group  `json:"-"`
}

// NewEnvelope builds an Envelope, normalizing nil slices to empty ones so
// they marshal as "[]" rather than "null".
func NewEnvelope(t target.Target, groups []work.Group, warnings []string, errs []SourceError) Envelope {
	if warnings == nil {
		warnings = []string{}
	}
	if errs == nil {
		errs = []SourceError{}
	}
	return Envelope{
		SchemaVersion: 1, Target: TargetInfoFrom(t), Items: flattenItems(groups),
		Warnings: warnings, Errors: errs, Groups: groups,
	}
}

// JSON writes e as indented JSON, matching docs/output.md.
func JSON(w io.Writer, e Envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(e)
}

const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[31m"
	colorYellow = "\x1b[33m"
	colorBold   = "\x1b[1m"
	colorDim    = "\x1b[2m"
	colorCyan   = "\x1b[36m"
)

func colorize(useColor bool, code, s string) string {
	if !useColor || s == "" {
		return s
	}
	return code + s + colorReset
}

// Text writes e grouped by repo, then by PR or branch, in the order
// e.Groups already appears (callers are expected to pass deterministically
// sorted groups, as collect.Run produces).
func Text(w io.Writer, e Envelope, useColor bool) error {
	if len(e.Groups) == 0 {
		fmt.Fprintln(w, "No work found.")
	}
	tw := &textWriter{w: w, useColor: useColor}
	for _, g := range e.Groups {
		g.Accept(tw)
	}
	for _, wmsg := range e.Warnings {
		fmt.Fprintln(w, colorize(useColor, colorYellow, "warning: "+wmsg))
	}
	for _, em := range e.Errors {
		fmt.Fprintln(w, colorize(useColor, colorRed, fmt.Sprintf("error: %s: %s", em.Source, em.Message)))
	}
	return nil
}

// textWriter implements work.GroupVisitor, printing a repo header only
// when the repo changes from the previous group (groups are pre-sorted by
// repo, so this needs no lookahead or buffering).
type textWriter struct {
	w        io.Writer
	useColor bool
	curRepo  string
	wroteAny bool
}

func (tw *textWriter) repoHeader(repo string) {
	if !tw.wroteAny || repo != tw.curRepo {
		fmt.Fprintln(tw.w, colorize(tw.useColor, colorBold, repo))
		tw.curRepo, tw.wroteAny = repo, true
	}
}

func (tw *textWriter) VisitPullRequest(p work.PullRequest) {
	tw.repoHeader(p.Repo)
	fmt.Fprintln(tw.w, "  "+prHeader(p))
	il := itemLineWriter{w: tw.w, useColor: tw.useColor}
	for _, it := range p.Items {
		it.Accept(il)
	}
}

func (tw *textWriter) VisitBranch(b work.Branch) {
	tw.repoHeader(b.Repo)
	fmt.Fprintln(tw.w, "  branch "+b.Name)
	for _, f := range b.Failures {
		writeCheckFailureLine(tw.w, "branch-check-failure", f, tw.useColor)
	}
}

func prHeader(p work.PullRequest) string {
	draft := ""
	if p.IsDraft {
		draft = " (draft)"
	}
	return fmt.Sprintf("PR #%d%s %s %s", p.Number, draft, p.Title, p.URL)
}

// itemLineWriter implements work.PRItemVisitor, printing one PR's items.
type itemLineWriter struct {
	w        io.Writer
	useColor bool
}

func (l itemLineWriter) VisitReviewThread(t work.ReviewThread) {
	loc := t.Thread.Path
	if t.Thread.Line > 0 {
		loc = fmt.Sprintf("%s:%d", loc, t.Thread.Line)
	}
	outdated := colorize(l.useColor, colorDim, boolLabel(t.Thread.IsOutdated, " (outdated)"))
	fmt.Fprintf(l.w, "    %s %s%s %s\n", colorize(l.useColor, colorCyan, "[review-thread]"), loc, outdated, t.Thread.URL)
	for _, c := range t.Thread.Comments {
		fmt.Fprintf(l.w, "        %s: %s\n", c.Author, firstLine(c.Body))
	}
}

func (l itemLineWriter) VisitPRCheckFailure(c work.PRCheckFailure) {
	writeCheckFailureLine(l.w, "pr-check-failure", c.CheckFailure, l.useColor)
}

func (l itemLineWriter) VisitMergeConflict(m work.MergeConflict) {
	fmt.Fprintf(l.w, "    %s conflicts with %s\n", colorize(l.useColor, colorCyan, "[merge-conflict]"), m.BaseRef)
}

func writeCheckFailureLine(w io.Writer, kind string, f work.CheckFailure, useColor bool) {
	fmt.Fprintf(w, "    %s %s %s %s%s\n",
		colorize(useColor, colorCyan, "["+kind+"]"), f.Check.Name,
		colorize(useColor, colorRed, f.Check.Conclusion), f.Check.URL, runInfo(f.Check))
}

func boolLabel(b bool, label string) string {
	if b {
		return label
	}
	return ""
}

func runInfo(c work.Check) string {
	if c.RunID == 0 {
		return ""
	}
	return fmt.Sprintf(" (run %d job %d)", c.RunID, c.JobID)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}
