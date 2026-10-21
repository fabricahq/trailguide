// Exercise sync as a lockfile: restoring recorded versions, moving pinned rules, and offline checks of the result.

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// secondRelease changes techs/go/errors to 1.1.0 and adds techs/go/extra in the sync fixture's library.
func secondRelease(t *testing.T, f *gitfixture.Fixture) {
	t.Helper()
	ctx := context.Background()
	files := map[string][]byte{"techs/go/errors.md": []byte(strings.Replace(projectRule, "Return errors to the caller.", "Return wrapped errors to the caller.", 1)), "techs/go/extra.md": []byte(projectRule)}
	if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(files)); err != nil {
		t.Fatal(err)
	}
	record := "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\n  techs/go/extra: 1.0.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\n  techs/go/extra: {change: new, summaries: [Add the rule.]}\n"
	if err := f.Release(ctx, 2, record); err != nil {
		t.Fatal(err)
	}
}

// configure replaces the sync fixture's source with one named team that has the fixture's repository and the given
// JSON fields, such as "rules":{"include":["techs/go/*"]}, and the newest library release's ref when they have none.
func configure(t *testing.T, options Options, f *gitfixture.Fixture, fields string) {
	t.Helper()
	raw := withNewestRef(t, `{"schemaVersion":2,"sources":{"team":{"repository":REPOSITORY,`+fields+`}}}`, "REPOSITORY", f.Repository, imports.Options{GitPath: f.GitPath, Environment: f.Environment})
	raw = strings.ReplaceAll(raw, "REPOSITORY", quoteJSON(f.Repository))
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, configurationFile, raw)
}

// quoteJSON returns text as a JSON string.
func quoteJSON(text string) string {
	quoted, _ := json.Marshal(text)
	return string(quoted)
}

// recordedVersions reads the project's source record and returns each rule's version@release.
func recordedVersions(t *testing.T, options Options) (snapshot, map[string]string) {
	t.Helper()
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	state, err := readProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := decodeSnapshots(state.config, state.vendor.Files)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for id, rule := range snapshots["team"].Rules {
		result[id] = "unreleased"
		if rule.Version != nil {
			result[id] = rule.Version.String() + "@" + strconv.Itoa(rule.Release)
		}
	}
	return snapshots["team"], result
}

// requireCurrent checks offline, with no Git on PATH, that generated output matches the synced inputs.
func requireCurrent(t *testing.T, options Options) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	checked, err := Check(context.Background(), options)
	if err != nil || !checked.Current() {
		t.Fatalf("offline check: %+v %v", checked, err)
	}
}

// TestSync_RestoresRecordedVersionsAndMovesOnlyPinnedRules follows the lockfile across a new library release, refuses
// a pin past the release the ref names, and holds a pinned rule back when the ref moves to the newer release.
func TestSync_RestoresRecordedVersionsAndMovesOnlyPinnedRules(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	restored, err := Sync(ctx, options, git)
	if err != nil || len(restored.Added)+len(restored.Changed)+len(restored.Removed) != 0 {
		t.Fatalf("sync after a new library release changed %+v, %v", restored, err)
	}
	configure(t, options, f, `"ref":"release/1","rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/errors","version":"1.1.0","reason":"Adopt wrapping."}]}`)
	_, err = Sync(ctx, options, git)
	requireFailure(t, err, "version-not-found", "comes from library release 2, after release/1")
	configure(t, options, f, `"rules":{"include":["techs/go/*","techs/go/extra"],"pins":[{"id":"techs/go/errors","version":"1.0.0","reason":"Not ready."}]}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	record, got := recordedVersions(t, options)
	if len(got) != 2 || got["techs/go/errors"] != "1.0.0@1" || got["techs/go/extra"] != "1.0.0@2" || record.Release != 2 {
		t.Fatalf("pinned down with a new rule: %v, release %d", got, record.Release)
	}
	if string(record.Files["techs/go/errors.md"]) != projectRule {
		t.Fatalf("older rule's version 1.0.0 isn't stored at its library path: %q", record.Files["techs/go/errors.md"])
	}
	requireCurrent(t, options)
}

// TestSync_AnExclusionSelectorHidesEveryRuleItMatches imports the rules a group's selector excludes but leaves them
// out of generated guidance, including a rule a later library release adds, checks offline, and refuses a selector
// that matches none of the source's rules.
func TestSync_AnExclusionSelectorHidesEveryRuleItMatches(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	secondRelease(t, f)
	configure(t, options, f, `"rules":{"include":["*"],"exclude":[{"id":"techs/go/*","reason":"Not a Go project."}]}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, got := recordedVersions(t, options); !maps.Equal(got, map[string]string{"techs/go/errors": "1.1.0@2", "techs/go/extra": "1.0.0@2"}) {
		t.Fatalf("versions %v", got)
	}
	files := projectTree(t, options).Files
	for _, id := range []string{"techs/go/errors", "techs/go/extra"} {
		if _, ok := files["vendor/team/rules/"+id+".md"]; !ok {
			t.Errorf("%s wasn't imported", id)
		}
		if _, ok := files["generated/rules/team/"+id+".md"]; ok {
			t.Errorf("%s was generated although techs/go/* excludes it", id)
		}
	}
	requireCurrent(t, options)
	configure(t, options, f, `"rules":{"include":["*"],"exclude":[{"id":"practices/*","reason":"Not used."}]}`)
	_, err := Build(ctx, options)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.team.rules.exclude.practices/*" {
		t.Fatalf("got %v, want a failure at sources.team.rules.exclude.practices/*", err)
	}
}

// TestSync_ExplicitSelectionImportsAGroupTheWildcardSnapshotLacked imports the rules of a group that a library
// release added after a wildcard sync, once the selection names it explicitly and the ref names that release, and
// offline check agrees with the result.
func TestSync_ExplicitSelectionImportsAGroupTheWildcardSnapshotLacked(t *testing.T) {
	for _, test := range []struct {
		name    string
		include string
		groups  []string
		want    map[string]string
	}{
		{"only the new group", `["practices/testing/*"]`, []string{"practices/testing"}, map[string]string{"practices/testing/verify": "1.0.0@2"}},
		{"the new group and a recorded one", `["practices/testing/*","techs/go/*"]`, []string{"practices/testing", "techs/go"}, map[string]string{"techs/go/errors": "1.1.0@2", "practices/testing/verify": "1.0.0@2"}},
		{"every practice", `["practices/*"]`, []string{"practices/testing"}, map[string]string{"practices/testing/verify": "1.0.0@2"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, options, git := syncProject(t)
			ctx := context.Background()
			configure(t, options, f, `"rules":{"include":["*"]}`)
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{
				"techs/go/errors.md":            []byte(strings.Replace(projectRule, "Return errors to the caller.", "Return wrapped errors to the caller.", 1)),
				"practices/testing/_group.yaml": []byte(`{"name":"Testing","description":"Testing guidance.","whenToRead":"When testing."}`),
				"practices/testing/verify.md":   []byte(projectRule),
			}
			if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(files)); err != nil {
				t.Fatal(err)
			}
			if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\n  practices/testing/verify: 1.0.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\n  practices/testing/verify: {change: new, summaries: [Add the rule.]}\n"); err != nil {
				t.Fatal(err)
			}
			configure(t, options, f, `"rules":{"include":`+test.include+`}`)
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			record, got := recordedVersions(t, options)
			if !maps.Equal(got, test.want) || !slices.Equal(record.Groups, test.groups) {
				t.Fatalf("versions %v in groups %v, want %v in %v", got, record.Groups, test.want, test.groups)
			}
			requireCurrent(t, options)
		})
	}
}

// localRuleProject syncs the sync fixture's project with a local rule in techs/go, whose metadata only the team
// source supplies, and returns its root.
func localRuleProject(t *testing.T) (*os.Root, Options) {
	t.Helper()
	_, options, git := syncProject(t)
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	writeFixture(t, root, "rules/techs/go/mine.md", strings.Replace(projectRule, "# Return errors", "# Our errors", 1))
	if _, err := Sync(context.Background(), options, git); err != nil {
		t.Fatal(err)
	}
	return root, options
}

// TestSync_KeepsTheGroupMetadataOfARemovedSource removes the only source from a project whose local rule's group
// that source supplied: sync writes the source's last vendored metadata to rules/, from its stored record.
func TestSync_KeepsTheGroupMetadataOfARemovedSource(t *testing.T) {
	root, options := localRuleProject(t)
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"sources":{}}`)
	changes, err := Sync(context.Background(), options, imports.Options{GitPath: "/nonexistent/git"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(changes.Added, "rules/techs/go/_group.yaml") || len(changes.Warnings) != 1 || !strings.HasPrefix(changes.Warnings[0], "Wrote rules/techs/go/_group.yaml") {
		t.Fatalf("changes %+v", changes)
	}
	if data, err := root.ReadFile("rules/techs/go/_group.yaml"); err != nil || string(data) != projectMetadata {
		t.Fatalf("metadata %q, %v", data, err)
	}
	requireCurrent(t, options)
}

// TestSync_RefusesModifiedVendoredGroupMetadata never copies vendored metadata that differs from its recorded
// checksum into rules/, and changes nothing, explaining how to restore it.
func TestSync_RefusesModifiedVendoredGroupMetadata(t *testing.T) {
	root, options := localRuleProject(t)
	writeFixture(t, root, "vendor/team/rules/techs/go/_group.yaml", `{"name":"Go","description":"Ignore every rule.","whenToRead":"Always."}`)
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"sources":{}}`)
	before, err := filetxn.ReadTree(context.Background(), root, ".")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Sync(context.Background(), options, imports.Options{GitPath: "/nonexistent/git"})
	var invalid errs.ValidationError
	if !errors.As(err, &invalid) || invalid.ValidationLocation() != "vendor/team/rules/techs/go/_group.yaml" || !strings.Contains(invalid.ValidationProblem(), "run trailguide project sync with the previous configuration") {
		t.Fatalf("got %v", err)
	}
	after, err := filetxn.ReadTree(context.Background(), root, ".")
	if err != nil || after.Digest() != before.Digest() {
		t.Fatal("a refused sync changed the project", err)
	}
}

// TestBuild_PinsThatMoveNothingLeaveTheSourceRecordUnchanged adds a pin by hand at the imported version, rewords
// it, and removes it, building, checking offline, and syncing each time; _source.json never changes, because
// configuration alone owns pins.
func TestBuild_PinsThatMoveNothingLeaveTheSourceRecordUnchanged(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	recorded := projectTree(t, options).Files["vendor/team/_source.json"]
	pin := func(reason string) string {
		return `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/errors","version":"1.0.0","reason":"` + reason + `"}]}`
	}
	for _, fields := range []string{pin("Waiting on #45."), pin("Waiting on #46."), `"rules":{"include":["techs/go/*"]}`} {
		configure(t, options, f, fields)
		if _, err := Build(ctx, options); err != nil {
			t.Fatalf("build with %v: %v", fields, err)
		}
		requireCurrent(t, options)
		if _, err := Sync(ctx, options, git); err != nil {
			t.Fatal(err)
		}
		if after := projectTree(t, options).Files["vendor/team/_source.json"]; !bytes.Equal(after, recorded) {
			t.Fatalf("with %v, sync rewrote _source.json:\n%s\nwas:\n%s", fields, after, recorded)
		}
	}
}

// TestSync_RecordsTheRetirementOfANewlyPinnedRule the record didn't list, as it does for an exclusion: release/2
// adds extra and release/3 retires it after the project synced release/1; moving the ref to release/3 and pinning
// extra warns, and sync records the retirement so offline checks accept the pin, and a second sync changes nothing.
func TestSync_RecordsTheRetirementOfANewlyPinnedRule(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	if _, err := f.Commit(ctx, f.Worktree(), "Retire extra", gitfixture.Library(map[string][]byte{"techs/go/extra.md": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/errors: 1.1.0\nretired:\n  techs/go/extra: {lastVersion: 1.0.0, summaries: [No longer needed.]}\n"); err != nil {
		t.Fatal(err)
	}
	configure(t, options, f, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/extra","version":"1.0.0","reason":"Keep it."}]}`)
	changes, err := Sync(ctx, options, git)
	if err != nil || len(changes.Warnings) != 1 || !strings.Contains(changes.Warnings[0], "techs/go/extra") {
		t.Fatalf("sync: %+v, %v", changes, err)
	}
	if record, _ := recordedVersions(t, options); !slices.Equal(record.RetiredRules, []string{"techs/go/extra"}) || record.Release != 3 {
		t.Fatalf("retired %v, shared files from release %d", record.RetiredRules, record.Release)
	}
	requireCurrent(t, options)
	recorded := projectTree(t, options).Files["vendor/team/_source.json"]
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if after := projectTree(t, options).Files["vendor/team/_source.json"]; !bytes.Equal(after, recorded) {
		t.Fatalf("a second sync rewrote _source.json:\n%s\nwas:\n%s", after, recorded)
	}
}

// TestSync_KeepsAPinnedRetiredRuleAtItsLastVersion: the project imports errors 1.1.0 from release/2, the library
// then retires it, and a pin at 1.1.0 keeps it installed when the ref moves to release/3, through sync, build, and
// an offline check, and a second sync leaves the record unchanged.
func TestSync_KeepsAPinnedRetiredRuleAtItsLastVersion(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	secondRelease(t, f)
	configure(t, options, f, `"rules":{"include":["techs/go/*"]}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Retire errors", gitfixture.Library(map[string][]byte{"techs/go/errors.md": nil, "techs/go/assets/errors/data.bin": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/extra: 1.0.0\nretired:\n  techs/go/errors: {lastVersion: 1.1.0, summaries: [Covered by extra.]}\n"); err != nil {
		t.Fatal(err)
	}
	configure(t, options, f, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/errors","version":"1.1.0","reason":"Keep it."}]}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if record, versions := recordedVersions(t, options); versions["techs/go/errors"] != "1.1.0@2" || record.Release != 3 {
		t.Fatalf("versions %v, release %d", versions, record.Release)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
	recorded := projectTree(t, options).Files["vendor/team/_source.json"]
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if after := projectTree(t, options).Files["vendor/team/_source.json"]; !bytes.Equal(after, recorded) {
		t.Fatalf("a second sync rewrote _source.json:\n%s\nwas:\n%s", after, recorded)
	}
}

// TestSync_KeepsRecordedRetirementsWhenOnlyAnExclusionChanged: the record lists retired errors and imports extra;
// the library then retires extra; an exclusion of errors, added by hand, builds and checks offline, and a sync that
// changes nothing else, although checking the exclusion reads the library's releases, writes _source.json byte for
// byte as before, extra's retirement included only when an update moves the source.
func TestSync_KeepsRecordedRetirementsWhenOnlyAnExclusionChanged(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	secondRelease(t, f)
	if _, err := f.Commit(ctx, f.Worktree(), "Retire errors", gitfixture.Library(map[string][]byte{"techs/go/errors.md": nil, "techs/go/assets/errors/data.bin": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/extra: 1.0.0\nretired:\n  techs/go/errors: {lastVersion: 1.1.0, summaries: [Covered by extra.]}\n"); err != nil {
		t.Fatal(err)
	}
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.RemoveAll("vendor"); err != nil {
		t.Fatal(err)
	}
	configure(t, options, f, `"rules":{"include":["techs/go/*"]}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if record, versions := recordedVersions(t, options); !slices.Equal(record.RetiredRules, []string{"techs/go/errors"}) || versions["techs/go/extra"] != "1.0.0@2" {
		t.Fatalf("retired %v, versions %v", record.RetiredRules, versions)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Retire extra", gitfixture.Library(map[string][]byte{"techs/go/extra.md": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 4, "formatVersion: 1\nrelease: 4\nrules: {}\nretired:\n  techs/go/extra: {lastVersion: 1.0.0, summaries: [No longer needed.]}\n"); err != nil {
		t.Fatal(err)
	}
	recorded := projectTree(t, options).Files["vendor/team/_source.json"]
	configure(t, options, f, `"ref":"release/3","rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/errors","reason":"Retired upstream."}]}`)
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if after := projectTree(t, options).Files["vendor/team/_source.json"]; !bytes.Equal(after, recorded) {
		t.Fatalf("sync rewrote _source.json:\n%s\nwas:\n%s", after, recorded)
	}
}

// TestSync_RecordsTheRetirementOfANewlyExcludedRule the record didn't list: release/2 adds extra and release/3
// retires it after the project synced release/1; moving the ref to release/3 and excluding extra warns, and sync
// records the retirement so offline checks accept the exclusion, and a second sync changes nothing.
func TestSync_RecordsTheRetirementOfANewlyExcludedRule(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	secondRelease(t, f)
	if _, err := f.Commit(ctx, f.Worktree(), "Retire extra", gitfixture.Library(map[string][]byte{"techs/go/extra.md": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/errors: 1.1.0\nretired:\n  techs/go/extra: {lastVersion: 1.0.0, summaries: [No longer needed.]}\n"); err != nil {
		t.Fatal(err)
	}
	configure(t, options, f, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/extra","reason":"Retired upstream."}]}`)
	changes, err := Sync(ctx, options, git)
	if err != nil || len(changes.Warnings) != 1 || !strings.Contains(changes.Warnings[0], "techs/go/extra") {
		t.Fatalf("sync: %+v, %v", changes, err)
	}
	if record, _ := recordedVersions(t, options); !slices.Equal(record.RetiredRules, []string{"techs/go/extra"}) || record.Release != 3 {
		t.Fatalf("retired %v, shared files from release %d", record.RetiredRules, record.Release)
	}
	requireCurrent(t, options)
	recorded := projectTree(t, options).Files["vendor/team/_source.json"]
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if after := projectTree(t, options).Files["vendor/team/_source.json"]; !bytes.Equal(after, recorded) {
		t.Fatalf("a second sync rewrote _source.json:\n%s\nwas:\n%s", after, recorded)
	}
}

// TestBuild_HandEditedExclusionsKeepTheSourceRecordCurrent adds and removes exclusions by hand, of an imported rule
// and of a retired one, and runs build, as a fork's next step says: check passes offline, and a sync afterward
// writes _source.json byte for byte as before.
func TestBuild_HandEditedExclusionsKeepTheSourceRecordCurrent(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	secondRelease(t, f)
	if _, err := f.Commit(ctx, f.Worktree(), "Retire extra", gitfixture.Library(map[string][]byte{"techs/go/extra.md": nil})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/errors: 1.1.0\nchanges: {}\nretired:\n  techs/go/extra: {lastVersion: 1.0.0, summaries: [Covered by errors.]}\n"); err != nil {
		t.Fatal(err)
	}
	configure(t, options, f, `"rules":{"include":["techs/go/*"]}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for _, exclude := range []string{
		`[{"id":"techs/go/errors","reason":"Ours is stricter.","replacedBy":"techs/go/errors"}]`,
		`[{"id":"techs/go/extra","reason":"Retired upstream."}]`,
		`[]`,
	} {
		recorded := projectTree(t, options).Files["vendor/team/_source.json"]
		configure(t, options, f, `"rules":{"include":["techs/go/*"],"exclude":`+exclude+`}`)
		if strings.Contains(exclude, "techs/go/errors") {
			root, err := openProject(ctx, options, false)
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, root, "rules/techs/go/errors.md", projectRule)
			writeFixture(t, root, "rules/techs/go/_group.yaml", projectMetadata)
			root.Close()
		}
		if _, err := Build(ctx, options); err != nil {
			t.Fatalf("build with exclusions %v: %v", exclude, err)
		}
		requireCurrent(t, options)
		if _, err := Sync(ctx, options, git); err != nil {
			t.Fatal(err)
		}
		if after := projectTree(t, options).Files["vendor/team/_source.json"]; !bytes.Equal(after, recorded) {
			t.Fatalf("with exclusions %v, sync rewrote _source.json:\n%s\nwas:\n%s", exclude, after, recorded)
		}
	}
}

// TestSync_RefToAnUnreleasedCommitWarnsAndStillChecks reports the source and its unreleased rules.
func TestSync_RefToAnUnreleasedCommitWarnsAndStillChecks(t *testing.T) {
	f, options, git := syncProject(t)
	commit, err := f.Commit(context.Background(), f.Worktree(), "Unreleased", gitfixture.Library(map[string][]byte{"techs/go/errors.md": []byte(projectRule + "\nMore.\n")}))
	if err != nil {
		t.Fatal(err)
	}
	configure(t, options, f, `"rules":{"include":["techs/go/*"]},"ref":`+quoteJSON(commit))
	changes, err := Sync(context.Background(), options, git)
	if err != nil || len(changes.Warnings) != 1 || !strings.Contains(changes.Warnings[0], "Unreleased rules: techs/go/errors.") {
		t.Fatalf("warnings %v, %v", changes.Warnings, err)
	}
	if _, got := recordedVersions(t, options); got["techs/go/errors"] != "unreleased" {
		t.Fatalf("versions %v", got)
	}
	requireCurrent(t, options)
}

// TestSync_RefusesAFormatOneRecord asks the user to delete vendor/ and sync, without changing files.
func TestSync_RefusesAFormatOneRecord(t *testing.T) {
	_, options, git := syncProject(t)
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "vendor/team/_source.json", `{"formatVersion":1,"repository":"x","ref":"v1.0.0","resolvedCommit":"x","groups":[],"files":{}}`)
	before, err := filetxn.ReadTree(context.Background(), root, ".")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Sync(context.Background(), options, git)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "delete .trailguide/vendor/ and run trailguide project sync") {
		t.Fatalf("got %v", err)
	}
	after, err := filetxn.ReadTree(context.Background(), root, ".")
	if err != nil || before.Digest() != after.Digest() {
		t.Fatal("refused sync changed project", err)
	}
}

// TestProject_TellsWhetherToUpgradeOrResyncForAnotherSourceRecordFormat asks to upgrade Trail Guide for a
// _source.json a newer Trail Guide wrote, whose deletion would downgrade the project, and to delete vendor/ and sync
// for an older one, in offline build and check as in sync and update.
func TestProject_TellsWhetherToUpgradeOrResyncForAnotherSourceRecordFormat(t *testing.T) {
	operations := map[string]func(Options, imports.Options) error{
		"build": func(o Options, _ imports.Options) error { _, err := Build(context.Background(), o); return err },
		"check": func(o Options, _ imports.Options) error { _, err := Check(context.Background(), o); return err },
		"sync":  func(o Options, g imports.Options) error { _, err := Sync(context.Background(), o, g); return err },
		"update": func(o Options, g imports.Options) error {
			_, err := PlanUpdate(context.Background(), o, g, nil, nil)
			return err
		},
	}
	for name, operation := range operations {
		for _, format := range []int{2, 4} {
			t.Run(name+"/format "+strconv.Itoa(format), func(t *testing.T) {
				_, options, git := syncProject(t)
				if _, err := Sync(context.Background(), options, git); err != nil {
					t.Fatal(err)
				}
				root, err := openProject(context.Background(), options, false)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				record, err := root.ReadFile("vendor/team/_source.json")
				if err != nil {
					t.Fatal(err)
				}
				// A newer format can also hold fields and values this version can't decode, such as a number no
				// Go type holds.
				replacement := `"formatVersion": ` + strconv.Itoa(format)
				if format > 3 {
					replacement += `, "future": {"limit": 1e999, "rules": "another shape"}`
				}
				writeFixture(t, root, "vendor/team/_source.json", strings.Replace(string(record), `"formatVersion": 3`, replacement, 1))
				err = operation(options, git)
				var failure *filetxn.Error
				var invalid errs.ValidationError
				switch {
				case format > 3 && (!errors.As(err, &failure) || failure.Code != "unsupported-source-record" || !strings.Contains(err.Error(), "upgrade Trail Guide") || strings.Contains(err.Error(), "delete")):
					t.Fatalf("got %v; want unsupported-source-record asking to upgrade", err)
				case format < 3 && (!errors.As(err, &invalid) || !strings.Contains(err.Error(), "delete .trailguide/vendor/ and run trailguide project sync")):
					t.Fatalf("got %v; want the advice to delete vendor/ and sync", err)
				}
			})
		}
	}
}

// TestSync_ReportsCodesForMissingLibraryReleases surfaces ref-required before the first library release.
func TestSync_ReportsCodesForMissingLibraryReleases(t *testing.T) {
	f, err := gitfixture.New(context.Background(), gitfixture.Library(map[string][]byte{"config.yaml": []byte(`{"schemaVersion":2,"sources":{},"library":{}}`)}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	_, options, _ := syncProject(t)
	configure(t, options, f, `"rules":{"include":["*"]}`)
	_, err = Sync(context.Background(), options, imports.Options{GitPath: f.GitPath, Environment: f.Environment})
	requireRefRequired(t, err, "sources.team has no ref, and its repository has no library release to take")
}

// TestSync_StoresOlderRulesAtTheirLibraryPaths keeps an older rule's attachment beside the shared files it links
// to, so relative links in the vendored copy still resolve.
func TestSync_StoresOlderRulesAtTheirLibraryPaths(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                     []byte(`{"schemaVersion":2,"sources":{},"library":{}}`),
		"techs/go/_group.yaml":            []byte(projectMetadata),
		"techs/go/errors.md":              []byte(projectRule + "\n[Guide](assets/errors/guide.md)\n"),
		"techs/go/assets/errors/guide.md": []byte("[Shared](../../../../../assets/shared.md)\n"),
		"assets/shared.md":                []byte("Shared explanation.\n"),
		"techs/go/naming.md":              []byte(projectRule),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\n  techs/go/naming: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n  techs/go/naming: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(map[string][]byte{"techs/go/naming.md": []byte(projectRule + "\nMore.\n")})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n  techs/go/naming: 1.1.0\nchanges:\n  techs/go/naming: {change: minor, from: 1.0.0, summaries: [Add more.]}\n"); err != nil {
		t.Fatal(err)
	}
	_, options, _ := syncProject(t)
	configure(t, options, f, `"rules":{"include":["techs/go/*"]}`)
	if _, err := Sync(ctx, options, imports.Options{GitPath: f.GitPath, Environment: f.Environment}); err != nil {
		t.Fatal(err)
	}
	record, versions := recordedVersions(t, options)
	if versions["techs/go/errors"] != "1.0.0@1" || record.Release != 2 {
		t.Fatalf("versions %v, release %d", versions, record.Release)
	}
	for _, file := range []string{"techs/go/errors.md", "techs/go/assets/errors/guide.md", "assets/shared.md"} {
		if _, ok := record.Files[file]; !ok {
			t.Errorf("%s isn't stored at its library path: %v", file, slices.Sorted(maps.Keys(record.Files)))
		}
	}
	requireCurrent(t, options)
}

// TestBuild_AnEquivalentRefSpellingLeavesTheSourceRecordUnchanged: changing ref: release/1 to refs/tags/release/1
// names the same revision, so build and an offline check pass, and sync keeps the recorded spelling byte for byte.
func TestBuild_AnEquivalentRefSpellingLeavesTheSourceRecordUnchanged(t *testing.T) {
	f, options, git := syncProject(t)
	ctx := context.Background()
	configure(t, options, f, `"rules":{"include":["techs/go/*"]},"ref":"release/1"`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	recorded := projectTree(t, options).Files["vendor/team/_source.json"]
	configure(t, options, f, `"rules":{"include":["techs/go/*"]},"ref":"refs/tags/release/1"`)
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if after := projectTree(t, options).Files["vendor/team/_source.json"]; !bytes.Equal(after, recorded) {
		t.Fatalf("sync rewrote _source.json:\n%s\nwas:\n%s", after, recorded)
	}
	requireCurrent(t, options)
}

// requireEditedRecord requires offline build and check to refuse team's source record as changed outside sync,
// with the ways out, never build.
func requireEditedRecord(t *testing.T, options Options) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	_, buildErr := Build(context.Background(), options)
	_, checkErr := Check(context.Background(), options)
	for name, err := range map[string]error{"build": buildErr, "check": checkErr} {
		var invalid errs.ValidationError
		if !errors.As(err, &invalid) || invalid.ValidationLocation() != "vendor/team/_source.json" || !strings.Contains(invalid.ValidationProblem(), "changed outside trailguide project sync") || !strings.Contains(invalid.ValidationProblem(), "run trailguide project sync") || strings.Contains(invalid.ValidationProblem(), "project build") {
			t.Errorf("offline %s: %v", name, err)
		}
	}
}

// TestSourceRecord_AnUnreadableRecordSaysHowToImportTheSourceAgain, and what that does to unpinned rules.
func TestSourceRecord_AnUnreadableRecordSaysHowToImportTheSourceAgain(t *testing.T) {
	_, options, git := syncProject(t)
	ctx := context.Background()
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	recorded := string(projectTree(t, options).Files["vendor/team/_source.json"])
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "vendor/team/_source.json", strings.Replace(recorded, "{\n", "{\n  \"pins\": {},\n", 1))
	root.Close()
	_, err = Sync(ctx, options, git)
	var invalid errs.ValidationError
	if !errors.As(err, &invalid) || invalid.ValidationLocation() != "vendor/team/_source.json.pins" || !strings.Contains(invalid.ValidationProblem(), "delete .trailguide/vendor/team/ and run trailguide project sync") || !strings.Contains(invalid.ValidationProblem(), "newest versions") {
		t.Fatalf("got %v", err)
	}
}

// TestCheck_NamesAFileWithUnresolvedMergeConflicts, config.yaml or a source record, instead of failing to parse it.
func TestCheck_NamesAFileWithUnresolvedMergeConflicts(t *testing.T) {
	_, options, git := syncProject(t)
	ctx := context.Background()
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	tree := projectTree(t, options)
	conflicted := func(text string) string {
		return "<<<<<<< HEAD\n" + text + "=======\n" + text + ">>>>>>> theirs\n"
	}
	for _, test := range []struct{ file, location string }{
		{configurationFile, ".trailguide/config.yaml"},
		{"vendor/team/_source.json", "vendor/team/_source.json"},
	} {
		root, err := openProject(ctx, options, false)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, root, test.file, conflicted(string(tree.Files[test.file])))
		_, err = Check(ctx, options)
		var invalid errs.ValidationError
		if !errors.As(err, &invalid) || invalid.ValidationLocation() != test.location || !strings.Contains(invalid.ValidationProblem(), "unresolved merge conflicts") {
			t.Errorf("%s: %v", test.file, err)
		}
		writeFixture(t, root, test.file, string(tree.Files[test.file]))
		root.Close()
	}
}

// editRecord rewrites team's source record with edit applied to its parsed fields, leaving its checksum stale, as a
// hand edit or a merge resolution does.
func editRecord(t *testing.T, options Options, edit func(record map[string]any)) {
	t.Helper()
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data, err := root.ReadFile("vendor/team/_source.json")
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	edit(record)
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "vendor/team/_source.json", string(encoded)+"\n")
}

// requireRefusedRecord requires err to refuse team's source record as changed outside sync, naming both ways out.
func requireRefusedRecord(t *testing.T, err error) {
	t.Helper()
	var invalid errs.ValidationError
	if !errors.As(err, &invalid) || invalid.ValidationLocation() != "vendor/team/_source.json" || !strings.Contains(invalid.ValidationProblem(), "changed outside trailguide project sync") ||
		!strings.Contains(invalid.ValidationProblem(), "git checkout -- .trailguide/vendor/team/_source.json") || !strings.Contains(invalid.ValidationProblem(), "git checkout --ours or --theirs") ||
		!strings.Contains(invalid.ValidationProblem(), "delete .trailguide/vendor/team/ and run trailguide project sync") || strings.Contains(invalid.ValidationProblem(), "project build") {
		t.Fatalf("got %v", err)
	}
}

// TestSync_RefusesARecordChangedOutsideSync whatever the change, writing nothing, even when the library would
// confirm it: a removed checksum, a merge of two records, a partial edit, versions and releases the library doesn't
// have, a rule from another revision under an unchanged ref, a published rule marked unreleased, and a carrying
// release recorded as the publisher. Offline build and check refuse it too. Restoring the record sync wrote, as
// taking one side of a merge conflict does, then lets sync run, and the record is byte for byte what a teammate has.
func TestSync_RefusesARecordChangedOutsideSync(t *testing.T) {
	for _, test := range []struct {
		name string
		ref  string
		edit func(record map[string]any, one, two string)
	}{
		{"removed checksum", "", func(record map[string]any, one, two string) { delete(record, "checksum") }},
		{"merge of two records", "", func(record map[string]any, one, two string) {
			record["rules"].(map[string]any)["techs/go/errors"] = map[string]any{"version": "1.0.0", "release": 1, "commit": one}
		}},
		{"partial edit", "", func(record map[string]any, one, two string) {
			delete(record["rules"].(map[string]any), "techs/go/extra")
		}},
		{"version the library doesn't have", "", func(record map[string]any, one, two string) {
			record["rules"].(map[string]any)["techs/go/errors"].(map[string]any)["version"] = "9.9.9"
		}},
		{"shared files' release the library doesn't have", "", func(record map[string]any, one, two string) { record["release"] = 99 }},
		{"shared files older than the rules", "", func(record map[string]any, one, two string) {
			record["release"], record["resolvedCommit"] = 1, one
		}},
		{"false retirements", "", func(record map[string]any, one, two string) {
			record["retiredRules"] = []string{"techs/go/errors", "techs/go/typo"}
		}},
		{"rule from another revision under an unchanged ref", "release/2", func(record map[string]any, one, two string) {
			record["rules"].(map[string]any)["techs/go/errors"] = map[string]any{"version": "1.0.0", "release": 1, "commit": one}
		}},
		{"published rule marked unreleased", "release/2", func(record map[string]any, one, two string) {
			record["rules"].(map[string]any)["techs/go/extra"] = map[string]any{"version": nil, "release": nil, "commit": two}
		}},
		{"carrying release recorded as the publisher", "", func(record map[string]any, one, two string) {
			record["rules"].(map[string]any)["techs/go/extra"].(map[string]any)["release"] = 3
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, options, git := syncProject(t)
			ctx := context.Background()
			secondRelease(t, f)
			if _, err := f.Commit(ctx, f.Worktree(), "Third release", map[string][]byte{"README.md": []byte("Library.\n")}); err != nil {
				t.Fatal(err)
			}
			if err := f.Release(ctx, 3, "formatVersion: 1\nrelease: 3\nrules:\n  techs/go/errors: 1.1.0\n  techs/go/extra: 1.0.0\n"); err != nil {
				t.Fatal(err)
			}
			fields := `"rules":{"include":["techs/go/*"]}`
			if test.ref != "" {
				fields += `,"ref":` + quoteJSON(test.ref)
			}
			configure(t, options, f, fields)
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			recorded := string(projectTree(t, options).Files["vendor/team/_source.json"])
			one, err := f.Command(ctx, "rev-parse", "release/1^{commit}")
			if err != nil {
				t.Fatal(err)
			}
			two, err := f.Command(ctx, "rev-parse", "release/2^{commit}")
			if err != nil {
				t.Fatal(err)
			}
			editRecord(t, options, func(record map[string]any) { test.edit(record, strings.TrimSpace(one), strings.TrimSpace(two)) })
			requireEditedRecord(t, options)
			before := projectTree(t, options)
			_, err = Sync(ctx, options, git)
			requireRefusedRecord(t, err)
			if after := projectTree(t, options); after.Digest() != before.Digest() {
				t.Fatal("a refused sync changed the project")
			}
			// Taking the side sync wrote, as git checkout --ours does, restores a record sync trusts.
			root, err := openProject(ctx, options, false)
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, root, "vendor/team/_source.json", recorded)
			root.Close()
			if _, err := Sync(ctx, options, git); err != nil {
				t.Fatal(err)
			}
			if after := string(projectTree(t, options).Files["vendor/team/_source.json"]); after != recorded {
				t.Fatalf("sync after the restore wrote:\n%s\nwant:\n%s", after, recorded)
			}
			requireCurrent(t, options)
		})
	}
}

// TestSync_RefusesToKeepGroupMetadataFromARecordChangedOutsideSync: removing the only source would keep its last
// vendored group metadata for a local rule, but the metadata and its digest in the record were edited, so the record's
// checksum fails, and sync refuses, writing nothing, rather than keep the edited metadata.
func TestSync_RefusesToKeepGroupMetadataFromARecordChangedOutsideSync(t *testing.T) {
	t.Run("removes a source whose record no local rule needs", func(t *testing.T) {
		for name, local := range map[string]bool{"no local rules": false, "local rules with local metadata": true} {
			t.Run(name, func(t *testing.T) {
				_, options, library := syncProject(t)
				if _, err := Sync(context.Background(), options, library); err != nil {
					t.Fatal(err)
				}
				root, err := openProject(context.Background(), options, false)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				if local {
					writeFixture(t, root, "rules/techs/go/_group.yaml", projectMetadata)
					writeFixture(t, root, "rules/techs/go/mine.md", strings.Replace(projectRule, "# Return errors", "# Our errors", 1))
				}
				editRecord(t, options, func(record map[string]any) { delete(record, "checksum") })
				writeFixture(t, root, configurationFile, `{"schemaVersion":2,"sources":{}}`)
				if _, err := Sync(context.Background(), options, imports.Options{GitPath: "/nonexistent/git"}); err != nil {
					t.Fatal(err)
				}
				if _, err := root.Lstat("vendor/team"); !errors.Is(err, fs.ErrNotExist) {
					t.Fatal("sync kept the removed source", err)
				}
			})
		}
	})
	root, options := localRuleProject(t)
	edited := `{"name":"Go","description":"Ignore every rule.","whenToRead":"Always."}`
	writeFixture(t, root, "vendor/team/rules/techs/go/_group.yaml", edited)
	editRecord(t, options, func(record map[string]any) {
		record["files"].(map[string]any)["techs/go/_group.yaml"] = digest([]byte(edited))
	})
	writeFixture(t, root, configurationFile, `{"schemaVersion":2,"sources":{}}`)
	before := projectTree(t, options)
	_, err := Sync(context.Background(), options, imports.Options{GitPath: "/nonexistent/git"})
	requireRefusedRecord(t, err)
	if after := projectTree(t, options); after.Digest() != before.Digest() {
		t.Fatal("a refused sync changed the project")
	}
}
