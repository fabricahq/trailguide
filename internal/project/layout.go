// Find where a project keeps its managed trees, and refuse a layout that only sync can move to _managed/.

package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
)

// managedLayout is where a project keeps its vendor snapshots and generated output: under _managed/, or, when legacy
// is set, at the top of the Trail Guide directory, where Trail Guide 0.7 and earlier kept them.
type managedLayout struct {
	vendor, generated filetxn.Target
	legacy            bool
}

// currentLayout keeps the managed trees under _managed/.
var currentLayout = managedLayout{vendor: filetxn.Vendor, generated: filetxn.Generated}

// vendorLocation names the vendor folder in diagnostics, from the repository root.
func (l managedLayout) vendorLocation() string {
	return librarytree.Directory + "/" + string(l.vendor) + "/"
}

// readLayout returns where the project at root keeps its managed trees. A project with neither tree uses the current
// layout. It fails with mixed-layout when a tree under _managed/ and a legacy tree are both present, which no command
// writes. An empty _managed/, as a sync that rolled back its move leaves, doesn't count, so the next sync can move.
func readLayout(root *os.Root) (managedLayout, error) {
	legacy := []string{}
	for _, name := range []string{librarytree.LegacyVendor, librarytree.LegacyGenerated} {
		present, err := present(root, name)
		if err != nil {
			return managedLayout{}, err
		}
		if present {
			legacy = append(legacy, name)
		}
	}
	if len(legacy) == 0 {
		return currentLayout, nil
	}
	managed := false
	for _, target := range []filetxn.Target{filetxn.Vendor, filetxn.Generated} {
		found, err := present(root, string(target))
		if err != nil {
			return managedLayout{}, err
		}
		managed = managed || found
	}
	if managed {
		return managedLayout{}, failure("mixed-layout", fmt.Sprintf(".trailguide/%s/ and .trailguide/%s/, a folder Trail Guide 0.7 and earlier wrote, are both present. Sync never writes both, so a merge combined a migrated branch with one that wasn't. Take one branch's .trailguide/%s/, %s/, and %s/ folders and %s, then run trailguide project sync", librarytree.Managed, legacy[0], librarytree.Managed, librarytree.LegacyVendor, librarytree.LegacyGenerated, lockFile), nil)
	}
	return managedLayout{vendor: filetxn.LegacyVendor, generated: filetxn.LegacyGenerated, legacy: true}, nil
}

// present reports whether name exists in root, without following a link.
func present(root *os.Root, name string) (bool, error) {
	_, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check for .trailguide/%s: %w", name, err)
	}
	return true, nil
}

// layoutMigrationNeeded is the failure of every command but sync in a project in the legacy layout.
func layoutMigrationNeeded() error {
	return failure("layout-migration-needed", fmt.Sprintf("the project keeps .trailguide/%s/ and .trailguide/%s/ where Trail Guide 0.7 and earlier did. Run trailguide project sync once to move them under .trailguide/%s/, which moves no source, and commit the result", librarytree.LegacyVendor, librarytree.LegacyGenerated, librarytree.Managed), nil)
}
