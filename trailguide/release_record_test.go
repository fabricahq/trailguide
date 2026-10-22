// Check release tag message parsing against independent fixtures, including record consistency.

package trailguide_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

// TestReleaseMessageFixtures checks the notes, the parsed record, and exact diagnostics.
func TestReleaseMessageFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/release-records/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID, Tag, Message string
		Expected         struct {
			OK    bool
			Notes string
			Value json.RawMessage
			// Error's Kind is "unsupported" for a record schema version newer than the parser reads, "notReleaseTag" for a tag
			// name that isn't release/<number>, and empty for a validation error.
			Error *struct{ Message, Location, Kind string }
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.ID, func(t *testing.T) {
			notes, got, err := trailguide.ParseReleaseMessage(test.Tag, []byte(test.Message))
			if !test.Expected.OK && test.Expected.Error.Kind == "unsupported" {
				var unsupported *trailguide.UnsupportedReleaseRecordError
				if !errors.As(err, &unsupported) || err.Error() != test.Expected.Error.Message || unsupported.Location != test.Expected.Error.Location {
					t.Fatalf("got %+v, %v; want %+v", got, err, test.Expected.Error)
				}
				return
			}
			if !test.Expected.OK && test.Expected.Error.Kind == "notReleaseTag" {
				if !errors.Is(err, trailguide.ErrNotReleaseTag) || err.Error() != test.Expected.Error.Message {
					t.Fatalf("got %+v, %v; want %+v", got, err, test.Expected.Error)
				}
				return
			}
			if !test.Expected.OK {
				var validation *trailguide.ValidationError
				if !errors.As(err, &validation) || err.Error() != test.Expected.Error.Message || validation.Location != test.Expected.Error.Location {
					t.Fatalf("got %+v, %v; want %+v", got, err, test.Expected.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var expected trailguide.ReleaseRecord
			if err := json.Unmarshal(test.Expected.Value, &expected); err != nil {
				t.Fatal(err)
			}
			if notes != test.Expected.Notes || !reflect.DeepEqual(got, expected) {
				t.Fatalf("got %q, %+v; want %q, %+v", notes, got, test.Expected.Notes, expected)
			}
		})
	}
}

// recordWith returns a release/2 record whose section, one of rules, changes, retired, or libraryFiles, has count
// entries, with the rules entries the changes need, up to the 10,000 rules allows.
func recordWith(section string, count int) []byte {
	var record strings.Builder
	record.WriteString("schemaVersion: 1\nrelease: 2\n")
	if section == "rules" || section == "changes" {
		ruleCount := count
		if section == "changes" {
			ruleCount = min(count, 10_000)
		}
		record.WriteString("rules:\n")
		for i := range ruleCount {
			fmt.Fprintf(&record, "  techs/go/r%d: 1.0.0\n", i)
		}
	} else {
		record.WriteString("rules: {}\n")
	}
	switch section {
	case "changes":
		record.WriteString("changes:\n")
		for i := range count {
			fmt.Fprintf(&record, "  techs/go/r%d: {change: new, summaries: [Add.]}\n", i)
		}
	case "retired":
		record.WriteString("retired:\n")
		for i := range count {
			fmt.Fprintf(&record, "  techs/go/r%d: {lastVersion: 1.0.0, summaries: [Gone.]}\n", i)
		}
	case "libraryFiles":
		record.WriteString("libraryFiles:\n")
		for i := range count {
			fmt.Fprintf(&record, "  - assets/f%d.md\n", i)
		}
	}
	return []byte(record.String())
}

// TestParseReleaseRecord_LimitsEachCollection accepts each collection at its documented limit and refuses one
// more entry, naming the limit.
func TestParseReleaseRecord_LimitsEachCollection(t *testing.T) {
	for section, limit := range map[string]int{"rules": 10_000, "changes": 10_000, "retired": 10_000, "libraryFiles": 20_000} {
		t.Run(section, func(t *testing.T) {
			if _, err := trailguide.ParseReleaseRecord(recordWith(section, limit), "release/2"); err != nil {
				t.Fatalf("refused %d entries: %v", limit, err)
			}
			_, err := trailguide.ParseReleaseRecord(recordWith(section, limit+1), "release/2")
			var invalid *trailguide.ValidationError
			want := fmt.Sprintf("expected at most %d,000 entries", limit/1000)
			if !errors.As(err, &invalid) || invalid.Location != "release/2."+section || invalid.Problem != want {
				t.Fatalf("got %v; want %q", err, want)
			}
		})
	}
}

// TestParseReleaseRecord_RefusesDuplicateLibraryFilesInLargeLists finds a duplicate path at the end of the
// longest list the limit allows.
func TestParseReleaseRecord_RefusesDuplicateLibraryFilesInLargeLists(t *testing.T) {
	record := append(recordWith("libraryFiles", 19_999), "  - assets/f0.md\n"...)
	_, err := trailguide.ParseReleaseRecord(record, "release/2")
	var invalid *trailguide.ValidationError
	if !errors.As(err, &invalid) || invalid.Location != "release/2.libraryFiles[19999]" || !strings.Contains(invalid.Problem, "duplicate path") {
		t.Fatalf("got %v", err)
	}
}

// TestParseReleaseRecord_IgnoresUnknownFieldsAtEveryLevel reads a record that a later format extended at the top
// level, in a change, and in a retirement, with values of any shape, as the record without them.
func TestParseReleaseRecord_IgnoresUnknownFieldsAtEveryLevel(t *testing.T) {
	const known = "schemaVersion: 1\nrelease: 3\n" +
		"rules:\n  techs/go/a: 1.1.0\n" +
		"changes:\n  techs/go/a:\n    change: minor\n    from: 1.0.0\n    summaries: [Add an example.]\n" +
		"retired:\n  techs/go/b:\n    lastVersion: 2.0.0\n    replacedBy: techs/go/a\n    summaries: [Covered by a.]\n" +
		"libraryFiles: [techs/go/_group.yaml]\n"
	const extended = "schemaVersion: 1\nrelease: 3\npublishedAt: 2026-10-01T12:00:00Z\nsigner: {name: Fixture, keys: [1, 2]}\n" +
		"rules:\n  techs/go/a: 1.1.0\n" +
		"changes:\n  techs/go/a:\n    change: minor\n    from: 1.0.0\n    summaries: [Add an example.]\n    notes: [{id: 7}]\n    breaking: false\n" +
		"retired:\n  techs/go/b:\n    lastVersion: 2.0.0\n    replacedBy: techs/go/a\n    summaries: [Covered by a.]\n    retiredAt: 2026-10-01\n" +
		"libraryFiles: [techs/go/_group.yaml]\nfutureSection:\n  techs/go/a: {anything: ~}\n"
	want, err := trailguide.ParseReleaseRecord([]byte(known), "release/3")
	if err != nil {
		t.Fatal(err)
	}
	got, err := trailguide.ParseReleaseRecord([]byte(extended), "release/3")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
}

// TestParseReleaseRecord_AppliesDocumentRulesToUnknownFields refuses anchors, aliases, explicit tags, duplicate
// keys, and several documents even in fields it would otherwise ignore.
func TestParseReleaseRecord_AppliesDocumentRulesToUnknownFields(t *testing.T) {
	for name, extra := range map[string]string{
		"anchor":               "future: &shared {a: 1}\n",
		"alias":                "future: {a: 1}\nlater: *shared\n",
		"explicit tag":         "future: !!str 1\n",
		"duplicate key":        "future: 1\nfuture: 2\n",
		"nested duplicate key": "future: {a: 1, a: 2}\n",
		"two documents":        "---\nfuture: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := trailguide.ParseReleaseRecord([]byte(record2+extra), "release/2"); err == nil {
				t.Fatal("accepted the record")
			}
		})
	}
}

// TestParseReleaseRecord_ChecksTheSchemaVersionBeforeTheContent reads schema version 1, reports a newer one with
// *UnsupportedReleaseRecordError whatever the rest of the record holds, and refuses a missing or malformed schema
// version as an invalid record.
func TestParseReleaseRecord_ChecksTheSchemaVersionBeforeTheContent(t *testing.T) {
	if record, err := trailguide.ParseReleaseRecord([]byte(record2), "release/2"); err != nil || record.Release != 2 {
		t.Fatalf("schema version %d: got %+v, %v", trailguide.ReleaseRecordSchemaVersion, record, err)
	}
	for name, input := range map[string]string{
		"newer schema version":                "schemaVersion: 2\nrelease: 2\nrules:\n  techs/go/a: 1.0.0\n",
		"newer schema version, other content": "schemaVersion: 2\nrelease: second\nrules: [techs/go/a]\nchanges: none\n",
		"newer schema version, version last":  "release: second\nschemaVersion: 2\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := trailguide.ParseReleaseRecord([]byte(input), "release/2")
			var unsupported *trailguide.UnsupportedReleaseRecordError
			if !errors.As(err, &unsupported) || unsupported.SchemaVersion != 2 || unsupported.Location != "release/2.schemaVersion" {
				t.Fatalf("got %v; want UnsupportedReleaseRecordError for schema version 2", err)
			}
		})
	}
	for name, version := range map[string]string{"missing": "", "zero": "schemaVersion: 0\n", "negative": "schemaVersion: -1\n", "text": "schemaVersion: one\n", "list": "schemaVersion: [1]\n"} {
		t.Run(name, func(t *testing.T) {
			_, err := trailguide.ParseReleaseRecord([]byte(version+"release: 2\nrules: {}\n"), "release/2")
			var unsupported *trailguide.UnsupportedReleaseRecordError
			if err == nil || errors.As(err, &unsupported) {
				t.Fatalf("got %v; want an invalid record", err)
			}
		})
	}
}
