// Check the skills a library curates against the repositories it curates them from, at the refs it declares.

package library

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// CuratedSkill is a skill of a source a library curates skills from: Name is the name it installs under, and Uses
// the skills of the same repository it uses, by their IDs there, which a project installs with it.
type CuratedSkill struct {
	Name string
	Uses []string
}

// CuratedReader reads the source a library curates skills from, as its manifest declares it under name: it
// resolves the source's ref, or its newest library release without one, and returns each skill of ids, and each
// skill those use, which a project installs with them, by their IDs in the repository, leaving out those the
// repository has no skill for there, and the revision's name for messages, such as main or release/3.
type CuratedReader func(ctx context.Context, name string, source rules.CuratedSource, ids []string) (map[string]CuratedSkill, string, error)

// requireCuratedSkills fails when manifest, the library's configuration, lists a curated skill that its source
// doesn't have at the ref the library declares, or that uses one the source doesn't have, or when two skills it offers projects, its own in authored, its
// skills/ folder, the curated ones it lists, and the curated ones those use, which projects install with them, have
// the same install name, which agents find skills by. read reads each curated source; nil skips the check.
func requireCuratedSkills(ctx context.Context, manifest []byte, authored *filetxn.Tree, read CuratedReader) error {
	declaration, err := rules.ParseLibraryManifest(manifest, manifestPath)
	if err != nil || declaration == nil || read == nil {
		return err
	}
	// Every skill the library authors reaches projects, since one no level lists is offered.
	names := map[string]string{}
	if authored != nil {
		for file := range authored.Files {
			if name, found := strings.CutSuffix(file, "/"+skills.File); found && !strings.Contains(name, "/") {
				names[name] = skills.AuthoredID(name)
			}
		}
	}
	listed := declaration.Skills.Curated()
	wanted := map[string][]string{}
	for _, id := range listed {
		source, skill, _ := SplitCurated(id)
		wanted[source] = append(wanted[source], skill)
	}
	installed, revisions := map[string]map[string]CuratedSkill{}, map[string]string{}
	for _, source := range slices.Sorted(maps.Keys(wanted)) {
		if installed[source], revisions[source], err = read(ctx, source, declaration.Curated[source], wanted[source]); err != nil {
			return err
		}
	}
	for _, id := range listed {
		source, skill, _ := SplitCurated(id)
		if _, ok := installed[source][skill]; !ok {
			field := "library.skills." + declaration.Skills.Level(id)
			return &ValidationError{Location: manifestPath + ": " + field, Problem: field + " names " + id + ", which " + source + " doesn't have at " + revisions[source] + "; check the skill ID, or set sources." + source + ".ref to a revision that has it"}
		}
	}
	// A curated skill installs under the name its SKILL.md declares, which needn't be its folder's.
	seen := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		seen[name] = names[name]
	}
	for _, id := range listed {
		source, skill, _ := SplitCurated(id)
		name := installed[source][skill].Name
		if other, clash := seen[name]; clash {
			return &ValidationError{Location: manifestPath + ": library.skills", Problem: "library.skills lists " + other + " and " + id + ", which both install as " + name + "; list one of them, or rename the folder of your own skill"}
		}
		seen[name] = id
	}
	forks := map[string]string{}
	if config, err := rules.ParseConfigurationYAML(manifest); err == nil {
		for _, source := range config.Sources {
			for skill, exclusion := range source.SkillExclude {
				if exclusion.ReplacedBy != "" {
					forks[source.Name+":"+skill] = exclusion.ReplacedBy
				}
			}
		}
	}
	for _, id := range listed {
		source, skill, _ := SplitCurated(id)
		for _, used := range usedSkills(installed[source], skill) {
			usedID := source + ":" + used
			field := "library.skills." + declaration.Skills.Level(id)
			if _, found := installed[source][used]; !found {
				return &ValidationError{Location: manifestPath + ": " + field, Problem: id + " uses " + usedID + ", which " + source + " doesn't have at " + revisions[source] + ", so projects can't install " + id + "; stop listing it, or set sources." + source + ".ref to a revision that has both"}
			}
			name := installed[source][used].Name
			other, clash := seen[name]
			if !clash {
				seen[name] = usedID
				continue
			}
			if other == usedID {
				continue
			}
			problem := id + " uses " + usedID + ", which projects install with it as " + name + ", beside " + other + ", which also installs as " + name
			if forks[usedID] == other {
				problem = id + " uses " + usedID + ", which projects install with it as " + name + " beside your fork " + other + "; fork " + id + " too, so it uses your fork, or stop listing it"
			} else {
				problem += "; stop listing one of them, or rename the folder of your own skill"
			}
			return &ValidationError{Location: manifestPath + ": " + field, Problem: problem}
		}
	}
	return nil
}

// usedSkills returns, sorted, the skills, by ID, that the skill id of known uses, directly or through the skills it
// uses, including those known lacks, whose uses it can't follow.
func usedSkills(known map[string]CuratedSkill, id string) []string {
	found := map[string]bool{}
	pending := slices.Clone(known[id].Uses)
	for len(pending) > 0 {
		used := pending[0]
		pending = pending[1:]
		if found[used] || used == id {
			continue
		}
		found[used] = true
		pending = append(pending, known[used].Uses...)
	}
	return slices.Sorted(maps.Keys(found))
}
