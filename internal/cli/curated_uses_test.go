// Exercise library check diagnostics and project sync warnings, in human output and JSON, when a library's skill
// uses a skill the library curates from another repository.

package cli

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// usingCurated returns a SKILL.md for the skill name whose metadata lists used as the skills it uses.
func usingCurated(name string, used ...string) string {
	return "---\nname: " + name + "\ndescription: Use " + name + ".\nmetadata:\n  trailguide-skills-used: |\n    " + strings.Join(used, "\n    ") + "\n---\n# " + name + "\n"
}

// curatingLibraryConfig curates pocock's deploy skill.
const curatingLibraryConfig = "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  formatVersion: 1\n  skills:\n    offered: [pocock:skills/ops/deploy, pocock:skills/grill]\n"

// TestLibraryCheck_ExplainsAUseOfASkillTheLibraryDoesNotCurate covers, in human and JSON output, the diagnostics for
// a skill's use of a curated skill whose source name is wrong, and for a use that leaves out the curated skill's source.
func TestLibraryCheck_ExplainsAUseOfASkillTheLibraryDoesNotCurate(t *testing.T) {
	for _, test := range []struct {
		name, used, want string
	}{
		{"misspelled source", "pocok:skills/ops/deploy", "but library.skills lists no skill from a source named pocok; the library curates skills from pocock. Correct the source name, or name a skill of this library as skills/<name>"},
		{"bare ID", "skills/grill", "; the library curates pocock:skills/grill, so name it that way"},
	} {
		t.Run(test.name, func(t *testing.T) {
			skills := map[string]string{"add-skill/SKILL.md": usingCurated("add-skill", test.used)}
			out, diagnostic, code := checkCuratingLibraryWith(t, curatingLibraryConfig, skills)
			if code == 0 || !strings.Contains(out+diagnostic, test.want) {
				t.Fatalf("exit %d, want %q:\n%s%s", code, test.want, out, diagnostic)
			}
			out, diagnostic, code = checkCuratingLibraryWith(t, curatingLibraryConfig, skills, "--json")
			if code == 0 || !strings.Contains(jsonError(t, out), test.want) {
				t.Fatalf("exit %d, want %q in JSON:\n%s%s", code, test.want, out, diagnostic)
			}
		})
	}
}

// TestLibraryCheck_ExplainsAUseOfACuratedSkillTheLibraryDoesNotList covers, in human and JSON output, a skill's use
// of a skill from a source the library curates from, which library.skills doesn't list.
func TestLibraryCheck_ExplainsAUseOfACuratedSkillTheLibraryDoesNotList(t *testing.T) {
	skills := map[string]string{"add-skill/SKILL.md": usingCurated("add-skill", "pocock:skills/team/review")}
	want := "which isn't a skill the library curates; list it in library.skills, at a level or as a dependency, so projects that install the skill can install it too"
	out, diagnostic, code := checkCuratingLibraryWith(t, curatingLibraryConfig, skills)
	if code == 0 || !strings.Contains(out+diagnostic, want) {
		t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
	}
	out, diagnostic, code = checkCuratingLibraryWith(t, curatingLibraryConfig, skills, "--json")
	if code == 0 || !strings.Contains(jsonError(t, out), want) {
		t.Fatalf("exit %d, want %q in JSON:\n%s%s", code, want, out, diagnostic)
	}
}

// TestLibraryCheck_AcceptsAUseOfACuratedSkill accepts a skill's use of a skill the library curates.
func TestLibraryCheck_AcceptsAUseOfACuratedSkill(t *testing.T) {
	skills := map[string]string{"add-skill/SKILL.md": usingCurated("add-skill", "pocock:skills/ops/deploy")}
	if out, diagnostic, code := checkCuratingLibraryWith(t, curatingLibraryConfig, skills); code != 0 || !strings.Contains(out, "Library is valid") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
}

// jsonError returns the error message of a failed command's JSON output.
func jsonError(t *testing.T, out string) string {
	t.Helper()
	var result struct {
		OK    bool
		Error struct{ Message string }
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.OK {
		t.Fatalf("want a failed result, %v:\n%s", err, out)
	}
	return result.Error.Message
}

// TestSync_WarnsWhenTheProjectExcludesACuratedSkillALibrarysSkillUses syncs, in human and JSON output, a project
// that installs a library's skill and excludes the curated skill it uses, and warns that the use isn't installed.
func TestSync_WarnsWhenTheProjectExcludesACuratedSkillALibrarysSkillUses(t *testing.T) {
	ctx := context.Background()
	pocock, err := gitfixture.New(ctx, map[string][]byte{
		"skills/ops/deploy/SKILL.md": []byte("---\nname: deploy\ndescription: Deploy a service.\n---\n# Deploy\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pocock.Close() })
	if _, err := pocock.Command(ctx, "tag", "stable", pocock.FirstCommit); err != nil {
		t.Fatal(err)
	}
	team, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                           []byte(curatingLibraryConfig),
		".trailguide/skills/add-skill/SKILL.md": []byte(usingCurated("add-skill", "pocock:skills/ops/deploy")),
		"techs/go/_group.yaml":                  []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":                    updateRule("errors 1.0.0"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = team.Close() })
	if err := team.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	if team.Environment, err = team.Route(map[string]*gitfixture.Fixture{"team": team, "pocock": pocock}); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: team}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init", "--skill-targets", "claude"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: git@fixture.invalid:team\n    ref: release/1\n    skills:\n      include: [skills/add-skill]\n      exclude:\n        - id: pocock:skills/ops/deploy\n          reason: Not wanted.\n")
	want := "team:skills/add-skill uses team:pocock:skills/ops/deploy, which the project excludes without a replacement, so the skill it uses isn't installed. Stop excluding the skill, or remove the use from the skill that uses it, which you can fork. A curated skill can't be replaced."
	out, diagnostic, code := u.run(t, "project", "sync")
	if code != 0 || !strings.Contains(out+diagnostic, "Warning: "+want) {
		t.Fatalf("exit %d, want the warning %q:\n%s%s", code, want, out, diagnostic)
	}
	out, diagnostic, code = u.run(t, "project", "sync", "--json")
	var result struct {
		OK    bool
		Value struct{ Warnings []string }
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || !result.OK || !slices.Contains(result.Value.Warnings, want) {
		t.Fatalf("exit %d, %v, want the warning %q:\n%s%s", code, err, want, out, diagnostic)
	}
}
