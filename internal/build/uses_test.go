// Test how generation links installed skills to the skills they use, and shows why each is installed.

package build

import (
	"errors"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// skillFile returns a SKILL.md for skill name whose trailguide-skills-used lists used.
func skillFile(name string, used ...string) []byte {
	text := "---\nname: " + name + "\ndescription: " + name + ".\n"
	if len(used) > 0 {
		text += "metadata:\n  trailguide-skills-used: |\n    " + strings.Join(used, "\n    ") + "\n"
	}
	return []byte(text + "---\n# " + name + "\n")
}

// generateUsing generates the fixture's project with team installing the skills in snapshot, each with its SKILL.md
// from files, configured skills.uses, and authored skills local, installing into .claude/skills/.
func generateUsing(t *testing.T, snapshot map[string]library.ImportedSkill, files map[string][]byte, configured map[string][]string, local map[string]SkillFiles) (Output, error) {
	t.Helper()
	config, libraries := fixture(t, `[]`)
	config.SkillTargets = []string{"claude"}
	config.Sources[0].SkillUses = configured
	team := libraries["team"]
	team.Snapshot.Skills = snapshot
	team.Snapshot.Files = map[string][]byte{}
	for id, data := range files {
		team.Snapshot.Files[id+"/SKILL.md"] = data
	}
	libraries["team"] = team
	return Generate(config, libraries, nil, Options{LocalSkills: locate(config, local)})
}

func TestGenerateShowsWhichSkillsEachSkillUsesAndWhyItIsInstalled(t *testing.T) {
	snapshot := map[string]library.ImportedSkill{
		"skills/build": {Level: "offered", Included: true, UsedBy: []string{}},
		"skills/tdd":   {Level: "offered", UsedBy: []string{"skills/build"}},
		"skills/how":   {Level: "offered", Included: true, UsedBy: []string{"skills/build"}},
	}
	files := map[string][]byte{"skills/build": skillFile("build", "skills/tdd"), "skills/tdd": skillFile("tdd"), "skills/how": skillFile("how")}
	local := map[string]SkillFiles{"skills/release": {Files: map[string][]byte{"SKILL.md": skillFile("release", "team:skills/build")}, Executable: []string{}}}
	output, err := generateUsing(t, snapshot, files, map[string][]string{"skills/build": {"skills/how"}}, local)
	if err != nil {
		t.Fatal(err)
	}
	index := string(output.Files["SKILLS.md"])
	for _, want := range []string{
		"- **Uses:** `how`, `tdd`\n- **Installed because:** included, and used by `release`\n",
		"### tdd\n\ntdd.\n\n- **Skill:** `team:skills/tdd`, from release/1, commit `" + commit + "`\n- **Level:** offered by `team`, release 1\n- **Installed because:** used by `build`\n",
		"### how\n\nhow.\n\n- **Skill:** `team:skills/how`, from release/1, commit `" + commit + "`\n- **Level:** offered by `team`, release 1\n- **Installed because:** included, and used by `build`\n",
		"- **Skill:** `skills/release`, authored in this project\n- **Uses:** `build`\n- **Installed at:**",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("SKILLS.md lacks\n%s\nin\n%s", want, index)
		}
	}
}

func TestGenerateRejectsAnAuthoredSkillThatUsesASkillTheProjectDoesNotInstall(t *testing.T) {
	snapshot := map[string]library.ImportedSkill{"skills/build": {Level: "offered", Included: true, UsedBy: []string{}}}
	files := map[string][]byte{"skills/build": skillFile("build")}
	authored := func(name string, used ...string) SkillFiles {
		return SkillFiles{Files: map[string][]byte{"SKILL.md": skillFile(name, used...)}, Executable: []string{}}
	}
	tests := map[string]struct {
		local    map[string]SkillFiles
		location string
		problem  string
	}{
		"a missing authored skill": {
			map[string]SkillFiles{"skills/release": authored("release", "skills/tag")}, "skills/local/release/SKILL.md",
			"trailguide-skills-used names skills/tag, which isn't a skill in .trailguide/skills/; write the skill, or name a source's skill with its source name, such as fabrica:skills/tag",
		},
		"an unknown source": {
			map[string]SkillFiles{"skills/release": authored("release", "other:skills/build")}, "skills/local/release/SKILL.md",
			"trailguide-skills-used names other:skills/build, but the configuration has no source named other; add the source, or correct the source name",
		},
		"a source's skill the project doesn't install": {
			map[string]SkillFiles{"skills/release": authored("release", "team:skills/tdd")}, "skills/local/release/SKILL.md",
			"trailguide-skills-used names team:skills/tdd, which the project doesn't install; add skills/tdd to sources.team.skills.include, then run trailguide project sync",
		},
		"a curated skill the library doesn't list": {
			map[string]SkillFiles{"skills/release": authored("release", "team:anthropic:skills/skill-creator")}, "skills/local/release/SKILL.md",
			"trailguide-skills-used names team:anthropic:skills/skill-creator, which library team doesn't curate at the revision the project installs; remove it, or declare the skill's repository as a source of your own and name the skill with that source's name",
		},
		"authored skills in a cycle": {
			map[string]SkillFiles{"skills/release": authored("release", "skills/tag"), "skills/tag": authored("tag", "skills/release")}, "skills/local/release/SKILL.md",
			"trailguide-skills-used forms a cycle: skills/release -> skills/tag -> skills/release. A skill installs with every skill it uses, so uses can't form a cycle; remove one of them",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := generateUsing(t, snapshot, files, nil, test.local)
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || validation.ValidationProblem() != test.problem {
				t.Fatalf("got %v\nwant %s: %s", err, test.location, test.problem)
			}
		})
	}
}

func TestGenerateRedirectsAUseOfAnExcludedSkillToItsReplacement(t *testing.T) {
	snapshot := map[string]library.ImportedSkill{
		"skills/build": {Level: "offered", Included: true, UsedBy: []string{}},
		"skills/tdd":   {Level: "offered", UsedBy: []string{"skills/build"}},
		"skills/how":   {Level: "offered", Included: true, UsedBy: []string{}},
	}
	files := map[string][]byte{"skills/build": skillFile("build", "skills/tdd", "skills/how"), "skills/tdd": skillFile("tdd"), "skills/how": skillFile("how")}
	config, libraries := fixture(t, `[]`)
	config.SkillTargets = []string{"claude"}
	config.Sources[0].SkillExclude = map[string]rules.SkillExclusion{"skills/how": {Reason: "Not needed."}}
	config.Sources[0].SkillForks = map[string]rules.SkillFork{"skills/tdd": {As: "skills/our-tdd", Reason: "Ours."}}
	team := libraries["team"]
	team.Snapshot.Skills = snapshot
	team.Snapshot.Files = map[string][]byte{}
	for id, data := range files {
		team.Snapshot.Files[id+"/SKILL.md"] = data
	}
	libraries["team"] = team
	local := map[string]SkillFiles{"skills/our-tdd": {Files: map[string][]byte{"SKILL.md": skillFile("our-tdd")}, Executable: []string{}}}
	output, err := Generate(config, libraries, nil, Options{LocalSkills: locate(config, local)})
	if err != nil {
		t.Fatal(err)
	}
	if index := string(output.Files["SKILLS.md"]); !strings.Contains(index, "### build\n\nbuild.\n\n- **Skill:** `team:skills/build`") || !strings.Contains(index, "- **Uses:** `our-tdd`\n") {
		t.Errorf("SKILLS.md doesn't show build using our-tdd:\n%s", index)
	}
	if len(output.ExcludedUses) != 1 || !strings.Contains(output.ExcludedUses[0], "team:skills/build uses team:skills/how, which the project excludes without a replacement") {
		t.Fatalf("excluded uses = %q", output.ExcludedUses)
	}
}
