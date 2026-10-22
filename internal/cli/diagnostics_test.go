// Verify human error visibility through the native executable and real terminals.

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/terminalfixture"
)

// TestHumanErrorDisplay keeps errors visible in terminals and readable in redirected output.
func TestHumanErrorDisplay(t *testing.T) {
	binary := buildCLI(t)
	for _, tc := range []struct {
		name     string
		terminal terminalfixture.Terminal
		color    bool
	}{
		{"color", terminalfixture.Terminal{Type: "xterm-256color"}, true},
		{"no-color", terminalfixture.Terminal{Type: "xterm-256color", NoColor: true}, false},
		{"dumb", terminalfixture.Terminal{Type: "dumb"}, false},
		{"unknown", terminalfixture.Terminal{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Redirected output stays plain even when the environment declares a color terminal.
			t.Setenv("TERM", tc.terminal.Type)
			t.Setenv("NO_COLOR", "")
			result, err := tc.terminal.Run(context.Background(), binary, t.TempDir(), []string{"project", "add", "rule"}, nil)
			if err != nil || result.ExitCode != 2 {
				t.Fatal(err, result)
			}
			label := "Error:"
			if tc.color {
				label = "\x1b[1;31mError:\x1b[0m"
			}
			if !strings.HasPrefix(result.Transcript, label+" missing rule path") {
				t.Fatal(result.Transcript)
			}
			if !tc.color && strings.Contains(result.Transcript, "\x1b[") {
				t.Fatal(result.Transcript)
			}
			out, diagnostic, code := runCLI(t, binary, t.TempDir(), "project", "build")
			if code != 1 || out != "" || !strings.HasPrefix(diagnostic, "Error: ") || strings.Contains(diagnostic, "\x1b[") {
				t.Fatal(code, out, diagnostic)
			}
		})
	}
}
