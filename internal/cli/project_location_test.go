// Keep project storage fixed and reject removed configuration flags before any writes.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProjectCommandsRejectCustomConfiguration(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	custom := filepath.Join(directory, "custom.json")
	if err := os.WriteFile(custom, []byte(`{"schemaVersion":2,"sources":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	before := projectFileContents(t, directory)
	for _, command := range [][]string{
		{"project", "init"}, {"project", "build"}, {"project", "sync"}, {"project", "check"},
		{"project", "add", "source", "team"},
		{"project", "add", "group", "techs/go"},
		{"project", "add", "rule", "techs/go/errors"},
	} {
		t.Run(strings.Join(command, "/"), func(t *testing.T) {
			for _, flags := range [][]string{{"--config", custom}, {"--config=" + custom}, {"--config", custom, "--help"}} {
				for _, jsonMode := range []bool{false, true} {
					args := append(append([]string{}, command...), flags...)
					if jsonMode {
						args = append(args, "--json")
					}
					out, diagnostic, code := runCLI(t, binary, directory, args...)
					if code != 2 {
						t.Fatal(args, code, out, diagnostic)
					}
					if jsonMode {
						var result struct {
							OK    bool
							Error responseError
						}
						if err := json.Unmarshal([]byte(out), &result); err != nil || result.OK || diagnostic != "" || !strings.Contains(result.Error.Message, "unknown flag: --config") {
							t.Fatal(err, out, diagnostic)
						}
					} else if out != "" || !strings.Contains(diagnostic, "unknown flag: --config") {
						t.Fatal(out, diagnostic)
					}
					if !reflect.DeepEqual(before, projectFileContents(t, directory)) {
						t.Fatal("removed flag wrote files")
					}
				}
			}
			out, diagnostic, code := runCLI(t, binary, directory, append(command, "--help")...)
			if code != 0 || diagnostic != "" || strings.Contains(out, "--config") {
				t.Fatal(code, out, diagnostic)
			}
		})
	}
}

func TestProjectUsesOnlyRootTrailGuideDirectory(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	// An unrelated configuration file must never become project state implicitly.
	custom := filepath.Join(directory, "config.yaml")
	original := []byte(`{"schemaVersion":2,"sources":{}}`)
	if err := os.WriteFile(custom, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"build", "sync", "check"} {
		if out, diagnostic, code := runCLI(t, binary, directory, "project", command); code != 1 {
			t.Fatal(command, code, out, diagnostic)
		}
	}
	for _, command := range []string{"init", "build", "check"} {
		if out, diagnostic, code := runCLI(t, binary, directory, "project", command); code != 0 {
			t.Fatal(command, code, out, diagnostic)
		}
	}
	for _, name := range []string{"config.yaml", "README.md", "_managed/generated/RULES.md"} {
		if _, err := os.Stat(filepath.Join(directory, ".trailguide", name)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(custom)
	if err != nil || string(data) != string(original) {
		t.Fatal("changed unrelated config", err)
	}
}

func TestProjectRootFailuresAreConsistent(t *testing.T) {
	binary := buildCLI(t)
	commands := [][]string{{"build"}, {"sync"}, {"check"}, {"add", "group", "techs/go"}, {"add", "rule", "techs/go/errors"}, {"add", "source", "team"}}
	for _, state := range []string{"missing-directory", "missing-config", "symlink"} {
		t.Run(state, func(t *testing.T) {
			directory, target := t.TempDir(), t.TempDir()
			if out, diagnostic, code := runCLI(t, binary, target, "project", "init"); code != 0 {
				t.Fatal(code, out, diagnostic)
			}
			want := "run trailguide project init"
			switch state {
			case "missing-config":
				if err := os.Mkdir(filepath.Join(directory, ".trailguide"), 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(filepath.Join(target, ".trailguide"), filepath.Join(directory, ".trailguide")); err != nil {
					t.Fatal(err)
				}
				want = "without symlinks"
			}
			before := projectFileContents(t, target)
			for _, command := range commands {
				args := append([]string{"project"}, command...)
				args = append(args, "--json")
				out, diagnostic, code := runCLI(t, binary, directory, args...)
				var result response
				if json.Unmarshal([]byte(out), &result) != nil || code != 1 || result.OK || result.Error == nil || diagnostic != "" || !strings.Contains(result.Error.Message, want) {
					t.Errorf("%v: code=%d stdout=%s stderr=%s", args, code, out, diagnostic)
				}
			}
			if !reflect.DeepEqual(before, projectFileContents(t, target)) {
				t.Fatal("changed symlink target")
			}
		})
	}
}
