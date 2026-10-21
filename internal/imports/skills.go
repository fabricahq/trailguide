// Find, choose, and read the skills a source installs, all from the one revision the source's plan names.

package imports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// librarySkillsFolder is where a library keeps the skills it publishes, relative to its repository root.
const librarySkillsFolder = librarytree.Directory + "/" + skills.Directory + "/"

// skillFolders returns the skills in files, a commit's tree by repository path, keyed by skill ID, each with its
// files keyed by their path in the vendor snapshot. A library's skills are the folders directly in its
// .trailguide/skills/ that hold a SKILL.md, with IDs skills/<name> and snapshot paths in skills/. Any other
// repository's are the folders that hold a SKILL.md, other than the repository root and a folder inside another
// skill's folder, which belongs to that skill; their IDs and snapshot paths are their repository paths.
func skillFolders(files map[string]treeEntry, isLibrary bool) map[string]map[string]treeEntry {
	ids := []string{}
	for name := range files {
		folder, found := strings.CutSuffix(name, "/"+skills.File)
		if !found {
			continue
		}
		if isLibrary {
			if skill, inside := strings.CutPrefix(folder, librarySkillsFolder); inside && !strings.Contains(skill, "/") {
				ids = append(ids, skills.AuthoredID(skill))
			}
			continue
		}
		// A colon separates a curated skill's source from its ID, so a folder whose path has one can't be named.
		if !strings.Contains(folder, ":") {
			ids = append(ids, folder)
		}
	}
	slices.Sort(ids)
	// Sorting puts each folder before the folders inside it, so an outer skill is kept before its inner ones are seen.
	kept := []string{}
	for _, id := range ids {
		if !slices.ContainsFunc(kept, func(outer string) bool { return strings.HasPrefix(id, outer+"/") }) {
			kept = append(kept, id)
		}
	}
	result := map[string]map[string]treeEntry{}
	for _, id := range kept {
		result[id] = map[string]treeEntry{}
	}
	for name, entry := range files {
		stored := name
		if isLibrary {
			inside, found := strings.CutPrefix(name, librarytree.Directory+"/")
			if !found {
				continue
			}
			stored = inside
		}
		for id := range result {
			if strings.HasPrefix(stored, id+"/") {
				entry.name = stored
				result[id][stored] = entry
				break
			}
		}
	}
	return result
}

// chooseSkills returns, sorted, the IDs of the skills the source installs from available, the IDs of the skills in
// the plan's revision, given levels, the levels its library gives them, or nil for a repository that isn't a
// library: a library's required skills, its recommended ones when skills.include has recommended, for "*" every
// skill except its dependencies and those in a folder whose name starts with a dot, such as another tool's installed
// copies in .claude/skills/, and each skill it names, which requireNoNamedDependencies has checked isn't a dependency. A named skill the revision lacks fails: with skill-not-in-revision when
// the revision is the one the source record locks, since sync never moves a source, and otherwise as invalid
// configuration. Excluding or forking a required skill fails too, since a project that uses the library can't diverge
// from it, as does excluding or forking one LibraryRequires lists. Each skill skills.forks names is chosen when
// available has it, so updates can compare it with its fork, whatever skills.include selects; one available lacks,
// such as a skill the source deleted, isn't, and its fork stays, even when skills.include names it. A level exclusion, recommended or "*" in skills.exclude, then leaves
// out the chosen skills of its levels, as rules.Source.ExcludesLevel says, but never a required one.
// The skills the library curates, whose IDs have a colon, are curateSkills' to choose, and naming one of a repository
// that isn't a library fails, since only a library curates skills; the skills the configuration's own library section
// requires of the source, LibraryRequires, are chosen as required too.
func chooseSkills(source rules.Source, plan sourcePlan, available []string, levels *rules.SkillLevels) ([]string, error) {
	chosen := []string{}
	revision := revisionName(plan.release, plan.commit)
	for _, id := range source.LibraryRequires {
		if !slices.Contains(available, id) {
			return nil, &ValidationError{Location: "library.skills.required", Problem: fmt.Sprintf("lists %s:%s, which sources.%s doesn't have at %s; check the skill ID, or set sources.%s.ref to a revision that has it", source.Name, id, source.Name, revision, source.Name)}
		}
	}
	diverging := source.SkillDivergences()
	for _, id := range slices.Sorted(maps.Keys(diverging)) {
		if slices.Contains(source.LibraryRequires, id) {
			return nil, &ValidationError{Location: diverging[id], Problem: rules.LibraryRequiresProblem(source.Name, id)}
		}
	}
	chosen = append(chosen, source.LibraryRequires...)
	if levels != nil {
		own := ownSkillLevels(*levels)
		levels = &own
		for _, id := range levels.Listed() {
			if !slices.Contains(available, id) {
				return nil, fail("unsupported-content", fmt.Sprintf("sources.%s: the library's manifest at %s gives skill %s a level, but the library has no such skill there. Ask the library's maintainer to fix library.skills; trailguide library check catches this.", source.Name, revision, id), nil)
			}
		}
		chosen = append(chosen, levels.Required...)
		for _, id := range slices.Sorted(maps.Keys(diverging)) {
			if slices.Contains(levels.Required, id) {
				return nil, &ValidationError{Location: diverging[id], Problem: rules.RequiredSkillProblem(source.Name, id, plan.release)}
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(source.SkillForks)) {
		if slices.Contains(available, id) {
			chosen = append(chosen, id)
		}
	}
	for _, id := range source.Skills {
		switch {
		case id == skills.Wildcard:
			chosen = append(chosen, slices.DeleteFunc(slices.Clone(available), func(id string) bool {
				return library.HiddenSkill(id) || levels != nil && levels.Level(id) == skills.LevelDependency
			})...)
		case id == skills.Recommended && levels == nil:
			return nil, &ValidationError{Location: "sources." + source.Name + ".skills.include", Problem: fmt.Sprintf("the repository isn't a library at %s, so it recommends no skills; name its skills by ID, or include \"*\" for every skill it has", revision)}
		case id == skills.Recommended:
			chosen = append(chosen, levels.Recommended...)
		case slices.Contains(available, id):
			chosen = append(chosen, id)
		case isForked(source.SkillForks, id):
			// The source deleted a skill the project forks, and the fork stays.
		case strings.Contains(id, ":") && levels != nil:
		case strings.Contains(id, ":"):
			return nil, &ValidationError{Location: "sources." + source.Name + ".skills.include", Problem: fmt.Sprintf("names %s, but only a library's curated skills have IDs with a colon, and the repository isn't a library at %s, so it curates none; name its skills by their folder, such as skills/to-spec", id, revision)}
		default:
			return nil, skillNotInRevision(source, plan, "sources."+source.Name+".skills.include", id)
		}
	}
	if _, recommended := source.SkillExcludeLevels[skills.Recommended]; recommended && levels == nil {
		return nil, &ValidationError{Location: "sources." + source.Name + ".skills.exclude." + skills.Recommended, Problem: fmt.Sprintf("the repository isn't a library at %s, so it recommends no skills to exclude; exclude its skills by ID, or exclude \"*\" to install none of them", revision)}
	}
	chosen = slices.DeleteFunc(chosen, func(id string) bool {
		level := skills.LevelOffered
		if levels != nil {
			level = levels.Level(id)
		}
		return !slices.Contains(source.LibraryRequires, id) && source.ExcludesLevel(id, level)
	})
	slices.Sort(chosen)
	return slices.Compact(chosen), nil
}

// ownSkillLevels returns levels without the skills the library curates.
func ownSkillLevels(levels rules.SkillLevels) rules.SkillLevels {
	own := func(ids []string) []string {
		return slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return strings.Contains(id, ":") })
	}
	return rules.SkillLevels{Required: own(levels.Required), Recommended: own(levels.Recommended), Offered: own(levels.Offered), Dependencies: own(levels.Dependencies)}
}

// skillNotInRevision reports that field, a configuration field of source, names skill id, which the plan's revision
// lacks: with skill-not-in-revision when the revision is the one the source record locks, since sync never moves a
// source, and otherwise as invalid configuration at field.
func skillNotInRevision(source rules.Source, plan sourcePlan, field, id string) error {
	revision := revisionName(plan.release, plan.commit)
	if plan.locked != "" && plan.locked == plan.commit {
		next := fmt.Sprintf("Run trailguide project update %s, which moves the source to its newest library release, and takes the skill from it if it has it, or check the skill ID.", source.Name)
		retag := fmt.Sprintf("set sources.%s.ref to a tag or commit that has it, then run trailguide project sync, or check the skill ID.", source.Name)
		switch {
		case plan.nonLibrary && source.Ref.Kind() == rules.GitRefBranch:
			next = fmt.Sprintf("Run trailguide project update %s, which takes the branch's newest commit, and the skill if it has it, or check the skill ID.", source.Name)
		case plan.nonLibrary && source.Ref.Bare():
			next = fmt.Sprintf("If sources.%s.ref names a branch, run trailguide project update %s, which takes the branch's newest commit, and the skill if it has it. Otherwise, %s", source.Name, source.Name, retag)
		case source.Ref.Release() > 0:
			next = fmt.Sprintf("Run trailguide project update %s, which moves the source to its newest library release and sets sources.%s.ref to it, and takes the skill from it if it has it, or check the skill ID.", source.Name, source.Name)
		case !source.Ref.IsZero() && !source.Ref.Latest():
			next = "To take the skill, " + retag
		}
		return fail("skill-not-in-revision", fmt.Sprintf("%s names %s, which isn't in %s, the revision vendor/%s/_source.json records for the source. Sync never moves a source, so it takes every skill from that revision. %s", field, id, revision, source.Name, next), nil)
	}
	return &ValidationError{Location: field, Problem: fmt.Sprintf("the source has no skill %s at %s; check the skill ID. A library's skills are the folders in its .trailguide/skills/, such as skills/build-feature; another repository's are the folders that hold a SKILL.md, named by their path from the repository root", id, revision)}
}

// requireConfiguredUses checks that every skill the source's skills.uses lists is one of available, the IDs of the
// skills in the plan's revision, failing as skillNotInRevision does. Whether each key is installed is
// installedSkills' to check, once it knows what the source installs.
func requireConfiguredUses(source rules.Source, plan sourcePlan, available []string) error {
	for _, id := range slices.Sorted(maps.Keys(source.SkillUses)) {
		for _, used := range source.SkillUses[id] {
			if !slices.Contains(available, used) {
				return skillNotInRevision(source, plan, "sources."+source.Name+".skills.uses."+id, used)
			}
		}
	}
	return nil
}

// installedSkills returns every skill the source installs, by ID, with why: chosen, the skills skills.include
// selects, and every skill they use, transitively. uses maps each skill chosen reaches that the plan's revision has
// to the skills it uses, by its trailguide-skills-used and the source's skills.uses. A skill that uses one the revision lacks,
// skills that use each other in a cycle, and a skills.uses key the source doesn't install fail as invalid
// configuration of the source's skills, naming the skills involved.
func installedSkills(source rules.Source, plan sourcePlan, chosen []string, uses map[string][]string) (map[string]skills.Installed, error) {
	location := "sources." + source.Name + ".skills"
	installed, err := skills.ResolveUses(chosen, uses)
	var missing *skills.MissingUseError
	var cycle *skills.UseCycleError
	switch {
	case errors.As(err, &missing):
		installing := missing.Skill
		if missing.Via != missing.Skill {
			installing = fmt.Sprintf("%s, which %s uses", missing.Skill, missing.Via)
		}
		return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("%s uses %s, which isn't a skill in %s: %s names it in trailguide-skills-used. Trail Guide installs every skill an installed skill uses, so it can't install %s. Ask the source's maintainer to correct trailguide-skills-used, or stop including %s", missing.Skill, missing.Missing, revisionName(plan.release, plan.commit), missing.Skill, installing, missing.Via)}
	case errors.As(err, &cycle):
		return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("the skills the source installs use each other in a cycle: %s. Trail Guide installs every skill an installed skill uses, so uses can't form a cycle. Ask the source's maintainer to correct trailguide-skills-used, or remove the use from %s.uses", strings.Join(cycle.Cycle, " -> "), location)}
	case err != nil:
		return nil, err
	}
	for _, id := range slices.Sorted(maps.Keys(source.SkillUses)) {
		if _, ok := installed[id]; !ok {
			return nil, &ValidationError{Location: location + ".uses." + id, Problem: id + " isn't a skill the source installs, so it has no uses to declare; a key of skills.uses names a skill that skills.include selects, or one those skills use. Include the skill, or remove the entry"}
		}
	}
	return installed, nil
}

// skillUses reads the SKILL.md of every skill chosen reaches in folders, the skills of the plan's commit, and returns
// what each uses: its trailguide-skills-used lines, then the skills the source's skills.uses adds. It fetches the
// SKILL.md files one level of uses at a time, so it never downloads those of skills nothing installed uses. A used
// skill folders lacks is absent from the result, for installedSkills to report. A library's skill can also use a
// skill the library curates, named with its curated source's name; isLibrary leaves those out, for curateSkills to
// install. A skill of forked, which the project forks, uses none: its fork replaces it, so the skills the source's
// copy uses aren't installed for it. It fails when a SKILL.md it reads has unreadable frontmatter.
func (r *repository) skillUses(ctx context.Context, source rules.Source, folders map[string]map[string]treeEntry, chosen []string, isLibrary bool, forked []string) (map[string][]string, error) {
	uses := map[string][]string{}
	for pending := chosen; len(pending) > 0; {
		entries := map[string]treeEntry{}
		for _, id := range pending {
			if _, seen := uses[id]; seen {
				continue
			}
			if slices.Contains(forked, id) {
				uses[id] = []string{}
				continue
			}
			if entry, ok := folders[id][id+"/"+skills.File]; ok {
				entries[id] = entry
			}
		}
		if err := r.prefetch(ctx, slices.Collect(maps.Values(entries))); err != nil {
			return nil, err
		}
		files := map[string]treeEntry{}
		for _, entry := range entries {
			files[entry.name] = entry
		}
		input := newGitFiles(ctx, r, files)
		pending = nil
		for _, id := range slices.Sorted(maps.Keys(entries)) {
			file := entries[id].name
			data, err := input.ReadFile(file)
			if err != nil {
				return nil, err
			}
			metadata, err := skills.ReadMetadata(data, "vendor/"+source.Name+"/"+file)
			if err != nil {
				return nil, err
			}
			used := metadata.SkillsUsed
			if isLibrary {
				used = slices.DeleteFunc(slices.Clone(used), isCuratedUse)
			}
			uses[id] = slices.Concat(used, source.SkillUses[id])
			pending = append(pending, uses[id]...)
		}
	}
	return uses, nil
}

// isCuratedUse reports whether line, a line of trailguide-skills-used in a library's skill, names a skill the
// library curates, which has its curated source's name and a colon before its ID.
func isCuratedUse(line string) bool { return strings.Contains(line, ":") }

// curatedUses returns, by the ID of each curated skill that a skill of installed uses, such as
// anthropic:skills/skill-creator, the IDs of those skills, sorted. installed holds a library source's skills by ID,
// and files their files by snapshot path; curated skills, and the skills they use, use none of the library's. It
// fails when a SKILL.md has unreadable frontmatter, reading it at where and its path.
func curatedUses(installed map[string]library.ImportedSkill, files map[string][]byte, where string) (map[string][]string, error) {
	result := map[string][]string{}
	for _, id := range slices.Sorted(maps.Keys(installed)) {
		if _, _, curated := library.SplitCurated(id); curated {
			continue
		}
		file := library.SkillFolder(id) + "/" + skills.File
		metadata, err := skills.ReadMetadata(files[file], where+"/"+file)
		if err != nil {
			return nil, err
		}
		for _, line := range metadata.SkillsUsed {
			if isCuratedUse(line) && !slices.Contains(result[line], id) {
				result[line] = append(result[line], id)
			}
		}
	}
	return result, nil
}

// isLibrary reports whether a fetched commit holds a library: a .trailguide/config.yaml with a library section, or a
// rule-library.yaml in the layout before version 2. A configuration it can't read counts as a library's, so the
// catalog loader reports what is wrong with it.
func (r *repository) isLibrary(ctx context.Context, commit string) (bool, error) {
	tree, err := r.repositoryTree(ctx, commit)
	if err != nil {
		return false, err
	}
	if _, legacy := tree[librarytree.LegacyManifest]; legacy {
		return true, nil
	}
	data, found, err := r.manifest(ctx, tree)
	if err != nil || !found {
		return false, err
	}
	_, encoded, err := decode.YAML(data, librarytree.RepositoryPath(librarytree.Manifest))
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(encoded, &fields) != nil {
		return true, nil
	}
	_, library := fields["library"]
	return library, nil
}

// skillLevels returns the levels the library at a fetched commit gives its skills, or nil when the commit holds no
// library. A library in the layout before version 2 gives none, so each of its skills is offered.
func (r *repository) skillLevels(ctx context.Context, commit string) (*rules.SkillLevels, error) {
	tree, err := r.repositoryTree(ctx, commit)
	if err != nil {
		return nil, err
	}
	if _, legacy := tree[librarytree.LegacyManifest]; legacy {
		return &rules.SkillLevels{Required: []string{}, Recommended: []string{}, Offered: []string{}, Dependencies: []string{}}, nil
	}
	data, found, err := r.manifest(ctx, tree)
	if err != nil || !found {
		return nil, err
	}
	declaration, err := rules.ParseLibraryManifest(data, librarytree.RepositoryPath(librarytree.Manifest))
	if err != nil || declaration == nil {
		return nil, err
	}
	return &declaration.Skills, nil
}

// manifest reads .trailguide/config.yaml from tree, a commit's tree by repository path, reporting false when it has
// none.
func (r *repository) manifest(ctx context.Context, tree map[string]treeEntry) ([]byte, bool, error) {
	file := librarytree.RepositoryPath(librarytree.Manifest)
	entry, ok := tree[file]
	if !ok {
		return nil, false, nil
	}
	data, err := newGitFiles(ctx, r, map[string]treeEntry{file: entry}).ReadFile(file)
	return data, err == nil, err
}

// importedSkills is what a source installs of its skills: their files and executable files by snapshot path, each
// skill's record by ID, and the level of every skill the source offers at the revision.
type importedSkills struct {
	files      map[string][]byte
	executable []string
	skills     map[string]library.ImportedSkill
	available  map[string]library.ImportedSkill
}

// readSkills reads the files of the skills the source installs at the plan's commit, a library's when isLibrary is
// set: the skills chooseSkills chooses, and every skill they use, transitively, recording why each is installed.
// retained is the bytes the source's import already read, which count toward the same retained limit. It fails for a
// skill file that is a symbolic link or submodule, over the import byte limits, as installedSkills does for the
// skills' uses, and as requireSkillBasedOn does for a replacement's basedOn.
func (r *repository) readSkills(ctx context.Context, source rules.Source, plan sourcePlan, isLibrary bool, retained int) (importedSkills, error) {
	result := importedSkills{files: map[string][]byte{}, executable: []string{}, skills: map[string]library.ImportedSkill{}, available: map[string]library.ImportedSkill{}}
	if err := r.requireSkillBasedOn(ctx, source); err != nil {
		return importedSkills{}, err
	}
	tree, err := r.repositoryTree(ctx, plan.commit)
	if err != nil {
		return importedSkills{}, err
	}
	var levels *rules.SkillLevels
	if isLibrary {
		if levels, err = r.skillLevels(ctx, plan.commit); err != nil {
			return importedSkills{}, err
		}
	}
	folders := skillFolders(tree, isLibrary)
	available := slices.Sorted(maps.Keys(folders))
	for _, id := range available {
		result.available[id] = importedSkill(id, levels)
	}
	if err := r.requireNoNamedDependencies(ctx, source, plan, folders, levels); err != nil {
		return importedSkills{}, err
	}
	chosen, err := chooseSkills(source, plan, available, levels)
	if err != nil {
		return importedSkills{}, err
	}
	if err := requireConfiguredUses(source, plan, available); err != nil {
		return importedSkills{}, err
	}
	uses, err := r.skillUses(ctx, source, folders, chosen, isLibrary, slices.Collect(maps.Keys(source.SkillForks)))
	if err != nil {
		return importedSkills{}, err
	}
	installed, err := installedSkills(source, plan, chosen, uses)
	if err != nil {
		return importedSkills{}, err
	}
	entries := map[string]treeEntry{}
	for _, id := range slices.Sorted(maps.Keys(installed)) {
		skill := importedSkill(id, levels)
		skill.Included, skill.UsedBy = installed[id].Included, installed[id].UsedBy
		result.skills[id] = skill
		maps.Copy(entries, folders[id])
	}
	if result.files, result.executable, err = r.readFiles(ctx, source.Name, entries, retained); err != nil {
		return importedSkills{}, err
	}
	return result, nil
}

// readFiles reads the skill files entries lists, by snapshot path, for the source named source, returning their
// bytes and, sorted, the paths of the executable ones. retained is the bytes the source's import already read, which
// count toward the same retained limit. It fails for a symbolic link or submodule, which Trail Guide doesn't install.
func (r *repository) readFiles(ctx context.Context, source string, entries map[string]treeEntry, retained int) (map[string][]byte, []string, error) {
	files, executable := map[string][]byte{}, []string{}
	prefetch := []treeEntry{}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if !entries[name].mode.IsRegular() {
			return nil, nil, fail("unsupported-content", fmt.Sprintf("sources.%s: skill file %s is a symbolic link or submodule, which Trail Guide doesn't install. Ask the skill's maintainer to replace it with a file.", source, name), nil)
		}
		prefetch = append(prefetch, entries[name])
	}
	if err := r.prefetch(ctx, prefetch); err != nil {
		return nil, nil, err
	}
	input := newGitFiles(ctx, r, entries)
	input.total = retained
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		data, err := input.ReadFile(name)
		if err != nil {
			return nil, nil, err
		}
		files[name] = data
		if entries[name].mode.Perm()&0o100 != 0 {
			executable = append(executable, name)
		}
	}
	return files, executable, nil
}

// importedSkill returns the record of skill id given levels, the levels its library gives its skills, or nil for a
// repository that isn't a library, whose skills have no level.
func importedSkill(id string, levels *rules.SkillLevels) library.ImportedSkill {
	if levels == nil {
		return library.ImportedSkill{}
	}
	return library.ImportedSkill{Level: levels.Level(id)}
}

// skillLevelChanges returns, sorted by ID, the update preview's rows for the skills whose level changes between the
// skills recorded imported and the library at commit, the revision an update takes them from. A recorded source
// that isn't a library, or a commit that holds none, has none.
func (r *repository) skillLevelChanges(ctx context.Context, recorded *library.Snapshot, commit string) ([]SkillLevelUpdate, error) {
	rows := []SkillLevelUpdate{}
	if recorded == nil || recorded.NonLibrary {
		return rows, nil
	}
	levels, err := r.skillLevels(ctx, commit)
	if err != nil || levels == nil {
		return rows, err
	}
	tree, err := r.repositoryTree(ctx, commit)
	if err != nil {
		return nil, err
	}
	after := map[string]string{}
	for id := range skillFolders(tree, true) {
		after[id] = levels.Level(id)
	}
	// A curated skill is in the library's lists, not its skills/ folder.
	for _, id := range levels.Curated() {
		after[id] = levels.Level(id)
	}
	ids := slices.Sorted(maps.Keys(after))
	for id := range recorded.AvailableSkills {
		if _, kept := after[id]; !kept {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		from, to := recorded.AvailableSkills[id].Level, after[id]
		if from != to && (from != "" && to != "" || from == skills.LevelRequired || to == skills.LevelRequired) {
			rows = append(rows, SkillLevelUpdate{ID: id, From: from, To: to})
		}
	}
	return rows, nil
}

// isForked reports whether forks, a source's skills.forks, forks skill id.
func isForked(forks map[string]rules.SkillFork, id string) bool {
	_, forked := forks[id]
	return forked
}

// requireNoNamedDependencies fails when the source's skills.include names a skill that levels, its library's levels,
// or nil for a repository that isn't a library, list in library.skills.dependencies, own or curated: such a skill
// installs only with the skills that use it, so the project can't choose it. The error names the library's skills in
// folders, the skills of the plan's commit, that use it, by their trailguide-skills-used or the source's skills.uses,
// so the project can include one of those instead; for a curated dependency only the library's skills that use it
// are known here, so when none does, it points to the curated skills of its source, which may use it in their own
// repository.
func (r *repository) requireNoNamedDependencies(ctx context.Context, source rules.Source, plan sourcePlan, folders map[string]map[string]treeEntry, levels *rules.SkillLevels) error {
	if levels == nil {
		return nil
	}
	named := slices.DeleteFunc(slices.Clone(source.Skills), func(id string) bool { return levels.Level(id) != skills.LevelDependency })
	if len(named) == 0 {
		return nil
	}
	users, err := r.skillUsers(ctx, source, folders)
	if err != nil {
		return err
	}
	id := named[0]
	problem := fmt.Sprintf("names %s, which the library lists in library.skills.%s, so it installs only with the library's skills that use it, and a project can't choose it. ", id, skills.Dependencies)
	curatedFrom, _, curated := library.SplitCurated(id)
	switch {
	case len(users[id]) == 0 && curated:
		// The curated skills that use it in their own repository are in that repository, which import doesn't read here.
		problem += fmt.Sprintf("It installs with the skills the library curates from %s that use it; include one of those instead, or remove it from skills.include", curatedFrom)
	case len(users[id]) == 0:
		problem += fmt.Sprintf("No skill of the library uses it at %s; remove it from skills.include, and ask the library's maintainer to fix library.skills, which trailguide library check catches", revisionName(plan.release, plan.commit))
	default:
		problem += fmt.Sprintf("Include a skill that uses it instead, %s, or remove it from skills.include", strings.Join(users[id], " or "))
	}
	return &ValidationError{Location: "sources." + source.Name + ".skills.include", Problem: problem}
}

// skillUsers returns, by the ID of each skill that a skill in folders, a library's skills, uses, by its
// trailguide-skills-used or the source's skills.uses, the IDs of the skills that use it, sorted. A curated skill
// keeps the ID the library lists it under, such as anthropic:skills/skill-creator. It fails when a SKILL.md has
// unreadable frontmatter.
func (r *repository) skillUsers(ctx context.Context, source rules.Source, folders map[string]map[string]treeEntry) (map[string][]string, error) {
	entries := map[string]treeEntry{}
	for id, files := range folders {
		if entry, ok := files[id+"/"+skills.File]; ok {
			entries[entry.name] = entry
		}
	}
	if err := r.prefetch(ctx, slices.Collect(maps.Values(entries))); err != nil {
		return nil, err
	}
	input := newGitFiles(ctx, r, entries)
	result := map[string][]string{}
	for _, id := range slices.Sorted(maps.Keys(folders)) {
		used := slices.Clone(source.SkillUses[id])
		if _, ok := folders[id][id+"/"+skills.File]; ok {
			file := id + "/" + skills.File
			data, err := input.ReadFile(file)
			if err != nil {
				return nil, err
			}
			metadata, err := skills.ReadMetadata(data, "vendor/"+source.Name+"/"+file)
			if err != nil {
				return nil, err
			}
			used = append(used, metadata.SkillsUsed...)
		}
		for _, skill := range used {
			if skill != id && !slices.Contains(result[skill], id) {
				result[skill] = append(result[skill], id)
			}
		}
	}
	return result, nil
}
