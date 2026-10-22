// Test moving the managed trees from where Trail Guide 0.7 kept them to _managed/ in one transaction.

package filetxn

import (
	"context"
	"errors"
	"io/fs"
	"testing"
)

// TestApply_MovesTheManagedTreesFromTheLegacyLayout writes both trees under _managed/ and removes them from their
// legacy places in one transaction, which a failed input check leaves undone.
func TestApply_MovesTheManagedTreesFromTheLegacyLayout(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		root := openProject(t)
		writeFixture(t, root, "vendor/team/rule.md", "old rule")
		writeFixture(t, root, "generated/RULES.md", "old output")
		output := map[Target]map[string][]byte{
			Vendor:          {"team/rule.md": []byte("new rule")},
			Generated:       {"RULES.md": []byte("new output")},
			LegacyVendor:    nil,
			LegacyGenerated: nil,
		}
		err := WithWriter(context.Background(), root, func(w *Writer) error {
			return w.Apply(output, func() error {
				if interrupted {
					return errors.New("inputs changed")
				}
				return nil
			})
		})
		if interrupted {
			if err == nil {
				t.Fatal("a failed input check applied the move")
			}
			for name, want := range map[string]string{"vendor/team/rule.md": "old rule", "generated/RULES.md": "old output"} {
				if data, err := root.ReadFile(name); err != nil || string(data) != want {
					t.Fatalf("%s: got %q, want %q: %v", name, data, want, err)
				}
			}
			if _, err := root.Stat("_managed"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("_managed: %v, want it absent", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{"_managed/vendor/team/rule.md": "new rule", "_managed/generated/RULES.md": "new output"} {
			if data, err := root.ReadFile(name); err != nil || string(data) != want {
				t.Fatalf("%s: got %q, want %q: %v", name, data, want, err)
			}
		}
		for _, name := range []string{"vendor", "generated"} {
			if _, err := root.Stat(name); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("%s: %v, want it removed", name, err)
			}
		}
		if err := RequireIdle(root); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRecovery_RefusesAManagedTreeUnderManagedInAnOlderJournal refuses a journal older than format 11 that names a
// tree under _managed/, which only format 11 can, so recovery never trusts a journal it can't have written.
func TestRecovery_RefusesAManagedTreeUnderManagedInAnOlderJournal(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "_managed/generated/old", "before")
	before, err := ReadTree(context.Background(), root, string(Generated))
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 10, Entries: []journalEntry{{Generated, treeDigest(before), digest([]byte("after")), true}}}); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
}
