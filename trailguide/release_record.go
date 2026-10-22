// Parse the release record in a library release tag's message without accessing Git.

package trailguide

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skills"
)

// ReleaseRecord is the permanent record of one library release, read from its release/<number> tag's message. Rule
// IDs are library rule IDs, such as practices/testing/verify-retries. A parsed record's maps and LibraryFiles are
// never nil.
type ReleaseRecord struct {
	// Release is the library release number, starting at 1.
	Release int `json:"release"`
	// Rules maps every rule current after this library release to its version.
	Rules map[string]RuleVersion `json:"rules"`
	// Changes lists the rules this library release added or published new versions of; each is also in Rules.
	Changes map[string]RecordedChange `json:"changes"`
	// Retired lists the rules this library release retired; none of them is in Rules.
	Retired map[string]RetiredRule `json:"retired"`
	// LibraryFiles lists changed library-wide files, such as group metadata and shared assets, in authored order.
	LibraryFiles []string `json:"libraryFiles"`
	// Skills records the library's skills, by skill ID, such as skills/build-feature. It is nil when the record has
	// no skills field: the library never had a skill, or a Trail Guide without skill support published it.
	Skills *SkillRecord `json:"skills,omitempty"`
}

// SkillRecord is a release record's skills field: each skill's version, change, and retirement, in the same form as
// the record's rule fields. A parsed record's maps are never nil.
type SkillRecord struct {
	// Versions maps every skill current after this library release to its version; it is empty when every skill is
	// retired.
	Versions map[string]RuleVersion `json:"versions"`
	// Changes lists the skills this library release added or published new versions of; each is also in Versions.
	Changes map[string]RecordedChange `json:"changes"`
	// Retired lists the skills this library release retired; none of them is in Versions, and each ReplacedBy is.
	Retired map[string]RetiredRule `json:"retired"`
}

// RecordedChange is one rule's or skill's change in a library release. From is nil exactly when Change is new.
type RecordedChange struct {
	Change Change       `json:"change"`
	From   *RuleVersion `json:"from,omitempty"`
	// Summaries holds one summary per change note that named the rule, in note order; it is never empty, and each
	// is one line of text without control characters.
	Summaries []string `json:"summaries"`
}

// RetiredRule is a rule or skill a library release retired. ReplacedBy is empty when nothing replaces it. Summaries is
// as in RecordedChange.
type RetiredRule struct {
	LastVersion RuleVersion `json:"lastVersion"`
	ReplacedBy  string      `json:"replacedBy,omitempty"`
	Summaries   []string    `json:"summaries"`
}

// ReleaseRecordFormat is the newest release record format this version of Trail Guide writes and reads, recorded in
// each record's formatVersion. It changes only for incompatible changes: readers ignore fields they don't know, so new
// optional fields keep the format.
const ReleaseRecordFormat = 1

// UnsupportedReleaseRecordError reports a release record whose formatVersion is above ReleaseRecordFormat: a newer
// Trail Guide published it, and only a version of Trail Guide, and of this package, that supports its format can read
// it. Check for it with errors.As.
type UnsupportedReleaseRecordError struct {
	// Location is the record's formatVersion field, such as release/4.formatVersion.
	Location string
	// FormatVersion is the record's format, above ReleaseRecordFormat.
	FormatVersion int
}

// Error names the record's format and asks the reader to upgrade Trail Guide.
func (e *UnsupportedReleaseRecordError) Error() string {
	return e.Location + ": the release record uses format " + strconv.Itoa(e.FormatVersion) + ", but this version of Trail Guide reads only format " + strconv.Itoa(ReleaseRecordFormat) + "; upgrade Trail Guide to read this library release"
}

// Limits on a release record's collections bound the work of reading a record from a library nobody vetted. A
// library holds at most 10,000 files, so a library release can't list more rules, changes, or retirements, and its
// libraryFiles can name at most every file of the previous library release and of the new one.
const (
	maxRecordedRules = 10_000
	maxLibraryFiles  = 20_000
)

// requireAtMost fails when a collection at location holds more than limit entries.
func requireAtMost(count, limit int, location string) error {
	if count > limit {
		return invalid(location, "expected at most "+thousands(limit)+" entries")
	}
	return nil
}

// thousands writes a positive count with comma separators, such as 10,000.
func thousands(count int) string {
	text := strconv.Itoa(count)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	return text
}

// ParseReleaseRecord parses a release record: the YAML after the last --- line of a library release tag's message.
// location names the record in errors, such as its tag name. It reads formatVersion before anything else and
// fails with *UnsupportedReleaseRecordError when it's above ReleaseRecordFormat, so it never misreads a newer
// format. It ignores fields it doesn't know, at any level and whatever their values, so later formats can add
// fields that older readers skip; the record must still be one YAML document without anchors, aliases, explicit
// tags, or duplicate keys. It validates every field it knows, including that each change leads to the version in
// rules and that the first library release lists every rule and skill as new, and it refuses more than 10,000
// entries in rules, changes, retired, or any map of skills, and more than 20,000 in libraryFiles.
func ParseReleaseRecord(input []byte, location string) (ReleaseRecord, error) {
	record, err := parseReleaseRecord(input, location)
	return record, translate(err)
}

func parseReleaseRecord(input []byte, location string) (ReleaseRecord, error) {
	document, err := decode.Document(input, location)
	if err != nil {
		return ReleaseRecord{}, err
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return ReleaseRecord{}, invalid(location, "expected an object")
	}
	// The format comes first: a later format may hold content this version can't interpret at all.
	var format json.RawMessage
	if node := field(root, "formatVersion"); node != nil {
		// A value that isn't a plain scalar leaves format empty, which recordFormat reports as invalid.
		format, _ = decode.JSON(node, location+".formatVersion")
	}
	if err := recordFormat(format, location+".formatVersion"); err != nil {
		return ReleaseRecord{}, err
	}
	fields, err := knownFields(root, location, map[string][]string{"release": nil, "rules": nil, "libraryFiles": nil, "changes": {"change", "from", "summaries"}, "retired": {"lastVersion", "replacedBy", "summaries"}})
	if err != nil {
		return ReleaseRecord{}, err
	}
	record := ReleaseRecord{Changes: map[string]RecordedChange{}, Retired: map[string]RetiredRule{}, LibraryFiles: []string{}}
	if record.Release, err = releaseNumber(fields["release"], location+".release"); err != nil {
		return ReleaseRecord{}, err
	}
	if record.Rules, err = recordedVersions(fields["rules"], ruleIDs, location+".rules"); err != nil {
		return ReleaseRecord{}, err
	}
	if raw, ok := fields["changes"]; ok {
		if record.Changes, err = recordedChanges(raw, record.Rules, ruleIDs, location+".changes"); err != nil {
			return ReleaseRecord{}, err
		}
	}
	if raw, ok := fields["retired"]; ok {
		if record.Retired, err = retiredRules(raw, record.Rules, ruleIDs, location+".retired"); err != nil {
			return ReleaseRecord{}, err
		}
	}
	if raw, ok := fields["libraryFiles"]; ok {
		if record.LibraryFiles, err = libraryFiles(raw, location+".libraryFiles"); err != nil {
			return ReleaseRecord{}, err
		}
	}
	if node := field(root, "skills"); node != nil {
		if record.Skills, err = skillRecord(node, location+".skills"); err != nil {
			return ReleaseRecord{}, err
		}
	}
	if record.Release == 1 {
		if err := requireFirstRelease(record, location); err != nil {
			return ReleaseRecord{}, err
		}
	}
	return record, nil
}

// field returns the value of mapping's field name, or nil when it has none.
func field(mapping *yaml.Node, name string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if key := mapping.Content[i]; key.Tag == "!!str" && key.Value == name {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// knownFields converts to JSON, strictly, the fields of mapping that known names, in document order, and ignores
// every other field, whatever its value. A known field with entry field names holds a mapping from IDs to entries,
// whose named fields are converted the same way; an entry, or a known field, of any other shape converts whole,
// so its validator reports the wrong type.
func knownFields(mapping *yaml.Node, location string, known map[string][]string) (map[string]json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i], mapping.Content[i+1]
		entryFields, isKnown := known[key.Value]
		if key.Tag != "!!str" || !isKnown {
			continue
		}
		where := location + "." + key.Value
		var err error
		if entryFields == nil || value.Kind != yaml.MappingNode {
			fields[key.Value], err = decode.JSON(value, where)
		} else {
			fields[key.Value], err = knownEntries(value, where, entryFields)
		}
		if err != nil {
			return nil, err
		}
	}
	return fields, nil
}

// knownEntries converts a mapping from IDs to entries to a JSON object, keeping only each entry's named fields.
func knownEntries(mapping *yaml.Node, location string, names []string) (json.RawMessage, error) {
	entries := map[string]json.RawMessage{}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i], mapping.Content[i+1]
		if key.Tag != "!!str" {
			return nil, invalid(location, "mapping keys must be strings; quote wildcard selectors")
		}
		where := location + "." + key.Value
		var err error
		if value.Kind != yaml.MappingNode {
			entries[key.Value], err = decode.JSON(value, where)
		} else {
			known := map[string][]string{}
			for _, name := range names {
				known[name] = nil
			}
			var fields map[string]json.RawMessage
			if fields, err = knownFields(value, where, known); err == nil {
				entries[key.Value], err = json.Marshal(fields)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(entries)
}

// skillRecord reads a record's skills field, an object whose versions are required, ignoring fields it doesn't know
// as the record does.
func skillRecord(node *yaml.Node, location string) (*SkillRecord, error) {
	if node.Kind != yaml.MappingNode {
		return nil, invalid(location, "expected an object with the skills' versions")
	}
	fields, err := knownFields(node, location, map[string][]string{"versions": nil, "changes": {"change", "from", "summaries"}, "retired": {"lastVersion", "replacedBy", "summaries"}})
	if err != nil {
		return nil, err
	}
	raw, ok := fields["versions"]
	if !ok {
		return nil, invalid(location+".versions", "expected every current skill's version")
	}
	record := &SkillRecord{Changes: map[string]RecordedChange{}, Retired: map[string]RetiredRule{}}
	if record.Versions, err = recordedVersions(raw, skillIDs, location+".versions"); err != nil {
		return nil, err
	}
	if raw, ok := fields["changes"]; ok {
		if record.Changes, err = recordedChanges(raw, record.Versions, skillIDs, location+".changes"); err != nil {
			return nil, err
		}
	}
	if raw, ok := fields["retired"]; ok {
		if record.Retired, err = retiredRules(raw, record.Versions, skillIDs, location+".retired"); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// recordedIDs is what a record's IDs of one kind, rules or skills, name, and how to check one.
type recordedIDs struct {
	// noun names one, such as rule; versions names the field that lists every current one, such as rules.
	noun, versions string
	validate       func(id, location string) error
}

var (
	ruleIDs  = recordedIDs{noun: "rule", versions: "rules", validate: librarytree.ValidateRuleID}
	skillIDs = recordedIDs{noun: "skill", versions: "skills.versions", validate: validateSkillID}
)

// validateSkillID accepts the ID of a library's own skill, skills/<name>, whose name follows the skill name grammar.
func validateSkillID(id, location string) error {
	if name, found := strings.CutPrefix(id, skills.Directory+"/"); found && skills.ValidName(name) {
		return nil
	}
	return invalid(location, "expected a skill ID, such as skills/build-feature, got "+decode.Quote(id))
}

// requireFirstRelease requires the first library release to publish every rule and skill as new, so each one's
// history starts with a changes entry for 1.0.0, and to retire nothing, since nothing had a version before it.
func requireFirstRelease(record ReleaseRecord, location string) error {
	if err := requireAllNew(record.Rules, record.Changes, record.Retired, "rule", location+".changes", location+".retired"); err != nil {
		return err
	}
	if record.Skills != nil {
		return requireAllNew(record.Skills.Versions, record.Skills.Changes, record.Skills.Retired, "skill", location+".skills.changes", location+".skills.retired")
	}
	return nil
}

// requireAllNew requires every one of versions to be new in changes, and retired to be empty.
func requireAllNew(versions map[string]RuleVersion, changes map[string]RecordedChange, retired map[string]RetiredRule, noun, changesLocation, retiredLocation string) error {
	if len(retired) > 0 {
		return invalid(retiredLocation, "the first library release can't retire "+noun+"s")
	}
	for _, id := range slices.Sorted(maps.Keys(versions)) {
		if change, ok := changes[id]; !ok || change.Change != ChangeNew {
			return invalid(changesLocation+"."+id, "the first library release must list every "+noun+" as new")
		}
	}
	return nil
}

// recordFormat accepts ReleaseRecordFormat, reporting a larger whole number as a format this version can't read.
func recordFormat(input json.RawMessage, location string) error {
	format, err := strconv.Atoi(string(input))
	switch {
	case err != nil || format < 1:
		return invalid(location, "expected the release record format, a whole number such as 1")
	case format > ReleaseRecordFormat:
		return &UnsupportedReleaseRecordError{Location: location, FormatVersion: format}
	}
	return nil
}

// releaseNumber accepts a positive integer below one billion.
func releaseNumber(input json.RawMessage, location string) (int, error) {
	number, err := strconv.Atoi(string(input))
	if err != nil || number < 1 || number >= 1_000_000_000 {
		return 0, invalid(location, "expected a library release number, starting at 1")
	}
	return number, nil
}

// recordedVersions reads the required map of every current rule, or skill, to its version; it may be empty.
func recordedVersions(input json.RawMessage, kind recordedIDs, location string) (map[string]RuleVersion, error) {
	entries, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if err := requireAtMost(len(entries), maxRecordedRules, location); err != nil {
		return nil, err
	}
	versions := map[string]RuleVersion{}
	for _, id := range slices.Sorted(maps.Keys(entries)) {
		if err := kind.validate(id, location+"."+id); err != nil {
			return nil, err
		}
		version, err := versionField(entries[id], location+"."+id)
		if err != nil {
			return nil, err
		}
		versions[id] = version
	}
	return versions, nil
}

// recordedChanges requires each changed rule's, or skill's, version in versions to follow from its change and
// previous version.
func recordedChanges(input json.RawMessage, versions map[string]RuleVersion, kind recordedIDs, location string) (map[string]RecordedChange, error) {
	entries, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if err := requireAtMost(len(entries), maxRecordedRules, location); err != nil {
		return nil, err
	}
	changes := map[string]RecordedChange{}
	for _, id := range slices.Sorted(maps.Keys(entries)) {
		entryLocation := location + "." + id
		if err := kind.validate(id, entryLocation); err != nil {
			return nil, err
		}
		fields, err := decode.Object(entries[id], entryLocation)
		if err != nil {
			return nil, err
		}
		var change RecordedChange
		var name string
		if json.Unmarshal(fields["change"], &name) != nil {
			return nil, invalid(entryLocation+".change", "expected new, major, minor, or patch")
		}
		change.Change = Change(name)
		if change.Summaries, err = recordSummaries(fields["summaries"], entryLocation+".summaries"); err != nil {
			return nil, err
		}
		current, ok := versions[id]
		if !ok {
			return nil, invalid(entryLocation, "changed "+kind.noun+" is missing from "+kind.versions)
		}
		raw, hasFrom := fields["from"]
		switch change.Change {
		case ChangeNew:
			if hasFrom {
				return nil, invalid(entryLocation+".from", "a new "+kind.noun+" has no previous version")
			}
			if current != FirstRuleVersion {
				return nil, invalid(entryLocation, "a new "+kind.noun+" must be version 1.0.0 in "+kind.versions+", got "+current.String())
			}
		case ChangeMajor, ChangeMinor, ChangePatch:
			if !hasFrom {
				return nil, invalid(entryLocation+".from", "expected the "+kind.noun+"'s previous version")
			}
			from, err := versionField(raw, entryLocation+".from")
			if err != nil {
				return nil, err
			}
			next, err := from.Next(change.Change)
			if err != nil {
				return nil, invalid(entryLocation, err.Error())
			}
			if next != current {
				return nil, invalid(entryLocation, "a "+name+" change from "+from.String()+" leads to "+next.String()+", but "+kind.versions+" records "+current.String())
			}
			change.From = &from
		default:
			return nil, invalid(entryLocation+".change", "unknown change "+decode.Quote(name)+"; expected new, major, minor, or patch")
		}
		changes[id] = change
	}
	return changes, nil
}

// retiredRules requires each retired rule, or skill, to be absent from versions and any replacement to be present.
func retiredRules(input json.RawMessage, versions map[string]RuleVersion, kind recordedIDs, location string) (map[string]RetiredRule, error) {
	entries, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if err := requireAtMost(len(entries), maxRecordedRules, location); err != nil {
		return nil, err
	}
	retired := map[string]RetiredRule{}
	for _, id := range slices.Sorted(maps.Keys(entries)) {
		entryLocation := location + "." + id
		if err := kind.validate(id, entryLocation); err != nil {
			return nil, err
		}
		if _, current := versions[id]; current {
			return nil, invalid(entryLocation, "a retired "+kind.noun+" can't also be in "+kind.versions)
		}
		fields, err := decode.Object(entries[id], entryLocation)
		if err != nil {
			return nil, err
		}
		var rule RetiredRule
		if rule.LastVersion, err = versionField(fields["lastVersion"], entryLocation+".lastVersion"); err != nil {
			return nil, err
		}
		if rule.Summaries, err = recordSummaries(fields["summaries"], entryLocation+".summaries"); err != nil {
			return nil, err
		}
		if raw, ok := fields["replacedBy"]; ok {
			if json.Unmarshal(raw, &rule.ReplacedBy) != nil {
				return nil, invalid(entryLocation+".replacedBy", "expected a "+kind.noun+" ID")
			}
			if _, current := versions[rule.ReplacedBy]; !current {
				return nil, invalid(entryLocation+".replacedBy", "replacement "+decode.Quote(rule.ReplacedBy)+" is missing from "+kind.versions)
			}
		}
		retired[id] = rule
	}
	return retired, nil
}

// libraryFiles reads contained relative paths, keeping their order and rejecting duplicates and files that
// belong to a rule's version.
func libraryFiles(input json.RawMessage, location string) ([]string, error) {
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil || items == nil {
		return nil, invalid(location, "expected a list of file paths")
	}
	if err := requireAtMost(len(items), maxLibraryFiles, location); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for i, item := range items {
		path, err := decode.Path(item, location+"["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		if seen[path] {
			return nil, invalid(location+"["+strconv.Itoa(i)+"]", "duplicate path "+decode.Quote(path))
		}
		if librarytree.IsRuleContent(path) {
			return nil, invalid(location+"["+strconv.Itoa(i)+"]", decode.Quote(path)+" belongs to a rule's version, not the library-wide files")
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths, nil
}

// versionField reads a rule version from YAML text. 1.3.0 is text in YAML; a number such as 1.0 is rejected.
func versionField(input json.RawMessage, location string) (RuleVersion, error) {
	var text string
	if json.Unmarshal(input, &text) != nil {
		return RuleVersion{}, invalid(location, "expected a rule version, such as 1.3.0")
	}
	return ParseRuleVersion(text, location)
}

// recordSummaries reads a release record's non-empty list of summaries, one per change note, in order. Each follows
// a change note's summary rules.
func recordSummaries(input json.RawMessage, location string) ([]string, error) {
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil || len(items) == 0 {
		return nil, invalid(location, "expected a list with one summary per change note")
	}
	summaries := make([]string, len(items))
	for i, item := range items {
		summary, err := decode.Line(item, location+"["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		summaries[i] = summary
	}
	return summaries, nil
}
