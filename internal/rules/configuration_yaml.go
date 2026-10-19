// Decode and edit project YAML while retaining comments and existing scalar values.

package rules

import (
	"bytes"
	"maps"
	"slices"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// ParseConfigurationYAML validates one UTF-8 YAML document using the configuration schema.
// Only string-keyed mappings, sequences, and JSON scalar types are supported; aliases and tags are rejected.
func ParseConfigurationYAML(input []byte) (Configuration, error) {
	_, data, err := decode.YAML(input, "configuration")
	if err != nil {
		return Configuration{}, err
	}
	return ParseConfiguration(data)
}

// AppendConfigurationSource adds a source without dropping existing comments or reordering entries. Its rules.include
// lists the source's wildcard, then its groups as selectors such as techs/go/*, then its individually selected rules;
// its pins and exclude lists follow in rule ID order, and are omitted when empty, as ref is. rules is omitted for a
// source that includes no rules. skills.include lists the source's skill selectors, and skills.uses the skills each
// skill uses beyond what it declares, in skill ID order; each is omitted when empty, and skills when both are.
// Pinned versions are written quoted, such as version: "1.3.0".
// Folded scalars use literal style in the result because the YAML encoder can change their values.
// The complete resulting configuration is validated before any bytes are returned.
func AppendConfigurationSource(input []byte, alias string, source Source) ([]byte, error) {
	document, data, err := decode.YAML(input, "configuration")
	if err != nil {
		return nil, err
	}
	if _, err := ParseConfiguration(data); err != nil {
		return nil, err
	}
	sources := mappingValue(document.Content[0], "sources")
	if mappingValue(sources, alias) != nil {
		return nil, invalid("sources."+alias, "source already exists")
	}
	include := []string{}
	if source.Groups.Pattern != "" {
		include = append(include, source.Groups.Pattern)
	}
	for _, group := range source.Groups.Groups {
		include = append(include, group+"/*")
	}
	include = append(include, source.Rules...)
	type selection struct {
		Include []string `yaml:"include"`
	}
	type skillSelection struct {
		Include []string            `yaml:"include,omitempty"`
		Uses    map[string][]string `yaml:"uses,omitempty"`
	}
	declaration := struct {
		Repository string          `yaml:"repository"`
		Ref        string          `yaml:"ref,omitempty"`
		Rules      *selection      `yaml:"rules,omitempty"`
		Skills     *skillSelection `yaml:"skills,omitempty"`
	}{Repository: source.Repository, Ref: source.Ref.String()}
	if len(include) > 0 {
		declaration.Rules = &selection{include}
	}
	if len(source.Skills) > 0 || len(source.SkillUses) > 0 {
		declaration.Skills = &skillSelection{source.Skills, source.SkillUses}
	}
	var entry yaml.Node
	if err := entry.Encode(declaration); err != nil {
		return nil, err
	}
	if ruleFields := mappingValue(&entry, "rules"); ruleFields != nil {
		if err := addEntries(ruleFields, "pins", pinNodes(source.Pins), "sources."+alias+".rules"); err != nil {
			return nil, err
		}
		if err := addEntries(ruleFields, "exclude", exclusionNodes(source.Exclude), "sources."+alias+".rules"); err != nil {
			return nil, err
		}
	}
	sources.Content = append(sources.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: alias}, &entry)
	sources.Style = 0
	return encodeConfiguration(document)
}

// SetConfigurationSkillTargets sets skillTargets to targets, keys of SkillTargetFolders in the order given, replacing
// the field, or adding it after schemaVersion, without dropping other comments or reordering entries. The complete
// resulting configuration is validated before any bytes are returned.
func SetConfigurationSkillTargets(input []byte, targets []string) ([]byte, error) {
	document, data, err := decode.YAML(input, "configuration")
	if err != nil {
		return nil, err
	}
	if _, err := ParseConfiguration(data); err != nil {
		return nil, err
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, target := range targets {
		list.Content = append(list.Content, textNode(target))
	}
	root := document.Content[0]
	if existing := mappingValue(root, "skillTargets"); existing != nil {
		*existing = *list
		return encodeConfiguration(document)
	}
	at := 0
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "schemaVersion" {
			at = i + 2
		}
	}
	root.Content = slices.Insert(root.Content, at, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "skillTargets"}, list)
	root.Style = 0
	return encodeConfiguration(document)
}

// AppendConfigurationLibrary adds a library section, the manifest, holding value, such as an empty map or one with
// a license, without dropping existing comments or reordering entries. It fails when the configuration already has
// a library section. The complete resulting configuration is validated before any bytes are returned.
func AppendConfigurationLibrary(input []byte, value any) ([]byte, error) {
	document, data, err := decode.YAML(input, "configuration")
	if err != nil {
		return nil, err
	}
	if _, err := ParseConfiguration(data); err != nil {
		return nil, err
	}
	root := document.Content[0]
	if mappingValue(root, "library") != nil {
		return nil, invalid("library", "the configuration already has a library section")
	}
	var section yaml.Node
	if err := section.Encode(value); err != nil {
		return nil, err
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "library"}, &section)
	root.Style = 0
	return encodeConfiguration(document)
}

// SourceEdit lists pins and exclusions to add to one source, pins to remove from it, basedOn versions to set, and
// skill exclusions to add; each may be empty or nil.
type SourceEdit struct {
	Pins    map[string]Pin
	Exclude map[string]Exclusion
	// Unpin names rules whose existing pins the edit removes.
	Unpin []string
	// BasedOn sets the basedOn version of each existing replacement it names, adding or replacing the field.
	BasedOn map[string]trailguide.RuleVersion
	// SkillInclude names skills to add to skills.include, each after the existing selectors, unless the list
	// already names it or has "*".
	SkillInclude []string
	// SkillExclude holds the entries to add to skills.exclude, by skill ID.
	SkillExclude map[string]SkillExclusion
	// SkillUnrename names skills whose existing skills.rename entries the edit removes.
	SkillUnrename []string
}

// EditConfigurationSource adds pins, exclusions, and skill exclusions to the existing source alias, removes the pins
// edit.Unpin names and the skill renames edit.SkillUnrename names, and sets the basedOn version of the existing replacements edit.BasedOn names, keeping a replaced
// value's comment, without dropping other comments or reordering entries. Setting basedOn of a rule the source
// doesn't replace fails. A rules.pins list or skills.rename mapping left empty is removed; a pin or rename to remove that the source lacks
// fails. New
// entries follow the source's existing ones in rule or skill ID order, in a rules.pins, rules.exclude, or
// skills.exclude list that is created when absent. Versions and commits are written quoted, such as
// version: "1.3.0", so YAML reads them as text. A rule or skill the source already pins or excludes by its ID fails
// rather than being replaced. Folded scalars use literal style, as in AppendConfigurationSource. The complete
// resulting configuration is validated before any bytes are returned.
func EditConfigurationSource(input []byte, alias string, edit SourceEdit) ([]byte, error) {
	document, data, err := decode.YAML(input, "configuration")
	if err != nil {
		return nil, err
	}
	if _, err := ParseConfiguration(data); err != nil {
		return nil, err
	}
	source := mappingValue(mappingValue(document.Content[0], "sources"), alias)
	if source == nil {
		return nil, invalid("sources."+alias, "source doesn't exist")
	}
	source.Style = 0
	if err := removeSkillRenames(source, edit.SkillUnrename, "sources."+alias+".skills"); err != nil {
		return nil, err
	}
	addSkillIncludes(source, edit.SkillInclude)
	if err := addSkillExclusions(source, edit.SkillExclude, "sources."+alias+".skills"); err != nil {
		return nil, err
	}
	where := "sources." + alias + ".rules"
	ruleFields := mappingValue(source, "rules")
	if ruleFields == nil {
		if len(edit.Pins)+len(edit.Exclude)+len(edit.Unpin)+len(edit.BasedOn) > 0 {
			return nil, invalid(where, "the source includes no rules, so it has no rules to pin or exclude")
		}
		return encodeConfiguration(document)
	}
	ruleFields.Style = 0
	if err := removeEntries(ruleFields, "pins", edit.Unpin, where); err != nil {
		return nil, err
	}
	if err := addEntries(ruleFields, "pins", pinNodes(edit.Pins), where); err != nil {
		return nil, err
	}
	if err := addEntries(ruleFields, "exclude", exclusionNodes(edit.Exclude), where); err != nil {
		return nil, err
	}
	for _, id := range slices.Sorted(maps.Keys(edit.BasedOn)) {
		_, exclusion := listEntry(mappingValue(ruleFields, "exclude"), id)
		if exclusion == nil || mappingValue(exclusion, "replacedBy") == nil {
			return nil, invalid(where+".exclude."+id, "the source doesn't replace this rule, so it has no basedOn version to set")
		}
		version := edit.BasedOn[id]
		if existing := mappingValue(exclusion, "basedOn"); existing != nil {
			existing.Kind, existing.Tag, existing.Value, existing.Style = yaml.ScalarNode, "!!str", version.String(), yaml.DoubleQuotedStyle
			continue
		}
		exclusion.Style = 0
		exclusion.Content = append(exclusion.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "basedOn"}, versionNode(version))
	}
	return encodeConfiguration(document)
}

// addSkillIncludes appends each of ids to the skills.include list of source, creating the skills mapping and the
// list when they're absent, unless the list already names it or has "*".
func addSkillIncludes(source *yaml.Node, ids []string) {
	if len(ids) == 0 {
		return
	}
	skillFields := mappingValue(source, "skills")
	if skillFields == nil {
		skillFields = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		source.Content = append(source.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "skills"}, skillFields)
	}
	skillFields.Style = 0
	include := mappingValue(skillFields, "include")
	if include == nil {
		include = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		skillFields.Content = append([]*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "include"}, include}, skillFields.Content...)
	}
	for _, id := range ids {
		if !slices.ContainsFunc(include.Content, func(entry *yaml.Node) bool { return entry.Value == id || entry.Value == skills.Wildcard }) {
			include.Content = append(include.Content, textNode(id))
		}
	}
}

// addSkillExclusions appends exclude, in skill ID order, to the skills.exclude list of source, creating the list
// when it's absent. It fails, at a location under where, when the source includes no skills or already excludes
// one of them.
func addSkillExclusions(source *yaml.Node, exclude map[string]SkillExclusion, where string) error {
	if len(exclude) == 0 {
		return nil
	}
	skillFields := mappingValue(source, "skills")
	if skillFields == nil {
		return invalid(where, "the source includes no skills, so it has no skills to exclude")
	}
	skillFields.Style = 0
	nodes := map[string]*yaml.Node{}
	for id, exclusion := range exclude {
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "reason"}, textNode(exclusion.Reason)}}
		if exclusion.ReplacedBy != "" {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "replacedBy"}, textNode(exclusion.ReplacedBy))
		}
		if !exclusion.BasedOn.IsZero() {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "basedOn"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: exclusion.BasedOn.Canonical(), Style: yaml.DoubleQuotedStyle})
		}
		nodes[id] = node
	}
	return addEntries(skillFields, "exclude", nodes, where)
}

// removeSkillRenames deletes the entry of each of ids from the skills.rename mapping of source, and the mapping
// itself once it's empty. An ID the mapping lacks fails with its location under where.
func removeSkillRenames(source *yaml.Node, ids []string, where string) error {
	if len(ids) == 0 {
		return nil
	}
	skillFields := mappingValue(source, "skills")
	renames := mappingValue(skillFields, "rename")
	for _, id := range ids {
		if !removeKey(renames, id) {
			return invalid(where+".rename."+id, "the source has no such entry to remove")
		}
	}
	if len(renames.Content) == 0 {
		removeKey(skillFields, "rename")
	}
	return nil
}

// removeKey deletes key and its value from mapping, reporting whether mapping had it. A nil mapping has no keys.
func removeKey(mapping *yaml.Node, key string) bool {
	if mapping == nil {
		return false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = slices.Delete(mapping.Content, i, i+2)
			return true
		}
	}
	return false
}

// mappingValue returns the value of key in mapping, or nil when mapping is nil or lacks the key.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// listEntry returns the index and mapping of the entry whose id is id in list, or -1 and nil when list is nil or
// has no such entry.
func listEntry(list *yaml.Node, id string) (int, *yaml.Node) {
	if list == nil {
		return -1, nil
	}
	for i, entry := range list.Content {
		if key := mappingValue(entry, "id"); key != nil && key.Value == id {
			return i, entry
		}
	}
	return -1, nil
}

// removeEntries deletes the entry of each of ids from the list field of parent, and the field itself once it's
// empty. An ID the list lacks fails with its location under where.
func removeEntries(parent *yaml.Node, field string, ids []string, where string) error {
	if len(ids) == 0 {
		return nil
	}
	target := mappingValue(parent, field)
	for _, id := range ids {
		index, _ := listEntry(target, id)
		if index < 0 {
			return invalid(where+"."+field+"."+id, "the source has no such entry to remove")
		}
		target.Content = slices.Delete(target.Content, index, index+1)
	}
	if len(target.Content) == 0 {
		for i := 0; i+1 < len(parent.Content); i += 2 {
			if parent.Content[i].Value == field {
				parent.Content = slices.Delete(parent.Content, i, i+2)
				break
			}
		}
	}
	return nil
}

// addEntries appends entries, in ID order, to the list field of parent, each a mapping that begins with its id,
// creating the field when it's absent and entries isn't empty. An ID the list already holds fails with its location
// under where.
func addEntries(parent *yaml.Node, field string, entries map[string]*yaml.Node, where string) error {
	if len(entries) == 0 {
		return nil
	}
	target := mappingValue(parent, field)
	if target == nil {
		target = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}, target)
	}
	target.Style = 0
	for _, id := range slices.Sorted(maps.Keys(entries)) {
		if _, existing := listEntry(target, id); existing != nil {
			return invalid(where+"."+field+"."+id, "the source already has this entry; edit it in the configuration instead")
		}
		entry := entries[id]
		entry.Content = append([]*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "id"}, textNode(id)}, entry.Content...)
		target.Content = append(target.Content, entry)
	}
	return nil
}

// pinNodes encodes each pin, without its id, as a mapping whose version is double-quoted.
func pinNodes(pins map[string]Pin) map[string]*yaml.Node {
	nodes := map[string]*yaml.Node{}
	for id, pin := range pins {
		nodes[id] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"},
			versionNode(pin.Version),
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "reason"},
			textNode(pin.Reason),
		}}
	}
	return nodes
}

// exclusionNodes encodes each exclusion, without its id, as a mapping with its reason and, when set, replacedBy and
// basedOn.
func exclusionNodes(exclude map[string]Exclusion) map[string]*yaml.Node {
	nodes := map[string]*yaml.Node{}
	for id, exclusion := range exclude {
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "reason"}, textNode(exclusion.Reason)}}
		if exclusion.ReplacedBy != "" {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "replacedBy"}, textNode(exclusion.ReplacedBy))
		}
		if exclusion.BasedOn != nil {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "basedOn"}, versionNode(*exclusion.BasedOn))
		}
		nodes[id] = node
	}
	return nodes
}

// versionNode encodes a rule version double-quoted, such as "1.3.0", so YAML reads it as text.
func versionNode(version trailguide.RuleVersion) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: version.String(), Style: yaml.DoubleQuotedStyle}
}

// textNode returns a string scalar in the style the encoder chooses for text, quoting it when YAML would
// otherwise read another type.
func textNode(text string) *yaml.Node {
	node := &yaml.Node{}
	// Encoding a string never fails.
	_ = node.Encode(text)
	return node
}

// encodeConfiguration writes an edited document with two-space indentation, keeping folded scalars' values, and
// validates the result.
func encodeConfiguration(document *yaml.Node) ([]byte, error) {
	normalizeFoldedScalars(document)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	if _, err := ParseConfigurationYAML(out.Bytes()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// The YAML encoder can change folded scalar values; literal style preserves them.
func normalizeFoldedScalars(node *yaml.Node) {
	if node.Style&yaml.FoldedStyle != 0 {
		node.Style = node.Style&^yaml.FoldedStyle | yaml.LiteralStyle
	}
	for _, child := range node.Content {
		normalizeFoldedScalars(child)
	}
}

// ReplaceLibrarySkill returns input, a library's configuration, with the entry of its library.skills levels that
// names the skill from replaced by to, at the same level and position, keeping comments and formatting elsewhere. It
// fails when no level lists from. The complete resulting configuration is validated before any bytes are returned.
func ReplaceLibrarySkill(input []byte, from, to string) ([]byte, error) {
	document, data, err := decode.YAML(input, "configuration")
	if err != nil {
		return nil, err
	}
	if _, err := ParseConfiguration(data); err != nil {
		return nil, err
	}
	levels := mappingValue(mappingValue(document.Content[0], "library"), "skills")
	replaced := false
	for _, level := range skills.Levels {
		list := mappingValue(levels, level)
		if list == nil || list.Kind != yaml.SequenceNode {
			continue
		}
		for _, entry := range list.Content {
			if entry.Kind == yaml.ScalarNode && entry.Value == from {
				entry.Value, entry.Style, replaced = to, 0, true
			}
		}
	}
	if !replaced {
		return nil, invalid("library.skills", "no level lists "+from)
	}
	output, err := encodeConfiguration(document)
	if err != nil {
		return nil, err
	}
	if _, err := ParseConfigurationYAML(output); err != nil {
		return nil, err
	}
	return output, nil
}
