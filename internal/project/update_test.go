// Exercise project update against a real library: previews that write nothing, decisions written to config.yaml
// with the output they select, exactly the previewed versions, and refusal after concurrent edits.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// syncedProject syncs the sync fixture's project at release/1, then publishes release/2, in which
// techs/go/errors moves to 1.1.0 and techs/go/extra is new.
func syncedProject(t *testing.T) (Options, imports.Options, func(*testing.T)) {
	t.Helper()
	f, options, git := syncProject(t)
	if _, err := Sync(context.Background(), options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	thirdRelease := func(t *testing.T) {
		t.Helper()
		files := map[string][]byte{"techs/go/errors.md": []byte(strings.Replace(projectRule, "Return errors to the caller.", "Always return wrapped errors.", 1))}
		if _, err := f.Commit(context.Background(), f.Worktree(), "Third release", gitfixture.Library(files)); err != nil {
			t.Fatal(err)
		}
		record := "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/errors: 2.0.0\n  techs/go/extra: 1.0.0\nchanges:\n  techs/go/errors: {change: major, from: 1.1.0, summaries: [Require wrapping.]}\n"
		if err := f.Release(context.Background(), 3, record); err != nil {
			t.Fatal(err)
		}
	}
	return options, git, thirdRelease
}

// projectTree reads every file of the project's Trail Guide directory.
func projectTree(t *testing.T, options Options) *filetxn.Tree {
	t.Helper()
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	tree, err := filetxn.ReadTree(context.Background(), root, ".")
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// TestUpdate_PreviewWritesNothingAndApplyInstallsIt previews a minor change and a new rule, then applies both and
// moves the source's ref to the release it installs.
func TestUpdate_PreviewWritesNothingAndApplyInstallsIt(t *testing.T) {
	options, git, _ := syncedProject(t)
	before := projectTree(t, options)
	plan, err := PlanUpdate(context.Background(), options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := preview.Sources[0].Rules
	if preview.Applied || !preview.Moves() || len(rows) != 2 || rows[0].ID != "techs/go/errors" || rows[0].Change != imports.UpdateMinor || rows[1].ID != "techs/go/extra" || rows[1].Change != imports.UpdateNew {
		t.Fatalf("preview %+v", preview)
	}
	if after := projectTree(t, options); after.Digest() != before.Digest() {
		t.Fatal("planning and previewing changed the project")
	}
	applied, err := plan.Apply(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || !slices.Contains(applied.Changed, "config.yaml") || !slices.Contains(applied.Changed, "vendor/team/rules/techs/go/errors.md") || !slices.Contains(applied.Added, "vendor/team/rules/techs/go/extra.md") {
		t.Fatalf("applied %+v", applied.FileChanges)
	}
	if _, got := recordedVersions(t, options); !reflect.DeepEqual(got, map[string]string{"techs/go/errors": "1.1.0@2", "techs/go/extra": "1.0.0@2"}) {
		t.Fatalf("versions %v", got)
	}
	if config := readRepositoryFile(t, options, ".trailguide/config.yaml"); !strings.Contains(config, `"ref": "release/2"`) || strings.Contains(config, "release/1") {
		t.Fatalf("config.yaml after update:\n%s\nwant ref release/2", config)
	}
	requireCurrent(t, options)
}

// TestUpdate_DecisionsWritePinsAndExclusionsWithTheOutput keeps a changed rule with a pin and excludes the new
// rule, writing both to config.yaml in the same update.
func TestUpdate_DecisionsWritePinsAndExclusionsWithTheOutput(t *testing.T) {
	options, git, _ := syncedProject(t)
	plan, err := PlanUpdate(context.Background(), options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	decisions := []UpdateDecision{{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "Waiting on #45."}, {Source: "team", Rule: "techs/go/extra", Kind: DecisionExclude, Reason: "Not for this project."}}
	applied, err := plan.Apply(context.Background(), decisions)
	if err != nil {
		t.Fatal(err)
	}
	rows := applied.Sources[0].Rules
	if rows[0].Decision != "keep" || rows[0].Reason != "Waiting on #45." || rows[1].Decision != "exclude" || applied.Moves() != true || !slices.Contains(applied.Changed, "config.yaml") {
		t.Fatalf("applied %+v", applied)
	}
	tree := projectTree(t, options)
	config, err := rules.ParseConfigurationYAML(tree.Files["config.yaml"])
	if err != nil {
		t.Fatal(err)
	}
	team := config.Sources[0]
	if team.Pins["techs/go/errors"].Version.String() != "1.0.0" || team.Pins["techs/go/errors"].Reason != "Waiting on #45." || team.Exclude["techs/go/extra"].Reason != "Not for this project." {
		t.Fatalf("configuration %s", tree.Files["config.yaml"])
	}
	if !strings.Contains(string(tree.Files["config.yaml"]), `version: "1.0.0"`) {
		t.Fatalf("pin version isn't quoted:\n%s", tree.Files["config.yaml"])
	}
	// The excluded rule is still imported so the project can review its changes, but agents don't read it.
	if _, got := recordedVersions(t, options); !reflect.DeepEqual(got, map[string]string{"techs/go/errors": "1.0.0@1", "techs/go/extra": "1.0.0@2"}) {
		t.Fatalf("versions %v", got)
	}
	if _, ok := tree.Files["generated/rules/team/techs/go/extra.md"]; ok {
		t.Fatal("generated an excluded rule")
	}
	requireCurrent(t, options)
}

// TestUpdate_AppliesThePreviewedVersionsAfterANewerLibraryRelease ignores a library release published after the
// preview.
func TestUpdate_AppliesThePreviewedVersionsAfterANewerLibraryRelease(t *testing.T) {
	options, git, thirdRelease := syncedProject(t)
	plan, err := PlanUpdate(context.Background(), options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	thirdRelease(t)
	if _, err := plan.Apply(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, got := recordedVersions(t, options); got["techs/go/errors"] != "1.1.0@2" {
		t.Fatalf("versions %v", got)
	}
}

// TestUpdate_RefusesALibraryReleaseTagMovedAfterThePreview writes nothing when the release tag the preview chose
// versions from names another commit by the time the update is applied, although the original commit remains.
func TestUpdate_RefusesALibraryReleaseTagMovedAfterThePreview(t *testing.T) {
	ctx := context.Background()
	f, options, git := syncProject(t)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	moveReleaseTag(t, f, "release/2")
	requireRefusedMove(t, plan, options)
}

// TestUpdate_RefusesASharedFilesReleaseTagMovedAfterThePreview refuses an update that moves only the shared files
// when their library release tag moved after the preview.
func TestUpdate_RefusesASharedFilesReleaseTagMovedAfterThePreview(t *testing.T) {
	ctx := context.Background()
	f, options, git := syncProject(t)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Describe the group", gitfixture.Library(map[string][]byte{"techs/go/_group.yaml": []byte(strings.Replace(projectMetadata, "Go guidance.", "Guidance for Go code.", 1))})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\nchanges: {}\nlibraryFiles: [techs/go/_group.yaml]\n"); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview, err := plan.Preview(context.Background(), nil); err != nil || !preview.Moves() {
		t.Fatalf("an update of shared files alone doesn't move anything: %+v, %v", preview, err)
	}
	moveReleaseTag(t, f, "release/2")
	requireRefusedMove(t, plan, options)
}

// moveReleaseTag moves the fixture's release tag name, keeping its message, to a new commit.
func moveReleaseTag(t *testing.T, f *gitfixture.Fixture, name string) {
	t.Helper()
	ctx := context.Background()
	object, err := f.Command(ctx, "cat-file", "tag", name)
	if err != nil {
		t.Fatal(err)
	}
	_, message, _ := strings.Cut(object, "\n\n")
	if _, err := f.Commit(ctx, f.Worktree(), "After "+name, map[string][]byte{"README.md": []byte("Moved.\n")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Command(ctx, "tag", "--force", "--annotate", "--cleanup=verbatim", "--message", message+"\n", name); err != nil {
		t.Fatal(err)
	}
}

// requireRefusedMove applies plan and requires invalid-release-tag with the project unchanged.
func requireRefusedMove(t *testing.T, plan *UpdatePlan, options Options) {
	t.Helper()
	before := projectTree(t, options)
	_, err := plan.Apply(context.Background(), nil)
	var failure *imports.Error
	if !errors.As(err, &failure) || failure.Code != "invalid-release-tag" {
		t.Fatalf("wanted invalid-release-tag, got %v", err)
	}
	if after := projectTree(t, options); after.Digest() != before.Digest() {
		t.Fatal("a refused update changed the project")
	}
}

// TestUpdate_AppliesAnUnreleasedCommitRefWithoutReadingReleaseTags applies an update of a source whose ref names a
// commit with only unreleased rules after the library gains an invalid release tag, as sync of the source does,
// since neither depends on a library release.
func TestUpdate_AppliesAnUnreleasedCommitRefWithoutReadingReleaseTags(t *testing.T) {
	ctx := context.Background()
	f, options, git := syncProject(t)
	commit, err := f.Commit(ctx, f.Worktree(), "Unreleased change", gitfixture.Library(map[string][]byte{"techs/go/errors.md": []byte(strings.Replace(projectRule, "Return errors to the caller.", "Return errors with context.", 1))}))
	if err != nil {
		t.Fatal(err)
	}
	configure(t, options, f, `"rules":{"include":["techs/go/*"]},"ref":`+quoteJSON(commit))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if recorded, got := recordedVersions(t, options); recorded.Release != 0 || !reflect.DeepEqual(got, map[string]string{"techs/go/errors": "unreleased"}) {
		t.Fatalf("recorded release %d, versions %v", recorded.Release, got)
	}
	// A lightweight release tag is invalid, and reading the release history would refuse it.
	if _, err := f.Command(ctx, "tag", "release/2"); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatalf("update of a source that depends on no library release: %v", err)
	}
	requireCurrent(t, options)
}

// TestUpdate_RefusesAProjectChangedAfterThePreview writes nothing when configuration changed in between.
func TestUpdate_RefusesAProjectChangedAfterThePreview(t *testing.T) {
	for _, file := range []string{configurationFile, "rules/techs/go/errors.md", "vendor/team/rules/techs/go/errors.md", "generated/RULES.md"} {
		t.Run(file, func(t *testing.T) {
			options, git, _ := syncedProject(t)
			plan, err := PlanUpdate(context.Background(), options, git, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			root, err := openProject(context.Background(), options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			data, err := root.ReadFile(file)
			if errors.Is(err, os.ErrNotExist) {
				data, err = []byte(projectRule), nil
			}
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, root, file, string(data)+"\n")
			before := projectTree(t, options)
			_, err = plan.Apply(context.Background(), nil)
			projectCode(t, err, "concurrent-change")
			if after := projectTree(t, options); after.Digest() != before.Digest() {
				t.Fatal("a refused update changed the project")
			}
		})
	}
}

// TestUpdate_RejectsDecisionsThePreviewDoesntOffer refuses each as invalid arguments, naming its flag and rule, and
// writes nothing.
func TestUpdate_RejectsDecisionsThePreviewDoesntOffer(t *testing.T) {
	options, git, _ := syncedProject(t)
	plan, err := PlanUpdate(context.Background(), options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := projectTree(t, options)
	for name, decisions := range map[string][]UpdateDecision{
		"keep a new rule":       {{Source: "team", Rule: "techs/go/extra", Kind: DecisionKeep, Reason: "No."}},
		"exclude a moved rule":  {{Source: "team", Rule: "techs/go/errors", Kind: DecisionExclude, Reason: "No."}},
		"unknown rule":          {{Source: "team", Rule: "techs/go/missing", Kind: DecisionKeep, Reason: "No."}},
		"blank reason":          {{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: " "}},
		"two decisions":         {{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "A."}, {Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "B."}},
		"another source's rule": {{Source: "other", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "No."}},
	} {
		t.Run(name, func(t *testing.T) {
			_, previewErr := plan.Preview(context.Background(), decisions)
			_, applyErr := plan.Apply(context.Background(), decisions)
			last := decisions[len(decisions)-1]
			flag := map[UpdateDecisionKind]string{DecisionKeep: "--keep", DecisionExclude: "--exclude"}[last.Kind]
			for _, err := range []error{previewErr, applyErr} {
				var failure *filetxn.Error
				var unchanged *UnchangedError
				if !errors.As(err, &failure) || failure.Code != "invalid-arguments" || !strings.HasPrefix(failure.Problem, flag+" "+last.Source+":"+last.Rule+": ") || !errors.As(err, &unchanged) {
					t.Fatalf("got %v", err)
				}
			}
		})
	}
	if after := projectTree(t, options); after.Digest() != before.Digest() {
		t.Fatal("a rejected decision changed the project")
	}
}

// twoLibraryProject syncs a project that imports techs/go from two libraries, alpha and beta, at their release/1,
// then publishes each library's release/2, in which techs/go/errors moves to 1.1.0 and techs/go/retry is retired.
func twoLibraryProject(t *testing.T) (Options, imports.Options) {
	t.Helper()
	ctx := context.Background()
	fixtures := map[string]*gitfixture.Fixture{}
	for _, name := range []string{"alpha", "beta"} {
		f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
			"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{}}`),
			"techs/go/_group.yaml": []byte(projectMetadata),
			"techs/go/errors.md":   []byte(strings.Replace(projectRule, "# Return errors", "# Return errors, from "+name, 1)),
			"techs/go/retry.md":    []byte(projectRule),
		}))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\n  techs/go/retry: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n  techs/go/retry: {change: new, summaries: [Add the rule.]}\n"); err != nil {
			t.Fatal(err)
		}
		fixtures[name] = f
	}
	environment, err := fixtures["alpha"].Route(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: "1.2.3"}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, configurationFile, "schemaVersion: 2\nsources:\n  alpha:\n    repository: git@fixture.invalid:alpha\n    ref: release/1\n    rules: {include: [techs/go/*]}\n  beta:\n    repository: git@fixture.invalid:beta\n    ref: release/1\n    rules: {include: [techs/go/*]}\n")
	git := imports.Options{GitPath: fixtures["alpha"].GitPath, Environment: environment}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for name, f := range fixtures {
		if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(map[string][]byte{"techs/go/errors.md": []byte(strings.Replace(projectRule, "# Return errors", "# Return wrapped errors, from "+name, 1)), "techs/go/retry.md": nil})); err != nil {
			t.Fatal(err)
		}
		if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\nretired:\n  techs/go/retry: {lastVersion: 1.0.0, summaries: [No longer needed.]}\n"); err != nil {
			t.Fatal(err)
		}
	}
	return options, git
}

// sourceFiles returns the project's files that belong to source: its vendored snapshot and its generated rules
// and library page.
func sourceFiles(t *testing.T, options Options, source string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for name, data := range projectTree(t, options).Files {
		for _, prefix := range []string{"vendor/" + source + "/", "generated/rules/" + source + "/", "generated/libraries/" + source + "/"} {
			if strings.HasPrefix(name, prefix) {
				files[name] = string(data)
			}
		}
	}
	return files
}

// TestUpdate_ScopedToOneLibraryLeavesTheOtherUnchanged updates only alpha, as a whole or one rule of it, while
// beta also has a newer version and a retirement: beta's record, vendored files, and generated rules keep their
// bytes.
func TestUpdate_ScopedToOneLibraryLeavesTheOtherUnchanged(t *testing.T) {
	for _, test := range []struct {
		name   string
		target imports.UpdateTarget
		// alpha lists alpha's rules after the update.
		alpha []string
	}{
		{"SOURCE", imports.UpdateTarget{Source: "alpha"}, []string{"techs/go/errors"}},
		{"SOURCE:RULE", imports.UpdateTarget{Source: "alpha", Rule: "techs/go/errors"}, []string{"techs/go/errors", "techs/go/retry"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, git := twoLibraryProject(t)
			beta := sourceFiles(t, options, "beta")
			plan, err := PlanUpdate(context.Background(), options, git, []imports.UpdateTarget{test.target}, nil)
			if err != nil {
				t.Fatal(err)
			}
			applied, err := plan.Apply(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(applied.Sources) != 1 || applied.Sources[0].Name != "alpha" {
				t.Fatalf("updated %+v", applied.Sources)
			}
			if after := sourceFiles(t, options, "beta"); !reflect.DeepEqual(after, beta) || len(beta) == 0 {
				t.Fatalf("updating alpha changed beta's files:\nbefore %v\nafter %v", slices.Sorted(maps.Keys(beta)), slices.Sorted(maps.Keys(after)))
			}
			alpha := sourceFiles(t, options, "alpha")
			var record struct {
				Rules map[string]struct{ Version string }
			}
			if err := json.Unmarshal([]byte(alpha["vendor/alpha/_source.json"]), &record); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(slices.Sorted(maps.Keys(record.Rules)), test.alpha) || record.Rules["techs/go/errors"].Version != "1.1.0" {
				t.Fatalf("alpha's rules %+v, want %v with errors at 1.1.0", record.Rules, test.alpha)
			}
			requireCurrent(t, options)
		})
	}
}

// TestUpdate_RetiringAnExcludedRuleKeepsTheExclusionValidOffline warns that the exclusion no longer does anything,
// and records it so the next offline check still passes.
func TestUpdate_RetiringAnExcludedRuleKeepsTheExclusionValidOffline(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	secondRelease(t, f)
	configure(t, options, f, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/extra","reason":"Not used."}]}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Retire extra", gitfixture.Library(map[string][]byte{"techs/go/extra.md": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/errors: 1.1.0\nretired:\n  techs/go/extra: {lastVersion: 1.0.0, summaries: [No longer needed.]}\n"); err != nil {
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
	if len(applied.Sources[0].Rules) != 0 || len(applied.Warnings) != 1 || !strings.HasPrefix(applied.Warnings[0], "sources.team.rules.exclude names techs/go/extra") {
		t.Fatalf("rows %+v, warnings %v", applied.Sources[0].Rules, applied.Warnings)
	}
	if _, got := recordedVersions(t, options); !reflect.DeepEqual(got, map[string]string{"techs/go/errors": "1.1.0@2"}) {
		t.Fatalf("versions %v", got)
	}
	requireCurrent(t, options)
}

// TestUpdate_ScopedUpdateThatKeepsItsRuleLeavesTheSharedFiles: a scoped update would move techs/go/errors to its
// release/2 version and the shared files with it, but keeping the rule moves nothing, so the shared files stay.
func TestUpdate_ScopedUpdateThatKeepsItsRuleLeavesTheSharedFiles(t *testing.T) {
	options, git, _ := syncedProject(t)
	ctx := context.Background()
	plan, err := PlanUpdate(ctx, options, git, []imports.UpdateTarget{{Source: "team", Rule: "techs/go/errors"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview, err := plan.Preview(context.Background(), nil); err != nil || preview.Sources[0].SharedFiles == nil {
		t.Fatalf("without a decision the shared files move with the rule: %+v, %v", preview.Sources, err)
	}
	keep := []UpdateDecision{{Source: "team", Rule: "techs/go/errors", Kind: DecisionKeep, Reason: "Not yet."}}
	preview, err := plan.Preview(context.Background(), keep)
	if err != nil || preview.Sources[0].SharedFiles != nil || preview.Moves() {
		t.Fatalf("keeping the only moved rule still moves shared files: %+v, %v", preview.Sources, err)
	}
	if _, err := plan.Apply(ctx, keep); err != nil {
		t.Fatal(err)
	}
	if record, versions := recordedVersions(t, options); record.Release != 1 || versions["techs/go/errors"] != "1.0.0@1" {
		t.Fatalf("shared files from release %d, versions %v", record.Release, versions)
	}
}
