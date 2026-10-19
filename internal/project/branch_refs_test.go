// Test that a source of skills that isn't a library can follow a branch: sync and build keep the commit its source
// record holds, and update moves it to the branch's newest commit.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// branchSkills returns a repository of skills that isn't a library, with the skill tools/review on its branch main.
func branchSkills(t *testing.T) *gitfixture.Fixture {
	t.Helper()
	f, err := gitfixture.New(context.Background(), map[string][]byte{"tools/review/SKILL.md": []byte(reviewSkill)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// branchProject syncs a project that installs every skill of f from its branch ref, such as main.
func branchProject(t *testing.T, f *gitfixture.Fixture, ref string) (Options, imports.Options) {
	t.Helper()
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"skills":{"repository":REPOSITORY,"ref":"`+ref+`","skills":{"include":["*"]}}}}`)
	if _, err := Sync(context.Background(), options, git); err != nil {
		t.Fatal(err)
	}
	return options, git
}

// recordedCommit returns the commit the source record of skills holds.
func recordedCommit(t *testing.T, options Options) string {
	t.Helper()
	var record struct {
		Commit string `json:"resolvedCommit"`
	}
	if err := json.Unmarshal([]byte(readRepositoryFile(t, options, ".trailguide/vendor/skills/_source.json")), &record); err != nil {
		t.Fatal(err)
	}
	return record.Commit
}

// headCommit returns the commit f's served repository has checked out.
func headCommit(t *testing.T, f *gitfixture.Fixture) string {
	t.Helper()
	commit, err := f.CommandIn(context.Background(), f.Worktree(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(commit)
}

// requireFailure fails the test unless err is an import failure with code whose message contains text.
func requireFailure(t *testing.T, err error, code, text string) {
	t.Helper()
	var failure *imports.Error
	if !errors.As(err, &failure) || failure.Code != code || !strings.Contains(err.Error(), text) {
		t.Fatalf("got %v; want %s mentioning %q", err, code, text)
	}
}

func TestSync_KeepsABranchsRecordedCommitAndUpdateMovesItToTheNewest(t *testing.T) {
	ctx := context.Background()
	f := branchSkills(t)
	options, git := branchProject(t, f, "main")
	if got := recordedCommit(t, options); got != f.LatestCommit {
		t.Fatalf("sync recorded %s; want main's newest commit %s", got, f.LatestCommit)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Improve review", map[string][]byte{
		"tools/review/SKILL.md": []byte(strings.Replace(reviewSkill, "# Review", "# Review carefully", 1)),
		"tools/review/lint.sh":  []byte("#!/bin/sh\necho lint\n"),
	}); err != nil {
		t.Fatal(err)
	}
	makeExecutable(t, f, "tools/review/lint.sh")
	moved, err := f.Commit(ctx, f.Worktree(), "Make lint.sh executable", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	if got := recordedCommit(t, options); got != f.LatestCommit || strings.Contains(readRepositoryFile(t, options, ".claude/skills/review/SKILL.md"), "carefully") {
		t.Fatalf("sync or build moved the source to %s", got)
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "skills"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := preview.Sources[0]
	if !source.FollowsRef || source.Ref != "main" || !reflect.DeepEqual(source.Commit, &imports.CommitMove{From: f.LatestCommit, To: moved}) || source.TagMoved || !preview.Moves() {
		t.Fatalf("preview = %+v; want main moving from %s to %s", source, f.LatestCommit, moved)
	}
	want := []imports.SkillUpdate{{ID: "tools/review", Change: imports.SkillChanged, Files: []string{"SKILL.md", "lint.sh"}, Scripts: []imports.ScriptUpdate{{Path: "lint.sh", Change: imports.SkillAdded}}}}
	if !reflect.DeepEqual(source.Skills, want) || preview.ScriptsWarning() == "" {
		t.Fatalf("skill rows = %+v; want review's changes with lint.sh flagged", source.Skills)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := recordedCommit(t, options); got != moved || !strings.Contains(readRepositoryFile(t, options, ".claude/skills/review/SKILL.md"), "carefully") {
		t.Fatalf("update recorded %s; want %s and the skill from it", got, moved)
	}
	plan, err = PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview, err = plan.Preview(ctx, nil); err != nil || preview.Sources[0].Commit != nil || preview.Moves() {
		t.Fatalf("preview after the update = %+v, %v; want nothing to move", preview.Sources, err)
	}
}

func TestSync_ACommitTheBranchNoLongerHasFailsAndUpdateTakesTheNewest(t *testing.T) {
	ctx := context.Background()
	f := branchSkills(t)
	options, git := branchProject(t, f, "main")
	// Rewrite main's newest commit and remove every trace of the original, as a force-push does.
	for _, args := range [][]string{{"tag", "--delete", "v1.0.0", "v1.2.0"}, {"commit", "--quiet", "--amend", "--allow-empty", "-m", "Rewritten"}, {"reflog", "expire", "--expire=now", "--all"}, {"gc", "--quiet", "--prune=now"}} {
		if _, err := f.Command(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	rewritten, err := f.Command(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Sync(ctx, options, git)
	requireFailure(t, err, "version-not-found", "Run trailguide project update skills to take the commit sources.skills.ref names now.")
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := recordedCommit(t, options); got != rewritten {
		t.Fatalf("update recorded %s; want the rewritten commit %s", got, rewritten)
	}
}

func TestUpdate_NamesTheBranchItCannotResolve(t *testing.T) {
	ctx := context.Background()
	f := branchSkills(t)
	if _, err := f.Command(ctx, "branch", "skills"); err != nil {
		t.Fatal(err)
	}
	options, git := branchProject(t, f, "skills")
	if _, err := f.Command(ctx, "branch", "--move", "skills", "renamed"); err != nil {
		t.Fatal(err)
	}
	// Sync keeps the recorded commit, which main still has, so it needs no branch.
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	_, err := PlanUpdate(ctx, options, git, nil, nil)
	requireFailure(t, err, "version-not-found", "sources.skills.ref: the repository has no branch or tag skills; check the ref, and whether a branch of that name was renamed or deleted.")
}

func TestSync_FollowsABranchNamedLikeALibraryRelease(t *testing.T) {
	f := branchSkills(t)
	if _, err := f.Command(context.Background(), "branch", "release/5"); err != nil {
		t.Fatal(err)
	}
	options, _ := branchProject(t, f, "release/5")
	if got := recordedCommit(t, options); got != f.LatestCommit {
		t.Fatalf("sync recorded %s; want release/5's newest commit %s", got, f.LatestCommit)
	}
}

func TestSync_ABareReleaseNameOfABranchAndATagIsAmbiguous(t *testing.T) {
	ctx := context.Background()
	f := branchSkills(t)
	for _, args := range [][]string{{"branch", "release/5"}, {"tag", "release/5"}} {
		if _, err := f.Command(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"skills":{"repository":REPOSITORY,"ref":"release/5","skills":{"include":["*"]}}}}`)
	_, err := Sync(ctx, options, git)
	requireFailure(t, err, "ambiguous-ref", "the repository has both a branch and a tag named release/5.")
}

func TestUpdate_ATargetedBranchSourceThatSyncHasNotRecordedTakesTheBranch(t *testing.T) {
	ctx := context.Background()
	f := branchSkills(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"skills":{"repository":REPOSITORY,"ref":"main","skills":{"include":["*"]}}}}`)
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "skills"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := recordedCommit(t, options); got != f.LatestCommit {
		t.Fatalf("update recorded %s; want main's newest commit %s", got, f.LatestCommit)
	}
}

// A library source can follow a branch too: sync records the branch's commit and warns that the source takes the
// library's unreleased changes, sync keeps that commit, and a targeted update moves it to the branch's newest
// commit, with the same warning.
func TestUpdate_ALibrarySourceOnABranchMovesToItsNewestCommitAndWarns(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"skills":{"repository":REPOSITORY,"ref":"main","rules":{"include":["techs/go/*"]},"skills":{"include":["*"]}}}}`)
	synced, err := Sync(ctx, options, git)
	if err != nil {
		t.Fatal(err)
	}
	const warning = "Source skills follows the library's branch main, so it takes changes the library hasn't released"
	if !slices.ContainsFunc(synced.Warnings, func(w string) bool { return strings.HasPrefix(w, warning) }) {
		t.Fatalf("sync warnings = %q; want one that the source follows the library's branch", synced.Warnings)
	}
	if got, want := recordedCommit(t, options), headCommit(t, f); got != want {
		t.Fatalf("sync recorded %s; want main's newest commit %s", got, want)
	}
	moved, err := f.Commit(ctx, f.Worktree(), "Review carefully", map[string][]byte{
		".trailguide/skills/review/SKILL.md": []byte(strings.Replace(reviewSkill, "# Review", "# Review carefully", 1)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if got := recordedCommit(t, options); got == moved {
		t.Fatal("sync moved the library's branch to its newest commit")
	}
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "skills"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := preview.Sources[0]
	if !source.FollowsRef || source.Commit == nil || source.Commit.To != moved || source.TagMoved {
		t.Fatalf("preview = %+v; want main moving to %s", source, moved)
	}
	if !slices.ContainsFunc(preview.Warnings, func(w string) bool { return strings.HasPrefix(w, warning) }) {
		t.Fatalf("update warnings = %q; want one that the source follows the library's branch", preview.Warnings)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := recordedCommit(t, options); got != moved || !strings.Contains(readRepositoryFile(t, options, ".claude/skills/review/SKILL.md"), "carefully") {
		t.Fatalf("update recorded %s; want %s and the skill from it", got, moved)
	}
}

// A library source whose bare ref names a tag holds that revision: update resolves the name to learn it isn't a
// branch, and leaves the source where sync keeps it, even after the tag moves.
func TestUpdate_ALibrarySourceOnABareTagStaysWhereSyncKeepsIt(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	if err := f.Tag(ctx, f.Worktree(), "v1", "v1"); err != nil {
		t.Fatal(err)
	}
	tagged := headCommit(t, f)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"skills":{"repository":REPOSITORY,"ref":"v1","rules":{"include":["techs/go/*"]},"skills":{"include":["*"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Review carefully", map[string][]byte{
		".trailguide/skills/review/SKILL.md": []byte(strings.Replace(reviewSkill, "# Review", "# Review carefully", 1)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.CommandIn(ctx, f.Worktree(), "tag", "--force", "--annotate", "--message", "v1", "v1"); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if source := preview.Sources[0]; source.FollowsRef || source.Commit != nil || preview.Moves() {
		t.Fatalf("preview = %+v; want the tagged library to stay at %s", source, tagged)
	}
}
