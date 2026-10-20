// Test parsing and editing a source's skills.exclude and skills.forks: the skills it leaves out and the authored
// skills it installs in their place.

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
		"not a list":      {"      exclude: skills/review\n", "sources.team.skills.exclude", "expected a list"},
		"level replaced":  {"      exclude:\n        - id: recommended\n          reason: None.\n          replacedBy: skills/r\n", "sources.team.skills.exclude.recommended", "leaves out skills by level, so it takes only a reason"},
		"level no reason": {"      exclude:\n        - id: \"*\"\n", "sources.team.skills.exclude.*.reason", "expected nonempty text"},
		"invalid ID":      {"      exclude:\n        - id: skills/../x\n          reason: None.\n", "sources.team.skills.exclude[0].id", "expected a skill ID"},
		"repeated ID":     {entry + "        - id: skills/review\n          reason: Again.\n", at, "listed more than once"},
		"missing reason":  {"      exclude:\n        - id: skills/review\n", at + ".reason", "expected nonempty text"},
		"blank reason":    {"      exclude:\n        - id: skills/review\n          reason: \" \"\n", at + ".reason", "expected nonempty text"},
		"unknown field":   {entry + "          version: 1.0.0\n", at, "version"},
		"old fork shape":  {entry + "          replacedBy: skills/our-review\n          basedOn: \"" + commit + "\"\n", at, "skills.exclude no longer records forks"},
		"basedOn alone":   {entry + "          basedOn: \"" + commit + "\"\n", at, "skills.exclude no longer records forks"},
		"bare ID entry":   {"      exclude:\n        - skills/review\n", "sources.team.skills.exclude[0]", ""},
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

func TestParseConfigurationShowsTheForkForAnOldForkEntry(t *testing.T) {
	_, err := rules.ParseConfigurationYAML([]byte(skillSource + "      exclude:\n        - id: skills/ops/deploy\n          reason: Ours.\n          replacedBy: skills/deploy\n          basedOn: \"" + commit + "\"\n"))
	want := "forks:\n  skills/ops/deploy:\n    as: skills/deploy\n    basedOn: \"" + commit + "\"\n    reason: Ours."
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v; want it to show:\n%s", err, want)
	}
}

func TestParseConfigurationReadsSkillForks(t *testing.T) {
	// A source can exist only for its forks, so skills.forks needs no include.
	const forkSource = "schemaVersion: 2\nsources:\n  team:\n    repository: https://example.invalid/team.git\n    ref: main\n    skills:\n      forks:\n"
	config, err := rules.ParseConfigurationYAML([]byte(forkSource + "        skills/engineering/fix-bug:\n          as: skills/fix-bug\n          basedOn: \"" + commit + "\"\n          reason: Ours.\n        skills/review:\n          as: skills/our-review\n          reason: Written from scratch.\n"))
	if err != nil {
		t.Fatal(err)
	}
	source := config.Sources[0]
	want := map[string]rules.SkillFork{
		"skills/engineering/fix-bug": {As: "skills/fix-bug", Reason: "Ours.", BasedOn: mustRef(t, commit)},
		"skills/review":              {As: "skills/our-review", Reason: "Written from scratch."},
	}
	if !reflect.DeepEqual(source.SkillForks, want) || len(source.Skills) != 0 {
		t.Fatalf("forks = %+v, include = %v; want %+v and no include", source.SkillForks, source.Skills, want)
	}
	if source.ExcludesLevel("skills/review", "offered") {
		t.Fatal("a level exclusion leaves out a forked skill, which the source must import to compare")
	}
	if got := source.SkillDivergences(); got["skills/review"] != "sources.team.skills.forks.skills/review" {
		t.Fatalf("divergences = %v", got)
	}
}

func TestParseConfigurationRejectsInvalidSkillForks(t *testing.T) {
	const at = "sources.team.skills.forks.skills/review"
	fork := func(fields string) string { return "      forks:\n        skills/review:\n" + fields }
	const valid = "          as: skills/our-review\n          reason: Ours.\n"
	tests := map[string]struct{ skills, location, problem string }{
		"not a mapping":      {"      forks: [skills/review]\n", "sources.team.skills.forks", "expected a mapping of skill IDs to forks"},
		"a level":            {"      forks:\n        recommended:\n" + valid, "sources.team.skills.forks.recommended", "a fork names one skill by ID"},
		"invalid ID":         {"      forks:\n        skills/../x:\n" + valid, "sources.team.skills.forks.skills/../x", "expected a skill ID"},
		"missing as":         {fork("          reason: Ours.\n"), at + ".as", "expected the ID of the authored skill"},
		"as a folder":        {fork("          as: .trailguide/skills/our-review/\n          reason: Ours.\n"), at + ".as", "name the authored skill by its ID, skills/our-review"},
		"as a name":          {fork("          as: our-review\n          reason: Ours.\n"), at + ".as", "name the authored skill by its ID, skills/our-review"},
		"as a path":          {fork("          as: skills/a/b\n          reason: Ours.\n"), at + ".as", "expected the ID of an authored skill in .trailguide/skills/"},
		"missing reason":     {fork("          as: skills/our-review\n"), at + ".reason", "expected nonempty text"},
		"unknown field":      {fork(valid + "          replacedBy: skills/x\n"), at, "replacedBy"},
		"basedOn short":      {fork(valid + "          basedOn: \"0123456\"\n"), at + ".basedOn", `the full 40-character commit SHA the fork was copied from, in quotes, got "0123456"`},
		"basedOn not quoted": {fork(valid + "          basedOn: 12\n"), at + ".basedOn", "the full 40-character commit SHA the fork was copied from, in quotes"},
		"shared fork":        {fork(valid) + "        skills/z:\n" + valid, "sources.team.skills.forks.skills/z.as", "skills/our-review is already the fork of skills/review"},
		"also excluded":      {"      exclude:\n        - id: skills/review\n          reason: No.\n" + fork(valid), at, "also excludes this skill"},
		"also renamed":       {fork(valid) + "      rename:\n        skills/review: team-review\n", "sources.team.skills.rename.skills/review", "installs under its own name, our-review"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := rules.ParseConfigurationYAML([]byte(skillSource + test.skills))
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || !strings.Contains(validation.ValidationProblem(), test.problem) {
				t.Fatalf("got %v; want %q at %s", err, test.problem, test.location)
			}
		})
	}
}

func TestEditConfigurationSourceAddsSkillForks(t *testing.T) {
	input := "# Team skills.\n" + skillSource + "      forks:\n        skills/zz:\n          as: skills/zz\n          reason: Last.\n"
	out, err := rules.EditConfigurationSource([]byte(input), "team", rules.SourceEdit{SkillForks: map[string]rules.SkillFork{
		"skills/review": {As: "skills/our-review", Reason: "Written from scratch."},
		"skills/fix":    {As: "skills/fix", Reason: "Ours.", BasedOn: mustRef(t, commit)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "      forks:\n        skills/zz:\n          as: skills/zz\n          reason: Last.\n        skills/fix:\n          as: skills/fix\n          basedOn: \"" + commit + "\"\n          reason: Ours.\n        skills/review:\n          as: skills/our-review\n          reason: Written from scratch.\n"
	if !strings.HasPrefix(string(out), "# Team skills.\n") || !strings.HasSuffix(string(out), want) {
		t.Fatalf("configuration:\n%s\nwant it to end with:\n%s", out, want)
	}
	config, err := rules.ParseConfigurationYAML(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Sources[0].SkillForks["skills/fix"]; got.BasedOn.Canonical() != commit {
		t.Fatalf("basedOn = %q", got.BasedOn.Canonical())
	}
	_, err = rules.EditConfigurationSource([]byte(input), "team", rules.SourceEdit{SkillForks: map[string]rules.SkillFork{"skills/zz": {As: "skills/zz2", Reason: "Again."}}})
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "sources.team.skills.forks.skills/zz" || !strings.Contains(validation.ValidationProblem(), "already forks this skill") {
		t.Fatalf("got %v; want a validation error for the existing fork", err)
	}
}

func TestEditConfigurationSourceForksFromASourceWithoutSkills(t *testing.T) {
	input := "schemaVersion: 2\nsources:\n  pocock:\n    repository: https://example.invalid/pocock.git\n"
	out, err := rules.EditConfigurationSource([]byte(input), "pocock", rules.SourceEdit{
		SkillForks: map[string]rules.SkillFork{"skills/ops/deploy": {As: "skills/deploy", Reason: "Ours.", BasedOn: mustRef(t, commit)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "    skills:\n      forks:\n        skills/ops/deploy:\n          as: skills/deploy\n          basedOn: \"" + commit + "\"\n          reason: Ours.\n"
	if !strings.HasSuffix(string(out), want) {
		t.Fatalf("configuration:\n%s\nwant it to end with:\n%s", out, want)
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
