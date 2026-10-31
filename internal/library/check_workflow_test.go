// Check the GitHub Actions workflow library init writes against the documented workflow.

package library

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestInitialize_WritesTheDocumentedCheckWorkflow matches the guide's workflow outside the install step.
func TestInitialize_WritesTheDocumentedCheckWorkflow(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	result, err := Initialize(context.Background(), withToolVersion(options, "0.2.0"), "", nil)
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
	guide, err := os.ReadFile("../../_docs/src/content/docs/guides/version-rules.md")
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

// TestInitialize_WritesOneWorkflowForEveryBuild, since the workflow reads the release to install from the
// configuration when it runs.
func TestInitialize_WritesOneWorkflowForEveryBuild(t *testing.T) {
	for _, version := range []string{"0.2.0", "0.3.0-rc.1", "0.0.0-development"} {
		options := Options{Directory: t.TempDir()}
		if _, err := Initialize(context.Background(), withToolVersion(options, version), "", nil); err != nil {
			t.Fatal(version, err)
		}
		data, err := os.ReadFile(filepath.Join(options.Directory, ".github/workflows/trailguide.yml"))
		if err != nil || !bytes.Equal(data, checkWorkflow) {
			t.Fatalf("%s: %v\n%s", version, err, data)
		}
	}
}

// TestCheckWorkflowRelease reads the release a workflow from an earlier trailguide pins, and none from the
// current workflow.
func TestCheckWorkflowRelease(t *testing.T) {
	for workflow, want := range map[string]string{
		"run: |\n          version=0.9.0\n          archive=x\n": "0.9.0",
		"run: |\n          version=0.9.0-rc.1\n":                 "",
		string(checkWorkflow):                                    "",
	} {
		directory := t.TempDir()
		edit(t, directory, map[string]string{".github/workflows/trailguide.yml": workflow})
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		version, ok, err := CheckWorkflowRelease(context.Background(), root)
		root.Close()
		if err != nil || ok != (want != "") || ok && version.String() != want {
			t.Errorf("%q: %v %v %v, want %q", workflow, version, ok, err, want)
		}
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, ok, err := CheckWorkflowRelease(context.Background(), root); ok || err != nil {
		t.Fatalf("no workflow: %v %v", ok, err)
	}
}

// TestInitialize_KeepsAnExistingWorkflow never overwrites a workflow at the same path.
func TestInitialize_KeepsAnExistingWorkflow(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	custom := "name: Custom\n"
	edit(t, options.Directory, map[string]string{".github/workflows/trailguide.yml": custom})
	result, err := Initialize(context.Background(), withToolVersion(options, "0.2.0"), "", nil)
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
		if _, err := Initialize(context.Background(), withToolVersion(options, version), "", nil); errorCode(err) != "invalid-version" {
			t.Fatalf("%q: %v", version, err)
		}
		if _, err := os.Stat(options.Directory); !os.IsNotExist(err) {
			t.Fatalf("%q: created the library", version)
		}
	}
}
