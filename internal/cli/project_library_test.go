// Check that one repository whose .trailguide/config.yaml has both sources and a library section works as a
// project and as a library.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// TestRepository_IsBothAProjectAndALibrary syncs, builds, checks, and updates the project a repository keeps in
// .trailguide/, importing another library, while the same repository checks, records changes to, and publishes the
// library its .trailguide/rules/ holds, whose library releases leave out the project's sources and its vendored and
// generated files.
func TestRepository_IsBothAProjectAndALibrary(t *testing.T) {
	binary := buildCLI(t)
	ctx := context.Background()
	upstream := versionedLibrary(t, true)
	config := "schemaVersion: 2\nsources:\n  upstream:\n    repository: git@fixture.invalid:upstream\n    ref: release/1\n    rules:\n      include:\n        - techs/go/*\nlibrary: {schemaVersion: 1}\n"
	team, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                   []byte(config),
		"practices/testing/_group.yaml": []byte("name: Testing\ndescription: Testing guidance.\nwhenToRead: When testing.\n"),
		"practices/testing/a.md":        []byte(libraryRule("Test the retry limit.")),
		"practices/testing/b.md":        []byte(libraryRule("Test the backoff.")),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = team.Close() })
	if _, err := team.Command(ctx, "tag", "--annotate", "--cleanup=verbatim", "--message", releaseOne, "release/1"); err != nil {
		t.Fatal(err)
	}
	environment, err := team.Route(map[string]*gitfixture.Fixture{"upstream": upstream, "team": team})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := team.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := team.CommandIn(ctx, dir, "remote", "set-url", "origin", "git@fixture.invalid:team"); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, environment, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", args, code, out, diagnostic)
		}
		return out
	}
	commit := func(message string) {
		t.Helper()
		for _, args := range [][]string{{"add", "--all"}, {"commit", "--quiet", "-m", message}, {"push", "--quiet", "origin", "main"}} {
			if _, err := team.CommandIn(ctx, dir, args...); err != nil {
				t.Fatal(err)
			}
		}
	}

	// As a project, the repository imports the upstream library and builds it with its own rules.
	run("project", "sync")
	run("project", "build")
	run("project", "check")
	rules, err := os.ReadFile(filepath.Join(dir, ".trailguide", "generated", "RULES.md"))
	if err != nil || !strings.Contains(string(rules), "techs/go") || !strings.Contains(string(rules), "practices/testing") {
		t.Fatalf("generated guidance lacks the imported or the project's own group: %v\n%s", err, rules)
	}
	commit("Import the upstream library")

	// As a library, its vendored and generated files aren't library files, so nothing changed since release/1.
	if out := run("library", "check"); !strings.HasPrefix(out, "Library is valid: 1 group(s), 2 rule(s).\n") || strings.Contains(out, "Pending library release") {
		t.Fatalf("library check:\n%s", out)
	}
	writeFiles(t, dir, map[string]string{"practices/testing/a.md": libraryRule("Test the retry limit and one past it.")})
	run("library", "change", "practices/testing/a", "--bump", "patch", "--summary", "Test one past the limit.", "--non-interactive")
	out := run("library", "check", "--json")
	var check struct {
		Value struct {
			PendingRelease struct {
				Release      int
				LibraryFiles []string
				Rules        []struct{ ID, Change string }
			}
		}
	}
	if err := json.Unmarshal([]byte(out), &check); err != nil {
		t.Fatal(err)
	}
	if pending := check.Value.PendingRelease; pending.Release != 2 || len(pending.LibraryFiles) != 0 || len(pending.Rules) != 1 || pending.Rules[0].ID != "practices/testing/a" || pending.Rules[0].Change != "patch" {
		t.Fatalf("pending library release %+v\n%s", pending, out)
	}
	commit("Test one past the limit")
	released, _, code := runCLIWithEnvironment(t, binary, dir, environment, "library", "release", "--no-github-release")
	if code != 0 || !strings.Contains(released, "Published library release 2.\n") || !strings.Contains(released, "  practices/testing/a  patch  1.0.0 -> 1.0.1\n") {
		t.Fatalf("library release: exit %d\n%s", code, released)
	}
	// Another project imports only the library's own files from release/2.
	consumer := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, consumer, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	if err := os.WriteFile(filepath.Join(consumer, ".trailguide", "config.yaml"), []byte("schemaVersion: 2\nsources:\n  team:\n    repository: git@fixture.invalid:team\n    ref: release/1\n    rules:\n      include: [practices/testing/*]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, consumer, environment, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	imported := []string{}
	vendor := filepath.Join(consumer, ".trailguide", "vendor", "team")
	if err := filepath.WalkDir(vendor, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			relative, _ := filepath.Rel(vendor, path)
			imported = append(imported, filepath.ToSlash(relative))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"config.yaml", "rules/practices/testing/_group.yaml", "rules/practices/testing/a.md", "rules/practices/testing/b.md"}; !slices.Equal(imported, want) {
		t.Fatalf("imported %v, want %v", imported, want)
	}
	if manifest, err := os.ReadFile(filepath.Join(vendor, "config.yaml")); err != nil || string(manifest) != `{"library":{"schemaVersion":1},"sources":{}}`+"\n" {
		t.Fatalf("imported a manifest with more than the library section: %v\n%s", err, manifest)
	}

	// As a project again, it updates the upstream library to its next library release.
	if _, err := upstream.Commit(ctx, upstream.Worktree(), "Clarify errors", gitfixture.Library(map[string][]byte{"techs/go/errors.md": []byte("---\ntitle: Return errors\nimpact: HIGH\nimpactDescription: Keep failures visible.\nwhenToRead: When calling functions.\n---\nReturn errors to the caller, wrapped.\n")})); err != nil {
		t.Fatal(err)
	}
	if err := upstream.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.1\nchanges:\n  techs/go/errors: {change: patch, from: 1.0.0, summaries: [Wrap errors.]}\n"); err != nil {
		t.Fatal(err)
	}
	run("project", "update", "--yes")
	run("project", "check")
	vendored, err := os.ReadFile(filepath.Join(dir, ".trailguide", "vendor", "upstream", "rules", "techs", "go", "errors.md"))
	if err != nil || !strings.Contains(string(vendored), "wrapped") {
		t.Fatalf("update didn't import upstream's release/2: %v\n%s", err, vendored)
	}
	if out := run("library", "check"); !strings.HasPrefix(out, "Library is valid: 1 group(s), 2 rule(s).\n") {
		t.Fatalf("library check after the project update:\n%s", out)
	}
}
