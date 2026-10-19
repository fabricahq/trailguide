// Verify YAML configuration semantics and refusal of ambiguous or unsupported syntax.

package rules_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
	"go.yaml.in/yaml/v4"
)

func TestYAMLConfigurationMatchesValidSchemaFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/configuration/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID, Input string
		Expected  struct{ OK bool }
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		if !tc.Expected.OK {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(tc.Input), &value); err != nil {
				t.Fatal(err)
			}
			encoded, err := yaml.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			want, err := rules.ParseConfiguration([]byte(tc.Input))
			if err != nil {
				t.Fatal(err)
			}
			got, err := rules.ParseConfigurationYAML(encoded)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("YAML differs: %s\n%+v\n%v", encoded, got, err)
			}
		})
	}
}

func TestYAMLConfigurationRejectsAmbiguousInput(t *testing.T) {
	for name, input := range map[string]string{
		"empty": "", "nonobject": "[]", "duplicate": "schemaVersion: 2\nschemaVersion: 2\nsources: {}",
		"unknown": "schemaVersion: 2\nsources: {}\nextra: true", "multiple": "schemaVersion: 2\nsources: {}\n---\nschemaVersion: 2\nsources: {}",
		"anchor": "schemaVersion: 2\nsources: &sources {}", "alias": "schemaVersion: 2\nsources: *sources",
		"tag": "schemaVersion: !!int 2\nsources: {}", "merge": "schemaVersion: 2\nsources: {<<: {}}",
		"nonstringkey": "schemaVersion: 2\nsources: {123: {}}", "null": "schemaVersion: 2\nsources: null",
		"infinity": "schemaVersion: .inf\nsources: {}", "utf8": "schemaVersion: 2\nsources: {}\n#\xff",
		"deep":           "schemaVersion: 2\nsources: " + strings.Repeat("[", 40) + strings.Repeat("]", 40),
		"duplicatealias": "schemaVersion: 2\nsources: {team: {}, team: {}}",
		"version 1":      "schemaVersion: 1\nsources: {}",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := rules.ParseConfigurationYAML([]byte(input))
			if err == nil || !reflect.DeepEqual(got, rules.Configuration{}) {
				t.Fatalf("accepted invalid input: %+v, %v", got, err)
			}
		})
	}
}

// TestAppendConfigurationSource preserves authored presentation while validating the edited document.
func TestAppendConfigurationSource(t *testing.T) {
	input := []byte(`# Project guidance
schemaVersion: 2
sources:
  existing:
    repository: 'https://github.com/acme/existing.git' # keep the selected library
    ref: 'v0.0.9'
    rules: {include: ['*'], exclude: []}
`)
	source := rules.Source{Repository: "https://github.com/acme/added.git", Ref: ref(t, "v0.1.0"), Groups: rules.GroupSelection{Pattern: "*"}}
	out, err := rules.AppendConfigurationSource(input, "added", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{"# Project guidance", "'https://github.com/acme/existing.git' # keep the selected library", "ref: 'v0.0.9'", "rules: {include: ['*'], exclude: []}"} {
		if !strings.Contains(string(out), retained) {
			t.Fatalf("lost authored presentation %q:\n%s", retained, out)
		}
	}
	if strings.Index(string(out), "  existing:") > strings.Index(string(out), "  added:") {
		t.Fatalf("reordered existing source:\n%s", out)
	}
	got, err := rules.ParseConfigurationYAML(out)
	if err != nil || len(got.Sources) != 2 || got.Sources[0].Name != "added" || got.Sources[0].Repository != source.Repository {
		t.Fatalf("invalid appended source: %+v, %v", got, err)
	}
	if out, err := rules.AppendConfigurationSource(input, "existing", source); err == nil || out != nil {
		t.Fatalf("accepted duplicate source: %s, %v", out, err)
	}
}

func TestAppendConfigurationSourcePreservesFoldedExclusions(t *testing.T) {
	for _, header := range []string{">", ">-", ">+"} {
		t.Run(header, func(t *testing.T) {
			input := []byte("schemaVersion: 2\nsources:\n  existing:\n    repository: https://example.invalid/existing.git\n    ref: v1.0.0\n    rules:\n      include: ['*']\n      exclude:\n        - id: techs/go/old\n          reason: " + header + " # keep this reason\n            Heading:\n\n              * first item\n              * second item\n\n")
			before, err := rules.ParseConfigurationYAML(input)
			if err != nil {
				t.Fatal(err)
			}
			want := before.Sources[0].Exclude["techs/go/old"].Reason
			source := rules.Source{Repository: "https://example.invalid/added.git", Ref: ref(t, "v1.0.0"), Groups: rules.GroupSelection{Pattern: "*"}}
			for _, alias := range []string{"second", "third"} {
				source.Repository = "https://example.invalid/" + alias + ".git"
				input, err = rules.AppendConfigurationSource(input, alias, source)
				if err != nil {
					t.Fatal(err)
				}
				config, err := rules.ParseConfigurationYAML(input)
				if err != nil {
					t.Fatal(err)
				}
				for _, existing := range config.Sources {
					if existing.Name == "existing" && existing.Exclude["techs/go/old"].Reason != want {
						t.Fatalf("%s changed exclusion from %q to %q:\n%s", alias, want, existing.Exclude["techs/go/old"].Reason, input)
					}
				}
				if !strings.Contains(string(input), "# keep this reason") {
					t.Fatalf("%s lost exclusion comment:\n%s", alias, input)
				}
			}
		})
	}
}

// TestAppendConfigurationSourceWritesEverySuppliedField round-trips selections, pins, and exclusions, writing the
// wildcard, then groups as selectors, then rules in rules.include, and omitting empty fields.
func TestAppendConfigurationSourceWritesEverySuppliedField(t *testing.T) {
	input := []byte("schemaVersion: 2\nsources: {}\n")
	pinned, err := trailguide.ParseRuleVersion("1.3.0", "version")
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		source  rules.Source
		written []string
		omitted []string
	}{
		"rules only": {
			source:  rules.Source{Repository: "https://example.invalid/rules.git", Groups: rules.GroupSelection{Groups: []string{}}, Rules: []string{"practices/testing/verify-retry-limits", "techs/go/wrap-errors"}},
			written: []string{"    rules:\n      include:\n        - practices/testing/verify-retry-limits\n        - techs/go/wrap-errors\n"},
			omitted: []string{"ref:", "pins:", "exclude:"},
		},
		"ref": {
			source:  rules.Source{Repository: "https://example.invalid/rules.git", Ref: ref(t, "release/5"), Groups: rules.GroupSelection{Groups: []string{"techs/go"}}},
			written: []string{"    ref: release/5\n    rules:\n      include:\n        - techs/go/*\n"},
			omitted: []string{"pins:", "exclude:"},
		},
		"wildcard, groups, and rules": {
			source:  rules.Source{Repository: "https://example.invalid/rules.git", Groups: rules.GroupSelection{Pattern: "practices/*", Groups: []string{"techs/go", "techs/react"}}, Rules: []string{"techs/python/typed"}},
			written: []string{"      include:\n        - practices/*\n        - techs/go/*\n        - techs/react/*\n        - techs/python/typed\n"},
			omitted: []string{"ref:", "pins:", "exclude:"},
		},
		"pins and exclusions": {
			source: rules.Source{
				Repository: "https://example.invalid/rules.git", Groups: rules.GroupSelection{Pattern: "*", Groups: []string{}},
				Pins: map[string]rules.Pin{"techs/go/a": {Version: pinned, Reason: "Waiting on review."}},
				Exclude: map[string]rules.Exclusion{
					"techs/go/b":    {Reason: "Not applicable."},
					"techs/go/c":    {Reason: "Project policy.", ReplacedBy: "techs/go/c"},
					"techs/react/*": {Reason: "No frontend."},
				},
			},
			written: []string{
				"      pins:\n        - id: techs/go/a\n          version: \"1.3.0\"\n          reason: Waiting on review.\n",
				"      exclude:\n        - id: techs/go/b\n          reason: Not applicable.\n        - id: techs/go/c\n          reason: Project policy.\n          replacedBy: techs/go/c\n        - id: techs/react/*\n",
			},
			omitted: []string{"ref:"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := rules.AppendConfigurationSource(input, "team", test.source)
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range test.written {
				if !strings.Contains(string(out), text) {
					t.Fatalf("missing %q:\n%s", text, out)
				}
			}
			for _, text := range test.omitted {
				if strings.Contains(string(out), text) {
					t.Fatalf("wrote %q:\n%s", text, out)
				}
			}
			got, err := rules.ParseConfigurationYAML(out)
			if err != nil {
				t.Fatal(err)
			}
			want := test.source
			want.Name = "team"
			if want.Rules == nil {
				want.Rules = []string{}
			}
			if want.Pins == nil {
				want.Pins = map[string]rules.Pin{}
			}
			if want.Exclude == nil {
				want.Exclude = map[string]rules.Exclusion{}
			}
			if want.Skills == nil {
				want.Skills = []string{}
			}
			if want.SkillUses == nil {
				want.SkillUses = map[string][]string{}
			}
			if want.SkillExclude == nil {
				want.SkillExclude = map[string]rules.SkillExclusion{}
			}
			if want.SkillExcludeLevels == nil {
				want.SkillExcludeLevels = map[string]string{}
			}
			if len(got.Sources) != 1 || !reflect.DeepEqual(got.Sources[0], want) {
				t.Fatalf("got %+v; want %+v\n%s", got.Sources, want, out)
			}
		})
	}
}

// TestAppendConfigurationSourceValidatesInputFirst preserves errors from the existing document.
func TestAppendConfigurationSourceValidatesInputFirst(t *testing.T) {
	for _, input := range []string{"schemaVersion: 2", "schemaVersion: 2\nsources: []", "schemaVersion: 2\nsources: {}\nsources: {}", "schemaVersion: 1\nsources: {}"} {
		_, want := rules.ParseConfigurationYAML([]byte(input))
		out, got := rules.AppendConfigurationSource([]byte(input), "invalid alias", rules.Source{})
		if want == nil || got == nil || got.Error() != want.Error() || out != nil {
			t.Fatalf("input error lost: output %s, got %v, want %v", out, got, want)
		}
	}
}

// TestAppendConfigurationLibrary adds a library section after a project's sources, keeping their presentation, and
// refuses a second section and an invalid one.
func TestAppendConfigurationLibrary(t *testing.T) {
	input := []byte(`# Project guidance
schemaVersion: 2
sources:
  existing:
    repository: 'https://github.com/acme/existing.git' # keep the selected library
    rules: {include: ['*']}
`)
	out, err := rules.AppendConfigurationLibrary(input, map[string]any{"license": map[string]any{"spdxExpression": "MIT", "file": "LICENSE.md", "notices": []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{"# Project guidance", "'https://github.com/acme/existing.git' # keep the selected library", "rules: {include: ['*']}"} {
		if !strings.Contains(string(out), retained) {
			t.Fatalf("lost authored presentation %q:\n%s", retained, out)
		}
	}
	if strings.Index(string(out), "sources:") > strings.Index(string(out), "library:") {
		t.Fatalf("library section precedes the sources:\n%s", out)
	}
	got, err := rules.ParseConfigurationYAML(out)
	if err != nil || len(got.Sources) != 1 || got.Library == nil || got.Library.License == nil || *got.Library.License.SPDXExpression != "MIT" {
		t.Fatalf("invalid appended library section: %+v, %v", got, err)
	}
	if again, err := rules.AppendConfigurationLibrary(out, map[string]any{}); err == nil || again != nil || err.Error() != "library: the configuration already has a library section" {
		t.Fatalf("accepted a second library section: %s, %v", again, err)
	}
	if invalid, err := rules.AppendConfigurationLibrary(input, map[string]any{"skills": map[string]any{"required": "skills/ship"}}); err == nil || invalid != nil {
		t.Fatalf("accepted an invalid library section: %s, %v", invalid, err)
	}
	for _, input := range []string{"schemaVersion: 2", "schemaVersion: 1\nsources: {}"} {
		_, want := rules.ParseConfigurationYAML([]byte(input))
		out, got := rules.AppendConfigurationLibrary([]byte(input), map[string]any{})
		if want == nil || got == nil || got.Error() != want.Error() || out != nil {
			t.Fatalf("input error lost: output %s, got %v, want %v", out, got, want)
		}
	}
}
