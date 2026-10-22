// Check that projects consume a library through its published contract, whatever its project settings, and that
// the library format decides whether they can.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// formattedLibrary serves a library whose configuration is config and whose first library release publishes
// techs/go/errors at 1.0.0.
func formattedLibrary(t *testing.T, config string) *gitfixture.Fixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte(config),
		"techs/go/_group.yaml": []byte("name: Go\ndescription: Go guidance.\nwhenToRead: When writing Go.\n"),
		"techs/go/errors.md":   []byte("---\ntitle: Return errors\nimpact: HIGH\nimpactDescription: Keep failures visible.\nwhenToRead: When calling functions.\n---\nReturn errors to the caller.\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	return f
}

// consumingProject initializes a project whose team source imports f's Go rules at release/1, and returns its
// directory and a function that runs the CLI there.
func consumingProject(t *testing.T, f *gitfixture.Fixture) (string, func(args ...string) (string, string, int)) {
	t.Helper()
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	config := "schemaVersion: 2\nsources:\n  team:\n    repository: " + f.Repository + "\n    ref: release/1\n    rules:\n      include: [techs/go/*]\n"
	if err := os.WriteFile(filepath.Join(directory, ".trailguide", "config.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return directory, func(args ...string) (string, string, int) {
		return runCLIWithEnvironment(t, binary, directory, f.Environment, args...)
	}
}

// TestSyncAndBuild_ConsumeALibraryWhateverItsProjectSettings imports a library whose configuration uses a project
// schema version, project settings, and a manifest field this Trail Guide doesn't know, then builds and checks the
// project offline from the snapshot, which keeps only the published manifest.
func TestSyncAndBuild_ConsumeALibraryWhateverItsProjectSettings(t *testing.T) {
	f := formattedLibrary(t, `schemaVersion: 9
skillTargets: [some-future-agent]
futureProjectSetting: {any: shape}
sources:
  upstream:
    repository: not a repository address
    rules: {include: [nothing/valid/*], newSelector: true}
library:
  formatVersion: 1
  description: A field a later Trail Guide may add, which consumers can ignore.
`)
	directory, run := consumingProject(t, f)
	for _, args := range [][]string{{"project", "sync"}, {"project", "build"}, {"project", "check"}} {
		if out, diagnostic, code := run(args...); code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", args, code, out, diagnostic)
		}
	}
	rules, err := os.ReadFile(filepath.Join(directory, ".trailguide", "generated", "RULES.md"))
	if err != nil || !strings.Contains(string(rules), "techs/go") {
		t.Fatalf("generated guidance lacks the library's group: %v\n%s", err, rules)
	}
	manifest, err := os.ReadFile(filepath.Join(directory, ".trailguide", "vendor", "team", "config.yaml"))
	want := `{"library":{"description":"A field a later Trail Guide may add, which consumers can ignore.","formatVersion":1},"sources":{}}` + "\n"
	if err != nil || string(manifest) != want {
		t.Fatalf("vendored manifest %s, %v; want only the published contract: %s", manifest, err, want)
	}
}

// TestSync_ReportsAnInvalidOrUnsupportedLibraryFormat refuses a library without a format, with a malformed one, or
// with invalid published fields as a validation error at the field, and one in another format with
// unsupported-library-format.
func TestSync_ReportsAnInvalidOrUnsupportedLibraryFormat(t *testing.T) {
	const manifest = "team/.trailguide/config.yaml: "
	for _, test := range []struct {
		name, library, kind, code, location string
	}{
		{"no format", "library: {}\n", "validation", "", manifest + "library.formatVersion"},
		{"a malformed format", "library: {formatVersion: latest}\n", "validation", "", manifest + "library.formatVersion"},
		{"another format", "library: {formatVersion: 2, skills: {required: skills/everything}}\n", "operation", "unsupported-library-format", ""},
		{"invalid skill levels", "library: {formatVersion: 1, skills: {required: skills/everything}}\n", "validation", "", manifest + "library.skills.required"},
		{"an invalid curated ref", "sources:\n  helper: {repository: https://example.invalid/helper.git, ref: 'refs/remotes/origin/main'}\nlibrary: {formatVersion: 1, skills: {offered: [helper:skills/help]}}\n", "validation", "", manifest + "sources.helper.ref"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, run := consumingProject(t, formattedLibrary(t, "schemaVersion: 2\n"+test.library))
			out, _, code := run("project", "sync", "--json")
			var result response
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(out, err)
			}
			if code != 1 || result.Error == nil || result.Error.Kind != test.kind || result.Error.Code != test.code || result.Error.Location != test.location {
				t.Fatalf("exit %d, response %s", code, out)
			}
			if test.code != "" && (!strings.Contains(result.Error.Message, "format 2") || !strings.Contains(result.Error.Message, "upgrade Trail Guide")) {
				t.Fatalf("message %q doesn't name the formats and the upgrade", result.Error.Message)
			}
		})
	}
}

// TestLibraryCheck_RejectsWhatConsumersIgnore holds the library's author to the whole configuration: a manifest
// field this Trail Guide doesn't know, which consumers ignore, fails library check.
func TestLibraryCheck_RejectsWhatConsumersIgnore(t *testing.T) {
	binary := buildCLI(t)
	dir := t.TempDir()
	for _, args := range [][]string{{"library", "init"}, {"library", "check"}} {
		if out, diagnostic, code := runCLI(t, binary, dir, args...); code != 0 {
			t.Fatal(args, code, out, diagnostic)
		}
	}
	config := filepath.Join(dir, ".trailguide", "config.yaml")
	if err := os.WriteFile(config, []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n  description: Shared rules.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, binary, dir, "library", "check", "--json")
	var result response
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(out, err)
	}
	if code != 1 || result.Error == nil || result.Error.Kind != "validation" || !strings.Contains(result.Error.Message, "unknown field description") {
		t.Fatalf("exit %d, response %s", code, out)
	}
}
