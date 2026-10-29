// Compose the native pipeline into offline build and read-only project checks.

package project

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// Options identifies the project root and controls rendering.
type Options struct {
	// Directory is the project root; empty means the process working directory.
	Directory   string
	ToolVersion string
	// IndexMaxLines defaults to 750 when zero. Negative limits are rejected by the renderer.
	IndexMaxLines int
	// GroupInlineMaxBytes uses the renderer's 8 KiB default when nil; zero forces summaries.
	GroupInlineMaxBytes *int
	// Home is the home directory whose personal skill folders sync, update, and check read, never write, to warn
	// about personal skills with the name of a skill the project installs. Empty reads none.
	Home string
}

// FileChanges lists sorted changed paths. Build uses generated-relative paths; sync and update prefix managed tree
// names, list rules/<group>/_group.yaml when they add an authored group's metadata, and update also lists
// config.yaml when it writes pins or exclusions.
// Empty lists mean matching tree output; Guide reports a separate managed-guide update, and Agents a change to the
// Trail Guide section of the repository's AGENTS.md.
type FileChanges struct {
	Added   []string      `json:"added"`
	Changed []string      `json:"changed"`
	Removed []string      `json:"removed"`
	Guide   *GuideChange  `json:"guide,omitempty"`
	Agents  *AgentsChange `json:"agents,omitempty"`
	// Warnings explain configuration sync and update tolerated, in source order: entries naming retired rules, and
	// sources importing a ref that isn't a library release. Then, in group order, each authored group metadata file
	// they wrote, then skills installed nowhere for want of skill targets, then, for build too, each rule an
	// authored skill cites that its library retired, which check rejects, and each use of a skill the project
	// excludes without a replacement. Last, one for each personal skill in Options.Home with the name of a skill the
	// project installs, which build doesn't report.
	Warnings []string `json:"warnings"`
	// Recovered reports that the command first recovered an interrupted earlier command, restoring or finishing
	// that command's files, which the lists above don't include.
	Recovered bool `json:"recovered"`
}

// GuideChange reports a managed-guide update separately from generated-relative file paths.
type GuideChange struct {
	Path    string `json:"path"` // Relative to the configuration directory.
	Created bool   `json:"created"`
}

// Build generates rules offline and refreshes the managed project guide and the Trail Guide section of the repository's
// AGENTS.md in one recoverable transaction. Cancellation, validation failure, or detected edits preserve existing
// output; errors return no changes.
func Build(ctx context.Context, options Options) (FileChanges, error) {
	if err := ctx.Err(); err != nil {
		return FileChanges{}, err
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		return FileChanges{}, err
	}
	defer root.Close()
	var changes FileChanges
	recovered := false
	err = filetxn.WithWriter(ctx, root, func(w *filetxn.Writer) error {
		recovered = w.Recovered()
		before, err := readProject(ctx, root)
		if err != nil {
			return err
		}
		guide, err := planProjectGuide(ctx, root)
		if err != nil {
			return err
		}
		output, err := prepareProject(ctx, before, options)
		if err != nil {
			return err
		}
		installs, err := planSkillFolders(ctx, root, before.generated, output)
		if err != nil {
			return err
		}
		agents, err := planAgentsSection(ctx, root, before.config, options)
		if err != nil {
			return err
		}
		defer agents.close()
		changes = compareFiles(treeFiles(before.generated), output.Files)
		changes.Warnings = append(retiredCitationWarnings(output), output.ExcludedUses...)
		// Build reports paths relative to _managed/generated/, three folders below the repository root.
		mergeChanges(&changes, prefixChanges(installs.changes, "../../../"))
		changes.Warnings = append(changes.Warnings, renameWarnings(output)...)
		targets := map[filetxn.Target]map[string][]byte{filetxn.Generated: output.Files}
		maps.Copy(targets, installs.targets)
		includeProjectGuide(targets, guide, &changes)
		agents.include(targets, &changes)
		return w.ApplyModes(targets, installs.executable, func() error {
			if err := requireUnchanged(ctx, root, before); err != nil {
				return err
			}
			if err := requireLinkedFilesUnchanged(ctx, before.directory, output.LinkedFiles); err != nil {
				return err
			}
			if err := agents.requireUnchanged(ctx); err != nil {
				return err
			}
			if err := requireSkillFoldersUnchanged(ctx, root, installs); err != nil {
				return err
			}
			return requireGuideUnchanged(ctx, root, guide)
		})
	})
	if err != nil {
		return FileChanges{}, err
	}
	changes.Recovered = recovered
	return changes, nil
}

// checkedProject is what a check found: how generated output and the requested files differ from what build would
// write, and each installed skill folder build would add, replace, or remove. The generated changes' warnings are the
// personal skill warnings for the skills generated/ records as installed.
type checkedProject struct {
	generated, files FileChanges
	skills           []skillFolderChange
}

// checkWithFiles checks generated output, the installed skill folders, and additional root-relative files in one
// optimistic snapshot. Missing files are reported as additions; invalid paths, unreadable files, concurrent edits, a
// folder in a skill target that Trail Guide would install into but didn't install, and a rule an authored skill cites
// that its library retired are errors.
func checkWithFiles(ctx context.Context, options Options, expected map[string][]byte) (checkedProject, error) {
	return checkProjectFiles(ctx, options, expected, nil)
}

// checkProjectFiles is checkWithFiles, also calling inspect, when not nil, with the Trail Guide directory and the
// project's configuration before it rechecks the snapshot, for checks of files outside the Trail Guide directory.
func checkProjectFiles(ctx context.Context, options Options, expected map[string][]byte, inspect func(*os.Root, rules.Configuration) error) (checkedProject, error) {
	if err := ctx.Err(); err != nil {
		return checkedProject{}, err
	}
	if err := rules.ValidatePaths(expected, nil); err != nil {
		return checkedProject{}, err
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		return checkedProject{}, err
	}
	defer root.Close()
	if err := filetxn.RequireIdle(root); err != nil {
		return checkedProject{}, err
	}
	before, err := readProject(ctx, root)
	if err != nil {
		return checkedProject{}, err
	}
	files, err := readCheckFiles(ctx, root, expected)
	if err != nil {
		return checkedProject{}, err
	}
	output, err := prepareProject(ctx, before, options)
	if err != nil {
		return checkedProject{}, err
	}
	if len(output.RetiredCitations) > 0 {
		return checkedProject{}, output.RetiredCitations[0]
	}
	installs, err := planSkillFolders(ctx, root, before.generated, output)
	if err != nil {
		return checkedProject{}, err
	}
	if inspect != nil {
		if err := inspect(root, before.config); err != nil {
			return checkedProject{}, err
		}
	}
	if err := requireCheckUnchanged(ctx, root, before, expected, files); err != nil {
		return checkedProject{}, err
	}
	if err := requireLinkedFilesUnchanged(ctx, before.directory, output.LinkedFiles); err != nil {
		return checkedProject{}, err
	}
	if err := requireSkillFoldersUnchanged(ctx, root, installs); err != nil {
		return checkedProject{}, err
	}
	generated := compareFiles(treeFiles(before.generated), output.Files)
	generated.Warnings = personalSkillWarnings(options.Home, filepath.Dir(root.Name()), treeFiles(before.generated)[build.InstalledSkillsFile])
	return checkedProject{generated: generated, files: compareFiles(files, expected), skills: installs.folders}, nil
}

// readCheckFiles retains only requested files, preserving absence as a missing map entry.
func readCheckFiles(ctx context.Context, root *os.Root, expected map[string][]byte) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range slices.Sorted(maps.Keys(expected)) {
		data, err := filetxn.ReadFile(ctx, root, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files[name] = data
	}
	return files, nil
}

// requireCheckUnchanged verifies both inventories before returning a single freshness decision.
func requireCheckUnchanged(ctx context.Context, root *os.Root, before projectState, expected, files map[string][]byte) error {
	if err := requireUnchanged(ctx, root, before); err != nil {
		return err
	}
	after, err := readCheckFiles(ctx, root, expected)
	if err != nil {
		return err
	}
	if !maps.EqualFunc(files, after, bytes.Equal) {
		return failure("concurrent-change", "checked project files changed during the operation; retry after edits finish", nil)
	}
	return filetxn.RequireIdle(root)
}

// projectState holds original bytes and inventories for optimistic change detection before writing. local is the
// rules/ folder, with files keyed by their path in it, and assets the assets/ folder, with files keyed by their path
// in it; authoredFiles combines them. skills is the skills/ folder of authored skills. lock is the
// lockfile, or nil when the project has none. vendor and generated are the managed trees where layout says the
// project keeps them.
type projectState struct {
	config                                   rules.Configuration
	configBytes                              []byte
	lock                                     []byte
	layout                                   managedLayout
	local, assets, skills, vendor, generated *filetxn.Tree
	// directory is the Trail Guide directory's path, beside the files outside it that authored skills link to.
	directory string
}

// records returns the project's records, as readRecords reads them for every command but sync, which alone moves a
// project in the legacy layout: for one, every other command fails with layout-migration-needed.
func (s projectState) records() (sourceRecords, error) {
	stored, err := readRecords(s.config, s.lock, treeFiles(s.vendor), s.layout, false, nil)
	if err == nil && s.layout.legacy {
		return sourceRecords{}, layoutMigrationNeeded()
	}
	return stored, err
}

// readProject validates configuration and retains every authored and managed byte used for change detection. It
// fails when local/ has appeared since the project opened, so the input recheck refuses it too.
func readProject(ctx context.Context, root *os.Root) (projectState, error) {
	if err := requireNoLocalFolder(ctx, root); err != nil {
		return projectState{}, err
	}
	data, config, err := configuration(ctx, root)
	if err != nil {
		return projectState{}, err
	}
	lock, err := filetxn.ReadOptional(ctx, root, lockFile)
	if err != nil {
		return projectState{}, err
	}
	layout, err := readLayout(root)
	if err != nil {
		return projectState{}, err
	}
	state := projectState{config: config, configBytes: data, lock: lock, layout: layout, directory: root.Name()}
	for _, item := range []struct {
		name  string
		value **filetxn.Tree
	}{{"rules", &state.local}, {"assets", &state.assets}, {skills.Directory, &state.skills}, {string(layout.vendor), &state.vendor}, {string(layout.generated), &state.generated}} {
		tree, err := filetxn.ReadTree(ctx, root, item.name)
		if err != nil {
			return projectState{}, err
		}
		*item.value = tree
	}
	return state, nil
}

// prepareProject verifies persisted identity, reloads native library semantics from the verified bytes, resolves,
// and renders in memory.
func prepareProject(ctx context.Context, state projectState, options Options) (build.Output, error) {
	stored, err := state.records()
	if err != nil {
		return build.Output{}, err
	}
	snapshots, err := decodeSnapshots(state.config, stored, treeFiles(state.vendor))
	if err != nil {
		return build.Output{}, err
	}
	if err := verifyExecutable(snapshots, state.vendor); err != nil {
		return build.Output{}, err
	}
	libraries := map[string]build.Library{}
	for _, source := range state.config.Sources {
		snapshot := snapshots[source.Name]
		catalog, err := loadSnapshot(ctx, source, snapshot)
		if err != nil {
			return build.Output{}, err
		}
		libraries[source.Name] = build.Library{Catalog: catalog, Snapshot: snapshot}
	}
	return renderProject(ctx, state, libraries, options)
}

// loadSnapshot validates the library content of a verified snapshot, assembling each rule from the library
// release that published it, and checks that it holds exactly the recorded rules and files besides its skills' and
// the files its delivered skills link to, which must all be there. A snapshot of a repository that isn't a library
// has an empty catalog.
func loadSnapshot(ctx context.Context, source rules.Source, snapshot snapshot) (library.Catalog, error) {
	references, err := snapshot.ReferenceFiles(source.Name, func(id string) bool { return deliversSkill(source, id) })
	if err != nil {
		return library.Catalog{}, err
	}
	ruleFiles := snapshot
	ruleFiles.Files = maps.Clone(snapshot.Files)
	maps.DeleteFunc(ruleFiles.Files, func(file string, _ []byte) bool { return snapshot.IsSkillFile(file) })
	if snapshot.NonLibrary {
		for _, file := range slices.Sorted(maps.Keys(ruleFiles.Files)) {
			if !references[file] {
				return library.Catalog{}, invalidSnapshot(librarytree.Vendor+"/"+source.Name+"/"+file, "belongs to no installed skill, and no installed skill links to it; run trailguide project sync")
			}
		}
		return library.Catalog{Rules: []string{}, Groups: []library.Group{}, SupportingFiles: map[string][]byte{}}, nil
	}
	snapshot = ruleFiles
	// An individually selected rule the snapshot doesn't import was retired when it was recorded.
	individual := []string{}
	for _, id := range source.Rules {
		if _, ok := snapshot.Rules[id]; ok {
			individual = append(individual, id)
		}
	}
	catalog, err := library.LoadSource(ctx, snapshot.Source(), source.Name, source.Groups, individual)
	if err != nil {
		return library.Catalog{}, err
	}
	// A file the skills link to is in the catalog too when the library's rules use it.
	loaded := catalog.Files()
	maps.DeleteFunc(snapshot.Files, func(file string, _ []byte) bool {
		_, inCatalog := loaded[file]
		return references[file] && !inCatalog
	})
	if err := verifyLoadedSnapshot(source, catalog, snapshot); err != nil {
		return library.Catalog{}, err
	}
	return catalog, nil
}

// renderProject resolves local definitions and renders the same output for offline builds and sync.
func renderProject(ctx context.Context, state projectState, libraries map[string]build.Library, options Options) (build.Output, error) {
	if err := ctx.Err(); err != nil {
		return build.Output{}, err
	}
	authored, err := authoredFiles(state)
	if err != nil {
		return build.Output{}, err
	}
	local, err := authoredSkills(state.skills)
	if err != nil {
		return build.Output{}, err
	}
	output, err := build.Generate(state.config, libraries, authored, build.Options{IndexMaxLines: options.IndexMaxLines, GroupInlineMaxBytes: options.GroupInlineMaxBytes, LocalSkills: local})
	if err != nil {
		return build.Output{}, clashFailure(err)
	}
	// Installed copies' links point at the files the project's skills name, so those must be there after the write.
	if output.LinkedFiles, err = requireAuthoredSkillReferences(ctx, state, output); err != nil {
		return build.Output{}, err
	}
	if err := ctx.Err(); err != nil {
		return build.Output{}, err
	}
	return output, nil
}

// requireUnchanged compares exact input/output identities immediately before a write or final check result.
func requireUnchanged(ctx context.Context, root *os.Root, before projectState) error {
	after, err := readProject(ctx, root)
	if err != nil {
		return err
	}
	if !sameProject(before, after) {
		return failure("concurrent-change", "project inputs or managed output changed during the operation; retry after edits finish", nil)
	}
	return nil
}

// sameProject reports whether two reads hold the same layout, configuration and lockfile bytes, and rules, assets,
// skills, vendor, and generated trees.
func sameProject(a, b projectState) bool {
	return a.layout == b.layout && bytes.Equal(a.configBytes, b.configBytes) && (a.lock == nil) == (b.lock == nil) && bytes.Equal(a.lock, b.lock) && a.local.Digest() == b.local.Digest() && a.assets.Digest() == b.assets.Digest() && a.skills.Digest() == b.skills.Digest() && a.vendor.Digest() == b.vendor.Digest() && a.generated.Digest() == b.generated.Digest()
}

// authoredFiles returns the project's authored files keyed as build.Generate takes them: each file in rules/ by its
// path there, and each file in assets/ as assets/<path>. It fails for a file in rules/assets/, which would take a
// shared asset's key: shared assets belong in assets/.
func authoredFiles(state projectState) (map[string][]byte, error) {
	files := map[string][]byte{}
	for name, data := range treeFiles(state.local) {
		if strings.HasPrefix(name, "assets/") {
			return nil, &ValidationError{Location: "rules/" + name, Problem: "rules/ holds groups under techs/ and practices/, so shared assets belong in .trailguide/assets/; move this file there"}
		}
		files[name] = data
	}
	for name, data := range treeFiles(state.assets) {
		files["assets/"+name] = data
	}
	return files, nil
}

// treeFiles projects original bytes while treating an absent directory as an empty inventory.
func treeFiles(tree *filetxn.Tree) map[string][]byte {
	if tree == nil {
		return nil
	}
	return tree.Files
}

// compareFiles produces deterministic byte-level changes without treating timestamp changes as output changes.
func compareFiles(before, after map[string][]byte) FileChanges {
	changes := FileChanges{Added: []string{}, Changed: []string{}, Removed: []string{}, Warnings: []string{}}
	for _, name := range slices.Sorted(maps.Keys(after)) {
		data, ok := before[name]
		if !ok {
			changes.Added = append(changes.Added, name)
		} else if !bytes.Equal(data, after[name]) {
			changes.Changed = append(changes.Changed, name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(before)) {
		if _, ok := after[name]; !ok {
			changes.Removed = append(changes.Removed, name)
		}
	}
	return changes
}

// verifyLoadedSnapshot requires the loaded catalog to hold exactly the snapshot's recorded groups, rules, and
// files, each stored where the snapshot's rule versions place it, with unchanged bytes.
func verifyLoadedSnapshot(source rules.Source, catalog library.Catalog, snapshot snapshot) error {
	groups := []string{}
	loaded := []string{}
	for _, group := range catalog.Groups {
		if source.Groups.Includes(group.ID) {
			groups = append(groups, group.ID)
		}
		for _, rule := range group.Rules {
			loaded = append(loaded, strings.TrimSuffix(rule.Path, ".md"))
		}
	}
	slices.Sort(loaded)
	if !slices.Equal(loaded, slices.Sorted(maps.Keys(snapshot.Rules))) {
		return failure("invalid-snapshot", source.Name+": the snapshot's rule files differ from the rules its record lists; run trailguide project sync", nil)
	}
	files := catalog.Files()
	if !slices.Equal(groups, snapshot.Groups) || !slices.Equal(slices.Sorted(maps.Keys(files)), slices.Sorted(maps.Keys(snapshot.Files))) {
		return failure("invalid-snapshot", source.Name+": recorded groups or inventory differ from the selected library; run trailguide project sync", nil)
	}
	for _, file := range slices.Sorted(maps.Keys(files)) {
		if !bytes.Equal(files[file], snapshot.Files[file]) {
			return failure("concurrent-change", source.Name+":"+file+": loaded content differs from verified snapshot bytes; retry after edits finish", nil)
		}
	}
	return nil
}

// deliversSkill reports whether the project delivers source's installed skill id, which it doesn't when its
// configuration excludes or forks it.
func deliversSkill(source rules.Source, id string) bool {
	_, excluded := source.SkillExclude[id]
	_, forked := source.SkillForks[id]
	return !excluded && !forked
}
