// Preview how an update changes the skills a source installs, flagging each executable script it adds or changes.

package imports

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
)

// SkillChange is how an update affects one skill, or one script in a skill, as the update preview lists it.
type SkillChange string

// The update preview's skill changes, in the order it lists them.
const (
	SkillChanged SkillChange = "changed"
	SkillAdded   SkillChange = "added"
	SkillRemoved SkillChange = "removed"
)

var skillChangeOrder = []SkillChange{SkillChanged, SkillAdded, SkillRemoved}

// SkillUpdate is one skill's row in the update preview: a skill the update starts or stops installing, or one whose
// files differ from the copy the project's source record locks.
type SkillUpdate struct {
	ID     string      `json:"id"`
	Change SkillChange `json:"change"`
	// Files lists, sorted and relative to the skill's folder, the files of a changed skill that the update adds,
	// changes, or removes, including a file whose executable bit changes. It is empty, never nil, for other rows.
	Files []string `json:"files"`
	// Scripts lists, sorted by path, each file that is executable at the update's revision and that the update adds
	// or changes, including one it makes executable; every executable file of an added skill is added. It is empty,
	// never nil, when the row flags none.
	Scripts []ScriptUpdate `json:"scripts"`
}

// ScriptUpdate is an executable file the update adds to a skill or changes in it.
type ScriptUpdate struct {
	// Path is relative to the skill's folder.
	Path string `json:"path"`
	// Change is SkillAdded for a file the skill didn't have, and SkillChanged for one whose contents changed or that
	// becomes executable.
	Change SkillChange `json:"change"`
}

// skillPreview is what the update preview shows of a source's skills at one revision: rows, as skillChanges gives
// them, or err, the failure reading them there, such as a skill skills.include names that the revision lacks, which
// installing that revision would fail with too; and replaced, the replaced skill rows that revision gives, as
// replacedSkills lists them.
type skillPreview struct {
	rows     []SkillUpdate
	err      error
	replaced []ReplacedSkillUpdate
	levels   []SkillLevelUpdate
	// manifest is the library's .trailguide/config.yaml at the revision, or nil when it has none, and used the curated
	// skills the library's skills the source installs there use, as curatedUses gives them; curated and revisions are
	// what planCurated plans from them.
	manifest  []byte
	used      map[string][]string
	curated   []CuratedUpdate
	revisions map[string]library.CuratedSource
}

// previewSkills reads the skills source installs at each of plans' revisions and compares them with recorded, the
// source's snapshot with its skills' files, or nil when the project has none, returning each revision's preview by
// commit. It leaves out the skills the source excludes, which the project doesn't install, and lists a replaced one
// as a replaced skill row of that revision instead. A revision whose skills it can't read keeps its failure, so only
// choosing that revision fails; a cancellation, or a failure to compare a replaced skill, fails at once.
func (r *repository) previewSkills(ctx context.Context, source rules.Source, recorded *library.Snapshot, plans []sourcePlan) (map[string]skillPreview, error) {
	previews := map[string]skillPreview{}
	for _, plan := range plans {
		if _, read := previews[plan.commit]; read {
			continue
		}
		replaced, err := r.replacedSkills(ctx, source, recorded, plan.commit)
		if err != nil {
			return nil, err
		}
		levels, err := r.skillLevelChanges(ctx, recorded, plan.commit)
		if err != nil {
			return nil, err
		}
		var manifest []byte
		if !plan.nonLibrary {
			if manifest, err = r.manifestAt(ctx, plan.commit); err != nil {
				return nil, err
			}
		}
		installed, err := r.readSkills(ctx, source, plan, !plan.nonLibrary, 0)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, gitexec.ContextFailure(ctxErr)
		}
		if err != nil {
			previews[plan.commit] = skillPreview{err: err, replaced: replaced, levels: levels, manifest: manifest}
			continue
		}
		used, err := curatedUses(installed.skills, installed.files, source.Name+"@"+plan.commit)
		if err != nil {
			previews[plan.commit] = skillPreview{err: err, replaced: replaced, levels: levels, manifest: manifest}
			continue
		}
		// A curated skill's rows are its curated source's, which planCurated compares at that source's revisions.
		rows := slices.DeleteFunc(skillChanges(recorded, installed), func(row SkillUpdate) bool {
			_, excluded := source.SkillExclude[row.ID]
			_, forked := source.SkillForks[row.ID]
			return excluded || forked || strings.Contains(row.ID, ":")
		})
		previews[plan.commit] = skillPreview{rows: rows, replaced: replaced, levels: levels, manifest: manifest, used: used}
	}
	return previews, nil
}

// skillChanges compares the skills recorded holds, with their files, to installed, and returns a row for each skill
// that only one of them has, and for each skill whose files or executable bits differ, sorted by change in the order
// of the SkillChange constants, then by ID. It is empty, never nil, when nothing differs. A nil recorded holds no
// skills.
func skillChanges(recorded *library.Snapshot, installed importedSkills) []SkillUpdate {
	before := library.Snapshot{Skills: map[string]library.ImportedSkill{}}
	if recorded != nil {
		before = *recorded
	}
	rows := []SkillUpdate{}
	ids := slices.Sorted(maps.Keys(installed.skills))
	for id := range before.Skills {
		if _, kept := installed.skills[id]; !kept {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if _, kept := installed.skills[id]; !kept {
			rows = append(rows, SkillUpdate{ID: id, Change: SkillRemoved, Files: []string{}, Scripts: []ScriptUpdate{}})
			continue
		}
		files, scripts := skillFileChanges(id, before, installed)
		_, had := before.Skills[id]
		switch {
		case !had:
			rows = append(rows, SkillUpdate{ID: id, Change: SkillAdded, Files: []string{}, Scripts: scripts})
		case len(files) > 0:
			rows = append(rows, SkillUpdate{ID: id, Change: SkillChanged, Files: files, Scripts: scripts})
		}
	}
	slices.SortFunc(rows, func(a, b SkillUpdate) int {
		if order := slices.Index(skillChangeOrder, a.Change) - slices.Index(skillChangeOrder, b.Change); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return rows
}

// skillFileChanges returns, relative to the folder of skill id and sorted, the files whose contents, presence, or
// executable bit differ between before and installed, and the scripts among them: those executable in installed.
// The skill's folder can differ between them, as when its library moves it into an origin folder.
func skillFileChanges(id string, before library.Snapshot, installed importedSkills) ([]string, []ScriptUpdate) {
	beforeFolder, installedFolder := before.SkillFolder(id)+"/", library.SkillFolderIn(installed.files, id)+"/"
	names := map[string]bool{}
	for _, side := range []struct {
		files  map[string][]byte
		prefix string
	}{{before.Files, beforeFolder}, {installed.files, installedFolder}} {
		for name := range side.files {
			if relative, inside := strings.CutPrefix(name, side.prefix); inside {
				names[relative] = true
			}
		}
	}
	files := []string{}
	scripts := []ScriptUpdate{}
	for _, relative := range slices.Sorted(maps.Keys(names)) {
		old, had := before.Files[beforeFolder+relative]
		current, has := installed.files[installedFolder+relative]
		executable := slices.Contains(installed.executable, installedFolder+relative)
		if had && has && bytes.Equal(old, current) && slices.Contains(before.Executable, beforeFolder+relative) == executable {
			continue
		}
		files = append(files, relative)
		switch {
		case has && executable && !had:
			scripts = append(scripts, ScriptUpdate{Path: relative, Change: SkillAdded})
		case has && executable:
			scripts = append(scripts, ScriptUpdate{Path: relative, Change: SkillChanged})
		}
	}
	return files, scripts
}
