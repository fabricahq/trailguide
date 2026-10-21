// Adapt managed directories, the two project-guide filenames, the configuration, the authored files under rules/,
// and the installed skill folders in the repository's skill targets that sync and update write to the same
// recoverable transaction.

package filetxn

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// GuideReadme and GuideStandalone are managed files, not permission to replace a general project README.
// Callers must establish guide ownership and recheck its original bytes before publication.
const (
	GuideReadme     Target = "README.md"
	GuideStandalone Target = "TRAILGUIDE.md"
)

// Config is the project configuration. Only project update replaces it, together with vendor and generated output,
// so its pins and exclusions and the output they select are installed or recovered together.
const Config Target = "config.yaml"

// LocalGroupMetadata is the metadata file of the authored group with ID group, rules/<group>/_group.yaml; group must
// be a valid group ID. Sync and update create it together with vendor and generated output when the project's
// authored rules in the group lose the only imported copy of its metadata, so recovery keeps the two consistent. Its
// directory must already exist.
func LocalGroupMetadata(group string) Target { return Target("rules/" + group + "/_group.yaml") }

// maxLocalGroupMetadata bounds the local group metadata files one transaction can create, so its journal stays
// small enough to read during recovery.
const maxLocalGroupMetadata = 1000

// LocalRule is the Markdown file of the authored rule at file, a rule path relative to rules/, such as
// techs/go/errors.md, and LocalRuleAssets is that rule's asset directory, rules/techs/go/assets/errors. Only
// project update replaces them, when it replaces a fork with a newer one, so recovery restores or finishes the rule,
// its assets, the configuration that records the fork's version, and the output generated from them together. A
// nil file map for LocalRuleAssets removes the directory. Apply creates their missing parent directories.
func LocalRule(file string) Target { return Target("rules/" + file) }

// LocalRuleAssets is the asset directory of the local rule at file; see LocalRule.
func LocalRuleAssets(file string) Target {
	return Target("rules/" + strings.TrimSuffix(rules.RuleAssetDirectory(file), "/"))
}

// maxLocalRules bounds the local rules one transaction can replace, each with its asset directory, so its journal
// stays small enough to read during recovery.
const maxLocalRules = 1000

// AuthoredSkill is the folder of the authored skill named name, skills/<name>, a fork of a source's skill. Only
// project update replaces it, when it merges the source's changes into the fork, so recovery restores or finishes
// the fork, the configuration that records the commit it's based on, and the output generated from them together.
func AuthoredSkill(name string) Target { return Target(skills.Directory + "/" + name) }

// maxAuthoredSkills bounds the authored skill folders one transaction can replace, so its journal stays small enough
// to read during recovery.
const maxAuthoredSkills = 1000

// authoredSkillTarget reports whether target is an AuthoredSkill target of a valid skill name.
func authoredSkillTarget(target Target) bool {
	name, authored := strings.CutPrefix(string(target), skills.Directory+"/")
	return authored && skills.ValidName(name)
}

// AgentsFile is the repository's AGENTS.md, beside the Trail Guide directory, which holds a section Trail Guide maintains.
// Init, build, sync, and update replace it together with their other output, so recovery restores or finishes it with
// them. Callers must establish that the file's text outside the section is unchanged.
const AgentsFile Target = "../AGENTS.md"

// SkillFolder is the installed skill folder at folder, a path relative to the repository root, the parent of the
// Trail Guide directory: a skill target's folder and an install name, such as .claude/skills/build-feature. Sync,
// update, and build replace or, with a nil file map, remove it together with generated output, so recovery restores
// or finishes them together. Apply creates its missing parent directories. Callers must establish that Trail Guide
// installed an existing folder before replacing or removing it.
func SkillFolder(folder string) Target { return Target(skillPrefix + folder) }

// skillPrefix begins every SkillFolder target, whose folder is outside the Trail Guide directory.
const skillPrefix = "../"

// maxSkillFolders bounds the skill folders one transaction can replace or remove, so its journal stays small enough
// to read during recovery.
const maxSkillFolders = 2000

// skillFolderTarget returns the repository-relative folder of a SkillFolder target, reporting whether target is
// one: a skill target's folder and a valid install name.
func skillFolderTarget(target Target) (string, bool) {
	folder, ok := strings.CutPrefix(string(target), skillPrefix)
	if !ok || !skills.ValidName(path.Base(folder)) {
		return "", false
	}
	for _, parent := range rules.SkillTargetFolders {
		if path.Dir(folder) == parent {
			return folder, true
		}
	}
	return "", false
}

func guideTarget(target Target) bool { return target == GuideReadme || target == GuideStandalone }

// localGroupMetadataTarget reports whether target is a LocalGroupMetadata target of a valid group ID.
func localGroupMetadataTarget(target Target) bool {
	group, authored := strings.CutPrefix(string(target), "rules/")
	group, metadata := strings.CutSuffix(group, "/_group.yaml")
	return authored && metadata && librarytree.ValidateGroupID(group, "group") == nil
}

// localRuleTarget reports whether target is a LocalRule target: the Markdown file of a rule path under rules/.
func localRuleTarget(target Target) bool {
	file, authored := strings.CutPrefix(string(target), "rules/")
	id, versioned := librarytree.VersionedRule(file)
	return authored && versioned && file == id+".md"
}

// localRuleAssetsTarget reports whether target is a LocalRuleAssets target: the asset directory of a rule path
// under rules/.
func localRuleAssetsTarget(target Target) bool {
	directory, authored := strings.CutPrefix(string(target), "rules/")
	parent := path.Dir(directory)
	if !authored || path.Base(parent) != "assets" {
		return false
	}
	file := path.Dir(parent) + "/" + path.Base(directory) + ".md"
	return localRuleTarget(LocalRule(file)) && LocalRuleAssets(file) == target
}

// localTarget reports whether target is an authored file or directory under rules/ or skills/, which follows the
// fixed targets in a transaction.
func localTarget(target Target) bool {
	return localGroupMetadataTarget(target) || localRuleTarget(target) || localRuleAssetsTarget(target) || authoredSkillTarget(target)
}

// fileTarget reports whether target is a single file rather than a directory tree.
func fileTarget(target Target) bool {
	return guideTarget(target) || target == Config || target == AgentsFile || localGroupMetadataTarget(target) || localRuleTarget(target)
}

func validTarget(target Target) bool {
	_, skill := skillFolderTarget(target)
	return target == Vendor || target == Generated || fileTarget(target) || localRuleAssetsTarget(target) || authoredSkillTarget(target) || skill
}

// removable reports whether a nil file map may remove target: a local rule's asset directory or a skill folder.
func removable(target Target) bool {
	_, skill := skillFolderTarget(target)
	return localRuleAssetsTarget(target) || skill
}

// place is where a target and its transaction entries are: the root that holds them, the target's path in it, and
// the transaction directory's path in it. Rename publishes within one root, so a skill folder or AGENTS.md, which are
// outside the Trail Guide directory, use the repository root for their staged output and backup too. file reports a
// file target in the repository root, which renames publish without replacing a file that appeared meanwhile.
type place struct {
	root *os.Root
	live string
	txn  string
	file bool
}

// entry returns the path in p.root of the transaction entry named prefix plus target's entry name, such as its
// staged output, new-<name>, or its backup.
func (p place) entry(prefix string, target Target) string {
	return path.Join(p.txn, prefix+entryName(target))
}

// roots resolves targets to places. project is the Trail Guide directory; repository, its parent, is opened when a
// skill folder or AGENTS.md first needs it, and close releases it.
type roots struct {
	project    *os.Root
	repository *os.Root
}

// place returns where target is.
func (r *roots) place(target Target) (place, error) {
	folder, skill := skillFolderTarget(target)
	if target == AgentsFile {
		folder = strings.TrimPrefix(string(target), skillPrefix)
	} else if !skill {
		return place{r.project, string(target), transactionName, false}, nil
	}
	if r.repository == nil {
		repository, err := os.OpenRoot(filepath.Dir(r.project.Name()))
		if err != nil {
			return place{}, fmt.Errorf("open the repository root for %s: %w", folder, err)
		}
		r.repository = repository
	}
	return place{r.repository, folder, path.Join(filepath.Base(r.project.Name()), transactionName), !skill}, nil
}

// close releases the repository root when place opened it.
func (r *roots) close() error {
	if r.repository == nil {
		return nil
	}
	return r.repository.Close()
}

// requireRealParents fails with unsafe-path when a parent directory of live, a target's path in root, such as
// rules/techs for authored group metadata, is a symbolic link or not a directory, so publication and recovery never
// reach another file through it. A missing parent passes: the target is absent.
func requireRealParents(root *os.Root, live string) error {
	parts := strings.Split(live, "/")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[:i], "/")
		info, err := root.Lstat(parent)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return failure("unsafe-path", parent+": expected a directory without symlinks", nil)
		}
	}
	return nil
}

// entryName names target's staged output, backup, and displaced copy inside the transaction directory, which holds
// no subdirectories for them: the slashes of a path under rules/ become +, which no target contains.
func entryName(target Target) string { return strings.ReplaceAll(string(target), "/", "+") }

// renameCheck is where Apply probes that this filesystem can publish a file target without replacing another file.
var renameCheck = path.Join(transactionName, "rename-check")

// readTarget represents a managed file as a one-file inventory with a stable key across staging and recovery.
func readTarget(ctx context.Context, root *os.Root, target Target, location string) (*Tree, error) {
	if !fileTarget(target) {
		return ReadTree(ctx, root, location)
	}
	data, err := ReadOptional(ctx, root, location)
	if err != nil || data == nil {
		return nil, err
	}
	return &Tree{Files: map[string][]byte{string(target): data}}, nil
}

// writeTarget creates staged output exclusively; file payloads must contain exactly their declared filename. A
// tree's executable files are those executable lists.
func writeTarget(ctx context.Context, root *os.Root, target Target, location string, files map[string][]byte, executable []string) error {
	if !fileTarget(target) {
		return writeTree(ctx, root, location, files, executable)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, ok := files[string(target)]
	if !ok || len(files) != 1 {
		return failure("invalid-target", fmt.Sprintf("%s: expected exactly one matching file", target), nil)
	}
	file, err := root.OpenFile(location, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

// renameManaged publishes file targets, and probes publishing them, without replacing a file created after the last
// validation. Parent descriptors come from os.Root so the platform rename retains root confinement.
func renameManaged(root *os.Root, from, to string) error {
	return renameManagedWith(root, from, to, renameExclusive)
}

// renameManagedWith supplies the platform boundary for filesystem capability tests.
func renameManagedWith(root *os.Root, from, to string, exclusive func(int, string, int, string) error) error {
	if !fileTarget(Target(to)) && path.Dir(to) != renameCheck {
		return root.Rename(from, to)
	}
	return renameWithout(root, from, to, exclusive)
}

// renameWithout renames from to to, both in root, through exclusive, which fails rather than replace a file at to.
func renameWithout(root *os.Root, from, to string, exclusive func(int, string, int, string) error) error {
	source, err := root.Open(path.Dir(from))
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := root.Open(path.Dir(to))
	if err != nil {
		return err
	}
	defer destination.Close()
	return exclusive(int(source.Fd()), path.Base(from), int(destination.Fd()), path.Base(to))
}

// probeFileRename checks actual staged bytes on this filesystem before any live output is displaced.
// Hard-link fallbacks are unsuitable: interrupted link/unlink pairs violate recovery's no-hard-link policy.
func (w *Writer) probeFileRename(target Target, staged string) error {
	if err := w.root.Mkdir(renameCheck, 0700); err != nil {
		return err
	}
	destination := path.Join(renameCheck, entryName(target))
	if err := w.rename(staged, destination); err != nil {
		return fmt.Errorf("cannot safely publish %s on this filesystem; existing files were not changed: %w", target, err)
	}
	if err := w.root.Rename(destination, staged); err != nil {
		return err
	}
	return w.root.Remove(renameCheck)
}
