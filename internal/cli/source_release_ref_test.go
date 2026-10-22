// Check that project add source writes the library release a source takes, and that sync names the ref to write.

package cli

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
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
		if code != 0 || !strings.Contains(out, "Set sources.team.ref to release/1, the library's newest release.") || !strings.Contains(string(config), "    ref: release/1\n") {
			t.Fatalf("exit %d, stdout:\n%s\nstderr: %s\nconfig.yaml:\n%s", code, out, diagnostic, config)
		}
		if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync"); code != 0 {
			t.Fatal(code, out, diagnostic)
		}
	}
}

// TestUpdate_TakesTheEditedReleaseWithANewRuleAndNoFurther: with release 1 installed and release 3 published, a ref
// edited to release/2 that also selects a rule release 2 added installs release 2, keeps ref: release/2, and says
// release 3 is available.
func TestUpdate_TakesTheEditedReleaseWithANewRuleAndNoFurther(t *testing.T) {
	ctx := context.Background()
	u := newUpdateFixture(t)
	if _, err := u.fixture.Commit(ctx, u.fixture.Worktree(), "Third release", gitfixture.Library(map[string][]byte{"techs/go/verify.md": updateRule("verify 1.1.0")})); err != nil {
		t.Fatal(err)
	}
	if err := u.fixture.Release(ctx, 3, "schemaVersion: 1\nrelease: 3\nrules:\n  techs/go/backoff: 2.0.0\n  techs/go/errors: 2.0.0\n  techs/go/format: 1.0.1\n  techs/go/loaders: 1.1.0\n  techs/go/naming: 1.1.0\n  techs/go/verify: 1.1.0\nchanges:\n  techs/go/verify: {change: minor, from: 1.0.0, summaries: [Add a case.]}\n"); err != nil {
		t.Fatal(err)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nsources:\n  team:\n    repository: "+u.fixture.Repository+"\n    ref: release/2\n    rules:\n      include:\n        - techs/go/errors\n        - techs/go/verify\n")
	out, diagnostic, code := u.run(t, "project", "update", "team", "--yes", "--json")
	var result struct {
		OK    bool
		Value struct {
			Sources []struct {
				SetsRef   string
				Available int
			}
		}
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || !result.OK || len(result.Value.Sources) != 1 || result.Value.Sources[0].SetsRef != "" || result.Value.Sources[0].Available != 3 {
		t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
	}
	config, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "config.yaml"))
	if err != nil || !strings.Contains(string(config), "    ref: release/2\n") {
		t.Fatalf("config.yaml, %v:\n%s", err, config)
	}
	data, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "lock.json"))
	var lock struct {
		Sources map[string]struct{ Release int }
	}
	if err == nil {
		err = json.Unmarshal(data, &lock)
	}
	if err != nil || lock.Sources["team"].Release != 2 {
		t.Fatalf("lock.json, %v:\n%s", err, data)
	}
	if out, diagnostic, code := u.run(t, "project", "check"); code != 0 {
		t.Fatalf("check: exit %d\n%s%s", code, out, diagnostic)
	}
}
