// Package fake is a test double for internal/github.Client. It dispatches
// each call to a fixture by GraphQL operation name plus the exact variables
// passed, so pagination and multi-query sources can be driven from static
// JSON files without a real GitHub host.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sync"
)

// Call records one Query invocation, for assertions in tests.
type Call struct {
	Operation string
	Variables map[string]any
}

// Client is a github.Client that serves canned responses.
type Client struct {
	mu       sync.Mutex
	fixtures map[string]json.RawMessage
	errors   map[string]error
	Calls    []Call
}

// NewClient returns an empty fake client; populate it with SetFixture(File)
// before use.
func NewClient() *Client {
	return &Client{fixtures: map[string]json.RawMessage{}, errors: map[string]error{}}
}

// SetFixture registers the raw JSON response for a given operation name and
// exact variable set.
func (c *Client) SetFixture(operation string, vars map[string]any, raw json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fixtures[Key(operation, vars)] = raw
}

// SetFixtureFile registers the contents of path as the response for a given
// operation name and exact variable set.
func (c *Client) SetFixtureFile(operation string, vars map[string]any, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c.SetFixture(operation, vars, raw)
	return nil
}

// SetFixtureError makes the given operation/variables pair fail with err
// instead of returning a fixture.
func (c *Client) SetFixtureError(operation string, vars map[string]any, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors[Key(operation, vars)] = err
}

// Query implements github.Client.
func (c *Client) Query(_ context.Context, query string, vars map[string]any, out any) error {
	op := operationName(query)
	key := Key(op, vars)

	c.mu.Lock()
	c.Calls = append(c.Calls, Call{Operation: op, Variables: vars})
	err, hasErr := c.errors[key]
	raw, hasFixture := c.fixtures[key]
	c.mu.Unlock()

	if hasErr {
		return err
	}
	if !hasFixture {
		return fmt.Errorf("fake github client: no fixture for operation %q, key %q", op, key)
	}
	return json.Unmarshal(raw, out)
}

var opNameRe = regexp.MustCompile(`(?:query|mutation)\s+(\w+)`)

func operationName(query string) string {
	m := opNameRe.FindStringSubmatch(query)
	if m == nil {
		return ""
	}
	return m[1]
}

// Key builds the dispatch key for an operation name and variable set.
// Exported so tests can assert exactly what a source queried for.
func Key(operation string, vars map[string]any) string {
	b, err := json.Marshal(vars)
	if err != nil {
		return operation + "|<unmarshalable vars>"
	}
	return operation + "|" + string(b)
}
