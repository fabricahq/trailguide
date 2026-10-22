// Register the supported commands for consuming projects.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/spf13/cobra"
)

// newProjectCommand groups project operations with their flags and help sections.
func newProjectCommand(options Options, output *commandOutput) *cobra.Command {
	command := &cobra.Command{Use: "project", Short: "Manage rules for this project"}
	command.Long = command.Short + "\n\nTrail Guide stores this project's configuration and rules in the .trailguide/\ndirectory at its root. Rules can be project-only, imported from libraries, or both.\nRun init from the Git repository root. Other project commands can run from any\nsubdirectory. Outside Git, run commands from the project root." + documentationHelp
	command.AddGroup(
		&cobra.Group{ID: "main", Title: "Main commands:"},
		&cobra.Group{ID: "utility", Title: "Utility commands:"},
	)
	for _, name := range []string{"sync", "build", "check"} {
		descriptions := map[string]string{"sync": "Fetch configured sources, build guidance, and install skills", "build": "Rebuild guidance and installed skills using local snapshots", "check": "Check configuration, generated guidance, and installed skills"}
		cmd := &cobra.Command{Use: name, Short: descriptions[name], Args: cobra.NoArgs}
		var retake []string
		if name == "sync" {
			cmd.Flags().StringArrayVar(&retake, "retake", nil, "Resolve `SOURCE` again at its ref, ignoring its lock entry; repeat for more sources")
		}
		if name == "check" {
			cmd.Long = "Check project configuration, generated guidance, installed skills, and the Trail Guide guide without changing files or fetching libraries. This checks file consistency, not whether application code follows the rules."
		}
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			directory, err := commandDirectory(cmd.Context(), options.Directory, "project", false)
			if err != nil {
				return err
			}
			projectOptions := project.Options{Directory: directory, ToolVersion: options.Version, Home: options.home()}
			var changes project.FileChanges
			switch name {
			case "sync":
				changes, err = project.Sync(cmd.Context(), projectOptions, options.Git, retake...)
			case "build":
				changes, err = project.Build(cmd.Context(), projectOptions)
			case "check":
				report, checkErr := checkProject(cmd.Context(), projectOptions)
				if checkErr == nil {
					output.report = projectCheckedReport(report, directory, options.Directory)
				}
				return checkErr
			}
			if err != nil {
				return err
			}
			output.report = projectChangesReport(name, changes, directory, options.Directory)
			return nil
		}
		command.AddCommand(cmd)
	}
	command.AddCommand(projectInitCommand(options, output), projectUpdateCommand(options, output))
	add := &cobra.Command{Use: "add", Short: "Add a project-only rule, project-only group, or source, or fork a skill"}
	add.AddCommand(projectSourceCommand(options, output), projectGroupCommand(options, output), projectRuleCommand(options, output), projectSkillCommand(options, output))
	command.AddCommand(add, projectListCommand(options, output))
	for _, child := range command.Commands() {
		child.GroupID = "main"
		if child.Name() == "build" || child.Name() == "check" {
			child.GroupID = "utility"
		}
	}

	return command
}

func projectInitCommand(options Options, output *commandOutput) *cobra.Command {
	initialize, f := newAuthoringCommand("init", "Set up Trail Guide in this project", cobra.NoArgs, options.Directory)
	initialize.Long = initialize.Short + "\n\nCreates the .trailguide/ directory at the Git repository root, with the project\nconfiguration, config.yaml, and a guide, README.md, that explains how to manage\nrules. It also adds a section to AGENTS.md that tells your coding agent to read\nthe rules, creating the file if needed. Run it from the repository root;\noutside Git, run it from the project root. Running it again keeps your\nconfiguration and rules, and refreshes an outdated, unedited guide and section.\n\nA new configuration installs skills into .claude/skills/ and .agents/skills/.\nChoose others with --skill-targets: repeat it for claude and agents, or pass\nnone to install no copies. On an existing configuration, init changes the skill\ntargets only when you pass --skill-targets.\n\nLearn more: https://trailguide.fabricahq.com/start-here/set-up-project/"
	var skillTargets []string
	initialize.Flags().StringArrayVar(&skillTargets, "skill-targets", nil, "Where to install skills (repeat): claude for .claude/skills/, agents for .agents/skills/, or none")
	initialize.RunE = func(cmd *cobra.Command, _ []string) error {
		targets, err := parseSkillTargetFlags(skillTargets, cmd.Flags().Changed("skill-targets"))
		if err != nil {
			return err
		}
		target, err := f.options(cmd.Context(), true)
		if err != nil {
			return err
		}
		target.ToolVersion = options.Version
		result, err := project.InitializeWithSkillTargets(cmd.Context(), target, targets)
		if err != nil {
			return err
		}
		output.report = projectInitializedReport(result, f.directory)
		return nil
	}
	return initialize
}

// parseSkillTargetFlags returns the skill targets --skill-targets names, an empty list for none, or nil when the
// flag wasn't passed, so init keeps or defaults them.
func parseSkillTargetFlags(values []string, passed bool) ([]string, error) {
	if !passed {
		return nil, nil
	}
	targets := []string{}
	for _, value := range values {
		switch {
		case value == "none" && len(values) > 1:
			return nil, usage(fmt.Errorf("--skill-targets none can't be combined with a skill target"))
		case value == "none":
		case value != "claude" && value != "agents":
			return nil, usage(fmt.Errorf("--skill-targets must be claude, for .claude/skills/, agents, for .agents/skills/, or none, got %q", value))
		case slices.Contains(targets, value):
			return nil, usage(fmt.Errorf("--skill-targets %s is given more than once; give each skill target once", value))
		default:
			targets = append(targets, value)
		}
	}
	return targets, nil
}

func projectSourceCommand(options Options, output *commandOutput) *cobra.Command {
	source, sf := newAuthoringCommand("source NAME", "Configure a source of rules and skills (without fetching)", requiredArgument("source name", "team", "The name identifies this source in your project configuration."), options.Directory)
	for name, description := range map[string]string{"repository": "Git repository URL", "ref": "Library release (release/N), latest-release, branch, tag, or full commit SHA to take the source from; defaults to the newest library release"} {
		sf.add(source, name, description)
	}
	source.Long = sourceSelectionHelp + documentationHelp
	sf.prompts = map[string]string{"repository": "Git repository URL"}
	var ruleSelectors, skillSelectors []string
	source.Flags().StringArrayVar(&ruleSelectors, "rules", nil, "Rule `selector` for rules.include (repeat): a rule ID, a group prefix such as techs/go/*, or *")
	source.Flags().StringArrayVar(&skillSelectors, "skills", nil, "Skill `selector` for skills.include (repeat): a skill ID such as skills/build-feature, recommended, or *")
	source.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := sf.options(cmd.Context(), false)
		if err != nil {
			return err
		}
		plan, err := project.PlanSource(cmd.Context(), args[0], target)
		if err != nil {
			return err
		}
		sf.introduction = sourceSelectionIntroduction(args[0])
		if err := sf.collectSource(&ruleSelectors, &skillSelectors); err != nil {
			return err
		}
		ref, chosen := sf.value("ref"), ""
		if ref == "" {
			if ref, err = newestReleaseRef(cmd.Context(), args[0], sf.value("repository"), options.Git); err != nil {
				return err
			}
			chosen = ref
		}
		result, err := plan.Commit(cmd.Context(), project.SourceInput{Repository: sf.value("repository"), Ref: ref, Rules: ruleSelectors, Skills: skillSelectors})
		if err != nil {
			return err
		}
		output.report = sourceAddedReport(result, authoringScope{workdir: sf.directory}, args[0], chosen)
		return nil
	}
	return source
}

// newestReleaseRef returns release/<number> for the newest library release of repository, the ref project add source
// records without --ref, listing the repository's release tags. A repository without any fails with code
// ref-required, asking for --ref.
func newestReleaseRef(ctx context.Context, name, repository string, git imports.Options) (string, error) {
	address, _ := json.Marshal(repository)
	if _, err := rules.ParseRepository(address, "--repository"); err != nil {
		return "", err
	}
	newest, err := imports.NewestRelease(ctx, repository, git)
	if err != nil {
		return "", fmt.Errorf("list the library releases of sources.%s: %w", name, err)
	}
	if newest == 0 {
		return "", &filetxn.Error{Code: "ref-required", Problem: "the repository has no release/<number> tags, so it isn't a library or hasn't published its first library release; pass --ref with the branch, tag, or full commit SHA to take it from"}
	}
	return fmt.Sprintf("release/%d", newest), nil
}

func projectGroupCommand(options Options, output *commandOutput) *cobra.Command {
	group, gf := newAuthoringCommand("group GROUP_PATH", "Create a project-only rule group", requiredArgument("group path", "practices/testing", "Use a category and group slug, such as practices/testing or techs/go."), options.Directory)
	gf.addGroupFlags(group, "")
	group.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := gf.options(cmd.Context(), false)
		if err != nil {
			return err
		}
		plan, err := project.PlanLocalGroup(cmd.Context(), args[0], target)
		if err != nil {
			return err
		}
		gf.introduction = groupIntroduction(args[0], false)
		if err := gf.require("name", "description", "when-to-read"); err != nil {
			return err
		}
		result, err := plan.Commit(cmd.Context(), gf.group(""))
		if err != nil {
			return err
		}
		output.report = groupCreatedReport(result.Added, result.Changed, result.Warnings, args[0], authoringScope{workdir: gf.directory})
		return nil
	}
	return group
}

func projectRuleCommand(options Options, output *commandOutput) *cobra.Command {
	rule, rf := newAuthoringCommand("rule RULE_PATH", "Create a project-only rule or unfinished draft", requiredArgument("rule path", "practices/testing/my-rule", "Include the group path and rule slug, without .md."), options.Directory)
	rf.addRuleFlags(rule)
	rule.Long = strings.TrimSuffix(rule.Long, documentationHelp) + "\n\nTo fork one version of a library rule instead, copying it into this project so\nyou can change what it says, use --from LIBRARY@VERSION (e.g. team@1.3.0) with\nthe library rule's ID as RULE_PATH. When the project imports the rule from that\nlibrary, the fork replaces it, and --reason records why." + documentationHelp
	rf.add(rule, "from", "Fork `LIBRARY@VERSION` of a library rule, such as team@1.3.0, instead of writing one")
	rf.add(rule, "reason", "Why the project uses the fork instead of the rule it imports; required when it imports the rule")
	rule.RunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("from") {
			return forkRule(cmd, args[0], rf, options, output)
		}
		if cmd.Flags().Changed("reason") {
			return usage(fmt.Errorf("--reason applies only to a fork; add --from LIBRARY@VERSION, or omit --reason"))
		}
		target, err := rf.options(cmd.Context(), false)
		if err != nil {
			return err
		}
		plan, err := project.PlanLocalRule(cmd.Context(), args[0], target)
		if err != nil {
			return err
		}
		metadata, body, err := rf.collectRule(cmd.Context(), args[0], false)
		if err != nil {
			return err
		}
		result, err := plan.Commit(cmd.Context(), metadata, body)
		if err != nil {
			return err
		}
		output.report = ruleCreatedReport(result.Added, result.Changed, result.Warnings, body == nil, authoringScope{workdir: rf.directory}, args[0], false)
		return nil
	}
	return rule
}
