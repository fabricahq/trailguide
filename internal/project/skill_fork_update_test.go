package project

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
)

// forkedSkillProject syncs a project that includes every skill of skillLibrary, forks skills/build-feature, lets
// edit change the fork's SKILL.md, and publishes secondSkillRelease, returning the project, the commit the fork is
// based on, and the commit of the release.
func forkedSkillProject(t *testing.T, edit func(string) string) (Options, imports.Options, string, string) {
	t.Helper()
	ctx := context.Background()
	f, commit := skillLibrary(t)
	options, git := skillProject(t, f, skillConfig(""))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	forkSkill(t, options, "team:skills/build-feature", "We build with make.")
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "skills/build-feature/SKILL.md", edit(readRepositoryFile(t, options, ".trailguide/skills/build-feature/SKILL.md")))
	root.Close()
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	return options, git, commit, secondSkillRelease(t, f)
}

// updateSkillFork is the decision that merges the source's changes into the fork of skills/build-feature.
var updateSkillFork = []UpdateDecision{{Source: "team", Rule: "skills/build-feature", Kind: DecisionUpdateFork}}

// TestUpdate_MergesTheSourcesChangesIntoASkillFork merges the source's changes into a fork without edits and into
// one whose edits touch other lines, keeping those edits, adds the source's new file, and moves basedOn to the
// commit merged, so the next update has nothing to list.
func TestUpdate_MergesTheSourcesChangesIntoASkillFork(t *testing.T) {
	ctx := context.Background()
	for name, edit := range map[string]func(string) string{
		"no edits": func(fork string) string { return fork },
		"other lines": func(fork string) string {
			return strings.Replace(fork, "Build a feature from a spec.", "Build a feature from a spec with make.", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			options, git, commit, later := forkedSkillProject(t, edit)
			plan, err := PlanUpdate(ctx, options, git, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			preview, err := plan.Preview(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if rows := preview.Sources[0].ReplacedSkills; len(rows) != 1 || rows[0].Merge != imports.SkillMergeClean || rows[0].Conflicts != nil || rows[0].BasedOn != commit {
				t.Fatalf("replaced skills = %+v; want one that merges cleanly", rows)
			}
			applied, err := plan.Apply(ctx, updateSkillFork)
			if err != nil {
				t.Fatal(err)
			}
			if rows := applied.Sources[0].ReplacedSkills; rows[0].Decision != string(DecisionUpdateFork) {
				t.Fatalf("replaced skills = %+v; want the fork update marked", rows)
			}
			for _, want := range []string{".trailguide/config.yaml", "skills/build-feature/SKILL.md", "skills/build-feature/scripts/check.sh"} {
				if !strings.Contains(strings.Join(append(applied.Changed, applied.Added...), "\n"), strings.TrimPrefix(want, ".trailguide/")) {
					t.Fatalf("changes = %+v; want %s", applied.FileChanges, want)
				}
			}
			fork := readRepositoryFile(t, options, ".trailguide/skills/build-feature/SKILL.md")
			if !strings.Contains(fork, "Run scripts/check.sh first.") || strings.Contains(fork, "Run scripts/run.sh.") || !strings.Contains(fork, edit("Build a feature from a spec.")) {
				t.Fatalf("merged fork:\n%s", fork)
			}
			if !exists(t, options, ".trailguide/skills/build-feature/scripts/check.sh") {
				t.Fatal("the merge didn't add the source's new script")
			}
			if config := readRepositoryFile(t, options, ".trailguide/config.yaml"); !strings.Contains(config, "basedOn: \""+later+"\"") {
				t.Fatalf("config didn't move basedOn to %s:\n%s", later, config)
			}
			if installed := readRepositoryFile(t, options, ".claude/skills/build-feature/SKILL.md"); !strings.Contains(installed, "Run scripts/check.sh first.") {
				t.Fatalf("the installed fork lacks the merged change:\n%s", installed)
			}
			again, err := PlanUpdate(ctx, options, git, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if preview, err := again.Preview(ctx, nil); err != nil || len(preview.Sources[0].ReplacedSkills) != 0 {
				t.Fatalf("the next preview = %+v, %v; want no forked skill to list", preview.Sources, err)
			}
			if result, err := Check(ctx, options); err != nil || !result.Current() {
				t.Fatalf("check = %+v, %v", result, err)
			}
		})
	}
}

// TestUpdate_RefusesToMergeConflictingChangesIntoASkillFork lists the files whose changes conflict and refuses the
// fork update, writing nothing.
func TestUpdate_RefusesToMergeConflictingChangesIntoASkillFork(t *testing.T) {
	ctx := context.Background()
	options, git, _, later := forkedSkillProject(t, func(fork string) string {
		return strings.Replace(fork, "Run scripts/run.sh.", "Run make.", 1)
	})
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows := preview.Sources[0].ReplacedSkills; len(rows) != 1 || rows[0].Merge != imports.SkillMergeConflicts || strings.Join(rows[0].Conflicts, ",") != "SKILL.md" {
		t.Fatalf("replaced skills = %+v; want SKILL.md to conflict", rows)
	}
	fork := readRepositoryFile(t, options, ".trailguide/skills/build-feature/SKILL.md")
	config := readRepositoryFile(t, options, ".trailguide/config.yaml")
	_, err = plan.Apply(ctx, updateSkillFork)
	var failure *filetxn.Error
	if !errors.As(err, &failure) || failure.Code != "invalid-arguments" || !strings.Contains(err.Error(), "touch the same lines of SKILL.md") || !strings.Contains(err.Error(), later) {
		t.Fatalf("got %v; want a refusal naming SKILL.md and the commit to record", err)
	}
	if readRepositoryFile(t, options, ".trailguide/skills/build-feature/SKILL.md") != fork || readRepositoryFile(t, options, ".trailguide/config.yaml") != config {
		t.Fatal("the refused update changed the fork or its basedOn")
	}
}

// TestUpdate_RefusesToMergeIntoASkillForkWithoutBasedOn refuses before reading any source, since without basedOn
// Trail Guide can't tell the fork's edits from the source's changes.
func TestUpdate_RefusesToMergeIntoASkillForkWithoutBasedOn(t *testing.T) {
	ctx := context.Background()
	options, git, commit, _ := forkedSkillProject(t, func(fork string) string { return fork })
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, configurationFile, strings.Replace(readRepositoryFile(t, options, ".trailguide/config.yaml"), "          basedOn: \""+commit+"\"\n", "", 1))
	root.Close()
	_, err = PlanUpdate(ctx, options, git, nil, updateSkillFork)
	var failure *filetxn.Error
	if !errors.As(err, &failure) || failure.Code != "invalid-arguments" || !strings.Contains(err.Error(), "records no basedOn") {
		t.Fatalf("got %v; want a refusal without basedOn", err)
	}
}
