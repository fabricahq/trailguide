// Test that project update previews the skills it changes and flags the executable scripts it adds or changes,
// against a library whose second release changes a skill's scripts.

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

	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// The warnings an update gives when its skill rows flag a script, before and after it is applied.
const (
	previewScriptsWarning = "some skills in this update add or change scripts. Trail Guide never runs them, but your agents may; read them before you confirm."
	appliedScriptsWarning = "some skills in this update added or changed scripts. Trail Guide never runs them, but your agents may; read them before you commit the update."
)

// scriptLibrary returns a library whose release/1 publishes techs/go/errors 1.0.0 and the skill build-feature, with
// the executable scripts run.sh and lint.sh and a reference.
func scriptLibrary(t *testing.T) *gitfixture.Fixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                                          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`),
		"techs/go/_group.yaml":                                 []byte(projectMetadata),
		"techs/go/errors.md":                                   []byte(projectRule),
		".trailguide/skills/build-feature/SKILL.md":            []byte(buildSkill),
		".trailguide/skills/build-feature/scripts/run.sh":      []byte("#!/bin/sh\necho build\n"),
		".trailguide/skills/build-feature/scripts/lint.sh":     []byte("#!/bin/sh\necho lint\n"),
		".trailguide/skills/build-feature/references/notes.md": []byte("Notes.\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	makeExecutable(t, f, ".trailguide/skills/build-feature/scripts/run.sh", ".trailguide/skills/build-feature/scripts/lint.sh")
	if _, err := f.Commit(ctx, f.Worktree(), "Make the scripts executable", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	return f
}

// publishScriptRelease publishes scriptLibrary's release/2, which moves errors to 1.1.0, changes run.sh and the
// reference, leaves lint.sh as it was, adds the executable setup.sh, and adds the skill plan with the executable
// plan.sh.
func publishScriptRelease(t *testing.T, f *gitfixture.Fixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.Commit(ctx, f.Worktree(), "Change the scripts", gitfixture.Library(map[string][]byte{
		"techs/go/errors.md":                                   []byte(strings.Replace(projectRule, "Return errors to the caller.", "Return errors to the caller, wrapped.", 1)),
		".trailguide/skills/build-feature/scripts/run.sh":      []byte("#!/bin/sh\necho build everything\n"),
		".trailguide/skills/build-feature/scripts/setup.sh":    []byte("#!/bin/sh\necho setup\n"),
		".trailguide/skills/build-feature/references/notes.md": []byte("More notes.\n"),
		".trailguide/skills/plan/SKILL.md":                     []byte("---\nname: plan\ndescription: Plan a change.\n---\n# Plan\n"),
		".trailguide/skills/plan/scripts/plan.sh":              []byte("#!/bin/sh\necho plan\n"),
	})); err != nil {
		t.Fatal(err)
	}
	makeExecutable(t, f, ".trailguide/skills/build-feature/scripts/setup.sh", ".trailguide/skills/plan/scripts/plan.sh")
	if _, err := f.Commit(ctx, f.Worktree(), "Make the new scripts executable", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Wrap errors.]}\n"); err != nil {
		t.Fatal(err)
	}
}

// makeExecutable sets the executable bits of names, paths in the fixture's working tree.
func makeExecutable(t *testing.T, f *gitfixture.Fixture, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.Chmod(filepath.Join(f.Worktree(), filepath.FromSlash(name)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// scriptProject syncs a project that includes build-feature from scriptLibrary's release/1, publishes release/2,
// and then sets the source's skills.include to include, a JSON list, such as one that adds plan, which only
// release/2 has.
func scriptProject(t *testing.T, include string) (Options, imports.Options) {
	t.Helper()
	ctx := context.Background()
	f := scriptLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"rules":{"include":["techs/go/*"]},"skills":{"include":["skills/build-feature"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	publishScriptRelease(t, f)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, strings.ReplaceAll(`{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"ref":"release/1","rules":{"include":["techs/go/*"]},"skills":{"include":`+include+`}}}}`, "REPOSITORY", quoteJSON(f.Repository)))
	return options, git
}

// scriptRows are the skill rows scriptProject's update shows: build-feature changes run.sh, adds setup.sh, and
// changes a reference that isn't a script, and plan is added with its script.
var scriptRows = []imports.SkillUpdate{
	{
		ID: "skills/build-feature", Change: imports.SkillChanged,
		Files:   []string{"references/notes.md", "scripts/run.sh", "scripts/setup.sh"},
		Scripts: []imports.ScriptUpdate{{Path: "scripts/run.sh", Change: imports.SkillChanged}, {Path: "scripts/setup.sh", Change: imports.SkillAdded}},
	},
	{ID: "skills/plan", Change: imports.SkillAdded, Files: []string{}, Scripts: []imports.ScriptUpdate{{Path: "scripts/plan.sh", Change: imports.SkillAdded}}},
}

func TestUpdate_FlagsTheScriptsAnUpdateAddsOrChangesBeforeAndAfterApplying(t *testing.T) {
	ctx := context.Background()
	options, git := scriptProject(t, `["skills/build-feature","skills/plan"]`)
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := preview.Sources[0].Skills; !reflect.DeepEqual(got, scriptRows) {
		t.Fatalf("preview skills = %+v\nwant %+v", got, scriptRows)
	}
	if !slices.Contains(preview.Warnings, previewScriptsWarning) || !preview.Moves() {
		t.Fatalf("preview warnings = %q, moves = %v; want the scripts warning", preview.Warnings, preview.Moves())
	}
	if exists(t, options, ".claude/skills/build-feature/scripts/setup.sh") {
		t.Fatal("the preview installed a script")
	}
	applied, err := plan.Apply(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := applied.Sources[0].Skills; !reflect.DeepEqual(got, scriptRows) {
		t.Fatalf("applied skills = %+v\nwant %+v", got, scriptRows)
	}
	if !slices.Contains(applied.Warnings, appliedScriptsWarning) || slices.Contains(applied.Warnings, previewScriptsWarning) {
		t.Fatalf("applied warnings = %q; want only the applied scripts warning", applied.Warnings)
	}
	for _, script := range []string{".claude/skills/build-feature/scripts/setup.sh", ".claude/skills/plan/scripts/plan.sh"} {
		info, err := os.Stat(filepath.Join(options.Directory, filepath.FromSlash(script)))
		if err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("%s: %v, %v; want the installed script executable", script, info, err)
		}
	}
	again, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after, err := again.Preview(ctx, nil); err != nil || len(after.Sources[0].Skills) != 0 || after.ScriptsWarning() != "" || after.Moves() {
		t.Fatalf("preview after the update = %+v, %v; want no skill rows or warning", after, err)
	}
}

// TestUpdate_FlagsNoScriptsWhenADecisionKeepsTheSkillsWhereTheyAre keeps the only rule a scoped update moves, so
// the shared files and skills stay at release/1.
func TestUpdate_FlagsNoScriptsWhenADecisionKeepsTheSkillsWhereTheyAre(t *testing.T) {
	ctx := context.Background()
	options, git := scriptProject(t, `["skills/build-feature"]`)
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "team", Rule: "techs/go/errors"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := moved.Sources[0].Skills; !reflect.DeepEqual(got, scriptRows[:1]) || moved.ScriptsWarning() == "" {
		t.Fatalf("skills when errors moves = %+v\nwant %+v and the scripts warning", got, scriptRows[:1])
	}
	keep := []UpdateDecision{{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "Not yet."}}
	kept, err := plan.Preview(ctx, keep)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept.Sources[0].Skills) != 0 || slices.Contains(kept.Warnings, previewScriptsWarning) {
		t.Fatalf("skills with errors kept = %+v, warnings %q; want none", kept.Sources[0].Skills, kept.Warnings)
	}
	applied, err := plan.Apply(ctx, keep)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Sources[0].Skills) != 0 || exists(t, options, ".claude/skills/build-feature/scripts/setup.sh") {
		t.Fatalf("the update with errors kept changed the skills: %+v", applied.Sources[0].Skills)
	}
}

// TestUpdate_TakesSkillsFromTheRevisionTheDecisionsChoose keeps the only rule a scoped update moves, so the shared
// files and skills stay at release/1, which lacks the newly included plan, and installing it would fail.
func TestUpdate_TakesSkillsFromTheRevisionTheDecisionsChoose(t *testing.T) {
	ctx := context.Background()
	options, git := scriptProject(t, `["skills/build-feature","skills/plan"]`)
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "team", Rule: "techs/go/errors"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := moved.Sources[0].Skills; !reflect.DeepEqual(got, scriptRows) {
		t.Fatalf("skills when errors moves = %+v\nwant %+v", got, scriptRows)
	}
	keep := []UpdateDecision{{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "Not yet."}}
	for name, run := range map[string]func() (UpdateResult, error){
		"preview": func() (UpdateResult, error) { return plan.Preview(ctx, keep) },
		"apply":   func() (UpdateResult, error) { return plan.Apply(ctx, keep) },
	} {
		_, err := run()
		var failure *imports.Error
		if !errors.As(err, &failure) || failure.Code != "skill-not-in-revision" || !strings.Contains(err.Error(), "skills/plan") {
			t.Fatalf("%s with errors kept: %v; want skill-not-in-revision for skills/plan", name, err)
		}
	}
	if exists(t, options, ".claude/skills/plan") {
		t.Fatal("a refused update installed the skill")
	}
}
