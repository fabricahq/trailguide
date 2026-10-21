// Parse a library's skill levels: which of its skills projects that use it must, should, or may install, and which
// install only with the skills that use them.

package rules

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
// manifest names none, and a skill is in at most one list. Whether each own skill exists is checked against the library's skills/
// folder, and whether each curated one exists when a project imports it.
type SkillLevels struct {
	Required     []string `json:"required"`
	Recommended  []string `json:"recommended"`
	Offered      []string `json:"offered"`
	Dependencies []string `json:"dependencies"`
}

// SkillLists names the lists of library.skills, the levels from the strongest to the weakest, then the dependencies.
var SkillLists = append(slices.Clone(skills.Levels), skills.Dependencies)

// Level returns the level of the library's skill id: the level whose list names it, skills.LevelDependency for one
// Dependencies names, or offered, the level of every own skill no list names. A curated skill no list names doesn't
// reach projects; see Curated.
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

// parseSkillLevels validates library.skills at location: an object of required, recommended, offered, and dependencies
// lists of skill IDs, each listed once, each curated one from one of sources, the names the configuration's sources
// declare. A missing object lists none.
func parseSkillLevels(input json.RawMessage, location string, sources []string) (SkillLevels, error) {
	result := SkillLevels{Required: []string{}, Recommended: []string{}, Offered: []string{}, Dependencies: []string{}}
	if input == nil {
		return result, nil
	}
	fields, err := decode.Object(input, location)
	if err != nil {
		return SkillLevels{}, err
	}
	if err := decode.KnownFields(fields, SkillLists, location); err != nil {
		return SkillLevels{}, err
	}
	seen := map[string]string{}
	for _, level := range SkillLists {
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

// RequiredSkillProblem explains why a project can't exclude or fork skill id, which the library that source names
// requires in release, or 0 for a revision other than a library release.
func RequiredSkillProblem(source, id string, release int) string {
	at := ""
	if release != 0 {
		at = fmt.Sprintf(" in release %d", release)
	}
	return fmt.Sprintf("%s's library requires %s%s, so every project that uses the library installs it, and the project can't exclude or fork it. To diverge from it, stop using sources.%s, or ask the library's maintainer to make the skill recommended or offered", source, id, at, source)
}

// LibraryRequiresProblem explains why the repository can't exclude skill id of source, which its own library section
// requires, so the projects that use the library install it.
func LibraryRequiresProblem(source, id string) string {
	return fmt.Sprintf("library.skills.required lists %s:%s, so every project that uses this library installs it, and so does this repository; stop excluding it, or remove it from library.skills.required", source, id)
}
