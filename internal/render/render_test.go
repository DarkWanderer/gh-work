package render

import (
	"bytes"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/DarkWanderer/gh-work/internal/target"
	"github.com/DarkWanderer/gh-work/internal/work"
)

var update = flag.Bool("update", false, "update golden files")

// sampleGroups reproduces, via the grouped work.Group model, the exact item
// sequence the old flat-item golden fixtures were built from: PRRT_1,
// PRRT_2, CR_1, PR_kwDOA1 on PR #1, then SC_1 on branch main. Keeping the
// same golden files unchanged proves the --json v1 contract didn't move.
func sampleGroups() []work.Group {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return []work.Group{
		work.PullRequest{
			ID: "PR_1", Repo: "o/r", Number: 1, Title: "Fix sync bug",
			URL: "https://github.com/o/r/pull/1", HeadRef: "fix-sync", IsDraft: false,
			Items: []work.PRItem{
				work.ReviewThread{
					ID: "PRRT_1",
					Thread: work.Thread{
						Path: "main.go", Line: 14, IsOutdated: false, URL: "https://github.com/o/r/pull/1#discussion_r1",
						Comments: []work.Comment{{Author: "codex", Body: "This looks wrong.\nSee line above.", URL: "https://github.com/o/r/pull/1#r1", CreatedAt: created}},
					},
				},
				work.ReviewThread{
					ID: "PRRT_2",
					Thread: work.Thread{
						Path: "main.go", Line: 40, IsOutdated: true, URL: "https://github.com/o/r/pull/1#discussion_r2",
						Comments: []work.Comment{{Author: "alice", Body: "Still applies?", URL: "https://github.com/o/r/pull/1#r2", CreatedAt: created}},
					},
				},
				work.PRCheckFailure{CheckFailure: work.CheckFailure{
					ID: "CR_1", Commit: "abc1234",
					Check: work.Check{Name: "sync", Conclusion: "FAILURE", URL: "https://github.com/o/r/pull/1/checks/1", Workflow: "CI", RunID: 100, JobID: 200},
				}},
				work.MergeConflict{ID: "PR_kwDOA1", BaseRef: "main"},
			},
		},
		work.Branch{
			Repo: "o/r", Name: "main", Commit: "def5678",
			Failures: []work.CheckFailure{
				{ID: "SC_1", Commit: "def5678", Check: work.Check{Name: "ci/status", Conclusion: "ERROR", URL: "https://github.com/o/r/commit/def5678"}},
			},
		},
	}
}

func sampleEnvelope() Envelope {
	return NewEnvelope(target.Repo{Owner: "o", Name: "r"}, sampleGroups(),
		[]string{"search result count exceeds 1000; some PRs may be missing"},
		[]SourceError{{Source: "branch-checks", Message: "rate limited"}})
}

func goldenCompare(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file %s: %v (run with -update to create it)", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

func TestJSONGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleEnvelope()); err != nil {
		t.Fatal(err)
	}
	goldenCompare(t, "testdata/envelope.json.golden", buf.Bytes())
}

func TestTextGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, sampleEnvelope(), false); err != nil {
		t.Fatal(err)
	}
	goldenCompare(t, "testdata/envelope.txt.golden", buf.Bytes())
}

func TestTextGoldenColor(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, sampleEnvelope(), true); err != nil {
		t.Fatal(err)
	}
	goldenCompare(t, "testdata/envelope.color.txt.golden", buf.Bytes())
}

func TestTextEmpty(t *testing.T) {
	var buf bytes.Buffer
	e := NewEnvelope(target.Repo{Owner: "o", Name: "r"}, nil, nil, nil)
	if err := Text(&buf, e, false); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "No work found.\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestTextGroupsRepoHeaderOnlyOncePerRepo(t *testing.T) {
	groups := []work.Group{
		work.Branch{Repo: "o/r", Name: "develop", Failures: []work.CheckFailure{{ID: "a", Check: work.Check{Name: "x", Conclusion: "ERROR"}}}},
		work.Branch{Repo: "o/r", Name: "main", Failures: []work.CheckFailure{{ID: "b", Check: work.Check{Name: "x", Conclusion: "ERROR"}}}},
		work.Branch{Repo: "o/other", Name: "main", Failures: []work.CheckFailure{{ID: "c", Check: work.Check{Name: "x", Conclusion: "ERROR"}}}},
	}
	e := NewEnvelope(target.Org{Owner: "o"}, groups, nil, nil)
	var buf bytes.Buffer
	if err := Text(&buf, e, false); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if n := bytesCount(got, "o/r\n"); n != 1 {
		t.Fatalf("repo header \"o/r\" printed %d times, want 1:\n%s", n, got)
	}
	if n := bytesCount(got, "o/other\n"); n != 1 {
		t.Fatalf("repo header \"o/other\" printed %d times, want 1:\n%s", n, got)
	}
}

func bytesCount(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

func TestEnvelopeEmptySlicesMarshalAsEmptyArrays(t *testing.T) {
	var buf bytes.Buffer
	e := NewEnvelope(target.Org{Owner: "o"}, nil, nil, nil)
	if err := JSON(&buf, e); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, want := range []string{`"items": []`, `"warnings": []`, `"errors": []`} {
		if !bytes.Contains([]byte(s), []byte(want)) {
			t.Fatalf("expected %q in output, got:\n%s", want, s)
		}
	}
}

func TestTargetInfoFrom(t *testing.T) {
	cases := []struct {
		in   target.Target
		want TargetInfo
	}{
		{target.Org{Owner: "o"}, TargetInfo{Kind: "org", Owner: "o"}},
		{target.Repo{Owner: "o", Name: "r"}, TargetInfo{Kind: "repo", Owner: "o", Repo: "r"}},
		{target.PR{Owner: "o", Name: "r", Number: 5}, TargetInfo{Kind: "pr", Owner: "o", Repo: "r", Number: 5}},
	}
	for _, c := range cases {
		got := TargetInfoFrom(c.in)
		if got != c.want {
			t.Errorf("TargetInfoFrom(%#v) = %#v, want %#v", c.in, got, c.want)
		}
	}
}
