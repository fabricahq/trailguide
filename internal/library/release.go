// Publish a library release: tag the checked-out commit of the default branch, push the tag, and announce it.

package library

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/fabricahq/trailguide/internal/releasetag"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// ReleaseRequest selects what trailguide library release does.
type ReleaseRequest struct {
	Options
	// DryRun shows what would be published without creating a tag or a GitHub Release page.
	DryRun bool
	// NoGitHubRelease publishes the tag without a GitHub Release page.
	NoGitHubRelease bool
}

// ReleaseResult describes a library release that was published, finished, or previewed, or that there was
// nothing to publish.
type ReleaseResult struct {
	// Repository is where the library release is pushed, the upstream remote's repository, without credentials.
	Repository string `json:"repository"`
	Remote     string `json:"remote"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	// GitHubRepository is OWNER/REPO when the repository is on GitHub.com, and empty for other hosts.
	GitHubRepository string `json:"githubRepository,omitempty"`
	DryRun           bool   `json:"dryRun"`
	// Release is the library release's number, and 0 when there's nothing to publish.
	Release int `json:"release"`
	// Tag is the release tag, such as release/4; it's empty when there's nothing to publish.
	Tag string `json:"tag,omitempty"`
	// Published reports that the remote already had the tag, on this commit, before this run.
	Published bool `json:"published"`
	// Rules lists each rule the library release changes, adds, or retires, sorted by ID, and Skills each skill, as
	// PendingRelease does; Skills names levels and forks only for a library release this run plans.
	Rules        []PendingRule  `json:"rules"`
	Skills       []PendingSkill `json:"skills"`
	LibraryFiles []string       `json:"libraryFiles"`
	Notes        string         `json:"notes,omitempty"`
	// TagCreated reports that this run pushed the tag to the remote, creating it first unless an interrupted
	// run left it in the clone.
	TagCreated bool `json:"tagCreated"`
	// GitHubRelease is nil when no GitHub Release page was created or found: on a dry run, with
	// NoGitHubRelease, on other hosts, and when there's nothing to publish.
	GitHubRelease *GitHubReleasePage `json:"githubRelease,omitempty"`
	Warnings      []string           `json:"warnings"`
	// ForkedSkills lists the skills the library forked whose sources changed them since, as CheckResult does; it is
	// empty, never nil, when there are none.
	ForkedSkills []ForkedSkill `json:"forkedSkills"`
}

// GitHubReleasePage is a library release's announcement on GitHub.com.
type GitHubReleasePage struct {
	// Created reports that this run created the page; false means it already existed.
	Created bool   `json:"created"`
	URL     string `json:"url,omitempty"`
}

// Release publishes the pending change notes and library-wide changes as the next library release: an
// annotated release/<number> tag on the checked-out commit, pushed to the upstream remote, and, for a
// repository on GitHub.com, a GitHub Release page created with gh. It changes no files in the library.
//
// It fetches first, and refuses unless the checked-out branch is the remote's default branch, matches the
// remote exactly, and has no uncommitted library changes, and library check passes. When a release tag
// already tags the commit, it creates only what's missing, such as the GitHub Release page after gh failed.
// A dry run fetches, checks, and describes the library release without creating anything. When the tag is
// published but the GitHub Release page fails, it returns the result so far, without GitHubRelease, with the error;
// every other failure returns a zero result.
func Release(ctx context.Context, request ReleaseRequest) (ReleaseResult, error) {
	root, err := openLibrary(ctx, request.Options, false)
	if err != nil {
		return ReleaseResult{}, err
	}
	defer root.Close()
	if err = requireIdle(ctx, root); err != nil {
		return ReleaseResult{}, err
	}
	git, err := openLibraryGit(ctx, root.Name(), request.Git)
	if err != nil {
		return ReleaseResult{}, err
	}
	if git == nil {
		return ReleaseResult{}, failure("not-a-repository", "a library release is a Git tag, so the library must be the root of a Git repository.", nil)
	}
	if err = git.requireFullHistory(ctx); err != nil {
		return ReleaseResult{}, err
	}
	branch, err := git.upstream(ctx)
	if err != nil {
		return ReleaseResult{}, err
	}
	result := ReleaseResult{Repository: displayRepository(branch.pushURL), Remote: branch.remote, Branch: branch.branch, GitHubRepository: gitHubRepository(branch.pushURL), DryRun: request.DryRun, Rules: []PendingRule{}, Skills: []PendingSkill{}, LibraryFiles: []string{}, ForkedSkills: []ForkedSkill{}}
	remote, err := git.readRemote(ctx, branch)
	if err != nil {
		return ReleaseResult{}, err
	}
	if result.Commit, err = git.headCommit(ctx); err != nil {
		return ReleaseResult{}, err
	}
	unpushedTag, err := git.syncReleaseTags(ctx, branch, remote, result.Commit)
	if err != nil {
		return ReleaseResult{}, err
	}
	unpushed := unpushedTag.Number
	if err = git.requireCurrent(ctx, branch, remote.head, result.Commit); err != nil {
		return ReleaseResult{}, err
	}
	// A release tag only this clone has is checked against the library release this commit would publish after
	// the latest one the remote published, instead of serving as the baseline itself.
	git = git.withoutUnpublished(unpushed)
	checked, err := checkLibrary(ctx, root, git, request.Curated, request.Forks)
	if err != nil {
		return ReleaseResult{}, err
	}
	result.Warnings, result.ForkedSkills = checked.result.Warnings, checked.result.ForkedSkills
	if err = git.requireCommitted(ctx, result.Commit, checked.input); err != nil {
		return ReleaseResult{}, err
	}
	// gh is nil unless a GitHub Release page applies. It and the tagger are checked only when there's
	// something to publish, but before anything is created; a dry run checks gh too, so it refuses as the real run
	// would.
	var gh *gitHubCLI
	requireGitHubCLI := func() error {
		if result.GitHubRepository == "" || request.NoGitHubRelease {
			return nil
		}
		cli, err := findGitHubCLI(ctx, request.Git.Environment, root.Name())
		gh = &cli
		return err
	}
	var object string
	// created is set when this run creates the tag, so a failed push deletes only a tag this run made.
	var created bool
	switch latest := checked.changes.history.latest; {
	case unpushed == 0 && latest != nil && latest.commit == result.Commit:
		// The remote already has this commit's library release; finish only what's missing.
		describe(&result, latest.record, latest.notes, skillContext{})
		result.Published = true
		object = latest.object
		if err = requireGitHubCLI(); err != nil {
			return ReleaseResult{}, err
		}
	case checked.plan.release != remote.latest()+1:
		return ReleaseResult{}, failure("release-tag-mismatch", branch.remote+"'s newest library release is release/"+strconv.Itoa(remote.latest())+", but "+branch.branch+"'s history reaches only release/"+strconv.Itoa(checked.plan.release-1)+". Release tags belong on the default branch; don't create them by hand.", nil)
	default:
		planned, err := planRelease(ctx, git, checked, result.Commit)
		if err != nil {
			return ReleaseResult{}, err
		}
		if unpushed != 0 {
			// An interrupted run left its tag on this commit; push it only if it publishes what this run would.
			if err = git.requireUnpublishedTag(ctx, unpushedTag, planned); err != nil {
				return ReleaseResult{}, err
			}
			describe(&result, planned.record, planned.notes, checked.skills)
			object = unpushedTag.Object
			if err = requireGitHubCLI(); err != nil {
				return ReleaseResult{}, err
			}
			break
		}
		if planned.empty {
			return result, nil
		}
		describe(&result, planned.record, planned.notes, checked.skills)
		// tagger stays empty on a dry run, which slightly underestimates the tag's size.
		var tagger string
		if !request.DryRun {
			if tagger, err = git.requireCommitterIdentity(ctx); err != nil {
				return ReleaseResult{}, err
			}
		}
		if err = requireGitHubCLI(); err != nil {
			return ReleaseResult{}, err
		}
		if err = requireTagSize(result.Tag, tagObjectSize(result.Tag, result.Commit, tagger, planned.message)); err != nil {
			return ReleaseResult{}, err
		}
		if request.DryRun {
			return result, nil
		}
		if err = requireLibraryUnchanged(ctx, root, checked.input); err != nil {
			return ReleaseResult{}, err
		}
		if err = requireIdle(ctx, root); err != nil {
			return ReleaseResult{}, err
		}
		if object, err = git.createTag(ctx, result.Tag, result.Commit, planned.message); err != nil {
			return ReleaseResult{}, err
		}
		unpushed, created = planned.record.Release, true
	}
	if request.DryRun {
		return result, nil
	}
	if unpushed == result.Release {
		if err = git.pushTag(ctx, branch, result.Tag, object, created); err != nil {
			return ReleaseResult{}, err
		}
		result.TagCreated = true
	}
	if gh != nil {
		page, err := publishReleasePage(ctx, *gh, result.GitHubRepository, result.Tag, result.Notes)
		if err != nil {
			// The tag is published, so the result says so beside the error.
			return result, err
		}
		result.GitHubRelease = &page
	}
	return result, nil
}

// plannedRelease is the library release a commit publishes after the latest library release in its history.
type plannedRelease struct {
	record  trailguide.ReleaseRecord
	notes   string
	message []byte
	// empty reports that the commit changes no rules, no skills, and no library-wide files, so there's nothing to
	// publish.
	empty bool
}

// planRelease computes the record, notes, and tag message of the library release that head, which checked
// validated, publishes after the latest library release in checked's history.
func planRelease(ctx context.Context, git *libraryGit, checked checkedLibrary, head string) (plannedRelease, error) {
	libraryFiles, err := git.changedLibraryFiles(ctx, head, checked.changes.history.latest, rules.LicensePaths(checked.input.license))
	if err != nil {
		return plannedRelease{}, err
	}
	planned := plannedRelease{record: checked.plan.record(libraryFiles), empty: checked.plan.empty() && len(libraryFiles) == 0}
	planned.notes = renderReleaseNotes(planned.record, checked.skills)
	if planned.message, err = releaseMessage(planned.notes, planned.record); err != nil {
		return plannedRelease{}, err
	}
	return planned, nil
}

// requireUnpublishedTag refuses to publish tag, a release tag only this clone has, unless its release notes and
// record are exactly those of planned, the library release its commit would publish now.
func (g *libraryGit) requireUnpublishedTag(ctx context.Context, tag releasetag.Tag, planned plannedRelease) error {
	name := tag.Name()
	// The whole object counts, including a signature the record's parser ignores, because projects read the object.
	if err := requireTagSize(name, tag.Size); err != nil {
		return fmt.Errorf("%w This clone's %s is that large; delete it with git tag --delete %s", err, name, name)
	}
	body, err := g.runner.Output(ctx, g.dir, []string{"cat-file", "tag", tag.Object}, releasetag.MaxBytes+64*1024)
	if err != nil {
		return fmt.Errorf("read tag=%q: %w", name, err)
	}
	release, err := releasetag.ParseObject(name, body)
	// A newer Trail Guide may have created the tag and stopped before pushing it; this version can't compare it.
	var unsupported *trailguide.UnsupportedReleaseRecordError
	if errors.As(err, &unsupported) {
		return unsupportedRecord(name, unsupported.FormatVersion)
	}
	if err == nil && !planned.empty && release.Notes == planned.notes && reflect.DeepEqual(release.Record, planned.record) {
		return nil
	}
	return failure("release-tag-mismatch", name+" exists only in this clone, and it isn't the library release this commit publishes: its release notes or record differ from what trailguide library release computes from the change notes since the remote's latest library release. Only trailguide library release creates release tags; delete this one with git tag --delete "+name+", then run trailguide library release again.", nil)
}

// describe fills in what a library release publishes, with the skill levels and forks of skills.
func describe(result *ReleaseResult, record trailguide.ReleaseRecord, notes string, skills skillContext) {
	result.Release = record.Release
	result.Tag = "release/" + strconv.Itoa(record.Release)
	result.Rules = releaseRules(record)
	result.Skills = releaseSkills(record, skills)
	result.LibraryFiles = record.LibraryFiles
	result.Notes = notes
}

// publishReleasePage finds tag's GitHub Release page, creating it with notes as its body when it's missing.
func publishReleasePage(ctx context.Context, gh gitHubCLI, repository, tag, notes string) (GitHubReleasePage, error) {
	url, exists, err := gh.releasePage(ctx, repository, tag)
	if err != nil {
		return GitHubReleasePage{}, err
	}
	if exists {
		return GitHubReleasePage{URL: url}, nil
	}
	url, err = gh.createReleasePage(ctx, repository, tag, notes)
	if err != nil {
		return GitHubReleasePage{}, err
	}
	return GitHubReleasePage{Created: true, URL: url}, nil
}
