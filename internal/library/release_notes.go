// Render a library release's notes and tag message from its release record, without accessing Git or files.

package library

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/trailguide"
)

// The notes write each paragraph and each list item on one line, because GitHub renders a GitHub Release page's
// body with hard line breaks, so a line break inside a sentence would show on the page.

// majorChangesAdvice opens the major changes section of every library release's notes.
const majorChangesAdvice = "Code that complied with the previous rule version could fail the new one, so review these before updating."

// sharedFilesSentence follows the rule sections of a library release that also changes library-wide files,
// except the first, which adds every file because nothing existed before it. The notes never list those files
// by path; the release record does.
const sharedFilesSentence = "This library release also updates shared files, such as group descriptions or shared assets."

// majorSkillChangesAdvice opens the Skills section of a library release that makes a major skill change.
const majorSkillChangesAdvice = "A major skill change can change what the skill produces or requires, so review these before updating."

// PendingSkill is one skill's change in a library release, named as PendingRule names a rule's, with what the
// release notes say about the skill beyond its version: its level, and the source skill it forks.
type PendingSkill struct {
	ID string `json:"id"`
	// Change is empty for a skill whose only change is its level or fork, which From, LastVersion, and Summaries
	// are absent for.
	Change      trailguide.Change       `json:"change,omitempty"`
	From        *trailguide.RuleVersion `json:"from,omitempty"`
	To          *trailguide.RuleVersion `json:"to,omitempty"`
	LastVersion *trailguide.RuleVersion `json:"lastVersion,omitempty"`
	ReplacedBy  string                  `json:"replacedBy,omitempty"`
	Summaries   []string                `json:"summaries,omitempty"`
	// Level is the skill's level after the library release, such as recommended, or dependency for one that
	// installs only with the skills that use it; it is empty for a retired skill. PreviousLevel is its level in the
	// latest library release when that differs, and empty otherwise and for a new skill.
	Level         string `json:"level,omitempty"`
	PreviousLevel string `json:"previousLevel,omitempty"`
	// Fork is the source skill the skill forks, when it's new and a fork, or became a fork, or its fork moved to
	// another commit; it is nil otherwise.
	Fork *SkillForkChange `json:"fork,omitempty"`
}

// SkillForkChange is the source skill a library's skill forks, as its source's skills.forks records it.
type SkillForkChange struct {
	// Source names the library's source, and ID is the skill's ID in it.
	Source string `json:"source"`
	ID     string `json:"id"`
	// Added reports that the skill is new, or wasn't a fork of this source skill in the latest library release;
	// otherwise its fork moved from PreviousBasedOn to BasedOn. Each is a commit, or empty when the fork records none.
	Added           bool   `json:"added"`
	BasedOn         string `json:"basedOn,omitempty"`
	PreviousBasedOn string `json:"previousBasedOn,omitempty"`
}

// skillContext is what release notes say about skills that their versions don't: levels and forks, which the
// manifest, a library-wide file, sets. A zero skillContext knows no levels or forks.
type skillContext struct {
	// levels maps each of the library's own skills to its level after the library release, and previousLevels each
	// to its level in the latest library release; previousLevels is nil when that library release had no
	// library manifest with skill levels, so no level change is reported.
	levels, previousLevels map[string]string
	// forks and previousForks map the ID of each of the library's own skills that forks a source's skill to its fork,
	// after the library release and in the latest one.
	forks, previousForks map[string]skillFork
}

// skillFork is the source skill one of the library's skills forks: the source's name, the skill's ID in it, and the
// commit the fork is based on, or empty.
type skillFork struct {
	source, id, basedOn string
}

// releaseSkills lists, in ID order, each skill a release record changed, added, or retired, and each skill whose
// level or fork context says changed without a new version.
func releaseSkills(record trailguide.ReleaseRecord, context skillContext) []PendingSkill {
	list := []PendingSkill{}
	if record.Skills == nil {
		return list
	}
	for _, id := range slices.Sorted(maps.Keys(record.Skills.Versions)) {
		next := record.Skills.Versions[id]
		skill := PendingSkill{ID: id, To: &next, Level: context.levels[id]}
		change, changed := record.Skills.Changes[id]
		if changed {
			skill.Change, skill.From, skill.Summaries = change.Change, change.From, slices.Clone(change.Summaries)
		}
		isNew := changed && change.Change == trailguide.ChangeNew
		if previous, known := context.previousLevels[id]; known && !isNew && previous != skill.Level {
			skill.PreviousLevel = previous
		}
		if fork, forked := context.forks[id]; forked {
			previous, wasForked := context.previousForks[id]
			switch {
			case isNew || !wasForked || previous.source != fork.source || previous.id != fork.id:
				skill.Fork = &SkillForkChange{Source: fork.source, ID: fork.id, Added: true, BasedOn: fork.basedOn}
			case previous.basedOn != fork.basedOn:
				skill.Fork = &SkillForkChange{Source: fork.source, ID: fork.id, BasedOn: fork.basedOn, PreviousBasedOn: previous.basedOn}
			}
		}
		if changed || skill.PreviousLevel != "" || skill.Fork != nil {
			list = append(list, skill)
		}
	}
	for id, retired := range record.Skills.Retired {
		last := retired.LastVersion
		list = append(list, PendingSkill{ID: id, Change: trailguide.ChangeRetired, LastVersion: &last, ReplacedBy: retired.ReplacedBy, Summaries: slices.Clone(retired.Summaries)})
	}
	slices.SortFunc(list, func(a, b PendingSkill) int { return strings.Compare(a.ID, b.ID) })
	return list
}

// releaseRules lists each rule a release record changed, added, or retired, in ID order.
func releaseRules(record trailguide.ReleaseRecord) []PendingRule {
	list := []PendingRule{}
	for id, change := range record.Changes {
		next := record.Rules[id]
		list = append(list, PendingRule{ID: id, Change: change.Change, From: change.From, To: &next, Summaries: slices.Clone(change.Summaries)})
	}
	for id, retired := range record.Retired {
		last := retired.LastVersion
		list = append(list, PendingRule{ID: id, Change: trailguide.ChangeRetired, LastVersion: &last, ReplacedBy: retired.ReplacedBy, Summaries: slices.Clone(retired.Summaries)})
	}
	slices.SortFunc(list, func(a, b PendingRule) int { return strings.Compare(a.ID, b.ID) })
	return list
}

// renderReleaseNotes returns a library release's Markdown notes, without a trailing newline: a line counting
// the rule and skill changes, a section for each kind of rule change that has entries, in the order new, major, minor,
// patch, and retired, so readers meet the rules they'd adopt before the changes to rules they have, a Skills section
// for the skills releaseSkills lists with context, a sentence noting shared files when a library release after the
// first also lists library-wide files, and collapsed tables of every rule's and every skill's version. Each rule and
// skill is a list item with its summaries, one per change note, as nested items, except in the first library release,
// whose rules and skills all have the same placeholder summary. Every paragraph and list item is one line.
// Library-wide files are never listed.
func renderReleaseNotes(record trailguide.ReleaseRecord, context skillContext) string {
	var out strings.Builder
	out.WriteString(countLine(record))
	sections := []struct {
		change  trailguide.Change
		heading string
	}{{trailguide.ChangeNew, "New rules"}, {trailguide.ChangeMajor, "Major changes"}, {trailguide.ChangeMinor, "Minor changes"}, {trailguide.ChangePatch, "Patch changes"}}
	for _, section := range sections {
		var items []string
		for _, id := range slices.Sorted(maps.Keys(record.Changes)) {
			change := record.Changes[id]
			if change.Change != section.change {
				continue
			}
			item := "- **" + id + "** "
			if change.From != nil {
				item += "`" + change.From.String() + "` → "
			}
			item += "`" + record.Rules[id].String() + "`"
			if record.Release > 1 {
				item += summaryLines(change.Summaries)
			}
			items = append(items, item)
		}
		if len(items) == 0 {
			continue
		}
		out.WriteString("\n\n## " + section.heading + "\n\n")
		if section.change == trailguide.ChangeMajor {
			out.WriteString(majorChangesAdvice + "\n\n")
		}
		out.WriteString(strings.Join(items, "\n"))
	}
	if len(record.Retired) > 0 {
		out.WriteString("\n\n## Retired rules\n\n")
		var items []string
		for _, id := range slices.Sorted(maps.Keys(record.Retired)) {
			retired := record.Retired[id]
			item := "- **" + id + "**, last version `" + retired.LastVersion.String() + "`"
			if retired.ReplacedBy != "" {
				item += ", replaced by **" + retired.ReplacedBy + "**"
			}
			items = append(items, item+summaryLines(retired.Summaries))
		}
		out.WriteString(strings.Join(items, "\n"))
	}
	writeSkills(&out, record.Release, releaseSkills(record, context))
	if record.Release > 1 && len(record.LibraryFiles) > 0 && len(record.Changes)+len(record.Retired)+skillChanges(record) > 0 {
		out.WriteString("\n\n" + sharedFilesSentence)
	}
	writeVersions(&out, "rule", record.Rules)
	if record.Skills != nil {
		writeVersions(&out, "skill", record.Skills.Versions)
	}
	return out.String()
}

// writeVersions writes a collapsed table of every rule's, or skill's, version, when there are any.
func writeVersions(out *strings.Builder, noun string, versions map[string]trailguide.RuleVersion) {
	if len(versions) == 0 {
		return
	}
	heading := strings.ToUpper(noun[:1]) + noun[1:]
	out.WriteString("\n\n<details>\n<summary>All " + noun + " versions in this library release</summary>\n\n| " + heading + " | Version |\n| --- | --- |\n")
	for _, id := range slices.Sorted(maps.Keys(versions)) {
		out.WriteString("| " + id + " | " + versions[id].String() + " |\n")
	}
	out.WriteString("\n</details>")
}

// writeSkills writes the Skills section: each new, major, minor, patch, and retired skill, in that order and then by
// ID, followed by each skill whose only change is its level or fork. Each item names the skill's version, change,
// level, and fork, and nests its summaries, except in release 1, whose summaries are all firstSkillSummary.
func writeSkills(out *strings.Builder, release int, list []PendingSkill) {
	if len(list) == 0 {
		return
	}
	order := map[trailguide.Change]int{trailguide.ChangeNew: 0, trailguide.ChangeMajor: 1, trailguide.ChangeMinor: 2, trailguide.ChangePatch: 3, trailguide.ChangeRetired: 4, "": 5}
	sorted := slices.Clone(list)
	slices.SortStableFunc(sorted, func(a, b PendingSkill) int { return order[a.Change] - order[b.Change] })
	out.WriteString("\n\n## Skills\n\n")
	if slices.ContainsFunc(sorted, func(skill PendingSkill) bool { return skill.Change == trailguide.ChangeMajor }) {
		out.WriteString(majorSkillChangesAdvice + "\n\n")
	}
	items := make([]string, 0, len(sorted))
	for _, skill := range sorted {
		item := "- **" + skill.ID + "**"
		switch {
		case skill.Change == trailguide.ChangeRetired:
			item += ", last version `" + skill.LastVersion.String() + "`, retired"
			if skill.ReplacedBy != "" {
				item += ", replaced by **" + skill.ReplacedBy + "**"
			}
		case skill.From != nil:
			item += " `" + skill.From.String() + "` → `" + skill.To.String() + "`, " + string(skill.Change)
		case skill.Change != "":
			item += " `" + skill.To.String() + "`, " + string(skill.Change)
		default:
			item += " `" + skill.To.String() + "`"
		}
		switch {
		case skill.PreviousLevel != "":
			item += ", now " + skill.Level + ", was " + skill.PreviousLevel
		case skill.Level != "":
			item += ", " + skill.Level
		}
		if fork := skill.Fork; fork != nil {
			item += forkPhrase(*fork)
		}
		if release > 1 || skill.Change == trailguide.ChangeRetired {
			item += summaryLines(skill.Summaries)
		}
		items = append(items, item)
	}
	out.WriteString(strings.Join(items, "\n"))
}

// forkPhrase describes the source skill a skill forks, such as ", forked from **dzhng:skills/x** at `8105a34`", or,
// for a fork that moved, the commits it moved between.
func forkPhrase(fork SkillForkChange) string {
	source := "**" + fork.Source + ":" + fork.ID + "**"
	if fork.Added {
		phrase := ", forked from " + source
		if fork.BasedOn != "" {
			phrase += " at `" + shortCommit(fork.BasedOn) + "`"
		}
		return phrase
	}
	phrase := ", fork of " + source
	if fork.BasedOn != "" {
		phrase += " now based on `" + shortCommit(fork.BasedOn) + "`"
	} else {
		phrase += " no longer based on a commit"
	}
	if fork.PreviousBasedOn != "" {
		phrase += ", was `" + shortCommit(fork.PreviousBasedOn) + "`"
	}
	return phrase
}

// shortCommit abbreviates a full commit SHA to its first 7 characters, as GitHub shows commits.
func shortCommit(commit string) string {
	return commit[:min(len(commit), 7)]
}

// skillChanges counts the skills a release record changed, added, or retired.
func skillChanges(record trailguide.ReleaseRecord) int {
	if record.Skills == nil {
		return 0
	}
	return len(record.Skills.Changes) + len(record.Skills.Retired)
}

// countLine opens the notes, such as "Library release 4 changes 4 rules: 3 new and 1 major.", on one line, counting
// each kind of change in the order of the sections, and adding a sentence that counts the skill changes the same way.
// A library release that changes only skills counts those instead, one that changes no rules or skills says it updates
// shared files instead, and the first library release says how many rules and skills it publishes, since they are all
// new.
func countLine(record trailguide.ReleaseRecord) string {
	skillCount := 0
	if record.Skills != nil {
		skillCount = len(record.Skills.Changes)
	}
	if record.Release == 1 {
		published := []string{}
		if count := len(record.Changes); count > 0 || skillCount == 0 {
			published = append(published, counted(count, "rule"))
		}
		if skillCount > 0 {
			published = append(published, counted(skillCount, "skill"))
		}
		if len(record.Changes)+skillCount == 0 {
			return "Library release 1 publishes no rules, only shared files, such as group descriptions or shared assets."
		}
		return "Library release 1 publishes " + series(published) + "."
	}
	heading := "Library release " + strconv.Itoa(record.Release) + " changes "
	rules := changeCounts(record.Changes, record.Retired, "rule")
	var skills string
	if record.Skills != nil {
		skills = changeCounts(record.Skills.Changes, record.Skills.Retired, "skill")
	}
	switch {
	case rules != "" && skills != "":
		return heading + rules + ". It also changes " + skills + "."
	case rules != "":
		return heading + rules + "."
	case skills != "":
		return heading + skills + "."
	case record.Skills != nil:
		return heading + "no rules or skills. It updates shared files, such as group descriptions or shared assets."
	}
	return heading + "no rules. It updates shared files, such as group descriptions or shared assets."
}

// changeCounts counts changes and retirements of rules or skills, such as "4 rules: 3 new and 1 major", in the order
// new, major, minor, patch, and retired, or returns "" when there are none.
func changeCounts(changes map[string]trailguide.RecordedChange, retired map[string]trailguide.RetiredRule, noun string) string {
	counts := map[trailguide.Change]int{trailguide.ChangeRetired: len(retired)}
	for _, change := range changes {
		counts[change.Change]++
	}
	total := len(changes) + len(retired)
	if total == 0 {
		return ""
	}
	var kinds []string
	for _, change := range []trailguide.Change{trailguide.ChangeNew, trailguide.ChangeMajor, trailguide.ChangeMinor, trailguide.ChangePatch, trailguide.ChangeRetired} {
		if counts[change] > 0 {
			kinds = append(kinds, strconv.Itoa(counts[change])+" "+string(change))
		}
	}
	return counted(total, noun) + ": " + series(kinds)
}

// counted writes a count of a noun, such as "1 rule" or "3 skills".
func counted(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(count) + " " + noun + "s"
}

// series joins items as English prose: "a", "a and b", or "a, b, and c".
func series(items []string) string {
	switch len(items) {
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}

// summaryLines writes each summary as a nested list item of its rule's item.
func summaryLines(summaries []string) string {
	var out strings.Builder
	for _, summary := range summaries {
		out.WriteString("\n  - " + summary)
	}
	return out.String()
}

// releaseMessage returns a library release tag's message: the notes, a line containing only ---, and the YAML
// release record. It fails unless the message parses back to exactly these notes and this record, so a tag
// never carries a record that projects would read differently or reject.
func releaseMessage(notes string, record trailguide.ReleaseRecord) ([]byte, error) {
	encoded, err := encodeReleaseRecord(record)
	if err != nil {
		return nil, err
	}
	message := notes + "\n---\n" + string(encoded)
	tag := "release/" + strconv.Itoa(record.Release)
	parsedNotes, parsed, err := trailguide.ParseReleaseMessage(tag, []byte(message))
	if err != nil {
		return nil, fmt.Errorf("render the release record for %s: %w", tag, err)
	}
	if parsedNotes != notes || !reflect.DeepEqual(parsed, normalizedRecord(record)) {
		return nil, failure("invalid-release-record", "the release record for "+tag+" doesn't read back as written", nil)
	}
	return []byte(message), nil
}

// normalizedRecord is how the parser returns record: absent sections become empty ones.
func normalizedRecord(record trailguide.ReleaseRecord) trailguide.ReleaseRecord {
	if record.Rules == nil {
		record.Rules = map[string]trailguide.RuleVersion{}
	}
	if record.Changes == nil {
		record.Changes = map[string]trailguide.RecordedChange{}
	}
	if record.Retired == nil {
		record.Retired = map[string]trailguide.RetiredRule{}
	}
	if record.LibraryFiles == nil {
		record.LibraryFiles = []string{}
	}
	if skills := record.Skills; skills != nil {
		normalized := trailguide.SkillRecord{Versions: skills.Versions, Changes: skills.Changes, Retired: skills.Retired}
		if normalized.Versions == nil {
			normalized.Versions = map[string]trailguide.RuleVersion{}
		}
		if normalized.Changes == nil {
			normalized.Changes = map[string]trailguide.RecordedChange{}
		}
		if normalized.Retired == nil {
			normalized.Retired = map[string]trailguide.RetiredRule{}
		}
		record.Skills = &normalized
	}
	return record
}

// encodeReleaseRecord writes the record's YAML, starting with its formatVersion, with sorted keys, leaving out
// empty changes, retired, and libraryFiles sections, and the skills section when the record has none. rules, and
// skills.versions in a skills section, are always present, since the parser requires them.
func encodeReleaseRecord(record trailguide.ReleaseRecord) ([]byte, error) {
	text := func(value string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value} }
	mapping := func(pairs ...*yaml.Node) *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Content: pairs} }
	versions := func(entries map[string]trailguide.RuleVersion) *yaml.Node {
		node := mapping()
		for _, id := range slices.Sorted(maps.Keys(entries)) {
			node.Content = append(node.Content, text(id), text(entries[id].String()))
		}
		return node
	}
	number := func(value int) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)}
	}
	list := func(values []string) *yaml.Node {
		sequence := &yaml.Node{Kind: yaml.SequenceNode}
		for _, value := range values {
			sequence.Content = append(sequence.Content, text(value))
		}
		return sequence
	}
	// changeEntries writes the changes and retired fields of rules or skills, leaving out an empty one.
	changeEntries := func(changed map[string]trailguide.RecordedChange, retired map[string]trailguide.RetiredRule) []*yaml.Node {
		var fields []*yaml.Node
		if len(changed) > 0 {
			changes := mapping()
			for _, id := range slices.Sorted(maps.Keys(changed)) {
				change := changed[id]
				entry := mapping(text("change"), text(string(change.Change)))
				if change.From != nil {
					entry.Content = append(entry.Content, text("from"), text(change.From.String()))
				}
				entry.Content = append(entry.Content, text("summaries"), list(change.Summaries))
				changes.Content = append(changes.Content, text(id), entry)
			}
			fields = append(fields, text("changes"), changes)
		}
		if len(retired) > 0 {
			section := mapping()
			for _, id := range slices.Sorted(maps.Keys(retired)) {
				rule := retired[id]
				entry := mapping(text("lastVersion"), text(rule.LastVersion.String()))
				if rule.ReplacedBy != "" {
					entry.Content = append(entry.Content, text("replacedBy"), text(rule.ReplacedBy))
				}
				entry.Content = append(entry.Content, text("summaries"), list(rule.Summaries))
				section.Content = append(section.Content, text(id), entry)
			}
			fields = append(fields, text("retired"), section)
		}
		return fields
	}
	document := mapping(text("formatVersion"), number(trailguide.ReleaseRecordFormat), text("release"), number(record.Release), text("rules"), versions(record.Rules))
	document.Content = append(document.Content, changeEntries(record.Changes, record.Retired)...)
	if len(record.LibraryFiles) > 0 {
		document.Content = append(document.Content, text("libraryFiles"), list(record.LibraryFiles))
	}
	if skills := record.Skills; skills != nil {
		section := mapping(text("versions"), versions(skills.Versions))
		section.Content = append(section.Content, changeEntries(skills.Changes, skills.Retired)...)
		document.Content = append(document.Content, text("skills"), section)
	}
	// A width of -1 keeps each summary whole.
	return yaml.Dump(document, yaml.WithV3Defaults(), yaml.WithIndent(2), yaml.WithLineWidth(-1))
}
