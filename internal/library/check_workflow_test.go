// Check the GitHub Actions workflow library init writes against the documented workflow.

package library

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestInitialize_WritesTheDocumentedCheckWorkflow pins the running version and matches the guide's workflow outside the install step.
func TestInitialize_WritesTheDocumentedCheckWorkflow(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	result, err := Initialize(context.Background(), options, "", nil, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(options.Directory, ".github", "workflows", "trailguide.yml")
	if !strings.Contains(strings.Join(result.Written(), "\n"), path) {
		t.Fatalf("files %q omit the workflow", result.Written())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	if !strings.Contains(workflow, "\n          version=0.2.0\n") || strings.Contains(workflow, checkWorkflowVersion) {
		t.Fatalf("workflow isn't pinned to v0.2.0:\n%s", workflow)
	}
	guide, err := os.ReadFile("../../docs/src/content/docs/guides/version-rules.md")
	if err != nil {
		t.Fatal(err)
	}
	section := guide[strings.Index(string(guide), "## Check changes in CI"):]
	documented := regexp.MustCompile("(?s)```yaml\\n(.*?)```").FindSubmatch(section)
	if documented == nil {
		t.Fatal("the guide has no check workflow")
	}
	// The guide abbreviates the install step's environment and script as "run: ...".
	abbreviated := regexp.MustCompile(`(?s)(        )env:\n.*?\n(      - run: trailguide library check)`).ReplaceAllString(workflow, "${1}run: ...\n${2}")
	if abbreviated != string(documented[1]) {
		t.Fatalf("workflow differs from the guide outside its install step:\n%s\nguide:\n%s", abbreviated, documented[1])
	}
}

// TestInitialize_PinsReleasesAndInstallsTheLatestForDevelopmentBuilds, which no release published.
func TestInitialize_PinsReleasesAndInstallsTheLatestForDevelopmentBuilds(t *testing.T) {
	latest := `version=$(gh release view --repo fabricahq/trailguide --json tagName --jq '.tagName | ltrimstr("v")')`
	for version, installed := range map[string]string{
		"0.2.0":                   "version=0.2.0",
		"0.3.0-rc.1":              "version=0.3.0-rc.1",
		"0.0.0-development":       latest,
		"0.0.0-dev.g0123456789ab": latest,
	} {
		options := Options{Directory: t.TempDir()}
		if _, err := Initialize(context.Background(), options, "", nil, version); err != nil {
			t.Fatal(version, err)
		}
		data, err := os.ReadFile(filepath.Join(options.Directory, ".github/workflows/trailguide.yml"))
		if err != nil || !strings.Contains(string(data), "\n          "+installed+"\n") {
			t.Fatalf("%s: want %q in:\n%s", version, installed, data)
		}
	}
}

// TestInitialize_KeepsAnExistingWorkflow never overwrites a workflow at the same path.
func TestInitialize_KeepsAnExistingWorkflow(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	custom := "name: Custom\n"
	edit(t, options.Directory, map[string]string{".github/workflows/trailguide.yml": custom})
	result, err := Initialize(context.Background(), options, "", nil, "0.2.0")
	if err != nil || strings.Contains(strings.Join(result.Written(), "\n"), "trailguide.yml") {
		t.Fatal(result, err)
	}
	if data, err := os.ReadFile(filepath.Join(options.Directory, ".github/workflows/trailguide.yml")); err != nil || string(data) != custom {
		t.Fatalf("%q %v", data, err)
	}
}

// TestInitialize_RejectsVersionsThatArentReleaseTags before writing anything.
func TestInitialize_RejectsVersionsThatArentReleaseTags(t *testing.T) {
	for _, version := range []string{"v0.2.0", "0.2", "0.2.0 && curl example.invalid", ""} {
		options := Options{Directory: filepath.Join(t.TempDir(), "library")}
		if _, err := Initialize(context.Background(), options, "", nil, version); errorCode(err) != "invalid-version" {
			t.Fatalf("%q: %v", version, err)
		}
		if _, err := os.Stat(options.Directory); !os.IsNotExist(err) {
			t.Fatalf("%q: created the library", version)
		}
	}
}
