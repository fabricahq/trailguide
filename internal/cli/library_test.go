// Exercise library commands through the compiled native CLI with no runtime executables on PATH.

package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestLibraryCLIProcess checks terms preservation, complete authoring, and actionable unfinished-draft failure.
func TestLibraryCLIProcess(t *testing.T) {
	binary := buildCLI(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "terms.txt"), []byte("Terms\r\n"), 0600)
	os.WriteFile(filepath.Join(dir, "body.md"), []byte("Return failures."), 0600)
	commands := [][]string{
		{"library", "init", "--directory", "library", "--spdx", "MIT", "--license-file", "terms.txt"},
		{"library", "add", "group", "techs/go", "--directory", "library", "--name", "Go", "--description", "Go guidance.", "--when-to-read", "When editing Go."},
		{"library", "add", "rule", "techs/go/errors", "--directory", "library", "--title", "Return errors", "--impact", "HIGH", "--impact-description", "Preserve failures.", "--when-to-read", "When calling functions.", "--body-file", "body.md"},
		{"library", "check", "--directory", "library"},
	}
	for _, args := range commands {
		out, diagnostic, code := runCLI(t, binary, dir, args...)
		if code != 0 || out == "" || diagnostic != "" {
			t.Fatal(args, code, out, diagnostic)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "library/LICENSE.md"))
	if string(data) != "Terms\r\n" {
		t.Fatal("terms changed")
	}
	draft := append([]string{}, commands[2][:len(commands[2])-2]...)
	draft[3] = "techs/go/draft"
	out, diagnostic, code := runCLI(t, binary, dir, draft...)
	if code != 0 {
		t.Fatal(draft, code, out, diagnostic)
	}
	out, diagnostic, code = runCLI(t, binary, dir, "library", "check", "--directory", "library")
	if code != 1 || out != "" || !strings.Contains(diagnostic, "trailguide:draft") {
		t.Fatal(code, out, diagnostic)
	}
}

// TestLibraryGuideExamples executes the shipped README examples and preserves publisher customization on repeat init.
func TestLibraryGuideExamples(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "library", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	guidePath := filepath.Join(directory, "README.md")
	guide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?ms)^```sh\\n(.*?)^```$").FindAllSubmatch(guide, -1)
	if len(blocks) == 0 {
		t.Fatal("no command examples in library README")
	}
	for index, block := range blocks {
		script := strings.ReplaceAll(string(block[1]), "trailguide ", "'"+strings.ReplaceAll(binary, "'", "'\"'\"'")+"' ")
		command := exec.Command("/bin/sh", "-eu", "-c", script)
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("README example %d failed: %v\n%s", index+1, err, output)
		}
	}
	for _, path := range []string{"techs/go/_group.yaml", "techs/go/README.md", "techs/go/return-errors.md"} {
		if _, err := os.Stat(libraryFile(directory, path)); err != nil {
			t.Fatal(err)
		}
	}
	customized := append(guide, []byte("\nPublisher-specific release instructions.\n")...)
	if err := os.WriteFile(guidePath, customized, 0600); err != nil {
		t.Fatal(err)
	}
	// Rerunning init in a library that has groups suggests validating it rather than adding a first group.
	out, diagnostic, code := runCLI(t, binary, directory, "library", "init")
	if code != 0 || !strings.Contains(out, "No files changed.\n") || !strings.Contains(out, "Next: Validate the library:\n  trailguide library check\n") || strings.Contains(out, "Add a group and rule") {
		t.Fatal(code, out, diagnostic)
	}
	after, err := os.ReadFile(guidePath)
	if err != nil || string(after) != string(customized) {
		t.Fatal("repeat init changed publisher README", err)
	}
}

// TestLibraryInit_WritesACheckWorkflowThatInstallsTheMinimum, which a release build records and a development build
// leaves out, so its workflow installs the latest release.
func TestLibraryInit_WritesACheckWorkflowThatInstallsTheMinimum(t *testing.T) {
	for version, minimum := range map[string]string{"1.2.3": "\nminTrailguideVersion: 1.2.3\n", "": ""} {
		directory := t.TempDir()
		var out, diagnostic strings.Builder
		if code := Run(context.Background(), []string{"library", "init"}, Streams{Out: &out, Err: &diagnostic}, Options{Directory: directory, Version: version}); code != 0 {
			t.Fatal(code, out.String(), diagnostic.String())
		}
		workflow, err := os.ReadFile(filepath.Join(directory, ".github", "workflows", "trailguide.yml"))
		if err != nil || !strings.Contains(string(workflow), "\n          version=$(sed -nE \"s/^minTrailguideVersion:") {
			t.Fatalf("%q: %s %v", version, workflow, err)
		}
		config, err := os.ReadFile(filepath.Join(directory, ".trailguide", "config.yaml"))
		if err != nil || strings.Contains(string(config), "minTrailguideVersion") != (minimum != "") || minimum != "" && !strings.Contains(string(config), minimum) {
			t.Fatalf("%q: config.yaml:\n%s %v", version, config, err)
		}
	}
}
