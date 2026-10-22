// Check release tag names and the split of tag messages into notes and a record at their boundaries.

package trailguide_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

// TestParseReleaseTag_AcceptsOnlyCanonicalReleaseNumbers returns the number of release/<number> and refuses every
// other spelling with ErrNotReleaseTag.
func TestParseReleaseTag_AcceptsOnlyCanonicalReleaseNumbers(t *testing.T) {
	for _, test := range []struct {
		name string
		want int
	}{
		{"release/1", 1},
		{"release/10", 10},
		{"release/999999999", 999_999_999},
		{"release/0", 0},
		{"release/01", 0},
		{"release/-1", 0},
		{"release/1a", 0},
		{"release/1000000000", 0},
		{"release/", 0},
		{"releases/1", 0},
		{"refs/tags/release/1", 0},
		{"v1.0.0", 0},
		{"", 0},
	} {
		got, err := trailguide.ParseReleaseTag(test.name)
		if test.want == 0 {
			if !errors.Is(err, trailguide.ErrNotReleaseTag) || got != 0 {
				t.Errorf("ParseReleaseTag(%q) = %d, %v; want ErrNotReleaseTag", test.name, got, err)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Errorf("ParseReleaseTag(%q) = %d, %v; want %d", test.name, got, err, test.want)
		}
	}
}

// record2 is a minimal valid release record for release/2.
const record2 = "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/a: 1.0.0\n"

// TestParseReleaseMessage_SplitsAtTheLastSeparatorLine keeps earlier --- lines in the notes, without trailing line
// breaks.
func TestParseReleaseMessage_SplitsAtTheLastSeparatorLine(t *testing.T) {
	for _, test := range []struct {
		name, message, notes string
	}{
		{"one separator", "Notes.\n---\n" + record2, "Notes."},
		{"several separators", "Intro.\n---\nMore notes.\n---\n\n---\n" + record2, "Intro.\n---\nMore notes.\n---"},
		{"CRLF line endings", "Notes.\r\n---\r\n" + strings.ReplaceAll(record2, "\n", "\r\n"), "Notes."},
		{"empty notes", "---\n" + record2, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			notes, record, err := trailguide.ParseReleaseMessage("release/2", []byte(test.message))
			if err != nil || notes != test.notes || record.Release != 2 || record.Rules["techs/go/a"] != trailguide.FirstRuleVersion {
				t.Fatalf("got %q, %+v, %v; want notes %q", notes, record, err, test.notes)
			}
		})
	}
}

// TestParseReleaseMessage_RefusesMessagesWithoutARecord fails when no line is exactly ---, including a separator
// with surrounding spaces, or when the text after the last separator isn't the record.
func TestParseReleaseMessage_RefusesMessagesWithoutARecord(t *testing.T) {
	for name, message := range map[string]string{
		"no separator":           "Notes.\n" + record2,
		"indented separator":     "Notes.\n ---\n" + record2,
		"separator with spaces":  "Notes.\n--- \n" + record2,
		"longer separator":       "Notes.\n----\n" + record2,
		"record before notes":    record2 + "---\nNotes.\n",
		"empty message":          "",
		"separator only":         "---\n",
		"record for release one": "Notes.\n---\nschemaVersion: 1\nrelease: 1\nrules: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := trailguide.ParseReleaseMessage("release/2", []byte(message)); err == nil || errors.Is(err, trailguide.ErrNotReleaseTag) {
				t.Fatalf("got %v; want an invalid record", err)
			}
		})
	}
}

// TestParseReleaseMessage_RefusesTagNamesThatArentReleases reports the tag name with ErrNotReleaseTag before reading
// the message.
func TestParseReleaseMessage_RefusesTagNamesThatArentReleases(t *testing.T) {
	_, _, err := trailguide.ParseReleaseMessage("release/02", []byte("Notes.\n---\n"+record2))
	if !errors.Is(err, trailguide.ErrNotReleaseTag) {
		t.Fatalf("got %v; want ErrNotReleaseTag", err)
	}
}

// TestParseReleaseMessage_IgnoresTheSignatureOfASignedTag reads a message that still holds the signature Git appends
// to a signed tag's message, whether it is signed with GPG, SSH, or X.509, and keeps it out of the notes.
func TestParseReleaseMessage_IgnoresTheSignatureOfASignedTag(t *testing.T) {
	for _, signature := range []string{
		"-----BEGIN PGP SIGNATURE-----\n\niQEzBAABCAAdFiEE\n-----END PGP SIGNATURE-----\n",
		"-----BEGIN SSH SIGNATURE-----\nU1NIU0lH\n-----END SSH SIGNATURE-----\n",
		"-----BEGIN SIGNED MESSAGE-----\nMIIF\n-----END SIGNED MESSAGE-----\n",
	} {
		notes, record, err := trailguide.ParseReleaseMessage("release/2", []byte("Notes.\n---\n"+record2+signature))
		if err != nil || notes != "Notes." || record.Release != 2 || len(record.Rules) != 1 {
			t.Fatalf("got %q, %+v, %v", notes, record, err)
		}
	}
}
