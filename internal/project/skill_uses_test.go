// Test that sync installs the skills an included skill uses, from the same source, and that build and check keep the
// recorded skills in step with trailguide-skills-used and skills.uses.

package project

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/imports"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// usingSkill returns a SKILL.md for skill name whose trailguide-skills-used lists used.
func usingSkill(name string, used ...string) []byte {
	text := "---\nname: " + name + "\ndescription: Use " + name + ".\n"
	if len(used) > 0 {
		text += "metadata:\n  trailguide-skills-used: |\n    " + strings.Join(used, "\n    ") + "\n"
	}
	return []byte(text + "---\n# " + name + "\n")
}

// usingRepository returns a repository that isn't a library, whose skills are files, by repository path.
func usingRepository(t *testing.T, files map[string][]byte) *gitfixture.Fixture {
	t.Helper()
	f, err := gitfixture.New(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// setConfiguration replaces the project's configuration with config, JSON with the fixture's repository in place of
// REPOSITORY, given the newest library release's ref when it has none.
func setConfiguration(t *testing.T, options Options, f *gitfixture.Fixture, config string) {
	t.Helper()
	root, err := openProject(context.Background(), options, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	config = withNewestRef(t, config, "REPOSITORY", f.Repository, imports.Options{GitPath: f.GitPath, Environment: f.Environment})
	writeFixture(t, root, configurationFile, strings.ReplaceAll(config, "REPOSITORY", quoteJSON(f.Repository)))
}

func TestSync_InstallsEverySkillAnIncludedSkillUsesFromTheSameSource(t *testing.T) {
	ctx := context.Background()
	f := usingRepository(t, map[string][]byte{
		"skills/poteto-mode/SKILL.md": usingSkill("poteto-mode", "skills/architect"),
		"skills/architect/SKILL.md":   usingSkill("architect", "skills/how"),
		"skills/how/SKILL.md":         usingSkill("how"),
		"skills/tdd/SKILL.md":         usingSkill("tdd"),
		"skills/unused/SKILL.md":      usingSkill("unused"),
	})
	const withUses = `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"pstack":{"repository":REPOSITORY,"ref":"v1.0.0","skills":{"include":["skills/poteto-mode"],"uses":{"skills/architect":["skills/tdd"]}}}}}`
	options, git := skillProject(t, f, withUses)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"poteto-mode", "architect", "how", "tdd"} {
		if !exists(t, options, ".claude/skills/"+name+"/SKILL.md") {
			t.Errorf("sync didn't install %s", name)
		}
	}
	if exists(t, options, ".claude/skills/unused") {
		t.Error("sync installed a skill nothing uses")
	}
	record := entryText(t, options, "pstack")
	for _, want := range []string{
		`"skills/architect": {"included": false, "level": null, "usedBy": ["skills/poteto-mode"]}`,
		`"skills/poteto-mode": {"included": true, "level": null, "usedBy": []}`,
		`"skills/tdd": {"included": false, "level": null, "usedBy": ["skills/architect"]}`,
	} {
		if !strings.Contains(record, want) {
			t.Errorf("lock entry lacks\n%s\nin\n%s", want, record)
		}
	}
	index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md")
	for _, want := range []string{
		"- **Uses:** `how`, `tdd`\n- **Installed because:** used by `poteto-mode`\n",
		"- **Uses:** `architect`\n- **Installed because:** included\n",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("SKILLS.md lacks\n%s\nin\n%s", want, index)
		}
	}
	if result, err := Check(ctx, options); err != nil || !result.Current() {
		t.Fatalf("check = %+v, %v", result, err)
	}

	// Removing the skills.uses entry leaves a record that installs a skill nothing uses: build and check ask for sync,
	// which removes it.
	setConfiguration(t, options, f, strings.Replace(withUses, `,"uses":{"skills/architect":["skills/tdd"]}`, "", 1))
	_, err := Build(ctx, options)
	var validation errs.ValidationError
	if !errors.As(err, &validation) || validation.ValidationLocation() != "lock.json.sources.pstack" || !strings.Contains(validation.ValidationProblem(), "run trailguide project sync") {
		t.Fatalf("build = %v; want the record reported as out of date", err)
	}
	if _, err := Check(ctx, options); !errors.As(err, &validation) || validation.ValidationLocation() != "lock.json.sources.pstack" {
		t.Fatalf("check = %v; want the record reported as out of date", err)
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if exists(t, options, ".claude/skills/tdd") || !exists(t, options, ".claude/skills/how") {
		t.Fatal("sync didn't remove the skill only skills.uses brought in")
	}

	// A skills.uses entry offline checks can't confirm against the record also asks for sync.
	setConfiguration(t, options, f, strings.Replace(withUses, `"skills/architect":["skills/tdd"]`, `"skills/how":["skills/unused"]`, 1))
	if _, err := Check(ctx, options); !errors.As(err, &validation) || !strings.Contains(validation.ValidationProblem(), "run trailguide project sync") {
		t.Fatalf("check = %v; want sync asked for", err)
	}
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if !exists(t, options, ".claude/skills/unused") {
		t.Fatal("sync didn't install the skill skills.uses adds")
	}
}

func TestSync_InstallsTheSkillsALibrarySkillUses(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`),
		"techs/go/_group.yaml": []byte(projectMetadata),
		"techs/go/errors.md":   []byte(projectRule),
		".trailguide/skills/library/fix-bug/SKILL.md":      usingSkill("fix-bug", "skills/review-tests"),
		".trailguide/skills/library/review-tests/SKILL.md": usingSkill("review-tests"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, firstRelease); err != nil {
		t.Fatal(err)
	}
	options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["agents"],"sources":{"team":{"repository":REPOSITORY,"skills":{"include":["skills/fix-bug"]}}}}`)
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if !exists(t, options, ".agents/skills/review-tests/SKILL.md") {
		t.Fatal("sync didn't install the skill fix-bug uses")
	}
	if index := readRepositoryFile(t, options, ".trailguide/_managed/generated/SKILLS.md"); !strings.Contains(index, "- **Level:** offered by `team`, release 1\n- **Installed because:** used by `fix-bug`\n") {
		t.Fatalf("SKILLS.md:\n%s", index)
	}
}

func TestSync_FailsWithoutWritingWhenAUsedSkillCannotBeFound(t *testing.T) {
	ctx := context.Background()
	f := usingRepository(t, map[string][]byte{
		"skills/poteto-mode/SKILL.md": usingSkill("poteto-mode", "skills/architect"),
		"skills/architect/SKILL.md":   usingSkill("architect", "skills/how"),
		"skills/loop/SKILL.md":        usingSkill("loop", "skills/back"),
		"skills/back/SKILL.md":        usingSkill("back", "skills/loop"),
	})
	tests := map[string]struct {
		skills   string
		code     string
		location string
		problem  string
	}{
		"a use its metadata declares": {
			`{"include":["skills/poteto-mode"]}`, "", "sources.pstack.skills",
			"skills/architect uses skills/how, which isn't a skill in commit " + f.FirstCommit + ": skills/architect names it in trailguide-skills-used. Trail Guide installs every skill an installed skill uses, so it can't install skills/architect, which skills/poteto-mode uses. Ask the source's maintainer to correct trailguide-skills-used, or stop including skills/poteto-mode",
		},
		"a use skills.uses declares": {
			`{"include":["skills/loop"],"uses":{"skills/loop":["skills/gone"]}}`, "", "sources.pstack.skills.uses.skills/loop",
			"the source has no skill skills/gone at commit " + f.FirstCommit + "; check the skill ID",
		},
		"skills that use each other": {
			`{"include":["skills/loop"]}`, "", "sources.pstack.skills",
			"the skills the source installs use each other in a cycle: skills/loop -> skills/back -> skills/loop",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			options, git := skillProject(t, f, `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"pstack":{"repository":REPOSITORY,"ref":"v1.0.0","skills":`+test.skills+`}}}`)
			_, err := Sync(ctx, options, git)
			var validation errs.ValidationError
			if !errors.As(err, &validation) || validation.ValidationLocation() != test.location || !strings.Contains(validation.ValidationProblem(), test.problem) {
				t.Fatalf("got %v\nwant %s: %s", err, test.location, test.problem)
			}
			if exists(t, options, ".claude") || exists(t, options, ".trailguide/_managed/vendor/pstack") {
				t.Fatal("failed sync wrote files")
			}
		})
	}
}

// TestSync_FailsWhenSkillsUsesNamesASkillTheLockedRevisionLacks shows that sync never moves a source for a skill
// that skills.uses adds, even when a newer commit has it.
func TestSync_FailsWhenSkillsUsesNamesASkillTheLockedRevisionLacks(t *testing.T) {
	ctx := context.Background()
	f := usingRepository(t, map[string][]byte{"skills/architect/SKILL.md": usingSkill("architect")})
	const config = `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"pstack":{"repository":REPOSITORY,"ref":"v1.0.0","skills":{"include":["skills/architect"]USES}}}}`
	options, git := skillProject(t, f, strings.Replace(config, "USES", "", 1))
	if _, err := Sync(ctx, options, git); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Add a skill", map[string][]byte{"skills/how/SKILL.md": usingSkill("how")}); err != nil {
		t.Fatal(err)
	}
	setConfiguration(t, options, f, strings.Replace(config, "USES", `,"uses":{"skills/architect":["skills/how"]}`, 1))
	_, err := Sync(ctx, options, git)
	var failure *imports.Error
	if !errors.As(err, &failure) || failure.Code != "skill-not-in-revision" || !strings.Contains(err.Error(), "sources.pstack.skills.uses.skills/architect names skills/how, which isn't in commit "+f.FirstCommit) || !strings.Contains(err.Error(), "If sources.pstack.ref names a branch, run trailguide project update pstack") || !strings.Contains(err.Error(), "Otherwise, set sources.pstack.ref to a tag or commit that has it") {
		t.Fatalf("got %v; want skill-not-in-revision", err)
	}
	if exists(t, options, ".claude/skills/how") {
		t.Fatal("refused sync installed the skill")
	}
}
