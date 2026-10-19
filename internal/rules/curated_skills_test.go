// Test that a library lists curated skills only from the sources its configuration declares.

package rules_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
)

func TestParseConfiguration_ReadsCuratedSkillsAndTheirSources(t *testing.T) {
	config, err := rules.ParseConfigurationYAML([]byte("schemaVersion: 2\nsources:\n  pocock:\n    repository: https://github.com/mattpocock/skills.git\n    ref: main-2026\nlibrary:\n  skills:\n    required: [pocock:skills/ops/deploy]\n    recommended: [pocock:skills/engineering/to-spec, skills/build-feature]\n"))
	if err != nil {
		t.Fatal(err)
	}
	listed := config.Library.Skills
	if listed.Level("pocock:skills/ops/deploy") != skills.LevelRequired || listed.Level("skills/build-feature") != skills.LevelRecommended {
		t.Fatalf("levels = %+v", listed)
	}
	if got := listed.Curated(); !slices.Equal(got, []string{"pocock:skills/engineering/to-spec", "pocock:skills/ops/deploy"}) {
		t.Fatalf("curated = %v", got)
	}
	if source := config.Library.Curated["pocock"]; source.Repository != "https://github.com/mattpocock/skills.git" || source.Ref.String() != "main-2026" {
		t.Fatalf("curated source = %+v", source)
	}
	if requires := config.Sources[0].LibraryRequires; !slices.Equal(requires, []string{"skills/ops/deploy"}) {
		t.Fatalf("the library repository's own source requires %v; want the required curated skill", requires)
	}
}

func TestParseConfiguration_RejectsInvalidCuratedSkills(t *testing.T) {
	for name, test := range map[string]struct {
		config, location, problem string
	}{
		"undeclared source": {
			config:   "schemaVersion: 2\nsources: {}\nlibrary:\n  skills:\n    offered: [pocock:skills/to-spec]\n",
			location: "library.skills.offered[0]",
			problem:  "pocock:skills/to-spec comes from pocock, which isn't a source in sources; add sources.pocock with the repository the skill comes from, and a ref unless the repository publishes library releases",
		},
		"listed twice": {
			config:   "schemaVersion: 2\nsources:\n  pocock: {repository: https://github.com/mattpocock/skills.git, ref: v1}\nlibrary:\n  skills:\n    required: [pocock:skills/to-spec]\n    offered: [pocock:skills/to-spec]\n",
			location: "library.skills.offered[0]",
			problem:  "pocock:skills/to-spec is already required; give each skill one level",
		},
		"invalid source name": {
			config:   "schemaVersion: 2\nsources: {}\nlibrary:\n  skills:\n    offered: [Pocock:skills/to-spec]\n",
			location: "library.skills.offered[0]",
			problem:  `expected a curated skill as the name of a source in sources, a colon, and the skill's ID in that source's repository, such as mattpocock:skills/to-spec, got "Pocock:skills/to-spec"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := rules.ParseConfigurationYAML([]byte(test.config))
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || validation.ValidationProblem() != test.problem {
				t.Fatalf("got %v; want %s: %s", err, test.location, test.problem)
			}
		})
	}
}
