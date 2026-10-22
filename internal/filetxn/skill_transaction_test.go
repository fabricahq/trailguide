// Exercise installing, replacing, and removing skill folders outside the Trail Guide directory together with managed
// trees: executable files, process death, edits, links, and refused targets.

package filetxn

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// openSkillProject returns the repository root and its Trail Guide directory, which holds the previous managed
// trees; .claude/skills holds the previous copy of the installed skill review and another tool's skill.
func openSkillProject(t *testing.T) (repository, root *os.Root) {
	t.Helper()
	repository = openProject(t)
	if err := repository.Mkdir(".trailguide", 0700); err != nil {
		t.Fatal(err)
	}
	root, err := repository.OpenRoot(".trailguide")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	// The writer finds the repository root by the Trail Guide directory's name.
	if root.Name() != filepath.Join(repository.Name(), ".trailguide") {
		t.Fatalf("root name %s", root.Name())
	}
	writeFixture(t, root, "_managed/vendor/team/rule.md", "old rule")
	writeFixture(t, root, "_managed/generated/RULES.md", "old output")
	writeFixture(t, repository, ".claude/skills/review/SKILL.md", "old review")
	writeFixture(t, repository, ".claude/skills/theirs/SKILL.md", "their skill")
	return repository, root
}

// skillUpdate installs build in both targets, with an executable script, replaces review with new text, or removes
// it when removeReview is set, and replaces the managed trees.
func skillUpdate(removeReview bool) (map[Target]map[string][]byte, map[Target][]string) {
	build := map[string][]byte{"SKILL.md": []byte("new build"), "scripts/verify.sh": []byte("#!/bin/sh\n")}
	review := map[string][]byte{"SKILL.md": []byte("new review")}
	if removeReview {
		review = nil
	}
	output := map[Target]map[string][]byte{
		Vendor:                               {"team/rule.md": []byte("new rule")},
		Generated:                            {"RULES.md": []byte("new output")},
		SkillFolder(".claude/skills/build"):  build,
		SkillFolder(".agents/skills/build"):  build,
		SkillFolder(".claude/skills/review"): review,
	}
	executable := map[Target][]string{SkillFolder(".claude/skills/build"): {"scripts/verify.sh"}, SkillFolder(".agents/skills/build"): {"scripts/verify.sh"}}
	return output, executable
}

// repositoryFiles returns the text of every file under the skill targets, by path from the repository root.
func repositoryFiles(t *testing.T, repository *os.Root) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, folder := range []string{".claude", ".agents"} {
		tree, err := ReadTree(context.Background(), repository, folder)
		if err != nil {
			t.Fatal(err)
		}
		if tree == nil {
			continue
		}
		for name, data := range tree.Files {
			files[folder+"/"+name] = string(data)
		}
	}
	return files
}

// requireSkillProject checks the managed trees' state and the files in the skill targets.
func requireSkillProject(t *testing.T, repository, root *os.Root, state string, want map[string]string) {
	t.Helper()
	for name, suffix := range map[string]string{"_managed/vendor/team/rule.md": " rule", "_managed/generated/RULES.md": " output"} {
		data, err := root.ReadFile(name)
		if err != nil || string(data) != state+suffix {
			t.Fatalf("%s: got %q, want %q: %v", name, data, state+suffix, err)
		}
	}
	if got := repositoryFiles(t, repository); !reflect.DeepEqual(got, want) {
		t.Fatalf("skill targets:\n%v\nwant\n%v", got, want)
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}

var (
	oldSkills = map[string]string{".claude/skills/review/SKILL.md": "old review", ".claude/skills/theirs/SKILL.md": "their skill"}
	newSkills = map[string]string{
		".claude/skills/review/SKILL.md": "new review", ".claude/skills/theirs/SKILL.md": "their skill",
		".claude/skills/build/SKILL.md": "new build", ".claude/skills/build/scripts/verify.sh": "#!/bin/sh\n",
		".agents/skills/build/SKILL.md": "new build", ".agents/skills/build/scripts/verify.sh": "#!/bin/sh\n",
	}
)

// TestApply_InstallsSkillFoldersWithTheirExecutableFiles writes each skill folder outside the Trail Guide directory,
// creating .agents/skills, replaces the existing folder, keeps the executable bit of the files listed, and leaves
// another tool's skill alone.
func TestApply_InstallsSkillFoldersWithTheirExecutableFiles(t *testing.T) {
	repository, root := openSkillProject(t)
	output, executable := skillUpdate(false)
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.ApplyModes(output, executable, nil) }); err != nil {
		t.Fatal(err)
	}
	requireSkillProject(t, repository, root, "new", newSkills)
	for _, folder := range []string{".claude/skills/build", ".agents/skills/build"} {
		tree, err := ReadTree(context.Background(), repository, folder)
		if err != nil || !reflect.DeepEqual(tree.Executable, []string{"scripts/verify.sh"}) {
			t.Fatalf("%s executable files: %+v, %v", folder, tree, err)
		}
		info, err := repository.Lstat(folder + "/SKILL.md")
		if err != nil || info.Mode().Perm()&0o111 != 0 {
			t.Fatalf("%s/SKILL.md mode %v, %v", folder, info, err)
		}
	}
}

// TestApply_RemovesASkillFolderAndItsEmptiedTarget removes an installed skill's folder, then the target folder it
// left empty, and treats removing an absent folder as nothing to do.
func TestApply_RemovesASkillFolderAndItsEmptiedTarget(t *testing.T) {
	repository, root := openSkillProject(t)
	output, executable := skillUpdate(true)
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.ApplyModes(output, executable, nil) }); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{".claude/skills/theirs/SKILL.md": "their skill", ".claude/skills/build/SKILL.md": "new build", ".claude/skills/build/scripts/verify.sh": "#!/bin/sh\n", ".agents/skills/build/SKILL.md": "new build", ".agents/skills/build/scripts/verify.sh": "#!/bin/sh\n"}
	requireSkillProject(t, repository, root, "new", want)
	removal := map[Target]map[string][]byte{SkillFolder(".agents/skills/build"): nil, SkillFolder(".agents/skills/absent"): nil}
	if err := WithWriter(context.Background(), root, func(w *Writer) error { return w.Apply(removal, nil) }); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Lstat(".agents/skills"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("the emptied target folder is still there", err)
	}
	if _, err := repository.Lstat(".agents"); err != nil {
		t.Fatal("the target folder's parent is gone", err)
	}
}

// TestApply_KeepsASkillFolderEditedDuringStaging refuses to replace an installed folder edited meanwhile, and keeps
// the edit and the previous output.
func TestApply_KeepsASkillFolderEditedDuringStaging(t *testing.T) {
	repository, root := openSkillProject(t)
	output, executable := skillUpdate(false)
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		return w.ApplyModes(output, executable, func() error {
			writeFixture(t, repository, ".claude/skills/review/notes.md", "my notes")
			return nil
		})
	})
	projectCode(t, err, "concurrent-change")
	want := map[string]string{".claude/skills/review/SKILL.md": "old review", ".claude/skills/review/notes.md": "my notes", ".claude/skills/theirs/SKILL.md": "their skill"}
	requireSkillProject(t, repository, root, "old", want)
}

// TestApply_RefusesALinkedSkillTarget refuses a skill target folder that is a symbolic link, writing nothing.
func TestApply_RefusesALinkedSkillTarget(t *testing.T) {
	repository, root := openSkillProject(t)
	if err := os.Symlink(filepath.Join(repository.Name(), ".claude"), filepath.Join(repository.Name(), ".agents")); err != nil {
		t.Fatal(err)
	}
	output, executable := skillUpdate(false)
	err := WithWriter(context.Background(), root, func(w *Writer) error { return w.ApplyModes(output, executable, nil) })
	projectCode(t, err, "unsafe-path")
	if data, err := root.ReadFile("_managed/generated/RULES.md"); err != nil || string(data) != "old output" {
		t.Fatalf("generated output changed: %q, %v", data, err)
	}
}

// TestApply_RefusesInvalidSkillTargets refuses folders outside the skill targets, invalid install names, and
// executable files the output doesn't have.
func TestApply_RefusesInvalidSkillTargets(t *testing.T) {
	_, root := openSkillProject(t)
	for _, target := range []Target{"../.cursor/skills/review", "../.claude/skills/Review", "../.claude/skills", "../.claude/skills/review/nested", "../../.claude/skills/review"} {
		err := WithWriter(context.Background(), root, func(w *Writer) error {
			return w.Apply(map[Target]map[string][]byte{target: {"SKILL.md": []byte("x")}}, nil)
		})
		projectCode(t, err, "invalid-target")
	}
	for name, executable := range map[string]map[Target][]string{
		"missing file": {SkillFolder(".claude/skills/build"): {"absent.sh"}},
		"file target":  {Config: {"config.yaml"}},
	} {
		t.Run(name, func(t *testing.T) {
			output, _ := skillUpdate(false)
			output[Config] = map[string][]byte{"config.yaml": []byte("schemaVersion: 2\n")}
			err := WithWriter(context.Background(), root, func(w *Writer) error { return w.ApplyModes(output, executable, nil) })
			projectCode(t, err, "invalid-target")
		})
	}
}

// TestKilledSkillWriterRecovers stops the writer at each step of replacing a skill folder; the next writer restores
// the previous folders and trees, removing the new ones, or, once the commit marker exists, keeps everything new.
func TestKilledSkillWriterRecovers(t *testing.T) {
	if directory := os.Getenv("TRAILGUIDE_SKILL_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		stop := os.Getenv("TRAILGUIDE_SKILL_STOP")
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
			output, executable := skillUpdate(false)
			return w.ApplyModes(output, executable, nil)
		})
		t.Fatalf("child did not stop at rename: %v", err)
	}
	for _, test := range []struct{ name, stop, state string }{
		{"_managed/generated", "_managed/generated", "old"},
		{"first skill folder", ".agents/skills/build", "old"},
		{"backup", ".trailguide/" + transactionName + "/" + backupPrefix + "..+.claude+skills+review", "old"},
		{"replaced folder", ".claude/skills/review", "old"},
		{"committed", transactionName + "/committed.json", "new"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, root := openSkillProject(t)
			child := exec.Command(os.Args[0], "-test.run=^TestKilledSkillWriterRecovers$")
			child.Env = append(os.Environ(), "TRAILGUIDE_SKILL_CHILD="+root.Name(), "TRAILGUIDE_SKILL_STOP="+test.stop)
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 42 {
				t.Fatal(err, string(output))
			}
			if err := WithWriter(context.Background(), root, func(*Writer) error { return nil }); err != nil {
				t.Fatal(err)
			}
			want := oldSkills
			if test.state == "new" {
				want = newSkills
			}
			requireSkillProject(t, repository, root, test.state, want)
		})
	}
}

// TestRecovery_RefusesASkillFolderInAnOlderJournal preserves a journal whose format can't hold a skill folder.
func TestRecovery_RefusesASkillFolderInAnOlderJournal(t *testing.T) {
	repository, root := openSkillProject(t)
	folder := &Tree{Files: map[string][]byte{"SKILL.md": []byte("old review")}}
	if err := root.Mkdir(transactionName, 0700); err != nil {
		t.Fatal(err)
	}
	record := journalRecord{FormatVersion: 6, Entries: []journalEntry{{SkillFolder(".claude/skills/review"), treeDigest(folder), treeDigest(nil), true}}}
	if err := durableJSON(root, transactionName+"/journal.json", record); err != nil {
		t.Fatal(err)
	}
	projectCode(t, WithWriter(context.Background(), root, func(*Writer) error { return nil }), "recovery-required")
	if got := repositoryFiles(t, repository); !reflect.DeepEqual(got, oldSkills) {
		t.Fatalf("recovery changed skill folders: %v", got)
	}
}

// TestTreeDigestCoversExecutableFiles changes a tree's digest when only a file's executable bit changes, and keeps
// the digest of a tree without executable files.
func TestTreeDigestCoversExecutableFiles(t *testing.T) {
	plain := &Tree{Files: map[string][]byte{"run.sh": []byte("x")}, Directories: []string{}}
	executable := &Tree{Files: plain.Files, Directories: []string{}, Executable: []string{"run.sh"}}
	if plain.Digest() == executable.Digest() {
		t.Fatal("the executable bit doesn't change the digest")
	}
	if empty := (&Tree{Files: plain.Files, Directories: []string{}, Executable: []string{}}); empty.Digest() != plain.Digest() {
		t.Fatal("an empty executable list changes the digest")
	}
}
