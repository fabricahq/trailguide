// Exercise refs that name a library release: a missing ref, a project migrating to one, latest, and update.

package project

import (
	"context"
	"strings"
	"testing"
)

// writeTeamConfig writes the sync fixture's configuration with the given ref line for sources.team, or none.
func writeTeamConfig(t *testing.T, options Options, repository, ref string) {
	t.Helper()
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, "schemaVersion: 2\nsources:\n  team:\n    repository: "+repository+"\n"+ref+"    rules:\n      include: [techs/go/*]\n")
}

// TestProject_ASourceWithoutARefFailsNamingTheReleaseToWrite: before a first sync the error names the library's
// newest release; after one, sync, update, build, and an offline check each name the release the project records,
// even after the library publishes a newer one, and write nothing.
func TestProject_ASourceWithoutARefFailsNamingTheReleaseToWrite(t *testing.T) {
	ctx := context.Background()
	f, options, git := syncProject(t)
	writeTeamConfig(t, options, f.Repository, "")
	_, err := Sync(ctx, options, git)
	requireRefRequired(t, err, "Add ref: release/1 to sources.team, the library's newest release, or ref: latest to follow each new library release")
	writeTeamConfig(t, options, f.Repository, "    ref: release/1\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	writeTeamConfig(t, options, f.Repository, "")
	before := projectTree(t, options)
	recorded := "Add ref: release/1 to sources.team, the release this project records, or ref: latest"
	_, err = Sync(ctx, options, git)
	requireRefRequired(t, err, recorded)
	_, err = PlanUpdate(ctx, options, git, nil, nil)
	requireRefRequired(t, err, recorded)
	_, err = Build(ctx, options)
	requireRefRequired(t, err, recorded)
	t.Setenv("PATH", t.TempDir())
	_, err = Check(ctx, options)
	requireRefRequired(t, err, recorded)
	if projectTree(t, options).Digest() != before.Digest() {
		t.Fatal("a command refused for a missing ref changed the project")
	}
}

// TestProject_AddingTheRecordedReleaseAsTheRefChangesNothing migrates a project whose record follows releases
// without a ref, as every project synced before refs were required does: adding ref: release/N for the release it
// records, in either spelling, passes an offline check, and sync and build then write nothing.
func TestProject_AddingTheRecordedReleaseAsTheRefChangesNothing(t *testing.T) {
	ctx := context.Background()
	f, options, git := syncProject(t)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	for _, ref := range []string{"release/1", "refs/tags/release/1"} {
		writeTeamConfig(t, options, f.Repository, "    ref: "+ref+"\n")
		before := projectTree(t, options)
		requireCurrent(t, options)
		for name, run := range map[string]func() (FileChanges, error){
			"sync":  func() (FileChanges, error) { return Sync(ctx, options, git) },
			"build": func() (FileChanges, error) { return Build(ctx, options) },
		} {
			changes, err := run()
			if err != nil || len(changes.Added)+len(changes.Changed)+len(changes.Removed) != 0 {
				t.Fatalf("%s with ref %s: %+v, %v", name, ref, changes, err)
			}
		}
		if projectTree(t, options).Digest() != before.Digest() {
			t.Fatalf("ref %s changed the project", ref)
		}
	}
}

// TestUpdate_KeepsTheRefsSpellingWhenItWritesTheNewRelease writes refs/tags/release/2 for a ref spelled with
// refs/tags/.
func TestUpdate_KeepsTheRefsSpellingWhenItWritesTheNewRelease(t *testing.T) {
	ctx := context.Background()
	f, options, git := syncProject(t)
	writeTeamConfig(t, options, f.Repository, "    ref: refs/tags/release/1\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if source := preview.Sources[0]; source.SetsRef != "refs/tags/release/2" {
		t.Fatalf("preview sets ref %q; want refs/tags/release/2", source.SetsRef)
	}
	if _, err := plan.Apply(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if config := readRepositoryFile(t, options, ".trailguide/config.yaml"); !strings.Contains(config, "    ref: refs/tags/release/2\n") {
		t.Fatalf("config.yaml:\n%s", config)
	}
	requireCurrent(t, options)
}

// TestSync_LatestTakesEachNewReleaseAndUpdateHasNothingToMove: with ref: latest, sync warns that the source skips
// the update preview, takes a new library release as soon as it exists, and checks offline; update has nothing to
// move and leaves the ref as it is.
func TestSync_LatestTakesEachNewReleaseAndUpdateHasNothingToMove(t *testing.T) {
	ctx := context.Background()
	f, options, git := syncProject(t)
	writeTeamConfig(t, options, f.Repository, "    ref: latest\n")
	first, err := Sync(ctx, options, git)
	if err != nil || len(first.Warnings) != 1 || !strings.Contains(first.Warnings[0], "Source team follows the newest library release, ref: latest") {
		t.Fatalf("sync: %+v, %v", first, err)
	}
	secondRelease(t, f)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if record, versions := recordedVersions(t, options); record.Release != 2 || versions["techs/go/errors"] != "1.1.0@2" || versions["techs/go/extra"] != "1.0.0@2" {
		t.Fatalf("release %d, versions %v", record.Release, versions)
	}
	requireCurrent(t, options)
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := plan.Preview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if source := preview.Sources[0]; !source.FollowsLatest || source.SetsRef != "" || len(source.Rules) != 0 || preview.Moves() {
		t.Fatalf("preview %+v", source)
	}
}
