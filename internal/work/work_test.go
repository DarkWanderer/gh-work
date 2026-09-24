package work

import (
	"reflect"
	"testing"
)

// recordingGroupVisitor records which Visit* method was called and with
// what value, so Accept's dispatch can be asserted without a type switch.
type recordingGroupVisitor struct {
	called string
	pr     PullRequest
	branch Branch
}

func (r *recordingGroupVisitor) VisitPullRequest(p PullRequest) { r.called, r.pr = "pr", p }
func (r *recordingGroupVisitor) VisitBranch(b Branch)           { r.called, r.branch = "branch", b }

func TestPullRequestAcceptDispatchesToVisitPullRequest(t *testing.T) {
	pr := PullRequest{ID: "PR_1", Repo: "o/r", Number: 1}
	var rv recordingGroupVisitor
	pr.Accept(&rv)
	if rv.called != "pr" || !reflect.DeepEqual(rv.pr, pr) {
		t.Fatalf("Accept dispatched to %q with %#v", rv.called, rv.pr)
	}
}

func TestBranchAcceptDispatchesToVisitBranch(t *testing.T) {
	b := Branch{Repo: "o/r", Name: "main"}
	var rv recordingGroupVisitor
	b.Accept(&rv)
	if rv.called != "branch" || !reflect.DeepEqual(rv.branch, b) {
		t.Fatalf("Accept dispatched to %q with %#v", rv.called, rv.branch)
	}
}

func TestGroupSortKey(t *testing.T) {
	cases := []struct {
		name string
		g    Group
		want string
	}{
		{"pr 1", PullRequest{Number: 1}, "0" + "0000000001"},
		{"pr 10", PullRequest{Number: 10}, "0" + "0000000010"},
		{"branch main", Branch{Name: "main"}, "1main"},
		{"branch develop", Branch{Name: "develop"}, "1develop"},
	}
	for _, c := range cases {
		if got := c.g.SortKey(); got != c.want {
			t.Errorf("%s: SortKey() = %q, want %q", c.name, got, c.want)
		}
	}
	// A PR-numbered group must sort before any branch group regardless of
	// number/name, and PR numbers must compare numerically, not lexically.
	if !(PullRequest{Number: 2}.SortKey() < Branch{Name: "aaa"}.SortKey()) {
		t.Fatal("PR groups should sort before branch groups")
	}
	if !(PullRequest{Number: 2}.SortKey() < PullRequest{Number: 10}.SortKey()) {
		t.Fatal("PR #2 should sort before PR #10")
	}
}

func TestGroupRepo(t *testing.T) {
	if (PullRequest{Repo: "o/r"}).GroupRepo() != "o/r" {
		t.Fatal("PullRequest.GroupRepo()")
	}
	if (Branch{Repo: "o/r"}).GroupRepo() != "o/r" {
		t.Fatal("Branch.GroupRepo()")
	}
}

// recordingPRItemVisitor records which Visit* method was called and with
// what value.
type recordingPRItemVisitor struct {
	called   string
	thread   ReviewThread
	check    PRCheckFailure
	conflict MergeConflict
}

func (r *recordingPRItemVisitor) VisitReviewThread(t ReviewThread) { r.called, r.thread = "thread", t }
func (r *recordingPRItemVisitor) VisitPRCheckFailure(c PRCheckFailure) {
	r.called, r.check = "check", c
}
func (r *recordingPRItemVisitor) VisitMergeConflict(m MergeConflict) {
	r.called, r.conflict = "conflict", m
}

func TestPRItemAcceptDispatchesToMatchingVisitorMethod(t *testing.T) {
	thread := ReviewThread{ID: "PRRT_1", Thread: Thread{Path: "a.go"}}
	var rv recordingPRItemVisitor
	thread.Accept(&rv)
	if rv.called != "thread" || !reflect.DeepEqual(rv.thread, thread) {
		t.Fatalf("ReviewThread.Accept dispatched to %q", rv.called)
	}

	check := PRCheckFailure{CheckFailure{ID: "CR_1", Commit: "abc", Check: Check{Name: "build"}}}
	rv = recordingPRItemVisitor{}
	check.Accept(&rv)
	if rv.called != "check" || rv.check != check {
		t.Fatalf("PRCheckFailure.Accept dispatched to %q", rv.called)
	}

	conflict := MergeConflict{ID: "PR_1", BaseRef: "main"}
	rv = recordingPRItemVisitor{}
	conflict.Accept(&rv)
	if rv.called != "conflict" || rv.conflict != conflict {
		t.Fatalf("MergeConflict.Accept dispatched to %q", rv.called)
	}
}

func TestPRItemAccessors(t *testing.T) {
	items := []PRItem{
		ReviewThread{ID: "a"},
		PRCheckFailure{CheckFailure{ID: "b"}},
		MergeConflict{ID: "c"},
	}
	wantKinds := []Kind{KindReviewThread, KindPRCheckFailure, KindMergeConflict}
	wantIDs := []string{"a", "b", "c"}
	for i, it := range items {
		if it.ItemKind() != wantKinds[i] {
			t.Errorf("item %d: kind = %v, want %v", i, it.ItemKind(), wantKinds[i])
		}
		if it.ItemID() != wantIDs[i] {
			t.Errorf("item %d: id = %v, want %v", i, it.ItemID(), wantIDs[i])
		}
	}
}

func TestIsFailingCheckRunConclusion(t *testing.T) {
	failing := []string{"FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED"}
	for _, c := range failing {
		if !IsFailingCheckRunConclusion(c) {
			t.Errorf("%s should be failing", c)
		}
	}
	passing := []string{"SUCCESS", "NEUTRAL", "SKIPPED", "CANCELLED", "STALE", ""}
	for _, c := range passing {
		if IsFailingCheckRunConclusion(c) {
			t.Errorf("%s should not be failing", c)
		}
	}
}

func TestIsFailingStatusState(t *testing.T) {
	if !IsFailingStatusState("FAILURE") || !IsFailingStatusState("ERROR") {
		t.Fatal("FAILURE and ERROR should be failing")
	}
	for _, s := range []string{"SUCCESS", "PENDING", "EXPECTED"} {
		if IsFailingStatusState(s) {
			t.Errorf("%s should not be failing", s)
		}
	}
}
