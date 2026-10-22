// Test that sync, update, and check warn about personal skills with the name of an installed skill, in a fake home
// directory they never write to.

package project

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// installedRecord is an installed-skills.json that lists review, from the team source, in both skill targets, and
// plan, authored and recorded without targets.
const installedRecord = `{"schemaVersion":1,"skills":[` +
	`{"name":"plan","id":"skills/plan","targets":[]},` +
	`{"name":"review","id":"team:skills/review","targets":[".agents/skills/review",".claude/skills/review"]}]}`

// The warning's line for each agent that reads both a personal skill and the project's copy.
const (
	claudeCodeLine = "  Claude Code uses your personal skill.\n"
	codexLine      = "  Codex shows both.\n"
	grokBuildLine  = "  Grok Build reads both, and its docs don't say which it uses.\n"
	vsCodeLine     = "  VS Code reads both, and its docs don't say which it uses.\n"
)

// reviewWarning is the warning for a personal skill review in folder, such as .claude/skills, when the project installs
// team:skills/review into both skill targets, with agentLines between its first and last lines.
func reviewWarning(folder string, agentLines ...string) string {
	return "your personal skill ~/" + folder + "/review has the same name as this project's review (team:skills/review, installed at .agents/skills/review and .claude/skills/review).\n" +
		strings.Join(agentLines, "") +
		"Rename or remove your personal skill to use the project's."
}

// writeHomeFile writes text to name, a slash-separated path under directory, creating its folders.
func writeHomeFile(t *testing.T, directory, name, text string) {
	t.Helper()
	file := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPersonalSkillWarnings_NamesEachPersonalSkillWithAnInstalledSkillsName(t *testing.T) {
	tests := []struct {
		name string
		// arrange builds a fake home and repository in two empty folders, and returns the home and repository to read.
		arrange func(t *testing.T, home, repository string) (string, string)
		want    []string
	}{
		{"no personal skills", func(t *testing.T, home, repository string) (string, string) { return home, repository }, nil},
		{"missing home", func(t *testing.T, home, repository string) (string, string) {
			return filepath.Join(home, "missing"), repository
		}, nil},
		{"empty home", func(t *testing.T, _, repository string) (string, string) { return "", repository }, nil},
		{"relative home", func(t *testing.T, _, repository string) (string, string) {
			return filepath.Join("relative", "home"), repository
		}, nil},
		{"a Claude Code skill", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, ".claude/skills/review/SKILL.md", "# Mine\n")
			return home, repository
		}, []string{reviewWarning(".claude/skills", claudeCodeLine, grokBuildLine, vsCodeLine)}},
		{"one in every known folder", func(t *testing.T, home, repository string) (string, string) {
			for _, folder := range []string{".copilot/skills", ".grok/skills", ".codex/skills", ".agents/skills", ".claude/skills"} {
				writeHomeFile(t, home, folder+"/review/SKILL.md", "# Mine\n")
			}
			return home, repository
		}, []string{
			reviewWarning(".claude/skills", claudeCodeLine, grokBuildLine, vsCodeLine),
			reviewWarning(".agents/skills", codexLine, grokBuildLine, vsCodeLine),
			reviewWarning(".codex/skills"),
			reviewWarning(".grok/skills", grokBuildLine),
			reviewWarning(".copilot/skills", vsCodeLine),
		}},
		{"a folder without SKILL.md", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, ".claude/skills/review/notes.md", "# Notes\n")
			return home, repository
		}, nil},
		{"a SKILL.md that is a folder", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, ".claude/skills/review/SKILL.md/notes.md", "# Notes\n")
			return home, repository
		}, nil},
		{"a file with the skill's name", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, ".claude/skills/review", "# Mine\n")
			return home, repository
		}, nil},
		{"another name", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, ".claude/skills/reviews/SKILL.md", "# Mine\n")
			return home, repository
		}, nil},
		{"a skill installed without targets", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, ".claude/skills/plan/SKILL.md", "# Mine\n")
			return home, repository
		}, nil},
		{"a linked skill folder", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, "dotfiles/review/SKILL.md", "# Mine\n")
			writeHomeFile(t, home, ".claude/skills/other/SKILL.md", "# Other\n")
			if err := os.Symlink(filepath.Join(home, "dotfiles/review"), filepath.Join(home, ".claude/skills/review")); err != nil {
				t.Fatal(err)
			}
			return home, repository
		}, []string{reviewWarning(".claude/skills", claudeCodeLine, grokBuildLine, vsCodeLine)}},
		{"a skills folder linked to another", func(t *testing.T, home, repository string) (string, string) {
			writeHomeFile(t, home, ".claude/skills/review/SKILL.md", "# Mine\n")
			if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(home, ".claude/skills"), filepath.Join(home, ".agents/skills")); err != nil {
				t.Fatal(err)
			}
			return home, repository
		}, []string{reviewWarning(".claude/skills", claudeCodeLine, grokBuildLine, vsCodeLine)}},
		{"a repository that is the home directory", func(t *testing.T, _, repository string) (string, string) {
			writeHomeFile(t, repository, ".claude/skills/review/SKILL.md", "# Installed\n")
			writeHomeFile(t, repository, ".agents/skills/review/SKILL.md", "# Installed\n")
			return repository, repository
		}, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home, repository := test.arrange(t, t.TempDir(), t.TempDir())
			got := personalSkillWarnings(home, repository, []byte(installedRecord))
			if got == nil || !slices.Equal(got, test.want) {
				t.Fatalf("warnings = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPersonalSkillWarnings_NamesAnAgentOnlyWhenItReadsBothCopies(t *testing.T) {
	tests := []struct {
		personal, target string
		lines            []string
	}{
		{".claude/skills", ".claude/skills", []string{claudeCodeLine, grokBuildLine, vsCodeLine}},
		{".claude/skills", ".agents/skills", []string{vsCodeLine}},
		{".agents/skills", ".agents/skills", []string{codexLine, vsCodeLine}},
		{".agents/skills", ".claude/skills", []string{grokBuildLine, vsCodeLine}},
		{".grok/skills", ".claude/skills", []string{grokBuildLine}},
		{".grok/skills", ".agents/skills", nil},
		{".copilot/skills", ".agents/skills", []string{vsCodeLine}},
		{".codex/skills", ".agents/skills", nil},
	}
	for _, test := range tests {
		t.Run(test.personal+" beside "+test.target, func(t *testing.T) {
			home := t.TempDir()
			writeHomeFile(t, home, test.personal+"/review/SKILL.md", "# Mine\n")
			record := `{"schemaVersion":1,"skills":[{"name":"review","id":"team:skills/review","targets":["` + test.target + `/review"]}]}`
			want := "your personal skill ~/" + test.personal + "/review has the same name as this project's review (team:skills/review, installed at " + test.target + "/review).\n" +
				strings.Join(test.lines, "") + "Rename or remove your personal skill to use the project's."
			if got := personalSkillWarnings(home, t.TempDir(), []byte(record)); !slices.Equal(got, []string{want}) {
				t.Fatalf("warnings = %q, want %q", got, []string{want})
			}
		})
	}
}

func TestPersonalSkillWarnings_ReadsNoSkillsFromAnUnreadableRecord(t *testing.T) {
	home := t.TempDir()
	writeHomeFile(t, home, ".claude/skills/review/SKILL.md", "# Mine\n")
	for _, record := range []string{"", "{", `{"schemaVersion":2,"skills":[{"name":"review","id":"team:skills/review","targets":[".claude/skills/review"]}]}`} {
		if got := personalSkillWarnings(home, t.TempDir(), []byte(record)); got == nil || len(got) != 0 {
			t.Errorf("record %q: warnings = %q, want none", record, got)
		}
	}
}

// homeState records every path under home with its mode, size, modification time, and contents, to show that a
// command left the home directory unchanged.
func homeState(t *testing.T, home string) map[string]string {
	t.Helper()
	state := map[string]string{}
	err := filepath.WalkDir(home, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		contents := ""
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			contents = string(data)
		}
		state[name] = info.Mode().String() + " " + info.ModTime().Format(time.RFC3339Nano) + " " + contents
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestSyncCheckAndUpdate_WarnAboutPersonalSkillsWithAnInstalledSkillsName(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/review"]}}}}`)
	options.Home = t.TempDir()

	// With no personal skills, as on CI, nothing warns.
	changes, err := Sync(ctx, options, git)
	if err != nil || len(changes.Warnings) != 0 {
		t.Fatalf("sync without personal skills: warnings %q, %v", changes.Warnings, err)
	}
	checked, err := Check(ctx, options)
	if err != nil || !checked.Current() || checked.Warnings == nil || len(checked.Warnings) != 0 {
		t.Fatalf("check without personal skills = %+v, %v", checked, err)
	}

	writeHomeFile(t, options.Home, ".claude/skills/review/SKILL.md", "# My review\n")
	before := homeState(t, options.Home)
	want := []string{reviewWarning(".claude/skills", claudeCodeLine, grokBuildLine, vsCodeLine)}
	changes, err = Sync(ctx, options, git)
	if err != nil || !slices.Equal(changes.Warnings, want) {
		t.Fatalf("sync: warnings %q, %v", changes.Warnings, err)
	}
	checked, err = Check(ctx, options)
	if err != nil || !checked.Current() || !slices.Equal(checked.Warnings, want) {
		t.Fatalf("check = %+v, %v; want current with the warning", checked, err)
	}
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil || !slices.Equal(preview.Warnings, want) {
		t.Fatalf("update preview: warnings %q, %v", preview.Warnings, err)
	}
	applied, err := plan.Apply(ctx, nil)
	if err != nil || !slices.Equal(applied.Warnings, want) {
		t.Fatalf("update: warnings %q, %v", applied.Warnings, err)
	}
	if after := homeState(t, options.Home); !maps.Equal(before, after) {
		t.Fatalf("home changed:\nbefore %v\nafter  %v", before, after)
	}
	if !strings.Contains(readRepositoryFile(t, options, ".claude/skills/review/SKILL.md"), "# Review") {
		t.Fatal("the project's installed copy isn't the project's skill")
	}
}
