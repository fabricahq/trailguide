// Read published rule versions from a real library's release tags, with the shared assets their files link to.

package imports

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// forkRule is a rule document whose body is text.
func forkRule(text string) []byte {
	return []byte("---\ntitle: Return errors\nimpact: HIGH\nimpactDescription: Preserve failures.\nwhenToRead: When calling functions.\n---\n" + text + "\n")
}

// forkLibrary publishes release/1 with techs/go/errors at 1.0.0, linking to shared assets directly and through
// an asset, and release/2, in which errors moves to 1.1.0 without those links.
func forkLibrary(t *testing.T) (*gitfixture.Fixture, Options) {
	t.Helper()
	ctx := context.Background()
	f := newLibraryFixture(t, map[string][]byte{
		"config.yaml":                     []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1,"license":{"file":"LICENSE","notices":[".trailguide/assets/NOTICE.md"]}}}`),
		"LICENSE":                         []byte("Terms\n"),
		"techs/go/_group.yaml":            []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When editing Go."}`),
		"techs/go/errors.md":              forkRule("[Guide](../../../assets/guide.md) [Notes](assets/errors/notes.md) [Notice](../../../assets/NOTICE.md)"),
		"techs/go/assets/errors/notes.md": []byte("![Diagram](../../../../../assets/diagrams/flow.svg)\n"),
		"techs/go/other.md":               forkRule("Other."),
		"techs/go/assets/other/data.bin":  {0, 255},
		"assets/NOTICE.md":                []byte("Notice\n"),
		"assets/guide.md":                 []byte("[More](more.md) [Terms](../../LICENSE)\n"),
		"assets/more.md":                  []byte("[Guide](guide.md)\n"),
		"assets/diagrams/flow.svg":        []byte("<svg/>"),
		"assets/unrelated.md":             []byte("Unrelated.\n"),
	})
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\n  techs/go/other: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n  techs/go/other: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(map[string][]byte{"techs/go/errors.md": forkRule("Wrap errors."), "techs/go/assets/errors/notes.md": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\n  techs/go/other: 1.0.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\n"); err != nil {
		t.Fatal(err)
	}
	return f, Options{GitPath: f.GitPath, Environment: f.Environment}
}

// readPublished reads version of rule id from the fixture as source team.
func readPublished(t *testing.T, f *gitfixture.Fixture, options Options, id, version string) (PublishedRule, error) {
	t.Helper()
	parsed, err := trailguide.ParseRuleVersion(version, "version")
	if err != nil {
		t.Fatal(err)
	}
	return ReadPublishedRule(context.Background(), rules.Source{Name: "team", Repository: f.Repository}, id, parsed, options)
}

// TestReadPublishedRule_ReadsAnOlderVersionWithItsLinkedSharedAssets reads 1.0.0 from release/1 after release/2
// changed it: the rule's own files, the shared assets reached through links, and its group's metadata, but no
// declared terms, unlinked shared assets, or other rules.
func TestReadPublishedRule_ReadsAnOlderVersionWithItsLinkedSharedAssets(t *testing.T) {
	f, options := forkLibrary(t)
	commit, err := f.Command(context.Background(), "rev-parse", "release/1^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	got, err := readPublished(t, f, options, "techs/go/errors", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"assets/diagrams/flow.svg", "assets/guide.md", "assets/more.md", "techs/go/assets/errors/notes.md", "techs/go/errors.md"}
	if got.Release != 1 || got.Commit != commit || !reflect.DeepEqual(slices.Sorted(maps.Keys(got.Files)), want) {
		t.Fatalf("got release %d at %s with %v; want release 1 at %s with %v", got.Release, got.Commit, slices.Sorted(maps.Keys(got.Files)), commit, want)
	}
	if !strings.Contains(string(got.Files["techs/go/errors.md"]), "[Guide]") || string(got.Files["assets/diagrams/flow.svg"]) != "<svg/>" || !strings.Contains(string(got.GroupMetadata), `"name":"Go"`) {
		t.Fatalf("unexpected bytes: %q, %q", got.Files["techs/go/errors.md"], got.GroupMetadata)
	}
}

// TestReadPublishedRule_ReadsTheNewestVersionFromItsRelease reads 1.1.0 from release/2, which links to nothing.
func TestReadPublishedRule_ReadsTheNewestVersionFromItsRelease(t *testing.T) {
	f, options := forkLibrary(t)
	commit, err := f.Command(context.Background(), "rev-parse", "release/2^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	got, err := readPublished(t, f, options, "techs/go/errors", "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Release != 2 || got.Commit != commit || !reflect.DeepEqual(slices.Sorted(maps.Keys(got.Files)), []string{"techs/go/errors.md"}) || string(got.Files["techs/go/errors.md"]) != string(forkRule("Wrap errors.")) {
		t.Fatalf("got release %d with %v", got.Release, slices.Sorted(maps.Keys(got.Files)))
	}
}

// TestReadPublishedRule_ReportsMissingVersions names the published versions of a rule that never published the
// requested one, and fails with version-not-found for an unknown rule and releases-not-found before any library
// release.
func TestReadPublishedRule_ReportsMissingVersions(t *testing.T) {
	f, options := forkLibrary(t)
	unreleased := newLibraryFixture(t, map[string][]byte{"techs/go/errors.md": forkRule("Unreleased.")})
	for _, test := range []struct {
		name, id, version, code, message string
		fixture                          *gitfixture.Fixture
	}{
		{"unknown version", "techs/go/errors", "1.2.0", "version-not-found", "Its published versions, newest first: 1.1.0, 1.0.0.", f},
		{"unknown rule", "techs/go/missing", "1.0.0", "version-not-found", "never published a rule techs/go/missing", f},
		{"no library release", "techs/go/errors", "1.0.0", "releases-not-found", "hasn't published its first library release", unreleased},
	} {
		t.Run(test.name, func(t *testing.T) {
			git := options
			git.Environment = test.fixture.Environment
			_, err := readPublished(t, test.fixture, git, test.id, test.version)
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != test.code || !strings.Contains(failure.Problem, test.message) {
				t.Fatalf("got %v; want %s containing %q", err, test.code, test.message)
			}
		})
	}
}
