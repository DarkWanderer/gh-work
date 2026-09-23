package cmd

import (
	"sort"
	"strings"

	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/source/branchchecks"
	"github.com/DarkWanderer/gh-work/internal/source/prchecks"
	"github.com/DarkWanderer/gh-work/internal/source/reviewthreads"
)

// allSources is the registry of known work sources. Adding a new source
// (see AGENTS.md) means: a new work.Item variant, a new package under
// internal/source implementing source.Source, and one line here.
var allSources = []source.Source{
	reviewthreads.New(),
	prchecks.New(),
	branchchecks.New(),
}

func availableSourceNames() []string {
	names := make([]string, len(allSources))
	for i, s := range allSources {
		names[i] = s.Name()
	}
	sort.Strings(names)
	return names
}

// selectSources parses a comma-separated --source value into the matching
// registered sources, preserving registry order. An empty value selects
// every registered source.
func selectSources(csv string) ([]source.Source, error) {
	if strings.TrimSpace(csv) == "" {
		return allSources, nil
	}
	wanted := map[string]bool{}
	for _, name := range strings.Split(csv, ",") {
		wanted[strings.TrimSpace(name)] = true
	}

	var selected []source.Source
	for _, s := range allSources {
		if wanted[s.Name()] {
			selected = append(selected, s)
			delete(wanted, s.Name())
		}
	}
	if len(wanted) > 0 {
		unknown := make([]string, 0, len(wanted))
		for name := range wanted {
			unknown = append(unknown, name)
		}
		sort.Strings(unknown)
		return nil, usageErrorf("unknown --source %q (available: %s)", strings.Join(unknown, ","), strings.Join(availableSourceNames(), ", "))
	}
	return selected, nil
}
