// Check that project add source writes the library release a source takes, and that sync names the ref to write.

package cli

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAddSource_WritesTheNewestLibraryReleaseAsTheRef: without --ref, project add source writes ref: release/N for
// the library's newest release and says so; a repository without library releases fails with ref-required and
// writes nothing.
func TestAddSource_WritesTheNewestLibraryReleaseAsTheRef(t *testing.T) {
	binary := buildCLI(t)
	for _, released := range []bool{true, false} {
		f := versionedLibrary(t, released)
		directory := t.TempDir()
		if out, diagnostic, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
			t.Fatal(code, out, diagnostic)
		}
		before := projectFileContents(t, directory)
		out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "add", "source", "team", "--repository", f.Repository, "--rules", "techs/go/*", "--non-interactive")
		config, err := os.ReadFile(filepath.Join(directory, ".trailguide", "config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if !released {
			if code != 1 || !strings.Contains(diagnostic, "the repository has no release/<number> tags, so it isn't a library or hasn't published its first library release; pass --ref") || !maps.Equal(before, projectFileContents(t, directory)) {
				t.Fatalf("exit %d, stderr %q", code, diagnostic)
			}
			continue
		}
		if code != 0 || !strings.Contains(out, "Set sources.team.ref to release/1, the library's newest release. To follow each new release instead, set it to latest.") || !strings.Contains(string(config), "    ref: release/1\n") {
			t.Fatalf("exit %d, stdout:\n%s\nstderr: %s\nconfig.yaml:\n%s", code, out, diagnostic, config)
		}
		if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync"); code != 0 {
			t.Fatal(code, out, diagnostic)
		}
	}
}
