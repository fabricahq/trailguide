// Collect a project update's scope, decisions, and confirmation, then preview or apply it through project.PlanUpdate.

package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
	"github.com/spf13/cobra"
)

// projectUpdateCommand previews newer rule versions and applies them after confirmation or with --yes.
func projectUpdateCommand(options Options, output *commandOutput) *cobra.Command {
	cmd := &cobra.Command{Use: "update [SOURCE | SOURCE:RULE ...]", Short: "Preview and apply newer rules and skills from your sources", Args: cobra.ArbitraryArgs}
	cmd.Long = "Preview newer versions of the rules and skills your sources supply, new rules\nand skills, and retirements, then apply them after you confirm. With no\narguments, every source is updated. SOURCE, such as team, updates one source's\nrules and skills. SOURCE:RULE, such as team:techs/go/errors, moves only that\nrule and adds no new rules. No command updates one skill: a source's skills\nall come from the revision that supplies its shared files, so they move\ntogether whenever it does, also when SOURCE:RULE needs a newer release. A\nsource whose ref is a branch moves to the branch's newest commit; a library\nsource whose ref is a tag or commit doesn't move.\n\nIn a terminal, update asks about each major change and retirement of a rule\n(adopt it, or keep the current version with a pin), each new rule (add it, or\nexclude it), and each local rule that replaces a changed library rule (review the\nchanges later, or replace your rule with a fork of the newest version,\ndiscarding your edits), and each forked skill whose source's changes merge\ninto it with no text conflicts (review them later, or merge them, keeping your\nedits), then asks you to confirm. Without a terminal, or with --json, it only shows the\npreview unless you pass --yes. The update applies exactly the versions the\npreview showed." + documentationHelp
	f := &authoringFlags{command: cmd, values: map[string]*singleString{}, directory: options.Directory}
	cmd.PostRunE = f.finishPrompts
	cmd.Flags().Bool("yes", false, "Apply the previewed changes without asking; required without a terminal or with --json")
	var keep, exclude, forks, merges []string
	cmd.Flags().StringArrayVar(&keep, "keep", nil, "Pin `SOURCE:RULE` at its current version instead of updating it (repeat); requires --reason")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Exclude the new rule `SOURCE:RULE` instead of adding it (repeat); requires --reason")
	cmd.Flags().StringArrayVar(&forks, "update-fork", nil, "Replace your local rule that replaces `SOURCE:RULE` with a fork of the newest version, discarding your edits, and set its basedOn to that version (repeat)")
	cmd.Flags().StringArrayVar(&merges, "merge-fork", nil, "Merge the source's changes into your fork of `SOURCE:SKILL`, keeping your edits, and set its basedOn to the commit merged; refuses when they conflict with your edits (repeat)")
	f.add(cmd, "reason", "Reason recorded with each pin and exclusion the update writes")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Every refusal says that the update wrote nothing, as a failed sync or update does.
		targets, err := updateTargets(args)
		if err != nil {
			return usage(&project.UnchangedError{Err: err})
		}
		decisions, err := flagDecisions(keep, exclude, forks, merges, f.value("reason"))
		if err != nil {
			return usage(&project.UnchangedError{Err: err})
		}
		directory, err := commandDirectory(cmd.Context(), options.Directory, "project", false)
		if err != nil {
			return &project.UnchangedError{Err: err}
		}
		f.status("Reading library releases...")
		plan, err := project.PlanUpdate(cmd.Context(), project.Options{Directory: directory, ToolVersion: options.Version, Home: options.home()}, options.Git, targets, decisions)
		if err != nil {
			return err
		}
		location := updateLocation{root: directory, workdir: options.Directory, recovered: plan.Recovered()}
		if updatesForks(decisions) {
			f.status("Reading forked rule versions...")
		}
		preview, err := plan.Preview(cmd.Context(), decisions)
		if err != nil {
			return err
		}
		yes, _ := cmd.Flags().GetBool("yes")
		interactive := !yes && f.interactive()
		if !yes && !interactive {
			output.report = updateReport(preview, false, false, location)
			return nil
		}
		// A terminal shows the preview once, before the questions, so the report after them doesn't repeat it.
		asked := interactive && preview.Moves()
		if asked {
			flagged := len(decisions)
			if decisions, err = askUpdateDecisions(f, plan, preview, decisions); err != nil {
				return location.promptFailure(err)
			}
			// Answers change the update, so people confirm what they'll get.
			if len(decisions) > flagged {
				if updatesForks(decisions[flagged:]) {
					f.status("Reading forked rule versions...")
				}
				shown := preview
				if preview, err = plan.Preview(cmd.Context(), decisions); err != nil {
					return err
				}
				f.introduction += "\n" + answersSummary(preview.Sources)
				// Keeping a rule can move the skills to another revision, whose scripts people read before consenting.
				f.introduction += answeredSkills(shown, preview)
			}
			f.introduction += "\n"
			answer, err := askChoice(f, "Apply the update?", [2]string{"yes", "no"})
			if err != nil {
				return location.promptFailure(err)
			}
			if answer != "yes" {
				output.report = updateReport(preview, true, true, location)
				return nil
			}
		}
		f.status("Fetching the update...")
		result, err := plan.Apply(cmd.Context(), decisions)
		if err != nil {
			return err
		}
		result.FileChanges = withExcludedLinkWarnings(result.FileChanges)
		output.report = updateReport(result, false, asked, location)
		return nil
	}
	return cmd
}

// updatesForks reports whether decisions replace a fork, which reads the forked version from its library.
func updatesForks(decisions []project.UpdateDecision) bool {
	return slices.ContainsFunc(decisions, func(decision project.UpdateDecision) bool { return decision.Kind == project.DecisionUpdateFork })
}

// updateTargets parses SOURCE and SOURCE:RULE arguments; whether each names a configured source and an imported
// rule is checked against the project.
func updateTargets(args []string) ([]imports.UpdateTarget, error) {
	targets := []imports.UpdateTarget{}
	for _, arg := range args {
		source, rule, scoped := strings.Cut(arg, ":")
		if source == "" {
			return nil, fmt.Errorf("%q: expected SOURCE or SOURCE:RULE, such as team or team:techs/go/errors", arg)
		}
		if scoped {
			if err := librarytree.ValidateRuleID(rule, arg); err != nil {
				if skills.ValidateID(rule, arg) != nil {
					return nil, err
				}
				// A path under a group may be a malformed rule ID or a non-library skill in a folder of that name, so
				// it keeps the rule error and adds the skill guidance.
				if group := strings.ToLower(rule); strings.HasPrefix(group, "techs/") || strings.HasPrefix(group, "practices/") {
					return nil, fmt.Errorf("%w. If %s is a skill, run trailguide project update %s: a source's skills all come from one revision, so they update only together", err, rule, source)
				}
				return nil, fmt.Errorf("%s: SOURCE:RULE names one rule, such as team:techs/go/errors, and %s isn't a rule ID. A source's skills all come from one revision, so they update only together: run trailguide project update %s", arg, rule, source)
			}
		}
		targets = append(targets, imports.UpdateTarget{Source: source, Rule: rule})
	}
	return targets, nil
}

// flagDecisions turns --keep and --exclude into decisions with --reason, and --update-fork and --merge-fork into
// decisions without one, ignoring repeated values. --reason is required with --keep or --exclude and accepted only
// with one of them, and two of the flags naming one rule or skill fail.
func flagDecisions(keep, exclude, forks, merges []string, reason string) ([]project.UpdateDecision, error) {
	if len(keep)+len(exclude) > 0 && reason == "" {
		return nil, fmt.Errorf("--keep and --exclude require --reason, which is recorded with each pin and exclusion")
	}
	if reason != "" && len(keep)+len(exclude) == 0 {
		return nil, fmt.Errorf("--reason requires --keep or --exclude")
	}
	decisions := []project.UpdateDecision{}
	seen := map[string]bool{}
	// decided maps each SOURCE:RULE to the flag that decided it, so two flags can't decide one rule.
	decided := map[string]string{}
	for _, group := range []struct {
		flag   string
		kind   project.UpdateDecisionKind
		values []string
	}{{"--keep", project.DecisionKeep, keep}, {"--exclude", project.DecisionExclude, exclude}, {"--update-fork", project.DecisionUpdateFork, forks}, {"--merge-fork", project.DecisionMergeFork, merges}} {
		for _, value := range group.values {
			source, rule, ok := strings.Cut(value, ":")
			if group.kind == project.DecisionMergeFork {
				if !ok || source == "" || !forkedSkillID(rule) {
					return nil, fmt.Errorf("%s %q: expected SOURCE:SKILL, such as team:skills/build-feature", group.flag, value)
				}
			} else if !ok || source == "" {
				return nil, fmt.Errorf("%s %q: expected SOURCE:RULE, such as team:techs/go/errors", group.flag, value)
			} else if err := librarytree.ValidateRuleID(rule, group.flag+" "+value); err != nil {
				// A skill ID passes on, so the project names the flag a forked skill takes.
				if group.kind != project.DecisionUpdateFork || !forkedSkillID(rule) {
					return nil, err
				}
			}
			if seen[group.flag+value] {
				continue
			}
			if other, ok := decided[value]; ok {
				return nil, fmt.Errorf("%s and %s both name %s; decide each rule once", other, group.flag, value)
			}
			decided[value] = group.flag
			seen[group.flag+value] = true
			decision := project.UpdateDecision{Source: source, Rule: rule, Kind: group.kind, Reason: reason}
			if group.kind == project.DecisionUpdateFork || group.kind == project.DecisionMergeFork {
				decision.Reason = ""
			}
			decisions = append(decisions, decision)
		}
	}
	return decisions, nil
}

// forkedSkillID reports whether id can name a skill a source forks: a skill ID, such as skills/build-feature, or the
// ID of a skill the source's library curates, such as pocock:skills/to-spec.
func forkedSkillID(id string) bool {
	_, skill, _ := strings.Cut(id, ":")
	if skill == "" {
		skill = id
	}
	return skills.ValidateID(skill, id) == nil
}

// updateIntroduction is the preview a terminal shows before it asks about the update: the sources' changes, the
// minimum raise and its warnings, and the scripts warning, so people read what the update breaks and the scripts it
// brings before they decide.
func updateIntroduction(preview project.UpdateResult) string {
	var intro strings.Builder
	formatUpdatePreview(&intro, preview.Sources)
	formatMinimumRaise(&intro, preview)
	for _, warning := range preview.RaiseWarnings {
		fmt.Fprintf(&intro, "\nWarning: %s\n", warning)
	}
	if warning := preview.ScriptsWarning(); warning != "" {
		fmt.Fprintf(&intro, "\nWarning: %s\n", warning)
	}
	return intro.String()
}

// askUpdateDecisions shows the preview and asks about each major change, retirement, new rule, and replaced rule the
// flags didn't decide, naming the local files that replacing each fork of plan overwrites. It returns the flags'
// decisions followed by one for each rule kept, excluded, or whose fork is replaced. The preview stays in
// f.introduction when there is nothing to ask.
func askUpdateDecisions(f *authoringFlags, plan *project.UpdatePlan, preview project.UpdateResult, decisions []project.UpdateDecision) ([]project.UpdateDecision, error) {
	f.introduction = updateIntroduction(preview)
	for _, source := range preview.Sources {
		for _, row := range source.Rules {
			// A pin keeps a rule where it is, except that a fork can still follow the newest version.
			if row.Decision != "" || row.Pin != nil && row.Change != imports.UpdateReplaced {
				continue
			}
			// Each question names its rule on a line of its own, so prompts stay short enough not to wrap.
			name := source.Name + ":" + row.ID
			var context, question, reasonLabel string
			var choices [2]string
			switch row.Change {
			case imports.UpdateMajor:
				choices = [2]string{"adopt", "keep"}
				context = fmt.Sprintf("%s: major change, %s -> %s.", name, row.From, row.To)
				question = fmt.Sprintf("Adopt it, or keep %s?", row.From)
				reasonLabel = "Reason for keeping it:"
			case imports.UpdateRetired:
				choices = [2]string{"drop", "keep"}
				context = fmt.Sprintf("%s: retired.", name)
				question = fmt.Sprintf("Drop it, or keep %s?", row.From)
				reasonLabel = "Reason for keeping it:"
			case imports.UpdateNew:
				choices = [2]string{"add", "exclude"}
				context = fmt.Sprintf("%s: new rule, %s.", name, row.To)
				question = "Add it, or exclude it?"
				reasonLabel = "Reason for excluding it:"
			case imports.UpdateReplaced:
				version := row.ReviewedVersion()
				choices = [2]string{"later", "replace"}
				context = fmt.Sprintf("%s: replaced by %s, with library changes up to %s.", name, row.LocalRule, version)
				if replaced := plan.ReplacedFiles(row.LocalRule); len(replaced) > 0 {
					context += fmt.Sprintf("\nReplacing your rule with a fork of %s replaces or removes:\n  %s", version, strings.Join(replaced, "\n  "))
				}
				question = fmt.Sprintf("Review later, or replace your local rule with %s?", version)
			default:
				continue
			}
			f.introduction += "\n" + context + "\n"
			answer, err := askChoice(f, question, choices)
			if err != nil {
				return nil, err
			}
			if answer == choices[0] {
				continue
			}
			if row.Change == imports.UpdateReplaced {
				decisions = append(decisions, project.UpdateDecision{Source: source.Name, Rule: row.ID, Kind: project.DecisionUpdateFork})
				continue
			}
			reason, err := f.askValidated(reasonLabel, func(value string) error {
				if value == "" {
					return fmt.Errorf("give a reason to record with the decision")
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			kind := project.DecisionKeep
			if row.Change == imports.UpdateNew {
				kind = project.DecisionExclude
			}
			decisions = append(decisions, project.UpdateDecision{Source: source.Name, Rule: row.ID, Kind: kind, Reason: reason})
		}
		for _, row := range forkedSkillRows(source) {
			if row.Decision != "" || row.Merge != imports.SkillMergeNoConflicts {
				continue
			}
			f.introduction += fmt.Sprintf("\n%s:%s: forked as %s. Merging the source's changes up to commit %s\ninto it finds no text conflicts with your edits. Changes to different lines can\nstill contradict each other, so review the merged skill.\n", source.Name, row.ID, row.LocalSkill, shortCommit(row.To))
			answer, err := askChoice(f, "Review later, or merge them into your skill?", [2]string{"later", "merge"})
			if err != nil {
				return nil, err
			}
			if answer == "merge" {
				decisions = append(decisions, project.UpdateDecision{Source: source.Name, Rule: row.ID, Kind: project.DecisionMergeFork})
			}
		}
	}
	return decisions, nil
}

// forkedSkillRows returns the source's rows for the skills it forks, its own and its curated sources'.
func forkedSkillRows(source imports.SourceUpdate) []imports.ReplacedSkillUpdate {
	rows := slices.Clone(source.ReplacedSkills)
	for _, curated := range source.Curated {
		rows = append(rows, curated.Replaced...)
	}
	return rows
}

// askChoice asks question until the answer is one of the two choices, or its first letter, ignoring case.
func askChoice(f *authoringFlags, question string, choices [2]string) (string, error) {
	var chosen string
	_, err := f.askValidated(fmt.Sprintf("%s [%s/%s]:", question, choices[0], choices[1]), func(value string) error {
		for _, choice := range choices {
			if strings.EqualFold(value, choice) || strings.EqualFold(value, choice[:1]) {
				chosen = choice
				return nil
			}
		}
		return fmt.Errorf("%q isn't one of the answers; answer %s or %s, or %s or %s for short", value, choices[0], choices[1], choices[0][:1], choices[1][:1])
	})
	return chosen, err
}

// answersSummary lists the rows the user's answers decided: each kept rule with the version a new pin keeps it
// at, and each excluded new rule, with the reasons, and each replaced fork, with the files it replaces and removes.
func answersSummary(sources []imports.SourceUpdate) string {
	var out strings.Builder
	out.WriteString("Your answers:\n")
	for _, source := range sources {
		for _, row := range source.Rules {
			switch row.Decision {
			case "keep":
				fmt.Fprintf(&out, "  Keep %s:%s at %s.\n    Reason: %s\n", source.Name, row.ID, row.From, row.Reason)
			case "exclude":
				fmt.Fprintf(&out, "  Exclude %s:%s.\n    Reason: %s\n", source.Name, row.ID, row.Reason)
			case "update-fork":
				fmt.Fprintf(&out, "  Replace your rule for %s:%s with a fork of %s", source.Name, row.ID, row.ReviewedVersion())
				files := forkFileLines(row)
				if len(files) == 0 {
					out.WriteString(".\n")
					continue
				}
				out.WriteString(":\n")
				for _, line := range files {
					fmt.Fprintf(&out, "    %s\n", line)
				}
			}
		}
		for _, row := range forkedSkillRows(source) {
			if row.Decision == "merge-fork" {
				fmt.Fprintf(&out, "  Merge the changes to %s:%s, up to commit %s, into %s.\n", source.Name, row.ID, shortCommit(row.To), row.LocalSkill)
			}
		}
	}
	return out.String()
}

// updateLocation is where an update ran: the project at root, from workdir, and whether planning first recovered an
// interrupted earlier command, which changed files even when the update writes none.
type updateLocation struct {
	root, workdir string
	recovered     bool
}

// promptFailure reports an interrupted or ended prompt, saying that recovery changed files when it did.
func (l updateLocation) promptFailure(err error) error {
	if !l.recovered {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return &project.UnchangedError{Err: err, Recovered: true}
	}
	var invalid *usageError
	if errors.As(err, &invalid) {
		return usage(errors.New("terminal input ended. " + project.WriteOutcome(true)))
	}
	return err
}

// updateReport shows the preview, unless shown says a terminal already showed it, and, once applied, the files
// the update changed in the project, relative to the working directory. A preview explains how to apply it;
// cancelled says the user declined to. Saying what was written, it reports a recovery that planning did first.
func updateReport(result project.UpdateResult, cancelled, shown bool, location updateLocation) commandReport {
	var out strings.Builder
	if !shown {
		formatUpdatePreview(&out, result.Sources)
		formatMinimumRaise(&out, result)
	}
	// An update that applied nothing, not even a refresh of the managed guide, reads like a preview with nothing to
	// apply.
	nothing := result.Applied && !result.Moves() && len(result.Added)+len(result.Changed)+len(result.Removed) == 0 && result.Guide == nil
	if result.Applied && !nothing {
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(projectChangesReport("update", result.FileChanges, location.root, location.workdir).human)
		return commandReport{value: result, human: out.String()}
	}
	// A blank line sets the warnings apart from the preview, as a terminal shows them before its questions.
	if out.Len() > 0 && len(result.Warnings) > 0 {
		out.WriteByte('\n')
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&out, "Warning: %s\n", warning)
	}
	written := project.WriteOutcome(location.recovered)
	switch {
	case cancelled:
		out.WriteString("Update cancelled. " + written + "\n")
	case result.Moves() && !nothing && location.recovered:
		out.WriteString("\nThis is a preview. " + written + "\nTo apply it, run the command again with --yes, or in a terminal to answer each\nquestion and confirm.\n")
	case result.Moves() && !nothing:
		out.WriteString("\nThis is a preview; no files were written. To apply it, run the command again\nwith --yes, or in a terminal to answer each question and confirm.\n")
	case out.Len() > 0:
		out.WriteString("\nNo rule updates are available. " + written + "\n")
	default:
		out.WriteString("No rule updates are available. " + written + "\n")
	}
	return commandReport{value: result, human: out.String()}
}

// formatMinimumRaise says, after the preview's sources, that the update raises minTrailguideVersion, which other
// people's older Trail Guide releases then refuse.
func formatMinimumRaise(out *strings.Builder, result project.UpdateResult) {
	if result.SetsMinimumVersion == "" {
		return
	}
	if out.Len() > 0 {
		out.WriteByte('\n')
	}
	verb := "Sets"
	if result.Applied {
		verb = "Set"
	}
	fmt.Fprintf(out, "%s minTrailguideVersion to %s in .trailguide/config.yaml, so older releases\nof trailguide refuse this project.\n", verb, result.SetsMinimumVersion)
}

// updateKindWidth fits the longest change name, replaced.
const updateKindWidth = len(imports.UpdateReplaced)

// formatUpdatePreview lists each source's rule rows with the change, rule ID, and versions in aligned columns, then
// its skill rows with the change and skill ID, each row's details indented beneath them, including a change of the
// skill's level, the level changes of skills without a row of their own, and any move of its shared
// files. It writes nothing when no source has
// anything to show, so the caller's summary stands alone.
func formatUpdatePreview(out *strings.Builder, sources []imports.SourceUpdate) {
	idWidth := 0
	listed := false
	for _, source := range sources {
		for _, row := range source.Rules {
			idWidth = max(idWidth, len(row.ID))
		}
		for _, row := range source.ReplacedSkills {
			idWidth = max(idWidth, len(row.ID))
		}
		for _, row := range source.SkillLevels {
			idWidth = max(idWidth, len(row.ID))
		}
		listed = listed || len(source.Rules) > 0 || len(source.Skills) > 0 || len(source.ReplacedSkills) > 0 || len(source.SkillLevels) > 0 || len(source.Curated) > 0 || source.Ref != "" || source.SharedFiles != nil
	}
	if !listed {
		return
	}
	indent := strings.Repeat(" ", 2+updateKindWidth+2)
	for _, source := range sources {
		out.WriteString(source.Name + "\n")
		switch {
		case source.FollowsRef && source.Commit != nil:
			fmt.Fprintf(out, "  %s: %s -> %s\n", source.Ref, shortCommit(source.Commit.From), shortCommit(source.Commit.To))
			if source.TagMoved {
				fmt.Fprintf(out, "%sThe tag %s now names a different commit than at the last update.\n", indent, source.Ref)
			}
		case source.FollowsRef:
			fmt.Fprintf(out, "  %s: no new commit.\n", source.Ref)
		case source.Ref != "":
			fmt.Fprintf(out, "  Imports %s with ref, so update doesn't move it.\n", source.Ref)
		case len(source.Rules) == 0 && len(source.Skills) == 0 && source.SharedFiles == nil && len(source.Curated) == 0:
			out.WriteString("  No rule updates.\n")
		}
		for _, row := range source.Rules {
			fmt.Fprintf(out, "  %-*s  %-*s  %s\n", updateKindWidth, row.Change, idWidth, row.ID, updateVersions(row))
			for _, line := range updateDetails(source.Name, row) {
				fmt.Fprintf(out, "%s%s\n", indent, line)
			}
		}
		levels := levelsByID(source.SkillLevels)
		formatSkillRows(out, source.Skills, levels)
		for _, row := range source.ReplacedSkills {
			fmt.Fprintf(out, "  %-*s  %-*s  %s\n", updateKindWidth, imports.UpdateReplaced, idWidth, row.ID, commitSpan(row.From, row.To))
			writeLevelLines(out, levels, row.ID)
			for _, line := range replacedSkillDetails(source.Name, row) {
				fmt.Fprintf(out, "%s%s\n", indent, line)
			}
		}
		formatLevelRows(out, source, idWidth)
		formatCuratedRows(out, source.Name, source.Curated, levels)
		if shared := source.SharedFiles; shared != nil {
			fmt.Fprintf(out, "  Shared files: release %d -> %d\n", shared.From, shared.To)
			if source.SetsRef != "" {
				fmt.Fprintf(out, "  Sets sources.%s.ref to %s in .trailguide/config.yaml.\n", source.Name, source.SetsRef)
			}
		}
		if source.Available != 0 {
			fmt.Fprintf(out, "  Release %d is available; run trailguide project update %s again to move to it.\n", source.Available, source.Name)
		}
	}
}

// formatCuratedRows lists the sources library curates skills from, each on a line naming the library, the curated
// source, and how its revision moves, with the rows of its skills, each with its level change from levels, if any,
// and of the curated skills the project replaces, beneath it.
func formatCuratedRows(out *strings.Builder, library string, rows []imports.CuratedUpdate, levels map[string]imports.SkillLevelUpdate) {
	indent := strings.Repeat(" ", 2+updateKindWidth+2)
	for _, row := range rows {
		fmt.Fprintf(out, "  %s curates %s  %s\n", library, row.Name, curatedSpan(row))
		if row.TagMoved {
			fmt.Fprintf(out, "%sThe tag %s now names a different commit than at the last update.\n", indent, row.Ref)
		}
		formatSkillRows(out, row.Changes, levels)
		for _, replaced := range row.Replaced {
			fmt.Fprintf(out, "  %-*s  %s  %s\n", updateKindWidth, imports.UpdateReplaced, replaced.ID, commitSpan(replaced.From, replaced.To))
			writeLevelLines(out, levels, replaced.ID)
			for _, line := range replacedSkillDetails(library, replaced) {
				fmt.Fprintf(out, "%s%s\n", indent, line)
			}
		}
	}
}

// formatLevelRows lists the source's skill level changes that no skill or replaced skill row of the source or its
// curated sources shows, since the skill's files don't change, as rows of their own with the skill ID, in a column idWidth wide,
// and the levels it moves between, each row's detail indented beneath it.
func formatLevelRows(out *strings.Builder, source imports.SourceUpdate, idWidth int) {
	shown := map[string]bool{}
	for _, row := range source.Skills {
		shown[row.ID] = true
	}
	for _, row := range source.ReplacedSkills {
		shown[row.ID] = true
	}
	for _, curated := range source.Curated {
		for _, row := range curated.Changes {
			shown[row.ID] = true
		}
		for _, row := range curated.Replaced {
			shown[row.ID] = true
		}
	}
	indent := strings.Repeat(" ", 2+updateKindWidth+2)
	for _, row := range source.SkillLevels {
		if shown[row.ID] {
			continue
		}
		fmt.Fprintf(out, "  %-*s  %-*s  %s\n", updateKindWidth, "level", idWidth, row.ID, levelSpan(row))
		fmt.Fprintf(out, "%s%s\n", indent, levelDetail(row))
	}
}

// writeLevelLines writes, indented under the row of skill id, the skill's level change from levels, if it has one.
func writeLevelLines(out *strings.Builder, levels map[string]imports.SkillLevelUpdate, id string) {
	if level, ok := levels[id]; ok {
		indent := strings.Repeat(" ", 2+updateKindWidth+2)
		fmt.Fprintf(out, "%slevel: %s\n%s%s\n", indent, levelSpan(level), indent, levelDetail(level))
	}
}

// levelsByID returns rows by skill ID.
func levelsByID(rows []imports.SkillLevelUpdate) map[string]imports.SkillLevelUpdate {
	levels := map[string]imports.SkillLevelUpdate{}
	for _, row := range rows {
		levels[row.ID] = row
	}
	return levels
}

// levelSpan shows the levels a skill's level row moves between, with "none" for a side where the library has no
// such skill.
func levelSpan(row imports.SkillLevelUpdate) string {
	from, to := cmp.Or(row.From, "none"), cmp.Or(row.To, "none")
	return from + " -> " + to
}

// levelDetail returns the line shown under a skill's level row: what the change does to the project.
func levelDetail(row imports.SkillLevelUpdate) string {
	switch {
	case row.From == "":
		return "New required skill: it installs whatever skills.include says."
	case row.To == "":
		return "The library removes this required skill, so it's no longer installed."
	case row.To == skills.LevelRequired:
		return "Now required: it installs whatever skills.include says, and the project can't exclude or fork it."
	case row.From == skills.LevelRequired:
		return "No longer required: it stays installed only when skills.include selects it."
	case row.To == skills.LevelRecommended:
		return "Now recommended: it installs when skills.include has recommended."
	}
	return `No longer recommended: it installs only when skills.include names it or has "*".`
}

// formatSkillRows lists a source's skill rows with the change and skill ID, each row's details indented beneath them:
// first its level change from levels, if any, then the details of its files.
func formatSkillRows(out *strings.Builder, rows []imports.SkillUpdate, levels map[string]imports.SkillLevelUpdate) {
	indent := strings.Repeat(" ", 2+updateKindWidth+2)
	for _, row := range rows {
		fmt.Fprintf(out, "  %-*s  %s\n", updateKindWidth, row.Change, row.ID)
		writeLevelLines(out, levels, row.ID)
		for _, line := range skillUpdateDetails(row) {
			fmt.Fprintf(out, "%s%s\n", indent, line)
		}
	}
}

// answeredSkills lists the skill, level, and curated rows of each source whose rows in after, the preview with the
// answers, differ from those in before, the preview shown before the questions, then after's scripts warning, or
// returns empty when no source's rows differ.
func answeredSkills(before, after project.UpdateResult) string {
	shown := map[string]imports.SourceUpdate{}
	for _, source := range before.Sources {
		shown[source.Name] = source
	}
	var out strings.Builder
	for _, source := range after.Sources {
		if reflect.DeepEqual(shown[source.Name].Skills, source.Skills) && reflect.DeepEqual(shown[source.Name].SkillLevels, source.SkillLevels) && reflect.DeepEqual(shown[source.Name].Curated, source.Curated) {
			continue
		}
		if out.Len() == 0 {
			out.WriteString("\nYour answers change the update's skills:\n")
		}
		out.WriteString(source.Name + "\n")
		if len(source.Skills) == 0 && len(source.SkillLevels) == 0 && len(source.Curated) == 0 {
			out.WriteString("  No skill changes.\n")
		}
		levels := levelsByID(source.SkillLevels)
		formatSkillRows(&out, source.Skills, levels)
		idWidth := 0
		for _, row := range source.SkillLevels {
			idWidth = max(idWidth, len(row.ID))
		}
		// This list leaves out the source's own replaced rows, so their level changes get rows of their own.
		unreplaced := source
		unreplaced.ReplacedSkills = nil
		formatLevelRows(&out, unreplaced, idWidth)
		formatCuratedRows(&out, source.Name, source.Curated, levels)
	}
	if warning := after.ScriptsWarning(); out.Len() > 0 && warning != "" {
		fmt.Fprintf(&out, "\nWarning: %s\n", warning)
	}
	return out.String()
}

// skillUpdateDetails returns the lines shown under a skill's row: the files a changed skill's update changes, then
// one line for each script the update adds or changes.
func skillUpdateDetails(row imports.SkillUpdate) []string {
	lines := []string{}
	if len(row.Files) > 0 {
		lines = append(lines, "Changed: "+strings.Join(row.Files, ", "))
	}
	for _, script := range row.Scripts {
		lines = append(lines, "script "+string(script.Change)+": "+script.Path)
	}
	return lines
}

// shortCommit returns the first 12 digits of a commit SHA, enough to tell commits apart in a preview.
func shortCommit(commit string) string { return commit[:min(len(commit), 12)] }

// commitSpan shows the commits a skill row moves between, or only to when from is empty.
func commitSpan(from, to string) string {
	if from == "" || from == to {
		return "commit " + shortCommit(to)
	}
	return "commit " + shortCommit(from) + " -> " + shortCommit(to)
}

// curatedSpan shows how a curated source's revision moves: between library releases, such as release 3 -> 4, or
// between commits after the ref they were resolved from, such as main: 3f9c2e1a5b7d -> 8a41d07c2e9f, with "none" for
// a side where the project takes no skills from the source; or only the ref when the commit doesn't move.
func curatedSpan(row imports.CuratedUpdate) string {
	ref := cmp.Or(row.Ref, "newest release")
	switch {
	case row.Release != nil:
		return fmt.Sprintf("release %d -> %d", row.Release.From, row.Release.To)
	case row.Commit == nil:
		return ref
	}
	from, to := "none", "none"
	if row.Commit.From != "" {
		from = shortCommit(row.Commit.From)
	}
	if row.Commit.To != "" {
		to = shortCommit(row.Commit.To)
	}
	return ref + ": " + from + " -> " + to
}

// replacedSkillDetails returns the lines shown under a row of source for a forked skill: the fork, what the row
// compares with, and the files that changed, or that the source no longer has the skill, then whether the changes
// merge into the fork, and how to merge them.
func replacedSkillDetails(source string, row imports.ReplacedSkillUpdate) []string {
	lines := []string{}
	switch {
	case row.Removed:
		return append(lines, "Your skill: "+row.LocalSkill+".", "The source no longer has this skill at commit "+shortCommit(row.To)+". Your skill stays installed; the update doesn't change it.")
	case row.BasedOn != "":
		lines = append(lines, "Your skill: "+row.LocalSkill+", based on commit "+shortCommit(row.BasedOn)+".", "Changed since commit "+shortCommit(row.BasedOn)+", the copy your skill is based on:")
	default:
		lines = append(lines, "Your skill: "+row.LocalSkill+".", "Changed since the imported commit "+shortCommit(row.From)+"; your skill may already have some of these changes:")
	}
	for _, file := range row.Files {
		lines = append(lines, "  "+file)
	}
	field := "sources." + source + ".skills.forks." + row.ID + ".basedOn"
	switch {
	case row.Decision == "merge-fork":
		return append(lines, "The update merges these changes into your skill, keeping your edits, and sets", field+" to commit "+shortCommit(row.To)+".", "Review the merged skill: changes to different lines can still contradict each other.")
	case row.Merge == imports.SkillMergeNoConflicts:
		return append(lines, "Merging them into your skill finds no text conflicts with your edits. To merge", "them and set basedOn to commit "+shortCommit(row.To)+", pass --merge-fork "+source+":"+row.ID+".")
	case row.Merge == imports.SkillMergeConflicts:
		lines = append(lines, "Your edits and these changes touch the same lines of:")
		for _, file := range row.Conflicts {
			lines = append(lines, "  "+file)
		}
		return append(lines, "Merge them into your skill by hand, then set "+field, fmt.Sprintf("to %q.", row.To))
	}
	return append(lines, "Compare them with your skill: the update doesn't change it.")
}

// updateVersions shows where a row's rule moves, or the one version it has.
func updateVersions(row imports.RuleUpdate) string {
	switch {
	case row.From != nil && row.To != nil && *row.To == *row.From:
		return row.From.String()
	case row.Change == imports.UpdateNew:
		return row.To.String()
	case row.Change == imports.UpdateRetired && *row.From != *row.LastVersion:
		return row.From.String() + ", last version " + row.LastVersion.String()
	case row.To == nil:
		return row.From.String()
	}
	return row.From.String() + " -> " + row.To.String()
}

// updateDetails returns the lines shown under a row of source: its replacement, summaries, local rule, pin, and
// decision.
func updateDetails(source string, row imports.RuleUpdate) []string {
	lines := []string{}
	switch {
	case row.ReplacementLocalRule != "" && row.ReplacementRetired:
		lines = append(lines, "Replaced by "+row.ReplacedBy+", which the library also retired, in favor of "+row.CurrentReplacement+", which your project already replaces with "+row.ReplacementLocalRule+".")
	case row.ReplacementLocalRule != "":
		lines = append(lines, "Replaced by "+row.ReplacedBy+", which your project already replaces with "+row.ReplacementLocalRule+".")
	case row.ReplacementRetired && row.CurrentReplacement != "":
		lines = append(lines, "Replaced by "+row.ReplacedBy+", which the library also retired, in favor of "+row.CurrentReplacement+".")
	case row.ReplacementRetired:
		lines = append(lines, "Replaced by "+row.ReplacedBy+", which the library also retired without a replacement.")
	case row.ReplacedBy != "":
		lines = append(lines, "Replaced by "+row.ReplacedBy+".")
	}
	// A row spanning several versions labels each summary with its version, so people can tell which change is which.
	spans := slices.ContainsFunc(row.SummaryVersions, func(version trailguide.RuleVersion) bool { return version != row.SummaryVersions[0] })
	for i, summary := range row.Summaries {
		if spans && i < len(row.SummaryVersions) {
			summary = row.SummaryVersions[i].String() + ": " + summary
		}
		lines = append(lines, summary)
	}
	forked := row.Decision == "update-fork"
	switch {
	case row.BasedOn != nil:
		lines = append(lines, "Your rule: "+row.LocalRule+", based on "+row.BasedOn.String()+".")
		if !forked {
			lines = append(lines, "Changes since "+row.BasedOn.String()+", the version your rule is based on.")
		}
	case row.LocalRule != "":
		lines = append(lines, "Your rule: "+row.LocalRule+".")
		// Without basedOn, the row can only compare with the imported version.
		if !forked {
			lines = append(lines, "Changes since the imported version "+row.From.String()+"; your rule may already have some.")
		}
	}
	if row.Change == imports.UpdateReplaced && !forked {
		lines = append(lines, "To replace your rule and its assets with a fork of "+row.ReviewedVersion().String()+",", "pass --update-fork "+source+":"+row.ID+".")
	}
	switch {
	case row.Change == imports.UpdatePinned:
		lines = append(lines, "Newest version: "+row.Newest.String()+".", "Reason: "+row.Pin.Reason)
	case row.Pin != nil:
		lines = append(lines, "Your pin keeps it at "+row.Pin.Version.String()+".", "Reason: "+row.Pin.Reason)
	case row.Decision == "keep":
		lines = append(lines, "Kept at "+row.From.String()+" by a new pin.", "Reason: "+row.Reason)
	case row.Decision == "exclude":
		lines = append(lines, "Excluded by a new exclusion.", "Reason: "+row.Reason)
	}
	// A pin keeps only the imported copy, so the fork's replacement shows beside it.
	if forked {
		lines = append(lines, "Your rule becomes a fork of "+row.ReviewedVersion().String()+".")
		lines = append(lines, forkFileLines(row)...)
	}
	return lines
}

// forkFileLines names, one per line, each local file a row's fork update replaces with the new fork's copy, then
// each it removes because the new fork doesn't have it.
func forkFileLines(row imports.RuleUpdate) []string {
	lines := []string{}
	for _, file := range row.Overwrites {
		lines = append(lines, "Replaces "+file)
	}
	for _, file := range row.Removes {
		lines = append(lines, "Removes "+file)
	}
	return lines
}
