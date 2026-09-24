// Package render turns a collected Envelope into the two output forms
// `gh work` supports: indented JSON for agents, and grouped, optionally
// colored text for humans.
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

// TargetInfoFrom converts a target.Target into its JSON representation.
func TargetInfoFrom(t target.Target) TargetInfo {
	switch v := t.(type) {
	case target.Org:
		return TargetInfo{Kind: "org", Owner: v.Owner}
	case target.Repo:
		return TargetInfo{Kind: "repo", Owner: v.Owner, Repo: v.Name}
	case target.PR:
		return TargetInfo{Kind: "pr", Owner: v.Owner, Repo: v.Name, Number: v.Number}
	default:
		panic(fmt.Sprintf("render: unknown target type %T", t))
	}
}

// SourceError is a per-source failure that did not prevent other sources
// from reporting items (partial failure).
type SourceError struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

// Envelope is the full `--json` output contract (schemaVersion: 1).
type Envelope struct {
	SchemaVersion int           `json:"schemaVersion"`
	Target        TargetInfo    `json:"target"`
	Items         []work.Item   `json:"items"`
	Warnings      []string      `json:"warnings"`
	Errors        []SourceError `json:"errors"`
}

// NewEnvelope builds an Envelope, normalizing nil slices to empty ones so
// they marshal as "[]" rather than "null".
func NewEnvelope(t target.Target, items []work.Item, warnings []string, errs []SourceError) Envelope {
	if items == nil {
		items = []work.Item{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	if errs == nil {
		errs = []SourceError{}
	}
	return Envelope{SchemaVersion: 1, Target: TargetInfoFrom(t), Items: items, Warnings: warnings, Errors: errs}
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

// Text writes e grouped by repo, then by PR or branch, in the order items
// already appear (callers are expected to pass deterministically sorted
// items, as collect.Run produces).
func Text(w io.Writer, e Envelope, useColor bool) error {
	if len(e.Items) == 0 {
		fmt.Fprintln(w, "No work found.")
	}
	for _, g := range groupItems(e.Items) {
		fmt.Fprintln(w, colorize(useColor, colorBold, g.repo))
		for _, sg := range g.subgroups {
			fmt.Fprintln(w, "  "+sg.header)
			for _, it := range sg.items {
				writeItemLines(w, it, useColor)
			}
		}
	}
	for _, wmsg := range e.Warnings {
		fmt.Fprintln(w, colorize(useColor, colorYellow, "warning: "+wmsg))
	}
	for _, em := range e.Errors {
		fmt.Fprintln(w, colorize(useColor, colorRed, fmt.Sprintf("error: %s: %s", em.Source, em.Message)))
	}
	return nil
}

type subgroup struct {
	header string
	items  []work.Item
}

type repoGroup struct {
	repo      string
	subgroups []subgroup
}

// groupItems buckets a pre-sorted item list into contiguous repo, then
// PR-or-branch groups, without re-sorting.
func groupItems(items []work.Item) []repoGroup {
	var groups []repoGroup
	var curRepo, curKey string
	for _, it := range items {
		repo := it.ItemRepo()
		key, header := subgroupKeyAndHeader(it)
		if len(groups) == 0 || curRepo != repo {
			groups = append(groups, repoGroup{repo: repo})
		}
		g := &groups[len(groups)-1]
		if len(g.subgroups) == 0 || curRepo != repo || curKey != key {
			g.subgroups = append(g.subgroups, subgroup{header: header})
		}
		curRepo, curKey = repo, key
		sg := &g.subgroups[len(g.subgroups)-1]
		sg.items = append(sg.items, it)
	}
	return groups
}

func subgroupKeyAndHeader(it work.Item) (key, header string) {
	switch v := it.(type) {
	case work.ReviewThread:
		return prKey(v.PR), prHeader(v.PR)
	case work.PRCheckFailure:
		return prKey(v.PR), prHeader(v.PR)
	case work.MergeConflict:
		return prKey(v.PR), prHeader(v.PR)
	case work.BranchCheckFailure:
		return "branch:" + v.Branch, "branch " + v.Branch
	default:
		return "", ""
	}
}

func prKey(pr work.PRRef) string { return fmt.Sprintf("pr:%d", pr.Number) }

func prHeader(pr work.PRRef) string {
	draft := ""
	if pr.IsDraft {
		draft = " (draft)"
	}
	return fmt.Sprintf("PR #%d%s %s %s", pr.Number, draft, pr.Title, pr.URL)
}

func writeItemLines(w io.Writer, it work.Item, useColor bool) {
	switch v := it.(type) {
	case work.ReviewThread:
		loc := v.Thread.Path
		if v.Thread.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, v.Thread.Line)
		}
		outdated := colorize(useColor, colorDim, boolLabel(v.Thread.IsOutdated, " (outdated)"))
		fmt.Fprintf(w, "    %s %s%s %s\n", colorize(useColor, colorCyan, "[review-thread]"), loc, outdated, v.Thread.URL)
		for _, c := range v.Thread.Comments {
			fmt.Fprintf(w, "        %s: %s\n", c.Author, firstLine(c.Body))
		}
	case work.PRCheckFailure:
		fmt.Fprintf(w, "    %s %s %s %s%s\n",
			colorize(useColor, colorCyan, "[pr-check-failure]"), v.Check.Name,
			colorize(useColor, colorRed, v.Check.Conclusion), v.Check.URL, runInfo(v.Check))
	case work.BranchCheckFailure:
		fmt.Fprintf(w, "    %s %s %s %s%s\n",
			colorize(useColor, colorCyan, "[branch-check-failure]"), v.Check.Name,
			colorize(useColor, colorRed, v.Check.Conclusion), v.Check.URL, runInfo(v.Check))
	case work.MergeConflict:
		fmt.Fprintf(w, "    %s conflicts with %s\n", colorize(useColor, colorCyan, "[merge-conflict]"), v.BaseRef)
	}
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
