// Find the library's versioned skills, and what its release notes say about their levels and forks and about the
// library schema version.

package library

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// skillFolders returns the folder names in skills/ of skill IDs, skills/<name>.
func skillFolders(ids []string) []string {
	folders := make([]string, len(ids))
	for i, id := range ids {
		folders[i] = strings.TrimPrefix(id, skills.Directory+"/")
	}
	return folders
}

// releaseManifestContext reads the levels and forks of current, the library's skills, and the library schema version
// from manifest, the working tree's, and from the manifest of latest, the latest library release, which is nil before
// the first one. A latest library release made before the layout of version 2, or whose manifest doesn't parse,
// contributes no levels, and no forks. A manifest library check validated always parses, so manifest's errors are
// returned.
func (g *libraryGit) releaseManifestContext(ctx context.Context, latest *publishedRelease, manifest []byte, current []string) (manifestContext, error) {
	result := manifestContext{schemaVersion: declaredSchemaVersion(manifest)}
	var err error
	if result.levels, result.forks, err = skillLevelsAndForks(manifest, current); err != nil {
		return manifestContext{}, err
	}
	if g == nil || latest == nil {
		return result, nil
	}
	previous, legacy, err := g.releasedManifest(ctx, latest)
	if err != nil {
		return result, err
	}
	// A schema this version can't read still has a version, so it's read before the levels and forks.
	result.previousSchemaVersion = declaredSchemaVersion(previous)
	if legacy {
		return result, nil
	}
	// An earlier library release passed check with the Trail Guide that published it, which may have read its
	// manifest differently; a manifest this version rejects only leaves the notes without level and fork changes.
	if levels, forks, err := skillLevelsAndForks(previous, current); err == nil {
		result.previousLevels, result.previousForks = levels, forks
	}
	return result, nil
}

// publishedManifestContext rebuilds the levels and forks that release, a published library release, reported: those of
// its own manifest, compared with those of the library release before it. A library release without skills, or whose
// manifest this version can't read, reports none, and one whose previous library release can't be read reports no
// level or fork changes.
func (g *libraryGit) publishedManifestContext(ctx context.Context, release *publishedRelease) (manifestContext, error) {
	if release.record.Skills == nil {
		return manifestContext{}, nil
	}
	manifest, legacy, err := g.releasedManifest(ctx, release)
	if err != nil || legacy {
		return manifestContext{}, err
	}
	var result manifestContext
	if result.levels, result.forks, err = skillLevelsAndForks(manifest, slices.Sorted(maps.Keys(release.record.Skills.Versions))); err != nil {
		return manifestContext{}, nil
	}
	if release.number == 1 {
		return result, nil
	}
	previous := &publishedRelease{number: release.number - 1}
	previous.object = "refs/tags/" + previous.tagName()
	if context, err := g.releaseManifestContext(ctx, previous, manifest, slices.Sorted(maps.Keys(release.record.Skills.Versions))); err == nil {
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

// declaredSchemaVersion returns the library schema version manifest declares in library.schemaVersion, or in
// library.formatVersion, its name before Trail Guide v0.8.0, without validating anything else, so it reads the
// manifest of a library release in any library schema version. It returns 0 when manifest declares none, or doesn't
// parse.
func declaredSchemaVersion(manifest []byte) int {
	_, data, err := decode.YAML(manifest, manifestPath)
	if err != nil {
		return 0
	}
	var parsed struct {
		Library *struct {
			SchemaVersion *int `json:"schemaVersion"`
			FormatVersion *int `json:"formatVersion"`
		} `json:"library"`
	}
	if json.Unmarshal(data, &parsed) != nil || parsed.Library == nil {
		return 0
	}
	switch {
	case parsed.Library.SchemaVersion != nil:
		return *parsed.Library.SchemaVersion
	case parsed.Library.FormatVersion != nil:
		return *parsed.Library.FormatVersion
	}
	return 0
}
