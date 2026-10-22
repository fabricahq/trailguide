// Compare the library author's clone with its upstream remote, then create and push one release tag.

package library

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/releasetag"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// maxRemoteListing bounds the output of listing the remote's default branch and release tags.
const maxRemoteListing = 8 * 1024 * 1024

// upstream is where the checked-out branch publishes.
type upstream struct {
	// branch is the checked-out branch's short name, such as main.
	branch string
	// remote is the upstream remote's name, such as origin, and url its fetch URL after insteadOf rewriting.
	remote, url string
	// pushURL is the one URL git push uses for remote, naming the same repository as url.
	pushURL string
	// ref is the branch on the remote, such as refs/heads/main.
	ref string
	// tracking is the local remote-tracking branch, such as refs/remotes/origin/main, which the fetch updates.
	tracking string
}

// remoteState is what the upstream remote holds, read without fetching.
type remoteState struct {
	// defaultBranch is the ref the remote's HEAD names, such as refs/heads/main.
	defaultBranch string
	// head is the commit at the upstream branch, or "" when the remote lacks it.
	head string
	// tags maps each release/<number> tag's number to its object ID.
	tags map[int]string
}

// latest returns the highest release number on the remote, or 0 before the first library release.
func (r remoteState) latest() int {
	if len(r.tags) == 0 {
		return 0
	}
	return slices.Max(slices.Collect(maps.Keys(r.tags)))
}

// upstream finds the checked-out branch and its upstream remote. It fails on a detached HEAD, a branch
// without an upstream on a remote, and a remote without a URL.
func (g *libraryGit) upstream(ctx context.Context) (upstream, error) {
	symbolic, err := g.runner.Run(ctx, g.dir, []string{"symbolic-ref", "--quiet", "HEAD"}, 4096, nil)
	if err != nil {
		return upstream{}, fmt.Errorf("find the checked-out branch: %w", err)
	}
	ref := strings.TrimSpace(string(symbolic.Output))
	branch, onBranch := strings.CutPrefix(ref, "refs/heads/")
	if symbolic.Status != 0 || !onBranch {
		return upstream{}, failure("detached-head", "HEAD isn't on a branch. Check out the library's default branch, then run trailguide library release again.", nil)
	}
	listing, err := g.runner.Output(ctx, g.dir, []string{"for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)%00%(upstream)", ref}, 64*1024)
	if err != nil {
		return upstream{}, fmt.Errorf("find the upstream of branch=%q: %w", branch, err)
	}
	fields := strings.Split(strings.TrimSuffix(string(listing), "\n"), "\x00")
	// The remote-tracking branch, fields[2], is empty unless the remote's fetch refspec maps the upstream branch.
	if len(fields) != 3 || fields[0] == "" || fields[0] == "." || !strings.HasPrefix(fields[1], "refs/heads/") || fields[2] == "" {
		return upstream{}, failure("no-upstream", branch+" has no upstream branch on a remote, so there's nothing to compare it with. Push it with git push --set-upstream, then run trailguide library release again.", nil)
	}
	result := upstream{branch: branch, remote: fields[0], ref: fields[1], tracking: fields[2]}
	url, err := g.runner.Output(ctx, g.dir, []string{"remote", "get-url", "--", result.remote}, 64*1024)
	if err != nil {
		return upstream{}, fmt.Errorf("find the URL of remote=%q: %w", result.remote, err)
	}
	result.url = strings.TrimSpace(string(url))
	if result.pushURL, err = g.pushDestination(ctx, result.remote, result.url); err != nil {
		return upstream{}, err
	}
	return result, nil
}

// pushDestination returns the one URL git push would push remote to, which must name the repository remote
// fetches from, fetchURL, so the release tag goes where its number and content were checked. It refuses
// several push URLs, which could publish the tag to only some of them.
func (g *libraryGit) pushDestination(ctx context.Context, remote, fetchURL string) (string, error) {
	listing, err := g.runner.Output(ctx, g.dir, []string{"remote", "get-url", "--push", "--all", "--", remote}, 64*1024)
	if err != nil {
		return "", fmt.Errorf("find the push URLs of remote=%q: %w", remote, err)
	}
	urls := strings.Fields(string(listing))
	if len(urls) != 1 {
		return "", failure("push-destination", remote+" pushes to "+strconv.Itoa(len(urls))+" URLs, so a library release could reach only some of them. Configure a single push URL for "+remote+", such as with git config --unset-all remote."+remote+".pushurl, then run trailguide library release again.", nil)
	}
	if !sameRepository(fetchURL, urls[0]) {
		return "", failure("push-destination", remote+" fetches from "+displayRepository(fetchURL)+" but pushes to "+displayRepository(urls[0])+". A library release is checked against the repository it's fetched from, so it must be pushed there too. Remove the push URL with git config --unset-all remote."+remote+".pushurl, or set it to the fetch repository, then run trailguide library release again.", nil)
	}
	return urls[0], nil
}

// sameRepository reports whether two remote URLs name the same repository, comparing recognized repositories
// by identity, such as github.com/acme/rules for its HTTPS and SSH URLs, other URLs without credentials, and
// URLs that can't be parsed exactly.
func sameRepository(a, b string) bool {
	first, firstOK := parseRemote(a)
	second, secondOK := parseRemote(b)
	if firstOK && secondOK {
		return first.Identity == second.Identity
	}
	firstAddress, firstParsed := withoutCredentials(a)
	secondAddress, secondParsed := withoutCredentials(b)
	if !firstParsed || !secondParsed {
		return a == b
	}
	return firstAddress == secondAddress
}

// requireCommitterIdentity fails before anything changes when Git has no tagger identity for the tag.
// It returns the identity as the tag's tagger line records it, such as "Name <email> 1727640000 -0700".
func (g *libraryGit) requireCommitterIdentity(ctx context.Context) (string, error) {
	result, err := g.runner.Run(ctx, g.dir, []string{"var", "GIT_COMMITTER_IDENT"}, 64*1024, nil)
	if err != nil {
		return "", fmt.Errorf("find Git's committer identity: %w", err)
	}
	if result.Status != 0 {
		return "", failure("git-identity", "Git doesn't know your name and email, which the release tag records as its tagger. Set user.name and user.email, or GIT_COMMITTER_NAME and GIT_COMMITTER_EMAIL, then run trailguide library release again.", nil)
	}
	return strings.TrimSpace(string(result.Output)), nil
}

// tagObjectSize is the size of the unsigned annotated tag Git writes for message on commit, with tagger as
// its tagger line.
func tagObjectSize(name, commit, tagger string, message []byte) int {
	return len("object "+commit+"\ntype commit\ntag "+name+"\ntagger "+tagger+"\n\n") + len(message)
}

// requireTagSize refuses a release tag object larger than the 8 MiB that reading release tags accepts.
func requireTagSize(name string, size int) error {
	if size <= releasetag.MaxBytes {
		return nil
	}
	return failure("release-too-large", name+" would be a tag of "+strconv.Itoa(size)+" bytes, but release tags can be at most 8 MiB (8,388,608 bytes), or later library releases couldn't read it. Publish the pending changes in smaller library releases, or shorten their change notes' summaries, then run trailguide library release again.", nil)
}

// headCommit returns the checked-out commit; a branch without commits fails.
func (g *libraryGit) headCommit(ctx context.Context) (string, error) {
	result, err := g.runner.Run(ctx, g.dir, []string{"rev-parse", "--verify", "--quiet", "HEAD^{commit}"}, 4096, nil)
	if err != nil {
		return "", fmt.Errorf("find the library's HEAD commit: %w", err)
	}
	head := strings.TrimSpace(string(result.Output))
	if result.Status != 0 || !objectID.MatchString(head) {
		return "", failure("no-commits", "the library has no commits yet. Commit and push it, then run trailguide library release again.", nil)
	}
	return head, nil
}

// readRemote lists the remote's default branch, the upstream branch's commit, and its release tags, as
// parseRemoteListing reads them.
func (g *libraryGit) readRemote(ctx context.Context, u upstream) (remoteState, error) {
	result, err := g.runner.Run(ctx, g.dir, []string{"ls-remote", "--symref", "--", u.remote, "HEAD", u.ref, "refs/tags/release/*"}, maxRemoteListing, nil)
	if err != nil {
		return remoteState{}, fmt.Errorf("list the branches and release tags of remote=%q: %w", u.remote, err)
	}
	if result.Status != 0 {
		return remoteState{}, fetchFailure("read", u.remote, result.Diagnostics)
	}
	return parseRemoteListing(string(result.Output), u)
}

// parseRemoteListing reads git ls-remote --symref output for the remote's HEAD, the upstream branch, and its
// release tags. It fails when the listing is malformed, has more than 20,000 tag records, or shows that the
// upstream branch isn't the remote's default branch.
// The advertised default branch's name comes from the server, so a refusal never shows it.
func parseRemoteListing(listing string, u upstream) (remoteState, error) {
	state := remoteState{tags: map[int]string{}}
	records := 0
	for line := range strings.Lines(listing) {
		value, name, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "\t")
		if !ok {
			return remoteState{}, failure("git-failed", "Git listed "+u.remote+"'s references in an unexpected format", nil)
		}
		if target, symbolic := strings.CutPrefix(value, "ref: "); symbolic {
			if name == "HEAD" {
				state.defaultBranch = target
			}
			continue
		}
		if !objectID.MatchString(value) {
			return remoteState{}, failure("git-failed", "Git listed "+u.remote+"'s references in an unexpected format", nil)
		}
		if name == u.ref {
			state.head = value
		}
		// Like imports, count every tag record, including the peeled ^{} records of annotated tags.
		if strings.HasPrefix(name, "refs/tags/") {
			if records++; records > maxReleaseTags {
				return remoteState{}, failure("limit-exceeded", u.remote+"'s listing of release tags exceeds 20,000 records", nil)
			}
		}
		// Trail Guide ignores other tags under release/, such as release/01, and peeled ^{} entries.
		if number, err := trailguide.ParseReleaseTag(strings.TrimPrefix(name, "refs/tags/")); err == nil && strings.HasPrefix(name, "refs/tags/") {
			state.tags[number] = value
		}
	}
	if state.defaultBranch == "" {
		return remoteState{}, failure("not-default-branch", u.remote+" doesn't report a default branch, so trailguide library release can't confirm it's publishing from it.", nil)
	}
	if u.ref != state.defaultBranch {
		return remoteState{}, failure("not-default-branch", "library releases are published from "+u.remote+"'s default branch, but "+u.branch+" tracks "+strings.TrimPrefix(u.ref, "refs/heads/")+", which isn't it. Check out the default branch, which git remote show "+u.remote+" names, then run trailguide library release again.", nil)
	}
	return state, nil
}

// localReleaseTags maps the number of each release/<number> tag in the clone, reachable or not, to the tag.
func (g *libraryGit) localReleaseTags(ctx context.Context) (map[int]releasetag.Tag, error) {
	listed, err := g.listReleaseTags(ctx, "")
	if err != nil {
		return nil, err
	}
	tags := map[int]releasetag.Tag{}
	for _, tag := range listed {
		tags[tag.Number] = tag
	}
	return tags, nil
}

// syncReleaseTags fetches the upstream branch and every release tag the clone lacks. It refuses when a
// release tag in the clone differs from the remote's, or when the clone has a release tag the remote lacks,
// unless that tag is on head and numbered one past the remote's latest, as a run that stopped before pushing its
// tag leaves it; the caller must still check that tag's content. It also refuses when the fetch brings a branch
// tip or release tag other than the ones remote lists, so every later comparison uses what was fetched. It
// returns the unpushed tag, or a zero Tag when there is none.
func (g *libraryGit) syncReleaseTags(ctx context.Context, u upstream, remote remoteState, head string) (releasetag.Tag, error) {
	local, err := g.localReleaseTags(ctx)
	if err != nil {
		return releasetag.Tag{}, err
	}
	refspecs := []string{"+" + u.ref + ":" + u.tracking}
	for _, number := range slices.Sorted(maps.Keys(remote.tags)) {
		name := "release/" + strconv.Itoa(number)
		tag, ok := local[number]
		switch {
		case !ok:
			refspecs = append(refspecs, "refs/tags/"+name+":refs/tags/"+name)
		case tag.Object != remote.tags[number]:
			return releasetag.Tag{}, failure("release-tag-mismatch", name+" in this clone differs from "+name+" on "+u.remote+". Release tags must never change, because projects may have imported them. Find out which one is the original and restore it; to discard this clone's copy, run git tag --delete "+name+".", nil)
		}
	}
	unpublished := 0
	for _, number := range slices.Sorted(maps.Keys(local)) {
		if _, published := remote.tags[number]; published {
			continue
		}
		name := "release/" + strconv.Itoa(number)
		if number != remote.latest()+1 || local[number].Target != head {
			return releasetag.Tag{}, failure("release-tag-mismatch", name+" exists in this clone but not on "+u.remote+". Only trailguide library release creates release tags; if you created it by hand, delete it with git tag --delete "+name+", then run trailguide library release again.", nil)
		}
		unpublished = number
	}
	result, err := g.runner.Run(ctx, g.dir, []string{"fetch", "--no-tags", "--stdin", "--", u.remote}, 1024*1024, []byte(strings.Join(refspecs, "\n")+"\n"))
	if err != nil {
		return releasetag.Tag{}, fmt.Errorf("fetch remote=%q: %w", u.remote, err)
	}
	if result.Status != 0 {
		return releasetag.Tag{}, fetchFailure("fetch from", u.remote, result.Diagnostics)
	}
	if err := g.requireFetched(ctx, u, remote); err != nil {
		return releasetag.Tag{}, err
	}
	return local[unpublished], nil
}

// requireFetched refuses when what the fetch brought differs from what listing the remote showed: the branch
// moved, or a release tag changed, while trailguide library release was reading the remote.
func (g *libraryGit) requireFetched(ctx context.Context, u upstream, remote remoteState) error {
	changed := u.remote + " changed while trailguide library release was reading it: "
	tip, err := g.runner.Output(ctx, g.dir, []string{"rev-parse", "--verify", "--end-of-options", u.tracking + "^{commit}"}, 4096)
	if err != nil {
		return fmt.Errorf("read the fetched branch=%q: %w", u.tracking, err)
	}
	if strings.TrimSpace(string(tip)) != remote.head {
		return failure("remote-changed", changed+"someone pushed to "+strings.TrimPrefix(u.ref, "refs/heads/")+". Pull their changes, then run trailguide library release again.", nil)
	}
	local, err := g.localReleaseTags(ctx)
	if err != nil {
		return err
	}
	for _, number := range slices.Sorted(maps.Keys(remote.tags)) {
		if local[number].Object != remote.tags[number] {
			return failure("remote-changed", changed+"release/"+strconv.Itoa(number)+" changed. Release tags must never change; find out who changed it, then run trailguide library release again.", nil)
		}
	}
	return nil
}

// requireCurrent refuses unless head is the commit at the remote's branch, explaining how they differ.
func (g *libraryGit) requireCurrent(ctx context.Context, u upstream, remoteHead, head string) error {
	remoteName := u.remote + "/" + strings.TrimPrefix(u.ref, "refs/heads/")
	if remoteHead == "" {
		return failure("branch-differs", remoteName+" doesn't exist. Push "+u.branch+", then run trailguide library release again.", nil)
	}
	if remoteHead == head {
		return nil
	}
	counts, err := g.runner.Output(ctx, g.dir, []string{"rev-list", "--left-right", "--count", head + "..." + remoteHead}, 4096)
	if err != nil {
		return fmt.Errorf("compare branch=%q with %s: %w", u.branch, remoteName, err)
	}
	fields := strings.Fields(string(counts))
	if len(fields) != 2 {
		return failure("git-failed", "Git compared the branches in an unexpected format", nil)
	}
	ahead, aheadErr := strconv.ParseUint(fields[0], 10, 63)
	behind, behindErr := strconv.ParseUint(fields[1], 10, 63)
	if aheadErr != nil || behindErr != nil {
		return failure("git-failed", "Git compared the branches in an unexpected format", nil)
	}
	switch {
	case behind == 0:
		return failure("branch-differs", u.branch+" has "+commits(ahead)+" that "+remoteName+" doesn't. A library release publishes only pushed commits: push them, then run trailguide library release again.", nil)
	case ahead == 0:
		return failure("branch-differs", remoteName+" has "+commits(behind)+" that "+u.branch+" doesn't. Pull them, then run trailguide library release again.", nil)
	}
	return failure("branch-differs", u.branch+" and "+remoteName+" have diverged: each has commits the other doesn't. Reconcile them, then run trailguide library release again.", nil)
}

// commits counts commits in prose, such as "1 commit" or "3 commits".
func commits(count uint64) string {
	if count == 1 {
		return "1 commit"
	}
	return strconv.FormatUint(count, 10) + " commits"
}

// requireCommitted refuses unless head commits exactly the bytes check read, as regular files, including the
// library's tracked skill files with their executable bits, so the library release publishes what check validated,
// even where a Git filter such as Git LFS stores other content.
func (g *libraryGit) requireCommitted(ctx context.Context, head string, input checkInput) error {
	library, err := g.libraryEntries(ctx, head, libraryPaths(rules.LicensePaths(input.license)))
	if err != nil {
		return fmt.Errorf("list the files of commit=%s: %w", head, err)
	}
	skillEntries, err := g.treeEntries(ctx, head, []string{skillsDirectory})
	if err != nil {
		return fmt.Errorf("list the skills of commit=%s: %w", head, err)
	}
	format, err := g.runner.Output(ctx, g.dir, []string{"rev-parse", "--show-object-format"}, 4096)
	if err != nil {
		return fmt.Errorf("find the library's object format: %w", err)
	}
	// Both maps are keyed by repository path, which the problems name.
	entries, captured := skillEntries, map[string][]byte{}
	for name, entry := range library {
		entries[librarytree.RepositoryPath(name)] = entry
	}
	for _, files := range []map[string][]byte{input.tree.Files, input.notes} {
		for name, data := range files {
			captured[librarytree.RepositoryPath(name)] = data
		}
	}
	// Check dropped the skill files Git ignores, such as a cache a script wrote, which aren't part of a skill.
	executable := map[string]bool{}
	if input.skills != nil {
		for name, data := range input.skills.Files {
			captured[skillsDirectory+"/"+name] = data
		}
		for _, name := range input.skills.Executable {
			executable[skillsDirectory+"/"+name] = true
		}
	}
	var problems []string
	for location, entry := range entries {
		data, ok := captured[location]
		switch {
		case !ok:
			problems = append(problems, location+" was deleted, and the deletion isn't committed")
		case entry.kind != "blob" || (entry.mode != "100644" && entry.mode != "100755"):
			problems = append(problems, location+" is committed as a symbolic link or submodule, not a file")
		case entry.object != blobID(strings.TrimSpace(string(format)), data):
			problems = append(problems, location+" differs from its committed copy")
		case strings.HasPrefix(location, skillsDirectory+"/") && executable[location] != (entry.mode == "100755"):
			// A skill's version covers its files' executable bits, so the commit must have them as check read them.
			problems = append(problems, location+" has an executable bit that differs from its committed copy")
		}
	}
	for location := range captured {
		if _, ok := entries[location]; !ok {
			problems = append(problems, location+" isn't committed")
		}
	}
	if len(problems) > 0 {
		slices.Sort(problems)
		return failure("uncommitted-changes", "a library release publishes the checked-out commit exactly, but these library files differ from it:\n  - "+strings.Join(problems, "\n  - ")+"\nCommit and push your changes, or discard them. Files that a Git filter changes, such as Git LFS files, can't be published. Then run trailguide library release again.", nil)
	}
	return nil
}

// blobID returns the ID Git gives a blob holding exactly data, with no filters or line-ending conversion, in the
// repository's object format, sha1 or sha256.
func blobID(format string, data []byte) string {
	header := "blob " + strconv.Itoa(len(data)) + "\x00"
	if format == "sha256" {
		sum := sha256.Sum256(append([]byte(header), data...))
		return hex.EncodeToString(sum[:])
	}
	sum := sha1.Sum(append([]byte(header), data...))
	return hex.EncodeToString(sum[:])
}

// libraryPaths are the library paths holding every library-owned file and change note, given the license and
// notice files, terms.
func libraryPaths(terms []string) []string {
	return append([]string{librarytree.Manifest, "assets", "practices", "techs", changesDirectory}, terms...)
}

// changedLibraryFiles lists the library-wide files that changed between the latest library release, which is
// nil before the first one, and head. terms are head's declared license and notice files; the latest library
// release's declared ones count too, so renaming or removing one lists the old path.
func (g *libraryGit) changedLibraryFiles(ctx context.Context, head string, latest *publishedRelease, terms []string) ([]string, error) {
	released := map[string]string{}
	if latest != nil {
		previous, err := g.releasedTerms(ctx, latest)
		if err != nil {
			return nil, err
		}
		for _, name := range previous {
			if !slices.Contains(terms, name) {
				terms = append(slices.Clip(terms), name)
			}
		}
		if released, err = g.libraryFiles(ctx, latest.object, libraryPaths(terms)); err != nil {
			return nil, fmt.Errorf("list the files of %s: %w", latest.tagName(), err)
		}
	}
	current, err := g.libraryFiles(ctx, head, libraryPaths(terms))
	if err != nil {
		return nil, fmt.Errorf("list the files of commit=%s: %w", head, err)
	}
	return diffLibraryFiles(current, released, terms), nil
}

// pendingLibraryFiles lists the library-wide files among files, the working tree's, that differ from the latest
// library release, including deleted ones, in path order, as changedLibraryFiles would once they're committed.
// terms are the working tree's declared license and notice files. It lists none for a library outside Git, which g
// is nil for, and before the first library release, when latest is nil.
func (g *libraryGit) pendingLibraryFiles(ctx context.Context, latest *publishedRelease, files map[string][]byte, terms []string) ([]string, error) {
	if g == nil || latest == nil {
		return []string{}, nil
	}
	previous, err := g.releasedTerms(ctx, latest)
	if err != nil {
		return nil, err
	}
	for _, name := range previous {
		if !slices.Contains(terms, name) {
			terms = append(slices.Clip(terms), name)
		}
	}
	released, err := g.libraryFiles(ctx, latest.object, libraryPaths(terms))
	if err != nil {
		return nil, fmt.Errorf("list the files of %s: %w", latest.tagName(), err)
	}
	names := []string{}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if libraryWide(name, terms) {
			names = append(names, name)
		}
	}
	current, err := g.hashFiles(ctx, names)
	if err != nil {
		return nil, err
	}
	if data, ok := files[librarytree.Manifest]; ok {
		current[librarytree.Manifest] = manifestDigest(data)
	}
	return diffLibraryFiles(current, released, terms), nil
}

// releasedTerms returns the library paths of the license and notice files the latest library release's manifest
// declared. A library release made before the layout of version 2 kept its manifest in rule-library.yaml, whose
// declared paths are library paths; its terms are read without validation, which that release passed.
func (g *libraryGit) releasedTerms(ctx context.Context, latest *publishedRelease) ([]string, error) {
	manifest, legacy, err := g.releasedManifest(ctx, latest)
	if err != nil {
		return nil, err
	}
	if legacy {
		return LegacyTerms(manifest), nil
	}
	location := latest.tagName() + ":" + manifestPath
	declaration, err := rules.ParseLibraryManifest(manifest, location)
	if err != nil {
		return nil, err
	}
	if declaration == nil {
		return nil, failure("invalid-library", location+": the configuration has no library section", nil)
	}
	return rules.LicensePaths(declaration.License), nil
}

// releasedManifest returns the manifest of the latest library release, and whether it's a rule-library.yaml from
// before the layout of version 2. It fails with invalid-library when that library release has no manifest.
func (g *libraryGit) releasedManifest(ctx context.Context, latest *publishedRelease) ([]byte, bool, error) {
	entries, err := g.treeEntries(ctx, latest.object, []string{librarytree.LegacyManifest, manifestPath})
	if err != nil {
		return nil, false, fmt.Errorf("list the files of %s: %w", latest.tagName(), err)
	}
	name, legacy := librarytree.LegacyManifest, true
	entry, ok := entries[name]
	if !ok {
		name, legacy = manifestPath, false
		if entry, ok = entries[name]; !ok {
			return nil, false, failure("invalid-library", latest.tagName()+":"+manifestPath+": missing library manifest", nil)
		}
	}
	manifest, err := g.runner.Output(ctx, g.dir, []string{"cat-file", "blob", entry.object}, maxFileBytes)
	if err != nil {
		return nil, false, fmt.Errorf("read %s:%s: %w", latest.tagName(), name, err)
	}
	return manifest, legacy, nil
}

// LegacyTerms returns the license and notice paths that a rule-library.yaml written before the layout of version 2
// declares, which are library paths, or none when it declares no license or doesn't parse.
func LegacyTerms(manifest []byte) []string {
	_, data, err := decode.YAML(manifest, librarytree.LegacyManifest)
	if err != nil {
		return nil
	}
	var parsed struct {
		License struct {
			File    string   `json:"file"`
			Notices []string `json:"notices"`
		} `json:"license"`
	}
	if json.Unmarshal(data, &parsed) != nil || parsed.License.File == "" {
		return nil
	}
	declaration := &trailguide.LicenseDeclaration{Files: []string{parsed.License.File}, AttributionFiles: parsed.License.Notices}
	return rules.LicensePaths(declaration)
}

// diffLibraryFiles lists the library-wide files that differ between two trees' files, head and released,
// including added and deleted files, in path order. Before the first library release, released is empty, so
// every library-wide file is listed. terms are the license and notice files either tree declares.
func diffLibraryFiles(head, released map[string]string, terms []string) []string {
	files := []string{}
	for _, name := range slices.Sorted(maps.Keys(head)) {
		if libraryWide(name, terms) && head[name] != released[name] {
			files = append(files, name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(released)) {
		if _, kept := head[name]; !kept && libraryWide(name, terms) {
			files = append(files, name)
		}
	}
	slices.Sort(files)
	return files
}

// libraryWide reports whether a file is one projects receive outside any rule's version: the manifest, the
// declared license and notice files, files in the shared assets/ directory, and group metadata in
// _group.yaml. Group READMEs are authoring notes that projects never receive, so they're neither.
func libraryWide(name string, terms []string) bool {
	if name == librarytree.Manifest || slices.Contains(terms, name) || strings.HasPrefix(name, "assets/") {
		return true
	}
	parts := strings.Split(name, "/")
	return len(parts) == 3 && (parts[0] == "practices" || parts[0] == "techs") && parts[2] == "_group.yaml"
}

// createTag writes an annotated tag on commit with exactly message, using the author's tagger identity and
// tag signing configuration, and returns the tag object's ID.
func (g *libraryGit) createTag(ctx context.Context, name, commit string, message []byte) (string, error) {
	result, err := g.runner.Run(ctx, g.dir, []string{"tag", "--annotate", "--cleanup=verbatim", "--file=-", "--", name, commit}, 64*1024, message)
	if err != nil {
		return "", fmt.Errorf("create tag=%q: %w", name, err)
	}
	if result.Status != 0 {
		return "", failure("tag-failed", "Git couldn't create the tag "+name+". Check that it doesn't already exist and that tag signing, if configured, works, then run trailguide library release again.", nil)
	}
	listing, err := g.runner.Output(ctx, g.dir, []string{"rev-parse", "--verify", "refs/tags/" + name}, 4096)
	if err != nil {
		return "", fmt.Errorf("read tag=%q: %w", name, err)
	}
	object := strings.TrimSpace(string(listing))
	if !objectID.MatchString(object) {
		return "", failure("git-failed", "Git reported the object ID of "+name+" in an unexpected format", nil)
	}
	// A signature can make the tag larger than its message suggested, so the created object is checked too.
	size, err := g.runner.Output(ctx, g.dir, []string{"cat-file", "-s", object}, 4096)
	if err != nil {
		return "", fmt.Errorf("read the size of tag=%q: %w", name, err)
	}
	objectSize, err := strconv.Atoi(strings.TrimSpace(string(size)))
	if err != nil {
		return "", failure("git-failed", "Git reported the size of "+name+" in an unexpected format", nil)
	}
	if err := requireTagSize(name, objectSize); err != nil {
		if _, deleteErr := g.runner.Run(context.WithoutCancel(ctx), g.dir, []string{"update-ref", "-d", "refs/tags/" + name, object}, 4096, nil); deleteErr != nil {
			return "", fmt.Errorf("%w The local tag remains; delete it with git tag --delete %s", err, name)
		}
		return "", err
	}
	return object, nil
}

// noExternalText ends each failure that Git's or a server's messages would explain, which Trail Guide never shows.
const noExternalText = "Trail Guide doesn't show messages from Git or the server; to read them, "

// ruleRefusals are the texts servers print when a repository rule or tag protection refuses a tag, such as
// GitHub's rulesets and GitLab's protected tags.
var ruleRefusals = []string{"rule violation", "ruleset", "protected tag", "tag protection"}

// gitHubErrorCode matches the codes GitHub gives the rules a push breaks, such as GH013 for a repository rule. A
// match is one of a fixed set of codes, so a message may name it.
var gitHubErrorCode = regexp.MustCompile(`\bGH0[0-9]{2}\b`)

// gitHubRuleCodes are the GitHub error codes for a rule that protects the tag: GH006 for a protected branch or
// tag, and GH013 for a repository rule violation. Other codes, such as GH001 for large files, mean other refusals.
var gitHubRuleCodes = []string{"GH006", "GH013"}

// Static explanations of a server Git couldn't verify.
const (
	certificateFailure = ": the HTTPS server's certificate couldn't be verified. Check that your system trusts it: Git's http.sslCAInfo setting, your system's certificate store, and any proxy that intercepts TLS"
	hostKeyFailure     = ": the SSH host key couldn't be verified. Check the server's entry in your known_hosts file"
)

// pushRefusal explains why Git couldn't push the tag name to remote, with a static cause chosen from Git's
// diagnostics, which it never shows: a repository rule or tag protection, naming GitHub's error code when there is
// one; another GitHub error code, which it names; a server Git couldn't verify or reach; refused access or
// authentication; a server hook; another refusal from the server; or no answer from the server, such as when a
// pre-push hook stopped the push.
func pushRefusal(name, remote string, diagnostics []byte) string {
	problem := "Git couldn't push " + name + " to " + remote
	// A server that refused the tag was reached, whatever its message says about connections.
	refused := gitexec.Mentions(diagnostics, "[remote rejected]", "remote:")
	switch code := string(gitHubErrorCode.Find(diagnostics)); {
	case slices.Contains(gitHubRuleCodes, code):
		problem += ": a repository rule refused the tag (GitHub error " + code + "). Check the repository's rulesets and tag protection rules, and that they let you create release/<number> tags"
	case code != "":
		problem += ": the server refused the tag for a reason Trail Guide doesn't recognize (GitHub error " + code + "). Check the repository's rules for tags and its server-side hooks"
	case gitexec.Mentions(diagnostics, ruleRefusals...):
		problem += ": a repository rule or tag protection refused the tag. Check the repository's rulesets and tag protection rules, and that they let you create release/<number> tags"
	case !refused && gitexec.Mentions(diagnostics, gitexec.HTTPSCertificateFailures...):
		problem += certificateFailure
	case !refused && gitexec.Mentions(diagnostics, gitexec.SSHHostKeyFailures...):
		problem += hostKeyFailure
	case !refused && gitexec.Mentions(diagnostics, gitexec.ConnectionFailures...):
		problem += ": Git couldn't connect to the server. Check the repository address and your network connection"
	case gitexec.Mentions(diagnostics, gitexec.AccessFailures...):
		problem += ": the server denied access, or authentication failed. Check your Git credentials and that they let you push tags to the repository"
	case gitexec.Mentions(diagnostics, "hook declined"):
		problem += ": a hook on the server declined the tag. Check the repository's server-side hooks, such as pre-receive and update hooks, or ask its administrator"
	case refused:
		problem += ": the server refused the tag for a reason Trail Guide doesn't recognize. Check the repository's rules for tags and its server-side hooks"
	default:
		problem += ". Check your network connection, your access to the repository, and any pre-push hook"
	}
	return problem + ", then run trailguide library release again. " + noExternalText + "push a test tag with git push."
}

// fetchFailure explains why Git couldn't read or fetch from remote, as action says, with a static cause chosen
// from Git's diagnostics, which it never shows: a server Git couldn't verify or reach, or refused access or a
// missing repository.
func fetchFailure(action, remote string, diagnostics []byte) error {
	problem := "Git couldn't " + action + " " + remote
	switch {
	case gitexec.Mentions(diagnostics, gitexec.HTTPSCertificateFailures...):
		problem += certificateFailure
	case gitexec.Mentions(diagnostics, gitexec.SSHHostKeyFailures...):
		problem += hostKeyFailure
	case gitexec.Mentions(diagnostics, gitexec.ConnectionFailures...):
		problem += ": Git couldn't connect to the server. Check the repository address and your network connection"
	case gitexec.Mentions(diagnostics, gitexec.AccessFailures...):
		problem += ": the server denied access, or the repository doesn't exist. Check the repository address, your Git credentials, and your access to the repository"
	default:
		problem += ". Check your network connection and access to the repository"
	}
	return failure("fetch-failed", problem+", then run trailguide library release again. "+noExternalText+"run git fetch "+remote+".", nil)
}

// pushTag pushes only the tag, running the author's pre-push hook. If the push fails, it deletes the local tag
// when this run created it, so the clone doesn't keep a release tag the remote lacks; a tag an interrupted run
// left stays, with its signature, for the next run to push. A remote that already has a different tag of that
// name fails with release-conflict.
func (g *libraryGit) pushTag(ctx context.Context, u upstream, name, object string, created bool) error {
	ref := "refs/tags/" + name
	pushed, err := g.runner.Run(ctx, g.dir, []string{"push", "--no-follow-tags", "--", u.remote, ref + ":" + ref}, 1024*1024, nil)
	if err == nil && pushed.Status == 0 {
		return nil
	}
	problem := failure("push-failed", "Git couldn't push "+name+" to "+u.remote+". Check your network connection, your access to the repository, and any pre-push hook, then run trailguide library release again.", err)
	if err == nil {
		problem = failure("push-failed", pushRefusal(name, u.remote, pushed.Diagnostics), nil)
	}
	if err == nil {
		switch remote, listErr := g.runner.Run(ctx, g.dir, []string{"ls-remote", "--", u.remote, ref}, 64*1024, nil); {
		case listErr != nil || remote.Status != 0:
		case strings.HasPrefix(string(remote.Output), object+"\t"):
			return nil
		case len(remote.Output) > 0:
			problem = failure("release-conflict", "someone else published "+name+" to "+u.remote+" first. Pull their changes, then run trailguide library release again if anything remains to publish.", nil)
		}
	}
	if !created {
		return fmt.Errorf("%w This clone's %s, from an earlier run, stays; if %s has another, delete this one with git tag --delete %s", problem, name, u.remote, name)
	}
	// Delete the tag even after cancellation, which may have interrupted the push.
	deleted, deleteErr := g.runner.Run(context.WithoutCancel(ctx), g.dir, []string{"update-ref", "-d", ref, object}, 4096, nil)
	if deleteErr != nil || deleted.Status != 0 {
		return fmt.Errorf("%w The local tag remains; delete it with git tag --delete %s", problem, name)
	}
	return problem
}
