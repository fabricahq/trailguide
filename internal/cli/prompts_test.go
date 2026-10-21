// Exercise actual terminal prompts and cancellation against the compiled CLI.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/internal/test/terminalfixture"
)

// TestInteractiveAuthoring uses a real PTY and verifies prompt results, refusal, EOF, and cancellation without partial writes.
func TestInteractiveAuthoring(t *testing.T) {
	binary := buildCLI(t)
	for _, tc := range []struct {
		name    string
		steps   []terminalfixture.Step
		code    int
		created bool
		flags   []string
	}{
		{"group", []terminalfixture.Step{{Prompt: "Group name:", Answer: "  Go  "}, {Prompt: "Group description:", Answer: "Go guidance."}, {Prompt: "When to read:", Answer: "When editing Go."}}, 0, true, nil},
		{"EOF", []terminalfixture.Step{{Prompt: "Group name:", EOF: true}}, 2, false, nil},
		{"interrupt", []terminalfixture.Step{{Prompt: "Group name:", Interrupt: true}}, 130, false, nil},
		{"typed-ctrl-c", []terminalfixture.Step{{Prompt: "Group name:", Answer: "\x03"}}, 130, false, nil},
		{"blank-retry", []terminalfixture.Step{{Prompt: "Group name:", Answer: "   "}, {Prompt: "Group name:", Answer: "Go"}, {Prompt: "Group description:", Answer: "Go guidance."}, {Prompt: "When to read:", Answer: "When editing Go."}}, 0, true, nil},
		{"blank-EOF", []terminalfixture.Step{{Prompt: "Group name:", Answer: "   "}, {Prompt: "Group name:", EOF: true}}, 2, false, nil},
		{"unattended", nil, 2, false, []string{"--non-interactive"}},
		{"json", nil, 2, false, []string{"--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			if _, stderr, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
				t.Fatal(stderr)
			}
			args := append([]string{"project", "add", "group", "techs/go"}, tc.flags...)
			result, err := terminalfixture.Run(context.Background(), binary, directory, args, tc.steps)
			if err != nil {
				t.Fatal(err, result)
			}
			if len(tc.flags) > 0 && (strings.Contains(result.Transcript, "Example group:") || strings.Contains(result.Stdout, "Example group:")) {
				t.Fatal("unattended mode showed interactive introduction", result)
			}
			if tc.name == "group" {
				for _, text := range []string{"Adding a project-only group at: techs/go", "This group belongs to this project.", "Its files are stored in .trailguide/rules/techs/go/.", "Example group:", "Name: Testing", "Description: Unit and integration testing.", "When to read: When writing or changing tests."} {
					if !strings.Contains(result.Transcript, text) {
						t.Fatalf("missing orientation %q: %s", text, result.Transcript)
					}
				}
				if strings.Index(result.Transcript, "Example group:") > strings.Index(result.Transcript, "Group name:") {
					t.Fatal("example shown after prompts", result.Transcript)
				}
			}
			if result.ExitCode != tc.code {
				t.Fatal(result)
			}
			data, err := os.ReadFile(filepath.Join(directory, ".trailguide/rules/techs/go/_group.yaml"))
			if tc.created {
				if !strings.HasSuffix(strings.ReplaceAll(result.Transcript, "\r\n", "\n"), "\n\n") {
					t.Fatal("missing blank line after prompts", result.Transcript)
				}
				if err != nil || !strings.Contains(string(data), `name: Go`) {
					t.Fatal(string(data), err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("unexpected file", err)
			}
			if _, err := os.Stat(filepath.Join(directory, ".trailguide/.trailguide.lock")); err == nil {
				t.Fatal("prompt left lock")
			}
		})
	}
}

// The prompts for a source's rule and skill selectors.
const (
	rulesPrompt  = "Rules (comma-separated selectors, or empty for none):"
	skillsPrompt = "Skills (skill IDs, recommended, or *; empty for required only):"
)

// TestInteractiveSource prompts for the repository and, unless --rules or --skills selects something, rules and skills;
// never for the ref, which defaults to the library's newest release.
func TestInteractiveSource(t *testing.T) {
	binary := buildCLI(t)
	repository := terminalfixture.Step{Prompt: "Git repository URL:", Answer: "https://github.com/acme/rules"}
	for _, test := range []struct {
		name  string
		flags []string
		steps []terminalfixture.Step
		want  map[string]any
	}{
		{"rules", []string{"--ref", "release/3"}, []terminalfixture.Step{repository, {Prompt: rulesPrompt, Answer: "techs/go/*, techs/rust/*"}, {Prompt: skillsPrompt, Answer: ""}},
			map[string]any{"repository": "https://github.com/acme/rules", "ref": "release/3", "rules": map[string]any{"include": []any{"techs/go/*", "techs/rust/*"}}}},
		{"skills", []string{"--ref", "release/3"}, []terminalfixture.Step{repository, {Prompt: rulesPrompt, Answer: ""}, {Prompt: skillsPrompt, Answer: "skills/review, skills/build-feature"}},
			map[string]any{"repository": "https://github.com/acme/rules", "ref": "release/3", "skills": map[string]any{"include": []any{"skills/build-feature", "skills/review"}}}},
		{"rule flag", []string{"--rules", "techs/go/wrap-errors", "--ref", "release/3"}, []terminalfixture.Step{repository},
			map[string]any{"repository": "https://github.com/acme/rules", "ref": "release/3", "rules": map[string]any{"include": []any{"techs/go/wrap-errors"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if _, stderr, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
				t.Fatal(stderr)
			}
			result, err := terminalfixture.Run(context.Background(), binary, directory, append([]string{"project", "add", "source", "team"}, test.flags...), test.steps)
			if err != nil || result.ExitCode != 0 {
				t.Fatal(err, result)
			}
			data, err := os.ReadFile(filepath.Join(directory, ".trailguide/config.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			var config struct{ Sources map[string]map[string]any }
			if err := yaml.Unmarshal(data, &config); err != nil || !reflect.DeepEqual(config.Sources["team"], test.want) {
				t.Fatal(string(data), err)
			}
		})
	}
}

// TestLongTerminalPaste preserves text beyond the operating system's canonical line buffer.
func TestLongTerminalPaste(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	if _, stderr, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
		t.Fatal(stderr)
	}
	description := strings.Repeat("x", 2000)
	result, err := terminalfixture.Run(context.Background(), binary, directory, []string{"project", "add", "group", "techs/go", "--name", "Go", "--when-to-read", "When editing Go."}, []terminalfixture.Step{{Prompt: "Group description:", Answer: description}})
	if err != nil || result.ExitCode != 0 {
		t.Fatal(err, result)
	}
	data, err := os.ReadFile(filepath.Join(directory, ".trailguide/rules/techs/go/_group.yaml"))
	if err != nil || !strings.Contains(string(data), description) {
		t.Fatal("long paste was lost", err)
	}
}

// TestInteractiveRuleRecoversDeadWriter checks advisory group lookup does not block authoritative writer recovery.
func TestInteractiveRuleRecoversDeadWriter(t *testing.T) {
	binary := buildCLI(t)
	for _, kind := range []string{"project", "library"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			// Project and library commands share the writer of the repository's Trail Guide directory.
			root := filepath.Join(directory, ".trailguide")
			if _, stderr, code := runCLI(t, binary, directory, kind, "init"); code != 0 {
				t.Fatal(stderr)
			}
			if _, stderr, code := runCLI(t, binary, directory, kind, "add", "group", "techs/go", "--name", "Go", "--description", "Go guidance.", "--when-to-read", "When editing Go."); code != 0 {
				t.Fatal(stderr)
			}
			process := exec.Command("/usr/bin/true")
			if err := process.Run(); err != nil {
				t.Fatal(err)
			}
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			lock := filepath.Join(root, ".trailguide-lock")
			if err := os.Mkdir(lock, 0700); err != nil {
				t.Fatal(err)
			}
			owner, _ := json.Marshal(map[string]any{"host": host, "pid": process.Process.Pid, "token": "finished-test-process"})
			if err := os.WriteFile(filepath.Join(lock, "owner.json"), owner, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{kind, "add", "rule", "techs/go/errors", "--title", "Return errors", "--impact", "HIGH", "--impact-description", "Preserve failures.", "--when-to-read", "When calling functions."}
			result, err := terminalfixture.Run(context.Background(), binary, directory, args, nil)
			if err != nil || result.ExitCode != 0 {
				t.Fatal(err, result)
			}
			if _, err := os.Stat(lock); !os.IsNotExist(err) {
				t.Fatal("lock not recovered", err)
			}
			if _, err := os.Stat(filepath.Join(root, "rules", "techs/go/errors.md")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
