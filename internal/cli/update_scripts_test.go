// Exercise how trailguide project update shows the scripts an update adds or changes in a skill, in human output,
// JSON, and a terminal, against libraries whose later releases change a skill's scripts.

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
	"github.com/fabricahq/trailguide/internal/test/terminalfixture"
)

// newScriptUpdateFixture syncs a project that installs the skill build-feature from a library's release/1, then
// publishes release/2, which moves techs/go/errors to 1.1.0 and, in build-feature, changes the executable run.sh
// and the reference notes.md, leaves the executable lint.sh as it was, and adds the executable setup.sh.
func newScriptUpdateFixture(t *testing.T) updateFixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`),
		"techs/go/_group.yaml": []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":   updateRule("errors 1.0.0"),
		".trailguide/skills/library/build-feature/SKILL.md":            []byte("---\nname: build-feature\ndescription: Build a feature from a spec.\n---\n# Build a feature\n"),
		".trailguide/skills/library/build-feature/scripts/run.sh":      []byte("#!/bin/sh\necho build\n"),
		".trailguide/skills/library/build-feature/scripts/lint.sh":     []byte("#!/bin/sh\necho lint\n"),
		".trailguide/skills/library/build-feature/references/notes.md": []byte("Notes.\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	chmodExecutable(t, f, ".trailguide/skills/library/build-feature/scripts/run.sh", ".trailguide/skills/library/build-feature/scripts/lint.sh")
	if _, err := f.Commit(ctx, f.Worktree(), "First release", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init", "--skill-targets", "claude"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: "+f.Repository+"\n    ref: release/1\n    rules:\n      include: [techs/go/*]\n    skills:\n      include: [skills/build-feature]\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Change the scripts", gitfixture.Library(map[string][]byte{
		"techs/go/errors.md": updateRule("errors 1.1.0"),
		".trailguide/skills/library/build-feature/scripts/run.sh":      []byte("#!/bin/sh\necho build everything\n"),
		".trailguide/skills/library/build-feature/scripts/setup.sh":    []byte("#!/bin/sh\necho setup\n"),
		".trailguide/skills/library/build-feature/references/notes.md": []byte("More notes.\n"),
	})); err != nil {
		t.Fatal(err)
	}
	chmodExecutable(t, f, ".trailguide/skills/library/build-feature/scripts/setup.sh")
	if _, err := f.Commit(ctx, f.Worktree(), "Make setup.sh executable", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Wrap errors.]}\n"); err != nil {
		t.Fatal(err)
	}
	return u
}

// chmodExecutable sets the executable bits of names, paths in the fixture's working tree.
func chmodExecutable(t *testing.T, f *gitfixture.Fixture, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.Chmod(filepath.Join(f.Worktree(), filepath.FromSlash(name)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// scriptUpdatePreview is how newScriptUpdateFixture's update previews its changes, before its closing advice.
const scriptUpdatePreview = "team\n" +
	"  minor     techs/go/errors  1.0.0 -> 1.1.0\n" +
	"            Wrap errors.\n" +
	"  changed   skills/build-feature\n" +
	"            Changed: references/notes.md, scripts/run.sh, scripts/setup.sh\n" +
	"            script changed: scripts/run.sh\n" +
	"            script added: scripts/setup.sh\n" +
	"  Shared files: release 1 -> 2\n" +
	"  Sets sources.team.ref to release/2 in .trailguide/config.yaml.\n" +
	"\n" +
	"Warning: some skills in this update add or change scripts. Trail Guide never runs them, but your agents may; read them before you confirm.\n"

func TestUpdate_PreviewFlagsEachScriptAndWarns(t *testing.T) {
	u := newScriptUpdateFixture(t)
	out, diagnostic, code := u.run(t, "project", "update")
	want := scriptUpdatePreview + "\nThis is a preview; no files were written. To apply it, run the command again\nwith --yes, or in a terminal to answer each question and confirm.\n"
	if code != 0 || diagnostic != "" || out != want {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, diagnostic, out, want)
	}
}

// TestUpdate_JSONReportsEachScriptAndTheWarning previews, then applies with --yes, which reports the scripts and
// warns again.
func TestUpdate_JSONReportsEachScriptAndTheWarning(t *testing.T) {
	u := newScriptUpdateFixture(t)
	wantSkills := []string{`{"id":"skills/build-feature","change":"changed","files":["references/notes.md","scripts/run.sh","scripts/setup.sh"],"scripts":[{"path":"scripts/run.sh","change":"changed"},{"path":"scripts/setup.sh","change":"added"}]}`}
	for _, run := range []struct {
		args    []string
		applied bool
		warning string
	}{
		{[]string{"project", "update", "--json"}, false, "some skills in this update add or change scripts. Trail Guide never runs them, but your agents may; read them before you confirm."},
		{[]string{"project", "update", "--json", "--yes"}, true, "some skills in this update added or changed scripts. Trail Guide never runs them, but your agents may; read them before you commit the update."},
	} {
		out, diagnostic, code := u.run(t, run.args...)
		var result struct {
			OK    bool
			Value struct {
				Applied bool
				Sources []struct {
					Skills []json.RawMessage
				}
				Warnings []string
			}
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || diagnostic != "" || !result.OK || result.Value.Applied != run.applied || len(result.Value.Sources) != 1 {
			t.Fatalf("%v: exit %d, %v:\n%s%s", run.args, code, err, out, diagnostic)
		}
		got := []string{}
		for _, row := range result.Value.Sources[0].Skills {
			got = append(got, compactJSON(t, row))
		}
		if !reflect.DeepEqual(got, wantSkills) || !reflect.DeepEqual(result.Value.Warnings, []string{run.warning}) {
			t.Fatalf("%v: skills:\n%s\nwarnings %q", run.args, strings.Join(got, "\n"), result.Value.Warnings)
		}
	}
	info, err := os.Stat(filepath.Join(u.directory, ".claude", "skills", "build-feature", "scripts", "setup.sh"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("installed setup.sh: %v, %v; want it executable", info, err)
	}
}

// TestUpdate_WarnsAboutScriptsBeforeAskingInATerminal declines the update after reading the warning.
func TestUpdate_WarnsAboutScriptsBeforeAskingInATerminal(t *testing.T) {
	u := newScriptUpdateFixture(t)
	before := projectFileContents(t, u.directory)
	result, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, []string{"project", "update"}, []terminalfixture.Step{{Prompt: "Apply the update? [yes/no]:", Answer: "no"}})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
	}
	transcript := strings.ReplaceAll(result.Transcript, "\r\n", "\n")
	if beforeQuestion, _, asked := strings.Cut(transcript, "Apply the update?"); !asked || !strings.Contains(beforeQuestion, scriptUpdatePreview) {
		t.Fatalf("the preview and its scripts warning didn't come before the confirmation:\n%s", result.Transcript)
	}
	if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a declined update changed the project")
	}
}

// newScriptRevisionFixture syncs a project that installs the skill build-feature from a library's release/1, then
// publishes release/2, a major change to techs/go/naming that adds the executable setup.sh to build-feature, and
// release/3, a major change to techs/go/errors that removes setup.sh again, so the skill matches release/1.
func newScriptRevisionFixture(t *testing.T) updateFixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`),
		"techs/go/_group.yaml": []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":   updateRule("errors 1.0.0"),
		"techs/go/naming.md":   updateRule("naming 1.0.0"),
		".trailguide/skills/library/build-feature/SKILL.md": []byte("---\nname: build-feature\ndescription: Build a feature from a spec.\n---\n# Build a feature\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if _, err := f.Commit(ctx, f.Worktree(), "First release", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\n  techs/go/naming: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n  techs/go/naming: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init", "--skill-targets", "claude"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nskillTargets: [claude]\nsources:\n  team:\n    repository: "+f.Repository+"\n    ref: release/1\n    rules:\n      include: [techs/go/*]\n    skills:\n      include: [skills/build-feature]\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Add a setup script", gitfixture.Library(map[string][]byte{
		"techs/go/naming.md": updateRule("naming 2.0.0"),
		".trailguide/skills/library/build-feature/scripts/setup.sh": []byte("#!/bin/sh\necho setup\n"),
	})); err != nil {
		t.Fatal(err)
	}
	chmodExecutable(t, f, ".trailguide/skills/library/build-feature/scripts/setup.sh")
	if _, err := f.Commit(ctx, f.Worktree(), "Make setup.sh executable", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.0.0\n  techs/go/naming: 2.0.0\nchanges:\n  techs/go/naming: {change: major, from: 1.0.0, summaries: [Rename everything.]}\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Remove the setup script", gitfixture.Library(map[string][]byte{
		"techs/go/errors.md": updateRule("errors 2.0.0"),
		".trailguide/skills/library/build-feature/scripts/setup.sh": nil,
	})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 3, "schemaVersion: 1\nrelease: 3\nrules:\n  techs/go/errors: 2.0.0\n  techs/go/naming: 2.0.0\nchanges:\n  techs/go/errors: {change: major, from: 1.0.0, summaries: [Wrap every error.]}\n"); err != nil {
		t.Fatal(err)
	}
	return u
}

// TestUpdate_ShowsTheScriptsAnswersChooseBeforeConfirming answers questions that move a scoped update's skills to
// another revision, and expects the terminal to show the skills and scripts warning of that revision, or that it
// changes no skills, before asking to apply the update.
func TestUpdate_ShowsTheScriptsAnswersChooseBeforeConfirming(t *testing.T) {
	keepErrors := "\nYour answers:\n" +
		"  Keep team:techs/go/errors at 1.0.0.\n" +
		"    Reason: Not yet.\n" +
		"\n" +
		"Your answers change the update's skills:\n" +
		"team\n" +
		"  changed   skills/build-feature\n" +
		"            Changed: scripts/setup.sh\n" +
		"            script added: scripts/setup.sh\n" +
		"\n" +
		"Warning: some skills in this update add or change scripts. Trail Guide never runs them, but your agents may; read them before you confirm.\n" +
		"\n"
	keepNaming := "\nYour answers:\n" +
		"  Keep team:techs/go/naming at 1.0.0.\n" +
		"    Reason: Not yet.\n" +
		"\n" +
		"Your answers change the update's skills:\n" +
		"team\n" +
		"  No skill changes.\n" +
		"\n"
	for _, run := range []struct {
		name string
		args []string
		// steps answer every question before the confirmation.
		steps []terminalfixture.Step
		// warned is whether the preview before the questions warns about scripts.
		warned bool
		want   string
	}{
		{"keeping errors adds a script", []string{"team:techs/go/errors", "team:techs/go/naming"}, []terminalfixture.Step{
			{Prompt: "Adopt it, or keep 1.0.0? [adopt/keep]:", Answer: "keep"},
			{Prompt: "Reason for keeping it:", Answer: "Not yet."},
			{Prompt: "Adopt it, or keep 1.0.0? [adopt/keep]:", Answer: "adopt"},
		}, false, keepErrors},
		{"keeping naming drops the script", []string{"team:techs/go/naming"}, []terminalfixture.Step{
			{Prompt: "Adopt it, or keep 1.0.0? [adopt/keep]:", Answer: "keep"},
			{Prompt: "Reason for keeping it:", Answer: "Not yet."},
		}, true, keepNaming},
	} {
		t.Run(run.name, func(t *testing.T) {
			u := newScriptRevisionFixture(t)
			before := projectFileContents(t, u.directory)
			steps := append(run.steps, terminalfixture.Step{Prompt: "Apply the update? [yes/no]:", Answer: "no"})
			result, err := terminalfixture.RunWithEnvironment(context.Background(), u.binary, u.directory, u.fixture.Environment, append([]string{"project", "update"}, run.args...), steps)
			if err != nil || result.ExitCode != 0 {
				t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
			}
			transcript := strings.ReplaceAll(result.Transcript, "\r\n", "\n")
			questions, _, _ := strings.Cut(transcript, "Your answers:\n")
			if strings.Contains(questions, "Warning: ") != run.warned {
				t.Fatalf("the preview before the questions warned %v, want %v:\n%s", !run.warned, run.warned, result.Transcript)
			}
			last := run.steps[len(run.steps)-1]
			_, answered, _ := strings.Cut(transcript[strings.LastIndex(transcript, last.Prompt):], last.Answer+"\n")
			if confirmation, _, asked := strings.Cut(answered, "Apply the update? [yes/no]:"); !asked || confirmation != run.want {
				t.Fatalf("before the confirmation:\n%s\nwant:\n%s", confirmation, run.want)
			}
			if after := projectFileContents(t, u.directory); !reflect.DeepEqual(before, after) {
				t.Fatal("a declined update changed the project")
			}
		})
	}
}
