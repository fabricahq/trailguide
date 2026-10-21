// Merge a source's changes to a forked skill into the project's fork during an update, refusing changes that conflict
// with the fork's own edits.

package project

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// skillForkUpdate is a skill fork the update merges the source's changes into: the authored skill named name, which
// replaces skill id of source. mergeSkillForks fills files.
type skillForkUpdate struct {
	source, id, name string
	// files is the merged fork, by path in its folder.
	files imports.SkillCopy
}

// skillMerge is the result of merging the source's changes to a forked skill into the fork: the merged copy, or the
// files whose changes conflict.
type skillMerge struct {
	merged    imports.SkillCopy
	conflicts []string
}

// mergeSkillForks merges, for each forked skill row of sources that has basedOn and whose source still has the
// skill, the source's changes since basedOn into the fork as the project was planned, marking the row with whether
// they merge and which files conflict. It fills each of forks, the decided merges, with its merged copy, and refuses
// with invalid-arguments one whose changes conflict. A merge is computed once per plan.
func (p *UpdatePlan) mergeSkillForks(ctx context.Context, sources []imports.SourceUpdate, forks []skillForkUpdate) error {
	if p.skillMerges == nil {
		p.skillMerges = map[string]skillMerge{}
	}
	for i := range sources {
		source := p.planned.config.Sources[slices.IndexFunc(p.planned.config.Sources, func(s rules.Source) bool { return s.Name == sources[i].Name })]
		rows := []*imports.ReplacedSkillUpdate{}
		for j := range sources[i].ReplacedSkills {
			rows = append(rows, &sources[i].ReplacedSkills[j])
		}
		for j := range sources[i].Curated {
			for k := range sources[i].Curated[j].Replaced {
				rows = append(rows, &sources[i].Curated[j].Replaced[k])
			}
		}
		for _, row := range rows {
			if row.Base == nil || row.Upstream == nil {
				continue
			}
			key := source.Name + ":" + row.ID + "@" + row.To
			result, ok := p.skillMerges[key]
			if !ok {
				var err error
				if result, err = p.mergeSkillFork(ctx, source, *row); err != nil {
					return err
				}
				p.skillMerges[key] = result
			}
			row.Merge, row.Conflicts = imports.SkillMergeNoConflicts, nil
			if len(result.conflicts) > 0 {
				row.Merge, row.Conflicts = imports.SkillMergeConflicts, slices.Clone(result.conflicts)
			}
		}
	}
	for i := range forks {
		row := previewSkillRow(sources, forks[i].source, forks[i].id)
		result := p.skillMerges[forks[i].source+":"+row.ID+"@"+row.To]
		if len(result.conflicts) > 0 {
			return invalidArguments(fmt.Sprintf("--merge-fork %s:%s: your fork's edits and the source's changes since commit %s touch the same lines of %s, so Trail Guide can't merge them. Merge the source's changes into %s by hand, then set sources.%s.skills.forks.%s.basedOn to %q", forks[i].source, forks[i].id, shortSHA(row.BasedOn), strings.Join(result.conflicts, ", "), row.LocalSkill, forks[i].source, forks[i].id, row.To))
		}
		forks[i].files = result.merged
	}
	return nil
}

// mergeSkillFork merges the source's changes to row's skill, from its copy at basedOn to its copy at the commit the
// update takes it from, into the fork, as forking either copy would have written it. A copy that can't be written as
// a fork, such as one whose frontmatter Trail Guide can't read, conflicts in its SKILL.md, for the fork's owner
// to merge by hand.
func (p *UpdatePlan) mergeSkillFork(ctx context.Context, source rules.Source, row imports.ReplacedSkillUpdate) (skillMerge, error) {
	name := path.Base(row.LocalSkill)
	base, err := forkedSkillCopy(source, row, *row.Base, name)
	if err != nil {
		return skillMerge{conflicts: []string{skills.File}}, nil
	}
	upstream, err := forkedSkillCopy(source, row, *row.Upstream, name)
	if err != nil {
		return skillMerge{conflicts: []string{skills.File}}, nil
	}
	merged, conflicts, err := imports.MergeSkill(ctx, p.git, base, authoredSkillCopy(p.planned, name), upstream)
	if err != nil {
		return skillMerge{}, err
	}
	return skillMerge{merged: merged, conflicts: conflicts}, nil
}

// forkedSkillCopy returns copy, the source's copy of row's skill, as forking it into the authored skill named name
// writes it: with its SKILL.md rewritten as PlanSkillFork rewrites it, and marked as a draft when the author must
// finish what Trail Guide couldn't rewrite.
func forkedSkillCopy(source rules.Source, row imports.ReplacedSkillUpdate, copy imports.SkillCopy, name string) (imports.SkillCopy, error) {
	files := maps.Clone(copy.Files)
	content, ok := files[skills.File]
	if !ok {
		return copy, nil
	}
	location := "sources." + source.Name + ".skills.forks." + row.ID
	id, prefix, qualifyRules, uses := row.ID, source.Name, row.Library, source.SkillUses[row.ID]
	if curatedFrom, curatedID, curated := library.SplitCurated(row.ID); curated {
		id, prefix, qualifyRules, uses = curatedID, source.Name+":"+curatedFrom, false, nil
	}
	metadata, err := skills.ReadMetadata(content, location)
	if err != nil {
		return imports.SkillCopy{}, err
	}
	if installed, err := skills.InstallName(id, metadata, location); err != nil || installed != name {
		content = skills.Rename(content, name)
		if metadata, err = skills.ReadMetadata(content, location); err != nil {
			return imports.SkillCopy{}, err
		}
	}
	content, fixes, err := forkedSkillFile(content, metadata, name, prefix, qualifyRules, uses, location)
	if err != nil {
		return imports.SkillCopy{}, err
	}
	if len(fixes) > 0 {
		content = skills.Mark(content, rules.DraftMarker)
	}
	files[skills.File] = content
	return imports.SkillCopy{Files: files, Executable: copy.Executable}, nil
}

// authoredSkillCopy returns the authored skill named name, by path in its folder, as the project state has it.
func authoredSkillCopy(state projectState, name string) imports.SkillCopy {
	result := imports.SkillCopy{Files: map[string][]byte{}, Executable: []string{}}
	for file, data := range treeFiles(state.skills) {
		if relative, ok := strings.CutPrefix(file, name+"/"); ok {
			result.Files[relative] = data
		}
	}
	if state.skills != nil {
		for _, file := range state.skills.Executable {
			if relative, ok := strings.CutPrefix(file, name+"/"); ok {
				result.Executable = append(result.Executable, relative)
			}
		}
	}
	return result
}

// previewSkillRow returns the forked skill row of skill id in source, among its own skills' rows and its curated
// sources', or nil when the preview doesn't list it.
func previewSkillRow(sources []imports.SourceUpdate, source, id string) *imports.ReplacedSkillUpdate {
	for i := range sources {
		if sources[i].Name != source {
			continue
		}
		for j := range sources[i].ReplacedSkills {
			if sources[i].ReplacedSkills[j].ID == id {
				return &sources[i].ReplacedSkills[j]
			}
		}
		for j := range sources[i].Curated {
			for k := range sources[i].Curated[j].Replaced {
				if sources[i].Curated[j].Replaced[k].ID == id {
					return &sources[i].Curated[j].Replaced[k]
				}
			}
		}
	}
	return nil
}

// shortSHA returns the first seven characters of commit, as Git abbreviates it.
func shortSHA(commit string) string { return commit[:min(7, len(commit))] }
