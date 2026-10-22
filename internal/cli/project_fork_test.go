// Exercise forking a library rule through the compiled CLI: output, JSON, prompts, exit statuses, and the build
// that replaces the imported rule, against a real library with two library releases.

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

// forkFixture is a project synced at a library's release/1, which publishes techs/go/errors, linking to a shared
// asset, and practices/testing/verify, which the project doesn't import. Its release/2 moves errors to 1.1.0.
type forkFixture struct {
	updateFixture
}

// newForkFixture syncs a project whose source team selects techs/go, then publishes release/2.
func newForkFixture(t *testing.T) forkFixture {
	t.Helper()
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                   []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`),
		"techs/go/_group.yaml":          []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":            updateRule("Read [the guide](../../../assets/guide.md)."),
		"practices/testing/_group.yaml": []byte(`{"name":"Testing","description":"Tests.","whenToRead":"When testing."}`),
		"practices/testing/verify.md":   updateRule("Verify."),
		"assets/guide.md":               []byte("Guide.\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules:\n  practices/testing/verify: 1.0.0\n  techs/go/errors: 1.0.0\nchanges:\n  practices/testing/verify: {change: new, summaries: [Add the rule.]}\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
		t.Fatal(err)
	}
	u := updateFixture{binary: buildCLI(t), directory: t.TempDir(), fixture: f}
	if out, diagnostic, code := runCLI(t, u.binary, u.directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	u.write(t, "config.yaml", "schemaVersion: 2\nsources:\n  team:\n    repository: "+f.Repository+"\n    ref: release/1\n    rules:\n      include:\n        - techs/go/*\n")
	if out, diagnostic, code := u.run(t, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(map[string][]byte{"techs/go/errors.md": updateRule("Wrap errors.")})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  practices/testing/verify: 1.0.0\n  techs/go/errors: 1.1.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [Add wrapping.]}\n"); err != nil {
		t.Fatal(err)
	}
	return forkFixture{u}
}

// read returns a file of the project's Trail Guide directory, or nil when it's missing.
func (f forkFixture) read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.directory, ".trailguide", name))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return data
}

// TestForkRule_ReplacesTheImportedRuleInTheBuild forks an older version of an imported rule, reports what it
// wrote and what to run next, and the next build uses the fork instead of the imported rule.
func TestForkRule_ReplacesTheImportedRuleInTheBuild(t *testing.T) {
	f := newForkFixture(t)
	out, diagnostic, code := f.run(t, "project", "add", "rule", "techs/go/errors", "--from", "team@1.0.0", "--reason", "Our wording.", "--non-interactive")
	if code != 0 || diagnostic != "" {
		t.Fatalf("exit %d:\n%s%s", code, out, diagnostic)
	}
	want := "Rule forked from team 1.0.0, published in library release 1.\nAdded:\n  .trailguide/rules/techs/go/assets/errors/guide.md\n  .trailguide/rules/techs/go/errors.md\nChanged:\n  .trailguide/config.yaml\n\n" +
		"Next: Edit the forked rule to change what it says. A fork has no version: it changes\n" +
		"only when you edit it. The library's license still applies to the copied text.\n" +
		"config.yaml now excludes team's techs/go/errors and names the fork as its replacement,\n" +
		"based on 1.0.0; trailguide project update lists the library's later changes to it.\n\n" +
		"Then rebuild this project's guidance:\n  trailguide project build\n  trailguide project check\n"
	if out != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out, want)
	}
	if got := string(f.read(t, "rules/techs/go/errors.md")); got != string(updateRule("Read [the guide](assets/errors/guide.md).")) {
		t.Fatalf("fork %q", got)
	}
	if !strings.Contains(string(f.read(t, "config.yaml")), "exclude:\n        - id: techs/go/errors\n          reason: Our wording.\n          replacedBy: techs/go/errors\n          basedOn: \"1.0.0\"\n") {
		t.Fatalf("configuration:\n%s", f.read(t, "config.yaml"))
	}
	for _, command := range [][]string{{"project", "build"}, {"project", "check"}} {
		if out, diagnostic, code := f.run(t, command...); code != 0 {
			t.Fatalf("%v: exit %d:\n%s%s", command, code, out, diagnostic)
		}
	}
	if f.read(t, "_managed/generated/rules/local/techs/go/errors.md") == nil || f.read(t, "_managed/generated/rules/team/techs/go/errors.md") != nil {
		t.Fatal("the build doesn't replace the imported rule with the fork")
	}
}

// TestForkRule_ReportsJSON forks a rule the project doesn't import, which needs no reason, and reports failures'
// codes and kinds.
func TestForkRule_ReportsJSON(t *testing.T) {
	f := newForkFixture(t)
	type response struct {
		OK    bool
		Value struct {
			Added     []string
			Changed   []string
			NextSteps []nextStep
		}
		Error struct{ Kind, Code, Location string }
	}
	run := func(want int, args ...string) response {
		t.Helper()
		out, diagnostic, code := f.run(t, append([]string{"--json", "project", "add", "rule"}, args...)...)
		var result response
		if code != want || diagnostic != "" || json.Unmarshal([]byte(out), &result) != nil {
			t.Fatalf("%v: exit %d, want %d:\n%s%s", args, code, want, out, diagnostic)
		}
		return result
	}
	config := string(f.read(t, "config.yaml"))
	forked := run(0, "practices/testing/verify", "--from", "team@1.0.0")
	steps := []nextStep{{Instruction: "Next: Edit the forked rule to change what it says. A fork has no version: it changes\nonly when you edit it. The library's license still applies to the copied text.", Commands: []string{}}, {Instruction: "Then rebuild this project's guidance:", Commands: []string{"trailguide project build", "trailguide project check"}}}
	if !forked.OK || len(forked.Value.Added) != 3 || len(forked.Value.Changed) != 0 || !reflect.DeepEqual(forked.Value.NextSteps, steps) || string(f.read(t, "config.yaml")) != config {
		t.Fatalf("fork %+v, configuration:\n%s", forked, f.read(t, "config.yaml"))
	}
	for _, test := range []struct {
		args                 []string
		exit                 int
		kind, code, location string
	}{
		{[]string{"techs/go/errors", "--from", "team@1.2.0", "--reason", "Ours."}, 1, "operation", "version-not-found", ""},
		{[]string{"practices/testing/verify", "--from", "team@1.0.0"}, 1, "operation", "already-exists", ""},
		{[]string{"techs/go/errors", "--from", "team@1.0.0"}, 2, "usage", "invalid-arguments", ""},
		{[]string{"techs/go/errors", "--from", "other@1.0.0", "--reason", "Ours."}, 1, "validation", "", "--from"},
	} {
		failed := run(test.exit, test.args...)
		if failed.OK || failed.Error.Kind != test.kind || failed.Error.Code != test.code || failed.Error.Location != test.location {
			t.Errorf("%v: %+v", test.args, failed.Error)
		}
	}
}

// TestForkRule_RejectsUsage exits 2 without writing for options a fork doesn't take and missing or misplaced
// reasons.
func TestForkRule_RejectsUsage(t *testing.T) {
	f := newForkFixture(t)
	before := projectFileContents(t, f.directory)
	for _, test := range []struct {
		args []string
		text string
	}{
		{[]string{"techs/go/errors", "--from", "team@1.0.0", "--reason", "Ours.", "--title", "Errors"}, "--title doesn't apply to a fork"},
		{[]string{"techs/go/errors", "--from", "team@1.0.0", "--reason", "Ours.", "--body-file", "body.md"}, "--body-file doesn't apply to a fork"},
		{[]string{"techs/go/errors", "--reason", "Ours."}, "--reason applies only to a fork"},
		{[]string{"techs/go/errors", "--from", "team"}, "expected LIBRARY@VERSION"},
		{[]string{"techs/go/errors", "--from", ""}, "--from"},
		{[]string{"techs/go/errors", "--from=", "--reason", "Ours."}, "--from"},
		{[]string{"techs/go/errors", "--from", "team@1.0"}, "--from"},
		{[]string{"techs/go/errors", "--from", "team@1.0.0", "--non-interactive"}, "--reason is required: the project imports techs/go/errors from team"},
		{[]string{"practices/testing/verify", "--from", "team@1.0.0", "--reason", "Ours."}, "no source imports practices/testing/verify from team"},
	} {
		out, diagnostic, code := f.run(t, append([]string{"project", "add", "rule"}, test.args...)...)
		if code != 2 || !strings.Contains(diagnostic, test.text) {
			t.Errorf("%v: exit %d, want 2 with %q:\n%s%s", test.args, code, test.text, out, diagnostic)
		}
	}
	if after := projectFileContents(t, f.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("a rejected fork changed the project")
	}
}

// TestForkRule_AsksForTheReasonInATerminal asks again after a blank answer and records the reason; ending input
// writes nothing.
func TestForkRule_AsksForTheReasonInATerminal(t *testing.T) {
	f := newForkFixture(t)
	before := projectFileContents(t, f.directory)
	args := []string{"project", "add", "rule", "techs/go/errors", "--from", "team@1.1.0"}
	ended, err := terminalfixture.RunWithEnvironment(context.Background(), f.binary, f.directory, f.fixture.Environment, args, []terminalfixture.Step{{Prompt: "Why use the fork instead of the imported rule?", EOF: true}})
	if err != nil || ended.ExitCode != 2 {
		t.Fatalf("exit %d, %v\n%s", ended.ExitCode, err, ended.Transcript)
	}
	if after := projectFileContents(t, f.directory); !reflect.DeepEqual(before, after) {
		t.Fatal("ending input changed the project")
	}
	steps := []terminalfixture.Step{
		{Prompt: "The project imports this rule from team, so the fork replaces it: config.yaml\r\ngets an exclusion that names the fork and records your reason.\r\n\r\nWhy use the fork instead of the imported rule?", Answer: ""},
		{Prompt: "provide a value for --reason", Answer: "Our wording."},
	}
	result, err := terminalfixture.RunWithEnvironment(context.Background(), f.binary, f.directory, f.fixture.Environment, args, steps)
	if err != nil || result.ExitCode != 0 || !strings.Contains(result.Stdout, "Rule forked from team 1.1.0, published in library release 2.") {
		t.Fatalf("exit %d, %v\n%s", result.ExitCode, err, result.Transcript)
	}
	if !strings.Contains(string(f.read(t, "config.yaml")), "reason: Our wording.\n          replacedBy: techs/go/errors\n") || string(f.read(t, "rules/techs/go/errors.md")) != string(updateRule("Wrap errors.")) {
		t.Fatalf("configuration:\n%s", f.read(t, "config.yaml"))
	}
}
