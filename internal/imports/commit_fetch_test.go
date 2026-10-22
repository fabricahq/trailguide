// Fetch the commits a source record names, and explain a failure from what Git reports instead of assuming the
// commit is missing.

package imports

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// failingCommitFetch returns a Git executable that runs f's Git, except that a fetch of commits by ID prints text
// to its diagnostics and fails, as a fetch interrupted by the network can while the server stays reachable.
func failingCommitFetch(t *testing.T, f *gitfixture.Fixture, text string) string {
	t.Helper()
	git, err := exec.LookPath(f.GitPath)
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" fetch \"*)\n  input=$(cat)\n  if printf '%s\\n' \"$input\" | grep -Eq '^[0-9a-f]{40}$'; then\n    printf '%s\\n' '" + text + "' >&2\n    exit 128\n  fi\n  printf '%s\\n' \"$input\" | exec '" + git + "' \"$@\" ;;\nesac\nexec '" + git + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return wrapper
}

// TestImport_ExplainsAFailedCommitFetchFromWhatGitReports: a recorded commit whose fetch fails because the
// connection dropped, or for a reason Trail Guide doesn't recognize, is never reported as missing, which would
// suggest choosing versions again; only a server that says it doesn't have the commit makes it missing, and then
// the advice keeps the project's versions before it mentions deleting vendor/.
func TestImport_ExplainsAFailedCommitFetchFromWhatGitReports(t *testing.T) {
	f := newLibraryFixture(t, libraryFiles())
	config := libraryConfig(t, f.Repository)
	options := Options{GitPath: f.GitPath, Environment: f.Environment}
	first, err := ImportLibraries(context.Background(), config, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	recorded := map[string]library.Snapshot{"team": first["team"].Snapshot}
	for _, test := range []struct{ name, text, code string }{
		{"connection reset", "error: RPC failed; curl 56 Recv failure: Connection reset by peer", "connection-failed"},
		{"unrecognized failure", "fatal: early EOF", "git-failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ImportLibraries(context.Background(), config, recorded, Options{GitPath: failingCommitFetch(t, f, test.text), Environment: f.Environment})
			requireCode(t, err, test.code)
			if strings.Contains(err.Error(), "missing") || strings.Contains(err.Error(), "Delete vendor") {
				t.Fatalf("a failed fetch was reported as a missing commit: %v", err)
			}
		})
	}
	t.Run("missing commit", func(t *testing.T) {
		snapshot := first["team"].Snapshot
		snapshot.Commit = strings.Repeat("1", 40)
		_, err := ImportLibraries(context.Background(), config, map[string]library.Snapshot{"team": snapshot}, options)
		requireCode(t, err, "version-not-found")
		message := err.Error()
		restore, deletion := strings.Index(message, "restore"), strings.Index(message, "--retake team")
		if !strings.Contains(message, "may have rewritten its history") || restore < 0 || deletion < restore || !strings.Contains(message, "which may import different content") {
			t.Fatalf("the advice doesn't keep the project's versions first: %v", err)
		}
	})
}

// TestImport_ExplainsAFailedRefFetchFromWhatGitReports: a fresh ref naming a commit the library has is never
// reported missing because its fetch failed for another reason, while a commit the server says it doesn't have is.
func TestImport_ExplainsAFailedRefFetchFromWhatGitReports(t *testing.T) {
	f := newLibraryFixture(t, libraryFiles())
	head, err := f.Command(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, ref, text, code string }{
		{"connection reset", head, "error: RPC failed; curl 56 Recv failure: Connection reset by peer", "connection-failed"},
		{"unrecognized failure", head, "fatal: early EOF", "git-failed"},
		{"missing commit", strings.Repeat("1", 40), "", "version-not-found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := libraryConfig(t, f.Repository)
			config.Sources[0].Ref = gitRef(t, test.ref)
			git := f.GitPath
			if test.text != "" {
				git = failingCommitFetch(t, f, test.text)
			}
			_, err := ImportLibraries(context.Background(), config, nil, Options{GitPath: git, Environment: f.Environment})
			requireCode(t, err, test.code)
			if test.code != "version-not-found" && strings.Contains(err.Error(), "has no tag or commit") {
				t.Fatalf("a failed fetch was reported as a missing ref: %v", err)
			}
		})
	}
}
