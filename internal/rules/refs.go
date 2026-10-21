// Classify Git refs and validate strict semantic-version tags without Git access.

package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/trailguide"
)

// GitRefKind distinguishes a complete commit SHA, a tag name, and a branch name.
type GitRefKind string

const (
	GitRefCommit GitRefKind = "commit"
	// GitRefTag is a tag written with refs/tags/, or a bare name, such as main or release/5, which a repository can
	// resolve as a branch instead; see GitRef.Bare.
	GitRefTag    GitRefKind = "tag"
	GitRefBranch GitRefKind = "branch"
)

// GitRef is a validated revision: a full commit SHA, a tag, or a branch, without establishing that it exists. It
// keeps the text as authored for display and records, and compares by its canonical form. Outside this package, only
// ParseGitRef and UnmarshalText build a nonzero GitRef, so every nonzero one is valid. The zero value means no ref.
type GitRef struct {
	kind GitRefKind
	// canonical is the lowercase 40-digit SHA of a commit, or the full name of a tag, including refs/tags/, or of a
	// branch, including refs/heads/. A bare name's is its tag's.
	canonical string
	// text is the ref as authored, such as release/5 or an uppercase SHA.
	text string
}

// IsZero reports whether the ref is the zero value, which means no ref.
func (r GitRef) IsZero() bool { return r.kind == "" }

// Kind returns GitRefCommit, GitRefTag, or GitRefBranch, or "" for the zero value. A bare name is GitRefTag.
func (r GitRef) Kind() GitRefKind { return r.kind }

// Bare reports whether the ref is a name written without refs/tags/ or refs/heads/, such as main or release/5. Only
// the repository decides whether such a name is a tag or a branch, though Kind and Canonical describe it as a tag.
func (r GitRef) Bare() bool {
	return r.kind == GitRefTag && !strings.HasPrefix(r.text, "refs/tags/")
}

// Name returns a tag's or branch's name without refs/tags/ or refs/heads/, such as main; it is empty for a commit
// and for the zero value.
func (r GitRef) Name() string {
	if r.kind == GitRefCommit {
		return ""
	}
	_, name, _ := strings.Cut(strings.TrimPrefix(r.canonical, "refs/"), "/")
	return name
}

// Canonical returns the lowercase SHA of a commit, or the full name of a tag, including refs/tags/, or of a branch,
// including refs/heads/, which Git resolves exactly; it is empty for the zero value. A bare name's is its tag's.
func (r GitRef) Canonical() string { return r.canonical }

// String returns the ref as authored, such as release/5 even when it equals refs/tags/release/5; it is empty for
// the zero value.
func (r GitRef) String() string { return r.text }

// Equal reports whether two refs have the same canonical form, so release/5 equals refs/tags/release/5 and a
// commit SHA equals its uppercase spelling, while main never equals refs/heads/main. Two zero values are equal.
func (r GitRef) Equal(other GitRef) bool {
	return r.kind == other.kind && r.canonical == other.canonical
}

// LatestReleaseRef is the ref of a library source that takes the library's newest library release, at its first
// sync and at each update.
const LatestReleaseRef = "latest-release"

// LatestRelease reports whether the ref is the word latest-release, which names a library's newest library release
// rather than a branch or tag; refs/heads/latest-release and refs/tags/latest-release name those.
func (r GitRef) LatestRelease() bool { return r.text == LatestReleaseRef }

// Release returns the number of the library release the ref names, such as 5 for release/5 or
// refs/tags/release/5, or 0 when it names no library release.
func (r GitRef) Release() int {
	if r.kind != GitRefTag {
		return 0
	}
	number, err := trailguide.ParseReleaseTag(r.Name())
	if err != nil {
		return 0
	}
	return number
}

// FollowsReleases reports whether the ref chooses a library release, one by number or latest-release, so a library source
// with it imports rule versions and can pin rules.
func (r GitRef) FollowsReleases() bool { return r.LatestRelease() || r.Release() > 0 }

// WithRelease returns a ref naming library release number, spelled as r is, with or without refs/tags/.
func (r GitRef) WithRelease(number int) GitRef {
	prefix := ""
	if strings.HasPrefix(r.text, "refs/tags/") {
		prefix = "refs/tags/"
	}
	name := "release/" + strconv.Itoa(number)
	return GitRef{kind: GitRefTag, canonical: "refs/tags/" + name, text: prefix + name}
}

// MarshalText returns the ref as authored, or empty text for the zero value.
func (r GitRef) MarshalText() ([]byte, error) { return []byte(r.text), nil }

// UnmarshalText parses text with ParseGitRef, reading empty text as the zero value, so MarshalText's output
// round-trips. Invalid text fails with a *ValidationError and leaves r unchanged.
func (r *GitRef) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*r = GitRef{}
		return nil
	}
	parsed, err := ParseGitRef(string(text), "ref")
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

var (
	commitRef      = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
	shortCommitRef = regexp.MustCompile(`^[a-fA-F0-9]{4,39}$`)
	unsafeRef      = regexp.MustCompile(`[\x00-\x20\x7f~^:?*\[\\]|\.\.|@\{`)
)

// ParseGitRef accepts a full commit SHA, a tag prefixed with refs/tags/, a branch prefixed with refs/heads/, or a
// bare name, such as main, which a repository resolves as its tag or branch of that name. Abbreviated commit-shaped
// names require a prefix. Errors return a zero GitRef. Location is the caller's complete diagnostic field path, such
// as sources.team.ref.
func ParseGitRef(ref, location string) (GitRef, error) {
	if strings.TrimFunc(ref, decode.IsSpace) == "" {
		return GitRef{}, invalid(location, "expected nonempty text")
	}
	if commitRef.MatchString(ref) {
		if strings.Trim(ref, "0") == "" {
			return GitRef{}, invalid(location, "the all-zero SHA names no commit; use a full commit SHA, a tag, or a branch")
		}
		return GitRef{kind: GitRefCommit, canonical: strings.ToLower(ref), text: ref}, nil
	}
	kind, prefix := GitRefTag, "refs/tags/"
	if strings.HasPrefix(ref, "refs/heads/") {
		kind, prefix = GitRefBranch, "refs/heads/"
	}
	name := strings.TrimPrefix(ref, prefix)
	if unsafeRef.MatchString(name) || name == "@" || strings.HasPrefix(name, "-") ||
		(strings.HasPrefix(ref, "refs/") && !strings.HasPrefix(ref, prefix)) {
		return GitRef{}, invalid(location, "expected a full commit SHA, a tag, or a branch name")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") || strings.HasSuffix(part, ".") {
			return GitRef{}, invalid(location, "expected a full commit SHA, a tag, or a branch name")
		}
	}
	if shortCommitRef.MatchString(ref) {
		return GitRef{}, invalid(location, "abbreviated commits are unsupported; use a full commit SHA, or a tag or branch name, such as release/2 or main")
	}
	return GitRef{kind: kind, canonical: prefix + name, text: ref}, nil
}

const (
	versionNumber     = `(?:0|[1-9][0-9]*)`
	versionPrerelease = `(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)`
	// The pinned npm parser bounds core components to JavaScript's safe integer limit.
	maxVersionComponent = 9007199254740991
)

var versionTag = regexp.MustCompile(`^(` + versionNumber + `)\.(` + versionNumber + `)\.(` + versionNumber + `)(?:-` + versionPrerelease + `(?:\.` + versionPrerelease + `)*)?(?:\+[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*)?$`)

// TagVersion validates a complete SemVer tag with an optional lowercase v.
// It retains prerelease and build metadata and never coerces partial versions.
// Invalid input returns an empty string and a ValidationError at the caller's location.
func TagVersion(tag, location string) (string, error) {
	version := strings.TrimPrefix(tag, "v")
	// All accepted characters are ASCII, so this byte limit equals npm's string limit.
	if len(version) > 256 {
		return "", invalid(location, "version must be at most 256 characters, excluding the optional v prefix")
	}
	parts := versionTag.FindStringSubmatch(version)
	if parts == nil {
		return "", invalid(location, "expected a complete semantic version tag, such as v1.2.3 or 1.2.3-beta.1+build.5")
	}
	for _, part := range parts[1:4] {
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil || number > maxVersionComponent {
			return "", invalid(location, "major, minor, and patch must each be at most 9007199254740991")
		}
	}
	return version, nil
}

// RefRequiredProblem explains that source has no ref, which every source needs. release is the library release
// that names the fix: the one the project records, when recorded is true, or else the library's newest. A release
// of 0 means the source isn't a library or has no library release, so only a branch, tag, or commit fits.
func RefRequiredProblem(source string, release int, recorded bool) string {
	if release == 0 {
		return fmt.Sprintf("sources.%s has no ref, and its repository has no library release to take, so the project must say which revision it takes: set sources.%s.ref in .trailguide/config.yaml to a branch, tag, or full commit SHA", source, source)
	}
	which := "the library's newest release"
	if recorded {
		which = "the release this project records"
	}
	return fmt.Sprintf("sources.%s has no ref, so .trailguide/config.yaml doesn't say which library release the project takes. Add ref: release/%d to sources.%s, %s, or ref: latest-release to take the newest release at each update, then run the command again", source, release, source, which)
}
