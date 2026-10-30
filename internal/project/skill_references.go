// Check that every local link in the project's own skills names a file the project's repository has once the
// command writes its output.

package project

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skillrefs"
	"github.com/fabricahq/trailguide/internal/skills"
)

// maxReferencedBytes bounds one file outside skills/ that an authored skill links to, as an import bounds a blob.
const maxReferencedBytes = 8 << 20

// authoredSkillsDirectory is the authored skills' folder, by path from the repository root.
const authoredSkillsDirectory = librarytree.Directory + "/" + skills.Directory

// requireAuthoredSkillReferences fails with a *skillrefs.Error when a link in the Markdown of an authored skill in
// state, the project's skills/ folder as the command would leave it, including the forks that replace skills from
// sources, or of a Markdown file those links reach, names a file the project's repository won't have once the
// command writes output, a folder, a symbolic link, or a file outside the repository. Files in the configured skill
// targets and in _managed/ are the ones output writes, and any other file is the repository's own. Build points the
// installed copies' links at the same files, so every agent target reaches them. It returns the bytes of each
// reached file of the repository's own outside skills/, by repository path, for the write to require unchanged.
func requireAuthoredSkillReferences(ctx context.Context, state projectState, output build.Output) (map[string][]byte, error) {
	if state.skills == nil || len(state.skills.Files) == 0 {
		return nil, nil
	}
	repository, err := os.OpenRoot(filepath.Dir(state.directory))
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	view := &projectView{ctx: ctx, root: repository, skills: state.skills, output: output, folders: map[string]bool{}, installed: map[string]string{}}
	for name := range state.skills.Files {
		for parent := filepath.ToSlash(filepath.Dir(name)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			view.folders[parent] = true
		}
	}
	// A link into skills/<source>/<name> for a skill source installs as name names its installed copy, as a fork's
	// link to a sibling skill of its source does, and build points it there.
	for _, skill := range build.InstalledSkills(output.Files[build.InstalledSkillsFile]) {
		source, _, fromSource := strings.Cut(skill.ID, ":")
		location := skills.Folder(source, skill.Name)
		if fromSource && len(skill.Targets) > 0 && !view.folders[location] {
			view.installed[authoredSkillsDirectory+"/"+location] = skill.Targets[0]
		}
	}
	for _, target := range state.config.SkillTargets {
		view.targets = append(view.targets, rules.SkillTargetFolders[target])
	}
	retention := skillrefs.Retention{View: view, Absent: skillrefs.Missing, AbsentProblem: "isn't in the project, or among the files Trail Guide installs and generates. Add the file, or fix the link."}
	// A link into the files output writes must name one, but their own links are build's to deliver, such as the
	// skills index's links into the vendor snapshot when no skill target is configured, so they aren't followed.
	retention.Folders = append([]string{librarytree.Directory + "/" + librarytree.Managed}, view.targets...)
	retention.Folders = append(retention.Folders, slices.Sorted(maps.Keys(view.installed))...)
	authored, err := authoredSkills(state.config, state.skills)
	if err != nil {
		return nil, err
	}
	for _, id := range slices.Sorted(maps.Keys(authored)) {
		location := authoredSkillsDirectory + "/" + authored[id].Folder()
		retention.Folders = append(retention.Folders, location)
		for _, name := range slices.Sorted(maps.Keys(authored[id].Files)) {
			if skillrefs.IsMarkdown(name) {
				retention.Start = append(retention.Start, location+"/"+name)
			}
		}
	}
	retained, err := retention.Retain()
	if err != nil {
		return nil, err
	}
	linked := map[string][]byte{}
	for _, file := range retained {
		if view.written(file) || strings.HasPrefix(file, authoredSkillsDirectory+"/") {
			continue
		}
		if linked[file], err = view.Read(file); err != nil {
			return nil, err
		}
	}
	return linked, nil
}

// requireLinkedFilesUnchanged fails with concurrent-change unless each of linked, the files the project's skills
// link to, by path from the root of the repository that holds the Trail Guide directory at directory, is still a
// regular file with the same bytes.
func requireLinkedFilesUnchanged(ctx context.Context, directory string, linked map[string][]byte) error {
	if len(linked) == 0 {
		return nil
	}
	repository, err := os.OpenRoot(filepath.Dir(directory))
	if err != nil {
		return err
	}
	defer repository.Close()
	view := &projectView{ctx: ctx, root: repository}
	for _, file := range slices.Sorted(maps.Keys(linked)) {
		if view.Stat(file) != skillrefs.File {
			return failure("concurrent-change", file+", which a skill links to, changed during the operation; retry after edits finish", nil)
		}
		data, err := view.Read(file)
		if err != nil || !bytes.Equal(data, linked[file]) {
			return failure("concurrent-change", file+", which a skill links to, changed during the operation; retry after edits finish", err)
		}
	}
	return nil
}

// projectView lends the project's repository, as the command leaves it, to skillrefs by repository path: the files
// of its skills/ folder from skills; the files output writes in the skill targets, targets, and in _managed/, of
// which only generated/ is there to link to; and any other file from root, bounded, refusing symbolic links, as it
// first reads it, so the bytes a check validates are the ones a write later requires unchanged. folders holds the
// folders skills' paths imply, and installed maps skills/<source>/<name>, by repository path, for each skill a source
// installs as name with no authored folder there, to the folder of its installed copy.
type projectView struct {
	ctx       context.Context
	root      *os.Root
	skills    *filetxn.Tree
	output    build.Output
	targets   []string
	folders   map[string]bool
	installed map[string]string
	read      map[string][]byte
}

// written reports whether output owns repository path file: it is in a skill target's folder or in _managed/.
func (v *projectView) written(file string) bool {
	if strings.HasPrefix(file, librarytree.Directory+"/"+librarytree.Managed+"/") {
		return true
	}
	for _, target := range v.targets {
		if strings.HasPrefix(file, target+"/") {
			return true
		}
	}
	return false
}

// writtenKind reports what output leaves at repository path file, which written owns.
func (v *projectView) writtenKind(file string) skillrefs.Kind {
	if name, ok := strings.CutPrefix(file, librarytree.Directory+"/"+librarytree.Generated+"/"); ok {
		return kindIn(maps.Keys(v.output.Files), name)
	}
	for folder, installed := range v.output.Skills {
		if name, ok := strings.CutPrefix(file, folder+"/"); ok {
			return kindIn(maps.Keys(installed.Files), name)
		}
		if file == folder {
			return skillrefs.Folder
		}
	}
	return skillrefs.Absent
}

// kindIn reports whether name is one of files, a folder holding one, or absent.
func kindIn(files func(func(string) bool), name string) skillrefs.Kind {
	kind := skillrefs.Absent
	for file := range files {
		if file == name {
			return skillrefs.File
		}
		if strings.HasPrefix(file, name+"/") {
			kind = skillrefs.Folder
		}
	}
	return kind
}

// installedAs returns the installed copy's folder and the rest of file when file, by repository path, is in
// skills/<source>/<name>, where name is a skill the command installs from source rather than an authored one.
func (v *projectView) installedAs(file string) (folder, rest string, ok bool) {
	for location, folder := range v.installed {
		if file == location {
			return folder, "", true
		}
		if rest, inside := strings.CutPrefix(file, location+"/"); inside {
			return folder, rest, true
		}
	}
	return "", "", false
}

// Stat reports what the repository holds at repository path file once the command writes its output.
func (v *projectView) Stat(file string) skillrefs.Kind {
	if folder, rest, ok := v.installedAs(file); ok {
		if rest == "" {
			return skillrefs.Folder
		}
		return kindIn(maps.Keys(v.output.Skills[folder].Files), rest)
	}
	if name, inside := strings.CutPrefix(file, authoredSkillsDirectory+"/"); inside && v.skills != nil {
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
	if v.written(file) {
		return v.writtenKind(file)
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

// Read returns the bytes of the regular file at repository path file, the same bytes each time, failing for one
// over maxReferencedBytes.
func (v *projectView) Read(file string) ([]byte, error) {
	if err := v.ctx.Err(); err != nil {
		return nil, err
	}
	if folder, rest, ok := v.installedAs(file); ok {
		return v.output.Skills[folder].Files[rest], nil
	}
	if name, inside := strings.CutPrefix(file, authoredSkillsDirectory+"/"); inside && v.skills != nil {
		return v.skills.Files[name], nil
	}
	if name, ok := strings.CutPrefix(file, librarytree.Directory+"/"+librarytree.Generated+"/"); ok && v.written(file) {
		return v.output.Files[name], nil
	}
	for folder, installed := range v.output.Skills {
		if name, ok := strings.CutPrefix(file, folder+"/"); ok {
			return installed.Files[name], nil
		}
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
		return nil, failure("limit-exceeded", file+", which a skill links to, exceeds 8 MiB.", errors.New("file too large"))
	}
	if v.read == nil {
		v.read = map[string][]byte{}
	}
	v.read[file] = data
	return data, nil
}
