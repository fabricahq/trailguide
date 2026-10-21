// Match pending change notes with the rules and skills that changed since the latest library release, and plan the
// next one.

package library

import (
	"maps"
	"slices"
	"strconv"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// PendingRelease previews the next library release: each rule and skill a pending change note names, or, before
// the first library release, every rule and skill.
type PendingRelease struct {
	Release int `json:"release"`
	// Rules is sorted by rule ID and empty when no rule has a pending change.
	Rules []PendingRule `json:"rules"`
	// Skills lists, sorted by skill ID, each skill with a pending change, and each skill whose level or fork changes
	// without one; it is empty when there are none.
	Skills []PendingSkill `json:"skills"`
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
	// currentSkills lists the IDs of the working tree's skills in sorted order, and changedSkills reports, for each
	// one history.skills published, whether its folder differs.
	currentSkills []string
	changedSkills map[string]bool
	// pending is sorted by path.
	pending []pendingNote
}

// firstReleaseSummary is every rule's summary in the first library release, which publishes each rule as new
// without change notes, and firstSkillSummary every skill's. A release record requires a summary for every change.
const (
	firstReleaseSummary = "Add the rule."
	firstSkillSummary   = "Add the skill."
)

// releasePlan is what the next library release would record, before library-wide files are compared.
type releasePlan struct {
	release int
	// versions holds every current rule's version after the library release.
	versions map[string]trailguide.RuleVersion
	// changes holds each new or changed rule, with one summary per note that named it, in note order;
	// the first library release, which has no notes, gives every rule firstReleaseSummary.
	changes map[string]trailguide.RecordedChange
	retired map[string]trailguide.RetiredRule
	// skills is the record's skills field, planned the same way; it is nil while the library has never had a skill.
	skills *trailguide.SkillRecord
}

// versionedKind is the view of libraryChanges for one kind of versioned item, rules or skills, that review and plan
// share.
type versionedKind struct {
	// noun names one, such as rule; remove says how to delete one, such as delete its folder; firstSummary is the
	// summary the first library release gives each.
	noun, remove, firstSummary string
	// baseline is the library release published versions come from, such as release/4, or empty when none has.
	baseline string
	// published holds the versions baseline published, and retired the release that retired each retired one.
	published map[string]trailguide.RuleVersion
	retired   map[string]int
	current   []string
	changed   map[string]bool
	// notes returns a note's entries of this kind.
	notes func(rules.ChangeNote) map[string]rules.NoteChange
}

// ruleKind returns the view of c for rules.
func (c libraryChanges) ruleKind() versionedKind {
	kind := versionedKind{noun: "rule", remove: "delete its Markdown file and asset directory", firstSummary: firstReleaseSummary, retired: c.history.retired, current: c.current, changed: c.changed,
		notes: func(note rules.ChangeNote) map[string]rules.NoteChange { return note.Rules }}
	if latest := c.history.latest; latest != nil {
		kind.baseline, kind.published = latest.tagName(), latest.record.Rules
	}
	return kind
}

// skillKind returns the view of c for skills, whose versions come from the newest library release that records them.
func (c libraryChanges) skillKind() versionedKind {
	kind := versionedKind{noun: "skill", remove: "delete its folder", firstSummary: firstSkillSummary, published: map[string]trailguide.RuleVersion{}, retired: c.history.retiredSkills, current: c.currentSkills, changed: c.changedSkills,
		notes: func(note rules.ChangeNote) map[string]rules.NoteChange { return note.Skills }}
	if base := c.history.skills; base != nil {
		kind.baseline, kind.published = base.tagName(), base.record.Versions
	}
	return kind
}

// review returns every mismatch between the pending notes and the rule and skill changes since the latest library
// release, rules first, each in ID and note order. It returns none before the first library release, when rules and
// skills need no notes.
func (c libraryChanges) review() []string {
	if c.history.latest == nil {
		return nil
	}
	return append(c.reviewKind(c.ruleKind()), c.reviewKind(c.skillKind())...)
}

// reviewKind is review for one kind of versioned item.
func (c libraryChanges) reviewKind(kind versionedKind) []string {
	current := map[string]bool{}
	for _, id := range kind.current {
		current[id] = true
	}
	named, retiring := c.named(kind)
	var problems []string
	for _, id := range kind.current {
		version, published := kind.published[id]
		switch release, retired := kind.retired[id]; {
		case retired:
			problems = append(problems, id+" reuses the ID of a "+kind.noun+" that release/"+strconv.Itoa(release)+" retired. Retired IDs can't be reused; give the "+kind.noun+" a new ID.")
		case !published && !named[id]:
			problems = append(problems, id+" is a new "+kind.noun+", and no pending change note names it. Record it with: trailguide library change "+id+" --summary '<what the "+kind.noun+" adds>'")
		case published && kind.changed[id] && !named[id]:
			problems = append(problems, id+" changed since "+kind.baseline+", where its version is "+version.String()+", and no pending change note names it. Record it with: trailguide library change "+id+" --bump <major|minor|patch> --summary '<what changed>'")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(kind.published)) {
		if !current[id] && len(retiring[id]) == 0 {
			problems = append(problems, id+", version "+kind.published[id].String()+", was deleted, and no pending change note retires it. Restore it, or record the retirement with: trailguide library change "+id+" --retire --summary '<why it's retired>'")
		}
	}
	for _, pending := range c.pending {
		entries := kind.notes(pending.note)
		for _, id := range slices.Sorted(maps.Keys(entries)) {
			if problem := c.reviewEntry(kind, pending.path, id, entries[id], current[id], len(retiring[id]) > 0); problem != "" {
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

// reviewEntry checks one note's change for one rule or skill against its state, returning a problem or "".
func (c libraryChanges) reviewEntry(kind versionedKind, path, id string, change rules.NoteChange, exists, retiring bool) string {
	version, published := kind.published[id]
	release, retired := kind.retired[id]
	prefix := path + " names " + id + ", which "
	switch {
	case change.Change == trailguide.ChangeRetired && exists:
		return prefix + "still exists. To retire it, " + kind.remove + "."
	case change.Change == trailguide.ChangeRetired && retired:
		return prefix + "release/" + strconv.Itoa(release) + " already retired."
	case change.Change == trailguide.ChangeRetired && !published:
		return prefix + "was never published, so it can't be retired. Remove it from the note."
	case change.Change == trailguide.ChangeRetired && change.ReplacedBy != "" && !slices.Contains(kind.current, change.ReplacedBy):
		return path + " retires " + id + " in favor of " + change.ReplacedBy + ", which isn't a " + kind.noun + " in the library."
	case change.Change == trailguide.ChangeRetired:
		return ""
	case !exists && !retiring:
		return prefix + "isn't a " + kind.noun + " in the library and isn't being retired."
	case !exists:
		return ""
	case change.Change == trailguide.ChangeNew && published:
		return prefix + "already has version " + version.String() + ". Record it as major, minor, or patch."
	case change.Change != trailguide.ChangeNew && !published:
		return prefix + "has no version yet. Record it as new."
	case change.Change != trailguide.ChangeNew && !kind.changed[id]:
		return prefix + "is unchanged since " + kind.baseline + ", so the note is stale. Remove the " + kind.noun + " from the note."
	}
	return ""
}

// namedRules reports which rules the pending notes name, and each retirement's changes by rule.
func (c libraryChanges) namedRules() (map[string]bool, map[string][]rules.NoteChange) {
	return c.named(c.ruleKind())
}

// named reports which rules or skills of kind the pending notes name, and each retirement's changes by ID.
func (c libraryChanges) named(kind versionedKind) (map[string]bool, map[string][]rules.NoteChange) {
	named := map[string]bool{}
	retiring := map[string][]rules.NoteChange{}
	for _, pending := range c.pending {
		for id, change := range kind.notes(pending.note) {
			named[id] = true
			if change.Change == trailguide.ChangeRetired {
				retiring[id] = append(retiring[id], change)
			}
		}
	}
	return named, retiring
}

// plan computes the next library release from notes that passed review. Before the first library release,
// every current rule and skill is new, with firstReleaseSummary or firstSkillSummary as its summary. Several notes on
// one rule or skill use the largest change; a retirement outweighs any other. It records skills once the library
// has a skill, or a library release recorded skills. It fails when a change would advance a version past the largest
// version number.
func (c libraryChanges) plan() (releasePlan, error) {
	plan := releasePlan{release: 1}
	if c.history.latest != nil {
		plan.release = c.history.latest.number + 1
	}
	var err error
	if plan.versions, plan.changes, plan.retired, err = c.planKind(c.ruleKind()); err != nil {
		return releasePlan{}, err
	}
	if len(c.currentSkills) > 0 || c.history.skills != nil {
		skills := &trailguide.SkillRecord{}
		if skills.Versions, skills.Changes, skills.Retired, err = c.planKind(c.skillKind()); err != nil {
			return releasePlan{}, err
		}
		plan.skills = skills
	}
	return plan, nil
}

// planKind is plan for one kind of versioned item: every current one's version, and the changes and retirements
// that lead to them.
func (c libraryChanges) planKind(kind versionedKind) (map[string]trailguide.RuleVersion, map[string]trailguide.RecordedChange, map[string]trailguide.RetiredRule, error) {
	versions, recorded, retired := map[string]trailguide.RuleVersion{}, map[string]trailguide.RecordedChange{}, map[string]trailguide.RetiredRule{}
	if c.history.latest == nil {
		for _, id := range kind.current {
			versions[id] = trailguide.FirstRuleVersion
			recorded[id] = trailguide.RecordedChange{Change: trailguide.ChangeNew, Summaries: []string{kind.firstSummary}}
		}
		return versions, recorded, retired, nil
	}
	for _, id := range kind.current {
		if version, ok := kind.published[id]; ok {
			versions[id] = version
		}
	}
	changes := map[string]trailguide.Change{}
	summaries := map[string][]string{}
	// Every pending note's summary is listed, in note order, wherever its rule or skill ends up.
	for _, pending := range c.pending {
		for id, change := range kind.notes(pending.note) {
			summaries[id] = append(summaries[id], pending.note.Summary)
			if change.Change == trailguide.ChangeRetired {
				retired[id] = trailguide.RetiredRule{LastVersion: kind.published[id], ReplacedBy: change.ReplacedBy}
			} else if previous, ok := changes[id]; ok {
				changes[id] = trailguide.LargerChange(previous, change.Change)
			} else {
				changes[id] = change.Change
			}
		}
	}
	for id, rule := range retired {
		rule.Summaries = summaries[id]
		retired[id] = rule
		delete(changes, id)
	}
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		change := trailguide.RecordedChange{Change: changes[id], Summaries: summaries[id]}
		next := trailguide.FirstRuleVersion
		if change.Change != trailguide.ChangeNew {
			from := kind.published[id]
			var err error
			if next, err = from.Next(change.Change); err != nil {
				return nil, nil, nil, failure("version-limit", id+": "+err.Error(), nil)
			}
			change.From = &from
		}
		versions[id] = next
		recorded[id] = change
	}
	return versions, recorded, retired, nil
}

// record returns the release record that publishes the plan with libraryFiles, the changed library-wide files.
func (p releasePlan) record(libraryFiles []string) trailguide.ReleaseRecord {
	return trailguide.ReleaseRecord{Release: p.release, Rules: p.versions, Changes: p.changes, Retired: p.retired, LibraryFiles: libraryFiles, Skills: p.skills}
}

// preview lists each changed, new, and retired rule and skill in ID order, with each skill's level and fork from
// context.
func (p releasePlan) preview(context skillContext) PendingRelease {
	record := p.record(nil)
	return PendingRelease{Release: p.release, Rules: releaseRules(record), Skills: releaseSkills(record, context), LibraryFiles: []string{}}
}

// empty reports whether the plan changes, adds, and retires no rules and no skills.
func (p releasePlan) empty() bool {
	return len(p.changes) == 0 && len(p.retired) == 0 && (p.skills == nil || len(p.skills.Changes)+len(p.skills.Retired) == 0)
}
