// Read the library's own release tags and compare working-tree files with the latest library release.

package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/releasetag"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// maxReleaseTags bounds the release tags one check reads, matching the tag-listing limit for imports.
const maxReleaseTags = 20_000

// objectID accepts a full SHA-1 or SHA-256 object name.
var objectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// shallowClone tells the author how to get the history that comparing with a library release needs.
const shallowClone = "this clone has partial history, so it can't be compared with the latest library release. Fetch the full history and tags, such as with git fetch --unshallow --tags; in CI, check out with fetch-depth: 0."

// missingReleaseTags explains a clone whose commit has change notes but no release tags, such as a clone made with
// git clone --no-tags, which would otherwise look like a library that never published a library release.
const missingReleaseTags = ".trailguide/changes/ holds change notes, but this clone has no release/<number> tags, so it can't be compared with the latest library release. Fetch the tags, such as with git fetch --tags; in CI, check out with fetch-depth: 0. If the library has never published a library release, delete the notes in .trailguide/changes/, which the first library release doesn't need."

// libraryGit runs Git in the library author's own repository, honoring their Git configuration.
type libraryGit struct {
	runner gitexec.Runner
	dir    string
	// unpublished is the number of a release tag that exists only in this clone, which the release history leaves
	// out, so it's checked against the library instead of trusted; it's 0 when there is none.
	unpublished int
}

// withoutUnpublished returns g with a release history that leaves out release tag number, which exists only in
// this clone. A number of 0 leaves out nothing.
func (g *libraryGit) withoutUnpublished(number int) *libraryGit {
	copied := *g
	copied.unpublished = number
	return &copied
}

// releaseHistory is what the library releases reachable from HEAD record.
type releaseHistory struct {
	// latest is nil before the first library release.
	latest *publishedRelease
	// retired maps every rule a reachable library release retired to that release's number, and replacedBy maps
	// each of those that named a replacement to it.
	retired    map[string]int
	replacedBy map[string]string
	// skills is the newest reachable library release whose record has a skills field, which skill versions and
	// skill changes are compared with, or nil when none has. It is older than latest when a Trail Guide without skill
	// support published the library releases after it.
	skills *skillRelease
	// retiredSkills and skillReplacedBy are retired and replacedBy for skills.
	retiredSkills   map[string]int
	skillReplacedBy map[string]string
}

// skillRelease is the library release that skill versions come from.
type skillRelease struct {
	number int
	record trailguide.SkillRecord
	// files maps the path of each entry in the skills/ folder at the release's commit, such as
	// build-feature/SKILL.md, to its mode, kind, and object.
	files map[string]treeEntry
}

// tagName returns the release tag's short name, such as release/4.
func (r *skillRelease) tagName() string { return "release/" + strconv.Itoa(r.number) }

// publishedRelease is the latest library release reachable from HEAD.
type publishedRelease struct {
	number int
	// object is the tag object's ID, and commit the commit it tags.
	object, commit string
	// notes are the release notes before the tag message's record, which a GitHub Release page repeats.
	notes  string
	record trailguide.ReleaseRecord
	// files maps the library path of each file under practices/, techs/, and changes/ at the release's commit to its
	// blob ID.
	files map[string]string
}

// tagName returns the release tag's short name, such as release/4.
func (r *publishedRelease) tagName() string { return "release/" + strconv.Itoa(r.number) }

// openLibraryGit returns nil when the library root isn't a Git repository's root, so it has no release history.
// The CLI always resolves a library inside Git to its repository root.
func openLibraryGit(ctx context.Context, dir string, options gitexec.Options) (*libraryGit, error) {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, failure("git-failed", "inspect the library's Git repository: "+err.Error(), err)
	}
	runner, err := gitexec.Owned(options)
	if err != nil {
		return nil, err
	}
	if err := runner.RequireVersion(ctx, dir); err != nil {
		return nil, err
	}
	return &libraryGit{runner: runner, dir: dir}, nil
}

// releaseTags lists the annotated release tags of commits reachable from HEAD in ascending number order, except
// the unpublished one. It fails in a shallow clone, which may lack tags and history, and with missing-release-tags
// when none is reachable but HEAD has change notes, which only a library release makes necessary. It returns none
// on an unborn branch. A nil receiver, a library outside Git, has no release tags.
func (g *libraryGit) releaseTags(ctx context.Context) ([]releasetag.Tag, error) {
	if g == nil {
		return nil, nil
	}
	if err := g.requireFullHistory(ctx); err != nil {
		return nil, err
	}
	head, err := g.runner.Run(ctx, g.dir, []string{"rev-parse", "--verify", "--quiet", "HEAD^{commit}"}, 4096, nil)
	if err != nil {
		return nil, fmt.Errorf("find the library's HEAD commit: %w", err)
	}
	if head.Status != 0 {
		return nil, nil
	}
	listed, err := g.listReleaseTags(ctx, "HEAD")
	if err != nil {
		return nil, err
	}
	tags := []releasetag.Tag{}
	for _, tag := range listed {
		if tag.Number == g.unpublished {
			continue
		}
		if tag.Type != "tag" || tag.TargetType != "commit" {
			return nil, failure("invalid-release-tag", tag.Name()+": expected an annotated tag on a commit, with a release record. Don't create release tags by hand; if you created "+tag.Name()+" by hand, delete it with git tag --delete "+tag.Name()+", then run the command again.", nil)
		}
		if tag.Size > releasetag.MaxBytes {
			return nil, failure("limit-exceeded", tag.Name()+": tag message exceeds 8 MiB", nil)
		}
		tags = append(tags, tag)
	}
	if len(tags) == 0 && g.unpublished == 0 {
		notes, err := g.runner.Run(ctx, g.dir, append([]string{"ls-tree", "--name-only", "HEAD", "--"}, withLegacyDirectory([]string{librarytree.RepositoryPath(changesDirectory)})...), 4096, nil)
		if err != nil {
			return nil, fmt.Errorf("list the library's committed change notes: %w", err)
		}
		if notes.Status == 0 && len(bytes.TrimSpace(notes.Output)) > 0 {
			return nil, failure("missing-release-tags", missingReleaseTags, nil)
		}
	}
	return tags, nil
}

// listReleaseTags lists the clone's release tags, or only those of commits reachable from merged when it isn't
// empty, in ascending number order. It fails with limit-exceeded past 20,000.
func (g *libraryGit) listReleaseTags(ctx context.Context, merged string) ([]releasetag.Tag, error) {
	tags, err := releasetag.List(ctx, g.runner, g.dir, merged)
	if err != nil {
		return nil, fmt.Errorf("list the library's release tags: %w", err)
	}
	if len(tags) > maxReleaseTags {
		return nil, failure("limit-exceeded", "library has more than 20,000 release tags", nil)
	}
	return tags, nil
}

// requireFullHistory fails with shallow-clone in a clone that may lack history and tags.
func (g *libraryGit) requireFullHistory(ctx context.Context) error {
	shallow, err := g.runner.Output(ctx, g.dir, []string{"rev-parse", "--is-shallow-repository"}, 4096)
	if err != nil {
		return fmt.Errorf("check whether the library is a shallow clone: %w", err)
	}
	if strings.TrimSpace(string(shallow)) != "false" {
		return failure("shallow-clone", shallowClone, nil)
	}
	return nil
}

// history reads every reachable release record and lists the latest release's rule and note files.
func (g *libraryGit) history(ctx context.Context) (releaseHistory, error) {
	history := releaseHistory{retired: map[string]int{}, replacedBy: map[string]string{}, retiredSkills: map[string]int{}, skillReplacedBy: map[string]string{}}
	tags, err := g.releaseTags(ctx)
	if err != nil || len(tags) == 0 {
		return history, err
	}
	// Only the latest library release's notes and record, and the newest skills field, are kept; the others
	// contribute their retirements.
	var latestRelease releasetag.Release
	skillsIndex := -1
	err = releasetag.Read(ctx, g.runner, g.dir, tags, func(i int, release releasetag.Release) error {
		addRetirements(history.retired, history.replacedBy, release.Record.Retired, tags[i].Number)
		if skills := release.Record.Skills; skills != nil {
			addRetirements(history.retiredSkills, history.skillReplacedBy, skills.Retired, tags[i].Number)
			history.skills, skillsIndex = &skillRelease{number: tags[i].Number, record: *skills}, i
		}
		if i == len(tags)-1 {
			latestRelease = release
		}
		return nil
	})
	var invalid *releasetag.RecordError
	var unsupported *trailguide.UnsupportedReleaseRecordError
	switch {
	case errors.As(err, &invalid) && errors.As(invalid.Err, &unsupported):
		return releaseHistory{}, unsupportedRecord(invalid.Tag, unsupported.FormatVersion)
	case invalid != nil:
		return releaseHistory{}, failure("invalid-release-tag", invalid.Error()+". Don't create or move release tags by hand; if you created "+invalid.Tag+" by hand, delete it with git tag --delete "+invalid.Tag+", then run the command again.", invalid.Err)
	}
	if err != nil {
		return releaseHistory{}, fmt.Errorf("read the library's release tags: %w", err)
	}
	latest := tags[len(tags)-1]
	files, err := g.releaseFiles(ctx, latest.Object)
	if err != nil {
		return releaseHistory{}, err
	}
	history.latest = &publishedRelease{number: latest.Number, object: latest.Object, commit: latest.Target, notes: latestRelease.Notes, record: latestRelease.Record, files: files}
	if history.skills != nil {
		if history.skills.files, err = g.skillFiles(ctx, tags[skillsIndex].Object); err != nil {
			return releaseHistory{}, fmt.Errorf("list the skills of %s: %w", history.skills.tagName(), err)
		}
	}
	return history, nil
}

// addRetirements records in retired, and in replacedBy for each that names a replacement, the rules or skills that
// record, release number's retirements, retired, unless an earlier library release retired them.
func addRetirements(retired map[string]int, replacedBy map[string]string, record map[string]trailguide.RetiredRule, number int) {
	for id, rule := range record {
		if _, ok := retired[id]; !ok {
			retired[id] = number
			if rule.ReplacedBy != "" {
				replacedBy[id] = rule.ReplacedBy
			}
		}
	}
}

// skillFiles maps the path of each entry in the library's skills/ folder in a commit's tree, or a tag's, such as
// build-feature/SKILL.md, to its mode, kind, and object.
func (g *libraryGit) skillFiles(ctx context.Context, treeish string) (map[string]treeEntry, error) {
	entries, err := g.treeEntries(ctx, treeish, []string{skillsDirectory})
	if err != nil {
		return nil, err
	}
	files := map[string]treeEntry{}
	for name, entry := range entries {
		if inside, found := strings.CutPrefix(name, skillsDirectory+"/"); found {
			files[inside] = entry
		}
	}
	return files, nil
}

// unsupportedRecord fails with unsupported-release-record for release tag tag, whose record a newer Trail Guide
// wrote in release record format version, asking the author to upgrade.
func unsupportedRecord(tag string, version int) error {
	return failure("unsupported-release-record", tag+" uses release record format "+strconv.Itoa(version)+", which this version of Trail Guide can't read. Upgrade Trail Guide, then run the command again.", nil)
}

// releaseFiles lists the blobs under practices/, techs/, and changes/ at a release tag's commit, by library path.
func (g *libraryGit) releaseFiles(ctx context.Context, tag string) (map[string]string, error) {
	files, err := g.libraryFiles(ctx, tag, []string{"practices", "techs", changesDirectory})
	if err != nil {
		return nil, fmt.Errorf("list the latest library release's files: %w", err)
	}
	return files, nil
}

// libraryFiles maps the library path of each file at or under the library paths, paths, in a commit's tree, or a
// tag's, to its blob ID, leaving out other entries, such as submodules. It reads a tree in either layout, as
// libraryEntries does.
func (g *libraryGit) libraryFiles(ctx context.Context, treeish string, paths []string) (map[string]string, error) {
	entries, err := g.libraryEntries(ctx, treeish, paths)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for name, entry := range entries {
		if entry.kind == "blob" {
			files[name] = entry.object
		}
	}
	if object, ok := files[librarytree.Manifest]; ok {
		data, err := g.runner.Output(ctx, g.dir, []string{"cat-file", "blob", object}, maxFileBytes)
		if err != nil {
			return nil, fmt.Errorf("read %s:%s: %w", treeish, librarytree.Manifest, err)
		}
		files[librarytree.Manifest] = manifestDigest(data)
	}
	return files, nil
}

// manifestDigest identifies what a library publishes of its manifest, data, which is a .trailguide/config.yaml or,
// in a library release made before the layout of version 2, a rule-library.yaml: its library section, as
// rules.PublishedManifest gives it, and the skill forks its release notes report, so a change to only the rest of
// the sources the repository uses as a project, or to a comment, isn't a library-wide change. A manifest that doesn't parse, which library check reports, is identified
// by its bytes.
func manifestDigest(data []byte) string {
	published := data
	if _, decoded, err := decode.YAML(data, librarytree.Manifest); err == nil {
		var fields map[string]json.RawMessage
		if json.Unmarshal(decoded, &fields) == nil {
			section := fields["library"]
			if _, current := fields["schemaVersion"]; !current {
				// rule-library.yaml held the library section's fields beside its format version.
				delete(fields, "formatVersion")
				section, _ = json.Marshal(fields)
			}
			if section != nil {
				if manifest, err := rules.PublishedManifestOf(section, fields["sources"]); err == nil {
					published = append(manifest, skillForks(fields["sources"])...)
				}
			}
		}
	}
	sum := sha256.Sum256(published)
	return "manifest:" + hex.EncodeToString(sum[:])
}

// skillForks returns, as JSON with sorted keys, what release notes report of each skill fork that sources, a
// configuration's sources given as JSON, records: the skill installed in its place and the commit it's based on. It
// returns nothing when no source records a fork.
func skillForks(sources json.RawMessage) []byte {
	type fork struct {
		As      string `json:"as"`
		BasedOn string `json:"basedOn,omitempty"`
	}
	var declared map[string]json.RawMessage
	_ = json.Unmarshal(sources, &declared)
	forks := map[string]map[string]fork{}
	for name, data := range declared {
		var source struct {
			Skills struct {
				Forks map[string]fork `json:"forks"`
			} `json:"skills"`
		}
		if json.Unmarshal(data, &source) == nil && len(source.Skills.Forks) > 0 {
			forks[name] = source.Skills.Forks
		}
	}
	if len(forks) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(forks)
	return encoded
}

// treeEntry is one entry of a recursive tree listing: a blob, including a symbolic link, or a submodule.
type treeEntry struct {
	// mode is the octal Git file mode, such as 100644 for a file or 120000 for a symbolic link.
	mode, kind, object string
}

// libraryEntries maps the library path of each entry at or under the library paths, paths, in a commit's tree, or a
// tag's, to its mode, kind, and object. A tree in the layout before version 2, which has rule-library.yaml at the
// repository root, keeps its library at the root, where each path is its library path, and its manifest stands in
// for config.yaml, so a library release made before the move compares with the moved working tree; it ignores that
// tree's .trailguide/, which held only a project's files.
func (g *libraryGit) libraryEntries(ctx context.Context, treeish string, paths []string) (map[string]treeEntry, error) {
	listed := []string{librarytree.LegacyManifest}
	for _, name := range paths {
		listed = append(listed, name)
		if repository := librarytree.RepositoryPath(name); repository != name {
			listed = append(listed, repository)
		}
	}
	tree, err := g.treeEntries(ctx, treeish, listed)
	if err != nil {
		return nil, err
	}
	_, legacy := tree[librarytree.LegacyManifest]
	entries := map[string]treeEntry{}
	for name, entry := range tree {
		library, ok := librarytree.LibraryPath(name)
		if legacy {
			switch {
			case name == librarytree.LegacyManifest:
				library, ok = librarytree.Manifest, true
			case name == librarytree.Manifest || strings.HasPrefix(name, librarytree.Directory+"/"):
				ok = false
			default:
				library, ok = name, true
			}
		}
		if ok && (legacy || name != librarytree.LegacyManifest) {
			entries[library] = entry
		}
	}
	return entries, nil
}

// treeEntries maps each entry at or under paths in a commit's tree, or a tag's, to its mode, kind, and object. A
// tree that keeps its Trail Guide directory under the name from before the rename, as every library release published
// before it does, lists that directory's entries as .trailguide/, as librarytree.Upgrade spells them.
func (g *libraryGit) treeEntries(ctx context.Context, treeish string, paths []string) (map[string]treeEntry, error) {
	listing, err := g.runner.Output(ctx, g.dir, append([]string{"ls-tree", "-r", "-z", "--full-tree", treeish, "--"}, withLegacyDirectory(paths)...), 32*1024*1024)
	if err != nil {
		return nil, err
	}
	entries := map[string]treeEntry{}
	for record := range bytes.SplitSeq(bytes.TrimSuffix(listing, []byte{0}), []byte{0}) {
		if len(record) == 0 {
			continue
		}
		header, name, ok := strings.Cut(string(record), "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || !objectID.MatchString(fields[2]) {
			return nil, failure("git-failed", "Git listed the library's files in an unexpected format", nil)
		}
		entries[name] = treeEntry{mode: fields[0], kind: fields[1], object: fields[2]}
		if len(entries) > maxFiles {
			return nil, failure("limit-exceeded", treeish+": library exceeds 10,000 files", nil)
		}
	}
	if !librarytree.UsesLegacyDirectory(maps.Keys(entries)) {
		return entries, nil
	}
	upgraded := make(map[string]treeEntry, len(entries))
	for name, entry := range entries {
		upgraded[librarytree.Upgrade(name)] = entry
	}
	return upgraded, nil
}

// withLegacyDirectory returns paths, repository paths, followed by the same path under the Trail Guide directory's
// name from before the rename for each one in the Trail Guide directory, so a listing finds either spelling.
func withLegacyDirectory(paths []string) []string {
	result := slices.Clone(paths)
	for _, name := range paths {
		if inside, found := strings.CutPrefix(name, librarytree.Directory+"/"); found {
			result = append(result, librarytree.LegacyDirectory+"/"+inside)
		}
	}
	return result
}

// changedRules reports, for each rule in working, whether its versioned files differ from the ones the latest
// library release published, comparing content as Git would store it. working maps a rule ID to the sorted
// working-tree paths of its Markdown file and asset directory's files.
func (g *libraryGit) changedRules(ctx context.Context, latest *publishedRelease, working map[string][]string) (map[string]bool, error) {
	paths := []string{}
	for _, names := range working {
		paths = append(paths, names...)
	}
	hashes, err := g.hashFiles(ctx, paths)
	if err != nil {
		return nil, err
	}
	changed := map[string]bool{}
	for id, names := range working {
		released := ruleFiles(id, maps.Keys(latest.files))
		changed[id] = !slices.Equal(names, released) || slices.ContainsFunc(names, func(name string) bool { return hashes[name] != latest.files[name] })
	}
	return changed, nil
}

// hashFiles returns the blob ID Git would store for each working-tree file, by its library path among paths,
// applying the repository's line-ending and filter attributes, so content compares equal to a committed copy.
func (g *libraryGit) hashFiles(ctx context.Context, paths []string) (map[string]string, error) {
	repository := make([]string, len(paths))
	for i, name := range paths {
		repository[i] = librarytree.RepositoryPath(name)
	}
	hashes, err := g.hashRepositoryFiles(ctx, repository)
	if err != nil {
		return nil, err
	}
	byLibraryPath := make(map[string]string, len(paths))
	for i, name := range paths {
		byLibraryPath[name] = hashes[repository[i]]
	}
	return byLibraryPath, nil
}

// hashRepositoryFiles is hashFiles for files named by their paths in the repository, such as the files of skills.
func (g *libraryGit) hashRepositoryFiles(ctx context.Context, paths []string) (map[string]string, error) {
	hashes := map[string]string{}
	if len(paths) == 0 {
		return hashes, nil
	}
	input := strings.Join(paths, "\n") + "\n"
	output, err := g.runner.Run(ctx, g.dir, []string{"hash-object", "--stdin-paths"}, len(paths)*65+4096, []byte(input))
	if err != nil {
		return nil, fmt.Errorf("hash the library's files: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(output.Output), "\n"), "\n")
	if output.Status != 0 || len(lines) != len(paths) {
		return nil, failure("git-failed", "Git could not hash the library's files", nil)
	}
	for i, line := range lines {
		if !objectID.MatchString(line) {
			return nil, failure("git-failed", "Git could not hash the library's files", nil)
		}
		hashes[paths[i]] = line
	}
	return hashes, nil
}

// changedSkills reports, for each of names, the folders of skills in tree, the tracked files of the working tree's
// skills/ folder, that base, the skills/ folder of the library release skill versions come from, also has, whether the
// skill's files differ from base's: in which files it has, in their content as Git would store it, or in their
// executable bit.
func (g *libraryGit) changedSkills(ctx context.Context, base map[string]treeEntry, tree *filetxn.Tree, names []string) (map[string]bool, error) {
	working := map[string][]string{}
	paths := []string{}
	if tree != nil {
		for name := range tree.Files {
			folder, _, _ := strings.Cut(name, "/")
			if slices.Contains(names, folder) {
				paths = append(paths, skillsDirectory+"/"+name)
				working[folder] = append(working[folder], name)
			}
		}
	}
	hashes, err := g.hashRepositoryFiles(ctx, paths)
	if err != nil {
		return nil, err
	}
	released := map[string][]string{}
	for name := range base {
		folder, _, _ := strings.Cut(name, "/")
		released[folder] = append(released[folder], name)
	}
	changed := map[string]bool{}
	for _, folder := range names {
		files := working[folder]
		slices.Sort(files)
		previous := released[folder]
		slices.Sort(previous)
		changed[folder] = !slices.Equal(files, previous) || slices.ContainsFunc(files, func(name string) bool {
			entry := base[name]
			mode := "100644"
			if slices.Contains(tree.Executable, name) {
				mode = "100755"
			}
			return entry.kind != "blob" || entry.mode != mode || entry.object != hashes[skillsDirectory+"/"+name]
		})
	}
	return changed, nil
}

// trackedSkills returns tree, the working tree's skills/ folder, without the files Git ignores and doesn't track,
// which no commit holds, and without the folders that held only such files, so that a library release records
// exactly the skills its commit delivers. A nil g, a library outside Git, keeps every file.
func (g *libraryGit) trackedSkills(ctx context.Context, tree *filetxn.Tree) (*filetxn.Tree, error) {
	if g == nil || tree == nil {
		return tree, nil
	}
	paths := make([]string, 0, len(tree.Files))
	for name := range tree.Files {
		paths = append(paths, skillsDirectory+"/"+name)
	}
	ignored, err := g.ignoredFiles(ctx, paths)
	if err != nil || len(ignored) == 0 {
		return tree, err
	}
	tracked := &filetxn.Tree{Files: map[string][]byte{}, Directories: []string{}}
	for name, data := range tree.Files {
		if !ignored[skillsDirectory+"/"+name] {
			tracked.Files[name] = data
		}
	}
	for _, name := range tree.Executable {
		if _, ok := tracked.Files[name]; ok {
			tracked.Executable = append(tracked.Executable, name)
		}
	}
	for _, directory := range tree.Directories {
		if !holdsFiles(tree.Files, directory) || holdsFiles(tracked.Files, directory) {
			tracked.Directories = append(tracked.Directories, directory)
		}
	}
	return tracked, nil
}

// holdsFiles reports whether files, by path, has one in directory or a folder in it.
func holdsFiles(files map[string][]byte, directory string) bool {
	for name := range files {
		if strings.HasPrefix(name, directory+"/") {
			return true
		}
	}
	return false
}

// ignoredFiles reports which of paths, repository paths of files in the working tree, Git ignores and doesn't track.
func (g *libraryGit) ignoredFiles(ctx context.Context, paths []string) (map[string]bool, error) {
	ignored := map[string]bool{}
	if len(paths) == 0 {
		return ignored, nil
	}
	output, err := g.runner.Run(ctx, g.dir, []string{"check-ignore", "-z", "--stdin"}, len(paths)*(4096+1), []byte(strings.Join(paths, "\x00")+"\x00"))
	if err != nil {
		return nil, fmt.Errorf("find the library's ignored files: %w", err)
	}
	// check-ignore exits with 1 when it ignores none of the paths.
	if output.Status != 0 && output.Status != 1 {
		return nil, failure("git-failed", "Git could not tell which of the library's files it ignores", nil)
	}
	for name := range bytes.SplitSeq(bytes.TrimSuffix(output.Output, []byte{0}), []byte{0}) {
		if len(name) > 0 {
			ignored[string(name)] = true
		}
	}
	return ignored, nil
}
