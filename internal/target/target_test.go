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

func TestTargetKind(t *testing.T) {
	if (Org{Owner: "o"}).Kind() != KindOrg {
		t.Fatal("Org.Kind() != KindOrg")
	}
	if (Repo{Owner: "o", Name: "r"}).Kind() != KindRepo {
		t.Fatal("Repo.Kind() != KindRepo")
	}
	if (PR{Owner: "o", Name: "r", Number: 1}).Kind() != KindPR {
		t.Fatal("PR.Kind() != KindPR")
	}
}

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
