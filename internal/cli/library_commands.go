// Expose native library initialization, authoring, and read-only validation independently of consuming projects.

package cli

import (
	"context"
	"fmt"

	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/spf13/cobra"
)

// newLibraryCommand registers library-specific location and explicit-input flags.
func newLibraryCommand(use, description string, args cobra.PositionalArgs, options Options) (*cobra.Command, *authoringFlags) {
	cmd := &cobra.Command{Use: use, Short: description, Args: args}
	flags := &authoringFlags{command: cmd, values: map[string]*singleString{}, directory: options.Directory, git: gitexec.Options{GitPath: options.Git.GitPath, Environment: options.Git.Environment}, imports: options.Git, version: options.Version}
	cmd.PostRunE = flags.finishPrompts
	flags.add(cmd, "directory", "Library directory (default Git root, or current directory outside Git)")
	cmd.Flags().Bool("non-interactive", false, "Require explicit flags; never prompt")
	return cmd, flags
}

// libraryOptions resolves the selected target without changing relative body or license inputs.
func (f *authoringFlags) libraryOptions(ctx context.Context, initialize bool) (library.Options, error) {
	directory := f.directory
	if f.value("directory") != "" {
		directory = f.file("directory")
	}
	directory, err := commandDirectory(ctx, directory, "library", initialize)
	return library.Options{Directory: directory, ToolVersion: f.version, Git: f.git, Curated: f.readCurated, ReadSkill: f.readSourceSkill, Forks: f.readForks}, err
}

// readSourceSkill reads a skill from a source the library declares, which a fork copies.
func (f *authoringFlags) readSourceSkill(ctx context.Context, name string, source rules.CuratedSource, id string) (library.SourceSkill, error) {
	return imports.ReadSourceSkill(ctx, name, source, id, f.imports)
}

// readForks compares the skills the library forked from a source with the source's copies.
func (f *authoringFlags) readForks(ctx context.Context, name string, source rules.CuratedSource, forks map[string]rules.SkillFork) ([]library.ForkedSkill, error) {
	return imports.LibraryForks(ctx, name, source, forks, f.imports)
}

// readCurated reads a source the library curates skills from, as a project that uses the library would.
func (f *authoringFlags) readCurated(ctx context.Context, name string, source rules.CuratedSource, ids []string) (map[string]library.CuratedSkill, string, error) {
	return imports.CuratedSkillNames(ctx, name, source, ids, f.imports)
}

// addLibraryCommands installs a separate command tree that never reads consumer configuration.
func addLibraryCommands(root *cobra.Command, options Options, output *commandOutput) {
	library := &cobra.Command{Use: "library", Short: "Create and maintain a shared rule library"}
	library.Long = library.Short + "\n\nA library keeps its manifest in the library section of .trailguide/config.yaml,\nits rules in .trailguide/rules/, shared assets in .trailguide/assets/, and change\nnotes in .trailguide/changes/. The same repository can also be a project.\n\nRun init from the Git repository root. Other library commands can run from any\nsubdirectory. Outside Git, run commands from the library root." + documentationHelp
	root.AddCommand(library)
	library.AddCommand(libraryInitCommand(options, output), libraryCheckCommand(options, output), libraryChangeCommand(options, output), libraryReleaseCommand(options, output))
	add := &cobra.Command{Use: "add", Short: "Add a library group, rule, or skill"}
	library.AddCommand(add)
	add.AddCommand(libraryGroupCommand(options, output), libraryRuleCommand(options, output), librarySkillCommand(options, output))
}

// libraryInitCommand reads explicit publisher terms before creating library-owned files.
func libraryInitCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("init", "Initialize a rule library without overwriting authored files", cobra.NoArgs, options)
	for name, description := range map[string]string{"name": "Library name, which names the folder of its own skills; defaults to the library folder's name", "spdx": "Library SPDX expression", "license-file": "Existing UTF-8 license text", "notice-file": "Existing UTF-8 notice text"} {
		f.add(cmd, name, description)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		target, err := f.libraryOptions(cmd.Context(), true)
		if err != nil {
			return err
		}
		if (f.value("spdx") == "") != (f.value("license-file") == "") {
			return usage(fmt.Errorf("supply --spdx and --license-file together"))
		}
		if f.value("notice-file") != "" && f.value("spdx") == "" {
			return usage(fmt.Errorf("--notice-file requires --spdx and --license-file"))
		}
		var terms *library.Terms
		if f.value("spdx") != "" {
			license, err := readAuthoringText(cmd.Context(), f.file("license-file"), "license text")
			if err != nil {
				return err
			}
			terms = &library.Terms{SPDXExpression: f.value("spdx"), License: license}
			if f.value("notice-file") != "" {
				notice, err := readAuthoringText(cmd.Context(), f.file("notice-file"), "notice text")
				if err != nil {
					return err
				}
				terms.Notice = &notice
			}
		}
		result, err := library.Initialize(cmd.Context(), target, f.value("name"), terms)
		if err != nil {
			return err
		}
		output.report = libraryInitializedReport(result, authoringScope{library: true, directory: f.value("directory"), workdir: f.directory})
		return nil
	}
	return cmd
}

// libraryCheckCommand reports counts, caveats, and the pending library release without changing library files.
func libraryCheckCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("check", "Validate the library and its change notes, and preview the next library release", cobra.NoArgs, options)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		target, err := f.libraryOptions(cmd.Context(), false)
		if err != nil {
			return err
		}
		result, err := library.Check(cmd.Context(), target)
		if err != nil {
			return err
		}
		output.report = libraryCheckedReport(result)
		return nil
	}
	return cmd
}

// libraryGroupCommand requires all group metadata before attempting exclusive publication.
func libraryGroupCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("group GROUP_PATH", "Create library group metadata", requiredArgument("group path", "practices/testing", "Use a category and group slug, such as practices/testing or techs/go."), options)
	f.addGroupFlags(cmd, "")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := f.libraryOptions(cmd.Context(), false)
		if err != nil {
			return err
		}
		plan, err := library.PlanGroup(cmd.Context(), args[0], target)
		if err != nil {
			return err
		}
		f.introduction = groupIntroduction(args[0], true)
		if err := f.require("name", "description", "when-to-read"); err != nil {
			return err
		}
		result, err := plan.Commit(cmd.Context(), f.group(""))
		if err != nil {
			return err
		}
		output.report = groupCreatedReport(result.Added, result.Changed, result.Warnings, args[0], authoringScope{library: true, directory: f.value("directory"), workdir: f.directory})
		return nil
	}
	return cmd
}

// libraryRuleCommand creates supplied guidance or a marked canonical draft in an existing group.
func libraryRuleCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("rule RULE_PATH", "Create a complete library rule or marked draft", requiredArgument("rule path", "practices/testing/my-rule", "Include the group path and rule slug, without .md."), options)
	f.addRuleFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := f.libraryOptions(cmd.Context(), false)
		if err != nil {
			return err
		}
		plan, err := library.PlanRule(cmd.Context(), args[0], target)
		if err != nil {
			return err
		}
		metadata, body, err := f.collectRule(cmd.Context(), args[0], true)
		if err != nil {
			return err
		}
		result, err := plan.Commit(cmd.Context(), metadata, body)
		if err != nil {
			return err
		}
		output.report = ruleCreatedReport(result.Added, result.Changed, result.Warnings, body == nil, authoringScope{library: true, directory: f.value("directory"), workdir: f.directory}, args[0], plan.NeedsChangeNote())
		return nil
	}
	return cmd
}
