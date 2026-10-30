// Exercise forking a source's skill through the compiled CLI: output, JSON, usage errors, the build that installs the
// fork in the skill's place, and the update preview that lists the source's later changes to the skill.

package cli

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// forkedSkill is the SKILL.md of the fixture library's build-feature skill at release/1.
const forkedSkill = "---\nname: build-feature\ndescription: Build a feature.\n---\n# Build a feature\n\n1. Run scripts/run.sh.\n"

// newSkillForkFixture syncs a project that installs every skill of a library whose release/1 publishes
// build-feature, with an executable script, and returns the fixture and release/1's commit.
func newSkillForkFixture(t *testing.T) (updateFixture, string) {
	t.Helper()
	return newSkillForkFixtureWith(t, nil)
}

// newSkillForkFixtureWith syncs a project as newSkillForkFixture does, from a library that also publishes extra, its
// files by path in the library's repository.
func newSkillForkFixtureWith(t *testing.T, extra map[string][]byte) (updateFixture, string) {
	t.Helper()
	ctx := context.Background()
	files := map[string][]byte{
		"config.yaml":                                     []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`),
		"techs/go/_group.yaml":                            []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":                              updateRule("Wrap errors."),
		".trailguide/skills/build-feature/SKILL.md":       []byte(forkedSkill),
		".trailguide/skills/build-feature/scripts/run.sh": []byte("#!/bin/sh\necho build\n"),
	}
	maps.Copy(files, extra)
	f, err := gitfixture.New(ctx, gitfixture.Library(files))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := os.Chmod(filepath.Join(f.Worktree(), ".trailguide/skills/build-feature/scripts/run.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	commit, err := f.Commit(ctx, f.Worktree(), "Make the script executable", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init", "--skill-targets", "claude"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: "+f.Repository+"\n    ref: release/1\n    skills:\n      include: ['*']\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	return u, commit
}

// TestForkSkill_InstallsTheForkInPlaceOfTheSkill forks a skill, reports what it wrote and what to run next, and the
// next build installs the fork; after the source changes the skill, update lists the changes without changing the
// fork.
func TestForkSkill_InstallsTheForkInPlaceOfTheSkill(t *testing.T) {
	u, commit := newSkillForkFixture(t)
	out, diagnostic, code := u.run(t, "project", "add", "skill", "--from", "team:skills/build-feature", "--reason", "We build with make.", "--non-interactive")
	want := "Skill forked from team:skills/build-feature at commit " + commit[:12] + ".\nAdded:\n  .trailguide/skills/build-feature/SKILL.md\n  .trailguide/skills/build-feature/scripts/run.sh\nChanged:\n  .trailguide/config.yaml\n\n" +
		"Next: Edit the forked skill to change its steps. A fork has no version: it changes\nonly when you edit it. The source's license still applies to the copied files.\n" +
		"config.yaml now forks team:skills/build-feature as skills/build-feature, based on commit " + commit[:12] + ";\ntrailguide project update lists the source's later changes to it.\n\n" +
		"Then install the fork in place of the source's skill:\n  trailguide project build\n  trailguide project check\n"
	if code != 0 || diagnostic != "" || out != want {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, diagnostic, out, want)
	}
	if info, err := os.Stat(filepath.Join(u.directory, ".trailguide/skills/build-feature/scripts/run.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the fork's script isn't executable: %v %v", info, err)
	}
	if out, diagnostic, code := u.run(t, "project", "build"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	installed, err := os.ReadFile(filepath.Join(u.directory, ".claude/skills/build-feature/SKILL.md"))
	if err != nil || !strings.Contains(string(installed), "edit .trailguide/skills/build-feature/ instead") {
		t.Fatalf("installed copy, %v:\n%s", err, installed)
	}
	if out, diagnostic, code := u.run(t, "project", "check"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}

	ctx := context.Background()
	later, err := u.fixture.Commit(ctx, u.fixture.Worktree(), "Change the skill", map[string][]byte{".trailguide/skills/build-feature/SKILL.md": []byte(strings.Replace(forkedSkill, "Run scripts/run.sh.", "Run make.", 1))})
	if err != nil {
		t.Fatal(err)
	}
	if err := u.fixture.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code = u.run(t, "project", "update")
	row := "  replaced  skills/build-feature  commit " + commit[:12] + " -> " + later[:12] + "\n" +
		"            Your skill: skills/build-feature, based on commit " + commit[:12] + ".\n" +
		"            Changed since commit " + commit[:12] + ", the copy your skill is based on:\n" +
		"              SKILL.md\n" +
		"            Merging them into your skill finds no text conflicts with your edits. To merge\n" +
		"            them and set basedOn to commit " + later[:12] + ", pass --merge-fork team:skills/build-feature.\n"
	if code != 0 || diagnostic != "" || !strings.Contains(out, row) {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant the row:\n%s", code, diagnostic, out, row)
	}
	out, _, code = u.run(t, "project", "update", "--json")
	var preview struct {
		OK    bool
		Value struct {
			Sources []struct {
				ReplacedSkills []map[string]any `json:"replacedSkills"`
			}
		}
	}
	wantRow := map[string]any{"id": "skills/build-feature", "localSkill": "skills/build-feature", "basedOn": commit, "from": commit, "to": later, "files": []any{"SKILL.md"}, "merge": "no-conflicts"}
	if err := json.Unmarshal([]byte(out), &preview); err != nil || code != 0 || len(preview.Value.Sources) != 1 || !reflect.DeepEqual(preview.Value.Sources[0].ReplacedSkills, []map[string]any{wantRow}) {
		t.Fatalf("exit %d, %v:\n%s", code, err, out)
	}
	// --update-fork replaces a rule's fork, discarding its edits, so it points a forked skill to --merge-fork.
	out, diagnostic, code = u.run(t, "project", "update", "--yes", "--update-fork", "team:skills/build-feature")
	if code != 2 || !strings.Contains(diagnostic, "pass --merge-fork team:skills/build-feature") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	out, diagnostic, code = u.run(t, "project", "update", "--yes", "--merge-fork", "team:skills/build-feature")
	if code != 0 || diagnostic != "" || !strings.Contains(out, "The update merges these changes into your skill, keeping your edits, and sets\n            sources.team.skills.forks.skills/build-feature.basedOn to commit "+later[:12]+".\n") || !strings.Contains(out, "  Change: skills/build-feature/SKILL.md\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	if fork, err := os.ReadFile(filepath.Join(u.directory, ".trailguide/skills/build-feature/SKILL.md")); err != nil || !strings.Contains(string(fork), "Run make.") {
		t.Fatalf("merged fork, %v:\n%s", err, fork)
	}
	if config, err := os.ReadFile(filepath.Join(u.directory, ".trailguide/config.yaml")); err != nil || !strings.Contains(string(config), `basedOn: "`+later+`"`) {
		t.Fatalf("config, %v:\n%s", err, config)
	}
	if out, diagnostic, code := u.run(t, "project", "check"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
}

// TestForkSkill_ForksASkillItCantAdaptAsADraftToFinish forks a skill whose frontmatter Trail Guide can't rewrite,
// marks the fork as a draft, lists what to fix, and build refuses the fork until the author finishes it.
func TestForkSkill_ForksASkillItCantAdaptAsADraftToFinish(t *testing.T) {
	ship := "---\nname: ship\ndescription: Ship a feature.\nmetadata: {trailguide-skills-used: \"skills/build-feature\"}\n---\n# Ship\n"
	u, _ := newSkillForkFixtureWith(t, map[string][]byte{".trailguide/skills/ship/SKILL.md": []byte(ship)})
	out, diagnostic, code := u.run(t, "project", "add", "skill", "--from", "team:skills/ship", "--reason", "We ship with make.", "--non-interactive")
	for _, want := range []string{
		"Until you finish it, trailguide project build refuses it.\nIn the fork:\n",
		"  - Name the skill ship, and cite the source's skills and rules with the source's name, such as team:skills/tdd;",
		"Then remove the <!-- trailguide:draft --> line.",
	} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("exit %d, want %q:\n%s%s", code, want, out, diagnostic)
		}
	}
	j, _ := newSkillForkFixtureWith(t, map[string][]byte{".trailguide/skills/ship/SKILL.md": []byte(ship)})
	out, diagnostic, code = j.run(t, "project", "add", "skill", "--from", "team:skills/ship", "--reason", "We ship with make.", "--json")
	var result struct {
		OK    bool
		Value struct {
			NextSteps []nextStep `json:"nextSteps"`
		}
	}
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.OK || len(result.Value.NextSteps) == 0 || !strings.Contains(result.Value.NextSteps[0].Instruction, "Then remove the <!-- trailguide:draft --> line.") {
		t.Fatalf("exit %d, want the fixes as the first next step:\n%s%s", code, out, diagnostic)
	}
	file := filepath.Join(u.directory, ".trailguide/skills/ship/SKILL.md")
	if skill, err := os.ReadFile(file); err != nil || !strings.HasSuffix(string(skill), "---\n<!-- trailguide:draft -->\n# Ship\n") {
		t.Fatalf("forked SKILL.md:\n%s, %v; want the source's, marked as a draft", skill, err)
	}
	if out, diagnostic, code := u.run(t, "project", "build"); code == 0 {
		t.Fatalf("build installed an unfinished fork:\n%s%s", out, diagnostic)
	}
	finished := "---\nname: ship\ndescription: Ship a feature.\nmetadata:\n  trailguide-skills-used: |\n    team:skills/build-feature\n---\n# Ship\n"
	if err := os.WriteFile(file, []byte(finished), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, diagnostic, code := u.run(t, "project", "build"); code != 0 {
		t.Fatalf("build refused the finished fork: exit %d:\n%s%s", code, out, diagnostic)
	}
}

// TestForkSkill_RejectsMissingAndExtraInputs exits 2 without writing, explaining what to pass instead.
func TestForkSkill_RejectsMissingAndExtraInputs(t *testing.T) {
	u, _ := newSkillForkFixture(t)
	before := projectFileContents(t, u.directory)
	for _, test := range []struct {
		args    []string
		message string
	}{
		{[]string{"--reason", "Ours."}, "missing skill name: name the skill to create, such as release-checklist, or fork a skill with --from SOURCE:SKILL."},
		{[]string{"build-feature", "--from", "team:skills/build-feature"}, "trailguide project add skill takes a skill name or --from, not both: a fork keeps the source skill's name, so leave out build-feature."},
		{[]string{"--from", "team:skills/build-feature", "--description", "Ours.", "--reason", "Ours."}, "--description doesn't apply to a fork, which keeps the source skill's frontmatter"},
		{[]string{"--from", "skills/build-feature", "--reason", "Ours."}, `"skills/build-feature": expected SOURCE:SKILL`},
		{[]string{"--from", "team:skills/build-feature"}, "--reason is required: the fork replaces team's skills/build-feature, and its skills.forks entry records why"},
	} {
		out, diagnostic, code := u.run(t, append([]string{"project", "add", "skill", "--non-interactive"}, test.args...)...)
		if code != 2 || out != "" || !strings.Contains(diagnostic, test.message) {
			t.Errorf("%v: exit %d, stdout %q, stderr:\n%s\nwant %q", test.args, code, out, diagnostic, test.message)
		}
	}
	if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a refused fork changed the project")
	}
}

// TestForkSkill_PointsLinksAtInstalledSkillsAndListsTheOthers forks a skill that links to a sibling skill the project
// installs and to a shared document: the fork's link to the sibling reaches its installed copy, and the link to the
// document is a fix to finish, after which sync installs the fork and drops the document.
func TestForkSkill_PointsLinksAtInstalledSkillsAndListsTheOthers(t *testing.T) {
	u, _ := newSkillForkFixtureWith(t, map[string][]byte{
		".trailguide/skills/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n\nCompare with [build](../build-feature/SKILL.md) and read the [guide](../../../docs/guide.md).\n"),
		"docs/guide.md":                      []byte("# Guide\n"),
	})
	out, diagnostic, code := u.run(t, "project", "add", "skill", "--from", "team:skills/review", "--reason", "Ours.", "--non-interactive")
	if code != 0 || diagnostic != "" {
		t.Fatal(code, out, diagnostic)
	}
	for _, want := range []string{
		"  - In SKILL.md, replace the link to ../../../docs/guide.md. It reaches docs/guide.md in team, which is outside the skill and every skill the project installs into its skill targets, so the fork can't reach it.",
		"Then install the fork in place of the source's skill. Sync, unlike build, also drops the files\nonly the source's skill linked to, from the revision .trailguide/lock.json records:\n  trailguide project sync\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout lacks %q:\n%s", want, out)
		}
	}
	data, err := os.ReadFile(filepath.Join(u.directory, ".trailguide/skills/review/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	fork := string(data)
	if !strings.Contains(fork, "[build](../build-feature/SKILL.md)") {
		t.Fatalf("fork:\n%s", fork)
	}
	u.write(t, "skills/review/SKILL.md", strings.NewReplacer(" and read the [guide](../../../docs/guide.md)", "", rules.DraftMarker+"\n", "").Replace(fork))
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	installed, err := os.ReadFile(filepath.Join(u.directory, ".claude/skills/review/SKILL.md"))
	if err != nil || !strings.Contains(string(installed), "[build](../build-feature/SKILL.md)") {
		t.Fatalf("installed fork: %v\n%s", err, installed)
	}
	if _, err := os.Stat(filepath.Join(u.directory, ".trailguide/_managed/vendor/team/_repository/docs/guide.md")); !os.IsNotExist(err) {
		t.Fatalf("sync kept the document only the source's skill linked to: %v", err)
	}
	if out, diagnostic, code := u.run(t, "project", "check"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
}

// TestForkSkill_ListsLinksToSiblingSkillsWithoutSkillTargets forks a skill that links to a sibling skill in a
// project without skill targets, where the sibling has no installed copy to link to, so the link is a fix to finish.
func TestForkSkill_ListsLinksToSiblingSkillsWithoutSkillTargets(t *testing.T) {
	u, _ := newSkillForkFixtureWith(t, map[string][]byte{
		".trailguide/skills/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n\nCompare with [build](../build-feature/SKILL.md).\n"),
	})
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: []\nsources:\n  team:\n    repository: "+u.fixture.Repository+"\n    ref: release/1\n    skills:\n      include: ['*']\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	out, diagnostic, code := u.run(t, "project", "add", "skill", "--from", "team:skills/review", "--reason", "Ours.", "--non-interactive")
	if want := "  - In SKILL.md, replace the link to ../build-feature/SKILL.md. It reaches .trailguide/skills/build-feature/SKILL.md in team"; code != 0 || !strings.Contains(out, want) {
		t.Fatalf("exit %d, stderr %q, stdout lacks %q:\n%s", code, diagnostic, want, out)
	}
}
