// Parse a source's skills.exclude, skills.forks, and skills.rename: the skills it leaves out, the authored skills it
// installs in place of its skills, and the names it installs skills under instead of their own.

package rules

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skills"
)

// SkillExclusion leaves one skill a source would install out of the installed skills, with the project's reason.
type SkillExclusion struct {
	Reason string `json:"reason"`
}

// parseSkillExclusions validates each entry of a source's skills.exclude, its skill ID and reason, and returns them
// by ID. An entry whose id is recommended or "*" is a level exclusion instead, returned in levels with its reason: it
// leaves out every skill of those levels that the source isn't required to install. A missing field is no
// exclusions. An entry with replacedBy or basedOn, which recorded a fork before skills.forks did, fails with the
// skills.forks entry to write instead. Whether the source installs each skill is checked when skills resolve.
func parseSkillExclusions(input json.RawMessage, location string) (result map[string]SkillExclusion, levels map[string]string, err error) {
	result, levels = map[string]SkillExclusion{}, map[string]string{}
	if input == nil {
		return result, levels, nil
	}
	ids, entries, err := listEntries(input, location, func(id, where string) error {
		if id == skills.Wildcard || id == skills.Recommended {
			return nil
		}
		return skills.ValidateID(id, where)
	})
	if err != nil {
		return nil, nil, err
	}
	for _, id := range ids {
		where := location + "." + id
		fields := entries[id]
		if id == skills.Wildcard || id == skills.Recommended {
			if err := decode.KnownFields(fields, []string{"reason"}, where); err != nil {
				return nil, nil, invalid(where, "an exclusion of "+decode.Quote(id)+" leaves out skills by level, so it takes only a reason; to replace one skill with your own, list it in skills.forks")
			}
			if levels[id], err = decode.Text(fields["reason"], where+".reason"); err != nil {
				return nil, nil, err
			}
			continue
		}
		if err := forkInExclusion(fields, id, where); err != nil {
			return nil, nil, err
		}
		if err := decode.KnownFields(fields, []string{"reason"}, where); err != nil {
			return nil, nil, err
		}
		reason, err := decode.Text(fields["reason"], where+".reason")
		if err != nil {
			return nil, nil, err
		}
		result[id] = SkillExclusion{Reason: reason}
	}
	return result, levels, nil
}

// forkInExclusion fails when fields, those of the skills.exclude entry of skill id at where, have replacedBy or
// basedOn, which recorded a fork before skills.forks did, showing the skills.forks entry that records it now.
func forkInExclusion(fields map[string]json.RawMessage, id, where string) error {
	_, replaced := fields["replacedBy"]
	_, based := fields["basedOn"]
	if !replaced && !based {
		return nil
	}
	as := "skills/<name>"
	if text, err := decode.Text(fields["replacedBy"], where+".replacedBy"); err == nil {
		as = text
	}
	entry := "  " + id + ":\n    as: " + as + "\n"
	var commit string
	if json.Unmarshal(fields["basedOn"], &commit) == nil && commit != "" {
		entry += "    basedOn: " + strconv.Quote(commit) + "\n"
	}
	reason := "..."
	if text, err := decode.Text(fields["reason"], where+".reason"); err == nil {
		reason = text
	}
	entry += "    reason: " + reason
	return invalid(where, "skills.exclude no longer records forks, so an exclusion takes only a reason. Move the entry to the source's skills.forks, which needs no skills.include entry for the skill, and remove it from skills.include if it's there only for the fork:\n\nforks:\n"+entry)
}

// parseSkillReplacement reads an authored skill's ID, skills/<name>, at location, explaining how to write a value
// that names the skill's folder, its SKILL.md, or its name instead.
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
		return "", invalid(location, "name the authored skill by its ID, "+skills.AuthoredID(bare)+", not "+decode.Quote(text))
	}
	return "", invalid(location, "expected the ID of an authored skill in .trailguide/skills/, such as skills/build-feature, got "+decode.Quote(text))
}

// parseBasedOnCommit reads a skill fork's basedOn: a full commit SHA, in quotes.
func parseBasedOnCommit(raw json.RawMessage, location string) (GitRef, error) {
	const expected = "expected the full 40-character commit SHA the fork was copied from, in quotes"
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

// SkillFork installs an authored skill in place of a source's skill, which the source supplies for comparison but
// doesn't install.
type SkillFork struct {
	// As is the ID of the authored skill installed in the forked skill's place, skills/<name>, which is also its
	// folder's path in the Trail Guide directory.
	As     string `json:"as"`
	Reason string `json:"reason"`
	// BasedOn is the commit of the source's repository, or, for a skill the source's library curates, of the
	// repository it curates the skill from, whose copy of the skill the fork was made from, which updates and
	// library check compare the source's newer revisions with. It is the zero GitRef when the configuration doesn't
	// record one, such as for a replacement written from scratch; a nonzero one is always a full commit SHA.
	// Configuration alone owns it.
	BasedOn GitRef `json:"basedOn,omitzero"`
}

// parseSkillForks validates a source's skills.forks, a mapping of the IDs of the source's skills to their forks, each
// with the authored skill's ID in as, a reason, and optionally the commit the fork is based on, and returns them by
// skill ID. Two forks can't share one authored skill. A missing field is no forks. Whether the source has each skill,
// and whether the authored skill exists, are checked when skills resolve; whether the repository has the basedOn
// commit, when sync reads the source.
func parseSkillForks(input json.RawMessage, location string) (map[string]SkillFork, error) {
	result := map[string]SkillFork{}
	if input == nil {
		return result, nil
	}
	entries, err := decode.Object(input, location)
	if err != nil {
		return nil, invalid(location, "expected a mapping of skill IDs to forks, such as skills/fix-bug: {as: skills/fix-bug, reason: ...}")
	}
	forkOf := map[string]string{}
	for _, id := range slices.Sorted(maps.Keys(entries)) {
		where := location + "." + id
		if id == skills.Wildcard || id == skills.Recommended {
			return nil, invalid(where, "a fork names one skill by ID, such as skills/fix-bug")
		}
		if err := skills.ValidateID(id, where); err != nil {
			return nil, err
		}
		fields, err := decode.Object(entries[id], where)
		if err != nil {
			return nil, err
		}
		if err := decode.KnownFields(fields, []string{"as", "reason", "basedOn"}, where); err != nil {
			return nil, err
		}
		var fork SkillFork
		if fields["as"] == nil {
			return nil, invalid(where+".as", "expected the ID of the authored skill installed in this skill's place, such as skills/fix-bug")
		}
		if fork.As, err = parseSkillReplacement(fields["as"], where+".as"); err != nil {
			return nil, err
		}
		if other, taken := forkOf[fork.As]; taken {
			return nil, invalid(where+".as", fmt.Sprintf("%s is already the fork of %s; each authored skill replaces one skill", fork.As, other))
		}
		forkOf[fork.As] = id
		if fork.Reason, err = decode.Text(fields["reason"], where+".reason"); err != nil {
			return nil, err
		}
		if raw, ok := fields["basedOn"]; ok {
			if fork.BasedOn, err = parseBasedOnCommit(raw, where+".basedOn"); err != nil {
				return nil, err
			}
		}
		result[id] = fork
	}
	return result, nil
}
