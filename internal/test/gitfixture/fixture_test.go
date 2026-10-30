// Check that the fixture supports a library author's workflow: clone, commit, push, and annotated release tags.

package gitfixture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/releasetag"
)

// TestClone_PushesCommitsAndTagsToTheFixture lets library release tests publish through the fixture's transport.
func TestClone_PushesCommitsAndTagsToTheFixture(t *testing.T) {
	ctx := context.Background()
	f, err := New(ctx, Library(map[string][]byte{"config.yaml": []byte("schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: library}\n")}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	checkout, err := f.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := f.Commit(ctx, checkout, "Add a change note", Library(map[string][]byte{"changes/one.yaml": []byte("summary: One.\n"), "config.yaml": nil}))
	if err != nil {
		t.Fatal(err)
	}
	message := "Notes.\n\n---\nschemaVersion: 1\nrelease: 1\nrules: {}\n"
	if err := f.Tag(ctx, checkout, "release/1", message); err != nil {
		t.Fatal(err)
	}
	if _, err := f.CommandIn(ctx, checkout, "push", "--quiet", "origin", "main", "release/1"); err != nil {
		t.Fatal(err)
	}
	if head, err := f.Command(ctx, "rev-parse", "main"); err != nil || head != commit {
		t.Fatalf("pushed main is %q, want %q: %v", head, commit, err)
	}
	// The working tree follows the pushed branch, so later fixture commits build on it.
	if _, err := os.Stat(filepath.Join(f.Directory, "repository", ".trailguide", "changes", "one.yaml")); err != nil {
		t.Fatal(err)
	}
	if got, err := f.Command(ctx, "tag", "--list", "--format=%(contents)", "release/1"); err != nil || got+"\n" != message {
		t.Fatalf("tag message %q, want %q: %v", got, message, err)
	}
}

// TestNew_ServesPartialCloneFilters lets imports fetch release tags without file contents.
func TestNew_ServesPartialCloneFilters(t *testing.T) {
	ctx := context.Background()
	f, err := New(ctx, map[string][]byte{"README.md": []byte("fixture\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	parent := t.TempDir()
	if _, err := f.CommandIn(ctx, parent, "clone", "--quiet", "--bare", "--filter=blob:none", f.Repository, "filtered"); err != nil {
		t.Fatal(err)
	}
	// A server that ignores the filter sends every blob, so none would be missing.
	missing, err := f.CommandIn(ctx, filepath.Join(parent, "filtered"), "rev-list", "--objects", "--missing=print", "--all")
	if err != nil || !strings.Contains(missing, "\n?") {
		t.Fatalf("blobs were not filtered: %q, %v", missing, err)
	}
	// Reading an omitted blob fetches it by ID, which the served protocol must allow.
	if got, err := f.CommandIn(ctx, filepath.Join(parent, "filtered"), "cat-file", "-p", "HEAD:README.md"); err != nil || got != "fixture" {
		t.Fatalf("omitted blob was not fetched on demand: %q, %v", got, err)
	}
}

// TestRelease_TagsARecordThatParses publishes a library release whose tag message Trail Guide can read.
func TestRelease_TagsARecordThatParses(t *testing.T) {
	ctx := context.Background()
	f, err := New(ctx, Library(map[string][]byte{"config.yaml": []byte("schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: library}\n")}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules: {}\n"); err != nil {
		t.Fatal(err)
	}
	object, err := f.Command(ctx, "cat-file", "tag", "release/1")
	if err != nil {
		t.Fatal(err)
	}
	if release, err := releasetag.ParseObject("release/1", []byte(object)); err != nil || release.Record.Release != 1 {
		t.Fatalf("got %+v, %v", release, err)
	}
}

// TestLibrary_PlacesLibraryFilesInTheTrailGuideDirectory keeps deletions and files outside the library.
func TestLibrary_PlacesLibraryFilesInTheTrailGuideDirectory(t *testing.T) {
	got := Library(map[string][]byte{"config.yaml": []byte("a"), "techs/go/errors.md": []byte("b"), "assets/x.svg": nil, "LICENSE.md": []byte("c")})
	want := map[string][]byte{".trailguide/config.yaml": []byte("a"), ".trailguide/rules/techs/go/errors.md": []byte("b"), ".trailguide/assets/x.svg": nil, "LICENSE.md": []byte("c")}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for name, data := range want {
		if placed, ok := got[name]; !ok || string(placed) != string(data) || (placed == nil) != (data == nil) {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
