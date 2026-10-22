// Resolve the rules each skill cites against the project's resolved rules, and render the section an installed
// SKILL.md ends with.

package build

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// citedRule is one rule a skill cites, as the project resolves it. Exactly one of Active, Excluded, Retired, and
// none of them, which means the project doesn't include the rule, describes it, except that Retired can accompany
// Active when a pin keeps a retired rule.
type citedRule struct {
	// ID is the cited rule's project ID, such as team:techs/go/errors, or local:<id> for an authored rule.
	ID string
	// Active is the rule the project applies for the cited one: the rule itself, or the authored rule that
	// replaces it; nil when the project applies neither.
	Active *resolvedRule
	// Excluded reports an exclusion without a replacement, whose reason is Reason.
	Excluded bool
	Reason   string
	// Retired reports that the source's record lists the rule as retired by its library.
	Retired bool
}

// citeRules resolves the rules each skill in resolved cites, in the order its trailguide-rules-used lists them, and returns
// a validation error at the skill's SKILL.md for each rule an authored skill cites that its library retired. A
// library's skill cites rules by their ID in that library; a skill from a repository that isn't a library cites
// none, since the repository has no rules; an authored skill cites the project's authored rules by bare ID and a
// source's rules by project ID. It fails when an authored skill cites an authored rule that doesn't exist, a source
// the configuration doesn't declare, a source that isn't a library, or a rule its source doesn't have. A source's
// snapshot holds only the rules the source includes, so only a rule in a group the source includes in full, whose
// every rule, retired ones too, the snapshot holds, can be known not to exist; a rule in any other group may exist,
// and is described as not included. Cited rules are for reference only: they never join RULES.md or a group page.
func citeRules(resolved *resolution, config rules.Configuration, libraries map[string]Library) ([]*ValidationError, error) {
	active := map[string]*resolvedRule{}
	for _, group := range resolved.Groups {
		for i := range group.Rules {
			rule := &group.Rules[i]
			active[rule.Rule.ID] = rule
			if rule.Upstream != nil {
				active[rule.Upstream.Source+":"+strings.TrimSuffix(rule.Upstream.File, ".md")] = rule
			}
		}
	}
	sources := map[string]rules.Source{}
	for _, source := range config.Sources {
		sources[source.Name] = source
	}
	retired := []*ValidationError{}
	for i := range resolved.Skills {
		skill := &resolved.Skills[i]
		if skill.Source != "" && libraries[skill.Source].Snapshot.NonLibrary {
			continue
		}
		location := skillLocation(*skill)
		for _, line := range skill.RulesUsed {
			citation := skills.ParseCitation(line)
			if skill.Source != "" {
				citation = skills.Citation{Source: skill.Source, ID: line}
			}
			switch _, configured := sources[citation.Source]; {
			case citation.Source == "":
				if active["local:"+citation.ID] == nil {
					return nil, invalid(location, fmt.Sprintf("%s names %s, which isn't a rule in .trailguide/rules/; write the rule, or cite a source's rule with its source name, such as fabrica:%s", skills.RulesUsedKey, citation.ID, citation.ID))
				}
				citation.Source = "local"
			case !configured:
				return nil, invalid(location, fmt.Sprintf("%s names %s, but the configuration has no source named %s; add the source, or correct the source name", skills.RulesUsedKey, citation, citation.Source))
			case libraries[citation.Source].Snapshot.NonLibrary:
				return nil, invalid(location, fmt.Sprintf("%s names %s, but %s isn't a library, so it has no rules to cite", skills.RulesUsedKey, citation, citation.Source))
			}
			cited := citedRule{ID: citation.String(), Active: active[citation.String()]}
			if citation.Source != "local" {
				snapshot := libraries[citation.Source].Snapshot
				cited.Retired = slices.Contains(snapshot.RetiredRules, citation.ID)
				if _, imported := snapshot.Rules[citation.ID]; skill.Source == "" && !imported && !cited.Retired && sources[citation.Source].Groups.Includes(path.Dir(citation.ID)) {
					return nil, invalid(location, fmt.Sprintf("%s names %s, which isn't a rule in %s's library; correct the rule ID, or remove it from %s", skills.RulesUsedKey, citation, citation.Source, skills.RulesUsedKey))
				}
				if exclusion, excluded := sources[citation.Source].Exclusion(citation.ID); excluded && exclusion.ReplacedBy == "" && cited.Active == nil {
					cited.Excluded, cited.Reason = true, exclusion.Reason
				}
			}
			if cited.Retired && skill.Source == "" {
				retired = append(retired, &ValidationError{Location: location, Problem: fmt.Sprintf("%s names %s, which its library retired; remove it from %s, or cite the rule that replaces it", skills.RulesUsedKey, cited.ID, skills.RulesUsedKey)})
			}
			skill.Cited = append(skill.Cited, cited)
		}
	}
	return retired, nil
}

// skillLocation returns where a skill's SKILL.md is, relative to the Trail Guide directory, for diagnostics.
func skillLocation(skill resolvedSkill) string {
	if skill.Source == "" {
		return skill.Folder + "/" + skills.File
	}
	return librarytree.Vendor + "/" + skill.Source + "/" + skill.Folder + "/" + skills.File
}

// citedSection returns the section an installed copy of skill's SKILL.md ends with, linking from installed, the
// installed SKILL.md's path from the repository root, to the project's version of each rule the skill cites; it is
// empty when the skill cites none.
func citedSection(skill resolvedSkill, installed string) string {
	if len(skill.Cited) == 0 {
		return ""
	}
	lines := []string{}
	for _, cited := range skill.Cited {
		lines = append(lines, "- "+citedLine(cited, installed))
	}
	return "## Rules this skill applies\n\nRead each rule in full before the step that applies it. These are the project's versions of the rules this skill cites, for reference: they don't join RULES.md.\n\n" + strings.Join(lines, "\n") + "\n"
}

// citedLine describes one cited rule: a link to the generated file of the rule the project applies for it, or why
// there is none. Links are relative to installed, a path from the repository root.
func citedLine(cited citedRule, installed string) string {
	id := "`" + cited.ID + "`"
	switch {
	case cited.Active != nil:
		label := id
		if cited.Active.Rule.ID != cited.ID {
			label = "`" + cited.Active.Rule.ID + "`, which replaces " + id
		} else if cited.Active.Origin.Version != nil {
			label += " " + cited.Active.Origin.Version.String()
		}
		file := librarytree.Directory + "/" + librarytree.Generated + "/" + rulePath(cited.Active.Rule)
		return label + ": [" + file + "](" + rules.RelativeLink(installed, file) + ")"
	case cited.Retired:
		return id + ": retired by its library, so the project has no version of it"
	case cited.Excluded:
		return id + ": excluded by this project: " + escapeText(cited.Reason)
	}
	return id + ": not included by this project, so it has no version of it"
}

// appendSection returns data with section after it, separated by a blank line, or data unchanged when section is
// empty.
func appendSection(data []byte, section string) []byte {
	if section == "" {
		return data
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text + "\n" + section)
}
