// Test parsing and editing a source's skills.exclude: the skills it leaves out and the authored skills replacing them.

package rules_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/rules"
)

// skillSource is a configuration whose source team includes every skill, followed by the YAML of its skills.exclude
// entries, if any.
const skillSource = "schemaVersion: 2\nsources:\n  team:\n    repository: https://example.invalid/team.git\n    skills:\n      include: [\"*\"]\n"

// commit is a full commit SHA for basedOn.
var commit = strings.Repeat("0123456789", 4)

func TestParseConfigurationReadsSkillExclusions(t *testing.T) {
	tests := map[string]struct {
		exclude string
		want    map[string]rules.SkillExclusion
	}{
		"none":       {"", map[string]rules.SkillExclusion{}},
		"empty list": {"      exclude: []\n", map[string]rules.SkillExclusion{}},
		"exclusion": {"      exclude:\n        - id: skills/review\n          reason: We review in CI.\n", map[string]rules.SkillExclusion{
			"skills/review": {Reason: "We review in CI."},
		}},
		"replacement without basedOn": {"      exclude:\n        - id: skills/review\n          reason: Ours.\n          replacedBy: skills/our-review\n", map[string]rules.SkillExclusion{
			"skills/review": {Reason: "Ours.", ReplacedBy: "skills/our-review"},
		}},
		"replacement with basedOn": {"      exclude:\n        - id: skills/engineering/fix-bug\n          reason: Ours.\n          replacedBy: skills/fix-bug\n          basedOn: \"" + commit + "\"\n", map[string]rules.SkillExclusion{
			"skills/engineering/fix-bug": {Reason: "Ours.", ReplacedBy: "skills/fix-bug", BasedOn: mustRef(t, commit)},
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := rules.ParseConfigurationYAML([]byte(skillSource + test.exclude))
			if err != nil {
				t.Fatal(err)
			}
			if got := config.Sources[0].SkillExclude; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("skill exclusions = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseConfigurationReadsLevelExclusions(t *testing.T) {
	config, err := rules.ParseConfigurationYAML([]byte(skillSource + "      exclude:\n        - id: recommended\n          reason: We pick our own.\n        - id: skills/review\n          reason: We review in CI.\n"))
	if err != nil {
		t.Fatal(err)
	}
	source := config.Sources[0]
	if want := map[string]string{"recommended": "We pick our own."}; !reflect.DeepEqual(source.SkillExcludeLevels, want) {
		t.Fatalf("level exclusions = %+v, want %+v", source.SkillExcludeLevels, want)
	}
	if _, ok := source.SkillExclude["skills/review"]; !ok || len(source.SkillExclude) != 1 {
		t.Fatalf("skill exclusions = %+v", source.SkillExclude)
	}
	for level, want := range map[string]bool{"required": false, "recommended": true, "offered": false} {
		if got := source.ExcludesLevel("skills/build", level); got != want {
			t.Errorf("recommended excludes %s = %v, want %v", level, got, want)
		}
	}
	config, err = rules.ParseConfigurationYAML([]byte(skillSource + "      exclude:\n        - id: \"*\"\n          reason: None of them.\n"))
	if err != nil {
		t.Fatal(err)
	}
	for level, want := range map[string]bool{"required": false, "recommended": true, "offered": true} {
		if got := config.Sources[0].ExcludesLevel("skills/build", level); got != want {
			t.Errorf("\"*\" excludes %s = %v, want %v", level, got, want)
		}
	}
}

func TestParseConfigurationRejectsInvalidSkillExclusions(t *testing.T) {
	const entry = "      exclude:\n        - id: skills/review\n          reason: Ours.\n"
	const at = "sources.team.skills.exclude.skills/review"
	tests := map[string]struct{ exclude, location, problem string }{
		"not a list":         {"      exclude: skills/review\n", "sources.team.skills.exclude", "expected a list"},
		"level replaced":     {"      exclude:\n        - id: recommended\n          reason: None.\n          replacedBy: skills/r\n", "sources.team.skills.exclude.recommended", "leaves out skills by level, so it takes only a reason"},
		"level no reason":    {"      exclude:\n        - id: \"*\"\n", "sources.team.skills.exclude.*.reason", "expected nonempty text"},
		"invalid ID":         {"      exclude:\n        - id: skills/../x\n          reason: None.\n", "sources.team.skills.exclude[0].id", "expected a skill ID"},
		"repeated ID":        {entry + "        - id: skills/review\n          reason: Again.\n", at, "listed more than once"},
		"missing reason":     {"      exclude:\n        - id: skills/review\n", at + ".reason", "expected nonempty text"},
		"blank reason":       {"      exclude:\n        - id: skills/review\n          reason: \" \"\n", at + ".reason", "expected nonempty text"},
		"unknown field":      {entry + "          version: 1.0.0\n", at, "version"},
		"replacement folder": {entry + "          replacedBy: .trailguide/skills/our-review/\n", at + ".replacedBy", "replacedBy names the authored skill by its ID, skills/our-review"},
		"replacement name":   {entry + "          replacedBy: our-review\n", at + ".replacedBy", "replacedBy names the authored skill by its ID, skills/our-review"},
		"replacement path":   {entry + "          replacedBy: skills/a/b\n", at + ".replacedBy", "expected the ID of an authored skill in .trailguide/skills/"},
		"basedOn alone":      {entry + "          basedOn: \"" + commit + "\"\n", at + ".basedOn", "so it needs replacedBy"},
		"basedOn short":      {entry + "          replacedBy: skills/r\n          basedOn: \"0123456\"\n", at + ".basedOn", `the full 40-character commit SHA the replacement was copied from, in quotes, got "0123456"`},
		"basedOn a version":  {entry + "          replacedBy: skills/r\n          basedOn: 1.0.0\n", at + ".basedOn", `the full 40-character commit SHA the replacement was copied from, in quotes, got "1.0.0"`},
		"basedOn not quoted": {entry + "          replacedBy: skills/r\n          basedOn: 12\n", at + ".basedOn", "the full 40-character commit SHA the replacement was copied from, in quotes"},
		"bare ID entry":      {"      exclude:\n        - skills/review\n", "sources.team.skills.exclude[0]", ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := rules.ParseConfigurationYAML([]byte(skillSource + test.exclude))
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || !strings.Contains(validation.ValidationProblem(), test.problem) {
				t.Fatalf("got %v; want %q at %s", err, test.problem, test.location)
			}
		})
	}
}

func TestEditConfigurationSourceAddsSkillExclusions(t *testing.T) {
	input := "# Team skills.\n" + skillSource + "      exclude:\n        - id: skills/zz\n          reason: Last.\n"
	out, err := rules.EditConfigurationSource([]byte(input), "team", rules.SourceEdit{SkillExclude: map[string]rules.SkillExclusion{
		"skills/review": {Reason: "We review in CI."},
		"skills/fix":    {Reason: "Ours.", ReplacedBy: "skills/fix", BasedOn: mustRef(t, commit)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "      exclude:\n        - id: skills/zz\n          reason: Last.\n        - id: skills/fix\n          reason: Ours.\n          replacedBy: skills/fix\n          basedOn: \"" + commit + "\"\n        - id: skills/review\n          reason: We review in CI.\n"
	if !strings.HasPrefix(string(out), "# Team skills.\n") || !strings.HasSuffix(string(out), want) {
		t.Fatalf("configuration:\n%s\nwant it to end with:\n%s", out, want)
	}
	config, err := rules.ParseConfigurationYAML(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Sources[0].SkillExclude["skills/fix"]; got.BasedOn.Canonical() != commit {
		t.Fatalf("basedOn = %q", got.BasedOn.Canonical())
	}
	for name, test := range map[string]struct{ input, location, problem string }{
		"no skills": {"schemaVersion: 2\nsources:\n  team:\n    repository: https://example.invalid/team.git\n    rules:\n      include: [\"*\"]\n", "sources.team.skills", "includes no skills"},
		"excluded":  {input, "sources.team.skills.exclude.skills/zz", "already has this entry"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := rules.EditConfigurationSource([]byte(test.input), "team", rules.SourceEdit{SkillExclude: map[string]rules.SkillExclusion{"skills/zz": {Reason: "Again."}}})
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || !strings.Contains(validation.ValidationProblem(), test.problem) {
				t.Fatalf("got %v; want %q at %s", err, test.problem, test.location)
			}
		})
	}
}

func TestEditConfigurationSourceIncludesTheSkillsItForks(t *testing.T) {
	input := "schemaVersion: 2\nsources:\n  pocock:\n    repository: https://example.invalid/pocock.git\n"
	out, err := rules.EditConfigurationSource([]byte(input), "pocock", rules.SourceEdit{
		SkillInclude: []string{"skills/ops/deploy"},
		SkillExclude: map[string]rules.SkillExclusion{"skills/ops/deploy": {Reason: "Ours.", ReplacedBy: "skills/deploy", BasedOn: mustRef(t, commit)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "    skills:\n      include:\n        - skills/ops/deploy\n      exclude:\n        - id: skills/ops/deploy\n          reason: Ours.\n          replacedBy: skills/deploy\n          basedOn: \"" + commit + "\"\n"
	if !strings.HasSuffix(string(out), want) {
		t.Fatalf("configuration:\n%s\nwant it to end with:\n%s", out, want)
	}
	out, err = rules.EditConfigurationSource([]byte(skillSource), "team", rules.SourceEdit{SkillInclude: []string{"skills/fix"}})
	if err != nil || strings.Contains(string(out), "skills/fix") {
		t.Fatalf("got %v:\n%s\nwant the source's include list unchanged", err, out)
	}
}

func TestReplaceLibrarySkillKeepsItsLevel(t *testing.T) {
	input := "schemaVersion: 2\nsources:\n  pocock:\n    repository: https://example.invalid/pocock.git\nlibrary:\n  skills:\n    # Everyone deploys.\n    required: [pocock:skills/ops/deploy, skills/review]\n"
	out, err := rules.ReplaceLibrarySkill([]byte(input), "pocock:skills/ops/deploy", "skills/deploy")
	if err != nil {
		t.Fatal(err)
	}
	if want := "    # Everyone deploys.\n    required: [skills/deploy, skills/review]\n"; !strings.HasSuffix(string(out), want) {
		t.Fatalf("configuration:\n%s\nwant it to end with:\n%s", out, want)
	}
	_, err = rules.ReplaceLibrarySkill([]byte(input), "pocock:skills/ops/grill", "skills/grill")
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "library.skills" {
		t.Fatalf("got %v; want a validation error at library.skills", err)
	}
}
