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

func sampleItems() []work.Item {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pr1 := work.PRRef{Number: 1, Title: "Fix sync bug", URL: "https://github.com/o/r/pull/1", HeadRef: "fix-sync", IsDraft: false}
	return []work.Item{
		work.ReviewThread{
			ID: "PRRT_1", Repo: "o/r", PR: pr1,
			Thread: work.Thread{
				Path: "main.go", Line: 14, IsOutdated: false, URL: "https://github.com/o/r/pull/1#discussion_r1",
				Comments: []work.Comment{{Author: "codex", Body: "This looks wrong.\nSee line above.", URL: "https://github.com/o/r/pull/1#r1", CreatedAt: created}},
			},
		},
		work.ReviewThread{
			ID: "PRRT_2", Repo: "o/r", PR: pr1,
			Thread: work.Thread{
				Path: "main.go", Line: 40, IsOutdated: true, URL: "https://github.com/o/r/pull/1#discussion_r2",
				Comments: []work.Comment{{Author: "alice", Body: "Still applies?", URL: "https://github.com/o/r/pull/1#r2", CreatedAt: created}},
			},
		},
		work.PRCheckFailure{
			ID: "CR_1", Repo: "o/r", PR: pr1, Commit: "abc1234",
			Check: work.Check{Name: "sync", Conclusion: "FAILURE", URL: "https://github.com/o/r/pull/1/checks/1", Workflow: "CI", RunID: 100, JobID: 200},
		},
		work.MergeConflict{
			ID: "PR_kwDOA1", Repo: "o/r", PR: pr1, BaseRef: "main",
		},
		work.BranchCheckFailure{
			ID: "SC_1", Repo: "o/r", Branch: "main", Commit: "def5678",
			Check: work.Check{Name: "ci/status", Conclusion: "ERROR", URL: "https://github.com/o/r/commit/def5678"},
		},
	}
}

func sampleEnvelope() Envelope {
	return NewEnvelope(target.Repo{Owner: "o", Name: "r"}, sampleItems(),
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
		in   interface{ Kind() target.Kind }
		want TargetInfo
	}{
		{target.Org{Owner: "o"}, TargetInfo{Kind: "org", Owner: "o"}},
		{target.Repo{Owner: "o", Name: "r"}, TargetInfo{Kind: "repo", Owner: "o", Repo: "r"}},
		{target.PR{Owner: "o", Name: "r", Number: 5}, TargetInfo{Kind: "pr", Owner: "o", Repo: "r", Number: 5}},
	}
	for _, c := range cases {
		got := TargetInfoFrom(c.in.(target.Target))
		if got != c.want {
			t.Errorf("TargetInfoFrom(%#v) = %#v, want %#v", c.in, got, c.want)
		}
	}
}
