// Fork a skill from a source the library declares into its skills/ folder, and compare its forks with their sources.

package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// SourceSkill is the copy of one skill that a source the library declares has at the revision the library's ref
// for the source names.
type SourceSkill struct {
	// Commit is the full commit SHA the skill was read at, and Revision its name for messages, such as main or
	// release/3.
	Commit, Revision string
	// NonLibrary reports that the source's repository has no library manifest at Commit.
	NonLibrary bool
	// Files holds the skill's files by path in its folder, and Executable, sorted, the paths of the executable ones.
	Files      map[string][]byte
	Executable []string
}

// ForkedSkill is a skill the library forked from one of its sources whose copy at the revision the library's ref
// for the source names differs from the copy the fork is based on.
type ForkedSkill struct {
	// Source names the library's source, ID is the skill's ID in it, and Skill the library's own skill that forks it,
	// as the fork's as names it.
	Source string `json:"source"`
	ID     string `json:"id"`
	Skill  string `json:"skill"`
	// BasedOn is the commit the fork was copied from, and Commit the one the source's ref resolves to now, which
	// Revision names, such as main or release/3.
	BasedOn  string `json:"basedOn"`
	Commit   string `json:"commit"`
	Revision string `json:"revision"`
	// Files lists, sorted and relative to the skill's folder, the files that differ between the two copies: in
	// contents, presence, or executable bit. Removed reports that the source no longer has the skill, and Files is
	// then empty.
	Files   []string `json:"files"`
	Removed bool     `json:"removed,omitempty"`
}

// SkillReader reads skill id from the source the library declares under name, with its repository and ref, at the
// revision a project that uses the library would take: the commit the ref names now, or, without a ref, the
// repository's newest library release.
type SkillReader func(ctx context.Context, name string, source rules.CuratedSource, id string) (SourceSkill, error)

// ForkReader compares the skills the library forked from the source it declares under name with the source's copies
// at the revision a SkillReader reads, given the source's forks, by skill ID, and returns a row, sorted by ID, for
// each one that differs from the copy its fork is based on.
type ForkReader func(ctx context.Context, name string, source rules.CuratedSource, forks map[string]rules.SkillFork) ([]ForkedSkill, error)

// SkillForkPlan is a planned fork of a skill from a source the library declares. Planning reads the library and the
// source without writing; Commit writes the fork.
type SkillForkPlan struct {
	source, id string
	options    Options
	// configBytes is the configuration planning read; Commit refuses to write when it changed.
	configBytes []byte
	// name is the skill's install name, which names the fork's folder, and skill the source's copy it forks.
	name  string
	skill SourceSkill
	// renamed reports whether the source's skills.rename gave the skill its install name; Commit then removes that
	// entry, since the fork replaces it.
	renamed bool
	// listed reports that library.skills lists the skill as one the library curates from the source; Commit lists
	// the fork in its place, at the same level.
	listed bool
	// files holds the fork's files, by path in the repository.
	files []filetxn.File
	// followsReleases reports that the library declares the source without a ref, so it takes the source's newest
	// library release.
	followsReleases bool
}

// PlanSkillFork checks the library and prepares a fork of skill id from the source the library declares under
// source, copied at the revision options.ReadSkill reads, into .trailguide/skills/<name>/, where name is the skill's
// install name, the one the source's skills.rename gives it when it has one, without writing. The fork's SKILL.md
// names the skill name, and its trailguide-skills-used names each skill of the source it uses as forkedLibrarySkill
// does. It fails when the library doesn't declare the source, already excludes or forks the skill, or already has an
// authored skill with that name or another fork that installs as it, since existing skills, exclusions, and forks
// are never replaced; when options.ReadSkill fails, such as for a skill the source's library requires; when the skill
// cites rules of a source that is a library, which the library can't cite; and when its frontmatter can't be
// rewritten, or the result fails the checks library check applies to the library's skills.
func PlanSkillFork(ctx context.Context, source, id string, options Options) (*SkillForkPlan, error) {
	plan := &SkillForkPlan{source: source, id: id}
	options, err := planAuthoring(ctx, options, func(root *os.Root, options Options) error {
		original, config, err := readLibraryConfiguration(ctx, root)
		if err != nil {
			return err
		}
		plan.configBytes = original
		declared, err := forkSource(config, source, id)
		if err != nil {
			return err
		}
		if options.ReadSkill == nil {
			return failure("invalid-operation", "expected a reader for the library's sources", nil)
		}
		if plan.skill, err = options.ReadSkill(ctx, source, rules.CuratedSource{Repository: declared.Repository, Ref: declared.Ref}, id); err != nil {
			return err
		}
		location := source + ":" + id + "/" + skills.File
		metadata, err := skills.ReadMetadata(plan.skill.Files[skills.File], location)
		if err != nil {
			return err
		}
		if name, renamed := declared.SkillRename[id]; renamed {
			plan.name, plan.renamed = name, true
		} else if plan.name, err = skills.InstallName(id, metadata, location); err != nil {
			return err
		}
		if err := requireForkTarget(ctx, root, config, plan.name); err != nil {
			return err
		}
		plan.listed = slices.Contains(config.Library.Skills.Listed(), source+":"+id)
		for _, name := range slices.Sorted(maps.Keys(plan.skill.Files)) {
			content := plan.skill.Files[name]
			if name == skills.File {
				if plan.renamed {
					content = skills.Rename(content, plan.name)
					if metadata, err = skills.ReadMetadata(content, location); err != nil {
						return err
					}
				}
				if content, err = forkedLibrarySkill(content, metadata, plan.name, declared, id, !plan.skill.NonLibrary, config.Library.Skills.Listed(), plan.skill.Commit, location); err != nil {
					return err
				}
			}
			plan.files = append(plan.files, filetxn.File{Path: path.Join(skillsDirectory, plan.name, name), Content: content, Executable: slices.Contains(plan.skill.Executable, name)})
		}
		plan.followsReleases = declared.Ref.IsZero()
		return nil
	})
	if err != nil {
		return nil, err
	}
	plan.options = options
	return plan, nil
}

// readLibraryConfiguration returns the library's .trailguide/config.yaml and its parsed configuration.
func readLibraryConfiguration(ctx context.Context, root *os.Root) ([]byte, rules.Configuration, error) {
	data, err := readOptionalBelow(ctx, root, manifestPath)
	if err != nil {
		return nil, rules.Configuration{}, err
	}
	config, err := rules.ParseConfigurationYAML(data)
	var validation errs.ValidationError
	if errors.As(err, &validation) {
		return nil, rules.Configuration{}, &ValidationError{Location: manifestPath + ": " + validation.ValidationLocation(), Problem: validation.ValidationProblem()}
	}
	if err == nil && config.Library == nil {
		return nil, rules.Configuration{}, failure("needs-init", manifestPath+" has no library section, so this repository isn't a library; run trailguide library init to add one", nil)
	}
	return data, config, err
}

// forkSource returns the source config declares under name, from which the library forks skill id. It fails when
// there is none, when id names a skill that source's library curates, and when the source already excludes or forks
// id.
func forkSource(config rules.Configuration, name, id string) (rules.Source, error) {
	index := slices.IndexFunc(config.Sources, func(source rules.Source) bool { return source.Name == name })
	if index < 0 {
		return rules.Source{}, &ValidationError{Location: "--from", Problem: "no source named " + name + " in " + manifestPath + "; add the repository the skill comes from to sources, then name it"}
	}
	if _, _, curated := SplitCurated(id); curated {
		return rules.Source{}, &ValidationError{Location: "--from", Problem: fmt.Sprintf("%s names a skill that source %s curates from another repository; declare that repository as a source of your own, and fork the skill from it", id, name)}
	}
	source := config.Sources[index]
	if _, excluded := source.SkillExclude[id]; excluded {
		return rules.Source{}, &ValidationError{Location: "sources." + name + ".skills.exclude." + id, Problem: "the source already excludes this skill, and a fork never replaces an existing exclusion; delete the entry to fork the skill"}
	}
	if _, forked := source.SkillForks[id]; forked {
		return rules.Source{}, &ValidationError{Location: "sources." + name + ".skills.forks." + id, Problem: "the library already forks this skill, and a fork never replaces an existing one; delete the entry and the fork's folder to fork the skill again"}
	}
	return source, nil
}

// requireForkTarget fails when the library already has the skill name in .trailguide/skills/, or another fork
// already installs that skill.
func requireForkTarget(ctx context.Context, root *os.Root, config rules.Configuration, name string) error {
	id := skills.AuthoredID(name)
	for _, source := range config.Sources {
		for _, forked := range slices.Sorted(maps.Keys(source.SkillForks)) {
			if source.SkillForks[forked].As == id {
				return &ValidationError{Location: "sources." + source.Name + ".skills.forks." + forked + ".as", Problem: fmt.Sprintf("this fork already installs as %s, the new fork's folder, and each authored skill replaces one skill; remove that fork first", id)}
			}
		}
	}
	return filetxn.RequireAbsent(ctx, root, path.Join(skillsDirectory, name))
}

// forkedLibrarySkill returns data, the SKILL.md of skill, by its ID in source, whose metadata is metadata, read at
// location, as the library's fork, skills/<name>, needs it: naming the skill name, and naming once each skill that
// its trailguide-skills-used, or source's skills.uses for it, names, as the library does: by the library's own fork
// of it, when source's skills.forks has one, and otherwise with the source's name, such as
// pstack:skills/tdd, since in the library a bare ID names the library's own skill. library reports that the source
// is a library, whose rules the fork can't cite, curated lists the skills the library's levels list, and commit is
// the commit the fork copies. It fails when the skill cites such rules, names a skill its source curates from another
// repository, or uses a skill the library neither forked nor lists, which projects couldn't install with the fork,
// when the frontmatter can't be rewritten, and when the result fails the checks library check applies to the
// library's skills.
func forkedLibrarySkill(data []byte, metadata skills.Metadata, name string, source rules.Source, skill string, library bool, curated []string, commit, location string) ([]byte, error) {
	byHand := forkLibrarySkillByHand(name, source.Name, commit)
	if library && len(metadata.RulesUsed) > 0 {
		return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("the skill cites rules of library %s, and a library's skills cite only the library's own rules. %s", source.Name, byHand)}
	}
	used := []string{}
	// The source's skills.uses applies to the skill the fork replaces, so the fork lists those skills itself.
	for _, id := range slices.Concat(metadata.SkillsUsed, source.SkillUses[skill]) {
		if strings.Contains(id, ":") {
			return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("%s names %s, a skill %s curates from another repository. The library can name it only by declaring that repository as a source of its own. %s", skills.SkillsUsedKey, id, source.Name, byHand)}
		}
		citation := source.Name + ":" + id
		if fork := source.SkillForks[id].As; fork != "" {
			citation = fork
		} else if !slices.Contains(curated, citation) {
			return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("the skill uses %s, which the library neither forked nor lists, so projects couldn't install it with the fork. List %s in library.skills, at any level, or fork it first, so this fork uses your fork", citation, citation)}
		}
		if !slices.Contains(used, citation) {
			used = append(used, citation)
		}
	}
	text, ok := skills.Fork(data, metadata, name, used, metadata.RulesUsed)
	if !ok {
		return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("Trail Guide can't rewrite this frontmatter for the fork, because it isn't written as block mappings Trail Guide can edit. The fork must name the skill %s, and name the skills it uses with the source's name, such as %s:skills/tdd. %s", name, source.Name, byHand)}
	}
	forked, err := skills.ReadMetadata(text, location)
	if err != nil {
		return nil, err
	}
	if problems := skills.Check(name, forked, false); len(problems) > 0 {
		return nil, &ValidationError{Location: location, Problem: fmt.Sprintf("the fork would fail library check, which checks the library's own skills: %s. %s", problems[0], byHand)}
	}
	return text, nil
}

// forkLibrarySkillByHand tells how to fork the skill named name, from the source named source at commit, into the
// library when the command can't.
func forkLibrarySkillByHand(name, source, commit string) string {
	return fmt.Sprintf("To fork the skill by hand, copy its folder into .trailguide/skills/%s/ and correct its frontmatter. Then add an entry for it to sources.%s.skills.forks with as: skills/%s, basedOn: \"%s\", and a reason", name, source, name, commit)
}

// Name returns the fork's name, which is the skill's install name, so its ID is skills/<name>.
func (p *SkillForkPlan) Name() string { return p.name }

// Skill returns the source's copy of the skill that the fork copies.
func (p *SkillForkPlan) Skill() SourceSkill { return p.skill }

// Listed reports that the library listed the skill as one it curates, so the fork takes its place at its level.
func (p *SkillForkPlan) Listed() bool { return p.listed }

// FollowsReleases reports that the library declares the fork's source without a ref, so it forked the skill from the
// source's newest library release.
func (p *SkillForkPlan) FollowsReleases() bool { return p.followsReleases }

// Commit writes the fork under writer ownership, and, in the same edit, adds the skill to the source's skills.forks,
// with the fork as as, the commit it copied as basedOn, and reason, removing the skill's skills.rename entry, and, when library.skills lists the skill the library curates, lists the fork in its place. It writes nothing when
// reason is blank, when the configuration changed after planning, or when any of the fork's files exists.
func (p *SkillForkPlan) Commit(ctx context.Context, reason string) (AuthoringResult, error) {
	if p == nil || p.name == "" {
		return AuthoringResult{}, failure("invalid-operation", "expected a planned skill fork", nil)
	}
	if strings.TrimSpace(reason) == "" {
		return AuthoringResult{}, &ValidationError{Location: "--reason", Problem: "give the reason the library publishes its own fork instead of the skill from " + p.source}
	}
	basedOn, err := rules.ParseGitRef(p.skill.Commit, "basedOn")
	if err != nil {
		return AuthoringResult{}, err
	}
	root, err := openLibrary(ctx, p.options, false)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer root.Close()
	dir, err := openTrailGuideDirectory(ctx, root, false)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer dir.Close()
	changes, err := filetxn.EditIn(ctx, dir, root, func() ([]filetxn.File, error) {
		original, config, err := readLibraryConfiguration(ctx, root)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(original, p.configBytes) {
			return nil, failure("concurrent-change", "the library's configuration changed after the fork was planned; run the command again", nil)
		}
		if err := requireForkTarget(ctx, root, config, p.name); err != nil {
			return nil, err
		}
		// The library repository is a project too, and the fork records where the skill came from, as a project's
		// fork does, so library check can compare it with the source's later copies.
		edit := rules.SourceEdit{SkillForks: map[string]rules.SkillFork{p.id: {As: skills.AuthoredID(p.name), Reason: reason, BasedOn: basedOn}}}
		if p.renamed {
			edit.SkillUnrename = []string{p.id}
		}
		edited, err := rules.EditConfigurationSource(original, p.source, edit)
		if err != nil {
			return nil, err
		}
		if p.listed {
			if edited, err = rules.ReplaceLibrarySkill(edited, p.source+":"+p.id, skills.AuthoredID(p.name)); err != nil {
				return nil, err
			}
		}
		return append(slices.Clone(p.files), filetxn.File{Path: manifestPath, Content: edited, Previous: original}), nil
	})
	return authoringResult(changes, err)
}

// forkedSkills compares the skills the library forked with the sources it forked them from, as manifest, its
// configuration, records them in each source's skills.forks, using forks, and returns a row for each that its
// source changed since, by source in configuration order, then by ID. A nil forks compares nothing.
func forkedSkills(ctx context.Context, manifest []byte, forks ForkReader) ([]ForkedSkill, error) {
	rows := []ForkedSkill{}
	if forks == nil {
		return rows, nil
	}
	config, err := rules.ParseConfigurationYAML(manifest)
	if err != nil {
		return nil, err
	}
	for _, source := range config.Sources {
		replaced := map[string]rules.SkillFork{}
		for id, fork := range source.SkillForks {
			if _, _, curated := SplitCurated(id); !curated && !fork.BasedOn.IsZero() {
				replaced[id] = fork
			}
		}
		if len(replaced) == 0 {
			continue
		}
		changed, err := forks(ctx, source.Name, rules.CuratedSource{Repository: source.Repository, Ref: source.Ref}, replaced)
		if err != nil {
			return nil, err
		}
		rows = append(rows, changed...)
	}
	return rows, nil
}
