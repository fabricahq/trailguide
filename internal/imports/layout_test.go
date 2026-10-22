// Import from libraries in the version 2 layout, from a library that is also a project, and refuse library
// releases made in the layout before version 2 unless only their unchanged rules' files are read.

package imports

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// legacyRule is a valid rule whose body links to the shared asset guide.md as link spells it.
func legacyRule(name, link string) []byte {
	return versionedRule(name + " [Guide](" + link + ")")
}

// movedHistory is a library whose release/1 keeps the layout before version 2, with rule a linking to a shared
// asset with an absolute link, rule b with a relative one, and rule c without links, and whose release/2 moved it
// into .trailguide/, giving b's link one more ../ as a patch change.
func movedHistory(t *testing.T) history {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, map[string][]byte{
		"rule-library.yaml":    []byte("formatVersion: 1\n"),
		"techs/go/_group.yaml": groupMetadata,
		"techs/go/a.md":        legacyRule("a 1.0.0", "/assets/guide.md"),
		"techs/go/b.md":        legacyRule("b 1.0.0", "../../assets/guide.md"),
		"techs/go/c.md":        versionedRule("c 1.0.0"),
		"assets/guide.md":      []byte("Guide.\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	h := history{fixture: f, commits: map[int]string{1: f.LatestCommit}}
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/a: 1.0.0\n  techs/go/b: 1.0.0\n  techs/go/c: 1.0.0\nchanges:\n  techs/go/a: {change: new, summaries: [Add the rule.]}\n  techs/go/b: {change: new, summaries: [Add the rule.]}\n  techs/go/c: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	moved := map[string][]byte{"rule-library.yaml": nil, "techs/go/_group.yaml": nil, "techs/go/a.md": nil, "techs/go/b.md": nil, "techs/go/c.md": nil, "assets/guide.md": nil}
	maps.Copy(moved, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte("schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 1}\n"),
		"techs/go/_group.yaml": groupMetadata,
		"techs/go/a.md":        legacyRule("a 1.0.0", "/assets/guide.md"),
		"techs/go/b.md":        legacyRule("b 1.0.0", "../../../assets/guide.md"),
		"techs/go/c.md":        versionedRule("c 1.0.0"),
		"assets/guide.md":      []byte("Guide.\n"),
	}))
	commit, err := f.Commit(ctx, f.Worktree(), "Move the library into .trailguide", moved)
	if err != nil {
		t.Fatal(err)
	}
	h.commits[2] = commit
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/a: 1.0.0\n  techs/go/b: 1.0.1\n  techs/go/c: 1.0.0\nchanges:\n  techs/go/b: {change: patch, from: 1.0.0, summaries: [Link to the moved guide.]}\n"); err != nil {
		t.Fatal(err)
	}
	return h
}

// TestImport_RefusesALibraryReleaseInTheLayoutBeforeVersion2 fails when the release a source selects supplies its
// manifest and library-wide files from the layout before version 2.
func TestImport_RefusesALibraryReleaseInTheLayoutBeforeVersion2(t *testing.T) {
	h := movedHistory(t)
	_, err := h.sync(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`), nil)
	requireCode(t, err, "unsupported-library-layout")
	want := "sources.team: the library's release/1 keeps its files in the layout from before this version of Trail Guide, with rule-library.yaml and rule folders at the repository root. This version reads a library's manifest from the library section of .trailguide/config.yaml and its rules from .trailguide/rules/, so it can't import release/1. Ask the library's maintainer to move the library's files into .trailguide/ and publish a library release, then select that release, such as with trailguide project update team."
	if !strings.HasSuffix(err.Error(), want) {
		t.Fatalf("got %v\nwant %s", err, want)
	}
}

// TestImport_KeepsRuleVersionsPublishedBeforeTheLibraryMoved imports unchanged rules from the library release that
// published them in the layout before version 2, with the same IDs and versions, and the moved library's
// library-wide files.
func TestImport_KeepsRuleVersionsPublishedBeforeTheLibraryMoved(t *testing.T) {
	h := movedHistory(t)
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.1@2", "techs/go/c": "1.0.0@1"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	if imported.Snapshot.Rules["techs/go/a"].Commit != h.commits[1] || imported.Snapshot.Commit != h.commits[2] {
		t.Fatalf("snapshot %+v", imported.Snapshot)
	}
	want := []string{"assets/guide.md", "config.yaml", "techs/go/_group.yaml", "techs/go/a.md", "techs/go/b.md", "techs/go/c.md"}
	if got := slices.Sorted(maps.Keys(imported.Snapshot.Files)); !reflect.DeepEqual(got, want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	if !imported.Snapshot.Rules["techs/go/a"].LegacyLayout || imported.Snapshot.Rules["techs/go/b"].LegacyLayout {
		t.Fatalf("legacy layout recorded as %+v", imported.Snapshot.Rules)
	}
}

// TestReadPublishedRule_ReadsAVersionPublishedBeforeTheLibraryMoved reads a rule version that a library release in
// the layout before version 2 published, reporting that layout, and refuses one whose links resolve differently now.
func TestReadPublishedRule_ReadsAVersionPublishedBeforeTheLibraryMoved(t *testing.T) {
	h := movedHistory(t)
	source := rules.Source{Name: "team", Repository: h.fixture.Repository}
	options := Options{GitPath: h.fixture.GitPath, Environment: h.fixture.Environment}
	published, err := ReadPublishedRule(context.Background(), source, "techs/go/a", trailguide.RuleVersion{Major: 1}, options)
	if err != nil {
		t.Fatal(err)
	}
	if !published.LegacyLayout || published.Commit != h.commits[1] || published.GroupMetadata == nil || published.Files["assets/guide.md"] == nil {
		t.Fatalf("published %+v", published)
	}
	_, err = ReadPublishedRule(context.Background(), source, "techs/go/b", trailguide.RuleVersion{Major: 1}, options)
	requireCode(t, err, "unsupported-library-layout")
	moved, err := ReadPublishedRule(context.Background(), source, "techs/go/b", trailguide.RuleVersion{Major: 1, Patch: 1}, options)
	if err != nil || moved.LegacyLayout || moved.Commit != h.commits[2] {
		t.Fatalf("published %+v, %v", moved, err)
	}
}

// TestImport_RefusesARuleVersionWhoseLinksMovedWithTheLibrary fails for a rule version published before the move
// whose relative link reached a shared asset from the repository root, which resolves elsewhere now.
func TestImport_RefusesARuleVersionWhoseLinksMovedWithTheLibrary(t *testing.T) {
	h := movedHistory(t)
	_, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/b","version":"1.0.0","reason":"Keep."}]}`), nil)
	requireCode(t, err, "unsupported-library-layout")
	want := "sources.team: rule techs/go/b version 1.0.0 comes from the library's release/1, which keeps its files in the layout from before this version of Trail Guide, and its link to assets/guide.md resolves differently in the current layout, so this version of Trail Guide can't import the rule's version. Select a version of techs/go/b that the library published after moving its files into .trailguide/, such as with trailguide project update team:techs/go/b, or ask the library's maintainer to publish one."
	if !strings.HasSuffix(err.Error(), want) {
		t.Fatalf("got %v\nwant %s", err, want)
	}
}

// TestImport_ReadsALibraryThatIsAlsoAProject imports only the library's own files from a repository whose
// configuration also has sources, leaving out its vendored and generated files, the README in its rules folder, and
// its configuration's sources.
func TestImport_ReadsALibraryThatIsAlsoAProject(t *testing.T) {
	ctx := context.Background()
	f := newLibraryFixture(t, map[string][]byte{
		"config.yaml":          []byte("schemaVersion: 2\nsources:\n  upstream:\n    repository: https://github.com/acme/upstream.git\n    rules:\n      include: ['techs/*']\nlibrary:\n  schemaVersion: 1\n  license:\n    spdxExpression: MIT\n    file: LICENSE.md\n    notices: []\n"),
		"LICENSE.md":           []byte("MIT License\n"),
		"techs/go/_group.yaml": groupMetadata,
		"techs/go/a.md":        versionedRule("a 1.0.0"),
	})
	if _, err := f.Commit(ctx, f.Worktree(), "Sync the project", map[string][]byte{
		".trailguide/vendor/upstream/config.yaml":        []byte("not this library's manifest\n"),
		".trailguide/vendor/upstream/techs/go/errors.md": []byte("Vendored.\n"),
		".trailguide/generated/RULES.md":                 []byte("Generated.\n"),
		".trailguide/rules/README.md":                    []byte("The project's guide.\n"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/a: 1.0.0\nchanges:\n  techs/go/a: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	h := history{fixture: f, commits: map[int]string{}}
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"LICENSE.md", "config.yaml", "techs/go/_group.yaml", "techs/go/a.md"}
	if got := slices.Sorted(maps.Keys(imported.Snapshot.Files)); !reflect.DeepEqual(got, want) || imported.Catalog.License == nil || *imported.Catalog.License.SPDXExpression != "MIT" {
		t.Fatalf("files %v, want %v; license %+v", got, want, imported.Catalog.License)
	}
	manifest := `{"library":{"license":{"file":"LICENSE.md","notices":[],"spdxExpression":"MIT"},"schemaVersion":1},"sources":{}}` + "\n"
	if got := string(imported.Snapshot.Files["config.yaml"]); got != manifest {
		t.Fatalf("manifest %s, want only the library section: %s", got, manifest)
	}
}

// TestImport_RefusesARepositoryWithoutALibrarySection fails for a project that isn't a library.
func TestImport_RefusesARepositoryWithoutALibrarySection(t *testing.T) {
	ctx := context.Background()
	f := newLibraryFixture(t, map[string][]byte{
		"config.yaml":          []byte("schemaVersion: 2\nsources: {}\n"),
		"techs/go/_group.yaml": groupMetadata,
		"techs/go/a.md":        versionedRule("a 1.0.0"),
	})
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/a: 1.0.0\nchanges:\n  techs/go/a: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	h := history{fixture: f, commits: map[int]string{}}
	_, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	if err == nil || !strings.Contains(err.Error(), "the configuration has no library section, so the repository isn't a library") {
		t.Fatalf("got %v; want the missing library section reported", err)
	}
}

// legacyLibrary is a library whose release/1 keeps files, by library path, in the layout before version 2, beside
// rule-library.yaml with manifest, publishing techs/go/a at 1.0.0, and whose release/2 moves the same files into
// .trailguide/ without changing a.
func legacyLibrary(t *testing.T, manifest string, files map[string][]byte) history {
	t.Helper()
	ctx := context.Background()
	release1 := map[string][]byte{"rule-library.yaml": []byte(manifest), "techs/go/_group.yaml": groupMetadata}
	maps.Copy(release1, files)
	f, err := gitfixture.New(ctx, release1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	h := history{fixture: f, commits: map[int]string{1: f.LatestCommit}}
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/a: 1.0.0\nchanges:\n  techs/go/a: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	moved := map[string][]byte{}
	for name := range release1 {
		moved[name] = nil
	}
	release2 := map[string][]byte{"config.yaml": []byte("schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 1}\n")}
	maps.Copy(release2, files)
	release2["techs/go/_group.yaml"] = groupMetadata
	maps.Copy(moved, gitfixture.Library(release2))
	commit, err := f.Commit(ctx, f.Worktree(), "Move the library into .trailguide", moved)
	if err != nil {
		t.Fatal(err)
	}
	h.commits[2] = commit
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/a: 1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	return h
}

// TestReadPublishedRule_RefusesAVersionWhoseAssetLinksMovedWithTheLibrary fails, as for the rule's own links, when
// a Markdown file in the rule's asset directory, published before the move, links relatively to a shared asset from
// the repository root, both when forking and when importing the version.
func TestReadPublishedRule_RefusesAVersionWhoseAssetLinksMovedWithTheLibrary(t *testing.T) {
	h := legacyLibrary(t, "formatVersion: 1\n", map[string][]byte{
		"techs/go/a.md":              legacyRule("a 1.0.0", "assets/a/notes.md"),
		"techs/go/assets/a/notes.md": []byte("[Guide](../../../../assets/guide.md)\n"),
		"assets/guide.md":            []byte("Guide.\n"),
	})
	source := rules.Source{Name: "team", Repository: h.fixture.Repository}
	options := Options{GitPath: h.fixture.GitPath, Environment: h.fixture.Environment}
	_, err := ReadPublishedRule(context.Background(), source, "techs/go/a", trailguide.RuleVersion{Major: 1}, options)
	requireCode(t, err, "unsupported-library-layout")
	if !strings.Contains(err.Error(), "its link to assets/guide.md resolves differently") {
		t.Fatalf("got %v; want the link that moved named", err)
	}
	_, err = h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	requireCode(t, err, "unsupported-library-layout")
}

// TestReadPublishedRule_LeavesOutANoticeThatALegacyManifestDeclares reads a rule version, published before the move,
// that links to a shared notice file its rule-library.yaml declares, without the notice, as for the current layout.
func TestReadPublishedRule_LeavesOutANoticeThatALegacyManifestDeclares(t *testing.T) {
	h := legacyLibrary(t, "formatVersion: 1\nlicense:\n  spdxExpression: MIT\n  file: LICENSE.md\n  notices: [assets/NOTICE.md]\n", map[string][]byte{
		"LICENSE.md":       []byte("MIT License\n"),
		"techs/go/a.md":    legacyRule("a 1.0.0", "/assets/NOTICE.md"),
		"assets/NOTICE.md": []byte("Notice.\n"),
	})
	source := rules.Source{Name: "team", Repository: h.fixture.Repository}
	options := Options{GitPath: h.fixture.GitPath, Environment: h.fixture.Environment}
	published, err := ReadPublishedRule(context.Background(), source, "techs/go/a", trailguide.RuleVersion{Major: 1}, options)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := slices.Sorted(maps.Keys(published.Files)), []string{"techs/go/a.md"}; !published.LegacyLayout || !reflect.DeepEqual(got, want) {
		t.Fatalf("files %v, want %v; legacy layout %v", got, want, published.LegacyLayout)
	}
}
