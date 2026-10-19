// Write the SKILL.md of a new authored skill: its frontmatter, and a body marked as an unfinished draft.

package skills

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// MaxDescription is the most characters the Agent Skills format allows in a skill's description.
const MaxDescription = 1024

// ValidateDescription returns what is wrong with description, a new skill's description, or nil: it must not be
// blank, and may have at most MaxDescription characters.
func ValidateDescription(description string) error {
	if strings.TrimSpace(description) == "" {
		return fmt.Errorf("the description is blank; say what the skill does and when to use it")
	}
	if count := utf8.RuneCountInString(description); count > MaxDescription {
		return fmt.Errorf("the description is %d characters; use at most %d", count, MaxDescription)
	}
	return nil
}

// ValidateType returns what is wrong with kind, a new skill's trailguide-skill-type, or nil when it is one of Types or
// empty, which leaves the type out.
func ValidateType(kind string) error {
	if kind != "" && !slices.Contains(Types, kind) {
		return fmt.Errorf("the type is %q; use %s, or leave it out", kind, strings.Join(Types, " or "))
	}
	return nil
}

// draftBody follows the frontmatter of a new skill's SKILL.md, after a blank line. Its first line is draftMarker, which
// library check and build reject until the author removes it.
const draftBody = `
# <Title of the task the skill carries out>

<Say in a sentence or two what the skill does, and what the agent should have when it finishes.>

## 1. <First step>

<Give the step one outcome the agent can check before moving on. Name each rule the step applies by its ID, and list
those IDs in metadata.trailguide-rules-used, one per line in a block string.>

## 2. <Next step>

<Say what to do when a step can't be completed, instead of leaving it to the agent.>

## Report

<Say what the agent hands back: what it changed, what it verified, and what it couldn't.>
`

// Draft returns the SKILL.md of a new authored skill named name, which must follow the name grammar, with
// description and kind, a trailguide-skill-type or empty to leave it out, in its frontmatter, and a body that starts
// with marker, the draft marker, then placeholders for the skill's steps. description and kind must pass
// ValidateDescription and ValidateType.
func Draft(name, description, kind, marker string) ([]byte, error) {
	frontmatter := struct {
		Name        string            `yaml:"name"`
		Description string            `yaml:"description"`
		Metadata    map[string]string `yaml:"metadata,omitempty"`
	}{Name: name, Description: description}
	if kind != "" {
		frontmatter.Metadata = map[string]string{TypeKey: kind}
	}
	header, err := yaml.Dump(frontmatter, yaml.WithV3Defaults(), yaml.WithIndent(2), yaml.WithLineWidth(-1))
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(header) + "---\n\n" + marker + "\n" + draftBody), nil
}
