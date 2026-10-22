// Leave a real trailguide project update interrupted partway through installing, at a chosen step, then check
// that the next update or sync, run through the real CLI, recovers the project and completes.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/trailguide/internal/filetxn"
)

// updateArguments are the arguments of the update the tests interrupt and then run again.
var updateArguments = []string{"project", "update", "--yes", "--keep", "team:techs/go/errors", "--reason", "Not yet."}

// runBounded runs the CLI in the fixture's project like run, but stops it, and every process it started, after
// two minutes or when the test ends, so a hung child can't outlive or stall the test.
func (u updateFixture) runBounded(t *testing.T, directory string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, u.binary, args...)
	command.Dir = directory
	command.Env = u.fixture.Environment
	command.WaitDelay = 5 * time.Second
	var out, diagnostic bytes.Buffer
	command.Stdout, command.Stderr = &out, &diagnostic
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("%v didn't finish within its deadline: %v\n%s%s", args, err, out.String(), diagnostic.String())
	}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return out.String(), diagnostic.String(), command.ProcessState.ExitCode()
}

// interruptedUpdate returns an update fixture whose update stopped after replacing the managed targets before
// blocked, one of generated and config.yaml, as a process killed there would leave it: a complete recovery journal,
// the new vendor/, and blocked's new copy staged but not installed. The real update runs to completion on a copy
// of the project, and the project's own writer then installs exactly that output until a directory at blocked's
// backup path stops it. Recovery can't roll back past that directory, so the transaction stays, and removing the
// directory leaves the interrupted state.
func interruptedUpdate(t *testing.T, blocked filetxn.Target) updateFixture {
	t.Helper()
	u := newUpdateFixture(t)
	project := filepath.Join(u.directory, ".trailguide")
	completed := filepath.Join(t.TempDir(), "project")
	if err := os.CopyFS(completed, os.DirFS(u.directory)); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code := u.runBounded(t, completed, updateArguments...); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	output := updateOutput(t, project, filepath.Join(completed, ".trailguide"))
	root, err := os.OpenRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	blocker := filepath.Join(".trailguide-transaction", "old-"+strings.ReplaceAll(string(blocked), "/", "+"))
	err = filetxn.WithWriter(context.Background(), root, func(w *filetxn.Writer) error {
		// Apply calls this after staging every target and before writing its journal.
		return w.Apply(output, func() error { return root.MkdirAll(filepath.Join(blocker, "blocker"), 0700) })
	})
	var failure *filetxn.Error
	if !errors.As(err, &failure) || failure.Code != "recovery-required" {
		t.Fatalf("the blocked update didn't stop with its transaction kept: %v", err)
	}
	if err := root.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"journal.json", "old-_managed+vendor", "new-" + strings.ReplaceAll(string(blocked), "/", "+")} {
		if _, err := root.Stat(filepath.Join(".trailguide-transaction", name)); err != nil {
			t.Fatalf("the interrupted transaction lacks %s: %v", name, err)
		}
	}
	// Replacement began: vendor/ already holds the update's output.
	if vendor, err := filetxn.ReadTree(context.Background(), root, "_managed/vendor"); err != nil || !maps.EqualFunc(vendor.Files, output[filetxn.Vendor], bytes.Equal) {
		t.Fatalf("_managed/vendor/ isn't the update's output: %v", err)
	}
	return u
}

// updateOutput returns what the update installed in completed, the Trail Guide directory of a copy of project
// that ran it: the vendor and generated trees, config.yaml, and the managed guide when it changed.
func updateOutput(t *testing.T, project, completed string) map[filetxn.Target]map[string][]byte {
	t.Helper()
	root, err := os.OpenRoot(completed)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	output := map[filetxn.Target]map[string][]byte{}
	for _, target := range []filetxn.Target{filetxn.Vendor, filetxn.Generated} {
		tree, err := filetxn.ReadTree(context.Background(), root, string(target))
		if err != nil {
			t.Fatal(err)
		}
		output[target] = tree.Files
	}
	for _, target := range []filetxn.Target{filetxn.Config, filetxn.GuideReadme} {
		after, err := os.ReadFile(filepath.Join(completed, string(target)))
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(filepath.Join(project, string(target)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			output[target] = map[string][]byte{string(target): after}
		}
	}
	if _, ok := output[filetxn.Config]; !ok {
		t.Fatal("the update didn't write its pin to config.yaml")
	}
	return output
}

// interruptions are the steps the tests stop an update after: replacing vendor/, and replacing vendor/ and
// generated/ but not yet config.yaml.
var interruptions = []filetxn.Target{filetxn.Generated, filetxn.Config}

// TestUpdate_RecoversAnInterruptedUpdateAndCompletes reruns the update after one that stopped partway.
func TestUpdate_RecoversAnInterruptedUpdateAndCompletes(t *testing.T) {
	for _, blocked := range interruptions {
		t.Run("before "+string(blocked), func(t *testing.T) {
			u := interruptedUpdate(t, blocked)
			if out, diagnostic, code := u.runBounded(t, u.directory, updateArguments...); code != 0 {
				t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
			}
			requireRecoveredUpdate(t, u)
			if want := map[string]string{"backoff": "1.0.0", "errors": "1.0.0", "format": "1.0.1", "loaders": "1.1.0", "naming": "1.1.0", "verify": "1.0.0"}; !maps.Equal(u.versions(t), want) {
				t.Fatalf("versions %v", u.versions(t))
			}
		})
	}
}

// TestSync_RecoversAnInterruptedUpdate rolls the update back and leaves a consistent project that checks offline.
func TestSync_RecoversAnInterruptedUpdate(t *testing.T) {
	for _, blocked := range interruptions {
		t.Run("before "+string(blocked), func(t *testing.T) {
			u := interruptedUpdate(t, blocked)
			if out, diagnostic, code := u.runBounded(t, u.directory, "project", "sync"); code != 0 {
				t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
			}
			requireRecoveredUpdate(t, u)
			// Rolling back restored the configuration with the output, so sync restores the versions from before the update.
			if want := map[string]string{"backoff": "1.0.0", "errors": "1.0.0", "format": "1.0.0", "loaders": "1.0.0", "naming": "1.0.0", "retry": "1.0.0"}; !maps.Equal(u.versions(t), want) {
				t.Fatalf("versions %v", u.versions(t))
			}
			if config, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "config.yaml")); err != nil || strings.Contains(string(config), "techs/go/errors:") {
				t.Fatalf("the rolled-back configuration kept the update's pin: %v\n%s", err, config)
			}
		})
	}
}

// requireRecoveredUpdate checks that no transaction remains and the project checks offline.
func requireRecoveredUpdate(t *testing.T, u updateFixture) {
	t.Helper()
	for _, name := range []string{".trailguide-transaction", ".trailguide-cleanup", ".trailguide-lock"} {
		if _, err := os.Stat(filepath.Join(u.directory, ".trailguide", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s remains: %v", name, err)
		}
	}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "check"); code != 0 {
		t.Fatalf("offline check: exit %d\n%s%s", code, out, diagnostic)
	}
}

// TestSyncAndBuild_SayWhenTheyFirstRecoveredAnInterruptedCommand, in human and JSON output, even when they then
// change no files of their own, and say nothing of it otherwise.
func TestSyncAndBuild_SayWhenTheyFirstRecoveredAnInterruptedCommand(t *testing.T) {
	u := newUpdateFixture(t)
	note := "First recovered an interrupted earlier command, which restored or finished that command's files.\n"
	for _, command := range []string{"sync", "build"} {
		t.Run(command, func(t *testing.T) {
			u.write(t, ".trailguide-transaction/staged", "left by an interrupted command")
			out, diagnostic, code := u.run(t, "project", command)
			if code != 0 || !strings.HasPrefix(out, note) {
				t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
			}
			u.write(t, ".trailguide-transaction/staged", "left by an interrupted command")
			out, diagnostic, code = u.run(t, "project", command, "--json")
			var result struct {
				OK    bool
				Value struct{ Recovered bool }
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || !result.OK || !result.Value.Recovered {
				t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
			}
			out, _, code = u.run(t, "project", command)
			if code != 0 || strings.Contains(out, "recovered") {
				t.Fatalf("a command that recovered nothing says it did: exit %d:\n%s", code, out)
			}
		})
	}
}
