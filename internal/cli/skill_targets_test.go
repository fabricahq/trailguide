// Verify project init writes, keeps, and replaces the configuration's skill targets, and sync installs a source's
// skills into them.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

func TestInitSetsSkillTargets(t *testing.T) {
	binary := buildCLI(t)
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"default", nil, "skillTargets: [claude, agents]\n"},
		{"one target", []string{"--skill-targets", "claude"}, "skillTargets: [claude]\n"},
		{"none", []string{"--skill-targets", "none"}, "skillTargets: []\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if out, diagnostic, code := runCLI(t, binary, directory, append([]string{"project", "init"}, test.args...)...); code != 0 {
				t.Fatal(code, out, diagnostic)
			}
			data, err := os.ReadFile(filepath.Join(directory, ".trailguide/config.yaml"))
			if err != nil || string(data) != "schemaVersion: 2\n"+test.want+"sources: {}\n" {
				t.Fatalf("config.yaml:\n%s %v", data, err)
			}
		})
	}
}

func TestInitReplacesSkillTargetsOnlyWhenAsked(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	config := filepath.Join(directory, ".trailguide/config.yaml")
	for _, step := range []struct {
		args []string
		want string
	}{
		{nil, "skillTargets: [claude, agents]"},
		{[]string{"--skill-targets", "agents"}, "skillTargets: [agents]"},
		{nil, "skillTargets: [agents]"},
		{[]string{"--skill-targets", "none"}, "skillTargets: []"},
	} {
		if out, diagnostic, code := runCLI(t, binary, directory, append([]string{"project", "init"}, step.args...)...); code != 0 {
			t.Fatal(code, out, diagnostic)
		}
		data, err := os.ReadFile(config)
		if err != nil || !strings.Contains(string(data), step.want+"\n") {
			t.Fatalf("after init %v, config.yaml:\n%s %v", step.args, data, err)
		}
	}
}

func TestInitRejectsInvalidSkillTargets(t *testing.T) {
	binary := buildCLI(t)
	for name, args := range map[string][]string{
		"unknown":         {"--skill-targets", "cursor"},
		"none and claude": {"--skill-targets", "none", "--skill-targets", "claude"},
		"repeated":        {"--skill-targets", "claude", "--skill-targets", "claude"},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			out, diagnostic, code := runCLI(t, binary, directory, append([]string{"project", "init"}, args...)...)
			if code != 2 || !strings.Contains(diagnostic, "--skill-targets") {
				t.Fatal(code, out, diagnostic)
			}
			if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
				t.Fatal("rejected init wrote files", entries, err)
			}
		})
	}
}

func TestSync_InstallsTheSkillsOfAnAddedSource(t *testing.T) {
	binary := buildCLI(t)
	f, err := gitfixture.New(context.Background(), map[string][]byte{"tools/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	for _, test := range []struct {
		targets, warning string
		added            []string
	}{
		{"claude", "", []string{"../.claude/skills/review/SKILL.md", "generated/SKILLS.md", "vendor/tools/tools/review/SKILL.md"}},
		{"none", "The project has no skill targets, so no copy of its 1 skill(s) was installed.", []string{"generated/SKILLS.md", "vendor/tools/tools/review/SKILL.md"}},
	} {
		t.Run(test.targets, func(t *testing.T) {
			directory := t.TempDir()
			for _, args := range [][]string{{"project", "init", "--skill-targets", test.targets}, {"project", "add", "source", "tools", "--repository", f.Repository, "--ref", "v1.0.0", "--skills", "*", "--non-interactive"}} {
				if out, diagnostic, code := runCLI(t, binary, directory, args...); code != 0 {
					t.Fatal(args, code, out, diagnostic)
				}
			}
			out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync", "--json")
			var result struct {
				OK    bool
				Value struct{ Added, Warnings []string }
			}
			if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.OK {
				t.Fatal(code, out, diagnostic)
			}
			for _, name := range test.added {
				if !slices.Contains(result.Value.Added, name) {
					t.Errorf("added %v lacks %s", result.Value.Added, name)
				}
			}
			if test.warning == "" && len(result.Value.Warnings) != 0 || test.warning != "" && (len(result.Value.Warnings) != 1 || !strings.HasPrefix(result.Value.Warnings[0], test.warning)) {
				t.Errorf("warnings = %v", result.Value.Warnings)
			}
			_, err := os.Stat(filepath.Join(directory, ".claude/skills/review/SKILL.md"))
			if installed := err == nil; installed != (test.targets == "claude") {
				t.Errorf("installed copy exists: %v", installed)
			}
		})
	}
}

func TestCheckNamesAnEditedInstalledSkillAndItsRepair(t *testing.T) {
	binary := buildCLI(t)
	f, err := gitfixture.New(context.Background(), map[string][]byte{"tools/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	directory := t.TempDir()
	for _, args := range [][]string{{"project", "init", "--skill-targets", "claude"}, {"project", "add", "source", "tools", "--repository", f.Repository, "--ref", "v1.0.0", "--skills", "*", "--non-interactive"}} {
		if out, diagnostic, code := runCLI(t, binary, directory, args...); code != 0 {
			t.Fatal(args, code, out, diagnostic)
		}
	}
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	installed := filepath.Join(directory, ".claude/skills/review/SKILL.md")
	if err := os.WriteFile(installed, []byte("# Edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := runCLI(t, binary, directory, "project", "check", "--json")
	var result struct {
		OK    bool
		Value projectCheckResult
		Error responseError
	}
	if code != 1 || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || result.OK || result.Error.Kind != "out-of-date" || len(result.Value.Problems) != 1 {
		t.Fatal(code, out, diagnostic)
	}
	problem := result.Value.Problems[0]
	if problem.Kind != "modified-skill" || problem.Path != "../.claude/skills/review" || problem.Skill != "tools:tools/review" || problem.Message != "Installed skill tools:tools/review differs from its source; don't edit installed copies, since build replaces them" || repairCommand(t, problem) != "trailguide project build" {
		t.Fatalf("problem = %+v", problem)
	}
	human, diagnostic, code := runCLI(t, binary, directory, "project", "check")
	if want := "  " + problem.Message + ": ../.claude/skills/review\n    Next: trailguide project build\n"; code != 1 || diagnostic != "" || !strings.Contains(human, want) {
		t.Fatalf("human output:\n%s\nwant\n%s", human, want)
	}
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "build"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	if data, err := os.ReadFile(installed); err != nil || !strings.Contains(string(data), "# Review") {
		t.Fatalf("build didn't reinstall the skill: %s %v", data, err)
	}
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "check"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
}

func TestBuildReportsInvalidFrontmatterYAMLAtItsLine(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	skill := filepath.Join(directory, ".trailguide/skills/fix-bug/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: fix-bug\ndescription: Fix a bug: reproduce it first.\n---\n# Fix\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const problem = "invalid YAML at line 3, column 23: mapping values are not allowed in this context; quote a value that contains a colon followed by a space"
	out, diagnostic, code := runCLI(t, binary, directory, "project", "build")
	if code != 1 || !strings.Contains(diagnostic, problem) || !strings.Contains(diagnostic, "Location: skills/fix-bug/SKILL.md") {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code = runCLI(t, binary, directory, "project", "build", "--json")
	var result struct {
		OK    bool
		Error responseError
	}
	if code != 1 || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || result.OK || result.Error.Kind != "validation" || result.Error.Location != "skills/fix-bug/SKILL.md" || !strings.Contains(result.Error.Message, problem) {
		t.Fatal(code, out, diagnostic)
	}
}

func TestBuildReportsAnInvalidSkillTypeAtTheSkill(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	skill := filepath.Join(directory, ".trailguide/skills/fix-bug/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: fix-bug\nmetadata:\n  trailguide-skill-type: workflow\n---\n# Fix\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"build", "check"} {
		out, diagnostic, code := runCLI(t, binary, directory, "project", command, "--json")
		var result struct {
			OK    bool
			Error responseError
		}
		if code != 1 || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || result.OK || result.Error.Kind != "validation" || result.Error.Location != "skills/fix-bug/SKILL.md" || !strings.Contains(result.Error.Message, `trailguide-skill-type is "workflow"; use playbook or review, or leave it out`) {
			t.Fatal(command, code, out, diagnostic)
		}
	}
	if _, err := os.Stat(filepath.Join(directory, ".claude/skills/fix-bug")); err == nil {
		t.Fatal("build installed an invalid skill")
	}
}

func TestSync_NamesTheSkillAndTheUsedSkillItCannotFind(t *testing.T) {
	binary := buildCLI(t)
	f, err := gitfixture.New(context.Background(), map[string][]byte{"skills/architect/SKILL.md": []byte("---\nname: architect\nmetadata:\n  trailguide-skills-used: |\n    skills/how\n---\n# Architect\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	directory := t.TempDir()
	for _, args := range [][]string{{"project", "init", "--skill-targets", "claude"}, {"project", "add", "source", "pstack", "--repository", f.Repository, "--ref", "v1.0.0", "--skills", "skills/architect", "--non-interactive"}} {
		if out, diagnostic, code := runCLI(t, binary, directory, args...); code != 0 {
			t.Fatal(args, code, out, diagnostic)
		}
	}
	problem := "skills/architect uses skills/how, which isn't a skill in commit " + f.FirstCommit + ": skills/architect names it in trailguide-skills-used."
	out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync", "--json")
	var result struct {
		OK    bool
		Error responseError
	}
	if code != 1 || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || result.OK || result.Error.Kind != "validation" || result.Error.Location != "sources.pstack.skills" || !strings.Contains(result.Error.Message, problem) {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync")
	if code != 1 || !strings.Contains(diagnostic, "Error: "+problem) || !strings.Contains(diagnostic, "Location: sources.pstack.skills\n") {
		t.Fatal(code, out, diagnostic)
	}
	if _, err := os.Stat(filepath.Join(directory, ".claude")); err == nil {
		t.Fatal("failed sync installed skills")
	}
}
