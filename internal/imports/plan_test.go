// Exercise choosing rule versions from real release tags: newest, pinned, recorded, retired, and ref imports.

package imports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// versionedRule is a valid rule whose body names its version, so tests can tell which version's file arrived.
func versionedRule(version string) []byte {
	return []byte("---\ntitle: Rule\nimpact: HIGH\nimpactDescription: Matters.\nwhenToRead: Always.\n---\n# Rule\n\nVersion " + version + " text.\n")
}

// groupMetadata is valid group metadata.
var groupMetadata = []byte(`{"name":"Group","description":"Rules.","whenToRead":"Always."}`)

// history is a library whose release history the tests share:
//
//   - release/1: techs/go/a and techs/go/b at 1.0.0, with a's own asset; practices/testing/c at 1.0.0; the empty
//     group techs/empty.
//   - release/2: a minor to 1.1.0, new techs/go/d at 1.0.0.
//   - release/3: b retired, replaced by d; a major to 2.0.0.
//
// Commits after release/3 are unreleased.
type history struct {
	fixture *gitfixture.Fixture
	commits map[int]string
}

// newHistory publishes the shared library history to a fixture repository.
func newHistory(t *testing.T) history {
	t.Helper()
	f := newLibraryFixture(t, map[string][]byte{
		"config.yaml":                   []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`),
		"techs/go/_group.yaml":          groupMetadata,
		"techs/go/a.md":                 versionedRule("a 1.0.0"),
		"techs/go/assets/a/diagram.bin": {1, 0},
		"techs/go/b.md":                 versionedRule("b 1.0.0"),
		"practices/testing/_group.yaml": groupMetadata,
		"practices/testing/c.md":        versionedRule("c 1.0.0"),
		"techs/empty/_group.yaml":       groupMetadata,
	})
	h := history{fixture: f, commits: map[int]string{}}
	h.release(t, 1, nil, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/a: 1.0.0\n  techs/go/b: 1.0.0\n  practices/testing/c: 1.0.0\nchanges:\n  techs/go/a: {change: new, summaries: [Add the rule.]}\n  techs/go/b: {change: new, summaries: [Add the rule.]}\n  practices/testing/c: {change: new, summaries: [Add the rule.]}\n")
	h.release(t, 2, map[string][]byte{"techs/go/a.md": versionedRule("a 1.1.0"), "techs/go/assets/a/diagram.bin": {1, 1}, "techs/go/d.md": versionedRule("d 1.0.0")},
		"formatVersion: 1\nrelease: 2\nrules:\n  techs/go/a: 1.1.0\n  techs/go/b: 1.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nchanges:\n  techs/go/a: {change: minor, from: 1.0.0, summaries: [Add an example.]}\n  techs/go/d: {change: new, summaries: [Add the rule.]}\n")
	h.release(t, 3, map[string][]byte{"techs/go/a.md": versionedRule("a 2.0.0"), "techs/go/b.md": nil},
		"formatVersion: 1\nrelease: 3\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nchanges:\n  techs/go/a: {change: major, from: 1.1.0, summaries: [Require more.]}\nretired:\n  techs/go/b: {lastVersion: 1.0.0, replacedBy: techs/go/d, summaries: [Covered by d.]}\n")
	return h
}

// release commits files, keyed by library path, to the fixture, when any, and publishes the library release number
// with record.
func (h history) release(t *testing.T, number int, files map[string][]byte, record string) {
	t.Helper()
	ctx := context.Background()
	commit, err := h.fixture.Commit(ctx, h.fixture.Worktree(), fmt.Sprintf("Release %d", number), gitfixture.Library(files))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.fixture.Release(ctx, number, record); err != nil {
		t.Fatal(err)
	}
	h.commits[number] = commit
}

// source parses a single source named team from its JSON fields, adding the fixture's repository.
func (h history) source(t *testing.T, fields string) rules.Configuration {
	t.Helper()
	raw := `{"schemaVersion":2,"sources":{"team":{"repository":` + quoteJSON(h.fixture.Repository) + `,` + fields + `}}}`
	config, err := rules.ParseConfiguration(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

// sync imports the configuration with an optional recorded snapshot, as project sync does.
func (h history) sync(t *testing.T, config rules.Configuration, recorded *library.Snapshot) (Library, error) {
	t.Helper()
	previous := map[string]library.Snapshot{}
	if recorded != nil {
		previous["team"] = *recorded
	}
	result, err := ImportLibraries(context.Background(), config, previous, Options{GitPath: h.fixture.GitPath, Environment: h.fixture.Environment})
	return result["team"], err
}

// versions summarizes each imported rule as version@release, or "unreleased".
func versions(snapshot library.Snapshot) map[string]string {
	result := map[string]string{}
	for id, rule := range snapshot.Rules {
		if rule.Version == nil {
			result[id] = "unreleased"
			continue
		}
		result[id] = fmt.Sprintf("%s@%d", rule.Version, rule.Release)
	}
	return result
}

// quoteJSON quotes text as a JSON string.
func quoteJSON(text string) string {
	data, _ := json.Marshal(text)
	return string(data)
}

// TestImport_NewSourceGetsEachRulesNewestVersionFromItsLibraryRelease stores every file at its library path.
func TestImport_NewSourceGetsEachRulesNewestVersionFromItsLibraryRelease(t *testing.T) {
	h := newHistory(t)
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := imported.Snapshot
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(snapshot), want)
	}
	if snapshot.Release != 3 || snapshot.Commit != h.commits[3] || snapshot.Rules["techs/go/d"].Commit != h.commits[2] {
		t.Fatalf("snapshot %+v", snapshot)
	}
	want := []string{"config.yaml", "techs/go/_group.yaml", "techs/go/a.md", "techs/go/assets/a/diagram.bin", "techs/go/d.md"}
	if got := slices.Sorted(maps.Keys(snapshot.Files)); !reflect.DeepEqual(got, want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	if string(snapshot.Files["techs/go/a.md"]) != string(versionedRule("a 2.0.0")) || len(imported.Warnings) != 0 {
		t.Fatalf("a.md %q, warnings %v", snapshot.Files["techs/go/a.md"], imported.Warnings)
	}
}

// TestImport_RecordedVersionsStayWhenNewerOnesExist restores a snapshot without adopting newer versions.
func TestImport_RecordedVersionsStayWhenNewerOnesExist(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	one, two := trailguide.RuleVersion{Major: 1}, trailguide.RuleVersion{Major: 1, Minor: 1}
	recorded := library.Snapshot{Repository: h.fixture.Repository, Release: 2, Commit: h.commits[2], Selection: config.Sources[0].Groups, Groups: []string{"techs/go"}, RuleSelection: []string{},
		Rules: map[string]library.ImportedRule{"techs/go/a": {Version: &two, Release: 2, Commit: h.commits[2]}, "techs/go/b": {Version: &one, Release: 1, Commit: h.commits[1]}}}
	imported, err := h.sync(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	// b stays although release/3 retired it, and d, added to the group after the record, doesn't join.
	if want := map[string]string{"techs/go/a": "1.1.0@2", "techs/go/b": "1.0.0@1"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	if imported.Snapshot.Release != 2 || string(imported.Snapshot.Files["techs/go/a.md"]) != string(versionedRule("a 1.1.0")) || string(imported.Snapshot.Files["techs/go/b.md"]) != string(versionedRule("b 1.0.0")) {
		t.Fatalf("snapshot %+v", imported.Snapshot)
	}
}

// TestImport_PinsMoveRulesUpAndDown chooses the pinned version for added or changed pins only.
func TestImport_PinsMoveRulesUpAndDown(t *testing.T) {
	h := newHistory(t)
	down, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/a","version":"1.0.0","reason":"Not ready."}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(down.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(down.Snapshot), want)
	}
	// A new source takes its library-wide files from the newest library release, and each rule's own files from the
	// library release that published its version.
	if down.Snapshot.Release != 3 || string(down.Snapshot.Files["techs/go/assets/a/diagram.bin"]) != string([]byte{1, 0}) {
		t.Fatalf("snapshot release %d, files %v", down.Snapshot.Release, slices.Sorted(maps.Keys(down.Snapshot.Files)))
	}
	up, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/a","version":"1.1.0","reason":"Ready for the example."}]}`), &down.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.1.0@2", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(up.Snapshot), want) || up.Snapshot.Release != 3 {
		t.Fatalf("versions %v, want %v; shared files from release %d, want 3", versions(up.Snapshot), want, up.Snapshot.Release)
	}
	// Removing the pin keeps the recorded version.
	unpinned, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), &up.Snapshot)
	if err != nil || versions(unpinned.Snapshot)["techs/go/a"] != "1.1.0@2" {
		t.Fatalf("versions %v, %v", versions(unpinned.Snapshot), err)
	}
	_, err = h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/a","version":"1.2.0","reason":"Typo."}]}`), &up.Snapshot)
	requireCode(t, err, "version-not-found")
	if !strings.Contains(err.Error(), "the rule never published version 1.2.0; check the pin. Its published versions, newest first: 2.0.0, 1.1.0, 1.0.0.") {
		t.Fatalf("the failure doesn't list the published versions: %v", err)
	}
}

// TestImport_NewlySelectedRulesGetTheirNewestVersion leaves recorded rules and adds only the new selection.
func TestImport_NewlySelectedRulesGetTheirNewestVersion(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	one := trailguide.RuleVersion{Major: 1}
	recorded := library.Snapshot{Repository: h.fixture.Repository, Release: 1, Commit: h.commits[1], Selection: config.Sources[0].Groups, Groups: []string{"techs/go"}, RuleSelection: []string{},
		Rules: map[string]library.ImportedRule{"techs/go/a": {Version: &one, Release: 1, Commit: h.commits[1]}}}
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"]}`), &recorded)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "practices/testing/c": "1.0.0@1"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	individual, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*","techs/go/d"]}`), &recorded)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(individual.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(individual.Snapshot), want)
	}
}

// TestImport_IndividuallySelectedRuleComesWithoutTheRestOfItsGroup brings its group metadata only.
func TestImport_IndividuallySelectedRuleComesWithoutTheRestOfItsGroup(t *testing.T) {
	h := newHistory(t)
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/d"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	if want := []string{"config.yaml", "techs/go/_group.yaml", "techs/go/d.md"}; !reflect.DeepEqual(slices.Sorted(maps.Keys(imported.Snapshot.Files)), want) {
		t.Fatalf("files %v", slices.Sorted(maps.Keys(imported.Snapshot.Files)))
	}
	if imported.Snapshot.Release != 3 || len(imported.Snapshot.Groups) != 0 || !reflect.DeepEqual(imported.Snapshot.RuleSelection, []string{"techs/go/d"}) {
		t.Fatalf("snapshot %+v", imported.Snapshot)
	}
}

// TestImport_EntriesNamingRetiredRulesWarnAndUnknownOnesFail covers rules, pins, and exclusions.
func TestImport_EntriesNamingRetiredRulesWarnAndUnknownOnesFail(t *testing.T) {
	h := newHistory(t)
	for _, test := range []struct {
		name, fields, warning, location string
	}{
		{"retired rules entry", `"rules":{"include":["techs/go/*","techs/go/b"]}`, "sources.team.rules.include names techs/go/b", ""},
		{"retired exclusion", `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/b","reason":"Old."}]}`, "sources.team.rules.exclude names techs/go/b", ""},
		{"retired pin", `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/b","version":"1.0.0","reason":"Keep."}]}`, "sources.team.rules.pins names techs/go/b", ""},
		{"unknown rules entry", `"rules":{"include":["techs/go/*","techs/go/missing"]}`, "", "sources.team.rules.include"},
		{"unknown exclusion", `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/missing","reason":"Typo."}]}`, "", "sources.team.rules.exclude.techs/go/missing"},
		{"exclusion of an unselected rule", `"rules":{"include":["techs/go/*"],"exclude":[{"id":"practices/testing/c","reason":"Not selected."}]}`, "", "sources.team.rules.exclude.practices/testing/c"},
		{"unknown pin", `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/missing","version":"1.0.0","reason":"Typo."}]}`, "", "sources.team.rules.pins.techs/go/missing"},
		{"exclusion of a retired rule outside the selection", `"rules":{"include":["practices/testing/*"],"exclude":[{"id":"techs/go/b","reason":"Old."}]}`, "", "sources.team.rules.exclude.techs/go/b"},
		{"pin of a retired rule outside the selection", `"rules":{"include":["practices/testing/*"],"pins":[{"id":"techs/go/b","version":"1.0.0","reason":"Keep."}]}`, "", "sources.team.rules.pins.techs/go/b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			imported, err := h.sync(t, h.source(t, test.fields), nil)
			if test.location != "" {
				var validation errs.ValidationError
				if !errors.As(err, &validation) || validation.ValidationLocation() != test.location {
					t.Fatalf("got %v, want a failure at %s", err, test.location)
				}
				return
			}
			if err != nil || len(imported.Warnings) != 1 || !strings.HasPrefix(imported.Warnings[0], test.warning) {
				t.Fatalf("warnings %v, %v", imported.Warnings, err)
			}
			if !slices.Equal(imported.Snapshot.RetiredRules, []string{"techs/go/b"}) {
				t.Fatalf("recorded retired rules %v, want techs/go/b", imported.Snapshot.RetiredRules)
			}
			if _, ok := imported.Snapshot.Rules["techs/go/b"]; ok {
				t.Fatal("imported a retired rule")
			}
		})
	}
}

// TestImport_PinnedRuleTheLibraryRetiredKeepsImporting keeps importing a pinned rule, at the version the record
// imported before the library retired it.
func TestImport_PinnedRuleTheLibraryRetiredKeepsImporting(t *testing.T) {
	h := newHistory(t)
	pinned := `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/b","version":"1.0.0","reason":"Keep."}]}`
	one := trailguide.RuleVersion{Major: 1}
	config := h.source(t, pinned)
	recorded := library.Snapshot{Repository: h.fixture.Repository, Release: 1, Commit: h.commits[1], Selection: config.Sources[0].Groups, Groups: []string{"techs/go"}, RuleSelection: []string{},
		Rules: map[string]library.ImportedRule{"techs/go/b": {Version: &one, Release: 1, Commit: h.commits[1]}}}
	// Selecting practices/testing makes this sync read the history, which knows b is retired.
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"],"pins":[{"id":"techs/go/b","version":"1.0.0","reason":"Keep."}]}`), &recorded)
	if err != nil || versions(imported.Snapshot)["techs/go/b"] != "1.0.0@1" || len(imported.Warnings) != 0 {
		t.Fatalf("versions %v, warnings %v, %v", versions(imported.Snapshot), imported.Warnings, err)
	}
}

// TestImport_RefToALibraryReleaseImportsWhatItPublished records each rule's version as that release lists it.
func TestImport_RefToALibraryReleaseImportsWhatItPublished(t *testing.T) {
	h := newHistory(t)
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"release/2"`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.1.0@2", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	if imported.Snapshot.Release != 2 || imported.Snapshot.Commit != h.commits[2] || len(imported.Warnings) != 0 {
		t.Fatalf("snapshot %+v, warnings %v", imported.Snapshot, imported.Warnings)
	}
	_, err = h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"release/9"`), nil)
	requireCode(t, err, "version-not-found")
}

// TestImport_RefToALibraryReleaseCommitDoesNotWarn names a release's commit by its SHA: the source records no
// library release, but every rule matches a published version, so nothing is unreleased to warn about.
func TestImport_RefToALibraryReleaseCommitDoesNotWarn(t *testing.T) {
	h := newHistory(t)
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"`+h.commits[2]+`"`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.1.0@2", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	if imported.Snapshot.Release != 0 || len(imported.Warnings) != 0 {
		t.Fatalf("release %d, warnings %v", imported.Snapshot.Release, imported.Warnings)
	}
}

// TestImport_RefToAnUnreleasedCommitWarnsAndRecordsNoVersionForChangedRules matches the rest to published versions.
func TestImport_RefToAnUnreleasedCommitWarnsAndRecordsNoVersionForChangedRules(t *testing.T) {
	h := newHistory(t)
	commit, err := h.fixture.Commit(context.Background(), h.fixture.Worktree(), "Unreleased change", gitfixture.Library(map[string][]byte{"techs/go/d.md": versionedRule("d unreleased")}))
	if err != nil {
		t.Fatal(err)
	}
	config := h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"`+commit+`"`)
	imported, err := h.sync(t, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "unreleased"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	if imported.Snapshot.Release != 0 || imported.Snapshot.Commit != commit || imported.Snapshot.Rules["techs/go/d"].Commit != commit {
		t.Fatalf("snapshot %+v", imported.Snapshot)
	}
	if len(imported.Warnings) != 1 || !strings.Contains(imported.Warnings[0], "Source team imports "+commit) || !strings.HasSuffix(imported.Warnings[0], "Unreleased rules: techs/go/d.") {
		t.Fatalf("warnings %v", imported.Warnings)
	}
	// Removing the ref keeps published versions and gives unreleased rules their newest version.
	removed, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), &imported.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(removed.Snapshot), want) || len(removed.Warnings) != 0 {
		t.Fatalf("versions %v, want %v; warnings %v", versions(removed.Snapshot), want, removed.Warnings)
	}
}

// TestImport_RefToADeletedLibraryReleaseFailsForNewlySelectedRules reports version-not-found, with recovery
// guidance, when the recorded ref's release tag is gone and a newly selected rule needs its release record.
func TestImport_RefToADeletedLibraryReleaseFailsForNewlySelectedRules(t *testing.T) {
	h := newHistory(t)
	first, err := h.sync(t, h.source(t, `"rules":{"include":["practices/testing/*"]},"ref":"release/2"`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.fixture.Command(context.Background(), "tag", "--delete", "release/2"); err != nil {
		t.Fatal(err)
	}
	restored, err := h.sync(t, h.source(t, `"rules":{"include":["practices/testing/*"]},"ref":"release/2"`), &first.Snapshot)
	if err != nil || restored.Snapshot.Commit != h.commits[2] {
		t.Fatalf("restoring without the tag: %+v, %v", restored.Snapshot, err)
	}
	_, err = h.sync(t, h.source(t, `"rules":{"include":["practices/testing/*","techs/go/*"]},"ref":"release/2"`), &first.Snapshot)
	requireCode(t, err, "version-not-found")
	if !strings.Contains(err.Error(), "change sources.team.ref") {
		t.Fatalf("no recovery guidance: %v", err)
	}
}

// TestImport_RefKeepsItsRecordedCommitAfterTheTagMoves restores the commit a tag named when it was recorded, also
// when the configuration writes the same ref another way, and keeps the recorded spelling of that ref.
func TestImport_RefKeepsItsRecordedCommitAfterTheTagMoves(t *testing.T) {
	h := newHistory(t)
	ctx := context.Background()
	if _, err := h.fixture.Command(ctx, "tag", "candidate", h.commits[1]); err != nil {
		t.Fatal(err)
	}
	config := h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"candidate"`)
	first, err := h.sync(t, config, nil)
	if err != nil || first.Snapshot.Commit != h.commits[1] {
		t.Fatalf("snapshot %+v, %v", first.Snapshot, err)
	}
	if _, err := h.fixture.Command(ctx, "tag", "--force", "candidate", h.commits[3]); err != nil {
		t.Fatal(err)
	}
	again, err := h.sync(t, config, &first.Snapshot)
	if err != nil || again.Snapshot.Commit != h.commits[1] || !reflect.DeepEqual(versions(again.Snapshot), versions(first.Snapshot)) {
		t.Fatalf("snapshot %+v, %v", again.Snapshot, err)
	}
	respelled, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"refs/tags/candidate"`), &first.Snapshot)
	if err != nil || respelled.Snapshot.Commit != h.commits[1] || respelled.Snapshot.Ref.String() != "candidate" {
		t.Fatalf("the same ref written another way: snapshot %+v, %v", respelled.Snapshot, err)
	}
}

// TestImport_ExclusionSelectorsMustMatchAnImportedRule imports the rules a selector excludes, since exclusions
// apply when guidance is built, and fails on a selector that matches none of the source's rules.
func TestImport_ExclusionSelectorsMustMatchAnImportedRule(t *testing.T) {
	h := newHistory(t)
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["*"],"exclude":[{"id":"techs/go/*","reason":"Not a Go project."}]}`), nil)
	if err != nil || len(imported.Warnings) != 0 {
		t.Fatalf("warnings %v, %v", imported.Warnings, err)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2", "practices/testing/c": "1.0.0@1"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	_, err = h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"practices/*","reason":"Typo."}]}`), nil)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.team.rules.exclude.practices/*" || !strings.Contains(validation.ValidationProblem(), "matches no rule this source imports") {
		t.Fatalf("got %v, want a failure at sources.team.rules.exclude.practices/*", err)
	}
}

// TestImport_KeptRuleWhoseGroupWasRemovedKeepsItsGroupMetadata takes a retained retired rule's group metadata
// from its own library release when the library-wide release no longer has the group.
func TestImport_KeptRuleWhoseGroupWasRemovedKeepsItsGroupMetadata(t *testing.T) {
	f := newLibraryFixture(t, map[string][]byte{
		"config.yaml":           []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`),
		"techs/old/_group.yaml": []byte(`{"name":"Old","description":"Old rules.","whenToRead":"Rarely."}`),
		"techs/old/x.md":        versionedRule("x 1.0.0"),
		"techs/go/_group.yaml":  groupMetadata,
		"techs/go/y.md":         versionedRule("y 1.0.0"),
	})
	h := history{fixture: f, commits: map[int]string{}}
	h.release(t, 1, nil, "formatVersion: 1\nrelease: 1\nrules:\n  techs/old/x: 1.0.0\n  techs/go/y: 1.0.0\nchanges:\n  techs/old/x: {change: new, summaries: [Add the rule.]}\n  techs/go/y: {change: new, summaries: [Add the rule.]}\n")
	first, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*","techs/old/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	h.release(t, 2, map[string][]byte{"techs/old/x.md": nil, "techs/old/_group.yaml": nil, "techs/go/y.md": versionedRule("y 1.1.0")},
		"formatVersion: 1\nrelease: 2\nrules:\n  techs/go/y: 1.1.0\nchanges:\n  techs/go/y: {change: minor, from: 1.0.0, summaries: [Add an example.]}\nretired:\n  techs/old/x: {lastVersion: 1.0.0, summaries: [No longer recommended.]}\n")
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*","techs/old/*"],"pins":[{"id":"techs/go/y","version":"1.1.0","reason":"Adopt."}]}`), &first.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/old/x": "1.0.0@1", "techs/go/y": "1.1.0@2"}; !reflect.DeepEqual(versions(imported.Snapshot), want) || imported.Snapshot.Release != 2 {
		t.Fatalf("versions %v, release %d", versions(imported.Snapshot), imported.Snapshot.Release)
	}
	if !strings.Contains(string(imported.Snapshot.Files["techs/old/_group.yaml"]), "Old rules.") {
		t.Fatalf("group metadata %q", imported.Snapshot.Files["techs/old/_group.yaml"])
	}
}

// TestImport_IgnoresUnselectedGroupMetadata fetches only the metadata of groups the source imports, so an
// unselected group whose _group.yaml is a submodule doesn't block the import.
func TestImport_IgnoresUnselectedGroupMetadata(t *testing.T) {
	ctx := context.Background()
	f := newLibraryFixture(t, map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`),
		"techs/go/_group.yaml": groupMetadata,
		"techs/go/y.md":        versionedRule("y 1.0.0"),
	})
	if _, err := f.Command(ctx, "update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("5", 40)+",.trailguide/rules/techs/rust/_group.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Command(ctx, "commit", "--quiet", "--message", "Add a submodule where metadata belongs"); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/y: 1.0.0\nchanges:\n  techs/go/y: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	h := history{fixture: f, commits: map[int]string{}}
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	if err != nil || versions(imported.Snapshot)["techs/go/y"] != "1.0.0@1" {
		t.Fatalf("versions %v, %v", versions(imported.Snapshot), err)
	}
}

// TestImport_RefusesALibraryReleaseDeclaringTermsInsideARulesVersion refuses a newer library release whose
// manifest declares a file in a pinned rule's asset directory as its license, rather than letting that library
// release replace the pinned version's file.
func TestImport_RefusesALibraryReleaseDeclaringTermsInsideARulesVersion(t *testing.T) {
	f := newLibraryFixture(t, map[string][]byte{
		"config.yaml":                       []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1}}`),
		"techs/go/_group.yaml":              groupMetadata,
		"techs/go/a.md":                     versionedRule("a 1.0.0"),
		"techs/go/assets/a/requirements.md": []byte("Original requirements.\n"),
		"techs/go/b.md":                     versionedRule("b 1.0.0"),
	})
	h := history{fixture: f, commits: map[int]string{}}
	h.release(t, 1, nil, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/a: 1.0.0\n  techs/go/b: 1.0.0\nchanges:\n  techs/go/a: {change: new, summaries: [Add the rule.]}\n  techs/go/b: {change: new, summaries: [Add the rule.]}\n")
	h.release(t, 2, map[string][]byte{
		"config.yaml":                       []byte(`{"schemaVersion":2,"sources":{},"library":{"formatVersion":1,"license":{"file":".trailguide/rules/techs/go/assets/a/requirements.md","notices":[]}}}`),
		"techs/go/assets/a/requirements.md": []byte("Replacement requirements.\n"),
		"techs/go/b.md":                     versionedRule("b 1.1.0"),
	}, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/a: 1.0.0\n  techs/go/b: 1.1.0\nchanges:\n  techs/go/b: {change: minor, from: 1.0.0, summaries: [Add an example.]}\n")
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/a","version":"1.0.0","reason":"Keep."}]}`), nil)
	var invalid errs.ValidationError
	if !errors.As(err, &invalid) || !strings.Contains(invalid.ValidationLocation(), ".trailguide/config.yaml: library.license.file") || !strings.Contains(invalid.ValidationProblem(), "rule's version") {
		t.Fatalf("imported %q with error %v; want the license declaration refused", imported.Snapshot.Files["techs/go/assets/a/requirements.md"], err)
	}
}

// TestImport_WildcardSelectsEveryGroupInScope imports each rule's newest version from every group, including
// empty ones.
func TestImport_WildcardSelectsEveryGroupInScope(t *testing.T) {
	h := newHistory(t)
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2", "practices/testing/c": "1.0.0@1"}; !reflect.DeepEqual(versions(imported.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(imported.Snapshot), want)
	}
	if want := []string{"practices/testing", "techs/empty", "techs/go"}; !reflect.DeepEqual(imported.Snapshot.Groups, want) {
		t.Fatalf("groups %v, want %v", imported.Snapshot.Groups, want)
	}
}

// TestImport_SourceWithoutRulesGetsTheNewestLibraryRelease takes library-wide files from the newest library
// release, and a later sync keeps the recorded one.
func TestImport_SourceWithoutRulesGetsTheNewestLibraryRelease(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/empty/*"]}`)
	imported, err := h.sync(t, config, nil)
	if err != nil || imported.Snapshot.Release != 3 || imported.Snapshot.Commit != h.commits[3] || len(imported.Snapshot.Rules) != 0 {
		t.Fatalf("snapshot %+v, %v", imported.Snapshot, err)
	}
	h.release(t, 4, nil, "formatVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nlibraryFiles:\n  - techs/empty/_group.yaml\n")
	again, err := h.sync(t, config, &imported.Snapshot)
	if err != nil || again.Snapshot.Release != 3 {
		t.Fatalf("snapshot %+v, %v", again.Snapshot, err)
	}
}

// TestImport_RefSelectsIndividualRulesAtItsRevision warns about entries naming rules the revision retired,
// including an exclusion when an update to a changed ref retires its rule.
func TestImport_RefSelectsIndividualRulesAtItsRevision(t *testing.T) {
	h := newHistory(t)
	earlier, err := h.sync(t, h.source(t, `"rules":{"include":["practices/testing/*","techs/go/b"],"exclude":[{"id":"techs/go/b","reason":"Replaced."}]},"ref":"release/2"`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/b": "1.0.0@1", "practices/testing/c": "1.0.0@1"}; !reflect.DeepEqual(versions(earlier.Snapshot), want) || len(earlier.Warnings) != 0 {
		t.Fatalf("versions %v, want %v; warnings %v", versions(earlier.Snapshot), want, earlier.Warnings)
	}
	config := h.source(t, `"rules":{"include":["practices/testing/*","techs/go/b"],"exclude":[{"id":"techs/go/b","reason":"Replaced."}]},"ref":"release/3"`)
	update, err := h.plan(t, config, &earlier.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	later := h.install(t, update, config)
	if want := map[string]string{"practices/testing/c": "1.0.0@1"}; !reflect.DeepEqual(versions(later.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(later.Snapshot), want)
	}
	if len(later.Warnings) != 2 || !strings.HasPrefix(later.Warnings[0], "sources.team.rules.include names techs/go/b") || !strings.HasPrefix(later.Warnings[1], "sources.team.rules.exclude names techs/go/b") {
		t.Fatalf("warnings %v", later.Warnings)
	}
}

// TestImport_WithoutLibraryReleasesFailsWithoutARef reports ref-required before the first library release.
func TestImport_WithoutLibraryReleasesFailsWithoutARef(t *testing.T) {
	f := newLibraryFixture(t, libraryFiles())
	config := libraryConfig(t, f.Repository)
	config.Sources[0].Ref = rules.GitRef{}
	_, err := ImportLibraries(context.Background(), config, nil, Options{GitPath: f.GitPath, Environment: f.Environment})
	requireCode(t, err, "ref-required")
	if !strings.Contains(err.Error(), "Set sources.team.ref to the branch, tag, or full commit SHA") {
		t.Fatalf("got %v; want the ref to set", err)
	}
}

// TestImport_RejectsAReleaseTagWithoutARecord refuses a lightweight or unparseable release/<number> tag.
func TestImport_RejectsAReleaseTagWithoutARecord(t *testing.T) {
	for name, tag := range map[string][]string{"lightweight": {"tag", "release/4"}, "no record": {"tag", "--annotate", "--message", "Notes only.", "release/4"}} {
		t.Run(name, func(t *testing.T) {
			h := newHistory(t)
			if _, err := h.fixture.Command(context.Background(), tag...); err != nil {
				t.Fatal(err)
			}
			_, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
			requireCode(t, err, "invalid-release-tag")
		})
	}
}

// TestImport_AsksToUpgradeForANewerReleaseRecordFormat refuses a release record written in a later format with
// its own code, telling the user to upgrade Trail Guide, and reads a record with fields it doesn't know.
func TestImport_AsksToUpgradeForANewerReleaseRecordFormat(t *testing.T) {
	h := newHistory(t)
	record := "release: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nsignedBy: {name: A later field}\n"
	h.release(t, 4, nil, "formatVersion: 1\n"+record)
	if _, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil); err != nil {
		t.Fatal(err)
	}
	h.release(t, 5, nil, "formatVersion: 2\n"+strings.Replace(record, "release: 4", "release: 5", 1))
	_, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	requireCode(t, err, "unsupported-release-record")
	if !strings.Contains(err.Error(), "release/5 uses release record format 2") || !strings.Contains(err.Error(), "Upgrade Trail Guide") {
		t.Fatalf("got %v", err)
	}
}

// TestImport_RejectsAReleaseTagOfSomethingOtherThanACommit refuses a release tag with a valid record whose target
// is a tree or a blob, so a non-commit never becomes a recorded commit.
func TestImport_RejectsAReleaseTagOfSomethingOtherThanACommit(t *testing.T) {
	for _, target := range []string{"HEAD^{tree}", "HEAD:.trailguide/config.yaml"} {
		t.Run(target, func(t *testing.T) {
			h := newHistory(t)
			ctx := context.Background()
			object, err := h.fixture.Command(ctx, "rev-parse", target)
			if err != nil {
				t.Fatal(err)
			}
			record := "formatVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\n"
			if _, err := h.fixture.Command(ctx, "tag", "--annotate", "--cleanup=verbatim", "--message", "Notes.\n\n---\n"+record, "release/4", object); err != nil {
				t.Fatal(err)
			}
			_, err = h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
			requireCode(t, err, "invalid-release-tag")
		})
	}
}

// TestImport_IgnoresOtherTagsUnderRelease reads only tags named release/<number>.
func TestImport_IgnoresOtherTagsUnderRelease(t *testing.T) {
	h := newHistory(t)
	for _, name := range []string{"release/04", "release/next", "v2"} {
		if _, err := h.fixture.Command(context.Background(), "tag", name); err != nil {
			t.Fatal(err)
		}
	}
	imported, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"]}`), nil)
	if err != nil || imported.Snapshot.Release != 3 {
		t.Fatalf("snapshot %+v, %v", imported.Snapshot, err)
	}
}

// TestImport_FailsWhenARecordedLibraryReleaseMoved refuses to mix a recorded commit with a moved tag.
func TestImport_FailsWhenARecordedLibraryReleaseMoved(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	first, err := h.sync(t, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	object, err := h.fixture.Command(context.Background(), "cat-file", "tag", "release/2")
	if err != nil {
		t.Fatal(err)
	}
	_, message, _ := strings.Cut(object, "\n\n")
	if _, err := h.fixture.Command(context.Background(), "tag", "--force", "--annotate", "--cleanup=verbatim", "--message", message+"\n", "release/2", h.commits[3]); err != nil {
		t.Fatal(err)
	}
	_, err = h.sync(t, h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"]}`), &first.Snapshot)
	requireCode(t, err, "invalid-release-tag")
}

// TestImport_RefusesAHostileReleaseRecordWithinTheDeadline refuses a small release tag whose record lists 100,000
// library-wide files, reporting the limit instead of spending longer than the import's deadline on it.
func TestImport_RefusesAHostileReleaseRecordWithinTheDeadline(t *testing.T) {
	h := newHistory(t)
	var message strings.Builder
	message.WriteString("Library release 4.\n\n---\nformatVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nlibraryFiles:\n")
	for i := range 100_000 {
		fmt.Fprintf(&message, "  - assets/f%d.md\n", i)
	}
	// The message is too long for a command-line argument.
	file := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(file, []byte(message.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.fixture.Command(context.Background(), "tag", "--annotate", "--cleanup=verbatim", "--file="+file, "release/4"); err != nil {
		t.Fatal(err)
	}
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	_, err := ImportLibraries(context.Background(), config, nil, Options{GitPath: h.fixture.GitPath, Environment: h.fixture.Environment, Timeout: 10 * time.Second})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != "invalid-release-tag" || !strings.Contains(failure.Problem, "libraryFiles: expected at most 20,000 entries") {
		t.Fatalf("got %v", err)
	}
}

// TestImport_ReleaseTagListingLimit refuses more release tags than the documented listing limit.
func TestImport_ReleaseTagListingLimit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	listing := `awk 'BEGIN { for (i = 1; i <= 20001; i++) printf "%040d\trefs/tags/release/%d\n", i, i }'`
	body := "#!/bin/sh\nfor arg in \"$@\"; do\n  if [ \"$arg\" = ls-remote ]; then " + listing + "; exit 0; fi\n  if [ \"$arg\" = --version ]; then echo 'git version 2.45.0'; exit 0; fi\ndone\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	repo, err := openRepository(context.Background(), rules.Source{Name: "team", Repository: "https://example.invalid/rules.git"}, Options{GitPath: script})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	_, err = repo.loadHistory(context.Background())
	requireCode(t, err, "limit-exceeded")
}

// TestImport_WarnsAboutSelectorsAnotherSelectorCovers names each include entry that a group selector or wildcard
// in the same list already covers, since the entry changes nothing.
func TestImport_WarnsAboutSelectorsAnotherSelectorCovers(t *testing.T) {
	h := newHistory(t)
	for _, test := range []struct {
		include string
		want    []string
	}{
		{`["techs/go/*","techs/go/a","practices/testing/c"]`, []string{"sources.team.rules.include names techs/go/a, a rule that techs/go/* in the same list already selects, so the entry changes nothing; delete it."}},
		{`["techs/*","techs/go/a","practices/testing/c"]`, []string{"sources.team.rules.include names techs/go/a, a rule that techs/* in the same list already selects, so the entry changes nothing; delete it."}},
		{`["*","techs/go/*"]`, []string{`sources.team.rules.include names techs/go/*, a group that "*" in the same list already selects, so the entry changes nothing; delete it.`}},
		{`["practices/*","techs/go/*"]`, []string{}},
	} {
		imported, err := h.sync(t, h.source(t, `"rules":{"include":`+test.include+`}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(imported.Warnings, test.want) {
			t.Errorf("%s: warnings %q, want %q", test.include, imported.Warnings, test.want)
		}
	}
}

// TestVersionList_ListsAtMostTenVersionsNewestFirst and counts the older ones.
func TestVersionList_ListsAtMostTenVersionsNewestFirst(t *testing.T) {
	history := releaseHistory{}
	for number := 1; number <= 12; number++ {
		version := trailguide.RuleVersion{Major: 1, Minor: number - 1}
		history.releases = append(history.releases, libraryRelease{number: number, record: trailguide.ReleaseRecord{Rules: map[string]trailguide.RuleVersion{"techs/go/a": version}, Changes: map[string]trailguide.RecordedChange{"techs/go/a": {Change: trailguide.ChangeMinor}}}})
	}
	if got, want := history.versionList("techs/go/a"), "1.11.0, 1.10.0, 1.9.0, 1.8.0, 1.7.0, 1.6.0, 1.5.0, 1.4.0, 1.3.0, 1.2.0, and 2 older"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	history.releases = history.releases[:10]
	if got := history.versionList("techs/go/a"); strings.Contains(got, "older") || !strings.HasSuffix(got, "1.0.0") {
		t.Fatalf("ten versions: %q", got)
	}
}

// TestImport_ExclusionOfADeselectedRetiredRuleOnlyWarns: the last sync imported and excluded b, which release/3
// retired; deselecting techs/go keeps the exclusion a warning, now and in later syncs, and the record lists b as
// retired, so offline checks accept the exclusion too.
func TestImport_ExclusionOfADeselectedRetiredRuleOnlyWarns(t *testing.T) {
	h := newHistory(t)
	excluded := `"exclude":[{"id":"techs/go/b","reason":"Not used."}]`
	recorded := h.record(t, h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"],`+excluded+`}`), 1, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1", "practices/testing/c": "1.0.0@1"})
	config := h.source(t, `"rules":{"include":["practices/testing/*"],`+excluded+`}`)
	snapshot := recorded
	for sync := 1; sync <= 2; sync++ {
		imported, err := h.sync(t, config, &snapshot)
		if err != nil {
			t.Fatalf("sync %d: %v", sync, err)
		}
		if len(imported.Warnings) != 1 || !strings.HasPrefix(imported.Warnings[0], "sources.team.rules.exclude names techs/go/b") || !slices.Contains(imported.Snapshot.RetiredRules, "techs/go/b") {
			t.Fatalf("sync %d: warnings %q, retired %v", sync, imported.Warnings, imported.Snapshot.RetiredRules)
		}
		snapshot = imported.Snapshot
	}
}

// TestImport_RecordsTheRetiredRulesOfAnExclusionSelector: the last sync selected techs/empty while it had no rules;
// the library later published and retired x in it. Adding an exclusion of techs/empty/* to an otherwise unchanged
// selection only warns, and the record lists x as retired, so offline checks accept the selector too.
func TestImport_RecordsTheRetiredRulesOfAnExclusionSelector(t *testing.T) {
	h := newHistory(t)
	include := `"rules":{"include":["techs/go/*","techs/empty/*"]`
	recorded, err := h.sync(t, h.source(t, include+`}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	h.release(t, 4, map[string][]byte{"techs/empty/x.md": versionedRule("x 1.0.0")},
		"formatVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  techs/empty/x: 1.0.0\n  practices/testing/c: 1.0.0\nchanges:\n  techs/empty/x: {change: new, summaries: [Add the rule.]}\n")
	h.release(t, 5, map[string][]byte{"techs/empty/x.md": nil},
		"formatVersion: 1\nrelease: 5\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nretired:\n  techs/empty/x: {lastVersion: 1.0.0, summaries: [No longer recommended.]}\n")
	imported, err := h.sync(t, h.source(t, include+`,"exclude":[{"id":"techs/empty/*","reason":"Old."}]}`), &recorded.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Warnings) != 1 || !strings.HasPrefix(imported.Warnings[0], "sources.team.rules.exclude names techs/empty/*, which matches only rules the library retired") {
		t.Fatalf("warnings %q", imported.Warnings)
	}
	if imported.Snapshot.Release != recorded.Snapshot.Release || !slices.Equal(imported.Snapshot.RetiredRules, []string{"techs/empty/x", "techs/go/b"}) {
		t.Fatalf("release %d, retired rules %v; want release %d with techs/empty/x and techs/go/b", imported.Snapshot.Release, imported.Snapshot.RetiredRules, recorded.Snapshot.Release)
	}
}

// TestImport_RejectsABasedOnVersionTheRuleNeverPublished, listing the versions it did publish, newest first.
func TestImport_RejectsABasedOnVersionTheRuleNeverPublished(t *testing.T) {
	h := newHistory(t)
	_, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/a","reason":"Ours.","replacedBy":"techs/go/a","basedOn":"3.0.0"}]}`), nil)
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != "version-not-found" || !strings.Contains(err.Error(), "sources.team.rules.exclude.techs/go/a.basedOn: the rule never published version 3.0.0; check basedOn. Its published versions, newest first: 2.0.0, 1.1.0, 1.0.0.") {
		t.Fatalf("got %v", err)
	}
	if _, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/a","reason":"Ours.","replacedBy":"techs/go/a","basedOn":"1.1.0"}]}`), nil); err != nil {
		t.Fatal(err)
	}
}

// TestImport_RefKeepsItsRecordedCommitAfterAReleaseTagMoves: a source whose ref names release/1, or a commit whose
// rules record versions from release/1, keeps importing its recorded commit after release/1 moves to another
// commit, while its original commit stays fetchable.
func TestImport_RefKeepsItsRecordedCommitAfterAReleaseTagMoves(t *testing.T) {
	for _, test := range []struct{ name, ref string }{{"library release", "release/1"}, {"commit", ""}} {
		t.Run(test.name, func(t *testing.T) {
			h := newHistory(t)
			ctx := context.Background()
			ref := test.ref
			if ref == "" {
				ref = h.commits[1]
			}
			config := h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"`+ref+`"`)
			first, err := h.sync(t, config, nil)
			if err != nil || first.Snapshot.Commit != h.commits[1] {
				t.Fatalf("snapshot %+v, %v", first.Snapshot, err)
			}
			message, err := h.fixture.Command(ctx, "tag", "--list", "--format=%(contents)", "release/1")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.fixture.Command(ctx, "tag", "--force", "--annotate", "--cleanup=verbatim", "--message", message+"\n", "release/1", h.commits[3]); err != nil {
				t.Fatal(err)
			}
			again, err := h.sync(t, config, &first.Snapshot)
			if err != nil || again.Snapshot.Commit != h.commits[1] || !reflect.DeepEqual(versions(again.Snapshot), versions(first.Snapshot)) {
				t.Fatalf("after release/1 moved: snapshot %+v, %v", again.Snapshot, err)
			}
		})
	}
}

// A branch's warning lists as changed since the last release a rule reverted to an older published version, as
// well as an unreleased one, and leaves out a rule at the version the newest library release lists.
func TestUnreleasedWarning_ListsRulesThatDifferFromTheNewestRelease(t *testing.T) {
	version := func(text string) *trailguide.RuleVersion {
		parsed, err := trailguide.ParseRuleVersion(text, "version")
		if err != nil {
			t.Fatal(err)
		}
		return &parsed
	}
	ref, err := rules.ParseGitRef("main", "ref")
	if err != nil {
		t.Fatal(err)
	}
	imported := map[string]library.ImportedRule{
		"techs/go/current":  {Version: version("2.0.0")},
		"techs/go/reverted": {Version: version("1.0.0")},
		"techs/go/new":      {},
	}
	current := map[string]trailguide.RuleVersion{"techs/go/current": *version("2.0.0"), "techs/go/reverted": *version("1.1.0")}
	warning := unreleasedWarning(rules.Source{Name: "team", Ref: ref}, imported, true, current)
	if want := " Rules changed since the last release: techs/go/new, techs/go/reverted."; !strings.HasSuffix(warning, want) {
		t.Fatalf("warning = %q; want it to end %q", warning, want)
	}
}
