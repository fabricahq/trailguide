// Check option grouping through real commands, including flags whose meaning depends on scope.

package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCommandOptionSections(t *testing.T) {
	binary := buildCLI(t)
	directory := t.TempDir()
	for _, test := range []struct {
		path, title      string
		specific, common []string
	}{
		{"", "Command options", []string{"license"}, []string{"help", "json", "version"}},
		{"project", "", nil, []string{"help", "json"}},
		{"project add", "", nil, []string{"help", "json"}},
		{"project init", "Command options", []string{"skill-targets"}, []string{"help", "json", "non-interactive"}},
		{"project sync", "Command options", []string{"retake"}, []string{"help", "json"}},
		{"project update", "Command options", []string{"yes", "keep", "exclude", "reason"}, []string{"help", "json"}},
		{"project build", "", nil, []string{"help", "json"}},
		{"project check", "", nil, []string{"help", "json"}},
		{"project add source", "Source options", []string{"repository", "ref", "rules", "skills"}, []string{"help", "json", "non-interactive"}},
		{"project add rule", "Rule options", []string{"title", "when-to-read", "impact", "impact-description", "body-file", "from", "reason"}, []string{"help", "json", "non-interactive"}},
		{"library", "", nil, []string{"help", "json"}},
		{"library add", "", nil, []string{"help", "json"}},
		{"library init", "Library options", []string{"spdx", "license-file", "notice-file"}, []string{"directory", "help", "json", "non-interactive"}},
		{"library check", "", nil, []string{"directory", "help", "json", "non-interactive"}},
		{"library add rule", "Rule options", []string{"title", "when-to-read", "impact", "impact-description", "body-file"}, []string{"directory", "help", "json", "non-interactive"}},
	} {
		t.Run(test.path, func(t *testing.T) {
			for _, structured := range []bool{false, true} {
				args := append(strings.Fields(test.path), "--help")
				if structured {
					args = append([]string{"--json"}, args...)
				}
				out, diagnostic, code := runCLI(t, binary, directory, args...)
				if code != 0 || diagnostic != "" {
					t.Fatal(args, code, out, diagnostic)
				}
				if structured {
					var result struct {
						OK    bool
						Value struct{ Text string }
					}
					if err := json.Unmarshal([]byte(out), &result); err != nil || !result.OK {
						t.Fatal(err, out)
					}
					out = result.Value.Text
				}
				before, common, found := strings.Cut(out, "Common options:\n")
				if !found || strings.Contains(out, "Flags:") {
					t.Fatal("missing common options", out)
				}
				// Examples may repeat flags after the options section.
				common, _, _ = strings.Cut(common, "\n\n")
				specific := ""
				if test.title != "" {
					_, specific, found = strings.Cut(before, test.title+":\n")
					if !found {
						t.Fatal("missing command-specific options", out)
					}
				} else if strings.Contains(before, " options:") {
					t.Fatal("empty options section", out)
				}
				for _, flag := range test.specific {
					if strings.Count(specific, "--"+flag+" ") != 1 || strings.Contains(common, "--"+flag+" ") {
						t.Fatal("wrong specific option section", flag, out)
					}
				}
				for _, flag := range test.common {
					if strings.Count(common, "--"+flag+" ") != 1 || strings.Contains(specific, "--"+flag+" ") {
						t.Fatal("wrong common option section", flag, out)
					}
				}
				if strings.Contains(common, "--config ") || (strings.HasPrefix(test.path, "project") && strings.Contains(common, "--directory ")) {
					t.Fatal("options from another scope leaked", out)
				}
			}
		})
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("help changed files", entries, err)
	}
}
