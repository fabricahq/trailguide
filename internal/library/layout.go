// Open a library repository in the version 2 layout, refuse the earlier layout, and lend its files to the catalog
// reader by library path.

package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
)

// legacyLayoutMoves is how many of a library's files the refusal of the layout before version 2 lists before it
// counts the rest.
const legacyLayoutMoves = 20

// legacyLayoutProblem tells a library still in the layout before version 2, which keeps the files at paths at its
// repository root, where each of the first legacyLayoutMoves belongs now, and how many more there are. A manifest,
// rule-library.yaml, must come first in paths. Empty paths mean only empty rule folders, to delete.
func legacyLayoutProblem(paths []string) string {
	const reads = "this version of Trail Guide reads a library's manifest from the library section of .trailguide/config.yaml, and its groups, shared assets, and change notes from .trailguide/. It no longer reads rule-library.yaml or rule folders at the repository root"
	if len(paths) == 0 {
		return reads + ". The rule folders there hold no files: delete them, then run trailguide library check."
	}
	moves := make([][2]string, 0, min(len(paths), legacyLayoutMoves))
	for _, from := range paths[:min(len(paths), legacyLayoutMoves)] {
		to := librarytree.RepositoryPath(from)
		if from == librarytree.LegacyManifest {
			to = "the library section of .trailguide/config.yaml"
		}
		moves = append(moves, [2]string{from, to})
	}
	problem := reads + ", and doesn't move them. Move them yourself, keeping each path below its folder:\n\n" +
		moveList(moves, len(paths)-len(moves)) + "\n\n"
	if paths[0] == librarytree.LegacyManifest {
		problem += `Then delete rule-library.yaml. Write its license fields, without formatVersion, in .trailguide/config.yaml, beside the sources the repository uses as a project, if any:

  schemaVersion: 2
  sources: {}
  library:
    formatVersion: 1
    license:
      spdxExpression: MIT
      file: LICENSE.md
      notices: []

License and notice paths stay relative to the repository root. `
	}
	return problem + "Rule IDs and versions don't change. A moved rule sits two folders deeper, so its relative links to shared assets need one more ../, such as ../../../assets/retry-lifecycle.svg, and its links to license files at the repository root need two more; links that start with /assets/ still work. Once the library has a library release, record each rule whose links change with trailguide library change --bump patch. Then run trailguide library check."
}

// moveList lists each of moves, a path and where it belongs, aligned on one line each, followed by a count of the
// more files, if any, that the list leaves out.
func moveList(moves [][2]string, more int) string {
	width := 0
	for _, move := range moves {
		width = max(width, len(move[0]))
	}
	lines := make([]string, 0, len(moves)+1)
	for _, move := range moves {
		lines = append(lines, fmt.Sprintf("  %-*s  ->  %s", width, move[0], move[1]))
	}
	switch {
	case more == 1:
		lines = append(lines, "  and 1 more file, moved the same way")
	case more > 1:
		lines = append(lines, fmt.Sprintf("  and %d more files, moved the same way", more))
	}
	return strings.Join(lines, "\n")
}

// legacyMarkers are the repository root's entries that mark a library in the layout before version 2, the manifest
// first, and legacyCompanions the shared assets and change notes that layout also kept there.
var (
	legacyMarkers    = []string{librarytree.LegacyManifest, "practices", "techs"}
	legacyCompanions = []string{"assets", "changes"}
)

// requireLibraryLayout fails, writing nothing, when the repository at root keeps a library in the layout before
// version 2: rule-library.yaml, or a techs or practices entry, at the repository root, whatever its kind. It lists
// where each of the library's files at the repository root belongs now. Without such an entry, it reads nothing
// else, so the repository's own assets/ and changes/ folders don't matter. It fails with ctx's error when ctx ends.
func requireLibraryLayout(ctx context.Context, root *os.Root) error {
	location := ""
	for _, name := range legacyMarkers {
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect the repository root's %s: %v", name, err)
		}
		location = name
		if info.IsDir() {
			location += "/"
		}
		break
	}
	if location == "" {
		return nil
	}
	var paths []string
	for _, name := range slices.Concat(legacyMarkers, legacyCompanions) {
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect the repository root's %s: %v", name, err)
		}
		if !info.IsDir() || name == librarytree.LegacyManifest {
			paths = append(paths, name)
			continue
		}
		files, err := listFiles(ctx, root.FS(), name)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return fmt.Errorf("list the files in the repository root's %s: %v", name, err)
		}
		paths = append(paths, files...)
	}
	// The manifest, when there is one, stays first.
	if len(paths) > 0 && paths[0] == librarytree.LegacyManifest {
		slices.Sort(paths[1:])
	} else {
		slices.Sort(paths)
	}
	return &ValidationError{Location: location, Problem: legacyLayoutProblem(paths)}
}

// listFiles returns the paths of every entry below directory in files that isn't a directory, without following
// links, so a link counts as one file. It fails when ctx ends, with ctx's error, or when a directory can't be read.
func listFiles(ctx context.Context, files fs.FS, directory string) ([]string, error) {
	var paths []string
	err := fs.WalkDir(files, directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, name)
		}
		return nil
	})
	return paths, err
}

// openTrailGuideDirectory opens the Trail Guide directory of the library repository at root, whose writer the
// repository's project and library commands share. create makes a missing directory; otherwise a missing one fails
// with needs-init. The caller closes the returned root.
func openTrailGuideDirectory(ctx context.Context, root *os.Root, create bool) (*os.Root, error) {
	directory := filepath.Join(root.Name(), librarytree.Directory)
	if create {
		return filetxn.Create(ctx, directory)
	}
	dir, err := filetxn.Open(ctx, directory)
	var missing *filetxn.Error
	if errors.As(err, &missing) && missing.Code == "missing-root" {
		return nil, failure("needs-init", "missing "+librarytree.Directory+"/config.yaml; run trailguide library init first", err)
	}
	return dir, err
}

// requireCurrentDirectory fails, writing nothing, when the repository at root has no Trail Guide directory but has one
// under the name Code Rules used, explaining how to rename it.
func requireCurrentDirectory(root *os.Root) error {
	if _, err := root.Lstat(librarytree.Directory); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if _, err := root.Lstat(librarytree.LegacyDirectory); err != nil {
		return nil
	}
	return &ValidationError{Location: librarytree.LegacyDirectory + "/", Problem: legacyDirectoryProblem}
}

// legacyDirectoryProblem tells a library that still keeps its files in the directory Code Rules used to move them.
const legacyDirectoryProblem = `Code Rules is now Trail Guide, and library commands read a library's files from .trailguide/. Rename the directory, keeping everything in it, then run the command again:

  git mv .code-rules .trailguide

Library releases published from .code-rules/ stay readable, so projects keep their versions. Projects that import this library need no changes. Update the line in AGENTS.md or CLAUDE.md that points agents to .code-rules/generated/RULES.md.`

// repositoryFiles lends a library repository's files, read through input by their repository paths, to the
// catalog reader by their library paths, as librarytree.RepositoryPath places them. The library root, ".", lists
// the library's top-level entries: its manifest and its assets, changes, practices, and techs folders, each spelled
// as the repository spells it.
type repositoryFiles struct{ input FileSource }

// Lstat inspects the file at library path name without following its final symbolic link.
func (r repositoryFiles) Lstat(name string) (fs.FileInfo, error) {
	if name == "." {
		return r.input.Lstat(name)
	}
	return r.input.Lstat(librarytree.RepositoryPath(name))
}

// ReadFile reads the file at library path name.
func (r repositoryFiles) ReadFile(name string) ([]byte, error) {
	return r.input.ReadFile(librarytree.RepositoryPath(name))
}

// ReadDir lists the directory at library path name, which keeps its entries' names.
func (r repositoryFiles) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return r.input.ReadDir(librarytree.RepositoryPath(name))
	}
	entries := []fs.DirEntry{}
	for _, directory := range []string{librarytree.Directory, librarytree.Directory + "/rules"} {
		listed, err := r.input.ReadDir(directory)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range listed {
			if library, ok := librarytree.LibraryPath(directory + "/" + entry.Name()); ok && !strings.Contains(library, "/") {
				entries = append(entries, entry)
			}
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

// requireIdle fails with busy while a writer holds, or a pending recovery waits in, the Trail Guide directory of the
// library repository at root, so a read-only command never reads a half-published edit.
func requireIdle(ctx context.Context, root *os.Root) error {
	dir, err := openTrailGuideDirectory(ctx, root, false)
	if err != nil {
		return err
	}
	defer dir.Close()
	return filetxn.RequireIdle(dir)
}
