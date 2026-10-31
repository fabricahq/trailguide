// Verify copyable website examples through the same resolver and renderer used by the CLI.

package build_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/build"
	"github.com/fabricahq/trailguide/internal/rules"
)

// TestDocumentationRules rejects documentation drift that would make copied examples fail a real build.
func TestDocumentationRules(t *testing.T) {
	// The homepage illustration is explicitly abbreviated; only complete walkthrough rules are copyable.
	for _, tc := range []struct{ file, pattern, title string }{
		{"../../_docs/src/content/docs/guides/write-rules.md", "(?s)```md\\n(---.*?)\\n```", "Verify retry limits"},
		{"../../_docs/src/content/docs/start-here/set-up-project.md", "(?s)```md\\n(---.*?)\\n```", "Test changed behavior"},
		{"../../_docs/src/content/docs/start-here/create-library.md", "(?s)```md\\n(---.*?)\\n```", "Make errors actionable"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if tc.pattern != "" {
				match := regexp.MustCompile(tc.pattern).FindSubmatch(data)
				if len(match) != 2 {
					t.Fatal("missing copyable rule example")
				}
				data = match[1]
			}
			config, err := rules.ParseConfiguration([]byte(`{"schemaVersion":2,"sources":{}}`))
			if err != nil {
				t.Fatal(err)
			}
			output, err := build.Generate(config, nil, map[string][]byte{
				"practices/testing/_group.yaml": []byte(`{"name":"Testing","description":"Verify behavior.","whenToRead":"Changing behavior."}`),
				"practices/testing/example.md":  data,
			}, build.Options{IndexMaxLines: 750})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(output.Files["rules/local/practices/testing/example.md"]), tc.title) {
				t.Fatal("missing rendered rule")
			}
		})
	}
}
