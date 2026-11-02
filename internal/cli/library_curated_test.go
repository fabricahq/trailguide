// Exercise how trailguide library check reads the repositories a library curates skills from, at the refs it declares.

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// checkCuratingLibrary runs library check in a library outside Git whose configuration is config, with an own
// skill, skills/review, and pocock, a repository that isn't a library, with the skills skills/ops/deploy,
// skills/team/review, and skills/ops/ship, whose SKILL.md names it review, tagged stable.
func checkCuratingLibrary(t *testing.T, config string) (string, string, int) {
	t.Helper()
	return checkCuratingLibraryWith(t, config, nil)
}

// checkCuratingLibraryWith runs library check, with args, on a library whose config is config and whose authored
// skills include skills, by path under .trailguide/skills/library/, besides review.
func checkCuratingLibraryWith(t *testing.T, config string, skills map[string]string, args ...string) (string, string, int) {
	t.Helper()
	ctx := context.Background()
	pocock, err := gitfixture.New(ctx, map[string][]byte{
		"skills/ops/deploy/SKILL.md":  []byte("---\nname: deploy\ndescription: Deploy a service.\n---\n# Deploy\n"),
		"skills/team/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n"),
		"skills/ops/ship/SKILL.md":    []byte("---\nname: review\ndescription: Ship and review a change.\n---\n# Ship\n"),
		"skills/grill/SKILL.md":       []byte("---\nname: grill\ndescription: Grill a plan.\n---\n# Grill\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pocock.Close() })
	if _, err := pocock.Command(ctx, "tag", "stable", pocock.FirstCommit); err != nil {
		t.Fatal(err)
	}
	environment, err := pocock.Route(map[string]*gitfixture.Fixture{"pocock": pocock})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	files := map[string]string{
		".trailguide/config.yaml":                    config,
		".trailguide/skills/library/review/SKILL.md": "---\nname: review\ndescription: Review a pull request.\n---\n# Review\n",
	}
	for name, text := range skills {
		files[".trailguide/skills/library/"+name] = text
	}
	for name, text := range files {
		file := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return runCLIWithEnvironment(t, buildCLI(t), directory, environment, append([]string{"library", "check"}, args...)...)
}

func TestLibraryCheck_AcceptsCuratedSkillsTheirSourceHas(t *testing.T) {
	out, diagnostic, code := checkCuratingLibrary(t, "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills:\n    required: [pocock:skills/ops/deploy]\n")
	if code != 0 || !strings.Contains(out, "Library is valid") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
}

func TestLibraryCheck_AcceptsCuratedSkillsFromABranch(t *testing.T) {
	for _, ref := range []string{"main", "refs/heads/main"} {
		out, diagnostic, code := checkCuratingLibrary(t, "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: "+ref+"\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills:\n    required: [pocock:skills/ops/deploy]\n")
		if code != 0 || !strings.Contains(out, "Library is valid") {
			t.Fatalf("%s: exit %d:\n%s%s", ref, code, out, diagnostic)
		}
	}
}

func TestLibraryCheck_NamesACuratedBranchItCannotResolve(t *testing.T) {
	out, diagnostic, code := checkCuratingLibrary(t, "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: trunk\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills:\n    required: [pocock:skills/ops/deploy]\n")
	want := "The library curates skills from its source pocock at ref trunk, which the repository doesn't resolve: the repository has no branch or tag trunk; check the ref, and whether a branch of that name was renamed or deleted. Fix sources.pocock.ref in .trailguide/config.yaml."
	if code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}

func TestLibraryCheck_RejectsACuratedSkillItsSourceLacks(t *testing.T) {
	out, diagnostic, code := checkCuratingLibrary(t, "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills:\n    offered: [pocock:skills/ops/grill]\n")
	want := "library.skills.offered names pocock:skills/ops/grill, which pocock doesn't have at stable; check the skill ID, or set sources.pocock.ref to a revision that has it"
	if code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}

func TestLibraryCheck_RejectsACuratedSkillWithTheNameOfItsOwn(t *testing.T) {
	out, diagnostic, code := checkCuratingLibrary(t, "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills:\n    recommended: [pocock:skills/team/review]\n")
	want := "library.skills lists skills/review and pocock:skills/team/review, which both install as review; list one of them, or rename the folder of your own skill"
	if code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}

func TestLibraryCheck_RejectsACuratedSkillThatInstallsWithTheNameOfItsOwn(t *testing.T) {
	out, diagnostic, code := checkCuratingLibrary(t, "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills:\n    offered: [pocock:skills/ops/ship]\n")
	want := "library.skills lists skills/review and pocock:skills/ops/ship, which both install as review"
	if code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}

func TestLibraryCheck_RequiresEveryCuratedDependencyToBeUsed(t *testing.T) {
	ctx := context.Background()
	pocock, err := gitfixture.New(ctx, map[string][]byte{
		"skills/spec/SKILL.md":      []byte("---\nname: spec\ndescription: Write a spec.\nmetadata:\n  trailguide-skills-used: |\n    skills/interview\n---\n# Spec\n"),
		"skills/interview/SKILL.md": []byte("---\nname: interview\ndescription: Interview the user.\n---\n# Interview\n"),
		"skills/grill/SKILL.md":     []byte("---\nname: grill\ndescription: Grill a plan.\n---\n# Grill\n"),
		"skills/unused/SKILL.md":    []byte("---\nname: unused\ndescription: Do nothing.\n---\n# Unused\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pocock.Close() })
	environment, err := pocock.Route(map[string]*gitfixture.Fixture{"pocock": pocock})
	if err != nil {
		t.Fatal(err)
	}
	check := func(dependencies string) (string, string, int) {
		t.Helper()
		directory := t.TempDir()
		files := map[string]string{
			".trailguide/config.yaml":                       "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: main\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills:\n    offered: [skills/add-skill, pocock:skills/spec]\n    dependencies: [" + dependencies + "]\n",
			".trailguide/skills/library/add-skill/SKILL.md": "---\nname: add-skill\ndescription: Add a skill.\nmetadata:\n  trailguide-skills-used: |\n    pocock:skills/grill\n---\n# Add a skill\n",
		}
		for name, text := range files {
			file := filepath.Join(directory, name)
			if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return runCLIWithEnvironment(t, buildCLI(t), directory, environment, "library", "check")
	}
	// add-skill uses grill, and the curated spec uses interview in its own repository.
	if out, diagnostic, code := check("pocock:skills/grill, pocock:skills/interview"); code != 0 || !strings.Contains(out, "Library is valid") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	out, diagnostic, code := check("pocock:skills/grill, pocock:skills/unused")
	if want := "library.skills.dependencies lists pocock:skills/unused, but no skill of the library names it"; code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d; want %q:\n%s%s", code, want, out, diagnostic)
	}
}
