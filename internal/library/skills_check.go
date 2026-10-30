// Check the frontmatter of the skills a library authors: each folder's name, and the metadata keys Trail Guide defines.

package library

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// skillsDirectory is where a library's skills live in its repository.
const skillsDirectory = ".trailguide/" + skills.Directory

// librarySkills returns the skills of tree, the library's skills/ folder, by ID, as skills.Read finds them in the
// origin folders manifest, the library's configuration, gives them. It fails with invalid-skill, listing every
// problem with the folder's layout, each starting with the path it's about, and when manifest doesn't parse. A nil
// tree has none.
func librarySkills(tree *filetxn.Tree, manifest []byte) (map[string]skills.Skill[[]byte], error) {
	if tree == nil {
		return map[string]skills.Skill[[]byte]{}, nil
	}
	config, err := rules.ParseConfigurationYAML(manifest)
	if err != nil {
		return nil, err
	}
	found, layoutProblems := skills.Read(tree.Files, tree.Executable, tree.Directories, config.SkillOrigins())
	if len(layoutProblems) > 0 {
		problems := []string{}
		for _, problem := range layoutProblems {
			problems = append(problems, problem.Location+": "+problem.Problem)
		}
		return nil, failure("invalid-skill", "skills don't pass library check:\n  - "+strings.Join(problems, "\n  - "), nil)
	}
	return found, nil
}

// skillLocation returns the path of the folder of skill in the library's Trail Guide directory, such as
// skills/acme/build-feature.
func skillLocation(skill skills.Skill[[]byte]) string {
	return skills.Directory + "/" + skill.Folder()
}

// skillProblems returns every problem in found, the library's skills by ID, in ID order, each starting with the file
// it's about, at its path in the library's skills/ folder: frontmatter skills.ReadMetadata or skills.Check rejects, a
// SKILL.md that still has the draft marker, and a cited rule that isn't one of current, the library's rules, naming
// the replacement when the library's release history or a pending change note retired it, and a skill a skill uses
// that isn't one of found or, when the line names a source, one of curated, the curated skills the library's levels
// list. skills.Read has already rejected every problem with the skills' folders.
func skillProblems(found map[string]skills.Skill[[]byte], current, curated []string, changes libraryChanges) []string {
	problems := []string{}
	_, retiring := changes.namedRules()
	for _, id := range slices.Sorted(maps.Keys(found)) {
		skill := found[id]
		location := skillLocation(skill) + "/" + skills.File
		metadata, err := skills.ReadMetadata(skill.Files[skills.File], location)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if rules.HasDraftMarker(skill.Files[skills.File]) {
			problems = append(problems, location+": complete the draft and remove its trailguide:draft marker")
		}
		for _, problem := range skills.Check(skill.Name, metadata, false) {
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
			} else if _, ok := found[id]; citation.Source == "" && !ok {
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

// uncalledDependencies returns a problem, in ID order, for each of dependencies, the skills library.skills.dependencies
// lists, that no project could install: a dependency installs only with the skills that use it, so each must be
// reachable through trailguide-skills-used from one of found, the library's skills by ID, that dependencies doesn't
// list, or be in curatedCalls, the curated skills that listed curated skills use. A dependency that only other
// unreachable dependencies use, such as two that use only each other, is reported too. A nil curatedCalls, when the
// curated sources weren't read, leaves curated dependencies unchecked. A SKILL.md that skillProblems rejects names
// none.
func uncalledDependencies(found map[string]skills.Skill[[]byte], dependencies []string, curatedCalls map[string]bool) []string {
	listed := map[string]bool{}
	for _, id := range dependencies {
		listed[id] = true
	}
	uses := map[string][]string{}
	called := map[string]bool{}
	reached := maps.Clone(curatedCalls)
	if reached == nil {
		reached = map[string]bool{}
	}
	pending := []string{}
	for _, id := range slices.Sorted(maps.Keys(found)) {
		metadata, err := skills.ReadMetadata(found[id].Files[skills.File], id+"/"+skills.File)
		if err != nil {
			continue
		}
		for _, used := range metadata.SkillsUsed {
			if used != id {
				uses[id] = append(uses[id], used)
				called[used] = true
			}
		}
		if !listed[id] {
			pending = append(pending, id)
		}
	}
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, used := range uses[id] {
			if !reached[used] {
				reached[used] = true
				pending = append(pending, used)
			}
		}
	}
	problems := []string{}
	for _, id := range dependencies {
		if _, _, curated := SplitCurated(id); curated && curatedCalls == nil {
			continue
		}
		switch {
		case reached[id]:
		case called[id]:
			problems = append(problems, fmt.Sprintf("library.skills.%s lists %s, but only other dependencies that no project would install name it in %s, and a dependency installs only with the skills that use it, so no project would install it. Name it in the %s of a skill a project can choose, or move it or a dependency that uses it to a level", skills.Dependencies, id, skills.SkillsUsedKey, skills.SkillsUsedKey))
		default:
			problems = append(problems, fmt.Sprintf("library.skills.%s lists %s, but no skill of the library names it in %s, and a dependency installs only with the skills that use it, so no project would install it. Name it in the %s of the skills that call it, or move it to a level", skills.Dependencies, id, skills.SkillsUsedKey, skills.SkillsUsedKey))
		}
	}
	return problems
}

// interfaceFileWarnings returns a warning, in ID order, for each of found, the library's skills by ID, that ships
// its own agents/openai.yaml holding only the interface text Trail Guide can generate from the skill's metadata,
// suggesting the metadata keys instead. A file with other settings, such as icons or a policy, is a supported
// escape hatch and gets no warning.
func interfaceFileWarnings(found map[string]skills.Skill[[]byte]) []string {
	warnings := []string{}
	for _, id := range slices.Sorted(maps.Keys(found)) {
		if data, ok := found[id].Files[skills.OpenAIFile]; ok && skills.InterfaceTextOnly(data) {
			warnings = append(warnings, fmt.Sprintf("%s/%s: it sets only interface text, which Trail Guide generates for each project from SKILL.md. Set metadata.%s, %s, and %s in SKILL.md instead, then delete this file.", skillLocation(found[id]), skills.OpenAIFile, skills.DisplayNameKey, skills.ShortDescriptionKey, skills.DefaultPromptKey))
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
		return "which isn't a skill the library curates; list it in library.skills, at a level or as a dependency, so projects that install the skill can install it too"
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

// skillsGuide explains a library's skills/ folder to people: what it publishes, how a skill's level is set, and how
// to publish a change.
//
//go:embed skills-guide.md
var skillsGuide string

// withSkillsGuide returns files with .trailguide/skills/README.md, the folder's guide, added when the library at root,
// its repository root, has none. It never replaces a guide, which the library may edit.
func withSkillsGuide(ctx context.Context, root *os.Root, files []filetxn.File) ([]filetxn.File, error) {
	name := path.Join(skillsDirectory, skills.Guide)
	existing, err := filetxn.ReadOptional(ctx, root, name)
	if err != nil || existing != nil {
		return files, err
	}
	return append(files, filetxn.File{Path: name, Content: []byte(skillsGuide)}), nil
}
