// Package source defines the plugin boundary for a work source. Adding a
// new kind of work (see AGENTS.md) means: a new work.Item variant, a new
// package here implementing Source, and one line registering it in
// internal/cmd.
package source

import (
	"context"

	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

// Options carries the parts of the CLI invocation a Source may need beyond
// the target itself.
type Options struct {
	// Filters are raw search qualifiers from repeated --filter flags,
	// appended to any PR search a source runs. Usage is rejected at the
	// CLI layer for PR targets, so a Source need not validate this itself.
	Filters []string
}

// Result is what a single Source.Collect call produces. A non-nil error
// from Collect is treated as a hard per-source failure (goes to the
// envelope's errors); Warnings are soft, non-fatal notices (e.g. a search
// result cap) reported alongside successfully collected Items.
type Result struct {
	Items    []work.Item
	Warnings []string
}

// Source is one pluggable kind of "work" gh-work knows how to collect.
type Source interface {
	// Name identifies the source for --source and for envelope errors.
	Name() string
	// Supports reports whether this source applies to the given target
	// kind at all (e.g. branch-checks does not apply to a PR target).
	Supports(t target.Target) bool
	// Collect fetches this source's items for t. Implementations must
	// respect ctx cancellation.
	Collect(ctx context.Context, gh github.Client, t target.Target, opts Options) (Result, error)
}
