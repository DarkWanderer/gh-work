// Package cmd wires up the `gh work` cobra command: flag parsing and
// validation, the watch loop, and exit codes.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/DarkWanderer/gh-work/internal/github"
)

// Options configures a run of gh-work. The zero value runs for real
// (os.Stdout/os.Stderr/os.Args, a live GitHub client, a real clock); tests
// override Stdout/Stderr/Args/GH/Sleep to run hermetically.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
	Args   []string

	// GH overrides the GitHub client (tests inject a fake); nil builds a
	// real one from gh's resolved auth.
	GH github.Client

	// Sleep overrides the --watch poll wait (tests inject an instant or
	// cancel-aware one); nil uses a real timer.
	Sleep func(ctx context.Context, d time.Duration) error
}

// exitError carries a specific process exit code. A nil err means the code
// alone is the signal (e.g. partial failure, interrupted) and nothing
// further needs printing — the reason is already visible in the envelope
// that was rendered before returning it.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("exit %d", e.code)
}

func (e *exitError) Unwrap() error { return e.err }

func usageErrorf(format string, a ...any) error {
	return &exitError{code: 2, err: fmt.Errorf(format, a...)}
}

func apiErrorf(format string, a ...any) error {
	return &exitError{code: 1, err: fmt.Errorf(format, a...)}
}

// exitCodeForFailure is 130 if ctx was cancelled (SIGINT raced the
// request), else 1 (a genuine API/partial failure).
func exitCodeForFailure(ctx context.Context) int {
	if ctx.Err() != nil {
		return 130
	}
	return 1
}

// Execute is the real entry point: os.Args, a signal-cancelable context for
// Ctrl-C, and the live GitHub client. It returns the process exit code;
// main.go does os.Exit(cmd.Execute()).
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return RunContext(ctx, Options{})
}

// RunContext runs gh-work with explicit context and Options, for both
// Execute and tests. It never calls os.Exit; it returns the exit code.
func RunContext(ctx context.Context, opts Options) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Args == nil {
		opts.Args = os.Args[1:]
	}

	workFound := false
	root := newRootCmd(opts, &workFound)
	root.SetArgs(opts.Args)
	root.SetOut(opts.Stdout)
	root.SetErr(opts.Stderr)
	root.SilenceUsage = true
	root.SilenceErrors = true

	err := root.ExecuteContext(ctx)

	var ee *exitError
	if errors.As(err, &ee) {
		if ee.err != nil {
			fmt.Fprintln(opts.Stderr, "gh work:", ee.err)
		}
		return ee.code
	}
	if err != nil {
		fmt.Fprintln(opts.Stderr, "gh work:", err)
		return 2
	}
	if workFound {
		return 8
	}
	return 0
}
