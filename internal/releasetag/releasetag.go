// Package releasetag reads a library's release/<number> tags from a Git repository: it lists their tag metadata and
// reads their release notes and records within the release tag limits. Callers own getting the tags into the
// repository, deciding which tags to trust, and describing failures in their own words.
package releasetag

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/trailguide"
)

// MaxBytes bounds one release tag object, its message and any signature included, in bytes. Publishing refuses a
// larger tag, and callers of Read refuse to read one.
const MaxBytes = 8 << 20

const (
	// maxListingBytes bounds Git's listing of release tags.
	maxListingBytes = 8 << 20
	// maxBatchBytes bounds the tag objects one Git process reads; more tags use several processes.
	maxBatchBytes = 16 << 20
)

// objectID accepts a full SHA-1 or SHA-256 object name.
var objectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Tag is one release/<number> tag as the repository holds it.
type Tag struct {
	Number int
	// Object is the tag's object ID and Type that object's type, "tag" for an annotated tag. Size is its length in
	// bytes.
	Object, Type string
	Size         int
	// Target is the object an annotated tag tags, and TargetType that object's type; both are empty for a
	// lightweight tag.
	Target, TargetType string
}

// Name returns the tag's short name, such as release/4.
func (t Tag) Name() string { return "release/" + strconv.Itoa(t.Number) }

// Release is what an annotated release tag's message holds: the release notes before the line containing only ---,
// and the release record after it.
type Release struct {
	Notes  string
	Record trailguide.ReleaseRecord
}

// RecordError reports a release tag whose message isn't release notes followed by a release record this version of
// Trail Guide can read, including a record whose number differs from the tag's. Err is the parser's error: a
// *trailguide.UnsupportedReleaseRecordError for a record in a newer schema version, which callers report separately, and
// otherwise an errs.ValidationError with its location.
type RecordError struct {
	Tag string
	Err error
}

// Error names the tag and the record's problem.
func (e *RecordError) Error() string { return "invalid release record: " + e.Problem() }

// Problem returns the record's problem starting with the tag, once: a parser error whose location already starts
// with the tag, such as release/3.release, names it itself.
func (e *RecordError) Problem() string {
	if problem := e.Err.Error(); strings.HasPrefix(problem, e.Tag+":") || strings.HasPrefix(problem, e.Tag+".") {
		return problem
	}
	return e.Tag + ": " + e.Err.Error()
}

// Unwrap returns the parser's validation error.
func (e *RecordError) Unwrap() error { return e.Err }

// objectTypes are the types a Git object can have; a listing with another fails, so no other text reaches a message.
var objectTypes = []string{"commit", "tree", "blob", "tag"}

// List returns the release/<number> tags in the repository at dir, in ascending number order, leaving out other
// tags under release/, such as release/01. When merged isn't empty, it lists only the tags reachable from that
// revision. It doesn't judge what the tags hold. A listing in an unexpected format fails with code git-failed.
func List(ctx context.Context, runner gitexec.Runner, dir, merged string) ([]Tag, error) {
	args := []string{"for-each-ref", "--format=%(refname)%00%(objecttype)%00%(objectname)%00%(objectsize)%00%(*objecttype)%00%(*objectname)"}
	if merged != "" {
		args = append(args, "--merged="+merged)
	}
	listing, err := runner.Output(ctx, dir, append(args, "refs/tags/release/"), maxListingBytes)
	if err != nil {
		return nil, err
	}
	unexpected := gitexec.Fail("git-failed", "Git listed release tags in an unexpected format.", nil)
	tags := []Tag{}
	for line := range strings.Lines(string(listing)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\x00")
		if len(fields) != 6 {
			return nil, unexpected
		}
		number, err := trailguide.ParseReleaseTag(strings.TrimPrefix(fields[0], "refs/tags/"))
		if err != nil {
			continue
		}
		size, err := strconv.Atoi(fields[3])
		if err != nil || size < 0 || !objectID.MatchString(fields[2]) || fields[5] != "" && !objectID.MatchString(fields[5]) ||
			!slices.Contains(objectTypes, fields[1]) || fields[4] != "" && !slices.Contains(objectTypes, fields[4]) {
			return nil, unexpected
		}
		tags = append(tags, Tag{Number: number, Object: fields[2], Type: fields[1], Size: size, Target: fields[5], TargetType: fields[4]})
	}
	slices.SortFunc(tags, func(a, b Tag) int { return a.Number - b.Number })
	return tags, nil
}

// Read passes the release notes and record of each of tags to each, in order, with the tag's index in tags. The
// tags must be annotated tags no larger than MaxBytes, as their listing shows them. It reads tag objects in
// batches of at most 16 MiB and keeps nothing it has passed on, so the memory it holds stays within one batch
// however many releases a library has; callers keep only what they need, such as records without notes. It stops
// between records when ctx ends, since parsing thousands of large records takes long. An invalid record fails
// with a *RecordError, objects that don't match their listing fail with code git-failed, and an error from each
// stops reading and is returned as is.
func Read(ctx context.Context, runner gitexec.Runner, dir string, tags []Tag, each func(int, Release) error) error {
	for start := 0; start < len(tags); {
		end, total := start, 0
		var input strings.Builder
		for end < len(tags) && (end == start || total+tags[end].Size <= maxBatchBytes) {
			total += tags[end].Size + 128
			input.WriteString(tags[end].Object + "\n")
			end++
		}
		result, err := runner.Run(ctx, dir, []string{"cat-file", "--batch"}, total+4096, []byte(input.String()))
		if err != nil {
			return err
		}
		if result.Status != 0 {
			return gitexec.Fail("git-failed", "Git could not read the library's release tags.", nil)
		}
		remaining := result.Output
		for i := start; i < end; i++ {
			if err := ctx.Err(); err != nil {
				return gitexec.ContextFailure(err)
			}
			tag := tags[i]
			header, rest, ok := bytes.Cut(remaining, []byte{'\n'})
			if !ok || string(header) != tag.Object+" tag "+strconv.Itoa(tag.Size) || len(rest) < tag.Size+1 || rest[tag.Size] != '\n' {
				return gitexec.Fail("git-failed", "Git returned inconsistent or incomplete release tags.", nil)
			}
			release, err := ParseObject(tag.Name(), rest[:tag.Size])
			if err != nil {
				return &RecordError{Tag: tag.Name(), Err: err}
			}
			if err := each(i, release); err != nil {
				return err
			}
			remaining = rest[tag.Size+1:]
		}
		start = end
	}
	return nil
}

// ParseObject returns the release notes and record in a raw annotated tag object, as git cat-file prints it, of the
// library release tag named tag. It skips the object's headers and reads its message with
// trailguide.ParseReleaseMessage, which ignores a signature after the message, and returns that function's
// errors.
func ParseObject(tag string, object []byte) (Release, error) {
	_, message, _ := bytes.Cut(object, []byte("\n\n"))
	notes, record, err := trailguide.ParseReleaseMessage(tag, message)
	if err != nil {
		return Release{}, err
	}
	return Release{Notes: notes, Record: record}, nil
}
