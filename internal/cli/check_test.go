// Exercise status reports and their suggested repairs through the real executable.

package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestCheckReportsCurrentProblems verifies all mismatch kinds and executes the reported repair commands.
func TestCheckReportsCurrentProblems(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	for _, command := range []string{"init", "build"} {
		if out, diagnostic, code := runCLI(t, binary, directory, "project", command); code != 0 {
			t.Fatal(code, out, diagnostic)
		}
	}
	for _, name := range []string{".trailguide/README.md", ".trailguide/_managed/generated/groups/README.md", "AGENTS.md"} {
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"_managed/generated/RULES.md", "_managed/generated/unexpected.md"} {
		if err := os.WriteFile(filepath.Join(directory, ".trailguide", name), []byte("Manual content."), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := projectFileContents(t, directory)
	out, diagnostic, code := runCLI(t, binary, directory, "project", "check", "--json")
	var result struct {
		OK    bool
		Value projectCheckResult
		Error responseError
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err, out)
	}
	if code != 1 || diagnostic != "" || result.OK || result.Error.Kind != "out-of-date" || result.Value.Status != "out-of-date" {
		t.Fatal(code, out, diagnostic)
	}
	kinds := map[string]string{}
	repairs := map[string]bool{}
	for _, problem := range result.Value.Problems {
		kinds[problem.Kind] = problem.Path
		if problem.Message == "" {
			t.Fatal(problem)
		}
		repairs[repairCommand(t, problem)] = true
	}
	want := map[string]string{
		"missing-file":    "_managed/generated/groups/README.md",
		"stale-contents":  "_managed/generated/RULES.md",
		"unexpected-file": "_managed/generated/unexpected.md",
		"outdated-readme": "README.md",
		// Paths are relative to the Trail Guide directory.
		"outdated-agents-section": "../AGENTS.md",
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatal(kinds, want)
	}
	var envelope struct{ Value map[string]json.RawMessage }
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"added", "changed", "removed"} {
		if _, exists := envelope.Value[key]; exists {
			t.Fatalf("check implies completed changes: %s", out)
		}
	}
	human, diagnostic, code := runCLI(t, binary, directory, "project", "check")
	if code != 1 || diagnostic != "" || !strings.Contains(human, "No files were changed.") {
		t.Fatal(code, human, diagnostic)
	}
	for _, problem := range result.Value.Problems {
		if !strings.Contains(human, problem.Message) || !strings.Contains(human, repairCommand(t, problem)) {
			t.Fatal("human and JSON reports disagree", human, problem)
		}
	}
	if !reflect.DeepEqual(before, projectFileContents(t, directory)) {
		t.Fatal("check wrote files")
	}
	for repair := range repairs {
		script := strings.Replace(repair, "trailguide", "'"+strings.ReplaceAll(binary, "'", "'\"'\"'")+"'", 1)
		command := exec.Command("/bin/sh", "-eu", "-c", script)
		command.Dir = directory
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatal(repair, err, string(output))
		}
		if !strings.Contains(string(output), "Trail Guide section added to a new file: AGENTS.md\n") {
			t.Fatal("build didn't report the AGENTS.md it created", string(output))
		}
	}
	out, diagnostic, code = runCLI(t, binary, directory, "project", "check", "--json")
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if code != 0 || diagnostic != "" || !result.OK || result.Value.Status != "up-to-date" || len(result.Value.Problems) != 0 {
		t.Fatal(code, out, diagnostic)
	}
}

// TestGuideRepairFromSubdirectory runs the reported repair where the user ran check.
func TestGuideRepairFromSubdirectory(t *testing.T) {
	binary := buildCLI(t)
	for _, state := range []string{"missing", "outdated", "edited"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			locationGit(t, root, "init", "--quiet", "--template=")
			child := filepath.Join(root, "src", "nested")
			if err := os.MkdirAll(child, 0700); err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"init", "build"} {
				if out, diagnostic, code := runCLI(t, binary, root, "project", action); code != 0 {
					t.Fatal(code, out, diagnostic)
				}
			}
			guide := filepath.Join(root, ".trailguide", "README.md")
			body := []byte("# Earlier guide\n")
			data := []byte(fmt.Sprintf("<!-- trailguide:project-guide sha256:%x -->\n%s", sha256.Sum256(body), body))
			if state == "edited" {
				data = append(data, []byte("Manual notes.\n")...)
			}
			if err := os.WriteFile(guide, data, 0600); err != nil {
				t.Fatal(err)
			}
			if state == "missing" {
				if err := os.Remove(guide); err != nil {
					t.Fatal(err)
				}
			}
			out, diagnostic, code := runCLI(t, binary, child, "project", "check", "--json")
			var result struct{ Value projectCheckResult }
			if code != 1 || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || len(result.Value.Problems) != 1 {
				t.Fatal(code, out, diagnostic)
			}
			problem := result.Value.Problems[0]
			if problem.Kind != "outdated-readme" {
				t.Fatal(problem)
			}
			human, diagnostic, code := runCLI(t, binary, child, "project", "check")
			if code != 1 || diagnostic != "" || !strings.Contains(human, repairCommand(t, problem)) {
				t.Fatal(code, human, diagnostic)
			}
			script := strings.Replace(repairCommand(t, problem), "trailguide", shellDirectory(binary), 1)
			repair := func() ([]byte, error) {
				cmd := exec.Command("/bin/sh", "-eu", "-c", script)
				cmd.Dir = child
				return cmd.CombinedOutput()
			}
			if state == "edited" {
				before := projectFileContents(t, root)
				output, err := repair()
				if err == nil || !strings.Contains(string(output), "manually edited project guide") || !strings.Contains(string(output), "rerun this command") {
					t.Fatal(err, string(output))
				}
				if !reflect.DeepEqual(before, projectFileContents(t, root)) {
					t.Fatal("repair overwrote manual edits")
				}
				if err := os.Rename(guide, guide+".saved"); err != nil {
					t.Fatal(err)
				}
			}
			if output, err := repair(); err != nil {
				t.Fatal(repairCommand(t, problem), err, string(output))
			}
			if out, diagnostic, code := runCLI(t, binary, child, "project", "check"); code != 0 {
				t.Fatal(code, out, diagnostic)
			}
		})
	}
}

// repairCommand returns the one command of a check problem's one next step, failing the test for any other shape.
func repairCommand(t *testing.T, problem checkProblem) string {
	t.Helper()
	if len(problem.NextSteps) != 1 || problem.NextSteps[0].Instruction == "" || len(problem.NextSteps[0].Commands) != 1 {
		t.Fatalf("problem %+v: want one next step with one command", problem)
	}
	return problem.NextSteps[0].Commands[0]
}
