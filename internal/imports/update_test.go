// Exercise planning project update against real release tags: preview rows, scopes, pins, and the installed lock.

package imports

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// record returns a snapshot of config's only source that imports each rule at its version@release.
func (h history) record(t *testing.T, config rules.Configuration, release int, imported map[string]string) library.Snapshot {
	t.Helper()
	source := config.Sources[0]
	snapshot := library.Snapshot{Repository: source.Repository, Release: release, Commit: h.commits[release], Selection: source.Groups, Groups: []string{}, RuleSelection: source.Rules, Rules: map[string]library.ImportedRule{}}
	for id, text := range imported {
		version, number, _ := strings.Cut(text, "@")
		parsed, err := trailguide.ParseRuleVersion(version, id)
		if err != nil {
			t.Fatal(err)
		}
		n, err := strconv.Atoi(number)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Rules[id] = library.ImportedRule{Version: &parsed, Release: n, Commit: h.commits[n]}
	}
	return snapshot
}

// plan plans an update of config from recorded, which may be nil.
func (h history) plan(t *testing.T, config rules.Configuration, recorded *library.Snapshot, targets ...UpdateTarget) (Update, error) {
	t.Helper()
	previous := map[string]library.Snapshot{}
	if recorded != nil {
		previous["team"] = *recorded
	}
	return PlanUpdate(context.Background(), config, previous, targets, Options{GitPath: h.fixture.GitPath, Environment: h.fixture.Environment})
}

// rows describes each preview row of the only source on one line: change, ID, versions, and details.
func rows(update Update) []string {
	result := []string{}
	for _, row := range update.Sources[0].Rules {
		line := string(row.Change) + " " + row.ID
		for _, version := range []struct {
			label string
			value *trailguide.RuleVersion
		}{{"from", row.From}, {"to", row.To}, {"newest", row.Newest}, {"last", row.LastVersion}} {
			if version.value != nil {
				line += " " + version.label + " " + version.value.String()
			}
		}
		if row.ReplacedBy != "" {
			line += " replacedBy " + row.ReplacedBy
		}
		if row.BasedOn != nil {
			line += " basedOn " + row.BasedOn.String()
		}
		if row.LocalRule != "" {
			line += " local " + row.LocalRule
		}
		if row.Pin != nil {
			line += " pin " + row.Pin.Version.String() + " (" + row.Pin.Reason + ")"
		}
		result = append(result, line+": "+strings.Join(row.Summaries, " | "))
	}
	return result
}

// install imports config as the update plans it, as project update does after its preview.
func (h history) install(t *testing.T, update Update, config rules.Configuration) Library {
	t.Helper()
	result, err := update.Import(context.Background(), config, Options{GitPath: h.fixture.GitPath, Environment: h.fixture.Environment}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result["team"]
}

// TestPlanUpdate_PreviewsMajorNewAndRetiredRulesAndInstallsThem moves a project from release/1 to release/3.
func TestPlanUpdate_PreviewsMajorNewAndRetiredRulesAndInstallsThem(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"]}`)
	recorded := h.record(t, config, 1, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1", "practices/testing/c": "1.0.0@1"})
	update, err := h.plan(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"major techs/go/a from 1.0.0 to 2.0.0: Add an example. | Require more.",
		"new techs/go/d to 1.0.0: Add the rule.",
		"retired techs/go/b from 1.0.0 last 1.0.0 replacedBy techs/go/d: Covered by d.",
	}
	if got := rows(update); !reflect.DeepEqual(got, want) || len(update.Warnings) != 0 {
		t.Fatalf("rows:\n%s\nwant:\n%s\nwarnings %v", strings.Join(got, "\n"), strings.Join(want, "\n"), update.Warnings)
	}
	installed := h.install(t, update, config)
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2", "practices/testing/c": "1.0.0@1"}; !reflect.DeepEqual(versions(installed.Snapshot), want) {
		t.Fatalf("installed %v, want %v", versions(installed.Snapshot), want)
	}
	if installed.Snapshot.Release != 3 || string(installed.Snapshot.Files["techs/go/a.md"]) != string(versionedRule("a 2.0.0")) {
		t.Fatalf("snapshot release %d, a.md %q", installed.Snapshot.Release, installed.Snapshot.Files["techs/go/a.md"])
	}
}

// TestPlanUpdate_SelectorsReachRulesTheLibraryAddsLater previews and installs a rule the library added to a group
// that a wildcard selects, and installs without asking about one that a selector also excludes.
func TestPlanUpdate_SelectorsReachRulesTheLibraryAddsLater(t *testing.T) {
	h := newHistory(t)
	for _, test := range []struct {
		fields string
		rows   []string
	}{
		{`"rules":{"include":["techs/*"]}`, []string{"new techs/go/d to 1.0.0: Add the rule."}},
		{`"rules":{"include":["*"],"exclude":[{"id":"techs/go/*","reason":"Not a Go project."}]}`, []string{}},
	} {
		config := h.source(t, test.fields)
		recorded := h.record(t, config, 1, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1", "practices/testing/c": "1.0.0@1"})
		update, err := h.plan(t, config, &recorded)
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, row := range rows(update) {
			if strings.HasPrefix(row, "new ") {
				got = append(got, row)
			}
		}
		if !reflect.DeepEqual(got, test.rows) {
			t.Errorf("%s: new rows %q, want %q", test.fields, got, test.rows)
		}
		if installed := h.install(t, update, config); installed.Snapshot.Rules["techs/go/d"].Version == nil {
			t.Errorf("%s: installed %v without techs/go/d", test.fields, versions(installed.Snapshot))
		}
	}
}

// TestPlanUpdate_KeptRulesStayAtTheirVersionsWhenPinned installs the planned update with the pins that --keep
// writes, keeping a major change and a retirement where they were.
func TestPlanUpdate_KeptRulesStayAtTheirVersionsWhenPinned(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	recorded := h.record(t, config, 1, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1"})
	update, err := h.plan(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	kept := h.source(t, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/a","version":"1.0.0","reason":"Not ready."},{"id":"techs/go/b","version":"1.0.0","reason":"Still useful."}]}`)
	installed := h.install(t, update, kept)
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(installed.Snapshot), want) || len(installed.Warnings) != 0 {
		t.Fatalf("installed %v, want %v; warnings %v", versions(installed.Snapshot), want, installed.Warnings)
	}
	if installed.Snapshot.Release != 3 {
		t.Fatalf("library-wide files from release %d, want the newest library release, 3, although no rule version comes from it", installed.Snapshot.Release)
	}
	// The next update lists both pins: the major change as pinned, and the retirement the pin keeps.
	again, err := h.plan(t, kept, &installed.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"retired techs/go/b from 1.0.0 last 1.0.0 replacedBy techs/go/d pin 1.0.0 (Still useful.): Covered by d.",
		"pinned techs/go/a from 1.0.0 newest 2.0.0 pin 1.0.0 (Not ready.): ",
	}
	if got := rows(again); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !reflect.DeepEqual(versions(h.install(t, again, kept).Snapshot), versions(installed.Snapshot)) {
		t.Fatal("an update of pinned rules moved them")
	}
}

// TestPlanUpdate_ReplacedAndExcludedRules lists a replaced rule's new version with the local rule, hides changes to
// a rule excluded without a replacement, and still moves both.
func TestPlanUpdate_ReplacedAndExcludedRules(t *testing.T) {
	h := newHistory(t)
	h.release(t, 4, map[string][]byte{"techs/go/d.md": versionedRule("d 1.0.1"), "practices/testing/c.md": versionedRule("c 1.1.0")},
		"schemaVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.1\n  practices/testing/c: 1.1.0\nchanges:\n  techs/go/d: {change: patch, from: 1.0.0, summaries: [Fix a typo.]}\n  practices/testing/c: {change: minor, from: 1.0.0, summaries: [Add an example.]}\n")
	config := h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"],"exclude":[{"id":"techs/go/d","reason":"Ours is stricter.","replacedBy":"techs/go/d"},{"id":"practices/testing/c","reason":"Not used."}]}`)
	recorded := h.record(t, config, 3, map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2", "practices/testing/c": "1.0.0@1"})
	update, err := h.plan(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rows(update), []string{"replaced techs/go/d from 1.0.0 to 1.0.1 local rules/techs/go/d.md: Fix a typo."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows %q, want %q", got, want)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.1@4", "practices/testing/c": "1.1.0@4"}; !reflect.DeepEqual(versions(h.install(t, update, config).Snapshot), want) {
		t.Fatalf("installed %v, want %v", versions(h.install(t, update, config).Snapshot), want)
	}
}

// TestPlanUpdate_ComparesAReplacementWithTheVersionItIsBasedOn: a replaced rule's row lists every change after the
// version its replacement is based on, whatever version the project imports, and none when the replacement is
// based on the newest version, while the imported copy still moves to it. A replaced rule the library retired gets a
// retired row, as other retired rules do.
func TestPlanUpdate_ComparesAReplacementWithTheVersionItIsBasedOn(t *testing.T) {
	h := newHistory(t)
	for _, test := range []struct {
		name, rule, basedOn, imported string
		want                          []string
		versions                      []string
	}{
		{"based on the newest version, importing an older one", "techs/go/a", "2.0.0", "1.0.0@1", []string{}, nil},
		{"based on an older version than it imports", "techs/go/a", "1.0.0", "2.0.0@3", []string{"replaced techs/go/a from 2.0.0 to 2.0.0 basedOn 1.0.0 local rules/techs/go/a.md: Add an example. | Require more."}, []string{"1.1.0", "2.0.0"}},
		{"based on a newer version than it imports", "techs/go/a", "1.1.0", "1.0.0@1", []string{"replaced techs/go/a from 1.0.0 to 2.0.0 basedOn 1.1.0 local rules/techs/go/a.md: Require more."}, []string{"2.0.0"}},
		{"retired upstream", "techs/go/b", "1.0.0", "1.0.0@1", []string{"retired techs/go/b from 1.0.0 last 1.0.0 replacedBy techs/go/d: Covered by d."}, []string{"1.0.0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := h.source(t, `"rules":{"include":["`+test.rule+`"],"exclude":[{"id":"`+test.rule+`","reason":"Ours.","replacedBy":"`+test.rule+`","basedOn":"`+test.basedOn+`"}]}`)
			recorded := h.record(t, config, 3, map[string]string{test.rule: test.imported})
			update, err := h.plan(t, config, &recorded)
			if err != nil {
				t.Fatal(err)
			}
			if got := rows(update); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("rows %q, want %q", got, test.want)
			}
			if len(test.want) > 0 {
				got := []string{}
				for _, version := range update.Sources[0].Rules[0].SummaryVersions {
					got = append(got, version.String())
				}
				if !reflect.DeepEqual(got, test.versions) {
					t.Fatalf("summary versions %v, want %v", got, test.versions)
				}
			}
			if test.rule == "techs/go/a" && versions(h.install(t, update, config).Snapshot)["techs/go/a"] != "2.0.0@3" {
				t.Fatal("the update didn't move the imported copy to the newest version")
			}
		})
	}
}

// TestPlanUpdate_PatchChangeAndEveryIntermediateSummary lists each version's summary, oldest first.
func TestPlanUpdate_PatchChangeAndEveryIntermediateSummary(t *testing.T) {
	h := newHistory(t)
	h.release(t, 4, map[string][]byte{"practices/testing/c.md": versionedRule("c 1.0.1")},
		"schemaVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.1\nchanges:\n  practices/testing/c: {change: patch, from: 1.0.0, summaries: [Fix a typo., Clarify a sentence.]}\n")
	h.release(t, 5, map[string][]byte{"practices/testing/c.md": versionedRule("c 1.0.2")},
		"schemaVersion: 1\nrelease: 5\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.2\nchanges:\n  practices/testing/c: {change: patch, from: 1.0.1, summaries: [Fix a link.]}\n")
	config := h.source(t, `"rules":{"include":["practices/testing/*"]}`)
	recorded := h.record(t, config, 1, map[string]string{"practices/testing/c": "1.0.0@1"})
	update, err := h.plan(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rows(update), []string{"patch practices/testing/c from 1.0.0 to 1.0.2: Fix a typo. | Clarify a sentence. | Fix a link."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows %q, want %q", got, want)
	}
}

// TestPlanUpdate_ScopedToOneRuleMovesOnlyThatRule leaves other rules, retirements, and new rules for later.
func TestPlanUpdate_ScopedToOneRuleMovesOnlyThatRule(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	recorded := h.record(t, config, 1, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1"})
	update, err := h.plan(t, config, &recorded, UpdateTarget{Source: "team", Rule: "techs/go/a"})
	if err != nil {
		t.Fatal(err)
	}
	if got := rows(update); len(got) != 1 || !strings.HasPrefix(got[0], "major techs/go/a from 1.0.0 to 2.0.0") {
		t.Fatalf("rows %q", got)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/b": "1.0.0@1"}; !reflect.DeepEqual(versions(h.install(t, update, config).Snapshot), want) {
		t.Fatalf("installed %v, want %v", versions(h.install(t, update, config).Snapshot), want)
	}
	whole, err := h.plan(t, config, &recorded, UpdateTarget{Source: "team", Rule: "techs/go/a"}, UpdateTarget{Source: "team"})
	if err != nil || len(rows(whole)) != 3 {
		t.Fatalf("a source target didn't include every rule: %q, %v", rows(whole), err)
	}
}

// TestPlanUpdate_StartsFromWhatSyncWouldImport doesn't preview a newly selected group's rules as new, since sync
// adds them on its own.
func TestPlanUpdate_StartsFromWhatSyncWouldImport(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	recorded := h.record(t, config, 3, map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"})
	update, err := h.plan(t, h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"]}`), &recorded)
	if err != nil || len(rows(update)) != 0 {
		t.Fatalf("rows %q, %v", rows(update), err)
	}
	fresh, err := h.plan(t, config, nil)
	if err != nil || len(rows(fresh)) != 0 {
		t.Fatalf("a source without a record previewed %q, %v", rows(fresh), err)
	}
}

// TestPlanUpdate_WarnsAboutEntriesTheUpdateLeavesNamingRetiredRules previews the retirement of an individually
// selected rule and warns, as sync will, that its entry and an exclusion no longer do anything.
func TestPlanUpdate_WarnsAboutEntriesTheUpdateLeavesNamingRetiredRules(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["practices/testing/*","techs/go/b"]}`)
	recorded := h.record(t, config, 1, map[string]string{"techs/go/b": "1.0.0@1", "practices/testing/c": "1.0.0@1"})
	update, err := h.plan(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows(update); len(got) != 1 || !strings.HasPrefix(got[0], "retired techs/go/b") {
		t.Fatalf("rows %q", got)
	}
	if len(update.Warnings) != 1 || !strings.HasPrefix(update.Warnings[0], "sources.team.rules.include names techs/go/b") {
		t.Fatalf("warnings %v", update.Warnings)
	}
	installed := h.install(t, update, config)
	if !reflect.DeepEqual(installed.Warnings, update.Warnings) {
		t.Fatalf("installing warned %v, the preview %v", installed.Warnings, update.Warnings)
	}
	excluded := h.source(t, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/b","reason":"Not used."}]}`)
	recorded = h.record(t, excluded, 1, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1"})
	update, err = h.plan(t, excluded, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows(update) {
		if strings.Contains(row, "techs/go/b") {
			t.Fatalf("previewed a change to a rule excluded without a replacement: %q", row)
		}
	}
	if len(update.Warnings) != 1 || !strings.HasPrefix(update.Warnings[0], "sources.team.rules.exclude names techs/go/b") {
		t.Fatalf("warnings %v", update.Warnings)
	}
}

// TestPlanUpdate_SharedFilesFollowAFullUpdate publishes a library release that changes only a group description:
// a full update moves the shared files to it without moving a rule, a scoped update and sync leave them, and a new
// import takes them from it.
func TestPlanUpdate_SharedFilesFollowAFullUpdate(t *testing.T) {
	h := newHistory(t)
	described := []byte(`{"name":"Group","description":"Rules, newly described.","whenToRead":"Always."}`)
	h.release(t, 4, map[string][]byte{"techs/go/_group.yaml": described},
		"schemaVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nchanges: {}\nlibraryFiles: [techs/go/_group.yaml]\n")
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	recorded := h.record(t, config, 3, map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"})

	synced, err := h.sync(t, config, &recorded)
	if err != nil || synced.Snapshot.Release != 3 || string(synced.Snapshot.Files["techs/go/_group.yaml"]) != string(groupMetadata) {
		t.Fatalf("sync moved the shared files to release %d, %v", synced.Snapshot.Release, err)
	}
	scoped, err := h.plan(t, config, &recorded, UpdateTarget{Source: "team", Rule: "techs/go/a"})
	if err != nil || scoped.Sources[0].SharedFiles != nil || h.install(t, scoped, config).Snapshot.Release != 3 {
		t.Fatalf("a scoped update moved the shared files: %+v, %v", scoped.Sources, err)
	}

	update, err := h.plan(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	if shared := update.Sources[0].SharedFiles; len(rows(update)) != 0 || shared == nil || *shared != (SharedFilesUpdate{From: 3, To: 4}) {
		t.Fatalf("rows %q, shared files %+v; want only shared files from release 3 to 4", rows(update), shared)
	}
	installed := h.install(t, update, config)
	if installed.Snapshot.Release != 4 || installed.Snapshot.Commit != h.commits[4] || string(installed.Snapshot.Files["techs/go/_group.yaml"]) != string(described) {
		t.Fatalf("installed shared files from release %d: %q", installed.Snapshot.Release, installed.Snapshot.Files["techs/go/_group.yaml"])
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(installed.Snapshot), want) {
		t.Fatalf("installed %v, want %v", versions(installed.Snapshot), want)
	}
	again, err := h.plan(t, config, &installed.Snapshot)
	if err != nil || again.Sources[0].SharedFiles != nil || len(rows(again)) != 0 {
		t.Fatalf("an update after the update still moves something: %+v, %v", again.Sources, err)
	}

	fresh, err := h.sync(t, config, nil)
	if err != nil || fresh.Snapshot.Release != 4 || string(fresh.Snapshot.Files["techs/go/_group.yaml"]) != string(described) {
		t.Fatalf("a new import took shared files from release %d, %v", fresh.Snapshot.Release, err)
	}
	selected, err := h.sync(t, h.source(t, `"rules":{"include":["techs/go/*","practices/testing/*"]}`), &recorded)
	if err != nil || selected.Snapshot.Release != 4 {
		t.Fatalf("a newly selected group took shared files from release %d, %v", selected.Snapshot.Release, err)
	}
}

// TestPlanUpdate_RefusesASharedFilesReleaseTagMovedBeforeThePreview records shared files from release/2, which no
// imported rule comes from, then moves that tag before planning: the update refuses rather than preview and
// install another commit's shared files under the same release number, with rules or without any.
func TestPlanUpdate_RefusesASharedFilesReleaseTagMovedBeforeThePreview(t *testing.T) {
	for _, test := range []struct {
		name, fields string
		imported     map[string]string
	}{
		{"source with rules", `"rules":{"include":["practices/testing/*"]}`, map[string]string{"practices/testing/c": "1.0.0@1"}},
		{"source without rules", `"rules":{"include":["techs/empty/*"]}`, map[string]string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHistory(t)
			config := h.source(t, test.fields)
			recorded := h.record(t, config, 2, test.imported)
			ctx := context.Background()
			object, err := h.fixture.Command(ctx, "cat-file", "tag", "release/2")
			if err != nil {
				t.Fatal(err)
			}
			_, message, _ := strings.Cut(object, "\n\n")
			if _, err := h.fixture.Commit(ctx, h.fixture.Worktree(), "Move release/2", map[string][]byte{"README.md": []byte("Moved.\n")}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.fixture.Command(ctx, "tag", "--force", "--annotate", "--cleanup=verbatim", "--message", message+"\n", "release/2"); err != nil {
				t.Fatal(err)
			}
			_, err = h.plan(t, config, &recorded)
			requireCode(t, err, "invalid-release-tag")
		})
	}
}

// TestPlanUpdate_SaysWhenARetiredRulesReplacementWasRetiredToo previews b, retired in favor of d, after d is retired
// in turn: without a replacement for d, and then in favor of a, which the preview names instead.
func TestPlanUpdate_SaysWhenARetiredRulesReplacementWasRetiredToo(t *testing.T) {
	for _, test := range []struct {
		name, retirement, current string
	}{
		{"without a replacement", "{lastVersion: 1.0.0, summaries: [Retire d.]}", ""},
		{"in favor of another rule", "{lastVersion: 1.0.0, replacedBy: techs/go/a, summaries: [Fold d into a.]}", "techs/go/a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHistory(t)
			h.release(t, 4, map[string][]byte{"techs/go/d.md": nil}, "schemaVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.0.0\n  practices/testing/c: 1.0.0\nretired:\n  techs/go/d: "+test.retirement+"\n")
			config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
			recorded := h.record(t, config, 1, map[string]string{"techs/go/a": "2.0.0@3", "techs/go/b": "1.0.0@1"})
			update, err := h.plan(t, config, &recorded)
			if err != nil {
				t.Fatal(err)
			}
			rows := update.Sources[0].Rules
			if len(rows) != 1 || rows[0].ID != "techs/go/b" || rows[0].ReplacedBy != "techs/go/d" || !rows[0].ReplacementRetired || rows[0].CurrentReplacement != test.current {
				t.Fatalf("rows %+v, want b replaced by d, which is retired, leading to %q", rows, test.current)
			}
		})
	}
}

// TestPlanUpdate_RefSourcesDontMove plans a source whose ref names a commit as sync does, with no rows.
func TestPlanUpdate_RefSourcesDontMove(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"`+h.commits[1]+`"`)
	update, err := h.plan(t, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(update.Sources) != 1 || update.Sources[0].Ref != h.commits[1] || len(update.Sources[0].Rules) != 0 {
		t.Fatalf("sources %+v", update.Sources)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1"}; !reflect.DeepEqual(versions(h.install(t, update, config).Snapshot), want) {
		t.Fatalf("installed %v", versions(h.install(t, update, config).Snapshot))
	}
}

// TestPlanUpdate_NamesTheSourceWhoseLibraryHasAnInvalidReleaseRecord keeps the source in a failure that its
// library causes, even when the failure wraps a validation error from parsing a release record.
func TestPlanUpdate_NamesTheSourceWhoseLibraryHasAnInvalidReleaseRecord(t *testing.T) {
	alpha, beta := newHistory(t), newHistory(t)
	if err := beta.fixture.Tag(context.Background(), beta.fixture.Worktree(), "release/4", "Notes.\n\n---\nschemaVersion: 1\nrelease: 4\nunknown: true\n"); err != nil {
		t.Fatal(err)
	}
	environment, err := alpha.fixture.Route(map[string]*gitfixture.Fixture{"alpha": alpha.fixture, "beta": beta.fixture})
	if err != nil {
		t.Fatal(err)
	}
	config, err := rules.ParseConfiguration(json.RawMessage(`{"schemaVersion":2,"sources":{"alpha":{"repository":"git@fixture.invalid:alpha","rules":{"include":["techs/go/*"]}},"beta":{"repository":"git@fixture.invalid:beta","rules":{"include":["techs/go/*"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = PlanUpdate(context.Background(), config, map[string]library.Snapshot{}, nil, Options{GitPath: alpha.fixture.GitPath, Environment: environment})
	requireCode(t, err, "invalid-release-tag")
	if !strings.HasPrefix(err.Error(), "source beta: ") {
		t.Fatalf("the failure doesn't name its source: %v", err)
	}
}

// TestPlanUpdate_RejectsTargetsItCantMove names the target in each failure. A library source whose ref names a
// commit doesn't move once sync has recorded it.
func TestPlanUpdate_RejectsTargetsItCantMove(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	refConfig := h.source(t, `"rules":{"include":["techs/go/*"]},"ref":"`+h.commits[1]+`"`)
	synced, err := h.sync(t, refConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		config   rules.Configuration
		recorded *library.Snapshot
		target   UpdateTarget
	}{
		"unknown source":    {config, nil, UpdateTarget{Source: "other"}},
		"ref source":        {refConfig, &synced.Snapshot, UpdateTarget{Source: "team"}},
		"unimported rule":   {config, nil, UpdateTarget{Source: "team", Rule: "practices/testing/c"}},
		"rule of a ref":     {refConfig, nil, UpdateTarget{Source: "team", Rule: "techs/go/a"}},
		"missing rule name": {config, nil, UpdateTarget{Source: "team", Rule: "techs/go/missing"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.plan(t, test.config, test.recorded, test.target)
			var validation errs.ValidationError
			where := test.target.Source
			if test.target.Rule != "" {
				where += ":" + test.target.Rule
			}
			if !errors.As(err, &validation) || validation.ValidationLocation() != where {
				t.Fatalf("got %v, want a failure at %s", err, where)
			}
		})
	}
}

// TestUpdateChangeClassifiesByTheLargestChangedComponent covers each component's boundary.
func TestUpdateChangeClassifiesByTheLargestChangedComponent(t *testing.T) {
	for _, test := range []struct {
		from, to string
		want     UpdateChange
	}{{"1.9.9", "2.0.0", UpdateMajor}, {"1.0.9", "1.1.0", UpdateMinor}, {"1.0.0", "1.0.1", UpdatePatch}, {"1.2.3", "3.2.3", UpdateMajor}} {
		from, _ := trailguide.ParseRuleVersion(test.from, "from")
		to, _ := trailguide.ParseRuleVersion(test.to, "to")
		if got := versionChange(from, to); got != test.want {
			t.Errorf("%s -> %s: %s, want %s", test.from, test.to, got, test.want)
		}
	}
}

// TestPlanUpdate_RecordsAnImportedRetiredRuleAsRetired: an update scoped to a leaves b, which the library retired,
// imported, so the record lists b both among its rules and among its retired rules, which marks it as kept.
func TestPlanUpdate_RecordsAnImportedRetiredRuleAsRetired(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"]}`)
	recorded := h.record(t, config, 1, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1"})
	update, err := h.plan(t, config, &recorded, UpdateTarget{Source: "team", Rule: "techs/go/a"})
	if err != nil {
		t.Fatal(err)
	}
	installed := h.install(t, update, config).Snapshot
	if _, imported := installed.Rules["techs/go/b"]; !imported || !slices.Contains(installed.RetiredRules, "techs/go/b") {
		t.Fatalf("rules %v, retired rules %v", versions(installed), installed.RetiredRules)
	}
}

// TestPlanUpdate_ListsAPinnedReplacementsChangesAfterItsBasedOnVersion: a pin keeps the imported copy where it is,
// even at the newest version, but the replaced row still lists every change after basedOn, with the newest version
// to incorporate, and the pin.
func TestPlanUpdate_ListsAPinnedReplacementsChangesAfterItsBasedOnVersion(t *testing.T) {
	h := newHistory(t)
	for _, test := range []struct{ name, pin, imported, row string }{
		{"pinned at the newest version", "2.0.0", "2.0.0@3", "replaced techs/go/a from 2.0.0 newest 2.0.0 basedOn 1.0.0 local rules/techs/go/a.md pin 2.0.0 (Keep.): Add an example. | Require more."},
		{"pinned at an older version", "1.0.0", "1.0.0@1", "replaced techs/go/a from 1.0.0 newest 2.0.0 basedOn 1.0.0 local rules/techs/go/a.md pin 1.0.0 (Keep.): Add an example. | Require more."},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := h.source(t, `"rules":{"include":["techs/go/a"],"exclude":[{"id":"techs/go/a","reason":"Ours.","replacedBy":"techs/go/a","basedOn":"1.0.0"}],"pins":[{"id":"techs/go/a","version":"`+test.pin+`","reason":"Keep."}]}`)
			recorded := h.record(t, config, 3, map[string]string{"techs/go/a": test.imported})
			update, err := h.plan(t, config, &recorded)
			if err != nil {
				t.Fatal(err)
			}
			if got := rows(update); !reflect.DeepEqual(got, []string{test.row}) {
				t.Fatalf("rows %q, want %q", got, test.row)
			}
			if got := versions(h.install(t, update, config).Snapshot)["techs/go/a"]; got != test.imported {
				t.Fatalf("the pin didn't keep the imported copy: %s", got)
			}
		})
	}
}

// TestPlanUpdate_ListsAPinnedReplacementWithoutBasedOnAsReplaced: without basedOn, a replaced row compares with the
// imported version, and a pin that keeps the imported copy hides no changes, since the pin governs only that copy.
func TestPlanUpdate_ListsAPinnedReplacementWithoutBasedOnAsReplaced(t *testing.T) {
	h := newHistory(t)
	for _, test := range []struct {
		name, pin, imported string
		want                []string
	}{
		{"pinned at an older version", "1.0.0", "1.0.0@1", []string{"replaced techs/go/a from 1.0.0 newest 2.0.0 local rules/techs/go/a.md pin 1.0.0 (Keep.): Add an example. | Require more."}},
		{"pinned at the newest version", "2.0.0", "2.0.0@3", []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := h.source(t, `"rules":{"include":["techs/go/a"],"exclude":[{"id":"techs/go/a","reason":"Ours.","replacedBy":"techs/go/a"}],"pins":[{"id":"techs/go/a","version":"`+test.pin+`","reason":"Keep."}]}`)
			recorded := h.record(t, config, 3, map[string]string{"techs/go/a": test.imported})
			update, err := h.plan(t, config, &recorded)
			if err != nil {
				t.Fatal(err)
			}
			if got := rows(update); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("rows %q, want %q", got, test.want)
			}
			if got := versions(h.install(t, update, config).Snapshot)["techs/go/a"]; got != test.imported {
				t.Fatalf("the pin didn't keep the imported copy: %s", got)
			}
		})
	}
}

// TestPlanUpdate_SaysWhenTheProjectAlreadyReplacedARetiredRulesReplacement: release/3 retires b in favor of d,
// which the project replaces with a local rule, so the retired row names that local rule.
func TestPlanUpdate_SaysWhenTheProjectAlreadyReplacedARetiredRulesReplacement(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/d","reason":"Ours.","replacedBy":"techs/go/d"}]}`)
	recorded := h.record(t, config, 2, map[string]string{"techs/go/a": "1.1.0@2", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"})
	update, err := h.plan(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range update.Sources[0].Rules {
		if row.ID == "techs/go/b" {
			if row.ReplacedBy != "techs/go/d" || row.ReplacementLocalRule != "rules/techs/go/d.md" {
				t.Fatalf("retired row %+v", row)
			}
			return
		}
	}
	t.Fatalf("no retired row for b: %+v", update.Sources[0].Rules)
}
