// Read a skill from a source a library declares, for a fork, and compare a library's forks with their sources.

package imports

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// ReadSourceSkill reads skill id from the source a library declares under name, at the revision a project that
// uses the library would take from declared, its repository and ref: the commit the ref names now, or, without a
// ref, the repository's newest library release. It fails when that revision has no such skill, when its library
// requires the skill, which every project that uses the library installs, or when a skill file is a symbolic link or
// submodule.
func ReadSourceSkill(ctx context.Context, name string, declared rules.CuratedSource, id string, options Options) (skill library.SourceSkill, err error) {
	err = withSourceRevision(ctx, name, declared, options, func(repo *repository, revision string, at string, release int, nonLibrary bool) error {
		entries, found, err := repo.skillAt(ctx, id, revision)
		if err != nil {
			return err
		}
		if !found {
			return &ValidationError{Location: "--from", Problem: fmt.Sprintf("source %s has no skill %s at %s; name a skill folder's path in its repository, such as skills/fix-bug", name, id, at)}
		}
		if !nonLibrary {
			manifest, err := repo.manifestAt(ctx, revision)
			if err != nil {
				return err
			}
			declaration, err := rules.ParseLibraryManifest(manifest, "sources."+name)
			if err != nil {
				return err
			}
			if declaration != nil && slices.Contains(declaration.Skills.Required, id) {
				return &ValidationError{Location: "--from", Problem: rules.RequiredSkillProblem(name, id, release)}
			}
		}
		files, executable, err := repo.readFiles(ctx, name, entries, 0)
		skill = library.SourceSkill{Commit: revision, Revision: at, NonLibrary: nonLibrary, Files: files, Executable: executable}
		return err
	})
	return skill, err
}

// LibraryForks compares the skills a library forked from the source it declares under name, as declared, with the
// source's copies at the revision ReadSourceSkill reads. forks holds the source's exclusions that name a
// replacement, by skill ID; an exclusion without basedOn has nothing to compare with. It returns a row, sorted by ID,
// for each skill whose copy differs, and fails when the repository has no basedOn commit, or that commit has no such
// skill.
func LibraryForks(ctx context.Context, name string, declared rules.CuratedSource, forks map[string]rules.SkillExclusion, options Options) (rows []library.ForkedSkill, err error) {
	rows = []library.ForkedSkill{}
	err = withSourceRevision(ctx, name, declared, options, func(repo *repository, revision string, at string, _ int, _ bool) error {
		for _, id := range slices.Sorted(maps.Keys(forks)) {
			exclusion := forks[id]
			if exclusion.BasedOn.IsZero() || exclusion.BasedOn.Canonical() == revision {
				continue
			}
			before, err := repo.skillEntries(ctx, basedOnField(name, "")(id), id, exclusion.BasedOn.Canonical())
			if err != nil {
				return err
			}
			after, found, err := repo.skillAt(ctx, id, revision)
			if err != nil {
				return err
			}
			row := library.ForkedSkill{Source: name, ID: id, Skill: exclusion.ReplacedBy, BasedOn: exclusion.BasedOn.Canonical(), Commit: revision, Revision: at, Files: []string{}, Removed: !found}
			if found {
				row.Files = differentEntries(before, after)
			}
			if row.Removed || len(row.Files) > 0 {
				rows = append(rows, row)
			}
		}
		return nil
	})
	return rows, err
}

// withSourceRevision opens the repository of the source a library declares under name, as declared, resolves and
// fetches its revision as a project that uses the library would, and calls read with the repository, the
// revision's commit and name, its library release number, or 0 when it isn't one, and whether it has no library
// manifest, within one import deadline.
func withSourceRevision(ctx context.Context, name string, declared rules.CuratedSource, options Options, read func(repo *repository, commit, at string, release int, nonLibrary bool) error) (err error) {
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return err
	}
	defer cancel()
	repo, err := openRepository(ctx, rules.Source{Name: name, Repository: declared.Repository}, options)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	revision, err := curatedSource{name: name, declared: declared, maintained: true}.resolve(ctx, repo)
	if err != nil {
		return err
	}
	at := revisionName(revision.Release, revision.Commit)
	if !declared.Ref.IsZero() {
		at = declared.Ref.String()
	}
	return read(repo, revision.Commit, at, revision.Release, revision.NonLibrary)
}
