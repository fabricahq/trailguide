// List every skill the project installs or could install from its sources, with the level its library gives it.

package project

import (
	"context"
	"maps"
	"slices"

	"github.com/fabricahq/trailguide/internal/library"
)

// The statuses a listed skill can have.
const (
	// SkillInstalled is a skill the project installs.
	SkillInstalled = "installed"
	// SkillAvailable is a source's skill the project doesn't include, so it could add it.
	SkillAvailable = "available"
	// SkillExcluded is a source's skill the project excludes, and SkillReplaced one it excludes and installs an
	// authored skill in place of.
	SkillExcluded = "excluded"
	SkillReplaced = "replaced"
)

// SkillListing lists each skill of each source, in source order and then by ID, then the project's authored skills.
type SkillListing struct {
	Skills []ListedSkill `json:"skills"`
}

// ListedSkill is one skill of a SkillListing.
type ListedSkill struct {
	// ID is the project ID: source:folder for a source's skill, or skills/<name> for an authored one.
	ID string `json:"id"`
	// Source names the source that supplies the skill, or is empty for an authored skill.
	Source string `json:"source,omitempty"`
	// Level is the skill's level in its library, or empty for an authored skill or one from a repository that isn't
	// a library, which is neither required nor recommended.
	Level string `json:"level,omitempty"`
	// SetBy names the library that set Level, or is nil when Level is empty.
	SetBy *LevelSetter `json:"setBy,omitempty"`
	// Status is SkillInstalled, SkillAvailable, SkillExcluded, or SkillReplaced.
	Status string `json:"status"`
	// ReplacedBy is the authored skill installed in place of a replaced skill, or empty.
	ReplacedBy string `json:"replacedBy,omitempty"`
}

// LevelSetter names the library, by source name, that set a skill's level, and the revision whose manifest set it:
// its library release, or 0 for a revision other than a library release, and its commit.
type LevelSetter struct {
	Source  string `json:"source"`
	Release int    `json:"release,omitempty"`
	Commit  string `json:"commit"`
}

// ListSkills lists the skills of the project's sources, as their records list them, including curated skills
// installed only because other curated skills use them, with no level, and its authored skills, without fetching
// sources or writing files. It fails, as build does, when a source record doesn't match the configuration, so the
// listing never describes a configuration sync hasn't applied.
func ListSkills(ctx context.Context, options Options) (SkillListing, error) {
	if err := ctx.Err(); err != nil {
		return SkillListing{}, err
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		return SkillListing{}, err
	}
	defer root.Close()
	state, err := readProject(ctx, root)
	if err != nil {
		return SkillListing{}, err
	}
	snapshots, err := decodeSnapshots(state.config, treeFiles(state.vendor))
	if err != nil {
		return SkillListing{}, err
	}
	authored, err := authoredSkills(state.skills)
	if err != nil {
		return SkillListing{}, err
	}
	listing := SkillListing{Skills: []ListedSkill{}}
	for _, source := range state.config.Sources {
		snapshot := snapshots[source.Name]
		// A curated skill installed only because other curated skills use it isn't one the library lists, so the
		// library sets no level for it.
		levels := maps.Clone(snapshot.AvailableSkills)
		for id := range snapshot.Skills {
			if _, listed := levels[id]; !listed && isCuratedSkill(id) {
				levels[id] = library.ImportedSkill{}
			}
		}
		for _, id := range slices.Sorted(maps.Keys(levels)) {
			skill := ListedSkill{ID: source.Name + ":" + id, Source: source.Name, Level: levels[id].Level, Status: SkillAvailable}
			if skill.Level != "" {
				skill.SetBy = &LevelSetter{Source: source.Name, Release: snapshot.Release, Commit: snapshot.Commit}
			}
			fork, forked := source.SkillForks[id]
			_, excluded := source.SkillExclude[id]
			_, installed := snapshot.Skills[id]
			switch {
			case forked:
				skill.Status, skill.ReplacedBy = SkillReplaced, fork.As
			case excluded:
				skill.Status = SkillExcluded
			case installed:
				skill.Status = SkillInstalled
			case source.ExcludesLevel(id, levels[id].Level):
				skill.Status = SkillExcluded
			}
			listing.Skills = append(listing.Skills, skill)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(authored)) {
		listing.Skills = append(listing.Skills, ListedSkill{ID: id, Status: SkillInstalled})
	}
	return listing, ctx.Err()
}
