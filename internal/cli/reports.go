// Build command reports while scope and inputs are known; both output modes share next steps.

package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/internal/rules"
)

// commandReport owns a completed command's data, human rendering, and optional reported failure.
// A failed check retains its report; execution errors are handled separately and never imply success.
type commandReport struct {
	value   any
	human   string
	failure *responseError
}

// nextStep keeps recovery or follow-up commands separate from their explanation for automation.
type nextStep struct {
	Instruction string   `json:"instruction"`
	Commands    []string `json:"commands"`
}

// authoringValue is an authoring command's JSON value: the files it created and changed, as absolute paths.
type authoringValue struct {
	Added     []string   `json:"added"`
	Changed   []string   `json:"changed"`
	Warnings  []string   `json:"warnings"`
	NextSteps []nextStep `json:"nextSteps"`
}

// authoringScope retains explicit invocation context without reading Cobra flags during rendering.
type authoringScope struct {
	library bool
	// directory is the --directory option, and workdir the working directory that report paths are relative to.
	directory string
	workdir   string
}

func (s authoringScope) command(action string) string {
	if !s.library {
		return "trailguide project " + action
	}
	command := "trailguide library " + action
	if s.directory != "" {
		command += " --directory='" + strings.ReplaceAll(s.directory, "'", "'\"'\"'") + "'"
	}
	return command
}

// authoredReport renders each next step once, after the report, and returns the steps in its value. Every list
// in the value is present, empty when there's nothing to report.
func authoredReport(out *strings.Builder, added, changed, warnings []string, steps []nextStep) commandReport {
	var next strings.Builder
	for i, step := range steps {
		if step.Commands == nil {
			steps[i].Commands = []string{}
		}
		if next.Len() > 0 {
			next.WriteByte('\n')
		}
		next.WriteString(step.Instruction + "\n")
		for _, command := range step.Commands {
			fmt.Fprintf(&next, "  %s\n", command)
		}
	}
	if next.Len() > 0 {
		out.WriteByte('\n')
		out.WriteString(next.String())
	}
	return commandReport{value: authoringValue{nonNil(added), nonNil(changed), nonNil(warnings), steps}, human: out.String()}
}

// nonNil returns list, or an empty list for nil, so JSON output shows [] rather than null.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// projectInitializedReport lists the files a first init added, relative to workdir, as other authoring reports do,
// or, when the project was already initialized, where its configuration is.
func projectInitializedReport(result project.AuthoringResult, workdir string) commandReport {
	var out strings.Builder
	if len(result.Written()) == 0 {
		out.WriteString("Trail Guide is already initialized.\nNo files changed.\n")
		for _, warning := range result.Warnings {
			fmt.Fprintf(&out, "Warning: %s\n", warning)
		}
		out.WriteString("\nTrail Guide directory: .trailguide\nProject configuration: .trailguide/config.yaml\n")
	} else {
		out.WriteString("Trail Guide initialized!\n")
		formatAuthored(&out, result.Added, result.Changed, result.Warnings, workdir)
	}
	steps := []nextStep{{Instruction: "Run trailguide project --help to manage this project's rules.", Commands: []string{"trailguide project --help"}}}
	if len(result.Written()) > 0 {
		steps = []nextStep{
			{Instruction: "Start with a project-only rule (example):", Commands: []string{"trailguide project add group practices/testing", "trailguide project add rule practices/testing/my-rule"}},
			{Instruction: "Or use a shared library or skills repository (example):", Commands: []string{"trailguide project add source team", "trailguide project sync"}},
			{Instruction: "AGENTS.md tells your coding agent to read the rules. If your agent reads another file, such as CLAUDE.md, import or link AGENTS.md there; see .trailguide/README.md."},
		}
	}
	return authoredReport(&out, result.Added, result.Changed, result.Warnings, steps)
}

// sourceAddedReport reports the source name added, saying which ref it recorded when the command chose chosen, the
// library's newest release, itself.
func sourceAddedReport(result project.AuthoringResult, scope authoringScope, name, chosen string) commandReport {
	var out strings.Builder
	if chosen != "" {
		fmt.Fprintf(&out, "Set sources.%s.ref to %s, the library's newest release.\n", name, chosen)
	}
	formatAuthored(&out, result.Added, result.Changed, result.Warnings, scope.workdir)
	return authoredReport(&out, result.Added, result.Changed, result.Warnings, []nextStep{{Instruction: "Next: Fetch the source, build guidance, and install its skills:", Commands: []string{"trailguide project sync"}}})
}

func libraryInitializedReport(result library.AuthoringResult, scope authoringScope) commandReport {
	var out strings.Builder
	formatAuthored(&out, result.Added, result.Changed, result.Warnings, scope.workdir)
	steps := []nextStep{}
	if !result.LicenseDeclared {
		steps = append(steps, nextStep{Instruction: "License is undeclared. Decide terms before sharing."})
	}
	if result.HasGroups {
		steps = append(steps, nextStep{Instruction: "Next: Validate the library:", Commands: []string{scope.command("check")}})
	} else {
		steps = append(steps, nextStep{Instruction: "Next: Add a group and rule, then validate the library:", Commands: []string{scope.command("add group practices/testing"), scope.command("add rule practices/testing/my-rule"), scope.command("check")}})
	}
	return authoredReport(&out, result.Added, result.Changed, result.Warnings, steps)
}

// ruleCreatedReport explains how to finish the rule; changeNote adds recording id's change note in a released library.
func ruleCreatedReport(added, changed, warnings []string, draft bool, scope authoringScope, id string, changeNote bool) commandReport {
	var out strings.Builder
	if draft {
		out.WriteString("Rule draft created.\n")
	} else {
		out.WriteString("Rule created from --body-file.\n")
	}
	formatAuthored(&out, added, changed, warnings, scope.workdir)
	instruction := "Review the Markdown file above. Make future edits directly in that file."
	if draft {
		instruction = "Next: Open the Markdown file above in your editor.\nKeep the metadata between the --- lines at the top. Below it:\n  - State the instructions and explain why they matter.\n  - Add correct and incorrect examples, then describe how to check compliance.\n  - Replace <...> placeholders and remove unused template sections."
		instruction += "\n  - Remove the " + rules.DraftMarker + " marker when the rule is complete."
	}
	commands := []string{scope.command("build"), scope.command("check")}
	if scope.library {
		commands = []string{scope.command("check")}
	}
	steps := []nextStep{{Instruction: instruction}, {Instruction: ruleReadyHeading(scope.library), Commands: commands}}
	if changeNote {
		steps = []nextStep{
			{Instruction: instruction},
			{Instruction: "After the first library release, every new rule needs a change note. After writing the rule text, record it with a summary for project maintainers:", Commands: []string{scope.command("change " + id + " --summary '<what the rule adds>'")}},
			{Instruction: "Then validate the library:", Commands: commands},
		}
	}
	return authoredReport(&out, added, changed, warnings, steps)
}

func groupCreatedReport(added, changed, warnings []string, groupPath string, scope authoringScope) commandReport {
	var out strings.Builder
	formatAuthored(&out, added, changed, warnings, scope.workdir)
	action := "build"
	if scope.library {
		action = "check"
	}
	steps := []nextStep{
		{Instruction: "Next: Add a rule to this group (replace my-rule with your rule's slug):", Commands: []string{scope.command("add rule " + groupPath + "/my-rule")}},
		{Instruction: ruleReadyHeading(scope.library), Commands: []string{scope.command(action)}},
	}
	return authoredReport(&out, added, changed, warnings, steps)
}

// agentsActions describes each AGENTS.md change in the human report.
var agentsActions = map[string]string{"created": "added to a new file", "added": "added", "updated": "updated"}

// withExcludedLinkWarnings returns changes with a warning, after its own, for each of its ExcludedSkillLinks, so
// human and JSON output report them alike.
func withExcludedLinkWarnings(changes project.FileChanges) project.FileChanges {
	changes.Warnings = slices.Clone(changes.Warnings)
	for _, link := range changes.ExcludedSkillLinks {
		references := "references"
		if link.Uses {
			references = "uses and references"
		}
		warning := fmt.Sprintf("%s %s %s, which this project excludes. %s won't be installed as a skill", link.Skill, references, link.Excluded, link.Excluded)
		if link.Uses {
			warning += ", so the use is dropped"
		}
		warning += ", but its referenced files will be retained for reading."
		if len(link.Chain) > 2 {
			warning += " The reference goes through " + strings.Join(link.Chain, " -> ") + "."
		}
		warning += fmt.Sprintf(" To leave them out too, exclude %s or fork it without the link.", link.Skill)
		changes.Warnings = append(changes.Warnings, warning)
	}
	return changes
}

// projectChangesReport lists the files sync, build, or update changed in the project at root, naming their directory
// relative to workdir, as authoring reports name paths.
func projectChangesReport(action string, result project.FileChanges, root, workdir string) commandReport {
	var out strings.Builder
	trailguide := relativeDirectory(project.Directory(root), workdir)
	// The counts below are the command's own, so a recovery that came first is said separately.
	if result.Recovered {
		out.WriteString("First recovered an interrupted earlier command, which restored or finished that command's files.\n")
	}
	if result.Guide != nil {
		status := "updated"
		if result.Guide.Created {
			status = "created"
		}
		fmt.Fprintf(&out, "Trail Guide guide %s: %s\n", status, filepath.Join(trailguide, result.Guide.Path))
	}
	if result.Agents != nil {
		fmt.Fprintf(&out, "Trail Guide section %s: %s\n", agentsActions[result.Agents.Action], filepath.Join(filepath.Dir(trailguide), result.Agents.Path))
	}
	fmt.Fprintf(&out, "%s complete: %d added, %d changed, %d removed.\n", strings.ToUpper(action[:1])+action[1:], len(result.Added), len(result.Changed), len(result.Removed))
	if len(result.Added)+len(result.Changed)+len(result.Removed) > 0 {
		if action == "build" {
			fmt.Fprintf(&out, "Paths relative to %s:\n", filepath.Join(trailguide, filepath.FromSlash(librarytree.Generated)))
		} else {
			fmt.Fprintf(&out, "Paths relative to %s:\n", trailguide)
		}
	}
	for _, group := range []struct {
		label string
		paths []string
	}{{"Add", result.Added}, {"Change", result.Changed}, {"Remove", result.Removed}} {
		for _, path := range group.paths {
			fmt.Fprintf(&out, "  %s: %s\n", group.label, path)
		}
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&out, "Warning: %s\n", warning)
	}
	return commandReport{value: result, human: out.String()}
}

func libraryCheckedReport(result library.CheckResult) commandReport {
	var out strings.Builder
	if result.SkillCount > 0 {
		fmt.Fprintf(&out, "Library is valid: %d group(s), %d rule(s), %d skill(s).\n", result.GroupCount, result.RuleCount, result.SkillCount)
	} else {
		fmt.Fprintf(&out, "Library is valid: %d group(s), %d rule(s).\n", result.GroupCount, result.RuleCount)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&out, "Warning: %s\n", warning)
	}
	formatForkedSkills(&out, result.ForkedSkills)
	// With nothing pending, check says what library release would, rather than describe an empty release. The first
	// library release always publishes the library-wide files, which check doesn't list before it.
	first := result.PendingRelease.Release == 1
	if len(result.PendingRelease.Rules)+len(result.PendingRelease.Skills)+len(result.PendingRelease.LibraryFiles) == 0 && !first {
		fmt.Fprintf(&out, "\nNothing to publish: no pending change notes and no library-wide changes since release/%d.\n", result.PendingRelease.Release-1)
		return commandReport{value: result, human: out.String()}
	}
	fmt.Fprintf(&out, "\nPending library release %d\n", result.PendingRelease.Release)
	if len(result.PendingRelease.Rules)+len(result.PendingRelease.Skills) == 0 {
		out.WriteString("  No rule changes are pending.\n")
		if first {
			out.WriteString("  The first library release publishes the library-wide files, such as group metadata and shared assets.\n")
		}
	}
	formatPendingRows(&out, result.PendingRelease.Rules, result.PendingRelease.Skills)
	if files := result.PendingRelease.LibraryFiles; len(files) > 0 {
		fmt.Fprintf(&out, "Library-wide files changed since release/%d:\n  %s\n", result.PendingRelease.Release-1, strings.Join(files, "\n  "))
	}
	return commandReport{value: result, human: out.String()}
}

// formatForkedSkills lists the skills the library forked whose sources changed them since the commit each fork is
// based on, with the files that differ, so the maintainer can compare them with the fork.
func formatForkedSkills(out *strings.Builder, rows []library.ForkedSkill) {
	if len(rows) == 0 {
		return
	}
	out.WriteString("\nForked skills that their sources changed:\n")
	for _, row := range rows {
		fmt.Fprintf(out, "  %s, forked from %s:%s at commit %s\n", row.Skill, row.Source, row.ID, shortCommit(row.BasedOn))
		if row.Removed {
			fmt.Fprintf(out, "    %s no longer has the skill at %s, commit %s.\n", row.Source, row.Revision, shortCommit(row.Commit))
			fmt.Fprintf(out, "    Your fork stays the library's own skill. To stop comparing it with the source, remove\n    sources.%s.skills.forks.%s.basedOn.\n", row.Source, row.ID)
			continue
		}
		fmt.Fprintf(out, "    Changed at %s, commit %s:\n", row.Revision, shortCommit(row.Commit))
		for _, file := range row.Files {
			fmt.Fprintf(out, "      %s\n", file)
		}
		fmt.Fprintf(out, "    Once your fork has the changes you want, set sources.%s.skills.forks.%s.basedOn\n    to %q.\n", row.Source, row.ID, row.Commit)
	}
	out.WriteString("  Compare them with your forks, which check leaves as they are.\n")
}

// pendingVersions shows a pending rule's current and next version, or the only one it has.
func pendingVersions(rule library.PendingRule) string {
	switch {
	case rule.LastVersion != nil && rule.ReplacedBy != "":
		return rule.LastVersion.String() + ", replaced by " + rule.ReplacedBy
	case rule.LastVersion != nil:
		return rule.LastVersion.String()
	case rule.From == nil:
		return rule.To.String()
	}
	return rule.From.String() + " -> " + rule.To.String()
}

// projectCheckedReport reports a project check of the project at root, naming its paths' directory relative to
// workdir.
func projectCheckedReport(result projectCheckResult, root, workdir string) commandReport {
	var out strings.Builder
	report := commandReport{value: result}
	if result.Status == "up-to-date" {
		out.WriteString("Status: up to date.\nGenerated guidance, installed skills, and the Trail Guide guide are current.\nNo files were changed.\n")
	} else {
		report.failure = &responseError{Kind: "out-of-date", Message: "this project's Trail Guide files are out of date; see the reported problems and next steps"}
		fmt.Fprintf(&out, "Status: out of date.\nNo files were changed.\nPaths relative to %s:\n\nProblems:\n", relativeDirectory(project.Directory(root), workdir))
		for _, problem := range result.Problems {
			fmt.Fprintf(&out, "  %s: %s\n    Next: %s\n", problem.Message, problem.Path, strings.Join(problem.NextSteps[0].Commands, "; "))
		}
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&out, "Warning: %s\n", warning)
	}
	report.human = out.String()
	return report
}

// relativeDirectory returns directory, which is absolute, relative to workdir, resolving symbolic links so a working
// directory reached through one, such as macOS's /var for /private/var, still compares. It returns directory as is
// when no relative path leads there.
func relativeDirectory(directory, workdir string) string {
	if absolute, err := filepath.Abs(workdir); err == nil {
		workdir = absolute
	}
	if resolved, err := filepath.EvalSymlinks(workdir); err == nil {
		workdir = resolved
	}
	if resolved, err := filepath.EvalSymlinks(directory); err == nil {
		directory = resolved
	}
	relative, err := filepath.Rel(workdir, directory)
	if err != nil {
		return directory
	}
	return relative
}
