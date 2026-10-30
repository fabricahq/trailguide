// Check guide and generated-output freshness against the same filesystem snapshot.

package project

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/filetxn"
)

// TestCheckWithFiles reports guide drift together with generated drift without writing repairs.
func TestCheckWithFiles(t *testing.T) {
	root, options := localProject(t)
	ctx := context.Background()
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	expected := map[string][]byte{"README.md": []byte("Current guide\n")}
	for _, state := range []string{"missing", "stale", "current"} {
		t.Run(state, func(t *testing.T) {
			if state == "missing" {
				if err := root.Remove("README.md"); err != nil {
					t.Fatal(err)
				}
			}
			if state == "stale" {
				writeFixture(t, root, "README.md", "Older guide\n")
			}
			if state == "current" {
				writeFixture(t, root, "README.md", string(expected["README.md"]))
			}
			before, err := filetxn.ReadTree(ctx, root, ".")
			if err != nil {
				t.Fatal(err)
			}
			checked, err := checkWithFiles(ctx, options, expected)
			generated, files := checked.generated, checked.files
			if err != nil || len(generated.Added)+len(generated.Changed)+len(generated.Removed) != 0 {
				t.Fatal(generated, files, err)
			}
			want := FileChanges{Added: []string{}, Changed: []string{}, Removed: []string{}, Warnings: []string{}, ExcludedSkillLinks: []build.ExcludedSkillLink{}}
			if state == "missing" {
				want.Added = []string{"README.md"}
			}
			if state == "stale" {
				want.Changed = []string{"README.md"}
			}
			if !reflect.DeepEqual(files, want) {
				t.Fatal(files, want)
			}
			after, err := filetxn.ReadTree(ctx, root, ".")
			if err != nil || before.Digest() != after.Digest() {
				t.Fatal("check changed files", err)
			}
		})
	}
}

// TestCheckSnapshotRejectsEdits covers both sides of the combined freshness decision, including missing guides.
func TestCheckSnapshotRejectsEdits(t *testing.T) {
	for _, change := range []string{"guide-created", "guide-edited", "guide-removed", "local-edited", "config-edited", "writer-started"} {
		t.Run(change, func(t *testing.T) {
			root, _ := localProject(t)
			ctx := context.Background()
			expected := map[string][]byte{"README.md": []byte("Current guide\n")}
			if change == "guide-created" {
				if err := root.Remove("README.md"); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFixture(t, root, "README.md", string(expected["README.md"]))
			}
			before, err := readProject(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			files, err := readCheckFiles(ctx, root, expected)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "guide-created", "guide-edited":
				writeFixture(t, root, "README.md", "Changed guide")
			case "guide-removed":
				if err := root.Remove("README.md"); err != nil {
					t.Fatal(err)
				}
			case "local-edited":
				writeFixture(t, root, "rules/techs/go/errors.md", projectRule+"New guidance\n")
			case "config-edited":
				writeFixture(t, root, "config.yaml", "{\n\"schemaVersion\":2,\"sources\":{}}\n")
			case "writer-started":
				if err := root.Mkdir(".trailguide-lock", 0700); err != nil {
					t.Fatal(err)
				}
			}
			err = requireCheckUnchanged(ctx, root, before, expected, files)
			if err == nil {
				t.Fatal("accepted changing project", change)
			}
		})
	}
}

// TestCheckWithFilesRejectsUnsafeGuide refuses following a link outside the project.
func TestCheckWithFilesRejectsUnsafeGuide(t *testing.T) {
	root, options := localProject(t)
	if err := root.Remove("README.md"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "guide")
	if err := os.WriteFile(outside, []byte("Outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root.Name(), "README.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := checkWithFiles(context.Background(), options, map[string][]byte{"README.md": []byte("Outside")}); err == nil {
		t.Fatal("followed guide symlink")
	}
}
