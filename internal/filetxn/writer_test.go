// Exercise real managed-directory replacement, interruption recovery, and confined filesystem reads.

package filetxn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openProject creates a disposable project root owned by the test.
func openProject(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// writeFixture writes an authored test file and creates its parent directories.
func writeFixture(t *testing.T, root *os.Root, name, text string) {
	t.Helper()
	if err := root.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

// projectCode checks a stable caller-visible category without matching incidental prose.
func projectCode(t *testing.T, err error, want string) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != want {
		t.Fatalf("wanted %s, got %v", want, err)
	}
}

// TestWriterReplacesOnlyManagedTrees verifies initial and repeated apply, stale cleanup, and authored preservation.
func TestWriterReplacesOnlyManagedTrees(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "rules/rule.md", "authored")
	output := map[Target]map[string][]byte{Vendor: {"team/file": {0, 255, 10}}, Generated: {"RULES.md": []byte("first")}}
	for i := 0; i < 2; i++ {
		if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(output, nil) }); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, root, "_managed/generated/stale.md", "stale")
	output[Generated] = map[string][]byte{"RULES.md": []byte("second")}
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(output, nil) }); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTree(context.Background(), root, "_managed/generated")
	if err != nil || len(got.Files) != 1 || string(got.Files["RULES.md"]) != "second" {
		t.Fatalf("generated: %v %v", got, err)
	}
	if data, err := root.ReadFile("rules/rule.md"); err != nil || string(data) != "authored" {
		t.Fatal("authored file changed")
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestWriterBusyAndExpiredHandle verifies ownership and prevents reuse after release.
func TestWriterBusyAndExpiredHandle(t *testing.T) {
	root := openProject(t)
	var saved *Writer
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		saved = w
		projectCode(t, RequireIdle(root), "busy")
		projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { t.Fatal("second writer entered"); return nil }), "busy")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	projectCode(t, saved.Apply(map[Target]map[string][]byte{Generated: {}}, nil), "invalid-operation")
}

// TestWriterConcurrentEditAndRenameFailure preserves a later edit and rolls back both targets on rename failure.
func TestWriterConcurrentEditAndRenameFailure(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "_managed/vendor/old", "_managed/vendor")
	writeFixture(t, root, "_managed/generated/old", "_managed/generated")
	output := map[Target]map[string][]byte{Vendor: {"new": []byte("v")}, Generated: {"new": []byte("g")}}
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(output, func() error { writeFixture(t, root, "_managed/generated/old", "user edit"); return nil })
	})
	projectCode(t, err, "concurrent-change")
	if b, _ := root.ReadFile("_managed/generated/old"); string(b) != "user edit" {
		t.Fatal("concurrent edit lost")
	}
	err = WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(output, func() error { return root.RemoveAll(transactionName + "/new-_managed+generated") })
	})
	if err == nil {
		t.Fatal("missing staging tree did not fail")
	}
	if b, _ := root.ReadFile("_managed/vendor/old"); string(b) != "_managed/vendor" {
		t.Fatal("first target not restored")
	}
	if b, _ := root.ReadFile("_managed/generated/old"); string(b) != "user edit" {
		t.Fatal("second target not restored")
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// stageInterruption constructs the exact journal/rename boundary used when a writer is killed.
func stageInterruption(t *testing.T, root *os.Root, committed bool) {
	t.Helper()
	ctx := context.Background()
	writeFixture(t, root, "generated/old", "before")
	before, err := ReadTree(ctx, root, "generated")
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeTree(ctx, root, transactionName+"/new-generated", map[string][]byte{"new": []byte("after")}, nil); err != nil {
		t.Fatal(err)
	}
	after, err := ReadTree(ctx, root, transactionName+"/new-generated")
	if err != nil {
		t.Fatal(err)
	}
	if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 1, Entries: []journalEntry{{LegacyGenerated, treeDigest(before), treeDigest(after), true}}}); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename("generated", transactionName+"/old-generated"); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename(transactionName+"/new-generated", "generated"); err != nil {
		t.Fatal(err)
	}
	if committed {
		if err := durableJSON(root, transactionName+"/committed.json", true); err != nil {
			t.Fatal(err)
		}
	}
}

// TestWriterRecoveryPreservesLaterEdits covers rollback, manual recovery, and committed cleanup semantics.
func TestWriterRecoveryPreservesLaterEdits(t *testing.T) {
	for _, kind := range []string{"rollback", "edited", "committed"} {
		t.Run(kind, func(t *testing.T) {
			root := openProject(t)
			stageInterruption(t, root, kind == "committed")
			projectCode(t, RequireIdle(root), "busy")
			if kind != "rollback" {
				writeFixture(t, root, "generated/new", "later edit")
			}
			err := WithWriter(context.Background(), root, func(*Writer) error { return nil })
			if kind == "edited" {
				projectCode(t, err, "recovery-required")
				if _, err := root.Stat(transactionName + "/old-generated/old"); err != nil {
					t.Fatal("backup lost")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			name, want := "generated/old", "before"
			if kind != "rollback" {
				name, want = "generated/new", "later edit"
			}
			if b, _ := root.ReadFile(name); string(b) != want {
				t.Fatalf("lost %s", kind)
			}
		})
	}
}

// TestWriterCancellation preserves existing output and keeps errors.Is usable.
func TestWriterCancellation(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "_managed/generated/old", "before")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := WithWriter(ctx, root, func(w *Writer) error {
		return w.Apply(map[Target]map[string][]byte{Generated: {"new": []byte("after")}}, func() error { cancel(); return nil })
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if b, _ := root.ReadFile("_managed/generated/old"); string(b) != "before" {
		t.Fatal("changed output on cancellation")
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestReadTreeRejectsUnsafeFiles exercises actual links, binary bytes, and empty-directory identity.
func TestReadTreeRejectsUnsafeFiles(t *testing.T) {
	root := openProject(t)
	ctx := context.Background()
	absent, err := ReadTree(ctx, root, "tree")
	if err != nil || absent != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir("tree", 0700); err != nil {
		t.Fatal(err)
	}
	empty, err := ReadTree(ctx, root, "tree")
	if err != nil || empty == nil || treeDigest(empty) == treeDigest(absent) {
		t.Fatal("lost absent/empty distinction")
	}
	if err := root.Symlink("../outside", "tree/link"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTree(ctx, root, "tree"); err == nil {
		t.Fatal("accepted symlink")
	}
	if err := root.Remove("tree/link"); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "tree/a", "bytes")
	if err := os.Link(filepath.Join(root.Name(), "tree/a"), filepath.Join(root.Name(), "tree/b")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTree(ctx, root, "tree"); err == nil {
		t.Fatal("accepted hardlink")
	}
}

// TestWriterRejectsAmbiguousOwners never steals missing, remote, malformed, or already claimed ownership.
func TestWriterRejectsAmbiguousOwners(t *testing.T) {
	for _, owner := range []string{`{}`, `{"host":"elsewhere","pid":1,"token":"x"}`, `not json`} {
		t.Run(owner, func(t *testing.T) {
			root := openProject(t)
			writeFixture(t, root, lockName+"/owner.json", owner)
			projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { t.Fatal("stole lock"); return nil }), "busy")
			if b, _ := root.ReadFile(lockName + "/owner.json"); string(b) != owner {
				t.Fatal("modified ambiguous lock")
			}
		})
	}
}

// TestKilledWriterReclaimsLock exercises actual process death with a completed first replacement.
func TestKilledWriterReclaimsLock(t *testing.T) {
	if dir := os.Getenv("TRAILGUIDE_WRITER_CHILD"); dir != "" {
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		writeFixture(t, root, "_managed/generated/old", "before")
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			rename := w.rename
			w.rename = func(old, new string) error {
				if err := rename(old, new); err != nil {
					return err
				}
				if old == "_managed/generated" {
					if err := root.WriteFile("ready", []byte("ready"), 0600); err != nil {
						return err
					}
					for {
						time.Sleep(time.Second)
					}
				}
				return nil
			}
			return w.Apply(map[Target]map[string][]byte{Generated: {"new": []byte("after")}}, nil)
		})
		t.Fatal(err)
		return
	}
	root := openProject(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestKilledWriterReclaimsLock$")
	cmd.Env = append(os.Environ(), "TRAILGUIDE_WRITER_CHILD="+root.Name())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := root.Stat("ready"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not reach replacement")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if b, _ := root.ReadFile("_managed/generated/old"); string(b) != "before" {
		t.Fatal("killed writer not rolled back")
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestJournalDigestMatchesReference freezes a meaningful cross-runtime recovery representation.
func TestJournalDigestMatchesReference(t *testing.T) {
	tree := &Tree{Files: map[string][]byte{"a": []byte("content")}, Directories: []string{"empty"}}
	encoded, _ := json.Marshal(struct {
		Directories []string    `json:"directories"`
		Files       [][2]string `json:"files"`
	}{[]string{"empty"}, [][2]string{{"a", digest([]byte("content"))}}})
	if treeDigest(tree) != digest(encoded) {
		t.Fatal("journal digest differs")
	}
	if validDigest(strings.Repeat("A", 64)) {
		t.Fatal("accepted noncanonical digest")
	}
}

// TestJournalUnicodeOrdering retains version-1 recovery identity for non-BMP filenames.
func TestJournalUnicodeOrdering(t *testing.T) {
	tree := &Tree{Files: map[string][]byte{"\ue000": []byte("a"), "\U00010000": []byte("b")}, Directories: []string{}}
	// JavaScript compares the high surrogate for U+10000 before the BMP U+E000.
	encoded, _ := json.Marshal(struct {
		Directories []string    `json:"directories"`
		Files       [][2]string `json:"files"`
	}{[]string{}, [][2]string{{"\U00010000", digest([]byte("b"))}, {"\ue000", digest([]byte("a"))}}})
	if treeDigest(tree) != digest(encoded) {
		t.Fatal("UTF-16 order changed")
	}
}

// TestRecoveryRejectsContradictoryExistence preserves intact output when a malformed journal says it was absent.
func TestRecoveryRejectsContradictoryExistence(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "generated/keep", "user data")
	before, err := ReadTree(context.Background(), root, "generated")
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 1, Entries: []journalEntry{{LegacyGenerated, treeDigest(before), digest([]byte("after")), false}}}); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if b, _ := root.ReadFile("generated/keep"); string(b) != "user data" {
		t.Fatal("malformed journal deleted output")
	}
	if _, err := root.Stat(transactionName + "/journal.json"); err != nil {
		t.Fatal("lost journal")
	}
}

// TestWriterPreservesEditAtBackupRename reproduces an edit after staging validation through the real writer.
func TestWriterPreservesEditAtBackupRename(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "_managed/generated/old", "before")
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		w.rename = func(from, to string) error {
			if from == "_managed/generated" {
				writeFixture(t, root, "_managed/generated/old", "late edit")
			}
			return root.Rename(from, to)
		}
		return w.Apply(map[Target]map[string][]byte{Generated: {"new": []byte("after")}}, nil)
	})
	if err == nil {
		t.Fatal("late edit was silently overwritten")
	}
	found := false
	for _, name := range []string{"_managed/generated/old", transactionName + "/old-_managed+generated/old"} {
		if data, _ := root.ReadFile(name); string(data) == "late edit" {
			found = true
		}
	}
	if !found {
		t.Fatal("late edit lost")
	}
}

// TestRecoveryPreservesEditAtRename verifies a late edit is quarantined instead of deleted by rollback.
func TestRecoveryPreservesEditAtRename(t *testing.T) {
	root := openProject(t)
	stageInterruption(t, root, false)
	err := recoverWithRename(root, func(from, to string) error {
		if from == "generated" {
			writeFixture(t, root, "generated/new", "late edit")
		}
		return root.Rename(from, to)
	})
	projectCode(t, err, "recovery-required")
	for name, want := range map[string]string{transactionName + "/discarded-generated/new": "late edit", transactionName + "/old-generated/old": "before"} {
		if data, _ := root.ReadFile(name); string(data) != want {
			t.Fatalf("lost %s", name)
		}
	}
	projectCode(t, recoverChanges(root), "recovery-required")
	data, err := root.ReadFile(transactionName + "/journal.json")
	var journal journalRecord
	if err != nil || json.Unmarshal(data, &journal) != nil || journal.FormatVersion != 2 {
		t.Fatal("quarantine must be protected from older recovery implementations")
	}
}

// TestRecoveryResumesAfterQuarantine verifies restart after displacing output or restoring its backup.
func TestRecoveryResumesAfterQuarantine(t *testing.T) {
	for _, restored := range []bool{false, true} {
		root := openProject(t)
		stageInterruption(t, root, false)
		if err := root.Rename("generated", transactionName+"/discarded-generated"); err != nil {
			t.Fatal(err)
		}
		if restored {
			if err := root.Rename(transactionName+"/old-generated", "generated"); err != nil {
				t.Fatal(err)
			}
		}
		if err := recoverChanges(root); err != nil {
			t.Fatal(err)
		}
		if data, _ := root.ReadFile("generated/old"); string(data) != "before" {
			t.Fatal("original not restored")
		}
		if err := RequireIdle(root); err != nil {
			t.Fatal(err)
		}
	}
}

// TestWriterPreservesNewEmptyTarget exercises creation after validation and after a backup move.
func TestWriterPreservesNewEmptyTarget(t *testing.T) {
	for _, existed := range []bool{false, true} {
		t.Run(fmt.Sprint(existed), func(t *testing.T) {
			root := openProject(t)
			if existed {
				writeFixture(t, root, "_managed/generated/old", "before")
			}
			var created os.FileInfo
			err := WithWriter(context.Background(), root, func(w *Writer) error {
				rename := w.rename
				w.rename = func(from, to string) error {
					if from == transactionName+"/new-_managed+generated" {
						if err := root.Mkdir("_managed/generated", 0700); err != nil {
							t.Fatal(err)
						}
						var err error
						created, err = root.Stat("_managed/generated")
						if err != nil {
							t.Fatal(err)
						}
					}
					return rename(from, to)
				}
				return w.Apply(map[Target]map[string][]byte{Generated: {"new": []byte("after")}}, nil)
			})
			if err == nil {
				t.Fatal("concurrent empty directory was overwritten")
			}
			current, statErr := root.Stat("_managed/generated")
			if statErr != nil || !os.SameFile(created, current) {
				t.Fatalf("lost concurrent directory: %v", statErr)
			}
			if existed {
				data, err := root.ReadFile(transactionName + "/old-_managed+generated/old")
				if err != nil || string(data) != "before" {
					t.Fatalf("lost backup: %v", err)
				}
			}
		})
	}
}

// TestWriterCancellationDuringReplacement completes rollback before returning cancellation.
func TestWriterCancellationDuringReplacement(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "_managed/vendor/old", "vendor before")
	writeFixture(t, root, "_managed/generated/old", "generated before")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := WithWriter(ctx, root, func(w *Writer) error {
		rename := w.rename
		w.rename = func(from, to string) error {
			err := rename(from, to)
			if err == nil && from == transactionName+"/new-_managed+vendor" {
				cancel()
			}
			return err
		}
		return w.Apply(map[Target]map[string][]byte{Vendor: {"new": []byte("after")}, Generated: {"new": []byte("after")}}, nil)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	for _, target := range []string{"_managed/vendor", "_managed/generated"} {
		data, err := root.ReadFile(target + "/old")
		if err != nil || string(data) != path.Base(target)+" before" {
			t.Fatalf("did not restore %s: %v", target, err)
		}
		if _, err := root.Stat(target + "/new"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retained new output in %s: %v", target, err)
		}
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestWriterWaitsForCodeRules refuses to read or write a project while a Code Rules process from before the rename
// holds its lock, or left a transaction that only it recovers.
func TestWriterWaitsForCodeRules(t *testing.T) {
	for _, name := range []string{".code-rules-lock", ".code-rules-transaction", ".code-rules-cleanup"} {
		t.Run(name, func(t *testing.T) {
			root := openProject(t)
			if err := root.Mkdir(name, 0700); err != nil {
				t.Fatal(err)
			}
			projectCode(t, RequireIdle(root), "busy")
			projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { t.Fatal("writer entered"); return nil }), "busy")
		})
	}
}

// openLegacyProject opens a project directory still named .code-rules, as Code Rules made it before the rename.
func openLegacyProject(t *testing.T) *os.Root {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".code-rules")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// TestWriterOfALegacyProjectHoldsTheCodeRulesLock takes the lock Code Rules honors while it writes a project still in
// .code-rules/, so a Code Rules process that starts meanwhile waits, and releases it afterward.
func TestWriterOfALegacyProjectHoldsTheCodeRulesLock(t *testing.T) {
	root := openLegacyProject(t)
	err := WithWriter(context.Background(), root, func(*Writer) error {
		if _, err := root.Stat(".code-rules-lock/owner.json"); err != nil {
			t.Errorf("a Code Rules process could write the project meanwhile: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat(".code-rules-lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("kept the Code Rules lock: %v", err)
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestWriterOfALegacyProjectWaitsForALiveCodeRulesWriter refuses to write a project in .code-rules/ while a live
// Code Rules process holds its lock, and leaves that lock alone.
func TestWriterOfALegacyProjectWaitsForALiveCodeRulesWriter(t *testing.T) {
	root := openLegacyProject(t)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir(".code-rules-lock", 0700); err != nil {
		t.Fatal(err)
	}
	owner, _ := json.Marshal(lockOwner{host, os.Getpid(), "token"})
	if err := root.WriteFile(".code-rules-lock/owner.json", owner, 0600); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { t.Fatal("writer entered"); return nil }), "busy")
	if data, err := root.ReadFile(".code-rules-lock/owner.json"); err != nil || string(data) != string(owner) {
		t.Fatalf("changed the Code Rules lock: %q, %v", data, err)
	}
}
