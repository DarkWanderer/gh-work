package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cli/go-gh/v2/pkg/term"

	"github.com/DarkWanderer/gh-work/internal/check"
	"github.com/DarkWanderer/gh-work/internal/collect"
	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/render"
	"github.com/DarkWanderer/gh-work/internal/target"
)

const minWatchInterval = 10 * time.Second

type runParams struct {
	args        []string
	filters     []string
	sourceNames string
	jsonOutput  bool
	exitStatus  bool
	watch       bool
	interval    time.Duration
	timeout     time.Duration
}

func run(ctx context.Context, opts Options, p runParams, workFound *bool) error {
	tgt, err := resolveTarget(p.args)
	if err != nil {
		return usageErrorf("%v", err)
	}

	if len(p.filters) > 0 && isPRTarget(tgt) {
		return usageErrorf("--filter cannot be used with a PR target")
	}
	if p.watch && p.interval < minWatchInterval {
		return usageErrorf("--interval must be at least %s", minWatchInterval)
	}

	checks, err := selectSources(p.sourceNames)
	if err != nil {
		return err
	}

	gh := opts.GH
	if gh == nil {
		gh, err = github.NewClient()
		if err != nil {
			return apiErrorf("building GitHub client: %v", err)
		}
	}

	if !p.watch {
		res := collect.Run(ctx, gh, tgt, checks, p.filters)
		if err := writeEnvelope(opts.Stdout, tgt, res, p.jsonOutput); err != nil {
			return apiErrorf("writing output: %v", err)
		}
		if len(res.Errors) > 0 {
			return &exitError{code: exitCodeForFailure(ctx)}
		}
		if p.exitStatus && res.HasWork() {
			*workFound = true
		}
		return nil
	}

	return watchLoop(ctx, opts, gh, tgt, checks, p, workFound)
}

func resolveTarget(args []string) (target.Target, error) {
	if len(args) == 0 {
		return target.FromCurrentRepo()
	}
	return target.Parse(args[0])
}

// prTargetVisitor reports whether a Target is a PR, via target.Visitor
// rather than a type assertion, so the check compiles against every Target
// variant.
type prTargetVisitor struct{ isPR bool }

func (*prTargetVisitor) VisitOrg(target.Org) error   { return nil }
func (*prTargetVisitor) VisitRepo(target.Repo) error { return nil }
func (v *prTargetVisitor) VisitPR(target.PR) error   { v.isPR = true; return nil }

func isPRTarget(t target.Target) bool {
	var v prTargetVisitor
	_ = t.Accept(&v) // prTargetVisitor never returns an error
	return v.isPR
}

func watchLoop(ctx context.Context, opts Options, gh github.Client, tgt target.Target, checks check.Set, p runParams, workFound *bool) error {
	watchCtx := ctx
	if p.timeout > 0 {
		var cancel context.CancelFunc
		watchCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	sleep := opts.Sleep
	if sleep == nil {
		sleep = realSleep
	}
	progress := term.IsTerminal(os.Stderr)

	for {
		res := collect.Run(watchCtx, gh, tgt, checks, p.filters)
		if len(res.Errors) > 0 {
			if writeErr := writeEnvelope(opts.Stdout, tgt, res, p.jsonOutput); writeErr != nil {
				return apiErrorf("writing output: %v", writeErr)
			}
			return &exitError{code: exitCodeForFailure(watchCtx)}
		}
		if res.HasWork() {
			if err := writeEnvelope(opts.Stdout, tgt, res, p.jsonOutput); err != nil {
				return apiErrorf("writing output: %v", err)
			}
			if p.exitStatus {
				*workFound = true
			}
			return nil
		}

		if progress {
			fmt.Fprintf(opts.Stderr, "gh work: no work yet, next check in %s\n", p.interval)
		}

		if err := sleep(watchCtx, p.interval); err != nil {
			if p.timeout > 0 && watchCtx.Err() == context.DeadlineExceeded {
				return writeEmptyOrFail(opts.Stdout, tgt, p.jsonOutput)
			}
			return &exitError{code: 130}
		}
	}
}

func writeEmptyOrFail(w io.Writer, tgt target.Target, jsonOutput bool) error {
	if err := writeEnvelope(w, tgt, collect.Result{}, jsonOutput); err != nil {
		return apiErrorf("writing output: %v", err)
	}
	return nil
}

func writeEnvelope(w io.Writer, tgt target.Target, res collect.Result, jsonOutput bool) error {
	env := render.NewEnvelope(tgt, res.Groups, res.Warnings, renderErrors(res.Errors))
	if jsonOutput {
		return render.JSON(w, env)
	}
	useColor := w == io.Writer(os.Stdout) && term.FromEnv().IsColorEnabled()
	return render.Text(w, env, useColor)
}

func renderErrors(errs []collect.SourceError) []render.SourceError {
	if errs == nil {
		return nil
	}
	out := make([]render.SourceError, len(errs))
	for i, e := range errs {
		out[i] = render.SourceError{Source: e.Source, Message: e.Message}
	}
	return out
}

func realSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
