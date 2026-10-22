// Check rules and change notes against a library's release tags in real Git repositories.

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// releaseOne publishes rules a and b at 1.0.0, as the first library release must.
const releaseOne = "Library release 1.\n---\nformatVersion: 1\nrelease: 1\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: new\n    summaries:\n      - Add a.\n  practices/testing/b:\n    change: new\n    summaries:\n      - Add b.\n"

// ruleText returns a complete rule document with one line of guidance.
func ruleText(guidance string) string {
	return "---\ntitle: Test retries\nimpact: HIGH\nimpactDescription: Catch retry bugs.\nwhenToRead: When changing retries.\n---\n" + guidance + "\n"
}

// libraryFiles is a library with one group, rule a with its own asset, and rule b.
func libraryFiles() map[string][]byte {
	return map[string][]byte{
		"config.yaml":                           []byte("schemaVersion: 2\nsources: {}\nlibrary: {formatVersion: 1}\n"),
		"practices/testing/_group.yaml":         []byte("name: Testing\ndescription: Testing guidance.\nwhenToRead: When testing.\n"),
		"practices/testing/a.md":                []byte(ruleText("Test the retry limit.")),
		"practices/testing/assets/a/example.go": []byte("package example\n"),
		"practices/testing/b.md":                []byte(ruleText("Test the backoff.")),
	}
}

// authorClone returns a library author's full clone of a fixture holding files, keyed by library path, and options
// that check it.
// Each tag message in tags is applied, in order, to the fixture's latest commit before cloning.
func authorClone(t *testing.T, files map[string][]byte, tags ...string) (*gitfixture.Fixture, Options) {
	t.Helper()
	ctx := context.Background()
	fixture, err := gitfixture.New(ctx, gitfixture.Library(files))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(); err != nil {
			t.Error(err)
		}
	})
	for i, message := range tags {
		if _, err := fixture.Command(ctx, "tag", "--annotate", "--cleanup=verbatim", "--message", message, "release/"+strconv.Itoa(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := fixture.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, Options{Directory: dir, Git: gitexec.Options{GitPath: fixture.GitPath, Environment: fixture.Environment}}
}

// edit writes each file in the library repository at dir, keyed by library path, removing those mapped to "".
func edit(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(librarytree.RepositoryPath(name)))
		if content == "" {
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// previewRows renders each pending rule as "id change current next", with "-" for an absent version.
func previewRows(preview PendingRelease) []string {
	rows := []string{}
	for _, rule := range preview.Rules {
		current, next := "-", "-"
		if rule.From != nil {
			current = rule.From.String()
		}
		if rule.LastVersion != nil {
			current = rule.LastVersion.String()
		}
		if rule.To != nil {
			next = rule.To.String()
		}
		row := rule.ID + " " + string(rule.Change) + " " + current + " " + next
		if rule.ReplacedBy != "" {
			row += " replacedBy " + rule.ReplacedBy
		}
		rows = append(rows, row)
	}
	return rows
}

// checkWithPlan runs library check on options' library, also returning the plan of the next library release.
func checkWithPlan(ctx context.Context, options Options) (CheckResult, releasePlan, error) {
	root, err := openLibrary(ctx, options, false)
	if err != nil {
		return CheckResult{}, releasePlan{}, err
	}
	defer root.Close()
	git, err := openLibraryGit(ctx, root.Name(), options.Git)
	if err != nil {
		return CheckResult{}, releasePlan{}, err
	}
	checked, err := checkLibrary(ctx, root, git, nil, nil)
	return checked.result, checked.plan, err
}

// errorCode returns a domain error's stable code, or "" for another error.
func errorCode(err error) string {
	var domain *filetxn.Error
	if errors.As(err, &domain) {
		return domain.Code
	}
	return ""
}

// TestCheck_RequiresNotesThatMatchChangesSinceTheLatestLibraryRelease covers every documented failure and the preview.
func TestCheck_RequiresNotesThatMatchChangesSinceTheLatestLibraryRelease(t *testing.T) {
	changedA := ruleText("Test the retry limit and one past it.")
	ruleC := ruleText("Test every retry.")
	for _, test := range []struct {
		name  string
		files map[string]string
		// problems are the expected failures' distinctive text; empty means the check passes with rows.
		problems []string
		rows     []string
		// retired maps each rule the plan retires to its expected summaries, one per note in note order.
		retired map[string][]string
	}{
		{name: "unchanged library", rows: []string{}},
		{name: "changed rule without a note", files: map[string]string{"practices/testing/a.md": changedA},
			problems: []string{"practices/testing/a changed since release/1, where its version is 1.0.0, and no pending change note names it. Record it with: trailguide library change practices/testing/a"}},
		{name: "changed rule asset without a note", files: map[string]string{"practices/testing/assets/a/example.go": "package changed\n"},
			problems: []string{"practices/testing/a changed since release/1"}},
		{name: "added rule asset without a note", files: map[string]string{"practices/testing/assets/a/more.go": "package more\n"},
			problems: []string{"practices/testing/a changed since release/1"}},
		{name: "deleted rule asset without a note", files: map[string]string{"practices/testing/assets/a": ""},
			problems: []string{"practices/testing/a changed since release/1"}},
		{name: "changed rule with a note", files: map[string]string{"practices/testing/a.md": changedA, "changes/one.yaml": "summary: Test one past the limit.\nrules:\n  practices/testing/a: minor\n"},
			rows: []string{"practices/testing/a minor 1.0.0 1.1.0"}},
		{name: "largest of several notes", files: map[string]string{"practices/testing/a.md": changedA, "changes/one.yaml": "summary: Fix a typo.\nrules:\n  practices/testing/a: patch\n", "changes/two.yaml": "summary: Require a test past the limit.\nrules:\n  practices/testing/a: major\n"},
			rows: []string{"practices/testing/a major 1.0.0 2.0.0"}},
		{name: "new rule without a note", files: map[string]string{"practices/testing/c.md": ruleC},
			problems: []string{"practices/testing/c is a new rule, and no pending change note names it. Record it with: trailguide library change practices/testing/c"}},
		{name: "new rule with a note", files: map[string]string{"practices/testing/c.md": ruleC, "changes/c.yaml": "summary: Add c.\nrules:\n  practices/testing/c: new\n"},
			rows: []string{"practices/testing/c new - 1.0.0"}},
		{name: "deleted rule without a retirement", files: map[string]string{"practices/testing/b.md": ""},
			problems: []string{"practices/testing/b, version 1.0.0, was deleted, and no pending change note retires it. Restore it, or record the retirement with: trailguide library change practices/testing/b --retire"}},
		{name: "rename", files: map[string]string{"practices/testing/b.md": "", "practices/testing/c.md": ruleC, "changes/rename.yaml": "summary: Rename b.\nrules:\n  practices/testing/b:\n    change: retired\n    replacedBy: practices/testing/c\n  practices/testing/c: new\n"},
			rows: []string{"practices/testing/b retired 1.0.0 - replacedBy practices/testing/c", "practices/testing/c new - 1.0.0"}},
		{name: "retirement outweighs a change", files: map[string]string{"practices/testing/b.md": "", "changes/one.yaml": "summary: Fix b.\nrules:\n  practices/testing/b: patch\n", "changes/two.yaml": "summary: Stop testing backoff.\nrules:\n  practices/testing/b: retired\n"},
			rows:    []string{"practices/testing/b retired 1.0.0 -"},
			retired: map[string][]string{"practices/testing/b": {"Fix b.", "Stop testing backoff."}}},
		{name: "replacement isn't a rule", files: map[string]string{"practices/testing/b.md": "", "changes/retire.yaml": "summary: Replace b.\nrules:\n  practices/testing/b:\n    change: retired\n    replacedBy: practices/testing/missing\n"},
			problems: []string{"changes/retire.yaml retires practices/testing/b in favor of practices/testing/missing, which isn't a rule in the library."}},
		{name: "different replacements", files: map[string]string{"practices/testing/b.md": "", "practices/testing/c.md": ruleC, "changes/c.yaml": "summary: Add c.\nrules:\n  practices/testing/c: new\n", "changes/one.yaml": "summary: Replace b.\nrules:\n  practices/testing/b:\n    change: retired\n    replacedBy: practices/testing/c\n", "changes/two.yaml": "summary: Retire b.\nrules:\n  practices/testing/b: retired\n"},
			problems: []string{"pending change notes name different replacements for practices/testing/b"}},
		{name: "stale note", files: map[string]string{"changes/stale.yaml": "summary: Fix a.\nrules:\n  practices/testing/a: patch\n"},
			problems: []string{"changes/stale.yaml names practices/testing/a, which is unchanged since release/1, so the note is stale."}},
		{name: "note names a missing rule", files: map[string]string{"changes/missing.yaml": "summary: Fix it.\nrules:\n  practices/testing/missing: patch\n"},
			problems: []string{"changes/missing.yaml names practices/testing/missing, which isn't a rule in the library and isn't being retired."}},
		{name: "new note for a published rule", files: map[string]string{"practices/testing/a.md": changedA, "changes/new.yaml": "summary: Add a.\nrules:\n  practices/testing/a: new\n"},
			problems: []string{"changes/new.yaml names practices/testing/a, which already has version 1.0.0. Record it as major, minor, or patch."}},
		{name: "version change for a new rule", files: map[string]string{"practices/testing/c.md": ruleC, "changes/c.yaml": "summary: Add c.\nrules:\n  practices/testing/c: minor\n"},
			problems: []string{"changes/c.yaml names practices/testing/c, which has no version yet. Record it as new."}},
		{name: "retirement of a rule that still exists", files: map[string]string{"changes/retire.yaml": "summary: Retire a.\nrules:\n  practices/testing/a: retired\n"},
			problems: []string{"changes/retire.yaml names practices/testing/a, which still exists. To retire it, delete its Markdown file and asset directory."}},
		{name: "retirement of an unpublished rule", files: map[string]string{"changes/retire.yaml": "summary: Retire c.\nrules:\n  practices/testing/c: retired\n"},
			problems: []string{"changes/retire.yaml names practices/testing/c, which was never published, so it can't be retired. Remove it from the note."}},
		{name: "several problems", files: map[string]string{"practices/testing/a.md": changedA, "practices/testing/b.md": ""},
			problems: []string{"practices/testing/a changed since release/1", "practices/testing/b, version 1.0.0, was deleted"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, options := authorClone(t, libraryFiles(), releaseOne)
			edit(t, options.Directory, test.files)
			result, plan, err := checkWithPlan(context.Background(), options)
			if len(test.problems) > 0 {
				if errorCode(err) != "change-notes" || !strings.Contains(err.Error(), "change notes don't match the rule changes since release/1:") {
					t.Fatalf("expected a change-notes failure, got %v", err)
				}
				for _, problem := range test.problems {
					if !strings.Contains(err.Error(), "\n  - "+problem) {
						t.Errorf("missing problem %q in:\n%s", problem, err)
					}
				}
				if got := strings.Count(err.Error(), "\n  - "); got != len(test.problems) {
					t.Errorf("reported %d problems, want %d:\n%s", got, len(test.problems), err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.PendingRelease.Release != 2 || !slices.Equal(previewRows(result.PendingRelease), test.rows) {
				t.Fatalf("pending release %d %q, want 2 %q", result.PendingRelease.Release, previewRows(result.PendingRelease), test.rows)
			}
			for id, summaries := range test.retired {
				if !slices.Equal(plan.retired[id].Summaries, summaries) {
					t.Fatalf("retired %s with summaries %q, want %q", id, plan.retired[id].Summaries, summaries)
				}
			}
		})
	}
}

// TestCheck_ComparesCommittedChangesWithTheReleaseTag finds changes committed after the tag, not just uncommitted ones.
func TestCheck_ComparesCommittedChangesWithTheReleaseTag(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	if _, err := fixture.Commit(ctx, options.Directory, "Change a", gitfixture.Library(map[string][]byte{"practices/testing/a.md": []byte(ruleText("Changed."))})); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(ctx, options); errorCode(err) != "change-notes" || !strings.Contains(err.Error(), "practices/testing/a changed since release/1") {
		t.Fatalf("committed change passed: %v", err)
	}
	if _, err := fixture.Commit(ctx, options.Directory, "Record a", gitfixture.Library(map[string][]byte{"changes/a.yaml": []byte("summary: Change a.\nrules:\n  practices/testing/a: patch\n")})); err != nil {
		t.Fatal(err)
	}
	result, err := Check(ctx, options)
	if err != nil || !slices.Equal(previewRows(result.PendingRelease), []string{"practices/testing/a patch 1.0.0 1.0.1"}) {
		t.Fatal(result, err)
	}
}

// TestCheck_UsesTheLatestReachableLibraryRelease reads versions, published notes, and retirements from every reachable tag.
func TestCheck_UsesTheLatestReachableLibraryRelease(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	dir := options.Directory
	// Release 2 changes a, retires b, and publishes the note that records both.
	note := "summary: Tighten a and retire b.\nrules:\n  practices/testing/a: major\n  practices/testing/b: retired\n"
	if _, err := fixture.Commit(ctx, dir, "Release 2", gitfixture.Library(map[string][]byte{"practices/testing/a.md": []byte(ruleText("Stricter.")), "practices/testing/b.md": nil, "changes/two.yaml": []byte(note)})); err != nil {
		t.Fatal(err)
	}
	two := "Library release 2.\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 2.0.0\nchanges:\n  practices/testing/a:\n    change: major\n    from: 1.0.0\n    summaries:\n      - Tighten a and retire b.\nretired:\n  practices/testing/b:\n    lastVersion: 1.0.0\n    summaries:\n      - Tighten a and retire b.\n"
	if err := fixture.Tag(ctx, dir, "release/2", two); err != nil {
		t.Fatal(err)
	}
	// A later release on another branch isn't in this branch's history.
	if _, err := fixture.CommandIn(ctx, dir, "switch", "--quiet", "--create", "elsewhere"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Commit(ctx, dir, "Elsewhere", gitfixture.Library(map[string][]byte{"README.md": []byte("elsewhere\n")})); err != nil {
		t.Fatal(err)
	}
	three := "Library release 3.\n---\nformatVersion: 1\nrelease: 3\nrules:\n  practices/testing/a: 2.0.0\n"
	if err := fixture.Tag(ctx, dir, "release/3", three); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.CommandIn(ctx, dir, "switch", "--quiet", "main"); err != nil {
		t.Fatal(err)
	}
	result, err := Check(ctx, options)
	if err != nil || result.PendingRelease.Release != 3 || len(result.PendingRelease.Rules) != 0 || len(result.Warnings) != 1 {
		t.Fatal(result, err)
	}
	edit(t, dir, map[string]string{"practices/testing/a.md": ruleText("Stricter still."), "changes/three.yaml": "summary: Stricter still.\nrules:\n  practices/testing/a: minor\n"})
	result, err = Check(ctx, options)
	if err != nil || !slices.Equal(previewRows(result.PendingRelease), []string{"practices/testing/a minor 2.0.0 2.1.0"}) {
		t.Fatal(result, err)
	}
	edit(t, dir, map[string]string{"changes/two.yaml": "summary: Reworded after publishing.\nrules:\n  practices/testing/a: major\n  practices/testing/b: retired\n"})
	result, err = Check(ctx, options)
	if err != nil || !slices.Contains(result.Warnings, "changes/two.yaml changed after a library release published it. Editing a published note has no effect.") {
		t.Fatal(result, err)
	}
	edit(t, dir, map[string]string{"practices/testing/b.md": ruleText("Back again."), "changes/b.yaml": "summary: Restore b.\nrules:\n  practices/testing/b: new\n"})
	if _, err := Check(ctx, options); errorCode(err) != "change-notes" || !strings.Contains(err.Error(), "practices/testing/b reuses the ID of a rule that release/2 retired. Retired IDs can't be reused; give the rule a new ID.") {
		t.Fatal(err)
	}
	edit(t, dir, map[string]string{"practices/testing/b.md": "", "changes/b.yaml": "summary: Retire b again.\nrules:\n  practices/testing/b: retired\n"})
	if _, err := Check(ctx, options); errorCode(err) != "change-notes" || !strings.Contains(err.Error(), "changes/b.yaml names practices/testing/b, which release/2 already retired.") {
		t.Fatal(err)
	}
}

// TestCheck_OnlyWarnsAboutAnInvalidEditToAPublishedNote, which has no effect, while an invalid pending note fails.
func TestCheck_OnlyWarnsAboutAnInvalidEditToAPublishedNote(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	if _, err := fixture.Commit(ctx, options.Directory, "Release 2", gitfixture.Library(map[string][]byte{"practices/testing/a.md": []byte(ruleText("Two.")), "changes/one.yaml": []byte("summary: Fix a.\nrules:\n  practices/testing/a: patch\n")})); err != nil {
		t.Fatal(err)
	}
	two := "Library release 2.\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.1\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: patch\n    from: 1.0.0\n    summaries:\n      - Fix a.\n"
	if err := fixture.Tag(ctx, options.Directory, "release/2", two); err != nil {
		t.Fatal(err)
	}
	edit(t, options.Directory, map[string]string{"changes/one.yaml": "summary: [unclosed\n"})
	result, err := Check(ctx, options)
	if err != nil || !slices.Contains(result.Warnings, "changes/one.yaml changed after a library release published it. Editing a published note has no effect.") {
		t.Fatal(result, err)
	}
	edit(t, options.Directory, map[string]string{"changes/two.yaml": "summary: [unclosed\n"})
	var validation errs.ValidationError
	if _, err := Check(ctx, options); !errors.As(err, &validation) || validation.ValidationLocation() != "changes/two.yaml" {
		t.Fatalf("accepted an invalid pending note: %v", err)
	}
}

// TestCheck_WarnsAboutADeletedPublishedNote because notes are never deleted.
func TestCheck_WarnsAboutADeletedPublishedNote(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	if _, err := fixture.Commit(ctx, options.Directory, "Release 2", gitfixture.Library(map[string][]byte{"practices/testing/a.md": []byte(ruleText("Two.")), "changes/one.yaml": []byte("summary: Fix a.\nrules:\n  practices/testing/a: patch\n")})); err != nil {
		t.Fatal(err)
	}
	two := "Library release 2.\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.1\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: patch\n    from: 1.0.0\n    summaries:\n      - Fix a.\n"
	if err := fixture.Tag(ctx, options.Directory, "release/2", two); err != nil {
		t.Fatal(err)
	}
	edit(t, options.Directory, map[string]string{"changes/one.yaml": ""})
	result, err := Check(ctx, options)
	if err != nil || !slices.Contains(result.Warnings, "changes/one.yaml was deleted after a library release published it. Notes are never deleted; restore it.") || len(result.PendingRelease.Rules) != 0 {
		t.Fatal(result, err)
	}
}

// TestCheck_WarnsWhenARetirementRetiresAnEarlierReplacement: release 2 retires b in favor of a, and a pending note
// retires a. Without a replacement for a, the warning says projects would be pointed at a retired rule and asks for
// one; with one, it says updates will point them on to it.
func TestCheck_WarnsWhenARetirementRetiresAnEarlierReplacement(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	files["practices/testing/c.md"] = []byte(ruleText("Test everything about retries."))
	fixture, options := authorClone(t, files, strings.Replace(releaseOne, "  practices/testing/b: 1.0.0\n", "  practices/testing/b: 1.0.0\n  practices/testing/c: 1.0.0\n", 1)+"  practices/testing/c:\n    change: new\n    summaries:\n      - Add c.\n")
	if _, err := fixture.Commit(ctx, options.Directory, "Release 2", gitfixture.Library(map[string][]byte{"practices/testing/b.md": nil, "changes/two.yaml": []byte("summary: Fold b into a.\nrules:\n  practices/testing/b: {change: retired, replacedBy: practices/testing/a}\n")})); err != nil {
		t.Fatal(err)
	}
	two := "Library release 2.\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/c: 1.0.0\nretired:\n  practices/testing/b:\n    lastVersion: 1.0.0\n    replacedBy: practices/testing/a\n    summaries:\n      - Fold b into a.\n"
	if err := fixture.Tag(ctx, options.Directory, "release/2", two); err != nil {
		t.Fatal(err)
	}
	warning := "The pending retirement of practices/testing/a retires the replacement that release/2 named for practices/testing/b, so projects still importing practices/testing/b would be pointed at a retired rule."
	result, err := Check(ctx, options)
	if err != nil || slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.HasPrefix(w, "The pending retirement") }) {
		t.Fatalf("warned before anything retired the replacement: %v, %v", result.Warnings, err)
	}
	edit(t, options.Directory, map[string]string{"practices/testing/a.md": "", "practices/testing/assets/a": "", "changes/three.yaml": "summary: Retire a.\nrules:\n  practices/testing/a: retired\n"})
	result, err = Check(ctx, options)
	if err != nil || !slices.Contains(result.Warnings, warning+" To let them follow it, name a replacement for practices/testing/a as replacedBy in the note that retires it.") {
		t.Fatalf("warnings %q, %v", result.Warnings, err)
	}
	edit(t, options.Directory, map[string]string{"changes/three.yaml": "summary: Fold a into c.\nrules:\n  practices/testing/a: {change: retired, replacedBy: practices/testing/c}\n"})
	result, err = Check(ctx, options)
	onward := "The pending retirement of practices/testing/a retires the replacement that release/2 named for practices/testing/b, so updates will point projects still importing practices/testing/b on to practices/testing/c, the replacement the note names."
	if err != nil || !slices.Contains(result.Warnings, onward) || slices.Contains(result.Warnings, warning) {
		t.Fatalf("warnings %q, %v", result.Warnings, err)
	}
}

// TestCheck_IgnoresLineEndingConversion compares content as Git stores it, so a checkout's converted line endings aren't a change.
func TestCheck_IgnoresLineEndingConversion(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	files[".gitattributes"] = []byte("*.md text eol=crlf\n")
	_, options := authorClone(t, files, releaseOne)
	data, err := os.ReadFile(filepath.Join(options.Directory, librarytree.RepositoryPath("practices/testing/a.md")))
	if err != nil || !strings.Contains(string(data), "\r\n") {
		t.Fatalf("checkout kept LF line endings: %q %v", data, err)
	}
	if result, err := Check(ctx, options); err != nil || len(result.PendingRelease.Rules) != 0 {
		t.Fatal(result, err)
	}
}

// TestCheck_BeforeTheFirstLibraryRelease needs no notes and previews every rule at 1.0.0, in Git or outside it.
func TestCheck_BeforeTheFirstLibraryRelease(t *testing.T) {
	ctx := context.Background()
	_, inGit := authorClone(t, libraryFiles())
	outside := Options{Directory: t.TempDir()}
	for name, content := range libraryFiles() {
		edit(t, outside.Directory, map[string]string{name: string(content)})
	}
	for name, options := range map[string]Options{"Git": inGit, "outside Git": outside} {
		t.Run(name, func(t *testing.T) {
			edit(t, options.Directory, map[string]string{"practices/testing/a.md": ruleText("Changed."), "changes/early.yaml": "summary: Notes before the first library release are only validated.\nrules:\n  practices/testing/missing: patch\n"})
			result, err := Check(ctx, options)
			want := []string{"practices/testing/a new - 1.0.0", "practices/testing/b new - 1.0.0"}
			if err != nil || result.PendingRelease.Release != 1 || !slices.Equal(previewRows(result.PendingRelease), want) {
				t.Fatal(result, err)
			}
			edit(t, options.Directory, map[string]string{"changes/early.yaml": "summary: Unknown change.\nrules:\n  practices/testing/a: huge\n"})
			var validation errs.ValidationError
			if _, err := Check(ctx, options); !errors.As(err, &validation) || validation.ValidationLocation() != "changes/early.yaml.rules.practices/testing/a" {
				t.Fatalf("invalid note accepted: %v", err)
			}
		})
	}
}

// TestCheck_RejectsHistoryItCannotCompare fails closed on a shallow clone and on hand-made release tags.
func TestCheck_RejectsHistoryItCannotCompare(t *testing.T) {
	ctx := context.Background()
	t.Run("shallow clone", func(t *testing.T) {
		fixture, options := authorClone(t, libraryFiles(), releaseOne)
		shallow := filepath.Join(t.TempDir(), "shallow")
		if _, err := fixture.CommandIn(ctx, filepath.Dir(shallow), "clone", "--quiet", "--depth=1", "--template=", fixture.Repository, shallow); err != nil {
			t.Fatal(err)
		}
		options.Directory = shallow
		if _, err := Check(ctx, options); errorCode(err) != "shallow-clone" || !strings.Contains(err.Error(), "git fetch --unshallow --tags") || !strings.Contains(err.Error(), "fetch-depth: 0") {
			t.Fatal(err)
		}
	})
	t.Run("clone without tags", func(t *testing.T) {
		files := libraryFiles()
		files["changes/clarify-a.yaml"] = []byte("summary: Clarify a.\nrules:\n  practices/testing/a: patch\n")
		fixture, options := authorClone(t, files, releaseOne)
		untagged := filepath.Join(t.TempDir(), "untagged")
		if _, err := fixture.CommandIn(ctx, filepath.Dir(untagged), "clone", "--quiet", "--no-tags", "--template=", fixture.Repository, untagged); err != nil {
			t.Fatal(err)
		}
		options.Directory = untagged
		if _, err := Check(ctx, options); errorCode(err) != "missing-release-tags" || !strings.Contains(err.Error(), "git fetch --tags") || !strings.Contains(err.Error(), "fetch-depth: 0") {
			t.Fatal(err)
		}
		if _, err := fixture.CommandIn(ctx, untagged, "fetch", "--quiet", "--tags"); err != nil {
			t.Fatal(err)
		}
		if _, err := Check(ctx, options); err != nil {
			t.Fatalf("check after fetching the tags: %v", err)
		}
	})
	// Every hand-made release tag fails with one code; an invalid record also keeps the parser's location.
	for name, test := range map[string]struct {
		tag      func(*gitfixture.Fixture, string) error
		location string
	}{
		"lightweight tag": {func(f *gitfixture.Fixture, dir string) error {
			_, err := f.CommandIn(ctx, dir, "tag", "release/2")
			return err
		}, ""},
		"mismatched number": {func(f *gitfixture.Fixture, dir string) error {
			return f.Tag(ctx, dir, "release/2", strings.Replace(releaseOne, "release: 1", "release: 3", 1))
		}, "release/2.release"},
		"missing record": {func(f *gitfixture.Fixture, dir string) error {
			return f.Tag(ctx, dir, "release/2", "Only notes.\n")
		}, "release/2"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, options := authorClone(t, libraryFiles(), releaseOne)
			if err := test.tag(fixture, options.Directory); err != nil {
				t.Fatal(err)
			}
			_, err := Check(ctx, options)
			var validation errs.ValidationError
			if errorCode(err) != "invalid-release-tag" || !strings.Contains(err.Error(), "by hand") {
				t.Fatalf("accepted %s: %v", name, err)
			}
			if test.location != "" && (!errors.As(err, &validation) || validation.ValidationLocation() != test.location) {
				t.Fatalf("%s lost its location: %v", name, err)
			}
		})
	}
	t.Run("tags other than release numbers", func(t *testing.T) {
		fixture, options := authorClone(t, libraryFiles(), releaseOne)
		for _, name := range []string{"release/01", "release/v2", "release/2/extra", "v3.0.0"} {
			if _, err := fixture.CommandIn(ctx, options.Directory, "tag", name); err != nil {
				t.Fatal(err)
			}
		}
		if result, err := Check(ctx, options); err != nil || result.PendingRelease.Release != 2 {
			t.Fatal(result, err)
		}
	})
}

// TestCheck_RejectsVersionsPastTheLimit reports a change that would advance a version past the largest number.
func TestCheck_RejectsVersionsPastTheLimit(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	two := "Library release 2.\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.999999999.0\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: minor\n    from: 1.999999998.0\n    summaries:\n      - Large.\n"
	if err := fixture.Tag(ctx, options.Directory, "release/2", two); err != nil {
		t.Fatal(err)
	}
	edit(t, options.Directory, map[string]string{"practices/testing/a.md": ruleText("Changed."), "changes/a.yaml": "summary: More.\nrules:\n  practices/testing/a: minor\n"})
	if _, err := Check(ctx, options); errorCode(err) != "version-limit" || !strings.Contains(err.Error(), "practices/testing/a: a minor change from 1.999999999.0 exceeds the largest rule version number") {
		t.Fatal(err)
	}
}

// TestCheck_RejectsFilesThatArentNotesInChanges keeps changes/ to change notes only.
func TestCheck_RejectsFilesThatArentNotesInChanges(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"other extension": {"changes/README.md": "Notes.\n"},
		"subdirectory":    {"changes/old/one.yaml": "summary: One.\nrules:\n  practices/testing/a: patch\n"},
	} {
		t.Run(name, func(t *testing.T) {
			options := Options{Directory: t.TempDir()}
			for name, content := range libraryFiles() {
				edit(t, options.Directory, map[string]string{name: string(content)})
			}
			edit(t, options.Directory, files)
			if _, err := Check(context.Background(), options); err == nil || !strings.Contains(err.Error(), "expected only change notes, as .yaml files, in .trailguide/changes/") {
				t.Fatal(err)
			}
		})
	}
}

// TestCheck_RejectsTermsInsideARulesVersion refuses a license file declared in a rule's asset directory, before
// and after the first library release, instead of accepting a layout that can't be published or treating a
// license edit as a rule change.
func TestCheck_RejectsTermsInsideARulesVersion(t *testing.T) {
	files := libraryFiles()
	files["config.yaml"] = []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n  license:\n    file: .trailguide/rules/practices/testing/assets/a/LICENSE\n    notices: []\n")
	files["practices/testing/assets/a/LICENSE"] = []byte("Terms.\n")
	for name, tags := range map[string][]string{"before the first library release": nil, "after a library release": {releaseOne}} {
		t.Run(name, func(t *testing.T) {
			_, options := authorClone(t, files, tags...)
			_, err := Check(context.Background(), options)
			var invalid errs.ValidationError
			if !errors.As(err, &invalid) || !strings.HasSuffix(invalid.ValidationLocation(), ".trailguide/config.yaml: library.license.file") || !strings.Contains(invalid.ValidationProblem(), "belongs to a rule's version") {
				t.Fatalf("got %v; want the license path refused", err)
			}
		})
	}
}

// TestCheck_AsksToUpgradeForANewerReleaseRecordFormat refuses a library release whose record a later Trail Guide
// wrote, with its own code.
func TestCheck_AsksToUpgradeForANewerReleaseRecordFormat(t *testing.T) {
	_, options := authorClone(t, libraryFiles(), strings.Replace(releaseOne, "formatVersion: 1", "formatVersion: 2", 1))
	_, err := Check(context.Background(), options)
	if errorCode(err) != "unsupported-release-record" || !strings.Contains(err.Error(), "Upgrade Trail Guide") {
		t.Fatalf("got %v", err)
	}
}
