// Test moving a project's managed trees from where Trail Guide 0.6 kept them to _managed/, and the skills/ guide.

package project

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
)

// TestSync_MovesTheManagedTreesUnderManaged: every other command refuses a project whose vendor/ and generated/
// are where Trail Guide 0.6 kept them, writing nothing, and sync moves them under _managed/ in one transaction,
// keeping the lockfile and every vendored file. The moved project is current offline.
func TestSync_MovesTheManagedTreesUnderManaged(t *testing.T) {
	ctx := context.Background()
	_, options, git := syncProject(t)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	toLegacyLayout(t, root)
	before := projectTree(t, options)
	_, buildErr := Build(ctx, options)
	_, checkErr := Check(ctx, options)
	_, updateErr := PlanUpdate(ctx, options, git, nil, nil)
	_, listErr := ListSkills(ctx, options)
	for name, err := range map[string]error{"build": buildErr, "check": checkErr, "update": updateErr, "list skills": listErr} {
		var failure *filetxn.Error
		if !errors.As(err, &failure) || failure.Code != "layout-migration-needed" || !strings.Contains(err.Error(), "Run trailguide project sync once") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if projectTree(t, options).Digest() != before.Digest() {
		t.Fatal("a command that asked to migrate changed the project")
	}
	changes, err := Sync(ctx, options, git)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(changes.Removed, "generated/RULES.md") || !slices.Contains(changes.Added, "_managed/generated/RULES.md") || !slices.Contains(changes.Removed, "vendor/team/rules/techs/go/errors.md") || !slices.Contains(changes.Added, "_managed/vendor/team/rules/techs/go/errors.md") || slices.Contains(changes.Changed, lockFile) {
		t.Errorf("changes = %+v; want vendor/ and generated/ moved and the lockfile unchanged", changes)
	}
	after := projectTree(t, options)
	for name, data := range before.Files {
		if strings.HasPrefix(name, "vendor/") && !bytes.Equal(after.Files["_managed/"+name], data) {
			t.Errorf("the move changed %s", name)
		}
	}
	if !bytes.Equal(after.Files[lockFile], before.Files[lockFile]) {
		t.Error("the move changed the lockfile")
	}
	for _, name := range []string{"vendor", "generated"} {
		if _, err := root.Stat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: %v, want it moved under _managed/", name, err)
		}
	}
	if agents := readRepositoryFile(t, options, "AGENTS.md"); !strings.Contains(agents, "Read `.trailguide/_managed/generated/RULES.md`") {
		t.Errorf("AGENTS.md doesn't point to the moved rules:\n%s", agents)
	}
	requireCurrent(t, options)
}

// TestProject_RefusesAMixedLayout fails, writing nothing, when _managed/ and a tree where Trail Guide 0.6 kept it
// are both present, as a merge of a moved branch with one that wasn't leaves them.
func TestProject_RefusesAMixedLayout(t *testing.T) {
	ctx := context.Background()
	_, options, git := syncProject(t)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "generated/RULES.md", "Older rules.\n")
	before := projectTree(t, options)
	_, buildErr := Build(ctx, options)
	_, syncErr := Sync(ctx, options, git)
	for name, err := range map[string]error{"build": buildErr, "sync": syncErr} {
		var failure *filetxn.Error
		if !errors.As(err, &failure) || failure.Code != "mixed-layout" || !strings.Contains(err.Error(), ".trailguide/generated/") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if projectTree(t, options).Digest() != before.Digest() {
		t.Fatal("a refusal changed the project")
	}
}

// TestAuthoredSkills_LeaveOutTheFoldersGuide reads skills/README.md as the folder's guide, not a skill, and still
// refuses any other file directly in skills/.
func TestAuthoredSkills_LeaveOutTheFoldersGuide(t *testing.T) {
	skill := []byte("---\nname: fix\ndescription: Fix a bug.\n---\n\nFix it.\n")
	found, err := authoredSkills(&filetxn.Tree{Files: map[string][]byte{"README.md": []byte(skillsGuide), "fix/SKILL.md": skill}})
	if err != nil || len(found) != 1 || found["skills/fix"].Files["SKILL.md"] == nil {
		t.Fatalf("got %+v, %v; want only skills/fix", found, err)
	}
	if _, err := authoredSkills(&filetxn.Tree{Files: map[string][]byte{"NOTES.md": nil}}); err == nil || !strings.Contains(err.Error(), "skills/NOTES.md") {
		t.Fatalf("got %v; want NOTES.md refused", err)
	}
}

// TestAddSkill_WritesTheFoldersGuideOnce writes skills/README.md beside the first skill a project authors and leaves
// a guide the project edited alone.
func TestAddSkill_WritesTheFoldersGuideOnce(t *testing.T) {
	ctx := context.Background()
	_, options := localProject(t)
	guide := filepath.Join(options.Directory, ".trailguide", "skills", "README.md")
	if err := os.Remove(guide); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fix", "review"} {
		plan, err := PlanLocalSkill(ctx, name, options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := plan.Commit(ctx, "Fix a bug.", ""); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(guide)
		if err != nil {
			t.Fatal(err)
		}
		if name == "fix" && string(data) != skillsGuide {
			t.Fatalf("skills/README.md = %q", data)
		}
		if name == "fix" {
			if err := os.WriteFile(guide, []byte("# Our skills\n"), 0600); err != nil {
				t.Fatal(err)
			}
		} else if string(data) != "# Our skills\n" {
			t.Fatalf("add skill replaced an edited guide: %q", data)
		}
	}
}
