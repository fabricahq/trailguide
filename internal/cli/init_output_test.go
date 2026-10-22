// Exercise initialization guidance and repeat setup through the real executable.

package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectInitGuidance(t *testing.T) {
	binary := buildCLI(t)
	t.Run("project-root", func(t *testing.T) {
		directory := t.TempDir()
		args := []string{"project", "init"}
		out, diagnostic, code := runCLI(t, binary, directory, args...)
		if code != 0 || diagnostic != "" || !strings.HasPrefix(out, "Trail Guide initialized!\n") {
			t.Fatal(code, out, diagnostic)
		}
		if strings.Contains(out, "Updated files:") || strings.Contains(out, directory) {
			t.Fatal("init led with internal file changes", out)
		}
		// Like other authoring commands, init lists the files it added, relative to the working directory.
		if !strings.Contains(out, "Added:\n  .trailguide/config.yaml\n  .trailguide/rules/README.md\n  .trailguide/README.md\n  .trailguide/skills/README.md\n  AGENTS.md\n") || strings.Contains(out, "Project configuration:") {
			t.Fatal("init doesn't list the files it added", out)
		}
		if !strings.Contains(out, "Or use a shared library or skills repository") || !strings.Contains(out, "trailguide project sync") {
			t.Fatal("missing shared-library path", out)
		}
		// Without this step, a user whose agent doesn't read AGENTS.md has rules that no agent reads.
		if !strings.Contains(out, "AGENTS.md tells your coding agent to read the rules. If your agent reads another file, such as CLAUDE.md, import or link AGENTS.md there") {
			t.Fatal("missing agent connection step", out)
		}
		if agents, err := os.ReadFile(filepath.Join(directory, "AGENTS.md")); err != nil || !strings.HasPrefix(string(agents), "<!-- trailguide:begin ") || !strings.Contains(string(agents), "Read `.trailguide/_managed/generated/RULES.md`") {
			t.Fatal("init didn't add the AGENTS.md section", string(agents), err)
		}
		// Execute the displayed project-only examples, supplying metadata to avoid terminal prompts.
		steps := 0
		for _, line := range strings.Split(out, "\n") {
			command := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(command, "trailguide project add group "):
				command += " --name Testing --description 'Testing practices.' --when-to-read 'When writing tests.'"
			case strings.HasPrefix(command, "trailguide project add rule "):
				command += " --title 'Test boundaries' --impact HIGH --impact-description 'Catch edge cases.' --when-to-read 'When writing tests.'"
			default:
				continue
			}
			command = strings.Replace(command, "trailguide", "'"+strings.ReplaceAll(binary, "'", "'\"'\"'")+"'", 1)
			child := exec.Command("/bin/sh", "-eu", "-c", command)
			child.Dir = directory
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatal(command, err, string(output))
			}
			steps++
		}
		if steps != 2 {
			t.Fatal("missing project-only examples", out)
		}
		out, diagnostic, code = runCLI(t, binary, directory, args...)
		if code != 0 || diagnostic != "" || !strings.HasPrefix(out, "Trail Guide is already initialized.\nNo files changed.\n") || strings.Contains(out, "my-rule") {
			t.Fatal("repeat init should report existing setup", code, out, diagnostic)
		}
		location := ".trailguide/config.yaml"
		if !strings.Contains(out, "Project configuration: "+location+"\n") || !strings.Contains(out, "Trail Guide directory: "+".trailguide"+"\n") {
			t.Fatal("repeat init should show the configuration path", out)
		}
		out, diagnostic, code = runCLI(t, binary, directory, append(args, "--json")...)
		var result struct {
			OK    bool
			Value map[string]json.RawMessage
		}
		if code != 0 || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || !result.OK || len(result.Value) != 4 || result.Value["warnings"] == nil || result.Value["added"] == nil || result.Value["changed"] == nil || result.Value["nextSteps"] == nil {
			t.Fatal("JSON contract changed", code, out, diagnostic)
		}
	})
}
