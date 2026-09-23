// Package target models the thing `gh work` was asked to inspect: an org
// (or user), a single repo, or a single PR. Target is a sealed interface so
// that callers who type-switch on it cannot forget a variant, and so a PR
// target always carries a number while an org target never does.
package target

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
)

// Kind identifies which concrete Target variant a value holds.
type Kind int

const (
	KindOrg Kind = iota
	KindRepo
	KindPR
)

func (k Kind) String() string {
	switch k {
	case KindOrg:
		return "org"
	case KindRepo:
		return "repo"
	case KindPR:
		return "pr"
	default:
		return "unknown"
	}
}

// Target is implemented by Org, Repo and PR. The unexported method seals the
// interface: only this package can produce new variants.
type Target interface {
	Kind() Kind
	sealed()
}

// Org is an organization or user login, e.g. "voron-software".
type Org struct {
	Owner string
}

func (Org) Kind() Kind { return KindOrg }
func (Org) sealed()    {}

// Repo is a single owner/name repository.
type Repo struct {
	Owner string
	Name  string
}

func (Repo) Kind() Kind { return KindRepo }
func (Repo) sealed()    {}

// String returns the "owner/name" form.
func (r Repo) String() string { return r.Owner + "/" + r.Name }

// PR is a single pull request within a repository.
type PR struct {
	Owner  string
	Name   string
	Number int
}

func (PR) Kind() Kind { return KindPR }
func (PR) sealed()    {}

// RepoString returns the "owner/name" form of the PR's repository.
func (p PR) RepoString() string { return p.Owner + "/" + p.Name }

// ErrInvalid is wrapped by every error Parse returns.
var ErrInvalid = errors.New("invalid target")

var (
	ownerRe = `[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?`
	repoRe  = `[A-Za-z0-9._-]+`

	reOrg  = regexp.MustCompile(`^` + ownerRe + `$`)
	reRepo = regexp.MustCompile(`^(` + ownerRe + `)/(` + repoRe + `)$`)
	rePR   = regexp.MustCompile(`^(` + ownerRe + `)/(` + repoRe + `)#([0-9]+)$`)
	reURL  = regexp.MustCompile(`^(?:https?://)?(?:www\.)?github\.com/(` + ownerRe + `)/(` + repoRe + `)(?:/pull/([0-9]+))?/?$`)
)

// Parse interprets s as one of: "owner", "owner/repo", "owner/repo#N", or a
// github.com URL to a repo or PR ("github.com/owner/repo[/pull/N]", with or
// without scheme). Anything else is a usage error wrapping ErrInvalid.
func Parse(s string) (Target, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: empty target", ErrInvalid)
	}

	if strings.Contains(s, "github.com/") {
		if m := reURL.FindStringSubmatch(s); m != nil {
			owner, repo, num := m[1], m[2], m[3]
			if num != "" {
				n, err := strconv.Atoi(num)
				if err != nil {
					return nil, fmt.Errorf("%w: %q: bad PR number", ErrInvalid, s)
				}
				return PR{Owner: owner, Name: repo, Number: n}, nil
			}
			return Repo{Owner: owner, Name: repo}, nil
		}
		return nil, fmt.Errorf("%w: %q: not a recognized github.com URL", ErrInvalid, s)
	}

	if strings.Contains(s, "#") {
		m := rePR.FindStringSubmatch(s)
		if m == nil {
			return nil, fmt.Errorf("%w: %q: expected owner/repo#N", ErrInvalid, s)
		}
		n, err := strconv.Atoi(m[3])
		if err != nil {
			return nil, fmt.Errorf("%w: %q: bad PR number", ErrInvalid, s)
		}
		return PR{Owner: m[1], Name: m[2], Number: n}, nil
	}

	if strings.Contains(s, "/") {
		m := reRepo.FindStringSubmatch(s)
		if m == nil {
			return nil, fmt.Errorf("%w: %q: expected owner/repo", ErrInvalid, s)
		}
		return Repo{Owner: m[1], Name: m[2]}, nil
	}

	if !reOrg.MatchString(s) {
		return nil, fmt.Errorf("%w: %q: expected an owner login", ErrInvalid, s)
	}
	return Org{Owner: s}, nil
}

// FromCurrentRepo resolves the target from the repo in the current working
// directory, as gh itself would (GH_REPO override, git remotes, etc).
func FromCurrentRepo() (Target, error) {
	r, err := repository.Current()
	if err != nil {
		return nil, fmt.Errorf("could not determine current repository: %w", err)
	}
	return Repo{Owner: r.Owner, Name: r.Name}, nil
}
