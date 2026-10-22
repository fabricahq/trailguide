// Publish library releases from authors' clones of real Git fixtures, which accept pushes.

package library

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/trailguide/internal/test/ghfixture"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// gitHubRepositoryURL points a clone's origin at GitHub.com; the fixture's transport still serves it.
const gitHubRepositoryURL = "git@github.com:acme/rules.git"

// tags lists dir's tags with the objects they name, one "name object" line each, so tests can tell whether
// anything changed.
func tags(t *testing.T, fixture *gitfixture.Fixture, dir string) string {
	t.Helper()
	out, err := fixture.CommandIn(context.Background(), dir, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/tags")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// remoteDir is the fixture repository that authors' clones fetch from and push to.
func remoteDir(fixture *gitfixture.Fixture) string {
	return filepath.Join(fixture.Directory, "repository")
}

// tagMessage returns an annotated tag's message in dir, without its header.
func tagMessage(t *testing.T, fixture *gitfixture.Fixture, dir, name string) string {
	t.Helper()
	object := catTag(t, fixture, dir, name)
	_, message, _ := strings.Cut(object, "\n\n")
	return message
}

// catTag returns an annotated tag's raw object in dir. The fixture trims surrounding whitespace.
func catTag(t *testing.T, fixture *gitfixture.Fixture, dir, name string) string {
	t.Helper()
	object, err := fixture.CommandIn(context.Background(), dir, "cat-file", "tag", name)
	if err != nil {
		t.Fatal(err)
	}
	return object + "\n"
}

// commitAndPush commits files, keyed by library path, in the author's clone and pushes them to the fixture.
func commitAndPush(t *testing.T, fixture *gitfixture.Fixture, dir string, files map[string][]byte) string {
	t.Helper()
	ctx := context.Background()
	commit, err := fixture.Commit(ctx, dir, "Change the library", gitfixture.Library(files))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.CommandIn(ctx, dir, "push", "--quiet", "origin", "main"); err != nil {
		t.Fatal(err)
	}
	return commit
}

// withGitHub points the clone's origin at GitHub.com and makes a fake gh, answering responses, the only program
// on the PATH that release uses.
func withGitHub(t *testing.T, fixture *gitfixture.Fixture, options *Options, responses []ghfixture.Response) *ghfixture.Fake {
	t.Helper()
	if _, err := fixture.CommandIn(context.Background(), options.Directory, "remote", "set-url", "origin", gitHubRepositoryURL); err != nil {
		t.Fatal(err)
	}
	return withFakeGitHubCLI(t, options, responses)
}

// withFakeGitHubCLI replaces PATH in options' environment with a directory holding only a fake gh.
func withFakeGitHubCLI(t *testing.T, options *Options, responses []ghfixture.Response) *ghfixture.Fake {
	t.Helper()
	bin := t.TempDir()
	fake, err := ghfixture.Install(bin, responses)
	if err != nil {
		t.Fatal(err)
	}
	environment := []string{}
	for _, item := range options.Git.Environment {
		if !strings.HasPrefix(item, "PATH=") {
			environment = append(environment, item)
		}
	}
	options.Git.Environment = append(environment, "PATH="+bin)
	return fake
}

// gitHubCalls are the gh invocations that publish tag's page when it doesn't exist yet, creating it with exit code.
func gitHubCalls(tag string, code int) []ghfixture.Response {
	return []ghfixture.Response{
		{Args: []string{"auth", "status", "--hostname", "github.com"}},
		{Args: []string{"release", "view", tag, "--repo", "github.com/acme/rules", "--json", "url", "--jq", ".url"}, Stderr: "release not found\n", ExitCode: 1},
		{Args: []string{"release", "create", tag, "--repo", "github.com/acme/rules", "--verify-tag", "--title", tag, "--notes-file", "-"}, Stdout: "https://github.com/acme/rules/releases/tag/" + tag + "\n", ExitCode: code},
	}
}

// requireCalls compares a fake gh's recorded calls with want's arguments and stdin.
func requireCalls(t *testing.T, fake *ghfixture.Fake, want []ghfixture.Call) {
	t.Helper()
	calls, err := fake.Calls()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("gh calls:\n%+v\nwant:\n%+v", calls, want)
	}
}

// TestRelease_FirstLibraryReleaseGivesEveryRuleOneAndPushesOnlyItsTag tags the commit with every rule at
// 1.0.0 and every library-wide file, but not the group README, as the configured committer, and pushes no
// other tag.
func TestRelease_FirstLibraryReleaseGivesEveryRuleOneAndPushesOnlyItsTag(t *testing.T) {
	ctx := context.Background()
	files := libraryFiles()
	files["practices/testing/README.md"] = []byte("# Testing\n\nHow to author testing rules.\n")
	fixture, options := authorClone(t, files)
	options.Git.Environment = append(slices.Clone(options.Git.Environment), "GIT_COMMITTER_NAME=Trail Guide Bot", "GIT_COMMITTER_EMAIL=trailguide-bot@noreply.invalid")
	if _, err := fixture.CommandIn(ctx, options.Directory, "tag", "unrelated"); err != nil {
		t.Fatal(err)
	}
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	notes := "Library release 1 publishes 2 rules.\n\n## New rules\n\n- **practices/testing/a** `1.0.0`\n- **practices/testing/b** `1.0.0`\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.0.0 |\n| practices/testing/b | 1.0.0 |\n\n</details>"
	record := "formatVersion: 1\nrelease: 1\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: new\n    summaries:\n      - Add the rule.\n  practices/testing/b:\n    change: new\n    summaries:\n      - Add the rule.\nlibraryFiles:\n  - config.yaml\n  - practices/testing/_group.yaml\n"
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/1"); message != notes+"\n---\n"+record {
		t.Fatalf("tag message:\n%s\nwant:\n%s\n---\n%s", message, notes, record)
	}
	if object := catTag(t, fixture, remoteDir(fixture), "release/1"); !strings.Contains(object, "\ntagger Trail Guide Bot <trailguide-bot@noreply.invalid> ") || !strings.Contains(object, "object "+fixture.LatestCommit+"\ntype commit\n") {
		t.Fatalf("tag object:\n%s", object)
	}
	if strings.Contains(tags(t, fixture, remoteDir(fixture)), "unrelated") {
		t.Fatal("pushed a tag other than the release tag")
	}
	local, err := fixture.CommandIn(ctx, options.Directory, "rev-parse", "refs/tags/release/1")
	if err != nil {
		t.Fatal(err)
	}
	if remote, err := fixture.Command(ctx, "rev-parse", "refs/tags/release/1"); err != nil || remote != local {
		t.Fatalf("the clone's release/1 is %s, the remote's %s: %v", local, remote, err)
	}
	want := ReleaseResult{Repository: fixture.Repository, Remote: "origin", Branch: "main", Commit: fixture.LatestCommit, Release: 1, Tag: "release/1", Rules: result.Rules, Skills: []PendingSkill{}, LibraryFiles: []string{"config.yaml", "practices/testing/_group.yaml"}, Notes: notes, TagCreated: true, Warnings: []string{"License is undeclared. Decide terms before sharing this library."}, ForkedSkills: []ForkedSkill{}}
	if !reflect.DeepEqual(result, want) || !slices.Equal(previewRows(PendingRelease{Rules: result.Rules}), []string{"practices/testing/a new - 1.0.0", "practices/testing/b new - 1.0.0"}) {
		t.Fatalf("result %+v\nwant %+v", result, want)
	}
	checked, err := Check(ctx, options)
	if err != nil || checked.PendingRelease.Release != 2 || len(checked.PendingRelease.Rules) != 0 {
		t.Fatal(checked, err)
	}
}

// TestRelease_LaterLibraryReleasePublishesPendingNotesAndLibraryWideFiles records each rule's change, the
// retirement, and the changed group metadata and shared asset.
func TestRelease_LaterLibraryReleasePublishesPendingNotesAndLibraryWideFiles(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	commit := commitAndPush(t, fixture, options.Directory, map[string][]byte{
		"practices/testing/a.md":        []byte(ruleText("Test the retry limit and one past it.")),
		"practices/testing/b.md":        nil,
		"practices/testing/c.md":        []byte(ruleText("Test every retry.")),
		"practices/testing/_group.yaml": []byte("name: Testing\ndescription: Testing guidance for retries.\nwhenToRead: When testing.\n"),
		"assets/diagram.svg":            []byte("<svg/>\n"),
		"README.md":                     []byte("Not part of the library.\n"),
		"changes/a.yaml":                []byte("summary: Test one past the limit.\nrules:\n  practices/testing/a: minor\n"),
		"changes/a-typo.yaml":           []byte("summary: Fix a typo.\nrules:\n  practices/testing/a: patch\n"),
		"changes/c.yaml":                []byte("summary: Fold b into a broader rule.\nrules:\n  practices/testing/b:\n    change: retired\n    replacedBy: practices/testing/c\n  practices/testing/c: new\n"),
	})
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	notes := "Library release 2 changes 3 rules: 1 new, 1 minor, and 1 retired.\n\n## New rules\n\n- **practices/testing/c** `1.0.0`\n  - Fold b into a broader rule.\n\n## Minor changes\n\n- **practices/testing/a** `1.0.0` → `1.1.0`\n  - Fix a typo.\n  - Test one past the limit.\n\n## Retired rules\n\n- **practices/testing/b**, last version `1.0.0`, replaced by **practices/testing/c**\n  - Fold b into a broader rule.\n\nThis library release also updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.1.0 |\n| practices/testing/c | 1.0.0 |\n\n</details>"
	record := "formatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.1.0\n  practices/testing/c: 1.0.0\nchanges:\n  practices/testing/a:\n    change: minor\n    from: 1.0.0\n    summaries:\n      - Fix a typo.\n      - Test one past the limit.\n  practices/testing/c:\n    change: new\n    summaries:\n      - Fold b into a broader rule.\nretired:\n  practices/testing/b:\n    lastVersion: 1.0.0\n    replacedBy: practices/testing/c\n    summaries:\n      - Fold b into a broader rule.\nlibraryFiles:\n  - assets/diagram.svg\n  - practices/testing/_group.yaml\n"
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/2"); message != notes+"\n---\n"+record {
		t.Fatalf("tag message:\n%s\nwant:\n%s\n---\n%s", message, notes, record)
	}
	if result.Commit != commit || result.Release != 2 || !result.TagCreated || result.Notes != notes || !slices.Equal(result.LibraryFiles, []string{"assets/diagram.svg", "practices/testing/_group.yaml"}) {
		t.Fatalf("%+v", result)
	}
}

// TestRelease_PublishesLibraryWideChangesWithoutRuleChanges records only the changed files.
func TestRelease_PublishesLibraryWideChangesWithoutRuleChanges(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"practices/testing/_group.yaml": []byte("name: Testing\ndescription: Better testing guidance.\nwhenToRead: When testing.\n")})
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	want := "Library release 2 changes no rules. It updates shared files, such as group descriptions or shared assets.\n\n<details>\n<summary>All rule versions in this library release</summary>\n\n| Rule | Version |\n| --- | --- |\n| practices/testing/a | 1.0.0 |\n| practices/testing/b | 1.0.0 |\n\n</details>\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/b: 1.0.0\nlibraryFiles:\n  - practices/testing/_group.yaml\n"
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/2"); message != want || !result.TagCreated || len(result.Rules) != 0 {
		t.Fatalf("result %+v, tag message:\n%s", result, message)
	}
}

// TestRelease_ListsRenamedAndRemovedLicenseFiles compares the license and notice files both library releases
// declare, so a renamed license and a removed notice are listed under their old paths too.
func TestRelease_ListsRenamedAndRemovedLicenseFiles(t *testing.T) {
	files := libraryFiles()
	files["config.yaml"] = []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n  license:\n    spdxExpression: MIT\n    file: LICENSE.md\n    notices:\n      - NOTICE.md\n")
	files["LICENSE.md"] = []byte("MIT License\n")
	files["NOTICE.md"] = []byte("Notice\n")
	fixture, options := authorClone(t, files, releaseOne)
	commitAndPush(t, fixture, options.Directory, map[string][]byte{
		"config.yaml": []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n  license:\n    spdxExpression: MIT\n    file: LICENSE.txt\n    notices: []\n"),
		"LICENSE.md":  nil,
		"LICENSE.txt": []byte("MIT License\n"),
		"NOTICE.md":   nil,
	})
	result, err := Release(context.Background(), ReleaseRequest{Options: options})
	if want := []string{"LICENSE.md", "LICENSE.txt", "NOTICE.md", "config.yaml"}; err != nil || !slices.Equal(result.LibraryFiles, want) {
		t.Fatalf("library-wide files %q, want %q: %v", result.LibraryFiles, want, err)
	}
}

// TestRelease_ReportsNothingToPublishWithoutNotesOrLibraryWideChanges ignores files outside the library and
// group READMEs, which projects never receive.
func TestRelease_ReportsNothingToPublishWithoutNotesOrLibraryWideChanges(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	commit := commitAndPush(t, fixture, options.Directory, map[string][]byte{"README.md": []byte("About the library.\n"), "practices/testing/README.md": []byte("# Testing\n\nHow to author testing rules.\n")})
	before := tags(t, fixture, remoteDir(fixture))
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil || result.Release != 0 || result.Tag != "" || result.TagCreated || result.Commit != commit || len(result.Rules) != 0 || len(result.LibraryFiles) != 0 {
		t.Fatal(result, err)
	}
	if tags(t, fixture, remoteDir(fixture)) != before {
		t.Fatal("published a tag with nothing to publish")
	}
}

// TestRelease_RefusesBeforeChangingAnything covers every precondition, leaving tags unchanged locally and on
// the remote, and never calling gh.
func TestRelease_RefusesBeforeChangingAnything(t *testing.T) {
	changedA := map[string][]byte{"practices/testing/a.md": []byte(ruleText("Changed.")), "changes/a.yaml": []byte("summary: Change a.\nrules:\n  practices/testing/a: patch\n")}
	for _, test := range []struct {
		name string
		// arrange prepares the author's clone, with a GitHub.com origin when github is set.
		arrange func(t *testing.T, fixture *gitfixture.Fixture, options *Options)
		github  bool
		gh      []ghfixture.Response
		// fetched is set when the refusal follows the fetch, which adds the remote's release tags to the clone.
		fetched bool
		code    string
		message string
	}{
		{name: "branch other than the default", code: "not-default-branch", message: "library releases are published from origin's default branch, but feature tracks feature, which isn't it. Check out the default branch, which git remote show origin names,",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "switch", "--quiet", "--create", "feature")
				run(t, fixture, options.Directory, "push", "--quiet", "--set-upstream", "origin", "feature")
			}},
		{name: "unpushed commit", code: "branch-differs", message: "main has 1 commit that origin/main doesn't. A library release publishes only pushed commits",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				if _, err := fixture.Commit(context.Background(), options.Directory, "Change a", gitfixture.Library(changedA)); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "commits to pull", code: "branch-differs", message: "origin/main has 2 commits that main doesn't. Pull them",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				for range 2 {
					if _, err := fixture.Commit(context.Background(), remoteDir(fixture), "Elsewhere", map[string][]byte{"README.md": []byte(t.Name())}); err != nil {
						t.Fatal(err)
					}
				}
			}},
		{name: "diverged branches", code: "branch-differs", message: "main and origin/main have diverged",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				if _, err := fixture.Commit(context.Background(), remoteDir(fixture), "Elsewhere", map[string][]byte{"README.md": []byte("remote\n")}); err != nil {
					t.Fatal(err)
				}
				if _, err := fixture.Commit(context.Background(), options.Directory, "Change a", gitfixture.Library(changedA)); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "failing library check", code: "change-notes", message: "practices/testing/a changed since release/1",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				commitAndPush(t, fixture, options.Directory, map[string][]byte{"practices/testing/a.md": []byte(ruleText("Changed."))})
			}},
		{name: "uncommitted changes", code: "uncommitted-changes", message: "these library files differ from it:\n  - .trailguide/changes/a.yaml isn't committed\n  - .trailguide/rules/practices/testing/a.md differs from its committed copy\n  - .trailguide/rules/practices/testing/b.md was deleted, and the deletion isn't committed\n  - .trailguide/rules/practices/testing/c.md isn't committed\nCommit and push your changes",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				edit(t, options.Directory, map[string]string{"practices/testing/a.md": ruleText("Changed."), "practices/testing/b.md": "", "practices/testing/c.md": ruleText("New."), "changes/a.yaml": "summary: Change a.\nrules:\n  practices/testing/a: patch\n  practices/testing/b: retired\n  practices/testing/c: new\n"})
			}},
		{name: "uncommitted skill changes", code: "uncommitted-changes", message: "these library files differ from it:\n  - .trailguide/skills/fix/SKILL.md differs from its committed copy\n  - .trailguide/skills/review/SKILL.md isn't committed\n  - .trailguide/skills/tdd/SKILL.md was deleted, and the deletion isn't committed\nCommit and push your changes",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				// The committed fix skill's name doesn't match its folder; only the working tree corrects it.
				commitAndPush(t, fixture, options.Directory, map[string][]byte{".trailguide/skills/fix/SKILL.md": []byte("---\nname: fixer\ndescription: Fix a bug.\n---\n"), ".trailguide/skills/tdd/SKILL.md": []byte("---\nname: tdd\ndescription: Test first.\n---\n"), "changes/skills.yaml": []byte("summary: Add skills.\nskills:\n  skills/fix: new\n  skills/review: new\n")})
				edit(t, options.Directory, map[string]string{".trailguide/skills/fix/SKILL.md": "---\nname: fix\ndescription: Fix a bug.\n---\n", ".trailguide/skills/review/SKILL.md": "---\nname: review\ndescription: Review a change.\n---\n", ".trailguide/skills/tdd": ""})
			}},
		{name: "file a Git filter changes", code: "uncommitted-changes", message: "  - .trailguide/assets/note.txt differs from its committed copy\n",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				// Like Git LFS, the clean filter commits other content than the checkout holds, so Git sees no change.
				run(t, fixture, options.Directory, "config", "filter.upper.clean", "/usr/bin/tr a-z A-Z")
				commitAndPush(t, fixture, options.Directory, map[string][]byte{".gitattributes": []byte(".trailguide/assets/*.txt filter=upper\n"), "assets/note.txt": []byte("checked content\n")})
				if status, err := fixture.CommandIn(context.Background(), options.Directory, "-c", "filter.upper.clean=/usr/bin/tr a-z A-Z", "status", "--porcelain"); err != nil || status != "" {
					t.Fatalf("the filtered file looks changed to Git: %q %v", status, err)
				}
			}},
		{name: "ignored library file", code: "uncommitted-changes", message: ".trailguide/rules/practices/testing/c.md isn't committed",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				commitAndPush(t, fixture, options.Directory, map[string][]byte{".gitignore": []byte("c.md\n")})
				edit(t, options.Directory, map[string]string{"practices/testing/c.md": ruleText("New."), "changes/c.yaml": "summary: Add c.\nrules:\n  practices/testing/c: new\n"})
			}},
		{name: "push URL for another repository", code: "push-destination", message: "origin fetches from https://github.com/acme/rules but pushes to https://github.com/alice/rules.",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "remote", "set-url", "origin", gitHubRepositoryURL)
				run(t, fixture, options.Directory, "remote", "set-url", "--push", "origin", "git@github.com:alice/rules.git")
			}},
		{name: "several push URLs", code: "push-destination", message: "origin pushes to 2 URLs, so a library release could reach only some of them.",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "remote", "set-url", "--add", "--push", "origin", fixture.Repository)
				run(t, fixture, options.Directory, "remote", "set-url", "--add", "--push", "origin", fixture.Repository+"-mirror")
			}},
		{name: "unreachable remote", code: "fetch-failed", message: "Git couldn't read origin: the server denied access, or the repository doesn't exist.",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing"))
			}},
		{name: "detached HEAD", code: "detached-head", message: "HEAD isn't on a branch",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "switch", "--quiet", "--detach")
			}},
		{name: "branch without an upstream", code: "no-upstream", message: "main has no upstream branch on a remote",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "branch", "--unset-upstream")
			}},
		{name: "remote without a fetch refspec", code: "no-upstream", message: "main has no upstream branch on a remote",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "config", "--unset-all", "remote.origin.fetch")
			}},
		{name: "moved release tag", code: "release-tag-mismatch", message: "release/1 in this clone differs from release/1 on origin",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "tag", "--annotate", "--force", "--cleanup=verbatim", "--message", strings.Replace(releaseOne, "Add a.", "Add rule a.", 1), "release/1")
			}},
		{name: "release tag only in this clone", code: "release-tag-mismatch", message: "release/3 exists in this clone but not on origin",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				run(t, fixture, options.Directory, "tag", "--annotate", "--message", "By hand.", "release/3", "HEAD~1")
			}},
		{name: "release tag on another branch", code: "release-tag-mismatch", fetched: true, message: "origin's newest library release is release/2, but main's history reaches only release/1",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				remote := remoteDir(fixture)
				run(t, fixture, remote, "switch", "--quiet", "--create", "elsewhere")
				if _, err := fixture.Commit(context.Background(), remote, "Elsewhere", map[string][]byte{"README.md": []byte("elsewhere\n")}); err != nil {
					t.Fatal(err)
				}
				run(t, fixture, remote, "tag", "--annotate", "--cleanup=verbatim", "--message", strings.Replace(releaseOne, "release: 1", "release: 2", 1), "release/2")
				run(t, fixture, remote, "switch", "--quiet", "main")
				commitAndPush(t, fixture, options.Directory, changedA)
			}},
		{name: "shallow clone", code: "shallow-clone", message: "git fetch --unshallow --tags",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				shallow := filepath.Join(t.TempDir(), "shallow")
				run(t, fixture, filepath.Dir(shallow), "clone", "--quiet", "--depth=1", "--template=", fixture.Repository, shallow)
				options.Directory = shallow
			}},
		{name: "unknown committer", code: "git-identity", message: "Git doesn't know your name and email",
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				commitAndPush(t, fixture, options.Directory, changedA)
				withoutIdentity(options)
			}},
		{name: "GitHub CLI not installed", code: "github-cli-missing", message: "Install it from https://cli.github.com", github: true,
			arrange: func(t *testing.T, fixture *gitfixture.Fixture, options *Options) {
				if err := os.Remove(filepath.Join(pathOf(options), "gh")); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "GitHub CLI signed out", code: "github-cli-signed-out", message: "Sign in with gh auth login, or pass --no-github-release", github: true,
			gh: []ghfixture.Response{{Args: []string{"auth", "status", "--hostname", "github.com"}, ExitCode: 1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture, options := authorClone(t, libraryFiles(), releaseOne)
			var fake *ghfixture.Fake
			if test.github {
				fake = withGitHub(t, fixture, &options, test.gh)
			}
			if test.arrange != nil {
				test.arrange(t, fixture, &options)
			}
			local, remote := tags(t, fixture, options.Directory), tags(t, fixture, remoteDir(fixture))
			_, err := Release(context.Background(), ReleaseRequest{Options: options})
			if errorCode(err) != test.code || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("got %q: %v\nwant %q: %s", errorCode(err), err, test.code, test.message)
			}
			if test.fetched {
				local = remote
			}
			if tags(t, fixture, options.Directory) != local || tags(t, fixture, remoteDir(fixture)) != remote {
				t.Fatal("a refused library release changed tags")
			}
			if fake != nil && test.code != "github-cli-signed-out" && test.code != "github-cli-missing" {
				requireCalls(t, fake, []ghfixture.Call{})
			}
		})
	}
}

// run runs Git in dir with the fixture's isolated configuration, failing the test on error.
func run(t *testing.T, fixture *gitfixture.Fixture, dir string, args ...string) {
	t.Helper()
	if _, err := fixture.CommandIn(context.Background(), dir, args...); err != nil {
		t.Fatal(err)
	}
}

// withoutIdentity leaves Git no committer name or email to record as a tag's tagger.
func withoutIdentity(options *Options) {
	environment := []string{}
	for _, item := range options.Git.Environment {
		if !strings.HasPrefix(item, "GIT_COMMITTER_") && !strings.HasPrefix(item, "GIT_AUTHOR_") && !strings.HasPrefix(item, "EMAIL=") {
			environment = append(environment, item)
		}
	}
	options.Git.Environment = append(environment, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=user.useConfigOnly", "GIT_CONFIG_VALUE_0=true")
}

// TestRelease_ReportsNothingToPublishWithoutTheToolsPublishingNeeds needs neither a tagger identity nor gh
// when there's nothing to publish.
func TestRelease_ReportsNothingToPublishWithoutTheToolsPublishingNeeds(t *testing.T) {
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"README.md": []byte("About the library.\n")})
	fake := withGitHub(t, fixture, &options, nil)
	if err := os.Remove(filepath.Join(pathOf(&options), "gh")); err != nil {
		t.Fatal(err)
	}
	withoutIdentity(&options)
	result, err := Release(context.Background(), ReleaseRequest{Options: options})
	if err != nil || result.Release != 0 {
		t.Fatal(result, err)
	}
	requireCalls(t, fake, []ghfixture.Call{})
}

// pathOf returns the PATH in options' environment.
func pathOf(options *Options) string {
	for _, item := range options.Git.Environment {
		if value, ok := strings.CutPrefix(item, "PATH="); ok {
			return value
		}
	}
	return ""
}

// TestRelease_CreatesTheGitHubReleasePageAndFinishesAfterItFails reruns to create only the missing page, and
// then finds the page without creating another.
func TestRelease_CreatesTheGitHubReleasePageAndFinishesAfterItFails(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"practices/testing/a.md": []byte(ruleText("Changed.")), "changes/a.yaml": []byte("summary: Clarify a.\nrules:\n  practices/testing/a: patch\n")})
	fake := withGitHub(t, fixture, &options, gitHubCalls("release/2", 1))
	_, err := Release(ctx, ReleaseRequest{Options: options})
	if errorCode(err) != "github-release-failed" || !strings.Contains(err.Error(), "The tag is published; check that gh works with gh auth status, and that it can reach the repository with gh repo view github.com/acme/rules, then run trailguide library release again, which creates the page with its notes only if it's still missing.") || strings.Contains(err.Error(), "gh release create") {
		t.Fatal(err)
	}
	message := tagMessage(t, fixture, remoteDir(fixture), "release/2")
	notes, _, _ := strings.Cut(message, "\n---\n")
	calls := []ghfixture.Call{
		{Args: gitHubCalls("release/2", 0)[0].Args},
		{Args: gitHubCalls("release/2", 0)[1].Args},
		{Args: gitHubCalls("release/2", 0)[2].Args, Stdin: notes + "\n"},
	}
	requireCalls(t, fake, calls)
	fake = withFakeGitHubCLI(t, &options, gitHubCalls("release/2", 0))
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil {
		t.Fatal(err)
	}
	if result.TagCreated || !result.Published || result.Notes != notes || result.GitHubRepository != "acme/rules" || result.Repository != "https://github.com/acme/rules" ||
		!reflect.DeepEqual(result.GitHubRelease, &GitHubReleasePage{Created: true, URL: "https://github.com/acme/rules/releases/tag/release/2"}) {
		t.Fatalf("%+v", result)
	}
	requireCalls(t, fake, calls)
	if tagMessage(t, fixture, remoteDir(fixture), "release/2") != message {
		t.Fatal("the rerun changed the tag")
	}
	fake = withFakeGitHubCLI(t, &options, []ghfixture.Response{
		{Args: gitHubCalls("release/2", 0)[0].Args},
		{Args: gitHubCalls("release/2", 0)[1].Args, Stdout: "https://github.com/acme/rules/releases/tag/release/2\n"},
	})
	result, err = Release(ctx, ReleaseRequest{Options: options})
	if err != nil || result.TagCreated || !reflect.DeepEqual(result.GitHubRelease, &GitHubReleasePage{URL: "https://github.com/acme/rules/releases/tag/release/2"}) {
		t.Fatal(result, err)
	}
	requireCalls(t, fake, []ghfixture.Call{{Args: calls[0].Args}, {Args: calls[1].Args}})
}

// TestRelease_FailsWithoutCreatingAPageGHCouldNotLookUp treats only gh's "release not found" as a missing page,
// so an API failure never leads to creating a second page.
func TestRelease_FailsWithoutCreatingAPageGHCouldNotLookUp(t *testing.T) {
	fixture, options := authorClone(t, libraryFiles())
	responses := gitHubCalls("release/1", 0)
	responses[1].Stderr = "HTTP 502: Bad Gateway (https://api.github.com/repos/acme/rules/releases/tags/release/1)\n"
	fake := withGitHub(t, fixture, &options, responses)
	_, err := Release(context.Background(), ReleaseRequest{Options: options})
	if errorCode(err) != "github-release-failed" || !strings.Contains(err.Error(), "couldn't look up the GitHub Release page for release/1: gh failed for a reason Trail Guide doesn't recognize. The tag is published; run gh release view release/1 --repo github.com/acme/rules to read gh's message") || strings.Contains(err.Error(), "Bad Gateway") {
		t.Fatal(err)
	}
	requireCalls(t, fake, []ghfixture.Call{{Args: responses[0].Args}, {Args: responses[1].Args}})
}

// TestRelease_PublishesThroughAPushURLForTheSameRepository accepts a push URL in another form for the fetch
// repository, and names the GitHub repository after the push destination.
func TestRelease_PublishesThroughAPushURLForTheSameRepository(t *testing.T) {
	fixture, options := authorClone(t, libraryFiles())
	withGitHub(t, fixture, &options, nil)
	run(t, fixture, options.Directory, "remote", "set-url", "--push", "origin", "ssh://git@github.com/Acme/Rules")
	result, err := Release(context.Background(), ReleaseRequest{Options: options, NoGitHubRelease: true})
	if err != nil || !result.TagCreated || result.GitHubRepository != "Acme/Rules" || result.Repository != "https://github.com/Acme/Rules" {
		t.Fatal(result, err)
	}
	if _, err := fixture.Command(context.Background(), "rev-parse", "--verify", "refs/tags/release/1"); err != nil {
		t.Fatal(err)
	}
}

// TestRelease_SkipsTheGitHubReleasePageWhenAsked publishes the tag without calling gh.
func TestRelease_SkipsTheGitHubReleasePageWhenAsked(t *testing.T) {
	fixture, options := authorClone(t, libraryFiles())
	fake := withGitHub(t, fixture, &options, nil)
	result, err := Release(context.Background(), ReleaseRequest{Options: options, NoGitHubRelease: true})
	if err != nil || !result.TagCreated || result.GitHubRelease != nil || result.GitHubRepository != "acme/rules" {
		t.Fatal(result, err)
	}
	requireCalls(t, fake, []ghfixture.Call{})
}

// TestRelease_DryRunDescribesTheLibraryReleaseWithoutPublishing leaves tags alone and never calls gh.
func TestRelease_DryRunDescribesTheLibraryReleaseWithoutPublishing(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	commit := commitAndPush(t, fixture, options.Directory, map[string][]byte{"practices/testing/a.md": []byte(ruleText("Changed.")), "changes/a.yaml": []byte("summary: Clarify a.\nrules:\n  practices/testing/a: patch\n")})
	fake := withGitHub(t, fixture, &options, []ghfixture.Response{{Args: []string{"auth", "status", "--hostname", "github.com"}}})
	local, remote := tags(t, fixture, options.Directory), tags(t, fixture, remoteDir(fixture))
	result, err := Release(ctx, ReleaseRequest{Options: options, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Release != 2 || result.Tag != "release/2" || result.Commit != commit || result.Branch != "main" || result.TagCreated || result.GitHubRelease != nil ||
		!strings.HasPrefix(result.Notes, "Library release 2 changes 1 rule: 1 patch.\n") || !slices.Equal(previewRows(PendingRelease{Rules: result.Rules}), []string{"practices/testing/a patch 1.0.0 1.0.1"}) {
		t.Fatalf("%+v", result)
	}
	if tags(t, fixture, options.Directory) != local || tags(t, fixture, remoteDir(fixture)) != remote {
		t.Fatal("a dry run changed tags")
	}
	// The dry run only checks that gh is signed in, as the real run would before creating anything.
	requireCalls(t, fake, []ghfixture.Call{{Args: []string{"auth", "status", "--hostname", "github.com"}}})
}

// TestRelease_DryRunRefusesAsTheRealRunWouldWithoutTheGitHubCLI reports a missing or signed-out gh on a dry run.
func TestRelease_DryRunRefusesAsTheRealRunWouldWithoutTheGitHubCLI(t *testing.T) {
	for _, test := range []struct {
		name, code string
		responses  []ghfixture.Response
	}{
		{"signed out", "github-cli-signed-out", []ghfixture.Response{{Args: []string{"auth", "status", "--hostname", "github.com"}, ExitCode: 1}}},
		{"not installed", "github-cli-missing", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, options := authorClone(t, libraryFiles(), releaseOne)
			commitAndPush(t, fixture, options.Directory, map[string][]byte{"practices/testing/a.md": []byte(ruleText("Changed.")), "changes/a.yaml": []byte("summary: Clarify a.\nrules:\n  practices/testing/a: patch\n")})
			withGitHub(t, fixture, &options, test.responses)
			if test.responses == nil {
				if err := os.Remove(filepath.Join(pathOf(&options), "gh")); err != nil {
					t.Fatal(err)
				}
			}
			for _, dryRun := range []bool{true, false} {
				if _, err := Release(context.Background(), ReleaseRequest{Options: options, DryRun: dryRun}); errorCode(err) != test.code {
					t.Fatalf("dry run %v: got %v, want %s", dryRun, err, test.code)
				}
			}
		})
	}
}

// TestRelease_StopsCleanlyWhenSomeoneElsePublishesFirst lets the author's pre-push hook publish release/2
// elsewhere first, so the push is rejected, and requires the clone to keep no release/2 of its own.
func TestRelease_StopsCleanlyWhenSomeoneElsePublishesFirst(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	commitAndPush(t, fixture, options.Directory, map[string][]byte{"practices/testing/a.md": []byte(ruleText("Changed.")), "changes/a.yaml": []byte("summary: Clarify a.\nrules:\n  practices/testing/a: patch\n")})
	// The hook runs Git with an empty environment, so nothing from the author's push reaches the other repository.
	hook := "#!/bin/sh\nexec /usr/bin/env -i GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_COMMITTER_NAME=Other GIT_COMMITTER_EMAIL=other@example.invalid " +
		gitfixture.Quote(fixture.GitPath) + " -C " + gitfixture.Quote(remoteDir(fixture)) + " -c core.hooksPath=/dev/null -c tag.gpgsign=false tag --annotate --message 'Published elsewhere.' release/2\n"
	hooks := filepath.Join(options.Directory, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-push"), []byte(hook), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := Release(ctx, ReleaseRequest{Options: options})
	if errorCode(err) != "release-conflict" || !strings.Contains(err.Error(), "someone else published release/2 to origin first") {
		t.Fatal(err)
	}
	if message := tagMessage(t, fixture, remoteDir(fixture), "release/2"); message != "Published elsewhere.\n" {
		t.Fatalf("the remote's release/2 changed:\n%s", message)
	}
	if strings.Contains(tags(t, fixture, options.Directory), "release/2") {
		t.Fatal("the clone kept a release/2 that disagrees with the remote")
	}
}

// TestRelease_RefusesWhenTheRemoteChangesBetweenListingAndFetching changes the remote just before the fetch,
// through a transport that runs the change on its second connection, after the listing.
func TestRelease_RefusesWhenTheRemoteChangesBetweenListingAndFetching(t *testing.T) {
	for _, test := range []struct {
		name string
		// change runs in the remote repository, with arguments for Git.
		change  string
		message string
	}{
		{"branch", "commit --quiet --allow-empty --message Elsewhere", "origin changed while trailguide library release was reading it: someone pushed to main."},
		{"release tag", "tag --force --annotate --cleanup=verbatim --message " + gitfixture.Quote(strings.Replace(releaseOne, "Add a.", "Add rule a.", 1)) + " release/1", "origin changed while trailguide library release was reading it: release/1 changed."},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, options := authorClone(t, libraryFiles(), releaseOne)
			// Without its own release/1, the clone fetches the remote's.
			run(t, fixture, options.Directory, "tag", "--delete", "release/1")
			state := filepath.Join(t.TempDir(), "connections")
			transport := filepath.Join(t.TempDir(), "ssh")
			// The change runs Git with an empty environment, so nothing from the author's Git reaches the remote.
			script := "#!/bin/sh\nn=$(/bin/cat " + gitfixture.Quote(state) + " 2>/dev/null || echo 0)\nn=$((n + 1))\necho $n > " + gitfixture.Quote(state) + "\n" +
				"if [ $n = 2 ]; then /usr/bin/env -i GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_AUTHOR_NAME=Other GIT_AUTHOR_EMAIL=other@example.invalid GIT_COMMITTER_NAME=Other GIT_COMMITTER_EMAIL=other@example.invalid " +
				gitfixture.Quote(fixture.GitPath) + " -C " + gitfixture.Quote(remoteDir(fixture)) + " -c core.hooksPath=/dev/null -c tag.gpgsign=false -c commit.gpgsign=false " + test.change + " >/dev/null 2>&1 || exit 1; fi\n" +
				"exec " + gitfixture.Quote(filepath.Join(fixture.Directory, "ssh")) + " \"$@\"\n"
			if err := os.WriteFile(transport, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			options.Git.Environment = append(slices.Clone(options.Git.Environment), "GIT_SSH_COMMAND="+gitfixture.Quote(transport))
			before := tags(t, fixture, remoteDir(fixture))
			_, err := Release(context.Background(), ReleaseRequest{Options: options})
			if errorCode(err) != "remote-changed" || !strings.Contains(err.Error(), test.message) {
				t.Fatal(err)
			}
			if connections, _ := os.ReadFile(state); string(connections) != "2\n" {
				t.Fatalf("connections: %q", connections)
			}
			if test.name == "branch" && tags(t, fixture, remoteDir(fixture)) != before {
				t.Fatal("published after the branch moved")
			}
		})
	}
}

// TestRequireTagSize_AcceptsTagsUpToTheReadersLimit refuses one byte more than reading release tags accepts.
func TestRequireTagSize_AcceptsTagsUpToTheReadersLimit(t *testing.T) {
	if err := requireTagSize("release/2", maxFileBytes); err != nil {
		t.Fatal(err)
	}
	if err := requireTagSize("release/2", maxFileBytes+1); errorCode(err) != "release-too-large" || !strings.Contains(err.Error(), "release/2 would be a tag of 8388609 bytes") {
		t.Fatal(err)
	}
}

// TestCreateTag_SizesTagsLikeGitAndDeletesOneTooLarge predicts an unsigned tag's size exactly, and deletes a
// created tag larger than the limit, as a signature could make it, instead of leaving it to be pushed.
func TestCreateTag_SizesTagsLikeGitAndDeletesOneTooLarge(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles())
	git, err := openLibraryGit(ctx, options.Directory, options.Git)
	if err != nil {
		t.Fatal(err)
	}
	tagger, err := git.requireCommitterIdentity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("Notes.\n---\nformatVersion: 1\nrelease: 1\nrules: {}\n")
	object, err := git.createTag(ctx, "release/1", fixture.LatestCommit, message)
	if err != nil {
		t.Fatal(err)
	}
	size, err := fixture.CommandIn(ctx, options.Directory, "cat-file", "-s", object)
	if err != nil || size != strconv.Itoa(tagObjectSize("release/1", fixture.LatestCommit, tagger, message)) {
		t.Fatalf("Git's tag has %s bytes, predicted %d: %v", size, tagObjectSize("release/1", fixture.LatestCommit, tagger, message), err)
	}
	large := []byte(strings.Repeat("x", maxFileBytes-tagObjectSize("release/2", fixture.LatestCommit, tagger, nil)+1))
	before := tags(t, fixture, options.Directory)
	if _, err := git.createTag(ctx, "release/2", fixture.LatestCommit, large); errorCode(err) != "release-too-large" {
		t.Fatal(err)
	}
	if tags(t, fixture, options.Directory) != before {
		t.Fatal("kept a tag too large to read")
	}
}

// TestRelease_RefusesATagTooLargeToRead stops before tagging when the notes and record exceed 8 MiB, here
// through one long summary that both rules repeat in the notes and in the record.
func TestRelease_RefusesATagTooLargeToRead(t *testing.T) {
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	summary := strings.Repeat("Explain the retry change. ", 90_000)
	commitAndPush(t, fixture, options.Directory, map[string][]byte{
		"practices/testing/a.md": []byte(ruleText("Changed a.")),
		"practices/testing/b.md": []byte(ruleText("Changed b.")),
		"changes/both.yaml":      []byte("summary: " + summary + "\nrules:\n  practices/testing/a: patch\n  practices/testing/b: patch\n"),
	})
	local, remote := tags(t, fixture, options.Directory), tags(t, fixture, remoteDir(fixture))
	if _, err := Release(context.Background(), ReleaseRequest{Options: options}); errorCode(err) != "release-too-large" {
		t.Fatal(err)
	}
	if tags(t, fixture, options.Directory) != local || tags(t, fixture, remoteDir(fixture)) != remote {
		t.Fatal("tagged a library release too large to read")
	}
}

// TestRelease_DeletesItsTagWhenThePushFails runs the author's pre-push hook, which refuses the push, and
// leaves no release tag in the clone or on the remote, so a rerun starts over.
func TestRelease_DeletesItsTagWhenThePushFails(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles())
	hooks := filepath.Join(options.Directory, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-push"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	local, remote := tags(t, fixture, options.Directory), tags(t, fixture, remoteDir(fixture))
	_, err := Release(ctx, ReleaseRequest{Options: options})
	if errorCode(err) != "push-failed" || !strings.Contains(err.Error(), "Git couldn't push release/1 to origin. Check your network connection, your access to the repository, and any pre-push hook") {
		t.Fatal(err)
	}
	if tags(t, fixture, options.Directory) != local || tags(t, fixture, remoteDir(fixture)) != remote {
		t.Fatal("a failed push left a release tag behind")
	}
	if err := os.Remove(filepath.Join(hooks, "pre-push")); err != nil {
		t.Fatal(err)
	}
	if result, err := Release(ctx, ReleaseRequest{Options: options}); err != nil || !result.TagCreated || result.Release != 1 {
		t.Fatal(result, err)
	}
}

// interruptedRelease creates the release tag that trailguide library release would publish from options'
// library, as a run interrupted before pushing it leaves it, and returns its release notes.
func interruptedRelease(t *testing.T, options Options) string {
	t.Helper()
	planned, head := planFixtureRelease(t, options)
	git, err := openLibraryGit(context.Background(), options.Directory, options.Git)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := git.createTag(context.Background(), "release/"+strconv.Itoa(planned.record.Release), head, planned.message); err != nil {
		t.Fatal(err)
	}
	return planned.notes
}

// planFixtureRelease returns the library release that trailguide library release would publish from options'
// library, and the commit it would tag.
func planFixtureRelease(t *testing.T, options Options) (plannedRelease, string) {
	t.Helper()
	ctx := context.Background()
	root, err := openLibrary(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	git, err := openLibraryGit(ctx, root.Name(), options.Git)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := checkLibrary(ctx, root, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err := git.headCommit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := planRelease(ctx, git, checked, head)
	if err != nil {
		t.Fatal(err)
	}
	return planned, head
}

// TestRelease_RefusesAnUnpublishedTagTooLargeToRead pushes an unpublished release tag of exactly 8 MiB, and
// refuses one a byte larger, counting a signature the release record's parser ignores.
func TestRelease_RefusesAnUnpublishedTagTooLargeToRead(t *testing.T) {
	for _, test := range []struct {
		name string
		size int
		code string
	}{{"at the limit", maxFileBytes, ""}, {"one byte over", maxFileBytes + 1, "release-too-large"}} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			fixture, options := authorClone(t, libraryFiles())
			planned, _ := planFixtureRelease(t, options)
			// A fixed date keeps the tag's header the same size when the tag is created again.
			fixture.Environment = append(slices.Clone(fixture.Environment), "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
			message := func(padding int) string {
				return string(planned.message) + "-----BEGIN SSH SIGNATURE-----\n" + strings.Repeat("A", padding) + "\n-----END SSH SIGNATURE-----\n"
			}
			file := filepath.Join(t.TempDir(), "message")
			tag := func(padding int) int {
				if err := os.WriteFile(file, []byte(message(padding)), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := fixture.CommandIn(ctx, options.Directory, "tag", "--force", "--annotate", "--cleanup=verbatim", "--file="+file, "release/1"); err != nil {
					t.Fatal(err)
				}
				size, err := fixture.CommandIn(ctx, options.Directory, "cat-file", "-s", "release/1")
				if err != nil {
					t.Fatal(err)
				}
				bytes, _ := strconv.Atoi(size)
				return bytes
			}
			if size := tag(test.size - tag(0)); size != test.size {
				t.Fatalf("tag of %d bytes, want %d", size, test.size)
			}
			remote := tags(t, fixture, remoteDir(fixture))
			result, err := Release(ctx, ReleaseRequest{Options: options})
			if errorCode(err) != test.code {
				t.Fatalf("got %+v, %v; want %q", result, err, test.code)
			}
			if pushed := tags(t, fixture, remoteDir(fixture)) != remote; pushed != (test.code == "") {
				t.Fatalf("pushed the tag: %v", pushed)
			}
		})
	}
}

// TestRelease_PushesATagAnInterruptedRunLeftOnTheCommit publishes a release tag that exists only in the clone,
// on HEAD, numbered one past the remote's latest, and keeps it when the push fails.
func TestRelease_PushesATagAnInterruptedRunLeftOnTheCommit(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles())
	notes := interruptedRelease(t, options)
	local := tags(t, fixture, options.Directory)
	// A failed push keeps the tag, which this run didn't create.
	hook := filepath.Join(options.Directory, ".git", "hooks", "pre-push")
	if err := os.MkdirAll(filepath.Dir(hook), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Release(ctx, ReleaseRequest{Options: options}); errorCode(err) != "push-failed" || !strings.Contains(err.Error(), "This clone's release/1, from an earlier run, stays") {
		t.Fatal(err)
	}
	if tags(t, fixture, options.Directory) != local {
		t.Fatal("a failed push deleted a tag an earlier run created")
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	result, err := Release(ctx, ReleaseRequest{Options: options})
	if err != nil || !result.TagCreated || result.Published || result.Release != 1 || result.Notes != notes {
		t.Fatal(result, err)
	}
	if tags(t, fixture, remoteDir(fixture)) != local {
		t.Fatal("the remote's tags differ from the clone's")
	}
	result, err = Release(ctx, ReleaseRequest{Options: options})
	if err != nil || result.TagCreated || !result.Published {
		t.Fatal(result, err)
	}
}

// TestRelease_AsksToUpgradeForAnUnpublishedTagInANewerFormat reports an unpublished release tag whose record
// format this version can't read as a reason to upgrade, not as a mismatch to delete.
func TestRelease_AsksToUpgradeForAnUnpublishedTagInANewerFormat(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles())
	// A newer Trail Guide created this tag, then stopped before pushing it.
	newer := "Library release 1.\n\n---\nformatVersion: 2\nrelease: 1\nrules: A later shape.\n"
	if err := fixture.Tag(ctx, options.Directory, "release/1", newer); err != nil {
		t.Fatal(err)
	}
	local, remote := tags(t, fixture, options.Directory), tags(t, fixture, remoteDir(fixture))
	_, err := Release(ctx, ReleaseRequest{Options: options})
	if errorCode(err) != "unsupported-release-record" || !strings.Contains(err.Error(), "Upgrade Trail Guide") || strings.Contains(err.Error(), "git tag --delete") {
		t.Fatalf("got %v; want unsupported-release-record asking to upgrade", err)
	}
	if tags(t, fixture, options.Directory) != local || tags(t, fixture, remoteDir(fixture)) != remote {
		t.Fatal("a refused library release changed the tags")
	}
}

// TestRelease_RefusesAnUnpublishedTagThatDoesntMatchTheLibrary checks a release tag found only in the clone
// against the library release its commit would publish after the latest one the remote published, and pushes
// nothing when they differ: a hand-made tag can't skip a missing change note or change what a note publishes.
func TestRelease_RefusesAnUnpublishedTagThatDoesntMatchTheLibrary(t *testing.T) {
	changed := map[string][]byte{"practices/testing/a.md": []byte(ruleText("Test the retry limit and one past it."))}
	unchanged := "Library release 2.\n\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 1.0.0\n  practices/testing/b: 1.0.0\n"
	for _, test := range []struct {
		name, code, message string
		files               map[string][]byte
	}{
		{"a change without a note", "change-notes", unchanged, changed},
		{"a record that differs from the notes", "release-tag-mismatch", unchanged, map[string][]byte{"practices/testing/a.md": changed["practices/testing/a.md"], "changes/a.yaml": []byte("summary: Test one past the limit.\nrules:\n  practices/testing/a: major\n")}},
		{"notes that differ from the record", "release-tag-mismatch", "Nothing risky.\n\n---\nformatVersion: 1\nrelease: 2\nrules:\n  practices/testing/a: 2.0.0\n  practices/testing/b: 1.0.0\nchanges:\n  practices/testing/a:\n    change: major\n    from: 1.0.0\n    summaries:\n      - Test one past the limit.\n", map[string][]byte{"practices/testing/a.md": changed["practices/testing/a.md"], "changes/a.yaml": []byte("summary: Test one past the limit.\nrules:\n  practices/testing/a: major\n")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			fixture, options := authorClone(t, libraryFiles(), releaseOne)
			commitAndPush(t, fixture, options.Directory, test.files)
			if err := fixture.Tag(ctx, options.Directory, "release/2", test.message); err != nil {
				t.Fatal(err)
			}
			remote := tags(t, fixture, remoteDir(fixture))
			_, err := Release(ctx, ReleaseRequest{Options: options})
			if errorCode(err) != test.code {
				t.Fatalf("got %v; want %s", err, test.code)
			}
			if tags(t, fixture, remoteDir(fixture)) != remote {
				t.Fatal("pushed a release tag that doesn't match the library")
			}
		})
	}
}

// TestFindGitHubCLI_StopsAGitHubCLIThatFloodsItsOutput stops a gh that keeps writing, ignoring broken pipes,
// with a descendant holding its output open, instead of waiting for it forever.
func TestFindGitHubCLI_StopsAGitHubCLIThatFloodsItsOutput(t *testing.T) {
	bin := t.TempDir()
	// The fake writes 64 KiB at a time, so it passes the 1 MiB limit in a few writes. With 16-byte writes, the
	// 65,536 writes that reaching the limit took could alone outlast the timeout below on a busy machine.
	script := "#!/bin/sh\ntrap '' PIPE\n/bin/sleep 30 &\nx=xxxxxxxxxxxxxxxx\nwhile [ ${#x} -lt 65536 ]; do x=$x$x; done\nwhile :; do printf %s \"$x\" 2>/dev/null; done\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := findGitHubCLI(context.Background(), []string{"PATH=" + bin}, t.TempDir())
		done <- err
	}()
	select {
	case err := <-done:
		if errorCode(err) != "limit-exceeded" {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("gh's output overflowed, and the release kept waiting for it")
	}
}

// TestRelease_RefusesALibraryOutsideGit because a library release is a tag.
func TestRelease_RefusesALibraryOutsideGit(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	for name, content := range libraryFiles() {
		edit(t, options.Directory, map[string]string{name: string(content)})
	}
	if _, err := Release(context.Background(), ReleaseRequest{Options: options}); errorCode(err) != "not-a-repository" {
		t.Fatal(err)
	}
}

// TestChangedLibraryFiles_ListsAddedChangedAndDeletedLibraryWideFiles leaves out rule content, change notes, and
// group READMEs.
func TestChangedLibraryFiles_ListsAddedChangedAndDeletedLibraryWideFiles(t *testing.T) {
	released := map[string]string{
		"config.yaml":                   "1",
		"LICENSE.md":                    "1",
		"assets/old.svg":                "1",
		"practices/testing/_group.yaml": "1",
		"practices/testing/README.md":   "1",
		"practices/testing/a.md":        "1",
		"practices/testing/assets/a/x":  "1",
		"changes/a.yaml":                "1",
	}
	head := map[string]string{
		"config.yaml":                   "1",
		"LICENSE.md":                    "2",
		"assets/new.svg":                "1",
		"practices/testing/_group.yaml": "1",
		"practices/testing/README.md":   "2",
		"practices/testing/a.md":        "2",
		"practices/testing/assets/a/x":  "2",
		"changes/a.yaml":                "2",
		"techs/go/_group.yaml":          "1",
	}
	want := []string{"LICENSE.md", "assets/new.svg", "assets/old.svg", "techs/go/_group.yaml"}
	if files := diffLibraryFiles(head, released, []string{"LICENSE.md"}); !slices.Equal(files, want) {
		t.Fatalf("got %q, want %q", files, want)
	}
	want = []string{"LICENSE.md", "assets/new.svg", "config.yaml", "practices/testing/_group.yaml", "techs/go/_group.yaml"}
	if files := diffLibraryFiles(head, map[string]string{}, []string{"LICENSE.md"}); !slices.Equal(files, want) {
		t.Fatalf("first library release: got %q, want %q", files, want)
	}
}

// TestParseRemoteListing_ReadsTheDefaultBranchAndReleaseTagsWithinTheRecordLimit counts every tag record,
// including peeled ones, toward 20,000, and ignores tags Trail Guide doesn't use.
func TestParseRemoteListing_ReadsTheDefaultBranchAndReleaseTagsWithinTheRecordLimit(t *testing.T) {
	main := upstream{branch: "main", remote: "origin", ref: "refs/heads/main"}
	commit, tag := strings.Repeat("a", 40), strings.Repeat("b", 64)
	header := "ref: refs/heads/main\tHEAD\n" + commit + "\tHEAD\n" + commit + "\trefs/heads/main\n"
	listing := func(records int) string {
		var out strings.Builder
		out.WriteString(header)
		for i := range records {
			out.WriteString(tag + "\trefs/tags/release/" + strconv.Itoa(i/2+1))
			if i%2 == 1 {
				out.WriteString("^{}")
			}
			out.WriteString("\n")
		}
		return out.String()
	}
	state, err := parseRemoteListing(listing(20_000)+commit+"\trefs/tags/release/01\n", main)
	if errorCode(err) != "limit-exceeded" {
		t.Fatalf("20,001 tag records: %v", err)
	}
	state, err = parseRemoteListing(listing(20_000), main)
	if err != nil || state.defaultBranch != "refs/heads/main" || state.head != commit || len(state.tags) != 10_000 || state.latest() != 10_000 || state.tags[1] != tag {
		t.Fatalf("20,000 tag records: %d tags, latest %d: %v", len(state.tags), state.latest(), err)
	}
	for _, test := range []struct{ name, listing, code string }{
		{"another default branch", "ref: refs/heads/trunk\tHEAD\n" + commit + "\trefs/heads/main\n", "not-default-branch"},
		{"no default branch", commit + "\trefs/heads/main\n", "not-default-branch"},
		{"malformed line", header + "release/1\n", "git-failed"},
		{"malformed object", header + "xyz\trefs/tags/release/1\n", "git-failed"},
	} {
		if _, err := parseRemoteListing(test.listing, main); errorCode(err) != test.code {
			t.Errorf("%s: %v, want %s", test.name, err, test.code)
		}
	}
}

// TestGitHubRepository_RecognizesOnlyGitHubDotCom, with or without credentials in the URL, and shows remotes
// without credentials, queries, or fragments.
func TestGitHubRepository_RecognizesOnlyGitHubDotCom(t *testing.T) {
	for _, test := range []struct{ url, github, display string }{
		{"git@github.com:Acme/Rules.git", "Acme/Rules", "https://github.com/Acme/Rules"},
		{"https://github.com/acme/rules", "acme/rules", "https://github.com/acme/rules"},
		{"ssh://git@github.com/acme/rules.git", "acme/rules", "https://github.com/acme/rules"},
		{"https://x-access-token:secret@github.com/acme/rules.git", "acme/rules", "https://github.com/acme/rules"},
		{"https://secret-token@github.com/acme/rules.git?secret=1#secret", "acme/rules", "https://github.com/acme/rules"},
		{"https://user:secret@host.example/library.git?access_token=secret#secret", "", "https://host.example/library.git"},
		{"ssh://git:secret@host.example:2222/library.git?secret", "", "ssh://git@host.example:2222/library.git"},
		{"git@host.example:library.git?secret", "", "git@host.example:library.git"},
		{"https://gitlab.com/acme/rules.git", "", "https://gitlab.com/acme/rules"},
		{"git@github-work:acme/rules.git", "", "git@github-work:acme/rules.git"},
		{"https://github.example.com/acme/rules.git", "", "https://github.example.com/acme/rules.git"},
		{"/srv/git/rules.git", "", "/srv/git/rules.git"},
		{"https://review-secret:password@github.com/acme/%zz", "", hiddenRemote},
		{"https://review-secret:password@host.example:port/library.git", "", hiddenRemote},
		{"https://someone:p@ss@host.example/library.git", "", "https://host.example/library.git"},
		{"//user:secret@host.invalid/rules", "", hiddenRemote},
		{"//host.invalid/rules", "", hiddenRemote},
		{"user:secret@host.invalid:rules", "", hiddenRemote},
		{"secret@host.invalid:rules?x#y", "", "secret@host.invalid:rules"},
	} {
		if github, display := gitHubRepository(test.url), displayRepository(test.url); github != test.github || display != test.display {
			t.Errorf("%s: GitHub repository %q, display %q; want %q, %q", test.url, github, display, test.github, test.display)
		}
	}
}

// TestSameRepository_ComparesURLsItCantParseExactly treats two different URLs it can't parse as different
// repositories, although both display the same way, and ignores credentials in URLs it can parse.
func TestSameRepository_ComparesURLsItCantParseExactly(t *testing.T) {
	for _, test := range []struct {
		a, b string
		same bool
	}{
		{"https://token@host.example/library.git", "https://host.example/library.git?secret", true},
		{"https://secret@host.example/%zz", "https://secret@host.example/%zz", true},
		{"https://secret@host.example/%zz", "https://secret@other.example/%zz", false},
	} {
		if got := sameRepository(test.a, test.b); got != test.same {
			t.Errorf("sameRepository(%q, %q) = %v, want %v", test.a, test.b, got, test.same)
		}
	}
}
