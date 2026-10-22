// Read the skills a library curates from the sources in its manifest, which its skill levels name.

package rules

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// CuratedSource is a source a library's manifest names that it curates skills from: the repository and the ref
// that each project resolves for itself when it updates.
type CuratedSource struct {
	Repository string `json:"repository"`
	// Ref is the zero GitRef when the library follows the repository's newest library release.
	Ref GitRef `json:"ref,omitzero"`
}

// curatedSources returns, by name, the sources of sources that the library's skills curate from, or nil when they
// curate from none.
func curatedSources(levels trailguide.SkillLevels, sources []Source) map[string]CuratedSource {
	if len(levels.Curated()) == 0 {
		return nil
	}
	result := map[string]CuratedSource{}
	for _, id := range levels.Curated() {
		name, _, _ := strings.Cut(id, ":")
		index := slices.IndexFunc(sources, func(source Source) bool { return source.Name == name })
		result[name] = CuratedSource{Repository: sources[index].Repository, Ref: sources[index].Ref}
	}
	return result
}

// curatedNames returns, sorted, the names of the sources the raw library section's skills curate from, without
// validating anything else, so the published manifest can carry those sources. It reads only the lists the library
// format defines, which are all that consumers read.
func curatedNames(section json.RawMessage) []string {
	var library struct {
		Skills map[string]json.RawMessage `json:"skills"`
	}
	if json.Unmarshal(section, &library) != nil {
		return []string{}
	}
	names := map[string]bool{}
	for _, list := range skills.Lists {
		var ids []string
		if json.Unmarshal(library.Skills[list], &ids) != nil {
			continue
		}
		for _, id := range ids {
			if name, _, curated := strings.Cut(id, ":"); curated {
				names[name] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(names))
}
