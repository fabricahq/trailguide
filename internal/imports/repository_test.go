// Exercise actual Git transport, exact revision fetches, and bounded process failures.

package imports

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// fixtureRepository creates a real repository served by an isolated local upload-pack helper.
func fixtureRepository(t *testing.T) *gitfixture.Fixture {
	t.Helper()
	f, err := gitfixture.New(context.Background(), map[string][]byte{"README.md": []byte("fixture\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	return f
}

// fetchRevision opens a partial repository for source and fetches ref, which it parses as the source's ref, unless
// ref is empty, returning the repository, which the caller closes, and the ref's commit. Failure closes the
// repository.
func fetchRevision(ctx context.Context, source rules.Source, ref string, options Options) (*repository, string, error) {
	if ref != "" {
		parsed, err := rules.ParseGitRef(ref, "ref")
		if err != nil {
			return nil, "", err
		}
		source.Ref = parsed
	}
	repo, err := openRepository(ctx, source, options)
	if err != nil {
		return nil, "", err
	}
	commit, _, err := repo.fetchRef(ctx, source)
	if err != nil {
		return nil, "", errors.Join(err, repo.Close())
	}
	return repo, commit, nil
}

// gitRef parses text as a ref, failing the test when it isn't one.
func gitRef(t *testing.T, text string) rules.GitRef {
	t.Helper()
	ref, err := rules.ParseGitRef(text, "ref")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// requireCode checks the stable failure category without matching Git's private diagnostics.
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

// TestFetchRevision exercises exact commits, lightweight and annotated tags, and missing refs.
func TestFetchRevision(t *testing.T) {
	f := fixtureRepository(t)
	options := Options{GitPath: f.GitPath, Environment: f.Environment}
	for _, tc := range []struct{ name, ref, want string }{
		{"commit", f.FirstCommit, f.FirstCommit}, {"lightweight", "v1.0.0", f.FirstCommit}, {"annotated", "v1.2.0", f.LatestCommit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			revision, commit, err := fetchRevision(context.Background(), rules.Source{Name: "team", Repository: f.Repository}, tc.ref, options)
			if err != nil {
				t.Fatal(err)
			}
			dir := revision.directory
			if commit != tc.want {
				t.Fatalf("commit %s", commit)
			}
			if _, err := os.Stat(filepath.Join(dir, "README.md")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("working files were checked out")
			}
			if err := revision.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("temporary repository retained")
			}
		})
	}
	revision, _, err := fetchRevision(context.Background(), rules.Source{Name: "team", Repository: f.Repository}, "missing", options)
	requireCode(t, err, "version-not-found")
	if revision != nil {
		t.Fatal("partial result")
	}
}

// TestFetchRef_ResolvesABranchOrTagToItsCommit resolves a bare name as the branch or the tag the repository has by
// that name, and a prefixed name as that kind only, reporting which one it named, and fails with ambiguous-ref for a
// bare name of both and version-not-found, naming the ref, for one of neither.
func TestFetchRef_ResolvesABranchOrTagToItsCommit(t *testing.T) {
	f := fixtureRepository(t)
	ctx := context.Background()
	options := Options{GitPath: f.GitPath, Environment: f.Environment}
	if _, err := f.Command(ctx, "branch", "stable", f.FirstCommit); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Command(ctx, "tag", "stable", f.LatestCommit); err != nil {
		t.Fatal(err)
	}
	resolve := func(ref string) (string, rules.GitRefKind, error) {
		source := rules.Source{Name: "skills", Repository: f.Repository, Ref: gitRef(t, ref)}
		repo, err := openRepository(ctx, source, options)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := repo.Close(); err != nil {
				t.Error(err)
			}
		}()
		return repo.fetchRef(ctx, source)
	}
	for _, tc := range []struct {
		ref, commit string
		kind        rules.GitRefKind
	}{
		{"main", f.LatestCommit, rules.GitRefBranch},
		{"refs/heads/main", f.LatestCommit, rules.GitRefBranch},
		{"v1.2.0", f.LatestCommit, rules.GitRefTag},
		{"refs/heads/stable", f.FirstCommit, rules.GitRefBranch},
		{"refs/tags/stable", f.LatestCommit, rules.GitRefTag},
	} {
		if commit, kind, err := resolve(tc.ref); err != nil || commit != tc.commit || kind != tc.kind {
			t.Errorf("%s: got %s, %s, %v; want %s, %s", tc.ref, commit, kind, err, tc.commit, tc.kind)
		}
	}
	_, _, err := resolve("stable")
	requireCode(t, err, "ambiguous-ref")
	if want := "sources.skills.ref: the repository has both a branch and a tag named stable. Set sources.skills.ref to refs/heads/stable for the branch, or to refs/tags/stable for the tag."; err.Error() != want {
		t.Errorf("ambiguous: %v", err)
	}
	for ref, want := range map[string]string{
		"gone":              "sources.skills.ref: the repository has no branch or tag gone; check the ref, and whether a branch of that name was renamed or deleted.",
		"refs/heads/gone":   "sources.skills.ref: the repository has no branch refs/heads/gone; check the ref, and whether the branch was renamed or deleted.",
		"refs/heads/v1.0.0": "sources.skills.ref: the repository has no branch refs/heads/v1.0.0; check the ref, and whether the branch was renamed or deleted.",
	} {
		_, _, err := resolve(ref)
		requireCode(t, err, "version-not-found")
		if err.Error() != want {
			t.Errorf("%s: %v", ref, err)
		}
	}
}

// TestFetchRejectsNonCommitTags refuses a tag that names a blob instead of a commit.
func TestFetchRejectsNonCommitTags(t *testing.T) {
	f := fixtureRepository(t)
	ctx := context.Background()
	options := Options{GitPath: f.GitPath, Environment: f.Environment}
	blob, err := f.Command(ctx, "rev-parse", "HEAD:README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Command(ctx, "tag", "blob", blob); err != nil {
		t.Fatal(err)
	}
	_, _, err = fetchRevision(ctx, rules.Source{Repository: f.Repository}, "blob", options)
	requireCode(t, err, "unsupported-content")
}

// TestEnvironmentIsolation verifies inherited repository state cannot alter the fetched commit.
func TestEnvironmentIsolation(t *testing.T) {
	f := fixtureRepository(t)
	environment := append(append([]string{}, f.Environment...), "GIT_DIR=/missing", "GIT_WORK_TREE=/missing", "GIT_INDEX_FILE=/missing", "GIT_OBJECT_DIRECTORY=/missing", "GIT_ALTERNATE_OBJECT_DIRECTORIES=/missing", "GIT_NAMESPACE=hidden", "GIT_SHALLOW_FILE=/missing")
	r, commit, err := fetchRevision(context.Background(), rules.Source{Repository: f.Repository}, "v1.0.0", Options{GitPath: f.GitPath, Environment: environment})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if commit != f.FirstCommit {
		t.Fatal("inherited repository state affected fetch")
	}
}

// TestExpiredContextCodes preserves separate timeout and cancellation categories before any Git process starts.
func TestExpiredContextCodes(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		want := "cancelled"
		cause := context.Canceled
		if deadline {
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			want = "timed-out"
			cause = context.DeadlineExceeded
		} else {
			cancel()
		}
		_, _, err := fetchRevision(ctx, rules.Source{}, "", Options{})
		requireCode(t, err, want)
		if !errors.Is(err, cause) {
			t.Fatal("lost context cause")
		}
		cancel()
	}
}

// TestFetchUsesEnvironmentPath resolves Git from the same environment supplied to its children.
func TestFetchUsesEnvironmentPath(t *testing.T) {
	f := fixtureRepository(t)
	bin := t.TempDir()
	if err := os.Symlink(f.GitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/missing-parent-path")
	env := append(append([]string{}, f.Environment...), "PATH="+bin+":/usr/bin:/bin")
	revision, commit, err := fetchRevision(context.Background(), rules.Source{Repository: f.Repository}, "v1.0.0", Options{Environment: env})
	if err != nil {
		t.Fatal(err)
	}
	defer revision.Close()
	if commit != f.FirstCommit {
		t.Fatal("wrong fetched revision")
	}
}

// TestFetchDoesNotFallBackToParentPath rejects missing Git in an explicitly replaced environment.
func TestFetchDoesNotFallBackToParentPath(t *testing.T) {
	f := fixtureRepository(t)
	_, _, err := fetchRevision(context.Background(), rules.Source{Repository: f.Repository}, "v1.0.0", Options{Environment: []string{"PATH=/missing-custom-path"}})
	requireCode(t, err, "git-unavailable")
}

// TestFetchSkipsRelativePathEntries finds trusted Git after an excluded current-directory executable.
func TestFetchSkipsRelativePathEntries(t *testing.T) {
	f := fixtureRepository(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 42\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(bin)
	env := append(append([]string{}, f.Environment...), "PATH=.:"+filepath.Dir(f.GitPath)+":/usr/bin:/bin")
	revision, commit, err := fetchRevision(context.Background(), rules.Source{Repository: f.Repository}, "v1.0.0", Options{Environment: env})
	if err != nil {
		t.Fatal(err)
	}
	defer revision.Close()
	if commit != f.FirstCommit {
		t.Fatal("wrong fetched revision")
	}
}

// TestFetchRevisionFailsBeforeStartingGit reports cancellation and a missing Git executable without partial state.
func TestFetchRevisionFailsBeforeStartingGit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := fetchRevision(ctx, rules.Source{}, "", Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, _, err = fetchRevision(context.Background(), rules.Source{Repository: "https://example.invalid/rules"}, "v1.0.0", Options{GitPath: "/missing/git"})
	requireCode(t, err, "git-unavailable")
}

// TestReadSkillsFetchesOnlyTheSkillsTheSourceInstalls downloads the files of the included skill and the skill it uses,
// but not the SKILL.md of a skill nothing installed uses, which a large repository may have many of.
func TestReadSkillsFetchesOnlyTheSkillsTheSourceInstalls(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{
		"skills/selected/SKILL.md": []byte("---\nname: selected\ndescription: Use selected.\nmetadata:\n  trailguide-skills-used: skills/used\n---\n"),
		"skills/used/SKILL.md":     []byte("---\nname: used\ndescription: Use used.\n---\n"),
		"skills/unused/SKILL.md":   []byte("---\nname: unused\ndescription: Use unused.\n---\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	source := rules.Source{Name: "team", Repository: f.Repository, Skills: []string{"skills/selected"}}
	repo, commit, err := fetchRevision(ctx, source, "v1.0.0", Options{GitPath: f.GitPath, Environment: f.Environment})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	imported, err := repo.readSkills(ctx, source, sourcePlan{commit: commit}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(imported.skills)); !slices.Equal(got, []string{"skills/selected", "skills/used"}) {
		t.Fatalf("installed %v", got)
	}
	tree, err := repo.repositoryTree(ctx, commit)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := repo.runner.Output(ctx, repo.directory, []string{"cat-file", "--batch-all-objects", "--batch-check=%(objectname)"}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if unused := tree["skills/unused/SKILL.md"].object; strings.Contains(string(listing), unused) {
		t.Fatalf("fetched skills/unused/SKILL.md (%s), which no installed skill uses", unused)
	}
}
