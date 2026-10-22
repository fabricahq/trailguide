// Register project list skills and project list rules, and render their listings.

package cli

import (
	"fmt"
	"strings"

	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/trailguide/projectview"
	"github.com/spf13/cobra"
)

// projectListCommand groups the project's listings.
func projectListCommand(options Options, output *commandOutput) *cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List what the project installs and could install"}
	skills := &cobra.Command{Use: "skills", Short: "List every installed and available skill, with its level", Args: cobra.NoArgs}
	skills.Long = "List each skill the project's sources offer, whether the project installs it,\nexcludes it, replaces it with its own skill, or could add it, and the level the\nsource's library gives it: required, recommended, or offered. Then list the\nproject's own skills. The listing reads the source records sync wrote, so run\ntrailguide project sync first after changing the configuration." + documentationHelp
	skills.RunE = func(cmd *cobra.Command, _ []string) error {
		directory, err := commandDirectory(cmd.Context(), options.Directory, "project", false)
		if err != nil {
			return err
		}
		listing, err := project.ListSkills(cmd.Context(), project.Options{Directory: directory, ToolVersion: options.Version})
		if err != nil {
			return err
		}
		output.report = commandReport{value: listing, human: formatSkillListing(listing)}
		return nil
	}
	rulesCommand := &cobra.Command{Use: "rules", Short: "List every rule the project holds, with its version and status", Args: cobra.NoArgs}
	rulesCommand.Long = "List each rule the project holds of its sources, with its version, the library release\nit comes from, and its status: active, pinned, excluded, replaced by an authored\nrule, or retired by its library. Then list the project's authored rules. The\nlisting reads .trailguide/config.yaml and .trailguide/lock.json, so run\ntrailguide project sync first after changing the configuration." + documentationHelp
	rulesCommand.RunE = func(cmd *cobra.Command, _ []string) error {
		directory, err := commandDirectory(cmd.Context(), options.Directory, "project", false)
		if err != nil {
			return err
		}
		listing, err := project.ListRules(cmd.Context(), project.Options{Directory: directory, ToolVersion: options.Version})
		if err != nil {
			return err
		}
		output.report = commandReport{value: listing, human: formatRuleListing(listing)}
		return nil
	}
	list.AddCommand(skills, rulesCommand)
	return list
}

// formatRuleListing renders the listing as aligned columns: the rule, its version, where it comes from, and its
// status.
func formatRuleListing(listing project.RuleListing) string {
	if len(listing.Rules) == 0 {
		return "The project holds no rules.\n"
	}
	rows := [][]string{{"RULE", "VERSION", "FROM", "STATUS"}}
	for _, rule := range listing.Rules {
		version, from := "-", "this project"
		if rule.Version != nil {
			version = rule.Version.String()
		}
		if rule.Source != "" {
			from = rule.Source + ", commit " + shortCommit(rule.Commit)
			if rule.Release != nil {
				from = fmt.Sprintf("%s, release %d", rule.Source, *rule.Release)
			}
		}
		rows = append(rows, []string{rule.ID, version, from, ruleStatus(rule)})
	}
	return formatColumns(rows)
}

// ruleStatus summarizes a listed rule's fields for people, such as "active, pinned at 1.0.0".
func ruleStatus(rule projectview.Rule) string {
	var parts []string
	switch {
	case rule.ReplacedBy != "":
		parts = append(parts, "replaced by "+rule.ReplacedBy)
	case rule.Exclusion != nil:
		parts = append(parts, "excluded")
	default:
		parts = append(parts, "active")
	}
	if rule.Replaces != nil {
		parts = append(parts, "replaces "+rule.Replaces.ID)
	}
	if rule.Pin != nil {
		parts = append(parts, "pinned at "+rule.Pin.Version.String())
	}
	if rule.Retired {
		parts = append(parts, "retired")
	}
	return strings.Join(parts, ", ")
}

// formatSkillListing renders the listing as aligned columns: the skill, its level, the library release that set
// it, and its status.
func formatSkillListing(listing project.SkillListing) string {
	if len(listing.Skills) == 0 {
		return "The project has no skills, and its sources offer none.\n"
	}
	rows := [][]string{{"SKILL", "LEVEL", "SET BY", "STATUS"}}
	for _, skill := range listing.Skills {
		level, setBy := "-", "-"
		switch {
		case skill.Level != "":
			level, setBy = skill.Level, skill.SetBy.Source
			if skill.SetBy.Release != 0 {
				setBy += fmt.Sprintf(", release %d", skill.SetBy.Release)
			} else {
				setBy += ", commit " + shortCommit(skill.SetBy.Commit)
			}
		case skill.Source == "":
			setBy = "this project"
		default:
			setBy = skill.Source + ", not a library"
		}
		status := skill.Status
		if skill.ReplacedBy != "" {
			status += " by " + skill.ReplacedBy
		}
		rows = append(rows, []string{skill.ID, level, setBy, status})
	}
	return formatColumns(rows)
}

// formatColumns aligns rows, the first of them a header, in columns two spaces apart.
func formatColumns(rows [][]string) string {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], len(cell))
		}
	}
	var out strings.Builder
	for _, row := range rows {
		line := ""
		for i, cell := range row {
			if i < len(row)-1 {
				line += fmt.Sprintf("%-*s  ", widths[i], cell)
			} else {
				line += cell
			}
		}
		out.WriteString(line + "\n")
	}
	return out.String()
}
