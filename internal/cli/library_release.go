// Publish, finish, or preview a library release through library.Release, and report what it created.

package cli

import (
	"fmt"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/spf13/cobra"
)

// libraryReleaseCommand publishes the pending change notes and library-wide changes as a library release.
func libraryReleaseCommand(options Options, output *commandOutput) *cobra.Command {
	cmd, f := newLibraryCommand("release", "Publish pending rule changes as a library release", cobra.NoArgs, options)
	cmd.Long = cmd.Short + ": an annotated\nrelease/<number> tag on the current commit, pushed to the remote, and, for\nrepositories on GitHub.com, a GitHub Release page created with the GitHub\nCLI, gh. No files change.\n\nThe command fetches first, and refuses unless you're on the remote's default\nbranch, your branch matches the remote exactly, and trailguide library check\npasses. If a run stops after pushing the tag, run it again to create what's\nmissing.\n\nPreview the release notes with --dry-run." + documentationHelp
	cmd.Flags().Bool("dry-run", false, "Show what would be published, without creating a tag or a GitHub Release page")
	cmd.Flags().Bool("no-github-release", false, "Skip creating the GitHub Release page")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		target, err := f.libraryOptions(cmd.Context(), false)
		if err != nil {
			return err
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		noGitHubRelease, _ := cmd.Flags().GetBool("no-github-release")
		result, err := library.Release(cmd.Context(), library.ReleaseRequest{Options: target, DryRun: dryRun, NoGitHubRelease: noGitHubRelease})
		if err != nil {
			// A published tag whose GitHub Release page failed is part of the JSON result; the error explains it to
			// people.
			if result.Release > 0 {
				output.report = commandReport{value: result}
			}
			return err
		}
		output.report = libraryReleaseReport(result, noGitHubRelease, authoringScope{library: true, directory: f.value("directory")})
		return nil
	}
	return cmd
}

// libraryReleaseReport says what the library release publishes and what this run created. A dry run also
// shows the complete release notes.
func libraryReleaseReport(result library.ReleaseResult, noGitHubRelease bool, scope authoringScope) commandReport {
	var out strings.Builder
	switch {
	case result.Release == 0:
		out.WriteString("Nothing to publish: no pending change notes and no library-wide changes since the latest library release.\n")
	case result.DryRun && result.Published:
		fmt.Fprintf(&out, "Dry run: library release %d is already published on this commit.\n", result.Release)
	case result.DryRun:
		fmt.Fprintf(&out, "Dry run: library release %d, not published.\n", result.Release)
	case result.Published && (result.GitHubRelease == nil || !result.GitHubRelease.Created):
		fmt.Fprintf(&out, "Library release %d was already published. Nothing changed.\n", result.Release)
	case result.Published:
		fmt.Fprintf(&out, "Finished publishing library release %d.\n", result.Release)
	default:
		fmt.Fprintf(&out, "Published library release %d.\n", result.Release)
	}
	fmt.Fprintf(&out, "  Repository:          %s\n  Branch:              %s\n  Commit:              %s\n", result.Repository, result.Branch, result.Commit)
	if result.Release > 0 {
		fmt.Fprintf(&out, "  Tag:                 %s (%s)\n", result.Tag, tagStatus(result))
		fmt.Fprintf(&out, "  GitHub Release page: %s\n", pageStatus(result, noGitHubRelease))
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&out, "Warning: %s\n", warning)
	}
	formatForkedSkills(&out, result.ForkedSkills)
	if result.Release == 0 {
		return commandReport{value: result, human: out.String()}
	}
	out.WriteString("\nRules:\n")
	if len(result.Rules) == 0 {
		out.WriteString("  No rule changes, only library-wide files.\n")
	}
	formatPendingRules(&out, result.Rules)
	if len(result.LibraryFiles) > 0 {
		out.WriteString("Library-wide files:\n")
		for _, name := range result.LibraryFiles {
			fmt.Fprintf(&out, "  %s\n", name)
		}
	}
	if result.DryRun {
		fmt.Fprintf(&out, "\nRelease notes:\n\n%s\n", result.Notes)
		next := "To publish it, run:"
		if result.Published {
			next = "To create anything that's missing, such as the GitHub Release page, run:"
		}
		command := scope.command("release")
		if noGitHubRelease {
			command += " --no-github-release"
		}
		fmt.Fprintf(&out, "\n%s\n  %s\n", next, command)
	}
	return commandReport{value: result, human: out.String()}
}

// tagStatus says whether this run created the tag, found it, or would create it.
func tagStatus(result library.ReleaseResult) string {
	switch {
	case result.DryRun && result.Published:
		return "already on " + result.Remote
	case result.DryRun:
		return "not created yet"
	case result.TagCreated:
		return "created and pushed to " + result.Remote
	}
	return "already on " + result.Remote
}

// pageStatus says whether this run created the GitHub Release page, found it, or why there is none.
func pageStatus(result library.ReleaseResult, noGitHubRelease bool) string {
	status := "already exists"
	switch {
	case result.GitHubRepository == "":
		return "none, because " + result.Remote + " isn't on GitHub.com"
	case noGitHubRelease:
		return "skipped (--no-github-release)"
	case result.DryRun && result.Published:
		// A dry run doesn't ask gh whether the page exists.
		return "created with gh for " + result.GitHubRepository + " if it's missing"
	case result.DryRun:
		return "created with gh for " + result.GitHubRepository + " when published"
	case result.GitHubRelease.Created:
		status = "created"
	}
	if result.GitHubRelease.URL == "" {
		return status
	}
	return result.GitHubRelease.URL + " (" + status + ")"
}

// formatPendingRules aligns each rule's ID, change, and versions in columns.
func formatPendingRules(out *strings.Builder, rules []library.PendingRule) {
	idWidth, changeWidth := 0, 0
	for _, rule := range rules {
		idWidth, changeWidth = max(idWidth, len(rule.ID)), max(changeWidth, len(rule.Change))
	}
	for _, rule := range rules {
		fmt.Fprintf(out, "  %-*s  %-*s  %s\n", idWidth, rule.ID, changeWidth, rule.Change, pendingVersions(rule))
	}
}
