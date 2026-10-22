// Verify complete inventory bounds include unused assets and empty directories.

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/librarytree"
)

// TestInventoryLimits checks exact bounds, aggregate bytes, entry counts, and cancellation.
func TestInventoryLimits(t *testing.T) {
	chunk := make([]byte, maxFileBytes)
	files := map[string][]byte{}
	for i := range 8 {
		files[string(rune('a'+i))] = chunk
	}
	if err := validateInventoryLimits(context.Background(), files, nil); err != nil {
		t.Fatal(err)
	}
	files["extra"] = []byte{1}
	if err := validateInventoryLimits(context.Background(), files, nil); err == nil {
		t.Fatal("aggregate limit ignored")
	}
	if err := validateInventoryLimits(context.Background(), map[string][]byte{"large": make([]byte, maxFileBytes+1)}, nil); err == nil {
		t.Fatal("file limit ignored")
	}
	if err := validateInventoryLimits(context.Background(), nil, make([]string, maxFiles)); err != nil {
		t.Fatal(err)
	}
	if err := validateInventoryLimits(context.Background(), map[string][]byte{"file": nil}, make([]string, maxFiles)); err == nil {
		t.Fatal("entry limit ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateInventoryLimits(ctx, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// countedInventorySource records actual file requests without changing bounded filesystem behavior.
type countedInventorySource struct {
	rootFiles
	reads []string
}

// ReadFile records each request before delegating to the bounded local reader.
func (s *countedInventorySource) ReadFile(name string) ([]byte, error) {
	s.reads = append(s.reads, name)
	return s.rootFiles.ReadFile(name)
}

// TestInventoryStopsAtOversizedAsset proves capture rejects early rather than loading later owned trees.
func TestInventoryStopsAtOversizedAsset(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"assets", "techs"} {
		if err := os.MkdirAll(filepath.Join(directory, librarytree.RepositoryPath(name)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, librarytree.RepositoryPath(librarytree.Manifest)), []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, librarytree.RepositoryPath("assets/large.bin")))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(256 * 1024 * 1024); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, librarytree.RepositoryPath("techs/later")), []byte("must not be read"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	source := &countedInventorySource{rootFiles: rootFiles{ctx: context.Background(), root: root}}
	inventory, err := readInventory(context.Background(), source)
	if err == nil || !strings.Contains(err.Error(), "limits") || inventory.Files != nil {
		t.Fatal(inventory, err)
	}
	if len(source.reads) != 2 || source.reads[1] != librarytree.RepositoryPath("assets/large.bin") {
		t.Fatal("capture continued past limit", source.reads)
	}
}

// TestInventoryPreservesPortablePaths rejects unsafe files and empty directories before reading their content.
func TestInventoryPreservesPortablePaths(t *testing.T) {
	for _, name := range []string{"bad\\name", "bad:name", "bad\nname", ".git"} {
		for _, isDir := range []bool{true, false} {
			t.Run(name, func(t *testing.T) {
				directory := t.TempDir()
				if err := os.Mkdir(filepath.Join(directory, librarytree.Directory), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, librarytree.RepositoryPath(librarytree.Manifest)), []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(directory, librarytree.RepositoryPath("assets")), 0700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(directory, librarytree.RepositoryPath("assets"), name)
				var err error
				if isDir {
					err = os.Mkdir(target, 0700)
				} else {
					err = os.WriteFile(target, []byte("unused"), 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				root, err := os.OpenRoot(directory)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				if _, err := readLocalInventory(context.Background(), root); err == nil {
					t.Fatal("unsafe path accepted")
				}
			})
		}
	}
}

// TestInventoryRejectsInvalidUTF8 checks spelling before filesystem access, including on hosts that forbid creating such names.
func TestInventoryRejectsInvalidUTF8(t *testing.T) {
	if _, err := (portableInventorySource{}).Lstat(string([]byte{'b', 255})); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
