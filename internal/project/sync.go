// Fetch libraries and refresh vendor, generated output, and the managed guide under one recoverable writer.

package project

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

// Sync imports the rule versions each source's snapshot records, choosing versions only where configuration asks
// for something a snapshot doesn't have, then validates and renders everything before replacing managed trees.
// Git settings are trusted caller options. Any failure returns no change report, and, unless a rollback left files
// to recover, an *UnchangedError. Authored files stay untouched,
// except that it adds the metadata of a group whose local rules would otherwise lose it, as install does.
func Sync(ctx context.Context, options Options, git imports.Options) (FileChanges, error) {
	// recovered reports that the writer first recovered an interrupted earlier command, which changed files.
	recovered := false
	if err := ctx.Err(); err != nil {
		return FileChanges{}, unchanged(err, recovered)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		return FileChanges{}, unchanged(err, recovered)
	}
	defer root.Close()
	var changes FileChanges
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
		recorded, err := recordedSnapshots(before.config, treeFiles(before.vendor))
		if err != nil {
			return err
		}
		git.GroupMetadata = groupsWithoutLocalMetadata(before)
		imported, err := imports.ImportLibraries(ctx, before.config, recorded, git)
		if err != nil {
			return err
		}
		changes, err = install(ctx, root, w, before, installation{guide: guide, config: before.config, imported: imported, options: options})
		return err
	})
	if err != nil {
		return FileChanges{}, unchanged(err, recovered)
	}
	changes.Recovered = recovered
	return changes, nil
}

// installation is what install writes. guide is the managed guide as planned under the writer. edited is nil unless
// config is an edited configuration whose bytes install writes to config.yaml. imported holds every source of config.
type installation struct {
	guide    *filetxn.File
	config   rules.Configuration
	edited   []byte
	imported map[string]imports.Library
	options  Options
	// forks are the forks an update replaces, each with its files read, and skillForks the skill forks it merges the
	// source's changes into, each with its merged files.
	forks      []forkUpdate
	skillForks []skillForkUpdate
}

// install renders the project from the installation's imported sources and replaces vendor and generated output,
// an older managed guide, config.yaml when edited, and the authored rule and asset directory of each fork it
// replaces, in one transaction of w. When authored rules would lose the only imported copy of their group's metadata,
// the transaction also writes that copy to rules/<group>/_group.yaml, with a warning. before is the project as read
// under w; any change to it before replacement fails with concurrent-change. The report lists vendor and generated
// paths, config.yaml when edited, authored group metadata it adds, and the authored files of each replaced fork. The
// transaction also refreshes the Trail Guide section of the repository's AGENTS.md, which the report lists separately.
func install(ctx context.Context, root *os.Root, w *filetxn.Writer, before projectState, in installation) (FileChanges, error) {
	snapshots := map[string]snapshot{}
	libraries := map[string]build.Library{}
	warnings := []string{}
	for _, source := range in.config.Sources {
		item := in.imported[source.Name]
		snapshots[source.Name] = item.Snapshot
		libraries[source.Name] = build.Library{Catalog: item.Catalog, Snapshot: item.Snapshot}
		warnings = append(warnings, item.Warnings...)
	}
	vendor, err := encodeSnapshots(in.config, snapshots)
	if err != nil {
		return FileChanges{}, err
	}
	kept, err := keptGroupMetadata(in.config, before, in.imported)
	if err != nil {
		return FileChanges{}, err
	}
	state := before
	state.config = in.config
	if len(kept)+len(in.forks) > 0 {
		local := maps.Clone(treeFiles(before.local))
		if local == nil {
			local = map[string][]byte{}
		}
		for group, data := range kept {
			local[group+"/_group.yaml"] = data
		}
		for _, fork := range in.forks {
			for name := range forkPaths(local, fork.file) {
				delete(local, name)
			}
			maps.Copy(local, fork.files)
		}
		state.local = &filetxn.Tree{Files: local}
	}
	if len(in.skillForks) > 0 {
		state.skills = mergedSkills(before.skills, in.skillForks)
	}
	output, err := renderProject(ctx, state, libraries, in.options)
	if err != nil {
		return FileChanges{}, err
	}
	installs, err := planSkillFolders(ctx, root, before.generated, output)
	if err != nil {
		return FileChanges{}, err
	}
	agents, err := planAgentsSection(ctx, root, in.config, in.options)
	if err != nil {
		return FileChanges{}, err
	}
	defer agents.close()
	changes := compareFiles(managedFiles(treeFiles(before.vendor), treeFiles(before.generated)), managedFiles(vendor, output.Files))
	// The report's paths are relative to the Trail Guide directory, one folder below the repository root.
	mergeChanges(&changes, prefixChanges(installs.changes, "../"))
	targets := map[filetxn.Target]map[string][]byte{filetxn.Vendor: vendor, filetxn.Generated: output.Files}
	maps.Copy(targets, installs.targets)
	executable := maps.Clone(installs.executable)
	executable[filetxn.Vendor] = vendorExecutable(snapshots)
	for _, group := range slices.Sorted(maps.Keys(kept)) {
		file := path.Join("rules", group, "_group.yaml")
		targets[filetxn.LocalGroupMetadata(group)] = map[string][]byte{file: kept[group]}
		changes.Added = append(changes.Added, file)
		warnings = append(warnings, fmt.Sprintf("Wrote %s, the metadata of group %s from the library that last supplied it, because your authored rules in the group need it and no imported rule supplies it anymore. It's now yours to edit.", file, group))
	}
	for _, fork := range in.forks {
		forkChanges := compareFiles(rulesPaths(forkPaths(treeFiles(before.local), fork.file)), rulesPaths(fork.files))
		changes.Added = append(changes.Added, forkChanges.Added...)
		changes.Changed = append(changes.Changed, forkChanges.Changed...)
		changes.Removed = append(changes.Removed, forkChanges.Removed...)
		assets := rules.RuleAssetDirectory(fork.file)
		var assetFiles map[string][]byte
		for name, data := range fork.files {
			if relative, ok := strings.CutPrefix(name, assets); ok {
				if assetFiles == nil {
					assetFiles = map[string][]byte{}
				}
				assetFiles[relative] = data
			}
		}
		targets[filetxn.LocalRule(fork.file)] = map[string][]byte{path.Join("rules", fork.file): fork.files[fork.file]}
		// A nil map removes the old asset directory when the new fork has no assets.
		targets[filetxn.LocalRuleAssets(fork.file)] = assetFiles
	}
	for _, fork := range in.skillForks {
		folder := skills.AuthoredID(fork.name)
		previous := map[string][]byte{}
		for name, data := range treeFiles(before.skills) {
			if relative, ok := strings.CutPrefix(name, fork.name+"/"); ok {
				previous[path.Join(folder, relative)] = data
			}
		}
		merged := map[string][]byte{}
		for name, data := range fork.files.Files {
			merged[path.Join(folder, name)] = data
		}
		forkChanges := compareFiles(previous, merged)
		changes.Added = append(changes.Added, forkChanges.Added...)
		changes.Changed = append(changes.Changed, forkChanges.Changed...)
		changes.Removed = append(changes.Removed, forkChanges.Removed...)
		targets[filetxn.AuthoredSkill(fork.name)] = fork.files.Files
		executable[filetxn.AuthoredSkill(fork.name)] = fork.files.Executable
	}
	slices.Sort(changes.Added)
	slices.Sort(changes.Changed)
	slices.Sort(changes.Removed)
	if warning := noSkillTargetsWarning(output, in.config.SkillTargets); warning != "" {
		warnings = append(warnings, warning)
	}
	warnings = append(warnings, retiredCitationWarnings(output)...)
	warnings = append(warnings, output.ExcludedUses...)
	warnings = append(warnings, personalSkillWarnings(in.options.Home, filepath.Dir(root.Name()), output.Files[build.InstalledSkillsFile])...)
	warnings = append(warnings, renameWarnings(output)...)
	changes.Warnings = warnings
	if in.edited != nil {
		targets[filetxn.Config] = map[string][]byte{configurationFile: in.edited}
		changes.Changed = append(changes.Changed, configurationFile)
		slices.Sort(changes.Changed)
	}
	includeProjectGuide(targets, in.guide, &changes)
	agents.include(targets, &changes)
	err = w.ApplyModes(targets, executable, func() error {
		if err := requireUnchanged(ctx, root, before); err != nil {
			return err
		}
		if err := agents.requireUnchanged(ctx); err != nil {
			return err
		}
		if err := requireSkillFoldersUnchanged(ctx, root, installs); err != nil {
			return err
		}
		return requireGuideUnchanged(ctx, root, in.guide)
	})
	if err != nil {
		return FileChanges{}, err
	}
	return changes, nil
}

// mergedSkills returns authored, the project's authored skills, with the folder of each of forks holding its merged
// files instead.
func mergedSkills(authored *filetxn.Tree, forks []skillForkUpdate) *filetxn.Tree {
	result := &filetxn.Tree{Files: map[string][]byte{}, Directories: []string{}, Executable: []string{}}
	if authored != nil {
		maps.Copy(result.Files, authored.Files)
		result.Directories = slices.Clone(authored.Directories)
		result.Executable = slices.Clone(authored.Executable)
	}
	for _, fork := range forks {
		prefix := fork.name + "/"
		inFork := func(name string) bool { return strings.HasPrefix(name, prefix) }
		maps.DeleteFunc(result.Files, func(name string, _ []byte) bool { return inFork(name) })
		result.Directories = slices.DeleteFunc(result.Directories, inFork)
		result.Executable = slices.DeleteFunc(result.Executable, inFork)
		for name, data := range fork.files.Files {
			result.Files[prefix+name] = data
			for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
				result.Directories = append(result.Directories, prefix+parent)
			}
		}
		for _, name := range fork.files.Executable {
			result.Executable = append(result.Executable, prefix+name)
		}
	}
	slices.Sort(result.Directories)
	result.Directories = slices.Compact(result.Directories)
	slices.Sort(result.Executable)
	return result
}

// forkPaths returns the files of local, by path relative to rules/, that belong to the authored rule at file: the
// rule and every file in its asset directory.
func forkPaths(local map[string][]byte, file string) map[string][]byte {
	assets := rules.RuleAssetDirectory(file)
	paths := map[string][]byte{}
	for name, data := range local {
		if name == file || strings.HasPrefix(name, assets) {
			paths[name] = data
		}
	}
	return paths
}

// rulesPaths returns files, which are keyed by path relative to rules/, keyed instead by path relative to the Code
// Rules directory.
func rulesPaths(files map[string][]byte) map[string][]byte {
	result := map[string][]byte{}
	for name, data := range files {
		result[path.Join("rules", name)] = data
	}
	return result
}

// groupsWithoutLocalMetadata returns, sorted, the groups of the project's authored rules that have no authored
// metadata.
func groupsWithoutLocalMetadata(state projectState) []string {
	local := treeFiles(state.local)
	groups := []string{}
	for file := range local {
		id, versioned := librarytree.VersionedRule(file)
		if !versioned || file != id+".md" {
			continue
		}
		group := strings.Join(strings.SplitN(id, "/", 3)[:2], "/")
		if _, ok := local[group+"/_group.yaml"]; !ok && !slices.Contains(groups, group) {
			groups = append(groups, group)
		}
	}
	slices.Sort(groups)
	return groups
}

// keptGroupMetadata returns, by group ID, the metadata to write to rules/ for each group whose authored rules would
// otherwise have none: the project has no authored metadata for it, no source now imports it, and a stored source
// record in vendor/ lists the group's metadata, including the record of a source the configuration no longer has.
// Each copy comes from the first such record: configured sources in configuration order, then removed sources in
// name order. A configured source's copy is the metadata in the library release that now supplies its shared
// files, from imported, when that release still has the group; otherwise, and for a removed source, it is the
// vendored copy. Records it can't read are skipped. It is empty, never nil, when no group needs one. It fails when
// a record was changed outside sync, or the vendored copy it would use differs from its record's checksum, so
// modified metadata never becomes authored guidance.
func keptGroupMetadata(config rules.Configuration, before projectState, imported map[string]imports.Library) (map[string][]byte, error) {
	supplied := map[string]bool{}
	for _, source := range config.Sources {
		for _, group := range imported[source.Name].Catalog.Groups {
			supplied[group.ID] = true
		}
	}
	needed := slices.DeleteFunc(groupsWithoutLocalMetadata(before), func(group string) bool { return supplied[group] })
	kept := map[string][]byte{}
	if len(needed) == 0 {
		return kept, nil
	}
	vendored := treeFiles(before.vendor)
	records := storedRecords(before.config, vendored)
	for _, group := range needed {
		for _, record := range records {
			// Nothing a record changed outside sync lists can be trusted, so one that holds a copy of the group's
			// metadata refuses; one that doesn't is never used.
			if record.changed != nil {
				if _, holds := vendored[record.name+"/"+librarytree.SnapshotPath(group+"/_group.yaml")]; holds {
					return nil, record.changed
				}
				continue
			}
			recorded, listed := record.digests[group+"/_group.yaml"]
			if !listed {
				continue
			}
			if current, ok := imported[record.name].GroupMetadata[group]; ok {
				kept[group] = current
				break
			}
			file := record.name + "/" + librarytree.SnapshotPath(group+"/_group.yaml")
			data, ok := vendored[file]
			if !ok || digest(data) != recorded {
				return nil, invalidSnapshot("vendor/"+file, fmt.Sprintf("missing or modified since sync imported it, so it can't become rules/%s/_group.yaml, the metadata your authored rules in the group need. Restore it: run trailguide project sync with the previous configuration, then change the configuration and sync again; or write rules/%s/_group.yaml yourself", group, group))
			}
			kept[group] = data
			break
		}
	}
	return kept, nil
}

// storedRecord is a source record in vendor/, named by its source. changed is the refusal of a record changed
// outside sync, which is then left unread, or nil for a valid record.
type storedRecord struct {
	name string
	parsedRecord
	changed error
}

// storedRecords returns every source record in vendor that is valid or changed outside sync: those of config's
// sources in configuration order, then those of sources config no longer has, in name order. Records that don't
// parse otherwise are left out.
func storedRecords(config rules.Configuration, vendored map[string][]byte) []storedRecord {
	names := []string{}
	for _, source := range config.Sources {
		names = append(names, source.Name)
	}
	removed := []string{}
	for file := range vendored {
		if name, ok := strings.CutSuffix(file, "/_source.json"); ok && !strings.Contains(name, "/") && !slices.Contains(names, name) {
			removed = append(removed, name)
		}
	}
	slices.Sort(removed)
	records := []storedRecord{}
	for _, name := range append(names, removed...) {
		data, ok := vendored[name+"/_source.json"]
		if !ok {
			continue
		}
		record, err := parseSourceRecord(data, name)
		switch {
		case isChangedOutsideSync(err, name):
			records = append(records, storedRecord{name: name, changed: err})
		case err == nil:
			records = append(records, storedRecord{name: name, parsedRecord: record})
		}
	}
	return records
}

// managedFiles prefixes source-relative and generated-relative paths for an unambiguous combined change report.
func managedFiles(vendor, generated map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(vendor)+len(generated))
	for name, data := range vendor {
		result["vendor/"+name] = data
	}
	for name, data := range generated {
		result["generated/"+name] = data
	}
	return result
}
