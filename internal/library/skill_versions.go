// Find the library's versioned skills and what its release notes say about their levels and forks.

package library

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// versionedSkills returns, sorted, the IDs of the skills in tree, the library's skills/ folder, that have versions:
// each folder with a valid skill name and a SKILL.md. skillProblems reports every other folder. A nil tree has none.
func versionedSkills(tree *filetxn.Tree) []string {
	ids := []string{}
	if tree == nil {
		return ids
	}
	for name := range tree.Files {
		if folder, file, _ := strings.Cut(name, "/"); file == skills.File && skills.ValidName(folder) {
			ids = append(ids, skills.AuthoredID(folder))
		}
	}
	slices.Sort(ids)
	return ids
}

// skillFolders returns the folder names in skills/ of skill IDs, skills/<name>.
func skillFolders(ids []string) []string {
	folders := make([]string, len(ids))
	for i, id := range ids {
		folders[i] = strings.TrimPrefix(id, skills.Directory+"/")
	}
	return folders
}

// releaseSkillContext reads the levels and forks of current, the library's skills, from manifest, the working
// tree's, and from the manifest of latest, the latest library release, which is nil before the first one. A latest
// library release made before the layout of version 2, or whose manifest doesn't parse, contributes no levels, and
// no forks. A manifest library check validated always parses, so manifest's errors are returned.
func (g *libraryGit) releaseSkillContext(ctx context.Context, latest *publishedRelease, manifest []byte, current []string) (skillContext, error) {
	var result skillContext
	var err error
	if result.levels, result.forks, err = skillLevelsAndForks(manifest, current); err != nil {
		return skillContext{}, err
	}
	if g == nil || latest == nil {
		return result, nil
	}
	previous, legacy, err := g.releasedManifest(ctx, latest)
	if err != nil || legacy {
		return result, err
	}
	// An earlier library release passed check with the Trail Guide that published it, which may have read its
	// manifest differently; a manifest this version rejects only leaves the notes without level and fork changes.
	if levels, forks, err := skillLevelsAndForks(previous, current); err == nil {
		result.previousLevels, result.previousForks = levels, forks
	}
	return result, nil
}

// publishedSkillContext rebuilds the levels and forks that release, a published library release, reported: those of
// its own manifest, compared with those of the library release before it. A library release without skills, or whose
// manifest this version can't read, reports none, and one whose previous library release can't be read reports no
// level or fork changes.
func (g *libraryGit) publishedSkillContext(ctx context.Context, release *publishedRelease) (skillContext, error) {
	if release.record.Skills == nil {
		return skillContext{}, nil
	}
	manifest, legacy, err := g.releasedManifest(ctx, release)
	if err != nil || legacy {
		return skillContext{}, err
	}
	var result skillContext
	if result.levels, result.forks, err = skillLevelsAndForks(manifest, slices.Sorted(maps.Keys(release.record.Skills.Versions))); err != nil {
		return skillContext{}, nil
	}
	if release.number == 1 {
		return result, nil
	}
	previous := &publishedRelease{number: release.number - 1}
	previous.object = "refs/tags/" + previous.tagName()
	if context, err := g.releaseSkillContext(ctx, previous, manifest, slices.Sorted(maps.Keys(release.record.Skills.Versions))); err == nil {
		result = context
	}
	return result, nil
}

// skillLevelsAndForks returns the level manifest gives each of ids, the library's own skills, and the fork of each
// that a source's skills.forks records. A manifest without a library section gives every skill offered.
func skillLevelsAndForks(manifest []byte, ids []string) (map[string]string, map[string]skillFork, error) {
	config, err := rules.ParseConfigurationYAML(manifest)
	if err != nil {
		return nil, nil, err
	}
	levels := map[string]string{}
	for _, id := range ids {
		levels[id] = skills.LevelOffered
		if config.Library != nil {
			levels[id] = config.Library.Skills.Level(id)
		}
	}
	forks := map[string]skillFork{}
	for _, source := range config.Sources {
		for _, id := range slices.Sorted(maps.Keys(source.SkillForks)) {
			fork := source.SkillForks[id]
			forks[fork.As] = skillFork{source: source.Name, id: id, basedOn: fork.BasedOn.Canonical()}
		}
	}
	return levels, forks, nil
}
