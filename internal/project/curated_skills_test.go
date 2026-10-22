// Test that a project installs the skills its library curates from other repositories, from those repositories, at
// revisions the project resolves and records itself.

package project

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

const (
	grillSkill  = "---\nname: grilling\ndescription: Ask hard questions about a plan.\n---\n# Grilling\n"
	deploySkill = "---\nname: deploy\ndescription: Deploy a service.\n---\n# Deploy\n\nRun scripts/deploy.sh.\n"
)

// curating is a project whose library, team, curates skills from pocock, a repository that isn't a library, at its
// tag stable: deploy is required, to-spec recommended, and grilling offered. to-spec uses interview, which team
// doesn't list.
type curating struct {
	team, pocock *gitfixture.Fixture
	options      Options
	git          imports.Options
	root         *os.Root
}

// teamSkills is the library.skills section of team's manifest that newCurating describes.
const teamSkills = "    required: [pocock:skills/ops/deploy]\n    recommended: [pocock:skills/engineering/to-spec]\n    offered: [pocock:skills/productivity/grilling]\n"

// newCurating returns a curating whose project uses team with config, JSON in which TEAM and POCOCK stand for the
// two repositories, and has run no command yet.
func newCurating(t *testing.T, config string) *curating {
	t.Helper()
	return newCuratingFrom(t, teamSkills, config)
}

// newCuratingFrom returns a curating as newCurating does, except that team's manifest lists listed, YAML indented
// under library.skills.
func newCuratingFrom(t *testing.T, listed, config string) *curating {
	t.Helper()
	return newCuratingWith(t, listed, nil, config)
}

// newCuratingAt returns a curating as newCuratingFrom does, except that team curates from pocock at ref.
func newCuratingAt(t *testing.T, ref, listed, config string) *curating {
	t.Helper()
	return newCurated(t, ref, listed, nil, config)
}

// newCuratingWith returns a curating as newCuratingFrom does, except that team's release also has authored, files by
// their repository paths, such as .trailguide/skills/add-skill/SKILL.md.
func newCuratingWith(t *testing.T, listed string, authored map[string][]byte, config string) *curating {
	t.Helper()
	return newCurated(t, "stable", listed, authored, config)
}

// newCurated returns a curating whose team curates from pocock at ref, lists listed, and also has authored.
func newCurated(t *testing.T, ref, listed string, authored map[string][]byte, config string) *curating {
	t.Helper()
	ctx := context.Background()
	pocock, err := gitfixture.New(ctx, map[string][]byte{
		"skills/engineering/to-spec/SKILL.md":   usingSkill("to-spec", "skills/engineering/interview"),
		"skills/engineering/interview/SKILL.md": usingSkill("interview"),
		"skills/productivity/grilling/SKILL.md": []byte(grillSkill),
		"skills/ops/deploy/SKILL.md":            []byte(deploySkill),
		"skills/ops/deploy/scripts/deploy.sh":   []byte("#!/bin/sh\necho deploy\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pocock.Close() })
	if _, err := pocock.Command(ctx, "tag", "stable", pocock.FirstCommit); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"config.yaml":          []byte("schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: " + ref + "\nlibrary:\n  schemaVersion: 1\n  skills:\n" + listed),
		"techs/go/_group.yaml": []byte(projectMetadata),
		"techs/go/errors.md":   []byte(projectRule),
	}
	maps.Copy(files, authored)
	team, err := gitfixture.New(ctx, gitfixture.Library(files))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = team.Close() })
	if err := team.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	environment, err := team.Route(map[string]*gitfixture.Fixture{"team": team, "pocock": pocock})
	if err != nil {
		t.Fatal(err)
	}
	b := &curating{team: team, pocock: pocock, git: imports.Options{GitPath: team.GitPath, Environment: environment}}
	b.root = openTestProject(t)
	b.options = Options{Directory: filepath.Dir(b.root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, b.options); err != nil {
		t.Fatal(err)
	}
	b.configure(t, config)
	return b
}

// configure writes the project's configuration, config, as newCurating takes it, giving team the newest library
// release's ref when it has none.
func (b *curating) configure(t *testing.T, config string) {
	t.Helper()
	config = withNewestRef(t, config, "TEAM", "git@fixture.invalid:team", b.git)
	config = strings.ReplaceAll(strings.ReplaceAll(config, "TEAM", quoteJSON("git@fixture.invalid:team")), "POCOCK", quoteJSON("git@fixture.invalid:pocock"))
	writeFixture(t, b.root, configurationFile, config)
}

// teamRecord returns team's lock entry, parsed.
func (b *curating) teamRecord(t *testing.T) map[string]any {
	t.Helper()
	return entryFields(t, b.options, "team")
}

func TestSync_InstallsCuratedSkillsFromTheRepositoryTheyComeFrom(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]},"skills":{"include":["pocock:skills/productivity/grilling"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deploy", "grilling"} {
		installed := readRepositoryFile(t, b.options, ".claude/skills/"+name+"/SKILL.md")
		if want := "<!-- trailguide:installed team:pocock:skills/"; !strings.Contains(installed, want) || !strings.Contains(installed, b.pocock.FirstCommit) {
			t.Errorf("%s:\n%s\nwant the curated skill's project ID and the commit its tag names", name, installed)
		}
	}
	if exists(t, b.options, ".claude/skills/to-spec") {
		t.Error("installed a recommended skill the project doesn't include")
	}
	if info, err := os.Stat(filepath.Join(b.options.Directory, ".claude/skills/deploy/scripts/deploy.sh")); err != nil || info.Mode().Perm()&0o100 != 0 {
		t.Errorf("deploy.sh: %v, %v; the fixture commits it without its executable bit", info, err)
	}
	if !exists(t, b.options, ".trailguide/_managed/vendor/team/curated/pocock/skills/ops/deploy/SKILL.md") {
		t.Error("didn't vendor the curated skill under curated/<source>/")
	}
	curated, _ := b.teamRecord(t)["curated"].(map[string]any)
	pocock, _ := curated["pocock"].(map[string]any)
	if pocock["commit"] != b.pocock.FirstCommit || pocock["ref"] != "stable" || pocock["kind"] != "non-library" || pocock["repository"] != "git@fixture.invalid:pocock" {
		t.Fatalf("curated record: %v", curated)
	}
	// The curated source's entry records its skills and files by their IDs and paths in its repository, and the
	// library's own lists leave them out.
	record := b.teamRecord(t)
	pocockSkills, _ := pocock["skills"].(map[string]any)
	pocockFiles, _ := pocock["files"].(map[string]any)
	if _, ok := pocockSkills["skills/ops/deploy"]; !ok || pocockFiles["skills/ops/deploy/SKILL.md"] == nil || pocock["executable"] == nil {
		t.Errorf("curated entry doesn't record deploy and its files: %v", pocock)
	}
	if _, ok := record["skills"].(map[string]any)["pocock:skills/ops/deploy"]; ok {
		t.Error("the library's own skills record a curated skill")
	}
	for file := range record["files"].(map[string]any) {
		if strings.HasPrefix(file, "curated/") {
			t.Errorf("the library's own files record the curated file %s", file)
		}
	}
	skillsIndex := readRepositoryFile(t, b.options, ".trailguide/_managed/generated/SKILLS.md")
	for _, want := range []string{"`team:pocock:skills/ops/deploy`, curated by `team` from git@fixture.invalid:pocock, commit `" + b.pocock.FirstCommit + "`", "**Level:** required by `team`, release 1", "**Level:** offered by `team`, release 1"} {
		if !strings.Contains(skillsIndex, want) {
			t.Errorf("SKILLS.md lacks %q:\n%s", want, skillsIndex)
		}
	}
	if result, err := Check(ctx, b.options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

func TestSync_IncludesRecommendedCuratedSkillsWithTheSkillsTheyUse(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["recommended"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deploy", "to-spec", "interview"} {
		if !exists(t, b.options, ".claude/skills/"+name+"/SKILL.md") {
			t.Errorf("didn't install %s", name)
		}
	}
	if exists(t, b.options, ".claude/skills/grilling") {
		t.Error("installed an offered skill the project doesn't include")
	}
	curated, _ := b.teamRecord(t)["curated"].(map[string]any)
	pocock, _ := curated["pocock"].(map[string]any)
	skills, _ := pocock["skills"].(map[string]any)
	interview, _ := skills["skills/engineering/interview"].(map[string]any)
	if interview["level"] != "offered" || interview["included"] != false || len(interview["usedBy"].([]any)) != 1 || interview["usedBy"].([]any)[0] != "skills/engineering/to-spec" {
		t.Fatalf("interview's record = %v; want a skill installed because to-spec uses it", interview)
	}
	if _, err := Check(ctx, b.options); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
	listing, err := ListSkills(ctx, b.options)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(listing.Skills, func(skill ListedSkill) bool { return skill.ID == "team:pocock:skills/engineering/interview" }); i < 0 || listing.Skills[i].Status != SkillInstalled || listing.Skills[i].Level != "" || listing.Skills[i].SetBy != nil {
		t.Errorf("listing = %+v; want the used curated skill installed, with no level, since the library doesn't list it", listing.Skills)
	}
}

func TestSync_InstallsTheCuratedSkillsALibrarysSkillUses(t *testing.T) {
	ctx := context.Background()
	authored := map[string][]byte{".trailguide/skills/add-skill/SKILL.md": usingSkill("add-skill", "pocock:skills/productivity/grilling")}
	b := newCuratingWith(t, teamSkills, authored, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["skills/add-skill"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"add-skill", "grilling", "deploy"} {
		if !exists(t, b.options, ".claude/skills/"+name+"/SKILL.md") {
			t.Errorf("didn't install %s", name)
		}
	}
	curated, _ := b.teamRecord(t)["curated"].(map[string]any)
	pocock, _ := curated["pocock"].(map[string]any)
	grilling, _ := pocock["skills"].(map[string]any)["skills/productivity/grilling"].(map[string]any)
	if grilling["level"] != "offered" || grilling["included"] != false || len(grilling["usedBy"].([]any)) != 0 || !reflect.DeepEqual(grilling["usedByLibrary"], []any{"skills/add-skill"}) {
		t.Fatalf("grilling's record = %v; want a skill installed because the library's add-skill uses it", grilling)
	}
	skillsIndex := readRepositoryFile(t, b.options, ".trailguide/_managed/generated/SKILLS.md")
	if !strings.Contains(skillsIndex, "**Uses:** `grilling`") {
		t.Errorf("SKILLS.md doesn't say add-skill uses grilling:\n%s", skillsIndex)
	}
	if result, err := Check(ctx, b.options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if _, preview := b.previewUpdate(t, imports.UpdateTarget{Source: "team"}); len(preview.Sources) != 1 || len(preview.Sources[0].Curated) != 0 {
		t.Errorf("update preview = %+v; want no curated changes", preview.Sources)
	}
}

func TestSync_WarnsWhenTheProjectExcludesACuratedSkillALibrarysSkillUses(t *testing.T) {
	ctx := context.Background()
	authored := map[string][]byte{".trailguide/skills/add-skill/SKILL.md": usingSkill("add-skill", "pocock:skills/productivity/grilling")}
	b := newCuratingWith(t, teamSkills, authored, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["skills/add-skill"],"exclude":[{"id":"pocock:skills/productivity/grilling","reason":"Not wanted."}]}}}}`)
	changes, err := Sync(ctx, b.options, b.git)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, b.options, ".claude/skills/grilling") {
		t.Error("installed a curated skill the project excludes")
	}
	want := "team:skills/add-skill uses team:pocock:skills/productivity/grilling, which the project excludes without a replacement, so the skill it uses isn't installed. Stop excluding the skill, or remove the use from the skill that uses it, which you can fork. A curated skill can't be replaced."
	if !slices.Contains(changes.Warnings, want) {
		t.Errorf("warnings = %q; want %q", changes.Warnings, want)
	}
}

func TestSync_RefusesACuratedSkillALibrarysSkillUsesButTheLibraryDoesNotList(t *testing.T) {
	ctx := context.Background()
	authored := map[string][]byte{".trailguide/skills/add-skill/SKILL.md": usingSkill("add-skill", "pocock:skills/engineering/interview")}
	b := newCuratingWith(t, teamSkills, authored, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["skills/add-skill"]}}}}`)
	_, err := Sync(ctx, b.options, b.git)
	if err == nil || !strings.Contains(err.Error(), "the library's skill skills/add-skill uses pocock:skills/engineering/interview, which the library doesn't list at any level or as a dependency") {
		t.Fatalf("got %v; want the unlisted curated skill refused", err)
	}
}

func TestSync_RefusesACuratedSkillTheLibraryDoesNotList(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["pocock:skills/ops/unlisted"]}}}}`)
	_, err := Sync(ctx, b.options, b.git)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.team.skills.include" || !strings.Contains(validation.ValidationProblem(), "the library doesn't list pocock:skills/ops/unlisted at any level or as a dependency") {
		t.Fatalf("got %v; want the unlisted curated skill refused", err)
	}
}

func TestSync_KeepsTheCuratedRevisionAndUpdateResolvesTheLibrarysRefAgain(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	worktree := b.pocock.Worktree()
	if _, err := b.pocock.Commit(ctx, worktree, "Improve deploy", map[string][]byte{"skills/ops/deploy/SKILL.md": []byte(strings.Replace(deploySkill, "# Deploy", "# Deploy safely", 1))}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pocock.CommandIn(ctx, worktree, "update-index", "--chmod=+x", "skills/ops/deploy/scripts/deploy.sh"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pocock.CommandIn(ctx, worktree, "commit", "--quiet", "-m", "Make deploy.sh executable"); err != nil {
		t.Fatal(err)
	}
	moved, err := b.pocock.CommandIn(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.pocock.Command(ctx, "tag", "--force", "stable", moved); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md"); strings.Contains(installed, "Deploy safely") {
		t.Fatal("sync moved the curated revision")
	}
	plan, err := PlanUpdate(ctx, b.options, b.git, []imports.UpdateTarget{{Source: "team"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Sources) != 1 || len(preview.Sources[0].Curated) != 1 {
		t.Fatalf("preview = %+v; want one curated row", preview.Sources)
	}
	if row := preview.Sources[0].Curated[0]; row.Name != "pocock" || row.Commit == nil || row.Commit.From != b.pocock.FirstCommit || row.Commit.To != moved || row.Ref != "stable" || !row.TagMoved {
		t.Fatalf("curated row = %+v", row)
	}
	changes := preview.Sources[0].Curated[0].Changes
	if len(changes) != 1 || changes[0].ID != "pocock:skills/ops/deploy" || strings.Join(changes[0].Files, ",") != "SKILL.md,scripts/deploy.sh" || len(changes[0].Scripts) != 1 || changes[0].Scripts[0].Path != "scripts/deploy.sh" {
		t.Fatalf("curated changes = %+v; want deploy's changed SKILL.md and its script made executable", changes)
	}
	if len(preview.Sources[0].SkillLevels) != 0 {
		t.Errorf("level rows = %+v; the library's levels didn't change", preview.Sources[0].SkillLevels)
	}
	if len(preview.Sources[0].Skills) != 0 {
		t.Errorf("skill rows = %+v; the curated skills are the curated row's", preview.Sources[0].Skills)
	}
	if preview.ScriptsWarning() == "" {
		t.Error("no scripts warning for a curated skill's script")
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md"); !strings.Contains(installed, "Deploy safely") || !strings.Contains(installed, moved) {
		t.Fatalf("update didn't take the curated skill from the tag's new commit:\n%s", installed)
	}
	if curated, _ := b.teamRecord(t)["curated"].(map[string]any); curated["pocock"].(map[string]any)["commit"] != moved {
		t.Fatalf("curated record: %v", curated)
	}
}

func TestSync_KeepsACuratedBranchsCommitAndUpdateTakesItsNewest(t *testing.T) {
	ctx := context.Background()
	b := newCuratingAt(t, "main", teamSkills, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	moved, err := b.pocock.Commit(ctx, b.pocock.Worktree(), "Improve deploy", map[string][]byte{"skills/ops/deploy/SKILL.md": []byte(strings.Replace(deploySkill, "# Deploy", "# Deploy safely", 1))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md"); strings.Contains(installed, "Deploy safely") || !strings.Contains(installed, b.pocock.LatestCommit) {
		t.Fatalf("sync moved the curated branch's commit:\n%s", installed)
	}
	plan, err := PlanUpdate(ctx, b.options, b.git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Sources) != 1 || len(preview.Sources[0].Curated) != 1 {
		t.Fatalf("preview = %+v; want one curated row", preview.Sources)
	}
	if row := preview.Sources[0].Curated[0]; row.Ref != "main" || row.Commit == nil || row.Commit.From != b.pocock.LatestCommit || row.Commit.To != moved || row.TagMoved {
		t.Fatalf("curated row = %+v; want main moving from %s to %s, which isn't a tag moving", row, b.pocock.LatestCommit, moved)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md"); !strings.Contains(installed, "Deploy safely") || !strings.Contains(installed, moved) {
		t.Fatalf("update didn't take the curated skill from the branch's newest commit:\n%s", installed)
	}
}

// A library source that follows its branch resolves its curated sources' refs again on update, as a library
// source without ref does, so a curated branch moves with it.
func TestUpdate_ALibraryOnABranchTakesItsCuratedBranchsNewestCommit(t *testing.T) {
	ctx := context.Background()
	b := newCuratingAt(t, "main", teamSkills, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"ref":"main","rules":{"include":["techs/go/*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	moved, err := b.pocock.Commit(ctx, b.pocock.Worktree(), "Improve deploy", map[string][]byte{"skills/ops/deploy/SKILL.md": []byte(strings.Replace(deploySkill, "# Deploy", "# Deploy safely", 1))})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(ctx, b.options, b.git, []imports.UpdateTarget{{Source: "team"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Sources) != 1 || len(preview.Sources[0].Curated) != 1 {
		t.Fatalf("preview = %+v; want one curated row", preview.Sources)
	}
	if row := preview.Sources[0].Curated[0]; row.Commit == nil || row.Commit.From != b.pocock.LatestCommit || row.Commit.To != moved {
		t.Fatalf("curated row = %+v; want main moving from %s to %s", row, b.pocock.LatestCommit, moved)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md"); !strings.Contains(installed, "Deploy safely") || !strings.Contains(installed, moved) {
		t.Fatalf("update didn't take the curated skill from the branch's newest commit:\n%s", installed)
	}
}

func TestSync_AProjectSourceForTheSameRepositoryOverridesTheLibrarysRef(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}},"mine":{"repository":POCOCK,"ref":"v1.2.0","skills":{"include":["skills/ops/deploy"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md")
	if !strings.Contains(installed, "trailguide:installed team:pocock:skills/ops/deploy "+b.pocock.LatestCommit) {
		t.Fatalf("deploy:\n%s\nwant the curated skill once, from the overriding source's revision", installed)
	}
	curated, _ := b.teamRecord(t)["curated"].(map[string]any)
	if pocock := curated["pocock"].(map[string]any); pocock["commit"] != b.pocock.LatestCommit || pocock["source"] != "mine" || pocock["ref"] != "v1.2.0" {
		t.Fatalf("curated record: %v", curated)
	}
	if result, err := Check(ctx, b.options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

func TestSync_ALibraryRepositoryInstallsTheSkillsItRequiresFromItsSources(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"pocock":{"repository":POCOCK,"ref":"stable"}},"library":{"schemaVersion":1,"skills":{"required":["pocock:skills/ops/deploy"],"offered":["pocock:skills/productivity/grilling"]}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md"); !strings.Contains(installed, "trailguide:installed pocock:skills/ops/deploy "+b.pocock.FirstCommit) {
		t.Fatalf("deploy:\n%s", installed)
	}
	if exists(t, b.options, ".claude/skills/grilling") {
		t.Error("installed a skill the library only offers")
	}
	if result, err := Check(ctx, b.options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

func TestSync_ALibraryRepositoryNeedsNoSkillsFromASourceItOnlyOffers(t *testing.T) {
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"pocock":{"repository":POCOCK,"ref":"stable"}},"library":{"schemaVersion":1,"skills":{"offered":["pocock:skills/productivity/grilling"]}}}`)
	if _, err := Sync(context.Background(), b.options, b.git); err != nil {
		t.Fatal(err)
	}
	if exists(t, b.options, ".claude/skills/grilling") {
		t.Error("installed a skill the library only offers")
	}
}

func TestSync_NeedsNoCuratedSourceForSkillsALibraryOnlyOffers(t *testing.T) {
	ctx := context.Background()
	b := newCuratingFrom(t, "    offered: [pocock:skills/productivity/grilling]\n", `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	record := b.teamRecord(t)
	if _, ok := record["curated"]; ok {
		t.Errorf("recorded a curated source the project takes no skills from: %v", record["curated"])
	}
	if available, _ := record["availableSkills"].(map[string]any); available["pocock:skills/productivity/grilling"] == nil {
		t.Errorf("availableSkills = %v; want the offered curated skill", available)
	}
	if result, err := Check(ctx, b.options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatalf("second sync: %v", err)
	}
}

func TestSync_UsesOfAnOverridingSourcesCopyReachTheCuratedSkill(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}},"mine":{"repository":POCOCK,"ref":"stable","skills":{"include":["skills/ops/deploy"]}}}}`)
	writeFixture(t, b.root, "skills/ship/SKILL.md", string(usingSkill("ship", "mine:skills/ops/deploy")))
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	skillsIndex := readRepositoryFile(t, b.options, ".trailguide/_managed/generated/SKILLS.md")
	if !strings.Contains(skillsIndex, "`team:pocock:skills/ops/deploy`") || strings.Contains(skillsIndex, "`mine:skills/ops/deploy`") {
		t.Fatalf("SKILLS.md:\n%s\nwant deploy once, as the curated skill", skillsIndex)
	}
	if !strings.Contains(skillsIndex, "**Uses:** `deploy`") {
		t.Errorf("SKILLS.md doesn't say ship uses deploy:\n%s", skillsIndex)
	}
	if result, err := Check(ctx, b.options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
}

// releaseTeam publishes release/2 of team with listed, YAML indented under library.skills, or no curated source when
// listed is empty, and techs/go/errors at 1.1.0.
func (b *curating) releaseTeam(t *testing.T, listed string) {
	t.Helper()
	ctx := context.Background()
	config := "schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 1}\n"
	if listed != "" {
		config = "schemaVersion: 2\nsources:\n  pocock:\n    repository: git@fixture.invalid:pocock\n    ref: stable\nlibrary:\n  schemaVersion: 1\n  skills:\n" + listed
	}
	files := gitfixture.Library(map[string][]byte{"config.yaml": []byte(config), "techs/go/errors.md": []byte(projectRule + "\nWrap them with context.\n")})
	if _, err := b.team.Commit(ctx, b.team.Worktree(), "Release 2", files); err != nil {
		t.Fatal(err)
	}
	if err := b.team.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\n"); err != nil {
		t.Fatal(err)
	}
}

// previewUpdate plans updating targets and returns the preview.
func (b *curating) previewUpdate(t *testing.T, targets ...imports.UpdateTarget) (*UpdatePlan, UpdateResult) {
	t.Helper()
	ctx := context.Background()
	plan, err := PlanUpdate(ctx, b.options, b.git, targets, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return plan, preview
}

func TestUpdate_PreviewsTheCuratedSkillsALibraryStopsListing(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	b.releaseTeam(t, "")
	plan, preview := b.previewUpdate(t, imports.UpdateTarget{Source: "team"})
	curated := preview.Sources[0].Curated
	if len(curated) != 1 || curated[0].Name != "pocock" || curated[0].Commit == nil || curated[0].Commit.From != b.pocock.FirstCommit || curated[0].Commit.To != "" || len(curated[0].Changes) != 1 || curated[0].Changes[0].ID != "pocock:skills/ops/deploy" || curated[0].Changes[0].Change != imports.SkillRemoved {
		t.Fatalf("curated rows = %+v; want deploy removed with its source", curated)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if exists(t, b.options, ".claude/skills/deploy") {
		t.Error("the update kept a skill the library no longer lists")
	}
}

func TestUpdate_AScopedUpdatePreviewsTheCuratedSkillsOfTheRevisionItTakes(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	b.releaseTeam(t, "    required: [pocock:skills/ops/deploy, pocock:skills/productivity/grilling]\n")
	plan, preview := b.previewUpdate(t, imports.UpdateTarget{Source: "team", Rule: "techs/go/errors"})
	curated := preview.Sources[0].Curated
	if len(curated) != 1 || curated[0].Commit != nil || len(curated[0].Changes) != 1 || curated[0].Changes[0].ID != "pocock:skills/productivity/grilling" || curated[0].Changes[0].Change != imports.SkillAdded {
		t.Fatalf("curated rows = %+v; want grilling added at the recorded commit", curated)
	}
	kept, err := plan.Preview(ctx, []UpdateDecision{{Kind: DecisionKeep, Source: "team", Rule: "techs/go/errors", Reason: "Not yet."}})
	if err != nil {
		t.Fatal(err)
	}
	if rows := kept.Sources[0].Curated; len(rows) != 0 {
		t.Errorf("curated rows after keeping the rule = %+v; the shared files, and the curated skills, stay", rows)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if !exists(t, b.options, ".claude/skills/grilling/SKILL.md") {
		t.Error("the update didn't install the skill release 2 requires")
	}
}

func TestUpdate_PreviewsTheCuratedSkillsAnOverridingSourceMoves(t *testing.T) {
	t.Run("declared before the last sync", func(t *testing.T) { testOverridingSourceMoves(t, false) })
	t.Run("declared since the last sync", func(t *testing.T) { testOverridingSourceMoves(t, true) })
}

// testOverridingSourceMoves updates mine, a project source that overrides the repository team curates deploy from,
// after kit, that repository, publishes a release that changes deploy. declaredLater adds mine only after the sync.
func testOverridingSourceMoves(t *testing.T, declaredLater bool) {
	ctx := context.Background()
	kit, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                        []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 1\n  skills:\n    offered: [skills/deploy]\n"),
		"techs/go/_group.yaml":               []byte(projectMetadata),
		"techs/go/errors.md":                 []byte(projectRule),
		".trailguide/skills/deploy/SKILL.md": []byte(deploySkill),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kit.Close() })
	if err := kit.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	team, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte("schemaVersion: 2\nsources:\n  kit:\n    repository: git@fixture.invalid:kit\nlibrary:\n  schemaVersion: 1\n  skills:\n    required: [kit:skills/deploy]\n"),
		"techs/go/_group.yaml": []byte(projectMetadata),
		"techs/go/errors.md":   []byte(projectRule),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = team.Close() })
	if err := team.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	environment, err := team.Route(map[string]*gitfixture.Fixture{"team": team, "kit": kit})
	if err != nil {
		t.Fatal(err)
	}
	git := imports.Options{GitPath: team.GitPath, Environment: environment}
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	teamSource := `"team":{"repository":"git@fixture.invalid:team","ref":"release/1","rules":{"include":["techs/go/*"]}}`
	mine := `"mine":{"repository":"git@fixture.invalid:kit","ref":"release/1","rules":{"include":["techs/go/*"]}}`
	if declaredLater {
		writeFixture(t, root, configurationFile, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{`+teamSource+`}}`)
	} else {
		writeFixture(t, root, configurationFile, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{`+teamSource+`,`+mine+`}}`)
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if declaredLater {
		writeFixture(t, root, configurationFile, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{`+teamSource+`,`+mine+`}}`)
	}
	if _, err := kit.Commit(ctx, kit.Worktree(), "Improve deploy", gitfixture.Library(map[string][]byte{".trailguide/skills/deploy/SKILL.md": []byte(strings.Replace(deploySkill, "# Deploy", "# Deploy safely", 1))})); err != nil {
		t.Fatal(err)
	}
	if err := kit.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\nchanges: {}\n"); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "mine"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Sources) != 2 || preview.Sources[1].Name != "team" || len(preview.Sources[1].Curated) != 1 {
		t.Fatalf("preview = %+v; want team's curated row after mine, in configuration order", preview.Sources)
	}
	if row := preview.Sources[1].Curated[0]; row.Name != "kit" || len(row.Changes) != 1 || row.Changes[0].ID != "kit:skills/deploy" || strings.Join(row.Changes[0].Files, ",") != "SKILL.md" {
		t.Fatalf("curated row = %+v; want deploy's changed SKILL.md", row)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, options, ".claude/skills/deploy/SKILL.md"); !strings.Contains(installed, "Deploy safely") {
		t.Fatalf("deploy:\n%s", installed)
	}
}

func TestSync_ALibraryRepositoryCannotExcludeASkillItRequires(t *testing.T) {
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"pocock":{"repository":POCOCK,"ref":"stable","skills":{"include":["skills/productivity/grilling"],"exclude":[{"id":"skills/ops/deploy","reason":"Not here."}]}}},"library":{"schemaVersion":1,"skills":{"required":["pocock:skills/ops/deploy"]}}}`)
	_, err := Sync(context.Background(), b.options, b.git)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.pocock.skills.exclude.skills/ops/deploy" || !strings.Contains(validation.ValidationProblem(), "library.skills.required lists pocock:skills/ops/deploy") {
		t.Fatalf("got %v; want excluding a skill the library requires refused", err)
	}
}

func TestCheck_RejectsACuratedSkillInTheLibrarysOwnRecord(t *testing.T) {
	ctx := context.Background()
	b := newCurating(t, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	// Move deploy into the library's own skills, under its ID in the library, with an integrity that matches.
	rewriteEntry(t, b.root, "team", func(record *sourceRecord) {
		pocock := record.Curated["pocock"]
		record.Skills["pocock:skills/ops/deploy"] = pocock.Skills["skills/ops/deploy"]
		delete(pocock.Skills, "skills/ops/deploy")
		pocock.Skills["skills/ops/other"] = recordSkill{Level: nullableLevel("offered"), Included: true, UsedBy: []string{}}
	})
	if _, err := Check(ctx, b.options); err == nil || !strings.Contains(err.Error(), "a curated skill is recorded in its curated source's skills") {
		t.Fatalf("check = %v; want the curated skill in the library's own skills refused", err)
	}
}

func TestSync_InstallsALevelExcludedCuratedSkillThatAnInstalledSkillUses(t *testing.T) {
	ctx := context.Background()
	listed := "    required: [pocock:skills/ops/deploy, pocock:skills/engineering/to-spec]\n    recommended: [pocock:skills/engineering/interview]\n    offered: [pocock:skills/productivity/grilling]\n"
	b := newCuratingFrom(t, listed, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"skills":{"include":["*"],"exclude":[{"id":"*","reason":"Only what the library requires."}]}}}}`)
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deploy", "to-spec", "interview"} {
		if !exists(t, b.options, ".claude/skills/"+name+"/SKILL.md") {
			t.Errorf("didn't install %s", name)
		}
	}
	if exists(t, b.options, ".claude/skills/grilling") {
		t.Error("installed an offered skill the project excludes by level")
	}
	curated, _ := b.teamRecord(t)["curated"].(map[string]any)
	pocock, _ := curated["pocock"].(map[string]any)
	skills, _ := pocock["skills"].(map[string]any)
	if interview, _ := skills["skills/engineering/interview"].(map[string]any); interview["included"] != false || len(interview["usedBy"].([]any)) != 1 {
		t.Fatalf("interview's record = %v; want a skill installed only because to-spec uses it", interview)
	}
	if _, err := Check(ctx, b.options); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
}

func TestSync_CuratesFromABranchNamedLikeALibraryRelease(t *testing.T) {
	ctx := context.Background()
	b := newCuratingAt(t, "release/5", teamSkills, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	if _, err := b.pocock.Command(ctx, "branch", "release/5"); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, b.options, b.git); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, b.options, ".claude/skills/deploy/SKILL.md"); !strings.Contains(installed, b.pocock.LatestCommit) {
		t.Fatalf("sync didn't take the curated skill from release/5's newest commit:\n%s", installed)
	}
}

func TestSync_ACuratedBareReleaseNameOfABranchAndATagIsAmbiguous(t *testing.T) {
	ctx := context.Background()
	b := newCuratingAt(t, "release/5", teamSkills, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":TEAM,"rules":{"include":["techs/go/*"]}}}}`)
	for _, args := range [][]string{{"branch", "release/5"}, {"tag", "release/5"}} {
		if _, err := b.pocock.Command(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Sync(ctx, b.options, b.git)
	requireFailure(t, err, "ambiguous-ref", "both a branch and a tag named release/5.")
}
