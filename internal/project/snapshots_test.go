// Exercise snapshot integrity and offline version checks through the encoding and decoding boundary.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/library"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// Commits of the fixture's two library releases.
var (
	releaseOne = strings.Repeat("1", 40)
	releaseTwo = strings.Repeat("2", 40)
)

// snapshotConfig parses one source named team from its JSON fields, adding ref release/2, the fixture's library
// release, when they have no ref.
func snapshotConfig(t *testing.T, fields string) rules.Configuration {
	t.Helper()
	if !strings.Contains(fields, `"ref":`) {
		fields = `"ref":"release/2",` + fields
	}
	config, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"sources":{"team":{"repository":"https://github.com/acme/rules",` + fields + `}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

// snapshotFixture supplies a source that follows rule versions: errors at 1.1.0 from library release 2, which
// supplies the library-wide files, and naming at 1.0.0 from library release 1.
// Files include original binary and CRLF bytes.
func snapshotFixture(t *testing.T) (rules.Configuration, map[string]snapshot) {
	t.Helper()
	config := snapshotConfig(t, `"rules":{"include":["techs/go/*"]}`)
	one, two := trailguide.RuleVersion{Major: 1}, trailguide.RuleVersion{Major: 1, Minor: 1}
	return config, map[string]snapshot{"team": {Repository: config.Sources[0].Repository, RetiredRules: []string{}, Release: 2, Commit: releaseTwo, Groups: []string{"techs/go"}, Selection: config.Sources[0].Groups, RuleSelection: []string{},
		Rules: map[string]library.ImportedRule{"techs/go/errors": {Version: &two, Release: 2, Commit: releaseTwo}, "techs/go/naming": {Version: &one, Release: 1, Commit: releaseOne, LegacyLayout: true}},
		Files: map[string][]byte{
			"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`),
			"techs/go/_group.yaml": []byte(`{"name":"Go","description":"Go rules","whenToRead":"When editing Go."}`),
			"techs/go/errors.md":   []byte(projectRule),
			"techs/go/naming.md":   []byte(projectRule),
			"assets/image.bin":     {0, 255, 10, 128}, "LICENSE": []byte("Terms\r\nPreserved\r\n"), "empty.txt": {},
		},
		SkillSelection: []string{}, Skills: map[string]library.ImportedSkill{}, AvailableSkills: map[string]library.ImportedSkill{}, Executable: []string{}}}
}

// requireSync checks that decoding fails with a validation error that tells the user to sync.
func requireSync(t *testing.T, got map[string]snapshot, err error) {
	t.Helper()
	var validation errs.ValidationError
	if got != nil || !errors.As(err, &validation) || !strings.HasPrefix(validation.ValidationLocation(), "lock.json.sources.team") || !strings.Contains(validation.ValidationProblem(), "run trailguide project sync") {
		t.Fatalf("got %v, %v; want a failure that asks for sync", got, err)
	}
}

// TestSnapshotsRoundTripOwnsExactBytes verifies deterministic records and independent ownership.
func TestSnapshotsRoundTripOwnsExactBytes(t *testing.T) {
	config, input := snapshotFixture(t)
	encoded, err := encodeStored(config, input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeStored(config, input)
	if err != nil || !reflect.DeepEqual(encoded, again) {
		t.Fatalf("unstable encoding: %v", err)
	}
	got, err := decodeStored(config, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("round trip: %#v", got)
	}
	got["team"].Files["LICENSE"][0] = 'X'
	if input["team"].Files["LICENSE"][0] == 'X' || encoded["team/LICENSE"][0] == 'X' {
		t.Fatal("returned bytes alias input")
	}
	encoded["team/assets/image.bin"][0] = 9
	if input["team"].Files["assets/image.bin"][0] != 0 {
		t.Fatal("encoded bytes alias input")
	}
}

// TestSnapshotRuleLinkToSharedAssetResolvesInVendor stores a rule where it is in the library's Trail Guide directory,
// under rules/, so its link to a shared asset, written from .trailguide/rules/techs/go/, reaches the vendored asset.
func TestSnapshotRuleLinkToSharedAssetResolvesInVendor(t *testing.T) {
	config, input := snapshotFixture(t)
	link := "../../../assets/image.bin"
	input["team"].Files["techs/go/errors.md"] = []byte(projectRule + "\nSee [the diagram](" + link + ").\n")
	encoded, err := encodeStored(config, input)
	if err != nil {
		t.Fatal(err)
	}
	rule := "team/rules/techs/go/errors.md"
	if _, ok := encoded[rule]; !ok {
		t.Fatalf("no vendored rule at vendor/%s; got %v", rule, slices.Sorted(maps.Keys(encoded)))
	}
	target, _, local, err := rules.RelativeTarget(link, "_managed/vendor/"+rule)
	if err != nil || !local || !strings.HasPrefix(target, "_managed/vendor/team/") {
		t.Fatalf("link resolves to %q, %v, %v; want a file in vendor/team/", target, local, err)
	}
	if _, ok := encoded[strings.TrimPrefix(target, "_managed/vendor/")]; !ok {
		t.Fatalf("link resolves to %s, which the snapshot doesn't have", target)
	}
	got, err := decodeStored(config, encoded)
	if err != nil || !reflect.DeepEqual(got, input) {
		t.Fatalf("round trip: %v", err)
	}
}

// TestSnapshotLockFormat writes the documented lock entry: the ref configuration names, the release and commit of
// the shared files, the commit of each library release a rule comes from, never pins, and no per-source records.
func TestSnapshotLockFormat(t *testing.T) {
	config, input := snapshotFixture(t)
	encoded, err := encodeStored(config, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := encoded["team/_source.json"]; ok {
		t.Fatal("wrote a per-source record")
	}
	var lock struct {
		LockVersion int                                   `json:"lockVersion"`
		Sources     map[string]map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(encoded[lockFile], &lock); err != nil {
		t.Fatal(err)
	}
	entry := lock.Sources["team"]
	if lock.LockVersion != 1 || !strings.HasPrefix(string(entry["integrity"]), `"sha256-`) {
		t.Fatalf("lock:\n%s", encoded[lockFile])
	}
	for field, want := range map[string]string{
		"kind": `"library"`, "ref": `"release/2"`, "release": "2", "commit": `"` + releaseTwo + `"`, "groups": `["techs/go"]`, "retiredRules": "[]",
		"selectors":      `{"groups":["techs/go"],"rules":[],"skills":[]}`,
		"releaseCommits": `{"1":"` + releaseOne + `","2":"` + releaseTwo + `"}`,
		"rules":          `{"techs/go/errors":{"release":2,"version":"1.1.0"},"techs/go/naming":{"legacyLayout":true,"release":1,"version":"1.0.0"}}`,
		"skills":         "{}", "availableSkills": "{}", "executable": "[]",
	} {
		var compact strings.Builder
		for _, line := range strings.Split(string(entry[field]), "\n") {
			compact.WriteString(strings.TrimSpace(line))
		}
		if got := strings.ReplaceAll(strings.ReplaceAll(compact.String(), `": `, `":`), ", ", ","); got != want {
			t.Errorf("%s = %s, want %s", field, got, want)
		}
	}
	for _, field := range []string{"pins", "exclude", "curated"} {
		if _, ok := entry[field]; ok {
			t.Errorf("wrote %s", field)
		}
	}
}

// TestSnapshotRepositoryAddressChangeRequiresSync rejects stale authored provenance even for the same repository.
func TestSnapshotRepositoryAddressChangeRequiresSync(t *testing.T) {
	for _, address := range []string{"git@github.com:acme/rules.git", "https://github.com/acme/rules.git", "https://github.com/ACME/rules"} {
		t.Run(address, func(t *testing.T) {
			config, snapshots := snapshotFixture(t)
			vendor, err := encodeStored(config, snapshots)
			if err != nil {
				t.Fatal(err)
			}
			config.Sources[0].Repository = address
			got, err := decodeStored(config, vendor)
			requireSync(t, got, err)
		})
	}
}

// TestSnapshotChangedSelectionOrRefRequiresSync rejects configuration the record doesn't describe.
func TestSnapshotChangedSelectionOrRefRequiresSync(t *testing.T) {
	for name, fields := range map[string]string{
		"added group":            `"rules":{"include":["techs/go/*","techs/rust/*"]}`,
		"wildcard":               `"rules":{"include":["*"]}`,
		"individual rule":        `"rules":{"include":["techs/go/*","practices/testing/limits"]}`,
		"pin to another version": `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/errors","version":"1.0.0","reason":"Not ready."}]}`,
		"pin of a rule it lacks": `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/other","version":"1.0.0","reason":"Keep."}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			config, snapshots := snapshotFixture(t)
			vendor, err := encodeStored(config, snapshots)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeStored(snapshotConfig(t, fields), vendor)
			requireSync(t, got, err)
		})
	}
}

// requireUpdate checks that decoding fails for a ref configuration changed since sync, which only an update takes.
func requireUpdate(t *testing.T, got map[string]snapshot, err error) {
	t.Helper()
	var validation errs.ValidationError
	if got != nil || !errors.As(err, &validation) || validation.ValidationLocation() != "lock.json.sources.team.ref" || !strings.Contains(validation.ValidationProblem(), "which sources.team.ref names, and sync doesn't move a source; run trailguide project update team") {
		t.Fatalf("got %v, %v; want a failure that asks for an update", got, err)
	}
}

// TestSnapshotAnEditedRefNeedsUpdateOnlyForAnotherRelease rejects a ref edited to name another library release than the lock entry's,
// which only an update takes, and asks for a sync for any other edited ref, which sync takes.
func TestSnapshotAnEditedRefNeedsUpdateOnlyForAnotherRelease(t *testing.T) {
	config, snapshots := snapshotFixture(t)
	vendor, err := encodeStored(config, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeStored(snapshotConfig(t, `"rules":{"include":["techs/go/*"]},"ref":"release/1"`), vendor)
	requireUpdate(t, got, err)
	for _, ref := range []string{"latest-release", "main"} {
		t.Run(ref, func(t *testing.T) {
			config, snapshots := snapshotFixture(t)
			vendor, err := encodeStored(config, snapshots)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeStored(snapshotConfig(t, `"rules":{"include":["techs/go/*"]},"ref":"`+ref+`"`), vendor)
			requireSync(t, got, err)
		})
	}
}

// TestSnapshotPinsThatMoveNothingNeedNoSync accepts, against a record written without pins, a pin at the imported
// version, with any reason, and a pin of a rule the record lists as retired, and the record never holds pins.
func TestSnapshotPinsThatMoveNothingNeedNoSync(t *testing.T) {
	pinned := snapshotConfig(t, `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/naming","version":"1.0.0","reason":"Waiting on #45."}]}`)
	_, snapshots := snapshotFixture(t)
	item := snapshots["team"]
	item.RetiredRules = []string{"techs/go/gone"}
	snapshots["team"] = item
	vendor, err := encodeStored(pinned, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(vendor[lockFile]), "pins") {
		t.Fatalf("the lock holds pins:\n%s", vendor[lockFile])
	}
	for name, fields := range map[string]string{
		"same pin":            `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/naming","version":"1.0.0","reason":"Waiting on #45."}]}`,
		"changed reason":      `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/naming","version":"1.0.0","reason":"Still waiting."}]}`,
		"added pin":           `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/naming","version":"1.0.0","reason":"Keep."},{"id":"techs/go/errors","version":"1.1.0","reason":"Keep."}]}`,
		"removed pin":         `"rules":{"include":["techs/go/*"]}`,
		"pin of retired rule": `"rules":{"include":["techs/go/*"],"pins":[{"id":"techs/go/gone","version":"3.0.0","reason":"Keep."}]}`,
	} {
		if _, err := decodeStored(snapshotConfig(t, fields), vendor); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestSnapshotRetiredEntriesRecordedAtSyncNeedNoSync accepts a pin or rules entry naming a rule the record lists as
// retired.
func TestSnapshotRetiredEntriesRecordedAtSyncNeedNoSync(t *testing.T) {
	config := snapshotConfig(t, `"rules":{"include":["techs/go/*","practices/testing/retired"],"pins":[{"id":"techs/go/gone","version":"1.0.0","reason":"Keep."}]}`)
	_, snapshots := snapshotFixture(t)
	item := snapshots["team"]
	item.RetiredRules, item.RuleSelection = []string{"practices/testing/retired", "techs/go/gone"}, config.Sources[0].Rules
	snapshots["team"] = item
	vendor, err := encodeStored(config, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeStored(config, vendor); err != nil {
		t.Fatal(err)
	}
}

// TestSnapshotExclusionOfAnImportedOrRetiredRuleNeedsNoSync accepts any exclusion naming a rule the snapshot imports
// or a rule its record lists as retired, however the exclusions changed since the record was written, because the
// record holds facts about the library, not the configuration's exclusions.
func TestSnapshotExclusionOfAnImportedOrRetiredRuleNeedsNoSync(t *testing.T) {
	config, snapshots := snapshotFixture(t)
	item := snapshots["team"]
	item.RetiredRules = []string{"techs/go/gone"}
	snapshots["team"] = item
	vendor, err := encodeStored(config, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	for name, fields := range map[string]string{
		"imported and retired":   `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/gone","reason":"Retired."},{"id":"techs/go/errors","reason":"Not for us."}]}`,
		"retired only":           `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/gone","reason":"Delete me."}]}`,
		"new imported exclusion": `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/naming","reason":"Not for us.","replacedBy":"techs/go/naming"}]}`,
		"no exclusions":          `"rules":{"include":["techs/go/*"]}`,
	} {
		if _, err := decodeStored(snapshotConfig(t, fields), vendor); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestSnapshotExclusionOfAnUnknownRuleRequiresSync refuses an exclusion whose rule isn't imported and that sync
// didn't record, such as a typo, so the intended rule doesn't stay active unnoticed.
func TestSnapshotExclusionOfAnUnknownRuleRequiresSync(t *testing.T) {
	config, snapshots := snapshotFixture(t)
	vendor, err := encodeStored(config, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeStored(snapshotConfig(t, `"rules":{"include":["techs/go/*"],"exclude":[{"id":"techs/go/erorrs","reason":"Typo."}]}`), vendor)
	var validation errs.ValidationError
	if got != nil || !errors.As(err, &validation) || validation.ValidationLocation() != "sources.team.rules.exclude.techs/go/erorrs" || !strings.Contains(validation.ValidationProblem(), "names no rule the library supplies") || !strings.Contains(validation.ValidationProblem(), "run trailguide project sync") {
		t.Fatalf("got %v, %v; want a failure naming the entry that asks for sync", got, err)
	}
}

// TestSnapshotCorruptionReturnsNoPartialResult covers malformed records and altered inventories.
func TestSnapshotCorruptionReturnsNoPartialResult(t *testing.T) {
	cases := []struct {
		name  string
		alter func(map[string][]byte)
	}{
		{"missing entry", func(v map[string][]byte) { v[lockFile] = []byte(`{"lockVersion":1,"sources":{}}`) }},
		{"missing content", func(v map[string][]byte) { delete(v, "team/LICENSE") }},
		{"missing older rule", func(v map[string][]byte) { delete(v, "team/rules/techs/go/naming.md") }},
		{"rule outside rules/", func(v map[string][]byte) {
			v["team/techs/go/naming.md"] = v["team/rules/techs/go/naming.md"]
			delete(v, "team/rules/techs/go/naming.md")
		}},
		{"changed bytes", func(v map[string][]byte) { v["team/LICENSE"] = []byte("Terms\nPreserved\n") }},
		{"extra content", func(v map[string][]byte) { v["team/untracked"] = []byte("x") }},
		{"removed source", func(v map[string][]byte) {
			v[lockFile] = []byte(strings.Replace(string(v[lockFile]), `"team": {`, `"retired": {`, 1))
		}},
		{"changed entry", func(v map[string][]byte) {
			v[lockFile] = []byte(strings.Replace(string(v[lockFile]), `"release": 2,`, `"release": 1,`, 1))
		}},
		{"per-source record too", func(v map[string][]byte) { v["team/_source.json"] = []byte("{}") }},
		{"invalid UTF8", func(v map[string][]byte) { v[lockFile] = []byte{255} }},
		{"null", func(v map[string][]byte) { v[lockFile] = []byte("null") }},
		{"trailing JSON", func(v map[string][]byte) { v[lockFile] = append(v[lockFile], []byte("{}")...) }},
		{"duplicate source", func(v map[string][]byte) {
			lock := string(v[lockFile])
			start, end := strings.Index(lock, `    "team": {`), strings.LastIndex(lock, "    }")
			v[lockFile] = []byte(lock[:end+5] + ",\n" + lock[start:])
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, s := snapshotFixture(t)
			v, e := encodeStored(c, s)
			if e != nil {
				t.Fatal(e)
			}
			tc.alter(v)
			got, e := decodeStored(c, v)
			var validation errs.ValidationError
			var failure *filetxn.Error
			if got != nil || !errors.As(e, &validation) && !errors.As(e, &failure) {
				t.Fatalf("got %v, %v", got, e)
			}
		})
	}
}

// TestLegacyRecordRelationships validates a per-source record that sync migrates: its identity, versions, and
// selection, rather than trusting hashes alone.
func TestLegacyRecordRelationships(t *testing.T) {
	rule := func(r map[string]any, id string) map[string]any {
		return r["rules"].(map[string]any)[id].(map[string]any)
	}
	cases := []struct {
		name  string
		alter func(map[string]any)
	}{
		{"format 1", func(r map[string]any) { r["formatVersion"] = 1 }},
		{"unknown", func(r map[string]any) { r["future"] = true }},
		{"wrong case", func(r map[string]any) { r["Repository"] = r["repository"]; delete(r, "repository") }},
		{"null optional", func(r map[string]any) { r["groupSelection"] = nil }},
		{"missing rules", func(r map[string]any) { delete(r, "rules") }},
		{"missing release", func(r map[string]any) { delete(r, "release") }},
		{"zero release", func(r map[string]any) { r["release"] = 0 }},
		{"older release", func(r map[string]any) { r["release"] = 1 }},
		{"invalid commit", func(r map[string]any) { r["resolvedCommit"] = "main" }},
		{"changed ref", func(r map[string]any) { r["ref"] = "v2.0.0" }},
		{"invalid ref", func(r map[string]any) { r["ref"] = "refs/remotes/origin/main" }},
		{"changed selection", func(r map[string]any) { r["groupSelection"] = "*" }},
		{"missing group", func(r map[string]any) { r["groups"] = []string{} }},
		{"duplicate group", func(r map[string]any) { r["groups"] = []string{"techs/go", "techs/go"} }},
		{"rule without release", func(r map[string]any) { rule(r, "techs/go/errors")["release"] = nil }},
		{"unreleased rule without ref", func(r map[string]any) {
			rule(r, "techs/go/errors")["release"] = nil
			rule(r, "techs/go/errors")["version"] = nil
		}},
		{"rule with invalid version", func(r map[string]any) { rule(r, "techs/go/errors")["version"] = "1.1" }},
		{"rule with another commit", func(r map[string]any) { rule(r, "techs/go/errors")["commit"] = releaseOne }},
		{"rule with unknown field", func(r map[string]any) { rule(r, "techs/go/errors")["notes"] = "x" }},
		{"invalid rule ID", func(r map[string]any) { r["rules"].(map[string]any)["Go"] = rule(r, "techs/go/errors") }},
		{"bad hash", func(r map[string]any) { r["files"].(map[string]any)["LICENSE"] = "123" }},
		{"reserved", func(r map[string]any) { r["files"].(map[string]any)["_source.json"] = strings.Repeat("a", 64) }},
		{"traversal", func(r map[string]any) { r["files"].(map[string]any)["../outside"] = strings.Repeat("a", 64) }},
		{"missing manifest", func(r map[string]any) { delete(r["files"].(map[string]any), "config.yaml") }},
		{"missing metadata", func(r map[string]any) { delete(r["files"].(map[string]any), "techs/go/_group.yaml") }},
		{"pin with a reason", func(r map[string]any) {
			r["pins"] = map[string]any{"techs/go/errors": map[string]any{"version": "1.1.0", "reason": "Keep."}}
		}},
		{"pin with an invalid version", func(r map[string]any) { r["pins"] = map[string]any{"techs/go/errors": "1.1"} }},
		{"pin of an invalid rule ID", func(r map[string]any) { r["pins"] = map[string]any{"Go": "1.1.0"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, s := snapshotFixture(t)
			v := legacyStored(t, c, s)
			if _, e := decodeLegacy(c, v); e != nil {
				t.Fatal(e)
			}
			var record map[string]any
			if e := json.Unmarshal(v["team/_source.json"], &record); e != nil {
				t.Fatal(e)
			}
			tc.alter(record)
			var e error
			if v["team/_source.json"], e = json.Marshal(record); e != nil {
				t.Fatal(e)
			}
			got, e := decodeLegacy(c, v)
			if got != nil || e == nil {
				t.Fatalf("accepted corrupt record: %v", got)
			}
		})
	}
}

// TestLockEntryRelationships validates a lock entry's identity, versions, and selection against configuration and
// the entry's own files, though its integrity matches, rather than trusting hashes alone.
func TestLockEntryRelationships(t *testing.T) {
	cases := map[string]func(*sourceRecord){
		"older release":              func(r *sourceRecord) { r.Release, r.Commit = 1, releaseOne },
		"changed selection":          func(r *sourceRecord) { r.Selection = json.RawMessage(`"*"`) },
		"missing group":              func(r *sourceRecord) { r.Groups = []string{} },
		"duplicate group":            func(r *sourceRecord) { r.Groups = []string{"techs/go", "techs/go"} },
		"invalid repository":         func(r *sourceRecord) { r.Repository = "not a repository" },
		"unreleased rule on release": func(r *sourceRecord) { r.Rules["techs/go/errors"] = recordRule{Commit: releaseTwo} },
		"missing manifest":           func(r *sourceRecord) { delete(r.Files, "config.yaml") },
		"missing metadata":           func(r *sourceRecord) { delete(r.Files, "techs/go/_group.yaml") },
		"traversal":                  func(r *sourceRecord) { r.Files["../outside"] = strings.Repeat("a", 64) },
		"reserved":                   func(r *sourceRecord) { r.Files["_source.json"] = strings.Repeat("a", 64) },
		"one release with two commits": func(r *sourceRecord) {
			r.Rules["techs/go/naming"] = recordRule{Version: r.Rules["techs/go/naming"].Version, Release: r.Rules["techs/go/errors"].Release, Commit: releaseOne}
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			c, s := snapshotFixture(t)
			v, e := encodeStored(c, s)
			if e != nil {
				t.Fatal(e)
			}
			v, e = editRecord(t, v, "team", edit)
			if e != nil {
				return
			}
			if got, e := decodeStored(c, v); got != nil || e == nil {
				t.Fatalf("accepted corrupt entry: %v", got)
			}
		})
	}
}

// TestSnapshotOfALibraryInTheLayoutBeforeVersion2IsUnsupported explains that an entry of a library's
// rule-library.yaml needs the library to move and the source to be resolved again.
func TestSnapshotOfALibraryInTheLayoutBeforeVersion2IsUnsupported(t *testing.T) {
	config, input := snapshotFixture(t)
	encoded, err := encodeStored(config, input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = editRecord(t, encoded, "team", func(record *sourceRecord) {
		record.Files["rule-library.yaml"] = record.Files["config.yaml"]
		delete(record.Files, "config.yaml")
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded["team/rule-library.yaml"] = encoded["team/config.yaml"]
	delete(encoded, "team/config.yaml")
	got, err := decodeStored(config, encoded)
	var validation errs.ValidationError
	want := "records a library in the layout from before this version of Trail Guide, with rule-library.yaml, which this version can't read. Once the library has moved its files into .trailguide/ and published a library release, run trailguide project sync --retake team to resolve the source again at its ref, which imports its unpinned rules at their newest versions"
	if got != nil || !errors.As(err, &validation) || validation.ValidationLocation() != "lock.json.sources.team" || validation.ValidationProblem() != want {
		t.Fatalf("got %v, %v; want %s", got, err, want)
	}
}

// TestSnapshotFormatOneIsUnsupported tells sync, which migrates per-source records, to delete vendor/ and sync, and
// offline reads to migrate first.
func TestSnapshotFormatOneIsUnsupported(t *testing.T) {
	config, _ := snapshotFixture(t)
	vendor := map[string][]byte{"team/_source.json": []byte(`{"formatVersion":1,"repository":"https://github.com/acme/rules","ref":"v1.0.0","resolvedCommit":"` + releaseOne + `","groups":["techs/go"],"files":{}}`)}
	_, err := readRecords(config, nil, vendor, currentLayout, true, nil)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(validation.ValidationProblem(), "delete .trailguide/vendor/ and run trailguide project sync") {
		t.Errorf("sync: %v", err)
	}
	var failure *filetxn.Error
	if _, err := decodeStored(config, vendor); !errors.As(err, &failure) || failure.Code != "lock-migration-needed" {
		t.Errorf("offline: %v", err)
	}
}

// TestRecordedSnapshotsReadRecordsWithoutCheckingConfiguration returns records for sync to compare.
func TestRecordedSnapshotsReadRecordsWithoutCheckingConfiguration(t *testing.T) {
	config, snapshots := snapshotFixture(t)
	vendor, err := encodeStored(config, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	vendor["team/LICENSE"] = []byte("Edited copy, which sync replaces.")
	changed := snapshotConfig(t, `"rules":{"include":["*"]}`)
	lock := vendor[lockFile]
	delete(vendor, lockFile)
	stored, err := readRecords(changed, lock, vendor, currentLayout, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorded := recordedSnapshots(changed, stored)
	want := snapshots["team"]
	want.Files = nil
	if !reflect.DeepEqual(recorded["team"], want) {
		t.Fatalf("got %+v, want %+v", recorded["team"], want)
	}
	stored, err = readRecords(changed, nil, map[string][]byte{}, currentLayout, true, nil)
	if recorded := recordedSnapshots(changed, stored); err != nil || len(recorded) != 0 {
		t.Fatalf("new source: %v, %v", recorded, err)
	}
}

// TestSnapshotRefChecks verifies what offline checks can about ref: the commit a SHA names, the library release
// a release tag names, and no library release for any other tag.
func TestSnapshotRefChecks(t *testing.T) {
	one := trailguide.RuleVersion{Major: 1}
	for _, test := range []struct {
		name, ref string
		release   int
		commit    string
		rules     map[string]library.ImportedRule
		ok        bool
	}{
		{"commit", releaseTwo, 0, releaseTwo, map[string]library.ImportedRule{"techs/go/errors": {Commit: releaseTwo}}, true},
		{"other commit", releaseOne, 0, releaseTwo, map[string]library.ImportedRule{"techs/go/errors": {Commit: releaseTwo}}, false},
		{"library release", "release/2", 2, releaseTwo, map[string]library.ImportedRule{"techs/go/errors": {Version: &one, Release: 1, Commit: releaseOne}}, true},
		{"another library release", "release/3", 2, releaseTwo, map[string]library.ImportedRule{"techs/go/errors": {Version: &one, Release: 1, Commit: releaseOne}}, false},
		{"rule from a later library release", "release/1", 1, releaseOne, map[string]library.ImportedRule{"techs/go/errors": {Version: &one, Release: 2, Commit: releaseTwo}}, false},
		{"unreleased rule from a library release", "release/2", 2, releaseTwo, map[string]library.ImportedRule{"techs/go/errors": {Commit: releaseTwo}}, false},
		{"other tag", "candidate", 0, releaseTwo, map[string]library.ImportedRule{"techs/go/errors": {Version: &one, Release: 1, Commit: releaseOne}}, true},
		{"other tag with a library release", "candidate", 2, releaseTwo, map[string]library.ImportedRule{"techs/go/errors": {Version: &one, Release: 1, Commit: releaseOne}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := snapshotConfig(t, `"rules":{"include":["techs/go/*"]},"ref":"`+test.ref+`"`)
			files := map[string][]byte{"config.yaml": []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`), "techs/go/_group.yaml": []byte(`{}`)}
			item := snapshot{Repository: config.Sources[0].Repository, Ref: gitRef(t, test.ref), Release: test.release, Commit: test.commit, Groups: []string{"techs/go"}, Selection: config.Sources[0].Groups, RuleSelection: []string{}, Rules: test.rules, Files: files}
			_, err := encodeStored(config, map[string]snapshot{"team": item})
			if (err == nil) != test.ok {
				t.Fatalf("got %v, want ok %v", err, test.ok)
			}
		})
	}
}

// TestSnapshotRefWrittenAnotherWayNeedsNoSync accepts a record of release/2 for a source that now writes the same ref
// as refs/tags/release/2, and asks for an update when the ref names another library release.
func TestSnapshotRefWrittenAnotherWayNeedsNoSync(t *testing.T) {
	recorded := snapshotConfig(t, `"rules":{"include":["techs/go/*"]},"ref":"release/2"`)
	_, snapshots := snapshotFixture(t)
	item := snapshots["team"]
	item.Ref = gitRef(t, "release/2")
	vendor, err := encodeStored(recorded, map[string]snapshot{"team": item})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeStored(snapshotConfig(t, `"rules":{"include":["techs/go/*"]},"ref":"refs/tags/release/2"`), vendor); err != nil {
		t.Fatal(err)
	}
	got, err := decodeStored(snapshotConfig(t, `"rules":{"include":["techs/go/*"]},"ref":"refs/tags/release/3"`), vendor)
	requireUpdate(t, got, err)
}

// TestSnapshotEmptySources encodes and decodes a local-only project and rejects a removed source's record.
func TestSnapshotEmptySources(t *testing.T) {
	c, s := snapshotFixture(t)
	v, e := encodeStored(c, s)
	if e != nil {
		t.Fatal(e)
	}
	empty := rules.Configuration{Sources: []rules.Source{}}
	encoded, e := encodeStored(empty, nil)
	if e != nil || len(encoded) != 1 {
		t.Fatalf("empty encode: %v", e)
	}
	decoded, e := decodeStored(empty, encoded)
	if e != nil || len(decoded) != 0 {
		t.Fatalf("empty decode: %v", e)
	}
	if got, e := decodeStored(empty, v); e == nil || got != nil {
		t.Fatal("accepted retired source")
	}
}

// TestSnapshotPortablePaths rejects ambiguous destination paths before encoding.
func TestSnapshotPortablePaths(t *testing.T) {
	for _, path := range []string{"../outside", "/absolute", "a\\b", "a:b", "a/./b", "empty.txt/child", "ASSETS/image.bin", "_source.json", "_SOURCE.json", "_source.json/child"} {
		t.Run(path, func(t *testing.T) {
			c, s := snapshotFixture(t)
			s["team"].Files[path] = []byte("x")
			if got, e := encodeStored(c, s); e == nil || got != nil {
				t.Fatalf("accepted %s", path)
			}
		})
	}
}

// TestSnapshotsDoNotReturnEarlierSourcesOnFailure verifies atomic results across source inventories.
func TestSnapshotsDoNotReturnEarlierSourcesOnFailure(t *testing.T) {
	config, snapshots := snapshotFixture(t)
	source := config.Sources[0]
	source.Name = "second"
	config.Sources = append(config.Sources, source)
	snapshots["second"] = snapshots["team"]
	vendor, err := encodeStored(config, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	delete(vendor, "second/LICENSE")
	if got, err := decodeStored(config, vendor); got != nil || err == nil {
		t.Fatalf("partial result: %v, %v", got, err)
	}
	second := snapshots["second"]
	second.Commit = "invalid"
	snapshots["second"] = second
	if got, err := encodeStored(config, snapshots); got != nil || err == nil {
		t.Fatalf("partial encoding: %v, %v", got, err)
	}
}

// TestSnapshotWildcardScope rejects a recorded group outside the selected wildcard's scope.
func TestSnapshotWildcardScope(t *testing.T) {
	config, snapshots := snapshotFixture(t)
	config.Sources[0].Groups = rules.GroupSelection{Pattern: "practices/*"}
	item := snapshots["team"]
	item.Selection = config.Sources[0].Groups
	snapshots["team"] = item
	if got, err := encodeStored(config, snapshots); got != nil || err == nil {
		t.Fatal("accepted technology group for practices wildcard")
	}
}

// skillSnapshots returns a library source, team, that installs skills/build with an executable script, and a source
// that isn't a library, tools, that installs tools/review from a pinned commit.
func skillSnapshots(t *testing.T) (rules.Configuration, map[string]snapshot) {
	t.Helper()
	_, snapshots := snapshotFixture(t)
	config, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"sources":{"team":{"repository":"https://github.com/acme/rules","ref":"release/2","rules":{"include":["techs/go/*"]},"skills":{"include":["skills/build"]}},"tools":{"repository":"https://github.com/acme/tools","ref":"` + releaseOne + `","skills":{"include":["*"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	team := snapshots["team"]
	team.SkillSelection = []string{"skills/build"}
	team.Skills = map[string]library.ImportedSkill{"skills/build": {Level: "offered", Included: true, UsedBy: []string{}}}
	team.AvailableSkills = map[string]library.ImportedSkill{"skills/build": {Level: "offered"}, "skills/deploy": {Level: "recommended"}}
	team.Files["skills/build/SKILL.md"] = []byte("---\nname: build\n---\n")
	team.Files["skills/build/run.sh"] = []byte("#!/bin/sh\n")
	team.Executable = []string{"skills/build/run.sh"}
	return config, map[string]snapshot{"team": team, "tools": {Repository: "https://github.com/acme/tools", Ref: gitRef(t, releaseOne), Commit: releaseOne, NonLibrary: true, Selection: rules.GroupSelection{Groups: []string{}},
		RetiredRules: []string{}, Groups: []string{}, RuleSelection: []string{}, Rules: map[string]library.ImportedRule{}, SkillSelection: []string{"*"},
		Skills: map[string]library.ImportedSkill{"tools/review": {Included: true, UsedBy: []string{}}}, AvailableSkills: map[string]library.ImportedSkill{"tools/review": {}, "tools/.hidden": {}}, Executable: []string{}, Files: map[string][]byte{"tools/review/SKILL.md": []byte("# Review\n")}}}
}

func TestSnapshotsRoundTripSkillsAndSourcesThatAreNotLibraries(t *testing.T) {
	config, input := skillSnapshots(t)
	encoded, err := encodeStored(config, input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeStored(config, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("round trip:\n%+v\nwant\n%+v", got, input)
	}
	if tools := lockEntry(t, encoded[lockFile], "tools"); tools.Kind != "non-library" || tools.Skills["tools/review"].Level != nil {
		t.Fatalf("lock:\n%s", encoded[lockFile])
	}
	if err := verifyExecutable(got, &filetxn.Tree{Files: map[string][]byte{}, Executable: []string{"team/skills/build/run.sh"}}); err != nil {
		t.Fatal(err)
	}
	if err := verifyExecutable(got, &filetxn.Tree{Files: map[string][]byte{}}); err == nil || !strings.Contains(err.Error(), "_managed/vendor/team/skills/build/run.sh") {
		t.Fatalf("got %v; want the vendored script's lost executable bit reported", err)
	}
}

func TestSnapshotRecordsRejectInvalidSkills(t *testing.T) {
	tests := map[string]struct {
		source string
		edit   func(*sourceRecord)
		want   string
	}{
		"library skill without a level": {"team", func(r *sourceRecord) { r.Skills["skills/build"] = recordSkill{} }, "with level required, recommended, offered, or dependency"},
		"unknown level": {"team", func(r *sourceRecord) {
			mandatory := "mandatory"
			r.AvailableSkills["skills/deploy"] = availableSkill{Level: &mandatory}
		}, "with level required, recommended, offered, or dependency"},
		"skill that isn't available": {"team", func(r *sourceRecord) { delete(r.AvailableSkills, "skills/build") }, "expected one of availableSkills"},
		"level that differs from the available skill's": {"team", func(r *sourceRecord) {
			required := "required"
			r.Skills["skills/build"] = recordSkill{Level: &required}
		}, "with the same level"},
		"library skill outside skills/": {"team", func(r *sourceRecord) {
			r.Skills["tools/build"] = r.Skills["skills/build"]
			r.Files["tools/build/SKILL.md"] = r.Files["skills/build/SKILL.md"]
		}, "with level required, recommended, offered, or dependency"},
		"other repository's skill with a level": {"tools", func(r *sourceRecord) { offered := "offered"; r.Skills["tools/review"] = recordSkill{Level: &offered} }, "with a null level"},
		"nested skill": {"tools", func(r *sourceRecord) {
			r.Skills["tools/review/inner"] = recordSkill{}
			r.Files["tools/review/inner/SKILL.md"] = r.Files["tools/review/SKILL.md"]
		}, "inside another skill's folder"},
		"skill without SKILL.md": {"tools", func(r *sourceRecord) {
			r.Skills["tools/other"], r.AvailableSkills["tools/other"] = recordSkill{Included: true, UsedBy: []string{}}, availableSkill{}
		}, "missing the skill's SKILL.md"},
		"executable file outside a skill": {"team", func(r *sourceRecord) { r.Executable = []string{"techs/go/errors.md"} }, "expected distinct skill files"},
		"unknown kind":                    {"team", func(r *sourceRecord) { r.Kind = "plugin" }, "expected library or non-library"},
		"skill neither included nor used": {"tools", func(r *sourceRecord) { r.Skills["tools/review"] = recordSkill{UsedBy: []string{}} }, "recorded only because other skills use it"},
		"used by an unrecorded skill": {"tools", func(r *sourceRecord) {
			r.Skills["tools/review"] = recordSkill{Included: true, UsedBy: []string{"tools/gone"}}
		}, "expected distinct other recorded skills"},
		"used by itself": {"tools", func(r *sourceRecord) {
			r.Skills["tools/review"] = recordSkill{Included: true, UsedBy: []string{"tools/review"}}
		}, "expected distinct other recorded skills"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config, input := skillSnapshots(t)
			vendor, err := encodeStored(config, input)
			if err != nil {
				t.Fatal(err)
			}
			if vendor, err = editRecord(t, vendor, test.source, test.edit); err == nil {
				_, err = decodeStored(config, vendor)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v; want %q", err, test.want)
			}
		})
	}
}

// TestSnapshotManifestIsReadAsTheLibrarysConsumer loads a snapshot's library offline, as build and check do, reading its
// manifest as the library's consumers read it: whatever the project schema version, project settings, or manifest
// fields this version doesn't know, but never without a supported library schema version.
func TestSnapshotManifestIsReadAsTheLibrarysConsumer(t *testing.T) {
	for name, test := range map[string]struct {
		manifest, location string
		unsupported        bool
	}{
		"the published contract":     {manifest: `{"library":{"schemaVersion":2,"name":"library"},"sources":{}}`},
		"project settings":           {manifest: `{"schemaVersion":9,"skillTargets":["future"],"sources":{"own":{"repository":"not an address"}},"library":{"schemaVersion":2,"name":"library"}}`},
		"an unknown manifest field":  {manifest: `{"library":{"schemaVersion":2,"name":"library","description":"Shared rules."},"sources":{}}`},
		"no schema version":          {manifest: `{"library":{},"sources":{}}`, location: "team/.trailguide/config.yaml: library.schemaVersion"},
		"a malformed schema version": {manifest: `{"library":{"schemaVersion":"1"},"sources":{}}`, location: "team/.trailguide/config.yaml: library.schemaVersion"},
		"another schema version":     {manifest: `{"library":{"schemaVersion":3},"sources":{}}`, unsupported: true},
	} {
		t.Run(name, func(t *testing.T) {
			config, input := snapshotFixture(t)
			team := input["team"]
			team.Files["config.yaml"] = []byte(test.manifest)
			encoded, err := encodeStored(config, input)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeStored(config, encoded)
			if err != nil {
				t.Fatal(err)
			}
			_, err = library.LoadSource(context.Background(), decoded["team"].Source(), "team", config.Sources[0].Groups, nil)
			var validation errs.ValidationError
			var unsupported *trailguide.UnsupportedLibrarySchemaError
			switch {
			case test.unsupported:
				if !errors.As(err, &unsupported) || unsupported.SchemaVersion != 3 {
					t.Fatalf("got %v; want an unsupported library schema version", err)
				}
			case test.location != "":
				if !errors.As(err, &validation) || validation.ValidationLocation() != test.location {
					t.Fatalf("got %v; want a validation error at %s", err, test.location)
				}
			case err != nil:
				t.Fatal(err)
			}
		})
	}
}
