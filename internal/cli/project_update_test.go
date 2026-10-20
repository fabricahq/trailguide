// Exercise trailguide project update through the compiled CLI: the preview, JSON, decisions, scopes, prompts, and
// exit statuses, against a real library with two library releases.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/project"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/internal/test/terminalfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

// updateRule is a valid rule whose body is text.
func updateRule(text string) []byte {
	return []byte("---\ntitle: Rule\nimpact: HIGH\nimpactDescription: Matters.\nwhenToRead: Always.\n---\n" + text + "\n")
}

// updateFixture is a project synced at a library's release/1, after the library published release/2.
type updateFixture struct {
	binary, directory string
	fixture           *gitfixture.Fixture
}

// newUpdateFixture syncs a project that pins techs/go/backoff and replaces techs/go/loaders with a local rule,
// then publishes release/2, which changes every other kind of rule: errors is major, naming minor, format patch,
// verify new, and retry retired in favor of verify.
func newUpdateFixture(t *testing.T) updateFixture {
	t.Helper()
	return newUpdateFixtureWith(t, buildCLI(t))
}

// newUpdateFixtureWith is newUpdateFixture with an already built CLI.
func newUpdateFixtureWith(t *testing.T, binary string) updateFixture {
	t.Helper()
	ctx := context.Background()
	files := map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{}}`),
		"techs/go/_group.yaml": []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
	}
	names := []string{"backoff", "errors", "format", "loaders", "naming", "retry"}
	record := "formatVersion: 1\nrelease: 1\nrules:\n"
	changes := "changes:\n"
	for _, name := range names {
		files["techs/go/"+name+".md"] = updateRule(name + " 1.0.0")
		record += "  techs/go/" + name + ": 1.0.0\n"
		changes += "  techs/go/" + name + ": {change: new, summaries: [Add the rule.]}\n"
	}
	f, err := gitfixture.New(ctx, gitfixture.Library(files))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, record+changes); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: binary, directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "rules/techs/go/use-data-loaders.md", string(updateRule("Our loaders rule.")))
	u.write(t, "config.yaml", "# Team rules\nschemaVersion: 2\nsources:\n  team:\n    repository: "+f.Repository+"\n    rules:\n      include:\n        - techs/go/*\n      pins:\n        - id: techs/go/backoff\n          version: \"1.0.0\"\n          reason: 'Waiting on #45.'\n      exclude:\n        - id: techs/go/loaders\n          reason: Ours covers our data layer.\n          replacedBy: techs/go/use-data-loaders\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	second := map[string][]byte{"techs/go/retry.md": nil, "techs/go/verify.md": updateRule("verify 1.0.0")}
	for name, version := range map[string]string{"backoff": "2.0.0", "errors": "2.0.0", "format": "1.0.1", "loaders": "1.1.0", "naming": "1.1.0"} {
		second["techs/go/"+name+".md"] = updateRule(name + " " + version)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(second)); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/backoff: 2.0.0\n  techs/go/errors: 2.0.0\n  techs/go/format: 1.0.1\n  techs/go/loaders: 1.1.0\n  techs/go/naming: 1.1.0\n  techs/go/verify: 1.0.0\n"+
		"changes:\n  techs/go/backoff: {change: major, from: 1.0.0, summaries: [Require jitter.]}\n  techs/go/errors: {change: major, from: 1.0.0, summaries: [Require wrapping.]}\n  techs/go/format: {change: patch, from: 1.0.0, summaries: [Fix a typo.]}\n"+
		"  techs/go/loaders: {change: minor, from: 1.0.0, summaries: [Add pagination.]}\n  techs/go/naming: {change: minor, from: 1.0.0, summaries: [Add an example.]}\n  techs/go/verify: {change: new, summaries: [Add the rule.]}\n"+
		"retired:\n  techs/go/retry: {lastVersion: 1.0.0, replacedBy: techs/go/verify, summaries: [Covered by verify.]}\n"); err != nil {
		t.Fatal(err)
	}
	return u
}

// write writes a file in the project's Trail Guide directory.
func (u updateFixture) write(t *testing.T, name, text string) {
	t.Helper()
	file := filepath.Join(u.directory, ".trailguide", name)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

// run runs the CLI in the project with Git and the fixture's transport.
func (u updateFixture) run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	return runCLIWithEnvironment(t, u.binary, u.directory, u.fixture.Environment, args...)
}

// versions returns each rule's recorded version@release from the source record.
func (u updateFixture) versions(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "vendor", "team", "_source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Rules map[string]struct {
			Version string
			Release int
		}
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for id, rule := range record.Rules {
		result[strings.TrimPrefix(id, "techs/go/")] = rule.Version
	}
	return result
}

// updatePreview is the complete human preview of the fixture's update.
const updatePreview = `team
  major     techs/go/errors   1.0.0 -> 2.0.0
            Require wrapping.
  minor     techs/go/naming   1.0.0 -> 1.1.0
            Add an example.
  patch     techs/go/format   1.0.0 -> 1.0.1
            Fix a typo.
  new       techs/go/verify   1.0.0
            Add the rule.
  retired   techs/go/retry    1.0.0
            Replaced by techs/go/verify.
            Covered by verify.
  replaced  techs/go/loaders  1.0.0 -> 1.1.0
            Add pagination.
            Your rule: rules/techs/go/use-data-loaders.md.
            Changes since the imported version 1.0.0; your rule may already have some.
            To replace your rule and its assets with a fork of 1.1.0,
            pass --update-fork team:techs/go/loaders.
  pinned    techs/go/backoff  1.0.0
            Newest version: 2.0.0.
            Reason: Waiting on #45.
  Shared files: release 1 -> 2
`

// TestUpdate_PreviewsWithoutATerminalAndWritesNothing exits 0 with the preview and how to apply it.
func TestUpdate_PreviewsWithoutATerminalAndWritesNothing(t *testing.T) {
	u := newUpdateFixture(t)
	before := projectFileContents(t, u.directory)
	out, diagnostic, code := u.run(t, "project", "update")
	want := updatePreview + "\nThis is a preview; no files were written. To apply it, run the command again\nwith --yes, or in a terminal to answer each question and confirm.\n"
	if code != 0 || diagnostic != "" || out != want {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, diagnostic, out, want)
	}
	if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a preview changed the project")
	}
}

// TestUpdate_JSONPreviewReportsEveryRowAndWritesNothing returns the rows in value.sources with applied false.
func TestUpdate_JSONPreviewReportsEveryRowAndWritesNothing(t *testing.T) {
	u := newUpdateFixture(t)
	before := projectFileContents(t, u.directory)
	out, diagnostic, code := u.run(t, "project", "update", "--json")
	var result struct {
		OK    bool
		Value struct {
			Applied bool
			Sources []struct {
				Name  string
				Rules []json.RawMessage
			}
			Added, Changed, Removed []string
		}
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || diagnostic != "" || !result.OK || result.Value.Applied {
		t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
	}
	if len(result.Value.Added)+len(result.Value.Changed)+len(result.Value.Removed) != 0 || len(result.Value.Sources) != 1 || result.Value.Sources[0].Name != "team" {
		t.Fatalf("value %+v", result.Value)
	}
	want := []string{
		`{"id":"techs/go/errors","change":"major","from":"1.0.0","to":"2.0.0","summaries":["Require wrapping."],"summaryVersions":["2.0.0"],"overwrites":[],"removes":[]}`,
		`{"id":"techs/go/naming","change":"minor","from":"1.0.0","to":"1.1.0","summaries":["Add an example."],"summaryVersions":["1.1.0"],"overwrites":[],"removes":[]}`,
		`{"id":"techs/go/format","change":"patch","from":"1.0.0","to":"1.0.1","summaries":["Fix a typo."],"summaryVersions":["1.0.1"],"overwrites":[],"removes":[]}`,
		`{"id":"techs/go/verify","change":"new","to":"1.0.0","summaries":["Add the rule."],"summaryVersions":["1.0.0"],"overwrites":[],"removes":[]}`,
		`{"id":"techs/go/retry","change":"retired","from":"1.0.0","lastVersion":"1.0.0","summaries":["Covered by verify."],"summaryVersions":["1.0.0"],"replacedBy":"techs/go/verify","overwrites":[],"removes":[]}`,
		`{"id":"techs/go/loaders","change":"replaced","from":"1.0.0","to":"1.1.0","summaries":["Add pagination."],"summaryVersions":["1.1.0"],"localRule":"rules/techs/go/use-data-loaders.md","overwrites":[],"removes":[]}`,
		`{"id":"techs/go/backoff","change":"pinned","from":"1.0.0","newest":"2.0.0","summaries":[],"summaryVersions":[],"pin":{"version":"1.0.0","reason":"Waiting on #45."},"overwrites":[],"removes":[]}`,
	}
	got := []string{}
	for _, row := range result.Value.Sources[0].Rules {
		got = append(got, compactJSON(t, row))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a JSON preview changed the project")
	}
}

// TestUpdate_YesAppliesWithKeepAndExclude pins a major change and a retirement, excludes the new rule, and
// applies the rest, writing config.yaml with the output.
func TestUpdate_YesAppliesWithKeepAndExclude(t *testing.T) {
	u := newUpdateFixture(t)
	out, diagnostic, code := u.run(t, "project", "update", "--yes", "--json", "--keep", "team:techs/go/errors", "--keep", "team:techs/go/retry", "--exclude", "team:techs/go/verify", "--reason", "Not yet.")
	var result struct {
		OK    bool
		Value struct {
			Applied bool
			Changed []string
			Sources []struct {
				Rules []struct{ ID, Decision, Reason string }
			}
		}
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || !result.OK || !result.Value.Applied || result.Value.Changed[0] != "config.yaml" {
		t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
	}
	decisions := map[string]string{}
	for _, row := range result.Value.Sources[0].Rules {
		if row.Decision != "" {
			decisions[row.ID] = row.Decision + ": " + row.Reason
		}
	}
	if want := map[string]string{"techs/go/errors": "keep: Not yet.", "techs/go/retry": "keep: Not yet.", "techs/go/verify": "exclude: Not yet."}; !reflect.DeepEqual(decisions, want) {
		t.Fatalf("decisions %v", decisions)
	}
	if want := map[string]string{"backoff": "1.0.0", "errors": "1.0.0", "format": "1.0.1", "loaders": "1.1.0", "naming": "1.1.0", "retry": "1.0.0", "verify": "1.0.0"}; !reflect.DeepEqual(u.versions(t), want) {
		t.Fatalf("versions %v", u.versions(t))
	}
	config, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"# Team rules\n", "        - id: techs/go/errors\n          version: \"1.0.0\"\n          reason: Not yet.\n", "        - id: techs/go/retry\n          version: \"1.0.0\"\n          reason: Not yet.\n", "        - id: techs/go/verify\n          reason: Not yet.\n"} {
		if !strings.Contains(string(config), text) {
			t.Fatalf("configuration lacks %q:\n%s", text, config)
		}
	}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "check"); code != 0 {
		t.Fatalf("offline check after the update: exit %d\n%s%s", code, out, diagnostic)
	}
	// Without --update-fork, the replaced rule's local rule and exclusion stay as they were.
	if fork, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "rules", "techs", "go", "use-data-loaders.md")); err != nil || string(fork) != string(updateRule("Our loaders rule.")) || strings.Contains(string(config), "basedOn") {
		t.Fatalf("the update changed the replacement, %v:\n%s\n%s", err, fork, config)
	}
	// The next preview lists the kept retirement and both pins, and nothing else.
	out, _, code = u.run(t, "project", "update")
	if code != 0 || !strings.Contains(out, "  retired   techs/go/retry    1.0.0\n            Replaced by techs/go/verify.\n            Covered by verify.\n            Your pin keeps it at 1.0.0.\n            Reason: Not yet.\n") || !strings.Contains(out, "No rule updates are available.") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

// basedOnConfig is the fixture's configuration with the loaders replacement based on version.
func (u updateFixture) basedOnConfig(version string) string {
	return "# Team rules\nschemaVersion: 2\nsources:\n  team:\n    repository: " + u.fixture.Repository + "\n    rules:\n      include:\n        - techs/go/*\n      pins:\n        - id: techs/go/backoff\n          version: \"1.0.0\"\n          reason: 'Waiting on #45.'\n      exclude:\n        - id: techs/go/loaders\n          reason: Ours covers our data layer.\n          replacedBy: techs/go/use-data-loaders\n          basedOn: \"" + version + "\" # forked\n"
}

// TestUpdate_ComparesAReplacementWithItsBasedOnVersion lists the library's changes after basedOn, in human and
// JSON output, replaces the local rule with a fork of the newest version with --update-fork, listing the file it
// overwrites, and lists nothing for it afterward. Sync refuses a basedOn version the rule never published, listing
// the ones it did.
func TestUpdate_ComparesAReplacementWithItsBasedOnVersion(t *testing.T) {
	u := newUpdateFixture(t)
	u.write(t, "config.yaml", u.basedOnConfig("1.0.0"))
	out, diagnostic, code := u.run(t, "project", "update")
	row := "  replaced  techs/go/loaders  1.0.0 -> 1.1.0\n            Add pagination.\n            Your rule: rules/techs/go/use-data-loaders.md, based on 1.0.0.\n            Changes since 1.0.0, the version your rule is based on.\n            To replace your rule and its assets with a fork of 1.1.0,\n            pass --update-fork team:techs/go/loaders.\n"
	if code != 0 || diagnostic != "" || !strings.Contains(out, row) {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant the row:\n%s", code, diagnostic, out, row)
	}
	out, _, code = u.run(t, "project", "update", "--json")
	want := `{"id":"techs/go/loaders","change":"replaced","from":"1.0.0","to":"1.1.0","summaries":["Add pagination."],"summaryVersions":["1.1.0"],"localRule":"rules/techs/go/use-data-loaders.md","basedOn":"1.0.0","overwrites":[],"removes":[]}`
	if code != 0 || !strings.Contains(compactJSON(t, json.RawMessage(out)), want) {
		t.Fatalf("exit %d, want the row %s in:\n%s", code, want, out)
	}
	// The preview of a fork update names the file it overwrites, in human and JSON output.
	out, _, code = u.run(t, "project", "update", "--update-fork", "team:techs/go/loaders")
	forked := "  replaced  techs/go/loaders  1.0.0 -> 1.1.0\n            Add pagination.\n            Your rule: rules/techs/go/use-data-loaders.md, based on 1.0.0.\n            Your rule becomes a fork of 1.1.0.\n            Replaces rules/techs/go/use-data-loaders.md\n"
	if code != 0 || !strings.Contains(out, forked) {
		t.Fatalf("exit %d, want the row:\n%s\nin:\n%s", code, forked, out)
	}
	out, diagnostic, code = u.run(t, "project", "update", "--yes", "--json", "--update-fork", "team:techs/go/loaders")
	type decidedRow struct {
		ID, Decision        string
		Overwrites, Removes []string
	}
	var result struct {
		OK    bool
		Value struct {
			Changed []string
			Sources []struct{ Rules []decidedRow }
		}
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || !result.OK || !slices.Contains(result.Value.Changed, "config.yaml") || !slices.Contains(result.Value.Changed, "rules/techs/go/use-data-loaders.md") {
		t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
	}
	if !slices.ContainsFunc(result.Value.Sources[0].Rules, func(row decidedRow) bool {
		return row.ID == "techs/go/loaders" && row.Decision == "update-fork" && reflect.DeepEqual(row.Overwrites, []string{"rules/techs/go/use-data-loaders.md"}) && len(row.Removes) == 0 && row.Removes != nil
	}) {
		t.Fatalf("rows %+v", result.Value.Sources[0].Rules)
	}
	config, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "config.yaml"))
	if err != nil || !strings.Contains(string(config), "          replacedBy: techs/go/use-data-loaders\n          basedOn: \"1.1.0\" # forked\n") {
		t.Fatalf("configuration, %v:\n%s", err, config)
	}
	if fork, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "rules", "techs", "go", "use-data-loaders.md")); err != nil || string(fork) != string(updateRule("loaders 1.1.0")) {
		t.Fatalf("local rule, %v:\n%s", err, fork)
	}
	if out, _, code := u.run(t, "project", "update"); code != 0 || strings.Contains(out, "techs/go/loaders") {
		t.Fatalf("exit %d, the next update still lists the replacement:\n%s", code, out)
	}
	u.write(t, "config.yaml", u.basedOnConfig("9.0.0"))
	failure := struct {
		OK    bool
		Error responseError
	}{}
	out, _, code = u.run(t, "project", "sync", "--json")
	if err := json.Unmarshal([]byte(out), &failure); err != nil || code != 1 || failure.Error.Code != "version-not-found" || !strings.Contains(failure.Error.Message, "sources.team.rules.exclude.techs/go/loaders.basedOn: the rule never published version 9.0.0; check basedOn. Its published versions, newest first: 1.1.0, 1.0.0.") {
		t.Fatalf("exit %d, %v:\n%s", code, err, out)
	}
}

// TestUpdate_SaysOnlyThatNothingIsAvailableAfterAnUpdate prints one line, without an empty per-source listing.
func TestUpdate_SaysOnlyThatNothingIsAvailableAfterAnUpdate(t *testing.T) {
	u := newUpdateFixture(t)
	if out, diagnostic, code := u.run(t, "project", "update", "--yes"); code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	out, diagnostic, code := u.run(t, "project", "update")
	// The pinned rule still has a newer version, so the preview lists it, and nothing else.
	if code != 0 || diagnostic != "" || strings.Contains(out, "No rule updates.\n") || !strings.HasSuffix(out, "\nNo rule updates are available. No files were written.\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}

// TestUpdate_ScopedToOneRuleMovesOnlyThatRule leaves the rest of the source where it was.
func TestUpdate_ScopedToOneRuleMovesOnlyThatRule(t *testing.T) {
	u := newUpdateFixture(t)
	out, diagnostic, code := u.run(t, "project", "update", "team:techs/go/naming", "--yes")
	// The rule's new version comes from release 2, so the shared files move with it, and no further.
	if code != 0 || !strings.HasPrefix(out, "team\n  minor     techs/go/naming  1.0.0 -> 1.1.0\n            Add an example.\n  Shared files: release 1 -> 2\n\nUpdate complete: ") {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	if want := map[string]string{"backoff": "1.0.0", "errors": "1.0.0", "format": "1.0.0", "loaders": "1.0.0", "naming": "1.1.0", "retry": "1.0.0"}; !reflect.DeepEqual(u.versions(t), want) {
		t.Fatalf("versions %v", u.versions(t))
	}
	out, _, code = u.run(t, "project", "update", "team", "--json")
	if code != 0 || !strings.Contains(out, `"id": "techs/go/verify"`) {
		t.Fatalf("a source target didn't preview its new rule: exit %d\n%s", code, out)
	}
}

// TestUpdate_RejectsInvalidRequestsWithoutWriting separates usage errors (exit 2), including decision flags the
// preview doesn't offer, from requests the project can't satisfy (exit 1), and says in each that it wrote nothing.
func TestUpdate_RejectsInvalidRequestsWithoutWriting(t *testing.T) {
	u := newUpdateFixture(t)
	before := projectFileContents(t, u.directory)
	for _, test := range []struct {
		args []string
		code int
		text string
	}{
		{[]string{"--keep", "team:techs/go/errors"}, 2, "require --reason"},
		{[]string{"--reason", "Why."}, 2, "--reason requires --keep or --exclude"},
		{[]string{"--keep", "techs/go/errors", "--reason", "Why."}, 2, "expected SOURCE:RULE"},
		{[]string{":techs/go/errors"}, 2, "expected SOURCE or SOURCE:RULE"},
		{[]string{"team:Techs/Go"}, 2, "team:Techs/Go"},
		{[]string{"other"}, 1, "Error: no source named other in .trailguide/config.yaml. No files were written.\n\nLocation: other\n"},
		{[]string{"team:techs/go/missing"}, 1, "Error: source team doesn't import this rule; name a rule it imports. No files were written.\n\nLocation: team:techs/go/missing\n"},
		{[]string{"--exclude", "team:techs/go/errors", "--reason", "Why.", "--yes"}, 2, "Error: --exclude team:techs/go/errors: the update doesn't add this rule, so there's nothing to exclude"},
		{[]string{"--keep", "team:techs/go/verify", "--reason", "Why.", "--yes"}, 2, "Error: --keep team:techs/go/verify: the update doesn't move or retire this rule, so there's nothing to keep"},
		{[]string{"--keep", "team:techs/go/backoff", "--reason", "Why.", "--yes"}, 2, "nothing to keep"},
		{[]string{"--keep", "other:techs/go/errors", "--reason", "Why.", "--yes"}, 2, "--keep other:techs/go/errors: the update doesn't move or retire this rule"},
		{[]string{"--keep", "team:techs/go/errors", "--reason", "Why.", "team:techs/go/naming"}, 2, "--keep team:techs/go/errors: the update doesn't move or retire this rule"},
		{[]string{"--update-fork", "team:techs/go/loaders", "--reason", "Why."}, 2, "--reason requires --keep or --exclude"},
		{[]string{"--update-fork", "team:techs/go/loaders", "--keep", "team:techs/go/loaders", "--reason", "Why."}, 2, "--keep and --update-fork both name team:techs/go/loaders"},
		{[]string{"--update-fork", "team:techs/go/loaders", "--exclude", "team:techs/go/loaders", "--reason", "Why."}, 2, "--exclude and --update-fork both name team:techs/go/loaders"},
		{[]string{"--update-fork", "techs/go/loaders"}, 2, "expected SOURCE:RULE"},
		{[]string{"--update-fork", "team:techs/go/errors", "--yes"}, 2, "Error: --update-fork team:techs/go/errors: sources.team.rules.exclude doesn't exclude this rule with replacedBy, so no local rule replaces it; to fork the rule, run trailguide project add rule techs/go/errors --from team@VERSION. No files were written.\n"},
		{[]string{"--update-fork", "other:techs/go/loaders", "--yes"}, 2, "--update-fork other:techs/go/loaders: no source named other in .trailguide/config.yaml"},
		{[]string{"team:techs/go/naming", "--update-fork", "team:techs/go/loaders", "--yes"}, 2, "--update-fork team:techs/go/loaders: the update doesn't list this rule as replaced"},
		{[]string{"--incorporated", "team:techs/go/loaders"}, 2, "unknown flag: --incorporated"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			out, diagnostic, code := u.run(t, append([]string{"project", "update"}, test.args...)...)
			if code != test.code || !strings.Contains(diagnostic, test.text) {
				t.Fatalf("exit %d, want %d with %q:\n%s%s", code, test.code, test.text, out, diagnostic)
			}
			// Every refusal of the update itself says that it wrote nothing; the parser's own errors don't.
			if !strings.Contains(test.text, "unknown flag") && !strings.Contains(diagnostic, "No files were written.") {
				t.Fatalf("the refusal doesn't say that no files were written:\n%s", diagnostic)
			}
		})
	}
	if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a rejected update changed the project")
	}
}

// TestUpdate_RefusesToUpdateTheForkOfARetiredRule as a usage error, without writing.
func TestUpdate_RefusesToUpdateTheForkOfARetiredRule(t *testing.T) {
	u := newUpdateFixture(t)
	u.write(t, "rules/techs/go/our-retry.md", string(updateRule("Our retry rule.")))
	u.write(t, "config.yaml", "# Team rules\nschemaVersion: 2\nsources:\n  team:\n    repository: "+u.fixture.Repository+"\n    rules:\n      include:\n        - techs/go/*\n      exclude:\n        - id: techs/go/retry\n          reason: Ours.\n          replacedBy: techs/go/our-retry\n")
	before := projectFileContents(t, u.directory)
	out, diagnostic, code := u.run(t, "project", "update", "--yes", "--json", "--update-fork", "team:techs/go/retry")
	var response struct {
		OK    bool
		Error responseError
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil || code != 2 || response.Error.Code != "invalid-arguments" || !strings.Contains(response.Error.Message, "--update-fork team:techs/go/retry: the library retired this rule, so it has no newest version to fork") {
		t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
	}
	if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a rejected update changed the project")
	}
}

// TestUpdate_LaterLeavesTheForkInATerminal: answering later applies the rest of the update and leaves the local rule
// and its exclusion as they were.
func TestUpdate_LaterLeavesTheForkInATerminal(t *testing.T) {
	u := newUpdateFixture(t)
	steps := []terminalfixture.Step{
		{Prompt: "[adopt/keep]:", Answer: "adopt"},
		{Prompt: "[add/exclude]:", Answer: "add"},
		{Prompt: "[drop/keep]:", Answer: "drop"},
		{Prompt: "Review later, or replace your local rule with 1.1.0? [later/replace]:", Answer: "l"},
		{Prompt: "Apply the update? [yes/no]:", Answer: "yes"},
	}
	result, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, []string{"project", "update"}, steps)
	if err != nil || result.ExitCode != 0 || !strings.HasPrefix(result.Stdout, "Update complete: ") {
		t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
	}
	config, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fork, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "rules", "techs", "go", "use-data-loaders.md"))
	if err != nil || string(fork) != string(updateRule("Our loaders rule.")) || strings.Contains(string(config), "basedOn") || u.versions(t)["loaders"] != "1.1.0" {
		t.Fatalf("the update changed the replacement, %v:\n%s\n%s", err, fork, config)
	}
}

// TestUpdate_AsksInATerminal keeps a major change and excludes the new rule through prompts, then confirms.
func TestUpdate_AsksInATerminal(t *testing.T) {
	u := newUpdateFixture(t)
	steps := []terminalfixture.Step{
		{Prompt: "team:techs/go/errors: major change, 1.0.0 -> 2.0.0.\r\nAdopt it, or keep 1.0.0? [adopt/keep]:", Answer: "maybe"},
		{Prompt: "\"maybe\" isn't one of the answers; answer adopt or keep, or a or k for short", Answer: "keep"},
		{Prompt: "Reason for keeping it:", Answer: "Waiting on review."},
		{Prompt: "team:techs/go/verify: new rule, 1.0.0.\r\nAdd it, or exclude it? [add/exclude]:", Answer: "e"},
		{Prompt: "Reason for excluding it:", Answer: "Covered locally."},
		{Prompt: "team:techs/go/retry: retired.\r\nDrop it, or keep 1.0.0? [drop/keep]:", Answer: "drop"},
		{Prompt: "team:techs/go/loaders: replaced by rules/techs/go/use-data-loaders.md, with library changes up to 1.1.0.\r\nReplacing your rule with a fork of 1.1.0 replaces or removes:\r\n  rules/techs/go/use-data-loaders.md\r\nReview later, or replace your local rule with 1.1.0? [later/replace]:", Answer: "r"},
		{Prompt: "Apply the update? [yes/no]:", Answer: "yes"},
	}
	result, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, []string{"project", "update"}, steps)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
	}
	beforePrompts, _, prompted := strings.Cut(strings.ReplaceAll(result.Transcript, "\r\n", "\n"), "Adopt it, or keep")
	// A status line says what the command waits on before it reads the libraries.
	if !prompted || !strings.HasPrefix(beforePrompts, "Reading library releases...\n"+updatePreview) {
		t.Fatalf("the status and the complete preview didn't come before the first prompt:\n%s", result.Transcript)
	}
	// The preview with the answers applied comes after the last question and before the confirmation.
	transcript := strings.ReplaceAll(result.Transcript, "\r\n", "\n")
	afterQuestions := transcript[strings.LastIndex(transcript, "Review later, or replace your local rule"):]
	revised, _, confirmation := strings.Cut(afterQuestions, "Apply the update? [yes/no]:")
	if !confirmation || !strings.Contains(revised, "Your answers:\n  Keep team:techs/go/errors at 1.0.0.\n    Reason: Waiting on review.\n  Exclude team:techs/go/verify.\n    Reason: Covered locally.\n  Replace your rule for team:techs/go/loaders with a fork of 1.1.0:\n    Replaces rules/techs/go/use-data-loaders.md\n") || strings.Contains(revised, "  major ") {
		t.Fatalf("the answers, without the preview again, didn't come before the confirmation:\n%s", result.Transcript)
	}
	// Reading the forked version after the answers, and fetching the update after the confirmation, each say so.
	if _, afterAnswer, _ := strings.Cut(transcript, "[later/replace]: r\n"); !strings.HasPrefix(afterAnswer, "Reading forked rule versions...\n") {
		t.Fatalf("no status before reading the fork:\n%s", result.Transcript)
	}
	if _, afterConfirmation, _ := strings.Cut(transcript, "Apply the update? [yes/no]: yes\n"); !strings.HasPrefix(afterConfirmation, "Fetching the update...\n") {
		t.Fatalf("no status before fetching the update:\n%s", result.Transcript)
	}
	// The terminal showed the preview once, so the result lists only what the update changed.
	if strings.Count(transcript, "  pinned    techs/go/backoff") != 1 || !strings.HasPrefix(result.Stdout, "Update complete: ") {
		t.Fatalf("the preview was shown more than once, or the result repeats it:\n%s", result.Transcript)
	}
	if want := map[string]string{"backoff": "1.0.0", "errors": "1.0.0", "format": "1.0.1", "loaders": "1.1.0", "naming": "1.1.0", "verify": "1.0.0"}; !reflect.DeepEqual(u.versions(t), want) {
		t.Fatalf("versions %v", u.versions(t))
	}
	data, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := rules.ParseConfigurationYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	if team := config.Sources[0]; team.Pins["techs/go/errors"].Reason != "Waiting on review." || team.Exclude["techs/go/verify"].Reason != "Covered locally." || team.Exclude["techs/go/loaders"].BasedOn == nil || team.Exclude["techs/go/loaders"].BasedOn.String() != "1.1.0" {
		t.Fatalf("configuration:\n%s\nstdout:\n%s", data, result.Stdout)
	}
	if fork, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "rules", "techs", "go", "use-data-loaders.md")); err != nil || string(fork) != string(updateRule("loaders 1.1.0")) {
		t.Fatalf("local rule, %v:\n%s", err, fork)
	}
	// With nothing left to move, a terminal update applies without questions and says, as a preview would, that
	// nothing was available or written.
	again, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, []string{"project", "update"}, nil)
	if err != nil || again.ExitCode != 0 || !strings.HasSuffix(again.Stdout, "\nNo rule updates are available. No files were written.\n") || strings.Contains(again.Stdout, "Update complete") {
		t.Fatalf("exit %d, %v\n%s", again.ExitCode, err, again.Transcript)
	}
}

// TestUpdate_TerminalCancellationWritesNothing covers declining, interrupting, and ending input.
func TestUpdate_TerminalCancellationWritesNothing(t *testing.T) {
	u := newUpdateFixture(t)
	first := terminalfixture.Step{Prompt: "Adopt it, or keep 1.0.0? [adopt/keep]:", Answer: "adopt"}
	for _, test := range []struct {
		name  string
		steps []terminalfixture.Step
		code  int
		text  string
	}{
		{"decline", []terminalfixture.Step{first, {Prompt: "[add/exclude]:", Answer: "add"}, {Prompt: "[drop/keep]:", Answer: "drop"}, {Prompt: "[later/replace]:", Answer: "later"}, {Prompt: "Apply the update? [yes/no]:", Answer: "no"}}, 0, "Update cancelled. No files were written."},
		{"interrupt", []terminalfixture.Step{first, {Prompt: "[add/exclude]:", Interrupt: true}}, 130, ""},
		{"end of input", []terminalfixture.Step{first, {Prompt: "[add/exclude]:", EOF: true}}, 2, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := projectFileContents(t, u.directory)
			result, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, []string{"project", "update"}, test.steps)
			if err != nil || result.ExitCode != test.code || !strings.Contains(result.Stdout, test.text) || strings.Contains(result.Transcript, "context canceled") || strings.Contains(result.Transcript, "EOF") {
				t.Fatalf("exit %d, %v\n%s\nstdout:\n%s", result.ExitCode, err, result.Transcript, result.Stdout)
			}
			if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
				t.Fatal("a cancelled update changed the project")
			}
			// An interrupted or ended prompt ends its line before the error.
			if transcript := strings.ReplaceAll(result.Transcript, "\r\n", "\n"); test.code != 0 && (strings.Contains(transcript, "]: Error:") || !strings.Contains(transcript, "\nError: ")) {
				t.Fatalf("the error shares the prompt's line:\n%s", result.Transcript)
			}
		})
	}
}

// TestUpdateDetails_LabelsSummariesWithTheirVersionsWhenARowSpansSeveral, and only then.
func TestUpdateDetails_LabelsSummariesWithTheirVersionsWhenARowSpansSeveral(t *testing.T) {
	one, two := trailguide.RuleVersion{Major: 1, Minor: 1}, trailguide.RuleVersion{Major: 2}
	spanning := imports.RuleUpdate{Change: imports.UpdateMajor, Summaries: []string{"Add an example.", "Require more."}, SummaryVersions: []trailguide.RuleVersion{one, two}}
	if got, want := updateDetails("team", spanning), []string{"1.1.0: Add an example.", "2.0.0: Require more."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	single := imports.RuleUpdate{Change: imports.UpdatePatch, Summaries: []string{"Fix a typo.", "Fix a link."}, SummaryVersions: []trailguide.RuleVersion{one, one}}
	if got, want := updateDetails("team", single), []string{"Fix a typo.", "Fix a link."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestBuildAndCheck_ReportAnUnknownExclusionTheSameWay, with the problem and its location on separate lines.
func TestBuildAndCheck_ReportAnUnknownExclusionTheSameWay(t *testing.T) {
	u := newUpdateFixture(t)
	u.write(t, "config.yaml", "schemaVersion: 2\nsources:\n  team:\n    repository: "+u.fixture.Repository+"\n    rules:\n      include:\n        - techs/go/*\n      pins:\n        - id: techs/go/backoff\n          version: \"1.0.0\"\n          reason: 'Waiting on #45.'\n      exclude:\n        - id: techs/go/loaders\n          reason: Ours covers our data layer.\n          replacedBy: techs/go/use-data-loaders\n        - id: techs/go/missing\n          reason: Typo.\n")
	want := "Error: names no rule the library supplies to this source; run trailguide project sync to check it against the library.\n\nLocation: sources.team.rules.exclude.techs/go/missing\n"
	for _, command := range []string{"build", "check"} {
		out, diagnostic, code := u.run(t, "project", command)
		if code != 1 || diagnostic != want {
			t.Errorf("%s: exit %d, stdout %q, stderr:\n%s\nwant:\n%s", command, code, out, diagnostic, want)
		}
	}
}

// TestLibraryReadme_MentionsRetainedTermsOnlyWhenTheLibraryDeclaresThem: the fixture's library declares no license,
// so its generated README says so and doesn't claim the folder keeps copies of terms.
func TestLibraryReadme_MentionsRetainedTermsOnlyWhenTheLibraryDeclaresThem(t *testing.T) {
	u := newUpdateFixture(t)
	data, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "generated", "libraries", "team", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "copies of declared library license and notice files") || !strings.Contains(string(data), "No library license declaration was supplied.") {
		t.Fatalf("README:\n%s", data)
	}
}

// TestBuild_NamesItsPathsRelativeToTheWorkingDirectory: check and build from a subdirectory say where their paths
// are from there, as authoring commands do.
func TestBuild_NamesItsPathsRelativeToTheWorkingDirectory(t *testing.T) {
	u := newUpdateFixture(t)
	if err := os.Remove(filepath.Join(u.directory, ".trailguide", "generated", "RULES.md")); err != nil {
		t.Fatal(err)
	}
	// A .git directory makes the project a repository, whose root commands find from a subdirectory.
	sub := filepath.Join(u.directory, "services", "api")
	if err := os.MkdirAll(filepath.Join(u.directory, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, u.binary, sub, "project", "check")
	if code != 1 || !strings.Contains(out, "Paths relative to ../../.trailguide:\n") {
		t.Fatalf("check: exit %d, stdout:\n%s", code, out)
	}
	out, diagnostic, code := runCLI(t, u.binary, sub, "project", "build")
	if code != 0 || diagnostic != "" || !strings.Contains(out, "Paths relative to ../../.trailguide/generated:\n  Add: RULES.md\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}

// TestUpdateDetails_ShowsAPinnedForksReplacementBesideThePin: a pin on the imported copy doesn't hide the fork's
// version or the files it overwrites.
func TestUpdateDetails_ShowsAPinnedForksReplacementBesideThePin(t *testing.T) {
	old, newest := trailguide.RuleVersion{Major: 1}, trailguide.RuleVersion{Major: 2}
	row := imports.RuleUpdate{Change: imports.UpdateReplaced, From: &old, Newest: &newest, LocalRule: "rules/techs/go/loaders.md", Pin: &rules.Pin{Version: old, Reason: "Not yet."}, Decision: "update-fork", Overwrites: []string{"rules/techs/go/loaders.md"}, Removes: []string{"rules/techs/go/assets/loaders/notes.md"}, Summaries: []string{}}
	want := []string{"Your rule: rules/techs/go/loaders.md.", "Your pin keeps it at 1.0.0.", "Reason: Not yet.", "Your rule becomes a fork of 2.0.0.", "Replaces rules/techs/go/loaders.md", "Removes rules/techs/go/assets/loaders/notes.md"}
	if got := updateDetails("team", row); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestUpdate_AsksAboutAPinnedForkInATerminal: a pin on the imported copy doesn't stop the terminal from offering to
// replace the fork with the newest version.
func TestUpdate_AsksAboutAPinnedForkInATerminal(t *testing.T) {
	u := newUpdateFixture(t)
	u.write(t, "config.yaml", "# Team rules\nschemaVersion: 2\nsources:\n  team:\n    repository: "+u.fixture.Repository+"\n    rules:\n      include:\n        - techs/go/*\n      pins:\n        - id: techs/go/backoff\n          version: \"1.0.0\"\n          reason: 'Waiting on #45.'\n        - id: techs/go/loaders\n          version: \"1.0.0\"\n          reason: Not yet.\n      exclude:\n        - id: techs/go/loaders\n          reason: Ours covers our data layer.\n          replacedBy: techs/go/use-data-loaders\n")
	steps := []terminalfixture.Step{
		{Prompt: "[adopt/keep]:", Answer: "adopt"},
		{Prompt: "[add/exclude]:", Answer: "add"},
		{Prompt: "[drop/keep]:", Answer: "drop"},
		{Prompt: "Review later, or replace your local rule with 1.1.0? [later/replace]:", Answer: "replace"},
		{Prompt: "Apply the update? [yes/no]:", Answer: "yes"},
	}
	result, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, []string{"project", "update"}, steps)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
	}
	if fork, err := os.ReadFile(filepath.Join(u.directory, ".trailguide", "rules", "techs", "go", "use-data-loaders.md")); err != nil || string(fork) != string(updateRule("loaders 1.1.0")) || u.versions(t)["loaders"] != "1.0.0" {
		t.Fatalf("local rule, %v:\n%s\nversions %v", err, fork, u.versions(t))
	}
}

// TestUpdateDetails_NamesTheLocalRuleThatAlreadyReplacesARetiredRulesReplacement instead of recommending it.
func TestUpdateDetails_NamesTheLocalRuleThatAlreadyReplacesARetiredRulesReplacement(t *testing.T) {
	last := trailguide.RuleVersion{Major: 1}
	row := imports.RuleUpdate{Change: imports.UpdateRetired, From: &last, LastVersion: &last, ReplacedBy: "techs/go/d", ReplacementLocalRule: "rules/techs/go/d.md"}
	if got := updateDetails("team", row); len(got) == 0 || got[0] != "Replaced by techs/go/d, which your project already replaces with rules/techs/go/d.md." {
		t.Fatalf("got %q", got)
	}
}

// TestUpdate_NeverSaysNoFilesWereWrittenAfterRecovering an interrupted earlier command: a preview, a declined update,
// and an interrupted one each say that recovery came first.
func TestUpdate_NeverSaysNoFilesWereWrittenAfterRecovering(t *testing.T) {
	recovered := "first recovered an interrupted earlier command"
	first := terminalfixture.Step{Prompt: "Adopt it, or keep 1.0.0? [adopt/keep]:", Answer: "adopt"}
	for _, test := range []struct {
		name  string
		steps []terminalfixture.Step
		code  int
	}{
		{"preview", nil, 0},
		{"decline", []terminalfixture.Step{first, {Prompt: "[add/exclude]:", Answer: "add"}, {Prompt: "[drop/keep]:", Answer: "drop"}, {Prompt: "[later/replace]:", Answer: "later"}, {Prompt: "Apply the update? [yes/no]:", Answer: "no"}}, 0},
		{"interrupt", []terminalfixture.Step{first, {Prompt: "[add/exclude]:", Interrupt: true}}, 130},
	} {
		t.Run(test.name, func(t *testing.T) {
			u := newUpdateFixture(t)
			u.write(t, ".trailguide-transaction/staged", "left by an interrupted command")
			var transcript string
			if test.steps == nil {
				out, diagnostic, code := u.run(t, "project", "update")
				if code != test.code {
					t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
				}
				transcript = out + diagnostic
			} else {
				result, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, []string{"project", "update"}, test.steps)
				if err != nil || result.ExitCode != test.code {
					t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
				}
				transcript = result.Transcript + result.Stdout
			}
			if strings.Contains(strings.ToLower(transcript), "no files were written") || !strings.Contains(transcript, recovered) {
				t.Fatalf("output:\n%s", transcript)
			}
		})
	}
	// A JSON preview, which only previews, reports the recovery as recovered.
	t.Run("JSON preview", func(t *testing.T) {
		u := newUpdateFixture(t)
		u.write(t, ".trailguide-transaction/staged", "left by an interrupted command")
		out, diagnostic, code := u.run(t, "project", "update", "--json")
		var result struct {
			OK    bool
			Value struct {
				Applied, Recovered bool
			}
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || !result.OK || result.Value.Applied || !result.Value.Recovered {
			t.Fatalf("exit %d, %v:\n%s%s", code, err, out, diagnostic)
		}
	})
}

// TestUpdateReport_CountsARefreshedGuideAsAWrite: an applied update that changed no rule but refreshed the managed
// guide reports the guide, rather than that no files were written.
func TestUpdateReport_CountsARefreshedGuideAsAWrite(t *testing.T) {
	result := project.UpdateResult{Applied: true, Sources: []imports.SourceUpdate{}, FileChanges: project.FileChanges{Added: []string{}, Changed: []string{}, Removed: []string{}, Guide: &project.GuideChange{Path: "README.md"}}}
	report := updateReport(result, false, false, updateLocation{root: t.TempDir(), workdir: t.TempDir()})
	if strings.Contains(report.human, "No files were written") || !strings.Contains(report.human, "Trail Guide guide updated") {
		t.Fatalf("report:\n%s", report.human)
	}
}

// skillLevelSkill is a valid SKILL.md for the skill name.
func skillLevelSkill(name string) []byte {
	return []byte("---\nname: " + name + "\ndescription: Do " + name + ".\n---\n# " + name + "\n")
}

// TestUpdate_PreviewsEachLevelChangeOnTheSkillsRow syncs a project that includes a library's recommended skills,
// then publishes a release that stops recommending review, requires the new audit, and only recommends ship, and
// checks that the preview shows each skill once: review and audit with their level changes beneath their rows, and
// ship, which the update neither installs nor removes, on a level row of its own.
func TestUpdate_PreviewsEachLevelChangeOnTheSkillsRow(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                        []byte(`{"schemaVersion":2,"sources":{},"library":{"skills":{"required":["skills/ship"],"recommended":["skills/review"],"offered":["skills/deploy"]}}}`),
		"techs/go/_group.yaml":               []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":                 updateRule("errors 1.0.0"),
		".trailguide/skills/ship/SKILL.md":   skillLevelSkill("ship"),
		".trailguide/skills/review/SKILL.md": skillLevelSkill("review"),
		".trailguide/skills/deploy/SKILL.md": skillLevelSkill("deploy"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: "+f.Repository+"\n    skills:\n      include: [recommended]\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	second := map[string][]byte{
		"config.yaml":                       []byte(`{"schemaVersion":2,"sources":{},"library":{"skills":{"required":["skills/audit"],"recommended":["skills/ship"],"offered":["skills/deploy","skills/review"]}}}`),
		".trailguide/skills/audit/SKILL.md": skillLevelSkill("audit"),
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Change levels", gitfixture.Library(second)); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\nchanges: {}\n"); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := u.run(t, "project", "update")
	want := "team\n" +
		"  added     skills/audit\n" +
		"            level: none -> required\n" +
		"            New required skill: it installs whatever skills.include says.\n" +
		"  removed   skills/review\n" +
		"            level: recommended -> offered\n" +
		"            No longer recommended: it installs only when skills.include names it or has \"*\".\n"
	if code != 0 || !strings.Contains(out, want) {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant it to contain:\n%s", code, diagnostic, out, want)
	}
	for _, id := range []string{"skills/audit", "skills/review", "skills/ship"} {
		if strings.Count(out, id) != 1 {
			t.Errorf("preview lists %s more than once:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "  level     skills/ship    required -> recommended\n            No longer required: it stays installed only when skills.include selects it.\n") {
		t.Errorf("preview lacks ship's own level row:\n%s", out)
	}
}
