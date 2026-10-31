// Run the check workflow's install step against a fake GitHub CLI and real checksum and archive tools, as GitHub
// Actions runs it.

package library

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeGitHubCLI stands in for gh. release view prints FAKE_LATEST, as the --jq filter would. release download
// copies the files of FAKE_ASSETS/<tag> matching each --pattern, as a glob, and fails when none match. attestation
// verify passes only for Trail Guide's release workflow on main and FAKE_ATTESTATION=pass, whatever SHA256SUMS holds,
// as for an older release's legitimately attested file. Each call is logged to FAKE_LOG.
const fakeGitHubCLI = `echo "gh $*" >> "$FAKE_LOG"
case "$1 $2" in
"release view") printf '%s\n' "$FAKE_LATEST" ;;
"release download")
  release="$FAKE_ASSETS/$3"; shift 3; found=
  while [ $# -gt 0 ]; do
    if [ "$1" = --pattern ]; then
      for file in "$release"/$2; do [ -f "$file" ] && cp "$file" . && found=1; done
      shift
    fi
    shift
  done
  [ -n "$found" ] ;;
"attestation verify")
  case "$*" in *"--repo fabricahq/trailguide --signer-workflow fabricahq/trailguide/.github/workflows/release-planner.yml --source-ref refs/heads/main"*) ;; *) exit 1 ;; esac
  [ "$FAKE_ATTESTATION" = pass ] ;;
*) exit 2 ;;
esac`

// installScript returns the generated workflow's install step script, without its YAML indentation.
func installScript(t *testing.T, workflow string) string {
	t.Helper()
	block := regexp.MustCompile(`(?s)\n        run: \|\n(.*?)\n      - `).FindStringSubmatch(workflow)
	if block == nil {
		t.Fatalf("no install script in:\n%s", workflow)
	}
	return regexp.MustCompile(`(?m)^ {10}`).ReplaceAllString(block[1], "") + "\n"
}

// releaseArchive returns a gzipped tar archive holding a trailguide executable that prints version.
func releaseArchive(t *testing.T, version string) []byte {
	t.Helper()
	var data bytes.Buffer
	compressed := gzip.NewWriter(&data)
	archive := tar.NewWriter(compressed)
	executable := []byte("#!/bin/sh\necho " + version + "\n")
	if err := archive.WriteHeader(&tar.Header{Name: "trailguide", Mode: 0755, Size: int64(len(executable))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(executable); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

// publish writes the assets of the GitHub Release page tagged tag under assets: files by name, and SHA256SUMS
// listing sums, a map from archive name to archive bytes.
func publish(t *testing.T, assets, tag string, files, sums map[string][]byte) {
	t.Helper()
	directory := filepath.Join(assets, tag)
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	var checksums strings.Builder
	for name, data := range sums {
		sum := sha256.Sum256(data)
		checksums.WriteString(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	}
	files["SHA256SUMS"] = []byte(checksums.String())
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCheckWorkflow_InstallsOnlyTheVerifiedRequestedVersion runs the install step with bash -e, as GitHub Actions
// does: it adds Trail Guide to PATH only when the attested checksums list exactly the requested version's archive
// with its bytes, so a release whose assets were replaced with an older release's fails instead of installing it,
// and it prints the installed version to the log and the job summary.
func TestCheckWorkflow_InstallsOnlyTheVerifiedRequestedVersion(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal("bash, which GitHub Actions uses, is required to run the install step:", err)
	}
	tools := t.TempDir()
	if err := os.WriteFile(filepath.Join(tools, "gh"), []byte("#!/bin/sh\n"+fakeGitHubCLI+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// GNU tar runs gzip to read the archive.
	for _, name := range []string{"sha256sum", "tar", "gzip", "grep", "mkdir", "cp", "yq"} {
		real, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("%s, which GitHub's Ubuntu runners have, is required to run the install step: %v", name, err)
		}
		if err := os.Symlink(real, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	// The Python yq, unlike the Go yq that GitHub's runners have, runs jq.
	if real, err := exec.LookPath("jq"); err == nil {
		if err := os.Symlink(real, filepath.Join(tools, "jq")); err != nil {
			t.Fatal(err)
		}
	}
	current, older := releaseArchive(t, "0.2.0"), releaseArchive(t, "0.1.0")
	currentName, olderName := "trailguide_0.2.0_linux_amd64.tar.gz", "trailguide_0.1.0_linux_amd64.tar.gz"
	for _, test := range []struct {
		// minimum is the configuration's minTrailguideVersion line, or empty for none, or the whole configuration
		// when it's a flow mapping.
		name, minimum, latest, attestation string
		// assets publishes the GitHub Release page v0.2.0.
		assets func(t *testing.T, assets string)
		// installed is the version the step installs, or "" when it must fail.
		installed string
	}{
		{"verified", "minTrailguideVersion: 0.2.0", "", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, "0.2.0"},
		{"quoted minimum with a comment", `minTrailguideVersion: "0.2.0" # raised by update`, "", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, "0.2.0"},
		// The latest release differs from the minimum, so a case that misreads the minimum fails to download it.
		{"quoted key", `"minTrailguideVersion": '0.2.0'`, "0.3.0", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, "0.2.0"},
		{"flow mapping", "{schemaVersion: 2, minTrailguideVersion: 0.2.0, sources: {}}", "0.3.0", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, "0.2.0"},
		{"quoted key in a large configuration", `"minTrailguideVersion": 0.2.0` + strings.Repeat("\n# filler that outlasts a pipe buffer", 4096), "0.3.0", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, "0.2.0"},
		{"a comment naming the field installs the latest release", "# minTrailguideVersion is unset", "0.2.0", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, "0.2.0"},
		{"attestation fails", "minTrailguideVersion: 0.2.0", "", "fail", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, ""},
		{"archive differs from its checksum", "minTrailguideVersion: 0.2.0", "", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: older}, map[string][]byte{currentName: current})
		}, ""},
		{"an older release's assets", "minTrailguideVersion: 0.2.0", "", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{olderName: older}, map[string][]byte{olderName: older})
		}, ""},
		{"an older archive under the requested name", "minTrailguideVersion: 0.2.0", "", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: older}, map[string][]byte{olderName: older})
		}, ""},
		{"no minimum installs the latest release", "", "0.2.0", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, "0.2.0"},
		{"no minimum with the latest release's assets replaced", "", "0.2.0", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{olderName: older}, map[string][]byte{olderName: older})
		}, ""},
		{"no minimum with an invalid latest version", "", "*", "pass", func(t *testing.T, assets string) {
			publish(t, assets, "v0.2.0", map[string][]byte{currentName: current}, map[string][]byte{currentName: current})
		}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := t.TempDir()
			config := "schemaVersion: 2\n" + test.minimum + "\nsources: {}\n"
			if strings.HasPrefix(test.minimum, "{") {
				config = test.minimum + "\n"
			}
			edit(t, runner, map[string]string{".trailguide/config.yaml": config})
			assets := filepath.Join(runner, "assets")
			test.assets(t, assets)
			githubPath, summary := filepath.Join(runner, "github-path"), filepath.Join(runner, "step-summary")
			command := exec.Command(bash, "--noprofile", "--norc", "-eo", "pipefail", "-c", installScript(t, string(checkWorkflow)))
			command.Dir = runner
			command.Env = []string{"PATH=" + tools, "RUNNER_TEMP=" + filepath.Join(runner, "temp"), "GITHUB_PATH=" + githubPath, "GITHUB_STEP_SUMMARY=" + summary, "GH_TOKEN=token", "FAKE_LOG=" + filepath.Join(runner, "log"), "FAKE_ASSETS=" + assets, "FAKE_LATEST=" + test.latest, "FAKE_ATTESTATION=" + test.attestation}
			output, err := command.CombinedOutput()
			if (err == nil) != (test.installed != "") {
				calls, _ := os.ReadFile(filepath.Join(runner, "log"))
				t.Fatalf("install step error %v, want installed %q:\n%s\ngh calls:\n%s", err, test.installed, output, calls)
			}
			path, _ := os.ReadFile(githubPath)
			executable, readErr := os.ReadFile(filepath.Join(runner, "temp", "trailguide", "trailguide"))
			if test.installed == "" {
				if len(path) > 0 {
					t.Fatalf("added %q to PATH after a failed install", path)
				}
				return
			}
			if string(path) != filepath.Join(runner, "temp", "trailguide")+"\n" || readErr != nil || !strings.Contains(string(executable), "echo "+test.installed+"\n") {
				t.Fatalf("GITHUB_PATH %q, executable %q %v; want %s installed", path, executable, readErr, test.installed)
			}
			// The log and the job summary say which version the check runs.
			reported, _ := os.ReadFile(summary)
			if want := "Installed Trail Guide " + test.installed + ".\n"; !strings.Contains(string(output), want) || string(reported) != want {
				t.Fatalf("log %q, step summary %q; want each to report %q", output, reported, want)
			}
		})
	}
}
