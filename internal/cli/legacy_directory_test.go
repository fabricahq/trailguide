// Check that projects and libraries that keep their files in .code-rules/, the directory Code Rules used before it
// was renamed Trail Guide, keep working or are told how to move.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// legacyLibrary returns a fixture whose release/1, made before the rename, published rules a and b from
// .code-rules/, and an author's full clone of it.
func legacyLibrary(t *testing.T) (*gitfixture.Fixture, string) {
	t.Helper()
	ctx := context.Background()
	files := map[string][]byte{}
	for name, data := range gitfixture.Library(map[string][]byte{
		"config.yaml":                   []byte("schemaVersion: 2\nsources: {}\nlibrary: {formatVersion: 1}\n"),
		"practices/testing/_group.yaml": []byte("name: Testing\ndescription: Testing guidance.\nwhenToRead: When testing.\n"),
		"practices/testing/a.md":        []byte(libraryRule("Test the retry limit.")),
		"practices/testing/b.md":        []byte(libraryRule("Test the backoff.")),
	}) {
		files[librarytree.LegacyDirectory+strings.TrimPrefix(name, librarytree.Directory)] = data
	}
	fixture, err := gitfixture.New(ctx, files)
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
	return fixture, dir
}

// TestProject_KeepsWorkingInTheLegacyDirectory syncs, builds, and checks a project that still keeps its files in
// .code-rules/, importing a library whose release keeps its files there too, and warns, in human and JSON output,
// that the directory should be renamed, without creating .trailguide/.
func TestProject_KeepsWorkingInTheLegacyDirectory(t *testing.T) {
	binary := buildCLI(t)
	team, _ := legacyLibrary(t)
	environment, err := team.Route(map[string]*gitfixture.Fixture{"team": team})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, dir, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	if err := os.Rename(filepath.Join(dir, librarytree.Directory), filepath.Join(dir, librarytree.LegacyDirectory)); err != nil {
		t.Fatal(err)
	}
	config := "schemaVersion: 2\nsources:\n  team:\n    repository: git@fixture.invalid:team\n    ref: release/1\n    rules:\n      include: [practices/testing/*]\n"
	if err := os.WriteFile(filepath.Join(dir, librarytree.LegacyDirectory, "config.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	warning := "Warning: " + legacyDirectoryWarning + "\n\n"
	for _, command := range []string{"sync", "build", "check"} {
		out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, environment, "project", command)
		if code != 0 || diagnostic != warning {
			t.Fatalf("project %s: exit %d, stderr %q, stdout:\n%s", command, code, diagnostic, out)
		}
	}
	vendored, err := os.ReadFile(filepath.Join(dir, librarytree.LegacyDirectory, "vendor", "team", "rules", "practices", "testing", "a.md"))
	if err != nil || !strings.Contains(string(vendored), "Test the retry limit.") {
		t.Fatalf("sync didn't import release/1 from the library's .code-rules/: %v\n%s", err, vendored)
	}
	if _, err := os.Lstat(filepath.Join(dir, librarytree.Directory)); !os.IsNotExist(err) {
		t.Fatalf("a project command created %s beside %s: %v", librarytree.Directory, librarytree.LegacyDirectory, err)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, environment, "project", "check", "--json")
	var result response
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || diagnostic != "" || !result.OK || len(result.Warnings) != 1 || result.Warnings[0] != legacyDirectoryWarning {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	// Renaming the directory, as the warning says, is all the project needs.
	if err := os.Rename(filepath.Join(dir, librarytree.LegacyDirectory), filepath.Join(dir, librarytree.Directory)); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, environment, "project", "build"); code != 0 || diagnostic != "" {
		t.Fatalf("build after the rename: exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, environment, "project", "check"); code != 0 || diagnostic != "" {
		t.Fatalf("check after the rename: exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}

// TestLibrary_MovesOutOfTheLegacyDirectory refuses library commands while the library keeps its files in
// .code-rules/, and, once the directory is renamed, compares the library with its release from .code-rules/ without
// reporting any change.
func TestLibrary_MovesOutOfTheLegacyDirectory(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := legacyLibrary(t)
	want := "Error: " + legacyLibraryDirectoryProblem + "\n\nLocation: .code-rules/\n"
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check"); code != 1 || out != "" || diagnostic != want {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "init"); code != 1 || out != "" || diagnostic != want {
		t.Fatalf("init: exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	if _, err := fixture.CommandIn(context.Background(), dir, "mv", librarytree.LegacyDirectory, librarytree.Directory); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.CommandIn(context.Background(), dir, "commit", "--quiet", "--message", "Rename .code-rules to .trailguide"); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check")
	if code != 0 || diagnostic != "" || !strings.HasSuffix(out, "\nNothing to publish: no pending change notes and no library-wide changes since release/1.\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}

// legacyLibraryDirectoryProblem is the problem library commands report for a library in .code-rules/.
const legacyLibraryDirectoryProblem = `Code Rules is now Trail Guide, and library commands read a library's files from .trailguide/. Rename the directory, keeping everything in it, then run the command again:

  git mv .code-rules .trailguide

Library releases published from .code-rules/ stay readable, so projects keep their versions. Projects that import this library need no changes. Update the line in AGENTS.md or CLAUDE.md that points agents to .code-rules/generated/RULES.md.`
