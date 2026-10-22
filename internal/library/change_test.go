// Record change notes against real library history, and check that the notes satisfy library check.

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// noteDay is the calendar day tests record notes on.
var noteDay = time.Date(2026, time.September, 29, 23, 30, 0, 0, time.UTC)

// recordChange plans and commits a note, returning its library path and its contents.
func recordChange(t *testing.T, options Options, request ChangeRequest) (string, string, error) {
	t.Helper()
	plan, err := PlanChange(context.Background(), request, options)
	if err != nil {
		return "", "", err
	}
	result, err := plan.Commit(context.Background(), request.Bump, request.Summary, noteDay)
	if err != nil {
		return "", "", err
	}
	if len(result.Written()) != 1 {
		t.Fatalf("wrote %q, want one note", result.Written())
	}
	relative, err := filepath.Rel(options.Directory, result.Written()[0])
	if err != nil {
		t.Fatal(err)
	}
	name, ok := librarytree.LibraryPath(filepath.ToSlash(relative))
	if !ok {
		t.Fatalf("wrote %s, outside the library", relative)
	}
	data, err := os.ReadFile(result.Written()[0])
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(name), string(data), nil
}

// TestChange_WritesNotesThatLibraryCheckAccepts records each kind of change and checks the pending release.
func TestChange_WritesNotesThatLibraryCheckAccepts(t *testing.T) {
	for _, test := range []struct {
		name    string
		files   map[string]string
		request ChangeRequest
		// note is the note's name before its random suffix.
		note string
		want string
		rows []string
	}{
		{name: "version change", files: map[string]string{"practices/testing/a.md": ruleText("Changed.")},
			request: ChangeRequest{IDs: []string{"practices/testing/a"}, Bump: trailguide.ChangeMinor, Summary: "Add a Python example of the retry-limit test."},
			note:    "changes/2026-09-29-a", want: "summary: Add a Python example of the retry-limit test.\nrules:\n  practices/testing/a: minor\n",
			rows: []string{"practices/testing/a minor 1.0.0 1.1.0"}},
		{name: "several rules", files: map[string]string{"practices/testing/a.md": ruleText("Changed."), "practices/testing/b.md": ruleText("Changed too.")},
			request: ChangeRequest{IDs: []string{"practices/testing/b", "practices/testing/a"}, Bump: trailguide.ChangeMajor, Summary: "Require a test at every limit."},
			note:    "changes/2026-09-29-b", want: "summary: Require a test at every limit.\nrules:\n  practices/testing/a: major\n  practices/testing/b: major\n",
			rows: []string{"practices/testing/a major 1.0.0 2.0.0", "practices/testing/b major 1.0.0 2.0.0"}},
		{name: "new rule", files: map[string]string{"practices/testing/verify-backoff.md": ruleText("New.")},
			request: ChangeRequest{IDs: []string{"practices/testing/verify-backoff"}, Summary: "Add a rule about testing retry backoff."},
			note:    "changes/2026-09-29-verify-backoff", want: "summary: Add a rule about testing retry backoff.\nrules:\n  practices/testing/verify-backoff: new\n",
			rows: []string{"practices/testing/verify-backoff new - 1.0.0"}},
		{name: "retirement with a replacement", files: map[string]string{"practices/testing/b.md": ""},
			request: ChangeRequest{IDs: []string{"practices/testing/b"}, Retire: true, ReplacedBy: "practices/testing/a", Summary: "Covered by a."},
			note:    "changes/2026-09-29-b", want: "summary: Covered by a.\nrules:\n  practices/testing/b:\n    change: retired\n    replacedBy: practices/testing/a\n",
			rows: []string{"practices/testing/b retired 1.0.0 - replacedBy practices/testing/a"}},
		{name: "retirement without a replacement", files: map[string]string{"practices/testing/a.md": "", "practices/testing/assets/a": ""},
			request: ChangeRequest{IDs: []string{"practices/testing/a"}, Retire: true, Summary: "Agents shouldn't test retries this way."},
			note:    "changes/2026-09-29-a", want: "summary: Agents shouldn't test retries this way.\nrules:\n  practices/testing/a: retired\n",
			rows: []string{"practices/testing/a retired 1.0.0 -"}},
		{name: "summary that YAML must quote", files: map[string]string{"practices/testing/a.md": ruleText("Changed.")},
			request: ChangeRequest{IDs: []string{"practices/testing/a"}, Bump: trailguide.ChangePatch, Summary: "  Fix: '#' is \"quoted\" - " + strings.Repeat("long ", 40) + " "},
			note:    "changes/2026-09-29-a",
			rows:    []string{"practices/testing/a patch 1.0.0 1.0.1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, options := authorClone(t, libraryFiles(), releaseOne)
			edit(t, options.Directory, test.files)
			name, content, err := recordChange(t, options, test.request)
			if err != nil || !regexp.MustCompile("^"+test.note+"-[0-9a-f]{6}\\.yaml$").MatchString(name) || (test.want != "" && content != test.want) {
				t.Fatalf("wrote %s:\n%s\nerror %v; want %s:\n%s", name, content, err, test.note, test.want)
			}
			note, err := rules.ParseChangeNote([]byte(content), name)
			if lines := strings.Split(content, "\n"); err != nil || note.Summary != strings.TrimSpace(test.request.Summary) || !strings.HasPrefix(lines[0], "summary: ") || lines[1] != "rules:" {
				t.Fatalf("note %+v does not keep the summary on one line: %v\n%s", note, err, content)
			}
			result, err := Check(context.Background(), options)
			if err != nil || !slices.Equal(previewRows(result.PendingRelease), test.rows) {
				t.Fatalf("check after the note: %q %v", previewRows(result.PendingRelease), err)
			}
		})
	}
}

// TestChange_NamesEachNoteUniquely never edits an existing note or reuses a published note's name.
func TestChange_NamesEachNoteUniquely(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	published := "summary: Published.\nrules:\n  practices/testing/a: patch\n"
	if _, err := fixture.Commit(ctx, options.Directory, "Release 2", gitfixture.Library(map[string][]byte{"practices/testing/a.md": []byte(ruleText("Two.")), "changes/2026-09-29-a-aaaaaa.yaml": []byte(published)})); err != nil {
		t.Fatal(err)
	}
	two := "Library release 2.\n---\nschemaVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.1\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: patch\n    from: 1.0.0\n    summaries:\n      - Published.\n"
	if err := fixture.Tag(ctx, options.Directory, "release/2", two); err != nil {
		t.Fatal(err)
	}
	// The published note is gone from the working tree, but its name stays taken.
	pending := "summary: Pending.\nrules:\n  practices/testing/a: patch\n"
	edit(t, options.Directory, map[string]string{"changes/2026-09-29-a-aaaaaa.yaml": "", "changes/2026-09-29-a-bbbbbb.yaml": pending, "practices/testing/a.md": ruleText("Three.")})
	git, err := openLibraryGit(ctx, options.Directory, options.Git)
	if err != nil {
		t.Fatal(err)
	}
	released, err := git.history(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	suffixes := []string{"aaaaaa", "bbbbbb", "cccccc"}
	next := func() (string, error) {
		suffix := suffixes[0]
		suffixes = suffixes[1:]
		return suffix, nil
	}
	if name, err := noteName(root, "practices/testing/a", noteDay, released.latest.files, next); err != nil || name != "changes/2026-09-29-a-cccccc.yaml" {
		t.Fatalf("chose %s: %v", name, err)
	}
	request := ChangeRequest{IDs: []string{"practices/testing/a"}, Bump: trailguide.ChangePatch, Summary: "Another fix."}
	first, _, err := recordChange(t, options, request)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := recordChange(t, options, request)
	if err != nil || second == first {
		t.Fatalf("wrote %s after %s: %v", second, first, err)
	}
	if data, err := os.ReadFile(filepath.Join(options.Directory, librarytree.RepositoryPath("changes/2026-09-29-a-bbbbbb.yaml"))); err != nil || string(data) != pending {
		t.Fatalf("changed an existing note: %q %v", data, err)
	}
}

// TestChange_NotesFromParallelBranchesMergeWithoutConflict records the same rule on the same day in two clones.
func TestChange_NotesFromParallelBranchesMergeWithoutConflict(t *testing.T) {
	ctx := context.Background()
	fixture, first := authorClone(t, libraryFiles(), releaseOne)
	secondDirectory, err := fixture.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Directory = secondDirectory
	edit(t, first.Directory, map[string]string{"practices/testing/a.md": ruleText("Test one past the limit.")})
	edit(t, second.Directory, map[string]string{"practices/testing/assets/a/example.go": "package example // at the limit\n"})
	for _, clone := range []struct {
		options Options
		bump    trailguide.Change
	}{{first, trailguide.ChangeMinor}, {second, trailguide.ChangePatch}} {
		if _, _, err := recordChange(t, clone.options, ChangeRequest{IDs: []string{"practices/testing/a"}, Bump: clone.bump, Summary: "Change a as " + string(clone.bump) + "."}); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Commit(ctx, clone.options.Directory, "Change a", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixture.CommandIn(ctx, first.Directory, "push", "--quiet", "origin", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.CommandIn(ctx, second.Directory, "pull", "--quiet", "--no-rebase", "--no-edit", "origin", "main"); err != nil {
		t.Fatalf("merging the parallel notes failed: %v", err)
	}
	result, err := Check(ctx, second)
	if err != nil || !slices.Equal(previewRows(result.PendingRelease), []string{"practices/testing/a minor 1.0.0 1.1.0"}) {
		t.Fatal(result, err)
	}
	if notes, err := os.ReadDir(filepath.Join(second.Directory, librarytree.RepositoryPath("changes"))); err != nil || len(notes) != 2 {
		t.Fatalf("expected both notes after the merge: %v %v", notes, err)
	}
}

// TestChange_RejectsNotesThatDontMatchTheLibrary fails before writing anything.
func TestChange_RejectsNotesThatDontMatchTheLibrary(t *testing.T) {
	a, b, c := "practices/testing/a", "practices/testing/b", "practices/testing/c"
	for _, test := range []struct {
		name    string
		files   map[string]string
		request ChangeRequest
		code    string
		message string
	}{
		{"unknown rule", nil, ChangeRequest{IDs: []string{c}, Summary: "Add c."}, "unknown-rule", c + " isn't a rule in the library."},
		{"deleted rule without --retire", map[string]string{"practices/testing/b.md": ""}, ChangeRequest{IDs: []string{b}, Bump: trailguide.ChangePatch, Summary: "Fix b."}, "unknown-rule", "To record its retirement, add --retire."},
		{"ID with .md", nil, ChangeRequest{IDs: []string{a + ".md"}, Bump: trailguide.ChangePatch, Summary: "Fix a."}, "unknown-rule", "Rule IDs omit the .md extension."},
		{"bump for a new rule", map[string]string{"practices/testing/c.md": ruleText("New.")}, ChangeRequest{IDs: []string{c}, Bump: trailguide.ChangeMinor, Summary: "Add c."}, "invalid-arguments", "--bump isn't accepted for new rules"},
		{"new and versioned rules", map[string]string{"practices/testing/c.md": ruleText("New.")}, ChangeRequest{IDs: []string{a, c}, Summary: "Add c."}, "invalid-arguments", "Record them in separate notes."},
		{"retirement of a rule that exists", nil, ChangeRequest{IDs: []string{a}, Retire: true, Summary: "Retire a."}, "invalid-change", a + " still exists."},
		{"retirement that keeps the asset directory", map[string]string{"practices/testing/a.md": ""}, ChangeRequest{IDs: []string{a}, Retire: true, Summary: "Retire a."}, "invalid-change", a + "'s asset directory, practices/testing/assets/a/, still exists. Delete it before recording the retirement."},
		{"retirement of an unpublished rule", nil, ChangeRequest{IDs: []string{c}, Retire: true, Summary: "Retire c."}, "invalid-change", c + " was never published, so it can't be retired."},
		{"bump for a retirement", map[string]string{"practices/testing/b.md": ""}, ChangeRequest{IDs: []string{b}, Retire: true, Bump: trailguide.ChangeMajor, Summary: "Retire b."}, "invalid-arguments", "--bump isn't accepted for retired rules."},
		{"replacement without --retire", nil, ChangeRequest{IDs: []string{a}, Bump: trailguide.ChangeMajor, ReplacedBy: b, Summary: "Replace a."}, "invalid-arguments", "--replaced-by requires --retire and a single rule"},
		{"replacement for several rules", map[string]string{"practices/testing/a.md": "", "practices/testing/b.md": ""}, ChangeRequest{IDs: []string{a, b}, Retire: true, ReplacedBy: c, Summary: "Replace both."}, "invalid-arguments", "--replaced-by requires --retire and a single rule"},
		{"duplicate rule", nil, ChangeRequest{IDs: []string{a, a}, Bump: trailguide.ChangePatch, Summary: "Fix a."}, "invalid-arguments", a + " is named more than once"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, options := authorClone(t, libraryFiles(), releaseOne)
			edit(t, options.Directory, test.files)
			_, err := PlanChange(context.Background(), test.request, options)
			if errorCode(err) != test.code || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("got %v, want %s: %s", err, test.code, test.message)
			}
			if _, err := os.Stat(filepath.Join(options.Directory, librarytree.RepositoryPath("changes"))); !os.IsNotExist(err) {
				t.Fatal("wrote changes/", err)
			}
		})
	}
}

// TestChange_RefusesAnUnchangedPublishedRule, which library check would reject as stale, before writing anything.
func TestChange_RefusesAnUnchangedPublishedRule(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	// A checkout's converted line endings aren't a change, as in library check.
	files[".gitattributes"] = []byte("*.md text eol=crlf\n")
	_, options := authorClone(t, files, releaseOne)
	request := ChangeRequest{IDs: []string{"practices/testing/a"}, Bump: trailguide.ChangePatch, Summary: "Fix a."}
	if _, err := PlanChange(ctx, request, options); errorCode(err) != "unchanged-rule" || !strings.Contains(err.Error(), "practices/testing/a hasn't changed since release/1. Edit the rule first, then record the change.") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(options.Directory, librarytree.RepositoryPath("changes"))); !os.IsNotExist(err) {
		t.Fatal("wrote changes/", err)
	}
	// Any versioned file counts: here, a new file in the rule's asset directory.
	edit(t, options.Directory, map[string]string{"practices/testing/assets/a/more.go": "package more\n"})
	if _, _, err := recordChange(t, options, request); err != nil {
		t.Fatal(err)
	}
	if result, err := Check(ctx, options); err != nil || !slices.Equal(previewRows(result.PendingRelease), []string{"practices/testing/a patch 1.0.0 1.0.1"}) {
		t.Fatal(result, err)
	}
}

// TestChange_WarnsWhenTheReplacementIsntARuleYet but still records the retirement, since the replacement only has
// to exist by the time a library release publishes it.
func TestChange_WarnsWhenTheReplacementIsntARuleYet(t *testing.T) {
	ctx := context.Background()
	_, options := authorClone(t, libraryFiles(), releaseOne)
	edit(t, options.Directory, map[string]string{"practices/testing/b.md": ""})
	warning := "practices/testing/c isn't a rule in the library yet. Add it before the next library release."
	for _, test := range []struct {
		replacement string
		warned      bool
	}{{"practices/testing/c", true}, {"practices/testing/a", false}} {
		plan, err := PlanChange(ctx, ChangeRequest{IDs: []string{"practices/testing/b"}, Retire: true, ReplacedBy: test.replacement}, options)
		if err != nil {
			t.Fatal(err)
		}
		result, err := plan.Commit(ctx, "", "Replace b.", noteDay)
		if err != nil || len(result.Written()) != 1 || slices.Contains(result.Warnings, warning) != test.warned || len(result.Warnings) > 1 {
			t.Fatalf("%s: %+v %v", test.replacement, result, err)
		}
	}
}

// TestChange_RequiresABumpAndSummaryToCommit lets prompts supply them, then requires them when writing.
func TestChange_RequiresABumpAndSummaryToCommit(t *testing.T) {
	ctx := context.Background()
	_, options := authorClone(t, libraryFiles(), releaseOne)
	edit(t, options.Directory, map[string]string{"practices/testing/a.md": ruleText("Changed.")})
	plan, err := PlanChange(ctx, ChangeRequest{IDs: []string{"practices/testing/a"}}, options)
	if err != nil || !plan.Versioned {
		t.Fatal(plan, err)
	}
	if _, err := plan.Commit(ctx, "", "Fix a.", noteDay); errorCode(err) != "invalid-arguments" || !strings.Contains(err.Error(), "--bump is required") {
		t.Fatal(err)
	}
	if _, err := plan.Commit(ctx, trailguide.ChangePatch, " ", noteDay); errorCode(err) != "invalid-arguments" || !strings.Contains(err.Error(), "--summary is required") {
		t.Fatal(err)
	}
	// The note parser rejects a summary of more than one line, so the command never writes one.
	var validation errs.ValidationError
	if _, err := plan.Commit(ctx, trailguide.ChangePatch, "One.\nTwo.", noteDay); !errors.As(err, &validation) || !strings.HasSuffix(validation.ValidationLocation(), ".summary") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(options.Directory, librarytree.RepositoryPath("changes"))); !os.IsNotExist(err) {
		t.Fatal("wrote a note with a multi-line summary", err)
	}
	if _, err := plan.Commit(ctx, trailguide.ChangePatch, "Fix a.", noteDay); err != nil {
		t.Fatal(err)
	}
}

// TestChange_RetiredRules can't retire a rule twice or reuse a retired rule's ID.
func TestChange_RetiredRules(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	if _, err := fixture.Commit(ctx, options.Directory, "Retire b", gitfixture.Library(map[string][]byte{"practices/testing/b.md": nil})); err != nil {
		t.Fatal(err)
	}
	two := "Library release 2.\n---\nschemaVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.0\nretired:\n  practices/testing/b:\n    lastVersion: 1.0.0\n    summaries:\n      - Retire b.\n"
	if err := fixture.Tag(ctx, options.Directory, "release/2", two); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanChange(ctx, ChangeRequest{IDs: []string{"practices/testing/b"}, Retire: true}, options); errorCode(err) != "invalid-change" || !strings.Contains(err.Error(), "practices/testing/b was already retired by release/2.") {
		t.Fatal(err)
	}
	edit(t, options.Directory, map[string]string{"practices/testing/b.md": ruleText("Back.")})
	if _, err := PlanChange(ctx, ChangeRequest{IDs: []string{"practices/testing/b"}}, options); errorCode(err) != "invalid-change" || !strings.Contains(err.Error(), "reuses the ID of a rule that release/2 retired") {
		t.Fatal(err)
	}
}

// TestChange_BeforeTheFirstLibraryRelease refuses, because rules need no notes yet.
func TestChange_BeforeTheFirstLibraryRelease(t *testing.T) {
	_, inGit := authorClone(t, libraryFiles())
	outside := Options{Directory: t.TempDir()}
	for name, content := range libraryFiles() {
		edit(t, outside.Directory, map[string]string{name: string(content)})
	}
	for name, options := range map[string]Options{"Git": inGit, "outside Git": outside} {
		t.Run(name, func(t *testing.T) {
			_, err := PlanChange(context.Background(), ChangeRequest{IDs: []string{"practices/testing/a"}, Summary: "Add a."}, options)
			if errorCode(err) != "no-library-release" || !strings.Contains(err.Error(), "rules need no change notes") {
				t.Fatal(err)
			}
		})
	}
}
