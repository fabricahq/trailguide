// Check what ParseLibraryManifest validates of a configuration, and what it leaves to the project that holds it.

package trailguide_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fabricahq/trailguide/trailguide"
)

// TestParseLibraryManifest_ReadsTheLibrarySection returns the license, as library paths, and the skill levels, sorted.
func TestParseLibraryManifest_ReadsTheLibrarySection(t *testing.T) {
	config := `schemaVersion: 2
sources:
  anthropic:
    repository: https://github.com/anthropics/skills.git
library:
  license:
    spdxExpression: MIT OR Apache-2.0
    file: LICENSE.md
    notices: [.trailguide/assets/NOTICE.md, LICENSE.md]
  skills:
    required: [skills/ship, anthropic:skills/skill-creator]
    offered: [skills/build-feature]
    dependencies: [skills/shared-setup]
`
	library, err := trailguide.ParseLibraryManifest([]byte(config), "acme/rules/.trailguide/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	expression := "MIT OR Apache-2.0"
	want := &trailguide.Library{
		License: &trailguide.LicenseDeclaration{SPDXExpression: &expression, Files: []string{"LICENSE.md"}, AttributionFiles: []string{"assets/NOTICE.md"}},
		Skills:  trailguide.SkillLevels{Required: []string{"anthropic:skills/skill-creator", "skills/ship"}, Recommended: []string{}, Offered: []string{"skills/build-feature"}, Dependencies: []string{"skills/shared-setup"}},
	}
	if !reflect.DeepEqual(library, want) {
		t.Fatalf("got %+v; want %+v", library, want)
	}
	if got := library.Skills.Curated(); !reflect.DeepEqual(got, []string{"anthropic:skills/skill-creator"}) {
		t.Fatalf("curated %v", got)
	}
	if got := library.Skills.Level("skills/shared-setup"); got != "dependency" {
		t.Fatalf("level %q; want dependency", got)
	}
}

// TestParseLibraryManifest_IgnoresTheProjectsConfiguration reads only the names of the sources, and none of the
// fields that configure the repository as a project, so a catalog accepts a library whose project settings a newer
// Trail Guide added or Trail Guide would reject.
func TestParseLibraryManifest_IgnoresTheProjectsConfiguration(t *testing.T) {
	for name, config := range map[string]string{
		"project settings":       "schemaVersion: 2\nskillTargets: [claude]\nmanageAgentsFile: false\nsources: {}\nlibrary: {}\n",
		"an unknown field":       "schemaVersion: 2\nfutureSetting: true\nsources: {}\nlibrary: {}\n",
		"an invalid source":      "schemaVersion: 2\nsources:\n  team: {repository: 42, unknown: true}\nlibrary: {}\n",
		"an invalid source name": "schemaVersion: 2\nsources:\n  Team: {}\nlibrary: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			library, err := trailguide.ParseLibraryManifest([]byte(config), "config.yaml")
			if err != nil || library == nil {
				t.Fatalf("got %+v, %v; want an empty library section", library, err)
			}
		})
	}
}

// TestParseLibraryManifest_ReturnsNilWithoutALibrarySection reports a project's configuration as no library.
func TestParseLibraryManifest_ReturnsNilWithoutALibrarySection(t *testing.T) {
	library, err := trailguide.ParseLibraryManifest([]byte("schemaVersion: 2\nsources: {}\n"), "config.yaml")
	if err != nil || library != nil {
		t.Fatalf("got %+v, %v; want nil", library, err)
	}
}

// TestParseLibraryManifest_RefusesAnInvalidManifest reports a *ValidationError at the location of the problem,
// starting with the manifest's location, and returns no library.
func TestParseLibraryManifest_RefusesAnInvalidManifest(t *testing.T) {
	const at = "acme/rules/.trailguide/config.yaml"
	for name, test := range map[string]struct{ config, location string }{
		"not UTF-8":                        {"schemaVersion: 2\nsources: {}\n#\xff", at},
		"not a mapping":                    {"[]", at},
		"a duplicate key":                  {"schemaVersion: 2\nsources: {}\nlibrary: {}\nlibrary: {}\n", at + ".library"},
		"an alias":                         {"schemaVersion: 2\nsources: &s {}\nlibrary: *s\n", at + ".sources"},
		"version 1":                        {"schemaVersion: 1\nsources: {}\nlibrary: {}\n", at + ": schemaVersion"},
		"no schema version":                {"sources: {}\nlibrary: {}\n", at + ": schemaVersion"},
		"no sources":                       {"schemaVersion: 2\nlibrary: {}\n", at + ": sources"},
		"sources that aren't a map":        {"schemaVersion: 2\nsources: []\nlibrary: {}\n", at + ": sources"},
		"a library that isn't a map":       {"schemaVersion: 2\nsources: {}\nlibrary: false\n", at + ": library"},
		"an unknown library field":         {"schemaVersion: 2\nsources: {}\nlibrary: {rules: []}\n", at + ": library"},
		"a license in a rule":              {"schemaVersion: 2\nsources: {}\nlibrary:\n  license: {file: .trailguide/rules/techs/go/x.md, notices: []}\n", at + ": library.license.file"},
		"an unknown SPDX ID":               {"schemaVersion: 2\nsources: {}\nlibrary:\n  license: {spdxExpression: Nope, file: LICENSE.md, notices: []}\n", at + ": library.license.spdxExpression"},
		"a skill listed twice":             {"schemaVersion: 2\nsources: {}\nlibrary:\n  skills: {required: [skills/a], offered: [skills/a]}\n", at + ": library.skills.offered[0]"},
		"a dependency projects can choose": {"schemaVersion: 2\nsources: {}\nlibrary:\n  skills: {offered: [skills/a], dependencies: [skills/a]}\n", at + ": library.skills.dependencies[0]"},
		"an invalid skill ID":              {"schemaVersion: 2\nsources: {}\nlibrary:\n  skills: {offered: [tools/a]}\n", at + ": library.skills.offered[0]"},
		"an undeclared source":             {"schemaVersion: 2\nsources: {}\nlibrary:\n  skills: {required: [anthropic:skills/skill-creator]}\n", at + ": library.skills.required[0]"},
	} {
		t.Run(name, func(t *testing.T) {
			library, err := trailguide.ParseLibraryManifest([]byte(test.config), at)
			var validation *trailguide.ValidationError
			if !errors.As(err, &validation) || validation.Location != test.location || library != nil {
				t.Fatalf("got %+v, %v; want a validation error at %s", library, err, test.location)
			}
		})
	}
}

// TestParseLibrarySection_NamesTheSectionInErrors starts error locations with the location the caller gives.
func TestParseLibrarySection_NamesTheSectionInErrors(t *testing.T) {
	_, err := trailguide.ParseLibrarySection([]byte(`{"skills":{"required":["team:skills/a"]}}`), []string{"other"}, "library")
	var validation *trailguide.ValidationError
	if !errors.As(err, &validation) || validation.Location != "library.skills.required[0]" {
		t.Fatalf("got %v; want a validation error at library.skills.required[0]", err)
	}
	library, err := trailguide.ParseLibrarySection([]byte(`{"skills":{"required":["team:skills/a"]}}`), []string{"team"}, "library")
	if err != nil || !reflect.DeepEqual(library.Skills.Required, []string{"team:skills/a"}) {
		t.Fatalf("got %+v, %v", library, err)
	}
}
