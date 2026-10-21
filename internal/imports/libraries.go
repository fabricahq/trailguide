// Package imports fetches library sources and preserves their original bytes with verified Git provenance.
// Sync and update each resolve a source into a plan of rule versions and commits, and one operation imports any
// plan, returning no partial result. Git revisions and temporary repositories stay private, and Git runs only
// through gitexec's isolated mode.
package imports

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
)

// Library keeps the parsed catalog and its byte-preserving, verified source snapshot together.
type Library struct {
	Catalog  library.Catalog  `json:"catalog"`
	Snapshot library.Snapshot `json:"snapshot"`
	// Warnings explain configuration the import tolerated: entries naming retired rules, and a ref that isn't a
	// library release. It is empty, never nil, when there are none.
	Warnings []string `json:"warnings"`
	// GroupMetadata maps each group Options.GroupMetadata names that Catalog lacks to its metadata's original bytes
	// in the revision that supplied the library-wide files, when that revision has it. It is empty, never nil, when
	// there is none; Snapshot doesn't record it.
	GroupMetadata map[string][]byte `json:"-"`
}

// ImportLibraries imports every configured source or returns no partial result. recorded holds each source's
// snapshot from the last sync, without files, keyed by source name; its versions are kept, and versions are
// chosen only where configuration asks for something a snapshot doesn't have. A source without a ref follows library
// releases, keeping the recorded one, as configuration did before Trail Guide required a ref; project commands refuse
// such a source before importing it. It never installs files in a consuming project. Source temporary state is closed
// on every path.
func ImportLibraries(ctx context.Context, configuration rules.Configuration, recorded map[string]library.Snapshot, options Options) (map[string]Library, error) {
	return importSources(ctx, configuration, options, curating{recorded: recorded}, func(ctx context.Context, repo *repository, source rules.Source) (sourcePlan, error) {
		return planSource(ctx, repo, source, recordedSnapshot(recorded, source.Name))
	})
}

// resolver chooses what one source imports, reading its library through repo only when a choice needs it.
type resolver func(ctx context.Context, repo *repository, source rules.Source) (sourcePlan, error)

// importSources imports every source of configuration as resolve plans it, then the skills each library source
// curates, at the revisions curate chooses, or returns no partial result.
func importSources(ctx context.Context, configuration rules.Configuration, options Options, curate curating, resolve resolver) (map[string]Library, error) {
	if err := ctx.Err(); err != nil {
		return nil, gitexec.ContextFailure(err)
	}
	result := make(map[string]Library, len(configuration.Sources))
	for _, source := range configuration.Sources {
		if _, exists := result[source.Name]; exists {
			return nil, fail("invalid-configuration", "Duplicate source alias.", nil)
		}
		imported, err := importLibrary(ctx, source, options, resolve)
		if err != nil {
			return nil, sourceError(source.Name, err)
		}
		result[source.Name] = imported
	}
	// Curating comes second, so a project source that declares a curated repository has its revision chosen.
	for _, source := range configuration.Sources {
		imported := result[source.Name]
		if err := curateSkills(ctx, configuration, source, &imported, result, curate, options); err != nil {
			return nil, sourceError(source.Name, err)
		}
		result[source.Name] = imported
	}
	return result, nil
}

// quote returns text as a JSON string.
func quote(text string) json.RawMessage {
	encoded, _ := json.Marshal(text)
	return encoded
}

// recordedSnapshot returns the snapshot recorded holds for a source, or nil when it holds none.
func recordedSnapshot(recorded map[string]library.Snapshot, source string) *library.Snapshot {
	if snapshot, ok := recorded[source]; ok {
		return &snapshot
	}
	return nil
}

// importLibrary applies one deadline across resolving what source imports, fetching, blob reads, validation, and
// snapshot construction.
func importLibrary(ctx context.Context, source rules.Source, options Options, resolve resolver) (_ Library, err error) {
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return Library{}, err
	}
	defer cancel()
	repo, err := openRepository(ctx, source, options)
	if err != nil {
		return Library{}, err
	}
	// Joining only a failed Close keeps err itself, so callers still see its identity, such as a validation error.
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	plan, err := resolve(ctx, repo, source)
	if err != nil {
		return Library{}, err
	}
	return repo.importPlan(ctx, source, plan, options.GroupMetadata)
}

// withTimeout returns ctx bounded by the options' import timeout, 120 seconds when it is zero, and fails with code
// invalid-options when it is negative. The caller owns the returned cancel function on success.
func withTimeout(ctx context.Context, options Options) (context.Context, context.CancelFunc, error) {
	timeout := options.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if timeout < 0 {
		return nil, nil, fail("invalid-options", "Import timeout must be positive or zero for the default.", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	return ctx, cancel, nil
}

// importPlan fetches the commits plan names, reads and validates the files it imports, and returns them with the
// snapshot that records them, and the metadata of each group in groups that the catalog lacks, when the plan's
// library-wide files have it. Every skill comes from the plan's commit. A plan of a repository that isn't a library
// imports only skills, with an empty catalog.
func (r *repository) importPlan(ctx context.Context, source rules.Source, plan sourcePlan, groups []string) (Library, error) {
	commits := []string{plan.commit}
	for _, rule := range plan.rules {
		commits = append(commits, rule.Commit)
	}
	if err := r.fetchCommits(ctx, commits, fmt.Sprintf("A commit that vendor/%s/_source.json records is missing from the library's repository, so the library may have rewritten its history. Ask the library's maintainer to restore it, or, to keep your rule versions, set sources.%s.ref to a library release tag or commit the library still has that publishes them, then run trailguide project sync. As a last resort, delete vendor/%s and run trailguide project sync, which imports every unpinned rule's newest version.", source.Name, source.Name, source.Name)); err != nil {
		return Library{}, err
	}
	if plan.nonLibrary {
		return r.importNonLibrary(ctx, source, plan)
	}
	individual := slices.Sorted(slices.Values(plan.individual))
	input, err := r.snapshotFiles(ctx, source, plan)
	if err != nil {
		return Library{}, err
	}
	catalog, err := library.LoadSource(ctx, input, source.Name, source.Groups, individual)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Library{}, gitexec.ContextFailure(err)
		}
		return Library{}, err
	}
	if loaded := catalogRules(catalog); !slices.Equal(loaded, slices.Sorted(maps.Keys(plan.rules))) {
		return Library{}, fail("unsupported-content", "The library's files don't match the rules its release records list. Don't create or move release tags by hand.", nil)
	}
	// A plan that update decided on keeps no ref, which the source's own stands in for. A source whose ref chooses a
	// library release records that release instead.
	ref := source.Ref
	switch {
	case plan.release != 0 && source.Ref.FollowsReleases():
		ref = rules.GitRef{}
	case plan.ref.Equal(source.Ref):
		ref = plan.ref
	}
	snapshot := library.Snapshot{
		Repository:    source.Repository,
		RetiredRules:  plan.retired,
		Ref:           ref,
		Release:       plan.release,
		Commit:        plan.commit,
		Selection:     rules.GroupSelection{Pattern: source.Groups.Pattern, Groups: slices.Clone(source.Groups.Groups)},
		Groups:        []string{},
		RuleSelection: slices.Clone(source.Rules),
		Rules:         plan.rules,
	}
	for _, group := range catalog.Groups {
		if source.Groups.Includes(group.ID) {
			snapshot.Groups = append(snapshot.Groups, group.ID)
		}
	}
	snapshot.Files = map[string][]byte{}
	for file, data := range catalog.Files() {
		snapshot.Files[file] = bytes.Clone(data)
	}
	// A library publishes only its manifest's library section, never the sources it uses as a project.
	if snapshot.Files[librarytree.Manifest], err = rules.PublishedManifest(snapshot.Files[librarytree.Manifest], source.Name+"/"+librarytree.RepositoryPath(librarytree.Manifest)); err != nil {
		return Library{}, err
	}
	metadata, err := requestedGroupMetadata(input, catalog, groups)
	if err != nil {
		return Library{}, err
	}
	imported, err := r.readSkills(ctx, source, plan, true, input.total)
	if err != nil {
		return Library{}, err
	}
	addSkills(&snapshot, source, imported)
	if err := ctx.Err(); err != nil {
		return Library{}, gitexec.ContextFailure(err)
	}
	return Library{Catalog: catalog, Snapshot: snapshot, Warnings: plan.warnings, GroupMetadata: metadata}, nil
}

// importNonLibrary imports the skills of a source whose repository has no library manifest at the plan's commit:
// a snapshot without rules, groups, or a library release, and an empty catalog.
func (r *repository) importNonLibrary(ctx context.Context, source rules.Source, plan sourcePlan) (Library, error) {
	imported, err := r.readSkills(ctx, source, plan, false, 0)
	if err != nil {
		return Library{}, err
	}
	snapshot := library.Snapshot{
		Repository:    source.Repository,
		RetiredRules:  []string{},
		Ref:           plan.ref,
		Commit:        plan.commit,
		Groups:        []string{},
		RuleSelection: []string{},
		Rules:         map[string]library.ImportedRule{},
		Files:         map[string][]byte{},
		NonLibrary:    true,
	}
	addSkills(&snapshot, source, imported)
	if err := ctx.Err(); err != nil {
		return Library{}, gitexec.ContextFailure(err)
	}
	catalog := library.Catalog{Rules: []string{}, Groups: []library.Group{}, SupportingFiles: map[string][]byte{}}
	return Library{Catalog: catalog, Snapshot: snapshot, Warnings: plan.warnings, GroupMetadata: map[string][]byte{}}, nil
}

// addSkills records the source's skill selection and the imported skills, with their files, in snapshot.
func addSkills(snapshot *library.Snapshot, source rules.Source, imported importedSkills) {
	snapshot.SkillSelection = slices.Clone(source.Skills)
	snapshot.Skills = imported.skills
	snapshot.AvailableSkills = imported.available
	snapshot.Executable = imported.executable
	maps.Copy(snapshot.Files, imported.files)
}

// requestedGroupMetadata reads, from input, the metadata of each of groups that catalog lacks and input has.
func requestedGroupMetadata(input *gitFiles, catalog library.Catalog, groups []string) (map[string][]byte, error) {
	result := map[string][]byte{}
	for _, group := range groups {
		file := group + "/_group.yaml"
		if slices.ContainsFunc(catalog.Groups, func(loaded library.Group) bool { return loaded.ID == group }) {
			continue
		}
		if _, err := input.Lstat(file); err != nil {
			continue
		}
		data, err := input.ReadFile(file)
		if err != nil {
			return nil, err
		}
		result[group] = data
	}
	return result, nil
}

// snapshotFiles assembles the files the plan imports: library-wide files from the plan's commit, and each rule's
// Markdown file and asset directory from its own commit. Other rules' files are left out. It first fetches, in
// one request, the blobs the catalog loader will read, except shared assets, which rules name only in their text:
// the manifest, terms, the metadata of the groups the source imports, and the imported rules' files.
func (r *repository) snapshotFiles(ctx context.Context, source rules.Source, plan sourcePlan) (*gitFiles, error) {
	main, err := r.tree(ctx, plan.commit)
	if err != nil {
		return nil, err
	}
	if err := r.requireLibraryLayout(source.Name, plan.release, plan.commit); err != nil {
		return nil, err
	}
	terms, err := r.terms(ctx, source.Name, main)
	if err != nil {
		return nil, err
	}
	imported := map[string]bool{}
	for id := range plan.rules {
		imported[ruleGroup(id)+"/_group.yaml"] = true
	}
	files := map[string]treeEntry{}
	prefetch := []treeEntry{}
	for file, entry := range main {
		if _, versioned := librarytree.VersionedRule(file); versioned {
			continue
		}
		files[file] = entry
		group, metadata := strings.CutSuffix(file, "/_group.yaml")
		if file == librarytree.Manifest || slices.Contains(terms, file) || metadata && (imported[file] || source.Groups.Includes(group)) {
			prefetch = append(prefetch, entry)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(plan.rules)) {
		owned, err := r.ruleFiles(ctx, plan.rules[id].Commit, id)
		if err != nil {
			return nil, err
		}
		if _, ok := owned[id+".md"]; !ok {
			return nil, fail("unsupported-content", fmt.Sprintf("The library's commit %s has no rule %s, although its release record lists the rule.", plan.rules[id].Commit, id), nil)
		}
		if err := r.requireLegacyRuleLinks(ctx, source.Name, id, plan.rules[id], owned); err != nil {
			return nil, err
		}
		rule := plan.rules[id]
		rule.LegacyLayout = r.legacy[rule.Commit]
		plan.rules[id] = rule
		for file, entry := range owned {
			files[file] = entry
			prefetch = append(prefetch, entry)
		}
	}
	restored, err := r.missingGroupMetadata(ctx, files, plan)
	if err != nil {
		return nil, err
	}
	for file, entry := range restored {
		files[file] = entry
		prefetch = append(prefetch, entry)
	}
	if err := r.prefetch(ctx, prefetch); err != nil {
		return nil, err
	}
	return newGitFiles(ctx, r, files), nil
}

// missingGroupMetadata returns, by path, the metadata of each imported rule's group that files lacks, such as a
// retired rule's group that the library-wide release removed. Each comes from the newest library release among
// the group's imported rules that still has it; a group none of them has stays missing, for the loader to report.
func (r *repository) missingGroupMetadata(ctx context.Context, files map[string]treeEntry, plan sourcePlan) (map[string]treeEntry, error) {
	byGroup := map[string][]library.ImportedRule{}
	for id, rule := range plan.rules {
		byGroup[ruleGroup(id)] = append(byGroup[ruleGroup(id)], rule)
	}
	restored := map[string]treeEntry{}
	for _, group := range slices.Sorted(maps.Keys(byGroup)) {
		metadata := group + "/_group.yaml"
		if _, ok := files[metadata]; ok {
			continue
		}
		candidates := byGroup[group]
		slices.SortFunc(candidates, func(a, b library.ImportedRule) int { return b.Release - a.Release })
		for _, rule := range candidates {
			tree, err := r.tree(ctx, rule.Commit)
			if err != nil {
				return nil, err
			}
			if entry, ok := tree[metadata]; ok {
				restored[metadata] = entry
				break
			}
		}
	}
	return restored, nil
}

// catalogRules returns the library rule IDs of every rule in catalog, sorted.
func catalogRules(catalog library.Catalog) []string {
	ids := []string{}
	for _, group := range catalog.Groups {
		for _, rule := range group.Rules {
			ids = append(ids, strings.TrimSuffix(rule.Path, ".md"))
		}
	}
	slices.Sort(ids)
	return ids
}
