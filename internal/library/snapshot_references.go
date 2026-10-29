// Find the files a snapshot retains only because its delivered skills link to them, from the snapshot alone.

package library

import (
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/skillrefs"
)

// Origins returns how each repository the snapshot holds files of maps its paths to the snapshot's: the source's
// own, keyed by "", and each source it curates skills from, by name.
func (s Snapshot) Origins() map[string]skillrefs.Origin {
	origins := map[string]skillrefs.Origin{"": {Library: !s.NonLibrary}}
	for name, curated := range s.Curated {
		origins[name] = skillrefs.Origin{Library: !curated.NonLibrary, Prefix: CuratedDirectory + "/" + name + "/"}
	}
	return origins
}

// OriginOf returns the name of the origin, as Origins keys it, whose folder of the snapshot holds the file at path
// key.
func (s Snapshot) OriginOf(key string) string {
	rest, found := strings.CutPrefix(key, CuratedDirectory+"/")
	if !found {
		return ""
	}
	name, _, _ := strings.Cut(rest, "/")
	if _, ok := s.Curated[name]; !ok {
		return ""
	}
	return name
}

// ReferenceFiles returns the paths of the files the snapshot holds outside its skills' folders because the links of
// the skills a project delivers reach them, as skillrefs.Retention.Retain finds them in the snapshot's files.
// delivered reports whether the project delivers the installed skill with that ID, which it doesn't when it
// excludes or forks it, and excluded whether it excludes it, so links reaching its Markdown are followed too. source
// names the source in errors. It fails as Retain does, naming a destination the
// snapshot lacks as not retained, so an explicit sync restores it from the recorded revision.
func (s Snapshot) ReferenceFiles(source string, delivered, excluded func(id string) bool) (map[string]bool, error) {
	view := snapshotView{files: s.Files, folders: map[string]bool{}}
	for key := range s.Files {
		for parent := key; strings.Contains(parent, "/"); {
			parent = parent[:strings.LastIndex(parent, "/")]
			view.folders[parent] = true
		}
	}
	result := map[string]bool{}
	origins := s.Origins()
	for _, name := range slices.Sorted(maps.Keys(origins)) {
		origin := origins[name]
		view.origin = origin
		retention := skillrefs.Retention{View: view, Origin: origin, Absent: skillrefs.NotRetained, AbsentProblem: "the project's copy of the source doesn't retain. Run trailguide project sync, which takes the files installed skills link to from the revision .trailguide/lock.json records, without moving the source.", Source: "sources." + source, Revision: s.Commit}
		if name != "" {
			retention.Source, retention.Revision = "sources."+source+", curated from "+name, s.Curated[name].Commit
		}
		for _, id := range slices.Sorted(maps.Keys(s.Skills)) {
			folder := SkillFolder(id)
			if s.OriginOf(folder+"/") != name {
				continue
			}
			repository, _ := origin.Path(folder)
			retention.Folders = append(retention.Folders, repository)
			if excluded(id) {
				retention.Reading = append(retention.Reading, repository)
			}
			if !delivered(id) {
				continue
			}
			for key := range s.Files {
				if strings.HasPrefix(key, folder+"/") && skillrefs.IsMarkdown(key) {
					file, _ := origin.Path(key)
					retention.Start = append(retention.Start, file)
				}
			}
		}
		retained, err := retention.Retain()
		if err != nil {
			return nil, err
		}
		for _, file := range retained {
			key, _ := origin.Key(file)
			result[key] = true
		}
	}
	return result, nil
}

// snapshotView lends one origin's files of a snapshot to skillrefs by repository path. folders holds every folder
// the snapshot's paths imply.
type snapshotView struct {
	files   map[string][]byte
	folders map[string]bool
	origin  skillrefs.Origin
}

// Stat reports a file or folder the snapshot holds at repository path file, or nothing, including for a path the
// snapshot can't hold.
func (v snapshotView) Stat(file string) skillrefs.Kind {
	key, ok := v.origin.Key(file)
	if !ok {
		return skillrefs.Absent
	}
	if _, ok := v.files[key]; ok {
		return skillrefs.File
	}
	if v.folders[key] {
		return skillrefs.Folder
	}
	return skillrefs.Absent
}

// Read returns the bytes of the file at repository path file.
func (v snapshotView) Read(file string) ([]byte, error) {
	key, _ := v.origin.Key(file)
	return v.files[key], nil
}
