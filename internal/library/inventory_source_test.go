// Exercise case-sensitive inventory paths independently of the host filesystem.

package library

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
)

// inventoryMapSource supplies immutable, case-sensitive fixture files.
type inventoryMapSource struct{ fs.FS }

// Lstat inspects fixture entries; these fixtures contain no symlinks.
func (s inventoryMapSource) Lstat(name string) (fs.FileInfo, error) { return fs.Stat(s.FS, name) }

// ReadFile returns the small fixture content.
func (s inventoryMapSource) ReadFile(name string) ([]byte, error) { return fs.ReadFile(s.FS, name) }

// ReadDir lists fixture directory entries.
func (s inventoryMapSource) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(s.FS, name)
}

// TestInventoryAbsentOptionalRoot does not invent a collision between declared terms and an absent practices tree.
func TestInventoryAbsentOptionalRoot(t *testing.T) {
	source := inventoryMapSource{fstest.MapFS{".trailguide/config.yaml": {Data: []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library","license":{"file":"Practices/LICENSE","notices":[]}}}`)}, "Practices/LICENSE": {Data: []byte("Terms")}}}
	inventory, err := readInventory(context.Background(), source)
	if err != nil || string(inventory.Files["Practices/LICENSE"]) != "Terms" {
		t.Fatal(inventory, err)
	}
}

// TestInventoryEmptyDirectoryAliases rejects real directory collisions even with no child files.
func TestInventoryEmptyDirectoryAliases(t *testing.T) {
	source := inventoryMapSource{fstest.MapFS{".trailguide/config.yaml": {Data: []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`)}, ".trailguide/assets/One": {Mode: fs.ModeDir}, ".trailguide/assets/one": {Mode: fs.ModeDir}}}
	if _, err := readInventory(context.Background(), source); err == nil {
		t.Fatal("directory alias accepted")
	}
}
