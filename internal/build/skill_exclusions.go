// Apply the skills each source excludes, and pair each fork with the skill it replaces.

package build

import (
	"fmt"
	"maps"
	"slices"

	"github.com/fabricahq/trailguide/internal/rules"
)

// skillReplacement is what an authored skill replaces: a source's skill, which the project forks, installing the
// authored skill in its place.
type skillReplacement struct {
	// Upstream is the forked skill as its source supplies it, without its files; its source needn't have it any more.
	Upstream resolvedSkill
	// Reason is the fork's reason, and BasedOn the commit the fork was copied from, or the zero GitRef.
	Reason  string
	BasedOn rules.GitRef
}

// excludedSkill is a source's skill that the project leaves out without a replacement.
type excludedSkill struct {
	// ID is the skill's project ID, source:folder.
	ID, Reason string
}

// skillReplacements returns, by authored skill ID, the skill each authored skill in local replaces. It fails when an
// exclusion names a skill its source's snapshot doesn't hold, when a fork's authored skill doesn't exist, and when
// forks of two sources install the same authored skill. A forked skill the source's revision no longer has still
// replaces it, so the fork keeps working after its source deletes the skill.
func skillReplacements(config rules.Configuration, libraries map[string]Library, local map[string]SkillFiles) (map[string]skillReplacement, error) {
	result := map[string]skillReplacement{}
	// replaces maps each authored skill that replaces one to the configuration entry that names it.
	replaces := map[string]string{}
	for _, source := range config.Sources {
		snapshot := libraries[source.Name].Snapshot
		for _, id := range slices.Sorted(maps.Keys(source.SkillExclude)) {
			if _, installs := snapshot.Skills[id]; !installs {
				return nil, invalid("sources."+source.Name+".skills.exclude."+id, fmt.Sprintf("the source doesn't install skill %s, so there's nothing to exclude; name a skill its skills.include selects, or remove the entry", id))
			}
		}
		for _, id := range slices.Sorted(maps.Keys(source.SkillForks)) {
			fork := source.SkillForks[id]
			where := "sources." + source.Name + ".skills.forks." + id + ".as"
			if _, authored := local[fork.As]; !authored {
				return nil, invalid(where, fmt.Sprintf("the project has no authored skill %s; add .trailguide/%s/SKILL.md, or remove the fork", fork.As, fork.As))
			}
			if other, taken := replaces[fork.As]; taken {
				return nil, invalid(where, fmt.Sprintf("%s already replaces the skill %s names; each authored skill replaces one skill", fork.As, other))
			}
			replaces[fork.As] = "sources." + source.Name + ".skills.forks." + id
			result[fork.As] = skillReplacement{Upstream: sourceSkill(source, snapshot, id), Reason: fork.Reason, BasedOn: fork.BasedOn}
		}
	}
	return result, nil
}

// excludedSkills returns, in source order and then by ID, the skills of sources that the project excludes without a
// replacement.
func excludedSkills(sources []resolvedSource) []excludedSkill {
	result := []excludedSkill{}
	for _, source := range sources {
		for _, id := range source.Skills {
			if exclusion, excluded := source.Configured.SkillExclude[id]; excluded {
				result = append(result, excludedSkill{ID: source.Name + ":" + id, Reason: exclusion.Reason})
			}
		}
	}
	return result
}
