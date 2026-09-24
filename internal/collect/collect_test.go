package collect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DarkWanderer/gh-work/internal/collect"
	"github.com/DarkWanderer/gh-work/internal/github"
	"github.com/DarkWanderer/gh-work/internal/source"
	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

type fakeSource struct {
	name     string
	supports func(target.Target) bool
	collect  func(ctx context.Context) (source.Result, error)
}

func (f fakeSource) Name() string                  { return f.name }
func (f fakeSource) Supports(t target.Target) bool { return f.supports(t) }
func (f fakeSource) Collect(ctx context.Context, _ github.Client, _ target.Target, _ source.Options) (source.Result, error) {
	return f.collect(ctx)
}

func alwaysTrue(target.Target) bool { return true }

func TestRunCollectsConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	mk := func(name string) fakeSource {
		return fakeSource{name: name, supports: alwaysTrue, collect: func(ctx context.Context) (source.Result, error) {
			started <- struct{}{}
			select {
			case <-release:
			case <-time.After(2 * time.Second):
				t.Errorf("%s: timed out waiting for concurrent sibling (sources are not running in parallel)", name)
			}
			return source.Result{}, nil
		}}
	}
	srcs := []source.Source{mk("a"), mk("b")}

	go func() {
		<-started
		<-started
		close(release)
	}()

	done := make(chan collect.Result, 1)
	go func() {
		done <- collect.Run(context.Background(), nil, target.Org{Owner: "o"}, srcs, source.Options{})
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not complete; sources likely ran sequentially and deadlocked on the release barrier")
	}
}

func TestRunOnlyInvokesSupportedSources(t *testing.T) {
	called := false
	unsupported := fakeSource{
		name:     "unsupported",
		supports: func(target.Target) bool { return false },
		collect: func(ctx context.Context) (source.Result, error) {
			called = true
			return source.Result{}, nil
		},
	}
	res := collect.Run(context.Background(), nil, target.PR{Owner: "o", Name: "r", Number: 1}, []source.Source{unsupported}, source.Options{})
	if called {
		t.Fatal("Collect was called on an unsupported source")
	}
	if len(res.Items) != 0 || len(res.Errors) != 0 {
		t.Fatalf("res = %#v, want empty", res)
	}
}

func TestRunPartialFailureKeepsOtherSourcesItems(t *testing.T) {
	failing := fakeSource{name: "failing", supports: alwaysTrue, collect: func(ctx context.Context) (source.Result, error) {
		return source.Result{}, errors.New("rate limited")
	}}
	ok := fakeSource{name: "ok", supports: alwaysTrue, collect: func(ctx context.Context) (source.Result, error) {
		return source.Result{Items: []work.Item{work.BranchCheckFailure{ID: "SC_1", Repo: "o/r", Branch: "main"}}}, nil
	}}

	res := collect.Run(context.Background(), nil, target.Org{Owner: "o"}, []source.Source{failing, ok}, source.Options{})

	if len(res.Items) != 1 {
		t.Fatalf("items = %#v, want 1", res.Items)
	}
	if len(res.Errors) != 1 || res.Errors[0].Source != "failing" || res.Errors[0].Message != "rate limited" {
		t.Fatalf("errors = %#v", res.Errors)
	}
}

func TestRunMergesWarnings(t *testing.T) {
	a := fakeSource{name: "a", supports: alwaysTrue, collect: func(ctx context.Context) (source.Result, error) {
		return source.Result{Warnings: []string{"a-warning"}}, nil
	}}
	b := fakeSource{name: "b", supports: alwaysTrue, collect: func(ctx context.Context) (source.Result, error) {
		return source.Result{Warnings: []string{"b-warning"}}, nil
	}}
	res := collect.Run(context.Background(), nil, target.Org{Owner: "o"}, []source.Source{a, b}, source.Options{})
	if len(res.Warnings) != 2 || res.Warnings[0] != "a-warning" || res.Warnings[1] != "b-warning" {
		t.Fatalf("warnings = %v, want [a-warning b-warning] in registration order", res.Warnings)
	}
}

func TestRunSortsItemsDeterministically(t *testing.T) {
	pr1 := work.PRRef{Number: 1}
	pr2 := work.PRRef{Number: 2}
	pr10 := work.PRRef{Number: 10}
	items := []work.Item{
		work.BranchCheckFailure{ID: "z1", Repo: "o/r", Branch: "main"},
		work.PRCheckFailure{ID: "c2", Repo: "o/r", PR: pr2},
		work.ReviewThread{ID: "t1a", Repo: "o/r", PR: pr1},
		work.PRCheckFailure{ID: "c1", Repo: "o/r", PR: pr1},
		work.MergeConflict{ID: "m1", Repo: "o/r", PR: pr1},
		work.ReviewThread{ID: "t10", Repo: "o/r", PR: pr10},
		work.BranchCheckFailure{ID: "a1", Repo: "o/r", Branch: "develop"},
		work.ReviewThread{ID: "t1z", Repo: "a/first", PR: pr1},
	}
	src := fakeSource{name: "s", supports: alwaysTrue, collect: func(ctx context.Context) (source.Result, error) {
		return source.Result{Items: items}, nil
	}}
	res := collect.Run(context.Background(), nil, target.Org{Owner: "o"}, []source.Source{src}, source.Options{})

	want := []string{"t1z", "m1", "c1", "t1a", "c2", "t10", "a1", "z1"}
	got := make([]string, len(res.Items))
	for i, it := range res.Items {
		got[i] = it.ItemID()
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestHasWork(t *testing.T) {
	if (collect.Result{}).HasWork() {
		t.Fatal("empty result should not have work")
	}
	if !(collect.Result{Items: []work.Item{work.BranchCheckFailure{ID: "a", Repo: "o/r"}}}).HasWork() {
		t.Fatal("non-empty result should have work")
	}
}
