package imports

import (
	"reflect"
	"strings"
	"testing"
)

// TestImport_ReleaseRefTakesThatReleaseAndAllowsPins: release/2 takes release 2's versions though release 3 exists,
// records no ref, holds a pinned rule at the pinned version, refuses a pin only a later release published, and moves
// every unpinned rule when the ref names another release.
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
	three, err := h.sync(t, h.source(t, `"ref":"release/3","rules":{"include":["techs/go/*"]}`), &two.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(three.Snapshot), want) || three.Snapshot.Release != 3 {
		t.Fatalf("release/3: versions %v, release %d", versions(three.Snapshot), three.Snapshot.Release)
	}
	one, err := h.sync(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`), &three.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "1.0.0@1", "techs/go/b": "1.0.0@1"}; !reflect.DeepEqual(versions(one.Snapshot), want) || one.Snapshot.Release != 1 {
		t.Fatalf("release/1: versions %v, release %d", versions(one.Snapshot), one.Snapshot.Release)
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

// TestImport_LatestFollowsTheNewestReleaseAtEverySync takes the newest library release whenever sync runs, warning
// that the source skips the update preview, and keeps a pinned rule where it is.
func TestImport_LatestFollowsTheNewestReleaseAtEverySync(t *testing.T) {
	h := newHistory(t)
	one, err := h.sync(t, h.source(t, `"ref":"release/1","rules":{"include":["techs/go/*"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	config := h.source(t, `"ref":"latest","rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/b","version":"1.0.0","reason":"Still ours."}]}`)
	latest, err := h.sync(t, config, &one.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "2.0.0@3", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(latest.Snapshot), want) || latest.Snapshot.Release != 3 || !latest.Snapshot.Ref.IsZero() {
		t.Fatalf("versions %v, snapshot %+v", versions(latest.Snapshot), latest.Snapshot)
	}
	if !strings.Contains(strings.Join(latest.Warnings, "\n"), "Source team follows the newest library release, ref: latest, so sync takes each new release as soon as the library publishes it") {
		t.Fatalf("warnings %v", latest.Warnings)
	}
	h.release(t, 4, map[string][]byte{"techs/go/a.md": versionedRule("a 2.1.0")}, "formatVersion: 1\nrelease: 4\nrules:\n  techs/go/a: 2.1.0\n  techs/go/d: 1.0.0\n  practices/testing/c: 1.0.0\nchanges:\n  techs/go/a: {change: minor, from: 2.0.0, summaries: [Add a case.]}\n")
	four, err := h.sync(t, config, &latest.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"techs/go/a": "2.1.0@4", "techs/go/b": "1.0.0@1", "techs/go/d": "1.0.0@2"}; !reflect.DeepEqual(versions(four.Snapshot), want) || four.Snapshot.Release != 4 {
		t.Fatalf("after release/4: versions %v, release %d", versions(four.Snapshot), four.Snapshot.Release)
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

// TestPlanUpdate_LatestHasNothingToMove: sync already took the newest release, so the preview has no rows and says
// the source follows the newest release.
func TestPlanUpdate_LatestHasNothingToMove(t *testing.T) {
	h := newHistory(t)
	config := h.source(t, `"ref":"latest","rules":{"include":["techs/go/*"]}`)
	synced, err := h.sync(t, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	update, err := h.plan(t, config, &synced.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if preview := update.Sources[0]; !preview.FollowsLatest || len(preview.Rules) != 0 || preview.SharedFiles != nil {
		t.Fatalf("preview %+v", preview)
	}
}
