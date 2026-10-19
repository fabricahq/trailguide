// Fetch commits, tags, and selected blobs of one library into an owned partial repository, without a checkout.

package imports

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
)

// Options selects trusted process settings. Zero values use Git from PATH and a 120-second deadline.
// Environment, when nonnil, replaces the inherited environment before isolation overrides are applied.
// These are application settings, never remote-library or configuration-file fields.
type Options struct {
	GitPath     string
	Environment []string
	Timeout     time.Duration
	// GroupMetadata lists groups whose metadata each import also returns, in Library.GroupMetadata, when the
	// revision that supplies the source's library-wide files has it, such as the groups of local rules that have no
	// local metadata. The import reads nothing else for them.
	GroupMetadata []string
}

// repository owns a temporary bare repository that fetches one library as a partial clone: commits and trees
// arrive with depth 1 and no blobs, and blobs arrive only when requested. It isn't safe for concurrent use.
type repository struct {
	// source names the configured source in diagnostics.
	source    string
	directory string
	runner    gitexec.Runner
	// present lists the commits already fetched, so later fetches request only missing ones.
	present map[string]bool
	// trees caches each fetched commit's parsed file tree by library path, and owned indexes it by the rule each
	// file belongs to. legacy lists the commits that keep the library in the layout before version 2. raw caches
	// each fetched commit's parsed file tree by repository path.
	trees  map[string]map[string]treeEntry
	raw    map[string]map[string]treeEntry
	owned  map[string]map[string]map[string]treeEntry
	legacy map[string]bool
}

// fetchRemote is the name of the library's remote; Git records a partial clone's promisor by remote name.
const fetchRemote = "origin"

// openRepository validates the source's address and creates the temporary repository. The caller owns Close
// on success; failure removes temporary state.
func openRepository(ctx context.Context, source rules.Source, options Options) (_ *repository, err error) {
	if err := ctx.Err(); err != nil {
		return nil, gitexec.ContextFailure(err)
	}
	address, _ := json.Marshal(source.Repository)
	if _, err := rules.ParseRepository(address, "sources."+source.Name+".repository"); err != nil {
		return nil, err
	}
	runner, err := gitexec.Isolated(gitexec.Options{GitPath: options.GitPath, Environment: options.Environment})
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "trailguide-git-*")
	if err != nil {
		return nil, fail("temporary-storage", "Cannot create temporary Git storage.", err)
	}
	repo := &repository{source: source.Name, directory: dir, runner: runner, present: map[string]bool{}, trees: map[string]map[string]treeEntry{}, raw: map[string]map[string]treeEntry{}, owned: map[string]map[string]map[string]treeEntry{}, legacy: map[string]bool{}}
	defer func() {
		if err != nil {
			err = errors.Join(err, repo.Close())
		}
	}()
	if err := runner.RequireVersion(ctx, dir); err != nil {
		return nil, err
	}
	if _, err = runner.Output(ctx, dir, []string{"init", "--bare", "--quiet", "--template="}, 4096); err != nil {
		return nil, err
	}
	if _, err = runner.Output(ctx, dir, []string{"remote", "add", fetchRemote, source.Repository}, 4096); err != nil {
		return nil, err
	}
	return repo, nil
}

// Close removes all temporary repository state. Callers must handle cleanup failures.
func (r *repository) Close() error {
	if r == nil || r.directory == "" {
		return nil
	}
	if err := os.RemoveAll(r.directory); err != nil {
		return fail("cleanup-failed", "Could not remove the temporary Git repository.", err)
	}
	r.directory = ""
	return nil
}

// fetch requests refspecs from the remote without blobs other than those it names, returning Git's result, whose
// Status is 0 when it succeeded. With commits, commits arrive without their history. Refspecs travel on standard
// input, so their number doesn't bound the command line.
func (r *repository) fetch(ctx context.Context, refspecs []string, commits bool) (gitexec.Result, error) {
	args := []string{"-c", "fetch.negotiationAlgorithm=noop", "fetch", "--quiet", "--filter=blob:none", "--no-tags", "--no-auto-gc", "--no-recurse-submodules", "--no-write-fetch-head", "--stdin", fetchRemote}
	if commits {
		args = slices.Insert(args, 4, "--depth=1")
	}
	return r.runner.Run(ctx, r.directory, args, 1<<20, []byte(strings.Join(refspecs, "\n")+"\n"))
}

// unreachable reports why a failed fetch failed: a repository that can't be reached, as remoteFailure explains
// it, or nil when the repository answers, so the requested revision is missing.
func (r *repository) unreachable(ctx context.Context) error {
	result, err := r.runner.Run(ctx, r.directory, []string{"ls-remote", "--exit-code", fetchRemote, "HEAD"}, 64<<10, nil)
	if err != nil {
		return err
	}
	if result.Status == 0 || result.Status == 2 {
		return nil
	}
	return remoteFailure(result.Diagnostics)
}

// bareRef returns the full name of the branch or tag that the source's bare ref, such as main, names in the
// repository. It fails with code ambiguous-ref when the repository has both, and with version-not-found when it has
// neither.
func (r *repository) bareRef(ctx context.Context, source rules.Source) (string, error) {
	branch, tag := "refs/heads/"+source.Ref.Name(), "refs/tags/"+source.Ref.Name()
	listed, err := r.runner.Run(ctx, r.directory, []string{"ls-remote", "--exit-code", fetchRemote, branch, tag}, 64<<10, nil)
	if err != nil {
		return "", err
	}
	// ls-remote exits with 2 when the repository answers and lists no ref the patterns match.
	if listed.Status != 0 && listed.Status != 2 {
		return "", remoteFailure(listed.Diagnostics)
	}
	found := map[string]bool{}
	for line := range strings.Lines(string(listed.Output)) {
		if _, name, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "\t"); ok && (name == branch || name == tag) {
			found[name] = true
		}
	}
	switch {
	case found[branch] && found[tag]:
		return "", fail("ambiguous-ref", fmt.Sprintf("sources.%s.ref: the repository has both a branch and a tag named %s. Set sources.%s.ref to %s for the branch, or to %s for the tag.", source.Name, source.Ref, source.Name, branch, tag), nil)
	case found[branch]:
		return branch, nil
	case found[tag]:
		return tag, nil
	}
	return "", fail("version-not-found", missingRef(source), nil)
}

// missingRef explains that the repository has no revision the source's ref names.
func missingRef(source rules.Source) string {
	ref := source.Ref
	switch {
	case ref.Bare():
		return fmt.Sprintf("sources.%s.ref: the repository has no branch or tag %s; check the ref, and whether a branch of that name was renamed or deleted.", source.Name, ref)
	case ref.Kind() == rules.GitRefBranch:
		return fmt.Sprintf("sources.%s.ref: the repository has no branch %s; check the ref, and whether the branch was renamed or deleted.", source.Name, ref)
	}
	return fmt.Sprintf("sources.%s.ref: the repository has no %s %s; check the ref.", source.Name, ref.Kind(), ref)
}

// fetchCommits fetches each commit that isn't present yet. A commit the server says it doesn't have fails with
// code version-not-found and missing's explanation; any other failure is explained as fetchFailure does.
func (r *repository) fetchCommits(ctx context.Context, commits []string, missing string) error {
	wanted := []string{}
	for _, commit := range commits {
		if !r.present[commit] && !slices.Contains(wanted, commit) {
			wanted = append(wanted, commit)
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	slices.Sort(wanted)
	fetched, err := r.fetch(ctx, wanted, true)
	if err != nil {
		return err
	}
	if fetched.Status != 0 {
		return fetchFailure(fetched.Diagnostics, "a commit that the source record names", missing)
	}
	for _, commit := range wanted {
		if _, err := r.commit(ctx, commit); err != nil {
			return err
		}
	}
	return nil
}

// fetchRef fetches the source's ref and returns its commit and what the ref named: a commit, or a tag or branch,
// which for a bare name, such as main, the repository decides, as bareRef does. A revision the repository doesn't
// have fails with code version-not-found.
func (r *repository) fetchRef(ctx context.Context, source rules.Source) (string, rules.GitRefKind, error) {
	ref := source.Ref
	name := ref.Canonical()
	if ref.Bare() {
		var err error
		if name, err = r.bareRef(ctx, source); err != nil {
			return "", "", err
		}
	}
	return r.fetchName(ctx, source, name)
}

// fetchName fetches name, the full name of the branch or tag the source's ref names, or the ref's commit, and
// returns its commit and what it named, as fetchRef does.
func (r *repository) fetchName(ctx context.Context, source rules.Source, name string) (string, rules.GitRefKind, error) {
	ref := source.Ref
	kind := ref.Kind()
	if strings.HasPrefix(name, "refs/heads/") {
		kind = rules.GitRefBranch
	}
	refspec := name
	if kind != rules.GitRefCommit {
		refspec = "+" + name + ":" + name
	}
	fetched, err := r.fetch(ctx, []string{refspec}, true)
	if err != nil {
		return "", "", err
	}
	if fetched.Status != 0 {
		return "", "", fetchFailure(fetched.Diagnostics, "sources."+source.Name+".ref", missingRef(source))
	}
	commit, err := r.commit(ctx, name)
	if err != nil {
		return "", "", err
	}
	if kind == rules.GitRefCommit && commit != ref.Canonical() {
		return "", "", fail("git-failed", "Git returned a different or unsupported commit identity.", nil)
	}
	return commit, kind, nil
}

// commit resolves a fetched revision to its commit and marks it present; a tag of something else is unsupported.
func (r *repository) commit(ctx context.Context, revision string) (string, error) {
	resolved, err := r.runner.Run(ctx, r.directory, []string{"rev-parse", "--verify", "--quiet", "--end-of-options", revision + "^{commit}"}, 4096, nil)
	if err != nil {
		return "", err
	}
	if resolved.Status != 0 {
		return "", fail("unsupported-content", "Requested tag does not resolve to a commit.", nil)
	}
	commit := strings.TrimSpace(string(resolved.Output))
	if !validObjectID(commit) {
		return "", fail("git-failed", "Git returned a different or unsupported commit identity.", nil)
	}
	r.present[commit] = true
	return commit, nil
}

// tree returns the parsed file tree of a fetched commit by library path, as libraryTree places it, listing it once.
// Listing needs no blobs.
func (r *repository) tree(ctx context.Context, commit string) (map[string]treeEntry, error) {
	if entries, ok := r.trees[commit]; ok {
		return entries, nil
	}
	parsed, err := r.repositoryTree(ctx, commit)
	if err != nil {
		return nil, err
	}
	entries, legacy := libraryTree(parsed)
	r.trees[commit], r.legacy[commit] = entries, legacy
	return entries, nil
}

// repositoryTree returns the parsed file tree of a fetched commit by repository path, listing it once. A commit that
// keeps its Trail Guide directory under the name from before the rename lists it as .trailguide/, as librarytree.Upgrade
// spells it.
func (r *repository) repositoryTree(ctx context.Context, commit string) (map[string]treeEntry, error) {
	if entries, ok := r.raw[commit]; ok {
		return entries, nil
	}
	data, err := r.runner.Output(ctx, r.directory, []string{"ls-tree", "-r", "-z", "--full-tree", commit}, maxTreeBytes)
	if err != nil {
		return nil, err
	}
	parsed, err := parseTree(data)
	if err != nil {
		return nil, err
	}
	if librarytree.UsesLegacyDirectory(maps.Keys(parsed)) {
		upgraded := make(map[string]treeEntry, len(parsed))
		for name, entry := range parsed {
			entry.name = librarytree.Upgrade(name)
			upgraded[entry.name] = entry
		}
		parsed = upgraded
	}
	r.raw[commit] = parsed
	return parsed, nil
}

// prefetch fetches the blobs among entries that aren't present yet in one request, so reading them later needs
// no request per file. Entries that aren't ordinary files, such as submodules, are skipped for the loader to
// reject if it reads them. Reading a blob that wasn't prefetched still fetches it on its own.
func (r *repository) prefetch(ctx context.Context, entries []treeEntry) error {
	objects := []string{}
	for _, entry := range entries {
		if entry.mode.IsRegular() {
			objects = append(objects, entry.object)
		}
	}
	if len(objects) == 0 {
		return nil
	}
	listing, err := r.runner.Output(ctx, r.directory, []string{"cat-file", "--batch-all-objects", "--batch-check=%(objectname)"}, 64<<20)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for line := range bytes.Lines(listing) {
		present[string(bytes.TrimSuffix(line, []byte("\n")))] = true
	}
	missing := []string{}
	for _, object := range objects {
		if !present[object] {
			present[object] = true
			missing = append(missing, object)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	fetched, err := r.fetch(ctx, missing, false)
	if err != nil {
		return err
	}
	if fetched.Status != 0 {
		return gitFailure("git-failed", "Could not fetch library files.", fetched.Diagnostics)
	}
	return nil
}

// validObjectID accepts canonical SHA-1 object IDs supported by the current source format.
func validObjectID(text string) bool {
	decoded, err := hex.DecodeString(text)
	return err == nil && len(decoded) == 20 && fmt.Sprintf("%x", decoded) == text
}
