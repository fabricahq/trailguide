// Resolve the skills a project installs, and render their installed copies, SKILLS.md, and installed-skills.json.

package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// SkillFiles is one skill folder's files, by path in the folder, and the paths of its executable files, sorted.
type SkillFiles struct {
	Files      map[string][]byte `json:"files"`
	Executable []string          `json:"executable"`
}

// ClashError reports two skills the project installs with the same install name, which names their installed
// folders, so both can't be installed.
type ClashError struct {
	Name string
	// IDs holds the two skills' project IDs, sorted.
	IDs [2]string
}

// Error names the skills and the next steps.
func (e *ClashError) Error() string {
	return fmt.Sprintf("%s and %s both install as %s. Rename one with skills.rename, or exclude one.", e.IDs[0], e.IDs[1], e.Name)
}

// resolvedSkill is one skill the project installs.
type resolvedSkill struct {
	// ID is the project ID: source:folder for a source's skill, or skills/<name> for an authored one.
	ID string
	// Source is the source's name, or empty for an authored skill; Folder is the skill's ID in its source or in the
	// repository's skills/ folder.
	Source, Folder string
	// Name is the install name; Description and Type are as SKILL.md declares them.
	Name, Description, Type string
	// RenamedFrom is the name SKILL.md declares when the project's skills.rename installs the skill under Name
	// instead, or empty.
	RenamedFrom string
	Files       SkillFiles
	// Repository, Ref, Commit, and Release identify the revision that supplied a source's skill; Release is 0 for a
	// revision other than a library release, and NonLibrary marks a source whose repository isn't a library.
	Repository, Ref, Commit string
	Release                 int
	NonLibrary              bool
	// Level is the skill's level in its library, or empty for an authored skill or one from another repository.
	// LevelRelease is the library release whose manifest set it, or 0 when the library source's revision isn't one;
	// it differs from Release for a curated skill.
	Level        string
	LevelRelease int
	// Curated names the source in the library's manifest that a curated skill comes from, whose repository and
	// revision Repository, Ref, Commit, and Release then identify, and whose files are in Vendored, the skill's
	// folder in the library source's vendor snapshot; both are empty for any other skill. Override names the
	// project's own source that supplied the curated revision, when one declares the same repository.
	Curated, Vendored, Override string
	// Targets lists, sorted, the folders the skill installs into, by path from the repository root.
	Targets []string
	// RulesUsed lists the lines of the skill's trailguide-rules-used, and Cited how the project resolves each of them, in
	// the same order; both are empty for a skill that cites no rules.
	RulesUsed []string
	Cited     []citedRule
	// SkillsUsed lists the lines of the skill's trailguide-skills-used, as written; nil when it has none.
	SkillsUsed []string
	// Included reports that the configuration includes the skill, as it includes every authored skill; a source's
	// skill that isn't included is installed because other skills use it.
	Included bool
	// Uses and UsedBy list, sorted, the project IDs of the installed skills this skill uses, by its trailguide-skills-used
	// and its source's skills.uses, and of those that use it; each is empty, never nil, when there are none.
	Uses, UsedBy []string
	// Replaces is the source's skill an authored skill is installed in place of, or nil when it replaces none.
	Replaces *skillReplacement
}

// resolveSkills returns, sorted by project ID, the skills of every configured source's snapshot that the source
// doesn't exclude or fork, and the authored skills in local, keyed by ID, skills/<name>, each with the skill it replaces and
// the skills it uses and those that use it. It fails when a SKILL.md is missing or its frontmatter unreadable, when
// an install name isn't valid, when an exclusion or fork is invalid, as skillReplacements checks it, when a rename names a
// skill the source doesn't install, with *ClashError when two skills share an install name, after renames, and as
// linkSkillUses does for an authored skill's uses. It also returns linkSkillUses' warnings about uses of skills the
// project excludes without a replacement.
func resolveSkills(config rules.Configuration, libraries map[string]Library, local map[string]SkillFiles) ([]resolvedSkill, []string, error) {
	replacements, err := skillReplacements(config, libraries, local)
	if err != nil {
		return nil, nil, err
	}
	result := []resolvedSkill{}
	for _, source := range config.Sources {
		snapshot := libraries[source.Name].Snapshot
		for _, id := range slices.Sorted(maps.Keys(snapshot.Skills)) {
			if _, excluded := source.SkillExclude[id]; excluded {
				continue
			}
			if _, forked := source.SkillForks[id]; forked {
				continue
			}
			skill := sourceSkill(source, snapshot, id)
			skill.Files = SkillFiles{snapshot.SkillFiles(id), snapshot.SkillExecutable(id)}
			if err := describeSkill(&skill, source.SkillRename[id], "vendor/"+source.Name+"/"+skill.Vendored+"/"+skills.File); err != nil {
				return nil, nil, err
			}
			result = append(result, skill)
		}
		for _, id := range slices.Sorted(maps.Keys(source.SkillRename)) {
			if _, installs := snapshot.Skills[id]; !installs {
				return nil, nil, invalid("sources."+source.Name+".skills.rename."+id, fmt.Sprintf("the source doesn't install skill %s, so there's nothing to rename; name a skill its skills.include selects, or remove the entry", id))
			}
		}
	}
	result, aliases := withoutCuratedCopies(result)
	for _, id := range slices.Sorted(maps.Keys(local)) {
		skill := resolvedSkill{ID: id, Folder: id, Files: local[id], Included: true}
		if replacement, replaces := replacements[id]; replaces {
			skill.Replaces = &replacement
		}
		if err := describeSkill(&skill, "", id+"/"+skills.File); err != nil {
			return nil, nil, err
		}
		result = append(result, skill)
	}
	slices.SortFunc(result, func(a, b resolvedSkill) int { return strings.Compare(a.ID, b.ID) })
	names := map[string]string{}
	for i, skill := range result {
		result[i].Targets = []string{}
		for _, folder := range config.SkillFolders() {
			result[i].Targets = append(result[i].Targets, folder+"/"+skill.Name)
		}
		if other, taken := names[skill.Name]; taken {
			return nil, nil, &ClashError{Name: skill.Name, IDs: [2]string{other, skill.ID}}
		}
		names[skill.Name] = skill.ID
	}
	warnings, err := linkSkillUses(result, config, aliases)
	if err != nil {
		return nil, nil, err
	}
	return result, warnings, nil
}

// sourceSkill returns the origin of skill id of source, whose snapshot is snapshot: the source's repository and
// revision for a skill it publishes, and, for a curated one, the repository and revision it was curated from.
func sourceSkill(source rules.Source, snapshot library.Snapshot, id string) resolvedSkill {
	skill := resolvedSkill{ID: source.Name + ":" + id, Source: source.Name, Folder: id, Vendored: id, Repository: source.Repository, Ref: source.Ref.String(), Commit: snapshot.Commit, Release: snapshot.Release, NonLibrary: snapshot.NonLibrary, Level: snapshot.Skills[id].Level, Included: snapshot.Skills[id].Included}
	if skill.Level != "" {
		skill.LevelRelease = snapshot.Release
	}
	if name, folder, curated := library.SplitCurated(id); curated {
		origin := snapshot.Curated[name]
		skill.Folder, skill.Vendored, skill.Curated, skill.Override = folder, library.SkillFolder(id), name, origin.Source
		skill.Repository, skill.Ref, skill.Commit, skill.Release, skill.NonLibrary = origin.Repository, origin.Ref.String(), origin.Commit, origin.Release, origin.NonLibrary
	}
	return skill
}

// withoutCuratedCopies drops from resolved each skill a project source installs that is also a curated skill
// resolved, from the same repository, folder, and commit, as an overriding source supplies it: a skill's identity is
// its repository and folder, so the curated one, which carries its library's level, is the one installed. It also
// returns, by the project ID of each dropped skill, the curated skill's, which uses of the dropped one reach instead.
func withoutCuratedCopies(resolved []resolvedSkill) ([]resolvedSkill, map[string]string) {
	aliases := map[string]string{}
	kept := slices.DeleteFunc(resolved, func(skill resolvedSkill) bool {
		if skill.Curated != "" {
			return false
		}
		i := slices.IndexFunc(resolved, func(curated resolvedSkill) bool {
			return curated.Override == skill.Source && curated.Curated != "" && curated.Folder == skill.Folder && curated.Commit == skill.Commit
		})
		if i >= 0 {
			aliases[skill.ID] = resolved[i].ID
		}
		return i >= 0
	})
	return kept, aliases
}

// describeSkill reads skill's SKILL.md, at location for diagnostics, for its install name, description, type, and
// cited rules. A non-empty rename, the name the source's skills.rename gives the skill, is its install name, and the
// name it replaces is recorded as RenamedFrom unless they're the same; the replaced name needn't follow the name
// grammar, so a rename can install a skill whose own name agents couldn't find it by. An authored skill must also be complete, without the draft marker, and its frontmatter must pass skills.Check, which reports its first problem; a
// source's skills were checked by their library, or, from a repository that isn't a library, install as they are.
func describeSkill(skill *resolvedSkill, rename, location string) error {
	data, ok := skill.Files.Files[skills.File]
	if !ok {
		return invalid(location, "missing the skill's SKILL.md")
	}
	metadata, err := skills.ReadMetadata(data, location)
	if err != nil {
		return err
	}
	if skill.Source == "" {
		if rules.HasDraftMarker(data) {
			return invalid(location, "complete the draft and remove its trailguide:draft marker")
		}
		if problems := skills.Check(path.Base(skill.Folder), metadata, true); len(problems) > 0 {
			return invalid(location, problems[0])
		}
	}
	original := metadata.Name
	if original == "" {
		original = path.Base(skill.Folder)
	}
	name, err := skills.InstallName(skill.Folder, metadata, location)
	switch {
	case rename != "" && rename != original:
		skill.Name, skill.RenamedFrom = rename, original
	case err != nil:
		return err
	default:
		skill.Name = name
	}
	skill.Description, skill.Type, skill.RulesUsed, skill.SkillsUsed = metadata.Description, metadata.Type, metadata.RulesUsed, metadata.SkillsUsed
	return nil
}

// marker returns the line an installed copy of skill's SKILL.md carries: the project ID and the commit of a source's
// skill, and, for an authored skill, where to edit it.
func (skill resolvedSkill) marker() string {
	if skill.Source == "" {
		return fmt.Sprintf("<!-- trailguide:installed %s. Do not edit; edit .trailguide/%s/ instead. -->", skill.ID, skill.Folder)
	}
	return fmt.Sprintf("<!-- trailguide:installed %s %s. Do not edit; trailguide project sync replaces this copy. -->", skill.ID, skill.Commit)
}

// installed returns the files of skill's installed copy: its files with the marker line in SKILL.md, whose name is
// the install name when the project renames the skill, and, when the skill cites rules, the section that links to the
// project's version of each one at its end. Every skill target folder is two levels below the repository root, so
// one copy's links work in every target.
func (skill resolvedSkill) installed() SkillFiles {
	files := maps.Clone(skill.Files.Files)
	if skill.RenamedFrom != "" {
		files[skills.File] = skills.Rename(files[skills.File], skill.Name)
	}
	from := skills.File
	if len(skill.Targets) > 0 {
		from = skill.Targets[0] + "/" + skills.File
	}
	files[skills.File] = appendSection(skills.Mark(files[skills.File], skill.marker()), citedSection(skill, from))
	return SkillFiles{Files: files, Executable: slices.Clone(skill.Files.Executable)}
}

// renderSkills returns the installed skill folders by path from the repository root, and, when the project installs
// any skill or excludes one of its sources' skills, SKILLS.md and installed-skills.json by path in generated/.
// excluded lists the skills the project excludes without a replacement.
func renderSkills(resolved []resolvedSkill, excluded []excludedSkill) (map[string]SkillFiles, map[string][]byte, error) {
	folders := map[string]SkillFiles{}
	files := map[string][]byte{}
	if len(resolved) == 0 && len(excluded) == 0 {
		return folders, files, nil
	}
	type record struct {
		Name    string   `json:"name"`
		ID      string   `json:"id"`
		Version *string  `json:"version"`
		Commit  *string  `json:"commit"`
		Targets []string `json:"targets"`
		Digest  string   `json:"digest"`
	}
	records := []record{}
	for _, skill := range resolved {
		installed := skill.installed()
		for _, target := range skill.Targets {
			folders[target] = installed
		}
		entry := record{Name: skill.Name, ID: skill.ID, Targets: skill.Targets, Digest: folderDigest(installed)}
		if skill.NonLibrary || skill.Curated != "" {
			entry.Commit = &skill.Commit
		}
		records = append(records, entry)
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(struct {
		FormatVersion   int      `json:"formatVersion"`
		GeneratedNotice string   `json:"generatedNotice"`
		Skills          []record `json:"skills"`
	}{1, generatedNotice + " " + strings.ReplaceAll(regenerationNotice, "`", ""), records}); err != nil {
		return nil, nil, fmt.Errorf("encode installed skills: %w", err)
	}
	files[InstalledSkillsFile] = encoded.Bytes()
	files["SKILLS.md"] = []byte(generatedMarkdown(skillsIndex(resolved, excluded)))
	return folders, files, nil
}

// InstalledSkillsFile is the record, in generated/, of the skill folders Trail Guide installed, which is how it knows
// which folders in the skill targets it owns.
const InstalledSkillsFile = "installed-skills.json"

// InstalledFolders reads the folders an installed-skills.json record lists, by path from the repository root,
// sorted. A record it can't read lists none, so no folder counts as installed by Trail Guide.
func InstalledFolders(data []byte) []string {
	return slices.Sorted(maps.Keys(InstalledSkillIDs(data)))
}

// InstalledSkillIDs reads the folders an installed-skills.json record lists, by path from the repository root, each
// with the project ID of the skill installed there. It keeps only folders in a skill target folder whose name
// follows the skill name grammar. A record it can't read lists none, so no folder counts as installed by Trail Guide.
func InstalledSkillIDs(data []byte) map[string]string {
	folders := map[string]string{}
	for _, skill := range InstalledSkills(data) {
		for _, target := range skill.Targets {
			if _, listed := folders[target]; !listed {
				folders[target] = skill.ID
			}
		}
	}
	return folders
}

// InstalledSkill is one skill an installed-skills.json record lists.
type InstalledSkill struct {
	// Name is the install name, and ID the project ID: source:folder, or skills/<name> for an authored skill.
	Name, ID string
	// Targets lists the folders the skill is installed in, by path from the repository root; it is empty when the
	// project has no skill targets.
	Targets []string
}

// InstalledSkills reads the skills an installed-skills.json record lists, in its order. It keeps only targets that
// are a skill target's folder holding a validly named skill folder. A record it can't read lists none.
func InstalledSkills(data []byte) []InstalledSkill {
	var record struct {
		FormatVersion int `json:"formatVersion"`
		Skills        []struct {
			Name    string   `json:"name"`
			ID      string   `json:"id"`
			Targets []string `json:"targets"`
		} `json:"skills"`
	}
	if json.Unmarshal(data, &record) != nil || record.FormatVersion != 1 {
		return []InstalledSkill{}
	}
	result := []InstalledSkill{}
	for _, skill := range record.Skills {
		installed := InstalledSkill{Name: skill.Name, ID: skill.ID, Targets: []string{}}
		for _, target := range skill.Targets {
			if parent := path.Dir(target); slices.Contains(slices.Collect(maps.Values(rules.SkillTargetFolders)), parent) && skills.ValidName(path.Base(target)) {
				installed.Targets = append(installed.Targets, target)
			}
		}
		result = append(result, installed)
	}
	return result
}

// folderDigest returns the SHA-256 of an installed folder: each file's path, executable bit, and contents.
func folderDigest(folder SkillFiles) string {
	entries := [][3]string{}
	for _, name := range slices.Sorted(maps.Keys(folder.Files)) {
		mode := "100644"
		if slices.Contains(folder.Executable, name) {
			mode = "100755"
		}
		sum := sha256.Sum256(folder.Files[name])
		entries = append(entries, [3]string{name, mode, hex.EncodeToString(sum[:])})
	}
	encoded, _ := json.Marshal(entries)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// skillsIndex renders SKILLS.md: one entry per installed skill, saying what it does, where it comes from, what it
// replaces, and where agents find it, then the skills the project excludes, each with its reason.
func skillsIndex(resolved []resolvedSkill, excluded []excludedSkill) string {
	sections := []string{"# Skills", "This project installs the agent skills below. Agents that load skills from a skill target folder find each one there. Any other agent can read a skill's `SKILL.md` at the path given and follow it when its description matches the task. These files are generated; edit the configuration or the authored skills in `.trailguide/skills/` and rebuild."}
	if slices.ContainsFunc(resolved, func(skill resolvedSkill) bool { return skill.Level != "" }) {
		sections = append(sections, "A skill's level says how strongly the library that supplies it asks projects to use it: every project that uses the library installs its required skills, and the team chose to install its recommended and offered ones.")
	}
	for _, skill := range resolved {
		lines := []string{"### " + skill.Name}
		if description := strings.TrimSpace(skill.Description); description != "" {
			lines = append(lines, escapeText(description))
		}
		facts := []string{}
		switch {
		case skill.Source == "":
			facts = append(facts, "- **Skill:** `"+skill.ID+"`, authored in this project")
		case skill.Curated != "":
			facts = append(facts, fmt.Sprintf("- **Skill:** `%s`, curated by `%s` from %s, commit `%s`", skill.ID, skill.Source, escapeText(skill.Repository), skill.Commit))
		case skill.Release != 0:
			facts = append(facts, fmt.Sprintf("- **Skill:** `%s`, from release/%d, commit `%s`", skill.ID, skill.Release, skill.Commit))
		default:
			facts = append(facts, fmt.Sprintf("- **Skill:** `%s`, commit `%s`", skill.ID, skill.Commit))
		}
		if skill.RenamedFrom != "" {
			facts = append(facts, "- **Renamed from:** `"+skill.RenamedFrom+"`, by the project's configuration. Its own text and other skills from `"+skill.Source+"` may still call it `"+skill.RenamedFrom+"`; they mean this skill.")
		}
		facts = append(facts, replacementFacts(skill.Replaces)...)
		if skill.Level != "" {
			setBy := "`" + skill.Source + "`"
			if skill.LevelRelease != 0 {
				setBy += fmt.Sprintf(", release %d", skill.LevelRelease)
			}
			facts = append(facts, "- **Level:** "+skill.Level+" by "+setBy)
		}
		if skill.Type != "" {
			facts = append(facts, "- **Type:** "+escapeText(skill.Type))
		}
		if len(skill.Cited) > 0 && len(skill.Targets) > 0 {
			facts = append(facts, fmt.Sprintf("- **Applies:** %d rule(s), listed at the end of its installed SKILL.md", len(skill.Cited)))
		} else if len(skill.Cited) > 0 {
			lines := []string{}
			for _, cited := range skill.Cited {
				lines = append(lines, "  - "+citedLine(cited, ".trailguide/generated/SKILLS.md"))
			}
			facts = append(facts, "- **Applies:**\n"+strings.Join(lines, "\n"))
		}
		facts = append(facts, useFacts(skill, resolved)...)
		if len(skill.Targets) == 0 {
			source := ".trailguide/" + skill.Folder + "/" + skills.File
			if skill.Source != "" {
				source = ".trailguide/vendor/" + skill.Source + "/" + skill.Vendored + "/" + skills.File
			}
			facts = append(facts, "- **Installed at:** nowhere, because the project has no skill targets. Read it at ["+source+"]("+rules.RelativeLink(".trailguide/generated/SKILLS.md", source)+").")
		} else {
			links := []string{}
			for _, target := range skill.Targets {
				file := target + "/" + skills.File
				links = append(links, "["+file+"]("+rules.RelativeLink(".trailguide/generated/SKILLS.md", file)+")")
			}
			facts = append(facts, "- **Installed at:** "+strings.Join(links, ", "))
		}
		lines = append(lines, strings.Join(facts, "\n"))
		sections = append(sections, strings.Join(lines, "\n\n"))
	}
	if len(excluded) > 0 {
		items := []string{}
		for _, skill := range excluded {
			items = append(items, "- `"+skill.ID+"`: "+escapeText(skill.Reason))
		}
		sections = append(sections, "## Excluded skills", "The project's configuration excludes these skills, so they aren't installed. Their sources still supply them, so updates can show their changes.", strings.Join(items, "\n"))
	}
	return strings.Join(sections, "\n\n") + "\n"
}

// replacementFacts returns the SKILLS.md lines that say which skill an authored skill replaces, the commit it was
// based on, and why, or none when it replaces none.
func replacementFacts(replacement *skillReplacement) []string {
	if replacement == nil {
		return nil
	}
	replaces := "- **Replaces:** `" + replacement.Upstream.ID + "`, which the project forks"
	if !replacement.BasedOn.IsZero() {
		replaces += ", based on its copy at commit `" + replacement.BasedOn.Canonical() + "`"
	}
	return []string{replaces, "- **Reason:** " + escapeText(replacement.Reason)}
}
