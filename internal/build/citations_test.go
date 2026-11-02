// Test how generation resolves the rules a skill cites and links each installed copy to the project's version.

package build

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/library"
)

// citingSkill returns an authored skill, skills/fix, whose trailguide-rules-used lists rulesUsed.
func citingSkill(rulesUsed ...string) map[string]SkillFiles {
	text := "---\nname: fix\ndescription: Fix a bug.\nmetadata:\n  trailguide-rules-used: |\n    " + strings.Join(rulesUsed, "\n    ") + "\n---\n# Fix\n"
	return map[string]SkillFiles{"skills/fix": {Files: map[string][]byte{"SKILL.md": []byte(text)}, Executable: []string{}}}
}

// generateCiting generates the fixture's project, with exclude as its source's exclusions, local as its authored
// rules, and skills as its authored skills, installing into .claude/skills/. alter, when not nil, changes the
// source's snapshot first.
func generateCiting(t *testing.T, exclude string, local map[string][]byte, skills map[string]SkillFiles, alter func(*library.Snapshot)) (Output, error) {
	t.Helper()
	config, libraries := fixture(t, exclude)
	config.SkillTargets = []string{"claude"}
	if alter != nil {
		team := libraries["team"]
		alter(&team.Snapshot)
		libraries["team"] = team
	}
	return Generate(config, libraries, local, Options{LocalSkills: locate(config, skills)})
}

func TestGenerateLinksAnInstalledSkillToEachRuleItCites(t *testing.T) {
	custom := map[string][]byte{"techs/go/custom.md": []byte(document)}
	link := func(file string) string {
		return "[.trailguide/_managed/generated/" + file + "](../../../.trailguide/_managed/generated/" + file + ")"
	}
	tests := map[string]struct {
		exclude, cited string
		local          map[string][]byte
		retire         bool
		want           string
	}{
		"included rule":       {`[]`, "team:techs/go/errors", nil, false, "- `team:techs/go/errors` 1.0.0: " + link("rules/team/techs/go/errors.md")},
		"authored rule":       {`[]`, "techs/go/custom", custom, false, "- `local:techs/go/custom`: " + link("rules/local/techs/go/custom.md")},
		"replaced rule":       {`[{"id":"techs/go/errors","reason":"Ours","replacedBy":"techs/go/custom"}]`, "team:techs/go/errors", custom, false, "- `local:techs/go/custom`, which replaces `team:techs/go/errors`: " + link("rules/local/techs/go/custom.md")},
		"excluded rule":       {`[{"id":"techs/go/*","reason":"We *log* instead"}]`, "team:techs/go/errors", nil, false, "- `team:techs/go/errors`: excluded by this project: We \\*log\\* instead"},
		"rule not included":   {`[]`, "team:practices/testing/verify", nil, false, "- `team:practices/testing/verify`: not included by this project, so it has no version of it"},
		"retired rule":        {`[]`, "team:techs/go/old", nil, true, "- `team:techs/go/old`: retired by its library, so the project has no version of it"},
		"pinned retired rule": {`[]`, "team:techs/go/errors", nil, true, "- `team:techs/go/errors` 1.0.0: " + link("rules/team/techs/go/errors.md")},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := generateCiting(t, test.exclude, test.local, citingSkill(test.cited), func(snapshot *library.Snapshot) {
				if test.retire {
					snapshot.RetiredRules = []string{strings.TrimPrefix(test.cited, "team:")}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			installed := string(output.Skills[".claude/skills/fix"].Files["SKILL.md"])
			if want := "# Fix\n\n## Rules this skill applies\n\n"; !strings.Contains(installed, want) || !strings.HasSuffix(installed, test.want+"\n") {
				t.Errorf("installed SKILL.md:\n%s\nwant it to end with the section and\n%s", installed, test.want)
			}
			if test.retire != (len(output.RetiredCitations) == 1) {
				t.Errorf("retired citations = %v", output.RetiredCitations)
			}
			if !strings.Contains(string(output.Files["SKILLS.md"]), "- **Applies:** 1 rule(s), listed at the end of its installed SKILL.md") {
				t.Errorf("SKILLS.md:\n%s", output.Files["SKILLS.md"])
			}
		})
	}
}

func TestGenerateReportsARetiredRuleAnAuthoredSkillCitesWithoutFailing(t *testing.T) {
	output, err := generateCiting(t, `[]`, nil, citingSkill("team:techs/go/old"), func(snapshot *library.Snapshot) {
		snapshot.RetiredRules = []string{"techs/go/old"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.RetiredCitations) != 1 || output.RetiredCitations[0].Error() != "skills/local/fix/SKILL.md: trailguide-rules-used names team:techs/go/old, which its library retired; remove it from trailguide-rules-used, or cite the rule that replaces it" {
		t.Fatalf("retired citations = %v", output.RetiredCitations)
	}
}

func TestGenerateKeepsCitedRulesOutOfTheRuleIndexes(t *testing.T) {
	without, err := generateCiting(t, `[]`, nil, citingSkill("team:techs/go/errors"), nil)
	if err != nil {
		t.Fatal(err)
	}
	with, err := generateCiting(t, `[]`, nil, citingSkill("team:techs/go/errors", "team:techs/rust/other", "team:practices/testing/verify"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"RULES.md", "groups/techs/go.md", "rules/team/techs/go/errors.md"} {
		if string(without.Files[name]) != string(with.Files[name]) {
			t.Errorf("citing more rules changed %s:\n%s", name, with.Files[name])
		}
	}
	for name := range with.Files {
		if strings.Contains(name, "other") || strings.Contains(name, "practices") {
			t.Errorf("a rule the project doesn't include has generated file %s", name)
		}
	}
}

func TestGenerateLinksALibrarySkillToTheRulesItCitesInItsLibrary(t *testing.T) {
	output, err := generateCiting(t, `[]`, nil, nil, func(snapshot *library.Snapshot) {
		snapshot.Skills = map[string]library.ImportedSkill{"skills/review": {Level: "offered"}}
		snapshot.Files = map[string][]byte{"skills/review/SKILL.md": []byte("---\nname: review\ndescription: Review.\nmetadata:\n  trailguide-skill-type: audit\n  trailguide-rules-used: |\n    techs/go/errors\n    techs/go/old\n---\n# Review\n")}
		snapshot.RetiredRules = []string{"techs/go/old"}
	})
	if err != nil {
		t.Fatal(err)
	}
	installed := string(output.Skills[".claude/skills/review"].Files["SKILL.md"])
	for _, want := range []string{"- `team:techs/go/errors` 1.0.0: [.trailguide/_managed/generated/rules/team/techs/go/errors.md]", "- `team:techs/go/old`: retired by its library"} {
		if !strings.Contains(installed, want) {
			t.Errorf("installed SKILL.md lacks %q:\n%s", want, installed)
		}
	}
	if len(output.RetiredCitations) != 0 {
		t.Errorf("a library's skill reported retired citations: %v", output.RetiredCitations)
	}
}

func TestGenerateListsCitedRulesInSkillsIndexWithoutSkillTargets(t *testing.T) {
	config, libraries := fixture(t, `[]`)
	output, err := Generate(config, libraries, nil, Options{LocalSkills: locate(config, citingSkill("team:techs/go/errors"))})
	if err != nil {
		t.Fatal(err)
	}
	index := string(output.Files["SKILLS.md"])
	if want := "- **Applies:**\n  - `team:techs/go/errors` 1.0.0: [.trailguide/_managed/generated/rules/team/techs/go/errors.md](rules/team/techs/go/errors.md)"; !strings.Contains(index, want) {
		t.Fatalf("SKILLS.md:\n%s\nwant\n%s", index, want)
	}
}

func TestGenerateAddsNoSectionForASkillThatCitesNoRules(t *testing.T) {
	skill := map[string]SkillFiles{"skills/fix": {Files: map[string][]byte{"SKILL.md": []byte("---\nname: fix\n---\n# Fix")}}}
	output, err := generateCiting(t, `[]`, nil, skill, nil)
	if err != nil {
		t.Fatal(err)
	}
	if installed := string(output.Skills[".claude/skills/fix"].Files["SKILL.md"]); strings.Contains(installed, "Rules this skill applies") || !strings.HasSuffix(installed, "# Fix") {
		t.Fatalf("installed SKILL.md:\n%s", installed)
	}
	if strings.Contains(string(output.Files["SKILLS.md"]), "Applies") {
		t.Fatalf("SKILLS.md:\n%s", output.Files["SKILLS.md"])
	}
}

func TestGenerateRejectsInvalidAuthoredSkills(t *testing.T) {
	tests := map[string]struct {
		skill map[string]SkillFiles
		alter func(*library.Snapshot)
		want  string
	}{
		"name that isn't the folder's": {map[string]SkillFiles{"skills/fix": {Files: map[string][]byte{"SKILL.md": []byte("---\nname: fixer\n---\n")}}}, nil, `name is "fixer", but the folder is fix; they must match`},
		"unknown type":                 {map[string]SkillFiles{"skills/fix": {Files: map[string][]byte{"SKILL.md": []byte("---\nname: fix\nmetadata:\n  trailguide-skill-type: design\n---\n")}}}, nil, `trailguide-skill-type is "design"; use playbook or review, or leave it out`},
		"missing authored rule":        {citingSkill("techs/go/missing"), nil, "trailguide-rules-used names techs/go/missing, which isn't a rule in .trailguide/rules/; write the rule, or cite a source's rule with its source name, such as fabrica:techs/go/missing"},
		"unknown source":               {citingSkill("other:techs/go/errors"), nil, "trailguide-rules-used names other:techs/go/errors, but the configuration has no source named other; add the source, or correct the source name"},
		"source that isn't a library":  {citingSkill("team:techs/go/errors"), func(snapshot *library.Snapshot) { snapshot.NonLibrary = true }, "trailguide-rules-used names team:techs/go/errors, but team isn't a library, so it has no rules to cite"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := generateCiting(t, `[]`, nil, test.skill, test.alter)
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != "skills/local/fix/SKILL.md" || validation.ValidationProblem() != test.want || output.Files != nil {
				t.Fatalf("got %v; want a validation error at skills/fix/SKILL.md: %s", err, test.want)
			}
		})
	}
}

// TestGenerateRejectsACitedRuleItsSourceDoesntHave fails at an authored skill's SKILL.md when it cites a rule in a
// group its source includes in full, whose snapshot then holds every rule the group has or retired, that isn't one
// of them, however the project excludes the group. A rule in a group the source doesn't include in full may exist,
// so it stays not included.
func TestGenerateRejectsACitedRuleItsSourceDoesntHave(t *testing.T) {
	const problem = "trailguide-rules-used names team:techs/go/error, which isn't a rule in team's library; correct the rule ID, or remove it from trailguide-rules-used"
	for name, exclude := range map[string]string{
		"included group":          `[]`,
		"group excluded by group": `[{"id":"techs/go/*","reason":"Ours"}]`,
		"group excluded by all":   `[{"id":"*","reason":"Ours"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := generateCiting(t, exclude, nil, citingSkill("team:techs/go/errors", "team:techs/go/error"), nil)
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != "skills/local/fix/SKILL.md" || validation.ValidationProblem() != problem || output.Files != nil {
				t.Fatalf("got %v; want a validation error at skills/fix/SKILL.md: %s", err, problem)
			}
		})
	}
	if _, err := generateCiting(t, `[]`, nil, citingSkill("team:techs/rust/fmt", "team:practices/testing/verify"), nil); err != nil {
		t.Fatalf("rejected rules in groups the source doesn't include: %v", err)
	}
}

func TestGenerateLeavesTheCitationsOfANonLibrarySkillAlone(t *testing.T) {
	output, err := generateCiting(t, `[]`, nil, nil, func(snapshot *library.Snapshot) {
		snapshot.NonLibrary = true
		snapshot.Skills = map[string]library.ImportedSkill{"tools/review": {}}
		snapshot.Files = map[string][]byte{"tools/review/SKILL.md": []byte("---\nname: review\nmetadata:\n  trailguide-rules-used: whatever:it/says\n---\n# Review\n")}
	})
	if err != nil {
		t.Fatal(err)
	}
	if installed := string(output.Skills[".claude/skills/review"].Files["SKILL.md"]); strings.Contains(installed, "Rules this skill applies") {
		t.Fatalf("installed SKILL.md:\n%s", installed)
	}
	if !slices.Equal(output.SkillIDs, []string{"team:tools/review"}) {
		t.Fatalf("skill IDs = %v", output.SkillIDs)
	}
}
