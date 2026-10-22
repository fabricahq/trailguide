// Exercise how library change, check, and release report skill versions, through the compiled CLI and real Git history.

package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// TestLibrarySkillVersions_ReportsANewRecommendedSkillInCheckAndRelease records a new skill's note, previews it with
// its level in check, refuses a rule as a skill's replacement as a usage error, and shows the skill in a dry run's
// summary and release notes.
func TestLibrarySkillVersions_ReportsANewRecommendedSkillInCheckAndRelease(t *testing.T) {
	binary := buildCLI(t)
	ctx := context.Background()
	fixture, dir := releasedLibrary(t)
	if _, err := fixture.Commit(ctx, dir, "Add a skill", gitfixture.Library(map[string][]byte{
		"config.yaml":                        []byte("schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n  skills:\n    recommended: [skills/review]\n"),
		".trailguide/skills/review/SKILL.md": []byte("---\nname: review\ndescription: Review a change.\n---\n# Review\n"),
	})); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "change", "skills/review", "--retire", "--replaced-by", "practices/testing/a", "--summary", "Covered by a.")
	if code != 2 || !strings.Contains(diagnostic, "--replaced-by practices/testing/a is a rule, but skills/review is a skill. A skill can only be replaced by a skill") {
		t.Fatalf("exit %d, stdout %q, stderr:\n%s", code, out, diagnostic)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "change", "skills/review", "--summary", "Add a review playbook.")
	if code != 0 || !strings.Contains(out, "Next: Commit the note with the change it records, then validate the library:") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check")
	if code != 0 || diagnostic != "" || !strings.HasPrefix(out, "Library is valid: 1 group(s), 2 rule(s), 1 skill(s).\n") || !strings.Contains(out, "\nPending library release 2\n  skills/review  new  1.0.0, recommended\nLibrary-wide files changed since release/1:\n  config.yaml\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "check", "--json")
	var response struct {
		OK    bool
		Value map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil || code != 0 || diagnostic != "" || !response.OK {
		t.Fatal(err, code, out, diagnostic)
	}
	var pending struct{ Skills json.RawMessage }
	if err := json.Unmarshal(response.Value["pendingRelease"], &pending); err != nil || string(response.Value["skillCount"]) != "1" {
		t.Fatal(err, out)
	}
	if got, want := compactJSON(t, pending.Skills), `[{"id":"skills/review","change":"new","to":"1.0.0","summaries":["Add a review playbook."],"level":"recommended"}]`; got != want {
		t.Fatalf("pending skills %s, want %s", got, want)
	}
	if _, err := fixture.Commit(ctx, dir, "Record the skill", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.CommandIn(ctx, dir, "push", "--quiet", "origin", "main"); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, code = runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "release", "--dry-run", "--no-github-release")
	if code != 0 || diagnostic != "" || !strings.Contains(out, "\nRules:\n  No rule changes.\nSkills:\n  skills/review  new  1.0.0, recommended\nLibrary-wide files:\n  config.yaml\n") || !strings.Contains(out, "\nRelease notes:\n\nLibrary release 2 changes 1 skill: 1 new.\n\n## Skills\n\n- **skills/review** `1.0.0`, new, recommended\n  - Add a review playbook.\n") {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}

// TestLibraryAddSkill_NextStepsIncludeTheChangeNote after the first library release.
func TestLibraryAddSkill_NextStepsIncludeTheChangeNote(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, fixture.Environment, "library", "add", "skill", "review", "--description", "Review a change.", "--non-interactive")
	want := "add skills/review to library.skills. After the first library release, every new skill\nneeds a change note. After writing the steps, record it with a summary for project maintainers:\n  trailguide library change skills/review --summary '<what the skill does>'\n\nThen validate the library:\n  trailguide library check\n"
	if code != 0 || diagnostic != "" || !strings.HasSuffix(out, want) {
		t.Fatalf("exit %d, stderr %q, stdout:\n%s", code, diagnostic, out)
	}
}
