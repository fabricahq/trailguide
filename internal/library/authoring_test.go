// Verify complete library authoring, original terms, read-only checks, and unused content validation.

package library

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// initRepository creates an empty Git repository in dir. It scrubs every inherited GIT_ variable and ignores
// user and system configuration, so a GIT_DIR set by a Git hook or rebase can't redirect it to another repository.
func initRepository(t *testing.T, dir string) {
	t.Helper()
	command := exec.Command("git", "init", "--quiet", "--template=", dir)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "GIT_") {
			command.Env = append(command.Env, item)
		}
	}
	command.Env = append(command.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
}

// TestInitRepository_IgnoresAnInheritedGitDir keeps tests run from a Git hook or rebase out of the enclosing repository.
func TestInitRepository_IgnoresAnInheritedGitDir(t *testing.T) {
	enclosing := t.TempDir()
	initRepository(t, enclosing)
	config := filepath.Join(enclosing, ".git", "config")
	before, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(enclosing, ".git"))
	target := t.TempDir()
	initRepository(t, target)
	if after, err := os.ReadFile(config); err != nil || !bytes.Equal(after, before) {
		t.Fatalf("changed the enclosing repository's configuration:\n%s", after)
	}
	if _, err := os.Stat(filepath.Join(target, ".git", "HEAD")); err != nil {
		t.Fatal(err)
	}
}

// TestLibraryLifecycle authors a licensed library, validates counts without writes, and rejects marked drafts.
func TestLibraryLifecycle(t *testing.T) {
	ctx := context.Background()
	options := Options{Directory: t.TempDir()}
	notice := "Notice\r\n"
	terms := &Terms{SPDXExpression: "MIT", License: "Original terms\r\n", Notice: &notice}
	result, err := Initialize(ctx, options, terms, "1.2.3")
	if err != nil || len(result.Written()) != 6 {
		t.Fatal(result, err)
	}
	guide, err := os.ReadFile(filepath.Join(options.Directory, ".trailguide", "skills", "README.md"))
	if err != nil || !strings.Contains(string(guide), "skill this library publishes") {
		t.Fatalf("skills/README.md: %q, %v", guide, err)
	}
	data, _ := os.ReadFile(filepath.Join(options.Directory, "LICENSE.md"))
	if string(data) != terms.License {
		t.Fatal("changed terms")
	}
	result, err = Initialize(ctx, options, nil, "1.2.3")
	if err != nil || len(result.Written()) != 0 {
		t.Fatal(result, err)
	}
	if _, err = Initialize(ctx, options, terms, "1.2.3"); err == nil {
		t.Fatal("overwrote terms")
	}
	metadata := trailguide.GroupMetadata{Name: "Go", Description: "Go guidance.", WhenToRead: "When editing Go."}
	if _, err = AddGroup(ctx, "techs/go", metadata, options); err != nil {
		t.Fatal(err)
	}
	body := "# Return failures\n\nReturn failures to the caller.\n"
	rule := rules.RuleMetadata{Title: "Return failures", Impact: "HIGH", ImpactDescription: "Preserve failures.", WhenToRead: "When calling fallible functions."}
	if _, err = AddRule(ctx, "techs/go/errors", rule, RuleOptions{Options: options, Body: &body}); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	before, err := filetxn.ReadTree(ctx, root, ".")
	if err != nil {
		t.Fatal(err)
	}
	check, err := Check(ctx, options)
	if err != nil || check.GroupCount != 1 || check.RuleCount != 1 || len(check.Warnings) != 0 {
		t.Fatal(check, err)
	}
	after, _ := filetxn.ReadTree(ctx, root, ".")
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("check modified library")
	}
	if _, err = AddRule(ctx, "techs/go/draft", rule, RuleOptions{Options: options}); err != nil {
		t.Fatal(err)
	}
	draft, err := os.ReadFile(filepath.Join(options.Directory, librarytree.RepositoryPath("techs/go/draft.md")))
	if err != nil || bytes.Count(draft, []byte(rules.DraftMarker)) != 1 {
		t.Fatal("expected exactly one draft marker", err)
	}
	if _, err = Check(ctx, options); err == nil || !strings.Contains(err.Error(), "draft") {
		t.Fatal("draft accepted", err)
	}
}

// TestLibraryCheckUnusedContent enforces every library-owned file even when no selected rule references it.
func TestLibraryCheckUnusedContent(t *testing.T) {
	for _, scenario := range []string{"empty", "orphan", "missing-link", "unrelated", "cross-rule", "malformed-rule", "symlink", "binary"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			options := Options{Directory: t.TempDir()}
			if _, err := Initialize(ctx, options, nil, "1.2.3"); err != nil {
				t.Fatal(err)
			}
			// write writes the file at library path name where the repository keeps it.
			write := func(name, content string) {
				t.Helper()
				name = librarytree.RepositoryPath(name)
				if err := os.MkdirAll(filepath.Dir(filepath.Join(options.Directory, name)), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(options.Directory, name), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "orphan":
				write("techs/go/assets/missing/image.bin", "binary")
			case "missing-link":
				write("assets/unreferenced.md", "[missing](absent.md)")
			case "unrelated":
				// An unborn branch has no library release, so check needs no change notes.
				initRepository(t, options.Directory)
				write(".git/objects/example", "arbitrary repository bytes")
				write("docs/design.txt", "unrelated documentation")
				write(".trailguide/_managed/vendor/team/config.yaml", "not this library's manifest")
				write(".trailguide/_managed/generated/RULES.md", "[missing](absent.md)")
				write(".trailguide/rules/README.md", "[missing](absent.md)")
			case "cross-rule":
				write("assets/shared.md", "[rule](/rules/techs/go/errors.md)")
			case "malformed-rule":
				write("techs/go/_group.yaml", `{"name":"Go","description":"Go.","whenToRead":"Go."}`)
				write("techs/go/errors.md", "Missing frontmatter")
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(options.Directory, librarytree.RepositoryPath("assets"))); err != nil {
					t.Fatal(err)
				}
			case "binary":
				write("assets/image.bin", string([]byte{0, 255, 254, 42}))
			}
			result, err := Check(ctx, options)
			valid := scenario == "empty" || scenario == "unrelated" || scenario == "binary"
			if valid {
				if err != nil || result.GroupCount != 0 || result.RuleCount != 0 || len(result.Warnings) != 1 {
					t.Fatal(result, err)
				}
			} else if err == nil {
				t.Fatal("invalid library accepted", result)
			}
		})
	}
}

// TestLibraryInitializationPreservesConflicts refuses a term-file collision before publishing the manifest.
func TestLibraryInitializationPreservesConflicts(t *testing.T) {
	directory := t.TempDir()
	os.WriteFile(filepath.Join(directory, "LICENSE.md"), []byte("existing terms"), 0600)
	_, err := Initialize(context.Background(), Options{Directory: directory}, &Terms{SPDXExpression: "MIT", License: "different"}, "1.2.3")
	if err == nil {
		t.Fatal("accepted terms collision")
	}
	if _, err = os.Stat(filepath.Join(directory, librarytree.RepositoryPath(librarytree.Manifest))); !os.IsNotExist(err) {
		t.Fatal("partial manifest", err)
	}
	data, _ := os.ReadFile(filepath.Join(directory, "LICENSE.md"))
	if string(data) != "existing terms" {
		t.Fatal("overwrote terms")
	}
}

// TestLibraryCheckRejectsConcurrentEdits covers files that the selected catalog does not reread.
func TestLibraryCheckRejectsConcurrentEdits(t *testing.T) {
	for _, name := range []string{"assets/guide.md", "assets/new.md", librarytree.Manifest, "LICENSE.md"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			ctx := context.Background()
			_, err := Initialize(ctx, Options{Directory: directory}, &Terms{SPDXExpression: "MIT", License: "Original terms"}, "1.2.3")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(directory, librarytree.RepositoryPath("assets")), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, librarytree.RepositoryPath("assets/guide.md")), []byte("Original guidance"), 0600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			before, err := libraryCheckInput(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, librarytree.RepositoryPath(name)), []byte("[changed](missing.md)"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := requireLibraryUnchanged(ctx, root, before); err == nil || !strings.Contains(err.Error(), "changed during validation") {
				t.Fatal(err)
			}
		})
	}
}

// TestCapturedLibraryIgnoresLaterEdits keeps catalog counts and inventory validation on exactly the same bytes.
func TestCapturedLibraryIgnoresLaterEdits(t *testing.T) {
	ctx := context.Background()
	options := Options{Directory: t.TempDir()}
	if _, err := Initialize(ctx, options, nil, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	snapshot, err := libraryCheckInput(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(options.Directory, librarytree.RepositoryPath("techs/go")), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(options.Directory, librarytree.RepositoryPath("techs/go/_group.yaml")), []byte(`{"name":"Go","description":"Go guidance","whenToRead":"When editing Go"}`), 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadSource(ctx, capturedLibrary{snapshot.tree}, "library", rules.GroupSelection{Pattern: "*"}, nil)
	if err != nil || len(catalog.Groups) != 0 {
		t.Fatal(catalog, err)
	}
	if err := requireLibraryUnchanged(ctx, root, snapshot); err == nil {
		t.Fatal("missed new group")
	}
	if err := os.Remove(filepath.Join(options.Directory, librarytree.RepositoryPath("techs/go/_group.yaml"))); err != nil {
		t.Fatal(err)
	}
	empty, err := libraryCheckInput(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSource(ctx, capturedLibrary{empty.tree}, "library", rules.GroupSelection{Pattern: "*"}, nil); err == nil {
		t.Fatal("empty group without metadata accepted")
	}
}

// TestLibraryCheckBoundsUnusedAssets rejects oversized unreferenced content without modifying it.
func TestLibraryCheckBoundsUnusedAssets(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	if _, err := Initialize(context.Background(), options, nil, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(options.Directory, librarytree.RepositoryPath("assets")), 0700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(options.Directory, librarytree.RepositoryPath("assets/archive.bin"))
	data := make([]byte, 9*1024*1024)
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(context.Background(), options); err == nil || !strings.Contains(err.Error(), "limits") {
		t.Fatal("oversized unused asset accepted", err)
	}
	after, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("check changed asset", err)
	}
}

// TestLibraryRulePreservesGroup leaves existing metadata unchanged when adding a rule.
func TestLibraryRulePreservesGroup(t *testing.T) {
	ctx := context.Background()
	options := Options{Directory: t.TempDir()}
	if _, err := Initialize(ctx, options, nil, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	existing := trailguide.GroupMetadata{Name: "Go", Description: "Concurrent guidance.", WhenToRead: "When editing Go."}
	if _, err := AddGroup(ctx, "techs/go", existing, options); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(options.Directory, librarytree.RepositoryPath("techs/go/_group.yaml"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := "Return errors.\n"
	metadata := rules.RuleMetadata{Title: "Errors", Impact: "HIGH", ImpactDescription: "Preserve failures.", WhenToRead: "When calling functions."}
	result, err := AddRule(ctx, "techs/go/errors", metadata, RuleOptions{Options: options, Body: &body})
	if err != nil || len(result.Written()) != 1 {
		t.Fatal(result, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing group changed", err)
	}
	if _, err := Check(ctx, options); err != nil {
		t.Fatal(err)
	}
}
