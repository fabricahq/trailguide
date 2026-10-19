// Exercise forking a source's skill through the compiled CLI: output, JSON, usage errors, the build that installs the
// fork in the skill's place, and the update preview that lists the source's later changes to the skill.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// forkedSkill is the SKILL.md of the fixture library's build-feature skill at release/1.
const forkedSkill = "---\nname: build-feature\ndescription: Build a feature.\n---\n# Build a feature\n\n1. Run scripts/run.sh.\n"

// newSkillForkFixture syncs a project that installs every skill of a library whose release/1 publishes
// build-feature, with an executable script, and returns the fixture and release/1's commit.
func newSkillForkFixture(t *testing.T) (updateFixture, string) {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                                     []byte(`{"schemaVersion":2,"sources":{},"library":{}}`),
		"techs/go/_group.yaml":                            []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":                              updateRule("Wrap errors."),
		".trailguide/skills/build-feature/SKILL.md":       []byte(forkedSkill),
		".trailguide/skills/build-feature/scripts/run.sh": []byte("#!/bin/sh\necho build\n"),
	}))
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
	if err := f.Release(ctx, 1, "formatVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init", "--skill-targets", "claude"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: "+f.Repository+"\n    skills:\n      include: ['*']\n")
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
		"config.yaml now excludes team:skills/build-feature and names skills/build-feature as its replacement,\nbased on commit " + commit[:12] + "; trailguide project update lists the source's later changes to it.\n\n" +
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
	if err := u.fixture.Release(ctx, 2, "formatVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n"); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code = u.run(t, "project", "update")
	row := "  replaced  skills/build-feature  commit " + commit[:12] + " -> " + later[:12] + "\n" +
		"            Your skill: skills/build-feature, based on commit " + commit[:12] + ".\n" +
		"            Changed since commit " + commit[:12] + ", the copy your skill is based on:\n" +
		"              SKILL.md\n" +
		"            Compare them with your skill: the update doesn't change it.\n"
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
	wantRow := map[string]any{"id": "skills/build-feature", "localSkill": "skills/build-feature", "basedOn": commit, "from": commit, "to": later, "files": []any{"SKILL.md"}}
	if err := json.Unmarshal([]byte(out), &preview); err != nil || code != 0 || len(preview.Value.Sources) != 1 || !reflect.DeepEqual(preview.Value.Sources[0].ReplacedSkills, []map[string]any{wantRow}) {
		t.Fatalf("exit %d, %v:\n%s", code, err, out)
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
		{[]string{"--from", "team:skills/build-feature"}, "--reason is required: the fork replaces team's skills/build-feature, and the exclusion that makes it the replacement records why"},
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
