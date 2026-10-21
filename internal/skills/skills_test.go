// Test skill IDs, install names, frontmatter reading, and installed-copy markers without files.

package skills_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/skills"
)

func TestValidNameFollowsTheAgentSkillsGrammar(t *testing.T) {
	for name, want := range map[string]bool{
		"build-feature": true, "tdd": true, "a1": true, "x": true,
		"": false, "Build": false, "-lead": false, "trail-": false, "double--hyphen": false, "under_score": false, "dot.name": false,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa":  true,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": false,
	} {
		if got := skills.ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestValidateIDAcceptsContainedFolderPaths(t *testing.T) {
	for id, valid := range map[string]bool{
		"skills/build-feature": true, "skills/engineering/to-spec": true, ".claude/skills/review": true, "tdd": true,
		"": false, "skills/": false, "/skills/x": false, "skills/../x": false, "skills/./x": false, "skills/*": false, "*": false, `skills\x`: false,
	} {
		err := skills.ValidateID(id, "skills.include")
		if (err == nil) != valid {
			t.Errorf("ValidateID(%q) = %v, want valid %v", id, err, valid)
		}
		var validation errs.ValidationError
		if err != nil && (!errors.As(err, &validation) || validation.ValidationLocation() != "skills.include") {
			t.Errorf("ValidateID(%q) error = %#v, want a validation error at skills.include", id, err)
		}
	}
}

func TestReadMetadataReadsTheFieldsGuidanceShows(t *testing.T) {
	tests := map[string]struct {
		input string
		want  skills.Metadata
	}{
		"full":                   {"---\nname: build-feature\ndescription: Build a feature.\nmetadata:\n  trailguide-skill-type: playbook\n  author: someone\n---\nSteps.\n", skills.Metadata{Name: "build-feature", Description: "Build a feature.", Type: "playbook", Frontmatter: true}},
		"crlf":                   {"---\r\nname: tdd\r\n---\r\nBody\r\n", skills.Metadata{Name: "tdd", Frontmatter: true}},
		"no frontmatter":         {"# A skill\n\nSteps.\n", skills.Metadata{}},
		"unknown fields kept":    {"---\nname: x\nallowed-tools: Bash\nlicense: MIT\n---\n", skills.Metadata{Name: "x", Frontmatter: true}},
		"closing at end of file": {"---\nname: x\n---", skills.Metadata{Name: "x", Frontmatter: true}},
		"cited rules and skills": {"---\nname: x\nmetadata:\n  trailguide-rules-used: |\n    techs/go/errors\n\n      fabrica:practices/testing/verify  \n  trailguide-skills-used: skills/tdd\n  trailguide-groups: x\n  trailguide-skill-type: review\n---\n", skills.Metadata{Name: "x", Type: "review", RulesUsed: []string{"techs/go/errors", "fabrica:practices/testing/verify"}, SkillsUsed: []string{"skills/tdd"}, ReservedKeys: []string{"trailguide-groups"}, Frontmatter: true}},
		"empty cited rules":      {"---\nname: x\nmetadata:\n  trailguide-rules-used: \"\"\n---\n", skills.Metadata{Name: "x", RulesUsed: []string{}, Frontmatter: true}},
		"interface text":         {"---\nname: x\nmetadata:\n  display-name: \"Add Skill\"\n  short-description: Create skills\n  default-prompt: \"Use $x.\"\n---\n", skills.Metadata{Name: "x", Interface: skills.Interface{DisplayName: "Add Skill", ShortDescription: "Create skills", DefaultPrompt: "Use $x."}, Frontmatter: true}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := skills.ReadMetadata([]byte(test.input), "skills/x/SKILL.md")
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ReadMetadata = %#v, %v; want %#v", got, err, test.want)
			}
		})
	}
}

func TestReadMetadataRejectsUnreadableFrontmatter(t *testing.T) {
	for name, input := range map[string]string{
		"unclosed":         "---\nname: x\n",
		"not a mapping":    "---\n- x\n---\n",
		"numeric name":     "---\nname: 12\n---\n",
		"list description": "---\nname: x\ndescription: [a]\n---\n",
		"scalar metadata":  "---\nname: x\nmetadata: text\n---\n",
		"boolean type":     "---\nname: x\nmetadata:\n  trailguide-skill-type: true\n---\n",
		"list rules used":  "---\nname: x\nmetadata:\n  trailguide-rules-used: [techs/go/errors]\n---\n",
		"number skills":    "---\nname: x\nmetadata:\n  trailguide-skills-used: 3\n---\n",
		"empty rules used": "---\nname: x\nmetadata:\n  trailguide-rules-used:\n---\n",
		"null skills used": "---\nname: x\nmetadata:\n  trailguide-skills-used: null\n---\n",
		"null type":        "---\nname: x\nmetadata:\n  trailguide-skill-type: ~\n---\n",
		"duplicate key":    "---\nname: x\nname: y\n---\n",
		"boolean display":  "---\nname: x\nmetadata:\n  display-name: true\n---\n",
		"list prompt":      "---\nname: x\nmetadata:\n  default-prompt: [a]\n---\n",
		"null short":       "---\nname: x\nmetadata:\n  short-description:\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := skills.ReadMetadata([]byte(input), "skills/x/SKILL.md"); err == nil {
				t.Fatal("ReadMetadata succeeded, want a validation error")
			}
		})
	}
}

func TestReadMetadataReportsTheYAMLErrorAtItsLineInTheFile(t *testing.T) {
	input := "---\nname: ratchet-files\ndescription: Ratchet source file size: find every file over budget.\n---\n# Ratchet\n"
	_, err := skills.ReadMetadata([]byte(input), "skills/ratchet-files/SKILL.md")
	want := "invalid YAML at line 3, column 38: mapping values are not allowed in this context; quote a value that contains a colon followed by a space"
	var invalid errs.ValidationError
	if !errors.As(err, &invalid) || invalid.ValidationLocation() != "skills/ratchet-files/SKILL.md" || invalid.ValidationProblem() != want {
		t.Fatalf("ReadMetadata = %v; want skills/ratchet-files/SKILL.md: %s", err, want)
	}
}

func TestInstallNameFallsBackToTheFolderName(t *testing.T) {
	tests := []struct {
		id       string
		metadata skills.Metadata
		want     string
		valid    bool
	}{
		{"skills/build-feature", skills.Metadata{Name: "build-feature"}, "build-feature", true},
		{"skills/engineering/to-spec", skills.Metadata{}, "to-spec", true},
		{"tools/Review", skills.Metadata{}, "", false},
		{"skills/x", skills.Metadata{Name: "Not Valid"}, "", false},
	}
	for _, test := range tests {
		got, err := skills.InstallName(test.id, test.metadata, test.id+"/SKILL.md")
		if got != test.want || (err == nil) != test.valid {
			t.Errorf("InstallName(%q, %#v) = %q, %v; want %q, valid %v", test.id, test.metadata, got, err, test.want, test.valid)
		}
	}
}

func TestMarkPlacesTheMarkerAfterTheFrontmatter(t *testing.T) {
	const marker = "<!-- marker -->"
	tests := map[string]struct{ input, want string }{
		"frontmatter":    {"---\nname: x\n---\nBody\n", "---\nname: x\n---\n" + marker + "\nBody\n"},
		"crlf":           {"---\r\nname: x\r\n---\r\nBody\r\n", "---\r\nname: x\r\n---\r\n" + marker + "\nBody\r\n"},
		"no frontmatter": {"# Skill\n", marker + "\n# Skill\n"},
		"no body":        {"---\nname: x\n---", "---\nname: x\n---\n" + marker + "\n"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := string(skills.Mark([]byte(test.input), marker)); got != test.want {
				t.Fatalf("Mark = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenameChangesOnlyTheNameField(t *testing.T) {
	tests := map[string]struct{ input, want string }{
		"plain":             {"---\nname: review\ndescription: Review.\n---\nUse review.\n", "---\nname: team-review\ndescription: Review.\n---\nUse review.\n"},
		"quoted":            {"---\ndescription: Review.\n\"name\": 'review' # upstream\nmetadata:\n  x: y\n---\nBody\n", "---\ndescription: Review.\nname: team-review\nmetadata:\n  x: y\n---\nBody\n"},
		"block scalar":      {"---\nname: >-\n  review\ndescription: Review.\n---\n", "---\nname: team-review\ndescription: Review.\n---\n"},
		"block scalar last": {"---\ndescription: Review.\nname: |-\n  review\n---\n", "---\ndescription: Review.\nname: team-review\n---\n"},
		"crlf":              {"---\r\nname: review\r\ndescription: Review.\r\n---\r\nBody\r\n", "---\r\nname: team-review\r\ndescription: Review.\r\n---\r\nBody\r\n"},
		"no name field":     {"---\ndescription: Review.\n---\nBody\n", "---\nname: team-review\ndescription: Review.\n---\nBody\n"},
		"no frontmatter":    {"# Review\n", "---\nname: team-review\n---\n# Review\n"},
		"no body":           {"---\nname: review\n---", "---\nname: team-review\n---"},
		"multiline plain":   {"---\nname: re\n  view\n# kept\ndescription: Review.\n---\n", "---\nname: team-review\n# kept\ndescription: Review.\n---\n"},
		"flow":              {"---\n{description: Review changes., name: review, metadata: {x: y}}\n---\nBody\n", "---\n{description: Review changes., name: team-review, metadata: {x: y}}\n---\nBody\n"},
		"flow without name": {"---\n{description: Review changes.}\n---\nBody\n", "---\n{name: team-review, description: Review changes.}\n---\nBody\n"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := skills.Rename([]byte(test.input), "team-review")
			if string(got) != test.want {
				t.Fatalf("Rename = %q, want %q", got, test.want)
			}
			metadata, err := skills.ReadMetadata(got, "SKILL.md")
			if err != nil || metadata.Name != "team-review" {
				t.Fatalf("renamed file reads as %#v, %v", metadata, err)
			}
		})
	}
}
