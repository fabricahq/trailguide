// Assemble generated guidance and untouched declared terms in memory.

package build

import (
	"bytes"
	"maps"
	"path"
	"strings"

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
	// Only a project with sources has a lockfile to point to.
	lock := ""
	if len(resolved.Sources) > 0 {
		lock = " [lock.json](../../../lock.json) records each source's revision and rule versions."
	}
	files["rules/README.md"] = []byte(generatedMarkdown("# Resolved rules\n\nThese files contain the complete resolved definitions after exclusions, replacements, and local additions. Start with [RULES.md](../RULES.md)." + lock + " Edit source inputs and rebuild.\n"))
	files["groups/README.md"] = []byte(generatedMarkdown("# Rule groups\n\nStart with [RULES.md](../RULES.md), then open relevant group indexes and read each applicable rule in full. Each group page provides reading instructions and either complete rules or summaries with explicit links to the full definitions.\n"))
	files[attributesFile] = []byte(attributes)
	folders, skillFiles, err := renderSkills(resolved.Skills, excludedSkills(resolved.Sources))
	if err != nil {
		return Output{}, err
	}
	maps.Copy(files, skillFiles)
	if len(resolved.References) > 0 {
		maps.Copy(files, resolved.References)
		files[ReferencesDirectory+"/README.md"] = []byte(generatedMarkdown(referencesReadme))
	}
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

// libraryReadme describes the license terms source declares, linking the copies Generate keeps beside it, without
// interpreting their legal meaning. A source's revision and rule versions are in the lockfile, and trailguide project
// list rules shows them, so the README doesn't repeat them.
func libraryReadme(source resolvedSource) string {
	file := "libraries/" + source.Name + "/README.md"
	sections := []string{"# " + escapeText(source.Name)}
	mappings := []licenseMapping{}
	if source.License != nil {
		mappings = licenseMappings(source.Name, source.License)
	}
	// Only a source that declares license or notice files has copies here to describe.
	if len(mappings) > 0 {
		sections = append(sections, "This folder retains byte-for-byte copies of the license and notice files this source declares. Do not edit these copies; change the upstream repository and run `trailguide project sync`.")
	}
	sections = append(sections, "## License terms")
	if source.License == nil {
		sections = append(sections, "No license declaration was supplied.")
	} else if source.License.SPDXExpression != nil {
		sections = append(sections, "**Declared license:** "+escapeText(*source.License.SPDXExpression))
	}
	for _, mapping := range mappings {
		sections = append(sections, "- ["+escapeText(path.Base(mapping.Generated))+"]("+rules.RelativeLink(file, mapping.Generated)+")")
	}
	sections = append(sections, "[lock.json](../../../../lock.json) records the revision this source resolved to and the version of each of its rules; run `trailguide project list rules` to see them with how the project uses each rule.")
	return strings.Join(sections, "\n\n") + "\n"
}

// attributesFile is the Git attributes file Generate writes in the generated folder, and attributes its contents,
// which mark every generated file as generated, so GitHub collapses them in pull request diffs. Vendor snapshots stay
// expanded: they hold each upstream change once, which generated output repeats.
const (
	attributesFile = ".gitattributes"
	attributes     = "# Generated by Trail Guide. Do not edit this file manually.\n* linguist-generated\n"
)
