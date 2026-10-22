// List, fetch, and read a library's release tags, and answer questions about its rules' versions.

package imports

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/trailguide/internal/releasetag"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// libraryRelease is one library release, read from its release/<number> tag.
type libraryRelease struct {
	number int
	// tag is the annotated tag object's ID; commit is the commit it tags.
	tag    string
	commit string
	record trailguide.ReleaseRecord
}

// releaseHistory holds every library release in ascending number order; it is empty before the first one.
type releaseHistory struct {
	releases []libraryRelease
}

// newest returns the library release with the highest number, or nil when there is none.
func (h releaseHistory) newest() *libraryRelease {
	if len(h.releases) == 0 {
		return nil
	}
	return &h.releases[len(h.releases)-1]
}

// upTo returns the history's library releases numbered up to number, or the whole history when number is 0.
func (h releaseHistory) upTo(number int) releaseHistory {
	if number == 0 {
		return h
	}
	end := len(h.releases)
	for end > 0 && h.releases[end-1].number > number {
		end--
	}
	return releaseHistory{releases: h.releases[:end]}
}

// release returns the library release numbered number, or nil when there is none.
func (h releaseHistory) release(number int) *libraryRelease {
	for i := range h.releases {
		if h.releases[i].number == number {
			return &h.releases[i]
		}
	}
	return nil
}

// publisher returns the library release that published version of rule id: the one whose changes entry for the
// rule leads to that version. It returns nil when the rule never published the version.
func (h releaseHistory) publisher(id string, version trailguide.RuleVersion) *libraryRelease {
	for i := range h.releases {
		record := h.releases[i].record
		if _, changed := record.Changes[id]; changed && record.Rules[id] == version {
			return &h.releases[i]
		}
	}
	return nil
}

// published returns, newest first, each library release that published a version of rule id.
func (h releaseHistory) published(id string) []*libraryRelease {
	result := []*libraryRelease{}
	for i := len(h.releases) - 1; i >= 0; i-- {
		if _, changed := h.releases[i].record.Changes[id]; changed {
			result = append(result, &h.releases[i])
		}
	}
	return result
}

// maxListedVersions bounds how many versions an error lists, so a long-lived rule's history stays readable.
const maxListedVersions = 10

// versionList lists, newest first, the versions rule id published, at most maxListedVersions of them followed by
// how many older ones there are, or "" when it published none.
func (h releaseHistory) versionList(id string) string {
	versions := []string{}
	for _, release := range h.published(id) {
		versions = append(versions, release.record.Rules[id].String())
	}
	if len(versions) > maxListedVersions {
		return strings.Join(versions[:maxListedVersions], ", ") + fmt.Sprintf(", and %d older", len(versions)-maxListedVersions)
	}
	return strings.Join(versions, ", ")
}

// retired reports whether any library release retired rule id.
func (h releaseHistory) retired(id string) bool {
	return h.retirement(id) != nil
}

// currentReplacement follows the replacements of retired rule id until one that the newest library release
// publishes, and returns it, or "" when a retirement names no replacement. A cycle, which only hand-made release
// tags could record, also returns "".
func (h releaseHistory) currentReplacement(id string) string {
	seen := map[string]bool{}
	for retired := h.retirement(id); retired != nil && retired.ReplacedBy != "" && !seen[id]; retired = h.retirement(id) {
		seen[id], id = true, retired.ReplacedBy
		if _, current := h.newest().record.Rules[id]; current {
			return id
		}
	}
	return ""
}

// retirement returns how a library release retired rule id, or nil when none did.
func (h releaseHistory) retirement(id string) *trailguide.RetiredRule {
	for _, release := range h.releases {
		if retired, ok := release.record.Retired[id]; ok {
			return &retired
		}
	}
	return nil
}

// summaries returns, oldest first, the summaries of every version of rule id newer than from, up to and including
// to, one per change note, and the version each belongs to, in the same order. A nil from includes every version up
// to to. Both are empty, never nil, when there are none.
func (h releaseHistory) summaries(id string, from *trailguide.RuleVersion, to trailguide.RuleVersion) ([]string, []trailguide.RuleVersion) {
	summaries, versions := []string{}, []trailguide.RuleVersion{}
	for _, release := range h.releases {
		change, changed := release.record.Changes[id]
		version := release.record.Rules[id]
		if changed && (from == nil || version.Compare(*from) > 0) && version.Compare(to) <= 0 {
			for _, summary := range change.Summaries {
				summaries, versions = append(summaries, summary), append(versions, version)
			}
		}
	}
	return summaries, versions
}

// loadHistory lists the library's release/<number> tags, fetches them without history or blobs, and reads their
// release records. Tags under release/ with other names are ignored. A lightweight or unreadable release tag,
// or one that moved while it was fetched, fails with code invalid-release-tag.
func (r *repository) loadHistory(ctx context.Context) (releaseHistory, error) {
	advertised, err := r.listReleases(ctx)
	if err != nil || len(advertised) == 0 {
		return releaseHistory{}, err
	}
	refspecs := make([]string, 0, len(advertised))
	for _, release := range advertised {
		name := "refs/tags/release/" + strconv.Itoa(release.number)
		refspecs = append(refspecs, "+"+name+":"+name)
	}
	fetched, err := r.fetch(ctx, refspecs, true)
	if err != nil {
		return releaseHistory{}, err
	}
	if fetched.Status != 0 {
		if err := r.unreachable(ctx); err != nil {
			return releaseHistory{}, err
		}
		return releaseHistory{}, gitFailure("git-failed", "Could not fetch the library's release tags.", fetched.Diagnostics)
	}
	tags, err := r.fetchedReleases(ctx, advertised)
	if err != nil {
		return releaseHistory{}, err
	}
	// Imports need only the records, so each release's notes are dropped as soon as it is read.
	err = releasetag.Read(ctx, r.runner, r.directory, tags, func(i int, release releasetag.Release) error {
		advertised[i].record = release.Record
		return nil
	})
	var invalid *releasetag.RecordError
	var unsupported *trailguide.UnsupportedReleaseRecordError
	switch {
	case errors.As(err, &invalid) && errors.As(invalid.Err, &unsupported):
		return releaseHistory{}, fail("unsupported-release-record", fmt.Sprintf("Library release tag %s uses release record schema version %d, which this version of Trail Guide can't read. Upgrade Trail Guide, then run the command again.", invalid.Tag, unsupported.SchemaVersion), nil)
	case invalid != nil:
		return releaseHistory{}, fail("invalid-release-tag", "Invalid release record: "+invalid.Problem()+". Don't create or move release tags by hand.", invalid.Err)
	}
	if err != nil {
		return releaseHistory{}, err
	}
	for _, release := range advertised {
		r.present[release.commit] = true
	}
	return releaseHistory{releases: advertised}, nil
}

// listReleases reads the remote's release/<number> tags, within the tag-listing limits, in ascending order.
func (r *repository) listReleases(ctx context.Context) ([]libraryRelease, error) {
	result, err := r.runner.Run(ctx, r.directory, []string{"ls-remote", "--tags", fetchRemote, "refs/tags/release/*"}, 8<<20+64<<10, nil)
	if err != nil {
		return nil, err
	}
	if result.Status != 0 {
		return nil, remoteFailure(result.Diagnostics)
	}
	tags, err := rules.ParseTagAdvertisement(string(result.Output))
	if err != nil {
		var advertisement *rules.TagAdvertisementError
		if errors.As(err, &advertisement) {
			return nil, fail(string(advertisement.Kind), advertisement.Problem, nil)
		}
		return nil, err
	}
	releases := []libraryRelease{}
	for _, name := range slices.Sorted(maps.Keys(tags)) {
		number, err := trailguide.ParseReleaseTag(name)
		if err != nil {
			continue
		}
		commit, annotated := tags[name+"^{}"]
		if !annotated {
			return nil, fail("invalid-release-tag", fmt.Sprintf("Library release tag %s isn't an annotated tag with a release record. Don't create or move release tags by hand.", name), nil)
		}
		releases = append(releases, libraryRelease{number: number, tag: tags[name], commit: commit})
	}
	slices.SortFunc(releases, func(a, b libraryRelease) int { return a.number - b.number })
	return releases, nil
}

// fetchedReleases returns the fetched release tag of each advertised library release, in the same order, checking
// that it is the tag object the listing advertised and that it tags the advertised object, which must be a commit,
// so a tag moved in between or tagging a tree or blob fails.
func (r *repository) fetchedReleases(ctx context.Context, advertised []libraryRelease) ([]releasetag.Tag, error) {
	listed, err := releasetag.List(ctx, r.runner, r.directory, "")
	if err != nil {
		return nil, err
	}
	fetched := map[int]releasetag.Tag{}
	for _, tag := range listed {
		fetched[tag.Number] = tag
	}
	tags := make([]releasetag.Tag, 0, len(advertised))
	for _, release := range advertised {
		name := "release/" + strconv.Itoa(release.number)
		tag, ok := fetched[release.number]
		if !ok || tag.Type != "tag" || tag.Object != release.tag || tag.Target != release.commit {
			return nil, fail("invalid-release-tag", fmt.Sprintf("Library release tag %s changed while Trail Guide read it; retry. Don't create or move release tags by hand.", name), nil)
		}
		if tag.TargetType != "commit" {
			return nil, fail("invalid-release-tag", fmt.Sprintf("Library release tag %s tags a %s, not a commit. Don't create or move release tags by hand.", name, tag.TargetType), nil)
		}
		if tag.Size > releasetag.MaxBytes {
			return nil, fail("limit-exceeded", fmt.Sprintf("Library release tag %s exceeds 8 MiB.", name), nil)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// NewestRelease returns the number of the newest release/<number> tag that repository advertises, or 0 when it has
// none, such as a repository that isn't a library, without fetching the tags or reading their release records.
func NewestRelease(ctx context.Context, repository string, options Options) (_ int, err error) {
	ctx, cancel, err := withTimeout(ctx, options)
	if err != nil {
		return 0, err
	}
	defer cancel()
	repo, err := openRepository(ctx, rules.Source{Name: "source", Repository: repository}, options)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	advertised, err := repo.listReleases(ctx)
	if err != nil || len(advertised) == 0 {
		return 0, err
	}
	return advertised[len(advertised)-1].number, nil
}
