// Parse a library change note from changes/ without accessing files.

package rules

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// ChangeNote records one change to one or more rules and skills for the next library release.
type ChangeNote struct {
	// Summary has no surrounding whitespace.
	Summary string `json:"summary"`
	// Rules maps library rule IDs, such as practices/testing/verify-retry-limits, to their change, and Skills maps
	// skill IDs, such as skills/build-feature, to theirs. Neither is nil, and at least one isn't empty.
	Rules  map[string]NoteChange `json:"rules"`
	Skills map[string]NoteChange `json:"skills"`
}

// NoteChange is one rule's or skill's change in a note. ReplacedBy is set only for a retired one that has a
// replacement, which is a rule for a rule and a skill for a skill.
type NoteChange struct {
	Change     trailguide.Change `json:"change"`
	ReplacedBy string            `json:"replacedBy,omitempty"`
}

// ParseChangeNote validates one change note's YAML. location names the note, such as changes/2026-09-29-verify-retry-limits-7f3a9c.yaml.
// Rule and skill IDs are checked for syntax only; whether each exists, and whether its change matches the library,
// is a library check.
func ParseChangeNote(input []byte, location string) (ChangeNote, error) {
	_, data, err := decode.YAML(input, location)
	if err != nil {
		return ChangeNote{}, err
	}
	fields, err := decode.Object(data, location)
	if err != nil {
		return ChangeNote{}, err
	}
	if err := decode.KnownFields(fields, []string{"summary", "rules", "skills"}, location); err != nil {
		return ChangeNote{}, err
	}
	summary, err := decode.Line(fields["summary"], location+".summary")
	if err != nil {
		return ChangeNote{}, err
	}
	note := ChangeNote{Summary: summary, Rules: map[string]NoteChange{}, Skills: map[string]NoteChange{}}
	_, hasRules := fields["rules"]
	_, hasSkills := fields["skills"]
	if !hasRules && !hasSkills {
		return ChangeNote{}, invalid(location, "expected rules or skills, naming what the note changes")
	}
	if hasRules {
		if note.Rules, err = noteChanges(fields["rules"], noteRules, location+".rules"); err != nil {
			return ChangeNote{}, err
		}
	}
	if hasSkills {
		if note.Skills, err = noteChanges(fields["skills"], noteSkills, location+".skills"); err != nil {
			return ChangeNote{}, err
		}
	}
	return note, nil
}

// noteKind is what a note's IDs of one kind, rules or skills, name, and how to check one.
type noteKind struct {
	noun     string
	validate func(id, location string) error
}

var (
	noteRules  = noteKind{noun: "rule", validate: librarytree.ValidateRuleID}
	noteSkills = noteKind{noun: "skill", validate: ValidateSkillID}
)

// ValidateSkillID accepts the ID of a library's own skill, skills/<name>, whose name follows the skill name grammar.
func ValidateSkillID(id, location string) error {
	if name, found := strings.CutPrefix(id, skills.Directory+"/"); found && skills.ValidName(name) {
		return nil
	}
	return invalid(location, "expected a skill ID, such as skills/build-feature, got "+decode.Quote(id))
}

// noteChanges reads a note's non-empty map of rule, or skill, IDs to their changes.
func noteChanges(input json.RawMessage, kind noteKind, location string) (map[string]NoteChange, error) {
	entries, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, invalid(location, "expected at least one "+kind.noun)
	}
	changes := map[string]NoteChange{}
	for _, id := range slices.Sorted(maps.Keys(entries)) {
		entryLocation := location + "." + id
		if err := kind.validate(id, entryLocation); err != nil {
			return nil, err
		}
		change, err := noteChange(entries[id], id, kind, entryLocation)
		if err != nil {
			return nil, err
		}
		changes[id] = change
	}
	return changes, nil
}

// noteChange accepts a change name, or an object that retires the rule, or skill, with an optional replacement of
// the same kind.
func noteChange(input json.RawMessage, id string, kind noteKind, location string) (NoteChange, error) {
	var name string
	if json.Unmarshal(input, &name) == nil {
		switch change := trailguide.Change(name); change {
		case trailguide.ChangeMajor, trailguide.ChangeMinor, trailguide.ChangePatch, trailguide.ChangeNew, trailguide.ChangeRetired:
			return NoteChange{Change: change}, nil
		}
		return NoteChange{}, invalid(location, "unknown change "+decode.Quote(name)+"; expected major, minor, patch, new, or retired")
	}
	fields, err := decode.Object(input, location)
	if err != nil {
		return NoteChange{}, invalid(location, "expected major, minor, patch, new, retired, or an object with change: retired")
	}
	if err := decode.KnownFields(fields, []string{"change", "replacedBy"}, location); err != nil {
		return NoteChange{}, err
	}
	if json.Unmarshal(fields["change"], &name) != nil || trailguide.Change(name) != trailguide.ChangeRetired {
		return NoteChange{}, invalid(location+".change", "expected retired; only a retirement can name a replacement")
	}
	result := NoteChange{Change: trailguide.ChangeRetired}
	if raw, ok := fields["replacedBy"]; ok {
		if json.Unmarshal(raw, &result.ReplacedBy) != nil {
			return NoteChange{}, invalid(location+".replacedBy", "expected a "+kind.noun+" ID")
		}
		if err := kind.validate(result.ReplacedBy, location+".replacedBy"); err != nil {
			return NoteChange{}, err
		}
		if result.ReplacedBy == id {
			return NoteChange{}, invalid(location+".replacedBy", "a "+kind.noun+" can't replace itself")
		}
	}
	return result, nil
}
