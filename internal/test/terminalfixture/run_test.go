// Check that a terminal run ends, with an error, when its prompt never appears, and that the child sees only the
// fixture's terminal settings.

package terminalfixture

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestRun_ReturnsWhenAPromptNeverAppearsAndTheProcessKeepsWriting keeps reading the terminal after giving up, so
// a process that writes more than the terminal buffers can still exit.
func TestRun_ReturnsWhenAPromptNeverAppearsAndTheProcessKeepsWriting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := RunWithEnvironment(ctx, "/bin/sh", t.TempDir(), os.Environ(), []string{"-c", "exec head -c 10000000 /dev/zero >&2"}, []Step{{Prompt: "never shown", Answer: "x"}})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "exceeds 2 MiB") {
			t.Fatalf("got %v, want the output limit", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the run never returned")
	}
}

// TestRun_SetsOnlyTheDeclaredTerminalSettings keeps a developer's color terminal from changing what a test sees.
func TestRun_SetsOnlyTheDeclaredTerminalSettings(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	for _, tc := range []struct {
		name     string
		terminal Terminal
		want     []string
	}{
		{"undeclared", Terminal{}, nil},
		{"declared", Terminal{Type: "dumb", NoColor: true}, []string{"TERM=dumb", "NO_COLOR=1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.terminal.Run(context.Background(), "/usr/bin/env", t.TempDir(), nil, nil)
			if err != nil {
				t.Fatal(err, result)
			}
			var got []string
			for _, item := range strings.Split(result.Stdout, "\n") {
				if strings.HasPrefix(item, "TERM=") || strings.HasPrefix(item, "NO_COLOR=") {
					got = append(got, item)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
