// Interpret source inputs independently of the project configuration's storage format.

package project

import (
	"encoding/json"
	"strings"

	"github.com/fabricahq/trailguide/internal/rules"
)

// SourceInput selects a repository, the rules and skills to include from it, and an optional exact ref. Rules holds
// rule selectors for rules.include: rule IDs, group prefixes such as techs/go/*, and the wildcards *, techs/*, and
// practices/*. Skills holds skill selectors for skills.include: skill IDs, such as skills/build-feature, and
// recommended, or only *. With neither, the source installs only its library's required skills. An empty Ref means
// the source follows rule versions. Pins and exclusions are configured separately.
type SourceInput struct {
	Repository string
	Ref        string
	Rules      []string
	Skills     []string
}

// ParseSourceRef returns the branch, tag, or full commit SHA, without surrounding whitespace, as a validated ref.
// Abbreviated commits and version ranges are rejected.
func ParseSourceRef(input string) (rules.GitRef, error) {
	return rules.ParseGitRef(strings.TrimSpace(input), "--ref")
}

// ParseSourceRules validates rule selectors, as --rules supplies them, and returns the groups and individual rules
// they select; no selectors select nothing.
func ParseSourceRules(selectors []string) (rules.GroupSelection, []string, error) {
	data, err := json.Marshal(append([]string{}, selectors...))
	if err != nil {
		return rules.GroupSelection{}, nil, err
	}
	return rules.ParseRuleSelection(data, "--rules")
}

// ParseSourceSkills validates skill selectors, as --skills supplies them, and returns them sorted; no selectors
// select nothing.
func ParseSourceSkills(selectors []string) ([]string, error) {
	if len(selectors) == 0 {
		return []string{}, nil
	}
	data, err := json.Marshal(selectors)
	if err != nil {
		return nil, err
	}
	return rules.ParseSkillSelection(data, "--skills")
}

func parseSourceInput(input SourceInput) (rules.Source, error) {
	source := rules.Source{Repository: input.Repository}
	var err error
	if input.Ref != "" {
		if source.Ref, err = ParseSourceRef(input.Ref); err != nil {
			return rules.Source{}, err
		}
	}
	if source.Groups, source.Rules, err = ParseSourceRules(input.Rules); err != nil {
		return rules.Source{}, err
	}
	if source.Skills, err = ParseSourceSkills(input.Skills); err != nil {
		return rules.Source{}, err
	}
	return source, nil
}
