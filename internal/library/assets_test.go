// Exercise complete asset directories, recursive dependencies, and unsafe targets on real filesystems.

package library_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
)

// TestLoadAssets retains complete owned trees and the shared assets that retained Markdown links reach, following
// links between shared files, while leaving unrelated rules and unlinked shared assets out.
func TestLoadAssets(t *testing.T) {
	files := validFiles()
	files["techs/go/errors.md"] += "\n![image](assets/errors/image.bin) [shared](/assets/guide.md)\n"
	files["techs/go/assets/errors/image.bin"] = "\x00\xff\r\n"
	files["techs/go/assets/errors/unused.bin"] = "unreferenced owned"
	files["assets/guide.md"] = "[cycle](guide.md) [next](next.md)"
	files["assets/next.md"] = "[cycle](guide.md)"
	files["assets/unreferenced.bin"] = "shared companion"
	files["techs/rust/other.md"] = "not adopted or parsed"
	_, root := fixture(t, files)
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{"techs/go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Rules) != 1 {
		t.Fatalf("adopted another rule: %+v", got.Groups)
	}
	for _, file := range []string{"techs/go/assets/errors/image.bin", "techs/go/assets/errors/unused.bin", "assets/guide.md", "assets/next.md"} {
		if string(got.SupportingFiles[file]) != files[file] {
			t.Errorf("lost bytes: %s", file)
		}
	}
	for _, file := range []string{"techs/rust/other.md", "assets/unreferenced.bin"} {
		if _, ok := got.SupportingFiles[file]; ok {
			t.Errorf("retained %s, which nothing selected links to", file)
		}
	}
}

// TestLoadRejectsAssetFailures returns no partial catalog for invalid dependency closures.
func TestLoadRejectsAssetFailures(t *testing.T) {
	for _, test := range []struct{ name, link, file, content string }{
		{"missing", "assets/errors/missing.png", "", ""},
		{"file as directory", "assets/errors/diagram.txt/child.txt", "techs/go/assets/errors/diagram.txt", "x"},
		{"escape", "../../../outside", "", ""},
		{"other owner", "assets/another/file.png", "techs/go/assets/another/file.png", "x"},
		{"arbitrary supporting", "data.txt", "techs/go/assets/errors/a.bin", "x"},
		{"invalid Markdown", "/assets/guide.md", "assets/guide.md", "\xff"},
		{"lfs", "assets/errors/image.png", "techs/go/assets/errors/image.png", "version https://git-lfs.github.com/spec/v1\noid sha256:123"},
		{"recursive escape", "/assets/guide.md", "assets/guide.md", "[bad](../../outside)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := validFiles()
			files["techs/go/errors.md"] += "\n[x](" + test.link + ")\n"
			if test.file != "" {
				files[test.file] = test.content
			}
			_, root := fixture(t, files)
			got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{"techs/go"}})
			var validation errs.ValidationError
			if !errors.As(err, &validation) || got.Groups != nil {
				t.Fatalf("expected validation failure without partial catalog: %+v, %v", got, err)
			}
		})
	}
}

// TestLoadSkipsUnusedSharedAndRejectsLinkedAssets keeps unrelated files out while refusing selected symlinks.
func TestLoadSkipsUnusedSharedAndRejectsLinkedAssets(t *testing.T) {
	files := validFiles()
	files["assets/bad.md"] = "[escape](../../outside)"
	directory, root := fixture(t, files)
	if _, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, librarytree.RepositoryPath("techs/go/assets/errors")), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, librarytree.RepositoryPath("assets/bad.md")), filepath.Join(directory, librarytree.RepositoryPath("techs/go/assets/errors/link.md"))); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Pattern: "*"}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink: %v", err)
	}
}

// TestLoadRejectsRuleLinks rejects dependencies on selected, unselected, or absent rules and links inside attachments.
func TestLoadRejectsRuleLinks(t *testing.T) {
	for _, test := range []struct{ name, from, link, target string }{
		{"selected", "techs/go/errors.md", "other.md", "techs/go/other.md"},
		{"unselected", "techs/go/errors.md", "../rust/other.md", "techs/rust/other.md"},
		{"missing", "techs/go/errors.md", "missing.md", ""},
		{"reference", "techs/go/errors.md", "", "techs/go/other.md"},
		{"shared attachment", "assets/guide.md", "/rules/techs/go/errors.md", ""},
		{"HTML attachment", "assets/guide.md", "", ""},
		{"owned attachment", "techs/go/assets/errors/guide.md", "../../errors.md", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := validFiles()
			if test.target != "" {
				files[test.target] = files["techs/go/errors.md"]
			}
			if test.from != "techs/go/errors.md" {
				files["techs/go/errors.md"] += "\n[guide](/" + strings.TrimPrefix(librarytree.RepositoryPath(test.from), librarytree.Directory+"/") + ")\n"
			}
			if test.name == "HTML attachment" {
				files[test.from] += `<a href="/rules/techs/go/errors.md">rule</a>`
			} else if test.name == "reference" {
				files[test.from] += "\n[other][rule]\n\n[rule]: other.md#details\n"
			} else {
				files[test.from] += "\n[other](" + test.link + ")\n"
			}
			_, root := fixture(t, files)
			got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{"techs/go"}})
			if err == nil || !strings.Contains(err.Error(), "links to other rule documents are not allowed") || got.Groups != nil {
				t.Fatalf("expected rule-link error without partial catalog: %+v, %v", got, err)
			}
		})
	}
}

// TestLoadAllowsDeclaredGroupTerms treats declared license Markdown whose path can't be a rule as supporting text,
// not an invalid rule.
func TestLoadAllowsDeclaredGroupTerms(t *testing.T) {
	files := validFiles()
	files["config.yaml"] = `{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1,"license":{"file":".trailguide/rules/techs/go/LICENSE.md","notices":[]}}}`
	files["techs/go/LICENSE.md"] = "License terms."
	files["techs/go/errors.md"] += "\n[terms](LICENSE.md)\n"
	_, root := fixture(t, files)
	got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{"techs/go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups[0].Rules) != 1 || string(got.SupportingFiles["techs/go/LICENSE.md"]) != "License terms." {
		t.Fatalf("did not retain terms separately from rules: %+v", got)
	}
}

// TestLoadRejectsTermsInsideRuleContent refuses a license or notice declared at a rule's path or inside an asset
// directory in a group, whether or not the source selects that group, because those files belong to a rule's version.
func TestLoadRejectsTermsInsideRuleContent(t *testing.T) {
	for _, term := range []string{"techs/go/terms.md", "techs/go/assets/LICENSE", "techs/go/assets/legal/nested/LICENSE", "techs/go/assets/errors/LICENSE"} {
		for _, groups := range [][]string{{}, {"techs/go"}} {
			t.Run(term+strings.Join(groups, ","), func(t *testing.T) {
				files := validFiles()
				manifest, err := json.Marshal(map[string]any{"schemaVersion": 2, "sources": map[string]any{}, "library": map[string]any{"schemaVersion": 1, "license": map[string]any{"file": "LICENSE", "notices": []string{librarytree.RepositoryPath(term)}}}})
				if err != nil {
					t.Fatal(err)
				}
				files["config.yaml"] = string(manifest)
				files["LICENSE"], files[term] = "License\r\n", "Notice\n"
				_, root := fixture(t, files)
				got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: groups})
				var invalid errs.ValidationError
				if !errors.As(err, &invalid) || invalid.ValidationLocation() != "team/.trailguide/config.yaml: library.license.notices[0]" || got.Groups != nil {
					t.Fatalf("got %+v, %v; want the notice path refused", got, err)
				}
			})
		}
	}
}

// caseInsensitiveFiles finds files under any spelling, as the default macOS and Windows filesystems do, while
// ReadDir reports each entry's real name.
type caseInsensitiveFiles struct{ files fstest.MapFS }

// resolve returns name spelled as the files spell it, or name itself when nothing matches.
func (c caseInsensitiveFiles) resolve(name string) string {
	resolved := "."
	for _, part := range strings.Split(name, "/") {
		entries, err := fs.ReadDir(c.files, resolved)
		if err != nil {
			return name
		}
		index := slices.IndexFunc(entries, func(entry fs.DirEntry) bool { return strings.EqualFold(entry.Name(), part) })
		if index < 0 {
			return name
		}
		resolved = path.Join(resolved, entries[index].Name())
	}
	return resolved
}

func (c caseInsensitiveFiles) Lstat(name string) (fs.FileInfo, error) {
	return fs.Stat(c.files, c.resolve(name))
}
func (c caseInsensitiveFiles) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(c.files, c.resolve(name))
}
func (c caseInsensitiveFiles) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(c.files, c.resolve(name))
}

// TestLoadRejectsSharedAssetLinksSpelledDifferently refuses a shared asset link whose spelling differs from the
// file's in any component, including the root assets directory, even where the filesystem would find the file:
// Git and other checkouts wouldn't.
func TestLoadRejectsSharedAssetLinksSpelledDifferently(t *testing.T) {
	for _, test := range []struct {
		name, file string
		valid      bool
	}{
		{"same spelling", "assets/diagram.png", true},
		{"root directory", "Assets/diagram.png", false},
		{"file", "assets/Diagram.png", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := fstest.MapFS{}
			for name, text := range validFiles() {
				files[name] = &fstest.MapFile{Data: []byte(text)}
			}
			files["techs/go/errors.md"] = &fstest.MapFile{Data: []byte(document + "\n![Diagram](/assets/diagram.png)\n")}
			files[test.file] = &fstest.MapFile{Data: []byte("shared")}
			_, err := library.LoadSource(context.Background(), caseInsensitiveFiles{files}, "team", rules.GroupSelection{Groups: []string{"techs/go"}}, nil)
			var validation errs.ValidationError
			if test.valid && err != nil || !test.valid && (!errors.As(err, &validation) || !strings.Contains(validation.ValidationProblem(), "spelled differently")) {
				t.Fatalf("got %v, valid %t", err, test.valid)
			}
		})
	}
}

// TestLoadRejectsMisspelledAssetLinks checks retained paths even on case-insensitive filesystems.
func TestLoadRejectsMisspelledAssetLinks(t *testing.T) {
	for _, target := range []string{"/assets/diagram.png", "/Assets/Diagram.png", "assets/errors/diagram.png"} {
		t.Run(target, func(t *testing.T) {
			files := validFiles()
			files["techs/go/errors.md"] += "\n[image](" + target + ")\n"
			files["assets/Diagram.png"] = "shared"
			files["techs/go/assets/errors/Diagram.png"] = "owned"
			_, root := fixture(t, files)
			got, err := library.Load(context.Background(), root, "team", rules.GroupSelection{Groups: []string{"techs/go"}})
			if err == nil || got.Groups != nil {
				t.Fatalf("accepted mismatched asset spelling: %+v, %v", got, err)
			}
		})
	}
}
