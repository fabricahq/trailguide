// Render resolved rules into standalone generated Markdown files.

package build

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// rulePath maps one source-qualified identity to its standalone generated path.
func rulePath(rule trailguide.Rule) string {
	return "rules/" + strings.Replace(rule.ID, ":", "/", 1) + ".md"
}

// renderRules renders a resolve result without modifying inputs or writing files.
// Each rule gets one standalone document; combined group delivery is a separate capability.
func renderRules(resolved resolution) (map[string]string, error) {
	output := map[string]string{}
	paths := renderSourcePaths(resolved)
	for _, group := range resolved.Groups {
		for _, active := range group.Rules {
			file := rulePath(active.Rule)
			text, err := renderRule(active, paths[active.Origin.Source], file)
			if err != nil {
				return nil, err
			}
			if _, exists := output[file]; exists {
				return nil, invalid(file, "duplicate output rule")
			}
			output[file] = generatedMarkdown(text)
		}
	}
	if err := validateOutputPaths(output); err != nil {
		return nil, err
	}
	return output, nil
}

// renderSourcePaths selects the retained inventory by the resolved rule's actual origin.
func renderSourcePaths(resolved resolution) map[string][]string {
	paths := map[string][]string{"local": resolved.LocalPaths}
	for _, source := range resolved.Sources {
		paths[source.Name] = source.Paths
	}
	return paths
}

// renderRule wraps rewritten guidance in applicability, origin, and original frontmatter sections.
func renderRule(active resolvedRule, paths []string, outputPath string) (string, error) {
	if !utf8.ValidString(active.Rule.Document) {
		return "", invalid(active.Rule.ID, "expected UTF-8 text")
	}
	split, err := trailguide.SplitDocument(active.Rule.Document, active.Rule.ID)
	if err != nil {
		return "", err
	}
	body, err := renderBody(split.Body, active, paths, outputPath)
	if err != nil {
		return "", err
	}
	source, err := sourceLink(active.Origin, outputPath)
	if err != nil {
		return "", err
	}
	r := active.Rule
	titleHeading := "#"
	if outputPath != rulePath(r) {
		titleHeading = "###"
	}
	sectionHeading := titleHeading + "#"
	lines := []string{titleHeading + " " + escapeText(r.Title), "", "Rule ID: `" + r.ID + "`", ""}
	if version := active.Origin.Version; version != nil {
		lines = append(lines, "Version: "+version.String(), "")
	}
	lines = append(lines, "**When to read:** "+escapeText(r.WhenToRead), "", "**Impact:** "+escapeText(string(r.Impact)), "", "**Why it matters:** "+escapeText(r.ImpactDescription), "", sectionHeading+" Guidance", "", body, "", sectionHeading+" Source and attribution", "", "**Rule source:** [Original rule]("+source+")")
	if outputPath != rulePath(r) {
		lines = append(lines, "", "**Separate rule file:** ["+escapeText(r.Title)+"]("+rules.RelativeLink(outputPath, rulePath(r))+")")
	}
	for _, attribution := range r.Attribution {
		lines = append(lines, "", "**Attribution:** ["+escapeText(attribution.Description)+"](<"+strings.NewReplacer("<", "%3C", ">", "%3E").Replace(attribution.URL)+">)")
	}
	if license := active.License; license != nil {
		if license.SPDXExpression != nil {
			lines = append(lines, "", "**Declared license:** "+escapeText(*license.SPDXExpression))
		}
		for _, mapping := range licenseMappings(active.Origin.Source, license) {
			lines = append(lines, "", "- ["+escapeText(path.Base(mapping.Generated))+"]("+rules.RelativeLink(outputPath, mapping.Generated)+")")
		}
	}
	fence := "```"
	for _, run := range strings.FieldsFunc(split.Frontmatter, func(r rune) bool { return r != '`' }) {
		if len(run) >= len(fence) {
			fence = strings.Repeat("`", len(run)+1)
		}
	}
	lines = append(lines, "", sectionHeading+"# Source metadata", "", fence+"yaml\n"+split.Frontmatter+"\n"+fence)
	return strings.Join(lines, "\n") + "\n", nil
}

// escapeText keeps metadata on one line and escapes Markdown punctuation.
func escapeText(value string) string {
	var result strings.Builder
	for _, r := range strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(value) {
		if strings.ContainsRune("\\`*_{}[]()<>#+|", r) {
			result.WriteByte('\\')
		}
		result.WriteRune(r)
	}
	return result.String()
}

// workspaceLink links from generated output back to authored input, or to an imported source's file at library path
// file, where the source's snapshot in vendor/ stores it.
func workspaceLink(outputPath, source, file string) string {
	if source == "local" {
		return rules.RelativeLink("generated/"+outputPath, authoredLocation(file))
	}
	return rules.RelativeLink("generated/"+outputPath, "vendor/"+source+"/"+librarytree.SnapshotPath(file))
}

// sourceLink prefers a recognized repository's immutable commit URL and falls back to retained source.
func sourceLink(origin ruleOrigin, outputPath string) (string, error) {
	if origin.Repository != "" {
		raw, _ := json.Marshal(origin.Repository)
		repository, err := rules.ParseRepository(raw, origin.Source)
		if err != nil {
			return "", err
		}
		file := librarytree.RepositoryPath(origin.File)
		if origin.legacyLayout {
			file = origin.File
		}
		if link, ok := repository.FileURL(origin.Commit, file, false); ok {
			return link, nil
		}
	}
	return workspaceLink(outputPath, origin.Source, origin.File), nil
}

// relocatedURL validates ownership and links local destinations to terms or retained files.
func relocatedURL(destination string, active resolvedRule, paths []string, outputPath string) (string, error) {
	if strings.HasPrefix(destination, "#") {
		if outputPath != rulePath(active.Rule) {
			return rules.RelativeLink(outputPath, rulePath(active.Rule)) + destination, nil
		}
		return destination, nil
	}
	resolveTarget := rules.LibraryTarget
	if active.Origin.Source == "local" {
		resolveTarget = authoredTarget
	}
	target, suffix, local, err := resolveTarget(destination, active.Origin.File)
	if err != nil {
		return "", err
	}
	if !local {
		return destination, nil
	}
	if err := rules.RequireAllowedTarget(active.Origin.File, target, rules.LicensePaths(active.License)); err != nil {
		return "", err
	}
	if license := active.License; license != nil {
		for _, mapping := range licenseMappings(active.Origin.Source, license) {
			if mapping.Source == target {
				return rules.RelativeLink(outputPath, mapping.Generated) + suffix, nil
			}
		}
	}
	if slices.Contains(paths, target) {
		return workspaceLink(outputPath, active.Origin.Source, target) + suffix, nil
	}
	if rules.AssetDirectory(target) != "" {
		return "", invalid(active.Rule.ID, "missing asset link destination: "+destination)
	}
	return "", invalid(active.Rule.ID, "missing retained link destination: "+destination)
}

// licenseMapping identifies a source term file and its fixed generated destination.
type licenseMapping struct{ Source, Generated, Kind string }

// licenseMappings assigns stable license and notice names while preserving declaration order.
func licenseMappings(source string, license *trailguide.LicenseDeclaration) []licenseMapping {
	root := "libraries/" + source + "/licenses"
	result := []licenseMapping{}
	for _, file := range license.Files {
		result = append(result, licenseMapping{file, root + "/LICENSE.md", "license"})
	}
	for i, file := range license.AttributionFiles {
		result = append(result, licenseMapping{file, fmt.Sprintf("%s/notices/%03d.md", root, i+1), "notice"})
	}
	return result
}
