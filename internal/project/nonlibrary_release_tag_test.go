// Check that a skills repository that isn't a library imports its skills from a tag shaped like a library release.

package project

import (
	"context"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// TestSync_ANonLibraryReleaseTagImportsThatRevision syncs a skills repository without a library manifest at its
// annotated release/2 tag, as at any other tag, and an update leaves it there.
func TestSync_ANonLibraryReleaseTagImportsThatRevision(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{"tools/review/SKILL.md": []byte("# Review\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Tag(ctx, f.Worktree(), "release/2", "Version 2\n"); err != nil {
		t.Fatal(err)
	}
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"tools":{"repository":REPOSITORY,"ref":"release/2","skills":{"include":["*"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, options, ".claude/skills/review/SKILL.md"); !strings.Contains(installed, "# Review") {
		t.Fatalf("installed skill:\n%s", installed)
	}
	requireCurrent(t, options)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := updateProject(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
}
