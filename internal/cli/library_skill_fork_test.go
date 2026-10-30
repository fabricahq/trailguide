// Test forking a skill into a library through the CLI, and the source changes library check lists for it.

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// forkingLibrary returns a library whose library.skills lists listed, YAML indented under it, from pocock at tag
// stable, whose release skill uses deploy, rollback uses a skill it lacks, and ratchet's frontmatter isn't valid YAML, with the pocock fixture and the
// environment that routes to it.
func forkingLibrary(t *testing.T, listed string) (string, *gitfixture.Fixture, []string) {
	t.Helper()
	ctx := context.Background()
	pocock, err := gitfixture.New(ctx, map[string][]byte{
		"skills/ops/deploy/SKILL.md":          []byte("---\nname: deploy\ndescription: Deploy a service.\n---\n# Deploy\n"),
		"skills/ops/deploy/scripts/deploy.sh": []byte("#!/bin/sh\necho deploy\n"),
		"skills/ops/release/SKILL.md":         []byte("---\nname: release\ndescription: Release a service.\nmetadata:\n  trailguide-skills-used: |\n    skills/ops/deploy\n---\n# Release\n"),
		"skills/ops/ratchet/SKILL.md":         []byte("---\nname: ratchet\ndescription: Ratchet file size so each file stays small: run it before commits.\n---\n# Ratchet\n"),
		"skills/ops/rollback/SKILL.md":        []byte("---\nname: rollback\ndescription: Roll back a release.\nmetadata:\n  trailguide-skills-used: |\n    skills/ops/missing\n---\n# Roll back\n"),
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
	config := "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  schemaVersion: 1\n  skills:\n" + listed
	file := filepath.Join(directory, ".trailguide", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return directory, pocock, environment
}

func TestLibraryAddSkill_ForksACuratedSkillAndCheckListsTheSourcesChanges(t *testing.T) {
	ctx := context.Background()
	directory, pocock, environment := forkingLibrary(t, "    required: [pocock:skills/ops/deploy]\n")
	binary := buildCLI(t)
	out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/deploy", "--reason", "Deploy with our pipeline.")
	if code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	skill, err := os.ReadFile(filepath.Join(directory, ".trailguide", "skills", "deploy", "SKILL.md"))
	if err != nil || !strings.Contains(string(skill), "# Deploy") {
		t.Fatalf("forked SKILL.md: %q, %v", skill, err)
	}
	if _, err := os.Stat(filepath.Join(directory, ".trailguide", "skills", "deploy", "scripts", "deploy.sh")); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(directory, ".trailguide", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"forks:\n        skills/ops/deploy:\n          as: skills/deploy", "basedOn: \"" + pocock.FirstCommit + "\"", "reason: Deploy with our pipeline.", "required: [skills/deploy]"} {
		if !strings.Contains(string(config), want) {
			t.Fatalf("config lacks %q:\n%s", want, config)
		}
	}
	if strings.Contains(string(config), "include:") || strings.Contains(string(config), "exclude:") {
		t.Fatalf("the fork needs no include or exclude entry:\n%s", config)
	}

	out, diagnostic, code = runCLIWithEnvironment(t, binary, directory, environment, "library", "check")
	if code != 0 || strings.Contains(out, "Forked skills") {
		t.Fatalf("exit %d before the source changed:\n%s%s", code, out, diagnostic)
	}

	changed, err := pocock.Commit(ctx, pocock.Worktree(), "Change deploy", map[string][]byte{"skills/ops/deploy/scripts/deploy.sh": []byte("#!/bin/sh\necho deploy v2\n")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pocock.Command(ctx, "tag", "--force", "stable", changed); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, directory, environment, "library", "check")
	if code != 0 || !strings.Contains(out, "Forked skills that their sources changed:") || !strings.Contains(out, "scripts/deploy.sh") {
		t.Fatalf("exit %d after the source changed:\n%s%s", code, out, diagnostic)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, directory, environment, "library", "check", "--json")
	for _, want := range []string{`"forkedSkills": [`, `"skill": "skills/deploy"`, `"basedOn": "` + pocock.FirstCommit + `"`, `"commit": "` + changed + `"`} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("exit %d, want %s:\n%s%s", code, want, out, diagnostic)
		}
	}

	deleted, err := pocock.Commit(ctx, pocock.Worktree(), "Delete deploy", map[string][]byte{"skills/ops/deploy/SKILL.md": nil, "skills/ops/deploy/scripts/deploy.sh": nil})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pocock.Command(ctx, "tag", "--force", "stable", deleted); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, directory, environment, "library", "check")
	if code != 0 || !strings.Contains(out, "pocock no longer has the skill at stable") || !strings.Contains(out, "Your fork stays the library's own skill.") {
		t.Fatalf("exit %d after the source deleted the skill:\n%s%s", code, out, diagnostic)
	}
}

func TestLibraryAddSkill_ForksASkillItCantAdaptAsADraftToFinish(t *testing.T) {
	directory, pocock, environment := forkingLibrary(t, "    offered: [pocock:skills/ops/ratchet]\n")
	binary := buildCLI(t)
	out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/ratchet", "--reason", "Ours.")
	for _, want := range []string{
		"Finish the fork: Trail Guide couldn't finish adapting the skill, so it marked the fork's",
		"Until you finish it, library check fails and projects can't install the fork.\nIn the fork:\n",
		"  - Correct its frontmatter: ",
		"  - Name the skill ratchet, and name the skills it uses with the source's name, such as pocock:skills/tdd.",
		"Then remove the <!-- trailguide:draft --> line.",
		"library.skills now lists skills/ratchet in place of pocock:skills/ops/ratchet",
	} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
		}
	}
	file := filepath.Join(directory, ".trailguide", "skills", "ratchet", "SKILL.md")
	skill, err := os.ReadFile(file)
	if want := "description: Ratchet file size so each file stays small: run it before commits.\n---\n<!-- trailguide:draft -->\n# Ratchet\n"; err != nil || !strings.HasSuffix(string(skill), want) {
		t.Fatalf("forked SKILL.md:\n%s, %v; want the source's, marked as a draft", skill, err)
	}
	config, err := os.ReadFile(filepath.Join(directory, ".trailguide", "config.yaml"))
	if err != nil || !strings.Contains(string(config), "as: skills/ratchet") || !strings.Contains(string(config), "basedOn: \""+pocock.FirstCommit+"\"") {
		t.Fatalf("config.yaml:\n%s, %v; want the fork recorded", config, err)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "check"); code == 0 || !strings.Contains(out+diagnostic, "skills/ratchet/SKILL.md") {
		t.Fatalf("library check passed an unfinished fork: exit %d:\n%s%s", code, out, diagnostic)
	}

	if err := os.WriteFile(file, []byte("---\nname: ratchet\ndescription: \"Ratchet file size so each file stays small: run it before commits.\"\n---\n# Ratchet\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "check"); code != 0 {
		t.Fatalf("library check refused the finished fork: exit %d:\n%s%s", code, out, diagnostic)
	}
}

func TestLibraryAddSkill_AsksADraftForkToListTheSkillsTheSourcesUsesAdds(t *testing.T) {
	directory, _, environment := forkingLibrary(t, "    offered: [pocock:skills/ops/deploy]\n")
	file := filepath.Join(directory, ".trailguide", "config.yaml")
	config, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	config = []byte(strings.Replace(string(config), "    ref: stable\n", "    ref: stable\n    skills:\n      include: [skills/ops/ratchet]\n      uses:\n        skills/ops/ratchet: [skills/ops/deploy]\n", 1))
	if err := os.WriteFile(file, config, 0600); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, buildCLI(t), directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/ratchet", "--reason", "Ours.")
	if want := "List these in trailguide-skills-used too, since sources.pocock.skills.uses adds them for the skill: pocock:skills/ops/deploy."; code != 0 || !strings.Contains(out, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}

func TestLibraryAddSkill_RefusesASkillTheSourceLacks(t *testing.T) {
	directory, _, environment := forkingLibrary(t, "    required: [pocock:skills/ops/deploy]\n")
	out, diagnostic, code := runCLIWithEnvironment(t, buildCLI(t), directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/grill", "--reason", "Ours.")
	if code == 0 || !strings.Contains(out+diagnostic, "source pocock has no skill skills/ops/grill at stable") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
}

func TestLibraryCheck_AsksToForkTheCuratedSkillThatUsesAFork(t *testing.T) {
	directory, _, environment := forkingLibrary(t, "    recommended: [pocock:skills/ops/deploy, pocock:skills/ops/release]\n")
	binary := buildCLI(t)
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/deploy", "--reason", "Ours."); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "check")
	want := "pocock:skills/ops/release uses pocock:skills/ops/deploy, which projects install with it as deploy beside your fork skills/deploy; fork pocock:skills/ops/release too"
	if code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/release", "--reason", "Ours."); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	if skill, err := os.ReadFile(filepath.Join(directory, ".trailguide", "skills", "release", "SKILL.md")); err != nil || !strings.Contains(string(skill), "trailguide-skills-used: |\n    skills/deploy\n") {
		t.Fatalf("forked release:\n%s, %v; want it to use the library's fork of deploy", skill, err)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "check"); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
}

func TestLibraryAddSkill_NamesTheCuratedSkillsAForkUses(t *testing.T) {
	directory, _, environment := forkingLibrary(t, "    recommended: [pocock:skills/ops/deploy, pocock:skills/ops/release]\n")
	binary := buildCLI(t)
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/release", "--reason", "Ours."); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	if skill, err := os.ReadFile(filepath.Join(directory, ".trailguide", "skills", "release", "SKILL.md")); err != nil || !strings.Contains(string(skill), "trailguide-skills-used: |\n    pocock:skills/ops/deploy\n") {
		t.Fatalf("forked release:\n%s, %v; want it to use the curated deploy", skill, err)
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, environment, "library", "check"); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
}

func TestLibraryAddSkill_AsksToListOrForkTheSkillsAForkUses(t *testing.T) {
	directory, _, environment := forkingLibrary(t, "    recommended: [pocock:skills/ops/release]\n")
	out, diagnostic, code := runCLIWithEnvironment(t, buildCLI(t), directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/release", "--reason", "Ours.")
	if want := "the skill uses pocock:skills/ops/deploy, which the library neither forked nor lists"; code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}

func TestLibraryCheck_RejectsACuratedSkillThatUsesOneItsSourceLacks(t *testing.T) {
	directory, _, environment := forkingLibrary(t, "    offered: [pocock:skills/ops/rollback]\n")
	out, diagnostic, code := runCLIWithEnvironment(t, buildCLI(t), directory, environment, "library", "check")
	if want := "pocock:skills/ops/rollback uses pocock:skills/ops/missing, which pocock doesn't have at stable"; code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}

func TestLibraryAddSkill_ReadsTheSkillsTheSourcesUsesAdds(t *testing.T) {
	directory, _, environment := forkingLibrary(t, "    recommended: [pocock:skills/ops/deploy]\n")
	file := filepath.Join(directory, ".trailguide", "config.yaml")
	config, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	config = []byte(strings.Replace(string(config), "    ref: stable\n", "    ref: stable\n    skills:\n      include: [skills/ops/deploy]\n      uses:\n        skills/ops/deploy: [skills/ops/release]\n", 1))
	if err := os.WriteFile(file, config, 0600); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, buildCLI(t), directory, environment, "library", "add", "skill", "--from", "pocock:skills/ops/deploy", "--reason", "Ours.")
	if want := "the skill uses pocock:skills/ops/release, which the library neither forked nor lists"; code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
}
