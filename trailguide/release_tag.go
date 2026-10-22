// Recognize library release tag names and split their messages into release notes and a release record.

package trailguide

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/fabricahq/trailguide/internal/gitsource"
)

// ErrNotReleaseTag identifies a tag name that isn't a library release tag, release/<number>. Trail Guide ignores
// such tags, so a catalog can skip them too. Check for it with errors.Is.
var ErrNotReleaseTag = errors.New("not a library release tag")

// releaseSeparator divides a release tag's Markdown notes from its YAML record.
const releaseSeparator = "---"

// ParseReleaseTag returns the library release number of a tag named release/<number>, such as release/4, from the
// tag's short name, without refs/tags/. Numbers start at 1 and are below one billion. It rejects every other name,
// including leading zeros, such as release/01, so each library release has exactly one tag name, with an error
// that wraps ErrNotReleaseTag.
func ParseReleaseTag(name string) (int, error) {
	number, ok := gitsource.ReleaseNumber(name)
	if !ok {
		return 0, fmt.Errorf("%s: %w: expected release/<number>, such as release/4", name, ErrNotReleaseTag)
	}
	return number, nil
}

// ParseReleaseMessage parses the message of the annotated library release tag named tag, returning its Markdown
// release notes and its release record. It splits the message at its last line containing only ---, so the notes
// may contain --- lines themselves, and it ignores a signature that Git appends to a signed tag's message. The
// record must be valid, as ParseReleaseRecord requires, and its release number must match the tag's. It fails
// with an error that wraps ErrNotReleaseTag when tag isn't a release tag name, and with
// *UnsupportedReleaseRecordError for a record in a newer schema version.
func ParseReleaseMessage(tag string, message []byte) (string, ReleaseRecord, error) {
	number, err := ParseReleaseTag(tag)
	if err != nil {
		return "", ReleaseRecord{}, err
	}
	lines := bytes.Split(unsigned(message), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if string(bytes.TrimRight(lines[i], "\r")) != releaseSeparator {
			continue
		}
		notes := string(bytes.TrimRight(bytes.Join(lines[:i], []byte("\n")), "\r\n"))
		record, err := ParseReleaseRecord(bytes.Join(lines[i+1:], []byte("\n")), tag)
		if err != nil {
			return "", ReleaseRecord{}, err
		}
		if record.Release != number {
			return "", ReleaseRecord{}, invalid(tag+".release", "the record is for library release "+strconv.Itoa(record.Release)+", but its tag is "+tag)
		}
		return notes, record, nil
	}
	return "", ReleaseRecord{}, invalid(tag, "expected release notes, a line containing only ---, and a release record")
}

// signatureHeaders begin a signature that Git appends to a signed tag's message.
var signatureHeaders = []string{"-----BEGIN PGP SIGNATURE-----", "-----BEGIN PGP MESSAGE-----", "-----BEGIN SSH SIGNATURE-----", "-----BEGIN SIGNED MESSAGE-----"}

// unsigned returns message without the signature Git appends to a signed tag's message, if it has one. Like Git,
// it treats the last line that starts a signature as the signature's start.
func unsigned(message []byte) []byte {
	end := len(message)
	for offset := 0; offset < len(message); {
		line := message[offset:]
		if slices.ContainsFunc(signatureHeaders, func(header string) bool { return bytes.HasPrefix(line, []byte(header)) }) {
			end = offset
		}
		next := bytes.IndexByte(line, '\n')
		if next < 0 {
			break
		}
		offset += next + 1
	}
	return message[:end]
}
