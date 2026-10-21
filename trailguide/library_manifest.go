// Parse a library's manifest, the library section of its configuration, and the license it declares.

package trailguide

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/librarytree"
)

// Library is a library's manifest: the library section of its configuration, which makes the repository a library.
type Library struct {
	// License is nil when the library declares no license.
	License *LicenseDeclaration `json:"license"`
	// Skills holds the levels the library gives its skills, its own and those it curates.
	Skills SkillLevels `json:"skills"`
}

// LicenseDeclaration records authored terms and paths, without interpreting legal meaning.
// A nil SPDXExpression means the library declared a file but no SPDX identifier. Files and AttributionFiles hold
// library paths, which are the declared paths except inside .trailguide/: a declared
// .trailguide/assets/NOTICE.md is assets/NOTICE.md.
type LicenseDeclaration struct {
	SPDXExpression   *string  `json:"spdxExpression"`
	Files            []string `json:"files"`
	AttributionFiles []string `json:"attributionFiles"`
}

// ParseLibraryManifest parses text, a library's whole .trailguide/config.yaml at the location that names it in
// errors, such as acme/rules/.trailguide/config.yaml, and returns its library section, or nil when the
// configuration has none, which means the repository isn't a library.
//
// It validates only what a library publishes: the text must be one YAML mapping, without anchors, aliases, explicit
// tags, or duplicate keys; schemaVersion must be 2; sources must be a mapping, of which it reads only the names; and
// the library section must be valid, as ParseLibrarySection requires. It doesn't read the configuration's other
// fields, which configure the repository as a project. So every configuration Trail Guide accepts parses here to the
// same library section, but a configuration this accepts can still be one Trail Guide rejects, such as one with an
// invalid source. It checks syntax only; callers read the declared license and notice files themselves.
func ParseLibraryManifest(text []byte, location string) (*Library, error) {
	if !utf8.Valid(text) {
		return nil, invalid(location, "expected UTF-8 text")
	}
	_, data, err := decode.YAML(text, location)
	if err != nil {
		return nil, translate(err)
	}
	fields, err := decode.Object(data, location)
	if err != nil {
		return nil, translate(err)
	}
	var schema float64
	if json.Unmarshal(fields["schemaVersion"], &schema) != nil || schema != 2 {
		return nil, invalid(location+": schemaVersion", "only version 2 is supported")
	}
	sources, err := decode.Object(fields["sources"], location+": sources")
	if err != nil {
		return nil, translate(err)
	}
	section, ok := fields["library"]
	if !ok {
		return nil, nil
	}
	return ParseLibrarySection(section, slices.Sorted(maps.Keys(sources)), location+": library")
}

// ParseLibrarySection parses section, a configuration's library section as JSON, for a caller that decodes the
// configuration and reads its other fields itself. location names the section in errors, such as
// acme/rules/.trailguide/config.yaml: library, and sources are the names of the configuration's sources, which
// curated skills must come from. The section may hold:
//
//   - license: the SPDX expression of the library's terms, its license file, and its notice files, at paths
//     relative to the repository root that must hold library-wide files, outside rules and their asset
//     directories; and
//   - skills: the library's skills at each level, required, recommended, and offered, each listed once, as the IDs
//     of its own skills, skills/<name>, or of curated skills, <source>:<skill ID>, such as
//     anthropic:skills/skill-creator.
//
// Unknown fields are rejected. It doesn't check that declared files or skills exist.
func ParseLibrarySection(section json.RawMessage, sources []string, location string) (*Library, error) {
	library, err := parseLibrary(section, sources, location)
	return library, translate(err)
}

// parseLibrary validates a library section at location, whose curated skills must curate from sources.
func parseLibrary(input json.RawMessage, sources []string, location string) (*Library, error) {
	fields, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if err := decode.KnownFields(fields, []string{"license", "skills"}, location); err != nil {
		return nil, err
	}
	result := &Library{}
	if raw, ok := fields["license"]; ok {
		if result.License, err = parseLicense(raw, location+".license"); err != nil {
			return nil, err
		}
	}
	if result.Skills, err = parseSkillLevels(fields["skills"], location+".skills", sources); err != nil {
		return nil, err
	}
	return result, nil
}

// parseLicense validates a license declaration and returns the library paths of its declared files, without
// checking file presence. Declared paths are relative to the repository root and must be library-wide files, never
// a rule's Markdown file or a file in an asset directory inside a group. Unknown fields are rejected.
func parseLicense(input json.RawMessage, location string) (*LicenseDeclaration, error) {
	fields, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if err := decode.KnownFields(fields, []string{"file", "notices", "spdxExpression"}, location); err != nil {
		return nil, err
	}
	file, err := termsPath(fields["file"], location+".file")
	if err != nil {
		return nil, err
	}
	var rawNotices []json.RawMessage
	if json.Unmarshal(fields["notices"], &rawNotices) != nil || rawNotices == nil {
		return nil, invalid(location+".notices", "expected an array of notice paths")
	}
	notices := make([]string, len(rawNotices))
	for i, raw := range rawNotices {
		path, err := termsPath(raw, fmt.Sprintf("%s.notices[%d]", location, i))
		if err != nil {
			return nil, err
		}
		notices[i] = path
	}
	var expression *string
	if raw, ok := fields["spdxExpression"]; ok {
		text, err := decode.Text(raw, location+".spdxExpression")
		if err != nil {
			return nil, err
		}
		if strings.ContainsFunc(text, controlCharacter) {
			return nil, invalid(location+".spdxExpression", "expected a single-line license expression")
		}
		if !validSPDXExpression(text) {
			return nil, invalid(location+".spdxExpression", "expected an SPDX expression using recognized identifiers or LicenseRef- custom terms")
		}
		expression = &text
	}
	result := LicenseDeclaration{SPDXExpression: expression, Files: []string{file}, AttributionFiles: []string{}}
	seen := map[string]bool{file: true}
	for _, path := range notices {
		if !seen[path] {
			result.AttributionFiles = append(result.AttributionFiles, path)
			seen[path] = true
		}
	}
	return &result, nil
}

// termsPath reads a declared license or notice path, relative to the repository root, and returns its library path.
// The file must be library-wide: outside .trailguide/, or in its assets/ or a group in its rules/. A rule's Markdown
// file or a file in an asset directory inside a group belongs to that rule's version, so declaring it would let a
// library release replace a rule version's content. A path in .code-rules/, which a library declared before Code
// Rules was renamed Trail Guide, reads as the same path in .trailguide/, where tree upgrades and git mv put its file. A file
// outside .trailguide/ keeps its repository path in a project's snapshot of the library, so it can't start with a name
// the snapshot keeps for the library's other files.
func termsPath(input json.RawMessage, location string) (string, error) {
	declared, err := decode.Path(input, location)
	if err != nil {
		return "", err
	}
	path, ok := librarytree.LibraryPath(librarytree.Upgrade(declared))
	if !ok || path == librarytree.Manifest || strings.HasPrefix(path, "changes/") || (path == declared && librarytree.InSnapshotNames(path)) {
		return "", invalid(location, decode.Quote(declared)+" can't hold a license or notice file; keep it outside .trailguide/, such as LICENSE.md at the repository root, or in .trailguide/assets/. At the repository root, _source.json, config.yaml, and the assets/, curated/, changes/, practices/, rules/, skills/, and techs/ folders are reserved for the library's own files and a project's copy of the library")
	}
	if librarytree.IsRuleContent(path) {
		return "", invalid(location, decode.Quote(declared)+" belongs to a rule's version; keep license and notice files outside rules and their asset directories, such as LICENSE.md at the repository root")
	}
	return path, nil
}

// controlCharacter identifies bytes that cannot occur in a single-line SPDX expression.
func controlCharacter(r rune) bool { return r < 32 || r == 127 }
