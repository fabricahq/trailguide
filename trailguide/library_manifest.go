// Parse a library's manifest, the library section of its configuration and the sources it curates from, as its
// consumers and its author read it.

package trailguide

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/gitsource"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/skills"
)

// LibraryFormat is the library format this version of Trail Guide reads and writes, which every library declares in
// its manifest's library.formatVersion. The format covers everything a library publishes: the manifest, the
// repository and ref of each source it curates skills from, rule files, group metadata, and the library's layout.
// Release records have their own format, ReleaseRecordFormat.
//
// Consumers ignore fields they don't know in the manifest, so a later Trail Guide can add a field without a new
// format when ignoring it keeps every reader correct. A change that readers must interpret, even an optional field,
// needs a new format.
const LibraryFormat = 1

// Library is a library's manifest: the library section of its configuration, which makes the repository a library,
// and the sources its skills curate from.
type Library struct {
	// License is nil when the library declares no license.
	License *LicenseDeclaration `json:"license"`
	// Skills holds the levels the library gives its skills, its own and those it curates.
	Skills SkillLevels `json:"skills"`
	// Curated maps the name of each source that Skills curates from to the repository and ref the library declares
	// for it. It is nil when the library curates nothing, and ParseLibrarySection, which reads no sources, leaves it
	// nil.
	Curated map[string]CuratedSource `json:"curated,omitempty"`
}

// CuratedSource is a source a library curates skills from, as its configuration's sources declare it: the
// repository as written, and the ref, which is empty when the library follows the repository's library releases.
// Each project resolves the ref for itself.
type CuratedSource struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref,omitempty"`
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

// UnsupportedLibraryFormatError reports a library whose library.formatVersion is a format this version of Trail
// Guide doesn't read. Check for it with errors.As. A missing or malformed formatVersion is a *ValidationError
// instead.
type UnsupportedLibraryFormatError struct {
	// Location is the manifest's formatVersion field, such as acme/rules/.trailguide/config.yaml: library.formatVersion.
	Location string
	// FormatVersion is the format the library declares.
	FormatVersion int
	// Supported is the format this version reads, LibraryFormat.
	Supported int
}

// Error names both formats and says what to upgrade.
func (e *UnsupportedLibraryFormatError) Error() string {
	advice := "upgrade Trail Guide, or the tool that reads the library, to a version that reads format " + strconv.Itoa(e.FormatVersion)
	if e.FormatVersion < e.Supported {
		advice = "the library must move to format " + strconv.Itoa(e.Supported) + " and publish a new library release"
	}
	return e.Location + ": the library uses library format " + strconv.Itoa(e.FormatVersion) + ", but this version reads only format " + strconv.Itoa(e.Supported) + "; " + advice
}

// ParseLibraryManifest parses text, a library's whole .trailguide/config.yaml at the location that names it in
// errors, such as acme/rules/.trailguide/config.yaml, as a library's consumers read it, and returns what the library
// publishes, or nil when the configuration has no library section, which means the repository isn't a library.
//
// It reads only the library's published contract. The text must be one YAML mapping, without anchors, aliases,
// explicit tags, or duplicate keys. The library section must declare formatVersion first: a missing or malformed
// one is a *ValidationError, and a format other than LibraryFormat is an *UnsupportedLibraryFormatError. The section
// must then be valid, as ParseLibrarySection requires, except that fields it doesn't know are ignored. For each
// source its skills curate from, sources must declare a valid repository and, optionally, a valid ref; two curated
// sources can't name one repository. It ignores schemaVersion, the other sources, the curated sources' other fields,
// and every other field, which configure the repository as a project, so a library is readable whatever its project
// settings. It checks syntax only; callers read the declared license and notice files themselves.
func ParseLibraryManifest(text []byte, location string) (*Library, error) {
	library, err := parseManifest(text, location)
	return library, translate(err)
}

// parseManifest is ParseLibraryManifest with the errors of internal packages untranslated.
func parseManifest(text []byte, location string) (*Library, error) {
	if !utf8.Valid(text) {
		return nil, invalid(location, "expected UTF-8 text")
	}
	_, data, err := decode.YAML(text, location)
	if err != nil {
		return nil, err
	}
	fields, err := decode.Object(data, location)
	if err != nil {
		return nil, err
	}
	section, ok := fields["library"]
	if !ok {
		return nil, nil
	}
	sections, err := decode.Object(section, location+": library")
	if err != nil {
		return nil, err
	}
	if err := libraryFormat(sections, location+": library", false); err != nil {
		return nil, err
	}
	sources, err := curatedSourceFields(fields["sources"], section, location+": sources")
	if err != nil {
		return nil, err
	}
	library, err := parseLibrary(section, slices.Sorted(maps.Keys(sources)), location+": library", false)
	if err != nil {
		return nil, err
	}
	if library.Curated, err = parseCuratedSources(sources, library.Skills.Curated(), location+": sources"); err != nil {
		return nil, err
	}
	return library, nil
}

// ParseLibrarySection parses section, a configuration's library section as JSON, as its author's tools validate it,
// for a caller that decodes the configuration and reads its other fields itself. location names the section in
// errors, such as acme/rules/.trailguide/config.yaml: library, and sources are the names of the configuration's
// sources, which curated skills must come from. The section holds:
//
//   - formatVersion: the library format, which must be LibraryFormat; a missing or malformed one is a
//     *ValidationError, and another format an *UnsupportedLibraryFormatError;
//   - license: the SPDX expression of the library's terms, its license file, and its notice files, at paths
//     relative to the repository root that must hold library-wide files, outside rules and their asset
//     directories; and
//   - skills: the library's skills at each level, required, recommended, and offered, each listed once, as the IDs
//     of its own skills, skills/<name>, or of curated skills, <source>:<skill ID>, such as
//     anthropic:skills/skill-creator.
//
// Unknown fields are rejected, unlike in ParseLibraryManifest. It doesn't check that declared files or skills exist,
// and leaves Curated nil.
func ParseLibrarySection(section json.RawMessage, sources []string, location string) (*Library, error) {
	fields, err := decode.Object(section, location)
	if err == nil {
		err = libraryFormat(fields, location, true)
	}
	if err != nil {
		return nil, translate(err)
	}
	library, err := parseLibrary(section, sources, location, true)
	return library, translate(err)
}

// libraryFormat checks the formatVersion of a library section's fields at location: a whole number that is
// LibraryFormat. A missing one is explained to the library's author, or to its consumers.
func libraryFormat(fields map[string]json.RawMessage, location string, author bool) error {
	location += ".formatVersion"
	raw, ok := fields["formatVersion"]
	switch {
	case !ok && author:
		return invalid(location, "missing; add formatVersion: "+strconv.Itoa(LibraryFormat)+", the library format this library publishes")
	case !ok:
		return invalid(location, "missing; the library doesn't declare the library format it publishes, so this version can't read it. Its maintainer must add formatVersion: "+strconv.Itoa(LibraryFormat)+" and publish a library release")
	}
	format, err := strconv.Atoi(string(raw))
	if err != nil || format < 1 {
		return invalid(location, "expected the library format, a whole number such as "+strconv.Itoa(LibraryFormat))
	}
	if format != LibraryFormat {
		return &UnsupportedLibraryFormatError{Location: location, FormatVersion: format, Supported: LibraryFormat}
	}
	return nil
}

// curatedSourceFields returns, by name, the entries of sources, a configuration's sources as JSON at location, that
// the raw library section's skills curate from, without validating them. Sources may be missing when the library
// curates nothing. A curated skill whose source sources doesn't declare is left for the section's parser to report.
func curatedSourceFields(sources, section json.RawMessage, location string) (map[string]json.RawMessage, error) {
	names := curatedNames(section)
	if len(names) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	declared, err := decode.Object(sources, location)
	if err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	for _, name := range names {
		if entry, ok := declared[name]; ok {
			result[name] = entry
		}
	}
	return result, nil
}

// curatedNames returns the names of the sources that the raw library section's skill lists name, without validating
// the section. It reads only the lists this format defines, since consumers ignore any other field.
func curatedNames(section json.RawMessage) []string {
	var library struct {
		Skills map[string]json.RawMessage `json:"skills"`
	}
	if json.Unmarshal(section, &library) != nil {
		return nil
	}
	names := map[string]bool{}
	for _, list := range skills.Lists {
		var ids []string
		if json.Unmarshal(library.Skills[list], &ids) != nil {
			continue
		}
		for _, id := range ids {
			if name, _, curated := strings.Cut(id, ":"); curated {
				names[name] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(names))
}

// parseCuratedSources validates the entries of sources at location that curated, the curated skill IDs of a valid
// library section, come from: each an object with a valid repository and an optional valid ref, its other fields
// ignored, and no two with one repository. It returns nil when curated is empty.
func parseCuratedSources(sources map[string]json.RawMessage, curated []string, location string) (map[string]CuratedSource, error) {
	if len(curated) == 0 {
		return nil, nil
	}
	result := map[string]CuratedSource{}
	repositories := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		where := location + "." + name
		fields, err := decode.Object(sources[name], where)
		if err != nil {
			return nil, err
		}
		repository, err := decode.Text(fields["repository"], where+".repository")
		if err != nil {
			return nil, err
		}
		address, err := gitsource.ParseRepository(fields["repository"], where+".repository")
		if err != nil {
			return nil, err
		}
		if other, declared := repositories[address.Identity]; declared {
			return nil, invalid(where, "repository "+repository+" is declared more than once: sources."+other+" curates from it too; curate each repository through one source")
		}
		repositories[address.Identity] = name
		source := CuratedSource{Repository: repository}
		if raw, ok := fields["ref"]; ok {
			ref, err := gitsource.ParseRefField(raw, where+".ref")
			if err != nil {
				return nil, err
			}
			source.Ref = ref.String()
		}
		result[name] = source
	}
	return result, nil
}

// parseLibrary validates a library section at location, whose curated skills must curate from sources, after its
// format: with strict, as its author's tools do, fields it doesn't know fail, and otherwise, as its consumers do,
// they're ignored.
func parseLibrary(input json.RawMessage, sources []string, location string, strict bool) (*Library, error) {
	fields, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if strict {
		if err := decode.KnownFields(fields, []string{"formatVersion", "license", "skills"}, location); err != nil {
			return nil, err
		}
	}
	result := &Library{}
	if raw, ok := fields["license"]; ok {
		if result.License, err = parseLicense(raw, location+".license", strict); err != nil {
			return nil, err
		}
	}
	if result.Skills, err = parseSkillLevels(fields["skills"], location+".skills", sources, strict); err != nil {
		return nil, err
	}
	return result, nil
}

// parseLicense validates a license declaration and returns the library paths of its declared files, without
// checking file presence. Declared paths are relative to the repository root and must be library-wide files, never
// a rule's Markdown file or a file in an asset directory inside a group. With strict, unknown fields are rejected;
// otherwise they're ignored.
func parseLicense(input json.RawMessage, location string, strict bool) (*LicenseDeclaration, error) {
	fields, err := decode.Object(input, location)
	if err != nil {
		return nil, err
	}
	if strict {
		if err := decode.KnownFields(fields, []string{"file", "notices", "spdxExpression"}, location); err != nil {
			return nil, err
		}
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
