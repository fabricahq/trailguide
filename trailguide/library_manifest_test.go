// Check what ParseLibraryManifest validates of a library's published contract as its consumers read it, what it
// leaves to the project that holds it, and what ParseLibrarySection rejects for the library's author.

package trailguide_test

import (
	"errors"
	"reflect"
	"strings"
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
  schemaVersion: 2
  name: library
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
		Name:    "library",
		License: &trailguide.LicenseDeclaration{SPDXExpression: &expression, Files: []string{"LICENSE.md"}, AttributionFiles: []string{"assets/NOTICE.md"}},
		Skills:  trailguide.SkillLevels{Required: []string{"anthropic:skills/skill-creator", "skills/ship"}, Recommended: []string{}, Offered: []string{"skills/build-feature"}, Dependencies: []string{"skills/shared-setup"}},
		Curated: map[string]trailguide.CuratedSource{"anthropic": {Repository: "https://github.com/anthropics/skills.git"}},
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

// TestParseLibraryManifest_IgnoresTheProjectsConfiguration reads none of the fields that configure the repository as
// a project, including schemaVersion and the sources the library doesn't curate from, so a consumer reads a library
// whatever project settings or project schema its author's Trail Guide uses.
func TestParseLibraryManifest_IgnoresTheProjectsConfiguration(t *testing.T) {
	const library = "library: {schemaVersion: 2, name: library}\n"
	for name, config := range map[string]string{
		"project settings":           "schemaVersion: 2\nskillTargets: [claude]\nmanageAgentsFile: false\nsources: {}\n" + library,
		"an unknown field":           "schemaVersion: 2\nfutureSetting: true\nsources: {}\n" + library,
		"an invalid source":          "schemaVersion: 2\nsources:\n  team: {repository: 42, unknown: true}\n" + library,
		"an invalid source name":     "schemaVersion: 2\nsources:\n  Team: {}\n" + library,
		"sources that aren't a map":  "schemaVersion: 2\nsources: []\n" + library,
		"no sources":                 "schemaVersion: 2\n" + library,
		"another schema version":     "schemaVersion: 3\nsources: {}\n" + library,
		"schema version 1":           "schemaVersion: 1\nsources: {}\n" + library,
		"no schema version":          "sources: {}\n" + library,
		"a malformed schema version": "schemaVersion: two\n" + library,
	} {
		t.Run(name, func(t *testing.T) {
			library, err := trailguide.ParseLibraryManifest([]byte(config), "config.yaml")
			if err != nil || library == nil {
				t.Fatalf("got %+v, %v; want an empty library section", library, err)
			}
		})
	}
}

// TestParseLibraryManifest_ReadsOnlyTheRepositoryAndRefOfCuratedSources returns each curated source's repository and
// ref, ignoring the project settings the library keeps beside them, such as the skills it installs for itself.
func TestParseLibraryManifest_ReadsOnlyTheRepositoryAndRefOfCuratedSources(t *testing.T) {
	config := `schemaVersion: 2
sources:
  anthropic:
    repository: https://github.com/anthropics/skills.git
    ref: main
    skills:
      include: [skills/skill-creator]
    futureSetting: true
  mattpocock:
    repository: git@github.com:mattpocock/skills.git
library:
  schemaVersion: 2
  name: library
  skills:
    recommended: [anthropic:skills/skill-creator, mattpocock:skills/to-spec]
`
	library, err := trailguide.ParseLibraryManifest([]byte(config), "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]trailguide.CuratedSource{
		"anthropic":  {Repository: "https://github.com/anthropics/skills.git", Ref: "main"},
		"mattpocock": {Repository: "git@github.com:mattpocock/skills.git"},
	}
	if !reflect.DeepEqual(library.Curated, want) {
		t.Fatalf("got %+v; want %+v", library.Curated, want)
	}
}

// TestParseLibraryManifest_IgnoresUnknownFieldsInTheLibrarySection accepts fields a later Trail Guide may add to the
// manifest without a new schema version, at every level of the library section, and still validates the fields it knows.
func TestParseLibraryManifest_IgnoresUnknownFieldsInTheLibrarySection(t *testing.T) {
	config := `schemaVersion: 2
sources: {}
library:
  schemaVersion: 2
  name: library
  description: Shared rules.
  license: {spdxExpression: MIT, file: LICENSE.md, notices: [], url: https://example.com/license}
  skills: {required: [skills/ship], featured: [skills/ship]}
`
	library, err := trailguide.ParseLibraryManifest([]byte(config), "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(library.Skills.Required, []string{"skills/ship"}) || library.License == nil || *library.License.SPDXExpression != "MIT" {
		t.Fatalf("got %+v; want the known fields", library)
	}
	// An unknown list doesn't make the sources it names published, so their project-only fields stay unvalidated.
	unknownList := "schemaVersion: 2\nsources:\n  helper: {repository: not an address}\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills: {featured: [helper:skills/help, {any: shape}]}\n"
	if library, err := trailguide.ParseLibraryManifest([]byte(unknownList), "config.yaml"); err != nil || library.Curated != nil {
		t.Fatalf("got %+v, %v; want no curated sources", library, err)
	}
	section := []byte(`{"schemaVersion":2,"name":"library","description":"Shared rules.","skills":{"required":["skills/ship"],"featured":["skills/ship"]}}`)
	for _, test := range []struct{ section, location string }{
		{`{"schemaVersion":2,"name":"library","description":"Shared rules."}`, "library"},
		{`{"schemaVersion":2,"name":"library","license":{"file":"LICENSE.md","notices":[],"url":"https://example.com"}}`, "library.license"},
		{`{"schemaVersion":2,"name":"library","skills":{"featured":["skills/ship"]}}`, "library.skills"},
	} {
		_, err := trailguide.ParseLibrarySection([]byte(test.section), nil, "library")
		var validation *trailguide.ValidationError
		if !errors.As(err, &validation) || validation.Location != test.location {
			t.Errorf("ParseLibrarySection(%s) got %v; want an unknown field at %s", test.section, err, test.location)
		}
	}
	if _, err := trailguide.ParseLibrarySection(section, nil, "library"); err == nil {
		t.Error("ParseLibrarySection accepted unknown fields")
	}
}

// TestParseLibraryManifest_ReturnsNilWithoutALibrarySection reports a project's configuration as no library.
func TestParseLibraryManifest_ReturnsNilWithoutALibrarySection(t *testing.T) {
	library, err := trailguide.ParseLibraryManifest([]byte("schemaVersion: 2\nsources: {}\n"), "config.yaml")
	if err != nil || library != nil {
		t.Fatalf("got %+v, %v; want nil", library, err)
	}
}

// TestParseLibraryManifest_ReportsAnUnsupportedSchema fails with *UnsupportedLibrarySchemaError, naming both schema
// versions and the field, for a library in a schema version this version doesn't read, before reading the rest of the
// section, which a later schema version may give another meaning.
func TestParseLibraryManifest_ReportsAnUnsupportedSchema(t *testing.T) {
	const at = "acme/rules/.trailguide/config.yaml"
	library, err := trailguide.ParseLibraryManifest([]byte("sources: {}\nlibrary:\n  schemaVersion: 3\n  skills: {required: skills/ship}\n"), at)
	var unsupported *trailguide.UnsupportedLibrarySchemaError
	if !errors.As(err, &unsupported) || library != nil {
		t.Fatalf("got %+v, %v; want an unsupported schema version", library, err)
	}
	want := trailguide.UnsupportedLibrarySchemaError{Location: at + ": library.schemaVersion", SchemaVersion: 3, Supported: trailguide.LibrarySchemaVersion}
	if *unsupported != want {
		t.Fatalf("got %+v; want %+v", *unsupported, want)
	}
	if text := err.Error(); !strings.Contains(text, "library schema version 3") || !strings.Contains(text, "reads only library schema version 2") || !strings.Contains(text, "upgrade Trail Guide") {
		t.Fatalf("got %q; want both schema versions and what to upgrade", text)
	}
	var validation *trailguide.ValidationError
	if errors.As(err, &validation) {
		t.Fatalf("an unsupported schema version is also a validation error: %v", err)
	}
	// A library of schema version 1 keeps its skills directly in skills/, without origin folders, so it must move.
	_, err = trailguide.ParseLibraryManifest([]byte("sources: {}\nlibrary: {schemaVersion: 1}\n"), at)
	if !errors.As(err, &unsupported) || unsupported.SchemaVersion != 1 || !strings.Contains(err.Error(), "the library must move to library schema version 2") {
		t.Fatalf("got %v; want schema version 1 refused with what the library must do", err)
	}
	_, err = trailguide.ParseLibrarySection([]byte(`{"schemaVersion":3}`), nil, "library")
	if !errors.As(err, &unsupported) || unsupported.Location != "library.schemaVersion" {
		t.Fatalf("ParseLibrarySection got %v; want an unsupported schema version", err)
	}
}

// TestParseLibraryManifest_RefusesAnInvalidManifest reports a *ValidationError at the location of the problem,
// starting with the manifest's location, and returns no library.
func TestParseLibraryManifest_RefusesAnInvalidManifest(t *testing.T) {
	const at = "acme/rules/.trailguide/config.yaml"
	curating := func(source string) string {
		return "sources:\n" + source + "library:\n  schemaVersion: 2\n  name: library\n  skills: {required: [anthropic:skills/skill-creator]}\n"
	}
	for name, test := range map[string]struct{ config, location string }{
		"not UTF-8":                               {"schemaVersion: 2\nsources: {}\n#\xff", at},
		"not a mapping":                           {"[]", at},
		"a duplicate key":                         {"schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: library}\nlibrary: {schemaVersion: 2, name: library}\n", at + ".library"},
		"an alias":                                {"schemaVersion: 2\nsources: &s {}\nlibrary: *s\n", at + ".sources"},
		"a library that isn't a map":              {"schemaVersion: 2\nsources: {}\nlibrary: false\n", at + ": library"},
		"no schema version":                       {"sources: {}\nlibrary: {skills: {}}\n", at + ": library.schemaVersion"},
		"a schema version that isn't a number":    {"sources: {}\nlibrary: {schemaVersion: one}\n", at + ": library.schemaVersion"},
		"a quoted schema version":                 {"sources: {}\nlibrary: {schemaVersion: '1'}\n", at + ": library.schemaVersion"},
		"a fractional schema version":             {"sources: {}\nlibrary: {schemaVersion: 1.5}\n", at + ": library.schemaVersion"},
		"schema version 0":                        {"sources: {}\nlibrary: {schemaVersion: 0}\n", at + ": library.schemaVersion"},
		"a notice where linked files go":          {"schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  license: {file: LICENSE.md, notices: [_repository/NOTICE.md]}\n", at + ": library.license.notices[0]"},
		"a license in a rule":                     {"schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  license: {file: .trailguide/rules/techs/go/x.md, notices: []}\n", at + ": library.license.file"},
		"an unknown SPDX ID":                      {"schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  license: {spdxExpression: Nope, file: LICENSE.md, notices: []}\n", at + ": library.license.spdxExpression"},
		"a skill listed twice":                    {"schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills: {required: [skills/a], offered: [skills/a]}\n", at + ": library.skills.offered[0]"},
		"a dependency projects can choose":        {"schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills: {offered: [skills/a], dependencies: [skills/a]}\n", at + ": library.skills.dependencies[0]"},
		"an invalid skill ID":                     {"schemaVersion: 2\nsources: {}\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills: {offered: [tools/a]}\n", at + ": library.skills.offered[0]"},
		"an undeclared source":                    {curating("  other: {repository: https://github.com/acme/other.git}\n"), at + ": library.skills.required[0]"},
		"curated sources that aren't a map":       {"sources: []\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills: {required: [anthropic:skills/skill-creator]}\n", at + ": sources"},
		"a curated source that isn't a map":       {curating("  anthropic: https://github.com/anthropics/skills.git\n"), at + ": sources.anthropic"},
		"a curated source without a repository":   {curating("  anthropic: {ref: main}\n"), at + ": sources.anthropic.repository"},
		"an invalid curated repository":           {curating("  anthropic: {repository: anthropics/skills}\n"), at + ": sources.anthropic.repository"},
		"an invalid curated ref":                  {curating("  anthropic: {repository: https://github.com/anthropics/skills.git, ref: 'refs/remotes/origin/main'}\n"), at + ": sources.anthropic.ref"},
		"an unquoted numeric curated ref":         {curating("  anthropic: {repository: https://github.com/anthropics/skills.git, ref: 1234567}\n"), at + ": sources.anthropic.ref"},
		"no name":                                 {"schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2}\n", at + ": library.name"},
		"a name that isn't a source name":         {"schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: Acme}\n", at + ": library.name"},
		"the name local":                          {"schemaVersion: 2\nsources: {}\nlibrary: {schemaVersion: 2, name: local}\n", at + ": library.name"},
		"the name of a curated source":            {"sources:\n  anthropic: {repository: https://github.com/anthropics/skills.git}\nlibrary:\n  schemaVersion: 2\n  name: anthropic\n  skills: {required: [anthropic:skills/skill-creator]}\n", at + ": library.name"},
		"two curated sources with one repository": {"sources:\n  a: {repository: https://github.com/acme/skills.git}\n  b: {repository: https://github.com/acme/skills}\nlibrary:\n  schemaVersion: 2\n  name: library\n  skills: {required: [a:skills/x, b:skills/y]}\n", at + ": sources.b"},
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
	_, err := trailguide.ParseLibrarySection([]byte(`{"schemaVersion":2,"name":"library","skills":{"required":["team:skills/a"]}}`), []string{"other"}, "library")
	var validation *trailguide.ValidationError
	if !errors.As(err, &validation) || validation.Location != "library.skills.required[0]" {
		t.Fatalf("got %v; want a validation error at library.skills.required[0]", err)
	}
	library, err := trailguide.ParseLibrarySection([]byte(`{"schemaVersion":2,"name":"library","skills":{"required":["team:skills/a"]}}`), []string{"team"}, "library")
	if err != nil || !reflect.DeepEqual(library.Skills.Required, []string{"team:skills/a"}) || library.Curated != nil {
		t.Fatalf("got %+v, %v", library, err)
	}
	if _, err := trailguide.ParseLibrarySection([]byte(`{}`), nil, "library"); !errors.As(err, &validation) || validation.Location != "library.schemaVersion" {
		t.Fatalf("got %v; want a missing schema version at library.schemaVersion", err)
	}
}
