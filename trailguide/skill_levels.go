// Parse a library's skill levels: which of its skills projects that use it must, should, or may install, and which
// install only with the skills that use them.

package trailguide

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/skills"
)

// SkillLevels lists, sorted, the IDs of the skills at each level the manifest names: the library's own skills,
// skills/<name>, and the skills it curates, the name of a source in its sources, a colon, and the skill's ID in that
// source's repository, such as mattpocock:skills/to-spec. Dependencies lists the skills the library has only because
// its other skills use them, which projects install only with those skills. Each list is empty, never nil, when the
// manifest names none, and a skill is in at most one list. Whether each own skill exists is checked against the
// library's skills/ folder, and whether each curated one exists when a project imports it.
type SkillLevels struct {
	Required     []string `json:"required"`
	Recommended  []string `json:"recommended"`
	Offered      []string `json:"offered"`
	Dependencies []string `json:"dependencies"`
}

// Level returns the level of the library's skill id, required, recommended, or offered: the level whose list names
// it, dependency for one Dependencies names, or offered, the level of every own skill no list names. A curated skill
// no list names doesn't reach projects; see Curated.
func (l SkillLevels) Level(id string) string {
	switch {
	case slices.Contains(l.Required, id):
		return skills.LevelRequired
	case slices.Contains(l.Recommended, id):
		return skills.LevelRecommended
	case slices.Contains(l.Dependencies, id):
		return skills.LevelDependency
	}
	return skills.LevelOffered
}

// Field returns the field of the manifest that gives skill id its level, such as library.skills.required, or
// library.skills.dependencies for a dependency.
func (l SkillLevels) Field(id string) string {
	if l.Level(id) == skills.LevelDependency {
		return "library.skills." + skills.Dependencies
	}
	return "library.skills." + l.Level(id)
}

// Listed returns, sorted, the IDs of every skill a list names.
func (l SkillLevels) Listed() []string {
	listed := slices.Concat(l.Required, l.Recommended, l.Offered, l.Dependencies)
	slices.Sort(listed)
	return listed
}

// Curated returns, sorted, the IDs of the curated skills the lists name. Only these curated skills reach projects.
func (l SkillLevels) Curated() []string {
	return slices.DeleteFunc(l.Listed(), func(id string) bool { return !strings.Contains(id, ":") })
}

// parseSkillLevels validates library.skills at location: an object of required, recommended, offered, and dependencies
// lists of skill IDs, each listed once, each curated one from one of sources, the names the configuration's sources
// declare. A missing object lists none. With strict, unknown fields are rejected; otherwise they're ignored.
func parseSkillLevels(input json.RawMessage, location string, sources []string, strict bool) (SkillLevels, error) {
	result := SkillLevels{Required: []string{}, Recommended: []string{}, Offered: []string{}, Dependencies: []string{}}
	if input == nil {
		return result, nil
	}
	fields, err := decode.Object(input, location)
	if err != nil {
		return SkillLevels{}, err
	}
	if strict {
		if err := decode.KnownFields(fields, skills.Lists, location); err != nil {
			return SkillLevels{}, err
		}
	}
	seen := map[string]string{}
	for _, level := range skills.Lists {
		raw, ok := fields[level]
		if !ok {
			continue
		}
		where := location + "." + level
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return SkillLevels{}, invalid(where, "expected a list of the library's skill IDs, such as [skills/build-feature]")
		}
		ids := []string{}
		for i, item := range items {
			at := fmt.Sprintf("%s[%d]", where, i)
			id, err := decode.Text(item, at)
			if err != nil {
				return SkillLevels{}, err
			}
			if _, _, curated := strings.Cut(id, ":"); curated {
				if err := validateCuratedSkill(id, at, sources); err != nil {
					return SkillLevels{}, err
				}
			} else if name, authored := strings.CutPrefix(id, skills.Directory+"/"); !authored || !skills.ValidName(name) {
				return SkillLevels{}, invalid(at, "expected the ID of one of the library's skills, skills/<name>, the folder's path in .trailguide/, such as skills/build-feature, got "+decode.Quote(id))
			}
			if other, listed := seen[id]; listed {
				switch {
				case other == level:
					return SkillLevels{}, invalid(at, id+" is listed more than once; list each skill once")
				case level == skills.Dependencies:
					return SkillLevels{}, invalid(at, id+" is already "+other+", so projects can choose it; a dependency installs only with the skills that use it, so list it in one place")
				}
				return SkillLevels{}, invalid(at, id+" is already "+other+"; give each skill one level")
			}
			seen[id] = level
			ids = append(ids, id)
		}
		slices.Sort(ids)
		switch level {
		case skills.LevelRequired:
			result.Required = ids
		case skills.LevelRecommended:
			result.Recommended = ids
		case skills.Dependencies:
			result.Dependencies = ids
		default:
			result.Offered = ids
		}
	}
	return result, nil
}

// validateCuratedSkill checks that id, listed at location, names a curated skill: the name of one of sources, a
// colon, and the skill's ID in that source's repository, such as mattpocock:skills/to-spec.
func validateCuratedSkill(id, location string, sources []string) error {
	source, skill, _ := strings.Cut(id, ":")
	if !skills.ValidSourceName(source) {
		return invalid(location, "expected a curated skill as the name of a source in sources, a colon, and the skill's ID in that source's repository, such as mattpocock:skills/to-spec, got "+decode.Quote(id))
	}
	if err := skills.ValidateID(skill, location); err != nil {
		return err
	}
	if !slices.Contains(sources, source) {
		return invalid(location, id+" comes from "+source+", which isn't a source in sources; add sources."+source+" with the repository the skill comes from, and a ref unless the repository publishes library releases")
	}
	return nil
}
