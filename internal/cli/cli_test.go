// Test native CLI exit statuses, streams, and filesystem behavior through an actual executable.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/librarytree"
)

// libraryFile returns where the library repository at dir keeps the file at library path name.
func libraryFile(dir, name string) string {
	return filepath.Join(dir, filepath.FromSlash(librarytree.RepositoryPath(name)))
}

// buildCLI compiles the real entry point so tests cover process exit and standard streams.
func buildCLI(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "trailguide")
	command := exec.Command("go", "build", "-o", binary, "../../cmd/trailguide")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	return binary
}

// runCLI runs the compiled command in an isolated project without any runtime binaries on PATH, and with an empty
// home directory, so no personal skills.
func runCLI(t *testing.T, binary, dir string, args ...string) (string, string, int) {
	t.Helper()
	return runCLIWithEnvironment(t, binary, dir, append(os.Environ(), "PATH="+filepath.Join(dir, "empty-path"), "HOME="+t.TempDir()), args...)
}

// runCLIWithGit runs the compiled command with only Git on PATH and no user or system Git configuration,
// as library commands in a repository need.
func runCLIWithGit(t *testing.T, binary, dir string, args ...string) (string, string, int) {
	t.Helper()
	return runCLIWithEnvironment(t, binary, dir, gitOnlyEnvironment(t), args...)
}

// gitOnlyEnvironment returns the test's environment with a PATH that holds only Git, without Git configuration.
func gitOnlyEnvironment(t *testing.T) []string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir()
	if err := os.Symlink(git, filepath.Join(path, "git")); err != nil {
		t.Fatal(err)
	}
	environment := []string{}
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "GIT_") && !strings.HasPrefix(item, "PATH=") {
			environment = append(environment, item)
		}
	}
	return append(environment, "PATH="+path, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
}

// runCLIWithEnvironment runs the compiled command with exactly the given environment.
func runCLIWithEnvironment(t *testing.T, binary, dir string, environment []string, args ...string) (string, string, int) {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Dir = dir
	command.Env = environment
	var out, diagnostic bytes.Buffer
	command.Stdout = &out
	command.Stderr = &diagnostic
	err := command.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return out.String(), diagnostic.String(), code
}

// TestCLIProcess covers help/usage without side effects and a complete offline build/check lifecycle.
func TestCLIProcess(t *testing.T) {
	binary := buildCLI(t)
	dir := t.TempDir()
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"root-help", nil, 0}, {"removed-help-command", []string{"help"}, 2}, {"help", []string{"project", "build", "--help"}, 0}, {"version", []string{"--version"}, 0},
		{"short-version", []string{"-v"}, 0}, {"unknown", []string{"no-such-command"}, 2},
		{"unknown-flag", []string{"project", "build", "--force"}, 2}, {"positional", []string{"project", "check", "unexpected"}, 2},
		{"missing-flag-value", []string{"library", "check", "--directory"}, 2}, {"duplicate", []string{"library", "check", "--directory", "one", "--directory", "two"}, 2},
		{"missing-project", []string{"project", "build"}, 1},
		{"flag-as-value", []string{"library", "check", "--directory", "--help"}, 2},
		{"short-flag-as-value", []string{"library", "check", "--directory", "-h"}, 2},
		{"blank-value", []string{"library", "check", "--directory", " \t "}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, diagnostic, code := runCLI(t, binary, dir, test.args...)
			if code != test.code {
				t.Fatalf("exit %d, want %d: stdout=%s stderr=%s", code, test.code, out, diagnostic)
			}
			if code == 0 && (out == "" || diagnostic != "") {
				t.Fatalf("help/version streams: %q %q", out, diagnostic)
			}
			if code != 0 && (out != "" || !strings.HasPrefix(diagnostic, "Error: ")) {
				t.Fatalf("error streams: %q %q", out, diagnostic)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatal("help/error modified project", entries, err)
			}
		})
	}
	root := filepath.Join(dir, ".trailguide")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(`{"schemaVersion":2,"sources":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code := runCLI(t, binary, dir, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code := runCLI(t, binary, dir, "project", "check", "--json")
	var response struct {
		Value projectCheckResult `json:"value"`
	}
	if code != 1 || diagnostic != "" || json.Unmarshal([]byte(out), &response) != nil || response.Value.Status != "out-of-date" || len(response.Value.Problems) == 0 {
		t.Fatalf("stale check: %d %s %s", code, out, diagnostic)
	}
	if _, err := os.Stat(filepath.Join(root, "_managed/generated")); !os.IsNotExist(err) {
		t.Fatal("check created output", err)
	}
	out, diagnostic, code = runCLI(t, binary, dir, "project", "build", "--json")
	if code != 0 || diagnostic != "" || !json.Valid([]byte(out)) {
		t.Fatalf("build: %d %s %s", code, out, diagnostic)
	}
	out, diagnostic, code = runCLI(t, binary, dir, "project", "check", "--json")
	if code != 0 || diagnostic != "" || json.Unmarshal([]byte(out), &response) != nil || response.Value.Status != "up-to-date" || len(response.Value.Problems) != 0 {
		t.Fatalf("clean check: %d %s %s", code, out, diagnostic)
	}
	out, diagnostic, code = runCLI(t, binary, dir, "project", "sync", "--json")
	if code != 0 || diagnostic != "" || !json.Valid([]byte(out)) {
		t.Fatalf("empty-source sync: %d %s %s", code, out, diagnostic)
	}
}
