// Share how library add skill and project add skill create a draft skill: their arguments, inputs, and report.

package cli

import (
	"fmt"
	"strings"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/spf13/cobra"
)

// skillArguments accepts the name of a skill to create, or none with --from, which forks a skill under its own name.
// example is a skill name for the error that asks for one.
func skillArguments(example string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		fork := cmd.Flags().Changed("from")
		switch {
		case len(args) > 1:
			return fmt.Errorf("expected one skill name; received %d arguments.\n\nUsage:\n  %s", len(args), cmd.UseLine())
		case len(args) == 1 && fork:
			return fmt.Errorf("%s takes a skill name or --from, not both: a fork keeps the source skill's name, so leave out %s.\n\nUsage:\n  %s", cmd.CommandPath(), args[0], cmd.UseLine())
		case len(args) == 0 && !fork:
			return fmt.Errorf("missing skill name: name the skill to create, such as %s, or fork a skill with --from SOURCE:SKILL.\n\nUsage:\n  %s", example, cmd.UseLine())
		}
		return nil
	}
}

// addSkillFlags registers the options that create a skill; a fork takes --from and --reason instead.
func (f *authoringFlags) addSkillFlags(cmd *cobra.Command) {
	f.add(cmd, "description", "What the skill does and when to use it, at most 1,024 characters")
	f.add(cmd, "type", "The skill's type, playbook or review, recorded as trailguide-skill-type; leave it out for neither")
	f.add(cmd, "from", "Fork this skill instead, as `SOURCE:SKILL`: a source and the skill's ID in it")
	f.add(cmd, "reason", "For a fork: why you use your own version instead of the source's skill")
	f.prompts = map[string]string{"description": "Description (what the skill does, then when to use it)"}
}

// refuseOptions fails, before any prompt, when cmd was given an option that names doesn't apply to: what, a fork or
// a new skill.
func refuseOptions(cmd *cobra.Command, what string, names ...string) error {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return usage(fmt.Errorf("--%s doesn't apply to %s", name, what))
		}
	}
	return nil
}

// collectSkill validates name and any --description and --type, so invalid input fails with exit code 2 before any
// prompt, and returns the type.
func (f *authoringFlags) collectSkill(cmd *cobra.Command, name string) (string, error) {
	if err := refuseOptions(cmd, "a new skill; it applies only to a fork with --from", "reason"); err != nil {
		return "", err
	}
	if !skills.ValidName(name) {
		return "", usage(fmt.Errorf("skill name %q: a skill name uses 1 to 64 lowercase letters, digits, and hyphens, without a leading, trailing, or doubled hyphen", name))
	}
	if cmd.Flags().Changed("description") {
		if err := skills.ValidateDescription(f.value("description")); err != nil {
			return "", usage(fmt.Errorf("--description: %w", err))
		}
	}
	if err := skills.ValidateType(f.value("type")); err != nil {
		return "", usage(fmt.Errorf("--type: %w", err))
	}
	return f.value("type"), nil
}

// askDescription prompts for a skill's description when --description wasn't given, after the plan checked the
// skill's folder.
func (f *authoringFlags) askDescription(folder string, isLibrary bool) (string, error) {
	f.introduction = skillIntroduction(folder, isLibrary)
	if !f.command.Flags().Changed("description") {
		if !f.interactive() {
			return "", usage(fmt.Errorf("--description is required: pass it as a flag, since without a terminal, or with --json or --non-interactive, the command can't ask for it"))
		}
		value, err := f.askValidated(f.prompts["description"]+":", skills.ValidateDescription)
		if err != nil {
			return "", err
		}
		f.values["description"].value = value
	}
	return f.value("description"), nil
}

// skillIntroduction explains, before the description prompt, where the skill goes, folder, its path in the
// repository, and what a description says.
func skillIntroduction(folder string, isLibrary bool) string {
	heading := "Adding a skill at: " + folder + "/"
	if !isLibrary {
		heading = "Adding a project-only skill at: " + folder + "/\n\nThis skill belongs to this project."
	}
	return heading + `

- A skill tells an agent how to carry out one kind of task, step by step.
- The description tells an agent when to load the skill: say what the skill
  does, then when to use it.

Example description:
  Fix a reported bug and prove it with a regression test. Use when a task
  starts from a bug report, a failing behavior, or an error someone saw.

`
}

// skillCreatedReport lists the draft's files and says how to complete it and what to run next; changeNote adds
// recording the skill's change note in a released library.
func skillCreatedReport(added, changed, warnings []string, name string, scope authoringScope, changeNote bool) commandReport {
	var out strings.Builder
	out.WriteString("Skill draft created.\n")
	formatAuthored(&out, added, changed, warnings, scope.workdir)
	instruction := "Next: Open SKILL.md above in your editor.\nKeep the frontmatter between the --- lines at the top. Below it:\n  - Number the steps, in the order the agent takes them.\n  - Name the rules each step applies, and list their IDs in metadata.trailguide-rules-used.\n  - Replace <...> placeholders and remove the sections you don't need." +
		"\n  - Remove the " + rules.DraftMarker + " marker when the skill is complete."
	steps := []nextStep{{Instruction: instruction}}
	switch {
	case scope.library && changeNote:
		steps = append(steps,
			nextStep{Instruction: "The skill is offered: projects install it only when they name it. To require or recommend it,\nadd skills/" + name + " to library.skills. After the first library release, every new skill\nneeds a change note. After writing the steps, record it with a summary for project maintainers:", Commands: []string{scope.command("change skills/" + name + " --summary '<what the skill does>'")}},
			nextStep{Instruction: "Then validate the library:", Commands: []string{scope.command("check")}})
	case scope.library:
		steps = append(steps, nextStep{Instruction: "The skill is offered: projects install it only when they name it. To require or recommend it,\nadd skills/" + name + " to library.skills. After writing the steps, run:", Commands: []string{scope.command("check")}})
	default:
		steps = append(steps, nextStep{Instruction: "Project skills are discovered automatically when you build; you don't need to list them in\nconfig.yaml. After writing the steps, to install the skill, run:", Commands: []string{scope.command("build"), scope.command("check")}})
	}
	return authoredReport(&out, added, changed, warnings, steps)
}
