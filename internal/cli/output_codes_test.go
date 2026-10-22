// Check that failures from Git and imports keep their stable codes in JSON output.

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

// TestSyncJSON_ReportsGitFailureCode lets scripts branch on why an import failed without parsing its message.
func TestSyncJSON_ReportsGitFailureCode(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	for _, args := range [][]string{
		{"project", "init"},
		{"project", "add", "source", "team", "--repository", "https://example.invalid/team.git", "--ref", "v1.2.3", "--rules", "*", "--non-interactive"},
	} {
		if out, diagnostic, code := runCLI(t, binary, directory, args...); code != 0 {
			t.Fatal(args, code, out, diagnostic)
		}
	}
	// runCLI puts no executables on PATH, so the import fails before contacting the repository.
	out, _, code := runCLI(t, binary, directory, "project", "sync", "--json")
	var result response
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(out, err)
	}
	if code != 1 || result.Error == nil || result.Error.Code != "git-unavailable" || result.Error.Kind != "operation" {
		t.Fatalf("exit %d, response %s", code, out)
	}
}

// versionedLibrary serves a library whose first library release publishes techs/go/errors at 1.0.0, or no
// library release when released is false.
func versionedLibrary(t *testing.T, released bool) *gitfixture.Fixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`),
		"techs/go/_group.yaml": []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":   []byte("---\ntitle: Return errors\nimpact: HIGH\nimpactDescription: Keep failures visible.\nwhenToRead: When calling functions.\n---\nReturn errors to the caller.\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if released {
		if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// syncWithConfiguration writes a project whose team source has fields, then runs sync with args.
func syncWithConfiguration(t *testing.T, f *gitfixture.Fixture, fields string, args ...string) (string, string, int) {
	t.Helper()
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	config := `{"schemaVersion":2,"sources":{"team":{"repository":"` + f.Repository + `",` + fields + `}}}`
	if err := os.WriteFile(filepath.Join(directory, ".trailguide", "config.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return runCLIWithEnvironment(t, binary, directory, f.Environment, append([]string{"project", "sync"}, args...)...)
}

// TestSyncJSON_ReportsRuleVersionCodes reports the documented codes for missing library releases and versions.
func TestSyncJSON_ReportsRuleVersionCodes(t *testing.T) {
	for _, test := range []struct {
		name, fields, code string
		released           bool
	}{
		{"no library release", `"rules":{"include":["techs/go/*"]}`, "ref-required", false},
		{"released library without a ref", `"rules":{"include":["techs/go/*"]}`, "ref-required", true},
		{"pin to an unpublished version", `"ref":"release/1","rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/errors","version":"2.0.0","reason":"Typo."}]}`, "version-not-found", true},
		{"missing ref", `"rules":{"include":["techs/go/*"]},"ref":"release/7"`, "version-not-found", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, _, code := syncWithConfiguration(t, versionedLibrary(t, test.released), test.fields, "--json")
			var result response
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(out, err)
			}
			if code != 1 || result.Error == nil || result.Error.Code != test.code {
				t.Fatalf("exit %d, response %s", code, out)
			}
		})
	}
}

// TestSync_PrintsWarningsForUnreleasedRefs shows the warning in both output modes.
func TestSync_PrintsWarningsForUnreleasedRefs(t *testing.T) {
	f := versionedLibrary(t, true)
	commit, err := f.Commit(context.Background(), f.Worktree(), "Unreleased", gitfixture.Library(map[string][]byte{"techs/go/errors.md": []byte("---\ntitle: Return errors\nimpact: HIGH\nimpactDescription: Keep failures visible.\nwhenToRead: When calling functions.\n---\nReturn wrapped errors to the caller.\n")}))
	if err != nil {
		t.Fatal(err)
	}
	fields := `"rules":{"include":["techs/go/*"]},"ref":"` + commit + `"`
	out, diagnostic, code := syncWithConfiguration(t, f, fields)
	if code != 0 || !strings.Contains(out, "Warning: Source team imports "+commit+", which isn't a library release") || !strings.Contains(out, "Unreleased rules: techs/go/errors.") {
		t.Fatalf("exit %d\n%s%s", code, out, diagnostic)
	}
	out, _, code = syncWithConfiguration(t, f, fields, "--json")
	var result struct {
		OK    bool
		Value struct{ Warnings []string }
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || !result.OK || len(result.Value.Warnings) != 1 {
		t.Fatalf("exit %d, response %s, %v", code, out, err)
	}
}
