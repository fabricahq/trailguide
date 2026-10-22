// Exercise library release refs across source imports and updates.

package imports

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestImport_ReleaseRefTakesThatReleaseAndAllowsPins: release/2 takes release 2's versions though release 3 exists,
// records no ref, holds a pinned rule at the pinned version, refuses a pin only a later release published, and refuses
// a ref edited to name another release, which only an update moves to.
func TestImport_ReleaseRefTakesThatReleaseAndAllowsPins(t *testing.T) {
	h := newHistory(t)
	two, err := h.sync(t, h.source(t, `"ref":"release/2","rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.1.0@2", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(two.Snapshot), want) {
		t.Fatalf("versions %v, want %v", versions(two.Snapshot), want)
	}
	if two.Snapshot.Release != 2 || two.Snapshot.Commit != h.commits[2] || !two.Snapshot.Ref.IsZero() || len(two.Warnings) != 0 {
		t.Fatalf("snapshot %+v, warnings %v", two.Snapshot, two.Warnings)
	}
	pinned, err := h.sync(t, h.source(t, `"ref":"release/2","rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/a","version":"1.0.0","reason":"Not ready."}]}`), &two.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(pinned.Snapshot), want) || pinned.Snapshot.Release != 2 {
		t.Fatalf("pinned: versions %v, release %d", versions(pinned.Snapshot), pinned.Snapshot.Release)
	}
	_, err = h.sync(t, h.source(t, `"ref":"release/2","rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/a","version":"2.0.0","reason":"Too new."}]}`), &two.Snapshot)
	requireCode(t, err, "version-not-found")
	if !strings.Contains(err.Error(), "version 2.0.0 comes from library release 3, after release/2, which sources.team.ref names") {
		t.Fatalf("got %v", err)
	}
	for ref, want := range map[string]string{
		"release/3": "sources.team.ref names release/3, but the project records library release 2, and sync doesn't move a source to another release. Run trailguide project update team to review the move to release 3 and take it, or set sources.team.ref back to release/2.",
		"release/1": "sources.team.ref names release/1, older than library release 2, which the project records; sync doesn't move a source",
	} {
		_, err := h.sync(t, h.source(t, `"ref":"`+ref+`","rules":{"include":["techs/go/*"]}`), &two.Snapshot)
		requireCode(t, err, "ref-changed")
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: got %v", ref, err)
		}
	}
}

// TestImport_ReleaseRefKeepsARecordWithoutARef: a record written before refs were required, which follows releases
// without a ref, is the release its ref now names, so sync keeps its versions, including one older than the
// release's own after a pin was removed.
func TestImport_ReleaseRefKeepsARecordWithoutARef(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"ref":"release/2","rules":{"include":["techs/go/*"]}`)
	recorded := h.record(t, config, 2, map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"})
	imported, err := h.sync(t, config, &recorded)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(imported.Snapshot), want) || imported.Snapshot.Release != 2 {
		t.Fatalf("versions %v, release %d", versions(imported.Snapshot), imported.Snapshot.Release)
	}
}

// TestImport_LatestReleaseKeepsTheRecordedRelease: a new source takes the newest library release, and sync keeps that release after the library publishes another, without a warning.
func TestImport_LatestReleaseKeepsTheRecordedRelease(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"ref":"latest-release","rules":{"include":["techs/go/*"]}`)
	three, err := h.sync(t, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(three.Snapshot), want) || three.Snapshot.Release != 3 || !three.Snapshot.Ref.IsZero() || len(three.Warnings) != 0 {
		t.Fatalf("versions %v, snapshot %+v, warnings %v", versions(three.Snapshot), three.Snapshot, three.Warnings)
	}
	h.release(t, 4, map[string][]byte{"techs/go/a.md": versionedRule("a 2.1.0")}, "schemaVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.1.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nchanges:\n  techs/go/a: {change: minor, from: 2.0.0, summaries: [Add a case.]}\n")
	kept, err := h.sync(t, config, &three.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(versions(kept.Snapshot), versions(three.Snapshot)) || kept.Snapshot.Release != 3 || len(kept.Warnings) != 0 {
		t.Fatalf("after release/4: versions %v, release %d, warnings %v", versions(kept.Snapshot), kept.Snapshot.Release, kept.Warnings)
	}
}

// TestPlanUpdate_ReleaseRefMovesToTheNewestRelease previews a source whose ref names an older library release as a
// source that follows releases, past the release its ref names, and installs the newest release without a ref.
func TestPlanUpdate_ReleaseRefMovesToTheNewestRelease(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`)
	one, err := h.sync(t, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	update, err := h.plan(t, config, &one.Snapshot, UpdateTarget{Source: "team"})
	if err != nil {
		t.Fatal(err)
	}
	preview := update.Sources[0]
	if preview.Ref != "" || preview.SharedFiles == nil || *preview.SharedFiles != (SharedFilesUpdate{From: 1, To: 3}) {
		t.Fatalf("preview %+v", preview)
	}
	if want := []string{"major techs/go/a from 1.0.0 to 2.0.0: Add an example. | Require more.", "new techs/go/d to 1.0.0: Add the rule.", "retired techs/go/b from 1.0.0 last 1.0.0 replacedBy techs/go/d: Covered by d."}; !reflect.DeepEqual(rows(update), want) {
		t.Fatalf("rows %q, want %q", rows(update), want)
	}
	installed := h.install(t, update, h.source(t, `"ref":"release/3","rules":{"include":["techs/go/*"]}`))
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(installed.Snapshot), want) || installed.Snapshot.Release != 3 || !installed.Snapshot.Ref.IsZero() {
		t.Fatalf("installed %v, snapshot %+v", versions(installed.Snapshot), installed.Snapshot)
	}
}

// TestPlanUpdate_LatestReleaseMovesToTheNewestRelease previews the move from the recorded release to the newest,
// with no ref to set, since latest-release already names it.
func TestPlanUpdate_LatestReleaseMovesToTheNewestRelease(t *testing.T) {
	h := newHistory(t)
	one, err := h.sync(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	config := h.source(t, `"ref":"latest-release","rules":{"include":["techs/go/*"]}`)
	kept, err := h.sync(t, config, &one.Snapshot)
	if err != nil || kept.Snapshot.Release != 1 {
		t.Fatalf("sync: %+v, %v", kept.Snapshot, err)
	}
	update, err := h.plan(t, config, &kept.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if preview := update.Sources[0]; preview.SharedFiles == nil || *preview.SharedFiles != (SharedFilesUpdate{From: 1, To: 3}) || preview.Available != 0 {
		t.Fatalf("preview %+v", preview)
	}
	installed := h.install(t, update, config)
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(installed.Snapshot), want) || installed.Snapshot.Release != 3 {
		t.Fatalf("installed %v, release %d", versions(installed.Snapshot), installed.Snapshot.Release)
	}
}

// TestPlanUpdate_EditedReleaseRefTakesExactlyThatRelease: with release/2 recorded at release 1, the update previews
// and installs release 2, not the newest, and says release 3 is available; a ref edited to an older release than
// the record's fails.
func TestPlanUpdate_EditedReleaseRefTakesExactlyThatRelease(t *testing.T) {
	h := newHistory(t)
	one, err := h.sync(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	config := h.source(t, `"ref":"release/2","rules":{"include":["techs/go/*"]}`)
	update, err := h.plan(t, config, &one.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if preview := update.Sources[0]; preview.SharedFiles == nil || *preview.SharedFiles != (SharedFilesUpdate{From: 1, To: 2}) || preview.Available != 3 {
		t.Fatalf("preview %+v", preview)
	}
	if want := []string{"minor techs/go/a from 1.0.0 to 1.1.0: Add an example.", "new techs/go/d to 1.0.0: Add the rule."}; !reflect.DeepEqual(rows(update), want) {
		t.Fatalf("rows %q, want %q", rows(update), want)
	}
	installed := h.install(t, update, config)
	if want := map[string]string{"techs/go/a": "1.1.0@2", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(installed.Snapshot), want) || installed.Snapshot.Release != 2 {
		t.Fatalf("installed %v, release %d", versions(installed.Snapshot), installed.Snapshot.Release)
	}
	_, err = h.plan(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`), &installed.Snapshot)
	requireCode(t, err, "ref-changed")
	if !strings.Contains(err.Error(), "older than library release 2, which the project records; sync doesn't move a source, and trailguide project update moves a source only forward") {
		t.Fatalf("got %v", err)
	}
}

// TestPlanUpdate_EditedReleaseRefWithoutAnyReleaseTagsFails: after the library deletes every release tag, an update
// of a ref edited to another release fails with version-not-found and names no newest release.
func TestPlanUpdate_EditedReleaseRefWithoutAnyReleaseTagsFails(t *testing.T) {
	h := newHistory(t)
	one, err := h.sync(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"release/1", "release/2", "release/3"} {
		if _, err := h.fixture.Command(context.Background(), "tag", "--delete", tag); err != nil {
			t.Fatal(err)
		}
	}
	_, err = h.plan(t, h.source(t, `"ref":"release/2","rules":{"include":["techs/go/*"]}`), &one.Snapshot)
	requireCode(t, err, "version-not-found")
	if !strings.Contains(err.Error(), "sources.team.ref names release/2, which the library doesn't have. The repository has no release/<number> tags now") {
		t.Fatalf("got %v", err)
	}
}

// TestPlanUpdate_EditedReleaseRefWithANewRuleStopsAtThatRelease: an edit to release/2 that also selects a rule
// release 2 added still takes release 2, not the newest, although choosing that rule already raises the plan to
// release 2.
func TestPlanUpdate_EditedReleaseRefWithANewRuleStopsAtThatRelease(t *testing.T) {
	h := newHistory(t)
	one, err := h.sync(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/a"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	config := h.source(t, `"ref":"release/2","rules":{"include":["techs/go/a","techs/go/d"]}`)
	update, err := h.plan(t, config, &one.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if preview := update.Sources[0]; preview.Available != 3 {
		t.Fatalf("preview %+v", preview)
	}
	installed := h.install(t, update, config)
	if want := map[string]string{"techs/go/a": "1.1.0@2", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(installed.Snapshot), want) || installed.Snapshot.Release != 2 {
		t.Fatalf("installed %v, release %d", versions(installed.Snapshot), installed.Snapshot.Release)
	}
}
