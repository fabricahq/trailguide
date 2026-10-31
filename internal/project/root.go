// Locate and validate the fixed Trail Guide directory for every project operation.

package project

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
	"github.com/fabricahq/trailguide/internal/rules"
)

const configurationFile = "config.yaml"

// openProject applies the same no-link policy to readers and writers; only initialization creates the root. It opens
// the directory Directory names. It fails, closing the root, when the directory still has the local/ folder that
// held authored files before rules/ and assets/.
func openProject(ctx context.Context, options Options, create bool) (*os.Root, error) {
	directory := Directory(options.Directory)
	var root *os.Root
	var err error
	if create {
		root, err = filetxn.Create(ctx, directory)
	} else {
		root, err = filetxn.Open(ctx, directory)
	}
	if errors.Is(err, os.ErrNotExist) && !create {
		return nil, failure("needs-init", fmt.Sprintf(".trailguide: missing Trail Guide directory; run trailguide project init first from %s", options.Directory), err)
	}
	if err != nil {
		return nil, err
	}
	if err := requireNoLocalFolder(ctx, root); err != nil {
		root.Close()
		return nil, err
	}
	if err := requireToolVersion(ctx, root, options.ToolVersion); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

// withWriter is filetxn.WithWriter, except that operation runs only once requireToolVersion accepts version for the
// configuration as the writer found it, after recovering an interrupted command, which can restore a configuration
// that requires a newer release.
func withWriter(ctx context.Context, root *os.Root, version string, operation func(*filetxn.Writer) error) error {
	return filetxn.WithWriter(ctx, root, func(w *filetxn.Writer) error {
		if err := requireToolVersion(ctx, root, version); err != nil {
			return err
		}
		return operation(w)
	})
}

// edit is filetxn.Edit, except that prepare runs only once requireToolVersion accepts version for the configuration
// as the writer found it, as withWriter checks it.
func edit(ctx context.Context, root *os.Root, version string, prepare func() ([]filetxn.File, error)) (filetxn.Changes, error) {
	return filetxn.Edit(ctx, root, func() ([]filetxn.File, error) {
		if err := requireToolVersion(ctx, root, version); err != nil {
			return nil, err
		}
		return prepare()
	})
}

// requireToolVersion fails with upgrade-required when rules.UpgradeRequired refuses version, this build's version, for
// the configuration's configuration, before anything else reads the configuration, whose fields this version may not know.
func requireToolVersion(ctx context.Context, root *os.Root, version string) error {
	data, err := filetxn.ReadOptional(ctx, root, configurationFile)
	if err != nil {
		return err
	}
	if problem, required := rules.UpgradeRequired(data, version); required {
		return failure("upgrade-required", problem, nil)
	}
	return nil
}

// Directory returns the path of the Trail Guide directory of the project at root: .trailguide/, or .code-rules/ when
// UsesLegacyDirectory reports the project still keeps its files there.
func Directory(root string) string {
	if UsesLegacyDirectory(root) {
		return filepath.Join(root, librarytree.LegacyDirectory)
	}
	return filepath.Join(root, librarytree.Directory)
}

// UsesLegacyDirectory reports whether the project at root has no Trail Guide directory but has a directory under the
// name Code Rules used, .code-rules/, which project commands then read and update in its place.
func UsesLegacyDirectory(root string) bool {
	if _, err := os.Lstat(filepath.Join(root, librarytree.Directory)); !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	info, err := os.Lstat(filepath.Join(root, librarytree.LegacyDirectory))
	return err == nil && info.IsDir()
}

// localFolderMoves is how many of local/'s files the refusal lists before it counts the rest.
const localFolderMoves = 20

// localFolderProblem tells a project that still keeps the files at paths, each relative to the Trail Guide directory
// and below local/, where each of the first localFolderMoves belongs now, and how many more there are.
func localFolderProblem(paths []string) string {
	const reads = "this version of Trail Guide reads authored rules and groups from .trailguide/rules/, and shared assets from .trailguide/assets/. It no longer reads .trailguide/local/"
	if len(paths) == 0 {
		return reads + ", which holds no files: delete it, then run trailguide project build."
	}
	moves := make([][2]string, 0, min(len(paths), localFolderMoves))
	for _, from := range paths[:min(len(paths), localFolderMoves)] {
		file := strings.TrimPrefix(from, "local/")
		to := "rules/" + file
		if strings.HasPrefix(file, "assets/") {
			to = file
		}
		moves = append(moves, [2]string{from, to})
	}
	return reads + ", and doesn't move its files. Move them yourself, keeping each path below local/, then delete local/:\n\n" +
		moveList(moves, len(paths)-len(moves)) +
		"\n\nRule IDs don't change, so config.yaml needs no edits. A moved rule sits one folder deeper, so its links to shared assets need one more ../, such as ../../../assets/retry-lifecycle.svg. Then run trailguide project build."
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

// requireNoLocalFolder fails, writing nothing, when the Trail Guide directory has an entry named local, whatever its
// kind, listing where each file in it belongs now in rules/ and assets/.
func requireNoLocalFolder(ctx context.Context, root *os.Root) error {
	info, err := root.Lstat("local")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check for .trailguide/local: %v", err)
	}
	var paths []string
	if info.IsDir() {
		paths, err = listFiles(ctx, root.FS(), "local")
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return fmt.Errorf("list the files in .trailguide/local: %v", err)
		}
	}
	return &ValidationError{Location: ".trailguide/local/", Problem: localFolderProblem(paths)}
}

// listFiles returns the sorted paths of every entry below directory in files that isn't a directory, without
// following links, so a link counts as one file. It fails when ctx ends, with ctx's error, or when a directory can't
// be read.
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
	slices.Sort(paths)
	return paths, err
}

// configuration validates the fixed configuration and retains its exact bytes for change detection.
func configuration(ctx context.Context, root *os.Root) ([]byte, rules.Configuration, error) {
	data, err := filetxn.ReadOptional(ctx, root, configurationFile)
	if err != nil {
		return nil, rules.Configuration{}, err
	}
	if data == nil {
		return nil, rules.Configuration{}, failure("needs-init", fmt.Sprintf(".trailguide/config.yaml: missing configuration; run trailguide project init first from %s", filepath.Dir(root.Name())), nil)
	}
	if err := rules.RequireResolvedMerge(data, ".trailguide/config.yaml", "run the command again"); err != nil {
		return nil, rules.Configuration{}, err
	}
	config, err := rules.ParseConfigurationYAML(data)
	return data, config, err
}
