// Parse and compare the Trail Guide versions that a configuration's minTrailguideVersion requires.

package rules

import (
	"encoding/json"
	"fmt"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/trailguide"
)

// MinimumVersionField is the configuration field that names the oldest Trail Guide release that may work on the
// project.
const MinimumVersionField = "minTrailguideVersion"

// ToolVersion is a Trail Guide release version, such as 0.9.0: plain major.minor.patch, as release tags spell it
// without their v. Versions are comparable with ==; use Compare to order them.
type ToolVersion struct {
	version trailguide.RuleVersion
}

// ParseToolVersion accepts only canonical major.minor.patch text, such as 0.9.0. location names the text in errors.
func ParseToolVersion(text, location string) (ToolVersion, error) {
	version, err := trailguide.ParseRuleVersion(text, location)
	if err != nil {
		return ToolVersion{}, invalid(location, "expected a Trail Guide release version, such as 0.9.0, got "+decode.Quote(text))
	}
	return ToolVersion{version}, nil
}

// ReleaseVersion parses running, the version of this build, reporting false for a development or prerelease build,
// such as 0.0.0-development, which neither checks nor records minTrailguideVersion.
func ReleaseVersion(running string) (ToolVersion, bool) {
	version, err := ParseToolVersion(running, "version")
	return version, err == nil
}

// String returns the canonical major.minor.patch text.
func (v ToolVersion) String() string { return v.version.String() }

// MarshalText writes the canonical text, so JSON shows a configuration's minTrailguideVersion as written.
func (v ToolVersion) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// Compare returns -1, 0, or 1 as v is older than, equal to, or newer than other.
func (v ToolVersion) Compare(other ToolVersion) int { return v.version.Compare(other.version) }

// parseMinimumVersion validates minTrailguideVersion; a missing field is nil.
func parseMinimumVersion(input json.RawMessage) (*ToolVersion, error) {
	if input == nil {
		return nil, nil
	}
	var text string
	if json.Unmarshal(input, &text) != nil {
		return nil, invalid(MinimumVersionField, "expected a Trail Guide release version, such as 0.9.0")
	}
	version, err := ParseToolVersion(text, MinimumVersionField)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// MinimumVersion returns the minTrailguideVersion of the configuration in input, interpreting no other value, so a
// configuration that a newer Trail Guide wrote, with fields or values this version doesn't know, still says which
// version it needs. It reports false when the configuration has none, or when input or the field can't be read, which
// ParseConfigurationYAML then reports.
func MinimumVersion(input []byte) (ToolVersion, bool) {
	document, err := decode.Document(input, "configuration")
	if err != nil {
		return ToolVersion{}, false
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return ToolVersion{}, false
	}
	node := mappingValue(root, MinimumVersionField)
	if node == nil {
		return ToolVersion{}, false
	}
	data, err := decode.JSON(node, MinimumVersionField)
	if err != nil {
		return ToolVersion{}, false
	}
	version, err := parseMinimumVersion(data)
	if err != nil || version == nil {
		return ToolVersion{}, false
	}
	return *version, true
}

// UpgradeRequired decides whether running, this build's version, may work on the configuration in config. It
// reports a problem to show the user, and true, when the configuration's minTrailguideVersion is newer than a release
// running. A development or prerelease build, a missing configuration, and one without the field may work on it.
func UpgradeRequired(config []byte, running string) (string, bool) {
	release, ok := ReleaseVersion(running)
	if !ok {
		return "", false
	}
	minimum, ok := MinimumVersion(config)
	if !ok || minimum.Compare(release) <= 0 {
		return "", false
	}
	return fmt.Sprintf(".trailguide/config.yaml: %s is %s, but this is trailguide %s; upgrade trailguide to %s or newer, then run the command again", MinimumVersionField, minimum, release, minimum), true
}
