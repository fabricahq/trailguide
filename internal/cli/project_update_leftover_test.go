// Leave a real fork update interrupted after it created its rule's first asset directory, then check that the empty
// directories it leaves under rules/ trouble no command.

package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// forkUpdateArguments replace the fixture's hand-written loaders rule, which has no assets, with a fork of loaders
// 1.2.0, which has one.
var forkUpdateArguments = []string{"project", "update", "--yes", "--update-fork", "team:techs/go/loaders"}

// interruptedForkUpdate returns an update fixture whose library publishes loaders 1.2.0 with an asset, and whose fork
// update stopped after creating rules/techs/go/assets, the missing parent of the new fork's asset directory, and
// replacing vendor/, as a process killed there would leave it. As interruptedUpdate does, it installs the real
// update's output with the project's own writer until a directory at generated/'s backup path stops it.
func interruptedForkUpdate(t *testing.T) updateFixture {
	t.Helper()
	u := newUpdateFixture(t)
	ctx := context.Background()
	if _, err := u.fixture.Commit(ctx, u.fixture.Worktree(), "Third release", gitfixture.Library(map[string][]byte{"techs/go/loaders.md": updateRule("loaders 1.2.0, with [an example](assets/loaders/example.md)."), "techs/go/assets/loaders/example.md": []byte("An example.\n")})); err != nil {
		t.Fatal(err)
	}
	if err := u.fixture.Release(ctx, 3, "schemaVersion: 1\nrelease: 3\nrules:\n  techs/go/backoff: 2.0.0\n  techs/go/errors: 2.0.0\n  techs/go/format: 1.0.1\n  techs/go/loaders: 1.2.0\n  techs/go/naming: 1.1.0\n  techs/go/verify: 1.0.0\nchanges:\n  techs/go/loaders: {change: minor, from: 1.1.0, summaries: [Add an example.]}\n"); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(u.directory, ".trailguide")
	completed := filepath.Join(t.TempDir(), "project")
	if err := os.CopyFS(completed, os.DirFS(u.directory)); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code := u.runBounded(t, completed, forkUpdateArguments...); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	output := updateOutput(t, project, filepath.Join(completed, ".trailguide"))
	completedRoot, err := os.OpenRoot(filepath.Join(completed, ".trailguide"))
	if err != nil {
		t.Fatal(err)
	}
	defer completedRoot.Close()
	rule, err := completedRoot.ReadFile("rules/techs/go/use-data-loaders.md")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := filetxn.ReadTree(ctx, completedRoot, "rules/techs/go/assets/use-data-loaders")
	if err != nil || assets == nil {
		t.Fatalf("the fork update wrote no assets: %v", err)
	}
	output[filetxn.LocalRule("techs/go/use-data-loaders.md")] = map[string][]byte{"rules/techs/go/use-data-loaders.md": rule}
	output[filetxn.LocalRuleAssets("techs/go/use-data-loaders.md")] = assets.Files
	root, err := os.OpenRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	blocker := filepath.Join(".trailguide-transaction", "old-_managed+generated")
	err = filetxn.WithWriter(ctx, root, func(w *filetxn.Writer) error {
		// Apply calls this after staging every target and before writing its journal and creating the parent.
		return w.Apply(output, func() error { return root.MkdirAll(filepath.Join(blocker, "blocker"), 0700) })
	})
	var failure *filetxn.Error
	if !errors.As(err, &failure) || failure.Code != "recovery-required" {
		t.Fatalf("the blocked update didn't stop with its transaction kept: %v", err)
	}
	if err := root.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Join(project, "rules", "techs", "go", "assets")); err != nil || len(entries) != 0 {
		t.Fatalf("the interrupted update didn't leave its empty parent: %v, %v", entries, err)
	}
	return u
}

// TestCommands_IgnoreTheEmptyDirectoriesAnInterruptedForkUpdateLeaves: whichever command recovers an interrupted fork
// update, the empty rules/techs/go/assets it leaves, and an empty deeper one, trouble no command afterward: build,
// check, sync, update, a fork into the group, and the fork update itself all succeed with no warning and no stale
// output.
func TestCommands_IgnoreTheEmptyDirectoriesAnInterruptedForkUpdateLeaves(t *testing.T) {
	for _, first := range [][]string{{"project", "build"}, {"project", "sync"}, forkUpdateArguments} {
		t.Run(strings.Join(first[1:], " "), func(t *testing.T) {
			u := interruptedForkUpdate(t)
			if err := os.MkdirAll(filepath.Join(u.directory, ".trailguide", "rules", "techs", "go", "nested", "assets"), 0700); err != nil {
				t.Fatal(err)
			}
			commands := [][]string{
				first,
				{"project", "build"},
				{"project", "check"},
				{"project", "sync"},
				{"project", "update"},
				{"project", "add", "rule", "techs/go/naming", "--from", "team@1.1.0", "--reason", "Ours."},
				{"project", "build"},
			}
			// A fork update that recovered has already replaced the fork, so only the others still have one to do.
			if first[1] != "update" {
				commands = append(commands, forkUpdateArguments)
			}
			for _, args := range append(commands, []string{"project", "check"}) {
				out, diagnostic, code := u.runBounded(t, u.directory, args...)
				if code != 0 || strings.Contains(out+diagnostic, "Warning") || strings.Contains(out+diagnostic, "out of date") {
					t.Fatalf("%v: exit %d:\n%s%s", args, code, out, diagnostic)
				}
			}
			fork, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "rules", "techs", "go", "assets", "use-data-loaders", "example.md"))
			if err != nil || string(fork) != "An example.\n" {
				t.Fatalf("the fork update didn't install the asset: %q, %v", fork, err)
			}
		})
	}
}
