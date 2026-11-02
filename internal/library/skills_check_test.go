// Test that library check reports every problem in the name and Trail Guide metadata keys of the library's skills.

package library

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/skills"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
	"github.com/fabricahq/trailguide/trailguide"
)

func TestSkillProblemsReportsEachInvalidSkill(t *testing.T) {
	current := []string{"practices/testing/a", "practices/testing/c"}
	changes := libraryChanges{
		history: releaseHistory{retired: map[string]int{"practices/testing/old": 2, "practices/testing/b": 3}, replacedBy: map[string]string{"practices/testing/b": "practices/testing/a"}},
		pending: []pendingNote{{path: "changes/retire.yaml", note: rules.ChangeNote{Rules: map[string]rules.NoteChange{"practices/testing/d": {Change: trailguide.ChangeRetired, ReplacedBy: "practices/testing/c"}}}}},
	}
	skill := func(metadata string) string {
		return "---\nname: fix\ndescription: Fix a bug.\nmetadata:\n" + metadata + "---\n# Fix\n"
	}
	tests := map[string]struct {
		files map[string]string
		want  []string
	}{
		"valid skills": {map[string]string{
			"fix/SKILL.md":    skill("  trailguide-skill-type: playbook\n  trailguide-rules-used: |\n    practices/testing/a\n    practices/testing/c\n  trailguide-skills-used: skills/review\n"),
			"review/SKILL.md": "---\nname: review\nmetadata:\n  trailguide-skill-type: review\n---\n",
		}, []string{}},
		"unreadable frontmatter": {map[string]string{"fix/SKILL.md": "---\nname: fix\n"}, []string{"skills/library/fix/SKILL.md: the frontmatter that starts on the first line has no closing --- line"}},
		"unknown type and name": {map[string]string{"fix/SKILL.md": "---\nname: fixer\nmetadata:\n  trailguide-skill-type: design\n---\n"}, []string{
			`skills/library/fix/SKILL.md: name is "fixer", but the folder is fix; they must match`,
			`skills/library/fix/SKILL.md: trailguide-skill-type is "design"; use playbook or review, or leave it out`,
		}},
		"cited rules that aren't current": {map[string]string{"fix/SKILL.md": skill("  trailguide-rules-used: |\n    practices/testing/missing\n    practices/testing/b\n    practices/testing/old\n    practices/testing/d\n    fabrica:practices/testing/a\n")}, []string{
			`skills/library/fix/SKILL.md: trailguide-rules-used names "fabrica:practices/testing/a"; cite a rule by its ID in this library, without a source name, such as practices/testing/verify-retries`,
			"skills/library/fix/SKILL.md: trailguide-rules-used names practices/testing/missing, which isn't a rule in this library",
			"skills/library/fix/SKILL.md: trailguide-rules-used names practices/testing/b, which is retired and replaced by practices/testing/a; cite the replacement",
			"skills/library/fix/SKILL.md: trailguide-rules-used names practices/testing/old, which is retired; remove it",
			"skills/library/fix/SKILL.md: trailguide-rules-used names practices/testing/d, which is retired and replaced by practices/testing/c; cite the replacement",
		}},
		"skills it uses": {map[string]string{"fix/SKILL.md": skill("  trailguide-skills-used: |\n    skills/fix\n    anthropic:skills/tdd\n    mattpocock:skills/tdd\n    anthropic:skills/skill-creator\n    skills/skill-creator\n")}, []string{
			"skills/library/fix/SKILL.md: trailguide-skills-used names the skill itself",
			"skills/library/fix/SKILL.md: trailguide-skills-used names anthropic:skills/tdd, which isn't a skill the library curates; list it in library.skills, at a level or as a dependency, so projects that install the skill can install it too",
			"skills/library/fix/SKILL.md: trailguide-skills-used names mattpocock:skills/tdd, but library.skills lists no skill from a source named mattpocock; the library curates skills from anthropic. Correct the source name, or name a skill of this library as skills/<name>",
			"skills/library/fix/SKILL.md: trailguide-skills-used names skills/skill-creator, which isn't a skill in this library; the library curates anthropic:skills/skill-creator, so name it that way",
		}},
		"skills it uses that the library doesn't have": {map[string]string{
			"fix/SKILL.md":    skill("  trailguide-skills-used: |\n    skills/review\n    skills/tdd\n    skills/notes\n"),
			"review/SKILL.md": "---\nname: review\n---\n",
		}, []string{
			"skills/library/fix/SKILL.md: trailguide-skills-used names skills/tdd, which isn't a skill in this library",
			"skills/library/fix/SKILL.md: trailguide-skills-used names skills/notes, which isn't a skill in this library",
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			files := map[string][]byte{}
			for file, text := range test.files {
				files["library/"+file] = []byte(text)
			}
			found, problems := skills.Read(files, nil, nil, skills.Origins{Own: "library"})
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			if got := skillProblems(found, current, []string{"anthropic:skills/skill-creator"}, changes); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("problems:\n%q\nwant\n%q", got, test.want)
			}
		})
	}
	if got := skillProblems(nil, current, nil, changes); len(got) != 0 {
		t.Fatalf("a library without skills/ has problems: %q", got)
	}
}

func TestCheck_RejectsASkillThatCitesARuleTheLibraryRetired(t *testing.T) {
	ctx := context.Background()
	fixture, options := authorClone(t, libraryFiles(), releaseOne)
	edit(t, options.Directory, map[string]string{".trailguide/skills/library/fix/SKILL.md": "---\nname: fix\nmetadata:\n  trailguide-skill-type: playbook\n  trailguide-rules-used: |\n    practices/testing/a\n    practices/testing/b\n---\n# Fix\n", "changes/skills.yaml": "summary: Add fix.\nskills:\n  skills/fix: new\n"})
	if _, err := Check(ctx, options); err != nil {
		t.Fatalf("check rejected a valid skill: %v", err)
	}
	if _, err := fixture.Commit(ctx, options.Directory, "Retire b", gitfixture.Library(map[string][]byte{"practices/testing/b.md": nil, "changes/two.yaml": []byte("summary: Fold b into a.\nrules:\n  practices/testing/b: {change: retired, replacedBy: practices/testing/a}\n")})); err != nil {
		t.Fatal(err)
	}
	_, err := Check(ctx, options)
	if errorCode(err) != "invalid-skill" || !strings.HasSuffix(err.Error(), "skills don't pass library check:\n  - skills/library/fix/SKILL.md: trailguide-rules-used names practices/testing/b, which is retired and replaced by practices/testing/a; cite the replacement") {
		t.Fatalf("got %v; want invalid-skill naming the retired rule", err)
	}
}

func TestCheck_RejectsASkillThatUsesASkillTheLibraryDoesntHave(t *testing.T) {
	ctx := context.Background()
	_, options := authorClone(t, libraryFiles(), releaseOne)
	edit(t, options.Directory, map[string]string{
		".trailguide/skills/library/fix/SKILL.md":    "---\nname: fix\nmetadata:\n  trailguide-skills-used: |\n    skills/review\n    skills/tdd\n---\n# Fix\n",
		".trailguide/skills/library/review/SKILL.md": "---\nname: review\n---\n# Review\n",
		"changes/skills.yaml":                        "summary: Add fix and review.\nskills:\n  skills/fix: new\n  skills/review: new\n",
	})
	_, err := Check(ctx, options)
	if errorCode(err) != "invalid-skill" || !strings.HasSuffix(err.Error(), "skills don't pass library check:\n  - skills/library/fix/SKILL.md: trailguide-skills-used names skills/tdd, which isn't a skill in this library") {
		t.Fatalf("got %v; want invalid-skill naming the skill and the skill it uses", err)
	}
	edit(t, options.Directory, map[string]string{".trailguide/skills/library/tdd/SKILL.md": "---\nname: tdd\n---\n# TDD\n", "changes/tdd.yaml": "summary: Add tdd.\nskills:\n  skills/tdd: new\n"})
	if _, err := Check(ctx, options); err != nil {
		t.Fatalf("check rejected a skill that uses skills the library has: %v", err)
	}
}

func TestUncuratedProblemWithoutCuratedSkills(t *testing.T) {
	got := uncuratedProblem(skills.ParseCitation("anthropic:skills/skill-creator"), nil)
	if want := "but library.skills lists no curated skills; name a skill of this library as skills/<name>, or list the curated skill in library.skills"; got != want {
		t.Fatalf("uncuratedProblem = %q; want %q", got, want)
	}
}

func TestCheck_WarnsOnlyAboutACodexInterfaceFileTheMetadataCanReplace(t *testing.T) {
	ctx := context.Background()
	_, options := authorClone(t, libraryFiles(), releaseOne)
	edit(t, options.Directory, map[string]string{
		".trailguide/skills/library/fix/SKILL.md":               "---\nname: fix\n---\n# Fix\n",
		".trailguide/skills/library/fix/agents/openai.yaml":     "interface:\n  display_name: Fix\n  default_prompt: Use $fix.\n",
		".trailguide/skills/library/fix/references/openai.yaml": "interface:\n  display_name: Not the interface file\n",
		".trailguide/skills/library/quiet/SKILL.md":             "---\nname: quiet\n---\n# Quiet\n",
		".trailguide/skills/library/quiet/agents/openai.yaml":   "interface:\n  display_name: Quiet\npolicy:\n  allow_implicit_invocation: false\n",
		".trailguide/skills/library/brand/SKILL.md":             "---\nname: brand\n---\n# Brand\n",
		".trailguide/skills/library/brand/agents/openai.yaml":   "interface:\n  display_name: Brand\n  icon_small: ./assets/icon.svg\n",
		"changes/skills.yaml":                                   "summary: Add skills.\nskills:\n  skills/brand: new\n  skills/fix: new\n  skills/quiet: new\n",
	})
	result, err := Check(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	want := "skills/library/fix/agents/openai.yaml: it sets only interface text, which Trail Guide generates for each project from SKILL.md. Set metadata.display-name, short-description, and default-prompt in SKILL.md instead, then delete this file."
	found := []string{}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "openai.yaml") {
			found = append(found, warning)
		}
	}
	if !reflect.DeepEqual(found, []string{want}) {
		t.Fatalf("warnings = %q; want only %q", found, want)
	}
}
