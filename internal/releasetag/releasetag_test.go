// Check listing and reading release tags against a real repository.

package releasetag_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/releasetag"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// TestParseObject_SkipsHeadersAndSignature reads the record from a signed tag object's message only, whether the
// tag is signed with GPG or SSH.
func TestParseObject_SkipsHeadersAndSignature(t *testing.T) {
	for _, signature := range []string{"-----BEGIN PGP SIGNATURE-----\nU0lH\n-----END PGP SIGNATURE-----\n", "-----BEGIN SSH SIGNATURE-----\nU1NIU0lH\n-----END SSH SIGNATURE-----\n"} {
		object := "object 0123456789012345678901234567890123456789\ntype commit\ntag release/2\ntagger Fixture <fixture@example.invalid> 0 +0000\n\nNotes.\n\n---\nschemaVersion: 1\nrelease: 2\nrules:\n  techs/go/a: 1.0.0\n" + signature
		release, err := releasetag.ParseObject("release/2", []byte(object))
		if err != nil || release.Notes != "Notes." || release.Record.Release != 2 || len(release.Record.Rules) != 1 {
			t.Fatalf("got %+v, %v", release, err)
		}
		if _, err := releasetag.ParseObject("release/3", []byte(object)); err == nil {
			t.Fatal("accepted a record for another library release")
		}
	}
}

// numbers returns the tags' numbers in order.
func numbers(tags []releasetag.Tag) []int {
	result := []int{}
	for _, tag := range tags {
		result = append(result, tag.Number)
	}
	return result
}

// TestListAndRead_ReleaseTagsInNumberOrder lists release/<number> tags numerically, leaves out other names and,
// when asked, tags off HEAD's history, and reads each annotated tag's notes and record.
func TestListAndRead_ReleaseTagsInNumberOrder(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{"README.md": []byte("Library.\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	dir := f.Worktree()
	for _, number := range []string{"2", "10"} {
		if err := f.Tag(ctx, dir, "release/"+number, "Library release "+number+".\n\n---\nschemaVersion: 1\nrelease: "+number+"\nrules: {}\n"); err != nil {
			t.Fatal(err)
		}
	}
	unreachable, err := f.Command(ctx, "commit-tree", "HEAD^{tree}", "-m", "Off the branch.")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"tag", "release/3", f.FirstCommit}, {"tag", "release/01"}, {"tag", "release/next"}, {"tag", "--annotate", "--message", "Notes only.", "release/4", unreachable}} {
		if _, err := f.Command(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	runner, err := gitexec.Isolated(gitexec.Options{GitPath: f.GitPath, Environment: f.Environment})
	if err != nil {
		t.Fatal(err)
	}

	all, err := releasetag.List(ctx, runner, dir, "")
	if err != nil || len(all) != 4 || all[0].Number != 2 || all[1].Number != 3 || all[2].Number != 4 || all[3].Number != 10 {
		t.Fatalf("listed %v, %v; want 2, 3, 4, 10", numbers(all), err)
	}
	if tag := all[0]; tag.Type != "tag" || tag.TargetType != "commit" || tag.Target != f.LatestCommit || tag.Size == 0 {
		t.Fatalf("annotated release/2: %+v", tag)
	}
	if tag := all[1]; tag.Type != "commit" || tag.Object != f.FirstCommit || tag.Target != "" || tag.TargetType != "" {
		t.Fatalf("lightweight release/3: %+v", tag)
	}
	reachable, err := releasetag.List(ctx, runner, dir, "HEAD")
	if err != nil || len(reachable) != 3 || reachable[2].Number != 10 {
		t.Fatalf("listed %v from HEAD, %v; want 2, 3, 10", numbers(reachable), err)
	}

	read := []releasetag.Release{}
	err = releasetag.Read(ctx, runner, dir, []releasetag.Tag{all[0], all[3]}, func(i int, release releasetag.Release) error {
		if i != len(read) {
			t.Errorf("release %d passed as index %d", len(read), i)
		}
		read = append(read, release)
		return nil
	})
	if err != nil || len(read) != 2 || read[0].Notes != "Library release 2." || read[0].Record.Release != 2 || read[1].Record.Release != 10 {
		t.Fatalf("read %+v, %v", read, err)
	}
	stop := errors.New("stop")
	calls := 0
	if err := releasetag.Read(ctx, runner, dir, []releasetag.Tag{all[0], all[3]}, func(int, releasetag.Release) error { calls++; return stop }); err != stop || calls != 1 {
		t.Fatalf("a caller's error: got %v after %d calls", err, calls)
	}
	err = releasetag.Read(ctx, runner, dir, []releasetag.Tag{all[0], all[2]}, func(int, releasetag.Release) error { return nil })
	var invalid *releasetag.RecordError
	var validation errs.ValidationError
	if !errors.As(err, &invalid) || invalid.Tag != "release/4" || !errors.As(err, &validation) || validation.ValidationLocation() != "release/4" {
		t.Fatalf("read a tag without a record: %v", err)
	}
}

// TestRead_HandsOverEachBatchBeforeReadingTheNext reads three release tags with 6 MiB of notes and a small record,
// more than one 16 MiB batch holds, and receives each release before Git reads the batch after it, so the reader
// never holds more than one batch of notes.
func TestRead_HandsOverEachBatchBeforeReadingTheNext(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{"README.md": []byte("Library.\n")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	notes := strings.Repeat("A long line of release notes.\n", 6<<20/30)
	for number := 1; number <= 3; number++ {
		message := filepath.Join(t.TempDir(), "message")
		record := fmt.Sprintf("schemaVersion: 1\nrelease: %d\nrules: {}\n", number)
		if err := os.WriteFile(message, []byte(notes+"\n---\n"+record), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Command(ctx, "tag", "--annotate", "--cleanup=verbatim", "--file="+message, fmt.Sprintf("release/%d", number)); err != nil {
			t.Fatal(err)
		}
	}
	// The wrapper counts the Git processes that read a batch of tag objects.
	batches := filepath.Join(t.TempDir(), "batches")
	git := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\ncase \"$*\" in *\"cat-file --batch\"*) echo batch >> " + gitfixture.Quote(batches) + " ;; esac\nexec " + gitfixture.Quote(f.GitPath) + " \"$@\"\n"
	if err := os.WriteFile(git, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runner, err := gitexec.Isolated(gitexec.Options{GitPath: git, Environment: f.Environment})
	if err != nil {
		t.Fatal(err)
	}
	tags, err := releasetag.List(ctx, runner, f.Worktree(), "")
	if err != nil || len(tags) != 3 {
		t.Fatalf("listed %v, %v", numbers(tags), err)
	}
	seen := []int{}
	err = releasetag.Read(ctx, runner, f.Worktree(), tags, func(i int, release releasetag.Release) error {
		if len(release.Notes) < 6<<20-64 || release.Record.Release != i+1 {
			t.Errorf("release %d: %d bytes of notes, record %+v", i+1, len(release.Notes), release.Record)
		}
		data, err := os.ReadFile(batches)
		if err != nil {
			return err
		}
		seen = append(seen, strings.Count(string(data), "batch"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Two 6 MiB tags fit in the first 16 MiB batch; the third needs a second one.
	if !slices.Equal(seen, []int{1, 1, 2}) {
		t.Fatalf("batches read before each release was handed over: %v, want [1 1 2]", seen)
	}
}

// TestRecordError_NamesTheTagOnce whether or not the parser's location already starts with it.
func TestRecordError_NamesTheTagOnce(t *testing.T) {
	for _, test := range []struct {
		location, want string
	}{
		{"release/3", "invalid release record: release/3: expected a mapping"},
		{"release/3.release", "invalid release record: release/3.release: expected a mapping"},
		{"record", "invalid release record: release/3: record: expected a mapping"},
	} {
		err := &releasetag.RecordError{Tag: "release/3", Err: &trailguide.ValidationError{Location: test.location, Problem: "expected a mapping"}}
		if got := err.Error(); got != test.want {
			t.Errorf("location %s: got %q, want %q", test.location, got, test.want)
		}
	}
}
