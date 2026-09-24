package work

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReviewThreadMarshalJSON(t *testing.T) {
	item := ReviewThread{
		ID:   "PRRT_1",
		Repo: "o/r",
		PR:   PRRef{Number: 1, Title: "t", URL: "https://x/1", HeadRef: "feat", IsDraft: false},
		Thread: Thread{
			Path: "a.go", Line: 14, IsOutdated: false, URL: "https://x/1#r1",
			Comments: []Comment{{Author: "bob", Body: "hi", URL: "https://x/1#c1", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}},
		},
	}

	b, err := json.Marshal(Item(item))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "review-thread" {
		t.Fatalf("kind = %v", got["kind"])
	}
	if got["id"] != "PRRT_1" || got["repo"] != "o/r" {
		t.Fatalf("id/repo = %v/%v", got["id"], got["repo"])
	}
	thread := got["thread"].(map[string]any)
	if thread["path"] != "a.go" || thread["line"] != float64(14) {
		t.Fatalf("thread = %v", thread)
	}
}

func TestPRCheckFailureMarshalJSON(t *testing.T) {
	item := PRCheckFailure{
		ID: "CR_1", Repo: "o/r",
		PR:     PRRef{Number: 2, Title: "t", URL: "https://x/2", HeadRef: "feat", IsDraft: true},
		Commit: "deadbeef",
		Check:  Check{Name: "build", Conclusion: "FAILURE", URL: "https://x/checks/1", Workflow: "CI", RunID: 100, JobID: 200},
	}
	b, err := json.Marshal(Item(item))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "pr-check-failure" {
		t.Fatalf("kind = %v", got["kind"])
	}
	check := got["check"].(map[string]any)
	if check["runId"] != float64(100) || check["jobId"] != float64(200) {
		t.Fatalf("check = %v", check)
	}
}

func TestBranchCheckFailureMarshalJSON(t *testing.T) {
	item := BranchCheckFailure{
		ID: "SC_1", Repo: "o/r", Branch: "main", Commit: "cafebabe",
		Check: Check{Name: "ci/status", Conclusion: "ERROR", URL: "https://x/status/1"},
	}
	b, err := json.Marshal(Item(item))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "branch-check-failure" || got["branch"] != "main" {
		t.Fatalf("got = %v", got)
	}
	check := got["check"].(map[string]any)
	if _, ok := check["runId"]; ok {
		t.Fatalf("runId should be omitted for non-Actions checks, got %v", check)
	}
}

func TestMergeConflictMarshalJSON(t *testing.T) {
	item := MergeConflict{
		ID:      "PR_kwDOA1",
		Repo:    "o/r",
		PR:      PRRef{Number: 3, Title: "t", URL: "https://x/3", HeadRef: "feat", IsDraft: false},
		BaseRef: "main",
	}
	b, err := json.Marshal(Item(item))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "merge-conflict" {
		t.Fatalf("kind = %v", got["kind"])
	}
	if got["id"] != "PR_kwDOA1" || got["repo"] != "o/r" || got["baseRef"] != "main" {
		t.Fatalf("got = %v", got)
	}
	pr := got["pr"].(map[string]any)
	if pr["number"] != float64(3) {
		t.Fatalf("pr = %v", pr)
	}
}

func TestItemAccessors(t *testing.T) {
	items := []Item{
		ReviewThread{ID: "a", Repo: "o/r"},
		PRCheckFailure{ID: "b", Repo: "o/r"},
		BranchCheckFailure{ID: "c", Repo: "o/r"},
		MergeConflict{ID: "d", Repo: "o/r"},
	}
	wantKinds := []Kind{KindReviewThread, KindPRCheckFailure, KindBranchCheckFailure, KindMergeConflict}
	for i, it := range items {
		if it.ItemKind() != wantKinds[i] {
			t.Errorf("item %d: kind = %v, want %v", i, it.ItemKind(), wantKinds[i])
		}
		if it.ItemRepo() != "o/r" {
			t.Errorf("item %d: repo = %v", i, it.ItemRepo())
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
