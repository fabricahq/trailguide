// Exercise how trailguide project update moves a source of skills that follows a branch, in human output and JSON.

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

// newBranchUpdateFixture syncs a project that installs every skill of a repository that isn't a library from its
// branch main, then commits a change to the skill review on main, returning the commits main moves between.
func newBranchUpdateFixture(t *testing.T) (updateFixture, string, string) {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{"tools/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init", "--skill-targets", "claude"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  skills:\n    repository: "+f.Repository+"\n    ref: main\n    skills:\n      include: [\"*\"]\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	moved, err := f.Commit(ctx, f.Worktree(), "Review carefully", map[string][]byte{"tools/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review carefully\n")})
	if err != nil {
		t.Fatal(err)
	}
	return u, f.LatestCommit, moved
}

func TestUpdate_PreviewsTheCommitABranchMovesTo(t *testing.T) {
	u, from, to := newBranchUpdateFixture(t)
	out, diagnostic, code := u.run(t, "project", "update", "skills")
	want := "skills\n" +
		"  main: " + from[:12] + " -> " + to[:12] + "\n" +
		"  changed   tools/review\n" +
		"            Changed: SKILL.md\n" +
		"\nThis is a preview; no files were written."
	if code != 0 || diagnostic != "" || !strings.HasPrefix(out, want) {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant it to start:\n%s", code, diagnostic, out, want)
	}
}

func TestUpdate_JSONReportsTheCommitABranchMovesTo(t *testing.T) {
	u, from, to := newBranchUpdateFixture(t)
	out, diagnostic, code := u.run(t, "project", "update", "--json", "--yes")
	var result struct {
		OK    bool
		Value struct {
			Applied bool
			Sources []json.RawMessage
		}
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || diagnostic != "" || !result.OK || !result.Value.Applied || len(result.Value.Sources) != 1 {
		t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
	}
	want := `{"name":"skills","ref":"main","followsRef":true,"commit":{"from":"` + from + `","to":"` + to + `"},"rules":[],"skills":[{"id":"tools/review","change":"changed","files":["SKILL.md"],"scripts":[]}],"replacedSkills":[],"skillLevels":[]}`
	if got := compactJSON(t, result.Value.Sources[0]); got != want {
		t.Fatalf("source:\n%s\nwant:\n%s", got, want)
	}
	// Sync keeps the commit the update recorded, so its installed skill names it.
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	installed, err := os.ReadFile(filepath.Join(u.directory, ".claude", "skills", "review", "SKILL.md"))
	if err != nil || !strings.Contains(string(installed), to) || !strings.Contains(string(installed), "carefully") {
		t.Fatalf("installed skill, %v:\n%s", err, installed)
	}
}
