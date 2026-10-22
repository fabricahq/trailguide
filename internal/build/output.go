// Assemble generated guidance and untouched declared terms in memory.

package build

import (
	"bytes"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// prepare combines rendering and group discovery pages with terms, returning no partial output.
// It accepts a resolve result and performs no filesystem or network operations.
func prepare(resolved resolution, options Options) (Output, error) {
	rendered, err := renderRules(resolved)
	if err != nil {
		return Output{}, err
	}
	// A provisional delivery default, like defaultIndexMaxLines, not a threshold validated against agent compliance.
	inlineMaxBytes := 8 * 1024
	if options.GroupInlineMaxBytes != nil {
		inlineMaxBytes = *options.GroupInlineMaxBytes
	}
	indexes, err := renderIndexes(resolved, options.IndexMaxLines, inlineMaxBytes)
	if err != nil {
		return Output{}, err
	}
	files := map[string][]byte{}
	for file, text := range rendered {
		files[file] = []byte(text)
	}
	for file, text := range indexes {
		if _, ok := files[file]; ok {
			return Output{}, invalid(file, "duplicate generated path")
		}
		files[file] = []byte(text)
	}
	for _, source := range resolved.Sources {
		if license := source.License; license != nil {
			for _, mapping := range licenseMappings(source.Name, license) {
				data, ok := source.Files[mapping.Source]
				if !ok {
					return Output{}, invalid(source.Name+":"+mapping.Source, "missing declared license or notice bytes")
				}
				if previous, ok := files[mapping.Generated]; ok && !bytes.Equal(previous, data) {
					return Output{}, invalid(mapping.Generated, "conflicting declared terms")
				}
				files[mapping.Generated] = bytes.Clone(data)
			}
		}
		files["libraries/"+source.Name+"/README.md"] = []byte(generatedMarkdown(libraryReadme(source)))
	}
	files["rules/README.md"] = []byte(generatedMarkdown("# Resolved rules\n\nThese files contain the complete resolved definitions after exclusions, replacements, and local additions. Start with [RULES.md](../RULES.md). [lock.json](../../lock.json) records each source's revision and rule versions. Edit source inputs and rebuild.\n"))
	files["groups/README.md"] = []byte(generatedMarkdown("# Rule groups\n\nStart with [RULES.md](../RULES.md), then open relevant group indexes and read each applicable rule in full. Each group page provides reading instructions and either complete rules or summaries with explicit links to the full definitions.\n"))
	folders, skillFiles, err := renderSkills(resolved.Skills, excludedSkills(resolved.Sources))
	if err != nil {
		return Output{}, err
	}
	maps.Copy(files, skillFiles)
	if err := validateOutputPaths(files); err != nil {
		return Output{}, err
	}
	ids := []string{}
	renamed := []RenamedSkill{}
	for _, skill := range resolved.Skills {
		ids = append(ids, skill.ID)
		if skill.RenamedFrom != "" {
			renamed = append(renamed, RenamedSkill{ID: skill.ID, Source: skill.Source, Folder: skill.Folder, From: skill.RenamedFrom, To: skill.Name})
		}
	}
	return Output{Files: files, Skills: folders, SkillIDs: ids, RenamedSkills: renamed}, nil
}

// libraryReadme summarizes the imported library release, each imported rule's version, the source's pins, and
// generated terms, without interpreting their legal meaning. It says when the source imports unreleased changes.
func libraryReadme(source resolvedSource) string {
	file := "libraries/" + source.Name + "/README.md"
	sections := []string{"# " + escapeText(source.Name)}
	// Only a library that declares license or notice files has copies here to describe.
	if source.License != nil && len(licenseMappings(source.Name, source.License)) > 0 {
		sections = append(sections, "This folder retains byte-for-byte copies of declared library license and notice files. Do not edit these copies; change the upstream library and run `trailguide project sync`.")
	}
	sections = append(sections, "**Repository:** "+escapeText(source.Repository))
	if source.NonLibrary {
		sections = append(sections, "**Not a library.** The repository has no library manifest at the resolved commit, so this source supplies only skills, all from that commit.")
	}
	if source.Release != 0 {
		sections = append(sections, fmt.Sprintf("**Library release:** release/%d", source.Release))
	}
	if source.Ref != "" {
		sections = append(sections, "**Requested revision:** "+escapeText(source.Ref))
	}
	sections = append(sections, "**Resolved commit:** `"+source.Commit+"`")
	if source.Ref != "" && source.Release == 0 && slices.ContainsFunc(slices.Collect(maps.Values(source.Versions)), func(rule library.ImportedRule) bool { return rule.Version == nil }) {
		sections = append(sections, "**Imported from unreleased changes.** This source's ref isn't a library release, so the source doesn't follow rule versions: rules with unreleased changes have no version to cite.")
	}
	if !source.NonLibrary {
		sections = append(sections, "## Rule versions", ruleVersionTable(source))
	}
	if len(source.Skills) > 0 {
		items := []string{}
		for _, id := range source.Skills {
			item := "- `" + id + "`"
			if fork, forked := source.Configured.SkillForks[id]; forked {
				item += ": replaced by the authored skill `" + fork.As + "`"
			} else if _, excluded := source.Configured.SkillExclude[id]; excluded {
				item += ": excluded"
			}
			items = append(items, item)
		}
		sections = append(sections, "## Skills", "This source supplies these skills from the resolved commit, and the project installs each one it doesn't exclude or replace. [SKILLS.md](../../SKILLS.md) says what each one does, where it's installed, and why the project excludes or replaces one.", strings.Join(items, "\n"))
	}
	if len(source.Pins) > 0 {
		sections = append(sections, "## Pins", "`trailguide project update` keeps these rules at their pinned versions.", pinList(source.Pins))
	}
	sections = append(sections, "## License terms")
	if source.License == nil {
		sections = append(sections, "No library license declaration was supplied.")
	}
	if license := source.License; license != nil {
		if license.SPDXExpression != nil {
			sections = append(sections, "**Declared license:** "+escapeText(*license.SPDXExpression))
		}
		for _, mapping := range licenseMappings(source.Name, license) {
			sections = append(sections, "- ["+escapeText(path.Base(mapping.Generated))+"]("+rules.RelativeLink(file, mapping.Generated)+")")
		}
	}
	sections = append(sections, "[lock.json](../../../lock.json) records this source's revision and rule versions.", "Use [RULES.md](../../RULES.md) to find resolved rules. These files are generated; edit source inputs and rebuild.")
	return strings.Join(sections, "\n\n") + "\n"
}

// ruleVersionTable lists each imported rule of source with its version, the library release that published it, and
// whether agents read it: an excluded rule, which the source still imports so updates can report its changes, is
// marked excluded, or replaced by its local rule, and a retired rule the source still imports is marked retired,
// with the ref or pin that keeps it.
func ruleVersionTable(source resolvedSource) string {
	versions := source.Versions
	if len(versions) == 0 {
		return "This source imports no rules."
	}
	rows := []string{"| Rule | Version | Library release | Status |", "| --- | --- | --- | --- |"}
	for _, id := range slices.Sorted(maps.Keys(versions)) {
		rule := versions[id]
		status := "Active"
		pin, pinned := source.Pins[id]
		switch exclusion, excluded := source.Configured.Exclusion(id); {
		case excluded && exclusion.ReplacedBy != "":
			status = "Replaced by `" + exclusion.ReplacementFile() + "`"
		case excluded:
			status = "Excluded"
		case slices.Contains(source.Retired, id) && source.Ref != "":
			status = "Retired upstream; kept by ref " + source.Ref
		case slices.Contains(source.Retired, id) && pinned:
			status = "Retired, pinned at " + pin.Version.String()
		case slices.Contains(source.Retired, id):
			status = "Retired; the next update drops it"
		}
		if rule.Version == nil {
			rows = append(rows, "| `"+id+"` | No version | Unreleased | "+status+" |")
			continue
		}
		rows = append(rows, fmt.Sprintf("| `%s` | %s | release/%d | %s |", id, rule.Version, rule.Release, status))
	}
	return strings.Join(rows, "\n")
}

// pinList lists each pinned rule, in ID order, with its pinned version and the reason the configuration gives.
func pinList(pins map[string]rules.Pin) string {
	items := []string{}
	for _, id := range slices.Sorted(maps.Keys(pins)) {
		items = append(items, fmt.Sprintf("- `%s`: %s. Reason: %s", id, pins[id].Version, escapeText(pins[id].Reason)))
	}
	return strings.Join(items, "\n")
}
