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
// and its release record, which lists every rule's version and what changed. ParseLibraryManifest then reads the
// library's license, skill levels, and curated sources from its configuration at the tagged commit, and
// ParseRule and ParseGroupMetadata read the rule files and _group.yaml files there.
//
// A library declares the library schema version it publishes in its manifest's library.schemaVersion.
// LibrarySchemaVersion is the one this package reads, and covers the manifest, curated sources, rule files, group
// metadata, and the library's layout. Release records have their own format, ReleaseRecordFormat. Read the manifest
// before a library's other files, so a library in another schema version fails with *UnsupportedLibrarySchemaError
// rather than an error about a file whose meaning that version changed. ParseLibraryManifest reads a library as its
// consumers do: it ignores the configuration's top-level schemaVersion and project settings, which only the
// library's author's Trail Guide reads.
//
// Errors start with the input's location, such as a tag name, a field in a record, or a file path, and describe
// the problem for people. Callers can inspect four errors: ErrNotReleaseTag, with errors.Is, for a tag name that
// isn't a library release tag; *UnsupportedReleaseRecordError, with errors.As, for a release record in a newer
// format than this version reads; *UnsupportedLibrarySchemaError, with errors.As, for a library in a schema version
// this version doesn't read; and *ValidationError, with errors.As, for the location and problem of any other invalid
// input, including a missing or malformed schema version. Every error these functions return means the input is invalid
// or unsupported.
//
// This is a public API that follows Trail Guide's own version: each Trail Guide release includes it, and it may
// change before Trail Guide 1.0.0.
package trailguide
