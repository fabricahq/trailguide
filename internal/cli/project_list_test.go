// Test how project list skills, project list rules, and the update preview show skills and rules.

package cli

import (
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/trailguide"
	"github.com/fabricahq/trailguide/trailguide/projectview"
)

func TestFormatSkillListing_AlignsLevelsAndWhoSetThem(t *testing.T) {
	commit := strings.Repeat("a", 40)
	listing := project.SkillListing{Skills: []project.ListedSkill{
		{ID: "team:skills/ship", Source: "team", Level: "required", SetBy: &project.LevelSetter{Source: "team", Release: 3, Commit: commit}, Status: project.SkillInstalled},
		{ID: "team:skills/deploy", Source: "team", Level: "offered", SetBy: &project.LevelSetter{Source: "team", Commit: commit}, Status: project.SkillReplaced, ReplacedBy: "skills/deploy"},
		{ID: "tools:tools/review", Source: "tools", Status: project.SkillAvailable},
		{ID: "skills/check-release", Status: project.SkillInstalled},
	}}
	want := `SKILL                 LEVEL     SET BY                     STATUS
team:skills/ship      required  team, release 3            installed
team:skills/deploy    offered   team, commit aaaaaaaaaaaa  replaced by skills/deploy
tools:tools/review    -         tools, not a library       available
skills/check-release  -         this project               installed
`
	if got := formatSkillListing(listing); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := formatSkillListing(project.SkillListing{}); got != "The project has no skills, and its sources offer none.\n" {
		t.Fatalf("empty listing: %q", got)
	}
}

func TestFormatRuleListing_SummarizesEachRulesStatus(t *testing.T) {
	v1, v2 := trailguide.RuleVersion{Major: 1}, trailguide.RuleVersion{Major: 2}
	one, three := 1, 3
	listing := project.RuleListing{Rules: []projectview.Rule{
		{ID: "team:techs/go/errors", Source: "team", Path: "techs/go/errors", Version: &v1, Release: &one, Exclusion: &projectview.Exclusion{Reason: "Ours."}, ReplacedBy: "local:techs/go/errors"},
		{ID: "team:techs/go/logging", Source: "team", Path: "techs/go/logging", Version: &v2, Release: &three, Exclusion: &projectview.Exclusion{Reason: "No."}},
		{ID: "team:techs/go/naming", Source: "team", Path: "techs/go/naming", Version: &v1, Release: &one, Active: true, Pin: &projectview.Pin{Version: v1, Reason: "Wait."}, Retired: true},
		{ID: "team:techs/go/draft", Source: "team", Path: "techs/go/draft", Commit: strings.Repeat("b", 40), Active: true},
		{ID: "local:techs/go/errors", Path: "techs/go/errors", Active: true, Replaces: &projectview.Replacement{ID: "team:techs/go/errors", BasedOn: &v1}},
	}}
	want := `RULE                   VERSION  FROM                       STATUS
team:techs/go/errors   1.0.0    team, release 1            replaced by local:techs/go/errors
team:techs/go/logging  2.0.0    team, release 3            excluded
team:techs/go/naming   1.0.0    team, release 1            active, pinned at 1.0.0, retired
team:techs/go/draft    -        team, commit bbbbbbbbbbbb  active
local:techs/go/errors  -        this project               active, replaces team:techs/go/errors
`
	if got := formatRuleListing(listing); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := formatRuleListing(project.RuleListing{}); got != "The project holds no rules.\n" {
		t.Fatalf("empty listing: %q", got)
	}
}

func TestFormatUpdatePreview_ShowsSkillLevelChanges(t *testing.T) {
	var out strings.Builder
	formatUpdatePreview(&out, []imports.SourceUpdate{{Name: "team", Rules: []imports.RuleUpdate{}, SharedFiles: &imports.SharedFilesUpdate{From: 1, To: 2}, Skills: []imports.SkillUpdate{
		{ID: "skills/audit", Change: imports.SkillAdded, Files: []string{}, Scripts: []imports.ScriptUpdate{}},
	}, SkillLevels: []imports.SkillLevelUpdate{
		{ID: "skills/audit", To: "required"},
		{ID: "skills/review", From: "recommended", To: "required"},
		{ID: "skills/ship", From: "required", To: "offered"},
	}}})
	for _, want := range []string{
		"  added     skills/audit\n            level: none -> required\n            New required skill: it installs whatever skills.include says.\n",
		"  level     skills/review  recommended -> required\n            Now required: it installs whatever skills.include says, and the project can't exclude or fork it.\n",
		"  level     skills/ship    required -> offered\n            No longer required: it stays installed only when skills.include selects it.\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("preview lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "level     skills/audit") {
		t.Errorf("preview shows the added skill's level change on a row of its own too:\n%s", out.String())
	}
}

// TestFormatUpdatePreview_SaysANewerReleaseIsAvailable: an update that takes the release an edited ref names says
// which newer release a second update moves to.
func TestFormatUpdatePreview_SaysANewerReleaseIsAvailable(t *testing.T) {
	var out strings.Builder
	formatUpdatePreview(&out, []imports.SourceUpdate{{Name: "team", Rules: []imports.RuleUpdate{}, SharedFiles: &imports.SharedFilesUpdate{From: 4, To: 7}, Available: 9, Skills: []imports.SkillUpdate{}}})
	if want := "  Shared files: release 4 -> 7\n  Release 9 is available; run trailguide project update team again to move to it.\n"; !strings.Contains(out.String(), want) {
		t.Errorf("preview lacks %q:\n%s", want, out.String())
	}
	if strings.Contains(out.String(), "Sets sources.team.ref") {
		t.Errorf("preview sets a ref:\n%s", out.String())
	}
}

func TestFormatUpdatePreview_ShowsCuratedSourcesAndTheirSkillChanges(t *testing.T) {
	var out strings.Builder
	formatUpdatePreview(&out, []imports.SourceUpdate{{Name: "team", Rules: []imports.RuleUpdate{}, Curated: []imports.CuratedUpdate{{
		Name: "pocock", Repository: "https://github.com/mattpocock/skills", Ref: "stable", TagMoved: true,
		Commit:  &imports.CommitMove{From: "1111111111111111111111111111111111111111", To: "2222222222222222222222222222222222222222"},
		Changes: []imports.SkillUpdate{{ID: "pocock:skills/ops/deploy", Change: imports.SkillChanged, Files: []string{"SKILL.md", "scripts/deploy.sh"}, Scripts: []imports.ScriptUpdate{{Path: "scripts/deploy.sh", Change: imports.SkillChanged}}}},
	}}}})
	want := "team\n" +
		"  team curates pocock  stable: 111111111111 -> 222222222222\n" +
		"            The tag stable now names a different commit than at the last update.\n" +
		"  changed   pocock:skills/ops/deploy\n" +
		"            Changed: SKILL.md, scripts/deploy.sh\n" +
		"            script changed: scripts/deploy.sh\n"
	if out.String() != want {
		t.Fatalf("preview:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestFormatUpdatePreview_ShowsTheCommitASourceThatFollowsItsRefMovesTo(t *testing.T) {
	var out strings.Builder
	formatUpdatePreview(&out, []imports.SourceUpdate{
		{Name: "skills", Ref: "main", FollowsRef: true, Rules: []imports.RuleUpdate{}, Commit: &imports.CommitMove{From: "1111111111111111111111111111111111111111", To: "2222222222222222222222222222222222222222"},
			Skills: []imports.SkillUpdate{{ID: "tools/review", Change: imports.SkillChanged, Files: []string{"SKILL.md"}, Scripts: []imports.ScriptUpdate{}}}},
		{Name: "pinned", Ref: "v1.0", FollowsRef: true, TagMoved: true, Rules: []imports.RuleUpdate{}, Commit: &imports.CommitMove{From: "3333333333333333333333333333333333333333", To: "4444444444444444444444444444444444444444"}},
		{Name: "still", Ref: "main", FollowsRef: true, Rules: []imports.RuleUpdate{}},
		{Name: "team", Ref: "release/2", Rules: []imports.RuleUpdate{}},
	})
	want := "skills\n" +
		"  main: 111111111111 -> 222222222222\n" +
		"  changed   tools/review\n" +
		"            Changed: SKILL.md\n" +
		"pinned\n" +
		"  v1.0: 333333333333 -> 444444444444\n" +
		"            The tag v1.0 now names a different commit than at the last update.\n" +
		"still\n" +
		"  main: no new commit.\n" +
		"team\n" +
		"  Imports release/2 with ref, so update doesn't move it.\n"
	if out.String() != want {
		t.Fatalf("preview:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestFormatUpdatePreview_ShowsACuratedSourceTheProjectNoLongerTakesSkillsFrom(t *testing.T) {
	var out strings.Builder
	formatUpdatePreview(&out, []imports.SourceUpdate{{Name: "team", Rules: []imports.RuleUpdate{}, Curated: []imports.CuratedUpdate{{
		Name: "pocock", Repository: "https://github.com/mattpocock/skills", Ref: "main", Commit: &imports.CommitMove{From: "1111111111111111111111111111111111111111"},
		Changes: []imports.SkillUpdate{{ID: "pocock:skills/ops/deploy", Change: imports.SkillRemoved, Files: []string{}, Scripts: []imports.ScriptUpdate{}}},
	}}}})
	want := "team\n" +
		"  team curates pocock  main: 111111111111 -> none\n" +
		"  removed   pocock:skills/ops/deploy\n"
	if out.String() != want {
		t.Fatalf("preview:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestFormatUpdatePreview_ShowsACuratedLibraryMovingBetweenReleases(t *testing.T) {
	var out strings.Builder
	formatUpdatePreview(&out, []imports.SourceUpdate{{Name: "team", Rules: []imports.RuleUpdate{}, Curated: []imports.CuratedUpdate{{
		Name: "shared", Repository: "https://github.com/example/shared", Commit: &imports.CommitMove{From: "1111111111111111111111111111111111111111", To: "2222222222222222222222222222222222222222"},
		Release: &imports.SharedFilesUpdate{From: 3, To: 4}, Changes: []imports.SkillUpdate{},
	}}}})
	if want := "team\n  team curates shared  release 3 -> 4\n"; out.String() != want {
		t.Fatalf("preview:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestFormatUpdatePreview_ShowsACuratedSkillsLevelChangeOnItsRow(t *testing.T) {
	var out strings.Builder
	formatUpdatePreview(&out, []imports.SourceUpdate{{Name: "team", Rules: []imports.RuleUpdate{},
		SkillLevels: []imports.SkillLevelUpdate{{ID: "pocock:skills/to-spec", From: "offered", To: "recommended"}},
		Curated: []imports.CuratedUpdate{{Name: "pocock", Ref: "stable",
			Commit:  &imports.CommitMove{From: "1111111111111111111111111111111111111111", To: "2222222222222222222222222222222222222222"},
			Changes: []imports.SkillUpdate{{ID: "pocock:skills/to-spec", Change: imports.SkillAdded, Files: []string{}, Scripts: []imports.ScriptUpdate{}}},
		}}}})
	want := "team\n" +
		"  team curates pocock  stable: 111111111111 -> 222222222222\n" +
		"  added     pocock:skills/to-spec\n" +
		"            level: offered -> recommended\n" +
		"            Now recommended: it installs when skills.include has recommended.\n"
	if out.String() != want {
		t.Fatalf("preview:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestFormatUpdatePreview_ShowsAReplacedSkillsLevelChangeOnItsRow(t *testing.T) {
	var out strings.Builder
	from, to := strings.Repeat("1", 40), strings.Repeat("2", 40)
	formatUpdatePreview(&out, []imports.SourceUpdate{{Name: "team", Rules: []imports.RuleUpdate{},
		SkillLevels:    []imports.SkillLevelUpdate{{ID: "skills/review", From: "recommended", To: "offered"}},
		ReplacedSkills: []imports.ReplacedSkillUpdate{{ID: "skills/review", LocalSkill: "skills/review", From: from, To: to, Files: []string{"SKILL.md"}}},
	}})
	if !strings.Contains(out.String(), "  replaced  skills/review  commit "+shortCommit(from)) || !strings.Contains(out.String(), "            level: recommended -> offered\n") {
		t.Fatalf("preview doesn't show the level change under the replaced row:\n%s", out.String())
	}
	if strings.Contains(out.String(), "  level     skills/review") {
		t.Fatalf("preview shows the replaced skill's level change on a row of its own too:\n%s", out.String())
	}
}
