// Merge a source's changes to a forked skill into the fork, file by file, refusing every file both changed in ways
// that touch the same lines.

package imports

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/fabricahq/trailguide/internal/gitexec"
)

// SkillCopy is a copy of a skill's files, by path in its folder, and, sorted, the paths of its executable files.
type SkillCopy struct {
	Files      map[string][]byte
	Executable []string
}

// file returns the copy's file at name, and whether it has one.
func (c SkillCopy) file(name string) (copyFile, bool) {
	data, ok := c.Files[name]
	return copyFile{data, slices.Contains(c.Executable, name)}, ok
}

// copyFile is one file of a SkillCopy.
type copyFile struct {
	data       []byte
	executable bool
}

// maxMergedFileBytes bounds the output of merging one file, which is never larger than the three copies together.
const maxMergedFileBytes = 64 << 20

// MergeSkill applies the changes upstream makes to base, the copy a fork was made from, to fork, and returns the
// merged copy and, sorted, the files whose changes conflict, with no merged copy when any do. A file only one side
// changed takes that side's copy, contents, presence, and executable bit alike. A text file both changed is merged
// line by line, as git merge-file merges it, keeping the executable bit a side changed, and conflicts when both
// changed the same lines, or when only one kept it, or both added it with other contents, or a copy is binary.
func MergeSkill(ctx context.Context, options Options, base, fork, upstream SkillCopy) (SkillCopy, []string, error) {
	merged := SkillCopy{Files: map[string][]byte{}, Executable: []string{}}
	conflicts := []string{}
	names := map[string]bool{}
	for _, side := range []SkillCopy{base, fork, upstream} {
		for name := range side.Files {
			names[name] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		original, inBase := base.file(name)
		ours, inFork := fork.file(name)
		theirs, inUpstream := upstream.file(name)
		same := func(a copyFile, inA bool, b copyFile, inB bool) bool {
			return inA == inB && (!inA || a.executable == b.executable && bytes.Equal(a.data, b.data))
		}
		result, inResult := ours, inFork
		switch {
		case same(original, inBase, ours, inFork):
			result, inResult = theirs, inUpstream
		case same(original, inBase, theirs, inUpstream), same(ours, inFork, theirs, inUpstream):
		case !inBase || !inFork || !inUpstream || binary(original.data) || binary(ours.data) || binary(theirs.data):
			conflicts = append(conflicts, name)
			continue
		default:
			data, clean, err := mergeFile(ctx, options, original.data, ours.data, theirs.data)
			if err != nil {
				return SkillCopy{}, nil, err
			}
			executable := ours.executable
			if ours.executable == original.executable {
				executable = theirs.executable
			}
			if !clean {
				conflicts = append(conflicts, name)
				continue
			}
			result = copyFile{data, executable}
		}
		if !inResult {
			continue
		}
		merged.Files[name] = result.data
		if result.executable {
			merged.Executable = append(merged.Executable, name)
		}
	}
	if len(conflicts) > 0 {
		return SkillCopy{}, conflicts, nil
	}
	return merged, conflicts, nil
}

// binary reports whether data holds a NUL byte, as Git judges a file binary.
func binary(data []byte) bool { return bytes.IndexByte(data, 0) >= 0 }

// mergeFile merges the changes theirs makes to base into ours with git merge-file, in a temporary directory, and
// returns the merged contents and whether they merged without conflicts.
func mergeFile(ctx context.Context, options Options, base, ours, theirs []byte) ([]byte, bool, error) {
	runner, err := gitexec.Isolated(gitexec.Options{GitPath: options.GitPath, Environment: options.Environment})
	if err != nil {
		return nil, false, err
	}
	dir, err := os.MkdirTemp("", "trailguide-merge-")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)
	for name, data := range map[string][]byte{"base": base, "ours": ours, "theirs": theirs} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return nil, false, err
		}
	}
	result, err := runner.Run(ctx, dir, []string{"merge-file", "--stdout", "--quiet", "ours", "base", "theirs"}, maxMergedFileBytes, nil)
	if err != nil {
		return nil, false, err
	}
	// git merge-file exits with the number of conflicts, up to 127, and with a larger or negative status when it
	// fails.
	switch {
	case result.Status == 0:
		return result.Output, true, nil
	case result.Status > 0 && result.Status <= 127:
		return nil, false, nil
	}
	return nil, false, fail("git-failed", "Git could not merge a forked skill's file.", nil)
}
