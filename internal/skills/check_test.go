// Test that Check reports every problem in an authored skill's name and Trail Guide metadata keys.

package skills_test

import (
	"reflect"
	"testing"

	"github.com/fabricahq/trailguide/internal/skills"
)

func TestCheckReportsEachProblemInAnAuthoredSkillsFrontmatter(t *testing.T) {
	valid := skills.Metadata{Name: "fix-bug", Frontmatter: true}
	with := func(change func(*skills.Metadata)) skills.Metadata {
		metadata := valid
		change(&metadata)
		return metadata
	}
	tests := map[string]struct {
		metadata  skills.Metadata
		qualified bool
		want      []string
	}{
		"valid without metadata": {valid, false, []string{}},
		"each type":              {with(func(m *skills.Metadata) { m.Type = "review" }), false, []string{}},
		"playbook":               {with(func(m *skills.Metadata) { m.Type = "playbook" }), false, []string{}},
		"citations of a library skill": {with(func(m *skills.Metadata) {
			m.RulesUsed, m.SkillsUsed = []string{"practices/testing/verify-retries", "techs/go/errors"}, []string{"skills/tdd"}
		}), false, []string{}},
		"citations of a project skill": {with(func(m *skills.Metadata) {
			m.RulesUsed, m.SkillsUsed = []string{"practices/release/tag-from-main", "fabrica:practices/testing/verify-retries"}, []string{"skills/tdd", "fabrica:skills/engineering/to-spec"}
		}), true, []string{}},
		"no frontmatter":           {skills.Metadata{}, false, []string{"expected YAML frontmatter at the start of the file"}},
		"missing name":             {with(func(m *skills.Metadata) { m.Name = "" }), false, []string{"name is required, and must be the folder's name, fix-bug"}},
		"name isn't folder":        {with(func(m *skills.Metadata) { m.Name = "fix-bugs" }), false, []string{`name is "fix-bugs", but the folder is fix-bug; they must match`}},
		"unknown type":             {with(func(m *skills.Metadata) { m.Type = "design" }), false, []string{`trailguide-skill-type is "design"; use playbook or review, or leave it out`}},
		"type spelled in capitals": {with(func(m *skills.Metadata) { m.Type = "Review" }), false, []string{`trailguide-skill-type is "Review"; use playbook or review, or leave it out`}},
		"reserved key":             {with(func(m *skills.Metadata) { m.ReservedKeys = []string{"trailguide-groups"} }), false, []string{`metadata."trailguide-groups" isn't a Trail Guide key; use trailguide-skill-type, trailguide-rules-used, or trailguide-skills-used`}},
		"qualified rule in a library": {with(func(m *skills.Metadata) { m.RulesUsed = []string{"fabrica:techs/go/errors"} }), false, []string{
			`trailguide-rules-used names "fabrica:techs/go/errors"; cite a rule by its ID in this library, without a source name, such as practices/testing/verify-retries`,
		}},
		"rule file name": {with(func(m *skills.Metadata) { m.RulesUsed = []string{"techs/go/errors.md"} }), true, []string{
			`trailguide-rules-used names "techs/go/errors.md"; cite a rule by its ID, without .md, such as techs/go/errors`,
		}},
		"group instead of rule": {with(func(m *skills.Metadata) { m.RulesUsed = []string{"techs/go"} }), true, []string{
			`trailguide-rules-used names "techs/go"; expected a rule ID, a rule's path in rules/ without .md, such as practices/testing/verify-retries`,
		}},
		"malformed source name": {with(func(m *skills.Metadata) { m.RulesUsed = []string{"Fabrica:techs/go/errors"} }), true, []string{
			`trailguide-rules-used names "Fabrica:techs/go/errors"; expected a source name, a colon, and a rule ID, such as fabrica:practices/testing/verify-retries`,
		}},
		"repeated rule":     {with(func(m *skills.Metadata) { m.RulesUsed = []string{"techs/go/errors", "techs/go/errors"} }), false, []string{"trailguide-rules-used names techs/go/errors twice"}},
		"skill uses itself": {with(func(m *skills.Metadata) { m.SkillsUsed = []string{"skills/fix-bug"} }), false, []string{"trailguide-skills-used names the skill itself"}},
		"skill by name only": {with(func(m *skills.Metadata) { m.SkillsUsed = []string{"tdd"} }), false, []string{
			`trailguide-skills-used names "tdd"; expected skills/ and a skill's name, such as skills/tdd`,
		}},
		"curated skill in a library": {with(func(m *skills.Metadata) { m.SkillsUsed = []string{"anthropic:skills/skill-creator"} }), false, []string{}},
		"qualified skill without an ID": {with(func(m *skills.Metadata) { m.SkillsUsed = []string{"fabrica:"} }), true, []string{
			`trailguide-skills-used names "fabrica:"; expected a source name, a colon, and the skill's ID in that source, such as fabrica:skills/tdd`,
		}},
		"repeated skill": {with(func(m *skills.Metadata) { m.SkillsUsed = []string{"skills/tdd", "skills/tdd"} }), false, []string{"trailguide-skills-used names skills/tdd twice"}},
		"several problems in order": {with(func(m *skills.Metadata) {
			m.Name, m.Type, m.RulesUsed, m.SkillsUsed = "other", "design", []string{"techs/go"}, []string{"skills/fix-bug"}
		}), false, []string{
			`name is "other", but the folder is fix-bug; they must match`,
			`trailguide-skill-type is "design"; use playbook or review, or leave it out`,
			`trailguide-rules-used names "techs/go"; expected a rule ID, a rule's path in rules/ without .md, such as practices/testing/verify-retries`,
			"trailguide-skills-used names the skill itself",
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := skills.Check("fix-bug", test.metadata, test.qualified); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Check =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}
