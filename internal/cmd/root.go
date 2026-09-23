package cmd

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newRootCmd(opts Options, workFound *bool) *cobra.Command {
	var (
		filters     []string
		sourceNames string
		jsonOutput  bool
		exitStatus  bool
		watch       bool
		interval    time.Duration
		timeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "work [<target>]",
		Short: "List actionable review-thread and check-failure work for an org, repo, or PR",
		Long: `gh work lists actionable work for an org, repo or PR:
  - unresolved PR review threads
  - failing checks on PR head commits
  - failing checks on repos' default branches

<target> is an owner, owner/repo, owner/repo#N, or a github.com URL to a
repo or PR. Omit it to use the repo in the current directory.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return run(c.Context(), opts, runParams{
				args:        args,
				filters:     filters,
				sourceNames: sourceNames,
				jsonOutput:  jsonOutput,
				exitStatus:  exitStatus,
				watch:       watch,
				interval:    interval,
				timeout:     timeout,
			}, workFound)
		},
	}

	cmd.Flags().StringArrayVar(&filters, "filter", nil, "extra search qualifier, e.g. involves:@me (repeatable; not valid with a PR target)")
	cmd.Flags().StringVar(&sourceNames, "source", "", "comma-separated sources to run: "+strings.Join(availableSourceNames(), ", ")+" (default: all)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit a machine-readable JSON envelope")
	cmd.Flags().BoolVar(&exitStatus, "exit-status", false, "exit 8 if any work was found")
	cmd.Flags().BoolVar(&watch, "watch", false, "keep polling until work is present")
	cmd.Flags().DurationVar(&interval, "interval", 60*time.Second, "poll interval for --watch (minimum 10s)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "give up --watch after this long and exit 0 with an empty result (0 = no timeout)")

	return cmd
}
