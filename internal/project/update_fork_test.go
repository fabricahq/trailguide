// Replace a fork with a fork of the library's newest version during an update, and check what it overwrites,
// records, and refuses.

package project

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// updateFork is the decision that replaces team's fork of techs/go/errors.
var updateFork = []UpdateDecision{{Source: "team", Rule: "techs/go/errors", Kind: DecisionUpdateFork}}

// thirdRelease is release/3 of the fork fixture's library: errors 2.0.0 drops its notes and data for a table and a
// new shared diagram, or, with retire, the library retires errors.
func thirdRelease(t *testing.T, f *gitfixture.Fixture, retire bool) {
	t.Helper()
	ctx := context.Background()
	files := map[string][]byte{
		"techs/go/errors.md":              []byte(forkedRule("Read [the table](assets/errors/table.md) and [the diagram](../../../assets/diagrams/new.svg).")),
		"techs/go/assets/errors/table.md": []byte("| Wrap |\n"),
		"techs/go/assets/errors/notes.md": nil,
		"techs/go/assets/errors/data.bin": nil,
		"assets/diagrams/new.svg":         []byte("<svg id=\"new\"/>"),
	}
	record := "formatVersion: 1\nrelease: 3\nrules:\n  practices/testing/verify: 1.0.0\n  techs/go/added: 1.0.0\n  techs/go/errors: 2.0.0\n  techs/go/licensed: 1.0.0\nchanges:\n  techs/go/errors: {change: major, from: 1.1.0, summaries: [Require a table.]}\n"
	if retire {
		files = map[string][]byte{"techs/go/errors.md": nil, "techs/go/assets/errors/notes.md": nil, "techs/go/assets/errors/data.bin": nil}
		record = "formatVersion: 1\nrelease: 3\nrules:\n  practices/testing/verify: 1.0.0\n  techs/go/added: 1.0.0\n  techs/go/licensed: 1.0.0\nchanges: {}\nretired:\n  techs/go/errors: {lastVersion: 1.1.0, summaries: [No longer needed.]}\n"
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Third release", gitfixture.Library(files)); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, record); err != nil {
		t.Fatal(err)
	}
}

// forkedProject forks techs/go/errors 1.0.0 into the fork fixture's project, edits the fork and adds an asset to
// it, adds another local rule with an asset, builds, and publishes release/3.
func forkedProject(t *testing.T, repository string) forkFixture {
	t.Helper()
	f := newForkFixture(t, repository)
	if _, err := f.fork(t, "techs/go/errors", "team@1.0.0", "Ours."); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "rules/techs/go/errors.md", forkedRule("Our edited wording."))
	writeFixture(t, root, "rules/techs/go/assets/errors/mine.md", "Our notes.\n")
	writeFixture(t, root, "rules/techs/go/own.md", forkedRule("See [our notes](assets/own/notes.md)."))
	writeFixture(t, root, "rules/techs/go/assets/own/notes.md", "Own notes.\n")
	if _, err := Build(context.Background(), f.options); err != nil {
		t.Fatal(err)
	}
	thirdRelease(t, f.fixture, false)
	return f
}

// planForkUpdate plans an update of f's project and previews it with decisions.
func planForkUpdate(t *testing.T, f forkFixture, decisions []UpdateDecision) (*UpdatePlan, UpdateResult) {
	t.Helper()
	plan, err := PlanUpdate(context.Background(), f.options, f.git, nil, decisions)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(context.Background(), decisions)
	if err != nil {
		t.Fatal(err)
	}
	return plan, preview
}

// localFork returns the files of the local rule at file, relative to rules/, and of its asset directory.
func localFork(files map[string][]byte, file string) map[string]string {
	fork := map[string]string{}
	assets := "rules/" + strings.TrimSuffix(file, ".md")
	assets = filepath.ToSlash(filepath.Join(filepath.Dir(assets), "assets", filepath.Base(assets))) + "/"
	for name, data := range files {
		if name == "rules/"+file || strings.HasPrefix(name, assets) {
			fork[name] = string(data)
		}
	}
	return fork
}

// TestUpdate_ReplacesAForkWithItsNewestVersion replaces an edited fork of 1.0.0 with exactly what forking 2.0.0
// into a new project writes, overwriting the edits and replacing its asset directory, sets basedOn to 2.0.0 in the
// same transaction, and leaves the other local rule alone. A teammate's sync afterward changes nothing.
func TestUpdate_ReplacesAForkWithItsNewestVersion(t *testing.T) {
	f := forkedProject(t, "git@github.com:acme/rules.git")
	ctx := context.Background()
	before := f.files(t)
	plan, preview := planForkUpdate(t, f, updateFork)
	row := previewRow(preview.Sources, "team", "techs/go/errors")
	// The new fork writes its rule again and has none of the old assets, which it removes.
	removes := []string{"rules/techs/go/assets/errors/data.bin", "rules/techs/go/assets/errors/diagrams/flow.svg", "rules/techs/go/assets/errors/guide.md", "rules/techs/go/assets/errors/mine.md", "rules/techs/go/assets/errors/more.md", "rules/techs/go/assets/errors/notes.md"}
	if row == nil || row.Decision != "update-fork" || row.To.String() != "2.0.0" || !reflect.DeepEqual(row.Overwrites, []string{"rules/techs/go/errors.md"}) || !reflect.DeepEqual(row.Removes, removes) {
		t.Fatalf("row %+v, want update-fork to 2.0.0 overwriting the rule and removing %v", row, removes)
	}
	if !reflect.DeepEqual(f.files(t), before) {
		t.Fatal("the preview wrote files")
	}
	applied, err := plan.Apply(ctx, updateFork)
	if err != nil {
		t.Fatal(err)
	}
	after := f.files(t)
	// A new project that forks 2.0.0 gets exactly the same rule and assets.
	fresh := newProjectSyncedTo(t, f)
	if _, err := fresh.fork(t, "techs/go/errors", "team@2.0.0", "Ours."); err != nil {
		t.Fatal(err)
	}
	want := localFork(fresh.files(t), "techs/go/errors.md")
	if got := localFork(after, "techs/go/errors.md"); !reflect.DeepEqual(got, want) {
		t.Fatalf("updated fork:\n%v\nwant the new fork:\n%v", got, want)
	}
	if !strings.Contains(want["rules/techs/go/errors.md"], "[the diagram](assets/errors/diagrams/new.svg)") || !strings.Contains(want["rules/techs/go/errors.md"], "Forked from version 2.0.0 of techs/go/errors") {
		t.Fatalf("the new fork doesn't rewrite its links or cite 2.0.0:\n%s", want["rules/techs/go/errors.md"])
	}
	for _, name := range []string{"rules/techs/go/own.md", "rules/techs/go/assets/own/notes.md", "rules/techs/go/_group.yaml"} {
		if string(after[name]) != string(before[name]) {
			t.Fatalf("%s changed", name)
		}
	}
	config := string(after["config.yaml"])
	if !strings.Contains(config, "          replacedBy: techs/go/errors\n          basedOn: \"2.0.0\"\n") {
		t.Fatalf("configuration:\n%s", config)
	}
	if !slices.Contains(applied.Changed, "rules/techs/go/errors.md") || !slices.Contains(applied.Changed, "config.yaml") || !slices.Contains(applied.Added, "rules/techs/go/assets/errors/table.md") || !slices.Contains(applied.Removed, "rules/techs/go/assets/errors/mine.md") {
		t.Fatalf("reported %+v", applied.FileChanges)
	}
	if !strings.Contains(string(after["_managed/generated/rules/local/techs/go/errors.md"]), "Read [the table]") {
		t.Fatal("the generated output doesn't have the new fork")
	}
	// A teammate who pulls the update and syncs gets exactly the same files.
	synced, err := Sync(ctx, f.options, f.git)
	if err != nil {
		t.Fatal(err)
	}
	if len(synced.Added)+len(synced.Changed)+len(synced.Removed) != 0 || !reflect.DeepEqual(f.files(t), after) {
		t.Fatalf("sync after the update changed %+v", synced)
	}
	again, preview := planForkUpdate(t, f, nil)
	if row := previewRow(preview.Sources, "team", "techs/go/errors"); row != nil || again == nil {
		t.Fatalf("the next update still lists the fork: %+v", row)
	}
}

// newProjectSyncedTo returns a new project with f's configuration, synced at f's library as it is now.
func newProjectSyncedTo(t *testing.T, f forkFixture) forkFixture {
	t.Helper()
	ctx := context.Background()
	root := openTestProject(t)
	options := Options{Directory: filepath.Dir(root.Name()), ToolVersion: f.options.ToolVersion}
	if _, err := Initialize(ctx, options); err != nil {
		t.Fatal(err)
	}
	config := string(f.files(t)["config.yaml"])
	config = config[:strings.Index(config, "      exclude:")]
	writeFixture(t, root, configurationFile, config)
	if _, err := Sync(ctx, options, f.git); err != nil {
		t.Fatal(err)
	}
	return forkFixture{fixture: f.fixture, options: options, git: f.git}
}

// TestUpdate_ReplacesAHandWrittenReplacementAndRecordsBasedOn overwrites a replacement at its own path, without
// basedOn, with a fork whose links follow that path, and records the fork's version as basedOn.
func TestUpdate_ReplacesAHandWrittenReplacementAndRecordsBasedOn(t *testing.T) {
	f := newForkFixture(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "rules/techs/go/our-errors.md", forkedRule("Our own rule."))
	writeFixture(t, root, "rules/techs/go/assets/our-errors/old.md", "Old.\n")
	writeFixture(t, root, configurationFile, string(f.files(t)["config.yaml"])+"      exclude:\n        - id: techs/go/errors\n          reason: Ours.\n          replacedBy: techs/go/our-errors\n")
	if _, err := Build(context.Background(), f.options); err != nil {
		t.Fatal(err)
	}
	thirdRelease(t, f.fixture, false)
	plan, preview := planForkUpdate(t, f, updateFork)
	if row := previewRow(preview.Sources, "team", "techs/go/errors"); row == nil || !reflect.DeepEqual(row.Overwrites, []string{"rules/techs/go/our-errors.md"}) || !reflect.DeepEqual(row.Removes, []string{"rules/techs/go/assets/our-errors/old.md"}) {
		t.Fatalf("row %+v", row)
	}
	if _, err := plan.Apply(context.Background(), updateFork); err != nil {
		t.Fatal(err)
	}
	after := f.files(t)
	if got := string(after["rules/techs/go/our-errors.md"]); got != forkedRule("Read [the table](assets/our-errors/table.md) and [the diagram](assets/our-errors/diagrams/new.svg).") {
		t.Fatalf("fork %q", got)
	}
	want := map[string]string{"rules/techs/go/our-errors.md": forkedRule("Read [the table](assets/our-errors/table.md) and [the diagram](assets/our-errors/diagrams/new.svg)."), "rules/techs/go/assets/our-errors/table.md": "| Wrap |\n", "rules/techs/go/assets/our-errors/diagrams/new.svg": "<svg id=\"new\"/>"}
	if got := localFork(after, "techs/go/our-errors.md"); !reflect.DeepEqual(got, want) {
		t.Fatalf("fork files %v", got)
	}
	if _, ok := after["rules/techs/go/errors.md"]; ok {
		t.Fatal("the update wrote a fork at the library rule's path")
	}
	if config := string(after["config.yaml"]); !strings.Contains(config, "          replacedBy: techs/go/our-errors\n          basedOn: \"2.0.0\"\n") {
		t.Fatalf("configuration:\n%s", config)
	}
}

// TestUpdate_GivesAReplacementItsGroupsFirstAssetDirectory replaces a replacement without assets, in a group whose
// local rules have none, with a fork that has some, creating rules/techs/go/assets.
func TestUpdate_GivesAReplacementItsGroupsFirstAssetDirectory(t *testing.T) {
	f := newForkFixture(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "rules/techs/go/our-errors.md", forkedRule("Our own rule."))
	writeFixture(t, root, configurationFile, string(f.files(t)["config.yaml"])+"      exclude:\n        - id: techs/go/errors\n          reason: Ours.\n          replacedBy: techs/go/our-errors\n")
	if _, err := Build(context.Background(), f.options); err != nil {
		t.Fatal(err)
	}
	thirdRelease(t, f.fixture, false)
	plan, _ := planForkUpdate(t, f, updateFork)
	if _, err := plan.Apply(context.Background(), updateFork); err != nil {
		t.Fatal(err)
	}
	if got := string(f.files(t)["rules/techs/go/assets/our-errors/table.md"]); got != "| Wrap |\n" {
		t.Fatalf("asset %q", got)
	}
}

// TestUpdate_ReplacesAForkWhosePinKeepsTheImportedCopy forks the newest version, though a pin keeps the imported
// copy, and the pin stays: it governs only that copy.
func TestUpdate_ReplacesAForkWhosePinKeepsTheImportedCopy(t *testing.T) {
	f := forkedProject(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	config := string(f.files(t)["config.yaml"])
	writeFixture(t, root, configurationFile, strings.Replace(config, "      exclude:\n", "      pins:\n        - id: techs/go/errors\n          version: \"1.0.0\"\n          reason: Not yet.\n      exclude:\n", 1))
	plan, preview := planForkUpdate(t, f, updateFork)
	if row := previewRow(preview.Sources, "team", "techs/go/errors"); row == nil || row.Pin == nil || row.To != nil || row.Newest.String() != "2.0.0" || row.Decision != "update-fork" {
		t.Fatalf("row %+v", row)
	}
	if _, err := plan.Apply(context.Background(), updateFork); err != nil {
		t.Fatal(err)
	}
	after := f.files(t)
	if got := string(after["rules/techs/go/errors.md"]); got != forkedRule("Read [the table](assets/errors/table.md) and [the diagram](assets/errors/diagrams/new.svg).") {
		t.Fatalf("fork %q", got)
	}
	if config := string(after["config.yaml"]); !strings.Contains(config, "      pins:\n        - id: techs/go/errors\n          version: \"1.0.0\"\n") || !strings.Contains(config, "          basedOn: \"2.0.0\"\n") {
		t.Fatalf("configuration:\n%s", config)
	}
	if version := lockEntry(t, after[lockFile], "team").Rules["techs/go/errors"].Version; version == nil || version.String() != "1.0.0" {
		t.Fatalf("the pin didn't keep the imported copy:\n%s", after[lockFile])
	}
}

// TestUpdate_RefusesToUpdateAForkItCant refuses, as invalid arguments and without writing, a rule no local rule
// replaces, an unknown source, and a reason, before reading any library, and then a rule the preview doesn't list
// as replaced, because nothing is newer than basedOn or the update leaves it out, and a rule the library retired.
func TestUpdate_RefusesToUpdateAForkItCant(t *testing.T) {
	invalid := func(t *testing.T, err error, text string) {
		t.Helper()
		var failure *filetxn.Error
		if !errors.As(err, &failure) || failure.Code != "invalid-arguments" || !strings.Contains(err.Error(), text) {
			t.Fatalf("wanted invalid-arguments with %q, got %v", text, err)
		}
	}
	t.Run("before reading libraries", func(t *testing.T) {
		f := forkedProject(t, "")
		root, err := openProject(context.Background(), f.options, false)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		writeFixture(t, root, configurationFile, strings.Replace(string(f.files(t)["config.yaml"]), "      exclude:\n", "      exclude:\n        - id: techs/go/licensed\n          reason: Not used.\n", 1))
		before := f.files(t)
		// No Git can run, so each refusal comes before reading a library.
		noGit := imports.Options{GitPath: filepath.Join(t.TempDir(), "missing-git")}
		for _, test := range []struct {
			decision UpdateDecision
			text     string
		}{
			{UpdateDecision{Source: "team", Rule: "techs/go/added", Kind: DecisionUpdateFork}, "--update-fork team:techs/go/added: sources.team doesn't exclude this rule with replacedBy"},
			{UpdateDecision{Source: "team", Rule: "techs/go/licensed", Kind: DecisionUpdateFork}, "--update-fork team:techs/go/licensed: sources.team doesn't exclude this rule with replacedBy"},
			{UpdateDecision{Source: "other", Rule: "techs/go/errors", Kind: DecisionUpdateFork}, "--update-fork other:techs/go/errors: no source named other"},
			{UpdateDecision{Source: "team", Rule: "techs/go/errors", Kind: DecisionUpdateFork, Reason: "Why."}, "records no reason"},
			{UpdateDecision{Source: "team", Rule: "techs/go/errors", Kind: DecisionMergeFork}, "--merge-fork team:techs/go/errors: a local rule replaces this rule, and --merge-fork merges only into a forked skill. To replace your rule with a fork of the newest version, discarding your edits, pass --update-fork team:techs/go/errors"},
		} {
			_, err := PlanUpdate(context.Background(), f.options, noGit, nil, []UpdateDecision{test.decision})
			invalid(t, err, test.text)
		}
		if !reflect.DeepEqual(f.files(t), before) {
			t.Fatal("a refused update changed the project")
		}
	})
	t.Run("not listed", func(t *testing.T) {
		f := newForkFixture(t, "")
		if _, err := f.fork(t, "techs/go/errors", "team@1.1.0", "Ours."); err != nil {
			t.Fatal(err)
		}
		before := f.files(t)
		for _, targets := range [][]imports.UpdateTarget{nil, {{Source: "team", Rule: "techs/go/licensed"}}} {
			plan, err := PlanUpdate(context.Background(), f.options, f.git, targets, updateFork)
			if err != nil {
				t.Fatal(err)
			}
			_, err = plan.Preview(context.Background(), updateFork)
			invalid(t, err, "--update-fork team:techs/go/errors: the update doesn't list this rule as replaced")
			_, err = plan.Apply(context.Background(), updateFork)
			invalid(t, err, "doesn't list this rule as replaced")
		}
		if !reflect.DeepEqual(f.files(t), before) {
			t.Fatal("a refused update changed the project")
		}
	})
	t.Run("retired", func(t *testing.T) {
		f := newForkFixture(t, "")
		if _, err := f.fork(t, "techs/go/errors", "team@1.0.0", "Ours."); err != nil {
			t.Fatal(err)
		}
		thirdRelease(t, f.fixture, true)
		before := f.files(t)
		plan, err := PlanUpdate(context.Background(), f.options, f.git, nil, updateFork)
		if err != nil {
			t.Fatal(err)
		}
		_, err = plan.Preview(context.Background(), updateFork)
		invalid(t, err, "--update-fork team:techs/go/errors: the library retired this rule, so it has no newest version to fork")
		if !reflect.DeepEqual(f.files(t), before) {
			t.Fatal("a refused update changed the project")
		}
	})
}

// TestUpdate_RefusesAForkWhoseReleaseTagMovedAfterThePreview: release/3, which published the fork's version, moves
// to changed content between the preview and the apply. Whether the update moves the imported copy from that
// release, as a whole-source or scoped update does, or a pin keeps the imported copy at an older release, the update
// refuses with invalid-release-tag and writes nothing, rather than forking the moved content.
func TestUpdate_RefusesAForkWhoseReleaseTagMovedAfterThePreview(t *testing.T) {
	for _, test := range []struct {
		name    string
		pinned  bool
		targets []imports.UpdateTarget
	}{
		{"whole source", false, nil},
		{"scoped", false, []imports.UpdateTarget{{Source: "team", Rule: "techs/go/errors"}}},
		{"pinned", true, nil},
		{"scoped and pinned", true, []imports.UpdateTarget{{Source: "team", Rule: "techs/go/errors"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := forkedProject(t, "")
			ctx := context.Background()
			if test.pinned {
				root, err := openProject(ctx, f.options, false)
				if err != nil {
					t.Fatal(err)
				}
				config := string(f.files(t)["config.yaml"])
				writeFixture(t, root, configurationFile, strings.Replace(config, "      exclude:\n", "      pins:\n        - id: techs/go/errors\n          version: \"1.0.0\"\n          reason: Not yet.\n      exclude:\n", 1))
				root.Close()
			}
			plan, err := PlanUpdate(ctx, f.options, f.git, test.targets, updateFork)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := plan.Preview(context.Background(), updateFork); err != nil {
				t.Fatal(err)
			}
			moveReleaseTagTo(t, f.fixture, "release/3", map[string][]byte{"techs/go/errors.md": []byte(forkedRule("Moved content."))})
			before := f.files(t)
			_, err = plan.Apply(ctx, updateFork)
			var failure *imports.Error
			if !errors.As(err, &failure) || failure.Code != "invalid-release-tag" {
				t.Fatalf("wanted invalid-release-tag, got %v", err)
			}
			if after := f.files(t); !reflect.DeepEqual(after, before) {
				t.Fatalf("a refused update changed the project; the fork is now:\n%s", after["rules/techs/go/errors.md"])
			}
		})
	}
}

// moveReleaseTagTo commits files on top of the library and moves the tag name to that commit, keeping its message.
func moveReleaseTagTo(t *testing.T, f *gitfixture.Fixture, name string, files map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	object, err := f.Command(ctx, "cat-file", "tag", name)
	if err != nil {
		t.Fatal(err)
	}
	_, message, _ := strings.Cut(object, "\n\n")
	if _, err := f.Commit(ctx, f.Worktree(), "Move "+name, gitfixture.Library(files)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Command(ctx, "tag", "--force", "--annotate", "--cleanup=verbatim", "--message", message+"\n", name); err != nil {
		t.Fatal(err)
	}
}

// TestUpdate_ForkUpdateWritesNothingWhenTheUpdateFails leaves every file as it was when a library release tag moved
// after the preview, or the fork changed after it.
func TestUpdate_ForkUpdateWritesNothingWhenTheUpdateFails(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, f forkFixture){
		"moved tag": func(t *testing.T, f forkFixture) { moveReleaseTag(t, f.fixture, "release/3") },
		"edited fork": func(t *testing.T, f forkFixture) {
			root, err := openProject(context.Background(), f.options, false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			writeFixture(t, root, "rules/techs/go/assets/errors/mine.md", "Edited after the preview.\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := forkedProject(t, "")
			plan, _ := planForkUpdate(t, f, updateFork)
			change(t, f)
			before := f.files(t)
			if _, err := plan.Apply(context.Background(), updateFork); err == nil {
				t.Fatal("the update applied")
			}
			after := f.files(t)
			if !reflect.DeepEqual(after, before) {
				for _, file := range slices.Sorted(maps.Keys(after)) {
					if string(after[file]) != string(before[file]) {
						t.Errorf("%s changed", file)
					}
				}
				t.Fatal("a failed update changed the project")
			}
		})
	}
}
