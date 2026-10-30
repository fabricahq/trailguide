// Check that every local link in the library's skills names a file the library's repository has, as a project
// that installs the skills retains it.

package library

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skillrefs"
	"github.com/fabricahq/trailguide/internal/skills"
)

// maxReferencedBytes bounds one referenced file outside the library's skills that check reads, as an import bounds
// a blob.
const maxReferencedBytes = 8 << 20

// requireSkillReferences fails with a *skillrefs.Error when a link in the Markdown of one of found, the skills of
// tree, the library's skills/ folder, by ID, or of a Markdown file those links reach, names a file the repository under root doesn't
// have, a folder, a symbolic link, a file outside the repository, or one a project's snapshot can't hold: projects
// that install any of the skills retain every file their links reach, from the same revision. A file Git ignores
// counts as missing, since a library release doesn't have it; git is nil outside Git. It reads only what the links
// reach, and returns the bytes of each reached file outside skills/, by repository path, so that a library release
// can require its commit to hold exactly those bytes, as it does for the library's own files.
func requireSkillReferences(ctx context.Context, root *os.Root, git *libraryGit, tree *filetxn.Tree, found map[string]skills.Skill[[]byte]) (map[string][]byte, error) {
	if tree == nil {
		return nil, nil
	}
	view := &workingTreeView{ctx: ctx, root: root, git: git, skills: tree, read: map[string][]byte{}, folders: map[string]bool{}}
	for name := range tree.Files {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			view.folders[parent] = true
		}
	}
	retention := skillrefs.Retention{View: view, Origin: skillrefs.Origin{Library: true}, Absent: skillrefs.Missing, AbsentProblem: "the library doesn't have. Add the file, or fix the link; projects that install the skill keep a copy of every file its links reach."}
	for _, id := range slices.Sorted(maps.Keys(found)) {
		folder := skillsDirectory + "/" + found[id].Folder()
		retention.Folders = append(retention.Folders, folder)
		for _, name := range slices.Sorted(maps.Keys(found[id].Files)) {
			if skillrefs.IsMarkdown(name) {
				retention.Start = append(retention.Start, folder+"/"+name)
			}
		}
	}
	retained, err := retention.Retain()
	if err != nil {
		return nil, err
	}
	reached := map[string][]byte{}
	for _, file := range retained {
		if strings.HasPrefix(file, skillsDirectory+"/") {
			continue
		}
		if reached[file], err = view.Read(file); err != nil {
			return nil, err
		}
	}
	return reached, nil
}

// workingTreeView lends the library's working tree to skillrefs by repository path: the files of its skills/
// folder from skills, as check read them, and any other file from root, bounded, refusing symbolic links. read
// caches the files read outside skills/, and folders holds the folders skills' paths imply.
type workingTreeView struct {
	ctx     context.Context
	root    *os.Root
	git     *libraryGit
	skills  *filetxn.Tree
	read    map[string][]byte
	folders map[string]bool
}

// Stat reports what the working tree holds at repository path file.
func (v *workingTreeView) Stat(file string) skillrefs.Kind {
	if name, inside := strings.CutPrefix(file, skillsDirectory+"/"); inside {
		if _, ok := v.skills.Files[name]; ok {
			return skillrefs.File
		}
		if v.folders[name] {
			return skillrefs.Folder
		}
		return skillrefs.Absent
	}
	if file == skillsDirectory {
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
	// A file a library release doesn't publish isn't there for a project.
	if file == librarytree.Directory || strings.HasPrefix(file, librarytree.Directory+"/"+librarytree.Managed+"/") {
		return skillrefs.Absent
	}
	if v.git != nil {
		ignored, err := v.git.ignoredFiles(v.ctx, []string{file})
		if err != nil || ignored[file] {
			return skillrefs.Absent
		}
	}
	return skillrefs.File
}

// Read returns the bytes of the regular file at repository path file, failing for one over maxReferencedBytes.
func (v *workingTreeView) Read(file string) ([]byte, error) {
	if err := v.ctx.Err(); err != nil {
		return nil, err
	}
	if name, inside := strings.CutPrefix(file, skillsDirectory+"/"); inside {
		return v.skills.Files[name], nil
	}
	if data, ok := v.read[file]; ok {
		return data, nil
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
		return nil, failure("limit-exceeded", file+", which a skill links to, exceeds 8 MiB, more than a project imports from one file.", errors.New("file too large"))
	}
	v.read[file] = data
	return data, nil
}
