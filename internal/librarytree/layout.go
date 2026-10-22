// Place a library's files in its repository: each file's library path, which rule IDs, release records, and vendor
// snapshots use, and where the file lives in the repository's Trail Guide directory.

package librarytree

import (
	"iter"
	"slices"
	"strings"
)

// Directory is the Trail Guide directory at the repository root. It holds a library's manifest, rules, shared
// assets, and change notes, beside the project files of the same repository.
const Directory = ".trailguide"

// LegacyDirectory is the Trail Guide directory's name from before Code Rules was renamed Trail Guide. Projects that
// haven't renamed it, and every library release published before the rename, keep their files there.
const LegacyDirectory = ".code-rules"

// UsesLegacyDirectory reports whether a tree, given by its repository paths, keeps its Trail Guide directory under
// LegacyDirectory: it has a file there and none in Directory.
func UsesLegacyDirectory(paths iter.Seq[string]) bool {
	legacy := false
	for name := range paths {
		if strings.HasPrefix(name, Directory+"/") {
			return false
		}
		legacy = legacy || strings.HasPrefix(name, LegacyDirectory+"/")
	}
	return legacy
}

// Upgrade returns the repository path file of a tree that keeps its Trail Guide directory under LegacyDirectory with
// that directory spelled as Directory, so the tree reads like one made after the rename.
func Upgrade(file string) string {
	if inside, found := strings.CutPrefix(file, LegacyDirectory+"/"); found {
		return Directory + "/" + inside
	}
	return file
}

// Manifest is the library path of the library's manifest: the repository's configuration, .trailguide/config.yaml,
// whose library section makes the repository a library.
const Manifest = "config.yaml"

// LegacyManifest is the manifest at the repository root of a library in the layout before version 2, which also
// kept its groups, shared assets, and change notes at the repository root.
const LegacyManifest = "rule-library.yaml"

// Managed is the folder of the Trail Guide directory that holds everything Trail Guide writes there besides the
// lockfile and the project guide: Vendor and Generated. The underscore marks it as Trail Guide's; no authored
// folder's name starts with one. A library never publishes it.
const Managed = "_managed"

// Vendor is the folder, relative to the Trail Guide directory, of a project's vendor snapshots, one folder per
// source; Generated is the folder of its generated guidance, which agents read.
const (
	Vendor    = Managed + "/vendor"
	Generated = Managed + "/generated"
)

// LegacyVendor and LegacyGenerated are where Trail Guide 0.7 and earlier kept Vendor and Generated, at the top of
// the Trail Guide directory. Sync moves them.
const (
	LegacyVendor    = "vendor"
	LegacyGenerated = "generated"
)

// RepositoryPath returns where the library file at library path file lives in its repository. The manifest is
// .trailguide/config.yaml; groups, at techs/ and practices/, live in .trailguide/rules/; shared assets, at assets/,
// and change notes, at changes/, live in .trailguide/. Any other path is a license or notice file, at the same path
// relative to the repository root.
func RepositoryPath(file string) string {
	if file == Manifest {
		return Directory + "/" + Manifest
	}
	switch first, _, _ := strings.Cut(file, "/"); first {
	case "techs", "practices":
		return Directory + "/rules/" + file
	case "assets", "changes":
		return Directory + "/" + file
	}
	return file
}

// SnapshotPath returns where the library file at library path file is stored in a project's snapshot of the
// library, _managed/vendor/<source>/: its path relative to the library's Trail Guide directory, as RepositoryPath places it,
// so a rule at rules/techs/go/x.md keeps its relative links to shared assets at assets/. A license or notice file
// outside the Trail Guide directory keeps its path relative to the repository root.
func SnapshotPath(file string) string {
	return strings.TrimPrefix(RepositoryPath(file), Directory+"/")
}

// snapshotNames are the top-level names in a project's snapshot of a library that SnapshotPath gives the files of
// its Trail Guide directory, or that the project keeps for its records of the source: _source.json, and curated/
// for the skills the library curates.
var snapshotNames = []string{"_source.json", "assets", "curated", "changes", Manifest, "rules", "skills"}

// InSnapshotNames reports whether file, the library path of a file outside the Trail Guide directory, such as a
// license file, starts with one of the names a snapshot keeps for the library's Trail Guide directory or the project's
// records of the source, in any letter case. Stored at its repository path, such a file could take the place of one
// of theirs, as rules/techs/go/errors.md at the repository root would take rule techs/go/errors's.
func InSnapshotNames(file string) bool {
	first, _, _ := strings.Cut(file, "/")
	return slices.ContainsFunc(snapshotNames, func(name string) bool { return strings.EqualFold(first, name) })
}

// LibraryPath returns the library path of the file or directory at repository path file, the inverse of
// RepositoryPath. It reports false for a path that holds no library file: anything else in .trailguide/, such as
// _managed/, or rules/README.md, and a path outside .trailguide/ whose library path would be one of a
// file inside it, such as assets/logo.svg or config.yaml at the repository root.
func LibraryPath(file string) (string, bool) {
	inside, found := strings.CutPrefix(file, Directory+"/")
	if !found {
		if file == Directory || RepositoryPath(file) != file {
			return "", false
		}
		return file, true
	}
	if rule, found := strings.CutPrefix(inside, "rules/"); found {
		if first, _, _ := strings.Cut(rule, "/"); first == "techs" || first == "practices" {
			return rule, true
		}
		return "", false
	}
	if first, _, _ := strings.Cut(inside, "/"); first == "assets" || first == "changes" || inside == Manifest {
		return inside, true
	}
	return "", false
}
