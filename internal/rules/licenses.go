// Read a library's manifest, strictly as its author's configuration or as its consumers read what it publishes, and
// give the canonical form of what it publishes.

package rules

import (
	"encoding/json"
	"errors"
	"slices"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/trailguide"
)

// Library is a library's manifest, which makes the repository a library, with the sources its skills curate from.
type Library struct {
	trailguide.Library
	// Curated maps the name of each source that Skills curates from to its repository and parsed ref, in place of
	// the embedded Library's; it is nil when the library curates nothing.
	Curated map[string]CuratedSource `json:"curated,omitempty"`
}

// ParseLibraryManifest validates text, a library's whole .trailguide/config.yaml, as ParseConfiguration does, for
// the library's author, and returns its library section, or nil when the configuration has none, which means the
// repository isn't a library. Unlike ParsePublishedManifest, which reads what a library publishes as its consumers
// do, it rejects a configuration whose other fields are invalid, and unknown fields in the library section. Errors
// prefix their location with location, such as team/config.yaml. It checks syntax only; callers read the declared
// files themselves.
func ParseLibraryManifest(text []byte, location string) (*Library, error) {
	if !utf8.Valid(text) {
		return nil, invalid(location, "expected UTF-8 text")
	}
	_, data, err := decode.YAML(text, location)
	if err != nil {
		return nil, err
	}
	config, err := ParseConfiguration(data)
	var validation errs.ValidationError
	if errors.As(err, &validation) {
		return nil, invalid(location+": "+validation.ValidationLocation(), validation.ValidationProblem())
	}
	var unsupported *trailguide.UnsupportedLibrarySchemaError
	if errors.As(err, &unsupported) {
		return nil, &trailguide.UnsupportedLibrarySchemaError{Location: location + ": " + unsupported.Location, SchemaVersion: unsupported.SchemaVersion, Supported: unsupported.Supported}
	}
	if err != nil {
		return nil, err
	}
	return config.Library, nil
}

// ParsePublishedManifest reads text, a library's .trailguide/config.yaml or a snapshot's copy of what it publishes,
// at location, as the library's consumers do, with trailguide.ParseLibraryManifest: only the library section and the
// repository and ref of each source it curates from, ignoring the configuration's schemaVersion and project
// settings. It returns nil for a configuration without a library section, and fails with a validation error or a
// *trailguide.UnsupportedLibrarySchemaError.
func ParsePublishedManifest(text []byte, location string) (*Library, error) {
	parsed, err := trailguide.ParseLibraryManifest(text, location)
	if err != nil || parsed == nil {
		return nil, err
	}
	result := &Library{Library: *parsed}
	result.Library.Curated = nil
	for name, source := range parsed.Curated {
		if result.Curated == nil {
			result.Curated = map[string]CuratedSource{}
		}
		curated := CuratedSource{Repository: source.Repository}
		if source.Ref != "" {
			// trailguide.ParseLibraryManifest validated the ref with the same parser.
			if curated.Ref, err = ParseGitRef(source.Ref, location+": sources."+name+".ref"); err != nil {
				return nil, err
			}
		}
		result.Curated[name] = curated
	}
	return result, nil
}

// PublishedManifest returns what a library publishes of text, its .trailguide/config.yaml at location, when it
// validates as ParsePublishedManifest reads it: the canonical manifest PublishedManifestOf gives, so the
// configuration's project settings, comments, and formatting stay in the repository. It returns nil for a
// configuration without a library section.
func PublishedManifest(text []byte, location string) ([]byte, error) {
	declaration, err := ParsePublishedManifest(text, location)
	if err != nil || declaration == nil {
		return nil, err
	}
	_, data, err := decode.YAML(text, location)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	return PublishedManifestOf(fields["library"], fields["sources"])
}

// PublishedManifestOf returns the canonical manifest of a library section and its configuration's sources, both
// given as JSON, without validating them: JSON with sorted object keys, holding the section as written and, under
// sources, the repository and ref of each source the section's skills curate from, which projects resolve for
// themselves. Two configurations that publish the same contract give the same bytes, whatever their project settings.
func PublishedManifestOf(section, sources json.RawMessage) ([]byte, error) {
	var value any
	if err := json.Unmarshal(section, &value); err != nil {
		return nil, err
	}
	var declared map[string]map[string]any
	_ = json.Unmarshal(sources, &declared)
	published := map[string]any{}
	for _, name := range curatedNames(section) {
		source := map[string]any{"repository": declared[name]["repository"]}
		if ref, ok := declared[name]["ref"]; ok {
			source["ref"] = ref
		}
		published[name] = source
	}
	encoded, err := json.Marshal(map[string]any{"sources": published, "library": value})
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// parseLibrary validates a configuration's library section strictly, as trailguide.ParseLibrarySection does, whose
// curated skills must curate from sources, the configuration's sources.
func parseLibrary(input json.RawMessage, sources []Source) (*Library, error) {
	names := []string{}
	for _, source := range sources {
		names = append(names, source.Name)
	}
	section, err := trailguide.ParseLibrarySection(input, names, "library")
	if err != nil {
		return nil, err
	}
	return &Library{Library: *section, Curated: curatedSources(section.Skills, sources)}, nil
}

// LicensePaths returns unique retained library paths in UTF-16 code-unit order without modifying the declaration.
func LicensePaths(license *trailguide.LicenseDeclaration) []string {
	paths := []string{}
	seen := make(map[string]bool)
	if license != nil {
		for _, list := range [][]string{license.Files, license.AttributionFiles} {
			for _, path := range list {
				if !seen[path] {
					paths = append(paths, path)
					seen[path] = true
				}
			}
		}
	}
	slices.SortFunc(paths, compareText)
	return paths
}

// compareText matches the pinned reference's deterministic UTF-16 ordering for Unicode paths.
func compareText(a, b string) int {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}
