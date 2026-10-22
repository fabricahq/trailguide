// Test that sync, update, and build install a source's skills and authored skills into every skill target.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

const (
	buildSkill  = "---\nname: build-feature\ndescription: Build a feature from a spec.\nmetadata:\n  trailguide-skill-type: playbook\n---\n# Build a feature\n\n1. Run scripts/run.sh.\n"
	reviewSkill = "---\nname: review\ndescription: Review a change.\n---\n# Review\n"
)

// skillLibrary returns a library whose release/1 publishes techs/go/errors and two skills, build-feature, with an
// executable script, and review, and the commit release/1 tags.
func skillLibrary(t *testing.T) (*gitfixture.Fixture, string) {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                                     []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`),
		"techs/go/_group.yaml":                            []byte(projectMetadata),
		"techs/go/errors.md":                              []byte(projectRule),
		".trailguide/skills/build-feature/SKILL.md":       []byte(buildSkill),
		".trailguide/skills/build-feature/scripts/run.sh": []byte("#!/bin/sh\necho build\n"),
		".trailguide/skills/review/SKILL.md":              []byte(reviewSkill),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := os.Chmod(filepath.Join(f.Worktree(), ".trailguide/skills/build-feature/scripts/run.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	commit, err := f.Commit(ctx, f.Worktree(), "Make the script executable", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	return f, commit
}

// skillProject initializes a project whose configuration is config, JSON with the fixture's repository in place of
// REPOSITORY, given the newest library release's ref when it has none, and returns its options and Git settings.
func skillProject(t *testing.T, f *gitfixture.Fixture, config string) (Options, imports.Options) {
	t.Helper()
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	git := imports.Options{GitPath: f.GitPath, Environment: f.Environment}
	writeFixture(t, root, configurationFile, strings.ReplaceAll(withNewestRef(t, config, "REPOSITORY", f.Repository, git), "REPOSITORY", quoteJSON(f.Repository)))
	return options, git
}

// readRepositoryFile returns the file at name, a path from the project's repository root.
func readRepositoryFile(t *testing.T, options Options, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(options.Directory, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// exists reports whether name, a path from the project's repository root, exists.
func exists(t *testing.T, options Options, name string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(options.Directory, filepath.FromSlash(name)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

func TestSync_InstallsSourceAndAuthoredSkillsIntoEverySkillTarget(t *testing.T) {
	ctx := context.Background()
	f, commit := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"rules":{"include":["techs/go/*"]},"skills":{"include":["*"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/check-release/SKILL.md", "---\nname: check-release\ndescription: Check a release.\n---\n# Check\n")
	changes, err := Sync(ctx, options, git)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../.claude/skills/build-feature/SKILL.md", "../.agents/skills/build-feature/scripts/run.sh", "../.claude/skills/check-release/SKILL.md", "vendor/team/skills/review/SKILL.md", "generated/SKILLS.md", "generated/installed-skills.json"} {
		if !slices.Contains(changes.Added, name) {
			t.Errorf("sync didn't report adding %s: %v", name, changes.Added)
		}
	}
	for _, target := range []string{".claude/skills", ".agents/skills"} {
		installed := readRepositoryFile(t, options, target+"/build-feature/SKILL.md")
		if want := "---\n<!-- trailguide:installed team:skills/build-feature " + commit + ". Do not edit; trailguide project sync replaces this copy. -->\n# Build"; !strings.Contains(installed, want) {
			t.Errorf("%s/build-feature/SKILL.md:\n%s\nwant the marker after the frontmatter", target, installed)
		}
		info, err := os.Stat(filepath.Join(options.Directory, target, "build-feature/scripts/run.sh"))
		if err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s run.sh isn't executable: %v %v", target, info, err)
		}
		if authored := readRepositoryFile(t, options, target+"/check-release/SKILL.md"); !strings.Contains(authored, "<!-- trailguide:installed skills/check-release. Do not edit; edit .trailguide/skills/check-release/ instead. -->") {
			t.Errorf("authored copy:\n%s", authored)
		}
	}
	rulesIndex := readRepositoryFile(t, options, ".trailguide/generated/RULES.md")
	if !strings.Contains(rulesIndex, "This project also installs agent skills. See [SKILLS.md](SKILLS.md)") {
		t.Errorf("RULES.md doesn't point to SKILLS.md:\n%s", rulesIndex)
	}
	index := readRepositoryFile(t, options, ".trailguide/generated/SKILLS.md")
	for _, want := range []string{"### build-feature", "Build a feature from a spec.", "**Skill:** `team:skills/build-feature`, from release/1, commit `" + commit + "`", "**Level:** offered by `team`, release 1", "**Type:** playbook", "[.claude/skills/build-feature/SKILL.md](../../.claude/skills/build-feature/SKILL.md)", "**Skill:** `skills/check-release`, authored in this project"} {
		if !strings.Contains(index, want) {
			t.Errorf("SKILLS.md lacks %q:\n%s", want, index)
		}
	}
	record := lockedEntry(t, options, "team")
	if record.Kind != "library" || !slices.Equal(record.Selectors.Skills, []string{"*"}) || len(record.Skills) != 2 || *record.Skills["skills/review"].Level != "offered" || !slices.Equal(record.Executable, []string{"skills/build-feature/scripts/run.sh"}) || record.Files["skills/build-feature/SKILL.md"] == "" {
		t.Errorf("source record = %+v", record)
	}
	var provenance struct {
		Skills []struct {
			ID     string `json:"id"`
			Origin struct {
				Source *string `json:"source"`
				Commit *string `json:"resolvedCommit"`
				Folder string  `json:"folder"`
			} `json:"origin"`
			Targets []string `json:"targets"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(readRepositoryFile(t, options, ".trailguide/generated/provenance.json")), &provenance); err != nil {
		t.Fatal(err)
	}
	if len(provenance.Skills) != 3 {
		t.Fatalf("provenance skills = %+v", provenance.Skills)
	}
	for _, skill := range provenance.Skills {
		switch skill.ID {
		case "team:skills/build-feature":
			if *skill.Origin.Source != "team" || *skill.Origin.Commit != commit || skill.Origin.Folder != "skills/build-feature" || !slices.Equal(skill.Targets, []string{".agents/skills/build-feature", ".claude/skills/build-feature"}) {
				t.Errorf("provenance of %s = %+v", skill.ID, skill)
			}
		case "skills/check-release":
			if skill.Origin.Source != nil || skill.Origin.Commit != nil {
				t.Errorf("provenance of %s = %+v", skill.ID, skill)
			}
		}
	}
	again, err := Build(ctx, options)
	if err != nil || len(again.Added)+len(again.Changed)+len(again.Removed) != 0 {
		t.Fatalf("build after sync = %+v, %v; want no changes", again, err)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check after sync = %+v, %v", result, err)
	}
}

func TestSync_RemovesTheFoldersOfSkillsNoLongerIncluded(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["*"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"ref":"release/1","skills":{"include":["skills/review"]}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
	changes, err := Sync(ctx, options, git)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(changes.Removed, "../.claude/skills/build-feature/scripts/run.sh") || !slices.Contains(changes.Removed, "vendor/team/skills/build-feature/SKILL.md") {
		t.Fatalf("removed = %v", changes.Removed)
	}
	if exists(t, options, ".claude/skills/build-feature") || !exists(t, options, ".claude/skills/review/SKILL.md") {
		t.Fatal("sync kept the removed skill's folder or removed the kept one")
	}
	writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"ref":"release/1","rules":{"include":["techs/go/*"]}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if exists(t, options, ".claude/skills") || exists(t, options, ".trailguide/generated/SKILLS.md") {
		t.Fatal("sync left an emptied skill target or SKILLS.md behind")
	}
}

func TestSync_RefusesToReplaceAFolderTrailGuideDidNotInstall(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/review"]}}}}`)
	personal := filepath.Join(options.Directory, ".claude/skills/review")
	if err := os.MkdirAll(personal, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personal, "SKILL.md"), []byte("My own review.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Sync(ctx, options, git)
	projectCode(t, err, "skill-folder-not-managed")
	if !strings.Contains(err.Error(), ".claude/skills/review already exists, and Trail Guide didn't install it") {
		t.Fatalf("got %v", err)
	}
	if readRepositoryFile(t, options, ".claude/skills/review/SKILL.md") != "My own review.\n" || exists(t, options, ".trailguide/vendor/team") {
		t.Fatal("refused sync changed files")
	}
}

func TestSync_InstallsSkillsFromARepositoryThatIsNotALibrary(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{
		"README.md":                   []byte("Skills.\n"),
		"tools/review/SKILL.md":       []byte(reviewSkill),
		"tools/review/checklist.md":   []byte("- Check it.\n"),
		".claude/skills/own/SKILL.md": []byte("---\nname: own\n---\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["agents"],"sources":{"skills":{"repository":REPOSITORY,"skills":{"include":["*"]}}}}`)
	_, err = Sync(ctx, options, git)
	requireRefRequired(t, err, "set sources.skills.ref in .trailguide/config.yaml to a branch, tag, or full commit SHA")
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["agents"],"sources":{"skills":{"repository":REPOSITORY,"ref":"v1.0.0","skills":{"include":["*"]}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readRepositoryFile(t, options, ".agents/skills/review/SKILL.md"), "trailguide:installed skills:tools/review "+f.FirstCommit) || !exists(t, options, ".agents/skills/review/checklist.md") || exists(t, options, ".agents/skills/own") {
		t.Fatal("sync didn't install exactly the repository's skill outside dot folders")
	}
	if record := lockedEntry(t, options, "skills"); record.Kind != "non-library" || record.Skills["tools/review"].UsedBy == nil {
		t.Fatalf("lock entry: %+v", record)
	}
	installed := readRepositoryFile(t, options, ".trailguide/generated/installed-skills.json")
	if !strings.Contains(installed, `"commit": "`+f.FirstCommit+`"`) {
		t.Fatalf("installed-skills.json:\n%s", installed)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
	writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["agents"],"sources":{"skills":{"repository":REPOSITORY,"ref":"v1.0.0","rules":{"include":["techs/go/*"]}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
	_, err = Sync(ctx, options, git)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.skills.rules" {
		t.Fatalf("got %v; want rules rejected for a repository that isn't a library", err)
	}
}

// TestSync_FailsForANewlyIncludedSkillTheLockedRevisionLacks then runs the update the failure names, which moves
// the source to the library release that has the skill.
func TestSync_FailsForANewlyIncludedSkillTheLockedRevisionLacks(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/review"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Add a skill", map[string][]byte{".trailguide/skills/plan/SKILL.md": []byte("---\nname: plan\n---\n# Plan\n")}); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"ref":"release/1","skills":{"include":["skills/plan","skills/review"]}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
	_, err = Sync(ctx, options, git)
	var failure *imports.Error
	if !errors.As(err, &failure) || failure.Code != "skill-not-in-revision" || !strings.Contains(err.Error(), "sources.team.skills.include names skills/plan, which isn't in release/1") || !strings.Contains(err.Error(), "Run trailguide project update team") {
		t.Fatalf("got %v; want skill-not-in-revision naming update team", err)
	}
	if exists(t, options, ".claude/skills/plan") {
		t.Fatal("refused sync installed the skill")
	}
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := plan.Apply(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(applied.Added, "../.claude/skills/plan/SKILL.md") || lockedEntry(t, options, "team").Release != 2 {
		t.Fatalf("update didn't move the source to release/2 and install the skill: %v", applied.Added)
	}
}

func TestSync_RecordsSkillsWithoutSkillTargetsAndWarns(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	for name, targets := range map[string]string{"empty": `"skillTargets":[],`, "missing": ""} {
		t.Run(name, func(t *testing.T) {
			options, git := skillProject(t, f, `{"schemaVersion":2,`+targets+`"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["*"]}}}}`)
			changes, err := Sync(ctx, options, git)
			if err != nil {
				t.Fatal(err)
			}
			if len(changes.Warnings) != 1 || !strings.Contains(changes.Warnings[0], "The project has no skill targets, so no copy of its 2 skill(s) was installed.") {
				t.Fatalf("warnings = %v", changes.Warnings)
			}
			if exists(t, options, ".claude") || exists(t, options, ".agents") || !exists(t, options, ".trailguide/vendor/team/skills/review/SKILL.md") {
				t.Fatal("sync installed copies or didn't record the skills")
			}
			if index := readRepositoryFile(t, options, ".trailguide/generated/SKILLS.md"); !strings.Contains(index, "**Installed at:** nowhere, because the project has no skill targets. Read it at [.trailguide/vendor/team/skills/review/SKILL.md](../vendor/team/skills/review/SKILL.md).") {
				t.Fatalf("SKILLS.md:\n%s", index)
			}
		})
	}
}

// TestSync_RenamesAClashingSkillAndWarnsThatMentionsKeepTheOldName starts with a source's skill and an authored
// skill that share a name, which fails, then renames the source's skill.
func TestSync_RenamesAClashingSkillAndWarnsThatMentionsKeepTheOldName(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/review"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/review/SKILL.md", "---\nname: review\ndescription: Our review.\n---\n# Our review\n")
	_, err = Sync(ctx, options, git)
	projectCode(t, err, "skill-name-clash")
	if !strings.Contains(err.Error(), "skills/review and team:skills/review both install as review. Rename one with skills.rename, or exclude one.") {
		t.Fatalf("clash error = %v", err)
	}
	writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"ref":"release/1","skills":{"include":["skills/review"],"rename":{"skills/review":"team-review"}}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
	const warning = "team:skills/review is installed as team-review. Mentions of review in skill text aren't rewritten; check the skills that call it by that name."
	changes, err := Sync(ctx, options, git)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changes.Warnings, []string{warning}) {
		t.Fatalf("warnings = %q", changes.Warnings)
	}
	renamed := readRepositoryFile(t, options, ".claude/skills/team-review/SKILL.md")
	if !strings.HasPrefix(renamed, "---\nname: team-review\ndescription: Review a change.\n---\n<!-- trailguide:installed team:skills/review ") || !strings.HasSuffix(renamed, "# Review\n") {
		t.Fatalf("renamed copy:\n%s", renamed)
	}
	if !strings.Contains(readRepositoryFile(t, options, ".claude/skills/review/SKILL.md"), "# Our review") {
		t.Fatal("the authored skill isn't installed under its own name")
	}
	if !strings.Contains(readRepositoryFile(t, options, ".trailguide/vendor/team/skills/review/SKILL.md"), "name: review\n") {
		t.Fatal("the rename changed the vendored copy")
	}
	if index := readRepositoryFile(t, options, ".trailguide/generated/SKILLS.md"); !strings.Contains(index, "### team-review\n\nReview a change.\n\n- **Skill:** `team:skills/review`, from release/1") || !strings.Contains(index, "- **Renamed from:** `review`, by the project's configuration. Its own text and other skills from `team` may still call it `review`; they mean this skill.") {
		t.Fatalf("SKILLS.md:\n%s", index)
	}
	if provenance := readRepositoryFile(t, options, ".trailguide/generated/provenance.json"); !strings.Contains(provenance, `"renamedFrom": "review"`) {
		t.Fatalf("provenance.json:\n%s", provenance)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
	if changes, err := Build(ctx, options); err != nil || !slices.Equal(changes.Warnings, []string{warning}) {
		t.Fatalf("build = %+v, %v; want the rename warning", changes, err)
	}
}

func TestSync_RejectsARenameOfASkillTheSourceDoesNotInstall(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/review"],"rename":{"skills/build-feature":"build"}}}}}`)
	_, err := Sync(ctx, options, git)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.team.skills.rename.skills/build-feature" || !strings.Contains(validation.ValidationProblem(), "the source doesn't install skill skills/build-feature") {
		t.Fatalf("got %v; want the rename rejected", err)
	}
	if exists(t, options, ".claude/skills") {
		t.Fatal("refused sync installed skills")
	}
}

func TestBuild_InstallsAuthoredSkillsOffline(t *testing.T) {
	ctx := context.Background()
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{}}`)
	writeFixture(t, root, "skills/tidy/SKILL.md", "---\nname: tidy\n---\n# Tidy\n")
	changes, err := Build(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(changes.Added, "../../.claude/skills/tidy/SKILL.md") || !slices.Contains(changes.Added, "SKILLS.md") {
		t.Fatalf("added = %v", changes.Added)
	}
	writeFixture(t, root, "skills/tidy/SKILL.md", "---\nname: tidy\n---\n# Tidy up\n")
	changes, err = Build(ctx, options)
	if err != nil || !slices.Contains(changes.Changed, "../../.claude/skills/tidy/SKILL.md") {
		t.Fatalf("rebuild = %+v, %v", changes, err)
	}
	if !strings.Contains(readRepositoryFile(t, options, ".claude/skills/tidy/SKILL.md"), "# Tidy up") {
		t.Fatal("build didn't replace the installed copy")
	}
}

func TestSync_RejectsASourceSkillAndAnAuthoredSkillWithOneName(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/review"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/review/SKILL.md", "---\nname: review\n---\n# Our review\n")
	_, err = Sync(ctx, options, git)
	projectCode(t, err, "skill-name-clash")
	if exists(t, options, ".claude/skills/review") {
		t.Fatal("a failed sync installed a skill")
	}
}

func TestBuild_RejectsInvalidAuthoredSkills(t *testing.T) {
	tests := map[string]struct {
		files    map[string]string
		location string
		code     string
	}{
		"file directly in skills":       {map[string]string{"skills/notes.md": "x"}, ".trailguide/skills/notes.md", ""},
		"folder without SKILL.md":       {map[string]string{"skills/tidy/notes.md": "x"}, ".trailguide/skills/tidy/", ""},
		"invalid folder name":           {map[string]string{"skills/Tidy/SKILL.md": "x"}, ".trailguide/skills/Tidy/", ""},
		"name that isn't the folder's":  {map[string]string{"skills/one/SKILL.md": "---\nname: shared\n---\n"}, "skills/one/SKILL.md", ""},
		"unknown skill type":            {map[string]string{"skills/one/SKILL.md": "---\nname: one\nmetadata:\n  trailguide-skill-type: design\n---\n"}, "skills/one/SKILL.md", ""},
		"cited rule that doesn't exist": {map[string]string{"skills/one/SKILL.md": "---\nname: one\nmetadata:\n  trailguide-rules-used: techs/go/missing\n---\n"}, "skills/one/SKILL.md", ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := openTestProject(t)
			options := Options{Directory: filepath.Dir(root.Name())}
			if _, err := Initialize(context.Background(), options); err != nil {
				t.Fatal(err)
			}
			for file, text := range test.files {
				writeFixture(t, root, file, text)
			}
			_, err := Build(context.Background(), options)
			if test.code != "" {
				projectCode(t, err, test.code)
				return
			}
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location {
				t.Fatalf("got %v; want a validation error at %s", err, test.location)
			}
		})
	}
}
