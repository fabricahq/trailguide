// Assemble generated guidance, untouched declared terms, and machine-readable provenance in memory.

package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/trailguide"
)

// prepare combines rendering and group discovery pages with terms and provenance, returning no partial output.
// It accepts a resolve result and performs no filesystem or network operations.
func prepare(resolved resolution, options Options) (Output, error) {
	if err := rules.ValidateToolVersion(options.ToolVersion); err != nil {
		return Output{}, err
	}
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
	files["rules/README.md"] = []byte(generatedMarkdown("# Resolved rules\n\nThese files contain the complete resolved definitions after exclusions, replacements, and local additions. Start with [RULES.md](../RULES.md). See [provenance.json](../provenance.json) for origins. Edit source inputs and rebuild.\n"))
	files["groups/README.md"] = []byte(generatedMarkdown("# Rule groups\n\nStart with [RULES.md](../RULES.md), then open relevant group indexes and read each applicable rule in full. Each group page provides reading instructions and either complete rules or summaries with explicit links to the full definitions.\n"))
	provenance, err := renderProvenance(resolved, options.ToolVersion)
	if err != nil {
		return Output{}, err
	}
	files["provenance.json"] = provenance
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
	sections = append(sections, "[provenance.json](../../provenance.json) records source revisions, origins, and original and generated term paths.", "Use [RULES.md](../../RULES.md) to find resolved rules. These files are generated; edit source inputs and rebuild.")
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

// provenanceLicense records original and generated term locations without copying their contents.
type provenanceLicense struct {
	SPDXExpression            *string  `json:"spdxExpression"`
	Files                     []string `json:"files"`
	AttributionFiles          []string `json:"attributionFiles"`
	GeneratedFiles            []string `json:"generatedFiles"`
	GeneratedAttributionFiles []string `json:"generatedAttributionFiles"`
}

// provenanceSource records the requested versions, the library release that supplied group metadata and terms,
// and the requested and actual selection. Pins, ref, release, and ruleSelection are omitted when empty, as in
// the source's _source.json.
type provenanceSource struct {
	Name string `json:"name"`
	// Kind is library, or non-library for a repository without a library manifest, which supplies only skills.
	Kind          string               `json:"kind"`
	Repository    string               `json:"repository"`
	Pins          map[string]rules.Pin `json:"pins,omitempty"`
	Ref           string               `json:"ref,omitempty"`
	Release       int                  `json:"release,omitempty"`
	Commit        string               `json:"resolvedCommit"`
	Groups        []string             `json:"groups"`
	Selection     rules.GroupSelection `json:"groupSelection"`
	RuleSelection []string             `json:"ruleSelection,omitempty"`
	LicenseFiles  []string             `json:"licenseFiles"`
	License       *provenanceLicense   `json:"license"`
	// SkillInclude is the source's skills.include, and Skills the IDs of the skills it installs.
	SkillInclude []string `json:"skillInclude"`
	Skills       []string `json:"skills"`
	// Curated records, by name in the library's manifest, the revision of each source the library curates the
	// project's installed skills from; it is omitted when there are none.
	Curated map[string]provenanceCurated `json:"curated,omitempty"`
}

// provenanceCurated is the revision of one source a library curates skills from, as the project resolved it.
// Source names the project's own source that supplied the revision, when one declares the same repository.
type provenanceCurated struct {
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	Ref        string `json:"ref,omitempty"`
	Release    int    `json:"release,omitempty"`
	Commit     string `json:"resolvedCommit"`
	Source     string `json:"source,omitempty"`
}

// provenanceSkill records an installed skill's origin, level, why it is installed, the skills it uses, and its
// installed folders.
type provenanceSkill struct {
	ID     string                `json:"id"`
	Name   string                `json:"name"`
	Type   *string               `json:"type"`
	Origin provenanceSkillOrigin `json:"origin"`
	// Level is the skill's level in its library, or null for an authored skill or one from a repository that isn't
	// a library. RequiredBy, RecommendedBy, and OfferedBy name the library that set that level, and its release, and
	// one of them is present only when the level brought the skill in.
	Level         *string          `json:"level"`
	RequiredBy    *provenanceLevel `json:"requiredBy,omitempty"`
	RecommendedBy *provenanceLevel `json:"recommendedBy,omitempty"`
	OfferedBy     *provenanceLevel `json:"offeredBy,omitempty"`
	// Included is false for a skill installed only because other skills use it. UsedBy and Uses list, sorted, the
	// project IDs of the installed skills that use it and that it uses.
	Included bool     `json:"included"`
	UsedBy   []string `json:"usedBy"`
	Uses     []string `json:"uses"`
	// RenamedFrom is the name the skill's SKILL.md declares when the project installs it under Name instead, or null.
	RenamedFrom *string `json:"renamedFrom"`
	// Targets lists the folders the skill is installed in, by path from the repository root.
	Targets []string `json:"targets"`
	// Upstream is the origin of the skill an authored skill replaces, Reason the exclusion's reason, and BasedOn the
	// commit the replacement was copied from; each is null when the skill replaces none, and BasedOn also when the
	// exclusion records none.
	Upstream *provenanceSkillOrigin `json:"upstream"`
	Reason   *string                `json:"replacementReason"`
	BasedOn  *string                `json:"basedOn"`
}

// provenanceLevel names the library, by source name, that set a skill's level, and the library release whose
// manifest set it, or null for a revision other than a library release.
type provenanceLevel struct {
	Library string `json:"library"`
	Release *int   `json:"release"`
}

// setLevel records skill's level in record, and which library set it when the level brought the skill in: a
// required skill always, a recommended one when include, the source's skills.include, has recommended or "*", and
// an offered one when it has "*". A skill the project names by ID and no level brings in records only its level.
func setLevel(record *provenanceSkill, skill resolvedSkill, include []string) {
	if skill.Level == "" {
		return
	}
	record.Level = &skill.Level
	by := &provenanceLevel{Library: skill.Source}
	if skill.LevelRelease != 0 {
		by.Release = &skill.LevelRelease
	}
	every := slices.Contains(include, skills.Wildcard)
	switch {
	case skill.Level == skills.LevelRequired:
		record.RequiredBy = by
	case skill.Level == skills.LevelRecommended && (every || slices.Contains(include, skills.Recommended)):
		record.RecommendedBy = by
	case skill.Level == skills.LevelOffered && every:
		record.OfferedBy = by
	}
}

// provenanceSkillOrigin identifies the source, repository, and commit that supplied a skill, and its folder there:
// in the source's repository, or, for an authored skill, whose source, repository, and commit are null, in the
// Trail Guide directory. Version and release are null until skills have versions.
type provenanceSkillOrigin struct {
	Source     *string `json:"source"`
	Repository *string `json:"repository"`
	Ref        *string `json:"ref"`
	Commit     *string `json:"resolvedCommit"`
	Folder     string  `json:"folder"`
	Version    *string `json:"version"`
	Release    *int    `json:"release"`
	// CuratedFrom names, for a curated skill, the source in the library's manifest it was curated from, whose
	// repository and commit Repository and Commit then are; it is null for any other skill.
	CuratedFrom *string `json:"curatedFrom"`
}

// skillOrigin returns the origin of skill: its source, repository, ref, and commit, which are null for an authored
// skill, and its folder.
func skillOrigin(skill resolvedSkill) provenanceSkillOrigin {
	origin := provenanceSkillOrigin{Folder: skill.Folder}
	if skill.Source != "" {
		origin.Source, origin.Repository, origin.Ref, origin.Commit = &skill.Source, &skill.Repository, nullableText(skill.Ref), &skill.Commit
	}
	origin.CuratedFrom = nullableText(skill.Curated)
	return origin
}

// provenanceGroup retains all guidance plus the sources chosen for display.
type provenanceGroup struct {
	ID        string          `json:"id"`
	Guidance  []groupGuidance `json:"guidance"`
	Effective []string        `json:"effectiveGuidanceSources"`
}

// provenanceRule records effective origin, replacement history, and declared attribution.
type provenanceRule struct {
	ID           string                   `json:"id"`
	Group        string                   `json:"group"`
	Origin       provenanceOrigin         `json:"origin"`
	Upstream     *provenanceOrigin        `json:"upstream"`
	Reason       *string                  `json:"replacementReason"`
	BasedOn      *trailguide.RuleVersion  `json:"basedOn"`
	LicenseBasis string                   `json:"licenseBasis"`
	License      *provenanceLicense       `json:"license"`
	Attribution  []trailguide.Attribution `json:"attribution"`
}

// termProvenance maps declared source terms to retained workspace and generated paths. The original paths are
// library paths, or, with a snapshot directory such as vendor/team/, where the snapshot stores the files.
func termProvenance(source, snapshot string, license *rules.LicenseDeclaration) *provenanceLicense {
	if license == nil {
		return nil
	}
	record := &provenanceLicense{SPDXExpression: license.SPDXExpression, Files: []string{}, AttributionFiles: []string{}, GeneratedFiles: []string{}, GeneratedAttributionFiles: []string{}}
	for _, mapping := range licenseMappings(source, license) {
		original := mapping.Source
		if snapshot != "" {
			original = snapshot + librarytree.SnapshotPath(mapping.Source)
		}
		if mapping.Kind == "license" {
			record.Files = append(record.Files, original)
			record.GeneratedFiles = append(record.GeneratedFiles, mapping.Generated)
		} else {
			record.AttributionFiles = append(record.AttributionFiles, original)
			record.GeneratedAttributionFiles = append(record.GeneratedAttributionFiles, mapping.Generated)
		}
	}
	return record
}

// renderProvenance serializes stable identities and policy outcomes, without redundant raw documents.
func renderProvenance(resolved resolution, version string) ([]byte, error) {
	result := struct {
		GeneratedNotice string             `json:"generatedNotice"`
		ToolVersion     string             `json:"toolVersion"`
		Sources         []provenanceSource `json:"sources"`
		Groups          []provenanceGroup  `json:"groups"`
		Rules           []provenanceRule   `json:"rules"`
		Skills          []provenanceSkill  `json:"skills"`
	}{GeneratedNotice: generatedNotice + " " + strings.ReplaceAll(regenerationNotice, "`", ""), ToolVersion: version, Sources: []provenanceSource{}, Groups: []provenanceGroup{}, Rules: []provenanceRule{}, Skills: []provenanceSkill{}}
	includes := map[string][]string{}
	for _, source := range resolved.Sources {
		includes[source.Name] = source.SkillSelection
		kind := "library"
		if source.NonLibrary {
			kind = "non-library"
		}
		record := provenanceSource{Name: source.Name, Kind: kind, Repository: source.Repository, Ref: source.Ref, Release: source.Release, Commit: source.Commit, Groups: source.Groups, Selection: source.Selection, RuleSelection: source.Rules, LicenseFiles: rules.LicensePaths(source.License), License: termProvenance(source.Name, "", source.License), SkillInclude: nonNilList(source.SkillSelection), Skills: nonNilList(source.Skills)}
		if len(source.Pins) > 0 {
			record.Pins = source.Pins
		}
		for name, curated := range source.Curated {
			if record.Curated == nil {
				record.Curated = map[string]provenanceCurated{}
			}
			kind := "library"
			if curated.NonLibrary {
				kind = "non-library"
			}
			record.Curated[name] = provenanceCurated{Kind: kind, Repository: curated.Repository, Ref: curated.Ref.String(), Release: curated.Release, Commit: curated.Commit, Source: curated.Source}
		}
		result.Sources = append(result.Sources, record)
	}
	for _, group := range resolved.Groups {
		effective := []string{}
		for _, guidance := range group.EffectiveGuidance {
			effective = append(effective, guidance.Source)
		}
		guidance := slices.Clone(group.Guidance)
		// Keep source guidance in its established order, with local guidance last.
		slices.SortStableFunc(guidance, func(a, b groupGuidance) int {
			if a.Source == "local" && b.Source != "local" {
				return 1
			}
			if a.Source != "local" && b.Source == "local" {
				return -1
			}
			return 0
		})
		result.Groups = append(result.Groups, provenanceGroup{group.ID, guidance, effective})
		for _, active := range group.Rules {
			basis := "undeclared"
			if active.License != nil {
				basis = "library"
			}
			result.Rules = append(result.Rules, provenanceRule{ID: active.Rule.ID, Group: active.Rule.Group, Origin: *originProvenance(&active.Origin), Upstream: originProvenance(active.Upstream), Reason: nullableText(active.Reason), BasedOn: active.BasedOn, LicenseBasis: basis, License: termProvenance(active.Origin.Source, "vendor/"+active.Origin.Source+"/", active.License), Attribution: active.Rule.Attribution})
		}
	}
	for _, skill := range resolved.Skills {
		record := provenanceSkill{ID: skill.ID, Name: skill.Name, Type: nullableText(skill.Type), Origin: skillOrigin(skill), RenamedFrom: nullableText(skill.RenamedFrom), Targets: skill.Targets}
		setLevel(&record, skill, includes[skill.Source])
		if replaced := skill.Replaces; replaced != nil {
			upstream := skillOrigin(replaced.Upstream)
			record.Upstream, record.Reason, record.BasedOn = &upstream, &replaced.Reason, nullableText(replaced.BasedOn.Canonical())
		}
		record.Included, record.UsedBy, record.Uses = skill.Included, skill.UsedBy, skill.Uses
		result.Skills = append(result.Skills, record)
	}
	slices.SortFunc(result.Sources, func(a, b provenanceSource) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(result.Groups, func(a, b provenanceGroup) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(result.Rules, func(a, b provenanceRule) int { return strings.Compare(a.ID, b.ID) })
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetIndent("", "  ")
	// This is a standalone JSON file; keep characters such as <, >, and & readable.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return nil, fmt.Errorf("encode provenance: %w", err)
	}
	return data.Bytes(), nil
}

// provenanceOrigin preserves explicit nulls for unavailable repository identity and version fields. Commit is
// the commit that supplied the rule's files; Version and Release are null for local rules and for imported
// files that aren't a published version.
type provenanceOrigin struct {
	Source     string                  `json:"source"`
	File       string                  `json:"file"`
	Repository *string                 `json:"repository"`
	Ref        *string                 `json:"ref"`
	Commit     *string                 `json:"resolvedCommit"`
	Version    *trailguide.RuleVersion `json:"version"`
	Release    *int                    `json:"release"`
}

// nonNilList returns list, or an empty list for nil, so JSON shows an empty list rather than null.
func nonNilList(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// nullableText represents absent optional provenance text as JSON null.
func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// originProvenance converts internal identity values to the public nullable provenance shape.
func originProvenance(origin *ruleOrigin) *provenanceOrigin {
	if origin == nil {
		return nil
	}
	result := &provenanceOrigin{origin.Source, origin.File, nullableText(origin.Repository), nullableText(origin.Ref), nullableText(origin.Commit), origin.Version, nil}
	if origin.Version != nil {
		release := origin.Release
		result.Release = &release
	}
	return result
}
