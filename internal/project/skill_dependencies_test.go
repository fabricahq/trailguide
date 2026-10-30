// Test that a library's dependencies install only with the skills that use them, and that a project can't choose one.

package project

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// dependencyLibrary returns a library whose release/1 publishes techs/go/errors and three skills: add-skill and
// deploy, offered, and format-skill, a dependency that add-skill uses.
func dependencyLibrary(t *testing.T) *gitfixture.Fixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                              []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library","skills":{"offered":["skills/add-skill","skills/deploy"],"dependencies":["skills/format-skill"]}}}`),
		"techs/go/_group.yaml":                     []byte(projectMetadata),
		"techs/go/errors.md":                       []byte(projectRule),
		".trailguide/skills/add-skill/SKILL.md":    usingSkill("add-skill", "skills/format-skill"),
		".trailguide/skills/format-skill/SKILL.md": usingSkill("format-skill"),
		".trailguide/skills/deploy/SKILL.md":       usingSkill("deploy"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSync_InstallsADependencyOnlyWithTheSkillsThatUseIt(t *testing.T) {
	f := dependencyLibrary(t)
	for name, test := range map[string]struct {
		include string
		want    []string
	}{
		"every skill":              {`"*"`, []string{"add-skill", "deploy", "format-skill"}},
		"the skill that uses it":   {`"skills/add-skill"`, []string{"add-skill", "format-skill"}},
		"a skill that doesn't use": {`"skills/deploy"`, []string{"deploy"}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":[`+test.include+`]}}}}`)
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			if got := installedSkills(t, options); !slices.Equal(got, test.want) {
				t.Fatalf("installed %v; want %v", got, test.want)
			}
			if _, err := Build(ctx, options); err != nil {
				t.Fatalf("build after sync: %v", err)
			}
			if result, err := Check(ctx, options); err != nil || !result.Current() {
				t.Fatalf("check = %+v, %v", result, err)
			}
			listing, err := ListSkills(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(listing.Skills, func(skill ListedSkill) bool { return skill.ID == "team:skills/format-skill" })
			if i < 0 || listing.Skills[i].Level != "dependency" {
				t.Fatalf("listing = %+v; want format-skill listed as a dependency", listing.Skills)
			}
			if !slices.Contains(test.want, "format-skill") {
				return
			}
			if skill := lockedEntry(t, options, "team").Skills["skills/format-skill"]; skill.Level == nil || *skill.Level != "dependency" || skill.Included || !slices.Equal(skill.UsedBy, []string{"skills/add-skill"}) {
				t.Fatalf("format-skill's record = %+v; want a dependency installed because add-skill uses it", skill)
			}
			index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md")
			for _, want := range []string{"- **Level:** dependency by `team`, release 1, which installs it only with the skills that use it", "- **Installed because:** used by `add-skill`"} {
				if !strings.Contains(index, want) {
					t.Errorf("SKILLS.md doesn't say %q:\n%s", want, index)
				}
			}
		})
	}
}

func TestSync_RefusesADependencyTheProjectNames(t *testing.T) {
	f := dependencyLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/format-skill"]}}}}`)
	_, err := Sync(context.Background(), options, git)
	var validation errs.ValidationError
	want := "names skills/format-skill, which the library lists in library.skills.dependencies, so it installs only with the library's skills that use it, and a project can't choose it. Include a skill that uses it instead, skills/add-skill, or remove it from skills.include"
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.team.skills.include" || validation.ValidationProblem() != want {
		t.Fatalf("got %v; want the named dependency refused", err)
	}
}

func TestSync_InstallsACuratedDependencyOnlyWithTheSkillsThatUseIt(t *testing.T) {
	ctx := context.Background()
	listed := "    offered: [skills/add-skill]\n    dependencies: [pocock:skills/productivity/grilling]\n"
	authored := map[string][]byte{".trailguide/skills/add-skill/SKILL.md": usingSkill("add-skill", "pocock:skills/productivity/grilling")}
	b := newCuratingWith(t, listed, authored, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	if got := installedSkills(t, b.options); !slices.Equal(got, []string{"add-skill", "grilling"}) {
		t.Fatalf("installed %v; want add-skill and the curated skill it uses", got)
	}
	curated, _ := b.teamRecord(t)["curated"].(map[string]any)
	pocock, _ := curated["pocock"].(map[string]any)
	grilling, _ := pocock["skills"].(map[string]any)["skills/productivity/grilling"].(map[string]any)
	if grilling["level"] != "dependency" || grilling["included"] != false {
		t.Fatalf("grilling's record = %v; want a dependency installed because add-skill uses it", grilling)
	}
	if result, err := Check(ctx, b.options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

func TestSync_RefusesACuratedDependencyTheProjectNames(t *testing.T) {
	listed := "    offered: [skills/add-skill]\n    dependencies: [pocock:skills/productivity/grilling]\n"
	authored := map[string][]byte{".trailguide/skills/add-skill/SKILL.md": usingSkill("add-skill", "pocock:skills/productivity/grilling")}
	b := newCuratingWith(t, listed, authored, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["pocock:skills/productivity/grilling"]}}}}`)
	_, err := Sync(context.Background(), b.options, b.git)
	if err == nil || !strings.Contains(err.Error(), "names pocock:skills/productivity/grilling, which the library lists in library.skills.dependencies") || !strings.Contains(err.Error(), "Include a skill that uses it instead, skills/add-skill") {
		t.Fatalf("got %v; want the named curated dependency refused", err)
	}
}

func TestSync_PointsANamedCuratedDependencyToTheCuratedSkillsThatUseIt(t *testing.T) {
	listed := "    offered: [pocock:skills/engineering/to-spec]\n    dependencies: [pocock:skills/engineering/interview]\n"
	b := newCuratingFrom(t, listed, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["pocock:skills/engineering/interview"]}}}}`)
	_, err := Sync(context.Background(), b.options, b.git)
	want := "It installs with the skills the library curates from pocock that use it; include one of those instead"
	if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "maintainer") {
		t.Fatalf("got %v; want the curated callers suggested", err)
	}
}
