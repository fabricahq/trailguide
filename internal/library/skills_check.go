// Check the frontmatter of the skills a library authors: each folder's name, and the metadata keys Trail Guide defines.

package library

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// skillsDirectory is where a library's skills live in its repository.
const skillsDirectory = ".trailguide/" + skills.Directory

// skillProblems returns every problem in the skills tree holds, by path in the library's skills/ folder, in path
// order, each starting with the file or folder it's about: a file directly in skills/, a folder whose name breaks the
// skill name grammar or that has no SKILL.md, frontmatter skills.ReadMetadata or skills.Check rejects, a SKILL.md that still has the draft marker, and a cited
// rule that isn't one of current, the library's rules, naming the replacement when the library's release history or
// a pending change note retired it, and a skill a skill uses that isn't a folder of skills/ with a SKILL.md or, when
// the line names a source, one of curated, the curated skills the library's levels list. A nil tree has no skills.
func skillProblems(tree *filetxn.Tree, current, curated []string, changes libraryChanges) []string {
	problems := []string{}
	if tree == nil {
		return problems
	}
	folders := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(tree.Files)) {
		folder, file, inFolder := strings.Cut(name, "/")
		if !inFolder {
			problems = append(problems, skills.Directory+"/"+name+": skills/ holds only skill folders")
			continue
		}
		folders[folder] = folders[folder] || file == skills.File
	}
	for _, directory := range tree.Directories {
		if _, listed := folders[directory]; !listed && !strings.Contains(directory, "/") {
			folders[directory] = false
		}
	}
	_, retiring := changes.namedRules()
	for _, folder := range slices.Sorted(maps.Keys(folders)) {
		location := skills.AuthoredID(folder)
		switch {
		case !skills.ValidName(folder):
			problems = append(problems, location+": a skill name uses 1 to 64 lowercase letters, digits, and hyphens, without a leading, trailing, or doubled hyphen")
			continue
		case !folders[folder]:
			problems = append(problems, location+": expected SKILL.md in the skill folder")
			continue
		}
		location += "/" + skills.File
		metadata, err := skills.ReadMetadata(tree.Files[folder+"/"+skills.File], location)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if rules.HasDraftMarker(tree.Files[folder+"/"+skills.File]) {
			problems = append(problems, location+": complete the draft and remove its trailguide:draft marker")
		}
		for _, problem := range skills.Check(folder, metadata, false) {
			problems = append(problems, location+": "+problem)
		}
		for _, id := range metadata.RulesUsed {
			if problem := citedRuleProblem(id, current, changes.history, retiring); problem != "" {
				problems = append(problems, location+": "+problem)
			}
		}
		missing := []string{}
		for _, id := range metadata.SkillsUsed {
			// A skill in the library is a folder with a SKILL.md, and a curated one is one its levels list, which
			// requireCuratedSkills checks its source has; skills.Check reports a use that isn't a skill ID.
			if !skills.ValidSkillCitation(id) || slices.Contains(missing, id) {
				continue
			}
			if citation := skills.ParseCitation(id); citation.Source != "" && !slices.Contains(curated, id) {
				missing = append(missing, id)
				problems = append(problems, fmt.Sprintf("%s: %s names %s, %s", location, skills.SkillsUsedKey, id, uncuratedProblem(citation, curated)))
			} else if name, _ := strings.CutPrefix(id, skills.Directory+"/"); citation.Source == "" && !folders[name] {
				missing = append(missing, id)
				problem := fmt.Sprintf("%s: %s names %s, which isn't a skill in this library", location, skills.SkillsUsedKey, id)
				for _, listed := range curated {
					if _, skill, _ := SplitCurated(listed); skill == id {
						problem += fmt.Sprintf("; the library curates %s, so name it that way", listed)
						break
					}
				}
				problems = append(problems, problem)
			}
		}
	}
	return problems
}

// interfaceFileWarnings returns a warning, in path order, for each skill in tree, the library's skills/ folder, that
// ships its own agents/openai.yaml holding only the interface text Trail Guide can generate from the skill's metadata,
// suggesting the metadata keys instead. A file with other settings, such as icons or a policy, is a supported
// escape hatch and gets no warning. A nil tree has none.
func interfaceFileWarnings(tree *filetxn.Tree) []string {
	warnings := []string{}
	if tree == nil {
		return warnings
	}
	for _, name := range slices.Sorted(maps.Keys(tree.Files)) {
		if folder, file, _ := strings.Cut(name, "/"); file == skills.OpenAIFile && skills.InterfaceTextOnly(tree.Files[name]) {
			warnings = append(warnings, fmt.Sprintf("%s/%s: it sets only interface text, which Trail Guide generates for each project from SKILL.md. Set metadata.%s, %s, and %s in SKILL.md instead, then delete this file.", skills.AuthoredID(folder), skills.OpenAIFile, skills.DisplayNameKey, skills.ShortDescriptionKey, skills.DefaultPromptKey))
		}
	}
	return warnings
}

// uncuratedProblem explains why citation, a line of trailguide-skills-used that names a source, isn't one of curated,
// the curated skills the library's levels list: no listed skill comes from its source, which is then misspelled or
// curates nothing, or the library hasn't listed the skill.
func uncuratedProblem(citation skills.Citation, curated []string) string {
	sources := []string{}
	for _, id := range curated {
		if source, _, _ := SplitCurated(id); !slices.Contains(sources, source) {
			sources = append(sources, source)
		}
	}
	switch {
	case slices.Contains(sources, citation.Source):
		return "which isn't a skill the library curates; list it in library.skills, at any level, so projects that install the skill can install it too"
	case len(sources) == 0:
		return "but library.skills lists no curated skills; name a skill of this library as skills/<name>, or list the curated skill in library.skills"
	}
	return fmt.Sprintf("but library.skills lists no skill from a source named %s; the library curates skills from %s. Correct the source name, or name a skill of this library as skills/<name>", citation.Source, strings.Join(sources, ", "))
}

// citedRuleProblem returns why id, a line of trailguide-rules-used, doesn't name a current rule of the library, or empty when
// it does or isn't a rule ID, which skills.Check reports instead. retiring holds the pending change notes'
// retirements by rule.
func citedRuleProblem(id string, current []string, history releaseHistory, retiring map[string][]rules.NoteChange) string {
	if slices.Contains(current, id) || !skills.ValidRuleCitation(id, false) {
		return ""
	}
	replacement, retired := "", false
	if _, ok := history.retired[id]; ok {
		replacement, retired = history.replacedBy[id], true
	}
	for _, change := range retiring[id] {
		replacement, retired = change.ReplacedBy, true
	}
	switch {
	case retired && replacement != "":
		return fmt.Sprintf("%s names %s, which is retired and replaced by %s; cite the replacement", skills.RulesUsedKey, id, replacement)
	case retired:
		return fmt.Sprintf("%s names %s, which is retired; remove it", skills.RulesUsedKey, id)
	}
	return fmt.Sprintf("%s names %s, which isn't a rule in this library", skills.RulesUsedKey, id)
}
