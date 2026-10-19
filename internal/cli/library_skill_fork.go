// Register library add skill, which forks a skill from a source the library declares through library.PlanSkillFork.

package cli

import (
	"fmt"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/spf13/cobra"
)

// librarySkillCommand registers library add skill. It only forks: a new skill is written by hand.
func librarySkillCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("skill --from SOURCE:SKILL", "Fork a skill from a source the library declares", librarySkillArguments, options)
	cmd.Long = cmd.Short + ", to publish your own version of it.\n\nCopies the skill, with every file's executable bit, from the source's repository\nat the revision its ref names now, or its newest library release without one,\ninto .trailguide/skills/<name>/, where <name> is the skill's name. The skills its\nSKILL.md uses get the source's name, such as pstack:skills/tdd, so they name\nskills the library curates from the source. config.yaml gets an entry in the\nsource's skills.exclude that names the fork as the skill's replacement, records\nthe commit it copied as basedOn, and records your reason; when library.skills\nlists the skill as one the library curates, the fork takes its place at the same\nlevel. trailguide library check then lists the source's later changes to the\nskill whenever its ref moves.\n\nTo write a new skill instead, create .trailguide/skills/<name>/SKILL.md." + documentationHelp
	f.add(cmd, "from", "The skill to fork, as `SOURCE:SKILL`: a source in the library's configuration and the skill's ID in its repository, such as pstack:skills/tdd")
	f.add(cmd, "reason", "Why the library publishes its own version instead of the source's skill")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("from") {
			return usage(fmt.Errorf("--from is required: name the skill to fork as SOURCE:SKILL, such as pstack:skills/tdd. To write a new skill instead, create .trailguide/skills/<name>/SKILL.md"))
		}
		from, err := project.ParseSkillForkSource(f.value("from"))
		if err != nil {
			return usage(err)
		}
		target, err := f.libraryOptions(cmd.Context(), false)
		if err != nil {
			return err
		}
		plan, err := library.PlanSkillFork(cmd.Context(), from.Source, from.Skill, target)
		if err != nil {
			return err
		}
		reason := f.value("reason")
		if !cmd.Flags().Changed("reason") {
			if !f.interactive() {
				return usage(fmt.Errorf("--reason is required: the fork replaces %s's %s, and the exclusion that records where it came from records why", from.Source, from.Skill))
			}
			f.introduction = fmt.Sprintf("Forking %s into:\n  .trailguide/skills/%s/\n\nconfig.yaml gets an exclusion that names the fork, records the commit it copies,\nand records your reason.\n\n", from, plan.Name())
			if reason, err = f.askValidated("Why publish your own version instead of the source's skill?", func(value string) error { return validateAnswer("reason", value) }); err != nil {
				return err
			}
		}
		result, err := plan.Commit(cmd.Context(), reason)
		if err != nil {
			return err
		}
		output.report = librarySkillForkedReport(result, from, plan, authoringScope{library: true, directory: f.value("directory"), workdir: f.directory})
		return nil
	}
	return cmd
}

// librarySkillArguments refuses a skill name, because library add skill only forks.
func librarySkillArguments(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("library add skill takes no arguments: it forks the skill --from names, keeping its name. To write a new skill instead, create .trailguide/skills/%s/SKILL.md.\n\nUsage:\n  %s", args[0], cmd.UseLine())
}

// librarySkillForkedReport lists the fork's files and explains what the library now records and publishes.
func librarySkillForkedReport(result library.AuthoringResult, from project.SkillForkSource, plan *library.SkillForkPlan, scope authoringScope) commandReport {
	var out strings.Builder
	fmt.Fprintf(&out, "Skill forked from %s at %s, commit %s.\n", from, plan.Skill().Revision, shortCommit(plan.Skill().Commit))
	formatAuthored(&out, result.Added, result.Changed, result.Warnings, scope.workdir)
	id := "skills/" + plan.Name()
	instruction := "Next: Edit the forked skill to change its steps. The source's license still applies\nto the copied files; credit the source in your library's notices." +
		"\nconfig.yaml now records that " + id + " is based on " + from.String() + " at commit\n" + shortCommit(plan.Skill().Commit) + ", so trailguide library check lists the source's later changes to it." +
		"\nProjects that use the library don't get " + from.Source + " from it; keep the source declared so\ncheck can keep comparing the fork with it."
	if plan.Listed() {
		instruction += "\nlibrary.skills now lists " + id + " in place of " + from.String() + ", at the same level."
	} else {
		instruction += "\nThe fork is offered. To require or recommend it, add " + id + " to library.skills."
	}
	steps := []nextStep{{Instruction: instruction}}
	// This repository is a project too, which installs the fork once it takes the revision the fork copied.
	if plan.FollowsReleases() {
		steps = append(steps, nextStep{Instruction: "To install the fork in this repository, update the source to the release the fork copied:", Commands: []string{"trailguide project update " + from.Source}})
	} else {
		steps = append(steps, nextStep{Instruction: "To install the fork in this repository, sync it:", Commands: []string{"trailguide project sync"}})
	}
	steps = append(steps, nextStep{Instruction: "Then validate the library:", Commands: []string{scope.command("check")}})
	return authoredReport(&out, result.Added, result.Changed, result.Warnings, steps)
}
