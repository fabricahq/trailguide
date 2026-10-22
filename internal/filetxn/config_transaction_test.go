// Exercise replacing the configuration together with managed trees: rollback, process death, and old journals.

package filetxn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// configUpdate is what project update installs: new configuration, vendor, and generated output together.
var configUpdate = map[Target]map[string][]byte{
	Config:    {"config.yaml": []byte("new config")},
	Vendor:    {"team/rule.md": []byte("new rule")},
	Generated: {"RULES.md": []byte("new output")},
}

// writeConfigProject writes the previous configuration, vendor, and generated output.
func writeConfigProject(t *testing.T, root *os.Root) {
	t.Helper()
	writeFixture(t, root, "config.yaml", "old config")
	writeFixture(t, root, "_managed/vendor/team/rule.md", "old rule")
	writeFixture(t, root, "_managed/generated/RULES.md", "old output")
}

// requireConfigProject checks that all three targets hold the previous or the new content, never a mixture.
func requireConfigProject(t *testing.T, root *os.Root, state string) {
	t.Helper()
	for name, suffix := range map[string]string{"config.yaml": " config", "_managed/vendor/team/rule.md": " rule", "_managed/generated/RULES.md": " output"} {
		data, err := root.ReadFile(name)
		if err != nil || string(data) != state+suffix {
			t.Fatalf("%s: got %q, want %q: %v", name, data, state+suffix, err)
		}
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

// TestApply_ReplacesConfigurationWithManagedTrees installs all three targets in one transaction.
func TestApply_ReplacesConfigurationWithManagedTrees(t *testing.T) {
	root := openProject(t)
	writeConfigProject(t, root)
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(configUpdate, nil) }); err != nil {
		t.Fatal(err)
	}
	requireConfigProject(t, root, "new")
}

// TestApply_RollsBackConfigurationWhenCommitFails restores the configuration along with both trees.
func TestApply_RollsBackConfigurationWhenCommitFails(t *testing.T) {
	root := openProject(t)
	writeConfigProject(t, root)
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		rename := w.rename
		w.rename = func(from, to string) error {
			if strings.HasSuffix(to, "committed.json") {
				return errors.New("injected commit failure")
			}
			return rename(from, to)
		}
		return w.Apply(configUpdate, nil)
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	requireConfigProject(t, root, "old")
}

// TestApply_RefusesAConfigurationEditedDuringStaging keeps the user's edit and every previous target.
func TestApply_RefusesAConfigurationEditedDuringStaging(t *testing.T) {
	root := openProject(t)
	writeConfigProject(t, root)
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.Apply(configUpdate, func() error {
			writeFixture(t, root, "config.yaml", "user edit")
			return nil
		})
	})
	projectCode(t, err, "concurrent-change")
	if data, _ := root.ReadFile("config.yaml"); string(data) != "user edit" {
		t.Fatalf("lost the user's edit: %q", data)
	}
	if data, _ := root.ReadFile("_managed/generated/RULES.md"); string(data) != "old output" {
		t.Fatalf("replaced output: %q", data)
	}
}

// TestKilledConfigWriterRecovers stops the writer after each rename; the next writer restores all three targets,
// or, once the commit marker exists, keeps all three new ones.
func TestKilledConfigWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_CONFIG_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			rename := w.rename
			w.rename = func(from, to string) error {
				err := rename(from, to)
				if err == nil && to == os.Getenv("TRAILGUIDE_CONFIG_STOP") {
					os.Exit(42)
				}
				return err
			}
			return w.Apply(configUpdate, nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	for stop, state := range map[string]string{
		transactionName + "/rename-check/config.yaml": "old",
		"_managed/vendor":                    "old",
		"_managed/generated":                 "old",
		transactionName + "/old-config.yaml": "old",
		"config.yaml":                        "old",
		transactionName + "/committed.json":  "new",
	} {
		t.Run(stop, func(t *testing.T) {
			root := openProject(t)
			writeConfigProject(t, root)
			child := exec.Command(os.Args[0], "-test.run=^TestKilledConfigWriterRecovers$")
			child.Env = append(os.Environ(), "TRAILGUIDE_CONFIG_CHILD="+root.Name(), "TRAILGUIDE_CONFIG_STOP="+stop)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			requireConfigProject(t, root, state)
		})
	}
}

// TestRecovery_RefusesAConfigurationEntryInAnOlderJournal preserves a journal whose format can't hold the
// configuration.
func TestRecovery_RefusesAConfigurationEntryInAnOlderJournal(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "config.yaml", "new config")
	writeFixture(t, root, transactionName+"/old-config.yaml", "old config")
	before := &Tree{Files: map[string][]byte{"config.yaml": []byte("old config")}}
	after := &Tree{Files: map[string][]byte{"config.yaml": []byte("new config")}}
	if err := durableJSON(root, transactionName+"/journal.json", journalRecord{FormatVersion: 3, Entries: []journalEntry{{Config, treeDigest(before), treeDigest(after), true}}}); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if data, err := root.ReadFile(transactionName + "/old-config.yaml"); err != nil || string(data) != "old config" {
		t.Fatal("lost the configuration backup", err)
	}
}
