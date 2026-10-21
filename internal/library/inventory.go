// Apply catalog resource limits to complete captured inventories used by library authoring checks.

package library

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/trailguide"
)

// validateInventoryLimits checks all captured files and directories, including unreferenced assets.
// It uses the same byte and entry limits as catalog loading; it does not validate content or paths.
func validateInventoryLimits(ctx context.Context, files map[string][]byte, directories []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(files)+len(directories) > maxFiles {
		return bad("library", "library exceeds filesystem entry limit")
	}
	total := 0
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		size := len(files[name])
		if size > maxFileBytes || size > maxTotalBytes-total {
			return bad(name, "library exceeds file or total read limits")
		}
		total += size
	}
	return nil
}

// inventorySnapshot owns a bounded snapshot of every library-owned file, including unused assets and empty directories.
type inventorySnapshot struct {
	Files       map[string][]byte
	Directories []string
	License     *trailguide.LicenseDeclaration
}

// readLocalInventory captures complete library content, by library path, through the catalog's bounded local reader
// from the repository at root. Unrelated repository files are ignored; the caller owns root and no files are written.
func readLocalInventory(ctx context.Context, root *os.Root) (inventorySnapshot, error) {
	if root == nil {
		return inventorySnapshot{}, bad("library", "expected an open filesystem root")
	}
	return readInventory(ctx, rootFiles{ctx: ctx, root: root})
}

// readInventory shares capture logic with bounded sources so tests can verify early read termination. input reads
// the library's repository by repository path; the snapshot holds files by library path.
func readInventory(ctx context.Context, input FileSource) (inventorySnapshot, error) {
	r := reader{ctx: ctx, input: portableInventorySource{repositoryFiles{input}}, files: map[string][]byte{}}
	if _, err := r.read(librarytree.Manifest); err != nil {
		return inventorySnapshot{}, err
	}
	license, err := r.license("library")
	if err != nil {
		return inventorySnapshot{}, err
	}
	inventory := inventorySnapshot{Files: r.files, Directories: []string{}, License: license}
	// walk visits each owned directory once and enforces limits before traversing later siblings.
	var walk func(string, bool, int) error
	walk = func(directory string, optional bool, depth int) error {
		if depth > 64 {
			return bad(directory, "directory depth exceeds limit")
		}
		entries, err := r.entries(directory, optional)
		if err != nil {
			return err
		}
		if _, exists := r.directories[directory]; !exists {
			return nil
		}
		if err := r.registerPath(directory); err != nil {
			return err
		}
		inventory.Directories = append(inventory.Directories, directory)
		if len(r.files)+len(inventory.Directories) > maxFiles {
			return bad(directory, "library exceeds filesystem entry limit")
		}
		for _, entry := range entries {
			name := path.Join(directory, entry.Name())
			if entry.IsDir() {
				if err := walk(name, false, depth+1); err != nil {
					return err
				}
			} else {
				if _, err := r.read(name); err != nil {
					return err
				}
				if len(r.files)+len(inventory.Directories) > maxFiles {
					return bad(name, "library exceeds filesystem entry limit")
				}
			}
		}
		return nil
	}
	for _, directory := range []string{"assets", "practices", "techs"} {
		if err := walk(directory, true, 1); err != nil {
			return inventorySnapshot{}, err
		}
	}
	slices.Sort(inventory.Directories)
	if err := validateInventoryLimits(ctx, inventory.Files, inventory.Directories); err != nil {
		return inventorySnapshot{}, err
	}
	return inventory, nil
}

// portableInventorySource preserves the authoring snapshot's portable-path policy before any file access.
type portableInventorySource struct{ FileSource }

// Lstat rejects unsafe spellings before the shared reader inspects or opens files and directories.
func (s portableInventorySource) Lstat(name string) (fs.FileInfo, error) {
	if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00") || !utf8.ValidString(name) {
		return nil, bad(name, "expected a contained relative file path")
	}
	if strings.ContainsFunc(name, func(r rune) bool { return r < 32 || r == 127 }) {
		return nil, bad(name, "control characters are unsupported in paths")
	}
	return s.FileSource.Lstat(name)
}
