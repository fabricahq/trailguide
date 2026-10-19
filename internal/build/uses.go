// Link each installed skill to the skills it uses and those that use it, and describe both in SKILLS.md.

package build

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// linkSkillUses sets the Uses and UsedBy of every skill in resolved, the project's installed skills. A source's skill
// uses the skills its trailguide-skills-used names, which are skill IDs in the same source, or, for a library's skill,
// the IDs of skills the library curates, such as anthropic:skills/skill-creator, and those its source's skills.uses
// adds; sync installed them all. An authored skill uses another authored skill by its bare ID, such as skills/tdd,
// and a source's skill by its project ID, such as fabrica:skills/tdd. It fails at an authored skill's SKILL.md when
// it uses a skill the project doesn't install, or when authored skills use each other in a cycle; a source's skill
// that uses one the project doesn't install fails as invalid output, because its source record, which sync and the
// offline checks keep complete, can't produce one. A skill that uses a source's skill the project excludes uses the
// authored skill that replaces it instead; with no replacement, the use is dropped, and linkSkillUses returns a
// warning for it, in skill order. A use of a skill in aliases, a project source's copy of a curated skill that
// withoutCuratedCopies dropped, reaches the curated skill it maps to.
func linkSkillUses(resolved []resolvedSkill, config rules.Configuration, aliases map[string]string) ([]string, error) {
	sources := map[string]rules.Source{}
	// excluded maps the project ID of each source's skill the project excludes to the authored skill that replaces
	// it, or "" when none does.
	excluded := map[string]string{}
	for _, source := range config.Sources {
		sources[source.Name] = source
		for id, exclusion := range source.SkillExclude {
			excluded[source.Name+":"+id] = exclusion.ReplacedBy
		}
	}
	warnings := []string{}
	// redirect returns the installed skill a use of id reaches, and false when the project excludes id without a
	// replacement, which it warns about.
	redirect := func(skill resolvedSkill, id string) (string, bool) {
		replacement, isExcluded := excluded[id]
		switch {
		case !isExcluded && aliases[id] != "":
			return aliases[id], true
		case !isExcluded:
			return id, true
		case replacement != "":
			return replacement, true
		}
		fix := "Add replacedBy to the exclusion, or stop excluding the skill."
		if _, rest, _ := strings.Cut(id, ":"); strings.Contains(rest, ":") {
			// A curated skill can't be replaced; see rules.ParseConfiguration's exclusion checks.
			fix = "Stop excluding the skill, or remove the use from the skill that uses it, which you can fork. A curated skill can't be replaced."
		}
		warnings = append(warnings, fmt.Sprintf("%s uses %s, which the project excludes without a replacement, so the skill it uses isn't installed. %s", skill.ID, id, fix))
		return "", false
	}
	uses := map[string][]string{}
	for _, skill := range resolved {
		uses[skill.ID] = nil
	}
	for _, skill := range resolved {
		location := skillLocation(skill)
		used := []string{}
		if skill.Source != "" {
			// A curated skill uses skills of the repository it was curated from, which the library source installs
			// as curated skills too, and its source's skills.uses names none of them.
			prefix, declared := skill.Source+":", sources[skill.Source].SkillUses[skill.Folder]
			if skill.Curated != "" {
				prefix, declared = prefix+skill.Curated+":", nil
			}
			for _, id := range slices.Concat(skill.SkillsUsed, declared) {
				// A curated skill's uses of skills its own library curates aren't installed with it.
				if skill.Curated != "" && strings.Contains(id, ":") {
					continue
				}
				target, kept := redirect(skill, prefix+id)
				if !kept {
					continue
				}
				if _, installed := uses[target]; !installed {
					return nil, invalid(location, fmt.Sprintf("the skill uses %s, which the project doesn't install; run trailguide project sync", id))
				}
				used = append(used, target)
			}
		} else {
			for _, line := range skill.SkillsUsed {
				citation := skills.ParseCitation(line)
				target, kept := redirect(skill, citation.String())
				if !kept {
					continue
				}
				if _, installed := uses[target]; !installed {
					return nil, invalid(location, authoredUseProblem(citation, sources))
				}
				used = append(used, target)
			}
		}
		uses[skill.ID] = used
	}
	ids := []string{}
	for _, skill := range resolved {
		ids = append(ids, skill.ID)
	}
	installed, err := skills.ResolveUses(ids, uses)
	if cycle := (*skills.UseCycleError)(nil); errors.As(err, &cycle) {
		return nil, invalid(cycle.Cycle[0]+"/"+skills.File, fmt.Sprintf("%s forms a cycle: %s. A skill installs with every skill it uses, so uses can't form a cycle; remove one of them", skills.SkillsUsedKey, strings.Join(cycle.Cycle, " -> ")))
	}
	if err != nil {
		return nil, err
	}
	for i, skill := range resolved {
		resolved[i].UsedBy = installed[skill.ID].UsedBy
		used := append([]string{}, uses[skill.ID]...)
		slices.Sort(used)
		resolved[i].Uses = slices.Compact(used)
	}
	return warnings, nil
}

// authoredUseProblem explains why an authored skill can't use citation, a line of its trailguide-skills-used that names no
// skill the project installs, such as a curated skill its library stopped listing. sources holds the configuration's
// sources by name.
func authoredUseProblem(citation skills.Citation, sources map[string]rules.Source) string {
	source, configured := sources[citation.Source]
	switch {
	case citation.Source == "":
		return fmt.Sprintf("%s names %s, which isn't a skill in .trailguide/skills/; write the skill, or name a source's skill with its source name, such as fabrica:%s", skills.SkillsUsedKey, citation.ID, citation.ID)
	case !configured:
		return fmt.Sprintf("%s names %s, but the configuration has no source named %s; add the source, or correct the source name", skills.SkillsUsedKey, citation, citation.Source)
	case strings.Contains(citation.ID, ":"):
		// The ID names a skill the source's library curates, which the project takes only through its lists.
		return fmt.Sprintf("%s names %s, which library %s doesn't curate at the revision the project installs; remove it, or declare the skill's repository as a source of your own and name the skill with that source's name", skills.SkillsUsedKey, citation, source.Name)
	}
	return fmt.Sprintf("%s names %s, which the project doesn't install; add %s to sources.%s.skills.include, then run trailguide project sync", skills.SkillsUsedKey, citation, citation.ID, source.Name)
}

// useFacts returns the SKILLS.md lines that say which installed skills skill uses, and, for a source's skill, why it
// is installed, naming skills by install name. resolved holds every installed skill.
func useFacts(skill resolvedSkill, resolved []resolvedSkill) []string {
	names := func(ids []string) string {
		quoted := []string{}
		for _, id := range ids {
			for _, other := range resolved {
				if other.ID == id {
					quoted = append(quoted, "`"+other.Name+"`")
				}
			}
		}
		return strings.Join(quoted, ", ")
	}
	facts := []string{}
	if len(skill.Uses) > 0 {
		facts = append(facts, "- **Uses:** "+names(skill.Uses))
	}
	if skill.Source == "" {
		return facts
	}
	switch {
	case !skill.Included && len(skill.UsedBy) == 0:
		facts = append(facts, "- **Installed because:** used by a skill the project excludes")
	case !skill.Included:
		facts = append(facts, "- **Installed because:** used by "+names(skill.UsedBy))
	case len(skill.UsedBy) > 0:
		facts = append(facts, "- **Installed because:** included, and used by "+names(skill.UsedBy))
	default:
		facts = append(facts, "- **Installed because:** included")
	}
	return facts
}
