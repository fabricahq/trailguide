// Parse a source's skills.exclude and skills.rename: the skills it leaves out, the authored skills that replace them,
// and the names it installs skills under instead of their own.

package rules

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skills"
)

// SkillExclusion leaves one skill a source would install out of the installed skills, with the project's reason.
type SkillExclusion struct {
	Reason string `json:"reason"`
	// ReplacedBy is empty, or the ID of the authored skill installed in the excluded skill's place, skills/<name>,
	// which is also its folder's path in the Trail Guide directory.
	ReplacedBy string `json:"replacedBy,omitempty"`
	// BasedOn is the commit of the source's repository, or, for a skill the source's library curates, of the
	// repository it curates the skill from, whose copy of the skill the replacement was made from, which
	// updates compare the source's newer revisions with. It is the zero GitRef when the project doesn't record one,
	// and only a replacement has one; a nonzero one is always a full commit SHA. Configuration alone owns it.
	BasedOn GitRef `json:"basedOn,omitzero"`
}

// parseSkillExclusions validates each entry of a source's skills.exclude: its skill ID and reason, and, optionally,
// the ID of the authored skill that replaces it and the commit the replacement is based on. A missing field is no
// exclusions. Whether the source installs each skill, and whether the replacement exists, are checked when skills
// resolve; whether the repository has the basedOn commit, when sync reads the source.
func parseSkillExclusions(input json.RawMessage, location string) (map[string]SkillExclusion, error) {
	result := map[string]SkillExclusion{}
	if input == nil {
		return result, nil
	}
	ids, entries, err := listEntries(input, location, func(id, where string) error {
		switch id {
		case skills.Wildcard:
			return invalid(where, `an exclusion names one skill by ID, such as skills/build-feature, not "*"; to install none of the source's skills, remove skills.include`)
		case skills.Recommended:
			return invalid(where, "an exclusion names one skill by ID, such as skills/build-feature, not recommended; to install none of the recommended skills, remove recommended from skills.include")
		}
		return skills.ValidateID(id, where)
	})
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		where := location + "." + id
		fields := entries[id]
		if err := decode.KnownFields(fields, []string{"reason", "replacedBy", "basedOn"}, where); err != nil {
			return nil, err
		}
		reason, err := decode.Text(fields["reason"], where+".reason")
		if err != nil {
			return nil, err
		}
		exclusion := SkillExclusion{Reason: reason}
		if raw, ok := fields["replacedBy"]; ok {
			if exclusion.ReplacedBy, err = parseSkillReplacement(raw, where+".replacedBy"); err != nil {
				return nil, err
			}
		}
		if raw, ok := fields["basedOn"]; ok {
			if exclusion.ReplacedBy == "" {
				return nil, invalid(where+".basedOn", "basedOn records the commit a replacement was copied from, so it needs replacedBy")
			}
			if exclusion.BasedOn, err = parseBasedOnCommit(raw, where+".basedOn"); err != nil {
				return nil, err
			}
		}
		result[id] = exclusion
	}
	return result, nil
}

// parseSkillReplacement reads replacedBy, the ID of an authored skill, skills/<name>, explaining how to write a
// value that names the skill's folder, its SKILL.md, or its name instead.
func parseSkillReplacement(raw json.RawMessage, location string) (string, error) {
	text, err := decode.Text(raw, location)
	if err != nil {
		return "", err
	}
	name, isID := strings.CutPrefix(text, skills.Directory+"/")
	if isID && skills.ValidName(name) {
		return text, nil
	}
	named := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(librarytree.Upgrade(text), librarytree.Directory+"/"), "/"+skills.File), "/")
	if bare := strings.TrimPrefix(named, skills.Directory+"/"); skills.ValidName(bare) {
		return "", invalid(location, "replacedBy names the authored skill by its ID, "+skills.AuthoredID(bare)+", not "+decode.Quote(text))
	}
	return "", invalid(location, "expected the ID of an authored skill in .trailguide/skills/, such as skills/build-feature, got "+decode.Quote(text))
}

// parseBasedOnCommit reads a skill replacement's basedOn: a full commit SHA, in quotes.
func parseBasedOnCommit(raw json.RawMessage, location string) (GitRef, error) {
	const expected = "expected the full 40-character commit SHA the replacement was copied from, in quotes"
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return GitRef{}, invalid(location, expected)
	}
	ref, err := ParseGitRef(text, location)
	if err != nil || ref.Kind() != GitRefCommit {
		return GitRef{}, invalid(location, expected+", got "+decode.Quote(text))
	}
	return ref, nil
}

// parseSkillRenames validates a source's skills.rename, a mapping of skill IDs to the names the project installs
// them under instead of their own, and returns it, or nil when it renames nothing.
func parseSkillRenames(input json.RawMessage, location string) (map[string]string, error) {
	if input == nil {
		return nil, nil
	}
	fields, err := decode.Object(input, location)
	if err != nil {
		return nil, invalid(location, "expected a mapping of skill IDs to new names, such as skills/review: team-review")
	}
	var result map[string]string
	for _, id := range slices.Sorted(maps.Keys(fields)) {
		where := location + "." + id
		switch id {
		case skills.Wildcard, "recommended":
			return nil, invalid(where, "a rename names one skill by ID, such as skills/review, and gives it a new name")
		}
		if err := skills.ValidateID(id, where); err != nil {
			return nil, err
		}
		var name string
		if json.Unmarshal(fields[id], &name) != nil || !skills.ValidName(name) {
			return nil, invalid(where, "expected the name to install the skill under, which follows the Agent Skills name grammar: 1 to 64 lowercase letters, digits, and hyphens, without a hyphen at either end or two in a row, such as team-review")
		}
		if result == nil {
			result = map[string]string{}
		}
		result[id] = name
	}
	return result, nil
}
