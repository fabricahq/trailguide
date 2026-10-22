// Exercise guide publication, rollback, and interrupted recovery alongside generated trees.

package filetxn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestGuideTransactionRollback restores both outputs when failure occurs after guide installation.
func TestGuideTransactionRollback(t *testing.T) {
	for _, guide := range []Target{GuideReadme, GuideStandalone} {
		t.Run(string(guide), func(t *testing.T) {
			root := openProject(t)
			writeFixture(t, root, string(guide), "old guide")
			writeFixture(t, root, "_managed/generated/old", "old output")
			err := WithWriter(context.Background(), root, func(w *Writer) error {
				rename := w.rename
				w.rename = func(from, to string) error {
					if strings.HasSuffix(to, "committed.json") {
						return errors.New("injected commit failure")
					}
					return rename(from, to)
				}
				return w.Apply(map[Target]map[string][]byte{Generated: {"new": []byte("new output")}, guide: {string(guide): []byte("new guide")}}, nil)
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			for name, want := range map[string]string{string(guide): "old guide", "_managed/generated/old": "old output"} {
				data, err := root.ReadFile(name)
				if err != nil || string(data) != want {
					t.Fatal(name, string(data), err)
				}
			}
			if err := RequireIdle(root); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestGuideCreationPreservesLateFile refuses to replace a user's file created just before publication.
func TestGuideCreationPreservesLateFile(t *testing.T) {
	root := openProject(t)
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		rename := w.rename
		w.rename = func(from, to string) error {
			if to == string(GuideReadme) {
				writeFixture(t, root, to, "late user notes")
			}
			return rename(from, to)
		}
		return w.Apply(map[Target]map[string][]byte{GuideReadme: {string(GuideReadme): []byte("generated guide")}}, nil)
	})
	if err == nil {
		t.Fatal("overwrote a late file")
	}
	data, err := root.ReadFile(string(GuideReadme))
	if err != nil || string(data) != "late user notes" {
		t.Fatal(string(data), err)
	}
}

// TestGuideTransactionRecovery recovers an interrupted mixed transaction and preserves later guide edits.
func TestGuideTransactionRecovery(t *testing.T) {
	for _, state := range []string{"rollback", "edited", "committed", "missing-journal"} {
		t.Run(state, func(t *testing.T) {
			root := openProject(t)
			before := &Tree{Files: map[string][]byte{"README.md": []byte("old guide")}}
			after := &Tree{Files: map[string][]byte{"README.md": []byte("new guide")}}
			writeFixture(t, root, transactionName+"/old-README.md", "old guide")
			writeFixture(t, root, "README.md", "new guide")
			if state != "missing-journal" {
				if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 3, Entries: []journalEntry{{GuideReadme, treeDigest(before), treeDigest(after), true}}}); err != nil {
					t.Fatal(err)
				}
			}
			if state == "committed" {
				if err := durableJSON(root, transactionName+"/committed.json", true); err != nil {
					t.Fatal(err)
				}
			}
			if state == "edited" {
				writeFixture(t, root, "README.md", "user notes")
			}
			err := WithWriter(context.Background(), root, func(*Writer) error { return nil })
			want := "old guide"
			switch state {
			case "committed":
				want = "new guide"
			case "edited":
				want = "user notes"
			case "missing-journal":
				want = "new guide"
			}
			if (state == "edited" || state == "missing-journal") != (err != nil) {
				t.Fatal(state, err)
			}
			data, readErr := root.ReadFile("README.md")
			if readErr != nil || string(data) != want {
				t.Fatal(string(data), readErr)
			}
			if err != nil {
				data, readErr = root.ReadFile(transactionName + "/old-README.md")
				if readErr != nil || string(data) != "old guide" {
					t.Fatal("lost backup", readErr)
				}
			}
		})
	}
}

// TestKilledGuideWriterRecovers exercises process death before and after the guide's final rename.
func TestKilledGuideWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_GUIDE_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			rename := w.rename
			w.rename = func(from, to string) error {
				err := rename(from, to)
				if err == nil && to == os.Getenv("TRAILGUIDE_GUIDE_STOP") {
					os.Exit(42)
				}
				return err
			}
			return w.Apply(map[Target]map[string][]byte{Generated: {"new": []byte("new output")}, GuideReadme: {"README.md": []byte("new guide")}}, nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	for _, stop := range []string{transactionName + "/rename-check/README.md", transactionName + "/old-README.md", "README.md"} {
		t.Run(stop, func(t *testing.T) {
			root := openProject(t)
			writeFixture(t, root, "_managed/generated/old", "old output")
			writeFixture(t, root, "README.md", "old guide")
			child := exec.Command(os.Args[0], "-test.run=^TestKilledGuideWriterRecovers$")
			child.Env = append(os.Environ(), "TRAILGUIDE_GUIDE_CHILD="+root.Name(), "TRAILGUIDE_GUIDE_STOP="+stop)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string]string{"README.md": "old guide", "_managed/generated/old": "old output"} {
				data, err := root.ReadFile(name)
				if err != nil || string(data) != want {
					t.Fatal(name, string(data), err)
				}
			}
			if err := RequireIdle(root); err != nil {
				t.Fatal(err)
			}
		})
	}
}
