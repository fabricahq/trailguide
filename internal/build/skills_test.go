// Test the installed skill folders, SKILLS.md, and installed-skills.json that generation renders for authored skills.

package build_test

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// skillConfig parses a configuration with no sources and the skill targets in targets, a JSON list.
func skillConfig(t *testing.T, targets string) rules.Configuration {
	t.Helper()
	config, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"skillTargets":` + targets + `,"sources":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestGenerateInstallsAuthoredSkillsIntoEachTarget(t *testing.T) {
	local := map[string]build.SkillFiles{"skills/tidy": {Files: map[string][]byte{"SKILL.md": []byte("---\nname: tidy\ndescription: Tidy | code.\n---\n# Tidy\n"), "run.sh": []byte("#!/bin/sh\n")}, Executable: []string{"run.sh"}}}
	output, err := build.Generate(skillConfig(t, `["claude","agents"]`), nil, nil, build.Options{ToolVersion: "test", LocalSkills: local})
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{".claude/skills/tidy", ".agents/skills/tidy"} {
		installed, ok := output.Skills[folder]
		if !ok || !strings.Contains(string(installed.Files["SKILL.md"]), "---\n<!-- trailguide:installed skills/tidy. Do not edit; edit .trailguide/skills/tidy/ instead. -->\n# Tidy") || !slices.Equal(installed.Executable, []string{"run.sh"}) {
			t.Errorf("%s = %+v", folder, installed)
		}
	}
	if !slices.Equal(output.SkillIDs, []string{"skills/tidy"}) {
		t.Errorf("skill IDs = %v", output.SkillIDs)
	}
	if got := build.InstalledFolders(output.Files[build.InstalledSkillsFile]); !slices.Equal(got, []string{".agents/skills/tidy", ".claude/skills/tidy"}) {
		t.Errorf("installed folders = %v", got)
	}
	if index := string(output.Files["SKILLS.md"]); !strings.Contains(index, "### tidy\n\nTidy \\| code.") {
		t.Errorf("SKILLS.md:\n%s", index)
	}
	if !strings.Contains(string(output.Files["RULES.md"]), "This project also installs agent skills.") {
		t.Errorf("RULES.md doesn't point to SKILLS.md:\n%s", output.Files["RULES.md"])
	}
}

func TestGenerateWithoutSkillsAddsNoSkillFiles(t *testing.T) {
	output, err := build.Generate(skillConfig(t, `["claude"]`), nil, nil, build.Options{ToolVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Skills) != 0 || output.Files["SKILLS.md"] != nil || output.Files[build.InstalledSkillsFile] != nil || strings.Contains(string(output.Files["RULES.md"]), "SKILLS.md") {
		t.Fatalf("output without skills has skill files: %v", output.Skills)
	}
}

func TestGenerateRejectsASourceSkillAndAnAuthoredSkillWithOneInstallName(t *testing.T) {
	commit := strings.Repeat("a", 40)
	config, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"skillTargets":[],"sources":{"tools":{"repository":"https://example.com/tools.git","ref":"` + commit + `","skills":{"include":["*"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	skill := []byte("---\nname: shared\n---\n")
	tools := build.Library{
		Catalog:  library.Catalog{Rules: []string{}, Groups: []library.Group{}, SupportingFiles: map[string][]byte{}},
		Snapshot: library.Snapshot{NonLibrary: true, Commit: commit, Skills: map[string]library.ImportedSkill{"tools/shared": {}}, Files: map[string][]byte{"tools/shared/SKILL.md": skill}},
	}
	_, err = build.Generate(config, map[string]build.Library{"tools": tools}, nil, build.Options{ToolVersion: "test", LocalSkills: map[string]build.SkillFiles{"skills/shared": {Files: map[string][]byte{"SKILL.md": skill}}}})
	var clash *build.ClashError
	if !errors.As(err, &clash) || clash.Name != "shared" || clash.IDs != [2]string{"skills/shared", "tools:tools/shared"} {
		t.Fatalf("got %v; want a clash of skills/shared and tools:tools/shared", err)
	}
}

func TestInstalledSkillIDsNamesTheSkillInEachFolder(t *testing.T) {
	record := `{"formatVersion":1,"skills":[{"id":"skills/tidy","targets":[".agents/skills/tidy",".claude/skills/tidy"]},{"id":"team:skills/review","targets":[".claude/skills/review"]}]}`
	want := map[string]string{".agents/skills/tidy": "skills/tidy", ".claude/skills/tidy": "skills/tidy", ".claude/skills/review": "team:skills/review"}
	if got := build.InstalledSkillIDs([]byte(record)); !maps.Equal(got, want) {
		t.Fatalf("installed skills = %v, want %v", got, want)
	}
}

func TestInstalledFoldersListsOnlySkillTargetFolders(t *testing.T) {
	record := `{"formatVersion":1,"skills":[{"targets":[".claude/skills/tidy","../outside/tidy",".claude/skills/../../x",".github/skills/tidy",".agents/skills/Bad_Name",".agents/skills/tidy"]}]}`
	if got := build.InstalledFolders([]byte(record)); !slices.Equal(got, []string{".agents/skills/tidy", ".claude/skills/tidy"}) {
		t.Fatalf("installed folders = %v", got)
	}
	for _, unreadable := range []string{"", "{", `{"formatVersion":2,"skills":[{"targets":[".claude/skills/tidy"]}]}`} {
		if got := build.InstalledFolders([]byte(unreadable)); len(got) != 0 {
			t.Errorf("InstalledFolders(%q) = %v, want none", unreadable, got)
		}
	}
}

// replacementConfig parses a configuration whose source team, from a non-library repository, includes every skill
// and has the skills fields in fields, JSON object members such as "forks":{...}.
func replacementConfig(t *testing.T, fields string) rules.Configuration {
	t.Helper()
	config, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":"https://example.invalid/team.git","ref":"main","skills":{"include":["*"],` + fields + `}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

// skillSnapshot returns team's snapshot, whose commit has the skills review and fix.
func skillSnapshot() map[string]build.Library {
	return map[string]build.Library{"team": {Snapshot: library.Snapshot{
		Commit: strings.Repeat("c", 40), NonLibrary: true, SkillSelection: []string{"*"}, Executable: []string{},
		Skills: map[string]library.ImportedSkill{"skills/review": {}, "skills/fix": {}},
		Files: map[string][]byte{
			"skills/review/SKILL.md": []byte("---\nname: review\ndescription: Review.\n---\n# Review\n"),
			"skills/fix/SKILL.md":    []byte("---\nname: fix\ndescription: Fix.\n---\n# Fix\n"),
		},
	}}}
}

func TestGenerateInstallsAForkInPlaceOfTheSkillItForks(t *testing.T) {
	config := replacementConfig(t, `"exclude":[{"id":"skills/fix","reason":"We fix in CI."}],"forks":{"skills/review":{"as":"skills/our-review","reason":"Ours.","basedOn":"`+strings.Repeat("b", 40)+`"}}`)
	local := map[string]build.SkillFiles{"skills/our-review": {Files: map[string][]byte{"SKILL.md": []byte("---\nname: our-review\ndescription: Our review.\n---\n# Review\n")}}}
	output, err := build.Generate(config, skillSnapshot(), nil, build.Options{ToolVersion: "test", LocalSkills: local})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(output.SkillIDs, []string{"skills/our-review"}) || output.Skills[".claude/skills/review"].Files != nil || output.Skills[".claude/skills/fix"].Files != nil {
		t.Fatalf("installed %v", output.SkillIDs)
	}
	index := string(output.Files["SKILLS.md"])
	for _, want := range []string{
		"- **Replaces:** `team:skills/review`, which the project forks, based on its copy at commit `" + strings.Repeat("b", 40) + "`\n- **Reason:** Ours.",
		"## Excluded skills\n\nThe project's configuration excludes these skills, so they aren't installed.",
		"- `team:skills/fix`: We fix in CI.",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("SKILLS.md lacks %q:\n%s", want, index)
		}
	}
}

func TestGenerateKeepsAForkOfASkillItsSourceDeleted(t *testing.T) {
	config := replacementConfig(t, `"forks":{"skills/plan":{"as":"skills/plan","reason":"Ours."}}`)
	local := map[string]build.SkillFiles{"skills/plan": {Files: map[string][]byte{"SKILL.md": []byte("---\nname: plan\ndescription: Plan.\n---\n# Plan\n")}}}
	output, err := build.Generate(config, skillSnapshot(), nil, build.Options{ToolVersion: "test", LocalSkills: local})
	if err != nil {
		t.Fatal(err)
	}
	if output.Skills[".claude/skills/plan"].Files == nil || !strings.Contains(string(output.Files["SKILLS.md"]), "- **Replaces:** `team:skills/plan`, which the project forks") {
		t.Fatalf("installed %v:\n%s", output.SkillIDs, output.Files["SKILLS.md"])
	}
}

func TestGenerateRejectsInvalidForks(t *testing.T) {
	local := map[string]build.SkillFiles{"skills/ours": {Files: map[string][]byte{"SKILL.md": []byte("---\nname: ours\n---\n")}}}
	twoSources, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"sources":{"other":{"repository":"https://example.invalid/other.git","ref":"main","skills":{"forks":{"skills/fix":{"as":"skills/ours","reason":"No."}}}},"team":{"repository":"https://example.invalid/team.git","ref":"main","skills":{"forks":{"skills/review":{"as":"skills/ours","reason":"No."}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	libraries := skillSnapshot()
	libraries["other"] = libraries["team"]
	for name, test := range map[string]struct {
		config            rules.Configuration
		location, problem string
	}{
		"excluded missing skill": {replacementConfig(t, `"exclude":[{"id":"skills/plan","reason":"No."}]`), "sources.team.skills.exclude.skills/plan", "the source doesn't install skill skills/plan"},
		"no authored skill":      {replacementConfig(t, `"forks":{"skills/review":{"as":"skills/missing","reason":"No."}}`), "sources.team.skills.forks.skills/review.as", "the project has no authored skill skills/missing; add .trailguide/skills/missing/SKILL.md"},
		"one replacing two":      {twoSources, "sources.team.skills.forks.skills/review.as", "skills/ours already replaces the skill sources.other.skills.forks.skills/fix names"},
	} {
		t.Run(name, func(t *testing.T) {
			libraries := skillSnapshot()
			if len(test.config.Sources) > 1 {
				libraries["other"] = libraries["team"]
			}
			_, err := build.Generate(test.config, libraries, nil, build.Options{ToolVersion: "test", LocalSkills: local})
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || !strings.Contains(validation.ValidationProblem(), test.problem) {
				t.Fatalf("got %v; want %q at %s", err, test.problem, test.location)
			}
		})
	}
}

// renameConfig parses a configuration whose source team, from a non-library repository, includes every skill and
// has the skills.rename mapping in rename, a JSON object.
func renameConfig(t *testing.T, rename string) rules.Configuration {
	t.Helper()
	config, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":"https://example.invalid/team.git","ref":"main","skills":{"include":["*"],"rename":` + rename + `}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestGenerateRenamesOnlyWhenTheNameDiffers(t *testing.T) {
	tests := map[string]struct {
		rename  string
		renamed []build.RenamedSkill
		folder  string
	}{
		"new name":  {`{"skills/review":"team-review"}`, []build.RenamedSkill{{ID: "team:skills/review", Source: "team", Folder: "skills/review", From: "review", To: "team-review"}}, ".claude/skills/team-review"},
		"same name": {`{"skills/review":"review"}`, []build.RenamedSkill{}, ".claude/skills/review"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := build.Generate(renameConfig(t, test.rename), skillSnapshot(), nil, build.Options{ToolVersion: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(output.RenamedSkills, test.renamed) || output.Skills[test.folder].Files == nil {
				t.Fatalf("renamed %+v, installed %v", output.RenamedSkills, slices.Sorted(maps.Keys(output.Skills)))
			}
		})
	}
}

func TestGenerateRejectsRenamesThatClash(t *testing.T) {
	_, err := build.Generate(renameConfig(t, `{"skills/review":"fix"}`), skillSnapshot(), nil, build.Options{ToolVersion: "test"})
	var clash *build.ClashError
	if !errors.As(err, &clash) || clash.Name != "fix" || clash.IDs != [2]string{"team:skills/fix", "team:skills/review"} {
		t.Fatalf("got %v; want a clash of team:skills/fix and team:skills/review", err)
	}
}

// TestGenerateRenamesASkillWhoseOwnNameIsInvalid installs a non-library skill whose folder name breaks the name
// grammar under the name skills.rename gives it, which only a rename can do.
func TestGenerateRenamesASkillWhoseOwnNameIsInvalid(t *testing.T) {
	libraries := skillSnapshot()
	team := libraries["team"]
	team.Snapshot.Skills = map[string]library.ImportedSkill{"tools/Review": {}}
	team.Snapshot.Files = map[string][]byte{"tools/Review/SKILL.md": []byte("# Review\n")}
	libraries["team"] = team
	if _, err := build.Generate(renameConfig(t, `{}`), libraries, nil, build.Options{ToolVersion: "test"}); err == nil || !strings.Contains(err.Error(), `the skill's name "Review" doesn't follow`) {
		t.Fatalf("without a rename: %v; want the invalid name rejected", err)
	}
	output, err := build.Generate(renameConfig(t, `{"tools/Review":"team-review"}`), libraries, nil, build.Options{ToolVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	want := []build.RenamedSkill{{ID: "team:tools/Review", Source: "team", Folder: "tools/Review", From: "Review", To: "team-review"}}
	installed := output.Skills[".claude/skills/team-review"].Files["SKILL.md"]
	if !reflect.DeepEqual(output.RenamedSkills, want) || !strings.HasPrefix(string(installed), "---\nname: team-review\n---\n") {
		t.Fatalf("renamed %+v, installed SKILL.md:\n%s", output.RenamedSkills, installed)
	}
}
