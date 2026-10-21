// Verify sync and build install a renamed skill and print the rename warning in both output modes, without failing.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSyncAndBuild_PrintTheRenameWarningInBothOutputModes renames the fixture's skill and checks that sync and build
// install it under its new name and print the warning, as a Warning line and in value.warnings, without failing.
func TestSyncAndBuild_PrintTheRenameWarningInBothOutputModes(t *testing.T) {
	u, _ := newSkillForkFixture(t)
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: "+u.fixture.Repository+"\n    ref: release/1\n    skills:\n      include: ['*']\n      rename:\n        skills/build-feature: team-build\n")
	const warning = "team:skills/build-feature is installed as team-build. Mentions of build-feature in skill text aren't rewritten; check the skills that call it by that name."
	for _, command := range [][]string{{"project", "sync"}, {"project", "build"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			out, diagnostic, code := u.run(t, command...)
			if code != 0 || diagnostic != "" || !strings.Contains("\n"+out, "\nWarning: "+warning+"\n") {
				t.Fatalf("human output: %d\n%s\n%s", code, out, diagnostic)
			}
			if _, err := os.Stat(filepath.Join(u.directory, ".claude/skills/team-build/SKILL.md")); err != nil {
				t.Fatalf("the renamed skill isn't installed: %v", err)
			}
			out, diagnostic, code = u.run(t, append(command, "--json")...)
			var result struct {
				OK    bool
				Value struct{ Warnings []string }
			}
			if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.OK || !reflect.DeepEqual(result.Value.Warnings, []string{warning}) {
				t.Fatalf("JSON output: %d\n%s\n%s", code, out, diagnostic)
			}
		})
	}
}
