// Test that library check and library release version the library's skills: their change notes, release record,
// and release notes, with each skill's level and fork.

package library

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// forkBase and movedBase are commits of the source a fork is based on.
const (
	forkBase  = "8105a34c1f2e3d4b5a69788796a5b4c3d2e1f0a9"
	movedBase = "1f2e3d4b5a69788796a5b4c3d2e1f0a98105a34c"
)

// ratchetConfig is a library manifest whose library forks dzhng's ratchet-files skill, based on basedOn, and lists the
// fork at level, with a notice file.
func ratchetConfig(basedOn, level string) []byte {
	return []byte("schemaVersion: 2\nsources:\n  dzhng:\n    repository: https://github.com/dzhng/skills\n    skills:\n      forks:\n        skills/engineering/ratchet-files:\n          as: skills/ratchet-files\n          basedOn: \"" + basedOn + "\"\n          reason: We publish our own version.\nlibrary:\n  schemaVersion: 2\n  name: library\n  license:\n    spdxExpression: MIT\n    file: LICENSE.md\n    notices:\n      - NOTICE.md\n  skills:\n    " + level + ": [skills/ratchet-files]\n")
}

// ratchetLibrary is libraryFiles with the license and notice ratchetConfig declares, before the library had skills.
func ratchetLibrary() map[string][]byte {
	files := libraryFiles()
	files["config.yaml"] = []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  license:\n    spdxExpression: MIT\n    file: LICENSE.md\n    notices:\n      - NOTICE.md\n")
	files["LICENSE.md"] = []byte("MIT License\n")
	files["NOTICE.md"] = []byte("Notice\n")
	return files
}

// ratchetSkill is the ratchet-files skill's SKILL.md.
const ratchetSkill = "---\nname: ratchet-files\ndescription: Ratchet a file count down.\n---\n# Ratchet files\n"

// TestRelease_NamesANewForkedSkillMadeRecommendedWithoutRuleChanges reproduces public-library's release/4, which
// added a forked skill at the recommended level and changed no rules: check requires the skill's change note, and the
// library release names the skill, its version, level, and fork in its notes and records it under skills.
func TestRelease_NamesANewForkedSkillMadeRecommendedWithoutRuleChanges(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, ratchetLibrary(), releaseOne)
	commitAndPush(t, fixture, options.Directory, map[string][]byte{
		"config.yaml": ratchetConfig(forkBase, "recommended"),
		"NOTICE.md":   []byte("Notice\n\nratchet-files is adapted from dzhng/skills.\n"),
		".trailguide/skills/dzhng/ratchet-files/SKILL.md":         []byte(ratchetSkill),
		".trailguide/skills/dzhng/ratchet-files/scripts/count.sh": []byte("#!/bin/sh\n"),
	})
	_, err := Check(ctx, options)
	if errorCode(err) != "change-notes" || !strings.Contains(err.Error(), "skills/ratchet-files is a new skill, and no pending change note names it. Record it with: trailguide library change skills/ratchet-files --summary '<what the skill adds>'") {
		t.Fatalf("got %v; want change-notes naming the new skill", err)
	}
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"changes/ratchet-files.yaml": []byte("summary: Ratchet a file count down, adapted from dzhng/skills.\nskills:\n  skills/ratchet-files: new\n")})
	checked, err := Check(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if pending := checked.PendingRelease.Skills; checked.SkillCount != 1 || len(pending) != 1 || pending[0].ID != "skills/ratchet-files" || pending[0].Change != "new" || pending[0].To.String() != "1.0.0" || pending[0].Level != "recommended" || pending[0].Fork == nil || !pending[0].Fork.Added {
		t.Fatalf("pending skills %+v, skill count %d", pending, checked.SkillCount)
	}
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	notes := "Library release 2 changes 1 skill: 1 new.\n\n## Skills\n\n- **skills/ratchet-files** `1.0.0`, new, recommended, forked from **dzhng:skills/engineering/ratchet-files** at `8105a34`\n  - Ratchet a file count down, adapted from dzhng/skills.\n\nThis library release also updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.0.0 |\n| practices/testing/b | 1.0.0 |\n\n</details>\n\n<details>\n<summary>All skill versions in this library release</summary>\n\n| Skill | Version |\n| --- | --- |\n| skills/ratchet-files | 1.0.0 |\n\n</details>"
	record := "schemaVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/b: 1.0.0\nlibraryFiles:\n  - NOTICE.md\n  - config.yaml\nskills:\n  versions:\n    skills/ratchet-files: 1.0.0\n  changes:\n    skills/ratchet-files:\n      change: new\n      summaries:\n        - Ratchet a file count down, adapted from dzhng/skills.\n"
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/2"); message != notes+"\n---\n"+record {
		t.Fatalf("tag message:\n%s\nwant:\n%s\n---\n%s", message, notes, record)
	}
	if len(result.Rules) != 0 || len(result.Skills) != 1 || result.Skills[0].Level != "recommended" || !slices.Equal(result.LibraryFiles, []string{"NOTICE.md", "config.yaml"}) {
		t.Fatalf("%+v", result)
	}
	// Run again on the published commit, the release reports the skill as it did when it published it.
	rerun, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil || !rerun.Published || len(rerun.Skills) != 1 || rerun.Skills[0].Level != "recommended" || rerun.Skills[0].Fork == nil || !rerun.Skills[0].Fork.Added || rerun.Skills[0].Fork.BasedOn != forkBase {
		t.Fatalf("%+v: %v", rerun, err)
	}
}

// TestRelease_PublishesAMovedAndARemovedFork moves a fork to another commit, then stops forking, without changing the
// skill's files: each publishes a library release whose notes name the fork's change.
func TestRelease_PublishesAMovedAndARemovedFork(t *testing.T) {
	ctx := context.Background()
	files := ratchetLibrary()
	files["config.yaml"] = ratchetConfig(forkBase, "recommended")
	files[".trailguide/skills/dzhng/ratchet-files/SKILL.md"] = []byte(ratchetSkill)
	fixture, options := authorClone(t, files)
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err != nil {
		t.Fatal(err)
	}
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"config.yaml": ratchetConfig(movedBase, "recommended")})
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil || !slices.Equal(result.LibraryFiles, []string{"config.yaml"}) {
		t.Fatalf("%+v: %v", result, err)
	}
	moved := "Library release 2 changes no rule or skill versions. It changes the level or fork of 1 skill.\n\n## Skills\n\n- **skills/ratchet-files** `1.0.0`, recommended, fork of **dzhng:skills/engineering/ratchet-files** now based on `1f2e3d4`, was `8105a34`\n\n<details>"
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/2"); !strings.HasPrefix(message, moved) {
		t.Fatalf("tag message:\n%s\nwant it to start with:\n%s", message, moved)
	}
	// The skill stops being a fork, so it moves to the library's own folder, which keeps its ID and version.
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"config.yaml": []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  license:\n    spdxExpression: MIT\n    file: LICENSE.md\n    notices:\n      - NOTICE.md\n  skills:\n    recommended: [skills/ratchet-files]\n"), ".trailguide/skills/dzhng/ratchet-files/SKILL.md": nil, ".trailguide/skills/library/ratchet-files/SKILL.md": []byte(ratchetSkill)})
	if err := os.RemoveAll(filepath.Join(options.Directory, ".trailguide", "skills", "dzhng")); err != nil {
		t.Fatal(err)
	}
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err != nil {
		t.Fatal(err)
	}
	removed := "Library release 3 changes no rule or skill versions. It changes the level or fork of 1 skill.\n\n## Skills\n\n- **skills/ratchet-files** `1.0.0`, recommended, no longer a fork of **dzhng:skills/engineering/ratchet-files**\n\n<details>"
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/3"); !strings.HasPrefix(message, removed) {
		t.Fatalf("tag message:\n%s\nwant it to start with:\n%s", message, removed)
	}
}

// TestRelease_VersionsChangedSkillsFromTheirNotes changes one skill's script mode, moves its fork, and raises its
// level, and retires another skill: check requires a note for each, rejects a stale one, and the library release
// advances the changed skill's version, retires the other, and names the level and fork changes.
func TestRelease_VersionsChangedSkillsFromTheirNotes(t *testing.T) {
	ctx := context.Background()
	files := ratchetLibrary()
	files["config.yaml"] = ratchetConfig(forkBase, "offered")
	files[".trailguide/skills/dzhng/ratchet-files/SKILL.md"] = []byte(ratchetSkill)
	files[".trailguide/skills/dzhng/ratchet-files/scripts/count.sh"] = []byte("#!/bin/sh\n")
	files[".trailguide/skills/library/triage/SKILL.md"] = []byte("---\nname: triage\ndescription: Triage a bug.\n---\n# Triage\n")
	files[".trailguide/skills/library/review/SKILL.md"] = []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n")
	fixture, options := authorClone(t, files)
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err != nil {
		t.Fatal(err)
	}
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/1"); !strings.HasPrefix(message, "Library release 1 publishes 2 rules and 3 skills.\n") || !strings.Contains(message, "- **skills/triage** `1.0.0`, new, offered\n") || !strings.Contains(message, "skills:\n  versions:\n    skills/ratchet-files: 1.0.0\n    skills/review: 1.0.0\n    skills/triage: 1.0.0\n  changes:\n") || !strings.Contains(message, "      summaries:\n        - Add the skill.\n") {
		t.Fatalf("first library release:\n%s", message)
	}

	script := filepath.Join(options.Directory, ".trailguide", "skills", "dzhng", "ratchet-files", "scripts", "count.sh")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(options.Directory, ".trailguide", "skills", "library", "triage")); err != nil {
		t.Fatal(err)
	}
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"config.yaml": ratchetConfig(movedBase, "required")})
	_, err := Check(ctx, options)
	want := "change notes don't match the rule and skill changes since release/1:\n  - skills/ratchet-files changed since release/1, where its version is 1.0.0, and no pending change note names it. Record it with: trailguide library change skills/ratchet-files --bump <major|minor|patch> --summary '<what changed>'\n  - skills/triage, version 1.0.0, was deleted, and no pending change note retires it. Restore it, or record the retirement with: trailguide library change skills/triage --retire --summary '<why it's retired>'"
	if errorCode(err) != "change-notes" || !strings.HasSuffix(err.Error(), want) {
		t.Fatalf("got %v; want\n%s", err, want)
	}
	commitAndPush(t, fixture, options.Directory, map[string][]byte{
		"changes/ratchet.yaml": []byte("summary: Run the counting script directly.\nskills:\n  skills/ratchet-files: minor\n"),
		"changes/triage.yaml":  []byte("summary: Covered by review.\nskills:\n  skills/triage:\n    change: retired\n    replacedBy: skills/review\n"),
		"changes/review.yaml":  []byte("summary: Stale.\nskills:\n  skills/review: patch\n"),
	})
	if _, err := Check(ctx, options); errorCode(err) != "change-notes" || !strings.HasSuffix(err.Error(), "\n  - changes/review.yaml names skills/review, which is unchanged since release/1, so the note is stale. Remove the skill from the note.") {
		t.Fatalf("got %v; want the stale note rejected", err)
	}
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"changes/review.yaml": nil})
	checked, err := Check(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if rows := skillRows(checked.PendingRelease.Skills); !slices.Equal(rows, []string{"skills/ratchet-files minor 1.0.0 1.1.0 required offered " + movedBase + " " + forkBase, "skills/triage retired 1.0.0 -   "}) {
		t.Fatalf("pending skills %q", rows)
	}
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err != nil {
		t.Fatal(err)
	}
	notes := "Library release 2 changes 2 skills: 1 minor and 1 retired.\n\n## Skills\n\n- **skills/ratchet-files** `1.0.0` → `1.1.0`, minor, now required, was offered, fork of **dzhng:skills/engineering/ratchet-files** now based on `1f2e3d4`, was `8105a34`\n  - Run the counting script directly.\n- **skills/triage**, last version `1.0.0`, retired, replaced by **skills/review**\n  - Covered by review.\n\nThis library release also updates shared files, such as group descriptions or shared assets.\n\n"
	record := "skills:\n  versions:\n    skills/ratchet-files: 1.1.0\n    skills/review: 1.0.0\n  changes:\n    skills/ratchet-files:\n      change: minor\n      from: 1.0.0\n      summaries:\n        - Run the counting script directly.\n  retired:\n    skills/triage:\n      lastVersion: 1.0.0\n      replacedBy: skills/review\n      summaries:\n        - Covered by review.\n"
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/2"); !strings.HasPrefix(message, notes) || !strings.HasSuffix(message, record) {
		t.Fatalf("tag message:\n%s\nwant it to start with:\n%s\nand end with:\n%s", message, notes, record)
	}

	// A library release that a Trail Guide without skill support publishes leaves skill versions to release/2.
	if err := fixture.Tag(ctx, options.Directory, "release/3", "Library release 3 changes no rules.\n---\nschemaVersion: 1\nrelease: 3\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/b: 1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	checked, err = Check(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	warning := "release/3 has no skills field, because a Trail Guide without skill support published it. Skill versions come from release/2. Raise the Trail Guide version this library's CI uses before publishing skills."
	if !slices.Contains(checked.Warnings, warning) || len(checked.PendingRelease.Skills) != 0 {
		t.Fatalf("warnings %q, pending skills %+v", checked.Warnings, checked.PendingRelease.Skills)
	}
}

// skillRows renders each pending skill as "id change current next level previousLevel basedOn previousBasedOn", with
// "-" for an absent version.
func skillRows(skills []PendingSkill) []string {
	rows := []string{}
	for _, skill := range skills {
		current, next := "-", "-"
		if skill.From != nil {
			current = skill.From.String()
		}
		if skill.LastVersion != nil {
			current = skill.LastVersion.String()
		}
		if skill.To != nil {
			next = skill.To.String()
		}
		row := skill.ID + " " + string(skill.Change) + " " + current + " " + next + " " + skill.Level + " " + skill.PreviousLevel
		if skill.Fork != nil {
			row += " " + skill.Fork.BasedOn + " " + skill.Fork.PreviousBasedOn
		} else {
			row += " "
		}
		rows = append(rows, row)
	}
	return rows
}

// TestChange_RecordsSkillNotesThatLibraryCheckAccepts records new, changed, and retired skills, together with a rule,
// and refuses notes check would reject.
func TestChange_RecordsSkillNotesThatLibraryCheckAccepts(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	files[".trailguide/skills/library/review/SKILL.md"] = []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n")
	files[".trailguide/skills/library/triage/SKILL.md"] = []byte("---\nname: triage\ndescription: Triage a bug.\n---\n# Triage\n")
	_, options := authorClone(t, files, releaseOne+"skills:\n  versions:\n    skills/review: 1.0.0\n    skills/triage: 1.0.0\n  changes:\n    skills/review: {change: new, summaries: [Add the skill.]}\n    skills/triage: {change: new, summaries: [Add the skill.]}\n")
	if _, _, err := recordChange(t, options, ChangeRequest{IDs: []string{"skills/review"}, Bump: "minor", Summary: "Check the tests."}); errorCode(err) != "unchanged-skill" || err.Error() != "skills/review hasn't changed since release/1. Edit the skill first, then record the change." {
		t.Fatalf("got %v; want unchanged-skill", err)
	}
	if _, _, err := recordChange(t, options, ChangeRequest{IDs: []string{"skills/triage"}, Retire: true, Summary: "Covered by review."}); errorCode(err) != "invalid-change" || err.Error() != "skills/triage still exists. Delete its folder before recording its retirement." {
		t.Fatalf("got %v; want the existing skill refused", err)
	}
	if _, _, err := recordChange(t, options, ChangeRequest{IDs: []string{"skills/triage"}, Retire: true, ReplacedBy: "practices/testing/a", Summary: "Covered by a."}); errorCode(err) != "invalid-arguments" {
		t.Fatalf("got %v; want a rule refused as a skill's replacement", err)
	}
	edit(t, options.Directory, map[string]string{
		".trailguide/skills/library/review/SKILL.md":  "---\nname: review\ndescription: Review a change.\n---\n# Review\n\nCheck the tests.\n",
		".trailguide/skills/library/triage":           "",
		".trailguide/skills/library/fix-bug/SKILL.md": "---\nname: fix-bug\ndescription: Fix a bug.\n---\n# Fix a bug\n",
		"practices/testing/verify-retries.md":         ruleText("Verify retries."),
	})
	for _, test := range []struct {
		request ChangeRequest
		want    string
	}{
		{ChangeRequest{IDs: []string{"skills/review"}, Bump: "minor", Summary: "Check the tests."}, "summary: Check the tests.\nskills:\n  skills/review: minor\n"},
		{ChangeRequest{IDs: []string{"skills/triage"}, Retire: true, ReplacedBy: "skills/fix-bug", Summary: "Covered by fix-bug."}, "summary: Covered by fix-bug.\nskills:\n  skills/triage:\n    change: retired\n    replacedBy: skills/fix-bug\n"},
		{ChangeRequest{IDs: []string{"skills/fix-bug", "practices/testing/verify-retries"}, Summary: "Fix bugs with a retry test."}, "summary: Fix bugs with a retry test.\nrules:\n  practices/testing/verify-retries: new\nskills:\n  skills/fix-bug: new\n"},
	} {
		if _, note, err := recordChange(t, options, test.request); err != nil || note != test.want {
			t.Fatalf("%v: note\n%s\nwant\n%s\n%v", test.request.IDs, note, test.want, err)
		}
	}
	checked, err := Check(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if rows := skillRows(checked.PendingRelease.Skills); !slices.Equal(rows, []string{"skills/fix-bug new - 1.0.0 offered  ", "skills/review minor 1.0.0 1.1.0 offered  ", "skills/triage retired 1.0.0 -   "}) {
		t.Fatalf("pending skills %q", rows)
	}
}

// TestCheck_LeavesFilesGitIgnoresOutOfASkillsVersion needs no note, and publishes nothing, for a file Git ignores in
// a skill's folder, such as a cache its script wrote, and library release finds the commit already published.
func TestCheck_LeavesFilesGitIgnoresOutOfASkillsVersion(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	files[".trailguide/skills/library/review/SKILL.md"] = []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n")
	files[".gitignore"] = []byte("__pycache__/\n")
	fixture, options := authorClone(t, files)
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err != nil {
		t.Fatal(err)
	}
	edit(t, options.Directory, map[string]string{".trailguide/skills/library/review/__pycache__/check.pyc": "cache"})
	checked, err := Check(ctx, options)
	if err != nil || len(checked.PendingRelease.Skills) != 0 {
		t.Fatalf("pending skills %+v: %v", checked.PendingRelease.Skills, err)
	}
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil || !result.Published || result.TagCreated || strings.Contains(tags(t, fixture, remoteDir(fixture)), "release/2") {
		t.Fatalf("%+v: %v", result, err)
	}
}

// TestRelease_RecordsOnlySkillsItsCommitDelivers refuses a note for a skill whose files Git ignores, which no commit
// holds, and publishes no version for it.
func TestRelease_RecordsOnlySkillsItsCommitDelivers(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	files[".gitignore"] = []byte(".trailguide/skills/library/local/\n")
	fixture, options := authorClone(t, files)
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err != nil {
		t.Fatal(err)
	}
	edit(t, options.Directory, map[string]string{".trailguide/skills/library/local/SKILL.md": "---\nname: local\ndescription: A personal skill.\n---\n# Local\n"})
	checked, err := Check(ctx, options)
	if err != nil || checked.SkillCount != 0 || len(checked.PendingRelease.Skills) != 0 {
		t.Fatalf("%+v: %v", checked, err)
	}
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"changes/local.yaml": []byte("summary: Add a personal skill.\nskills:\n  skills/local: new\n")})
	if _, err := Check(ctx, options); errorCode(err) != "change-notes" || !strings.Contains(err.Error(), "skills/local") {
		t.Fatalf("got %v; want the note for the ignored skill rejected", err)
	}
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err == nil || strings.Contains(tags(t, fixture, remoteDir(fixture)), "release/2") {
		t.Fatalf("published a release for an ignored skill: %v", err)
	}
}

// TestRelease_RefusesAnExecutableBitItsCommitLacks refuses to publish a skill's new version when the working tree
// makes a script executable but the commit doesn't.
func TestRelease_RefusesAnExecutableBitItsCommitLacks(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	files[".trailguide/skills/library/review/SKILL.md"] = []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n")
	files[".trailguide/skills/library/review/scripts/lint.sh"] = []byte("#!/bin/sh\n")
	fixture, options := authorClone(t, files)
	if _, err := Release(ctx, ReleaseRequest{Options: options}); err != nil {
		t.Fatal(err)
	}
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"changes/review.yaml": []byte("summary: Run the lint script directly.\nskills:\n  skills/review: patch\n")})
	if err := os.Chmod(filepath.Join(options.Directory, ".trailguide", "skills", "library", "review", "scripts", "lint.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if checked, err := Check(ctx, options); err != nil || len(checked.PendingRelease.Skills) != 1 || checked.PendingRelease.Skills[0].To.String() != "1.0.1" {
		t.Fatalf("%+v: %v", checked.PendingRelease.Skills, err)
	}
	_, err := Release(ctx, ReleaseRequest{Options: options})
	if errorCode(err) != "uncommitted-changes" || !strings.Contains(err.Error(), "  - .trailguide/skills/library/review/scripts/lint.sh has an executable bit that differs from its committed copy\n") || strings.Contains(tags(t, fixture, remoteDir(fixture)), "release/2") {
		t.Fatalf("got %v; want the uncommitted executable bit refused", err)
	}
}
