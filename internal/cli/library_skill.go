// Register library add skill, which creates a draft skill through library.PlanSkill, or forks a skill from a source
// the library declares through library.PlanSkillFork.

package cli

import (
	"fmt"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/spf13/cobra"
)

// librarySkillCommand registers library add skill: NAME creates a draft skill, and --from forks one under its name.
func librarySkillCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("skill NAME | --from SOURCE:SKILL", "Create a draft skill, or fork a skill from a source the library declares", skillArguments("fix-bug"), options)
	cmd.Long = cmd.Short + ".\n\nWith NAME, writes .trailguide/skills/NAME/SKILL.md with the name, the description,\nand the type in its frontmatter, and a body with the trailguide:draft marker.\nComplete the steps and remove the marker; library check fails until you do. A\nnew skill is offered; to require or recommend it, add it to library.skills.\n\nWith --from, forks the skill to publish your own version of it. Copies the\nskill, with every file's executable bit, from the source's repository at the\nrevision its ref names now, or its newest library release without one, into\n.trailguide/skills/<name>/, where <name> is the skill's name. The skills its\nSKILL.md uses get the source's name, such as pstack:skills/tdd, so they name\nskills the library curates from the source. config.yaml gets an entry in the\nsource's skills.forks that names the fork with as, records the commit it copied\nas basedOn, and records your reason; when library.skills lists the skill as one\nthe library curates, the fork takes its place at the same level. trailguide library check then lists the source's later changes to the\nskill whenever its ref moves." + documentationHelp
	f.addSkillFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return createLibrarySkill(cmd, f, args[0], output)
		}
		if err := refuseOptions(cmd, "a fork, which keeps the source skill's frontmatter", "description", "type"); err != nil {
			return err
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

// createLibrarySkill writes the draft skill name, validating its inputs before prompting for a missing description.
func createLibrarySkill(cmd *cobra.Command, f *authoringFlags, name string, output *commandOutput) error {
	kind, err := f.collectSkill(cmd, name)
	if err != nil {
		return err
	}
	target, err := f.libraryOptions(cmd.Context(), false)
	if err != nil {
		return err
	}
	plan, err := library.PlanSkill(cmd.Context(), name, target)
	if err != nil {
		return err
	}
	description, err := f.askDescription(name, true)
	if err != nil {
		return err
	}
	result, err := plan.Commit(cmd.Context(), description, kind)
	if err != nil {
		return err
	}
	output.report = skillCreatedReport(result.Added, result.Changed, result.Warnings, name, authoringScope{library: true, directory: f.value("directory"), workdir: f.directory})
	return nil
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
	steps := []nextStep{}
	if fixes := plan.Fixes(); len(fixes) > 0 {
		steps = append(steps, nextStep{Instruction: forkFixesInstruction(fixes, "library check fails and projects can't install the fork")})
	}
	steps = append(steps, nextStep{Instruction: instruction})
	// This repository is a project too, which installs the fork once it takes the revision the fork copied.
	if plan.FollowsReleases() {
		steps = append(steps, nextStep{Instruction: "To install the fork in this repository, update the source to the release the fork copied:", Commands: []string{"trailguide project update " + from.Source}})
	} else {
		steps = append(steps, nextStep{Instruction: "To install the fork in this repository, sync it:", Commands: []string{"trailguide project sync"}})
	}
	steps = append(steps, nextStep{Instruction: "Then validate the library:", Commands: []string{scope.command("check")}})
	return authoredReport(&out, result.Added, result.Changed, result.Warnings, steps)
}

// forkFixesInstruction asks the author to make fixes, the changes a fork's SKILL.md needs that Trail Guide couldn't
// make, and then remove the draft marker; consequence says what the marker blocks until then.
func forkFixesInstruction(fixes []string, consequence string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Finish the fork: Trail Guide couldn't finish adapting the skill's SKILL.md, so it marked\nthe fork as a draft. Until you finish it, %s.\nIn SKILL.md:\n", consequence)
	for _, fix := range fixes {
		fmt.Fprintf(&out, "  - %s\n", fix)
	}
	out.WriteString("Then remove the " + rules.DraftMarker + " line.")
	return out.String()
}
