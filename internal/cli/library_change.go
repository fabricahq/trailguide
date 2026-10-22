// Collect a library change note's options and prompts, then record it through library.PlanChange.

package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/trailguide"
	"github.com/spf13/cobra"
)

// changeLevels explains each version change the way the rule versions reference defines it.
const changeLevels = `  major  Work that complied with the previous version could fail this one.
  minor  Work that complied still complies, and this version adds new guidance.
  patch  Work that complied still complies, and this version adds no new guidance.`

// skillChangeLevels explains each version change of a skill the way the rule versions reference defines it.
const skillChangeLevels = `  major  Someone relying on the previous version could find this one produces or requires something different.
  minor  The skill adds steps or guidance, and everything that relied on the previous version still works.
  patch  The skill only clarifies its wording.`

// libraryChangeCommand writes a new change note, prompting in a terminal for a missing change level or summary.
func libraryChangeCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("change ID...", "Record rule and skill changes in a new change note", requiredArguments("rule or skill ID", "practices/testing/my-rule --bump minor --summary 'Add a Python example.'", "Name each rule by its path without .md, such as practices/testing/my-rule, and each skill as skills/NAME."), options)
	cmd.Long = cmd.Short + "\n\nAfter the first library release, every rule and skill change needs a change note in\n.trailguide/changes/. The next library release publishes the notes added since the previous one.\nName a rule by its path without .md, such as practices/testing/my-rule, and a skill as skills/NAME.\n\nFor rules and skills that have a version, choose --bump:\n" + changeLevels + "\nA skill's version describes what it produces and requires instead; see the documentation.\nLeave out --bump for a new rule or skill; it starts at version 1.0.0. To retire a rule,\ndelete its Markdown file and asset directory, or a skill's folder, then pass --retire.\n\nWrite the summary for project maintainers deciding whether to update." + documentationHelp
	f.add(cmd, "bump", "Change for rules and skills that have a version: major, minor, or patch")
	f.add(cmd, "summary", "One line describing the change for project maintainers")
	cmd.Flags().Bool("retire", false, "Record that the rules or skills are retired; delete their files first")
	f.add(cmd, "replaced-by", "With --retire and one rule or skill: the rule or skill that replaces it")
	f.prompts = map[string]string{"bump": "Change (major, minor, or patch)", "summary": "Summary"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := f.libraryOptions(cmd.Context(), false)
		if err != nil {
			return err
		}
		retire, _ := cmd.Flags().GetBool("retire")
		if err := changeFlagsUsage(f, args, retire); err != nil {
			return usage(err)
		}
		request := library.ChangeRequest{IDs: args, Bump: trailguide.Change(f.value("bump")), Summary: f.value("summary"), Retire: retire, ReplacedBy: f.value("replaced-by")}
		plan, err := library.PlanChange(cmd.Context(), request, target)
		if err != nil {
			return err
		}
		f.introduction = changeIntroduction(args, plan.Versioned, retire)
		if plan.Versioned {
			if err := f.require("bump"); err != nil {
				return err
			}
		}
		if err := f.require("summary"); err != nil {
			return err
		}
		result, err := plan.Commit(cmd.Context(), trailguide.Change(f.value("bump")), f.value("summary"), time.Now())
		if err != nil {
			return err
		}
		output.report = changeRecordedReport(result, authoringScope{library: true, directory: f.value("directory"), workdir: f.directory})
		return nil
	}
	return cmd
}

// changeFlagsUsage rejects option combinations the command never accepts, whatever the library holds.
func changeFlagsUsage(f *authoringFlags, ids []string, retire bool) error {
	switch bump := f.value("bump"); {
	case bump != "" && validateAnswer("bump", bump) != nil:
		return validateAnswer("bump", bump)
	case bump != "" && retire:
		return fmt.Errorf("--bump isn't accepted with --retire")
	case f.value("replaced-by") != "" && !retire:
		return fmt.Errorf("--replaced-by requires --retire")
	case f.value("replaced-by") != "" && len(ids) != 1:
		return fmt.Errorf("--replaced-by requires a single rule or skill ID")
	case f.value("replaced-by") != "" && isSkillID(f.value("replaced-by")) != isSkillID(ids[0]):
		replacement, retired := kindName(f.value("replaced-by")), kindName(ids[0])
		return fmt.Errorf("--replaced-by %s is a %s, but %s is a %s. A %s can only be replaced by a %s", f.value("replaced-by"), replacement, ids[0], retired, retired, retired)
	case strings.ContainsAny(f.value("summary"), "\r\n"):
		return fmt.Errorf("--summary must be one line")
	}
	return nil
}

// isSkillID reports whether id names a skill, skills/NAME, rather than a rule.
func isSkillID(id string) bool { return strings.HasPrefix(id, "skills/") }

// kindName names what id is: a rule or a skill.
func kindName(id string) string {
	if isSkillID(id) {
		return "skill"
	}
	return "rule"
}

// changeIntroduction orients the author before the first prompt for a change level or summary.
func changeIntroduction(ids []string, versioned, retire bool) string {
	switch {
	case retire:
		return fmt.Sprintf("Recording the retirement of: %s\n\nExplain in the summary why the %s is retired.\n\n", strings.Join(ids, ", "), kindName(ids[0]))
	case versioned && slices.ContainsFunc(ids, isSkillID):
		return fmt.Sprintf("Recording a change to: %s\n\nChoose how much it changed. For a rule:\n%s\nFor a skill:\n%s\nWhen unsure, choose the larger change.\n\nWrite the summary for project maintainers deciding whether to update:\nsay what changed in the obligation, guidance, or steps.\n\n", strings.Join(ids, ", "), changeLevels, skillChangeLevels)
	case versioned:
		return fmt.Sprintf("Recording a change to: %s\n\nChoose how much the rule changed:\n%s\nWhen unsure, choose the larger change.\n\nWrite the summary for project maintainers deciding whether to update:\nsay what changed in the obligation or guidance.\n\n", strings.Join(ids, ", "), changeLevels)
	case slices.ContainsFunc(ids, isSkillID):
		return fmt.Sprintf("Recording new rules or skills: %s\n\nNew rules and skills start at version 1.0.0. Write the summary for project\nmaintainers deciding whether to update: say what each adds.\n\n", strings.Join(ids, ", "))
	}
	return fmt.Sprintf("Recording a new rule: %s\n\nNew rules start at version 1.0.0. Write the summary for project maintainers\ndeciding whether to update: say what the rule adds.\n\n", strings.Join(ids, ", "))
}

// changeRecordedReport lists the new note and how to validate it with the rule or skill change.
func changeRecordedReport(result library.AuthoringResult, scope authoringScope) commandReport {
	var out strings.Builder
	out.WriteString("Change note created.\n")
	formatAuthored(&out, result.Added, result.Changed, result.Warnings, scope.workdir)
	steps := []nextStep{{Instruction: "Next: Commit the note with the change it records, then validate the library:", Commands: []string{scope.command("check")}}}
	return authoredReport(&out, result.Added, result.Changed, result.Warnings, steps)
}
