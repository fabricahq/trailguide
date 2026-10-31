// Test that library check rejects a skill's link to a file the library's repository wouldn't supply to projects.

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/skillrefs"
)

func TestCheck_RequiresEverySkillLinkToNameAFileTheLibraryPublishes(t *testing.T) {
	ctx := context.Background()
	_, options := authorClone(t, libraryFiles(), releaseOne)
	edit(t, options.Directory, map[string]string{
		".trailguide/skills/library/attack/SKILL.md":   "---\nname: attack\n---\nCompare [Redesign](../redesign/SKILL.md), and read [the checklist](../../../../docs/checklist.md) and [the essay](https://example.invalid/essay.md).\n",
		".trailguide/skills/library/redesign/SKILL.md": "---\nname: redesign\n---\nFollow [the steps](steps.md#first).\n",
		".trailguide/skills/library/redesign/steps.md": "# Steps\n",
		"docs/checklist.md":                            "Back to [Attack](../.trailguide/skills/library/attack/SKILL.md).\n",
		"changes/skills.yaml":                          "summary: Add attack and redesign.\nskills:\n  skills/attack: new\n  skills/redesign: new\n",
	})
	if _, err := Check(ctx, options); err != nil {
		t.Fatalf("check rejected links to files the library has: %v", err)
	}
	edit(t, options.Directory, map[string]string{"docs/checklist.md": "Also see [the guide](guide.md).\n"})
	_, err := Check(ctx, options)
	var failure *skillrefs.Error
	if !errors.As(err, &failure) || failure.Reason != skillrefs.Missing || failure.File != "docs/checklist.md" || failure.Target != "docs/guide.md" || strings.Join(failure.Chain, " -> ") != ".trailguide/skills/library/attack/SKILL.md -> docs/checklist.md" {
		t.Fatalf("got %v; want a missing reference reached through the checklist", err)
	}
	edit(t, options.Directory, map[string]string{"docs/guide.md": "# Guide\n", ".gitignore": "docs/guide.md\n"})
	if _, err := Check(ctx, options); !errors.As(err, &failure) || failure.Reason != skillrefs.Missing {
		t.Fatalf("got %v; want a file Git ignores reported missing, since no library release has it", err)
	}
	edit(t, options.Directory, map[string]string{".gitignore": "", "docs/checklist.md": "# Checklist\n"})
	if err := os.Symlink("steps.md", filepath.Join(options.Directory, ".trailguide/skills/library/redesign/linked.md")); err != nil {
		t.Fatal(err)
	}
	edit(t, options.Directory, map[string]string{".trailguide/skills/library/redesign/SKILL.md": "---\nname: redesign\n---\nFollow [the steps](linked.md).\n"})
	if _, err := Check(ctx, options); err == nil {
		t.Fatal("check accepted a link to a symbolic link")
	}
}
