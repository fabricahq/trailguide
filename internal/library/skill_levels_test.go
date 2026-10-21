// Test that library check accepts skill levels only for skills the library has.

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/librarytree"
)

func TestLibraryCheck_RequiresEveryLeveledSkillToExist(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	if _, err := Initialize(context.Background(), options, nil, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		file := filepath.Join(options.Directory, librarytree.Directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("skills/ship/SKILL.md", "---\nname: ship\ndescription: Ship a service.\n---\n# Ship\n")
	write("config.yaml", "schemaVersion: 2\nsources: {}\nlibrary:\n  skills:\n    required: [skills/ship]\n    recommended: [skills/review]\n")
	_, err := Check(context.Background(), options)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Location != ".trailguide/config.yaml: library.skills.recommended" || validation.Problem != "skills/review names no skill of the library, because .trailguide/skills/review/SKILL.md doesn't exist; add the skill, or remove it from the list" {
		t.Fatalf("got %v; want the missing recommended skill reported", err)
	}
	write("skills/review/SKILL.md", "---\nname: review\ndescription: Review a change.\n---\n# Review\n")
	if _, err := Check(context.Background(), options); err != nil {
		t.Fatalf("check with both skills: %v", err)
	}
}

func TestLibraryCheck_RequiresEveryDependencyToBeUsed(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	if _, err := Initialize(context.Background(), options, nil, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		file := filepath.Join(options.Directory, librarytree.Directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("skills/add-skill/SKILL.md", "---\nname: add-skill\ndescription: Add a skill.\n---\n# Add a skill\n")
	write("skills/format-skill/SKILL.md", "---\nname: format-skill\ndescription: Format a skill.\nmetadata:\n  trailguide-skills-used: |\n    skills/format-skill\n---\n# Format a skill\n")
	write("config.yaml", "schemaVersion: 2\nsources: {}\nlibrary:\n  skills:\n    recommended: [skills/add-skill]\n    dependencies: [skills/format-skill]\n")
	_, err := Check(context.Background(), options)
	want := "library.skills.dependencies lists skills/format-skill, but no skill of the library names it in trailguide-skills-used"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v; want the unused dependency reported", err)
	}
	write("skills/add-skill/SKILL.md", "---\nname: add-skill\ndescription: Add a skill.\nmetadata:\n  trailguide-skills-used: |\n    skills/format-skill\n---\n# Add a skill\n")
	write("skills/format-skill/SKILL.md", "---\nname: format-skill\ndescription: Format a skill.\n---\n# Format a skill\n")
	if _, err := Check(context.Background(), options); err != nil {
		t.Fatalf("check with the dependency used: %v", err)
	}
}
