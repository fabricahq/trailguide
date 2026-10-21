// Validate the frontmatter of a skill a repository authors: its name, and the metadata keys Trail Guide defines.

package skills

import (
	"fmt"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/librarytree"
)

// The metadata keys Trail Guide defines. Every other key that starts with trailguide- is reserved.
const (
	TypeKey       = "trailguide-skill-type"
	RulesUsedKey  = "trailguide-rules-used"
	SkillsUsedKey = "trailguide-skills-used"
)

// Types lists the values trailguide-skill-type accepts. A type is a label only: Trail Guide shows it and treats every skill
// the same way.
var Types = []string{"playbook", "review"}

// Citation is one ID that trailguide-rules-used or trailguide-skills-used names: a rule or skill ID, and, when the line qualifies
// it, the name of the source that supplies it.
type Citation struct {
	// Source is the source name before the first colon, or empty for a bare ID, which names a rule or skill the
	// skill's own repository authors.
	Source string
	ID     string
}

// ParseCitation splits a line of trailguide-rules-used or trailguide-skills-used into its source name and ID. It checks nothing.
func ParseCitation(line string) Citation {
	if source, id, found := strings.Cut(line, ":"); found {
		return Citation{Source: source, ID: id}
	}
	return Citation{ID: line}
}

// String returns the citation as written: source:id, or the bare ID.
func (c Citation) String() string {
	if c.Source == "" {
		return c.ID
	}
	return c.Source + ":" + c.ID
}

// Check returns every problem in the frontmatter of the skill a repository authors in skills/<folder>/, in a fixed
// order, or none: frontmatter that is missing, a name other than folder, a type that isn't in Types, a reserved trailguide-
// key, and trailguide-rules-used or trailguide-skills-used lines that aren't IDs, repeat an ID, or name the skill itself. qualified
// allows rule IDs that name a source, as a project's skills may; a library's skills cite only its own rules. Skill IDs
// may name a source either way: a project's skills use its sources' skills, and a library's use the skills it curates.
// It checks the IDs' form only, not that the rules and skills they name exist. Each problem is a sentence about
// SKILL.md, for the caller to place at its location.
func Check(folder string, metadata Metadata, qualified bool) []string {
	if !metadata.Frontmatter {
		return []string{"expected YAML frontmatter at the start of the file"}
	}
	problems := []string{}
	switch {
	case metadata.Name == "":
		problems = append(problems, "name is required, and must be the folder's name, "+folder)
	case metadata.Name != folder:
		problems = append(problems, fmt.Sprintf("name is %s, but the folder is %s; they must match", decode.Quote(metadata.Name), folder))
	}
	if metadata.Type != "" && !slices.Contains(Types, metadata.Type) {
		problems = append(problems, fmt.Sprintf("%s is %s; use %s, or leave it out", TypeKey, decode.Quote(metadata.Type), strings.Join(Types, " or ")))
	}
	for _, key := range metadata.ReservedKeys {
		problems = append(problems, fmt.Sprintf("metadata.%s isn't a Trail Guide key; use %s, %s, or %s", decode.Quote(key), TypeKey, RulesUsedKey, SkillsUsedKey))
	}
	problems = append(problems, checkLines(RulesUsedKey, metadata.RulesUsed, qualified, "", ruleProblem)...)
	return append(problems, checkLines(SkillsUsedKey, metadata.SkillsUsed, qualified, AuthoredID(folder), skillProblem)...)
}

// checkLines returns the problems of the lines of key: each line's own problem, as problem reports it, then a
// repeated ID, and a line that names self, the ID of the skill being checked, when self isn't empty.
func checkLines(key string, lines []string, qualified bool, self string, problem func(Citation, bool) string) []string {
	problems := []string{}
	seen := []string{}
	for _, line := range lines {
		citation := ParseCitation(line)
		switch text := problem(citation, qualified); {
		case text != "":
			problems = append(problems, fmt.Sprintf("%s names %s; %s", key, decode.Quote(line), text))
		case slices.Contains(seen, line):
			problems = append(problems, fmt.Sprintf("%s names %s twice", key, line))
		case self != "" && citation.Source == "" && citation.ID == self:
			problems = append(problems, key+" names the skill itself")
		}
		seen = append(seen, line)
	}
	return problems
}

// ValidRuleCitation reports whether line, a line of trailguide-rules-used, has the form of a rule ID, qualified by a source
// name only when qualified allows it, so that whether it names a rule can be checked.
func ValidRuleCitation(line string, qualified bool) bool {
	return ruleProblem(ParseCitation(line), qualified) == ""
}

// ValidSkillCitation reports whether line, a line of trailguide-skills-used, has the form of a skill ID: skills/ and a
// skill name, or a source name, a colon, and the skill's ID in that source. It doesn't check that the skill exists.
func ValidSkillCitation(line string) bool {
	return skillProblem(ParseCitation(line), true) == ""
}

// ruleProblem returns what is wrong with the form of a cited rule, or empty when it is a rule ID, qualified by a
// source name only when qualified allows it.
func ruleProblem(citation Citation, qualified bool) string {
	example := "practices/testing/verify-retries"
	if citation.Source != "" && !qualified {
		return "cite a rule by its ID in this library, without a source name, such as " + example
	}
	if citation.Source != "" && !sourceName(citation.Source) {
		return "expected a source name, a colon, and a rule ID, such as fabrica:" + example
	}
	if trimmed, found := strings.CutSuffix(citation.ID, ".md"); found && librarytree.ValidateRuleID(trimmed, "") == nil {
		return "cite a rule by its ID, without .md, such as " + trimmed
	}
	if librarytree.ValidateRuleID(citation.ID, "") != nil {
		return "expected a rule ID, a rule's path in rules/ without .md, such as " + example
	}
	return ""
}

// skillProblem returns what is wrong with the form of a skill a skill uses, or empty when it is a skill ID: skills/
// and a skill name for a skill in the same repository, or a source name, a colon, and the skill's ID in that source.
// It takes qualified, which it ignores, so that checkLines can check skill and rule lines alike.
func skillProblem(citation Citation, _ bool) string {
	switch {
	case citation.Source != "" && (!sourceName(citation.Source) || ValidateID(citation.ID, "") != nil):
		return "expected a source name, a colon, and the skill's ID in that source, such as fabrica:skills/tdd"
	case citation.Source == "":
		if name, found := strings.CutPrefix(citation.ID, Directory+"/"); !found || !ValidName(name) {
			return "expected skills/ and a skill's name, such as skills/tdd"
		}
	}
	return ""
}

// sourceName reports whether text has the form of a source name: a lowercase letter, then lowercase letters,
// digits, and hyphens. Configuration validates source names; this only keeps a malformed line from reading as a
// citation.
func sourceName(text string) bool {
	if text == "" || text[0] < 'a' || text[0] > 'z' {
		return false
	}
	for _, r := range text {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
