// Test that check reports installed skill folders that differ from their source, and authored skills that cite a
// rule their library retired, and that build links each installed skill to the rules it cites.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// skillProblems returns the problems of checked that are about installed skill folders.
func skillProblems(checked CheckResult) []Problem {
	problems := []Problem{}
	for _, problem := range checked.Problems {
		if problem.Skill != "" {
			problems = append(problems, problem)
		}
	}
	return problems
}

func TestCheck_ReportsEachInstalledSkillFolderThatDiffersFromItsSource(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["*"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/check-release/SKILL.md", "---\nname: check-release\ndescription: Check a release.\n---\n# Check\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
	repository := func(name string) string { return filepath.Join(options.Directory, filepath.FromSlash(name)) }
	edited := repository(".claude/skills/build-feature/SKILL.md")
	if err := os.WriteFile(edited, []byte(readRepositoryFile(t, options, ".claude/skills/build-feature/SKILL.md")+"An edit.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(repository(".agents/skills/build-feature/scripts/run.sh"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repository(".agents/skills/review")); err != nil {
		t.Fatal(err)
	}
	if err := root.RemoveAll("skills/check-release"); err != nil {
		t.Fatal(err)
	}
	before, err := filetxn.ReadTree(ctx, root, ".")
	if err != nil {
		t.Fatal(err)
	}
	checked, err := Check(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	want := []Problem{
		{Kind: ModifiedSkill, Path: "../.agents/skills/build-feature", Repair: Rebuild, Skill: "team:skills/build-feature"},
		{Kind: UnexpectedSkill, Path: "../.agents/skills/check-release", Repair: Rebuild, Skill: "skills/check-release"},
		{Kind: MissingSkill, Path: "../.agents/skills/review", Repair: Rebuild, Skill: "team:skills/review"},
		{Kind: ModifiedSkill, Path: "../.claude/skills/build-feature", Repair: Rebuild, Skill: "team:skills/build-feature"},
		{Kind: UnexpectedSkill, Path: "../.claude/skills/check-release", Repair: Rebuild, Skill: "skills/check-release"},
	}
	if got := skillProblems(checked); !reflect.DeepEqual(got, want) {
		t.Fatalf("skill problems:\n%+v\nwant\n%+v", got, want)
	}
	after, err := filetxn.ReadTree(ctx, root, ".")
	if err != nil || before.Digest() != after.Digest() || !strings.HasSuffix(readRepositoryFile(t, options, ".claude/skills/build-feature/SKILL.md"), "An edit.\n") {
		t.Fatalf("check changed files: %v", err)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
}

func TestCheck_ReportsAGeneratedCodexInterfaceFileThatDrifted(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude","agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["*"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/add-skill/SKILL.md", "---\nname: add-skill\ndescription: Add a skill.\nmetadata:\n  display-name: Add Skill\n---\n# Add\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
	generated := filepath.Join(options.Directory, ".agents/skills/add-skill/agents/openai.yaml")
	if data, err := os.ReadFile(generated); err != nil || !strings.Contains(string(data), "display_name: \"Add Skill\"") {
		t.Fatalf("generated file = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(options.Directory, ".claude/skills/add-skill/agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".claude/skills/add-skill has an agents folder: %v", err)
	}
	if err := os.WriteFile(generated, []byte("interface:\n  display_name: Edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checked, err := Check(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	want := []Problem{{Kind: ModifiedSkill, Path: "../.agents/skills/add-skill", Repair: Rebuild, Skill: "skills/add-skill"}}
	if got := skillProblems(checked); !reflect.DeepEqual(got, want) {
		t.Fatalf("skill problems:\n%+v\nwant\n%+v", got, want)
	}
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
}

func TestBuild_LinksAnAuthoredSkillToTheAuthoredRuleItCites(t *testing.T) {
	ctx := context.Background()
	root, options := localProject(t)
	writeFixture(t, root, "config.yaml", `{"schemaVersion":2,"skillTargets":["claude"],"sources":{}}`)
	writeFixture(t, root, "skills/tidy/SKILL.md", "---\nname: tidy\nmetadata:\n  trailguide-rules-used: techs/go/errors\n---\n# Tidy\n")
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, options, ".claude/skills/tidy/SKILL.md"); !strings.HasSuffix(installed, "- `local:techs/go/errors`: [.trailguide/_managed/generated/rules/local/techs/go/errors.md](../../../.trailguide/_managed/generated/rules/local/techs/go/errors.md)\n") {
		t.Fatalf("installed SKILL.md:\n%s", installed)
	}
	requireCurrent(t, options)
}

// retiringLibrary returns a library whose release/1 publishes techs/go/errors, and a function that publishes
// release/2, which retires it.
func retiringLibrary(t *testing.T) (*gitfixture.Fixture, func()) {
	t.Helper()
	ctx := context.Background()
	f, _ := skillLibrary(t)
	return f, func() {
		t.Helper()
		if _, err := f.Commit(ctx, f.Worktree(), "Retire errors", gitfixture.Library(map[string][]byte{"techs/go/errors.md": nil})); err != nil {
			t.Fatal(err)
		}
		if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules: {}\nretired:\n  techs/go/errors: {lastVersion: 1.0.0, summaries: [No longer recommended.]}\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheck_FailsWhenAnAuthoredSkillCitesARuleItsLibraryRetired(t *testing.T) {
	ctx := context.Background()
	f, retire := retiringLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"rules":{"include":["techs/go/*"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFixture(t, root, "skills/fix/SKILL.md", "---\nname: fix\ndescription: Fix a bug.\nmetadata:\n  trailguide-rules-used: |\n    team:techs/go/errors\n---\n# Fix\n")
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	installed := readRepositoryFile(t, options, ".claude/skills/fix/SKILL.md")
	if want := "\n## Rules this skill applies\n\nRead each rule in full before the step that applies it. These are the project's versions of the rules this skill cites, for reference: they don't join RULES.md.\n\n- `team:techs/go/errors` 1.0.0: [.trailguide/_managed/generated/rules/team/techs/go/errors.md](../../../.trailguide/_managed/generated/rules/team/techs/go/errors.md)\n"; !strings.HasSuffix(installed, want) {
		t.Fatalf("installed SKILL.md:\n%s\nwant it to end with\n%s", installed, want)
	}
	if _, err := os.Stat(filepath.Join(options.Directory, ".trailguide/_managed/generated/rules/team/techs/go/errors.md")); err != nil {
		t.Fatalf("the cited rule's link has no target: %v", err)
	}
	requireCurrent(t, options)

	retire()
	plan, err := PlanUpdate(ctx, options, git, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	const problem = "trailguide-rules-used names team:techs/go/errors, which its library retired; remove it from trailguide-rules-used, or cite the rule that replaces it"
	applied, err := plan.Apply(ctx, nil)
	if err != nil || !slices.Contains(applied.Warnings, "skills/fix/SKILL.md: "+problem) {
		t.Fatalf("update = %+v, %v; want a warning that the skill cites a retired rule", applied, err)
	}
	if installed := readRepositoryFile(t, options, ".claude/skills/fix/SKILL.md"); !strings.Contains(installed, "- `team:techs/go/errors`: retired by its library") {
		t.Fatalf("installed SKILL.md:\n%s", installed)
	}
	_, err = Check(ctx, options)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "skills/fix/SKILL.md" || validation.ValidationProblem() != problem {
		t.Fatalf("check = %v; want the retired rule named at skills/fix/SKILL.md", err)
	}
	changes, err := Build(ctx, options)
	if err != nil || !reflect.DeepEqual(changes.Warnings, []string{"skills/fix/SKILL.md: " + problem}) {
		t.Fatalf("build = %+v, %v", changes, err)
	}
	writeFixture(t, root, "skills/fix/SKILL.md", "---\nname: fix\ndescription: Fix a bug.\n---\n# Fix\n")
	if _, err := Build(ctx, options); err != nil {
		t.Fatal(err)
	}
	requireCurrent(t, options)
}

// TestSync_RejectsAnAuthoredSkillCitingARuleItsSourceDoesntHave fails sync, build, and check at an authored skill's
// SKILL.md when it cites a rule, such as a misspelled one, that isn't in a group its source includes in full, and
// lists a rule in a group the source doesn't include as not included.
func TestSync_RejectsAnAuthoredSkillCitingARuleItsSourceDoesntHave(t *testing.T) {
	ctx := context.Background()
	f, _ := skillLibrary(t)
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":REPOSITORY,"rules":{"include":["techs/go/*"]}}}}`)
	root, err := openProject(ctx, options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	citing := func(cited string) string {
		return "---\nname: fix\ndescription: Fix a bug.\nmetadata:\n  trailguide-rules-used: |\n    team:techs/go/" + cited + "\n    team:practices/testing/verify\n---\n# Fix\n"
	}
	const problem = "trailguide-rules-used names team:techs/go/error, which isn't a rule in team's library; correct the rule ID, or remove it from trailguide-rules-used"
	rejected := func(operation string, err error) {
		t.Helper()
		var validation errs.ValidationError
		if !errors.As(err, &validation) || validation.ValidationLocation() != "skills/fix/SKILL.md" || validation.ValidationProblem() != problem {
			t.Fatalf("%s = %v; want the misspelled rule named at skills/fix/SKILL.md", operation, err)
		}
	}
	writeFixture(t, root, "skills/fix/SKILL.md", citing("error"))
	_, err = Sync(ctx, options, git)
	rejected("sync", err)
	if exists(t, options, ".claude/skills/fix") {
		t.Fatal("sync installed the skill it rejected")
	}

	writeFixture(t, root, "skills/fix/SKILL.md", citing("errors"))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if installed := readRepositoryFile(t, options, ".claude/skills/fix/SKILL.md"); !strings.HasSuffix(installed, "- `team:techs/go/errors` 1.0.0: [.trailguide/_managed/generated/rules/team/techs/go/errors.md](../../../.trailguide/_managed/generated/rules/team/techs/go/errors.md)\n- `team:practices/testing/verify`: not included by this project, so it has no version of it\n") {
		t.Fatalf("installed SKILL.md:\n%s", installed)
	}
	requireCurrent(t, options)

	writeFixture(t, root, "skills/fix/SKILL.md", citing("error"))
	_, err = Build(ctx, options)
	rejected("build", err)
	_, err = Check(ctx, options)
	rejected("check", err)
}
