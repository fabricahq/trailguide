// Exercise replacing an authored skill folder, a skill fork that an update merges a source's changes into, together
// with the configuration, the managed trees, and the fork's installed copy, and recovering when the writer dies.

package filetxn

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

// TestApply_ReplacesAnAuthoredSkillWithTheConfiguration replaces the fork's folder, dropping a file the merge
// removed and keeping the executable bit of the files listed, in the transaction that replaces the configuration.
func TestApply_ReplacesAnAuthoredSkillWithTheConfiguration(t *testing.T) {
	_, root := openSkillProject(t)
	writeFixture(t, root, "config.yaml", "old config")
	writeFixture(t, root, "skills/fabrica/build/SKILL.md", "old fork")
	writeFixture(t, root, "skills/fabrica/build/notes.md", "dropped")
	output := map[Target]map[string][]byte{
		Config:                            {"config.yaml": []byte("new config")},
		AuthoredSkill("fabrica", "build"): {"SKILL.md": []byte("merged fork"), "scripts/check.sh": []byte("#!/bin/sh\n")},
	}
	executable := map[Target][]string{AuthoredSkill("fabrica", "build"): {"scripts/check.sh"}}
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.ApplyModes(output, executable, nil) }); err != nil {
		t.Fatal(err)
	}
	tree, err := ReadTree(context.Background(), root, "skills/fabrica/build")
	if err != nil {
		t.Fatal(err)
	}
	want := &Tree{Files: map[string][]byte{"SKILL.md": []byte("merged fork"), "scripts/check.sh": []byte("#!/bin/sh\n")}, Directories: []string{"scripts"}, Executable: []string{"scripts/check.sh"}}
	if !reflect.DeepEqual(tree, want) {
		t.Fatalf("fork = %+v; want %+v", tree, want)
	}
	if data, err := root.ReadFile("config.yaml"); err != nil || string(data) != "new config" {
		t.Fatalf("config = %q, %v", data, err)
	}
}

// TestApply_RefusesInvalidAuthoredSkillTargets refuses a folder that isn't a valid skill name in an origin folder,
// a skill folder directly under skills/, which only older journals name, and removing a fork's folder, which only
// the person who owns the fork does.
func TestApply_RefusesInvalidAuthoredSkillTargets(t *testing.T) {
	_, root := openSkillProject(t)
	for _, output := range []map[Target]map[string][]byte{
		{"skills/Build": {"SKILL.md": []byte("x")}},
		{"skills/build": {"SKILL.md": []byte("x")}},
		{"skills/fabrica/build/nested": {"SKILL.md": []byte("x")}},
		{"skills": {"build/SKILL.md": []byte("x")}},
		{AuthoredSkill("fabrica", "build"): nil},
	} {
		projectCode(t, WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(output, nil) }), "invalid-target")
	}
}

// TestRecovery_RefusesAnAuthoredSkillInAnOlderJournal treats an authored skill entry in a journal older than format
// 9 as invalid, leaving the fork as it is.
func TestRecovery_RefusesAnAuthoredSkillInAnOlderJournal(t *testing.T) {
	_, root := openSkillProject(t)
	writeFixture(t, root, "skills/fabrica/build/SKILL.md", "old fork")
	folder := &Tree{Files: map[string][]byte{"SKILL.md": []byte("old fork")}}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	record := journalRecord{FormatVersion: 8, Entries: []journalEntry{{AuthoredSkill("fabrica", "build"), treeDigest(folder), treeDigest(&Tree{Files: map[string][]byte{"SKILL.md": []byte("new fork")}}), true}}}
	if err := durableJSON(root, transactionName+"/journal.json", record); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if data, err := root.ReadFile("skills/fabrica/build/SKILL.md"); err != nil || string(data) != "old fork" {
		t.Fatalf("recovery changed the fork: %q, %v", data, err)
	}
}

// TestRecovery_RefusesAnAuthoredSkillInTheOtherJournalsLayout treats a skill folder directly under skills/ in a
// journal of format 12, and one in an origin folder in a journal of format 11, as invalid, since each format names
// authored skills only one way.
func TestRecovery_RefusesAnAuthoredSkillInTheOtherJournalsLayout(t *testing.T) {
	for _, test := range []struct {
		target  Target
		version int
	}{{"skills/build", 12}, {AuthoredSkill("fabrica", "build"), 11}} {
		_, root := openSkillProject(t)
		writeFixture(t, root, "skills/fabrica/build/SKILL.md", "old fork")
		folder := &Tree{Files: map[string][]byte{"SKILL.md": []byte("old fork")}}
		if err := root.Mkdir(transactionName, 0700); err != nil {
			t.Fatal(err)
		}
		record := journalRecord{FormatVersion: test.version, Entries: []journalEntry{{test.target, treeDigest(folder), treeDigest(&Tree{Files: map[string][]byte{"SKILL.md": []byte("new fork")}}), true}}}
		if err := durableJSON(root, transactionName+"/journal.json", record); err != nil {
			t.Fatal(err)
		}
		projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	}
}

// authoredSkillUpdate merges the fork build, dropping its notes, and writes the configuration that moves its basedOn,
// the managed trees, and the fork's installed copy, as an update that merges a source's changes into a fork does.
func authoredSkillUpdate() map[Target]map[string][]byte {
	return map[Target]map[string][]byte{
		Config:                              {"config.yaml": []byte("new config")},
		Vendor:                              {"team/rule.md": []byte("new rule")},
		Generated:                           {"RULES.md": []byte("new output")},
		AuthoredSkill("fabrica", "build"):   {"SKILL.md": []byte("new fork")},
		SkillFolder(".claude/skills/build"): {"SKILL.md": []byte("new fork")},
	}
}

// TestKilledAuthoredSkillWriterRecovers stops the writer after each rename of an update that merges a fork; the next
// writer restores the previous fork, configuration, trees, and installed copies, removing the new installed copy,
// or, once the commit marker exists, keeps everything new, so the fork and its basedOn never disagree.
func TestKilledAuthoredSkillWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_AUTHORED_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		stop := os.Getenv("TRAILGUIDE_AUTHORED_STOP")
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
			return w.Apply(authoredSkillUpdate(), nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	for _, test := range []struct{ name, stop, state string }{
		{"configuration", "config.yaml", "old"},
		{"installed copy", ".claude/skills/build", "old"},
		{"fork backed up", transactionName + "/" + backupPrefix + "skills+fabrica+build", "old"},
		{"fork", "skills/fabrica/build", "old"},
		{"committed", transactionName + "/committed.json", "new"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, root := openSkillProject(t)
			writeFixture(t, root, "config.yaml", "old config")
			writeFixture(t, root, "skills/fabrica/build/SKILL.md", "old fork")
			writeFixture(t, root, "skills/fabrica/build/notes.md", "old notes")
			child := exec.Command(os.Args[0], "-test.run=^TestKilledAuthoredSkillWriterRecovers$")
			child.Env = append(os.Environ(), "TRAILGUIDE_AUTHORED_CHILD="+root.Name(), "TRAILGUIDE_AUTHORED_STOP="+test.stop)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			installed := maps.Clone(oldSkills)
			fork := map[string][]byte{"SKILL.md": []byte("old fork"), "notes.md": []byte("old notes")}
			if test.state == "new" {
				installed[".claude/skills/build/SKILL.md"] = "new fork"
				fork = map[string][]byte{"SKILL.md": []byte("new fork")}
			}
			requireSkillProject(t, repository, root, test.state, installed)
			if data, err := root.ReadFile("config.yaml"); err != nil || string(data) != test.state+" config" {
				t.Fatalf("config = %q, %v; want %s config", data, err, test.state)
			}
			tree, err := ReadTree(context.Background(), root, "skills/fabrica/build")
			if err != nil || tree == nil || !reflect.DeepEqual(tree.Files, fork) {
				t.Fatalf("fork = %+v, %v; want %q", tree, err, fork)
			}
		})
	}
}
