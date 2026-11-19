package check

import (
	"testing"

	"github.com/DarkWanderer/gh-work/internal/scan"
	"github.com/DarkWanderer/gh-work/internal/work"
)

func TestPRAnnotationsNameAndNeeds(t *testing.T) {
	c := PRAnnotations{}
	if c.Name() != "pr-annotations" {
		t.Fatalf("Name() = %q", c.Name())
	}
	if c.Needs() != scan.HeadAnnotations {
		t.Fatalf("Needs() = %v", c.Needs())
	}
}

func TestPRAnnotationsInspect(t *testing.T) {
	lint := work.Check{Name: "lint", Conclusion: "SUCCESS", URL: "https://ci/lint"}
	build := work.Check{Name: "build", Conclusion: "FAILURE", URL: "https://ci/build"}
	failure := scan.Annotation{DatabaseID: 7, Level: "FAILURE", Path: "a.go", StartLine: 3, EndLine: 5, Title: "vet", Message: "bad", URL: "https://blob/a.go"}

	cases := []struct {
		name    string
		pr      scan.PullRequest
		wantIDs []string
	}{
		{"nil head", scan.PullRequest{}, nil},
		{
			"only FAILURE level counts",
			headWith(fakeContext{id: "CR_1", check: lint, annotations: []scan.Annotation{
				failure,
				{DatabaseID: 8, Level: "WARNING"},
				{DatabaseID: 9, Level: "NOTICE"},
			}}),
			[]string{"CR_1:7"},
		},
		{
			"reported regardless of the check's conclusion",
			headWith(
				fakeContext{id: "CR_1", check: lint, annotations: []scan.Annotation{failure}},
				fakeContext{id: "CR_2", failing: true, check: build, annotations: []scan.Annotation{{DatabaseID: 1, Level: "FAILURE"}}},
			),
			[]string{"CR_1:7", "CR_2:1"},
		},
		{
			"missing databaseId falls back to position",
			headWith(fakeContext{id: "CR_1", check: lint, annotations: []scan.Annotation{
				{Level: "WARNING"},
				{Level: "FAILURE"},
			}}),
			[]string{"CR_1:#1"},
		},
		{
			"contexts without annotations contribute nothing",
			headWith(fakeContext{id: "SC_1", failing: true, check: build}),
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items := PRAnnotations{}.Inspect(c.pr)
			if len(items) != len(c.wantIDs) {
				t.Fatalf("got %d items, want %d: %#v", len(items), len(c.wantIDs), items)
			}
			for i, it := range items {
				if it.ItemID() != c.wantIDs[i] || it.ItemKind() != work.KindPRAnnotation {
					t.Errorf("item %d = %#v, want id %q", i, it, c.wantIDs[i])
				}
			}
		})
	}
}

func TestPRAnnotationsCarriesCheckCommitAndAnnotation(t *testing.T) {
	check := work.Check{Name: "lint", Conclusion: "SUCCESS", URL: "https://ci/lint", Workflow: "CI", RunID: 1, JobID: 2}
	a := scan.Annotation{DatabaseID: 7, Level: "FAILURE", Path: "a.go", StartLine: 3, EndLine: 5, Title: "vet", Message: "bad", URL: "https://blob/a.go"}
	items := PRAnnotations{}.Inspect(headWith(fakeContext{id: "CR_1", check: check, annotations: []scan.Annotation{a}}))
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	want := work.PRAnnotation{
		ID: "CR_1:7", Commit: "abc1234", Check: check,
		Annotation: work.Annotation{Level: "FAILURE", Path: "a.go", StartLine: 3, EndLine: 5, Title: "vet", Message: "bad", URL: "https://blob/a.go"},
	}
	if items[0] != want {
		t.Fatalf("item = %#v, want %#v", items[0], want)
	}
}

func headWith(contexts ...scan.CheckContext) scan.PullRequest {
	return scan.PullRequest{Head: &scan.Commit{Oid: "abc1234", Contexts: contexts}}
}
