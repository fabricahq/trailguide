// Check that every exported parser reports invalid input as *ValidationError, whichever rule the input breaks.

package trailguide_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

const ruleDocument = "---\ntitle: Handle errors\nimpact: HIGH\nimpactDescription: Unhandled errors hide failures.\nwhenToRead: Before returning errors.\n---\n\nCheck every error.\n"

// TestParsers_ReportInvalidInputAsValidationError covers the shared decoding rules and the rule and group ID rules
// that Trail Guide checks in internal packages: each failure reaches callers as *ValidationError at the input's
// location, keeps its text, and carries no error type from outside this package.
func TestParsers_ReportInvalidInputAsValidationError(t *testing.T) {
	for _, test := range []struct {
		name, location, problem string
		parse                   func() error
	}{
		{
			name: "duplicate key", location: "release/2.release", problem: "duplicate field",
			parse: record("schemaVersion: 1\nrelease: 2\nrelease: 2\nrules: {}\n"),
		},
		{
			name: "explicit tag", location: "release/2.release", problem: "anchors, aliases, and explicit tags are not supported",
			parse: record("schemaVersion: 1\nrelease: !!int 2\nrules: {}\n"),
		},
		{
			name: "multi-line summary", location: "release/2.changes.techs/go/a.summaries[0]", problem: "expected one line",
			parse: record("schemaVersion: 1\nrelease: 2\nrules: {techs/go/a: 1.0.1}\nchanges:\n  techs/go/a: {change: patch, from: 1.0.0, summaries: [\"One\\nTwo\"]}\n"),
		},
		{
			name: "invalid rule ID", location: "release/2.rules.techs/Go/a", problem: "invalid group ID",
			parse: record("schemaVersion: 1\nrelease: 2\nrules: {techs/Go/a: 1.0.0}\n"),
		},
		{
			name: "uncontained library file", location: "release/2.libraryFiles[0]", problem: "expected a contained relative path",
			parse: record("schemaVersion: 1\nrelease: 2\nrules: {}\nlibraryFiles: [../outside.md]\n"),
		},
		{
			name: "invalid Unicode in a release message", location: "release/2", problem: "expected UTF-8 YAML",
			parse: func() error {
				_, _, err := trailguide.ParseReleaseMessage("release/2", []byte("Notes.\n---\nschemaVersion: 1\nrelease: 2\nrules: {}\n#\xff\n"))
				return err
			},
		},
		{
			name: "invalid Unicode in group metadata", location: "techs/go/_group.yaml", problem: "expected UTF-8 YAML",
			parse: func() error {
				_, err := trailguide.ParseGroupMetadata([]byte("name: Go\ndescription: Go.\nwhenToRead: \xff\n"), "techs/go/_group.yaml")
				return err
			},
		},
		{
			name: "unknown canonical group field", location: "list.techs/go", problem: "unknown field icon",
			parse: func() error {
				_, err := trailguide.ParseCanonicalGroups([]byte("techs/go:\n  name: Go\n  description: Go.\n  icon: go.svg\n"), "list")
				return err
			},
		},
		{
			name: "invalid canonical group ID", location: "list.techs/Go", problem: "invalid group ID",
			parse: func() error {
				_, err := trailguide.ParseCanonicalGroups([]byte("techs/Go:\n  name: Go\n  description: Go.\n"), "list")
				return err
			},
		},
		{
			name: "uncontained rule path", location: "fabrica:../a.md", problem: "expected a contained relative path",
			parse: func() error {
				_, err := trailguide.ParseRule(ruleDocument, "../a.md", "fabrica")
				return err
			},
		},
		{
			name: "duplicate frontmatter key", location: "fabrica:techs/go/a.md", problem: "duplicate mapping key",
			parse: func() error {
				_, err := trailguide.ParseRule(strings.Replace(ruleDocument, "impact: HIGH\n", "impact: HIGH\nimpact: LOW\n", 1), "techs/go/a.md", "fabrica")
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.parse()
			var invalid *trailguide.ValidationError
			if !errors.As(err, &invalid) || invalid.Location != test.location || !strings.Contains(invalid.Problem, test.problem) {
				t.Fatalf("got %v; want a *ValidationError at %s about %q", err, test.location, test.problem)
			}
			requireOnlyPublicErrors(t, err)
		})
	}
}

// TestParseRule_KeepsTheRulePathAroundAnInvalidGroupID reports a rule path whose group isn't a valid group ID with
// the path's context in the text and a *ValidationError for the group ID beneath it.
func TestParseRule_KeepsTheRulePathAroundAnInvalidGroupID(t *testing.T) {
	_, err := trailguide.ParseRule(ruleDocument, "techs/Go/a.md", "fabrica")
	var invalid *trailguide.ValidationError
	if !errors.As(err, &invalid) || invalid.Location != "fabrica:techs/Go/a.md" || !strings.HasPrefix(invalid.Problem, `invalid group ID "techs/Go"`) {
		t.Fatalf("got %v; want the group ID refused", err)
	}
	if want := `rule path "techs/Go/a.md": ` + invalid.Error(); err.Error() != want {
		t.Fatalf("got %q; want %q", err.Error(), want)
	}
	requireOnlyPublicErrors(t, err)
}

// record returns a function that parses input as the release record of release/2.
func record(input string) func() error {
	return func() error {
		_, err := trailguide.ParseReleaseRecord([]byte(input), "release/2")
		return err
	}
}

// requireOnlyPublicErrors fails when any error in err's chain comes from one of Trail Guide's internal packages, whose
// types callers outside the module can't name.
func requireOnlyPublicErrors(t *testing.T, err error) {
	t.Helper()
	for ; err != nil; err = errors.Unwrap(err) {
		kind := reflect.TypeOf(err)
		if kind.Kind() == reflect.Pointer {
			kind = kind.Elem()
		}
		if strings.Contains(kind.PkgPath(), "/internal/") {
			t.Fatalf("the error chain exposes %s from %s", kind.Name(), kind.PkgPath())
		}
	}
}
