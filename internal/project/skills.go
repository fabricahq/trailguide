// Read a project's authored skills, and plan the installed skill folders build, sync, and update replace or remove.

package project

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// authoredSkills returns the authored skills in tree, the project's skills/ folder, by ID, skills/<name>, as
// skills.Read finds them in the origin folders config's skill origins give them: the library's name or local for the
// project's own skills, and the source's name for a fork. It fails for the first problem skills.Read reports. A nil
// tree has none.
func authoredSkills(config rules.Configuration, tree *filetxn.Tree) (map[string]skills.Skill[[]byte], error) {
	if tree == nil {
		return map[string]skills.Skill[[]byte]{}, nil
	}
	found, problems := skills.Read(tree.Files, tree.Executable, tree.Directories, config.SkillOrigins())
	if len(problems) > 0 {
		return nil, &ValidationError{Location: ".trailguide/" + problems[0].Location, Problem: problems[0].Problem}
	}
	return found, nil
}

// skillFolderPath returns the path, in the Trail Guide directory, of the folder of skill name in the origin folder
// origin, such as skills/local/release-checklist.
func skillFolderPath(origin, name string) string {
	return path.Join(skills.Directory, skills.Folder(origin, name))
}

// requireNoSkillNamed fails when the Trail Guide directory has the folder of skill name in the origin folder origin,
// or a skill named name in any origin folder, since a skill's name is its ID.
func requireNoSkillNamed(ctx context.Context, root *os.Root, origin, name string) error {
	if err := filetxn.RequireAbsent(ctx, root, skillFolderPath(origin, name)); err != nil {
		return err
	}
	tree, err := filetxn.ReadTree(ctx, root, skills.Directory)
	if err != nil || tree == nil {
		return err
	}
	if other, taken := skills.DiscoverLayout(slices.Collect(maps.Keys(tree.Files)))[name]; taken {
		return failure("already-exists", skillFolderPath(other, name)+": already a skill named "+name+", and a skill's name is its ID; choose a different name", nil)
	}
	return nil
}

// skillInstall is what a write does to the installed skill folders: the targets and executable files it passes to
// filetxn, the folders it touches as read before planning, and the file changes, by path from the repository root.
type skillInstall struct {
	targets    map[filetxn.Target]map[string][]byte
	executable map[filetxn.Target][]string
	// read holds each folder the write touches, by path from the repository root, as read before planning; a
	// folder that was absent is nil.
	read    map[string]*filetxn.Tree
	changes FileChanges
	// folders lists, sorted by folder, each installed skill folder the write adds, replaces, or removes.
	folders []skillFolderChange
}

// skillFolderChange is one installed skill folder a write adds, replaces, or removes, as check reports it.
type skillFolderChange struct {
	// Folder is the folder's path from the repository root, such as .claude/skills/tdd.
	Folder string
	// Skill is the project ID of the skill installed there, as the generated or the current installed-skills.json
	// record names it.
	Skill string
	// Kind is MissingSkill for a folder the write adds, ModifiedSkill for one it replaces, and UnexpectedSkill for
	// one it removes.
	Kind ProblemKind
}

// planSkillFolders plans the installed skill folders of output, given generated, the generated tree before the
// write, whose installed-skills.json lists the folders Trail Guide installed. It replaces each folder output
// installs whose files differ, and removes each folder Trail Guide installed that output no longer installs. root is
// the project's Trail Guide directory, whose parent is the repository root. It fails with skill-folder-not-managed
// when a folder output installs already exists and Trail Guide didn't install it, and with unsafe-path when a skill
// target's folder is a symbolic link.
func planSkillFolders(ctx context.Context, root *os.Root, generated *filetxn.Tree, output build.Output) (skillInstall, error) {
	plan := skillInstall{targets: map[filetxn.Target]map[string][]byte{}, executable: map[filetxn.Target][]string{}, read: map[string]*filetxn.Tree{}}
	installed := build.InstalledSkillIDs(treeFiles(generated)[build.InstalledSkillsFile])
	owned := slices.Sorted(maps.Keys(installed))
	installing := build.InstalledSkillIDs(output.Files[build.InstalledSkillsFile])
	touched := slices.Sorted(maps.Keys(output.Skills))
	for _, folder := range owned {
		if !slices.Contains(touched, folder) {
			touched = append(touched, folder)
		}
	}
	slices.Sort(touched)
	if len(touched) == 0 {
		plan.changes = compareFiles(nil, nil)
		return plan, nil
	}
	read, err := readSkillFolders(ctx, root, touched)
	if err != nil {
		return skillInstall{}, err
	}
	plan.read = read
	before, after := map[string][]byte{}, map[string][]byte{}
	modeChanged := []string{}
	for _, folder := range touched {
		current := read[folder]
		want, installs := output.Skills[folder]
		if current != nil && !slices.Contains(owned, folder) {
			return skillInstall{}, failure("skill-folder-not-managed", fmt.Sprintf("%s already exists, and Trail Guide didn't install it, so it won't replace it. Move or delete the folder, or stop including the skill that installs there, then run the command again.", folder), nil)
		}
		if current != nil {
			for name, data := range current.Files {
				before[folder+"/"+name] = data
			}
		}
		if !installs {
			if current != nil {
				plan.targets[filetxn.SkillFolder(folder)] = nil
				plan.folders = append(plan.folders, skillFolderChange{Folder: folder, Skill: installed[folder], Kind: UnexpectedSkill})
			}
			continue
		}
		for name, data := range want.Files {
			after[folder+"/"+name] = data
		}
		var executable []string
		if current != nil {
			executable = current.Executable
			for name := range want.Files {
				if _, ok := current.Files[name]; ok && slices.Contains(executable, name) != slices.Contains(want.Executable, name) {
					modeChanged = append(modeChanged, folder+"/"+name)
				}
			}
		}
		if current != nil && maps.EqualFunc(current.Files, want.Files, bytes.Equal) && slices.Equal(nonNil(executable), want.Executable) {
			continue
		}
		target := filetxn.SkillFolder(folder)
		plan.targets[target] = want.Files
		plan.executable[target] = want.Executable
		kind := ModifiedSkill
		if current == nil {
			kind = MissingSkill
		}
		plan.folders = append(plan.folders, skillFolderChange{Folder: folder, Skill: installing[folder], Kind: kind})
	}
	plan.changes = compareFiles(before, after)
	for _, name := range modeChanged {
		if !slices.Contains(plan.changes.Changed, name) {
			plan.changes.Changed = append(plan.changes.Changed, name)
		}
	}
	slices.Sort(plan.changes.Changed)
	return plan, nil
}

// requireSkillFoldersUnchanged fails with concurrent-change when a folder plan read changed since, so a write never
// replaces or removes edits made while it ran.
func requireSkillFoldersUnchanged(ctx context.Context, root *os.Root, plan skillInstall) error {
	if len(plan.read) == 0 {
		return nil
	}
	after, err := readSkillFolders(ctx, root, slices.Sorted(maps.Keys(plan.read)))
	if err != nil {
		return err
	}
	for folder, tree := range plan.read {
		if (tree == nil) != (after[folder] == nil) || (tree != nil && tree.Digest() != after[folder].Digest()) {
			return failure("concurrent-change", folder+": the installed skill folder changed during the operation; retry after edits finish", nil)
		}
	}
	return nil
}

// readSkillFolders reads each folder, by path from the repository root, the parent of root, keeping a missing one
// as nil.
func readSkillFolders(ctx context.Context, root *os.Root, folders []string) (map[string]*filetxn.Tree, error) {
	repository, err := filetxn.Open(ctx, filepath.Dir(root.Name()))
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	read := map[string]*filetxn.Tree{}
	for _, folder := range folders {
		tree, err := filetxn.ReadTree(ctx, repository, folder)
		if err != nil {
			return nil, err
		}
		read[folder] = tree
	}
	return read, nil
}

// prefixChanges returns changes with prefix before each path.
func prefixChanges(changes FileChanges, prefix string) FileChanges {
	for _, list := range []*[]string{&changes.Added, &changes.Changed, &changes.Removed} {
		prefixed := []string{}
		for _, name := range *list {
			prefixed = append(prefixed, prefix+name)
		}
		*list = prefixed
	}
	return changes
}

// mergeChanges adds the paths of extra to changes, keeping each list sorted.
func mergeChanges(changes *FileChanges, extra FileChanges) {
	for _, list := range []struct {
		to   *[]string
		from []string
	}{{&changes.Added, extra.Added}, {&changes.Changed, extra.Changed}, {&changes.Removed, extra.Removed}} {
		*list.to = append(nonNil(*list.to), list.from...)
		slices.Sort(*list.to)
	}
}

// noSkillTargetsWarning returns the warning for a project that installs skills but has no skill targets, so no copy
// was installed, or empty when it has targets or no skills.
func noSkillTargetsWarning(output build.Output, targets []string) string {
	if len(targets) > 0 || len(output.SkillIDs) == 0 {
		return ""
	}
	return fmt.Sprintf("The project has no skill targets, so no copy of its %d skill(s) was installed. Agents can find them through .trailguide/_managed/generated/SKILLS.md. To install copies, set skillTargets in .trailguide/config.yaml, such as [claude, agents], then run trailguide project build.", len(output.SkillIDs))
}

// renameWarnings returns one warning for each skill output installs under the name skills.rename gives it, saying
// that only the installed SKILL.md's name changed, so text that mentions the old name still does.
func renameWarnings(output build.Output) []string {
	warnings := []string{}
	for _, skill := range output.RenamedSkills {
		warnings = append(warnings, fmt.Sprintf("%s is installed as %s. Mentions of %s in skill text aren't rewritten; check the skills that call it by that name.", skill.ID, skill.To, skill.From))
	}
	return warnings
}

// retiredCitationWarnings returns a warning for each rule an authored skill in output cites that its library
// retired. Build and sync install the skill anyway; check fails.
func retiredCitationWarnings(output build.Output) []string {
	warnings := []string{}
	for _, retired := range output.RetiredCitations {
		warnings = append(warnings, retired.Error())
	}
	return warnings
}

// clashFailure converts a *build.ClashError to its CLI failure, returning other errors unchanged.
func clashFailure(err error) error {
	var clash *build.ClashError
	if errors.As(err, &clash) {
		return failure("skill-name-clash", clash.Error(), nil)
	}
	return err
}

// skillsGuide explains the skills/ folder to people: what's theirs there, where installed copies go, and how to change
// a source's skill.
//
//go:embed skills-guide.md
var skillsGuide string

// withSkillsGuide returns files with skills/README.md, the folder's guide, added when the project at root has none, so
// a project that authors skills explains its skills/ folder. It never replaces a guide, which the project may edit.
func withSkillsGuide(ctx context.Context, root *os.Root, files []filetxn.File) ([]filetxn.File, error) {
	name := path.Join(skills.Directory, skills.Guide)
	existing, err := filetxn.ReadOptional(ctx, root, name)
	if err != nil || existing != nil {
		return files, err
	}
	return append(files, filetxn.File{Path: name, Content: []byte(skillsGuide)}), nil
}
