// Register project add skill, which creates a draft skill through project.PlanLocalSkill, or forks a skill a source
// installs through project.PlanSkillFork.

package cli

import (
	"fmt"
	"strings"

	"github.com/fabricahq/trailguide/internal/project"
	"github.com/spf13/cobra"
)

// projectSkillCommand registers project add skill: NAME creates a draft skill, and --from forks one under its name.
func projectSkillCommand(options Options, output *commandOutput) *cobra.Command {
	skill, sf := newAuthoringCommand("skill NAME | --from SOURCE:SKILL", "Create a draft skill, or fork a skill a source installs to replace it with your own", skillArguments("release-checklist"), options.Directory)
	skill.Long = skill.Short + "\n\nWith NAME, writes .trailguide/skills/NAME/SKILL.md with the name, the description,\nand the type in its frontmatter, and a body with the trailguide:draft marker.\nComplete the steps and remove the marker; build fails until you do.\n\nWith --from, copies the skill from vendor/, as the source's last sync recorded it,\nwith every file's executable bit, into .trailguide/skills/<name>/, where <name> is\nthe name agents already know the skill by. The skills and rules its SKILL.md\ncites get the source's name, such as fabrica:skills/tdd, so they still name the\nsource's, and the skills the source's skills.uses adds for it join\ntrailguide-skills-used. config.yaml gets an entry in the source's skills.forks\nthat names the fork with as, records the commit it copied as basedOn, and\nrecords your reason, so the fork installs in place of the source's skill and\ntrailguide project update lists the source's later changes to it." + documentationHelp
	sf.addSkillFlags(skill)
	skill.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return createProjectSkill(cmd, sf, args[0], output)
		}
		if err := refuseOptions(cmd, "a fork, which keeps the source skill's frontmatter", "description", "type"); err != nil {
			return err
		}
		from, err := project.ParseSkillForkSource(sf.value("from"))
		if err != nil {
			return usage(err)
		}
		target, err := sf.options(cmd.Context(), false)
		if err != nil {
			return err
		}
		plan, err := project.PlanSkillFork(cmd.Context(), from, target)
		if err != nil {
			return err
		}
		reason := sf.value("reason")
		if !cmd.Flags().Changed("reason") {
			if !sf.interactive() {
				return usage(fmt.Errorf("--reason is required: the fork replaces %s's %s, and its skills.forks entry records why", from.Source, from.Skill))
			}
			sf.introduction = fmt.Sprintf("Forking %s into:\n  .trailguide/skills/%s/\n\nThe fork replaces the source's skill: config.yaml gets an exclusion that names\nthe fork and records your reason.\n\n", from, plan.Name())
			if reason, err = sf.askValidated("Why use the fork instead of the source's skill?", func(value string) error { return validateAnswer("reason", value) }); err != nil {
				return err
			}
		}
		result, err := plan.Commit(cmd.Context(), reason)
		if err != nil {
			return err
		}
		output.report = skillForkedReport(result, from, plan, sf.directory)
		return nil
	}
	return skill
}

// createProjectSkill writes the draft skill name, validating its inputs before prompting for a missing description.
func createProjectSkill(cmd *cobra.Command, sf *authoringFlags, name string, output *commandOutput) error {
	kind, err := sf.collectSkill(cmd, name)
	if err != nil {
		return err
	}
	target, err := sf.options(cmd.Context(), false)
	if err != nil {
		return err
	}
	plan, err := project.PlanLocalSkill(cmd.Context(), name, target)
	if err != nil {
		return err
	}
	description, err := sf.askDescription(name, false)
	if err != nil {
		return err
	}
	result, err := plan.Commit(cmd.Context(), description, kind)
	if err != nil {
		return err
	}
	output.report = skillCreatedReport(result.Added, result.Changed, result.Warnings, name, authoringScope{workdir: sf.directory}, false)
	return nil
}

// skillForkedReport lists the fork's files and explains what the fork replaced and how to install it.
func skillForkedReport(result project.AuthoringResult, from project.SkillForkSource, plan *project.SkillForkPlan, workdir string) commandReport {
	var out strings.Builder
	fmt.Fprintf(&out, "Skill forked from %s at commit %s.\n", from, shortCommit(plan.BasedOn()))
	formatAuthored(&out, result.Added, result.Changed, result.Warnings, workdir)
	instruction := "Next: Edit the forked skill to change its steps. A fork has no version: it changes\nonly when you edit it. The source's license still applies to the copied files." +
		"\nconfig.yaml now forks " + from.String() + " as skills/" + plan.Name() + ", based on commit " + shortCommit(plan.BasedOn()) + ";\ntrailguide project update lists the source's later changes to it."
	steps := []nextStep{}
	if fixes := plan.Fixes(); len(fixes) > 0 {
		steps = append(steps, nextStep{Instruction: forkFixesInstruction(fixes, "trailguide project build refuses it")})
	}
	steps = append(steps, nextStep{Instruction: instruction}, nextStep{Instruction: "Then install the fork in place of the source's skill:", Commands: []string{"trailguide project build", "trailguide project check"}})
	return authoredReport(&out, result.Added, result.Changed, result.Warnings, steps)
}
