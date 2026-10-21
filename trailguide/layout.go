// Place a library's files in its repository, and derive the group and rule IDs of rule paths.

package trailguide

import "github.com/fabricahq/trailguide/internal/librarytree"

// Directory is the Trail Guide directory at the repository root. It holds a library's manifest, rules, shared
// assets, and change notes, beside the project files of the same repository.
const Directory = librarytree.Directory

// LegacyDirectory is the Trail Guide directory's name from before Code Rules was renamed Trail Guide. Every library
// release published before the rename keeps its files there.
const LegacyDirectory = librarytree.LegacyDirectory

// Manifest is the library path of the library's manifest: the repository's configuration, .trailguide/config.yaml,
// whose library section makes the repository a library. ParseLibraryManifest reads it.
const Manifest = librarytree.Manifest

// LegacyManifest is the manifest at the repository root of a library in the layout before version 2, which also
// kept its groups, shared assets, and change notes at the repository root. This package doesn't parse it.
const LegacyManifest = librarytree.LegacyManifest

// RepositoryPath returns where the library file at library path file lives in its repository. Library paths are
// the paths that rule IDs and release records use. The manifest is .trailguide/config.yaml; groups, at techs/ and
// practices/, live in .trailguide/rules/; shared assets, at assets/, and change notes, at changes/, live in
// .trailguide/. Any other path is a license or notice file, at the same path relative to the repository root.
func RepositoryPath(file string) string { return librarytree.RepositoryPath(file) }

// LibraryPath returns the library path of the file or directory at repository path file, the inverse of
// RepositoryPath. It reports false for a path that holds no library file: anything else in .trailguide/, such as
// vendor/, generated/, or rules/README.md, and a path outside .trailguide/ whose library path would be one of a
// file inside it, such as assets/logo.svg or config.yaml at the repository root.
func LibraryPath(file string) (string, bool) { return librarytree.LibraryPath(file) }

// GroupFromPath returns the ID of the group that holds the rule file at library path path, such as techs/go for
// techs/go/handle-errors.md. location names the path in errors. It checks syntax only, and fails for a path that
// can't be a rule file, such as one outside a group, one in an asset directory, or one that isn't Markdown.
func GroupFromPath(path, location string) (string, error) {
	group, err := librarytree.GroupFromPath(path, location)
	return group, translate(err)
}

// ValidateRuleID checks that id is a rule ID: a rule file's library path without its final .md, such as
// practices/testing/verify-retry-limits. location names the ID in errors. It checks syntax only; it doesn't
// establish that the rule exists.
func ValidateRuleID(id, location string) error {
	return translate(librarytree.ValidateRuleID(id, location))
}
