// Exercise replacing an authored skill folder, a skill fork that an update merges a source's changes into, together
// with the configuration and managed trees.

package filetxn

import (
	"context"
	"reflect"
	"testing"
)

// TestApply_ReplacesAnAuthoredSkillWithTheConfiguration replaces the fork's folder, dropping a file the merge
// removed and keeping the executable bit of the files listed, in the transaction that replaces the configuration.
func TestApply_ReplacesAnAuthoredSkillWithTheConfiguration(t *testing.T) {
	_, root := openSkillProject(t)
	writeFixture(t, root, "config.yaml", "old config")
	writeFixture(t, root, "skills/build/SKILL.md", "old fork")
	writeFixture(t, root, "skills/build/notes.md", "dropped")
	output := map[Target]map[string][]byte{
		Config:                 {"config.yaml": []byte("new config")},
		AuthoredSkill("build"): {"SKILL.md": []byte("merged fork"), "scripts/check.sh": []byte("#!/bin/sh\n")},
	}
	executable := map[Target][]string{AuthoredSkill("build"): {"scripts/check.sh"}}
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.ApplyModes(output, executable, nil) }); err != nil {
		t.Fatal(err)
	}
	tree, err := ReadTree(context.Background(), root, "skills/build")
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

// TestApply_RefusesInvalidAuthoredSkillTargets refuses a folder that isn't a valid skill name directly under skills/,
// and removing a fork's folder, which only the person who owns the fork does.
func TestApply_RefusesInvalidAuthoredSkillTargets(t *testing.T) {
	_, root := openSkillProject(t)
	for _, output := range []map[Target]map[string][]byte{
		{"skills/Build": {"SKILL.md": []byte("x")}},
		{"skills/build/nested": {"SKILL.md": []byte("x")}},
		{"skills": {"build/SKILL.md": []byte("x")}},
		{AuthoredSkill("build"): nil},
	} {
		projectCode(t, WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(output, nil) }), "invalid-target")
	}
}

// TestRecovery_RefusesAnAuthoredSkillInAnOlderJournal treats an authored skill entry in a journal older than format
// 9 as invalid, leaving the fork as it is.
func TestRecovery_RefusesAnAuthoredSkillInAnOlderJournal(t *testing.T) {
	_, root := openSkillProject(t)
	writeFixture(t, root, "skills/build/SKILL.md", "old fork")
	folder := &Tree{Files: map[string][]byte{"SKILL.md": []byte("old fork")}}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	record := journalRecord{FormatVersion: 8, Entries: []journalEntry{{AuthoredSkill("build"), treeDigest(folder), treeDigest(&Tree{Files: map[string][]byte{"SKILL.md": []byte("new fork")}}), true}}}
	if err := durableJSON(root, transactionName+"/journal.json", record); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if data, err := root.ReadFile("skills/build/SKILL.md"); err != nil || string(data) != "old fork" {
		t.Fatalf("recovery changed the fork: %q, %v", data, err)
	}
}
