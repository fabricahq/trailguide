// Exercise replacing the lockfile together with the vendor tree: a migration that creates the lockfile and removes a
// per-source record, and a sync that replaces it, through rollback, process death, and old journals.

package filetxn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// lockSync is what sync installs: the new lockfile and the vendor tree without the per-source record of Trail
// Guide 0.5.
var lockSync = map[Target]map[string][]byte{
	Lock:   {"lock.json": []byte("new lock")},
	Vendor: {"team/rule.md": []byte("new rule")},
}

// writeLockProject writes the previous vendor tree, with a per-source record, and, unless migrating, the previous
// lockfile.
func writeLockProject(t *testing.T, root *os.Root, migrating bool) {
	t.Helper()
	writeFixture(t, root, "vendor/team/rule.md", "old rule")
	if migrating {
		writeFixture(t, root, "vendor/team/_source.json", "old record")
	} else {
		writeFixture(t, root, "lock.json", "old lock")
	}
}

// requireLockProject checks that the lockfile and vendor tree are both the previous ones or both the new ones.
func requireLockProject(t *testing.T, root *os.Root, migrating bool, state string) {
	t.Helper()
	if data, err := root.ReadFile("vendor/team/rule.md"); err != nil || string(data) != state+" rule" {
		t.Fatalf("vendor/team/rule.md: got %q, want %q: %v", data, state+" rule", err)
	}
	lock, lockErr := root.ReadFile("lock.json")
	_, recordErr := root.Stat("vendor/team/_source.json")
	switch {
	case state == "new" && (lockErr != nil || string(lock) != "new lock" || !errors.Is(recordErr, os.ErrNotExist)):
		t.Fatalf("new state: lock %q, %v; record %v", lock, lockErr, recordErr)
	case state == "old" && migrating && (!errors.Is(lockErr, os.ErrNotExist) || recordErr != nil):
		t.Fatalf("old migrating state: lock %q, %v; record %v", lock, lockErr, recordErr)
	case state == "old" && !migrating && (lockErr != nil || string(lock) != "old lock"):
		t.Fatalf("old state: lock %q, %v", lock, lockErr)
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestApply_RollsBackTheLockfileWhenCommitFails restores the previous lockfile, or its absence, with the vendor
// tree.
func TestApply_RollsBackTheLockfileWhenCommitFails(t *testing.T) {
	for _, migrating := range []bool{true, false} {
		t.Run("migrating="+strconv.FormatBool(migrating), func(t *testing.T) {
			root := openProject(t)
			writeLockProject(t, root, migrating)
			err := WithWriter(context.Background(), root, func(w *Writer) error {
				rename := w.rename
				w.rename = func(from, to string) error {
					if strings.HasSuffix(to, "committed.json") {
						return errors.New("injected commit failure")
					}
					return rename(from, to)
				}
				return w.Apply(lockSync, nil)
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			requireLockProject(t, root, migrating, "old")
		})
	}
}

// TestKilledLockWriterRecovers stops the writer after each rename; the next writer restores the previous lockfile, or
// its absence, and vendor tree, or, once the commit marker exists, keeps both new ones.
func TestKilledLockWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_LOCK_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			rename := w.rename
			w.rename = func(from, to string) error {
				err := rename(from, to)
				if err == nil && to == os.Getenv("TRAILGUIDE_LOCK_STOP") {
					os.Exit(42)
				}
				return err
			}
			return w.Apply(lockSync, nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	for _, migrating := range []bool{true, false} {
		stops := map[string]string{
			transactionName + "/rename-check/lock.json": "old",
			transactionName + "/old-vendor":             "old",
			"vendor":                                    "old",
			"lock.json":                                 "old",
			transactionName + "/committed.json":         "new",
		}
		if !migrating {
			stops[transactionName+"/old-lock.json"] = "old"
		}
		for stop, state := range stops {
			t.Run("migrating="+strconv.FormatBool(migrating)+"/"+stop, func(t *testing.T) {
				root := openProject(t)
				writeLockProject(t, root, migrating)
				child := exec.Command(os.Args[0], "-test.run=^TestKilledLockWriterRecovers$")
				child.Env = append(os.Environ(), "TRAILGUIDE_LOCK_CHILD="+root.Name(), "TRAILGUIDE_LOCK_STOP="+stop)
				output, err := child.CombinedOutput()
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 42 {
					t.Fatal(err, string(output))
				}
				if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
					t.Fatal(err)
				}
				requireLockProject(t, root, migrating, state)
			})
		}
	}
}

// TestRecovery_RefusesALockfileEntryInAnOlderJournal preserves a journal whose format can't hold the lockfile.
func TestRecovery_RefusesALockfileEntryInAnOlderJournal(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "lock.json", "new lock")
	writeFixture(t, root, transactionName+"/old-lock.json", "old lock")
	before := &Tree{Files: map[string][]byte{"lock.json": []byte("old lock")}}
	after := &Tree{Files: map[string][]byte{"lock.json": []byte("new lock")}}
	if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 9, Entries: []journalEntry{{Lock, treeDigest(before), treeDigest(after), true}}}); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if data, err := root.ReadFile(transactionName + "/old-lock.json"); err != nil || string(data) != "old lock" {
		t.Fatal("lost the lockfile backup", err)
	}
}
