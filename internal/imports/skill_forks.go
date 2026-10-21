// Check the commits that skill forks are based on, and preview the changes a source made to forked skills.

package imports

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// ReplacedSkillUpdate is a row of the update preview for a skill the project forks, listed when the source's copy of
// the skill at the revision the update takes skills from differs from the copy the fork is based on, or the source
// no longer has the skill.
type ReplacedSkillUpdate struct {
	ID string `json:"id"`
	// LocalSkill is the fork, as the fork's as names it, which is also its folder relative to the Trail Guide
	// directory.
	LocalSkill string `json:"localSkill"`
	// BasedOn is the commit the fork was copied from, as skills.forks records it, or empty when it records none; the
	// row then compares with From.
	BasedOn string `json:"basedOn,omitempty"`
	// From is the commit the source record locks, and To the commit the update takes the source's skills from.
	From string `json:"from"`
	To   string `json:"to"`
	// Files lists, sorted and relative to the skill's folder, the files that differ between the compared copies: in
	// contents, presence, or executable bit. Removed reports that the source no longer has the skill at To, and Files
	// is then empty; the fork stays installed.
	Files   []string `json:"files"`
	Removed bool     `json:"removed,omitempty"`
}

// requireSkillBasedOn checks that the repository has the commit each of the source's skill forks is based on,
// failing with code version-not-found when it doesn't, and that the skill is in that commit, failing validation
// when it isn't. A fork of a skill the source's library curates is based on a commit of the repository the
// library curates it from, which curatedSource.requireBasedOn checks.
func (r *repository) requireSkillBasedOn(ctx context.Context, source rules.Source) error {
	return r.requireBasedOn(ctx, ownForks(source), basedOnField(source.Name, ""))
}

// requireBasedOn checks forks, by skill ID in the repository, as requireSkillBasedOn does. where names the basedOn
// field of a skill's fork, given its ID in the repository.
func (r *repository) requireBasedOn(ctx context.Context, replacements map[string]rules.SkillFork, where func(string) string) error {
	for _, id := range slices.Sorted(maps.Keys(replacements)) {
		based := replacements[id].BasedOn
		if based.IsZero() {
			continue
		}
		if _, err := r.skillEntries(ctx, where(id), id, based.Canonical()); err != nil {
			return err
		}
	}
	return nil
}

// ownForks returns the source's forks of skills the source itself publishes, by skill ID, leaving out those of
// skills its library curates.
func ownForks(source rules.Source) map[string]rules.SkillFork {
	result := maps.Clone(source.SkillForks)
	maps.DeleteFunc(result, func(id string, _ rules.SkillFork) bool {
		_, _, curated := library.SplitCurated(id)
		return curated
	})
	return result
}

// curatedForks returns the forks of source, a library source, of the skills its library curates from the curated
// source name, by the skill's ID in that source's repository.
func curatedForks(source rules.Source, name string) map[string]rules.SkillFork {
	result := map[string]rules.SkillFork{}
	for id, fork := range source.SkillForks {
		if curatedFrom, skill, curated := library.SplitCurated(id); curated && curatedFrom == name {
			result[skill] = fork
		}
	}
	return result
}

// curatedExclusions returns the exclusions of source, a library source, of the skills its library curates from the
// curated source name, by the skill's ID in that source's repository.
func curatedExclusions(source rules.Source, name string) map[string]rules.SkillExclusion {
	result := map[string]rules.SkillExclusion{}
	for id, exclusion := range source.SkillExclude {
		if curatedFrom, skill, curated := library.SplitCurated(id); curated && curatedFrom == name {
			result[skill] = exclusion
		}
	}
	return result
}

// basedOnField returns a function that names the basedOn field of the fork, in the source named source, of a
// skill by its ID in the repository: a skill the source publishes when curated is empty, and otherwise one its
// library curates from the curated source named curated.
func basedOnField(source, curated string) func(string) string {
	return func(id string) string {
		if curated != "" {
			id = curated + ":" + id
		}
		return "sources." + source + ".skills.forks." + id + ".basedOn"
	}
}

// skillEntries returns the tree entries of skill id at commit, by path in the skill's folder, fetching the commit
// when it isn't present. The fork whose basedOn field where names is based on commit, which it
// names in failures: code version-not-found when the repository doesn't have the commit, and a validation error
// when the commit doesn't have the skill.
func (r *repository) skillEntries(ctx context.Context, where, id, commit string) (map[string]treeEntry, error) {
	if err := r.fetchCommits(ctx, []string{commit}, fmt.Sprintf("%s: the repository has no commit %s; check basedOn, the full commit SHA that the fork was copied from.", where, commit)); err != nil {
		return nil, err
	}
	entries, found, err := r.skillAt(ctx, id, commit)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &ValidationError{Location: where, Problem: fmt.Sprintf("commit %s has no skill %s; check basedOn, the full commit SHA that the fork was copied from", commit, id)}
	}
	return entries, nil
}

// skillAt returns the tree entries of skill id at commit, which must be present, by path in the skill's folder,
// and whether the commit has the skill.
func (r *repository) skillAt(ctx context.Context, id, commit string) (map[string]treeEntry, bool, error) {
	isLibrary, err := r.isLibrary(ctx, commit)
	if err != nil {
		return nil, false, err
	}
	tree, err := r.repositoryTree(ctx, commit)
	if err != nil {
		return nil, false, err
	}
	folder, found := skillFolders(tree, isLibrary)[id]
	if !found {
		return nil, false, nil
	}
	entries := map[string]treeEntry{}
	for name, entry := range folder {
		entries[strings.TrimPrefix(name, id+"/")] = entry
	}
	return entries, true, nil
}

// replacedSkills returns a preview row, sorted by ID, for each skill the source forks whose copy at commit, the
// revision the update takes the source's skills from, differs from the copy its fork is based on: the fork's
// basedOn, or else the commit recorded locks; or that commit lacks, though the baseline had it. A fork without
// basedOn in a source with no record has nothing to compare with. The skills the source's library curates are
// compared in the repositories it curates them from, as curatedSource.plan does.
func (r *repository) replacedSkills(ctx context.Context, source rules.Source, recorded *library.Snapshot, commit string) ([]ReplacedSkillUpdate, error) {
	from := ""
	if recorded != nil {
		from = recorded.Commit
	}
	return r.replacedRows(ctx, source.Name, ownForks(source), basedOnField(source.Name, ""), "", from, commit)
}

// replacedRows returns the preview rows of replacements, by skill ID in the repository, which the source named source
// records, as replacedSkills returns them, comparing each skill's copy at commit with its baseline: the exclusion's basedOn, checked at the field where
// names, or else from, the commit the project recorded, which is empty when it recorded none. prefix starts each
// row's ID, such as the name of the curated source a library curates the skills from and a colon.
func (r *repository) replacedRows(ctx context.Context, source string, forks map[string]rules.SkillFork, where func(string) string, prefix, from, commit string) ([]ReplacedSkillUpdate, error) {
	rows := []ReplacedSkillUpdate{}
	for _, id := range slices.Sorted(maps.Keys(forks)) {
		fork := forks[id]
		before, baseline, err := r.replacementBaseline(ctx, source, fork, where(id), id, from)
		if err != nil {
			return nil, err
		}
		if baseline == "" || baseline == commit {
			continue
		}
		if err := r.fetchCommits(ctx, []string{commit}, fmt.Sprintf("The repository no longer has commit %s, which the update takes source %s's skills from. Run trailguide project update again.", commit, source)); err != nil {
			return nil, err
		}
		after, found, err := r.skillAt(ctx, id, commit)
		if err != nil {
			return nil, err
		}
		row := ReplacedSkillUpdate{ID: prefix + id, LocalSkill: fork.As, BasedOn: fork.BasedOn.Canonical(), From: from, To: commit, Files: []string{}}
		switch {
		case found:
			row.Files = differentEntries(before, after)
		case before != nil:
			row.Removed = true
		}
		if row.Removed || len(row.Files) > 0 {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// replacementBaseline returns the copy of skill id that fork is based on, by path in the skill's folder, and its
// commit: the fork's basedOn, checked as requireSkillBasedOn checks it at the field
// where names, or else from, the commit the record of the source named source locks, or "" when there is neither or
// the repository no longer has from. A skill from lacks has nil entries.
func (r *repository) replacementBaseline(ctx context.Context, source string, fork rules.SkillFork, where, id, from string) (map[string]treeEntry, string, error) {
	if based := fork.BasedOn; !based.IsZero() {
		entries, err := r.skillEntries(ctx, where, id, based.Canonical())
		return entries, based.Canonical(), err
	}
	if from == "" {
		return nil, "", nil
	}
	if err := r.fetchCommits(ctx, []string{from}, fmt.Sprintf("The repository no longer has commit %s, which vendor/%s/_source.json records. Run trailguide project sync to check the source.", from, source)); err != nil {
		// A branch rewritten since the project recorded from leaves nothing to compare with, which mustn't stop the
		// update that recovers the source.
		var failure *Error
		if errors.As(err, &failure) && failure.Code == "version-not-found" {
			return nil, "", nil
		}
		return nil, "", err
	}
	entries, _, err := r.skillAt(ctx, id, from)
	return entries, from, err
}

// differentEntries returns, sorted, the paths whose entries differ between before and after: present in only one,
// or with another object or mode, so a file that only becomes executable differs too.
func differentEntries(before, after map[string]treeEntry) []string {
	names := map[string]bool{}
	for _, entries := range []map[string]treeEntry{before, after} {
		for name := range entries {
			names[name] = true
		}
	}
	files := []string{}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		old, had := before[name]
		current, has := after[name]
		if !had || !has || old.object != current.object || old.mode != current.mode {
			files = append(files, name)
		}
	}
	return files
}
