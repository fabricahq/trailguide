// Keep every published copy of the AGENTS.md section identical to the section Trail Guide writes.

package project

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// agentInstructions matches the fenced "Skills and Rules" section that pages quote from AGENTS.md.
var agentInstructions = regexp.MustCompile("(?ms)^```markdown\\n(## Skills and Rules\\n.*?)^```$")

// TestAgentInstructions_MatchTheSection fails when the docs drift from the section Trail Guide writes.
func TestAgentInstructions_MatchTheSection(t *testing.T) {
	section := renderAgentsSection(".trailguide", "")
	lines := strings.SplitAfter(section, "\n")
	want := strings.Join(lines[1:len(lines)-2], "")
	for _, name := range []string{
		"../../_docs/src/content/docs/start-here/set-up-project.md",
		"../../_docs/src/content/docs/for-agents/index.md",
	} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := agentInstructions.FindSubmatch(data); got == nil || string(got[1]) != want {
			t.Errorf("%s: Skills and Rules section differs from the AGENTS.md section:\n%s", name, got)
		}
	}
}

// TestRepositoryGuide_IsCurrent fails when this repository's own guide is edited by hand or lags the template, since init then refuses to refresh it.
func TestRepositoryGuide_IsCurrent(t *testing.T) {
	data, err := os.ReadFile("../../.trailguide/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !unmodifiedProjectGuide(data) {
		t.Fatal(".trailguide/README.md: checksum doesn't match its contents; move it aside, then regenerate it with trailguide project init")
	}
	if string(data) != string(renderProjectGuide()) {
		t.Fatal(".trailguide/README.md: differs from project-guide.md; regenerate it with trailguide project init")
	}
}
