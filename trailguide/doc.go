// Package trailguide parses the files and Git tags of a Trail Guide library exactly as Trail Guide reads them:
// library release tag names and messages, the release records in those messages, the library's manifest (the library
// section of .trailguide/config.yaml), rule files, group metadata (_group.yaml), and the canonical group list. It also
// places each library file in the library's repository: RepositoryPath and LibraryPath map between the library
// paths that rule IDs and release records use and the paths in the repository. The format is specified at
// https://trailguide.fabricahq.com/reference/library-format/ and
// https://trailguide.fabricahq.com/reference/rule-versions/.
//
// The package only parses. It has no Git, filesystem, or network access: every function takes bytes or text the
// caller has already read, such as a tag message read with go-git, and returns typed values. Trail Guide itself
// reads libraries through this package, so the two can't disagree.
//
// A catalog typically reads a library in three steps. ParseReleaseTag recognizes the release/<number> tag names
// among a repository's tags. ParseReleaseMessage splits each such annotated tag's message into its release notes
// and its release record, which lists every rule's version and what changed. ParseRule and ParseGroupMetadata then
// read the rule files and _group.yaml files at the tagged commit, and ParseLibraryManifest reads the library's
// license and skill levels from its configuration there.
//
// Errors start with the input's location, such as a tag name, a field in a record, or a file path, and describe
// the problem for people. Callers can inspect three errors: ErrNotReleaseTag, with errors.Is, for a tag name that
// isn't a library release tag; *UnsupportedReleaseRecordError, with errors.As, for a release record in a newer
// format than this version reads; and *ValidationError, with errors.As, for the location and problem of any other
// invalid input. Every error these functions return means the input is invalid.
//
// This is a public API that follows Trail Guide's own version: each Trail Guide release includes it, and it may
// change before Trail Guide 1.0.0.
package trailguide
