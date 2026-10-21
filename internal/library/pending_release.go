// Match pending change notes with the rules that changed since the latest library release, and plan the next one.

package library

import (
	"maps"
	"slices"
	"strconv"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// PendingRelease previews the next library release: each rule a pending change note names, or, before the
// first library release, every rule.
type PendingRelease struct {
	Release int `json:"release"`
	// Rules is sorted by rule ID and empty when no rule has a pending change.
	Rules []PendingRule `json:"rules"`
	// LibraryFiles lists, in path order, the library-wide files in the working tree that differ from the latest
	// library release, including deleted ones, which the next library release would publish once committed. It is
	// empty before the first library release, which adds every file.
	LibraryFiles []string `json:"libraryFiles"`
}

// PendingRule is one rule's change in the next library release, named as the release record and the rows of
// trailguide project update name them.
type PendingRule struct {
	ID     string            `json:"id"`
	Change trailguide.Change `json:"change"`
	// From is the version the rule had before this library release; it is nil for a new or retired rule.
	From *trailguide.RuleVersion `json:"from,omitempty"`
	// To is the version this library release publishes; it is nil for a retired rule.
	To *trailguide.RuleVersion `json:"to,omitempty"`
	// LastVersion is a retired rule's final version, and nil for every other rule.
	LastVersion *trailguide.RuleVersion `json:"lastVersion,omitempty"`
	// ReplacedBy names a retired rule's replacement, when it has one.
	ReplacedBy string `json:"replacedBy,omitempty"`
	// Summaries holds one summary per change note that named the rule, in note order; it is never empty.
	Summaries []string `json:"summaries"`
}

// pendingNote is a change note added since the latest library release.
type pendingNote struct {
	path string
	note rules.ChangeNote
}

// libraryChanges is the state library check compares: what the latest library release published and
// what the working tree holds now.
type libraryChanges struct {
	history releaseHistory
	// current lists the working tree's rule IDs in sorted order.
	current []string
	// changed reports, for each current rule the latest library release published, whether its versioned content differs.
	changed map[string]bool
	// pending is sorted by path.
	pending []pendingNote
}

// firstReleaseSummary is every rule's summary in the first library release, which publishes each rule as new
// without change notes. A release record requires a summary for every change.
const firstReleaseSummary = "Add the rule."

// releasePlan is what the next library release would record, before library-wide files are compared.
type releasePlan struct {
	release int
	// versions holds every current rule's version after the library release.
	versions map[string]trailguide.RuleVersion
	// changes holds each new or changed rule, with one summary per note that named it, in note order;
	// the first library release, which has no notes, gives every rule firstReleaseSummary.
	changes map[string]trailguide.RecordedChange
	retired map[string]trailguide.RetiredRule
}

// review returns every mismatch between the pending notes and the rule changes since the latest library
// release, in rule and note order. It returns none before the first library release, when rules need no notes.
func (c libraryChanges) review() []string {
	latest := c.history.latest
	if latest == nil {
		return nil
	}
	current := map[string]bool{}
	for _, id := range c.current {
		current[id] = true
	}
	named, retiring := c.namedRules()
	var problems []string
	for _, id := range c.current {
		version, published := latest.record.Rules[id]
		switch release, retired := c.history.retired[id]; {
		case retired:
			problems = append(problems, id+" reuses the ID of a rule that release/"+strconv.Itoa(release)+" retired. Retired IDs can't be reused; give the rule a new ID.")
		case !published && !named[id]:
			problems = append(problems, id+" is a new rule, and no pending change note names it. Record it with: trailguide library change "+id+" --summary '<what the rule adds>'")
		case published && c.changed[id] && !named[id]:
			problems = append(problems, id+" changed since "+latest.tagName()+", where its version is "+version.String()+", and no pending change note names it. Record it with: trailguide library change "+id+" --bump <major|minor|patch> --summary '<what changed>'")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(latest.record.Rules)) {
		if !current[id] && len(retiring[id]) == 0 {
			problems = append(problems, id+", version "+latest.record.Rules[id].String()+", was deleted, and no pending change note retires it. Restore it, or record the retirement with: trailguide library change "+id+" --retire --summary '<why it's retired>'")
		}
	}
	for _, pending := range c.pending {
		for _, id := range slices.Sorted(maps.Keys(pending.note.Rules)) {
			if problem := c.reviewEntry(pending.path, id, pending.note.Rules[id], current[id], len(retiring[id]) > 0); problem != "" {
				problems = append(problems, problem)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(retiring)) {
		replacements := map[string]bool{}
		for _, change := range retiring[id] {
			replacements[change.ReplacedBy] = true
		}
		if len(replacements) > 1 {
			problems = append(problems, "pending change notes name different replacements for "+id+"; keep one")
		}
	}
	return problems
}

// reviewEntry checks one note's change for one rule against the rule's state, returning a problem or "".
func (c libraryChanges) reviewEntry(path, id string, change rules.NoteChange, exists, retiring bool) string {
	latest := c.history.latest
	version, published := latest.record.Rules[id]
	release, retired := c.history.retired[id]
	prefix := path + " names " + id + ", which "
	switch {
	case change.Change == trailguide.ChangeRetired && exists:
		return prefix + "still exists. To retire it, delete its Markdown file and asset directory."
	case change.Change == trailguide.ChangeRetired && retired:
		return prefix + "release/" + strconv.Itoa(release) + " already retired."
	case change.Change == trailguide.ChangeRetired && !published:
		return prefix + "was never published, so it can't be retired. Remove it from the note."
	case change.Change == trailguide.ChangeRetired && change.ReplacedBy != "" && !slices.Contains(c.current, change.ReplacedBy):
		return path + " retires " + id + " in favor of " + change.ReplacedBy + ", which isn't a rule in the library."
	case change.Change == trailguide.ChangeRetired:
		return ""
	case !exists && !retiring:
		return prefix + "isn't a rule in the library and isn't being retired."
	case !exists:
		return ""
	case change.Change == trailguide.ChangeNew && published:
		return prefix + "already has version " + version.String() + ". Record it as major, minor, or patch."
	case change.Change != trailguide.ChangeNew && !published:
		return prefix + "has no version yet. Record it as new."
	case change.Change != trailguide.ChangeNew && !c.changed[id]:
		return prefix + "is unchanged since " + latest.tagName() + ", so the note is stale. Remove the rule from the note."
	}
	return ""
}

// namedRules reports which rules the pending notes name, and each retirement's changes by rule.
func (c libraryChanges) namedRules() (map[string]bool, map[string][]rules.NoteChange) {
	named := map[string]bool{}
	retiring := map[string][]rules.NoteChange{}
	for _, pending := range c.pending {
		for id, change := range pending.note.Rules {
			named[id] = true
			if change.Change == trailguide.ChangeRetired {
				retiring[id] = append(retiring[id], change)
			}
		}
	}
	return named, retiring
}

// plan computes the next library release from notes that passed review. Before the first library release,
// every current rule is new, with firstReleaseSummary as its summary. Several notes on one rule use the largest change; a retirement outweighs any other.
// It fails when a change would advance a version past the largest rule version number.
func (c libraryChanges) plan() (releasePlan, error) {
	plan := releasePlan{release: 1, versions: map[string]trailguide.RuleVersion{}, changes: map[string]trailguide.RecordedChange{}, retired: map[string]trailguide.RetiredRule{}}
	latest := c.history.latest
	if latest == nil {
		for _, id := range c.current {
			plan.versions[id] = trailguide.FirstRuleVersion
			plan.changes[id] = trailguide.RecordedChange{Change: trailguide.ChangeNew, Summaries: []string{firstReleaseSummary}}
		}
		return plan, nil
	}
	plan.release = latest.number + 1
	for _, id := range c.current {
		if version, ok := latest.record.Rules[id]; ok {
			plan.versions[id] = version
		}
	}
	changes := map[string]trailguide.Change{}
	summaries := map[string][]string{}
	// Every pending note's summary is listed, in note order, wherever its rule ends up.
	for _, pending := range c.pending {
		for id, change := range pending.note.Rules {
			summaries[id] = append(summaries[id], pending.note.Summary)
			if change.Change == trailguide.ChangeRetired {
				plan.retired[id] = trailguide.RetiredRule{LastVersion: latest.record.Rules[id], ReplacedBy: change.ReplacedBy}
			} else if previous, ok := changes[id]; ok {
				changes[id] = trailguide.LargerChange(previous, change.Change)
			} else {
				changes[id] = change.Change
			}
		}
	}
	for id, retired := range plan.retired {
		retired.Summaries = summaries[id]
		plan.retired[id] = retired
		delete(changes, id)
	}
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		recorded := trailguide.RecordedChange{Change: changes[id], Summaries: summaries[id]}
		next := trailguide.FirstRuleVersion
		if recorded.Change != trailguide.ChangeNew {
			from := latest.record.Rules[id]
			var err error
			if next, err = from.Next(recorded.Change); err != nil {
				return releasePlan{}, failure("version-limit", id+": "+err.Error(), nil)
			}
			recorded.From = &from
		}
		plan.versions[id] = next
		plan.changes[id] = recorded
	}
	return plan, nil
}

// record returns the release record that publishes the plan with libraryFiles, the changed library-wide files.
func (p releasePlan) record(libraryFiles []string) trailguide.ReleaseRecord {
	return trailguide.ReleaseRecord{Release: p.release, Rules: p.versions, Changes: p.changes, Retired: p.retired, LibraryFiles: libraryFiles}
}

// preview lists each changed, new, and retired rule in ID order.
func (p releasePlan) preview() PendingRelease {
	return PendingRelease{Release: p.release, Rules: releaseRules(p.record(nil)), LibraryFiles: []string{}}
}

// empty reports whether the plan changes, adds, and retires no rules.
func (p releasePlan) empty() bool {
	return len(p.changes) == 0 && len(p.retired) == 0
}
