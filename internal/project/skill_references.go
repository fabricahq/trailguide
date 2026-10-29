// Check that every local link in the project's own skills names a file the project's repository has.

package project

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skillrefs"
	"github.com/fabricahq/trailguide/internal/skills"
)

// maxReferencedBytes bounds one file outside skills/ that an authored skill links to, as an import bounds a blob.
const maxReferencedBytes = 8 << 20

// authoredSkillsDirectory is the authored skills' folder, by path from the repository root.
const authoredSkillsDirectory = librarytree.Directory + "/" + skills.Directory

// requireAuthoredSkillReferences fails with a *skillrefs.Error when a link in the Markdown of an authored skill in
// tree, the project's skills/ folder as the command would leave it, including the forks that replace skills from
// sources, or of a Markdown file those links reach, names a file the project's repository doesn't have, a folder, a
// symbolic link, or a file outside the repository. Build points the installed copies' links at the same files, so
// every agent target reaches them. directory is the Trail Guide directory's path; the repository root holds it.
func requireAuthoredSkillReferences(ctx context.Context, directory string, tree *filetxn.Tree) error {
	if tree == nil || len(tree.Files) == 0 {
		return nil
	}
	repository, err := os.OpenRoot(filepath.Dir(directory))
	if err != nil {
		return err
	}
	defer repository.Close()
	view := &projectView{ctx: ctx, root: repository, skills: tree, folders: map[string]bool{}}
	for name := range tree.Files {
		for parent := filepath.ToSlash(filepath.Dir(name)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			view.folders[parent] = true
		}
	}
	retention := skillrefs.Retention{View: view, Absent: skillrefs.Missing, AbsentProblem: "the project doesn't have. Add the file, or fix the link; every agent target reaches the project's file through the installed copy."}
	for _, name := range slices.Sorted(maps.Keys(tree.Files)) {
		folder, _, inFolder := strings.Cut(name, "/")
		if !inFolder {
			continue
		}
		if _, ok := tree.Files[folder+"/"+skills.File]; !ok {
			continue
		}
		if location := authoredSkillsDirectory + "/" + folder; !slices.Contains(retention.Folders, location) {
			retention.Folders = append(retention.Folders, location)
		}
		if skillrefs.IsMarkdown(name) {
			retention.Start = append(retention.Start, authoredSkillsDirectory+"/"+name)
		}
	}
	_, err = retention.Retain()
	return err
}

// projectView lends the project's repository to skillrefs by repository path: the files of its skills/ folder from
// skills, as the command would leave them, and any other file from root, bounded, refusing symbolic links. folders
// holds the folders skills' paths imply.
type projectView struct {
	ctx     context.Context
	root    *os.Root
	skills  *filetxn.Tree
	folders map[string]bool
}

// Stat reports what the repository holds at repository path file.
func (v *projectView) Stat(file string) skillrefs.Kind {
	if name, inside := strings.CutPrefix(file, authoredSkillsDirectory+"/"); inside {
		if _, ok := v.skills.Files[name]; ok {
			return skillrefs.File
		}
		if v.folders[name] {
			return skillrefs.Folder
		}
		return skillrefs.Absent
	}
	if file == authoredSkillsDirectory {
		return skillrefs.Folder
	}
	parts := strings.Split(file, "/")
	for i := range parts {
		info, err := v.root.Lstat(strings.Join(parts[:i+1], "/"))
		switch {
		case err != nil:
			return skillrefs.Absent
		case info.Mode()&fs.ModeSymlink != 0:
			return skillrefs.Other
		case i < len(parts)-1 && !info.IsDir():
			return skillrefs.Absent
		case i == len(parts)-1 && info.IsDir():
			return skillrefs.Folder
		case i == len(parts)-1 && !info.Mode().IsRegular():
			return skillrefs.Other
		}
	}
	return skillrefs.File
}

// Read returns the bytes of the regular file at repository path file, failing for one over maxReferencedBytes.
func (v *projectView) Read(file string) ([]byte, error) {
	if err := v.ctx.Err(); err != nil {
		return nil, err
	}
	if name, inside := strings.CutPrefix(file, authoredSkillsDirectory+"/"); inside {
		return v.skills.Files[name], nil
	}
	handle, err := v.root.Open(file)
	if err != nil {
		return nil, failure("read-failed", "read "+file+", which a skill links to: "+err.Error(), err)
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, maxReferencedBytes+1))
	if err != nil {
		return nil, failure("read-failed", "read "+file+", which a skill links to: "+err.Error(), err)
	}
	if len(data) > maxReferencedBytes {
		return nil, failure("limit-exceeded", file+", which a skill links to, exceeds 8 MiB.", errors.New("file too large"))
	}
	return data, nil
}
