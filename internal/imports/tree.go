// Expose immutable Git tree metadata and bounded original blob bytes to the library loader.

package imports

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
)

const maxTreeBytes = 8 << 20
const maxBlobBytes = 8 << 20
const maxRetainedBytes = 64 << 20
const maxTreeFiles = 10000

// treeEntry implements file metadata without materializing or following Git entries.
type treeEntry struct {
	name   string
	mode   fs.FileMode
	object string
}

// Name returns the final component of an immutable tree path.
func (e treeEntry) Name() string { return path.Base(e.name) }

// Size is zero: listing a partial clone's tree doesn't fetch blobs, so sizes are checked when files are read.
func (e treeEntry) Size() int64 { return 0 }

// Mode preserves symlink and submodule distinction for the shared loader.
func (e treeEntry) Mode() fs.FileMode { return e.mode }

// ModTime is zero because Git trees do not contain per-file timestamps.
func (e treeEntry) ModTime() time.Time { return time.Time{} }

// IsDir identifies synthetic parent directories in the tree inventory.
func (e treeEntry) IsDir() bool { return e.mode.IsDir() }

// Sys returns no operating-system metadata for a Git object.
func (e treeEntry) Sys() any { return nil }

// gitFiles supplies metadata without I/O and fetches only files requested by shared catalog validation.
// Its entries can combine several commits' trees, because blobs are read by object ID from one repository.
type gitFiles struct {
	ctx         context.Context
	repo        *repository
	entries     map[string]treeEntry
	directories map[string][]fs.DirEntry
	total       int
}

// newGitFiles indexes files, a map from path to a file entry, adding the directories their paths imply.
func newGitFiles(ctx context.Context, repo *repository, files map[string]treeEntry) *gitFiles {
	entries := map[string]treeEntry{".": {name: ".", mode: fs.ModeDir | 0755}}
	for name, entry := range files {
		entries[name] = entry
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			entries[parent] = treeEntry{name: parent, mode: fs.ModeDir | 0755}
		}
	}
	source := &gitFiles{ctx: ctx, repo: repo, entries: entries, directories: map[string][]fs.DirEntry{}}
	for name, entry := range entries {
		if name == "." {
			continue
		}
		parent := path.Dir(name)
		source.directories[parent] = append(source.directories[parent], fs.FileInfoToDirEntry(entry))
	}
	for _, children := range source.directories {
		slices.SortFunc(children, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	}
	return source
}

// parseTree reads `git ls-tree -r -z` output, rejecting malformed framing and unsafe paths, and returns each
// file's entry by path. Directories aren't included; newGitFiles derives them.
func parseTree(data []byte) (map[string]treeEntry, error) {
	if len(data) > maxTreeBytes {
		return nil, fail("limit-exceeded", "Git tree listing exceeds 8 MiB.", nil)
	}
	if !utf8.Valid(data) || (len(data) > 0 && data[len(data)-1] != 0) {
		return nil, fail("unsupported-content", "Git tree must contain complete UTF-8 records.", nil)
	}
	entries := map[string]treeEntry{}
	directories := map[string]bool{}
	count := 0
	for _, record := range bytes.Split(bytes.TrimSuffix(data, []byte{0}), []byte{0}) {
		if len(data) == 0 {
			break
		}
		if len(record) == 0 {
			return nil, fail("unsupported-content", "Empty Git tree record.", nil)
		}
		count++
		if count > maxTreeFiles {
			return nil, fail("limit-exceeded", "Library tree exceeds 10,000 files.", nil)
		}
		header, name, ok := strings.Cut(string(record), "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || !validObjectID(fields[2]) || !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\\x00") || len(strings.Split(name, "/")) > 64 {
			return nil, fail("unsupported-content", "Unsupported Git tree entry or path.", nil)
		}
		entry := treeEntry{name: name, object: fields[2]}
		switch fields[0] + " " + fields[1] {
		case "100644 blob":
			entry.mode = 0644
		case "100755 blob":
			entry.mode = 0755
		case "120000 blob":
			entry.mode = fs.ModeSymlink | 0777
		case "160000 commit":
			entry.mode = fs.ModeIrregular
		default:
			return nil, fail("unsupported-content", "Unsupported Git tree object mode.", nil)
		}
		if _, ok := entries[name]; ok || directories[name] {
			return nil, fail("unsupported-content", "Duplicate or conflicting Git path.", nil)
		}
		entries[name] = entry
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, ok := entries[parent]; ok {
				return nil, fail("unsupported-content", "File and directory Git paths conflict.", nil)
			}
			directories[parent] = true
		}
	}
	return entries, nil
}

// libraryTree places a commit's files, given by repository path, at their library paths, as librarytree.LibraryPath
// does, leaving out the files outside the library, such as the repository's vendor/ and generated/ folders. It also
// reports whether the commit keeps its library in the layout before version 2, with rule-library.yaml at the
// repository root: such a commit's files keep their paths, including rule-library.yaml, which is read only for the
// license and notice files it declares, except its .trailguide/, which held only a project's files.
func libraryTree(files map[string]treeEntry) (map[string]treeEntry, bool) {
	_, legacy := files[librarytree.LegacyManifest]
	result := map[string]treeEntry{}
	for name, entry := range files {
		library, ok := librarytree.LibraryPath(name)
		if legacy {
			library = name
			ok = name != librarytree.Manifest && !strings.HasPrefix(name, librarytree.Directory+"/")
		}
		if ok {
			entry.name = library
			result[library] = entry
		}
	}
	return result, legacy
}

// legacyLayoutProblem explains why a source can't import revision, a library release or commit that keeps the
// library in the layout before version 2.
func legacyLayoutProblem(source, revision string) string {
	return fmt.Sprintf("sources.%s: the library's %s keeps its files in the layout from before this version of Trail Guide, with rule-library.yaml and rule folders at the repository root. This version reads a library's manifest from the library section of .trailguide/config.yaml and its rules from .trailguide/rules/, so it can't import %s. Ask the library's maintainer to move the library's files into .trailguide/ and publish a library release, then select that release, such as with trailguide project update %s.", source, revision, revision, source)
}

// revisionName names a library release by its tag, such as release/4, or another revision by its commit, when
// release is 0.
func revisionName(release int, commit string) string {
	if release != 0 {
		return "release/" + strconv.Itoa(release)
	}
	return "commit " + commit
}

// requireLibraryLayout fails with unsupported-library-layout when commit, a fetched commit whose tree was listed,
// which supplies a source's manifest and library-wide files, keeps the library in the layout before version 2.
// release is the commit's library release number, or 0 for another revision.
func (r *repository) requireLibraryLayout(source string, release int, commit string) error {
	if r.legacy[commit] {
		return fail("unsupported-library-layout", legacyLayoutProblem(source, revisionName(release, commit)), nil)
	}
	return nil
}

// requireLegacyRuleLinks fails with unsupported-library-layout when rule id, imported from a fetched commit in the
// layout before version 2, has a link that resolved to another file then than it does now: a relative link out of
// the rule folders, such as to a shared asset or a license file, in its Markdown file or a Markdown file in its asset
// directory. Such links resolved from the repository root then, and resolve from .trailguide/rules/ now, so the
// rule's version would import with broken links. owned holds the rule's files by library path.
func (r *repository) requireLegacyRuleLinks(ctx context.Context, source, id string, rule library.ImportedRule, owned map[string]treeEntry) error {
	if !r.legacy[rule.Commit] {
		return nil
	}
	documents := map[string]treeEntry{}
	for file, entry := range owned {
		if strings.HasSuffix(file, ".md") {
			documents[file] = entry
		}
	}
	if err := r.prefetch(ctx, slices.Collect(maps.Values(documents))); err != nil {
		return err
	}
	reader := newGitFiles(ctx, r, documents)
	for _, file := range slices.Sorted(maps.Keys(documents)) {
		data, err := reader.ReadFile(file)
		if err != nil {
			return err
		}
		before, err := rules.MarkdownTargets(string(data), file)
		if err != nil {
			return err
		}
		now, err := rules.LibraryTargets(string(data), file)
		if err == nil && slices.Equal(before, now) {
			continue
		}
		if len(before) == 0 {
			return err
		}
		index := max(slices.IndexFunc(before, func(target string) bool { return !slices.Contains(now, target) }), 0)
		version := ""
		if rule.Version != nil {
			version = " version " + rule.Version.String()
		}
		return fail("unsupported-library-layout", fmt.Sprintf("sources.%s: rule %s%s comes from the library's %s, which keeps its files in the layout from before this version of Trail Guide, and its link to %s resolves differently in the current layout, so this version of Trail Guide can't import the rule's version. Select a version of %s that the library published after moving its files into .trailguide/, such as with trailguide project update %s:%s, or ask the library's maintainer to publish one.", source, id, version, revisionName(rule.Release, rule.Commit), before[index], id, source, id), nil)
	}
	return nil
}

// ruleFiles returns the files of rule id at a fetched commit, its Markdown file and its asset directory's files,
// by path. The map is empty when the commit doesn't have the rule.
func (r *repository) ruleFiles(ctx context.Context, commit, id string) (map[string]treeEntry, error) {
	index, ok := r.owned[commit]
	if !ok {
		tree, err := r.tree(ctx, commit)
		if err != nil {
			return nil, err
		}
		index = map[string]map[string]treeEntry{}
		for file, entry := range tree {
			if owner, ok := librarytree.VersionedRule(file); ok {
				if index[owner] == nil {
					index[owner] = map[string]treeEntry{}
				}
				index[owner][file] = entry
			}
		}
		r.owned[commit] = index
	}
	return maps.Clone(index[id]), nil
}

// terms returns the library paths of the license and notice files that tree's library manifest declares, or none
// without a manifest or a library section, which the catalog loader reports. A tree in the layout before version 2
// declares them in its rule-library.yaml, which a library release already validated.
func (r *repository) terms(ctx context.Context, source string, tree map[string]treeEntry) ([]string, error) {
	if entry, ok := tree[librarytree.LegacyManifest]; ok {
		data, err := newGitFiles(ctx, r, map[string]treeEntry{librarytree.LegacyManifest: entry}).ReadFile(librarytree.LegacyManifest)
		if err != nil {
			return nil, err
		}
		return library.LegacyTerms(data), nil
	}
	entry, ok := tree[librarytree.Manifest]
	if !ok {
		return nil, nil
	}
	data, err := newGitFiles(ctx, r, map[string]treeEntry{librarytree.Manifest: entry}).ReadFile(librarytree.Manifest)
	if err != nil {
		return nil, err
	}
	declaration, err := rules.ParsePublishedManifest(data, source+"/"+librarytree.RepositoryPath(librarytree.Manifest))
	if err != nil || declaration == nil {
		return nil, err
	}
	return rules.LicensePaths(declaration.License), nil
}

// Lstat returns immutable object metadata, preserving links as links rather than following them.
func (g *gitFiles) Lstat(name string) (fs.FileInfo, error) {
	if err := g.ctx.Err(); err != nil {
		return nil, gitexec.ContextFailure(err)
	}
	entry, ok := g.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrNotExist}
	}
	return entry, nil
}

// ReadDir returns a copy of the bounded sorted child inventory.
func (g *gitFiles) ReadDir(name string) ([]fs.DirEntry, error) {
	info, err := g.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return slices.Clone(g.directories[name]), nil
}

// ReadFile verifies Git's batch framing before retaining a selected blob's original bytes, fetching the blob
// when it isn't present yet. It refuses a blob over 8 MiB, or one that would take the retained total over 64 MiB.
func (g *gitFiles) ReadFile(name string) ([]byte, error) {
	info, err := g.Lstat(name)
	if err != nil {
		return nil, err
	}
	entry := g.entries[name]
	if !info.Mode().IsRegular() {
		return nil, fail("unsupported-content", name+": symlinks and submodules are unsupported.", nil)
	}
	if g.total >= maxRetainedBytes {
		return nil, fail("limit-exceeded", "Selected library content exceeds import byte limits.", nil)
	}
	limit := min(maxBlobBytes, maxRetainedBytes-g.total)
	result, err := g.repo.runner.Run(g.ctx, g.repo.directory, []string{"cat-file", "--batch"}, limit+4096, []byte(entry.object+"\n"))
	if err != nil {
		return nil, err
	}
	if result.Status != 0 {
		return nil, gitFailure("git-failed", "Could not read library files.", result.Diagnostics)
	}
	header, body, ok := bytes.Cut(result.Output, []byte{'\n'})
	size, found := strings.CutPrefix(string(header), entry.object+" blob ")
	length, parseErr := strconv.Atoi(size)
	if !ok || !found || parseErr != nil || length < 0 || strconv.Itoa(length) != size {
		return nil, fail("git-failed", "Git returned inconsistent or incomplete blob contents.", nil)
	}
	if length > limit {
		return nil, fail("limit-exceeded", "Selected library content exceeds import byte limits.", nil)
	}
	if len(body) != length+1 || body[length] != '\n' {
		return nil, fail("git-failed", "Git returned inconsistent or incomplete blob contents.", nil)
	}
	g.total += length
	return bytes.Clone(body[:length]), nil
}
