package cmd

import (
	"sort"
	"strings"

	"github.com/DarkWanderer/gh-work/internal/check"
)

// availableSourceNames lists every check name for --source's help text and
// the unknown-source error, sorted for a stable, alphabetical display
// (check.All.Names() is registration order, not display order).
func availableSourceNames() []string {
	names := check.All.Names()
	sort.Strings(names)
	return names
}

// selectSources parses a comma-separated --source value into the matching
// checks from check.All, preserving registration order. An empty value
// selects every registered check. Adding a check (see AGENTS.md) means: a
// new PRCheck/BranchCheck implementation in internal/check, and one line in
// check.All — this function needs no changes.
func selectSources(csv string) (check.Set, error) {
	if strings.TrimSpace(csv) == "" {
		return check.All, nil
	}
	var names []string
	for _, name := range strings.Split(csv, ",") {
		names = append(names, strings.TrimSpace(name))
	}

	selected, unknown := check.All.Select(names)
	if len(unknown) > 0 {
		return check.Set{}, usageErrorf("unknown --source %q (available: %s)", strings.Join(unknown, ","), strings.Join(availableSourceNames(), ", "))
	}
	return selected, nil
}
