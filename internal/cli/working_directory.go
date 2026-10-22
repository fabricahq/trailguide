// Resolve CLI targets at repository boundaries without changing the caller's input-file paths.

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/project"
)

// commandDirectory resolves the nearest .git boundary for ordinary commands and refuses nested init.
// No Git executable is needed. Git files mark worktrees/submodules; their metadata is not traversed.
// Outside a repository, the supplied directory remains the target, including a not-yet-created library.
func commandDirectory(ctx context.Context, directory, scope string, initialize bool) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if scope == "library" {
		info, err := os.Lstat(absolute)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err == nil && !info.IsDir() {
			return "", &filetxn.Error{Code: "unsafe-path", Problem: absolute + ": expected a directory without symlinks"}
		}
	}
	if !initialize {
		info, err := os.Stat(absolute)
		if errors.Is(err, os.ErrNotExist) {
			return "", &filetxn.Error{Code: "needs-init", Problem: absolute + ": directory does not exist; choose an existing " + scope + " root", Cause: err}
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s: expected a directory", absolute)
		}
	}
	physical, err := physicalDirectory(ctx, absolute)
	if err != nil {
		return "", err
	}
	for current := physical; ; current = filepath.Dir(current) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		marker, err := os.Lstat(filepath.Join(current, ".git"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("locate repository: %w", err)
		}
		if err == nil {
			if !marker.IsDir() && !marker.Mode().IsRegular() {
				return "", &filetxn.Error{Code: "unsafe-path", Problem: filepath.Join(current, ".git") + ": expected a Git directory or file without symlinks"}
			}
			if initialize && current != physical {
				return "", &filetxn.Error{Code: "not-repository-root", Problem: fmt.Sprintf("initialize Trail Guide at the Git repository root, not in a subfolder. Run:\n  cd %s && trailguide %s init", shellDirectory(current), scope)}
			}
			warnLegacyDirectory(ctx, current, scope)
			return current, nil
		}
		if filepath.Dir(current) == current {
			warnLegacyDirectory(ctx, absolute, scope)
			return absolute, nil
		}
	}
}

// physicalDirectory resolves existing aliases while retaining a missing target suffix for init checks.
func physicalDirectory(ctx context.Context, directory string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err == nil {
		// A dangling link is an error, never a missing directory that init can create.
		resolved, err := filepath.EvalSymlinks(directory)
		if err != nil {
			return "", err
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf("%s: expected a directory", directory)
		}
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(directory)
	if parent == directory {
		return "", err
	}
	resolved, err := physicalDirectory(ctx, parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(directory)), nil
}

func shellDirectory(directory string) string {
	return "'" + strings.ReplaceAll(directory, "'", "'\"'\"'") + "'"
}

// legacyDirectoryWarning tells a project that keeps its files in the directory Code Rules used how to move them.
const legacyDirectoryWarning = `this project keeps its files in .code-rules/, the directory Code Rules used before it was renamed Trail Guide. Trail Guide still reads and updates it, but a later version won't. Rename it, keeping everything in it:

  git mv .code-rules .trailguide

In a project that isn't in a Git repository, use mv instead of git mv. Then point the line in AGENTS.md or CLAUDE.md that sends agents to .code-rules/generated/RULES.md at .trailguide/_managed/generated/RULES.md, and run trailguide project sync, which moves vendor/ and generated/ under _managed/.`

// warnLegacyDirectory warns, for a project command, when root, the project root, has no Trail Guide directory but has
// one under the name Code Rules used, which the project package then opens.
func warnLegacyDirectory(ctx context.Context, root, scope string) {
	if scope != "project" {
		return
	}
	if project.UsesLegacyDirectory(root) {
		warn(ctx, legacyDirectoryWarning)
	}
}
