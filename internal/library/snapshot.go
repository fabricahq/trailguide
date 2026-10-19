// Retain original library bytes with the record of the revisions and rule versions that produced them.

package library

import (
	"bytes"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// Snapshot owns one source's original library bytes and the record of what was imported, which works like a lockfile.
// A verified digest establishes integrity against the record, not authenticity of its origin.
// Binary content remains bytes; required text is checked by the loader.
type Snapshot struct {
	Repository string `json:"repository"`
	// RetiredRules lists, sorted, the rules the library had retired, as far as the snapshot's record knows, that the
	// source's groups or rules list selects; it is empty, never nil, when there are none. It is a fact about the
	// library, not the source's configuration, so offline checks can tell a pin or exclusion of a retired rule from
	// a typo. Configuration alone owns pins: each imported rule's version is what a pin must match.
	RetiredRules []string `json:"retiredRules"`
	// Ref repeats the source's ref when the snapshot was recorded, or is the zero GitRef.
	Ref rules.GitRef `json:"ref,omitzero"`
	// Release is the library release that supplied the library-wide files, or 0 when Ref names a revision other
	// than a library release.
	Release int `json:"release,omitempty"`
	// Commit is the full commit SHA of Release, or of the revision Ref names.
	Commit string `json:"resolvedCommit"`
	// Selection retains the requested group IDs or wildcard; Groups lists the groups imported in full.
	Selection rules.GroupSelection `json:"groupSelection"`
	Groups    []string             `json:"groups"`
	// RuleSelection lists the individually selected rule IDs, sorted; it is empty, never nil, when there are none.
	RuleSelection []string `json:"ruleSelection"`
	// Rules maps each imported rule's library rule ID, however it was selected, to its version record.
	Rules map[string]ImportedRule `json:"rules"`
	// Files maps each library path to its original bytes, excluding _source.json, except config.yaml, which holds
	// what the library publishes of its manifest, as rules.PublishedManifest gives it. The snapshot holds one version
	// of each imported rule, so every file is stored at its library path. Each skill's files are stored in the
	// folder SkillFolder names, such as skills/build-feature/SKILL.md.
	Files map[string][]byte `json:"files"`
	// NonLibrary reports that the source's repository has no library manifest at Commit, so the snapshot holds only
	// skills, at their paths in the repository, and no rules, release, or catalog files.
	NonLibrary bool `json:"nonLibrary,omitempty"`
	// SkillSelection repeats the source's skills.include selectors when the snapshot was recorded, sorted; it is
	// empty, never nil, when there are none.
	SkillSelection []string `json:"skillSelection"`
	// Skills maps the ID of each skill the source installs to its record; it is empty, never nil, when there are
	// none. Every skill the source publishes comes from Commit, and every curated skill from its Curated revision.
	Skills map[string]ImportedSkill `json:"skills"`
	// AvailableSkills maps the ID of every skill the source offers at Commit, installed or not, to its record, so
	// listings can show what else the project could install; it is empty, never nil, when there are none.
	AvailableSkills map[string]ImportedSkill `json:"availableSkills"`
	// Executable lists, sorted, the paths in Files whose executable bit the repository records; it is empty, never
	// nil, when there are none.
	Executable []string `json:"executable"`
	// Curated maps the name of each source in the library's manifest that supplies a curated skill the snapshot
	// holds to the revision that supplied it; it is nil when there are none. A curated skill's ID is
	// that name, a colon, and the skill's ID in the source's repository, such as mattpocock:skills/to-spec.
	Curated map[string]CuratedSource `json:"curated,omitempty"`
}

// CuratedSource records the revision of one source a library curates skills from, which the project resolved
// itself: a library release never locks it.
type CuratedSource struct {
	Repository string `json:"repository"`
	// Ref is the ref the revision was resolved from: the one the library's manifest names for the source, or the
	// project's own source's when Source overrides it; it is the zero GitRef when the curated repository follows its
	// newest library release.
	Ref rules.GitRef `json:"ref,omitzero"`
	// Release is the curated repository's library release at Commit, or 0 when Commit isn't one.
	Release int    `json:"release,omitempty"`
	Commit  string `json:"resolvedCommit"`
	// Source names the project's own source that declares the same repository, whose revision supplies the
	// curated skills in place of the library's ref; it is empty when the library's ref does.
	Source string `json:"source,omitempty"`
	// NonLibrary reports that the repository has no library manifest at Commit, so its skill IDs are their paths
	// from its root.
	NonLibrary bool `json:"nonLibrary,omitempty"`
}

// CuratedDirectory is the folder, in a library source's vendor snapshot, that holds the skills it curates, one
// folder per curated source.
const CuratedDirectory = "curated"

// SplitCurated returns the source name and the skill's ID in that source's repository for a curated skill's ID,
// such as mattpocock and skills/to-spec for mattpocock:skills/to-spec, and reports false for any other ID.
func SplitCurated(id string) (source, skill string, curated bool) {
	return strings.Cut(id, ":")
}

// SkillFolder returns the folder, in Files, that holds the files of skill id: its ID for a skill the source itself
// publishes, and curated/<source>/<ID> for a curated one.
func SkillFolder(id string) string {
	if source, skill, curated := SplitCurated(id); curated {
		return CuratedDirectory + "/" + source + "/" + skill
	}
	return id
}

// ImportedSkill records one installed skill from a source.
type ImportedSkill struct {
	// Level is the skill's level in its library, one of skills.Levels, or empty for a skill from a repository that
	// isn't a library.
	Level string `json:"level,omitempty"`
	// Included reports that the source's skills.include selects the skill; a skill it doesn't select is installed
	// because other installed skills of the source use it.
	Included bool `json:"included"`
	// UsedBy lists, sorted, the IDs of the source's installed skills that use this one, through their trailguide-skills-used
	// or the source's skills.uses; it is empty, never nil, when none does, which happens only when Included is set.
	UsedBy []string `json:"usedBy"`
}

// SelectedSkills returns, sorted, the IDs of the available skills that a source with skills.include selectors
// include installs: the required ones, the recommended ones for skills.Recommended, every one HiddenSkill doesn't
// hide for skills.Wildcard, and each one an ID names, less the ones excludes reports the source's level exclusions
// leave out by their level, which is never a required one.
func (s Snapshot) SelectedSkills(include []string, excludes func(level string) bool) []string {
	selected := []string{}
	for _, id := range slices.Sorted(maps.Keys(s.AvailableSkills)) {
		level := s.AvailableSkills[id].Level
		if level == skills.LevelRequired || (slices.Contains(include, skills.Wildcard) && !HiddenSkill(id) || slices.Contains(include, id) || level == skills.LevelRecommended && slices.Contains(include, skills.Recommended)) && !excludes(level) {
			selected = append(selected, id)
		}
	}
	return selected
}

// HiddenSkill reports whether skill id is in a folder whose name starts with a dot, such as another tool's installed
// copies in .claude/skills/, which "*" doesn't include.
func HiddenSkill(id string) bool {
	return slices.ContainsFunc(strings.Split(id, "/"), func(part string) bool { return strings.HasPrefix(part, ".") })
}

// SkillFiles returns the files of skill id, by path in its folder. The bytes are shared with the snapshot.
func (s Snapshot) SkillFiles(id string) map[string][]byte {
	files := map[string][]byte{}
	for name, data := range s.Files {
		if relative, ok := strings.CutPrefix(name, SkillFolder(id)+"/"); ok {
			files[relative] = data
		}
	}
	return files
}

// CuratedFrom returns what the snapshot holds of the source name it curates skills from, as a snapshot of that
// source's repository: its commit, its curated skills by their IDs there, and their files and executable files by
// path there. It is empty when the snapshot curates nothing from name.
func (s Snapshot) CuratedFrom(name string) Snapshot {
	result := Snapshot{Commit: s.Curated[name].Commit, Skills: map[string]ImportedSkill{}, Files: map[string][]byte{}, Executable: []string{}}
	for id, skill := range s.Skills {
		if source, folder, curated := SplitCurated(id); curated && source == name {
			result.Skills[folder] = skill
		}
	}
	prefix := CuratedDirectory + "/" + name + "/"
	for file, data := range s.Files {
		if relative, ok := strings.CutPrefix(file, prefix); ok {
			result.Files[relative] = data
		}
	}
	for _, file := range s.Executable {
		if relative, ok := strings.CutPrefix(file, prefix); ok {
			result.Executable = append(result.Executable, relative)
		}
	}
	return result
}

// SkillExecutable returns, sorted, the executable files of skill id, by path in its folder.
func (s Snapshot) SkillExecutable(id string) []string {
	executable := []string{}
	for _, name := range s.Executable {
		if relative, ok := strings.CutPrefix(name, SkillFolder(id)+"/"); ok {
			executable = append(executable, relative)
		}
	}
	return executable
}

// IsSkillFile reports whether file, a path in Files, belongs to one of the snapshot's skills.
func (s Snapshot) IsSkillFile(file string) bool {
	for id := range s.Skills {
		if strings.HasPrefix(file, SkillFolder(id)+"/") {
			return true
		}
	}
	return false
}

// ImportedRule records one imported rule's version and the library release that published it.
type ImportedRule struct {
	// Version is nil, and Release is 0, when the rule's files aren't a published version, which only a ref other
	// than a library release can import.
	Version *trailguide.RuleVersion `json:"version"`
	Release int                     `json:"release,omitempty"`
	// Commit is the full commit SHA of Release, or the snapshot's Commit when Version is nil.
	Commit string `json:"commit"`
	// LegacyLayout reports that Commit keeps the library in the layout before version 2, with the rule's files at
	// their library paths from the repository root rather than in .trailguide/rules/.
	LegacyLayout bool `json:"legacyLayout,omitempty"`
}

// Source returns the snapshot's files for the catalog loader to read.
func (s Snapshot) Source() FileSource {
	return newMemoryFiles(s.Files)
}

// memoryFiles lends in-memory files and the directories their paths imply to the catalog reader.
type memoryFiles struct {
	files       map[string][]byte
	directories map[string][]fs.DirEntry
}

// newMemoryFiles indexes the directories of files, which must be valid, contained paths.
func newMemoryFiles(files map[string][]byte) memoryFiles {
	children := map[string]map[string]fs.FileInfo{".": {}}
	for file, data := range files {
		name := file
		var info fs.FileInfo = memoryInfo{name: name, size: int64(len(data)), mode: 0644}
		for {
			parent := path.Dir(name)
			if children[parent] == nil {
				children[parent] = map[string]fs.FileInfo{}
			}
			children[parent][path.Base(name)] = info
			if parent == "." {
				break
			}
			name, info = parent, memoryInfo{name: parent, mode: fs.ModeDir | 0755}
		}
	}
	result := memoryFiles{files: files, directories: map[string][]fs.DirEntry{}}
	for directory, entries := range children {
		list := make([]fs.DirEntry, 0, len(entries))
		for _, name := range slices.Sorted(maps.Keys(entries)) {
			list = append(list, fs.FileInfoToDirEntry(entries[name]))
		}
		result.directories[directory] = list
	}
	return result
}

// Lstat reports files and the directories their paths imply.
func (m memoryFiles) Lstat(name string) (fs.FileInfo, error) {
	if data, ok := m.files[name]; ok {
		return memoryInfo{name: name, size: int64(len(data)), mode: 0644}, nil
	}
	if _, ok := m.directories[name]; ok {
		return memoryInfo{name: name, mode: fs.ModeDir | 0755}, nil
	}
	return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrNotExist}
}

// ReadFile lends original bytes; the catalog reader does not mutate its source.
func (m memoryFiles) ReadFile(name string) ([]byte, error) {
	if data, ok := m.files[name]; ok {
		return bytes.Clone(data), nil
	}
	return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
}

// ReadDir returns a copy of a directory's sorted entries.
func (m memoryFiles) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, ok := m.directories[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return slices.Clone(entries), nil
}

// memoryInfo supplies stable metadata for in-memory files and directories.
type memoryInfo struct {
	name string
	size int64
	mode fs.FileMode
}

// Name returns the final path component.
func (i memoryInfo) Name() string { return path.Base(i.name) }

// Size reports a file's byte count, or zero for a directory.
func (i memoryInfo) Size() int64 { return i.size }

// Mode distinguishes regular files from directories.
func (i memoryInfo) Mode() fs.FileMode { return i.mode }

// ModTime is zero because validation depends only on bytes.
func (i memoryInfo) ModTime() time.Time { return time.Time{} }

// IsDir reports whether the entry is a directory.
func (i memoryInfo) IsDir() bool { return i.mode.IsDir() }

// Sys has no operating-system identity.
func (i memoryInfo) Sys() any { return nil }
