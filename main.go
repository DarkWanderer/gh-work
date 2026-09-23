// Command gh-work is a gh CLI extension: `gh work` lists actionable work
// (unresolved review threads, failing checks) for an org, repo or PR.
package main

import (
	"os"

	"github.com/DarkWanderer/gh-work/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
