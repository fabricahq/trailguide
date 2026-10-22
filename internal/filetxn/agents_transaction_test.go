// Exercise replacing the repository's AGENTS.md, outside the Trail Guide directory, together with managed trees:
// creation, replacement, process death, and journals too old to hold it.

package filetxn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

// agentsUpdate replaces the managed trees and AGENTS.md.
func agentsUpdate() map[Target]map[string][]byte {
	return map[Target]map[string][]byte{
		Vendor:     {"team/rule.md": []byte("new rule")},
		Generated:  {"RULES.md": []byte("new output")},
		AgentsFile: {string(AgentsFile): []byte("new agents")},
	}
}

// requireAgentsProject checks the managed trees' state and AGENTS.md, which want is nil to require absent.
func requireAgentsProject(t *testing.T, repository, root *os.Root, state string, want []byte) {
	t.Helper()
	requireSkillProject(t, repository, root, state, oldSkills)
	got, err := ReadOptional(context.Background(), repository, "AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AGENTS.md is %q, want %q", got, want)
	}
}

// TestApplyAgentsFile creates AGENTS.md when it's missing and replaces it when it exists.
func TestApplyAgentsFile(t *testing.T) {
	for name, previous := range map[string][]byte{"missing": nil, "existing": []byte("old agents")} {
		t.Run(name, func(t *testing.T) {
			repository, root := openSkillProject(t)
			if previous != nil {
				writeFixture(t, repository, "AGENTS.md", string(previous))
			}
			if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(agentsUpdate(), nil) }); err != nil {
				t.Fatal(err)
			}
			requireAgentsProject(t, repository, root, "new", []byte("new agents"))
		})
	}
}

// TestKilledAgentsWriterRecovers stops the writer at each step of replacing AGENTS.md; the next writer restores the
// previous file and trees, or, once the commit marker exists, keeps everything new.
func TestKilledAgentsWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_AGENTS_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		stop := os.Getenv("TRAILGUIDE_AGENTS_STOP")
		err = WithWriter(context.Background(), root, func(w *Writer) error {
			rename := w.rename
			w.rename = func(from, to string) error {
				err := rename(from, to)
				if err == nil && to == stop {
					os.Exit(42)
				}
				return err
			}
			w.renameRepository = func(repository *os.Root, from, to string) error {
				err := repository.Rename(from, to)
				if err == nil && to == stop {
					os.Exit(42)
				}
				return err
			}
			return w.Apply(agentsUpdate(), nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	for _, test := range []struct{ name, stop, state string }{
		{"_managed/generated", "_managed/generated", "old"},
		{"backup", ".trailguide/" + transactionName + "/" + backupPrefix + "..+AGENTS.md", "old"},
		{"replaced file", "AGENTS.md", "old"},
		{"committed", transactionName + "/committed.json", "new"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, root := openSkillProject(t)
			writeFixture(t, repository, "AGENTS.md", "old agents")
			child := exec.Command(os.Args[0], "-test.run=^TestKilledAgentsWriterRecovers$")
			child.Env = append(os.Environ(), "TRAILGUIDE_AGENTS_CHILD="+root.Name(), "TRAILGUIDE_AGENTS_STOP="+test.stop)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			want := []byte("old agents")
			if test.state == "new" {
				want = []byte("new agents")
			}
			requireAgentsProject(t, repository, root, test.state, want)
		})
	}
}

// TestRecovery_RefusesAgentsFileInAnOlderJournal preserves a journal whose format can't hold AGENTS.md.
func TestRecovery_RefusesAgentsFileInAnOlderJournal(t *testing.T) {
	repository, root := openSkillProject(t)
	writeFixture(t, repository, "AGENTS.md", "old agents")
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	file := &Tree{Files: map[string][]byte{string(AgentsFile): []byte("old agents")}}
	record := journalRecord{FormatVersion: 7, Entries: []journalEntry{{AgentsFile, treeDigest(file), treeDigest(&Tree{Files: map[string][]byte{string(AgentsFile): []byte("new agents")}}), true}}}
	if err := durableJSON(root, transactionName+"/journal.json", record); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if got, err := repository.ReadFile("AGENTS.md"); err != nil || string(got) != "old agents" {
		t.Fatalf("recovery changed AGENTS.md: %q %v", got, err)
	}
}
