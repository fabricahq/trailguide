// Check that every library command refuses a library that still keeps its manifest and rule folders at the
// repository root.

package cli

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// legacyLibraryProblem is the problem every library command reports for the library below, in the layout before
// version 2.
const legacyLibraryProblem = `this version of Trail Guide reads a library's manifest from the library section of .trailguide/config.yaml, and its groups, shared assets, and change notes from .trailguide/. It no longer reads rule-library.yaml or rule folders at the repository root, and doesn't move them. Move them yourself, keeping each path below its folder:

  rule-library.yaml              ->  the library section of .trailguide/config.yaml
  practices/testing/_group.yaml  ->  .trailguide/rules/practices/testing/_group.yaml
  practices/testing/a.md         ->  .trailguide/rules/practices/testing/a.md
  practices/testing/b.md         ->  .trailguide/rules/practices/testing/b.md

Then delete rule-library.yaml. Write its license fields, without formatVersion, in .trailguide/config.yaml, beside the sources the repository uses as a project, if any:

  schemaVersion: 2
  sources: {}
  library:
    schemaVersion: 2
    name: acme
    license:
      spdxExpression: MIT
      file: LICENSE.md
      notices: []

License and notice paths stay relative to the repository root. Rule IDs and versions don't change. A moved rule sits two folders deeper, so its relative links to shared assets need one more ../, such as ../../../assets/retry-lifecycle.svg, and its links to license files at the repository root need two more; links that start with /assets/ still work. Once the library has a library release, record each rule whose links change with trailguide library change --bump patch. Then run trailguide library check.`

// TestLibraryCommands_RejectTheLayoutBeforeVersion2 refuses, writing nothing, every library command in a released
// library that keeps rule-library.yaml and its rule folders at the repository root, and lists where each of its files
// belongs now, in human and JSON output.
func TestLibraryCommands_RejectTheLayoutBeforeVersion2(t *testing.T) {
	binary := buildCLI(t)
	ctx := context.Background()
	fixture, err := gitfixture.New(ctx, map[string][]byte{
		"rule-library.yaml":             []byte("formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE.md\n  notices: []\n"),
		"LICENSE.md":                    []byte("MIT License\n"),
		"practices/testing/_group.yaml": []byte("name: Testing\ndescription: Testing guidance.\nwhenToRead: When testing.\n"),
		"practices/testing/a.md":        []byte(libraryRule("Test the retry limit.")),
		"practices/testing/b.md":        []byte(libraryRule("Test the backoff.")),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	if _, err := fixture.Command(ctx, "tag", "--annotate", "--cleanup=verbatim", "--message", releaseOne, "release/1"); err != nil {
		t.Fatal(err)
	}
	dir, err := fixture.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{"body.md": "Test every retry.\n"})
	before := projectFileContents(t, dir)
	for _, args := range [][]string{
		{"library", "init"},
		{"library", "check"},
		{"library", "add", "group", "techs/go", "--name", "Go", "--description", "Go guidance.", "--when-to-read", "When writing Go."},
		{"library", "add", "rule", "practices/testing/retries", "--title", "Test retries", "--impact", "HIGH", "--impact-description", "Catch retry bugs.", "--when-to-read", "When changing retries.", "--body-file", "body.md"},
		{"library", "change", "practices/testing/a", "--bump", "patch", "--summary", "Fix a.", "--non-interactive"},
		{"library", "release", "--dry-run"},
	} {
		t.Run(strings.Join(args[1:min(len(args), 3)], " "), func(t *testing.T) {
			want := "Error: " + legacyLibraryProblem + "\n\nLocation: rule-library.yaml\n"
			if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, args...); code != 1 || out != "" || diagnostic != want {
				t.Errorf("exit %d\n%s%s", code, out, diagnostic)
			}
			out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, append(args, "--json")...)
			var result response
			if err := json.Unmarshal([]byte(out), &result); err != nil || code != 1 || diagnostic != "" || result.OK || result.Error == nil {
				t.Fatalf("exit %d\n%s%s", code, out, diagnostic)
			}
			if got := *result.Error; got != (responseError{Kind: "validation", Message: "rule-library.yaml: " + legacyLibraryProblem, Location: "rule-library.yaml"}) {
				t.Errorf("error %+v", got)
			}
		})
	}
	if !reflect.DeepEqual(before, projectFileContents(t, dir)) {
		t.Fatal("a command changed the library")
	}
}
