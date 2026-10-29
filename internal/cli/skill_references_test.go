// Test that the CLI delivers skills' linked files offline and reports a link it can't deliver in JSON.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

func TestSkillReferences_BuildAndCheckOfflineAndReportBrokenLinks(t *testing.T) {
	ctx := context.Background()
	f, err := gitfixture.New(ctx, gitfixture.Library(map[string][]byte{
		"config.yaml":                          []byte(`{"schemaVersion":2,"sources":{},"library":{"schemaVersion":1}}`),
		".trailguide/skills/attack/SKILL.md":   []byte("---\nname: attack\ndescription: Attack.\n---\nCompare [Redesign](../redesign/SKILL.md).\n"),
		".trailguide/skills/redesign/SKILL.md": []byte("---\nname: redesign\ndescription: Redesign.\n---\nSee [the steps](steps.md).\n"),
		".trailguide/skills/redesign/steps.md": []byte("# Steps\n"),
		".trailguide/skills/broken/SKILL.md":   []byte("---\nname: broken\ndescription: Broken.\n---\nSee [Redesign](../redesign/SKILL.md) and [gone](../gone/SKILL.md).\n"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.Release(ctx, 1, "schemaVersion: 1\nrelease: 1\nrules: {}\nchanges: {}\n"); err != nil {
		t.Fatal(err)
	}
	binary := buildCLI(t)
	directory := t.TempDir()
	if out, diagnostic, code := runCLI(t, binary, directory, "project", "init"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	configure := func(skill string) {
		config := `{"schemaVersion":2,"skillTargets":["claude"],"sources":{"team":{"repository":"` + f.Repository + `","ref":"release/1","skills":{"include":["skills/` + skill + `"]}}}}`
		if err := os.WriteFile(filepath.Join(directory, ".trailguide", "config.yaml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configure("attack")
	if out, diagnostic, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync"); code != 0 {
		t.Fatal(code, out, diagnostic)
	}
	// Without Git or anything else on PATH, build and check can only use what the project retained.
	for _, args := range [][]string{{"project", "build", "--json"}, {"project", "check", "--json"}} {
		if out, diagnostic, code := runCLI(t, binary, directory, args...); code != 0 {
			t.Fatalf("%v: exit %d\n%s\n%s", args, code, out, diagnostic)
		}
	}
	installed, err := os.ReadFile(filepath.Join(directory, ".claude/skills/attack/SKILL.md"))
	if err != nil || !strings.Contains(string(installed), "[Redesign](../../../.trailguide/_managed/generated/references/team/skills/redesign/SKILL.md)") {
		t.Fatalf("installed attack:\n%s\n%v", installed, err)
	}
	configure("broken")
	out, _, code := runCLIWithEnvironment(t, binary, directory, f.Environment, "project", "sync", "--json")
	var result response
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(out, err)
	}
	if code != 1 || result.Error == nil || result.Error.Code != "reference-missing" || result.Error.Kind != "validation" || result.Error.Location != "sources.team: .trailguide/skills/broken/SKILL.md" || !strings.Contains(result.Error.Message, "links to ../gone/SKILL.md (.trailguide/skills/gone/SKILL.md) at commit ") {
		t.Fatalf("exit %d, response %s", code, out)
	}
	if reference := result.Error.Reference; reference == nil || reference.Destination != "../gone/SKILL.md" || reference.Target != ".trailguide/skills/gone/SKILL.md" || len(reference.Chain) != 1 {
		t.Fatalf("reference = %+v", result.Error.Reference)
	}
	if _, err := os.Stat(filepath.Join(directory, ".claude/skills/attack/SKILL.md")); err != nil {
		t.Fatalf("the failed sync removed the working installation: %v", err)
	}
}
