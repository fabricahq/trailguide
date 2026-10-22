// Exercise replacing a local rule and its asset directory together with managed trees: publication, removal,
// process death, edits, and refused targets.

package filetxn

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"testing"
)

// forkFile is the local rule the tests replace, relative to rules/.
const forkFile = "techs/go/fork.md"

// writeForkProject writes the fork with two assets, another local rule with its own asset, and the previous vendor
// and generated output.
func writeForkProject(t *testing.T, root *os.Root) {
	t.Helper()
	writeFixture(t, root, "rules/techs/go/fork.md", "old fork")
	writeFixture(t, root, "rules/techs/go/assets/fork/notes.md", "old notes")
	writeFixture(t, root, "rules/techs/go/assets/fork/extra/data.bin", "old data")
	writeFixture(t, root, "rules/techs/go/other.md", "other rule")
	writeFixture(t, root, "rules/techs/go/assets/other/notes.md", "other notes")
	writeFixture(t, root, "_managed/vendor/team/rule.md", "old rule")
	writeFixture(t, root, "_managed/generated/RULES.md", "old output")
}

// forkUpdate replaces the fork and its assets with assets, which nil removes, and the managed trees with new output.
func forkUpdate(assets map[string][]byte) map[Target]map[string][]byte {
	return map[Target]map[string][]byte{
		LocalRule(forkFile):       {"rules/" + forkFile: []byte("new fork")},
		LocalRuleAssets(forkFile): assets,
		Vendor:                    {"team/rule.md": []byte("new rule")},
		Generated:                 {"RULES.md": []byte("new output")},
	}
}

// localFiles returns every file under rules/ with its text.
func localFiles(t *testing.T, root *os.Root) map[string]string {
	t.Helper()
	tree, err := ReadTree(context.Background(), root, "rules")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for name, data := range tree.Files {
		files[name] = string(data)
	}
	return files
}

// requireForkProject checks that rules/ holds exactly want and the managed trees the matching output.
func requireForkProject(t *testing.T, root *os.Root, state string, want map[string]string) {
	t.Helper()
	for name, suffix := range map[string]string{"_managed/vendor/team/rule.md": " rule", "_managed/generated/RULES.md": " output"} {
		data, err := root.ReadFile(name)
		if err != nil || string(data) != state+suffix {
			t.Fatalf("%s: got %q, want %q: %v", name, data, state+suffix, err)
		}
	}
	if got := localFiles(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("local files:\n%v\nwant\n%v", got, want)
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// oldLocal is rules/ before the update, and newLocal after it replaces the fork's assets with new.md.
var (
	oldLocal = map[string]string{"techs/go/fork.md": "old fork", "techs/go/assets/fork/notes.md": "old notes", "techs/go/assets/fork/extra/data.bin": "old data", "techs/go/other.md": "other rule", "techs/go/assets/other/notes.md": "other notes"}
	newLocal = map[string]string{"techs/go/fork.md": "new fork", "techs/go/assets/fork/new.md": "new notes", "techs/go/other.md": "other rule", "techs/go/assets/other/notes.md": "other notes"}
)

// TestApply_ReplacesALocalRuleAndItsAssetDirectory installs the new fork and exactly its new assets, removing the
// old ones, and leaves the other local rule and its assets alone.
func TestApply_ReplacesALocalRuleAndItsAssetDirectory(t *testing.T) {
	root := openProject(t)
	writeForkProject(t, root)
	if err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), nil)
	}); err != nil {
		t.Fatal(err)
	}
	requireForkProject(t, root, "new", newLocal)
}

// TestApply_RemovesALocalRuleAssetDirectory removes the fork's asset directory when the new fork has no assets,
// and treats removing an absent one as nothing to do.
func TestApply_RemovesALocalRuleAssetDirectory(t *testing.T) {
	root := openProject(t)
	writeForkProject(t, root)
	for range 2 {
		if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(forkUpdate(nil), nil) }); err != nil {
			t.Fatal(err)
		}
	}
	// A transaction that only removes the absent directory has nothing to do.
	if err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(map[Target]map[string][]byte{LocalRuleAssets(forkFile): nil}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"techs/go/fork.md": "new fork", "techs/go/other.md": "other rule", "techs/go/assets/other/notes.md": "other notes"}
	requireForkProject(t, root, "new", want)
	if _, err := root.Lstat("rules/techs/go/assets/fork"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("the asset directory is still there", err)
	}
}

// TestApply_CreatesALocalRuleAssetDirectoryAndItsParent gives a fork without assets its first ones, creating
// rules/techs/go/assets, and removes that directory again when the transaction fails.
func TestApply_CreatesALocalRuleAssetDirectoryAndItsParent(t *testing.T) {
	setup := func(t *testing.T) *os.Root {
		root := openProject(t)
		writeFixture(t, root, "rules/techs/go/fork.md", "old fork")
		writeFixture(t, root, "_managed/vendor/team/rule.md", "old rule")
		writeFixture(t, root, "_managed/generated/RULES.md", "old output")
		return root
	}
	t.Run("applied", func(t *testing.T) {
		root := setup(t)
		if err := WithWriter(context.Background(), root, func(w *Writer) error {
			return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), nil)
		}); err != nil {
			t.Fatal(err)
		}
		requireForkProject(t, root, "new", map[string]string{"techs/go/fork.md": "new fork", "techs/go/assets/fork/new.md": "new notes"})
	})
	t.Run("refused", func(t *testing.T) {
		root := setup(t)
		err := WithWriter(context.Background(), root, func(w *Writer) error {
			return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), func() error { return failure("concurrent-change", "changed", nil) })
		})
		projectCode(t, err, "concurrent-change")
		requireForkProject(t, root, "old", map[string]string{"techs/go/fork.md": "old fork"})
		if _, err := root.Lstat("rules/techs/go/assets"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("the failed transaction left the directory it created", err)
		}
	})
}

// TestApply_ChangesNoLiveDirectoryBeforeTheInputsAreChecked: when Apply asks the caller to recheck its inputs, the
// live tree is still exactly as before, so a caller that compares directories doesn't mistake Apply's own parent
// directory for a concurrent edit.
func TestApply_ChangesNoLiveDirectoryBeforeTheInputsAreChecked(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "rules/techs/go/fork.md", "old fork")
	writeFixture(t, root, "_managed/vendor/team/rule.md", "old rule")
	writeFixture(t, root, "_managed/generated/RULES.md", "old output")
	before, err := ReadTree(context.Background(), root, "rules")
	if err != nil {
		t.Fatal(err)
	}
	if err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), func() error {
			if current, err := ReadTree(context.Background(), root, "rules"); err != nil || treeDigest(current) != treeDigest(before) {
				return failure("concurrent-change", "rules/ changed before the inputs were checked", err)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	requireForkProject(t, root, "new", map[string]string{"techs/go/fork.md": "new fork", "techs/go/assets/fork/new.md": "new notes"})
}

// writeAssetFreeFork writes a fork without assets, in a group without any, and the previous managed trees.
func writeAssetFreeFork(t *testing.T, root *os.Root) {
	t.Helper()
	writeFixture(t, root, "rules/techs/go/fork.md", "old fork")
	writeFixture(t, root, "_managed/vendor/team/rule.md", "old rule")
	writeFixture(t, root, "_managed/generated/RULES.md", "old output")
}

// requireEmptyDirectory fails unless name is an empty directory.
func requireEmptyDirectory(t *testing.T, root *os.Root, name string) {
	t.Helper()
	tree, err := ReadTree(context.Background(), root, name)
	if err != nil || tree == nil || len(tree.Files) != 0 {
		t.Fatalf("%s isn't an empty directory: %+v, %v", name, tree, err)
	}
}

// TestKilledWriterLeavesTheParentsItCreated stops the writer right after it creates rules/techs/go/assets, the new
// asset directory's missing parent, or after it began replacing targets. Recovery restores every target and leaves
// the directory, empty, since nothing can prove that the directory now at that path is the one the writer made.
func TestKilledWriterLeavesTheParentsItCreated(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_PARENT_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		stop := os.Getenv("TRAILGUIDE_PARENT_STOP")
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			mkdir, rename := w.mkdir, w.rename
			w.mkdir = func(name string) error {
				err := mkdir(name)
				if err == nil && stop == "after mkdir" {
					os.Exit(42)
				}
				return err
			}
			w.rename = func(from, to string) error {
				err := rename(from, to)
				if err == nil && stop == "after replacing vendor" && to == "_managed/vendor" {
					os.Exit(42)
				}
				return err
			}
			return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), nil)
		})
		t.Fatalf("child did not stop: %v", err)
	}
	for _, stop := range []string{"after mkdir", "after replacing vendor"} {
		t.Run(stop, func(t *testing.T) {
			root := openProject(t)
			writeAssetFreeFork(t, root)
			child := exec.Command(os.Args[0], "-test.run=^TestKilledWriterLeavesTheParentsItCreated$")
			child.Env = append(os.Environ(), "TRAILGUIDE_PARENT_CHILD="+root.Name(), "TRAILGUIDE_PARENT_STOP="+stop)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			requireForkProject(t, root, "old", map[string]string{"techs/go/fork.md": "old fork"})
			requireEmptyDirectory(t, root, "rules/techs/go/assets")
		})
	}
}

// TestApply_LeavesTheParentsItCreatedWhenItFails: an Apply that fails after creating a parent, here because it was
// cancelled, rolls back every target and leaves the parent, empty.
func TestApply_LeavesTheParentsItCreatedWhenItFails(t *testing.T) {
	root := openProject(t)
	writeAssetFreeFork(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := WithWriter(ctx, root, func(w *Writer) error {
		mkdir := w.mkdir
		w.mkdir = func(name string) error {
			err := mkdir(name)
			cancel()
			return err
		}
		return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), nil)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wanted cancellation, got %v", err)
	}
	requireForkProject(t, root, "old", map[string]string{"techs/go/fork.md": "old fork"})
	requireEmptyDirectory(t, root, "rules/techs/go/assets")
}

// TestApply_UsesAParentThatAppearedMeanwhile: a directory that appears at a missing parent's path after the journal
// is written is used as it is, with what it holds, and a file there is refused without touching it.
func TestApply_UsesAParentThatAppearedMeanwhile(t *testing.T) {
	for name, appear := range map[string]func(t *testing.T, root *os.Root, path string){
		"directory": func(t *testing.T, root *os.Root, path string) { writeFixture(t, root, path+"/theirs.md", "their file") },
		"file":      func(t *testing.T, root *os.Root, path string) { writeFixture(t, root, path, "their file") },
	} {
		t.Run(name, func(t *testing.T) {
			root := openProject(t)
			writeAssetFreeFork(t, root)
			err := WithWriter(context.Background(), root, func(w *Writer) error {
				mkdir := w.mkdir
				w.mkdir = func(path string) error {
					appear(t, root, path)
					return mkdir(path)
				}
				return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), nil)
			})
			// The file blocks the transaction's own targets under that path, so rollback stops and keeps the
			// transaction for recovery rather than risk the file.
			if name == "file" {
				projectCode(t, err, "recovery-required")
				if data, err := root.ReadFile("rules/techs/go/assets"); err != nil || string(data) != "their file" {
					t.Fatalf("their file: %q, %v", data, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			requireForkProject(t, root, "new", map[string]string{"techs/go/fork.md": "new fork", "techs/go/assets/fork/new.md": "new notes", "techs/go/assets/theirs.md": "their file"})
		})
	}
}

// TestApply_KeepsALocalRuleEditedDuringStaging refuses to replace a fork the user edited meanwhile, and keeps the
// previous fork, assets, and managed trees.
func TestApply_KeepsALocalRuleEditedDuringStaging(t *testing.T) {
	for name, edit := range map[string]string{"rule": "rules/techs/go/fork.md", "asset": "rules/techs/go/assets/fork/notes.md"} {
		t.Run(name, func(t *testing.T) {
			root := openProject(t)
			writeForkProject(t, root)
			err := WithWriter(context.Background(), root, func(w *Writer) error {
				return w.Apply(forkUpdate(map[string][]byte{"new.md": []byte("new notes")}), func() error {
					writeFixture(t, root, edit, "user edit")
					return nil
				})
			})
			projectCode(t, err, "concurrent-change")
			want := maps.Clone(oldLocal)
			want[edit[len("rules/"):]] = "user edit"
			requireForkProject(t, root, "old", want)
		})
	}
}

// TestApply_RejectsTargetsOutsideLocalRules refuses paths that aren't a local rule or its asset directory, and
// removing anything but an asset directory.
func TestApply_RejectsTargetsOutsideLocalRules(t *testing.T) {
	root := openProject(t)
	writeForkProject(t, root)
	for _, target := range []Target{"rules/techs/Go/fork.md", "rules/techs/go/assets/fork/notes.md", "rules/techs/go/assets", "rules/techs/go/README.md", "rules/techs/go/fork", "rules/other/go/fork.md", "rules/techs/go/assets/fork/nested/assets/x"} {
		err := WithWriter(context.Background(), root, func(w *Writer) error {
			return w.Apply(map[Target]map[string][]byte{target: {string(target): []byte("x")}}, nil)
		})
		projectCode(t, err, "invalid-target")
	}
	for _, target := range []Target{LocalRule(forkFile), Vendor, LocalGroupMetadata("techs/go")} {
		err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(map[Target]map[string][]byte{target: nil}, nil) })
		projectCode(t, err, "invalid-target")
	}
	requireForkProject(t, root, "old", oldLocal)
}

// TestKilledForkWriterRecovers stops the writer after each rename, replacing the assets or removing them; the next
// writer restores the previous fork, assets, and trees, or, once the commit marker exists, keeps everything new.
func TestKilledForkWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_FORK_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		var assets map[string][]byte
		if os.Getenv("TRAILGUIDE_FORK_ASSETS") != "" {
			assets = map[string][]byte{"new.md": []byte("new notes")}
		}
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			rename := w.rename
			w.rename = func(from, to string) error {
				err := rename(from, to)
				if err == nil && to == os.Getenv("TRAILGUIDE_FORK_STOP") {
					os.Exit(42)
				}
				return err
			}
			return w.Apply(forkUpdate(assets), nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	removed := map[string]string{"techs/go/fork.md": "new fork", "techs/go/other.md": "other rule", "techs/go/assets/other/notes.md": "other notes"}
	assetsBackup := transactionName + "/" + backupPrefix + "rules+techs+go+assets+fork"
	for _, test := range []struct {
		name, stop, assets string
		state              string
		want               map[string]string
	}{
		{"probe", renameCheck + "/rules+techs+go+fork.md", "yes", "old", oldLocal},
		{"_managed/vendor", "_managed/vendor", "yes", "old", oldLocal},
		{"_managed/generated", "_managed/generated", "yes", "old", oldLocal},
		{"rule", "rules/techs/go/fork.md", "yes", "old", oldLocal},
		{"assets backed up", assetsBackup, "yes", "old", oldLocal},
		{"assets", "rules/techs/go/assets/fork", "yes", "old", oldLocal},
		{"committed", transactionName + "/committed.json", "yes", "new", newLocal},
		{"removed assets backed up", assetsBackup, "", "old", oldLocal},
		{"committed removal", transactionName + "/committed.json", "", "new", removed},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := openProject(t)
			writeForkProject(t, root)
			child := exec.Command(os.Args[0], "-test.run=^TestKilledForkWriterRecovers$")
			child.Env = append(os.Environ(), "TRAILGUIDE_FORK_CHILD="+root.Name(), "TRAILGUIDE_FORK_STOP="+test.stop, "TRAILGUIDE_FORK_ASSETS="+test.assets)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			requireForkProject(t, root, test.state, test.want)
		})
	}
}

// TestRecovery_RefusesALocalRuleEntryInAnOlderJournal preserves a journal whose format can't hold a local rule, and
// one that removes a target other than a local rule's asset directory.
func TestRecovery_RefusesALocalRuleEntryInAnOlderJournal(t *testing.T) {
	assets := &Tree{Files: map[string][]byte{"notes.md": []byte("old notes")}}
	for name, record := range map[string]journalRecord{
		"format 5": {FormatVersion: 5, Entries: []journalEntry{{LocalRuleAssets(forkFile), treeDigest(assets), treeDigest(assets), true}}},
		"removal":  {FormatVersion: 6, Entries: []journalEntry{{Vendor, treeDigest(assets), treeDigest(nil), true}}},
	} {
		t.Run(name, func(t *testing.T) {
			root := openProject(t)
			writeForkProject(t, root)
			if err := root.Mkdir(transactionName, 0700); err != nil {
				t.Fatal(err)
			}
			if err := durableJSON(root, transactionName+"/journal.json", record); err != nil {
				t.Fatal(err)
			}
			projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
			if got := slices.Sorted(maps.Keys(localFiles(t, root))); !reflect.DeepEqual(got, slices.Sorted(maps.Keys(oldLocal))) {
				t.Fatalf("recovery changed local files: %v", got)
			}
		})
	}
}
