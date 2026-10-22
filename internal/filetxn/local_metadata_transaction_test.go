// Exercise creating local group metadata together with managed trees: publication, process death, and edits.

package filetxn

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"testing"
)

// localMetadata is the local group metadata file the tests create.
const localMetadata = "rules/techs/go/_group.yaml"

// metadataUpdate is what sync or update installs when local rules lose their group's imported metadata.
var metadataUpdate = map[Target]map[string][]byte{
	LocalGroupMetadata("techs/go"): {localMetadata: []byte("name: Go\n")},
	Vendor:                         {"team/rule.md": []byte("new rule")},
	Generated:                      {"RULES.md": []byte("new output")},
}

// writeMetadataProject writes a local rule without group metadata, and the previous vendor and generated output.
func writeMetadataProject(t *testing.T, root *os.Root) {
	t.Helper()
	writeFixture(t, root, "rules/techs/go/fork.md", "local rule")
	writeFixture(t, root, "_managed/vendor/team/rule.md", "old rule")
	writeFixture(t, root, "_managed/generated/RULES.md", "old output")
}

// requireMetadataProject checks that the metadata exists exactly when the managed trees hold the new output.
func requireMetadataProject(t *testing.T, root *os.Root, state string) {
	t.Helper()
	for name, suffix := range map[string]string{"_managed/vendor/team/rule.md": " rule", "_managed/generated/RULES.md": " output"} {
		data, err := root.ReadFile(name)
		if err != nil || string(data) != state+suffix {
			t.Fatalf("%s: got %q, want %q: %v", name, data, state+suffix, err)
		}
	}
	data, err := root.ReadFile(localMetadata)
	switch {
	case state == "new" && (err != nil || string(data) != "name: Go\n"):
		t.Fatalf("metadata: got %q, %v", data, err)
	case state == "old" && !errors.Is(err, fs.ErrNotExist):
		t.Fatalf("metadata exists with the old output: %q, %v", data, err)
	}
	if data, err := root.ReadFile("rules/techs/go/fork.md"); err != nil || string(data) != "local rule" {
		t.Fatalf("local rule: %q, %v", data, err)
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestApply_CreatesLocalGroupMetadataWithManagedTrees installs the metadata file beside the local rules.
func TestApply_CreatesLocalGroupMetadataWithManagedTrees(t *testing.T) {
	root := openProject(t)
	writeMetadataProject(t, root)
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(metadataUpdate, nil) }); err != nil {
		t.Fatal(err)
	}
	requireMetadataProject(t, root, "new")
}

// TestApply_KeepsLocalGroupMetadataCreatedDuringStaging refuses to replace metadata the user wrote meanwhile, and
// keeps the previous managed trees.
func TestApply_KeepsLocalGroupMetadataCreatedDuringStaging(t *testing.T) {
	root := openProject(t)
	writeMetadataProject(t, root)
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(metadataUpdate, func() error {
			writeFixture(t, root, localMetadata, "user metadata")
			return nil
		})
	})
	projectCode(t, err, "concurrent-change")
	if data, _ := root.ReadFile(localMetadata); string(data) != "user metadata" {
		t.Fatalf("lost the user's metadata: %q", data)
	}
	if data, _ := root.ReadFile("_managed/generated/RULES.md"); string(data) != "old output" {
		t.Fatalf("replaced output: %q", data)
	}
}

// TestApply_RejectsTargetsOutsideLocalGroupMetadata refuses invalid group IDs and other metadata paths.
func TestApply_RejectsTargetsOutsideLocalGroupMetadata(t *testing.T) {
	root := openProject(t)
	writeMetadataProject(t, root)
	for _, target := range []Target{"rules/techs/Go/_group.yaml", "rules/techs/go/sub/_group.yaml", "rules/../_group.yaml"} {
		err := WithWriter(context.Background(), root, func(w *Writer) error {
			return w.Apply(map[Target]map[string][]byte{target: {string(target): []byte("x")}}, nil)
		})
		projectCode(t, err, "invalid-target")
	}
	requireMetadataProject(t, root, "old")
}

// TestKilledMetadataWriterRecovers stops the writer after each rename; the next writer restores the previous trees
// and removes the metadata, or, once the commit marker exists, keeps everything new.
func TestKilledMetadataWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_METADATA_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			rename := w.rename
			w.rename = func(from, to string) error {
				err := rename(from, to)
				if err == nil && to == os.Getenv("TRAILGUIDE_METADATA_STOP") {
					os.Exit(42)
				}
				return err
			}
			return w.Apply(metadataUpdate, nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	for stop, state := range map[string]string{
		renameCheck + "/rules+techs+go+_group.yaml": "old",
		"_managed/vendor":                   "old",
		"_managed/generated":                "old",
		localMetadata:                       "old",
		transactionName + "/committed.json": "new",
	} {
		t.Run(stop, func(t *testing.T) {
			root := openProject(t)
			writeMetadataProject(t, root)
			child := exec.Command(os.Args[0], "-test.run=^TestKilledMetadataWriterRecovers$")
			child.Env = append(os.Environ(), "TRAILGUIDE_METADATA_CHILD="+root.Name(), "TRAILGUIDE_METADATA_STOP="+stop)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			requireMetadataProject(t, root, state)
		})
	}
}

// TestRecovery_RefusesLocalMetadataBehindASymlinkedParent keeps an interrupted transaction whose local metadata
// path reaches another file through a symbolic link in a parent directory, rather than discarding that file as
// the transaction's output.
func TestRecovery_RefusesLocalMetadataBehindASymlinkedParent(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "aliased/go/_group.yaml", "name: Go\n")
	if err := root.Mkdir("rules", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../aliased", root.Name()+"/rules/techs"); err != nil {
		t.Fatal(err)
	}
	target := LocalGroupMetadata("techs/go")
	after := &Tree{Files: map[string][]byte{string(target): []byte("name: Go\n")}}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 5, Entries: []journalEntry{{target, treeDigest(nil), treeDigest(after), false}}}); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if data, err := root.ReadFile("aliased/go/_group.yaml"); err != nil || string(data) != "name: Go\n" {
		t.Fatalf("recovery discarded the aliased file: %q, %v", data, err)
	}
	if _, err := root.Stat(transactionName + "/journal.json"); err != nil {
		t.Fatal("recovery lost the journal", err)
	}
}

// TestApply_RefusesLocalMetadataBehindASymlinkedParent publishes nothing through a symbolic link in a parent.
func TestApply_RefusesLocalMetadataBehindASymlinkedParent(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "aliased/go/fork.md", "local rule")
	if err := root.Mkdir("rules", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../aliased", root.Name()+"/rules/techs"); err != nil {
		t.Fatal(err)
	}
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(map[Target]map[string][]byte{LocalGroupMetadata("techs/go"): {localMetadata: []byte("name: Go\n")}}, nil)
	})
	projectCode(t, err, "unsafe-path")
	if _, err := root.Lstat("aliased/go/_group.yaml"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("published through a symbolic link", err)
	}
}

// TestRecovery_RefusesALocalMetadataEntryInAnOlderJournal preserves a journal whose format can't hold local group
// metadata.
func TestRecovery_RefusesALocalMetadataEntryInAnOlderJournal(t *testing.T) {
	root := openProject(t)
	writeMetadataProject(t, root)
	writeFixture(t, root, localMetadata, "name: Go\n")
	target := LocalGroupMetadata("techs/go")
	after := &Tree{Files: map[string][]byte{string(target): []byte("name: Go\n")}}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 4, Entries: []journalEntry{{target, treeDigest(nil), treeDigest(after), false}}}); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if data, err := root.ReadFile(localMetadata); err != nil || string(data) != "name: Go\n" {
		t.Fatal("recovery changed the metadata", err)
	}
}
