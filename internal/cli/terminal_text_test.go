// Check that human output escapes terminal control characters from libraries and other untrusted text.

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// TestTerminalText_EscapesControlCharactersAndKeepsNewlines covers each kind of character a terminal could act on.
func TestTerminalText_EscapesControlCharactersAndKeepsNewlines(t *testing.T) {
	for input, want := range map[string]string{
		"":                               "",
		"Plain text, é, and ✓.\nNext.\n": "Plain text, é, and ✓.\nNext.\n",
		"\x1b[2J\x1b[HNo risky changes.": `\x1b[2J\x1b[HNo risky changes.`,
		"a\rb\tc\x00d\x7fe":              `a\x0db\x09c\x00d\x7fe`,
		"\u009b2J\u0085":                 `\u009b2J\u0085`,
		"bad \xff\x9b byte":              `bad \xff\x9b byte`,
	} {
		if got := terminalText(input); got != want {
			t.Errorf("terminalText(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestUpdate_KeepsControlCharactersOffTheTerminal refuses a library release whose summary holds an escape sequence,
// and shows a diagnostic naming an argument with one as visible text, through the real binary, so a library can't
// clear or rewrite the preview people approve.
func TestUpdate_KeepsControlCharactersOffTheTerminal(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":2,"name":"library"}}`),
		"techs/go/_group.yaml": []byte(`{"name":"Go","description":"Go guidance.","whenToRead":"When writing Go."}`),
		"techs/go/errors.md":   updateRule("errors 1.0.0"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules:\n  techs/go/errors: 1.0.0\nchanges:\n  techs/go/errors: {change: new, summaries: [Add the rule.]}\n"); err != nil {
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
	if _, err := f.Commit(ctx, f.Worktree(), "Second release", gitfixture.Library(map[string][]byte{"techs/go/errors.md": updateRule("errors 1.1.0")})); err != nil {
		t.Fatal(err)
	}
	if err := f.Release(ctx, 2, "schemaVersion: 1\nrelease: 2\nrules:\n  techs/go/errors: 1.1.0\nchanges:\n  techs/go/errors: {change: minor, from: 1.0.0, summaries: [\"\\e[2J\\e[HNo risky changes.\\x9b\"]}\n"); err != nil {
		t.Fatal(err)
	}
	// A release record's summaries can't hold control characters, so the library release is refused.
	out, diagnostic, code := u.run(t, "project", "update")
	if code != 1 || strings.ContainsAny(out+diagnostic, "\x1b\u009b") || !strings.Contains(diagnostic, "release/2.changes.techs/go/errors.summaries[0]: expected text without control characters") {
		t.Fatalf("exit %d, stderr %q, stdout %q", code, diagnostic, out)
	}
	out, diagnostic, code = u.run(t, "project", "update", "team:techs/go/\x1b[2Jerrors")
	if code != 2 || out != "" || strings.Contains(diagnostic, "\x1b") || !strings.Contains(diagnostic, `team:techs/go/\x1b[2Jerrors`) {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, diagnostic)
	}
}
