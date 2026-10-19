// Parse project source declarations without resolving Git refs or reading files.

package rules

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// Configuration contains sources in alias order; an empty list is a local-only project.
type Configuration struct {
	Sources []Source `json:"sources"`
	// SkillTargets lists, as configured, where installed copies of skills go, each a key of SkillTargetFolders; it
	// is empty, never nil, when the configuration has none, which records skills without installing them.
	SkillTargets []string `json:"skillTargets"`
	// SkipAgentsFile is true when manageAgentsFile is false, so project commands leave the repository's AGENTS.md
	// alone; by default they maintain a Trail Guide section in it.
	SkipAgentsFile bool `json:"skipAgentsFile,omitempty"`
	// Library is the library section, the manifest, or nil when the repository isn't a library.
	Library *Library `json:"library,omitempty"`
}

// Source declares one remote library, the rules it selects, and the project's pins and exceptions.
// Rule IDs and selectors in Groups, Rules, Pins, and Exclude are library-relative and checked for syntax only;
// whether each names a rule the library supplies is checked against the imported catalog.
type Source struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	// Ref is the branch, tag, or full commit SHA the source imports, keeping its authored text, or the zero GitRef
	// when the source follows rule versions.
	Ref GitRef `json:"ref,omitzero"`
	// Groups holds the wildcard and group selectors of rules.include, such as techs/go/*; it is empty when the
	// source selects only individual rules.
	Groups GroupSelection `json:"groups"`
	// Rules lists the rule IDs rules.include selects individually, sorted; it is empty, never nil, when there are none.
	Rules []string `json:"rules"`
	// Pins maps rule IDs to pins; it is empty, never nil, when no rule is pinned, and always empty when Ref is set.
	Pins map[string]Pin `json:"pins"`
	// Exclude maps each excluded selector, a rule ID or a selector ending in *, to its exclusion; it is empty, never
	// nil, when the source has no exceptions. Only an exclusion of a rule ID can have a replacement. Look a rule up
	// with Exclusion, which also finds the selectors that match it.
	Exclude map[string]Exclusion `json:"exclude"`
	// Skills lists the skills.include selectors, sorted: skill IDs, each a folder path in the source's repository,
	// such as skills/build-feature, and skills.Recommended, or only skills.Wildcard. It is empty, never nil, when the
	// source includes no skills, though it still installs its library's required skills. Whether each ID names a
	// skill the source has is checked when sync reads the source.
	Skills []string `json:"skills"`
	// SkillUses maps a skill ID to the IDs of the skills it uses that its own trailguide-skills-used doesn't declare, each a
	// folder path in the source's repository, sorted; it is empty, never nil, when skills.uses declares none. Whether
	// each key is a skill the source installs, and each value a skill it has, is checked when sync reads the source.
	SkillUses map[string][]string `json:"skillUses,omitempty"`
	// SkillExclude maps the ID of each skill that skills.exclude leaves out to its exclusion; it is empty, never nil,
	// when there are none. An excluded skill is still imported, so updates can compare it with its replacement.
	SkillExclude map[string]SkillExclusion `json:"skillExclude,omitempty"`
	// SkillRename maps the ID of each skill that skills.rename installs under another name to that name; it is nil when
	// there are none, and never names an excluded skill. Whether the source installs each skill is
	// checked when skills resolve.
	SkillRename map[string]string `json:"skillRename,omitempty"`
	// LibraryRequires lists, sorted, the IDs of the source's skills that the configuration's own library section
	// requires, curating them, which the repository installs as a project too, so its maintainers work with the
	// skills it asks projects to use. It is nil when there are none.
	LibraryRequires []string `json:"libraryRequires,omitempty"`
	// Curated reports that the configuration's own library section curates skills from the source, which then needs
	// to install nothing itself.
	Curated bool `json:"curated,omitempty"`
}

// HasRules reports whether the source includes rules.
func (s Source) HasRules() bool { return !s.Groups.IsEmpty() || len(s.Rules) > 0 }

// SkillTargetFolders maps each skillTargets value to its folder, relative to the repository root, which holds one
// folder per installed skill.
var SkillTargetFolders = map[string]string{"claude": ".claude/skills", "agents": ".agents/skills"}

// SkillFolders returns the folder of each of the configuration's skill targets, sorted.
func (c Configuration) SkillFolders() []string {
	folders := []string{}
	for _, target := range c.SkillTargets {
		folders = append(folders, SkillTargetFolders[target])
	}
	slices.Sort(folders)
	return folders
}

// Pin keeps one rule at an exact published version, with the project's reason.
type Pin struct {
	Version trailguide.RuleVersion `json:"version" yaml:"version"`
	Reason  string                 `json:"reason" yaml:"reason"`
}

// Exclusion leaves one rule, or every rule a selector matches, out of generated guidance, with the project's reason.
type Exclusion struct {
	Reason string `json:"reason" yaml:"reason"`
	// ReplacedBy is empty, or the ID of the local rule agents read instead, whose file ReplacementFile returns.
	ReplacedBy string `json:"replacedBy,omitempty" yaml:"replacedBy,omitempty"`
	// BasedOn is the library version of the excluded rule that the replacement incorporates, which updates compare
	// the library's newer versions with; it is nil when the project doesn't record one, and only a replacement has
	// one. Configuration alone owns it.
	BasedOn *trailguide.RuleVersion `json:"basedOn,omitempty" yaml:"basedOn,omitempty"`
}

// Exclusion returns the exclusion that leaves rule id out of generated guidance: the entry for id itself, or else
// the entry of the narrowest selector that matches it, from a group's selector to "*". It reports false when no
// entry excludes the rule.
func (s Source) Exclusion(id string) (Exclusion, bool) {
	if exclusion, ok := s.Exclude[id]; ok {
		return exclusion, true
	}
	selectors := []string{"*"}
	if parts := strings.SplitN(id, "/", 3); len(parts) == 3 {
		selectors = []string{parts[0] + "/" + parts[1] + "/*", parts[0] + "/*", "*"}
	}
	for _, text := range selectors {
		if exclusion, ok := s.Exclude[text]; ok {
			return exclusion, true
		}
	}
	return Exclusion{}, false
}

// ReplacementFile returns the path, relative to the Trail Guide directory, of the authored rule that replaces the
// excluded rule, such as rules/techs/go/wrap-errors.md for ReplacedBy techs/go/wrap-errors, or "" when nothing
// replaces it.
func (e Exclusion) ReplacementFile() string {
	if e.ReplacedBy == "" {
		return ""
	}
	return "rules/" + e.ReplacedBy + ".md"
}

var sourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// versionOneProblem explains how to rewrite a version 1 configuration, which Trail Guide no longer reads, as version 2.
const versionOneProblem = `version 1 configuration is no longer supported, and Trail Guide doesn't convert it. Rewrite .trailguide/config.yaml as version 2, as in the example below: in each source, move groups and rules into rules.include, writing each group as its selector, such as techs/go/*, and keeping wildcards such as "*"; then move exclude and pins under rules as lists of entries with an id, writing each replacedBy as the ID of the authored rule it names, such as techs/go/our-errors for local/techs/go/our-errors.md. Move authored rules and groups from .trailguide/local/ to .trailguide/rules/, and shared assets from .trailguide/local/assets/ to .trailguide/assets/.

schemaVersion: 2
sources:
  team:
    repository: https://github.com/example/rules.git
    rules:
      include: [techs/go/*, practices/testing/verify-retry-limits]
      exclude:
        - id: techs/go/wrap-errors
          reason: Our errors package covers this.
      pins:
        - id: practices/testing/verify-retry-limits
          version: "1.3.0"
          reason: Waiting on the author's answer.

After rewriting it, run trailguide project check.`

// ParseConfiguration validates schema version 2, rejecting unknown fields and contradictory source declarations,
// and a version 1 configuration with an example of the version 2 shape. A library section is the manifest, which
// makes the repository a library too. Errors return no partial configuration. Strings retain authored spacing.
// Source aliases, group IDs, and rule IDs are sorted.
func ParseConfiguration(input json.RawMessage) (Configuration, error) {
	fields, err := decode.Object(input, "configuration")
	if err != nil {
		return Configuration{}, err
	}
	if _, ok := fields["localGroups"]; ok {
		return Configuration{}, invalid("localGroups", "remove localGroups; authored groups are discovered from rules/<group>/_group.yaml")
	}
	var schema float64
	versioned := json.Unmarshal(fields["schemaVersion"], &schema) == nil
	if versioned && schema == 1 {
		return Configuration{}, invalid("schemaVersion", versionOneProblem)
	}
	if err := decode.KnownFields(fields, []string{"schemaVersion", "skillTargets", "manageAgentsFile", "sources", "library"}, "configuration"); err != nil {
		return Configuration{}, err
	}
	if !versioned || schema != 2 {
		return Configuration{}, invalid("schemaVersion", "only version 2 is supported")
	}
	sources, err := decode.Object(fields["sources"], "sources")
	if err != nil {
		return Configuration{}, err
	}
	targets, err := parseSkillTargets(fields["skillTargets"])
	if err != nil {
		return Configuration{}, err
	}
	result := Configuration{Sources: make([]Source, 0, len(sources)), SkillTargets: targets}
	if raw, ok := fields["manageAgentsFile"]; ok {
		// A pointer tells null, which would otherwise leave manage false, from a boolean.
		var manage *bool
		if json.Unmarshal(raw, &manage) != nil || manage == nil {
			return Configuration{}, invalid("manageAgentsFile", "expected true or false")
		}
		result.SkipAgentsFile = !*manage
	}
	// repositories maps each repository identity to the first source, in name order, that imports it.
	repositories := make(map[string]string)
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		source, err := parseSource(name, sources[name], repositories)
		if err != nil {
			return Configuration{}, err
		}
		result.Sources = append(result.Sources, source)
	}
	if raw, ok := fields["library"]; ok {
		if result.Library, err = parseLibrary(raw, result.Sources); err != nil {
			return Configuration{}, err
		}
		for name := range result.Library.Curated {
			index := slices.IndexFunc(result.Sources, func(source Source) bool { return source.Name == name })
			result.Sources[index].Curated = true
		}
		for _, id := range result.Library.Skills.Required {
			if name, skill, curated := strings.Cut(id, ":"); curated {
				index := slices.IndexFunc(result.Sources, func(source Source) bool { return source.Name == name })
				result.Sources[index].LibraryRequires = append(result.Sources[index].LibraryRequires, skill)
			}
		}
	}
	for i := range result.Sources {
		slices.Sort(result.Sources[i].LibraryRequires)
	}
	return result, nil
}

// parseSkillTargets validates skillTargets, a list of SkillTargetFolders keys, each at most once. A missing field is
// no targets.
func parseSkillTargets(input json.RawMessage) ([]string, error) {
	targets := []string{}
	if input == nil {
		return targets, nil
	}
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil || items == nil {
		return nil, invalid("skillTargets", "expected a list of skill targets, such as [claude, agents], or [] to install no copies")
	}
	for i, item := range items {
		where := fmt.Sprintf("skillTargets[%d]", i)
		var target string
		if json.Unmarshal(item, &target) != nil {
			return nil, invalid(where, "expected claude or agents")
		}
		if _, known := SkillTargetFolders[target]; !known {
			return nil, invalid(where, "expected claude, for .claude/skills/, or agents, for .agents/skills/, got "+decode.Quote(target))
		}
		if slices.Contains(targets, target) {
			return nil, invalid(where, target+" is listed more than once; list each skill target once")
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// movedSourceFields maps each version 1 source field to where version 2 keeps it.
var movedSourceFields = map[string]string{
	"groups":  "select groups in rules.include with selectors such as techs/go/*",
	"pins":    "pins moved to rules.pins, a list of entries with an id",
	"exclude": "exclude moved to rules.exclude, a list of entries with an id",
}

// parseSource validates identity, then revision, then selection, then pins and exclusions.
func parseSource(name string, input json.RawMessage, repositories map[string]string) (Source, error) {
	where := "sources." + name
	if !sourceNamePattern.MatchString(name) || name == "local" {
		return Source{}, invalid(where, "invalid or reserved source name")
	}
	fields, err := decode.Object(input, where)
	if err != nil {
		return Source{}, err
	}
	for _, field := range slices.Sorted(maps.Keys(movedSourceFields)) {
		if _, ok := fields[field]; ok {
			return Source{}, invalid(where+"."+field, field+" isn't a version 2 source field; "+movedSourceFields[field])
		}
	}
	if err := decode.KnownFields(fields, []string{"repository", "ref", "rules", "skills"}, where); err != nil {
		return Source{}, err
	}
	repository, err := decode.Text(fields["repository"], where+".repository")
	if err != nil {
		return Source{}, err
	}
	address, err := ParseRepository(fields["repository"], where+".repository")
	if err != nil {
		return Source{}, err
	}
	if other, declared := repositories[address.Identity]; declared {
		return Source{}, invalid(where, "repository "+repository+" is declared more than once: sources."+other+" imports it too; import each repository with one source")
	}
	repositories[address.Identity] = name
	result := Source{Name: name, Repository: repository}
	if raw, ok := fields["ref"]; ok {
		if first := strings.TrimSpace(string(raw)); first != "" && strings.ContainsRune("-0123456789", rune(first[0])) {
			return Source{}, invalid(where+".ref", "expected a tag or full commit SHA in quotes; YAML reads an unquoted value of digits as a number")
		}
		text, err := decode.Text(raw, where+".ref")
		if err != nil {
			return Source{}, err
		}
		if result.Ref, err = ParseGitRef(text, where+".ref"); err != nil {
			return Source{}, err
		}
	}
	// A source that includes nothing still installs its library's required skills.
	_, hasRules := fields["rules"]
	result.Groups, result.Rules, result.Pins, result.Exclude = GroupSelection{}, []string{}, map[string]Pin{}, map[string]Exclusion{}
	if hasRules {
		if err := parseRuleFields(fields["rules"], where, &result); err != nil {
			return Source{}, err
		}
	}
	if err := parseSkillFields(fields["skills"], where+".skills", &result); err != nil {
		return Source{}, err
	}
	return result, nil
}

// parseSkillFields reads a source's skills object into source: include, a list of skill IDs or only "*"; uses, the
// skills each skill uses beyond what it declares; exclude, as parseSkillExclusions reads it; and rename, as
// parseSkillRenames reads it. A missing object includes no skills, declares no uses, excludes none, and renames
// none. include may be left out only beside rename. A skill can't be both excluded and renamed.
func parseSkillFields(input json.RawMessage, location string, source *Source) error {
	source.Skills, source.SkillUses, source.SkillExclude = []string{}, map[string][]string{}, map[string]SkillExclusion{}
	if input == nil {
		return nil
	}
	fields, err := decode.Object(input, location)
	if err != nil {
		return err
	}
	if err := decode.KnownFields(fields, []string{"include", "uses", "exclude", "rename"}, location); err != nil {
		return err
	}
	// A source installs its library's required skills without include, so renaming one needs no include.
	if _, renames := fields["rename"]; fields["include"] != nil || !renames {
		if source.Skills, err = ParseSkillSelection(fields["include"], location+".include"); err != nil {
			return err
		}
	}
	if source.SkillUses, err = parseSkillUses(fields["uses"], location+".uses"); err != nil {
		return err
	}
	if source.SkillExclude, err = parseSkillExclusions(fields["exclude"], location+".exclude"); err != nil {
		return err
	}
	if source.SkillRename, err = parseSkillRenames(fields["rename"], location+".rename"); err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(source.SkillRename)) {
		if _, excluded := source.SkillExclude[id]; excluded {
			return invalid(location+".rename."+id, "the source excludes this skill, so it isn't installed and has no name to change; remove the rename or the exclusion")
		}
	}
	return nil
}

// parseSkillUses validates skills.uses, a mapping of skill IDs to lists of the skill IDs each uses, each list
// non-empty, without repeats or the skill itself, and returns each list sorted. A missing field declares none.
func parseSkillUses(input json.RawMessage, location string) (map[string][]string, error) {
	uses := map[string][]string{}
	if input == nil {
		return uses, nil
	}
	fields, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	for _, id := range slices.Sorted(maps.Keys(fields)) {
		where := location + "." + id
		if err := skills.ValidateID(id, where); err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if json.Unmarshal(fields[id], &items) != nil || len(items) == 0 {
			return nil, invalid(where, "list the skills "+id+" uses, such as [skills/tdd]")
		}
		used := []string{}
		for i, item := range items {
			at := fmt.Sprintf("%s[%d]", where, i)
			other, err := decode.Text(item, at)
			if err != nil {
				return nil, err
			}
			if err := skills.ValidateID(other, at); err != nil {
				return nil, err
			}
			switch {
			case other == id:
				return nil, invalid(at, "a skill can't use itself")
			case slices.Contains(used, other):
				return nil, invalid(at, other+" is listed more than once; list each skill once")
			}
			used = append(used, other)
		}
		slices.Sort(used)
		uses[id] = used
	}
	return uses, nil
}

// ParseSkillSelection validates a source's skill selectors, a JSON list at location of distinct skill IDs, such as
// skills/build-feature, and recommended, or only "*", and returns them sorted. An empty or missing list fails.
func ParseSkillSelection(input json.RawMessage, location string) ([]string, error) {
	selected := []string{}
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil || len(items) == 0 {
		return nil, invalid(location, `select skills in include, a list of skill IDs such as skills/build-feature, recommended for the skills the source's library recommends, or "*" for every skill the source offers`)
	}
	for i, item := range items {
		where := fmt.Sprintf("%s[%d]", location, i)
		id, err := decode.Text(item, where)
		if err != nil {
			return nil, err
		}
		switch {
		case id == skills.Recommended+"/*":
			return nil, invalid(where, `recommended is one selector, without /*; it includes the skills the source's library recommends, so write recommended`)
		case id != skills.Wildcard && strings.HasSuffix(id, "/*"):
			return nil, invalid(where, `skill selectors have no prefixes; name each skill by ID, such as skills/build-feature, or include "*" for every skill the source offers`)
		case id != skills.Wildcard && id != skills.Recommended:
			if err := skills.ValidateID(id, where); err != nil {
				return nil, err
			}
		}
		if slices.Contains(selected, id) {
			return nil, invalid(where, id+" is listed more than once; list each skill once")
		}
		selected = append(selected, id)
	}
	if len(selected) > 1 && slices.Contains(selected, skills.Wildcard) {
		return nil, invalid(location, `"*" already includes every skill the source offers; remove the other entries`)
	}
	slices.Sort(selected)
	return selected, nil
}

// parseRuleFields reads a source's rules object into source: include, which must select something, then pins,
// which can't be combined with a ref, then exclude.
func parseRuleFields(input json.RawMessage, where string, source *Source) error {
	location := where + ".rules"
	fields, err := decode.Object(input, location)
	if err != nil {
		return err
	}
	if err := decode.KnownFields(fields, []string{"include", "exclude", "pins"}, location); err != nil {
		return err
	}
	if _, ok := fields["include"]; !ok {
		return invalid(location, "select rules in include, such as include: [techs/go/*]")
	}
	if source.Groups, source.Rules, err = ParseRuleSelection(fields["include"], location+".include"); err != nil {
		return err
	}
	if source.Groups.IsEmpty() && len(source.Rules) == 0 {
		return invalid(location+".include", "select at least one group, wildcard, or rule, such as techs/go/*")
	}
	if _, pinned := fields["pins"]; pinned && !source.Ref.IsZero() {
		return invalid(where, "rules.pins and ref can't be combined; remove ref to pin individual rules, or remove rules.pins to import one revision")
	}
	if source.Pins, err = parsePins(fields["pins"], location+".pins"); err != nil {
		return err
	}
	if source.Exclude, err = parseExclusions(fields["exclude"], location+".exclude"); err != nil {
		return err
	}
	return nil
}

// listEntries decodes a list of entry objects, each identified by its id, and returns each entry's fields and
// location by id, in list order. An entry's location before its id is known uses its index, such as
// sources.team.rules.pins[0].id; afterward it uses the id, such as sources.team.rules.pins.techs/go/errors. validate
// checks the id's syntax at its location. A repeated id fails at the repeated entry.
func listEntries(input json.RawMessage, location string, validate func(id, location string) error) ([]string, map[string]map[string]json.RawMessage, error) {
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil || items == nil {
		return nil, nil, invalid(location, "expected a list of entries, each with an id")
	}
	ids := []string{}
	entries := map[string]map[string]json.RawMessage{}
	for i, item := range items {
		where := fmt.Sprintf("%s[%d]", location, i)
		fields, err := decode.Object(item, where)
		if err != nil {
			return nil, nil, err
		}
		id, err := decode.Text(fields["id"], where+".id")
		if err != nil {
			return nil, nil, err
		}
		if err := validate(id, where+".id"); err != nil {
			return nil, nil, err
		}
		if _, repeated := entries[id]; repeated {
			return nil, nil, invalid(location+"."+id, "listed more than once; keep one entry")
		}
		delete(fields, "id")
		ids = append(ids, id)
		entries[id] = fields
	}
	return ids, entries, nil
}

// parsePins validates each pin's rule ID, exact version, and reason; a missing field is no pins.
func parsePins(input json.RawMessage, location string) (map[string]Pin, error) {
	result := map[string]Pin{}
	if input == nil {
		return result, nil
	}
	ids, entries, err := listEntries(input, location, func(id, where string) error {
		if strings.Contains(id, "*") || librarytree.ValidateGroupID(id, where) == nil {
			return invalid(where, "a pin keeps one rule at a version, so its id is a rule ID, such as techs/go/wrap-errors, not "+decode.Quote(id))
		}
		return librarytree.ValidateRuleID(id, where)
	})
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		where := location + "." + id
		fields := entries[id]
		if err := decode.KnownFields(fields, []string{"version", "reason"}, where); err != nil {
			return nil, err
		}
		version, err := quotedVersion(fields["version"], where+".version")
		if err != nil {
			return nil, err
		}
		reason, err := decode.Text(fields["reason"], where+".reason")
		if err != nil {
			return nil, err
		}
		result[id] = Pin{Version: version, Reason: reason}
	}
	return result, nil
}

// quotedVersion parses an exact rule version written as a JSON string, such as "1.3.0".
func quotedVersion(raw json.RawMessage, location string) (trailguide.RuleVersion, error) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return trailguide.RuleVersion{}, invalid(location, `expected an exact rule version in quotes, such as "1.3.0"`)
	}
	return trailguide.ParseRuleVersion(text, location)
}

// parseExclusions validates each exclusion's selector, reason, and, for a rule ID, the optional ID of the local rule
// that replaces it and the optional version the replacement is based on. A missing field is no exclusions. Whether
// the replacement's file exists is checked when rules resolve, and whether the library published the basedOn version
// when sync reads it.
func parseExclusions(input json.RawMessage, location string) (map[string]Exclusion, error) {
	result := map[string]Exclusion{}
	if input == nil {
		return result, nil
	}
	ids, entries, err := listEntries(input, location, func(id, where string) error {
		_, err := parseSelector(id, where)
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		where := location + "." + id
		fields := entries[id]
		if err := decode.KnownFields(fields, []string{"reason", "replacedBy", "basedOn"}, where); err != nil {
			return nil, err
		}
		reason, err := decode.Text(fields["reason"], where+".reason")
		if err != nil {
			return nil, err
		}
		exclusion := Exclusion{Reason: reason}
		if raw, ok := fields["replacedBy"]; ok {
			if isSelectorPattern(id) {
				return nil, invalid(where+".replacedBy", "a replacement takes the place of one rule, so its id is a rule ID, not a selector ending in *")
			}
			replacement, err := decode.Text(raw, where+".replacedBy")
			if err != nil {
				return nil, err
			}
			if named := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(replacement, "local/"), "rules/"), ".md"); named != replacement && librarytree.ValidateRuleID(named, where) == nil {
				return nil, invalid(where+".replacedBy", "replacedBy names the authored rule by its ID, "+named+", not by its file")
			}
			if err := librarytree.ValidateRuleID(replacement, where+".replacedBy"); err != nil {
				return nil, err
			}
			exclusion.ReplacedBy = replacement
		}
		if raw, ok := fields["basedOn"]; ok {
			if exclusion.ReplacedBy == "" {
				return nil, invalid(where+".basedOn", "basedOn records the version a replacement incorporates, so it needs replacedBy")
			}
			version, err := quotedVersion(raw, where+".basedOn")
			if err != nil {
				return nil, err
			}
			exclusion.BasedOn = &version
		}
		result[id] = exclusion
	}
	return result, nil
}
