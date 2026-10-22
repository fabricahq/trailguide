// Parse Git's bounded tag listing without running Git.

package rules

import (
	"regexp"
	"strings"
)

// lowercaseCommit matches a full commit SHA as Git advertises it.
var lowercaseCommit = regexp.MustCompile(`^[a-f0-9]{40}$`)

// TagAdvertisementKind identifies why a tag listing was refused.
type TagAdvertisementKind string

const (
	InvalidTagAdvertisement TagAdvertisementKind = "git-failed"
	TagLimitExceeded        TagAdvertisementKind = "limit-exceeded"
)

// TagAdvertisementError reports a tag listing that is malformed or exceeds the listing limits.
type TagAdvertisementError struct {
	Kind    TagAdvertisementKind
	Problem string
}

// Error returns a human diagnostic; callers should branch on Kind, not message text.
func (e *TagAdvertisementError) Error() string { return e.Problem }

const (
	// maxTagAdvertisementBytes and maxAdvertisedTags are the documented limits on Git's listing of tags.
	maxTagAdvertisementBytes = 8 * 1024 * 1024
	maxAdvertisedTags        = 20_000
)

// ParseTagAdvertisement reads `git ls-remote --tags` output into a map from tag name, without
// refs/tags/, to its lowercase object ID. Annotated tags keep both the tag object under their
// name and the peeled commit under name^{}. Every nonempty line counts toward the record limit,
// including peeled and repeated ones; a repeated tag keeps its last object.
// Any malformed line or exceeded limit returns a nil map and a *TagAdvertisementError.
func ParseTagAdvertisement(text string) (map[string]string, error) {
	if len(text) > maxTagAdvertisementBytes {
		return nil, &TagAdvertisementError{Kind: TagLimitExceeded, Problem: "Git tag advertisement exceeds 8 MiB."}
	}
	records := make(map[string]string)
	count := 0
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			continue
		}
		object, ref, ok := strings.Cut(line, "\t")
		tag := strings.TrimPrefix(ref, "refs/tags/")
		if !ok || len(object) != 40 || !lowercaseCommit.MatchString(object) || object != strings.ToLower(object) || tag == ref || tag == "" || strings.ContainsAny(tag, "\t\r") {
			return nil, &TagAdvertisementError{Kind: InvalidTagAdvertisement, Problem: "Git returned an unsupported tag advertisement."}
		}
		records[tag] = object
		count++
		if count > maxAdvertisedTags {
			return nil, &TagAdvertisementError{Kind: TagLimitExceeded, Problem: "The repository exceeds the version-discovery tag limit."}
		}
	}
	return records, nil
}
