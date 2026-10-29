// Retain the files that installed skills link to, from the revision that supplies the skills, so they can be read
// offline wherever the skills are delivered.

package imports

import (
	"context"
	"maps"
	"slices"

	"github.com/fabricahq/trailguide/internal/skillrefs"
)

// referenceInput is the part of one repository revision that retainReferences reads: tree, the revision's files by
// repository path; origin, how those paths map to the snapshot's; folders, the files of every skill the import
// takes, by skill ID and snapshot path; delivered, the IDs of the skills whose links a project delivers, which
// leaves out those its configuration excludes or forks; excluded, those it excludes, whose Markdown files the
// delivered skills' links reach are followed too; and files, the bytes of the skills' files already read, by
// snapshot path, with retained, every byte the source's import has read, which counts toward the limits.
type referenceInput struct {
	tree      map[string]treeEntry
	origin    skillrefs.Origin
	folders   map[string]map[string]treeEntry
	delivered []string
	excluded  []string
	files     map[string][]byte
	retained  int
	// source and revision identify the revision in errors.
	source, revision string
}

// retainReferences reads the files outside the taken skills' folders that the delivered skills' Markdown links
// reach, directly or through other files it retains or the excluded skills' Markdown files they reach, and returns their bytes by snapshot path, without the origin's
// prefix. It fails as skillrefs.Retention.Retain does, naming a destination the revision lacks as missing, and for a
// file over the import byte limits.
func (r *repository) retainReferences(ctx context.Context, in referenceInput) (map[string][]byte, error) {
	view := &gitView{input: newGitFiles(ctx, r, in.tree), read: map[string][]byte{}}
	view.input.total = in.retained
	retention := skillrefs.Retention{View: view, Origin: in.origin, Absent: skillrefs.Missing, AbsentProblem: "the revision doesn't have. Ask the source's maintainer to fix the link, or to add the file; trailguide library check reports it.", Source: in.source, Revision: in.revision}
	for _, id := range slices.Sorted(maps.Keys(in.folders)) {
		folder, _ := in.origin.Path(id)
		retention.Folders = append(retention.Folders, folder)
		if slices.Contains(in.excluded, id) {
			retention.Reading = append(retention.Reading, folder)
		}
	}
	for _, id := range in.delivered {
		for key := range in.folders[id] {
			file, _ := in.origin.Path(key)
			if data, ok := in.files[key]; ok {
				view.read[file] = data
			}
			if skillrefs.IsMarkdown(key) {
				retention.Start = append(retention.Start, file)
			}
		}
	}
	retained, err := retention.Retain()
	if err != nil {
		return nil, err
	}
	result := map[string][]byte{}
	for _, file := range retained {
		data, err := view.Read(file)
		if err != nil {
			return nil, err
		}
		key, _ := in.origin.Key(file)
		result[key] = data
	}
	return result, nil
}

// gitView lends a revision's tree to skillrefs, reading each file once through input, which enforces the import's
// byte limits. read holds the files already read, by repository path.
type gitView struct {
	input *gitFiles
	read  map[string][]byte
}

// Stat reports a regular file, a folder, a symbolic link or submodule, or nothing at repository path file.
func (v *gitView) Stat(file string) skillrefs.Kind {
	entry, ok := v.input.entries[file]
	switch {
	case !ok:
		return skillrefs.Absent
	case entry.mode.IsDir():
		return skillrefs.Folder
	case entry.mode.IsRegular():
		return skillrefs.File
	}
	return skillrefs.Other
}

// Read returns the bytes of the regular file at repository path file, reading it from Git the first time.
func (v *gitView) Read(file string) ([]byte, error) {
	if data, ok := v.read[file]; ok {
		return data, nil
	}
	data, err := v.input.ReadFile(file)
	if err != nil {
		return nil, err
	}
	v.read[file] = data
	return data, nil
}

// deliveredSkills returns, sorted, the skills of installed whose links a project delivers: those that exclude, the
// exclusions by ID, and forks, the forked skills, don't name, since the project installs neither.
func deliveredSkills[E, F any](installed []string, exclude map[string]E, forks map[string]F) []string {
	delivered := []string{}
	for _, id := range installed {
		_, excluded := exclude[id]
		_, forked := forks[id]
		if !excluded && !forked {
			delivered = append(delivered, id)
		}
	}
	slices.Sort(delivered)
	return delivered
}

// excludedSkills returns, sorted, the skills of taken that exclude, the exclusions by ID, names.
func excludedSkills[E any](taken []string, exclude map[string]E) []string {
	excluded := []string{}
	for _, id := range taken {
		if _, ok := exclude[id]; ok {
			excluded = append(excluded, id)
		}
	}
	slices.Sort(excluded)
	return excluded
}
