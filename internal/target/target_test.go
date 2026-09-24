package target

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Target
		wantErr bool
	}{
		{
			name:  "org",
			input: "voron-software",
			want:  Org{Owner: "voron-software"},
		},
		{
			name:  "repo",
			input: "voron-software/grafana-iac",
			want:  Repo{Owner: "voron-software", Name: "grafana-iac"},
		},
		{
			name:  "pr shorthand",
			input: "voron-software/grafana-iac#1",
			want:  PR{Owner: "voron-software", Name: "grafana-iac", Number: 1},
		},
		{
			name:  "repo url https",
			input: "https://github.com/voron-software/grafana-iac",
			want:  Repo{Owner: "voron-software", Name: "grafana-iac"},
		},
		{
			name:  "repo url no scheme",
			input: "github.com/voron-software/grafana-iac",
			want:  Repo{Owner: "voron-software", Name: "grafana-iac"},
		},
		{
			name:  "repo url trailing slash",
			input: "https://github.com/voron-software/grafana-iac/",
			want:  Repo{Owner: "voron-software", Name: "grafana-iac"},
		},
		{
			name:  "pr url",
			input: "https://github.com/voron-software/grafana-iac/pull/1",
			want:  PR{Owner: "voron-software", Name: "grafana-iac", Number: 1},
		},
		{
			name:  "pr url trailing slash",
			input: "https://github.com/voron-software/grafana-iac/pull/42/",
			want:  PR{Owner: "voron-software", Name: "grafana-iac", Number: 42},
		},
		{
			name:  "pr url no scheme",
			input: "github.com/voron-software/grafana-iac/pull/7",
			want:  PR{Owner: "voron-software", Name: "grafana-iac", Number: 7},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "too many segments",
			input:   "a/b/c",
			wantErr: true,
		},
		{
			name:    "invalid pr number",
			input:   "owner/repo#abc",
			wantErr: true,
		},
		{
			name:    "non-github host",
			input:   "https://example.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "double slash",
			input:   "owner//repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = %#v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

// recordingVisitor records which Visit* method was called and on what value.
type recordingVisitor struct {
	called string
	value  Target
}

func (r *recordingVisitor) VisitOrg(o Org) error   { r.called, r.value = "org", o; return nil }
func (r *recordingVisitor) VisitRepo(o Repo) error { r.called, r.value = "repo", o; return nil }
func (r *recordingVisitor) VisitPR(o PR) error     { r.called, r.value = "pr", o; return nil }

func TestTargetAcceptDispatchesToMatchingVisitorMethod(t *testing.T) {
	org := Org{Owner: "o"}
	var rv recordingVisitor
	if err := org.Accept(&rv); err != nil {
		t.Fatal(err)
	}
	if rv.called != "org" || rv.value != Target(org) {
		t.Fatalf("Org.Accept dispatched to %q with %#v", rv.called, rv.value)
	}

	repo := Repo{Owner: "o", Name: "r"}
	rv = recordingVisitor{}
	if err := repo.Accept(&rv); err != nil {
		t.Fatal(err)
	}
	if rv.called != "repo" || rv.value != Target(repo) {
		t.Fatalf("Repo.Accept dispatched to %q with %#v", rv.called, rv.value)
	}

	pr := PR{Owner: "o", Name: "r", Number: 1}
	rv = recordingVisitor{}
	if err := pr.Accept(&rv); err != nil {
		t.Fatal(err)
	}
	if rv.called != "pr" || rv.value != Target(pr) {
		t.Fatalf("PR.Accept dispatched to %q with %#v", rv.called, rv.value)
	}
}

func TestTargetAcceptPropagatesVisitorError(t *testing.T) {
	wantErr := errors.New("boom")
	v := erroringVisitor{err: wantErr}
	if err := (Org{Owner: "o"}).Accept(v); err != wantErr {
		t.Fatalf("Accept error = %v, want %v", err, wantErr)
	}
}

type erroringVisitor struct{ err error }

func (v erroringVisitor) VisitOrg(Org) error   { return v.err }
func (v erroringVisitor) VisitRepo(Repo) error { return v.err }
func (v erroringVisitor) VisitPR(PR) error     { return v.err }

func TestRepoString(t *testing.T) {
	if got := (Repo{Owner: "o", Name: "r"}).String(); got != "o/r" {
		t.Fatalf("Repo.String() = %q, want %q", got, "o/r")
	}
	if got := (PR{Owner: "o", Name: "r", Number: 5}).RepoString(); got != "o/r" {
		t.Fatalf("PR.RepoString() = %q, want %q", got, "o/r")
	}
}

func TestParseErrIsErrInvalid(t *testing.T) {
	_, err := Parse("")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected err to wrap ErrInvalid, got %v", err)
	}
}
