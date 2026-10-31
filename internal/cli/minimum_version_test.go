// Verify the command contract when a project's minTrailguideVersion is newer than the running Trail Guide.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/internal/test/terminalfixture"
)

// TestProjectCommands_RefuseAnOlderTrailguide records the running release at init, and makes an older release fail
// with upgrade-required, as one JSON response or a message on standard error.
func TestProjectCommands_RefuseAnOlderTrailguide(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	run := func(version string, args ...string) (string, string, int) {
		var out, diagnostic bytes.Buffer
		code := Run(ctx, args, Streams{Out: &out, Err: &diagnostic}, Options{Directory: directory, Home: t.TempDir(), Version: version})
		return out.String(), diagnostic.String(), code
	}
	if out, diagnostic, code := run("0.9.0", "project", "init", "--non-interactive"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	config, err := os.ReadFile(filepath.Join(directory, ".trailguide", "config.yaml"))
	if err != nil || !strings.Contains(string(config), "\nminTrailguideVersion: 0.9.0\n") {
		t.Fatalf("config.yaml:\n%s %v", config, err)
	}
	out, diagnostic, code := run("0.8.1", "project", "build", "--json")
	var result struct {
		OK    bool
		Error struct{ Kind, Code, Message string }
	}
	if code != 1 || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil || result.OK || result.Error.Kind != "operation" || result.Error.Code != "upgrade-required" || !strings.Contains(result.Error.Message, "upgrade trailguide to 0.9.0 or newer") {
		t.Fatal(code, out, diagnostic)
	}
	if out, diagnostic, code := run("0.8.1", "project", "check"); code != 1 || out != "" || !strings.Contains(diagnostic, "minTrailguideVersion is 0.9.0, but this is trailguide 0.8.1") {
		t.Fatal(code, out, diagnostic)
	}
	if out, diagnostic, code := run("0.9.0", "project", "build", "--json"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
}

// TestProjectUpdate_PreviewsAMinimumRaise shows a raise of minTrailguideVersion as an update to confirm, in human and
// JSON output, writes it only with --yes, and leaves an up-to-date minimum without an update to apply.
func TestProjectUpdate_PreviewsAMinimumRaise(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	run := func(version string, args ...string) (string, string, int) {
		var out, diagnostic bytes.Buffer
		code := Run(ctx, args, Streams{Out: &out, Err: &diagnostic}, Options{Directory: directory, Home: t.TempDir(), Version: version})
		return out.String(), diagnostic.String(), code
	}
	configPath := filepath.Join(directory, ".trailguide", "config.yaml")
	readConfig := func() string {
		t.Helper()
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if out, diagnostic, code := run("0.9.0", "project", "init", "--non-interactive"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	line := "Sets minTrailguideVersion to 0.10.0 in .trailguide/config.yaml, so older releases\nof trailguide refuse this project.\n"
	if out, diagnostic, code := run("0.10.0", "project", "update"); code != 0 || !strings.HasPrefix(out, line) || !strings.Contains(out, "This is a preview; no files were written.") || !strings.Contains(readConfig(), "minTrailguideVersion: 0.9.0") {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code := run("0.10.0", "project", "update", "--json")
	var preview struct {
		Value struct {
			Applied                  bool
			SetsMinTrailguideVersion string
		}
	}
	if code != 0 || json.Unmarshal([]byte(out), &preview) != nil || preview.Value.Applied || preview.Value.SetsMinTrailguideVersion != "0.10.0" {
		t.Fatal(code, out, diagnostic)
	}
	if out, diagnostic, code := run("0.10.0", "project", "update", "--yes"); code != 0 || !strings.HasPrefix(out, strings.Replace(line, "Sets", "Set", 1)) || !strings.Contains(out, "config.yaml") || !strings.Contains(readConfig(), "minTrailguideVersion: 0.10.0") {
		t.Fatal(code, out, diagnostic)
	}
	if out, diagnostic, code := run("0.10.0", "project", "update"); code != 0 || strings.Contains(out, "minTrailguideVersion") || !strings.Contains(out, "No rule updates are available.") {
		t.Fatal(code, out, diagnostic)
	}
}

// TestProjectUpdate_WarnsAboutAPinnedWorkflowBeforeConfirming, so people in a terminal learn that the raise fails
// the library's check workflow before they accept it, and declining writes nothing.
func TestProjectUpdate_WarnsAboutAPinnedWorkflowBeforeConfirming(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "trailguide")
	if output, err := exec.Command("go", "build", "-ldflags", "-X main.version=1.2.3", "-o", binary, "../../cmd/trailguide").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "init", "--non-interactive"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	files := map[string]string{
		".trailguide/config.yaml":          "schemaVersion: 2\nminTrailguideVersion: 1.0.0\nsources: {}\n",
		".github/workflows/trailguide.yml": "run: |\n          version=1.0.0\n",
	}
	for name, text := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := terminalfixture.Run(context.Background(), binary, directory, []string{"project", "update"}, []terminalfixture.Step{{Prompt: "Apply the update? [yes/no]:", Answer: "no"}})
	warning := "Warning: .github/workflows/trailguide.yml installs trailguide 1.0.0"
	transcript := strings.Join(strings.Fields(result.Transcript), " ")
	at := strings.Index(transcript, warning)
	if err != nil || result.ExitCode != 0 || at < 0 || at > strings.Index(transcript, "Apply the update?") {
		t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
	}
	if data, err := os.ReadFile(filepath.Join(directory, ".trailguide", "config.yaml")); err != nil || string(data) != files[".trailguide/config.yaml"] {
		t.Fatalf("declining changed config.yaml: %s %v", data, err)
	}
}

// TestUpdateIntroduction_WarnsAboutARaiseBeforeAsking, so a terminal shows that the library's pinned check workflow
// fails before people confirm the raise.
func TestUpdateIntroduction_WarnsAboutARaiseBeforeAsking(t *testing.T) {
	warning := ".github/workflows/trailguide.yml installs trailguide 0.9.0, which refuses this repository once minTrailguideVersion is 0.10.0, so its next check fails; change its version=0.9.0 line to version=0.10.0."
	intro := updateIntroduction(project.UpdateResult{SetsMinimumVersion: "0.10.0", RaiseWarnings: []string{warning}})
	if want := "Sets minTrailguideVersion to 0.10.0 in .trailguide/config.yaml, so older releases\nof trailguide refuse this project.\n\nWarning: " + warning + "\n"; intro != want {
		t.Fatalf("got:\n%s\nwant:\n%s", intro, want)
	}
}
