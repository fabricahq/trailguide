// Validate a configuration that holds a library section, the manifest, and publish what the library publishes of it.

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

// Library is a configuration's library section, the manifest, which makes the repository a library, with the sources
// its skills curate from.
type Library struct {
	trailguide.Library
	// Curated maps the name of each source that Skills curates from to its repository and ref, as the
	// configuration's sources declare them; it is nil when the library curates nothing.
	Curated map[string]CuratedSource `json:"curated,omitempty"`
}

// ParseLibraryManifest validates text, a library's whole .trailguide/config.yaml, as ParseConfiguration does, and
// returns its library section, or nil when the configuration has none, which means the repository isn't a library.
// Unlike trailguide.ParseLibraryManifest, which reads only what a library publishes, it rejects a configuration
// whose other fields are invalid, and returns the sources the library curates from. Errors prefix their location
// with location, such as team/config.yaml. It checks syntax only; callers read the declared files themselves.
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
	if err != nil {
		return nil, err
	}
	return config.Library, nil
}

// PublishedManifest returns what a library publishes of text, its whole .trailguide/config.yaml, when it validates
// as ParseLibraryManifest validates it: a configuration that holds only its library section, as canonical JSON, so
// the sources and other fields that configure the repository as a project, and its comments and formatting, stay in
// the repository. It returns nil for a configuration without a library section.
func PublishedManifest(text []byte, location string) ([]byte, error) {
	declaration, err := ParseLibraryManifest(text, location)
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

// PublishedManifestOf returns the configuration that PublishedManifest publishes for a library section and the
// configuration's sources, both given as JSON: schema version 2, the repository and ref of each source the
// section's skills curate from, which projects resolve for themselves, and the section, with object keys sorted.
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
	encoded, err := json.Marshal(map[string]any{"schemaVersion": 2, "sources": published, "library": value})
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// parseLibrary validates a configuration's library section, as trailguide.ParseLibrarySection does, whose curated
// skills must curate from sources, the configuration's sources.
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
