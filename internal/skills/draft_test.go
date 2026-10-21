// Test that a new skill's draft SKILL.md reads back with its frontmatter, and the limits on its description.

package skills_test

import (
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/skills"
)

func TestDraftReadsBackWithItsFrontmatterAndMarker(t *testing.T) {
	description := "Fix a bug: prove it with a test, even when the description runs well past the eighty characters YAML wraps at."
	data, err := skills.Draft("fix-bug", description, "review", "<!-- marker -->")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := skills.ReadMetadata(data, "SKILL.md")
	if err != nil || metadata.Name != "fix-bug" || metadata.Description != description || metadata.Type != "review" || len(skills.Check("fix-bug", metadata, false)) > 0 {
		t.Fatalf("%+v, %v:\n%s", metadata, err, data)
	}
	if !strings.Contains(string(data), "\n---\n\n<!-- marker -->\n") || strings.Count(string(data), "\n") > 30 {
		t.Fatalf("draft body:\n%s", data)
	}
	if untyped, err := skills.Draft("fix-bug", description, "", "<!-- marker -->"); err != nil || strings.Contains(string(untyped), "metadata:") {
		t.Fatalf("%v:\n%s", err, untyped)
	}
}

func TestValidateDescriptionCountsCharactersNotBytes(t *testing.T) {
	for description, valid := range map[string]bool{
		"":                          false,
		" \t\n":                     false,
		strings.Repeat("é", 1024):   true,
		strings.Repeat("é", 1025):   false,
		"Fix a bug. Use for a bug.": true,
	} {
		if err := skills.ValidateDescription(description); (err == nil) != valid {
			t.Errorf("%d characters: %v, want valid %v", len([]rune(description)), err, valid)
		}
	}
}
