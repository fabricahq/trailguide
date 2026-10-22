// Project and build the refs that configuration tests compare with their fixtures.

package rules_test

import (
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
)

// projectedRef is a ref's canonical form as the fixtures describe it: a commit's lowercase SHA, or a tag's or branch's
// full name.
type projectedRef struct {
	Kind rules.GitRefKind `json:"kind"`
	SHA  string           `json:"sha,omitempty"`
	Name string           `json:"name,omitempty"`
}

// projection returns ref's canonical form for comparison with a fixture.
func projection(ref rules.GitRef) projectedRef {
	if ref.Kind() == rules.GitRefCommit {
		return projectedRef{Kind: ref.Kind(), SHA: ref.Canonical()}
	}
	return projectedRef{Kind: ref.Kind(), Name: ref.Canonical()}
}

// ref parses text, failing the test when it isn't a valid ref.
func ref(t *testing.T, text string) rules.GitRef {
	t.Helper()
	parsed, err := rules.ParseGitRef(text, "ref")
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
