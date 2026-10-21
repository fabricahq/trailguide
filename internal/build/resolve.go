// Apply adoption policies while retaining source identity and original bytes.

package build

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// ruleOrigin identifies an effective definition; local definitions have no repository, commit, or version.
type ruleOrigin struct {
	Source     string `json:"source"`
	File       string `json:"file"`
	Repository string `json:"repository,omitempty"`
	Ref        string `json:"ref,omitempty"`
	// Commit is the commit that supplied the rule's files.
	Commit string `json:"resolvedCommit,omitempty"`
	// Version is nil, and Release is 0, for a local rule or an imported rule that isn't a published version.
	Version *trailguide.RuleVersion `json:"version,omitempty"`
	Release int                     `json:"release,omitempty"`
	// legacyLayout reports that Commit keeps File at the repository root, in the layout before version 2.
	legacyLayout bool
}

// resolvedRule owns one parsed effective document. Upstream is non-nil only for replacements.
type resolvedRule struct {
	Rule     trailguide.Rule `json:"rule"`
	Origin   ruleOrigin      `json:"origin"`
	Upstream *ruleOrigin     `json:"upstream"`
	Reason   string          `json:"replacementReason,omitempty"`
	// BasedOn is the library version a replacement incorporates, as its exclusion records it, or nil.
	BasedOn *trailguide.RuleVersion        `json:"basedOn,omitempty"`
	License *trailguide.LicenseDeclaration `json:"license"`
}

// groupGuidance identifies the source of one complete group metadata definition.
type groupGuidance struct {
	Source   string                   `json:"source"`
	Metadata trailguide.GroupMetadata `json:"metadata"`
}

// resolvedGroup contains effective rules and guidance chosen by resolve, plus original guidance for provenance.
type resolvedGroup struct {
	ID                string          `json:"id"`
	EffectiveGuidance []groupGuidance `json:"effectiveGuidance"`
	Guidance          []groupGuidance `json:"guidance"`
	Rules             []resolvedRule  `json:"rules"`
}

// resolvedSource records the adopted revision and complete retained inventory, including excluded rules.
// Files contains supporting bytes and inactive upstream documents; active rules own their original documents.
type resolvedSource struct {
	Name       string                          `json:"name"`
	Repository string                          `json:"repository"`
	Ref        string                          `json:"ref,omitempty"`
	Pins       map[string]rules.Pin            `json:"pins"`
	Release    int                             `json:"release,omitempty"`
	Commit     string                          `json:"resolvedCommit"`
	Selection  rules.GroupSelection            `json:"groupSelection"`
	Rules      []string                        `json:"ruleSelection"`
	Groups     []string                        `json:"groups"`
	Versions   map[string]library.ImportedRule `json:"rules"`
	// Configured is the source's configuration, whose exclusions the library README marks in its rule versions table.
	Configured rules.Source `json:"-"`
	// Retired lists the rules the library retired, as the source record does, so the README marks one it imports.
	Retired []string `json:"-"`
	// NonLibrary marks a source whose repository isn't a library; SkillSelection is its skills.include, and Skills
	// lists the IDs of the skills it installs, sorted.
	NonLibrary     bool     `json:"-"`
	SkillSelection []string `json:"-"`
	Skills         []string `json:"-"`
	// Curated is the snapshot's record of the revisions its curated skills come from.
	Curated map[string]library.CuratedSource `json:"-"`
	License *trailguide.LicenseDeclaration   `json:"license"`
	Paths   []string                         `json:"paths"`
	Files   map[string][]byte                `json:"retainedFiles"`
}

// resolution owns effective rules; supporting bytes are shared read-only with the input catalogs.
// LocalPaths lists every supplied local path for link resolution, including metadata and attachments.
// LocalFiles holds supporting bytes; active rules own their documents.
type resolution struct {
	// Skills lists the skills the project installs, by project ID.
	Skills     []resolvedSkill   `json:"skills"`
	Groups     []resolvedGroup   `json:"groups"`
	Sources    []resolvedSource  `json:"sources"`
	LocalPaths []string          `json:"localPaths"`
	LocalFiles map[string][]byte `json:"localSupportingFiles"`
}

// resolve applies exclusions and replacements to parsed catalogs, then adds local rules.
// Configuration must come from ParseConfiguration. Every candidate document is validated
// before exceptions, and any error returns a zero result. Inputs are never modified.
func resolve(config rules.Configuration, libraries map[string]Library, localFiles map[string][]byte) (resolution, error) {
	result := resolution{Groups: []resolvedGroup{}, Sources: []resolvedSource{}, LocalPaths: slices.Sorted(maps.Keys(localFiles)), LocalFiles: map[string][]byte{}}
	groups := map[string]*resolvedGroup{}
	declared := map[string]bool{}
	for _, source := range config.Sources {
		declared[source.Name] = true
	}
	for _, name := range slices.Sorted(maps.Keys(libraries)) {
		if !declared[name] {
			return resolution{}, invalid(name, "library is not declared in configuration")
		}
	}
	localRules, err := parseLocal(localFiles, groups)
	if err != nil {
		return resolution{}, err
	}
	if err := validateLocalLinks(localFiles, localRules); err != nil {
		return resolution{}, err
	}
	for file, data := range localFiles {
		if _, ok := localRules[file]; !ok {
			result.LocalFiles[file] = data
		}
	}
	used := map[string]bool{}
	for _, source := range config.Sources {
		supplied, ok := libraries[source.Name]
		if !ok {
			return resolution{}, invalid(source.Name, "missing library; load or sync the source")
		}
		if !supplied.Catalog.Selection.Equal(source.Groups) {
			return resolution{}, invalid(source.Name, "loaded group selection differs from configuration; reload the source")
		}
		for _, id := range supplied.Catalog.Rules {
			if !slices.Contains(source.Rules, id) {
				return resolution{}, invalid(source.Name, "loaded rule selection differs from configuration; reload the source")
			}
		}
		snapshot := supplied.Snapshot
		ref, err := rules.ParseGitRef(snapshot.Commit, source.Name+".resolvedCommit")
		if err != nil || ref.Kind() != rules.GitRefCommit {
			return resolution{}, invalid(source.Name, "resolvedCommit must be a full commit SHA")
		}
		if source.Ref.Kind() == rules.GitRefCommit && !source.Ref.Equal(ref) {
			return resolution{}, invalid(source.Name, "resolved commit differs from configured commit")
		}
		candidates := map[string]trailguide.Rule{}
		ids := []string{}
		seenGroups := map[string]bool{}
		for _, group := range supplied.Catalog.Groups {
			if seenGroups[group.ID] {
				return resolution{}, invalid(group.ID, "duplicate group")
			}
			seenGroups[group.ID] = true
			if err := librarytree.ValidateGroupID(group.ID, source.Name); err != nil {
				return resolution{}, err
			}
			if source.Groups.Includes(group.ID) {
				ids = append(ids, group.ID)
			}
			target := ensureGroup(groups, group.ID)
			target.Guidance = append(target.Guidance, groupGuidance{source.Name, group.Metadata})
			for _, candidate := range group.Rules {
				parsed, err := trailguide.ParseRule(candidate.Document, candidate.Path, source.Name)
				if err != nil {
					return resolution{}, err
				}
				if parsed.Group != group.ID {
					return resolution{}, invalid(parsed.ID, "rule belongs to a different group")
				}
				key := strings.TrimSuffix(parsed.Path, ".md")
				if _, exists := candidates[key]; exists {
					return resolution{}, invalid(parsed.ID, "duplicate rule")
				}
				candidates[key] = parsed
			}
		}
		slices.Sort(ids)
		for _, id := range source.Groups.Groups {
			if !slices.Contains(ids, id) {
				return resolution{}, invalid(source.Name, "loaded groups differ from configured selection")
			}
		}
		retained := maps.Clone(supplied.Catalog.SupportingFiles)
		if retained == nil {
			retained = map[string][]byte{}
		}
		for _, id := range slices.Sorted(maps.Keys(candidates)) {
			parsed := candidates[id]
			exclusion, excluded := source.Exclusion(id)
			if excluded && exclusion.ReplacedBy == "" {
				retained[parsed.Path] = []byte(parsed.Document)
				continue
			}
			version, recorded := snapshot.Rules[id]
			if !recorded || !validCommit(version.Commit) {
				return resolution{}, invalid(parsed.ID, "the source snapshot records no version or commit for this rule; run trailguide project sync")
			}
			origin := ruleOrigin{Source: source.Name, File: parsed.Path, Repository: source.Repository, Ref: source.Ref.String(), Commit: version.Commit, Version: version.Version, Release: version.Release, legacyLayout: version.LegacyLayout}
			active := resolvedRule{Rule: parsed, Origin: origin, License: supplied.Catalog.License}
			if excluded {
				file := exclusion.ReplacedBy + ".md"
				replacementRule, ok := localRules[file]
				if !ok {
					return resolution{}, invalid(exclusion.ReplacementFile(), "missing local replacement rule")
				}
				if used[file] {
					return resolution{}, invalid(exclusion.ReplacementFile(), "replacement file is reused for multiple targets")
				}
				if replacementRule.Group != parsed.Group {
					return resolution{}, invalid(exclusion.ReplacementFile(), "replacement must stay within the target group")
				}
				retained[parsed.Path] = []byte(parsed.Document)
				used[file] = true
				active = resolvedRule{Rule: replacementRule, Origin: ruleOrigin{Source: "local", File: file}, Upstream: &origin, Reason: exclusion.Reason, BasedOn: exclusion.BasedOn, License: nil}
			}
			group := ensureGroup(groups, parsed.Group)
			group.Rules = append(group.Rules, active)
		}
		result.Sources = append(result.Sources, resolvedSource{Name: source.Name, Repository: source.Repository, Ref: source.Ref.String(), Pins: source.Pins, Release: snapshot.Release, Commit: ref.Canonical(), Selection: source.Groups, Rules: source.Rules, Groups: ids, Versions: snapshot.Rules, Configured: source, Retired: snapshot.RetiredRules, License: supplied.Catalog.License, Paths: supplied.Catalog.Paths(), Files: retained, NonLibrary: snapshot.NonLibrary, SkillSelection: source.Skills, Skills: slices.Sorted(maps.Keys(snapshot.Skills)), Curated: snapshot.Curated})
	}
	for _, file := range slices.Sorted(maps.Keys(localRules)) {
		if used[file] {
			continue
		}
		parsed := localRules[file]
		group, ok := groups[parsed.Group]
		if !ok || len(group.Guidance) == 0 {
			return resolution{}, invalid(authoredLocation(file), "the rule's group has no metadata; add rules/"+parsed.Group+"/_group.yaml or include the group from a source")
		}
		group.Rules = append(group.Rules, resolvedRule{Rule: parsed, Origin: ruleOrigin{Source: "local", File: file}, License: nil})
	}
	for _, id := range slices.Sorted(maps.Keys(groups)) {
		group := groups[id]
		slices.SortFunc(group.Rules, func(a, b resolvedRule) int { return compareRuleIDs(a.Rule.ID, b.Rule.ID) })
		slices.SortFunc(group.Guidance, func(a, b groupGuidance) int { return strings.Compare(a.Source, b.Source) })
		group.EffectiveGuidance = resolveGuidance(group.Guidance)
		result.Groups = append(result.Groups, *group)
	}
	return result, nil
}

// validCommit accepts a full, lowercase commit SHA.
func validCommit(text string) bool {
	ref, err := rules.ParseGitRef(text, "commit")
	return err == nil && ref.Kind() == rules.GitRefCommit && ref.Canonical() == text
}

// ensureGroup returns a group accumulator with explicit empty collections.
func ensureGroup(groups map[string]*resolvedGroup, id string) *resolvedGroup {
	if group, ok := groups[id]; ok {
		return group
	}
	group := &resolvedGroup{ID: id, Guidance: []groupGuidance{}, Rules: []resolvedRule{}}
	groups[id] = group
	return group
}

// parseLocal validates every local definition before any replacement or exclusion can hide errors.
func parseLocal(files map[string][]byte, groups map[string]*resolvedGroup) (map[string]trailguide.Rule, error) {
	result := map[string]trailguide.Rule{}
	for _, file := range slices.Sorted(maps.Keys(files)) {
		if !fs.ValidPath(file) || file == "." || strings.ContainsAny(file, "\\:") || strings.ContainsFunc(file, func(r rune) bool { return r < 32 || r == 127 }) {
			return nil, invalid(authoredLocation(file), "expected a contained portable path")
		}
		if file == "README.md" || librarytree.IsGroupReadme(file) {
			continue
		}
		if slices.Contains(strings.Split(file, "/"), "assets") && !(strings.Count(file, "/") == 2 && path.Base(file) == "_group.yaml" && (strings.HasPrefix(file, "techs/") || strings.HasPrefix(file, "practices/"))) {
			continue
		}
		if path.Base(file) == "_group.yaml" {
			id := path.Dir(file)
			if err := librarytree.ValidateGroupID(id, authoredLocation(file)); err != nil {
				return nil, err
			}
			metadata, err := trailguide.ParseGroupMetadata(json.RawMessage(files[file]), authoredLocation(file))
			if err != nil {
				return nil, err
			}
			ensureGroup(groups, id).Guidance = append(ensureGroup(groups, id).Guidance, groupGuidance{Source: "local", Metadata: metadata})
			continue
		}
		if !strings.HasSuffix(file, ".md") || strings.HasPrefix(path.Base(file), "_") {
			continue
		}
		if rules.HasDraftMarker(files[file]) {
			return nil, invalid(authoredLocation(file), "complete the draft and remove its trailguide:draft marker")
		}
		parsed, err := trailguide.ParseRule(string(files[file]), file, "local")
		if err != nil {
			return nil, err
		}
		result[file] = parsed
	}
	return result, nil
}

// validateLocalLinks checks local rules and Markdown attachments against the shared destination policy, resolving
// each link from where its file sits in the Trail Guide directory. It checks retained attachments even if no active
// rule links to them; target existence belongs to rendering.
func validateLocalLinks(files map[string][]byte, localRules map[string]trailguide.Rule) error {
	for _, file := range slices.Sorted(maps.Keys(files)) {
		_, rule := localRules[file]
		if !rule && (rules.AssetDirectory(file) == "" || !strings.HasSuffix(file, ".md")) {
			continue
		}
		location := authoredLocation(file)
		targets, err := rules.MarkdownTargets(string(files[file]), location)
		if err != nil {
			return fmt.Errorf("%s: %w", location, err)
		}
		for _, target := range targets {
			key, err := authoredKey(target, location)
			if err != nil {
				return err
			}
			if err := rules.RequireAllowedTarget(file, key, nil); err != nil {
				return fmt.Errorf("%s: %w", location, err)
			}
		}
	}
	return nil
}

// invalid identifies input relationships that prevent an effective rule set from being produced.
func invalid(location, problem string) error {
	return &ValidationError{Location: location, Problem: problem}
}

// resolveGuidance chooses the complete local definition when present, otherwise retains all imported definitions.
func resolveGuidance(definitions []groupGuidance) []groupGuidance {
	for _, guidance := range definitions {
		if guidance.Source == "local" {
			return []groupGuidance{guidance}
		}
	}
	return slices.Clone(definitions)
}

// compareRuleIDs compares digit runs numerically and other bytes lexically, without integer overflow.
// Numerically equivalent spellings use the full ID as a deterministic tie-breaker.
func compareRuleIDs(a, b string) int {
	left, right := a, b
	for len(left) > 0 && len(right) > 0 {
		if left[0] >= '0' && left[0] <= '9' && right[0] >= '0' && right[0] <= '9' {
			i, j := digitRunEnd(left), digitRunEnd(right)
			x, y := strings.TrimLeft(left[:i], "0"), strings.TrimLeft(right[:j], "0")
			if len(x) < len(y) {
				return -1
			}
			if len(x) > len(y) {
				return 1
			}
			if order := strings.Compare(x, y); order != 0 {
				return order
			}
			left, right = left[i:], right[j:]
			continue
		}
		if left[0] < right[0] {
			return -1
		}
		if left[0] > right[0] {
			return 1
		}
		left, right = left[1:], right[1:]
	}
	if order := strings.Compare(left, right); order != 0 {
		return order
	}
	return strings.Compare(a, b)
}

// digitRunEnd finds the end of a leading ASCII decimal run in a rule ID.
func digitRunEnd(text string) int {
	i := 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	return i
}
