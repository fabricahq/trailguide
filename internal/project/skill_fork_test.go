// Test that a source's skills.exclude leaves skills out, and that a fork installs in place of the skill it replaces
// and has its source's later changes listed by update.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// skillConfig is a project configuration that installs every skill of the fixture's library into .claude/skills/,
// followed by exclude, the YAML of a skills.exclude list or empty.
func skillConfig(exclude string) string {
	return "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: REPOSITORY\n    skills:\n      include: ['*']\n" + exclude
}

// forkSkill plans and commits a fork of from with reason.
func forkSkill(t *testing.T, options Options, from, reason string) (*SkillForkPlan, AuthoringResult) {
	t.Helper()
	source, err := ParseSkillForkSource(from)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanSkillFork(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Commit(context.Background(), reason)
	if err != nil {
		t.Fatal(err)
	}
	return plan, result
}

// TestSync_LeavesOutAnExcludedSkill installs the source's other skills, keeps the excluded skill's files in vendor/
// so updates can compare them, and lists it with its reason in SKILLS.md.
func TestSync_LeavesOutAnExcludedSkill(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, skillConfig("      exclude:\n        - id: skills/review\n          reason: We review in CI.\n"))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if exists(t, options, ".claude/skills/review") || !exists(t, options, ".claude/skills/build-feature/SKILL.md") {
		t.Fatal("sync installed the excluded skill, or not the other one")
	}
	if !exists(t, options, ".trailguide/_managed/vendor/team/skills/library/review/SKILL.md") {
		t.Fatal("sync didn't keep the excluded skill's files in vendor/")
	}
	index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md")
	if !strings.Contains(index, "## Excluded skills") || !strings.Contains(index, "- `team:skills/review`: We review in CI.") || strings.Contains(index, "### review") {
		t.Fatalf("SKILLS.md doesn't list the excluded skill with its reason:\n%s", index)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check after sync = %+v, %v", result, err)
	}
}

// TestSync_RejectsAnExclusionOfASkillTheSourceDoesNotInstall fails before writing, naming the entry.
func TestSync_RejectsAnExclusionOfASkillTheSourceDoesNotInstall(t *testing.T) {
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: REPOSITORY\n    skills:\n      include: [skills/review]\n      exclude:\n        - id: skills/build-feature\n          reason: Not ours.\n")
	_, err := Sync(context.Background(), options, git)
	var invalid errs.ValidationError
	if !errors.As(err, &invalid) || invalid.ValidationLocation() != "sources.team.skills.exclude.skills/build-feature" || !strings.Contains(invalid.ValidationProblem(), "the source doesn't install skill skills/build-feature") {
		t.Fatalf("got %v; want a validation error naming the exclusion", err)
	}
	if exists(t, options, ".claude/skills/review") {
		t.Fatal("refused sync installed skills")
	}
}

// TestPlanSkillFork_InstallsTheForkInPlaceOfTheSkill copies the vendored skill with its executable bit, records the
// exclusion in the same write, and installs the fork in the original's place on build, offline.
func TestPlanSkillFork_InstallsTheForkInPlaceOfTheSkill(t *testing.T) {
	ctx := context.Background()
	f, commit := skillLibrary(t)
	options, git := skillProject(t, f, skillConfig(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	plan, result := forkSkill(t, options, "team:skills/build-feature", "We build with make.")
	if plan.Name() != "build-feature" || plan.BasedOn() != commit {
		t.Fatalf("plan = %s based on %s", plan.Name(), plan.BasedOn())
	}
	trailguide := filepath.Join(options.Directory, ".trailguide")
	if want := []string{filepath.Join(trailguide, "skills/team/build-feature/SKILL.md"), filepath.Join(trailguide, "skills/team/build-feature/scripts/run.sh")}; !reflect.DeepEqual(result.Added, want) || !reflect.DeepEqual(result.Changed, []string{filepath.Join(trailguide, "config.yaml")}) {
		t.Fatalf("result = %+v", result)
	}
	if fork := readRepositoryFile(t, options, ".trailguide/skills/team/build-feature/SKILL.md"); fork != buildSkill {
		t.Fatalf("fork's SKILL.md:\n%s", fork)
	}
	if info, err := os.Stat(filepath.Join(options.Directory, ".trailguide/skills/team/build-feature/scripts/run.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the fork's script isn't executable: %v %v", info, err)
	}
	data := []byte(readRepositoryFile(t, options, ".trailguide/config.yaml"))
	if want := "      forks:\n        skills/build-feature:\n          as: skills/build-feature\n          basedOn: \"" + commit + "\"\n          reason: We build with make.\n"; !strings.Contains(string(data), want) {
		t.Fatalf("config.yaml:\n%s\nwant:\n%s", data, want)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, options, ".claude/skills/build-feature/SKILL.md"); !strings.Contains(installed, "<!-- trailguide:installed skills/build-feature. Do not edit; edit .trailguide/skills/team/build-feature/ instead. -->") {
		t.Fatalf("installed copy isn't the fork:\n%s", installed)
	}
	index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md")
	for _, want := range []string{"**Skill:** `skills/build-feature`, authored in this project", "**Replaces:** `team:skills/build-feature`, which the project forks, based on its copy at commit `" + commit + "`", "**Reason:** We build with make."} {
		if !strings.Contains(index, want) {
			t.Errorf("SKILLS.md lacks %q:\n%s", want, index)
		}
	}
	if strings.Count(index, "**Replaces:**") != 1 {
		t.Errorf("SKILLS.md names a replacement for a skill the project doesn't fork:\n%s", index)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check after build = %+v, %v", result, err)
	}
	// A teammate's sync takes the same commit and finds nothing to change.
	if again, err := Sync(ctx, options, git); err != nil || len(again.Added)+len(again.Changed)+len(again.Removed) != 0 {
		t.Fatalf("sync after build = %+v, %v; want no changes", again, err)
	}
}

// TestPlanSkillFork_RefusesWhatItCannotFork fails before writing for each target it never replaces or can't read.
func TestPlanSkillFork_RefusesWhatItCannotFork(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, skillConfig("      exclude:\n        - id: skills/review\n          reason: We review in CI.\n"))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/local/build-feature/SKILL.md", "---\nname: build-feature\ndescription: Ours.\n---\n# Build\n")
	for _, test := range []struct {
		from, location, problem string
	}{
		{"other:skills/review", "--from", "no source named other in .trailguide/config.yaml"},
		{"team:skills/missing", "--from", "source team doesn't install skill skills/missing, as its last sync recorded"},
		{"team:skills/review", "sources.team.skills.exclude.skills/review", "the source already excludes this skill"},
	} {
		source, err := ParseSkillForkSource(test.from)
		if err != nil {
			t.Fatal(err)
		}
		_, err = PlanSkillFork(ctx, source, options)
		var invalid errs.ValidationError
		if !errors.As(err, &invalid) || invalid.ValidationLocation() != test.location || !strings.Contains(invalid.ValidationProblem(), test.problem) {
			t.Errorf("fork %s: got %v; want %s: %s", test.from, err, test.location, test.problem)
		}
	}
	source, _ := ParseSkillForkSource("team:skills/build-feature")
	_, err = PlanSkillFork(ctx, source, options)
	projectCode(t, err, "already-exists")
	if _, err := ParseSkillForkSource("skills/review"); err == nil || !strings.Contains(err.Error(), "expected SOURCE:SKILL") {
		t.Fatalf("parse without a source = %v", err)
	}
}

// TestPlanSkillFork_ForksARenamedSkillUnderItsInstallName names the fork and its SKILL.md by the rename, and swaps the
// rename entry for the exclusion, so build installs the fork where agents already found the skill.
func TestPlanSkillFork_ForksARenamedSkillUnderItsInstallName(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, skillConfig("      rename:\n        skills/review: team-review\n"))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	plan, _ := forkSkill(t, options, "team:skills/review", "Ours.")
	if plan.Name() != "team-review" {
		t.Fatalf("fork name = %s; want team-review", plan.Name())
	}
	if fork := readRepositoryFile(t, options, ".trailguide/skills/team/team-review/SKILL.md"); !strings.Contains(fork, "\nname: team-review\n") || strings.Contains(fork, "name: review\n") {
		t.Fatalf("fork's SKILL.md:\n%s", fork)
	}
	config := readRepositoryFile(t, options, ".trailguide/config.yaml")
	if strings.Contains(config, "rename") || !strings.Contains(config, "as: skills/team-review") {
		t.Fatalf("config.yaml:\n%s", config)
	}
	changes, err := Build(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if got := installedSkills(t, options); !slices.Contains(got, "team-review") || slices.Contains(got, "review") {
		t.Fatalf("installed %v; want the fork as team-review", got)
	}
	for _, warning := range changes.Warnings {
		if strings.Contains(warning, "is installed as") {
			t.Fatalf("build still warns about a rename: %q", warning)
		}
	}
}

// TestPlanSkillFork_CommitRefusesABlankReasonAndAConcurrentChange writes nothing in either case.
func TestPlanSkillFork_CommitRefusesABlankReasonAndAConcurrentChange(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, skillConfig(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	source, _ := ParseSkillForkSource("team:skills/review")
	plan, err := PlanSkillFork(ctx, source, options)
	if err != nil {
		t.Fatal(err)
	}
	var invalid errs.ValidationError
	if _, err := plan.Commit(ctx, " "); !errors.As(err, &invalid) || invalid.ValidationLocation() != "--reason" {
		t.Fatalf("blank reason = %v", err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, strings.ReplaceAll("# Edited.\n"+skillConfig(""), "REPOSITORY", quoteJSON(f.Repository)))
	_, err = plan.Commit(ctx, "Ours.")
	projectCode(t, err, "concurrent-change")
	if exists(t, options, ".trailguide/skills/team/review") {
		t.Fatal("a refused fork wrote files")
	}
}

// TestPlanSkillFork_CommitRefusesASourceAnUpdateMovedAfterPlanning writes nothing when an update, which leaves the
// configuration as it was, moves the source to a commit with another copy of the skill between plan and commit.
func TestPlanSkillFork_CommitRefusesASourceAnUpdateMovedAfterPlanning(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, skillConfig(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	source, _ := ParseSkillForkSource("team:skills/build-feature")
	plan, err := PlanSkillFork(ctx, source, options)
	if err != nil {
		t.Fatal(err)
	}
	secondSkillRelease(t, f)
	config := readRepositoryFile(t, options, ".trailguide/config.yaml")
	update, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := update.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	// The update moves only the source's ref, so the record alone changes what the fork is based on.
	updated := readRepositoryFile(t, options, ".trailguide/config.yaml")
	if strings.Replace(config, "release/1", "release/2", 1) != updated {
		t.Fatalf("the update changed more than the source's ref, so the test doesn't isolate the source record:\n%s", updated)
	}
	config = updated
	_, err = plan.Commit(ctx, "We build with make.")
	projectCode(t, err, "concurrent-change")
	if exists(t, options, ".trailguide/skills/team/build-feature") || readRepositoryFile(t, options, ".trailguide/config.yaml") != config {
		t.Fatal("a refused fork wrote files")
	}
}

// secondSkillRelease publishes release/2, which changes build-feature's SKILL.md and adds a script to it.
func secondSkillRelease(t *testing.T, f *gitfixture.Fixture) string {
	t.Helper()
	ctx := context.Background()
	commit, err := f.Commit(ctx, f.Worktree(), "Change a skill", map[string][]byte{
		".trailguide/skills/library/build-feature/SKILL.md":         []byte(strings.Replace(buildSkill, "Run scripts/run.sh.", "Run scripts/check.sh first.", 1)),
		".trailguide/skills/library/build-feature/scripts/check.sh": []byte("#!/bin/sh\necho check\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	return commit
}

// TestUpdate_ListsChangesToAReplacedSkill compares the source's newer copy of a replaced skill with the copy its
// replacement is based on, or, without basedOn, the locked copy, and leaves the replacement and basedOn alone.
func TestUpdate_ListsChangesToAReplacedSkill(t *testing.T) {
	ctx := context.Background()
	for _, recorded := range []bool{true, false} {
		t.Run(map[bool]string{true: "basedOn", false: "locked commit"}[recorded], func(t *testing.T) {
			f, commit := skillLibrary(t)
			options, git := skillProject(t, f, skillConfig(""))
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			forkSkill(t, options, "team:skills/build-feature", "We build with make.")
			basedOn := commit
			if !recorded {
				root, err := openProject(ctx, options, false)
				if err != nil {
					t.Fatal(err)
				}
				config := readRepositoryFile(t, options, ".trailguide/config.yaml")
				writeFixture(t, root, configurationFile, strings.Replace(config, "          basedOn: \""+commit+"\"\n", "", 1))
				root.Close()
				basedOn = ""
			}
			if _, err := Build(ctx, options); err != nil {
				t.Fatal(err)
			}
			later := secondSkillRelease(t, f)
			plan, err := PlanUpdate(ctx, options, git, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			preview, err := plan.Preview(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := []imports.ReplacedSkillUpdate{{ID: "skills/build-feature", LocalSkill: "skills/build-feature", BasedOn: basedOn, From: commit, To: later, Files: []string{"SKILL.md", "scripts/check.sh"}}}
			// Only a fork with basedOn has an unedited copy to merge the source's changes into.
			if recorded {
				want[0].Merge = imports.SkillMergeNoConflicts
			}
			if len(preview.Sources) != 1 || !reflect.DeepEqual(visibleSkillRows(preview.Sources[0].ReplacedSkills), want) {
				t.Fatalf("preview = %+v\nwant replaced skills %+v", preview.Sources, want)
			}
			fork := readRepositoryFile(t, options, ".trailguide/skills/team/build-feature/SKILL.md")
			config := readRepositoryFile(t, options, ".trailguide/config.yaml")
			if _, err := plan.Apply(ctx, nil); err != nil {
				t.Fatal(err)
			}
			// The update moves only the source's ref to the release it installs.
			if readRepositoryFile(t, options, ".trailguide/skills/team/build-feature/SKILL.md") != fork || readRepositoryFile(t, options, ".trailguide/config.yaml") != strings.Replace(config, "ref: release/1", "ref: release/2", 1) {
				t.Fatal("the update changed the fork or its exclusion")
			}
			if installed := readRepositoryFile(t, options, ".claude/skills/build-feature/SKILL.md"); !strings.Contains(installed, "edit .trailguide/skills/team/build-feature/ instead") {
				t.Fatalf("after the update, the installed copy isn't the fork:\n%s", installed)
			}
		})
	}
}

// TestSync_ChecksTheCommitAForkIsBasedOn fails with version-not-found for a commit the repository lacks, and
// with a validation error for a commit without the skill.
func TestSync_ChecksTheCommitAForkIsBasedOn(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	// A commit after release/1 removes the skill, so the repository has the commit but the commit lacks the skill.
	without, err := f.Commit(ctx, f.Worktree(), "Remove a skill", map[string][]byte{".trailguide/skills/library/build-feature/SKILL.md": nil, ".trailguide/skills/library/build-feature/scripts/run.sh": nil})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, basedOn string
		check         func(t *testing.T, err error)
	}{
		{"missing commit", strings.Repeat("a", 40), func(t *testing.T, err error) {
			var failure *imports.Error
			if !errors.As(err, &failure) || failure.Code != "version-not-found" || !strings.Contains(err.Error(), "sources.team.skills.forks.skills/build-feature.basedOn: the repository has no commit "+strings.Repeat("a", 40)) {
				t.Fatalf("got %v; want version-not-found naming basedOn", err)
			}
		}},
		{"commit without the skill", without, func(t *testing.T, err error) {
			var invalid errs.ValidationError
			if !errors.As(err, &invalid) || invalid.ValidationLocation() != "sources.team.skills.forks.skills/build-feature.basedOn" || !strings.Contains(invalid.ValidationProblem(), "commit "+without+" has no skill skills/build-feature") {
				t.Fatalf("got %v; want a validation error naming basedOn", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, git := skillProject(t, f, skillConfig("      forks:\n        skills/build-feature:\n          as: skills/build-feature\n          basedOn: \""+test.basedOn+"\"\n          reason: Ours.\n"))
			root, err := openProject(ctx, options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			writeFixture(t, root, "skills/team/build-feature/SKILL.md", buildSkill)
			_, err = Sync(ctx, options, git)
			test.check(t, err)
			if exists(t, options, ".claude/skills") {
				t.Fatal("refused sync installed skills")
			}
		})
	}
}

// TestForkedSkillFile_NamesTheSourceInAForkedSkillsCitations keeps a fork of a source's skill citing the source's
// rules and skills, and using the skills skills.uses adds, which in the project need the source's name, and gives a
// SKILL.md without a name one.
func TestForkedSkillFile_NamesTheSourceInAForkedSkillsCitations(t *testing.T) {
	const cited = "---\nname: fix-bug\nmetadata:\n  trailguide-skill-type: playbook\n  trailguide-rules-used: |\n    practices/testing/tdd\n    other:techs/go/errors\n  trailguide-skills-used: |-\n    skills/tdd\n---\n\nskills/tdd stays as written here.\n"
	const qualified = "---\nname: fix-bug\nmetadata:\n  trailguide-skill-type: playbook\n  trailguide-rules-used: |\n    team:practices/testing/tdd\n    other:techs/go/errors\n  trailguide-skills-used: |\n    team:skills/tdd\n---\n\nskills/tdd stays as written here.\n"
	for _, test := range []struct {
		name, original string
		library        bool
		uses           []string
		want           string
	}{
		{"library", cited, true, nil, qualified},
		{"repository that isn't a library", cited, false, nil, strings.Replace(qualified, "team:practices/testing/tdd", "practices/testing/tdd", 1)},
		{"curated skill", "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |-\n    anthropic:skills/skill-creator\n---\n", true, nil, "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |\n    team:anthropic:skills/skill-creator\n---\n"},
		{"nothing to qualify", "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |-\n    other:skills/tdd\n---\n", false, nil, "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |-\n    other:skills/tdd\n---\n"},
		{"quoted list", "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: \"skills/tdd\"\n  trailguide-skill-type: playbook\n---\n", true, nil, "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |\n    team:skills/tdd\n  trailguide-skill-type: playbook\n---\n"},
		{"plain list on two lines", "---\nname: fix-bug\nmetadata:\n    trailguide-skills-used:\n      skills/tdd\n---\n", true, nil, "---\nname: fix-bug\nmetadata:\n    trailguide-skills-used: |\n      team:skills/tdd\n---\n"},
		{"configured uses", "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |\n    skills/tdd\n---\n", true, []string{"skills/plan", "skills/tdd"}, "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |\n    team:skills/tdd\n    team:skills/plan\n---\n"},
		{"configured uses without metadata", "---\nname: fix-bug\n---\n# Fix\n", true, []string{"skills/plan"}, "---\nname: fix-bug\nmetadata:\n  trailguide-skills-used: |\n    team:skills/plan\n---\n# Fix\n"},
		{"no frontmatter", "# Fix\n", false, []string{"tools/plan"}, "---\nname: \"fix-bug\"\nmetadata:\n  trailguide-skills-used: |\n    team:tools/plan\n---\n# Fix\n"},
		{"no name", "---\r\ndescription: Fix it.\r\n---\r\n# Fix\r\n", false, nil, "---\r\nname: \"fix-bug\"\r\ndescription: Fix it.\r\n---\r\n# Fix\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := skills.ReadMetadata([]byte(test.original), "SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			got, fixes, err := forkedSkillFile([]byte(test.original), metadata, "fix-bug", "team", test.library, test.uses, "SKILL.md")
			if err != nil || len(fixes) > 0 || string(got) != test.want {
				t.Errorf("got %v, fixes %q:\n%s\nwant:\n%s", err, fixes, got, test.want)
			}
		})
	}
}

// TestForkedSkillFile_ListsWhatItCannotRewrite returns the SKILL.md with what the author must fix, rather than
// refuse, when the skill's citations would name the project's own skills, or when build would refuse the fork.
func TestForkedSkillFile_ListsWhatItCannotRewrite(t *testing.T) {
	for _, test := range []struct {
		name, original, fix string
	}{
		{"flow mapping", "---\nname: fix-bug\nmetadata: {trailguide-skills-used: \"skills/tdd\"}\n---\n", "Name the skill fix-bug, and cite the source's skills and rules with the source's name, such as team:skills/tdd"},
		{"invalid type", "---\nname: fix-bug\nmetadata:\n  trailguide-skill-type: workflow\n---\n", "Fix what build reports: trailguide-skill-type is \"workflow\"; use playbook or review"},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := skills.ReadMetadata([]byte(test.original), "SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			got, fixes, err := forkedSkillFile([]byte(test.original), metadata, "fix-bug", "team", true, nil, "SKILL.md")
			if err != nil || len(fixes) != 1 || !strings.Contains(fixes[0], test.fix) || string(got) != test.original {
				t.Fatalf("got %v, fixes %q:\n%s\nwant a fix saying %q", err, fixes, got, test.fix)
			}
		})
	}
}

// TestPlanSkillFork_ForksASkillWithoutFrontmatter installs the fork of a skill from a repository that isn't a
// library, whose SKILL.md has no frontmatter, since the fork gets frontmatter that names it.
func TestPlanSkillFork_ForksASkillWithoutFrontmatter(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{"tools/review/SKILL.md": []byte("# Review\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"tools":{"repository":REPOSITORY,"ref":"v1.0.0","skills":{"include":["*"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	forkSkill(t, options, "tools:tools/review", "Ours.")
	if fork := readRepositoryFile(t, options, ".trailguide/skills/tools/review/SKILL.md"); fork != "---\nname: \"review\"\n---\n# Review\n" {
		t.Fatalf("fork's SKILL.md:\n%s", fork)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, options, ".claude/skills/review/SKILL.md"); !strings.Contains(installed, "edit .trailguide/skills/tools/review/ instead") {
		t.Fatalf("installed copy isn't the fork:\n%s", installed)
	}
}

// TestPlanSkillFork_KeepsTheSkillsThatSkillsUsesAdds names in the fork's trailguide-skills-used the skill that the
// source's skills.uses adds for the forked skill, which applies only to the source's own skill.
func TestPlanSkillFork_KeepsTheSkillsThatSkillsUsesAdds(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/build-feature"],"uses":{"skills/build-feature":["skills/review"]}}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	forkSkill(t, options, "team:skills/build-feature", "We build with make.")
	if fork := readRepositoryFile(t, options, ".trailguide/skills/team/build-feature/SKILL.md"); !strings.Contains(fork, "  trailguide-skills-used: |\n    team:skills/review\n") {
		t.Fatalf("fork's SKILL.md:\n%s", fork)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md")
	if !strings.Contains(index, "- **Uses:** `review`") {
		t.Fatalf("SKILLS.md doesn't say the fork uses review:\n%s", index)
	}
}

// TestUpdate_ListsAReplacedSkillOnlyAtTheRevisionTheDecisionsChoose keeps the only rule a scoped update moves, so
// the skills stay at release/1, and the preview no longer lists release/2's changes to the forked skill.
func TestUpdate_ListsAReplacedSkillOnlyAtTheRevisionTheDecisionsChoose(t *testing.T) {
	ctx := context.Background()
	f := scriptLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"rules":{"include":["techs/go/*"]},"skills":{"include":["skills/build-feature"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	forkSkill(t, options, "team:skills/build-feature", "We build with make.")
	publishScriptRelease(t, f)
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "team", Rule: "techs/go/errors"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := moved.Sources[0].ReplacedSkills; len(got) != 1 || got[0].ID != "skills/build-feature" {
		t.Fatalf("replaced skills when errors moves = %+v; want build-feature", got)
	}
	keep := []UpdateDecision{{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "Not yet."}}
	for _, step := range []struct {
		name string
		run  func(context.Context, []UpdateDecision) (UpdateResult, error)
	}{{"preview", plan.Preview}, {"apply", plan.Apply}} {
		kept, err := step.run(ctx, keep)
		if err != nil {
			t.Fatal(err)
		}
		if got := kept.Sources[0].ReplacedSkills; got == nil || len(got) != 0 {
			t.Errorf("%s with errors kept lists replaced skills %+v; want none, since the skills stay at release/1", step.name, got)
		}
	}
}

// TestUpdate_KeepsAForkOfASkillItsSourceDeletes forks a skill that skills.include doesn't select, so the source
// supplies it only for comparison, and keeps the fork installed after a library release deletes the skill, listing
// the deletion in the update preview.
func TestUpdate_KeepsAForkOfASkillItsSourceDeletes(t *testing.T) {
	// A project may also still include the skill it forks by ID, which mustn't stop the update either.
	for name, include := range map[string]string{"forks only": "", "included too": "      include: [skills/build-feature]\n"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			f, commit := skillLibrary(t)
			options, git := skillProject(t, f, "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: REPOSITORY\n    skills:\n"+include+"      forks:\n        skills/build-feature:\n          as: skills/build-feature\n          basedOn: \""+commit+"\"\n          reason: We build with make.\n")
			root, err := openProject(ctx, options, false)
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, root, "skills/team/build-feature/SKILL.md", buildSkill)
			root.Close()
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			if got := installedSkills(t, options); !slices.Equal(got, []string{"build-feature"}) {
				t.Fatalf("installed %v; want only the fork", got)
			}
			if !exists(t, options, ".trailguide/_managed/vendor/team/skills/library/build-feature/SKILL.md") {
				t.Fatal("sync didn't import the forked skill to compare with the fork")
			}
			later, err := f.Commit(ctx, f.Worktree(), "Delete a skill", map[string][]byte{".trailguide/skills/library/build-feature/SKILL.md": nil, ".trailguide/skills/library/build-feature/scripts/run.sh": nil})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n"); err != nil {
				t.Fatal(err)
			}
			plan, err := PlanUpdate(ctx, options, git, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			applied, err := plan.Apply(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := []imports.ReplacedSkillUpdate{{ID: "skills/build-feature", LocalSkill: "skills/build-feature", BasedOn: commit, From: commit, To: later, Files: []string{}, Removed: true}}
			if len(applied.Sources) != 1 || !reflect.DeepEqual(applied.Sources[0].ReplacedSkills, want) {
				t.Fatalf("update = %+v\nwant replaced skills %+v", applied.Sources, want)
			}
			if got := installedSkills(t, options); !slices.Equal(got, []string{"build-feature"}) {
				t.Fatalf("after the update, installed %v; want the fork", got)
			}
			if exists(t, options, ".trailguide/_managed/vendor/team/skills/library/build-feature/SKILL.md") {
				t.Fatal("the update kept the deleted skill's vendored copy")
			}
			if again, err := Sync(ctx, options, git); err != nil || len(again.Added)+len(again.Changed)+len(again.Removed) != 0 {
				t.Fatalf("sync after the update = %+v, %v; want no changes", again, err)
			}
		})
	}
}

func TestSync_InstallsNoSkillOnlyTheForkedSkillUses(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	using := "---\nname: build-feature\ndescription: Build a feature from a spec.\nmetadata:\n  trailguide-skills-used: |-\n    skills/review\n---\n# Build a feature\n"
	commit, err := f.Commit(ctx, f.Worktree(), "Review each feature", map[string][]byte{".trailguide/skills/library/build-feature/SKILL.md": []byte(using)})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	options, git := skillProject(t, f, "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: REPOSITORY\n    skills:\n      forks:\n        skills/build-feature:\n          as: skills/build-feature\n          basedOn: \""+commit+"\"\n          reason: We build with make.\n")
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "skills/team/build-feature/SKILL.md", buildSkill)
	root.Close()
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if got := installedSkills(t, options); !slices.Equal(got, []string{"build-feature"}) {
		t.Fatalf("installed %v; want only the fork, which uses no other skill", got)
	}
}

// visibleSkillRows returns rows without the copies of each skill that the preview carries for merging, which its
// JSON leaves out.
func visibleSkillRows(rows []imports.ReplacedSkillUpdate) []imports.ReplacedSkillUpdate {
	result := slices.Clone(rows)
	for i := range result {
		result[i].Base, result[i].Upstream, result[i].Library = nil, nil, false
	}
	return result
}
