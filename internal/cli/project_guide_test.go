// Exercise the agent-facing project guide through the native executable.

package cli

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// TestInitCreatesAgentGuide verifies init exposes the main agent entry point and a read-only freshness check.
func TestInitCreatesAgentGuide(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	out, diagnostic, code := runCLI(t, binary, directory, "project", "init")
	if code != 0 {
		t.Fatal(out, diagnostic)
	}
	guidePath := filepath.Join(directory, ".trailguide", "README.md")
	guide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Add a group", "Add a rule", "Add a source", "Install skills", "trailguide project check"} {
		if !strings.Contains(string(guide), text) {
			t.Fatalf("guide missing %q", text)
		}
	}
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "build"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code = runCLI(t, binary, directory, "project", "check")
	if code != 0 || !strings.Contains(out, "up to date") {
		t.Fatal(code, out, diagnostic)
	}
	if err := os.WriteFile(guidePath, append(guide, []byte("\nMy custom text.\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code = runCLI(t, binary, directory, "project", "check", "--json")
	if code != 1 || !strings.Contains(out, `"ok": false`) || diagnostic != "" {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code = runCLI(t, binary, directory, "project", "init")
	if code != 1 || !strings.Contains(diagnostic, "README.md") {
		t.Fatal(code, out, diagnostic)
	}
	after, err := os.ReadFile(guidePath)
	if err != nil || !strings.HasSuffix(string(after), "My custom text.\n") {
		t.Fatal("init overwrote custom text", err)
	}
}

// TestProjectGuideExamples executes every shell example from the generated README against the real CLI and an isolated Git publisher.
func TestProjectGuideExamples(t *testing.T) {
	binary := buildCLI(t)
	rule := "---\ntitle: Preserve errors\nimpact: HIGH\nimpactDescription: Keep failures visible.\nwhenToRead: When calling fallible functions.\n---\nReturn errors to the caller.\n"
	for _, projectName := range []string{"project", "project 'quoted'"} {
		t.Run(projectName, func(t *testing.T) {
			ctx := context.Background()
			fixture, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
				"config.yaml":                               []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`),
				"techs/go/_group.yaml":                      []byte(`{"name":"Go","description":"Shared Go guidance.","whenToRead":"When writing Go."}`),
				"techs/go/shared.md":                        []byte(rule),
				".trailguide/skills/build-feature/SKILL.md": []byte("---\nname: build-feature\ndescription: Build a feature.\n---\n# Build a feature\n"),
			}))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := fixture.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := fixture.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/shared: 1.0.0\nchanges:\n  techs/go/shared: {change: new, summaries: [Add the rule.]}\n"); err != nil {
				t.Fatal(err)
			}
			updated := false
			directory := filepath.Join(t.TempDir(), projectName)
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			readme := "# My project\nKeep this authored README unchanged.\n"
			if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte(readme), 0600); err != nil {
				t.Fatal(err)
			}
			out, diagnostic, code := runCLI(t, binary, directory, "project", "init")
			if code != 0 {
				t.Fatal(out, diagnostic)
			}
			guide, err := os.ReadFile(filepath.Join(directory, ".trailguide", "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			blocks := regexp.MustCompile("(?ms)^```sh\\n(.*?)^```$").FindAllSubmatch(guide, -1)
			if len(blocks) == 0 {
				t.Fatal("no executable README examples")
			}
			for index, block := range blocks {
				script := strings.ReplaceAll(string(block[1]), "trailguide ", gitfixture.Quote(binary)+" ")
				script = strings.ReplaceAll(script, "https://github.com/example/engineering-rules", fixture.Repository)
				// The library publishes a newer version before the update example, so the example has something to apply.
				if strings.Contains(script, "project update") && !updated {
					if _, err := fixture.Commit(ctx, fixture.Worktree(), "Clarify shared", gitfixture.Library(map[string][]byte{"techs/go/shared.md": []byte(rule + "\nWrap them with context.\n")})); err != nil {
						t.Fatal(err)
					}
					if err := fixture.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/shared: 1.1.0\nchanges:\n  techs/go/shared: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\n"); err != nil {
						t.Fatal(err)
					}
					updated = true
				}
				command := exec.Command("/bin/sh", "-eu", "-c", script)
				command.Dir = directory
				command.Env = fixture.Environment
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("README example %d failed: %v\n%s", index+1, err, output)
				}
			}
			authored, err := os.ReadFile(filepath.Join(directory, "README.md"))
			if err != nil || string(authored) != readme {
				t.Fatal("changed authored README", err)
			}
			for _, file := range []string{"rules/README.md", "rules/techs/go/README.md"} {
				content, err := os.ReadFile(filepath.Join(directory, ".trailguide", file))
				if err != nil || !strings.Contains(string(content), "/README.md)") {
					t.Fatalf("%s does not link to the managed guide: %v", file, err)
				}
			}
			for _, path := range []string{"rules/techs/go/return-errors.md", "vendor/team/rules/techs/go/shared.md", "generated/RULES.md", "generated/groups/techs/go.md", "generated/SKILLS.md", "../.claude/skills/build-feature/SKILL.md", "../.agents/skills/build-feature/SKILL.md"} {
				if _, err := os.Stat(filepath.Join(directory, ".trailguide", path)); err != nil {
					t.Fatalf("README did not produce %s: %v", path, err)
				}
			}
			if shared, err := os.ReadFile(filepath.Join(directory, ".trailguide", "vendor/team/rules/techs/go/shared.md")); err != nil || !updated || !strings.Contains(string(shared), "Wrap them with context.") {
				t.Fatalf("the update example didn't apply the newer version: %v", err)
			}
		})
	}
}

// TestCheckVerifiesGuideAndGeneratedOutput exercises both independent checks without allowing writes.
func TestCheckVerifiesGuideAndGeneratedOutput(t *testing.T) {
	binary := buildCLI(t)
	for _, guideState := range []string{"current", "missing", "edited"} {
		for _, stale := range []bool{false, true} {
			name := guideState
			if stale {
				name += "/stale-output"
			}
			t.Run(name, func(t *testing.T) {
				directory := t.TempDir()
				for _, command := range []string{"init", "build"} {
					if out, diagnostic, code := runCLI(t, binary, directory, "project", command); code != 0 {
						t.Fatal(code, out, diagnostic)
					}
				}
				root := filepath.Join(directory, ".trailguide")
				guideName := "README.md"
				guidePath := filepath.Join(root, guideName)
				if guideState == "missing" {
					if err := os.Remove(guidePath); err != nil {
						t.Fatal(err)
					}
				} else if guideState == "edited" {
					if err := os.WriteFile(guidePath, []byte("Manually edited guide."), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if stale {
					if err := os.WriteFile(filepath.Join(root, "generated", "RULES.md"), []byte("Stale output."), 0600); err != nil {
						t.Fatal(err)
					}
				}
				before := projectFileContents(t, directory)
				for _, jsonMode := range []bool{false, true} {
					args := []string{"project", "check"}
					if jsonMode {
						args = append(args, "--json")
					}
					out, diagnostic, code := runCLI(t, binary, directory, args...)
					wantCode := 0
					if stale || guideState != "current" {
						wantCode = 1
					}
					if code != wantCode {
						t.Fatal(code, out, diagnostic)
					}
					if jsonMode {
						var result struct {
							OK    bool
							Value projectCheckResult
							Error *responseError
						}
						if err := json.Unmarshal([]byte(out), &result); err != nil {
							t.Fatal(err, out)
						}
						if diagnostic != "" || result.OK != (wantCode == 0) || (result.Error != nil) != (wantCode != 0) {
							t.Fatal(out, diagnostic)
						}
						kinds := map[string]bool{}
						for _, problem := range result.Value.Problems {
							kinds[problem.Kind] = true
						}
						if kinds["outdated-readme"] != (guideState != "current") || kinds["stale-contents"] != stale {
							t.Fatal(out)
						}
					} else {
						if !strings.Contains(out, "Status:") {
							t.Fatal(out, diagnostic)
						}
						if wantCode == 0 && !strings.Contains(out, "the Trail Guide guide are current") {
							t.Fatal(out)
						}
						if guideState != "current" && !strings.Contains(out, guideName) {
							t.Fatal(out, diagnostic)
						}
					}
					if after := projectFileContents(t, directory); !reflect.DeepEqual(before, after) {
						t.Fatal("check changed project files")
					}
				}
			})
		}
	}
	if out, diagnostic, code := runCLI(t, binary, t.TempDir(), "project", "init", "--check"); code != 2 || !strings.Contains(diagnostic, "unknown flag") {
		t.Fatal(code, out, diagnostic)
	}
}

// projectFileContents captures all file bytes to detect writes or extra files during read-only CLI checks.
func projectFileContents(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestCheckDoesNotInitialize leaves a missing project absent when the actual command fails.
func TestCheckDoesNotInitialize(t *testing.T) {
	directory := t.TempDir()
	out, diagnostic, code := runCLI(t, buildCLI(t), directory, "project", "check")
	if code != 1 {
		t.Fatal(code, out, diagnostic)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
}
