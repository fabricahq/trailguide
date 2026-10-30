// Verify sync, update, and check print the personal skill warning in both output modes, without failing.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tidyWarning is the warning for the personal skill ~/.claude/skills/tidy beside the project's authored tidy.
const tidyWarning = "your personal skill ~/.claude/skills/tidy has the same name as this project's tidy (skills/tidy, installed at .claude/skills/tidy).\n" +
	"  Claude Code uses your personal skill.\n" +
	"  Grok Build reads both, and its docs don't say which it uses.\n" +
	"  VS Code reads both, and its docs don't say which it uses.\n" +
	"Rename or remove your personal skill to use the project's."

// writeTestFile writes text to name, a slash-separated path under directory, creating its folders.
func writeTestFile(t *testing.T, directory, name, text string) {
	t.Helper()
	file := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncUpdateAndCheck_WarnAboutAPersonalSkillWithAnInstalledSkillsName(t *testing.T) {
	ctx := context.Background()
	directory, home := t.TempDir(), t.TempDir()
	if out, diagnostic, code := invokeWithHome(t, ctx, directory, home, "project", "init", "--skill-targets", "claude"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	writeTestFile(t, directory, ".trailguide/skills/local/tidy/SKILL.md", "---\nname: tidy\ndescription: Tidy up.\n---\n# Tidy\n")
	commands := [][]string{{"project", "sync"}, {"project", "check"}, {"project", "update", "--yes"}}

	// With no personal skills, as on CI, nothing warns.
	for _, command := range commands {
		out, diagnostic, code := invokeWithHome(t, ctx, directory, home, command...)
		if code != 0 || strings.Contains(out, "Warning") {
			t.Fatalf("%v without personal skills: %d %s %s", command, code, out, diagnostic)
		}
		out, diagnostic, code = invokeWithHome(t, ctx, directory, home, append(command, "--json")...)
		var result struct {
			OK    bool
			Value struct{ Warnings json.RawMessage }
		}
		if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.OK || string(result.Value.Warnings) != "[]" {
			t.Fatalf("%v --json without personal skills: %d %s %s", command, code, out, diagnostic)
		}
	}

	writeTestFile(t, home, ".claude/skills/tidy/SKILL.md", "---\nname: tidy\ndescription: My own tidy.\n---\n# Mine\n")
	for _, command := range commands {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			out, diagnostic, code := invokeWithHome(t, ctx, directory, home, command...)
			if code != 0 || diagnostic != "" || !strings.Contains("\n"+out, "\nWarning: "+tidyWarning+"\n") {
				t.Fatalf("human output: %d\n%s\n%s", code, out, diagnostic)
			}
			out, diagnostic, code = invokeWithHome(t, ctx, directory, home, append(command, "--json")...)
			var result struct {
				OK    bool
				Value struct {
					Status   string
					Warnings []string
				}
			}
			if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.OK || len(result.Value.Warnings) != 1 || result.Value.Warnings[0] != tidyWarning {
				t.Fatalf("JSON output: %d\n%s\n%s", code, out, diagnostic)
			}
			if command[1] == "check" && result.Value.Status != "up-to-date" {
				t.Fatalf("check status %q; the warning mustn't make the project out of date", result.Value.Status)
			}
		})
	}
}
