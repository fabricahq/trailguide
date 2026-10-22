// Exercise real directory reads, selection, byte preservation, and unsafe-file failures.

package library_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

const metadata = `{"name":" Go ","description":"Go rules","whenToRead":"When writing Go."}`
const document = "---\ntitle: Handle errors\nimpact: HIGH\nimpactDescription: Avoid losing failures.\nwhenToRead: When calling fallible functions.\n---\n# Handle errors\n\nReturn the error.\n"

// fixture creates an isolated on-disk library repository from files, keyed by library path, which it writes where
// librarytree.RepositoryPath places them, and registers both directory and handle cleanup.
func fixture(t *testing.T, files map[string]string) (string, *os.Root) {
	t.Helper()
	directory := t.TempDir()
	for path, text := range files {
		target := filepath.Join(directory, filepath.FromSlash(librarytree.RepositoryPath(path)))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	// Close the caller-owned root before the fixture directory is removed.
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	return directory, root
}

// validFiles provides original authored fixtures rather than production source material.
func validFiles() map[string]string {
	return map[string]string{"config.yaml": `{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`, "techs/go/_group.yaml": metadata, "techs/go/errors.md": document, "practices/testing/_group.yaml": metadata}
}

// TestLoadCatalog checks wildcard expansion, a wildcard combined with listed groups, empty groups, exact bytes, and
// explicit empty selection.
func TestLoadCatalog(t *testing.T) {
	files := validFiles()
	files["techs/go/assets/errors/image.bin"] = "\xff\x00"
	_, root := fixture(t, files)
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 2 || got.Groups[0].ID != "practices/testing" || len(got.Groups[0].Rules) != 0 || got.Groups[1].Rules[0].ID != "team:techs/go/errors" || got.Groups[1].Metadata.Name != "Go" {
		t.Fatalf("unexpected catalog %+v", got)
	}
	if got.Groups[1].Rules[0].Document != document {
		t.Fatal("changed original bytes")
	}
	if _, ok := got.SupportingFiles["techs/go/errors.md"]; ok {
		t.Fatal("rule document duplicated in supporting files")
	}
	if !reflect.DeepEqual(got.Paths(), []string{"config.yaml", "practices/testing/_group.yaml", "techs/go/_group.yaml", "techs/go/assets/errors/image.bin", "techs/go/errors.md"}) {
		t.Fatalf("incomplete read inventory: %v", got.Paths())
	}
	if string(got.SupportingFiles["techs/go/assets/errors/image.bin"]) != "\xff\x00" {
		t.Fatal("catalog lost binary asset bytes")
	}
	techs, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "techs/*"})
	if err != nil || len(techs.Groups) != 1 {
		t.Fatalf("techs: %+v, %v", techs, err)
	}
	mixed, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "techs/*", Groups: []string{"practices/testing", "techs/go"}})
	if err != nil || len(mixed.Groups) != 2 || mixed.Groups[0].ID != "practices/testing" || mixed.Groups[1].ID != "techs/go" {
		t.Fatalf("wildcard with groups: %+v, %v", mixed, err)
	}
	empty, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{}})
	if err != nil || len(empty.Groups) != 0 || !reflect.DeepEqual(empty.Paths(), []string{"config.yaml"}) {
		t.Fatalf("empty: %+v, %v", empty, err)
	}
}

// TestLoadRejectsInvalidLibraries checks partial-result prevention for selected content failures.
func TestLoadRejectsInvalidLibraries(t *testing.T) {
	for _, test := range []struct {
		name, path, text string
		remove           bool
	}{
		{"missing manifest", "config.yaml", "", true},
		{"missing metadata", "techs/go/_group.yaml", "", true},
		{"invalid rule", "techs/go/errors.md", "no frontmatter", false},
		{"blank body", "techs/go/errors.md", strings.Split(document, "# Handle")[0], false},
		{"invalid utf8", "techs/go/errors.md", "\xff", false},
		{"unsupported file", "techs/go/data.json", "{}", false},
		{"lfs pointer", "techs/go/errors.md", "version https://git-lfs.github.com/spec/v1\noid sha256:example", false},
	} {
		// Every malformed library is exercised through the filesystem API.
		t.Run(test.name, func(t *testing.T) {
			files := validFiles()
			if test.remove {
				delete(files, test.path)
			} else {
				files[test.path] = test.text
			}
			_, root := fixture(t, files)
			got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"})
			var validation errs.ValidationError
			if !errors.As(err, &validation) || !reflect.DeepEqual(got, library.Catalog{}) {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

// TestLoadSymlinksAndCancellation rejects linked selected entries and preserves context cancellation identity.
func TestLoadSymlinksAndCancellation(t *testing.T) {
	directory, root := fixture(t, validFiles())
	if err := os.Symlink(filepath.Join(t.TempDir(), "private.md"), filepath.Join(directory, librarytree.RepositoryPath("techs/go/link.md"))); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"}); err == nil {
		t.Fatal("accepted symlink")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := library.Load(ctx, root, "team", rules.GroupSelection{Pattern: "*"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{"techs/missing"}}); err == nil {
		t.Fatal("accepted missing explicit group")
	}
}

// TestLoadTermsAndLimits retains binary licenses and rejects files above the exact byte bound.
func TestLoadTermsAndLimits(t *testing.T) {
	files := validFiles()
	files["config.yaml"] = `{"schemaVersion":2,"sources":{},"library":{"formatVersion":1,"license":{"file":"LICENSE","notices":["NOTICE"],"spdxExpression":"MIT"}}}`
	files["LICENSE"] = "\xff\x00\r\n"
	files["NOTICE"] = "Notice\r\n"
	_, root := fixture(t, files)
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"})
	if err != nil || string(got.SupportingFiles["LICENSE"]) != files["LICENSE"] || string(got.SupportingFiles["NOTICE"]) != files["NOTICE"] {
		t.Fatalf("terms changed: %v", err)
	}
	for _, size := range []int{8 * 1024 * 1024, 8*1024*1024 + 1} {
		bounded := map[string]string{"config.yaml": files["config.yaml"], "LICENSE": strings.Repeat("x", size), "NOTICE": ""}
		_, root := fixture(t, bounded)
		_, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{}})
		if (err != nil) != (size > 8*1024*1024) {
			t.Fatalf("size %d: %v", size, err)
		}
	}
}

// TestLoadRequiresDeclaredTerms proves retained files are checked by the production reader.
func TestLoadRequiresDeclaredTerms(t *testing.T) {
	for _, missing := range []string{"LICENSE", "NOTICE"} {
		t.Run(missing, func(t *testing.T) {
			files := map[string]string{
				"config.yaml": "schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n  license:\n    file: LICENSE\n    notices: [NOTICE]\n",
				"LICENSE":     "", "NOTICE": "",
			}
			delete(files, missing)
			_, root := fixture(t, files)
			got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{}})
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != missing || !reflect.DeepEqual(got, library.Catalog{}) {
				t.Fatalf("missing %s: got %+v, %v", missing, got, err)
			}
		})
	}
}

// TestLoadRejectsLegacyManifestName requires the manifest in the library section of .trailguide/config.yaml.
func TestLoadRejectsLegacyManifestName(t *testing.T) {
	_, root := fixture(t, map[string]string{"rule-library.json": `{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`, "rule-library.yaml": "formatVersion: 1\n"})
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{}})
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != ".trailguide/config.yaml" || !reflect.DeepEqual(got, library.Catalog{}) {
		t.Fatalf("legacy manifest: got %+v, %v", got, err)
	}
}

// TestLoadLeavesFilesUnchanged verifies that catalog loading does not alter the fixture tree.
func TestLoadLeavesFilesUnchanged(t *testing.T) {
	files := validFiles()
	directory, root := fixture(t, files)
	if _, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"}); err != nil {
		t.Fatal(err)
	}
	count := 0
	// Compare every file after loading, including unselected content, with its authored bytes.
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		count++
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, _ := librarytree.LibraryPath(filepath.ToSlash(relative))
		expected, ok := files[file]
		if !ok || string(data) != expected {
			t.Errorf("unexpected or changed file: %s", relative)
		}
		return nil
	})
	if err != nil || count != len(files) {
		t.Fatalf("changed inventory: count %d, %v", count, err)
	}
}

// TestLoadRejectsDirectoryLinksAndInvalidSelections covers entry points before file interpretation.
func TestLoadRejectsDirectoryLinksAndInvalidSelections(t *testing.T) {
	directory, root := fixture(t, validFiles())
	if err := os.Symlink(filepath.Join(directory, librarytree.RepositoryPath("techs/go")), filepath.Join(directory, librarytree.RepositoryPath("techs/rust"))); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "techs/*"}); err == nil {
		t.Fatal("accepted linked group")
	}
	for _, selection := range []rules.GroupSelection{{Pattern: "other/*"}, {Groups: []string{"techs"}}, {Groups: []string{"techs/go", "techs/go"}}, {Pattern: "*", Groups: []string{"techs/missing"}}} {
		if _, err := library.Load(context.Background(), root, "team", selection); err == nil {
			t.Fatalf("accepted selection %+v", selection)
		}
	}
}

// TestLoadOwnsOriginalDocuments preserves authored text after source files change.
func TestLoadOwnsOriginalDocuments(t *testing.T) {
	files := validFiles()
	files["techs/go/errors.md"] = strings.ReplaceAll(strings.Replace(document, "title: Handle errors", "# Author comment\ntitle: 'Handle errors'\ntags: [errors, reliability]", 1), "\n", "\r\n")
	files["techs/go/other.md"] = document + "Keep  spacing.  \n"
	directory, root := fixture(t, files)
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "techs/*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Rules) != 2 {
		t.Fatalf("unexpected groups or rules: %+v", got.Groups)
	}
	for _, rule := range got.Groups[0].Rules {
		if err := os.WriteFile(filepath.Join(directory, filepath.FromSlash(librarytree.RepositoryPath(rule.Path))), []byte("changed after load"), 0600); err != nil {
			t.Fatal(err)
		}
		if rule.Document != files[rule.Path] {
			t.Fatalf("original document lost: %s", rule.Path)
		}
		if _, duplicated := got.SupportingFiles[rule.Path]; duplicated {
			t.Fatalf("rule also stored in supporting files: %s", rule.Path)
		}
		sections, err := trailguide.SplitDocument(rule.Document, rule.ID)
		if err != nil || !strings.Contains(sections.Frontmatter, "title:") || !strings.Contains(sections.Body, "Return the error.") {
			t.Fatalf("original sections unavailable: %+v, %v", sections, err)
		}
	}
	if string(got.SupportingFiles["techs/go/_group.yaml"]) != metadata {
		t.Fatal("supporting metadata bytes changed")
	}
}

// TestLoadRejectsReservedSource keeps imported identities separate from project rules.
func TestLoadRejectsReservedSource(t *testing.T) {
	_, root := fixture(t, validFiles())
	got, err := library.Load(context.Background(), root, "local", rules.GroupSelection{Pattern: "*"})
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "source" || !strings.Contains(err.Error(), "reserved") || got.Groups != nil {
		t.Fatalf("reserved source returned %+v, %v", got, err)
	}
}

// TestLoadRejectsHardLinks exercises a real outside file linked into a selected library.
func TestLoadRejectsHardLinks(t *testing.T) {
	directory, root := fixture(t, validFiles())
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(directory, librarytree.RepositoryPath("techs/go/linked.md"))); err != nil {
		t.Fatal(err)
	}
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"})
	var validation errs.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "hard links") || got.Groups != nil {
		t.Fatalf("hard link returned %+v, %v", got, err)
	}
}

// TestLoadTermsDoNotCreateGroups excludes direct and nested declared terms from wildcard discovery.
func TestLoadTermsDoNotCreateGroups(t *testing.T) {
	for _, file := range []string{"techs/LICENSE", "techs/legal/LICENSE", "techs/legal/nested/NOTICE"} {
		files := map[string]string{"config.yaml": `{"schemaVersion":2,"sources":{},"library":{"formatVersion":1,"license":{"file":"` + librarytree.RepositoryPath(file) + `","notices":[]}}}`, file: "terms"}
		_, root := fixture(t, files)
		got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"})
		if err != nil || len(got.Groups) != 0 || string(got.SupportingFiles[file]) != "terms" {
			t.Fatalf("%s: %+v, %v", file, got, err)
		}
	}
	files := validFiles()
	files["config.yaml"] = `{"schemaVersion":2,"sources":{},"library":{"formatVersion":1,"license":{"file":".trailguide/rules/techs/go/LICENSE","notices":[]}}}`
	files["techs/go/LICENSE"] = "terms"
	_, root := fixture(t, files)
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "techs/*"})
	if err != nil || len(got.Groups) != 1 {
		t.Fatalf("mixed group omitted: %+v, %v", got, err)
	}
}

// TestMixedTermsDiscoveryCountsOnce keeps explicit and wildcard selections equivalent near the discovery limit.
func TestMixedTermsDiscoveryCountsOnce(t *testing.T) {
	files := validFiles()
	document := files["techs/go/errors.md"]
	delete(files, "techs/go/errors.md")
	files["config.yaml"] = `{"schemaVersion":2,"sources":{},"library":{"formatVersion":1,"license":{"file":".trailguide/rules/techs/go/LICENSE","notices":[]}}}`
	files["techs/go/LICENSE"] = "terms"
	for i := range 5000 {
		files[fmt.Sprintf("techs/go/r%d.md", i)] = document
	}
	_, root := fixture(t, files)
	for _, selection := range []rules.GroupSelection{{Groups: []string{"techs/go"}}, {Pattern: "techs/*"}} {
		got, err := library.Load(context.Background(), root, "team", selection)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Groups) != 1 || len(got.Groups[0].Rules) != 5000 {
			t.Fatal("incomplete catalog")
		}
	}
}

// TestMetadataCanAlsoBeATerm preserves an empty group's metadata role during wildcard selection.
func TestMetadataCanAlsoBeATerm(t *testing.T) {
	files := validFiles()
	delete(files, "techs/go/errors.md")
	files["config.yaml"] = `{"schemaVersion":2,"sources":{},"library":{"formatVersion":1,"license":{"file":".trailguide/rules/techs/go/_group.yaml","notices":[]}}}`
	_, root := fixture(t, files)
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "techs/*"})
	if err != nil || len(got.Groups) != 1 {
		t.Fatalf("empty group lost: %+v, %v", got, err)
	}
}
