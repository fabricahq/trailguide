// Test creating a draft skill with library add skill and project add skill, and that check and build refuse the
// draft until it is complete.

package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/terminalfixture"
)

// draftSkill is the SKILL.md frontmatter add skill writes for fix-bug with a playbook type.
const draftSkill = "---\nname: fix-bug\ndescription: 'Fix a reported bug: prove it with a regression test.'\nmetadata:\n  trailguide-skill-type: playbook\n---\n\n<!-- trailguide:draft -->\n"

func TestLibraryAddSkill_CreatesADraftThatCheckRefusesUntilComplete(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "library", "init"); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	out, diagnostic, code := runCLI(t, binary, directory, "library", "add", "skill", "fix-bug", "--description", "Fix a reported bug: prove it with a regression test.", "--type", "playbook")
	if code != 0 || !strings.Contains(out, "Skill draft created.") || !strings.Contains(out, "add skills/fix-bug to library.skills") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	file := filepath.Join(directory, ".trailguide", "skills", "fix-bug", "SKILL.md")
	data, err := os.ReadFile(file)
	if err != nil || !strings.HasPrefix(string(data), draftSkill) {
		t.Fatalf("SKILL.md %v:\n%s", err, data)
	}
	out, diagnostic, code = runCLI(t, binary, directory, "library", "check")
	if code != 1 || !strings.Contains(diagnostic, "skills/fix-bug/SKILL.md: complete the draft and remove its trailguide:draft marker") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(data), "<!-- trailguide:draft -->\n", "", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code = runCLI(t, binary, directory, "library", "check"); code != 0 {
		t.Fatalf("exit %d after completing the draft:\n%s%s", code, out, diagnostic)
	}
	out, diagnostic, code = runCLI(t, binary, directory, "library", "add", "skill", "fix-bug", "--description", "Again.")
	if code == 0 || !strings.Contains(diagnostic, "already exists") {
		t.Fatalf("exit %d overwriting a skill:\n%s%s", code, out, diagnostic)
	}
}

// TestAddSkill_RefusesInvalidInputsBeforePrompting exits 2 without writing or prompting, in a library and a project.
func TestAddSkill_RefusesInvalidInputsBeforePrompting(t *testing.T) {
	binary := buildCLI(t)
	for _, scope := range []string{"library", "project"} {
		directory := t.TempDir()
		if out, diagnostic, code := runCLI(t, binary, directory, scope, "init"); code != 0 {
			t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
		}
		before := projectFileContents(t, directory)
		for _, test := range []struct {
			args    []string
			message string
		}{
			{[]string{"Fix_Bug"}, `skill name "Fix_Bug": a skill name uses 1 to 64 lowercase letters, digits, and hyphens`},
			{[]string{"fix-bug", "--description", strings.Repeat("a", 1025)}, "--description: the description is 1025 characters; use at most 1024"},
			{[]string{"fix-bug", "--type", "design"}, `--type: the type is "design"; use playbook or review, or leave it out`},
			{[]string{"fix-bug", "--reason", "Ours."}, "--reason doesn't apply to a new skill"},
			{[]string{"fix-bug", "--from", "team:skills/fix-bug"}, "takes a skill name or --from, not both: a fork keeps the source skill's name, so leave out fix-bug."},
			{nil, "missing skill name: name the skill to create"},
			{[]string{"fix-bug", "--non-interactive"}, "--description is required"},
		} {
			// A pseudo-terminal makes every command interactive, so a prompt would show in the transcript.
			result, err := terminalfixture.Run(context.Background(), binary, directory, append([]string{scope, "add", "skill"}, test.args...), nil)
			if err != nil {
				t.Fatal(err, result)
			}
			if result.ExitCode != 2 || !strings.Contains(result.Transcript, test.message) || strings.Contains(result.Transcript, "Description (") {
				t.Errorf("%s %v: exit %d, transcript:\n%s\nwant %q", scope, test.args, result.ExitCode, result.Transcript, test.message)
			}
		}
		if after := projectFileContents(t, directory); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s: a refused skill changed files", scope)
		}
	}
}

func TestProjectAddSkill_PromptsForTheDescriptionAndBuildRefusesTheDraft(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	steps := []terminalfixture.Step{{Prompt: "Description (what the skill does, then when to use it):", Answer: "   "}, {Prompt: "Description (what the skill does, then when to use it):", Answer: "Fix a reported bug: prove it with a regression test."}}
	result, err := terminalfixture.Run(context.Background(), binary, directory, []string{"project", "add", "skill", "fix-bug", "--type", "playbook"}, steps)
	if err != nil || result.ExitCode != 0 || !strings.Contains(result.Transcript, "Adding a project-only skill at: .trailguide/skills/fix-bug/") || !strings.Contains(result.Transcript, "the description is blank") {
		t.Fatal(err, result)
	}
	data, err := os.ReadFile(filepath.Join(directory, ".trailguide", "skills", "fix-bug", "SKILL.md"))
	if err != nil || !strings.HasPrefix(string(data), draftSkill) {
		t.Fatalf("SKILL.md %v:\n%s", err, data)
	}
	before := projectFileContents(t, directory)
	for _, action := range []string{"build", "sync", "check"} {
		out, diagnostic, code := runCLI(t, binary, directory, "project", action)
		if code != 1 || !strings.Contains(diagnostic, "skills/fix-bug/SKILL.md") || !strings.Contains(diagnostic, "remove its trailguide:draft marker") {
			t.Fatalf("%s: exit %d:\n%s%s", action, code, out, diagnostic)
		}
	}
	if after := projectFileContents(t, directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a draft skill let build or sync write files")
	}
}

func TestProjectAddSkill_WarnsWhenASourcesSkillHasTheName(t *testing.T) {
	u, _ := newSkillForkFixture(t)
	out, diagnostic, code := u.run(t, "project", "add", "skill", "build-feature", "--description", "Build a feature our way.")
	if code != 0 || !strings.Contains(out, "The project installs team:skills/build-feature as build-feature too, so trailguide project sync and build fail with skill-name-clash") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	out, diagnostic, code = u.run(t, "project", "add", "skill", "release-notes", "--description", "Write release notes.")
	if code != 0 || strings.Contains(out, "skill-name-clash") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
}
